package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/datastore"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/core/server"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
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
	direct := application.NewQueries(sqlanalytics.Queries{Store: store}, policy, "instance", "")
	local := queryclient.NewDirect(direct)
	httpServer := httptest.NewServer(server.NewDataHandler(t.Context(), sqlanalytics.Source{Store: store}, nil, "127.0.0.1", "instance"))
	t.Cleanup(httpServer.Close)
	remote, err := queryclient.New(httpServer.URL, httpServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	instance, err := local.Instance(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	advertised := append(serverfeatures.Capabilities{}, policy.Capabilities...)
	if !reflect.DeepEqual(instance.Capabilities, advertised) || string(instance.Kind) != string(policy.Kind) || instance.InstanceID != "instance" {
		t.Fatalf("direct instance ignores composition policy: %+v", instance)
	}
	period, tab, sort := "all", "models", "name"
	params := analytics.Query{Selection: analytics.Selection{Period: period}, Tab: tab, Sort: sort}
	want, err := remote.AllUsage(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	got, err := local.AllUsage(t.Context(), params)
	if err != nil || !reflect.DeepEqual(got, want) || len(got.Rows) != records || got.Summary.TotalTokens != records*120 {
		t.Fatalf("direct/HTTP results differ: got %+v want %+v error %v", got, want, err)
	}
	// The local terminal dashboard uses the direct reader; every viewer query shape
	// must match what HTTP clients see for the same snapshot.
	provider, model := []string{"openai"}, []string{"model-007"}
	tabs := []string{"tokens", "models", "providers", "harnesses", "sessions", "context", "repo"}
	for _, tab := range tabs {
		for _, bucket := range []string{"day", "week", "month", "year"} {
			for _, filtered := range []bool{false, true} {
				shape := analytics.Query{Selection: analytics.Selection{Period: period, Bucket: bucket}, Tab: tab}
				if filtered {
					shape.Selection.Providers, shape.Selection.Models = provider, model
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
		facets := application.FacetRequest{Query: analytics.Query{Selection: analytics.Selection{Period: period}, Tab: tab}}
		want, err := remote.Facets(t.Context(), facets)
		if err != nil {
			t.Fatal(tab, err)
		}
		if got, err := local.Facets(t.Context(), facets); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: direct/HTTP facets differ: got %+v want %+v error %v", tab, got, want, err)
		}
	}
	localStatus, err := local.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	remoteStatus, err := remote.Status(t.Context())
	if err != nil || !reflect.DeepEqual(localStatus, remoteStatus) {
		t.Fatalf("status differs: direct=%+v HTTP=%+v error=%v", localStatus, remoteStatus, err)
	}
	for _, page := range []int{1, 2, 999} {
		shape := params
		shape.Page, shape.PageSize = page, 2
		want, err := remote.Usage(t.Context(), shape)
		if err != nil {
			t.Fatal(err)
		}
		got, err := local.Usage(t.Context(), shape)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("page %d differs: direct=%+v HTTP=%+v error=%v", page, got, want, err)
		}
	}
	for _, client := range []*queryclient.Client{local, remote} {
		invalid := params
		invalid.PageSize = 201
		var invalidRequest *application.InvalidRequest
		if _, err := client.Usage(t.Context(), invalid); !errors.As(err, &invalidRequest) {
			t.Fatalf("invalid query classification: %v", err)
		}
		if result, err := client.WithDataset("foreign").Usage(t.Context(), params); !errors.Is(err, queryclient.ErrSnapshotChanged) || len(result.Rows) != 0 {
			t.Fatalf("dataset pin ignored: %+v, %v", result, err)
		}
	}
	if result, err := direct.AllUsage(t.Context(), params, records-1); err == nil || len(result.Rows) != 0 {
		t.Fatal("complete query ignored row bound", result, err)
	}
	invalid := params
	invalid.Tab = "invalid"
	if _, err := local.AllUsage(t.Context(), invalid); err == nil {
		t.Fatal("complete query bypassed shared validation")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if result, err := local.AllUsage(ctx, params); err == nil || len(result.Rows) != 0 {
		t.Fatal("canceled complete query returned rows", result, err)
	}
	if result, err := remote.AllUsage(ctx, params); !errors.Is(err, context.Canceled) || len(result.Rows) != 0 {
		t.Fatal("HTTP query ignored cancellation", result, err)
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
