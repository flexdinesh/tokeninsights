package pipeline

import (
	"context"
	"fmt"
	"sync"
)

type harnessDiscovery struct {
	harness Harness
	plan    harnessPlan
}

// Discovery is independent; status publication remains on the coordinator.
func discoverHarnessPlans(ctx context.Context, options SyncOptions, start func(Harness) error, record func(Harness, []Source) error) (map[Harness]harnessPlan, error) {
	plans := make(map[Harness]harnessPlan, len(options.Harnesses))
	if len(options.Harnesses) == 0 {
		return plans, nil
	}
	jobs := make(chan Harness, len(options.Harnesses))
	for _, harness := range options.Harnesses {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if start != nil {
			if err := start(harness); err != nil {
				return nil, err
			}
		}
		jobs <- harness
	}
	close(jobs)
	discoveryContext, cancel := context.WithCancel(ctx)
	results := make(chan harnessDiscovery, len(options.Harnesses))
	var running sync.WaitGroup
	for range sourceWorkerCount(options, len(options.Harnesses)) {
		running.Go(func() {
			for harness := range jobs {
				if discoveryContext.Err() != nil {
					return
				}
				adapter, ok := AdapterFor(harness)
				plan := harnessPlan{adapter: adapter}
				if !ok {
					plan.err = fmt.Errorf("unsupported harness %q", harness)
				} else {
					plan.sources, plan.err = adapter.Discover(discoveryContext, discoverOptions(parserOptionsForHarness(options, harness)))
				}
				select {
				case results <- harnessDiscovery{harness: harness, plan: plan}:
				case <-discoveryContext.Done():
					return
				}
			}
		})
	}
	defer func() { cancel(); running.Wait() }()
	for range options.Harnesses {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-results:
			if result.plan.err == nil && record != nil {
				if err := record(result.harness, result.plan.sources); err != nil {
					return nil, err
				}
			}
			plans[result.harness] = result.plan
		}
	}
	return plans, nil
}
