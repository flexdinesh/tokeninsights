package remoteaccess

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardproxy"
	"github.com/flexdinesh/tokeninsights/packages/core/datastore"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/core/tcptransport"
)

func TestRealBackendTLSAndBrowserPreserveSavedUsageAndReceipts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude"))
	// Seed accepted native evidence before the sole owner starts. Startup must
	// resume processing without capture or a transport-specific history copy.
	settings := localprotocol.Settings{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite")}
	store, err := datastore.Open(t.Context(), settings.ServerDBPath)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.Metadata(t.Context())
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 2,
		Data:    json.RawMessage(`{"type":"message","id":"message","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":100,"output":20,"reasoning":5,"cacheRead":30,"cacheWrite":40,"totalTokens":190}}}`),
		Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
	batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: 1, Entries: []evidence.Entry{{Sequence: 1, Record: record}}}
	encoded, err := json.Marshal(batch)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	accepted, err := store.Accept(t.Context(), evidence.ProtocolVersion, encoded)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	ready := make(chan localprotocol.Descriptor, 1)
	finished := make(chan error, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		finished <- localbackend.Serve(ctx, settings, io.Discard, func(d localprotocol.Descriptor) error { ready <- d; return nil })
	}()
	defer func() { cancel(); <-joined }()
	select {
	case <-ready:
	case err := <-finished:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	local, err := localclient.Connect(ctx, localclient.Options{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = local.Close() }()
	if err := local.WaitVisible(ctx); err != nil {
		t.Fatal(err)
	}
	gateway := startGateway(t, local)
	target := parseGateway(t, gateway)
	remote, err := localclient.ConnectRemote(ctx, tcptransport.Dialer(target), localclient.RemoteOptions{Token: target.Token})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = remote.Close() }()
	bridge, err := dashboardproxy.Start(ctx, remote.PrivateRoundTripper(), dashboardproxy.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bridge.Close() }()
	// The SPA redirects root to a dashboard tab. Reloading that URL must serve
	// the actual embedded application through both the bridge and TLS gateway.
	for _, route := range []string{"/tokens", "/sessions", "/models", "/providers", "/harnesses", "/repo", "/context"} {
		response, err := http.Get(bridge.URL + route + "?period=all")
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(body), "<!doctype html>") {
			t.Fatal("dashboard reload did not serve the embedded application", route, response.StatusCode, readErr)
		}
	}
	browser, err := queryclient.New(bridge.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	query := analytics.Query{Selection: analytics.Selection{Period: "all"}}
	baseline, err := local.Query.AllUsage(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	summary := baseline.Summary
	if baseline.DatabaseID != metadata.DatabaseID || baseline.FactCount != 1 || baseline.RowCount != 1 || summary.InputTokens != 100 || summary.OutputTokens != 15 || summary.ReasoningTokens != 5 || summary.CacheReadTokens != 30 || summary.CacheWriteTokens != 40 || summary.TotalTokens != 190 || summary.SessionCount != 1 {
		t.Fatal("saved evidence accounting changed", baseline)
	}
	for _, client := range []*queryclient.Client{remote.Query, browser} {
		usage, err := client.AllUsage(ctx, query)
		if err != nil || usage.Summary != baseline.Summary || !reflect.DeepEqual(usage.Rows, baseline.Rows) || usage.DatabaseID != baseline.DatabaseID || usage.DatasetID != baseline.DatasetID || usage.Revision != baseline.Revision || usage.InputRevision != baseline.InputRevision {
			t.Fatal("transport changed canonical usage or identity", usage, err)
		}
	}
	refresh, err := http.NewRequestWithContext(ctx, http.MethodPost, bridge.URL+"/api/viewer/refresh", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	refresh.Header.Set("Content-Type", "application/json")
	refresh.Header.Set("Origin", bridge.URL)
	response, err := http.DefaultClient.Do(refresh)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatal("real backend browser refresh rejected", response.StatusCode)
	}
	if _, err := local.Collect(ctx, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := local.WaitVisible(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := remote.Query.AllUsage(ctx, query)
	if err != nil || after.Summary != baseline.Summary || after.DatabaseID != baseline.DatabaseID {
		t.Fatal("default capture replaced saved history", after, err)
	}
	_ = bridge.Close()
	_ = gateway.Close()
	select {
	case <-remote.Done():
	case <-ctx.Done():
		t.Fatal("TLS viewer survived gateway shutdown")
	}
	if _, err := local.Query.Instance(ctx); err != nil {
		t.Fatal("remote gateway stopped local lease", err)
	}
	_ = remote.Close()
	_ = local.Close()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("final lease did not stop backend")
	}
	reopened, err := datastore.Open(ctx, settings.ServerDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	receipt, err := reopened.Receipt(ctx, "stream", "batch")
	if err != nil || receipt.Receipt != accepted.Receipt {
		t.Fatal("transport lifecycle changed accepted receipt", err)
	}
	// Retired account storage must stay absent beside the unchanged history files.
	if _, err := os.Stat(filepath.Join(root, "app.sqlite")); !os.IsNotExist(err) {
		t.Fatal("viewer created retired account storage", err)
	}
}
