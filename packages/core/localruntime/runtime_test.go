package localruntime_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/datastore"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
	"github.com/flexdinesh/tokeninsights/packages/core/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/core/server"
)

func pendingBatch(t *testing.T, store *datastore.Store) evidence.Receipt {
	t.Helper()
	metadata, err := store.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 2,
		Data:    json.RawMessage(`{"type":"message","id":"message","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":100,"output":20}}}`),
		Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
	batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: 1, Entries: []evidence.Entry{{Sequence: 1, Record: record}}}
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	response, err := store.Accept(t.Context(), evidence.ProtocolVersion, body)
	if err != nil {
		t.Fatal(err)
	}
	return response.Receipt
}

func TestRuntimeOwnsDatabaseResumesProcessingAndReleases(t *testing.T) {
	root := t.TempDir()
	collectorPath, dataPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "data.sqlite")
	store, err := datastore.Open(t.Context(), dataPath)
	if err != nil {
		t.Fatal(err)
	}
	receipt := pendingBatch(t, store)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, err := localruntime.Open(t.Context(), collectorPath, dataPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	if second, err := localruntime.Open(t.Context(), collectorPath, dataPath); !errors.Is(err, localruntime.ErrOwned) {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("second owner allowed: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := runtime.WaitVisible(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := runtime.Destination.Transport.Receipt(ctx, "stream", "batch")
	if err != nil || after.Receipt != receipt || after.Processing.Pending != 0 {
		t.Fatalf("restart lost acceptance: %+v %v", after, err)
	}
	if _, err := runtime.Store.Reprocess(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WaitVisible(ctx); err != nil {
		t.Fatal(err)
	}
	metadata, err := runtime.Store.Metadata(ctx)
	if err != nil || metadata.Generation != metadata.TargetGeneration || metadata.Generation <= 1 {
		t.Fatalf("generation not visible: %+v %v", metadata, err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := localruntime.Open(t.Context(), collectorPath, dataPath)
	if err != nil {
		t.Fatalf("ownership leaked: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDirectQueriesMatchHTTPWithoutNetworkDependency(t *testing.T) {
	root := t.TempDir()
	runtime, err := localruntime.Open(t.Context(), filepath.Join(root, "collector.sqlite"), filepath.Join(root, "data.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	pendingBatch(t, runtime.Store)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := runtime.WaitVisible(ctx); err != nil {
		t.Fatal(err)
	}
	instance, err := runtime.Query.Instance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if hostname, err := os.Hostname(); err != nil || instance.Hostname != hostname || instance.InstanceID != runtime.InstanceID {
		t.Fatal("direct local identity unavailable", instance, err)
	}
	remote := httptest.NewServer(server.NewDataHandlerWithOptions(ctx, sqlanalytics.Source{Store: runtime.Store}, io.Discard, server.DataHandlerOptions{Host: "127.0.0.1", InstanceID: runtime.InstanceID, Hostname: runtime.Hostname, Policy: runtime.Policy, Progress: runtime.Progress}))
	network, err := queryclient.New(remote.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	remoteInstance, err := network.Instance(ctx)
	if err != nil || remoteInstance.Hostname != instance.Hostname || remoteInstance.InstanceID != instance.InstanceID {
		t.Fatal("direct and HTTP identity differ", instance, remoteInstance, err)
	}
	period := "all"
	params := analytics.Query{Selection: analytics.Selection{Period: period}}
	for _, tab := range []string{"tokens", "models", "providers", "harnesses", "sessions", "context", "repo"} {
		params.Tab = tab
		direct, err := runtime.Query.AllUsage(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		response, err := network.AllUsage(ctx, params)
		if err != nil || !reflect.DeepEqual(direct, response) {
			t.Fatalf("%s query differs: direct=%+v remote=%+v error=%v", tab, direct, response, err)
		}
	}
	filters := application.FacetRequest{Query: analytics.Query{Selection: analytics.Selection{Period: period}}}
	directFacets, err := runtime.Query.Facets(ctx, filters)
	if err != nil {
		t.Fatal(err)
	}
	remoteFacets, err := network.Facets(ctx, filters)
	if err != nil || !reflect.DeepEqual(directFacets, remoteFacets) {
		t.Fatalf("facets differ: %+v %+v %v", directFacets, remoteFacets, err)
	}
	remote.Close()
	result, err := runtime.Query.AllUsage(ctx, params)
	if err != nil || result.FactCount != 1 {
		t.Fatalf("direct query required listener: %+v %v", result, err)
	}
}

func TestSQLiteSidecarRoleAliasesRejectBeforeCreatingStorage(t *testing.T) {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		t.Run(suffix, func(t *testing.T) {
			root := t.TempDir()
			collector := filepath.Join(root, "collector.sqlite")
			data := filepath.Join(root, "server.sqlite")
			for _, pair := range [][2]string{{collector, collector + suffix}, {data + suffix, data}} {
				if runtime, err := localruntime.Open(t.Context(), pair[0], pair[1]); err == nil {
					_ = runtime.Close()
					t.Fatal("SQLite sidecar alias accepted")
				}
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatal("sidecar alias mutated storage", entries, err)
			}
		})
	}
}

func TestStartupFailureReleasesDatabaseOwnership(t *testing.T) {
	root := t.TempDir()
	collectorPath, dataPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "data.sqlite")
	occupied := []byte("unrelated file")
	if err := os.WriteFile(dataPath, occupied, 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime, err := localruntime.Open(t.Context(), collectorPath, dataPath); err == nil {
		_ = runtime.Close()
		t.Fatal("opened invalid token database")
	}
	if contents, err := os.ReadFile(dataPath); err != nil || string(contents) != string(occupied) {
		t.Fatal("startup mutated unrelated file", err)
	}
	if err := os.Remove(dataPath); err != nil {
		t.Fatal(err)
	}
	runtime, err := localruntime.Open(t.Context(), collectorPath, dataPath)
	if err != nil {
		t.Fatal("failed startup leaked ownership", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeLeavesRetiredApplicationFilesUntouched(t *testing.T) {
	root := t.TempDir()
	collectorPath, dataPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
	retired := map[string][]byte{
		"app.sqlite":                     []byte("unrelated account storage"),
		"server.sqlite.application.json": []byte("old pairing guard"),
		"collector.sqlite.jobs.sqlite":   []byte("retired jobs"),
	}
	for name, body := range retired {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runtime, err := localruntime.Open(t.Context(), collectorPath, dataPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	for name, want := range retired {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(got) != string(want) {
			t.Fatal("retired file mutated", name, err)
		}
	}
}
