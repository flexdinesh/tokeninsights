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

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

func TestDirectCompleteQueryMatchesHTTPPagination(t *testing.T) {
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "server.sqlite"))
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
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, body); err != nil {
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
	policy, err := serverfeatures.NewLocalViewer(false)
	if err != nil {
		t.Fatal(err)
	}
	direct := server.NewDirectQuery(sqlanalytics.Queries{Store: store}, policy, "instance", "")
	local := queryclient.NewDirect(direct)
	httpServer := httptest.NewServer(server.NewDataHandler(t.Context(), sqlanalytics.Source{Store: store}, nil, "127.0.0.1", "instance", false))
	t.Cleanup(httpServer.Close)
	remote, err := queryclient.New(httpServer.URL, httpServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	instance, err := local.Instance(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	advertised := make([]string, 0, len(policy.Capabilities))
	for _, capability := range policy.Capabilities {
		advertised = append(advertised, string(capability))
	}
	if !reflect.DeepEqual(instance.Capabilities, advertised) || string(instance.ServerKind) != string(policy.Kind) || instance.InstanceId != "instance" {
		t.Fatalf("direct instance ignores composition policy: %+v", instance)
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
	// The local terminal dashboard uses the direct reader; every viewer query shape
	// must match what HTTP clients see for the same snapshot.
	provider, model := []string{"openai"}, []string{"model-007"}
	tabs := []api.UsageTab{api.UsageTabTokens, api.UsageTabModels, api.UsageTabProviders, api.UsageTabHarnesses, api.UsageTabSessions, api.UsageTabContext, api.UsageTabRepo}
	for _, tab := range tabs {
		for _, bucket := range []api.Bucket{api.BucketDay, api.BucketWeek, api.BucketMonth, api.BucketYear} {
			for _, filtered := range []bool{false, true} {
				shape := api.GetUsageParams{Period: &period, Tab: &tab, Bucket: &bucket}
				if filtered {
					shape.Provider, shape.Model = &provider, &model
				}
				want, err := remote.AllUsage(t.Context(), shape)
				if err != nil {
					t.Fatal(tab, bucket, err)
				}
				got, err := local.AllUsage(t.Context(), shape)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("%s/%s filtered=%v: direct/HTTP usage differ: got %+v want %+v error %v", tab, bucket, filtered, got, want, err)
				}
			}
		}
		facets := api.GetUsageFacetsParams{Period: &period, Tab: &tab}
		want, err := remote.Facets(t.Context(), facets)
		if err != nil {
			t.Fatal(tab, err)
		}
		if got, err := local.Facets(t.Context(), facets); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: direct/HTTP facets differ: got %+v want %+v error %v", tab, got, want, err)
		}
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
