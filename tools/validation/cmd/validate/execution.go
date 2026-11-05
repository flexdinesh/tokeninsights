package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

type groupTiming struct {
	QueueMS, PreparationMS, ExecutionMS, TotalMS int64
	Reused                                       bool
}

type groupResult struct {
	receipt  receipt
	timing   groupTiming
	executed bool
	err      error
}

// Formatting is a barrier even when explicitly selected after another group.
// On failure cancel this invocation only, then join every running check before
// releasing its source snapshot, home and artifact leases.
func scheduleChecks(ctx context.Context, groups []string, execute func(context.Context, string) groupResult, complete func(string, groupResult)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, group := range groups {
		if group == "format" {
			result := execute(ctx, group)
			complete(group, result)
			if result.err != nil {
				return result.err
			}
		}
	}
	type completion struct {
		group  string
		result groupResult
	}
	done := make(chan completion, len(groups))
	count := 0
	for _, group := range groups {
		if group == "format" {
			continue
		}
		count++
		go func() { done <- completion{group, execute(ctx, group)} }()
	}
	var firstError error
	for range count {
		finished := <-done
		complete(finished.group, finished.result)
		if firstError == nil && finished.result.err != nil {
			firstError = finished.result.err
			cancel()
		}
	}
	return firstError
}

func (r runner) executeGroup(sha, tree, group, key, out, base, attempt, cli string, force bool, checkEngine func() error) (result groupResult) {
	started := time.Now()
	defer func() { result.timing.TotalMS = time.Since(started).Milliseconds() }()
	path := filepath.Join(base, group, key+".json")
	// A failed forced attempt must supersede earlier success, including when it
	// is cancelled while waiting for machine admission.
	if force {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			result.err = err
			return
		}
	}
	release, err := slot(r.ctx, r.manifest.Groups[group].Class)
	result.timing.QueueMS = time.Since(started).Milliseconds()
	if err != nil {
		result.err = err
		return
	}
	defer release()
	result.executed = true
	snap, err := os.MkdirTemp("", "validation-source-")
	if err != nil {
		result.err = err
		return
	}
	defer os.RemoveAll(snap)
	if _, err = r.snapshot(sha, group, snap); err != nil {
		result.err = err
		return
	}
	home, err := os.MkdirTemp(r.home, group+"-")
	if err != nil {
		result.err = err
		return
	}
	defer os.RemoveAll(home)
	r.home = home
	r.logPath = filepath.Join(out, group+"-launcher.log")
	artifacts := filepath.Join(out, group)
	if r.manifest.Groups[group].Executor == "host" {
		result.timing.PreparationMS = time.Since(started).Milliseconds() - result.timing.QueueMS
		executionStart := time.Now()
		err = r.hostFormat(sha, snap, artifacts)
		result.timing.ExecutionMS = time.Since(executionStart).Milliseconds()
	} else if err = checkEngine(); err == nil {
		args := []string{"--mod", snap, "call", "check", "--source", snap, "--group", group, "--attempt", attempt}
		if r.storageBenchmarks {
			args = append(args, "--storage-benchmarks")
		}
		args = append(args, "export", "--path", artifacts)
		_, err = r.command(r.root, cli, args...)
	}
	if timingBytes, readErr := os.ReadFile(filepath.Join(artifacts, "timing.json")); readErr == nil {
		// Diagnostics never establish success; status and complete logs do.
		_ = json.Unmarshal(timingBytes, &result.timing)
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
	result.receipt = receipt{Version: receiptVersion, Group: group, Key: key, SHA: sha, Tree: tree, Artifacts: artifacts, Mode: mode, Completed: time.Now().UTC()}
	result.err = saveReceipt(path, result.receipt)
	return
}
