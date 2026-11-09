package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

func defaultGroups() selection {
	return selection{"format", "static", "tooling", "web", "go", "distribution", "browser", "deployment"}
}

func uniqueGroups(groups selection) selection {
	seen := make(map[string]bool)
	var unique selection
	for _, group := range groups {
		if !seen[group] {
			seen[group] = true
			unique = append(unique, group)
		}
	}
	return unique
}

// maxParallelGroups bounds one run's concurrent compose projects; each runner
// is limited to 2 CPUs by compose.yaml.
const maxParallelGroups = 3

// groupPriority starts the longest groups first to shorten wall time.
var groupPriority = []string{"go", "browser", "distribution", "deployment", "static", "tooling", "web", "benchmarks"}

func prioritized(groups []string) []string {
	ordered := slices.Clone(groups)
	rank := func(group string) int {
		if i := slices.Index(groupPriority, group); i >= 0 {
			return i
		}
		return len(groupPriority)
	}
	slices.SortStableFunc(ordered, func(a, b string) int { return rank(a) - rank(b) })
	return ordered
}

type groupTiming struct {
	QueueMS, PreparationMS, ExecutionMS, TotalMS int64
	Reused                                       bool
}

type groupResult struct {
	receipt  receipt
	timing   groupTiming
	executed bool
	err      error
	// log is the most relevant log for a failure; warning reports teardown
	// problems that do not change the check outcome.
	log, warning string
}

// scheduleChecks runs formatting first as a barrier: its failure stops the run
// before any slot or image build is spent. Remaining groups run after one
// admission, at most maxParallelGroups at once, longest first. A failure lets
// sibling groups finish; every failure is reported. Cancellation stops queued
// groups and joins running ones before admission is released.
func scheduleChecks(ctx context.Context, groups []string, admit func(context.Context) (func(), error), execute func(context.Context, string) groupResult, complete func(string, groupResult)) error {
	var rest []string
	for _, group := range groups {
		if group != "format" {
			rest = append(rest, group)
			continue
		}
		result := execute(ctx, group)
		complete(group, result)
		if result.err != nil {
			return result.err
		}
	}
	if len(rest) == 0 {
		return nil
	}
	release, err := admit(ctx)
	if err != nil {
		return err
	}
	defer release()
	queue := make(chan string, len(rest))
	for _, group := range prioritized(rest) {
		queue <- group
	}
	close(queue)
	type completion struct {
		group  string
		result groupResult
	}
	done := make(chan completion)
	var workers sync.WaitGroup
	for range min(maxParallelGroups, len(rest)) {
		workers.Go(func() {
			for group := range queue {
				if err := ctx.Err(); err != nil {
					done <- completion{group, groupResult{err: fmt.Errorf("%s: %w", group, err)}}
					continue
				}
				done <- completion{group, execute(ctx, group)}
			}
		})
	}
	go func() {
		workers.Wait()
		close(done)
	}()
	var failures []error
	for finished := range done {
		complete(finished.group, finished.result)
		if finished.result.err != nil {
			failures = append(failures, finished.result.err)
		}
	}
	return errors.Join(failures...)
}

// invocation is the run-wide state each group execution reads.
type invocation struct {
	sha, tree      string
	out, base      string
	source, temp   string
	force          bool
	slot           int
	queued         time.Time
	executionGroup func(string) string
}

func (r runner) executeGroup(inv invocation, group, key string) (result groupResult) {
	started := time.Now()
	result.timing.QueueMS = started.Sub(inv.queued).Milliseconds()
	defer func() { result.timing.TotalMS = time.Since(inv.queued).Milliseconds() }()
	path := filepath.Join(inv.base, group, key+".json")
	// A failed forced attempt must supersede earlier success, including when it
	// is cancelled while waiting for machine admission.
	if inv.force {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			result.err = err
			return
		}
	}
	if err := r.ctx.Err(); err != nil {
		result.err = fmt.Errorf("%s: %w", group, err)
		return
	}
	result.executed = true
	scratch, err := os.MkdirTemp(inv.temp, group+"-")
	if err != nil {
		result.err = err
		return
	}
	defer os.RemoveAll(scratch)
	home, err := os.MkdirTemp(r.home, group+"-")
	if err != nil {
		result.err = err
		return
	}
	defer os.RemoveAll(home)
	r.home = home
	r.logPath = filepath.Join(inv.out, group+"-launcher.log")
	artifacts := filepath.Join(inv.out, group)
	result.log = filepath.Join(artifacts, "check.log")
	if r.manifest.Groups[group].Executor == "host" {
		var source string
		source, err = r.prepareHostSource(inv.source, group, scratch)
		result.timing.PreparationMS = time.Since(started).Milliseconds()
		if err == nil {
			executionStart := time.Now()
			err = r.hostFormat(inv.sha, source, artifacts)
			result.timing.ExecutionMS = time.Since(executionStart).Milliseconds()
		}
	} else {
		err = r.executeContainer(inv, group, scratch, artifacts, &result)
	}
	if err == nil {
		err = evidence(artifacts)
	}
	if err != nil {
		result.err = fmt.Errorf("%s: %w (artifacts %s)", group, err, artifacts)
		return
	}
	mode := "default"
	if r.storageBenchmarks {
		mode = "storage-samples"
	}
	result.receipt = receipt{Version: receiptVersion, Group: group, Key: key, SHA: inv.sha, Tree: inv.tree, Artifacts: artifacts, Mode: mode, Completed: time.Now().UTC()}
	result.err = saveReceipt(path, result.receipt)
	return
}

// executeContainer materializes contexts from the fingerprint predicate, then
// builds, starts, runs and always tears down the group's compose project.
func (r runner) executeContainer(inv invocation, group, scratch, artifacts string, result *groupResult) error {
	preparation := time.Now()
	contexts, err := r.manifest.materialize(inv.source, group, filepath.Join(scratch, "contexts"))
	if err != nil {
		return err
	}
	run := containerRun{r: r, slot: inv.slot, group: group, task: inv.executionGroup(group), tree: inv.source, contexts: contexts, artifacts: artifacts, logs: inv.out, workdir: contexts["empty"]}
	defer func() {
		if err := run.teardown(r.ctx); err != nil {
			result.warning = err.Error()
		}
	}()
	logName, err := run.prepare(r.ctx)
	result.timing.PreparationMS = time.Since(preparation).Milliseconds()
	if err != nil {
		result.log = filepath.Join(inv.out, logName)
		return err
	}
	execution := time.Now()
	err = run.execute(r.ctx)
	result.timing.ExecutionMS = time.Since(execution).Milliseconds()
	if err != nil {
		if _, statErr := os.Stat(result.log); statErr != nil {
			result.log = filepath.Join(inv.out, group+"-run.log")
		}
	}
	return err
}

func evidence(path string) error {
	b, e := os.ReadFile(filepath.Join(path, "status.json"))
	if e != nil {
		return e
	}
	var s struct {
		ExitCode *int `json:"exitCode"`
	}
	if e = json.Unmarshal(b, &s); e != nil {
		return e
	}
	if s.ExitCode == nil || *s.ExitCode != 0 {
		return errors.New("check failed or incomplete status")
	}
	f, e := os.Stat(filepath.Join(path, "check.log"))
	if e != nil {
		return e
	}
	if !f.Mode().IsRegular() {
		return errors.New("missing check log")
	}
	return nil
}
