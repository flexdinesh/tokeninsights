package deployment_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

var clientBinary, serverBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ti-deployment-build-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	clientBinary, serverBinary = filepath.Join(dir, "tokeninsights"), filepath.Join(dir, "tokeninsights-server")
	for name, binary := range map[string]string{"tokeninsights": clientBinary, "tokeninsights-server": serverBinary} {
		command := exec.Command("go", "build", "-o", binary, "./cmd/"+name)
		command.Dir = "../.."
		if output, err := command.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build: %s %v\n", output, err)
			_ = os.RemoveAll(dir)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type client struct {
	root, config, collector, source string
	env                             []string
}

func isolatedEnvironment(root string) []string {
	env := []string{}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if key == "HOME" || key == "PATH" || strings.HasPrefix(key, "TOKENINSIGHTS_") || strings.HasPrefix(key, "XDG_") {
			continue
		}
		env = append(env, item)
	}
	return append(env, "PATH=", "HOME="+root, "XDG_CONFIG_HOME="+filepath.Join(root, "config"), "XDG_STATE_HOME="+filepath.Join(root, "state"), "XDG_RUNTIME_DIR="+filepath.Join(root, "run"), "XDG_DATA_HOME="+filepath.Join(root, "data"))
}

func newClient(t *testing.T) client {
	t.Helper()
	root, err := os.MkdirTemp("", "ti-client-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	c := client{root: root, config: filepath.Join(root, "config.json"), collector: filepath.Join(root, "data", "tokeninsights", "collector.sqlite"), source: filepath.Join(root, "source")}
	c.env = isolatedEnvironment(root)
	if err := os.MkdirAll(c.source, 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../../testdata/conformance/local-remote-deployment/source/pi/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c.source, "session.jsonl"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	return c
}

func (c client) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, clientBinary, append([]string{"--config-file", c.config}, args...)...)
	command.Env = c.env
	body, err := command.CombinedOutput()
	return string(body), err
}
func (c client) must(t *testing.T, args ...string) string {
	t.Helper()
	body, err := c.run(t, args...)
	if err != nil {
		t.Fatalf("%v: %s %v", args, body, err)
	}
	return body
}
func (c client) sync(t *testing.T) {
	t.Helper()
	c.must(t, "sync", "--harness", "pi", "--source-dir", c.source)
}

func startRemote(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "server.duckdb")
	command := exec.Command(serverBinary, "--listen", "0.0.0.0:0", "--server-db-path", path)
	command.Env = isolatedEnvironment(root)
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(root, "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = log
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		_ = command.Process.Signal(syscall.SIGTERM)
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(20 * time.Second):
			_ = command.Process.Kill()
			<-done
			t.Error("server shutdown timeout")
		}
		_ = log.Close()
	})
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		if scanner.Scan() {
			ready <- scanner.Text()
		}
	}()
	var target string
	select {
	case target = <-ready:
	case <-time.After(10 * time.Second):
		body, _ := os.ReadFile(log.Name())
		t.Fatalf("server readiness: %s", body)
	}
	if !strings.HasPrefix(target, "http://127.0.0.1:") {
		t.Fatal("unusable wildcard URL", target)
	}
	if _, err := os.Stat(filepath.Join(root, "config")); !os.IsNotExist(err) {
		t.Fatal("remote created client config", err)
	}
	if _, err := os.Stat(filepath.Join(root, "state")); !os.IsNotExist(err) {
		t.Fatal("remote created local service state", err)
	}
	return target, path
}

