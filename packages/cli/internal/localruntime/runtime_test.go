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

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/duckdb"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
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
	collectorPath, dataPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "data.duckdb")
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
	runtime, err := localruntime.Open(t.Context(), filepath.Join(root, "collector.sqlite"), filepath.Join(root, "data.duckdb"))
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
	if hostname, err := os.Hostname(); err != nil || instance.Hostname != hostname || instance.InstanceId != runtime.InstanceID {
		t.Fatal("direct local identity unavailable", instance, err)
	}
	remote := httptest.NewServer(server.NewDataHandlerWithOptions(ctx, duckdb.Source{Store: runtime.Store}, io.Discard, server.DataHandlerOptions{Host: "127.0.0.1", InstanceID: runtime.InstanceID, Hostname: runtime.Hostname, Policy: runtime.Policy, Progress: runtime.Progress}))
	network, err := queryclient.New(remote.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	remoteInstance, err := network.Instance(ctx)
	if err != nil || remoteInstance.Hostname != instance.Hostname || remoteInstance.InstanceId != instance.InstanceId {
		t.Fatal("direct and HTTP identity differ", instance, remoteInstance, err)
	}
	period := api.PeriodFilter("all")
	params := api.GetUsageParams{Period: &period}
	for _, tab := range []api.UsageTab{"tokens", "models", "providers", "harnesses", "sessions", "context", "repo"} {
		params.Tab = &tab
		direct, err := runtime.Query.AllUsage(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		response, err := network.AllUsage(ctx, params)
		if err != nil || !reflect.DeepEqual(direct, response) {
			t.Fatalf("%s query differs: direct=%+v remote=%+v error=%v", tab, direct, response, err)
		}
	}
	filters := api.GetUsageFacetsParams{Period: &period}
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
	if err != nil || (result.FactCount == nil || *result.FactCount != 1) {
		t.Fatalf("direct query required listener: %+v %v", result, err)
	}
}

func TestDerivedRoleAliasesRejectBeforeCreatingStorage(t *testing.T) {
	root := t.TempDir()
	collectorPath := filepath.Join(root, "collector.sqlite")
	for _, paths := range [][2]string{{collectorPath + ".jobs.sqlite", filepath.Join(root, "app.sqlite")}, {filepath.Join(root, "data.duckdb"), collectorPath + ".jobs.sqlite"}, {filepath.Join(root, "data.duckdb"), filepath.Join(root, "data.duckdb.application.json")}} {
		runtime, err := localruntime.OpenWithApp(t.Context(), collectorPath, paths[0], paths[1])
		if err == nil {
			_ = runtime.Close()
			t.Fatal("derived role alias allowed")
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("alias rejection mutated storage", entries, err)
	}
}

func TestStartupFailureReleasesDatabaseOwnership(t *testing.T) {
	root := t.TempDir()
	collectorPath, dataPath, appPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "data.duckdb"), filepath.Join(root, "app.sqlite")
	occupied := []byte("unrelated file")
	if err := os.WriteFile(appPath, occupied, 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime, err := localruntime.OpenWithApp(t.Context(), collectorPath, dataPath, appPath); err == nil {
		_ = runtime.Close()
		t.Fatal("opened invalid application database")
	}
	if contents, err := os.ReadFile(appPath); err != nil || string(contents) != string(occupied) {
		t.Fatal("startup mutated unrelated file", err)
	}
	if err := os.Remove(appPath); err != nil {
		t.Fatal(err)
	}
	runtime, err := localruntime.OpenWithApp(t.Context(), collectorPath, dataPath, appPath)
	if err != nil {
		t.Fatal("failed startup leaked storage or ownership", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
}
