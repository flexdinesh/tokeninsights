package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/duckdb"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqliteaccounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
)

func TestBackgroundPrintReturnsBeforeRemoteAcceptanceAndActuallySubmits(t *testing.T) {
	root := t.TempDir()
	isolateViewSources(t, root)
	data, err := datastore.OpenKind(t.Context(), filepath.Join(root, "remote.duckdb"), "hosted")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = data.Close() }()
	identity, err := data.DatabaseIdentity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	app, err := appstore.Open(t.Context(), filepath.Join(root, "remote-app.sqlite"), identity, "hosted")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = app.Close() }()
	auth := accounts.New(sqliteaccounts.NewSQLite(app, data))
	user, err := auth.CreateUser(t.Context(), "Alice")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.CreateToken(t.Context(), user.UserID, []string{accounts.Ingest}, nil)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := serverfeatures.New(serverfeatures.Hosted, false)
	if err != nil {
		t.Fatal(err)
	}
	handler := server.NewDataHandlerWithOptions(t.Context(), duckdb.Source{Store: data}, io.Discard, server.DataHandlerOptions{Host: "127.0.0.1", AllowIngestion: true, Policy: policy, Accounts: auth, PublicURL: "https://usage.example"})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	requested := make(chan struct{}, 1)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requested <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer func() { unblock(); remote.Close() }()
	source := filepath.Join(root, "sources", "pi", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"session","id":"background-session"}` + "\n" + `{"type":"message","id":"request","message":{"role":"assistant","timestamp":1767225600000,"usage":{"input":80,"output":20,"totalTokens":100}}}` + "\n"
	if err := os.WriteFile(source, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	settings := config.Defaults()
	settings.Mode = config.Distributed
	settings.ServerURL = remote.URL + "/"
	settings.ServerToken = token.Secret
	settings.CollectorDBPath = filepath.Join(root, "collector.sqlite")
	settings.ServerDBPath = filepath.Join(root, "unused.duckdb")
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	err = runSync(commandInvocation{context: ctx, stdout: &stdout, stderr: &stderr, settings: &settings}, []string{"--print", "--harness", "pi", "--source-dir", filepath.Join(root, "sources")})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != remote.URL+"\n" || !strings.Contains(stderr.String(), "Sync started:") {
		t.Fatal("wrong background output", stdout.String(), stderr.String())
	}
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not request server")
	}
	queue, err := syncjob.Open(t.Context(), settings.CollectorDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = queue.Close() }()
	job, err := queue.Latest(t.Context())
	if err != nil || job.State == "accepted" {
		t.Fatal("reported acceptance before network completed", job, err)
	}
	unblock()
	wait, stop := context.WithTimeout(t.Context(), 20*time.Second)
	defer stop()
	job, err = queue.Wait(wait, job.ID)
	if err != nil || job.State != "accepted" || job.Accepted != 2 {
		t.Fatal("worker failed", job, err)
	}
	for {
		worked, err := data.ProcessNext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	assertCLIQueryCount(t, data.SQL(), "SELECT SUM(total_tokens) FROM analytics.confirmed", 100)
	for _, path := range []string{queue.Path, queue.Path + "-wal"} {
		saved, err := os.ReadFile(path)
		if err == nil && bytes.Contains(saved, []byte(token.Secret)) {
			t.Fatal("credential persisted")
		}
	}
	if strings.Contains(stdout.String()+stderr.String(), token.Secret) {
		t.Fatal("credential printed")
	}
}

func TestDebugRequiresReadBeforeCaptureAndPrintRejectsLocal(t *testing.T) {
	root := t.TempDir()
	settings := config.Defaults()
	settings.CollectorDBPath = filepath.Join(root, "collector.sqlite")
	settings.ServerDBPath = filepath.Join(root, "server.duckdb")
	if err := runSync(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, settings: &settings}, []string{"--print"}); err == nil {
		t.Fatal("local print accepted")
	}
	if _, err := os.Stat(settings.CollectorDBPath); !os.IsNotExist(err) {
		t.Fatal("local print touched storage")
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v2/instance" {
			_ = json.NewEncoder(w).Encode(api.InstanceResponseV2{ApiVersion: "v2", InstanceId: "instance", DataEpoch: "database", DatasetId: "alice", DataReadiness: "ready", ServerKind: "hosted", Capabilities: []string{"raw-ingestion"}, Permissions: []api.InstanceResponseV2Permissions{"ingest"}})
			return
		}
		if r.URL.Path == "/api/v3/ingestion/capabilities" {
			_ = json.NewEncoder(w).Encode(evidence.Capabilities{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: "database", DatasetID: "alice", Completion: "acceptance", MaxBodyBytes: evidence.MaxBodyBytes, MaxEntries: evidence.MaxEntries})
			return
		}
		t.Error("debug captured/submitted before read preflight", r.URL.Path)
	}))
	defer remote.Close()
	settings.Mode = config.Distributed
	settings.ServerURL = remote.URL
	settings.ServerToken = "ingest-only"
	err := runSync(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, settings: &settings}, []string{"--debug"})
	if err == nil || !strings.Contains(err.Error(), "read permission") {
		t.Fatal(err)
	}
	if _, err := os.Stat(settings.CollectorDBPath); !os.IsNotExist(err) {
		t.Fatal("debug captured without read permission")
	}
}
