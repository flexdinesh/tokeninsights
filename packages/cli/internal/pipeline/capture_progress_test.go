package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func captureTestSources(t *testing.T, count int) string {
	t.Helper()
	root := t.TempDir()
	for i := range count {
		rawTestWrite(t, filepath.Join(root, fmt.Sprintf("session-%02d.jsonl", i)), fmt.Sprintf("{\"type\":\"session\",\"id\":\"session-%d\"}\n{\"type\":\"message\",\"id\":\"message-%d\",\"message\":{\"role\":\"assistant\",\"usage\":{\"input\":10,\"output\":2}}}\n", i, i))
	}
	return root
}

func checkCaptureMeasurements(t *testing.T, events []CaptureProgressEvent) {
	t.Helper()
	for _, event := range events {
		if event.Checked != event.Captured+event.Unchanged+event.Failed+event.Quarantined || event.Active < 0 || event.Checked < 0 || event.TotalKnown && event.Checked+event.Active > event.Total {
			t.Fatal("inconsistent source snapshot", event)
		}
		if event.Phase == CaptureReading && event.Active == 0 {
			t.Fatal("reading without an admitted reader", event)
		}
	}
}

func TestCaptureProgressReportsDiscoveryAdmissionCommittedSourcesAndReplay(t *testing.T) {
	root := captureTestSources(t, 6)
	store := rawTestStore(t)
	var callbacks, overlaps atomic.Int32
	var events []CaptureProgressEvent
	var coarse []SyncProgressEvent
	var observedChecked int
	var measurementErr error
	options := SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessPi}, workers: 2,
		Progress: func(event SyncProgressEvent) { coarse = append(coarse, event) },
		CaptureProgress: func(event CaptureProgressEvent) {
			if callbacks.Add(1) != 1 {
				overlaps.Add(1)
			}
			runtime.Gosched()
			events = append(events, event)
			if event.Checked > observedChecked {
				observedChecked = event.Checked
				var records int
				err := store.DB.QueryRow("SELECT count(*) FROM evidence_outbox").Scan(&records)
				wantRecords := event.Captured * 2
				if event.Unchanged > 0 {
					wantRecords = 12
				}
				if err != nil || records != wantRecords {
					measurementErr = fmt.Errorf("finalized snapshot preceded durable evidence: records=%d want=%d query=%v", records, wantRecords, err)
				}
			}
			callbacks.Add(-1)
		},
	}
	for _, replay := range []bool{false, true} {
		events, coarse = nil, nil
		observedChecked, measurementErr = 0, nil
		summary, err := Extract(t.Context(), options, store)
		if err != nil {
			t.Fatal(summary, err)
		}
		if measurementErr != nil {
			t.Fatal(measurementErr)
		}
		checkCaptureMeasurements(t, events)
		if first := events[0]; first.Phase != CaptureDiscovering || first.TotalKnown || first.Checked != 0 {
			t.Fatal("discovery fabricated a total", first)
		}
		if known := events[1]; known.Phase != CaptureWaiting || !known.TotalKnown || known.Total != 6 || known.Active != 0 {
			t.Fatal("enumerated sources claimed active work", known)
		}
		reading, saving := false, false
		for _, event := range events {
			reading = reading || event.Phase == CaptureReading
			saving = saving || event.Phase == CaptureSaving
			if event.Active > 2 {
				t.Fatal("reader count exceeded worker admission", event)
			}
		}
		if !reading || !saving || overlaps.Load() != 0 {
			t.Fatal("missing work phases or concurrent callbacks", events, overlaps.Load())
		}
		last := events[len(events)-1]
		want := CaptureProgressEvent{Harness: HarnessPi, Phase: CaptureComplete, TotalKnown: true, Total: 6, Checked: 6}
		status := SyncProgressSynced
		if replay {
			want.Unchanged, status = 6, SyncProgressSkipped
			if summary.RawFacts != 0 {
				t.Fatal("unchanged sources emitted evidence", summary)
			}
		} else {
			want.Captured = 6
			if summary.RawFacts != 12 {
				t.Fatal("source counts confused with records", summary)
			}
		}
		if last != want {
			t.Fatal("incorrect finalized sources", last, want)
		}
		wantCoarse := []SyncProgressEvent{{Harness: HarnessPi, Status: SyncProgressDiscovering}, {Harness: HarnessPi, Status: SyncProgressSyncing}, {Harness: HarnessPi, Status: status}}
		if !reflect.DeepEqual(coarse, wantCoarse) {
			t.Fatal("detailed measurements changed coarse callback", coarse)
		}
	}
	if len(rawTestRecords(t, store)) != 12 {
		t.Fatal("replay changed committed evidence")
	}
}