func assertUsage(t *testing.T, target, path string, facts, total int64) []string {
	t.Helper()
	var fixture struct {
		Facts      int64 `json:"facts"`
		Input      int64 `json:"input_tokens"`
		Output     int64 `json:"output_tokens"`
		Reasoning  int64 `json:"reasoning_tokens"`
		CacheRead  int64 `json:"cache_read_tokens"`
		CacheWrite int64 `json:"cache_write_tokens"`
		Total      int64 `json:"total_tokens"`
	}
	body, err := os.ReadFile("../../testdata/conformance/local-remote-deployment/expected/totals.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &fixture); err != nil || fixture.Facts != 1 || fixture.Total*facts != total {
		t.Fatal("invalid independent fixture oracle", fixture, err)
	}
	return assertComponents(t, target, path, [7]int64{facts, fixture.Input * facts, fixture.Output * facts, fixture.Reasoning * facts, fixture.CacheRead * facts, fixture.CacheWrite * facts, total})
}
func assertComponents(t *testing.T, target, path string, want [7]int64) []string {
	t.Helper()

	_ = path // HTTP owns the live data file; do not open a second DuckDB process.
	deadline := time.Now().Add(10 * time.Second)
	var data api.UsageResponse
	for {
		response, err := http.Get(target + "/api/v1/usage?period=all&tab=sessions")
		if err != nil {
			t.Fatal(err)
		}
		err = json.NewDecoder(response.Body).Decode(&data)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatal(err, response.StatusCode)
		}
		if data.Pending != nil && *data.Pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("processing did not finish", data)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if data.FactCount == nil || *data.FactCount != want[0] || data.Summary.Total != want[6] || data.Summary.Input != want[1] || data.Summary.Output != want[2] || data.Summary.Reasoning != want[3] || data.Summary.CacheRead != want[4] || data.Summary.CacheWrite != want[5] {
		t.Fatal("REST component/count oracle", data)
	}
	ids := []string{}
	for _, row := range data.Rows {
		ids = append(ids, row.Key)
	}

	return ids
}

func assertAcknowledged(t *testing.T, c client) {
	t.Helper()
	database, err := db.OpenWritable(c.collector)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	var journal, cursor, receipts int64
	if err := database.QueryRow("SELECT COUNT(*) FROM evidence_outbox").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow("SELECT COALESCE(MAX(acknowledged_sequence),0) FROM evidence_destinations").Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow("SELECT COUNT(*) FROM evidence_batches WHERE receipt_bytes IS NOT NULL").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if journal != 2 || cursor != 2 || receipts != 1 {
		t.Fatal("publication", journal, cursor, receipts)
	}
	if _, err := os.Stat(filepath.Join(c.root, "data", "tokeninsights", "server.duckdb")); !os.IsNotExist(err) {
		t.Fatal("remote sync created local server", err)
	}
	if _, err := os.Stat(filepath.Join(c.root, "state")); !os.IsNotExist(err) {
		t.Fatal("remote sync created lifecycle state", err)
	}
}

func TestConfigRemoteSyncTracerAndCopiedClients(t *testing.T) {
	target, path := startRemote(t)
	a, b := newClient(t), newClient(t)
	for _, c := range []client{a, b} {
		c.must(t, "config", "set", "server-url", target)
		c.sync(t)
		assertUsage(t, target, path, 1, 120)
		assertAcknowledged(t, c)
	}
	before := assertUsage(t, target, path, 1, 120)
	a.sync(t)
	after := assertUsage(t, target, path, 1, 120)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatal("repeat changed stable identities")
	}
	if output := a.must(t); !strings.Contains(output, target) {
		t.Fatal("bare invocation", output)
	}
	a.must(t, "collector", "reset-all", "--confirm")
	a.sync(t)
	after = assertUsage(t, target, path, 1, 120)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatal("rebuild changed stable identities")
	}
}

func TestRemoteFailureRetainsJournalAndNeverStartsLocal(t *testing.T) {
	c := newClient(t)
	c.must(t, "config", "set", "server-url", "http://127.0.0.1:1")
	if output, err := c.run(t, "sync", "--harness", "pi", "--source-dir", c.source); err == nil || !strings.Contains(output, "transport_failed") {
		t.Fatal(output, err)
	}
	database, err := db.OpenWritable(c.collector)
	if err != nil {
		t.Fatal(err)
	}
	var journal int
	if err := database.QueryRow("SELECT COUNT(*) FROM evidence_outbox").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	if journal != 2 {
		t.Fatal("work lost", journal)
	}
	target, path := startRemote(t)
	c.must(t, "config", "set", "server-url", target)
	c.must(t, "sync", "--publish-only")
	assertUsage(t, target, path, 1, 120)
	assertAcknowledged(t, c)
}

