package remoteserver

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/clientaddress"
)

func TestHostedProxyLoginBoundary(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "trusted"}[trusted], func(t *testing.T) {
			settings := Settings{Listen: "127.0.0.1:0", DBPath: filepath.Join(t.TempDir(), "server.sqlite"), PublicURL: "https://usage.example"}
			if trusted {
				var err error
				settings.TrustedProxies, err = clientaddress.Parse("127.0.0.1/32")
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			ready := make(chan string, 1)
			go func() { done <- Run(ctx, settings, io.Discard, func(url string) error { ready <- url; return nil }) }()
			defer func() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(5 * time.Second):
					t.Error("shutdown timeout")
				}
			}()
			var target string
			select {
			case target = <-ready:
			case <-time.After(5 * time.Second):
				t.Fatal("startup timeout")
			}
			client := &http.Client{Timeout: time.Second}
			login := func(address, origin string) int {
				t.Helper()
				r, err := http.NewRequestWithContext(t.Context(), "POST", target+"/api/v2/auth/session", strings.NewReader(`{"token":"invalid"}`))
				if err != nil {
					t.Fatal(err)
				}
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Origin", origin)
				r.Header.Set("X-Forwarded-For", address)
				r.Header.Set("X-Forwarded-Host", "evil.example")
				r.Header.Set("X-Forwarded-Proto", "https")
				response, err := client.Do(r)
				if err != nil {
					t.Fatal(err)
				}
				_ = response.Body.Close()
				return response.StatusCode
			}
			for range accounts.LoginRequestsPerMinute {
				if got := login("192.0.2.1", "https://usage.example"); got != 401 {
					t.Fatal("unexpected login", got)
				}
			}
			if got := login("192.0.2.1", "https://usage.example"); got != 429 {
				t.Fatal("limit not enforced", got)
			}
			want := 429
			if trusted {
				want = 401
			}
			if got := login("192.0.2.2", "https://usage.example"); got != want {
				t.Fatal("proxy trust did not control client isolation", got, want)
			}
			if got := login("192.0.2.3", "https://evil.example"); got != 403 {
				t.Fatal("forwarded headers changed origin policy", got)
			}
		})
	}
}
