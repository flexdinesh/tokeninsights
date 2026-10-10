package deployment_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
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
				"--publish", "127.0.0.1::8766", "--env", "TOKENINSIGHTS_LISTEN=0.0.0.0:8766",
				"--env", "TOKENINSIGHTS_PUBLIC_URL=https://usage.example", "--env", "TOKENINSIGHTS_STORAGE_BACKEND=" + backend}
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
				address := dockerOutput(t, "port", name, "8766/tcp")
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
			var user accounts.User
			if err := json.Unmarshal([]byte(dockerOutput(t, "exec", name, "tokeninsights-server", "admin", "user", "create", "Container user")), &user); err != nil {
				t.Fatal(err)
			}
			var token accounts.Token
			if err := json.Unmarshal([]byte(dockerOutput(t, "exec", name, "tokeninsights-server", "admin", "token", "create", user.UserID)), &token); err != nil {
				t.Fatal(err)
			}
			remoteTokens.Store(target, token.Secret)
			c := newClient(t)
			c.must(t, "config", "set", "server-url", target)
			c.sync(t)
			ids := assertUsage(t, target, 1, 120)
			assertAcknowledged(t, c)
			stop()
			remoteTokens.Delete(target)
			target = start()
			remoteTokens.Store(target, token.Secret)
			defer remoteTokens.Delete(target)
			if after := assertUsage(t, target, 1, 120); strings.Join(ids, ",") != strings.Join(after, ",") {
				t.Fatal("container replacement changed facts")
			}
			dockerOutput(t, "exec", name, "tokeninsights-server", "admin", "token", "revoke", token.TokenID)
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