func TestRemoteCommittedResponseLostReplaysExactRequestAndReceipt(t *testing.T) {
	target, path := startRemote(t)
	upstream, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	var loseFirst atomic.Bool
	loseFirst.Store(true)
	var requestsMu sync.Mutex
	var requests [][]byte
	var originalReceipt []byte
	proxy.ModifyResponse = func(response *http.Response) error {
		if response.Request.Method != http.MethodPost {
			return nil
		}
		if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
			return fmt.Errorf("unexpected ingestion status %d", response.StatusCode)
		}
		if loseFirst.Swap(false) {
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				return err
			}
			requestsMu.Lock()
			originalReceipt = body
			requestsMu.Unlock()
			return io.ErrUnexpectedEOF // Real upstream commit; suppress only its response.
		}
		return nil
	}
	proxy.ErrorHandler = func(out http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(out, "response lost", http.StatusBadGateway)
	}
	forwarder := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			body, err := io.ReadAll(request.Body)
			_ = request.Body.Close()
			if err != nil {
				http.Error(out, "request unreadable", http.StatusBadRequest)
				return
			}
			requestsMu.Lock()
			requests = append(requests, body)
			requestsMu.Unlock()
			request.Body = io.NopCloser(bytes.NewReader(body))
		}
		request.Host = upstream.Host
		proxy.ServeHTTP(out, request)
	}))
	defer forwarder.Close()
	c := newClient(t)
	c.must(t, "config", "set", "server-url", forwarder.URL)
	if output, err := c.run(t, "sync", "--harness", "pi", "--source-dir", c.source); err == nil {
		t.Fatal("lost receipt acknowledged", output)
	}
	before := assertUsage(t, target, path, 1, 120)
	database, err := db.OpenWritable(c.collector)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	var savedRequest, acknowledgedReceipt []byte
	var cursor int64
	if err := database.QueryRow("SELECT request_bytes FROM evidence_batches WHERE receipt_bytes IS NULL").Scan(&savedRequest); err != nil {
		t.Fatal("pending request missing", err)
	}
	if err := database.QueryRow("SELECT acknowledged_sequence FROM evidence_destinations").Scan(&cursor); err != nil || cursor != 0 {
		t.Fatal("lost response advanced cursor", cursor, err)
	}
	c.must(t, "sync", "--publish-only")
	if err := database.QueryRow("SELECT receipt_bytes FROM evidence_batches").Scan(&acknowledgedReceipt); err != nil {
		t.Fatal(err)
	}
	requestsMu.Lock()
	defer requestsMu.Unlock()
	if len(requests) != 2 || !bytes.Equal(requests[0], savedRequest) || !bytes.Equal(requests[1], savedRequest) || !sameRawReceipt(originalReceipt, acknowledgedReceipt) || len(originalReceipt) == 0 {
		t.Fatal("replay changed immutable request or committed receipt")
	}
	after := assertUsage(t, target, path, 1, 120)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatal("retry changed identities")
	}
	assertAcknowledged(t, c)
}

func TestLocalAndRemoteBinariesCannotOwnSameDatabase(t *testing.T) {
	t.Run("remote_then_local", func(t *testing.T) {
		target, path := startRemote(t)
		c := newClient(t)
		if output, err := c.run(t, "service", "start", "--server-db-path", path, "--port", "0"); err == nil {
			t.Fatal("local replaced remote owner", output)
		}
		assertUsage(t, target, path, 0, 0)
	})
	t.Run("local_then_remote", func(t *testing.T) {
		c := newClient(t)
		c.must(t, "config", "set", "port", "0")
		c.must(t, "service", "start")
		t.Cleanup(func() { c.must(t, "service", "stop") })
		path := filepath.Join(c.root, "data", "tokeninsights", "server.duckdb")
		command := exec.CommandContext(t.Context(), serverBinary, "--listen", "127.0.0.1:0", "--server-db-path", path)
		command.Env = c.env
		if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "owns database") {
			t.Fatal("remote replaced local owner", string(output), err)
		}
		c.must(t, "service", "status")
	})
}

func TestConfigDestinationSwitchingPreservesIndependentProgress(t *testing.T) {
	a, aPath := startRemote(t)
	b, bPath := startRemote(t)
	c := newClient(t)
	for _, target := range []string{a, b, a} {
		c.must(t, "config", "set", "server-url", target)
		c.sync(t)
	}
	assertUsage(t, a, aPath, 1, 120)
	assertUsage(t, b, bPath, 1, 120)
	database, err := db.OpenWritable(c.collector)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	var destinations, receipts int
	if err := database.QueryRow("SELECT COUNT(*) FROM evidence_destinations WHERE acknowledged_sequence=2").Scan(&destinations); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow("SELECT COUNT(*) FROM evidence_batches WHERE receipt_bytes IS NOT NULL").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if destinations != 2 || receipts != 2 {
		t.Fatal("destination progress", destinations, receipts)
	}
}

