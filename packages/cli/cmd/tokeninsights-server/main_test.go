package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/remoteserver"
)

func TestAdminCommandsUseRunningOwner(t *testing.T) {
	root := t.TempDir()
	socket := filepath.Join(root, "admin.sock")
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	settings := remoteserver.Settings{Listen: "127.0.0.1:0", DBPath: filepath.Join(root, "hosted.sqlite"), AdminSocket: socket}
	go func() {
		done <- remoteserver.Run(ctx, settings, io.Discard, func(url string) error { ready <- url; return nil })
	}()
	var target string
	select {
	case target = <-ready:
	case err := <-done:
		cancel()
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("startup timeout")
	}
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("shutdown timeout")
		}
	}()
	var output, stderr bytes.Buffer
	call := func(args ...string) {
		t.Helper()
		output.Reset()
		stderr.Reset()
		command := append([]string{"admin", "--admin-socket", socket}, args...)
		if err := run(t.Context(), command, &output, &stderr); err != nil {
			t.Fatal(err, stderr.String())
		}
	}
	call("user", "create", "alice")
	var user accounts.User
	if err := json.Unmarshal(output.Bytes(), &user); err != nil || user.UserID == "" || user.DatasetID == "" {
		t.Fatal("user output", output.String(), err)
	}
	call("token", "create", user.UserID, "--scopes", "ingest,read", "--expires", time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	var token accounts.Token
	if err := json.Unmarshal(output.Bytes(), &token); err != nil || token.Secret == "" || len(token.Permissions) != 2 || token.ExpiresAt == nil {
		t.Fatal("token output", output.String(), err)
	}
	if strings.Count(output.String(), token.Secret) != 1 || strings.Contains(stderr.String(), token.Secret) {
		t.Fatal("secret was not printed exactly once")
	}
	call("user", "reprocess", user.UserID)
	var result struct {
		Generation int64 `json:"generation"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Generation != 2 {
		t.Fatal("user reprocess", output.String(), err)
	}
	call("token", "revoke", token.TokenID)
	if strings.Contains(output.String(), token.Secret) {
		t.Fatal("revocation exposed secret")
	}
	// Recovery password stays off argv, stdout and stderr; existing sessions revoke.
	const password = "long operator reset password"
	output.Reset()
	stderr.Reset()
	args := []string{"--admin-socket", socket, "user", "reset-password", user.UserID}
	if err := runAdmin(t.Context(), args, strings.NewReader(password+"\n"), &output, &stderr); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), password) || strings.Contains(stderr.String(), password) {
		t.Fatal("password leaked")
	}
	login := func() *http.Cookie {
		t.Helper()
		body, err := json.Marshal(map[string]string{"username": "alice", "password": password})
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target+"/api/v2/auth/session", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", target)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusNoContent || len(response.Cookies()) != 1 {
			t.Fatal("operator password did not recover account", response.StatusCode)
		}
		return response.Cookies()[0]
	}
	cookie := login()
	if err := runAdmin(t.Context(), args, strings.NewReader("new operator reset password\n"), &output, &stderr); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target+"/api/v2/instance", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(cookie)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("operator recovery preserved browser session", response.StatusCode)
	}
	call("user", "disable", user.UserID)
}

func TestAdminPasswordInputBounded(t *testing.T) {
	for _, input := range []string{"", strings.Repeat("x", 1024)} {
		var output bytes.Buffer
		err := runAdmin(t.Context(), []string{"user", "reset-password", "user"}, strings.NewReader(input), &output, io.Discard)
		if err == nil || strings.Contains(err.Error(), input) && input != "" || output.Len() != 0 {
			t.Fatal("invalid stdin accepted or leaked", err)
		}
	}
}
