package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func cleanServerEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"STORAGE_BACKEND", "LISTEN", "SERVER_DB_PATH", "APP_DB_PATH", "PUBLIC_URL", "ADMIN_SOCKET", "TRUSTED_PROXIES", "POSTGRES_DSN", "POSTGRES_DSN_FILE", "SERVER_TOKEN"} {
		t.Setenv("TOKENINSIGHTS_"+key, "")
	}
}

func TestServerConfiguration(t *testing.T) {
	cleanServerEnv(t)
	root := t.TempDir()
	t.Setenv("TOKENINSIGHTS_PUBLIC_URL", "https://usage.example")
	t.Setenv("TOKENINSIGHTS_SERVER_DB_PATH", filepath.Join(root, "data.sqlite"))
	t.Setenv("TOKENINSIGHTS_LISTEN", "bad-env")
	s, _, err := serverSettings([]string{"--listen", "127.0.0.1:9000"}, io.Discard)
	if err != nil || s.Listen != "127.0.0.1:9000" || s.Backend != "sqlite" || s.PublicURL != "https://usage.example" {
		t.Fatal(s.Listen, err)
	}
	if _, _, err := serverSettings(nil, io.Discard); err == nil {
		t.Fatal("invalid environment accepted")
	}
	t.Setenv("TOKENINSIGHTS_LISTEN", "")
	t.Setenv("TOKENINSIGHTS_TRUSTED_PROXIES", "not-a-cidr")
	if _, _, err := serverSettings(nil, io.Discard); err == nil {
		t.Fatal("invalid proxy trust accepted")
	}
	if _, _, err := serverSettings([]string{"--trusted-proxies="}, io.Discard); err != nil {
		t.Fatal("empty flag did not override proxy environment", err)
	}
	t.Setenv("TOKENINSIGHTS_TRUSTED_PROXIES", "")
	t.Setenv("TOKENINSIGHTS_STORAGE_BACKEND", "postgres")
	t.Setenv("TOKENINSIGHTS_ADMIN_SOCKET", filepath.Join(root, "admin.sock"))
	secret := filepath.Join(root, "dsn")
	const dsn = "postgres://user:private-password@localhost/database"
	if err := os.WriteFile(secret, []byte(dsn+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKENINSIGHTS_POSTGRES_DSN_FILE", secret)
	if _, _, err := serverSettings(nil, io.Discard); err == nil {
		t.Fatal("mixed storage accepted")
	}
	t.Setenv("TOKENINSIGHTS_SERVER_DB_PATH", "")
	s, _, err = serverSettings(nil, io.Discard)
	if err != nil || s.PostgresDSN != dsn || s.Listen != defaultListen {
		t.Fatal("secret/environment resolution failed", err)
	}
	t.Setenv("TOKENINSIGHTS_POSTGRES_DSN", dsn)
	_, _, err = serverSettings(nil, io.Discard)
	if err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatal("secret conflict not rejected safely")
	}
	t.Setenv("TOKENINSIGHTS_POSTGRES_DSN", "")
	for _, contents := range []string{"", strings.Repeat("x", (64<<10)+1)} {
		if err := os.WriteFile(secret, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := serverSettings(nil, io.Discard); err == nil {
			t.Fatal("bad secret accepted")
		}
	}
	t.Setenv("TOKENINSIGHTS_POSTGRES_DSN_FILE", filepath.Join(root, "missing-private-password"))
	var output bytes.Buffer
	if err := run(t.Context(), nil, &output, &output); err == nil || strings.Contains(err.Error(), "private-password") || strings.Contains(output.String(), "private-password") {
		t.Fatal("secret read failure not sanitized")
	}
	output.Reset()
	if err := run(t.Context(), []string{"--version"}, &output, &output); err != nil {
		t.Fatal("version requires valid deployment", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("configuration opened storage", err)
	}
}

func TestHealthcheckUsesOnlyHTTPConfiguration(t *testing.T) {
	cleanServerEnv(t)
	t.Setenv("TOKENINSIGHTS_POSTGRES_DSN_FILE", "/missing-secret")
	var status atomic.Int32
	status.Store(http.StatusOK)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			t.Error("wrong probe", r.URL.Path)
		}
		w.WriteHeader(int(status.Load()))
	}))
	defer srv.Close()
	t.Setenv("TOKENINSIGHTS_LISTEN", strings.TrimPrefix(srv.URL, "http://"))
	if err := run(t.Context(), []string{"healthcheck"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	status.Store(http.StatusServiceUnavailable)
	if err := healthcheck(t.Context(), nil, io.Discard); err == nil {
		t.Fatal("unready server accepted")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/readyz", http.StatusFound)
	}))
	defer redirect.Close()
	status.Store(http.StatusOK)
	if err := healthcheck(t.Context(), []string{"--listen", strings.TrimPrefix(redirect.URL, "http://")}, io.Discard); err == nil {
		t.Fatal("redirect treated as readiness")
	}
	srv.Close()
	if err := healthcheck(t.Context(), nil, io.Discard); err == nil {
		t.Fatal("stopped server accepted")
	}
}
