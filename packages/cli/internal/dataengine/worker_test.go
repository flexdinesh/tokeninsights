package dataengine_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

type backend struct {
	load    func(context.Context) (dataengine.Work, bool, error)
	publish func(context.Context, dataengine.Work, evidence.Projection) (bool, error)
	fail    func(context.Context, dataengine.Work)
}

func (b backend) LoadWork(ctx context.Context) (dataengine.Work, bool, error) {
	return b.load(ctx)
}
func (b backend) PublishProjection(ctx context.Context, w dataengine.Work, p evidence.Projection) (bool, error) {
	return b.publish(ctx, w, p)
}
func (b backend) RecordFailure(ctx context.Context, w dataengine.Work) {
	b.fail(ctx, w)
}

func TestWorkerSerializesDatasetBackendsAndRetriesStaleProjection(t *testing.T) {
	worker := dataengine.NewWorker()
	loaded, release := make(chan struct{}), make(chan struct{})
	complete := make(chan error, 1)
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 2,
		Data:    json.RawMessage(`{"type":"message","id":"message","message":{"role":"assistant","timestamp":1700000000000,"usage":{"input":100,"output":20}}}`),
		Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
	work := dataengine.Work{DatasetID: "user-a", Root: "scope", Generation: 2, Revision: 3, Scopes: map[string]int64{"scope": 3}, Records: []evidence.Stored{{ID: evidence.EvidenceID(record), Record: record}}}
	a := backend{
		load: func(context.Context) (dataengine.Work, bool, error) {
			close(loaded)
			<-release
			return work, true, nil
		},
		publish: func(_ context.Context, received dataengine.Work, projection evidence.Projection) (bool, error) {
			if received.DatasetID != "user-a" || received.Generation != 2 || received.Scopes["scope"] != 3 || len(projection.Contributions) != 1 || projection.Contributions[0].Fact.TotalTokens != 120 {
				return false, errors.New("projection lost binding, fence or token components")
			}
			return false, nil // Component changed during interpretation; no mutation.
		},
		fail: func(context.Context, dataengine.Work) { t.Error("stale work recorded as a failure") },
	}
	go func() {
		worked, err := worker.ProcessNext(t.Context(), a)
		if err == nil && !worked {
			err = errors.New("stale component did not request immediate fresh work")
		}
		complete <- err
	}()
	<-loaded
	b := backend{load: func(context.Context) (dataengine.Work, bool, error) {
		t.Error("concurrent dataset bypassed instance worker serialization")
		return dataengine.Work{}, false, nil
	}}
	worked, err := worker.ProcessNext(t.Context(), b)
	close(release)
	if err != nil || worked {
		t.Fatal("concurrent processing did not yield", worked, err)
	}
	if err := <-complete; err != nil {
		t.Fatal(err)
	}
}

func TestPartialLoadFailureRetainsDatasetAndCancellationDoesNotBackoff(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry", true: "canceled"}[canceled], func(t *testing.T) {
			worker := dataengine.NewWorker()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("corrupt stored evidence")
			failures := 0
			b := backend{
				load: func(context.Context) (dataengine.Work, bool, error) {
					if canceled {
						cancel()
					}
					return dataengine.Work{DatasetID: "user-b", Root: "broken"}, false, failure
				},
				fail: func(_ context.Context, w dataengine.Work) {
					failures++
					if w.DatasetID != "user-b" || w.Root != "broken" {
						t.Error("retry crossed the failed dataset/component")
					}
				},
			}
			if worked, err := worker.ProcessNext(ctx, b); worked || !errors.Is(err, failure) {
				t.Fatal("partial load failure swallowed", worked, err)
			}
			if canceled && failures != 0 || !canceled && failures != 1 {
				t.Fatal("cancellation changed durable failure attempts", failures)
			}
		})
	}
}
