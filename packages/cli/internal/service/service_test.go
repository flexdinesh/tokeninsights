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
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/app"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
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

func TestLifecycleDetachedEmptyStartRefreshAndRestart(t *testing.T) {
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
	database, err := db.Open(options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	var jobs int
	if err := database.QueryRow("SELECT COUNT(*) FROM sync_jobs").Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	if jobs != 0 {
		t.Fatal("startup ingested")
	}
	again, err := Ensure(ctx, Options{DBPath: options.DBPath})
	if err != nil || again.Record.InstanceID != state.Record.InstanceID {
		t.Fatal("ensure replaced healthy instance", err)
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	t.Setenv("XDG_RUNTIME_DIR", "")
	fromSSH, err := Probe(ctx, options.DBPath)
	if err != nil || fromSSH.Record.InstanceID != state.Record.InstanceID {
		t.Fatal("runtime environment lost daemon", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	request, err := http.NewRequestWithContext(ctx, "GET", state.Record.URL+"/api/v1/usage?period=all", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("empty dashboard unavailable", response.StatusCode)
	}
	client := Client{Record: *state.Record}
	operation, err := client.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Wait(ctx, operation.ID); err != nil {
		t.Fatal(err)
	}
	database, err = db.Open(options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	var facts int
	if err := database.QueryRow("SELECT COUNT(*) FROM canonical_token_usage").Scan(&facts); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	if facts != 1 {
		t.Fatal("refresh failed to ingest", facts)
	}
	oldEpoch := state.Status.DataEpoch
	if _, err := Mutate(ctx, options.DBPath, app.Action{Kind: "reset-all"}, nil); err != nil {
		t.Fatal(err)
	}
	status, err := client.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if oldEpoch == status.DataEpoch {
		t.Fatal("reset reused data epoch")
	}
	restarted, err := Restart(ctx, Options{DBPath: options.DBPath})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Record.InstanceID == state.Record.InstanceID || restarted.Record.Config.Port != 0 {
		t.Fatal("restart lost identity/binding")
	}
	if err := Stop(ctx, options.DBPath); err != nil {
		t.Fatal(err)
	}
	stopped, err := Probe(ctx, options.DBPath)
	if err != nil || stopped.Running {
		t.Fatal("stop retained service", err)
	}
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

func TestStartupPreservesRecoveryStateUntilRefresh(t *testing.T) {
	options := environment(t)
	source(t, filepath.Join(os.Getenv("HOME"), ".pi", "agent", "sessions"))
	database, _, err := db.CreateIfMissing(options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("UPDATE database_lifecycle SET data_generation = 1 WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	state, err := Ensure(ctx, options)
	if err != nil || state.Status == nil || state.Status.DataReadiness != "recovery" {
		t.Fatal("recovery service unavailable", state, err)
	}
	compatibility, err := db.InspectCompatibility(ctx, options.DBPath)
	if err != nil || !compatibility.ResetRequired {
		t.Fatal("startup repaired existing database", compatibility, err)
	}
	request, err := http.NewRequestWithContext(ctx, "GET", state.Record.URL+"/api/v1/usage?period=all", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("incompatible analytics exposed", response.StatusCode)
	}
	client := Client{Record: *state.Record}
	operation, err := client.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Wait(ctx, operation.ID); err != nil {
		t.Fatal(err)
	}
	status, err := client.Status(ctx)
	if err != nil || status.DataReadiness != "ready" || status.DataEpoch == state.Status.DataEpoch {
		t.Fatal("explicit refresh did not publish new epoch", status, err)
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

func TestForwardingUsesCallerSourcesAndSurvivesDaemonCrash(t *testing.T) {
	options := environment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	state, err := Ensure(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(os.Getenv("HOME"), "custom")
	source(t, custom)
	sources, err := pipeline.ResolveSources(custom)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := Mutate(ctx, options.DBPath, app.Action{Kind: "sync", Sources: sources, Harnesses: []pipeline.Harness{pipeline.HarnessPi}, Normalize: true, FullRefresh: true, Now: time.Now()}, nil)
	if err != nil || summary.Canonical != 1 {
		t.Fatal("caller scope changed", summary, err)
	}
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
		f, held, err := lifetime(state.Record.Config.DBPath, false)
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
}

func TestPrivateControlRejectsWrongNonceAndInvalidJSON(t *testing.T) {
	options := environment(t)
	state, err := Ensure(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	client := Client{Record: *state.Record}
	client.Record.InstanceID = app.ID()
	if _, err := client.Refresh(context.Background()); err == nil {
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

func TestForwardedCancellationCancelsOriginalOperationWhileWaitingForWriter(t *testing.T) {
	options := environment(t)
	state, err := Ensure(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	release, err := db.AcquireWriterLock(context.Background(), options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	sources, err := pipeline.ResolveSources("")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := Mutate(ctx, options.DBPath, app.Action{Kind: "sync", Sources: sources, Harnesses: pipeline.SupportedHarnesses, Normalize: true}, nil)
		done <- err
	}()
	client := Client{Record: *state.Record}
	deadline, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var id string
	for id == "" {
		status, err := client.Status(deadline)
		if err != nil {
			t.Fatal(err)
		}
		if status.Active != nil {
			id = status.Active.ID
			break
		}
		select {
		case <-deadline.Done():
			t.Fatal("mutation never admitted")
		case <-ticker.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled CLI succeeded")
		}
	case <-deadline.Done():
		t.Fatal("cancelled CLI stuck")
	}
	operation, err := client.Operation(deadline, id)
	if err != nil || operation.State != "cancelled" {
		t.Fatal("original action not cancelled", operation, err)
	}
}
