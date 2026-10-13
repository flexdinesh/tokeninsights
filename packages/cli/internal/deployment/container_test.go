package deployment_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
)

func dockerOutput(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %s (%v)", args[0], out, err)
	}
	return strings.TrimSpace(string(out))
}

func dockerCleanup(t *testing.T, args ...string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = exec.CommandContext(ctx, "docker", args...).Run()
	})
}

func TestContainerDeployment(t *testing.T) {
	image := os.Getenv("TOKENINSIGHTS_TEST_SERVER_IMAGE")
	if image == "" {
		t.Skip("run pnpm test:container to build and test the server image")
	}
	pgImage := os.Getenv("TOKENINSIGHTS_TEST_POSTGRES_IMAGE")
	if pgImage == "" {
		t.Fatal("missing PostgreSQL test image")
	}
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			name := "ti-container-" + strings.ToLower(rand.Text())
			network, volume := name+"-net", name+"-data"
			dockerOutput(t, "network", "create", network)
			dockerCleanup(t, "network", "rm", network)
			dockerOutput(t, "volume", "create", volume)
			dockerCleanup(t, "volume", "rm", volume)
			args := []string{"run", "--detach", "--name", name, "--network", network, "--read-only",
				"--health-interval", "1s", "--health-start-period", "0s",
				"--tmpfs", "/run/tokeninsights:uid=10001,gid=10001,mode=0700", "--tmpfs", "/tmp:uid=10001,gid=10001,mode=0700",
				"--publish", "127.0.0.1::8767", "--env", "TOKENINSIGHTS_SERVER_PORT=8767",
				"--env", "TOKENINSIGHTS_BOOTSTRAP_USERNAME=admin", "--env", "TOKENINSIGHTS_BOOTSTRAP_PASSWORD_FILE=/run/secrets/bootstrap_password", "--env", "TOKENINSIGHTS_STORAGE_BACKEND=" + backend}
			passwordFile := filepath.Join(t.TempDir(), "bootstrap-password")
			if err := os.WriteFile(passwordFile, []byte(containerBootstrapPassword+"\n"), 0o444); err != nil {
				t.Fatal(err)
			}
			args = append(args, "--mount", "type=bind,source="+passwordFile+",target=/run/secrets/bootstrap_password,readonly")
			if backend == "sqlite" {
				args = append(args, "--mount", "type=volume,source="+volume+",target=/data", "--env", "TOKENINSIGHTS_SERVER_DB_PATH=/data/server.sqlite")
			} else {
				pg := name + "-pg"
				dockerCleanup(t, "rm", "-f", "-v", pg)
				dockerOutput(t, "run", "--detach", "--name", pg, "--network", network, "--network-alias", "database", "--env", "POSTGRES_PASSWORD=container-contract", pgImage)
				deadline := time.Now().Add(30 * time.Second)
				for exec.CommandContext(t.Context(), "docker", "exec", pg, "pg_isready", "-h", "127.0.0.1", "-U", "postgres").Run() != nil {
					if time.Now().After(deadline) {
						t.Fatal("PostgreSQL startup timed out")
					}
					time.Sleep(100 * time.Millisecond)
				}
				secret := filepath.Join(t.TempDir(), "dsn")
				// Synthetic fixture secret must be readable by the image's non-root UID.
				if err := os.WriteFile(secret, []byte("postgres://postgres:container-contract@database:5432/postgres?sslmode=disable\n"), 0o444); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--mount", "type=bind,source="+secret+",target=/run/secrets/postgres_dsn,readonly", "--env", "TOKENINSIGHTS_POSTGRES_DSN_FILE=/run/secrets/postgres_dsn")
			}
			args = append(args, image)
			dockerCleanup(t, "rm", "-f", "-v", name)
			start := func() string {
				dockerOutput(t, args...)
				address := dockerOutput(t, "port", name, "8767/tcp")
				target := "http://" + address
				deadline := time.Now().Add(30 * time.Second)
				for {
					response, err := deploymentHTTP.Get(target + "/readyz")
					if err == nil {
						_ = response.Body.Close()
						if response.StatusCode == 200 {
							break
						}
					}
					if time.Now().After(deadline) {
						t.Fatal("container readiness", dockerOutput(t, "logs", name))
					}
					time.Sleep(100 * time.Millisecond)
				}
				for dockerOutput(t, "inspect", "--format", "{{.State.Health.Status}}", name) != "healthy" {
					if time.Now().After(deadline) {
						t.Fatal("image healthcheck never became healthy")
					}
					time.Sleep(100 * time.Millisecond)
				}
				if uid := dockerOutput(t, "exec", name, "id", "-u"); uid != "10001" {
					t.Fatal("server must be non-root", uid)
				}
				return target
			}
			stop := func() {
				dockerOutput(t, "stop", "--time", "20", name)
				if code := dockerOutput(t, "inspect", "--format", "{{.State.ExitCode}}", name); code != "0" {
					t.Fatal("unclean SIGTERM", code, dockerOutput(t, "logs", name))
				}
				dockerOutput(t, "rm", "-v", name)
			}
			target := start()
			cookie := verifyBootstrapBrowser(t, target, containerBootstrapPassword)
			var user accounts.User
			browserCall(t, target, cookie, http.MethodGet, "/api/v2/account", nil, http.StatusOK, &user)
			if user.UserID == "" || user.DatasetID == "" || user.Username != "admin" {
				t.Fatal("bootstrap account", user)
			}
			var token accounts.Token
			browserCall(t, target, cookie, http.MethodPost, "/api/v2/account/keys", map[string]interface{}{"label": "Container collector", "permissions": []string{accounts.Read, accounts.Ingest}}, http.StatusCreated, &token)
			if token.Secret == "" {
				t.Fatal("API key secret missing")
			}
			remoteTokens.Store(target, token.Secret)
			c := newClient(t)
			c.must(t, "config", "set", "distributed.server-url", target)
			c.sync(t)
			ids := assertUsage(t, target, 1, 120)
			assertAcknowledged(t, c)
			browserCall(t, target, cookie, http.MethodPut, "/api/v2/account/password", map[string]string{"currentPassword": containerBootstrapPassword, "password": containerChangedPassword}, http.StatusNoContent, nil)
			browserCall(t, target, cookie, http.MethodGet, "/api/v2/account", nil, http.StatusUnauthorized, nil)
			stop()
			remoteTokens.Delete(target)
			target = start()
			cookie = verifyBootstrapBrowser(t, target, containerChangedPassword)
			var afterUser accounts.User
			browserCall(t, target, cookie, http.MethodGet, "/api/v2/account", nil, http.StatusOK, &afterUser)
			if afterUser.UserID != user.UserID || afterUser.DatasetID != user.DatasetID {
				t.Fatal("container replacement changed account identity")
			}
			// Existing env initialization cannot reset a password changed in settings.
			browserCall(t, target, nil, http.MethodPost, "/api/v2/auth/session", map[string]string{"username": "admin", "password": containerBootstrapPassword}, http.StatusUnauthorized, nil)
			remoteTokens.Store(target, token.Secret)
			defer remoteTokens.Delete(target)
			if after := assertUsage(t, target, 1, 120); strings.Join(ids, ",") != strings.Join(after, ",") {
				t.Fatal("container replacement changed facts")
			}
			// Private recovery accepts stdin and invalidates sessions; API keys remain.
			command := exec.CommandContext(t.Context(), "docker", "exec", "-i", name, "tokeninsights-server", "admin", "user", "reset-password", user.UserID)
			command.Stdin = strings.NewReader(containerRecoveryPassword + "\n")
			if output, err := command.CombinedOutput(); err != nil || bytes.Contains(output, []byte(containerRecoveryPassword)) {
				t.Fatalf("operator recovery failed or exposed password: %v", err)
			}
			browserCall(t, target, cookie, http.MethodGet, "/api/v2/account", nil, http.StatusUnauthorized, nil)
			cookie = verifyBootstrapBrowser(t, target, containerRecoveryPassword)
			if after := assertUsage(t, target, 1, 120); strings.Join(ids, ",") != strings.Join(after, ",") {
				t.Fatal("password recovery changed usage identity")
			}
			browserCall(t, target, cookie, http.MethodDelete, "/api/v2/account/keys/"+token.TokenID, nil, http.StatusNoContent, nil)
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target+"/api/v2/status", nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+token.Secret)
			response, err := deploymentHTTP.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode != http.StatusUnauthorized {
				t.Fatal("revocation not enforced", response.StatusCode)
			}
			stop()
		})
	}
}

const (
	containerBootstrapPassword = "container initial password"
	containerChangedPassword   = "container changed password"
	containerRecoveryPassword  = "container recovery password"
)

func browserCall(t *testing.T, target string, cookie *http.Cookie, method, path string, body interface{}, status int, result interface{}) []*http.Cookie {
	t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequestWithContext(t.Context(), method, target+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", target)
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := deploymentHTTP.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != status {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatal(path, response.StatusCode, string(responseBody))
	}
	if result != nil {
		if err := json.NewDecoder(response.Body).Decode(result); err != nil {
			t.Fatal(err)
		}
	}
	return response.Cookies()
}

// Real browser flow uses the request host; no public origin or TLS configuration.
func verifyBootstrapBrowser(t *testing.T, target, password string) *http.Cookie {
	t.Helper()
	cookies := browserCall(t, target, nil, http.MethodPost, "/api/v2/auth/session", map[string]string{"username": "admin", "password": password}, http.StatusNoContent, nil)
	if len(cookies) != 1 {
		t.Fatal("HTTP bootstrap login cookie count", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("HTTP cookie policy", cookie)
	}
	browserCall(t, target, cookie, http.MethodGet, "/api/v2/instance", nil, http.StatusOK, nil)
	return cookie
}
