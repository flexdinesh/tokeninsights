// Package storagecontract supplies reusable semantic suites for real storage
// adapters. Fixtures own setup/reopen/cleanup; assertions never execute SQL.
// Import this package only from tests.
package storagecontract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

type Dataset struct {
	Receiver   evidence.Receiver
	Queries    analytics.Repository
	Processing dataengine.Backend
	Reprocess  func(context.Context) (int64, error)
}

// Reopen closes the current fixture and opens the same durable storage. It must
// register cleanup for each opened resource, including on test failure.
type Tokens struct {
	Dataset       func(string) Dataset
	EnsureDataset func(context.Context, string) error
	Reopen        func() Tokens
}

type TokenFactory func(*testing.T) Tokens

func RunTokens(t *testing.T, factory TokenFactory) {
	t.Helper()
	for name, contract := range map[string]func(*testing.T, Tokens){
		"acceptance_replay_isolation": acceptance,
		"pending_reopen_cancellation": recovery,
		"revision_generation_fences":  fences,
		"query_components_pagination": queries,
		"concurrent_query_snapshot":   snapshots,
	} {
		t.Run(name, func(t *testing.T) { contract(t, factory(t)) })
	}
}

func dataset(t *testing.T, store Tokens, id string) Dataset {
	t.Helper()
	if err := store.EnsureDataset(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	return store.Dataset(id)
}

func record(id string) evidence.Record {
	return evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 2,
		Data:    json.RawMessage(fmt.Sprintf(`{"type":"message","id":%q,"message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":10,"output":7,"reasoning":2,"cacheRead":3,"cacheWrite":4,"totalTokens":24}}}`, id)),
		Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
}

func batch(t *testing.T, d Dataset, stream string, records ...evidence.Record) []byte {
	t.Helper()
	status, err := d.Queries.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	b := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: status.Metadata.DatabaseID, DatasetID: status.Metadata.DatasetID, StreamID: stream, BatchID: stream, FromSequence: 1, ToSequence: int64(len(records))}
	for i, r := range records {
		b.Entries = append(b.Entries, evidence.Entry{Sequence: int64(i + 1), Record: r})
	}
	body, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func accept(t *testing.T, d Dataset, body []byte) evidence.Receipt {
	t.Helper()
	r, err := d.Receiver.Accept(t.Context(), evidence.ProtocolVersion, body)
	if err != nil {
		t.Fatal(err)
	}
	return r.Receipt
}

func drain(t *testing.T, d Dataset) {
	t.Helper()
	worker := dataengine.NewWorker()
	for range 100 {
		worked, err := worker.ProcessNext(t.Context(), d.Processing)
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			return
		}
	}
	t.Fatal("processing did not drain")
}

func query() analytics.Query {
	return analytics.Query{Quality: "confirmed", Selection: viewer.Selection{Period: "all", Bucket: "day"}, Tab: "sessions", Sort: "name", Direction: "asc", Page: 1, PageSize: 50}
}

