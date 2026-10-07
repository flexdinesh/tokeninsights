package dataengine_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

type concurrentBackend struct {
	backend
	loadExcluding func(context.Context, []dataengine.Work, int64) (dataengine.Work, bool, error)
}

func (b concurrentBackend) LoadWorkExcluding(ctx context.Context, claims []dataengine.Work, limit int64) (dataengine.Work, bool, error) {
	return b.loadExcluding(ctx, claims, limit)
}
func concurrentWork(dataset string, bytes int64) dataengine.Work {
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 2,
		Data:    json.RawMessage(`{"type":"message","id":"message","message":{"role":"assistant","timestamp":1700000000000,"usage":{"input":100,"output":20}}}`),
		Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
	return dataengine.Work{DatasetID: dataset, Root: "same-scope", Generation: 1, Revision: 1, Bytes: bytes, Scopes: map[string]int64{"same-scope": 1}, Records: []evidence.Stored{{ID: evidence.EvidenceID(record), Record: record}}}
}

func TestConcurrentRunClaimsIndependentDatasetsAndJoins(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	worker := dataengine.NewWorker()
	entered := make(chan string, 2)
	release := make(chan struct{})
	joined := make(chan struct{})
	var mu sync.Mutex
	published := map[string]bool{}
	works := []dataengine.Work{concurrentWork("user-a", 1024), concurrentWork("user-b", 1024)}
	b := concurrentBackend{
		loadExcluding: func(_ context.Context, claims []dataengine.Work, limit int64) (dataengine.Work, bool, error) {
			mu.Lock()
			defer mu.Unlock()
			if len(claims) == 1 && limit <= 0 {
				return dataengine.Work{}, false, errors.New("second worker has no bounded byte budget")
			}
			for _, work := range works {
				claimed := published[work.DatasetID]
				for _, claim := range claims {
					if claim.DatasetID == work.DatasetID {
						claimed = true
					}
				}
				if !claimed {
					return work, true, nil
				}
			}
			return dataengine.Work{}, false, nil
		},
		backend: backend{
			publish: func(_ context.Context, work dataengine.Work, p evidence.Projection) (bool, error) {
				if len(p.Contributions) != 1 || p.Contributions[0].Fact.TotalTokens != 120 {
					return false, errors.New("lost token semantics")
				}
				entered <- work.DatasetID
				<-release
				mu.Lock()
				defer mu.Unlock()
				if published[work.DatasetID] {
					return false, errors.New("component processed twice")
				}
				published[work.DatasetID] = true
				return true, nil
			},
			fail: func(context.Context, dataengine.Work) { t.Error("valid independent component backed off") },
		},
	}
	go func() { worker.Run(ctx, b, func(err error) { t.Error(err) }); close(joined) }()
	seen := map[string]bool{}
	for range 2 {
		select {
		case dataset := <-entered:
			seen[dataset] = true
		case <-ctx.Done():
			close(release)
			<-joined
			t.Fatal("independent workers did not overlap")
		}
	}
	if !seen["user-a"] || !seen["user-b"] {
		t.Error("dataset-qualified claims collided", seen)
	}
	worked, err := worker.ProcessNext(ctx, backend{load: func(context.Context) (dataengine.Work, bool, error) {
		return dataengine.Work{}, false, errors.New("maintenance bypassed running dispatcher")
	}})
	if worked || err != nil {
		t.Error("maintenance did not yield to dispatcher", worked, err)
	}
	cancel()
	select {
	case <-joined:
		t.Error("Run returned before joining active jobs")
	default:
	}
	close(release)
	<-joined
	mu.Lock()
	defer mu.Unlock()
	if len(published) != 2 {
		t.Error("Run lost active workers", published)
	}
}

func TestHugeComponentRunsAloneWithoutLoadingSecond(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	worker := dataengine.NewWorker()
	entered := make(chan struct{})
	release := make(chan struct{})
	joined := make(chan struct{})
	var loads int
	b := concurrentBackend{
		loadExcluding: func(_ context.Context, claims []dataengine.Work, _ int64) (dataengine.Work, bool, error) {
			loads++
			if len(claims) != 0 || loads != 1 {
				return dataengine.Work{}, false, errors.New("huge component admitted concurrent work")
			}
			return concurrentWork("user", 64<<20), true, nil
		},
		backend: backend{
			publish: func(context.Context, dataengine.Work, evidence.Projection) (bool, error) {
				close(entered)
				<-release
				return true, nil
			},
			fail: func(context.Context, dataengine.Work) { t.Error("huge component failed") },
		},
	}
	go func() { worker.Run(ctx, b, func(err error) { t.Error(err) }); close(joined) }()
	select {
	case <-entered:
	case <-ctx.Done():
		close(release)
		<-joined
		t.Fatal("huge component starved")
	}
	// Wake must not cause another load while a huge claim exhausts admission.
	worker.Wake()
	cancel()
	close(release)
	<-joined
	if loads != 1 {
		t.Error("unbounded concurrent load", loads)
	}
}

func TestLoadFailureDoesNotSpinWhenFailureRecordingCannotPersist(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	loads := 0
	b := concurrentBackend{
		loadExcluding: func(context.Context, []dataengine.Work, int64) (dataengine.Work, bool, error) {
			loads++
			return dataengine.Work{DatasetID: "user", Root: "broken"}, false, errors.New("database unavailable")
		},
		backend: backend{fail: func(context.Context, dataengine.Work) {}},
	}
	dataengine.NewWorker().Run(ctx, b, nil)
	if loads != 1 {
		t.Error("failed durable backoff caused a hot selection loop", loads)
	}
}
