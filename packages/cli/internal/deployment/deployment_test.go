package deployment_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecontract"
)

var clientBinary, serverBinary string
var remoteTokens sync.Map
var deploymentHTTP = &http.Client{Timeout: 5 * time.Second}

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
	if len(args) > 0 && args[0] == "sync" {
		args = append([]string{"sync", "--wait"}, args[1:]...)
	}
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
	if len(args) == 4 && args[0] == "config" && args[1] == "set" && args[2] == "server-url" {
		if token, ok := remoteTokens.Load(args[3]); ok {
			command := exec.Command(clientBinary, "--config-file", c.config, "config", "set", "server-token")
			command.Env = c.env
			command.Stdin = strings.NewReader(token.(string) + "\n")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatal(string(output), err)
			}
		}
	}
	return body
}
func (c client) sync(t *testing.T) {
	t.Helper()
	c.must(t, "sync", "--harness", "pi", "--source-dir", c.source)
}

func startRemote(t *testing.T) (string, string) {
	t.Helper()
	r := newRemote(t, "sqlite")
	return r.target, r.path
}

func assertUsage(t *testing.T, target string, facts, total int64) []string {
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
	return assertComponents(t, target, [7]int64{facts, fixture.Input * facts, fixture.Output * facts, fixture.Reasoning * facts, fixture.CacheRead * facts, fixture.CacheWrite * facts, total})
}
func assertComponents(t *testing.T, target string, want [7]int64) []string {
	t.Helper()

	data := readUsage(t, target)
	if data.FactCount == nil || *data.FactCount != want[0] || data.Summary.Total != want[6] || data.Summary.Input != want[1] || data.Summary.Output != want[2] || data.Summary.Reasoning != want[3] || data.Summary.CacheRead != want[4] || data.Summary.CacheWrite != want[5] {
		t.Fatal("REST component/count oracle", data)
	}
	ids := []string{}
	for _, row := range data.Rows {
		ids = append(ids, row.Key)
	}

	return ids
}

func readUsage(t *testing.T, target string) api.UsageResponseV2 {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var data api.UsageResponseV2
	for {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target+"/api/v2/usage?period=all&tab=sessions", nil)
		if err != nil {
			t.Fatal(err)
		}
		if token, ok := remoteTokens.Load(target); ok {
			request.Header.Set("Authorization", "Bearer "+token.(string))
		}
		response, err := deploymentHTTP.Do(request)
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
	return data
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
	if _, err := os.Stat(filepath.Join(c.root, "data", "tokeninsights", "server.sqlite")); !os.IsNotExist(err) {
		t.Fatal("remote sync created local server", err)
	}
	if _, err := os.Stat(filepath.Join(c.root, "state")); !os.IsNotExist(err) {
		t.Fatal("remote sync created lifecycle state", err)
	}
}

// Simulate collector loss after foreground submission has closed storage.
// Keep ownership lock inodes and server history intact.
func (c client) deleteCollector(t *testing.T) {
	t.Helper()
	if err := os.Remove(c.collector); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(c.collector + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
}

func TestConfigRemoteSyncTracerAndCopiedClients(t *testing.T) {
	runRemoteBackends(t, configRemoteSyncTracerAndCopiedClients)
}

func configRemoteSyncTracerAndCopiedClients(t *testing.T, remote *remoteServer) {
	target := remote.target
	a, b := newClient(t), newClient(t)
	for _, c := range []client{a, b} {
		c.must(t, "config", "set", "server-url", target)
		c.sync(t)
		assertUsage(t, target, 1, 120)
		assertAcknowledged(t, c)
	}
	before := assertUsage(t, target, 1, 120)
	a.sync(t)
	after := assertUsage(t, target, 1, 120)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatal("repeat changed stable identities")
	}
	if output := a.must(t); !strings.Contains(output, "usage: tokeninsights") {
		t.Fatal("bare invocation", output)
	}
	a.deleteCollector(t)
	a.sync(t)
	after = assertUsage(t, target, 1, 120)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatal("rebuild changed stable identities")
	}
}

func TestRemoteFailureRetainsJournalAndRestartResumesAcceptedWork(t *testing.T) {
	runRemoteBackends(t, remoteFailureRetainsJournalAndRestartResumesAcceptedWork)
}