func TestConcurrentCopiedAndDistinctClients(t *testing.T) {
	target, path := startRemote(t)
	clients := []client{newClient(t), newClient(t), newClient(t)}
	// Third client has distinct native identities but exactly the same counters.
	body, err := os.ReadFile(filepath.Join(clients[2].source, "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(strings.ReplaceAll(string(body), "deployment-pi-session", "deployment-other-session"), "deployment-message", "deployment-other-message"))
	if err := os.WriteFile(filepath.Join(clients[2].source, "session.jsonl"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range clients {
		c.must(t, "config", "set", "server-url", target)
	}
	errors := make(chan error, len(clients))
	var workers sync.WaitGroup
	for _, c := range clients {
		workers.Go(func() {
			output, err := c.run(t, "sync", "--harness", "pi", "--source-dir", c.source)
			if err != nil {
				errors <- fmt.Errorf("%s: %w", output, err)
			}
		})
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	assertUsage(t, target, path, 2, 240)
	for _, c := range clients {
		assertAcknowledged(t, c)
		c.sync(t)
	}
	assertUsage(t, target, path, 2, 240)
}

func TestConfiguredLocalDefaultAndExplicitEmptyRemoteOverride(t *testing.T) {
	remote, path := startRemote(t)
	c := newClient(t)
	t.Cleanup(func() {
		output, err := c.run(t, "service", "stop")
		if err != nil {
			t.Error(output, err)
		}
	})
	c.must(t, "config", "set", "port", "0")
	c.must(t, "config", "set", "host", "0.0.0.0")
	c.must(t, "config", "set", "server-url", remote)
	c.must(t, "sync", "--harness", "pi", "--source-dir", c.source, "--server-url=")
	var state service.State
	if err := json.Unmarshal([]byte(c.must(t, "service", "status", "--json")), &state); err != nil {
		t.Fatal(err)
	}
	if !state.Running || state.Record == nil || !strings.HasPrefix(state.Record.Address, "0.0.0.0:") {
		t.Fatal("configured local bind", state)
	}
	assertUsage(t, state.Record.URL, filepath.Join(c.root, "data", "tokeninsights", "server.duckdb"), 1, 120)
	assertUsage(t, remote, path, 0, 0)
	c.env = append(c.env, "TOKENINSIGHTS_SERVER_URL=")
	c.sync(t)
	assertUsage(t, remote, path, 0, 0)
}

func TestAllHarnessesPublishThroughConfiguredRemoteBinaryAndRebuild(t *testing.T) {
	target, path := startRemote(t)
	a, b := newClient(t), newClient(t)
	for _, c := range []client{a, b} {
		if err := os.CopyFS(c.source, os.DirFS("../../testdata/conformance/collector-rebuild/source")); err != nil {
			t.Fatal(err)
		}
		sourceSQL, err := os.ReadFile(filepath.Join(c.source, "opencode", "source.sql"))
		if err != nil {
			t.Fatal(err)
		}
		sourceDB, err := sql.Open("sqlite", filepath.Join(c.source, "opencode", "opencode.db"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sourceDB.Exec(string(sourceSQL)); err != nil {
			t.Fatal(err)
		}
		if err := sourceDB.Close(); err != nil {
			t.Fatal(err)
		}
		c.must(t, "config", "set", "server-url", target)
		// Remote routing must not inspect the configured local database, even aliases.
		c.must(t, "config", "set", "server-db-path", c.collector)
		c.must(t, "sync", "--all", "--source-dir", c.source)
		assertComponents(t, target, path, [7]int64{12, 800, 148, 52, 96, 6, 1102})
	}
	before := assertComponents(t, target, path, [7]int64{12, 800, 148, 52, 96, 6, 1102})
	a.must(t, "collector", "reset-all", "--confirm")
	a.must(t, "sync", "--all", "--source-dir", a.source)
	after := assertComponents(t, target, path, [7]int64{12, 800, 148, 52, 96, 6, 1102})
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatal("all-harness rebuild changed identities")
	}
}

func sameRawReceipt(left, right []byte) bool {
	var original, acknowledged evidence.Response
	return json.Unmarshal(left, &original) == nil && json.Unmarshal(right, &acknowledged) == nil && original.Receipt == acknowledged.Receipt
}
