package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"bytes"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
	_ "modernc.org/sqlite"
)

// Exercise detached re-exec with real descriptors/sockets. Test binaries need
// this dispatch; otherwise their test runner would execute the suite again.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "__service-run" {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if err := Child(ctx); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func environment(t *testing.T) Options {
	t.Helper()
	root, err := os.MkdirTemp("", "ti-service-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	for key, value := range map[string]string{"HOME": root, "XDG_CONFIG_HOME": filepath.Join(root, "config"), "XDG_STATE_HOME": filepath.Join(root, "state"), "XDG_RUNTIME_DIR": filepath.Join(root, "run"), "XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_CONFIG_DIR": filepath.Join(root, "claude")} {
		t.Setenv(key, value)
	}
	if err := os.MkdirAll(os.Getenv("XDG_RUNTIME_DIR"), 0o700); err != nil {
		t.Fatal(err)
	}
	port := 0
	options := Options{DBPath: filepath.Join(root, "usage.sqlite"), Port: &port}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := Stop(ctx, options.DBPath); err != nil {
			t.Error(err)
		}
	})
	return options
}

func source(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	text := `{"type":"session","version":1,"id":"service-session","timestamp":"2026-09-01T00:00:00Z"}
{"type":"message","id":"m1","timestamp":"2026-09-01T00:00:01Z","message":{"role":"assistant","provider":"openai","model":"test","usage":{"input":100,"output":20,"totalTokens":120}}}`
	if err := os.WriteFile(filepath.Join(root, "session.jsonl"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleDetachedEmptyStartIngestAndRestart(t *testing.T) {
	options := environment(t)
	source(t, filepath.Join(os.Getenv("HOME"), ".pi", "agent", "sessions"))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	state, err := Ensure(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Running || state.Record.PID == os.Getpid() || state.Status.DataReadiness != "ready" {
		t.Fatal(state)
	}
	store, err := serverstore.Open(options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	var facts int
	if err := store.SQL().QueryRow("SELECT COUNT(*) FROM canonical_token_usage").Scan(&facts); err != nil || facts != 0 {
		t.Fatal("startup collected", facts, err)
	}
	_ = store.Close()
	again, err := Ensure(ctx, Options{DBPath: options.DBPath})
	if err != nil || again.Record.InstanceID != state.Record.InstanceID {
		t.Fatal("healthy service replaced", err)
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	t.Setenv("XDG_RUNTIME_DIR", "")
	fromSSH, err := Probe(ctx, options.DBPath)
	if err != nil || fromSSH.Record.InstanceID != state.Record.InstanceID {
		t.Fatal("runtime discovery lost", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	batch := savedBatch(t, state.Status.DataEpoch)
	first := postBatch(t, state.Record.URL, batch)
	second := postBatch(t, state.Record.URL, batch)
	if first != second || first.Inserted != 1 {
		t.Fatal("replay differs", first, second)
	}
	before := state.Status.DataEpoch
	restarted, err := Restart(ctx, Options{DBPath: options.DBPath})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Record.InstanceID == state.Record.InstanceID || restarted.Status.DataEpoch != before || restarted.Record.Config.Port != 0 {
		t.Fatal("restart identity/binding", restarted)
	}
	if replay := postBatch(t, restarted.Record.URL, batch); replay != first {
		t.Fatal("restart receipt lost", replay, first)
	}
	store, err = serverstore.Open(options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	var total int64
	if err := store.SQL().QueryRow("SELECT COUNT(*),SUM(total_tokens) FROM canonical_token_usage").Scan(&facts, &total); err != nil || facts != 1 || total != 120 {
		t.Fatal(facts, total, err)
	}
	if err := Stop(ctx, options.DBPath); err != nil {
		t.Fatal(err)
	}
	stopped, err := Probe(ctx, options.DBPath)
	if err != nil || stopped.Running {
		t.Fatal(stopped, err)
	}
}

func savedBatch(t *testing.T, databaseID string) []byte {
	t.Helper()
	fact := publication.Fact{Harness: "pi", Session: publication.Session{Harness: "pi", NativeID: "service-session", FirstOccurredAtMs: 1000, LastOccurredAtMs: 1000}, Message: &publication.Message{NativeID: "native-message", OccurredAtMs: 1000}, OccurredAtMs: 1000, Provider: "openai", ProviderSource: "explicit", Model: "test", UsageScope: "message", Quality: "exact", Countable: true, InputTokens: 100, OutputTokens: 20, TotalTokens: 120}
	publication.SetIDs(&fact)
	batch := publication.Batch{ProtocolVersion: 1, IdentityVersion: 1, SemanticsVersion: 1, DatabaseID: databaseID, StreamID: "test-stream", BatchID: "test-batch", FromSequence: 1, ToSequence: 1, Hostname: "collector-fixture", Entries: []publication.Entry{{Sequence: 1, Fact: fact}}}
	encoded, err := publication.EncodeBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func postBatch(t *testing.T, url string, body []byte) publication.Receipt {
	t.Helper()
	response, err := http.Post(url+"/api/v1/ingestion/batches", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("ingestion status%d %s", response.StatusCode, payload)
	}
	receipt, err := publication.DecodeReceipt(payload)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestConcurrentStartAndSymlinkIdentity(t *testing.T) {
	options := environment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var workers sync.WaitGroup
	results := make(chan State, 8)
	failures := make(chan error, 8)
	for range 8 {
		workers.Go(func() {
			state, err := Ensure(ctx, options)
			if err != nil {
				failures <- err
			} else {
				results <- state
			}
		})
	}
	workers.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	var id string
	for state := range results {
		if id == "" {
			id = state.Record.InstanceID
		}
		if id != state.Record.InstanceID {
			t.Fatal("duplicate service")
		}
	}
	alias := filepath.Join(filepath.Dir(options.DBPath), "alias.sqlite")
	if err := os.Symlink(options.DBPath, alias); err != nil {
		t.Fatal(err)
	}
	state, err := Probe(ctx, alias)
	if err != nil || state.Record.InstanceID != id {
		t.Fatal("alias lost service", err)
	}
}

func TestStartupRejectsCollectorDatabaseWithoutMutation(t *testing.T) {
	options := environment(t)
	database, _, err := db.CreateIfMissing(options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	before, err := os.ReadFile(options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Ensure(ctx, options); err == nil {
		t.Fatal("collector role accepted")
	}
	after, err := os.ReadFile(options.DBPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("collector modified", err)
	}
}

func TestStartupRefusesUnknownDatabaseWithoutModifyingIt(t *testing.T) {
	options := environment(t)
	before := []byte("not a tokeninsights database")
	if err := os.WriteFile(options.DBPath, before, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Ensure(ctx, options); err == nil {
		t.Fatal("unknown database accepted")
	}
	after, err := os.ReadFile(options.DBPath)
	if err != nil || string(after) != string(before) {
		t.Fatal("unknown database modified", err)
	}
}

func TestStatusMissingIsReadOnlyAndOccupiedPortDoesNotTakeOver(t *testing.T) {
	options := environment(t)
	before, err := os.ReadDir(filepath.Dir(options.DBPath))
	if err != nil {
		t.Fatal(err)
	}
	state, err := Probe(context.Background(), options.DBPath)
	if err != nil || state.Running {
		t.Fatal(state, err)
	}
	after, err := os.ReadDir(filepath.Dir(options.DBPath))
	if err != nil || len(before) != len(after) {
		t.Fatal("status created service artifacts")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	options.Port = &port
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Ensure(ctx, options); err == nil {
		t.Fatal("occupied port accepted")
	}
	if _, err := os.Stat(options.DBPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed bind created DB")
	}
}

func TestIngestedFactsAndReceiptSurviveDaemonCrash(t *testing.T) {
	options := environment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	state, err := Ensure(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	batch := savedBatch(t, state.Status.DataEpoch)
	receipt := postBatch(t, state.Record.URL, batch)
	process, err := os.FindProcess(state.Record.PID)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		f, held, err := lifetime(options.DBPath, false)
		if f != nil {
			_ = f.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
		if !held {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	restarted, err := Ensure(ctx, options)
	if err != nil || restarted.Record.InstanceID == state.Record.InstanceID {
		t.Fatal("stale runtime prevented restart", err)
	}
	if replay := postBatch(t, restarted.Record.URL, batch); replay != receipt {
		t.Fatal("crash lost durable receipt", replay, receipt)
	}
}

func TestPrivateControlRejectsWrongNonceAndInvalidJSON(t *testing.T) {
	options := environment(t)
	state, err := Ensure(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	client := Client{Record: *state.Record}
	client.Record.InstanceID = instanceID()
	if _, err := client.Status(context.Background()); err == nil {
		t.Fatal("wrong instance accepted")
	}
	p, key, err := identify(options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	files, err := servicePaths(key, false)
	if err != nil {
		t.Fatal(err)
	}
	var config Config
	if err := readFile(files.config, &config); err != nil || config.DBPath != p {
		t.Fatal("configuration not committed", err)
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > bodyLimit {
		t.Fatal("oversized configuration")
	}
	if bytes.Contains(data, []byte("sources")) || bytes.Contains(data, []byte(os.Getenv("CODEX_HOME"))) {
		t.Fatal("server config retained source configuration")
	}
	client.Record.InstanceID = state.Record.InstanceID
	var rejected struct {
		Error string `json:"error"`
	}
	if err := client.call(t.Context(), http.MethodPost, "/shutdown", map[string]string{"unexpected": "synthetic-private-marker"}, &rejected); err == nil {
		t.Fatal("invalid shutdown JSON accepted")
	}
	if _, err := client.Status(t.Context()); err != nil {
		t.Fatal("invalid shutdown stopped service", err)
	}
}

func TestCancelledStartupReleasesChildOwnershipBeforeReturning(t *testing.T) {
	options := environment(t)
	release, err := db.AcquireWriterLock(context.Background(), options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := Ensure(ctx, options); done <- err }()
	deadline, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		f, held, err := lifetime(options.DBPath, false)
		if f != nil {
			_ = f.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
		if held {
			break
		}
		select {
		case <-deadline.Done():
			t.Fatal("startup did not own lifetime")
		case <-ticker.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("startup cancellation lost", err)
		}
	case <-deadline.Done():
		t.Fatal("startup cancellation stuck")
	}
	f, held, err := lifetime(options.DBPath, false)
	if f != nil {
		_ = f.Close()
	}
	if err != nil || held {
		t.Fatal("failed startup retained live child", err)
	}
}

func TestForegroundShutdownUsesSameOwnership(t *testing.T) {
	options := environment(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, options, io.Discard, io.Discard) }()
	deadline, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := Probe(deadline, options.DBPath)
		if err == nil && state.Running {
			break
		}
		select {
		case <-deadline.Done():
			t.Fatal("foreground not ready")
		case <-ticker.C:
		}
	}
	if err := Stop(deadline, options.DBPath); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestLocalExposedBindingNeedsNoAuthenticationAndRemoteUsesSeparateRuntime(t *testing.T) {
	options := environment(t)
	host := "0.0.0.0"
	options.Host = &host
	if _, err := configuration(options); err != nil {
		t.Fatal(err)
	}
	options.Remote = true
	if _, err := configuration(options); err == nil || !strings.Contains(err.Error(), "tokeninsights-server") {
		t.Fatal("legacy remote runtime accepted", err)
	}
}

func TestSavedExposedConfigurationIsUnauthenticated(t *testing.T) {
	options := environment(t)
	config, err := configuration(options)
	if err != nil {
		t.Fatal(err)
	}
	config.Host = "0.0.0.0"
	files, err := servicePaths(config.DatabaseKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicFile(files.config, config); err != nil {
		t.Fatal(err)
	}
	if _, err := configuration(options); err != nil {
		t.Fatal(err)
	}
}

func TestLocalRejectsLegacyAuthenticationAndRemoteOptions(t *testing.T) {
	options := environment(t)
	token := "synthetic-private-token"
	options.Token = &token
	if _, err := Ensure(t.Context(), options); err == nil || strings.Contains(err.Error(), token) {
		t.Fatal("legacy token accepted or disclosed", err)
	}
	options.Token = nil
	options.Remote = true
	if _, err := Ensure(t.Context(), options); err == nil {
		t.Fatal("remote service accepted")
	}
}

func TestHardLinkedServerIdentityRejected(t *testing.T) {
	options := environment(t)
	store, err := serverstore.CreateIfMissing(options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	alias := filepath.Join(filepath.Dir(options.DBPath), "hardlink.sqlite")
	if err := os.Link(options.DBPath, alias); err != nil {
		t.Fatal(err)
	}
	if _, _, err := identify(options.DBPath); err == nil {
		t.Fatal("hard-linked server accepted")
	}
	// Remove the test alias before cleanup probes the original service path.
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
}

func TestLocalIngestionValidationRemainsJSONWithoutAuthentication(t *testing.T) {
	options := environment(t)
	state, err := Ensure(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	status, body := authenticatedRequest(t, http.MethodPost, state.Record.URL+"/api/v1/ingestion/batches", "", []byte("synthetic-private-body"))
	if status != http.StatusBadRequest || bytes.Contains(body, []byte("synthetic-private-body")) {
		t.Fatal(status, string(body))
	}
}