func remoteFailureRetainsJournalAndRestartResumesAcceptedWork(t *testing.T, remote *remoteServer) {
	target := remote.target
	upstream, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	var failFirst atomic.Bool
	failFirst.Store(true)
	var mu sync.Mutex
	var requests [][]byte
	forwarder := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			_ = r.Body.Close()
			if err != nil {
				http.Error(w, "request unreadable", 400)
				return
			}
			mu.Lock()
			requests = append(requests, body)
			mu.Unlock()
			r.Body = io.NopCloser(bytes.NewReader(body))
			if failFirst.Load() {
				hijacker, ok := w.(http.Hijacker)
				if !ok {
					t.Error("missing hijacker")
					return
				}
				connection, _, err := hijacker.Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = connection.Close()
				return
			}
		}
		r.Host = upstream.Host
		proxy.ServeHTTP(w, r)
	}))
	defer forwarder.Close()
	secret, _ := remoteTokens.Load(target)
	remoteTokens.Store(forwarder.URL, secret)
	defer remoteTokens.Delete(forwarder.URL)
	c := newClient(t)
	c.must(t, "config", "set", "server-url", forwarder.URL)
	if output, err := c.run(t, "sync", "--harness", "pi", "--source-dir", c.source); err == nil || !strings.Contains(output, "transport_failed") {
		t.Fatal(output, err)
	}
	database, err := db.OpenWritable(c.collector)
	if err != nil {
		t.Fatal(err)
	}
	var journal, cursor, receipts int
	var saved []byte
	if err := database.QueryRow("SELECT COUNT(*) FROM evidence_outbox").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow("SELECT acknowledged_sequence FROM evidence_destinations").Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow("SELECT COUNT(*) FROM evidence_batches WHERE receipt_bytes IS NOT NULL").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow("SELECT request_bytes FROM evidence_batches WHERE receipt_bytes IS NULL").Scan(&saved); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	if journal != 2 || cursor != 0 || receipts != 0 {
		t.Fatal("work lost or prematurely acknowledged", journal, cursor, receipts)
	}
	assertUsage(t, target, 0, 0)
	// Commit the exact retained request with processing stopped, then let only
	// the real server binary resume it. The client still lacks its receipt.
	var committed evidence.Response
	remote.offlineDataset(t, func(d storagecontract.Dataset) {
		var err error
		committed, err = d.Receiver.Accept(t.Context(), evidence.ProtocolVersion, saved)
		if err != nil || committed.Processing.Pending == 0 {
			t.Fatal("fixture must contain accepted, unprocessed work", committed, err)
		}
	})
	remote.start(t)
	before := assertUsage(t, target, 1, 120)
	failFirst.Store(false)
	c.must(t, "sync", "--publish-only")
	if after := assertUsage(t, target, 1, 120); !slices.Equal(before, after) {
		t.Fatal("replay changed identities")
	}
	assertAcknowledged(t, c)
	database, err = db.OpenWritable(c.collector)
	if err != nil {
		t.Fatal(err)
	}
	var acknowledged []byte
	err = database.QueryRow("SELECT receipt_bytes FROM evidence_batches").Scan(&acknowledged)
	_ = database.Close()
	if err != nil {
		t.Fatal(err)
	}
	var response evidence.Response
	if err := json.Unmarshal(acknowledged, &response); err != nil || response.Receipt != committed.Receipt {
		t.Fatal("restart changed committed receipt", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 {
		t.Fatal("retry count", len(requests))
	}
	for _, request := range requests {
		if !bytes.Equal(request, saved) {
			t.Fatal("retry changed immutable request")
		}
	}
}

func TestUnreachableRemoteFailsPreflightWithoutCollectionOrLocalServer(t *testing.T) {
	c := newClient(t)
	c.must(t, "config", "set", "server-url", "http://127.0.0.1:1")
	c.env = append(c.env, "TOKENINSIGHTS_ACCESS_TOKEN=fixture-token")
	if output, err := c.run(t, "sync", "--harness", "pi", "--source-dir", c.source); err == nil {
		t.Fatal("unreachable descriptor accepted", output)
	}
	for _, path := range []string{c.collector, filepath.Join(c.root, "data", "tokeninsights", "server.sqlite"), filepath.Join(c.root, "state")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("preflight created collector/local state", path, err)
		}
	}
}

func TestRemoteCommittedResponseLostReplaysExactRequestAndReceipt(t *testing.T) {
	runRemoteBackends(t, remoteCommittedResponseLostReplaysExactRequestAndReceipt)
}