func dashboard(t *testing.T, d Dataset) analytics.Dashboard {
	t.Helper()
	result, err := d.Queries.Dashboard(t.Context(), query(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func acceptance(t *testing.T, store Tokens) {
	a, b := dataset(t, store, "alice"), dataset(t, store, "bob")
	body := batch(t, a, "batch", record("message"))
	var initial evidence.Batch
	if err := json.Unmarshal(body, &initial); err != nil {
		t.Fatal(err)
	}
	initial.FromSequence, initial.ToSequence, initial.Entries[0].Sequence = 2, 2, 2
	var err error
	body, err = json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	// Whitespace is part of accepted request identity, not a JSON re-encoding.
	body = append([]byte(" \n"), body...)
	receipt := accept(t, a, body)
	if receipt.RequestHash != publication.RequestHash(body) || receipt.Accepted != 1 {
		t.Fatal("receipt lost exact bytes/count", receipt)
	}
	if got := accept(t, a, body); got != receipt {
		t.Fatal("replay changed immutable receipt")
	}
	if _, err := b.Receiver.Accept(t.Context(), evidence.ProtocolVersion, body); err == nil {
		t.Fatal("cross-dataset acceptance")
	}
	if _, err := b.Receiver.Receipt(t.Context(), "batch", "batch"); !errors.Is(err, evidence.ErrReceiptNotFound) {
		t.Fatal("receipt leaked across datasets", err)
	}
	if _, err := a.Receiver.Receipt(t.Context(), "absent", "absent"); !errors.Is(err, evidence.ErrReceiptNotFound) {
		t.Fatal("missing receipt contract", err)
	}
	before, err := a.Queries.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	conflict := batch(t, a, "batch", record("different"))
	if _, err := a.Receiver.Accept(t.Context(), evidence.ProtocolVersion, conflict); err == nil {
		t.Fatal("batch conflict accepted")
	}
	// A new first entry followed by a conflicting saved sequence must reject
	// atomically, including the entry encountered before the conflict.
	var overlap evidence.Batch
	if err := json.Unmarshal(body, &overlap); err != nil {
		t.Fatal(err)
	}
	overlap.BatchID = "overlap"
	overlap.FromSequence = 1
	overlap.Entries = []evidence.Entry{{Sequence: 1, Record: record("new")}, {Sequence: 2, Record: record("conflict")}}
	overlap.ToSequence = 2
	encoded, err := json.Marshal(overlap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Receiver.Accept(t.Context(), evidence.ProtocolVersion, encoded); err == nil {
		t.Fatal("sequence conflict accepted")
	}
	after, err := a.Queries.Status(t.Context())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("rejection mutated publication status", before, after, err)
	}
	if _, err := a.Receiver.Receipt(t.Context(), "batch", "overlap"); !errors.Is(err, evidence.ErrReceiptNotFound) {
		t.Fatal("rejection retained receipt", err)
	}
	drain(t, a)
	if got := dashboard(t, a); got.FactCount != 1 || got.Summary.TotalTokens != 24 {
		t.Fatal("replay/conflict changed totals", got)
	}
	if got := dashboard(t, b); got.FactCount != 0 || got.InputRevision != 0 {
		t.Fatal("other dataset mutated", got)
	}
	if _, err := store.Dataset("missing").Queries.Status(t.Context()); err == nil {
		t.Fatal("missing dataset fell back")
	}
}

func recovery(t *testing.T, store Tokens) {
	a := dataset(t, store, "alice")
	body := batch(t, a, "pending", record("saved"))
	receipt := accept(t, a, body)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := a.Receiver.Accept(ctx, evidence.ProtocolVersion, batch(t, a, "cancelled", record("cancelled"))); err == nil {
		t.Fatal("cancelled acceptance succeeded")
	}
	if _, err := a.Queries.Dashboard(ctx, query(), time.Now()); err == nil {
		t.Fatal("cancelled query succeeded")
	}
	work, found, err := a.Processing.LoadWork(t.Context())
	if err != nil || !found {
		t.Fatal("pending work missing", err)
	}
	projection, err := processor.Process(t.Context(), work.Records)
	if err != nil {
		t.Fatal(err)
	}
	if published, err := a.Processing.PublishProjection(ctx, work, projection); err == nil || published {
		t.Fatal("cancelled publication committed", err)
	}
	store = store.Reopen()
	a = store.Dataset("alice")
	before, err := a.Receiver.Receipt(t.Context(), "pending", "pending")
	if err != nil || before.Receipt != receipt || before.Processing.Pending != 1 {
		t.Fatal("pending receipt lost on reopen", before, err)
	}
	if _, err := a.Receiver.Receipt(t.Context(), "cancelled", "cancelled"); !errors.Is(err, evidence.ErrReceiptNotFound) {
		t.Fatal("cancelled acceptance persisted", err)
	}
	drain(t, a)
	if got := accept(t, a, body); got != receipt {
		t.Fatal("reopen/replay changed receipt")
	}
	if got := dashboard(t, a); got.FactCount != 1 || got.Summary.TotalTokens != 24 {
		t.Fatal("recovery changed usage", got)
	}
}

func fences(t *testing.T, store Tokens) {
	a := dataset(t, store, "alice")
	accept(t, a, batch(t, a, "first", record("first")))
	work, found, err := a.Processing.LoadWork(t.Context())
	if err != nil || !found {
		t.Fatal("missing work", err)
	}
	projection, err := processor.Process(t.Context(), work.Records)
	if err != nil {
		t.Fatal(err)
	}
	accept(t, a, batch(t, a, "second", record("second")))
	if published, err := a.Processing.PublishProjection(t.Context(), work, projection); err != nil || published {
		t.Fatal("stale revision published", err)
	}
	if got := dashboard(t, a); got.FactCount != 0 {
		t.Fatal("stale projection mutated history", got)
	}
	// Capture current work so the next rejection isolates the generation fence,
	// with matching scope revisions and membership.
	work, found, err = a.Processing.LoadWork(t.Context())
	if err != nil || !found {
		t.Fatal("missing latest work", err)
	}
	projection, err = processor.Process(t.Context(), work.Records)
	if err != nil {
		t.Fatal(err)
	}
	drain(t, a)
	saved := dashboard(t, a)
	if _, err := a.Reprocess(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := dashboard(t, a); got.Generation != saved.Generation || got.Summary != saved.Summary {
		t.Fatal("rebuild withdrew saved history", got)
	}
	if published, err := a.Processing.PublishProjection(t.Context(), work, projection); err != nil || published {
		t.Fatal("old generation published", err)
	}
	drain(t, a)
	current := dashboard(t, a)
	if current.Generation <= saved.Generation || current.Summary != saved.Summary || current.Pending != 0 {
		t.Fatal("rebuild lost usage/failed activation", current)
	}
}

func queries(t *testing.T, store Tokens) {
	a := dataset(t, store, "alice")
	for i := range 3 {
		r := record(fmt.Sprint(i))
		r.Context[0].Data = json.RawMessage(fmt.Sprintf(`{"type":"session","id":"session-%d"}`, i))
		accept(t, a, batch(t, a, fmt.Sprint(i), r))
	}
	drain(t, a)
	q := query()
	q.PageSize = 2
	first, err := a.Queries.Dashboard(t.Context(), q, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s := first.Summary
	// Pi output includes reasoning: canonical output is 5 and reasoning is 2
	// per message; all five canonical components sum to 24.
	if first.FactCount != 3 || first.RowCount != 3 || len(first.Rows) != 2 || s.InputTokens != 30 || s.OutputTokens != 15 || s.ReasoningTokens != 6 || s.CacheReadTokens != 9 || s.CacheWriteTokens != 12 || s.TotalTokens != 72 || s.SessionCount != 3 {
		t.Fatal("component/pagination contract", first)
	}
	q.Page = 2
	second, err := a.Queries.Dashboard(t.Context(), q, time.Now())
	if err != nil || len(second.Rows) != 1 || second.Rows[0].Key == first.Rows[0].Key || second.Summary != first.Summary || second.Revision != first.Revision {
		t.Fatal("pagination changed snapshot/rows", second, err)
	}
	all, err := a.Queries.AllDashboard(t.Context(), q, time.Now(), 3)
	if err != nil || len(all.Rows) != 3 || all.Summary != first.Summary {
		t.Fatal("complete query differs", all, err)
	}
	if _, err := a.Queries.AllDashboard(t.Context(), q, time.Now(), 2); err == nil {
		t.Fatal("complete query silently truncated")
	}
	q.Selection.Sessions = []string{"session-1"}
	filtered, err := a.Queries.Dashboard(t.Context(), q, time.Now())
	if err != nil || filtered.FactCount != 1 || filtered.Summary.TotalTokens != 24 || filtered.Page != 1 {
		t.Fatal("filter/page clamping differs", filtered, err)
	}
	q = query()
	q.Quality = "estimated"
	estimated, err := a.Queries.Dashboard(t.Context(), q, time.Now())
	if err != nil || estimated.FactCount != 0 || estimated.Summary.TotalTokens != 0 {
		t.Fatal("confirmed and estimated combined", estimated, err)
	}
	facets, err := a.Queries.Facets(t.Context(), query(), "", time.Now())
	if err != nil || len(facets.Sessions) != 3 || facets.DatasetID != first.DatasetID || facets.Revision != first.Revision {
		t.Fatal("facets differ", facets, err)
	}
}

func snapshots(t *testing.T, store Tokens) {
	a := dataset(t, store, "alice")
	// Prepare immutable requests before racing reads against publication.
	bodies := make([][]byte, 20)
	for i := range bodies {
		bodies[i] = batch(t, a, fmt.Sprint(i), record(fmt.Sprint(i)))
	}
	finished := make(chan error, 1)
	go func() {
		worker := dataengine.NewWorker()
		for _, body := range bodies {
			if _, err := a.Receiver.Accept(t.Context(), evidence.ProtocolVersion, body); err != nil {
				finished <- err
				return
			}
			if _, err := worker.ProcessNext(t.Context(), a.Processing); err != nil {
				finished <- err
				return
			}
		}
		finished <- nil
	}()
	// Always join publication before fixture cleanup, even on a failed assertion.
	defer func() {
		if err := <-finished; err != nil {
			t.Error(err)
		}
	}()
	for range 30 {
		got := dashboard(t, a)
		var total int64
		for _, row := range got.Rows {
			total += row.Total
		}
		if got.Summary.TotalTokens != got.FactCount*24 || total != got.Summary.TotalTokens {
			t.Fatal("mixed query snapshots", got)
		}
	}
}
