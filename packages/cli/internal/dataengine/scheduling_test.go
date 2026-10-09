package dataengine_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

func TestStaleRetryDeadlineSurvivesWakeupsAndAdmitsIndependentWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		worker := dataengine.NewWorker()
		hot := concurrentWork("hot", 64<<20)
		cold := concurrentWork("cold", 1024)
		coldReady, coldPublished, hotPublished := false, false, false
		var attempts []time.Time
		var mu sync.Mutex
		inspect := func(check func()) { mu.Lock(); defer mu.Unlock(); check() }
		b := concurrentBackend{
			loadExcluding: func(_ context.Context, exclusions []dataengine.Work, limit int64) (dataengine.Work, bool, error) {
				mu.Lock()
				defer mu.Unlock()
				for _, work := range []dataengine.Work{hot, cold} {
					blocked := false
					for _, excluded := range exclusions {
						blocked = blocked || excluded.DatasetID == work.DatasetID
					}
					if blocked {
						continue
					}
					blocked = work.DatasetID == "hot" && hotPublished || work.DatasetID == "cold" && (!coldReady || coldPublished)
					if !blocked {
						if work.DatasetID == "cold" && limit != 0 {
							t.Error("delayed oversized work consumed active byte admission")
						}
						return work, true, nil
					}
				}
				return dataengine.Work{}, false, nil
			},
			backend: backend{
				publish: func(_ context.Context, work dataengine.Work, projection evidence.Projection) (bool, error) {
					mu.Lock()
					defer mu.Unlock()
					if len(projection.Contributions) != 1 || projection.Contributions[0].Fact.TotalTokens != 120 {
						t.Error("scheduler lost accounting")
					}
					if work.DatasetID == "cold" {
						coldPublished = true
						return true, nil
					}
					attempts = append(attempts, time.Now())
					hotPublished = len(attempts) >= 4
					return hotPublished, nil
				},
				fail: func(context.Context, dataengine.Work) { t.Error("staleness became a durable failure") },
			},
		}
		start := time.Now()
		go worker.Run(ctx, b, func(err error) { t.Error(err) })
		synctest.Wait()
		inspect(func() {
			if len(attempts) != 1 || attempts[0] != start {
				t.Fatal("first attempt delayed", attempts)
			}
			coldReady = true
		})
		worker.Wake()
		synctest.Wait()
		inspect(func() {
			if !coldPublished || len(attempts) != 1 {
				t.Fatal("stale component blocked independent dataset or retried early")
			}
		})
		// Continuous arrivals may invalidate every attempt, but cannot keep moving
		// the scheduler deadline. Virtual time avoids timing assertions on a real host.
		for index := range 10 {
			time.Sleep(50 * time.Millisecond)
			worker.Wake()
			synctest.Wait()
			inspect(func() {
				if len(attempts) != 1+(index+1)/5 {
					t.Fatal("wakeups postponed or bypassed retry deadline", attempts)
				}
			})
		}
		// No wakeup: the deadline itself must retry well before the one-second poll.
		time.Sleep(250 * time.Millisecond)
		synctest.Wait()
		inspect(func() {
			if len(attempts) != 4 || !hotPublished {
				t.Fatal("idle dispatcher failed to retry", attempts)
			}
			hotPublished = false // New revision after successful publication is eager.
		})
		worker.Wake()
		synctest.Wait()
		inspect(func() {
			if len(attempts) != 5 || attempts[4] != attempts[3] {
				t.Fatal("success introduced a scheduling delay", attempts)
			}
		})
		cancel()
		synctest.Wait()
	})
}

func TestWorkerRestartDropsOnlyEphemeralStaleDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		work := concurrentWork("user", 1)
		attempts := 0
		b := concurrentBackend{
			loadExcluding: func(_ context.Context, excluded []dataengine.Work, _ int64) (dataengine.Work, bool, error) {
				return work, len(excluded) == 0, nil
			},
			backend: backend{
				publish: func(context.Context, dataengine.Work, evidence.Projection) (bool, error) {
					attempts++
					return false, nil
				},
				fail: func(context.Context, dataengine.Work) { t.Error("stale work failed") },
			},
		}
		start := time.Now()
		worker := dataengine.NewWorker()
		for run := range 2 {
			ctx, cancel := context.WithCancel(t.Context())
			go worker.Run(ctx, b, func(err error) { t.Error(err) })
			synctest.Wait()
			if attempts != run+1 || time.Now() != start {
				t.Error("restart inherited stale scheduling state", attempts)
			}
			cancel()
			synctest.Wait()
		}
	})
}

func TestRevisedInputAfterFailureDoesNotInheritStaleDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		worker := dataengine.NewWorker()
		work := concurrentWork("user", 1)
		failed, published, reports := false, false, 0
		b := concurrentBackend{
			loadExcluding: func(_ context.Context, excluded []dataengine.Work, _ int64) (dataengine.Work, bool, error) {
				if len(excluded) != 0 {
					return dataengine.Work{}, false, nil
				}
				return work, !published && (!failed || work.Revision == 2), nil
			},
			backend: backend{
				publish: func(_ context.Context, selected dataengine.Work, _ evidence.Projection) (bool, error) {
					if selected.Revision == 1 {
						return false, errors.New("publication failed")
					}
					published = true
					return true, nil
				},
				fail: func(context.Context, dataengine.Work) { failed = true },
			},
		}
		start := time.Now()
		go worker.Run(ctx, b, func(error) { reports++ })
		synctest.Wait()
		if !failed || reports != 1 || published {
			t.Fatal("failure did not retain durable retry/reporting")
		}
		work.Revision = 2
		work.Scopes = map[string]int64{work.Root: 2}
		worker.Wake()
		synctest.Wait()
		if !published || time.Now() != start {
			t.Fatal("revised input inherited stale delay from failure")
		}
		cancel()
		synctest.Wait()
	})
}