func remoteCommittedResponseLostReplaysExactRequestAndReceipt(t *testing.T, remote *remoteServer) {
	target := remote.target
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
		if loseFirst.Load() {
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				return err
			}
			requestsMu.Lock()
			if len(originalReceipt) == 0 {
				originalReceipt = body
			}
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
	secret, _ := remoteTokens.Load(target)
	remoteTokens.Store(forwarder.URL, secret)
	defer remoteTokens.Delete(forwarder.URL)
	c := newClient(t)
	c.must(t, "config", "set", "server-url", forwarder.URL)
	if output, err := c.run(t, "sync", "--harness", "pi", "--source-dir", c.source); err == nil {
		t.Fatal("lost receipt acknowledged", output)
	}
	before := assertUsage(t, target, 1, 120)
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
	remote.stop()
	remote.start(t) // Same endpoint, persisted accounts, dataset and receipt.
	loseFirst.Store(false)
	c.must(t, "sync", "--publish-only")
	if err := database.QueryRow("SELECT receipt_bytes FROM evidence_batches").Scan(&acknowledgedReceipt); err != nil {
		t.Fatal(err)
	}
	requestsMu.Lock()
	defer requestsMu.Unlock()
	if len(requests) != 4 || !bytes.Equal(requests[0], savedRequest) || !bytes.Equal(requests[1], savedRequest) || !sameRawReceipt(originalReceipt, acknowledgedReceipt) || len(originalReceipt) == 0 {
		t.Fatal("replay changed immutable request or committed receipt")
	}
	for _, request := range requests {
		if !bytes.Equal(request, savedRequest) {
			t.Fatal("retry changed request bytes")
		}
	}
	after := assertUsage(t, target, 1, 120)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatal("retry changed identities")
	}
	assertAcknowledged(t, c)
}

func TestLocalAndRemoteBinariesCannotOwnSameDatabase(t *testing.T) {
	t.Run("remote_then_local", func(t *testing.T) {
		target, path := startRemote(t)
		c := newClient(t)
		if output, err := c.run(t, "web", "--sync=false", "--open=false", "--server-db-path", path, "--port", "0"); err == nil {
			t.Fatal("local replaced remote owner", output)
		}
		assertUsage(t, target, 0, 0)
	})
	t.Run("local_then_remote", func(t *testing.T) {
		c := newClient(t)
		c.must(t, "config", "set", "port", "0")
		localURL := startLocal(t, c)
		path := filepath.Join(c.root, "data", "tokeninsights", "server.sqlite")
		command := exec.CommandContext(t.Context(), serverBinary, "--public-url", "https://usage.example", "--listen", "127.0.0.1:0", "--server-db-path", path)
		command.Env = c.env
		if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "owns database") {
			t.Fatal("remote replaced local owner", string(output), err)
		}
		assertUsage(t, localURL, 0, 0)
	})
}

func TestConfigDestinationSwitchingPreservesIndependentProgress(t *testing.T) {
	a, _ := startRemote(t)
	b, _ := startRemote(t)
	c := newClient(t)
	for _, target := range []string{a, b, a} {
		c.must(t, "config", "set", "server-url", target)
		c.sync(t)
	}
	assertUsage(t, a, 1, 120)
	assertUsage(t, b, 1, 120)
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
	runRemoteBackends(t, concurrentCopiedAndDistinctClients)
}

func concurrentCopiedAndDistinctClients(t *testing.T, remote *remoteServer) {
	target := remote.target
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
	assertUsage(t, target, 2, 240)
	for _, c := range clients {
		assertAcknowledged(t, c)
		c.sync(t)
	}
	assertUsage(t, target, 2, 240)
}

func TestConfiguredLocalDefaultAndExplicitEmptyRemoteOverride(t *testing.T) {
	remote, _ := startRemote(t)
	c := newClient(t)
	c.must(t, "config", "set", "port", "0")
	c.must(t, "config", "set", "host", "0.0.0.0")
	c.must(t, "config", "set", "server-url", remote)
	c.env = append(c.env, "TOKENINSIGHTS_ACCESS_TOKEN=")
	c.must(t, "sync", "--harness", "pi", "--source-dir", c.source, "--server-url=")
	localURL := startLocal(t, c)
	assertUsage(t, localURL, 1, 120)
	assertUsage(t, remote, 0, 0)
	c.env = append(c.env, "TOKENINSIGHTS_SERVER_URL=")
	c.sync(t) // Active web owner consumes this request directly.
	assertUsage(t, localURL, 1, 120)
	assertUsage(t, remote, 0, 0)
}

func startLocal(t *testing.T, c client) string {
	t.Helper()
	command := exec.Command(clientBinary, "--config-file", c.config, "web", "--sync=false", "--open=false", "--port", "0", "--server-url=")
	command.Env = c.env
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
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
		case <-time.After(10 * time.Second):
			_ = command.Process.Kill()
			<-done
			t.Error("local shutdown timeout")
		}
	})
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		if scanner.Scan() {
			ready <- strings.TrimPrefix(scanner.Text(), "Dashboard: ")
		}
	}()
	select {
	case target := <-ready:
		return target
	case <-time.After(10 * time.Second):
		t.Fatal("local readiness timeout")
	}
	return ""
}

