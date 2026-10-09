package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)
	settings := remoteserver.Settings{Listen: "127.0.0.1:0", DBPath: filepath.Join(root, "hosted.duckdb"), PublicURL: "https://usage.example", AdminSocket: socket}
	go func() {
		done <- remoteserver.Run(ctx, settings, io.Discard, func(string) error { ready <- struct{}{}; return nil })
	}()
	select {
	case <-ready:
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
	call("user", "create", "Alice")
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
	call("user", "disable", user.UserID)
}
