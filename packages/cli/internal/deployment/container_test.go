package deployment_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
)

type fixtureReply struct {
	UID      int    `json:"uid"`
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output"`
}

// Control operations include the image's bounded graceful shutdown.
var fixtureHTTP = &http.Client{Timeout: 30 * time.Second}

func fixtureCall(t *testing.T, control, operation string, body interface{}) fixtureReply {
	t.Helper()
	var reply fixtureReply
	browserCallWithClient(t, fixtureHTTP, control, nil, http.MethodPost, operation, body, http.StatusOK, &reply)
	return reply
}

func TestContainerDeployment(t *testing.T) {
	if os.Getenv("TOKENINSIGHTS_TEST_SQLITE_CONTROL_URL") == "" {
		t.Skip("run isolated deployment validation")
	}
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			prefix := "TOKENINSIGHTS_TEST_" + strings.ToUpper(backend)
			control, target := os.Getenv(prefix+"_CONTROL_URL"), os.Getenv(prefix+"_URL")
			if control == "" || target == "" {
				t.Fatal("missing fixture endpoints", backend)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
				defer cancel()
				request, err := http.NewRequestWithContext(ctx, http.MethodPost, control+"/stop", nil)
				if err == nil {
					response, err := fixtureHTTP.Do(request)
					if err == nil {
						_ = response.Body.Close()
					}
				}
			})
			start := func() string {
				if reply := fixtureCall(t, control, "/start", nil); reply.UID != 10001 {
					t.Fatal("server must be non-root", reply.UID)
				}
				deadline := time.Now().Add(30 * time.Second)
				for {
					response, err := deploymentHTTP.Get(target + "/readyz")
					if err == nil {
						_ = response.Body.Close()
						if response.StatusCode == http.StatusOK {
							break
						}
					}
					if time.Now().After(deadline) {
						var logs fixtureReply
						browserCall(t, control, nil, http.MethodGet, "/logs", nil, http.StatusOK, &logs)
						t.Fatal("container readiness", logs.Output)
					}
					time.Sleep(100 * time.Millisecond)
				}
				fixtureCall(t, control, "/health", nil)
				return target
			}
			stop := func() {
				if reply := fixtureCall(t, control, "/stop", nil); reply.ExitCode != 0 {
					t.Fatal("unclean SIGTERM", reply)
				}
			}
			target = start()
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
			reply := fixtureCall(t, control, "/reset", map[string]string{"userID": user.UserID, "password": containerRecoveryPassword})
			if strings.Contains(reply.Output, containerRecoveryPassword) {
				t.Fatal("operator recovery exposed password")
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
	return browserCallWithClient(t, deploymentHTTP, target, cookie, method, path, body, status, result)
}

func browserCallWithClient(t *testing.T, client *http.Client, target string, cookie *http.Cookie, method, path string, body interface{}, status int, result interface{}) []*http.Cookie {
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
	response, err := client.Do(request)
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
