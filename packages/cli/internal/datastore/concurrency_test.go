package datastore

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
)

func TestClaimedComponentSpanningCandidatePagesDoesNotBlockIndependentWork(t *testing.T) {
	store := testStore(t)
	const children = 40
	records := []evidence.Record{codexRecord("group-parent", "")}
	for index := range children {
		records = append(records, codexRecord(fmt.Sprintf("child-%02d", index), "group-parent"))
	}
	independent := piRecord("independent", 100)
	records = append(records, independent)
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "stream", "batch", records...)); err != nil {
		t.Fatal(err)
	}
	first, found, err := store.LoadWork(t.Context())
	if err != nil || !found || len(first.Scopes) != children+1 {
		t.Fatal("missing connected component", first, found, err)
	}
	second, found, err := store.LoadWorkExcluding(t.Context(), []dataengine.Work{first}, 32<<20)
	if err != nil || !found || second.Root != evidence.Scope(independent) {
		t.Fatal("claimed roots crossing a keyset page blocked independent work", second, found, err)
	}
}

func TestRunningWorkerPublishesIsolatedDatasetsAndJoins(t *testing.T) {
	root, alice, bob := hostedStores(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	joined := make(chan struct{})
	go func() {
		root.Run(ctx, func(err error) { t.Error(err) })
		close(joined)
	}()
	defer func() { cancel(); <-joined }()
	for _, store := range []*Store{alice, bob} {
		if _, err := store.Accept(ctx, evidence.ProtocolVersion, batchBody(t, store, "same-stream", "same-batch", piRecord("same-message", 100))); err != nil {
			t.Fatal(err)
		}
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready := true
		for _, store := range []*Store{alice, bob} {
			response, err := store.Receipt(ctx, "same-stream", "same-batch")
			if err != nil {
				t.Fatal(err)
			}
			ready = ready && response.Processing.Pending == 0
		}
		if ready {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("running workers did not publish both datasets")
		case <-ticker.C:
		}
	}
	for _, store := range []*Store{alice, bob} {
		if got := total(t, store, "analytics.confirmed"); got != 120 {
			t.Fatal("running publication lost isolation or usage", store.DatasetID(), got)
		}
	}
}

func TestClaimsSkipLateConnectedComponentAndFenceStalePublication(t *testing.T) {
	store := testStore(t)
	child := codexRecord("child", "parent")
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "child", "batch", child)); err != nil {
		t.Fatal(err)
	}
	first, found, err := store.LoadWork(t.Context())
	if err != nil || !found || len(first.Scopes) != 1 || first.Bytes <= 0 {
		t.Fatal("missing first component", first, found, err)
	}
	good := piRecord("good", 200)
	good.Context[0].Data = json.RawMessage(`{"type":"session","id":"independent"}`)
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "later", "batch", codexRecord("parent", ""), good)); err != nil {
		t.Fatal(err)
	}
	second, found, err := store.LoadWorkExcluding(t.Context(), []dataengine.Work{first}, 32<<20)
	if err != nil || !found || len(second.Scopes) != 1 || second.Root != evidence.Scope(good) {
		t.Fatal("late connected ancestor scheduled while child claimed", second, found, err)
	}
	projection, err := processor.Process(t.Context(), first.Records)
	if err != nil {
		t.Fatal(err)
	}
	if published, err := store.PublishProjection(t.Context(), first, projection); err != nil || published {
		t.Fatal("late connection did not fence stale publication", published, err)
	}
	drain(t, store)
	if got := total(t, store, "analytics.confirmed"); got != 340 {
		t.Fatal("late parent or independent usage lost/inflated", got)
	}
}

