package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
)

func TestDirectCompleteQueryMatchesHTTPPagination(t *testing.T) {
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "server.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	metadata, err := store.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	const records = 205
	batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: records}
	for i := range records {
		record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: int64(i + 1), Data: json.RawMessage(fmt.Sprintf(`{"type":"message","id":"message-%d","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"model-%03d","usage":{"input":100,"output":20}}}`, i, i)), Context: []evidence.Context{{Ordinal: 0, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
		batch.Entries = append(batch.Entries, evidence.Entry{Sequence: int64(i + 1), Record: record})
	}
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Accept(t.Context(), body); err != nil {
		t.Fatal(err)
	}
	for {
		worked, err := store.ProcessNext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	direct := server.NewDirectQuery(analytics.DuckDB{Store: store}, "instance", "")
	local := queryclient.NewDirect(direct)
	httpServer := httptest.NewServer(server.NewDataHandler(t.Context(), store, nil, "127.0.0.1", "instance", false))
	t.Cleanup(httpServer.Close)
	remote, err := queryclient.New(httpServer.URL, httpServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	period, tab, sort := api.Period("all"), api.UsageTab("models"), api.SortField("name")
	params := api.GetUsageParams{Period: &period, Tab: &tab, Sort: &sort}
	want, err := remote.AllUsage(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	got, err := local.AllUsage(t.Context(), params)
	if err != nil || !reflect.DeepEqual(got, want) || len(got.Rows) != records || got.Summary.Total != records*120 {
		t.Fatalf("direct/HTTP results differ: got %+v want %+v error %v", got, want, err)
	}
	if result, err := direct.AllUsage(t.Context(), params, records-1); err == nil || len(result.Rows) != 0 {
		t.Fatal("complete query ignored row bound", result, err)
	}
	invalid := params
	invalidTab := api.UsageTab("invalid")
	invalid.Tab = &invalidTab
	if _, err := local.AllUsage(t.Context(), invalid); err == nil {
		t.Fatal("complete query bypassed shared validation")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if result, err := local.AllUsage(ctx, params); err == nil || len(result.Rows) != 0 {
		t.Fatal("canceled complete query returned rows", result, err)
	}
	response, err := httpServer.Client().Get(httpServer.URL + "/api/v2/usage?period=all&tab=models&pageSize=201")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatal("HTTP page bound changed", response.StatusCode)
	}
}