func TestCaptureProgressEmptyAndInterruptedDiscovery(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(fmt.Sprintf("interrupted=%t", interrupted), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if interrupted {
				cancel()
			}
			var events []CaptureProgressEvent
			summary, err := Extract(ctx, SyncOptions{SourceDir: t.TempDir(), Harnesses: []Harness{HarnessPi}, CaptureProgress: func(event CaptureProgressEvent) { events = append(events, event) }}, rawTestStore(t))
			checkCaptureMeasurements(t, events)
			last := events[len(events)-1]
			if interrupted {
				if !errors.Is(err, context.Canceled) || last.Phase != CaptureInterrupted || last.TotalKnown || summary.Failed != 1 {
					t.Fatal("interrupted discovery fabricated counts", summary, err, last)
				}
			} else if err != nil || last.Phase != CaptureComplete || !last.TotalKnown || last.Total != 0 || last.Checked != 0 || summary.Skipped != 1 {
				t.Fatal("empty discovery not completed", summary, err, last)
			}
		})
	}
}

func TestCaptureProgressCommitRollbackAndReaderFailure(t *testing.T) {
	for _, action := range []string{"rewrite-before-commit", "remove-before-reading"} {
		t.Run(action, func(t *testing.T) {
			root := captureTestSources(t, 1)
			path := filepath.Join(root, "session-00.jsonl")
			store := rawTestStore(t)
			var events []CaptureProgressEvent
			var mutationErr error
			changed := false
			options := SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessPi}, workers: 1, CaptureProgress: func(event CaptureProgressEvent) {
				events = append(events, event)
				if changed {
					return
				}
				if action == "rewrite-before-commit" && event.Phase == CaptureSaving {
					changed = true
					mutationErr = os.WriteFile(path, []byte("{}\n"), 0o600)
				} else if action == "remove-before-reading" && event.Phase == CaptureWaiting && event.TotalKnown {
					changed = true
					mutationErr = os.Remove(path)
				}
			}}
			summary, err := Extract(t.Context(), options, store)
			if mutationErr != nil || err == nil || summary.Failed != 1 || summary.RawFacts != 0 {
				t.Fatal("source failure not reported", mutationErr, summary, err)
			}
			checkCaptureMeasurements(t, events)
			want := CaptureProgressEvent{Harness: HarnessPi, Phase: CaptureFailed, TotalKnown: true, Total: 1, Checked: 1, Failed: 1}
			if last := events[len(events)-1]; last != want {
				t.Fatal("failed source counted successful", last)
			}
			if len(rawTestRecords(t, store)) != 0 {
				t.Fatal("failed source leaked evidence")
			}
			var checkpoints int
			if err := store.DB.QueryRow("SELECT count(*) FROM evidence_sources").Scan(&checkpoints); err != nil || checkpoints != 0 {
				t.Fatal("failed source advanced checkpoints", checkpoints, err)
			}
		})
	}
}