func TestAllHarnessesPublishThroughConfiguredRemoteBinaryAndRebuild(t *testing.T) {
	runRemoteBackends(t, allHarnessesPublishThroughConfiguredRemoteBinaryAndRebuild)
}

func allHarnessesPublishThroughConfiguredRemoteBinaryAndRebuild(t *testing.T, remote *remoteServer) {
	target := remote.target
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
		assertComponents(t, target, [7]int64{12, 800, 148, 52, 96, 6, 1102})
	}
	before := assertComponents(t, target, [7]int64{12, 800, 148, 52, 96, 6, 1102})
	receipts := append(collectorReceipts(t, a), collectorReceipts(t, b)...)
	factsBefore := receiptFactIDs(t, target, receipts)
	a.deleteCollector(t)
	a.must(t, "sync", "--all", "--source-dir", a.source)
	after := assertComponents(t, target, [7]int64{12, 800, 148, 52, 96, 6, 1102})
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatal("all-harness rebuild changed identities")
	}
	receipts = append(receipts, collectorReceipts(t, a)...)
	if after := receiptFactIDs(t, target, receipts); !slices.Equal(factsBefore, after) {
		t.Fatal("collector rebuild changed native fact identities", factsBefore, after)
	}
	// Stage replacement with no worker, then recover all native harnesses using
	// the server alone. Preserve every old receipt while projections are replaced.
	var generation int64
	remote.offlineDataset(t, func(d storagecontract.Dataset) {
		var err error
		generation, err = d.Reprocess(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, found, err := d.Processing.LoadWork(t.Context()); err != nil || !found {
			t.Fatal("fixture must contain pending replacement work", found, err)
		}
	})
	remote.start(t)
	after = assertComponents(t, target, [7]int64{12, 800, 148, 52, 96, 6, 1102})
	if !slices.Equal(before, after) || !slices.Equal(factsBefore, receiptFactIDs(t, target, receipts)) {
		t.Fatal("replacement changed session or native fact identities")
	}
	data := readUsage(t, target)
	if data.Generation == nil || *data.Generation != generation {
		t.Fatal("restart did not activate replacement generation", data)
	}
}

func collectorReceipts(t *testing.T, c client) []evidence.Receipt {
	t.Helper()
	database, err := db.OpenWritable(c.collector)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	rows, err := database.Query("SELECT receipt_bytes FROM evidence_batches")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var receipts []evidence.Receipt
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			t.Fatal(err)
		}
		var saved evidence.Response
		if err := json.Unmarshal(body, &saved); err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, saved.Receipt)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(receipts) == 0 {
		t.Fatal("collector has no acknowledged batches")
	}
	return receipts
}

func receiptFactIDs(t *testing.T, target string, receipts []evidence.Receipt) []string {
	t.Helper()
	// Copies may select a witness from any batch. Collect confirmed identities
	// across all receipts; excluded candidates also carry IDs but are not facts.
	ids := map[string]bool{}
	for _, receipt := range receipts {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			target+"/api/v3/ingestion/batches/"+url.PathEscape(receipt.StreamID)+"/"+url.PathEscape(receipt.BatchID), nil)
		if err != nil {
			t.Fatal(err)
		}
		secret, ok := remoteTokens.Load(target)
		if !ok {
			t.Fatal("missing hosted credential")
		}
		request.Header.Set("Authorization", "Bearer "+secret.(string))
		response, err := deploymentHTTP.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var current evidence.Response
		err = json.NewDecoder(response.Body).Decode(&current)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || current.Receipt != receipt || current.Processing.Pending != 0 {
			t.Fatal("receipt changed or processing incomplete", current, response.StatusCode, err)
		}
		for _, item := range current.Processing.Items {
			if item.Disposition == "processed" && item.FactID != "" {
				ids[item.FactID] = true
			}
		}
	}
	if len(ids) != 12 {
		t.Fatal("all-harness confirmed receipt fact count", len(ids))
	}
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	slices.Sort(result)
	return result
}

func sameRawReceipt(left, right []byte) bool {
	var original, acknowledged evidence.Response
	return json.Unmarshal(left, &original) == nil && json.Unmarshal(right, &acknowledged) == nil && original.Receipt == acknowledged.Receipt
}