func TestWorkByteBudgetDefersLargeComponentWithoutStarvation(t *testing.T) {
	store := testStore(t)
	large := codexRecord("a-large", "")
	small := piRecord("small", 100)
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "stream", "batch", large, small)); err != nil {
		t.Fatal(err)
	}
	// Codex context makes this component larger than the independent Pi component.
	work, found, err := store.LoadWorkExcluding(t.Context(), nil, int64(lenMustMarshal(t, small)))
	if err != nil || !found || work.Root != evidence.Scope(small) {
		t.Fatal("byte admission failed", work, found, err)
	}
	next, found, err := store.LoadWorkExcluding(t.Context(), []dataengine.Work{work}, 1)
	if err != nil || found {
		t.Fatal("oversized remaining work admitted", next, found, err)
	}
	next, found, err = store.LoadWorkExcluding(t.Context(), []dataengine.Work{work}, 0)
	if err != nil || !found || next.Root != evidence.Scope(large) {
		t.Fatal("large component cannot run alone", next, found, err)
	}
}
func lenMustMarshal(t *testing.T, value evidence.Record) int {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return len(body)
}

func TestDelayedMembershipSkipsLateAncestorsWithoutBlockingOtherDatasets(t *testing.T) {
	root, alice, bob := hostedStores(t)
	if _, err := alice.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, alice, "child", "batch", codexRecord("child", "parent"))); err != nil {
		t.Fatal(err)
	}
	first, found, err := root.LoadWork(t.Context())
	if err != nil || !found {
		t.Fatal(first, found, err)
	}
	projection, err := processor.Process(t.Context(), first.Records)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := alice.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, alice, "parent", "batch", codexRecord("parent", ""))); err != nil {
		t.Fatal(err)
	}
	if published, err := root.PublishProjection(t.Context(), first, projection); err != nil || published {
		t.Fatal("late ancestor escaped fence", published, err)
	}
	// The scheduler retains only membership, not payload or byte admission.
	first.Records, first.Bytes = nil, 0
	if _, err := bob.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, bob, "child", "batch", codexRecord("child", ""))); err != nil {
		t.Fatal(err)
	}
	work, found, err := root.LoadWorkExcluding(t.Context(), []dataengine.Work{first}, 0)
	if err != nil || !found || work.DatasetID != "bob" {
		t.Fatal("delay crossed datasets", work, found, err)
	}
	if _, found, err := root.LoadWorkExcluding(t.Context(), []dataengine.Work{first, work}, 0); err != nil || found {
		t.Fatal("alternate root bypassed delayed membership", found, err)
	}
	var failures int
	if err := root.SQL().QueryRow("SELECT COUNT(*) FROM processing.scopes WHERE error_code<>'' OR attempts<>0 OR retry_at_ms<>0").Scan(&failures); err != nil || failures != 0 {
		t.Fatal("stale work changed durable backoff", failures, err)
	}
	// Removing only the ephemeral hint makes durable work immediately available.
	resumed, found, err := root.LoadWorkExcluding(t.Context(), []dataengine.Work{work}, 0)
	if err != nil || !found || resumed.DatasetID != "alice" || len(resumed.Scopes) != 2 {
		t.Fatal("pending component lost", resumed, found, err)
	}
	drain(t, root)
	for _, dataset := range []*Store{alice, bob} {
		if total(t, dataset, "analytics.confirmed") != 120 {
			t.Fatal("delay changed native accounting", dataset.DatasetID())
		}
	}
}

func TestComponentFailureBackoffFencesNewInputs(t *testing.T) {
	store := testStore(t)
	parent := codexRecord("parent", "")
	child := codexRecord("child", "parent")
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "first", "batch", parent, child)); err != nil {
		t.Fatal(err)
	}
	work, found, err := store.LoadWork(t.Context())
	if err != nil || !found || len(work.Scopes) != 2 {
		t.Fatal(work, found, err)
	}
	store.RecordFailure(t.Context(), work)
	if _, found, err := store.LoadWork(t.Context()); err != nil || found {
		t.Fatal("alternate root bypassed component backoff", found, err)
	}
	newer := codexRecord("parent", "")
	newer.Ordinal = 4
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "second", "batch", newer)); err != nil {
		t.Fatal(err)
	}
	store.RecordFailure(t.Context(), work) // Old worker failure must not delay newer revision.
	refreshed, found, err := store.LoadWork(t.Context())
	if err != nil || !found || refreshed.Scopes[work.Root] == work.Scopes[work.Root] {
		t.Fatal("stale failure delayed new inputs", refreshed, found, err)
	}
}