func TestCaptureProgressCancellationKeepsUnissuedSourcesOutstanding(t *testing.T) {
	root := captureTestSources(t, 6)
	store := rawTestStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var events []CaptureProgressEvent
	summary, err := Extract(ctx, SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessPi}, workers: 1, CaptureProgress: func(event CaptureProgressEvent) {
		events = append(events, event)
		if event.Phase == CaptureSaving {
			cancel()
		}
	}}, store)
	if !errors.Is(err, context.Canceled) || summary.RawFacts != 0 {
		t.Fatal("canceled capture succeeded", summary, err)
	}
	checkCaptureMeasurements(t, events)
	want := CaptureProgressEvent{Harness: HarnessPi, Phase: CaptureInterrupted, TotalKnown: true, Total: 6, Checked: 1, Failed: 1}
	if last := events[len(events)-1]; last != want {
		t.Fatal("unissued sources were finalized", last)
	}
	if len(rawTestRecords(t, store)) != 0 {
		t.Fatal("cancellation committed evidence")
	}
}

func TestCaptureProgressQuarantineDistinctFromFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rollout-2026-10-07T12-00-00-session.jsonl")
	rawTestWrite(t, path, codexTestHeader()+`{"type":"session_meta","payload":{"private":"`+strings.Repeat("x", maxEvidenceLineBytes)+`"}}`+"\n")
	store := rawTestStore(t)
	for range 2 {
		var events []CaptureProgressEvent
		summary, err := Extract(t.Context(), SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessCodex}, CaptureProgress: func(event CaptureProgressEvent) { events = append(events, event) }}, store)
		if err == nil || summary.Quarantined != 1 || summary.RawFacts != 0 {
			t.Fatal("quarantine did not preserve failure semantics", summary, err)
		}
		checkCaptureMeasurements(t, events)
		want := CaptureProgressEvent{Harness: HarnessCodex, Phase: CaptureFailed, TotalKnown: true, Total: 1, Checked: 1, Quarantined: 1}
		if last := events[len(events)-1]; last != want {
			t.Fatal("quarantined source included in ordinary failures", last)
		}
	}
}

func TestCaptureProgressOptionalLeavesCaptureResultsUnchanged(t *testing.T) {
	root := captureTestSources(t, 2)
	var previous Summary
	for _, enabled := range []bool{false, true} {
		options := SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessPi}}
		if enabled {
			options.CaptureProgress = func(CaptureProgressEvent) {}
		}
		store := rawTestStore(t)
		summary, err := Extract(t.Context(), options, store)
		if err != nil || summary.RawFacts != 4 || len(rawTestRecords(t, store)) != 4 {
			t.Fatal(summary, err)
		}
		if enabled && !reflect.DeepEqual(summary, previous) {
			t.Fatal("optional measurement changed capture outcome", summary, previous)
		}
		previous = summary
	}
}

func TestCaptureProgressKeepsHarnessMeasurementsSeparate(t *testing.T) {
	root := t.TempDir()
	for _, harness := range []Harness{HarnessPi, HarnessCodex} {
		if err := os.Mkdir(filepath.Join(root, string(harness)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 2 {
		rawTestWrite(t, filepath.Join(root, "pi", fmt.Sprintf("session-%d.jsonl", i)), fmt.Sprintf("{\"type\":\"session\",\"id\":\"pi-%d\"}\n", i))
	}
	rawTestWrite(t, filepath.Join(root, "codex", "rollout-2026-10-07T12-00-00-session.jsonl"), codexTestHeader()+codexTestUsage(10))
	var events []CaptureProgressEvent
	last := make(map[Harness]CaptureProgressEvent)
	summary, err := Extract(t.Context(), SyncOptions{SourceDir: root, CaptureProgress: func(event CaptureProgressEvent) {
		events = append(events, event)
		last[event.Harness] = event
	}}, rawTestStore(t))
	if err != nil || summary.RawFacts != 4 {
		t.Fatal(summary, err)
	}
	checkCaptureMeasurements(t, events)
	for _, harness := range SupportedHarnesses {
		want := CaptureProgressEvent{Harness: harness, Phase: CaptureComplete, TotalKnown: true}
		switch harness {
		case HarnessPi:
			want.Total, want.Checked, want.Captured = 2, 2, 2
		case HarnessCodex:
			want.Total, want.Checked, want.Captured = 1, 1, 1
		}
		if last[harness] != want {
			t.Fatal("harness counts crossed boundaries", last[harness], want)
		}
	}
}
