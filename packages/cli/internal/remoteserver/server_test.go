package remoteserver

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRemoteRuntimeWithoutClientState(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(key, filepath.Join(root, "must-not-exist"))
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Settings{PublicURL: "https://usage.example", Listen: "0.0.0.0:0", DBPath: filepath.Join(root, "server.sqlite")}, io.Discard, func(url string) error { ready <- url; return nil })
	}()
	var target string
	select {
	case target = <-ready:
	case err := <-done:
		t.Fatal("startup", err)
	case <-time.After(5 * time.Second):
		t.Fatal("startup timeout")
	}
	for _, endpoint := range []string{"/", "/api/v2/instance", "/api/v3/ingestion/capabilities"} {
		response, err := http.Get(target + endpoint)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		want := 200
		if endpoint != "/" {
			want = 401
		}
		if response.StatusCode != want {
			t.Fatal(endpoint, response.StatusCode)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "must-not-exist")); !os.IsNotExist(err) {
		t.Fatal("remote created local state", err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
}

func TestRemoteSettingsRejectBeforeStorage(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "server.sqlite")
	for _, settings := range []Settings{{Listen: "bad", DBPath: path}, {Listen: "127.0.0.1:65536", DBPath: path}, {Listen: "example.test:8765", DBPath: path}, {Listen: "127.0.0.1:0"}} {
		if err := Run(t.Context(), settings, io.Discard, nil); err == nil {
			t.Fatal("invalid settings accepted", settings)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("validation created storage", entries, err)
	}
}

func TestRemoteDatabaseHasSingleOwnerWithoutLocalControl(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sqlite")
	settings := Settings{PublicURL: "https://usage.example", Listen: "127.0.0.1:0", DBPath: path}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- Run(ctx, settings, io.Discard, func(url string) error { ready <- url; return nil }) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("startup timeout")
	}
	if err := Run(t.Context(), settings, io.Discard, nil); err == nil || !strings.Contains(err.Error(), "owns database") {
		t.Fatal("competing remote owner", err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
}
