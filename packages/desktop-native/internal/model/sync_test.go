package model

import (
	"errors"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

// statusAt is a ready, fully processed status of testInstance's dataset
// at revision and generation.
func statusAt(revision, generation int64) application.Status {
	return application.Status{InstanceID: testInstance.InstanceID, Readiness: "ready", Metadata: application.Publication{
		DatabaseID: "db-1", DatasetID: testInstance.DatasetID, Revision: revision, Generation: generation, TargetGeneration: generation,
	}}
}

// progressOf is testInstance's collector progress with attempts.
func progressOf(attempts ...collectorprogress.Attempt) collectorprogress.Snapshot {
	return collectorprogress.Snapshot{InstanceID: testInstance.InstanceID, Attempts: attempts}
}

func attempt(stage string, updatedAtMs int64) collectorprogress.Attempt {
	return collectorprogress.Attempt{AttemptID: "attempt-1", Stage: stage, Harnesses: map[string]string{"codex": "running"}, UpdatedAtMS: updatedAtMs}
}

// at is a time offset from clock.
func at(offset time.Duration) time.Time { return clock.Add(offset) }

// loaded loads instance into the harness's window.
func (h *harness) loaded(instance application.Instance) {
	h.t.Helper()
	h.window.Load()
	h.expectCall("instance")
	h.backend.instances <- instanceResult{instance: instance}
	h.apply()
}

// answerStatus answers a running status read and applies its result.
func (h *harness) answerStatus(status application.Status, err error) {
	h.backend.statuses <- statusResult{status: status, err: err}
	h.apply()
}

// answerProgress answers a running progress read and applies its result.
func (h *harness) answerProgress(progress collectorprogress.Snapshot, err error) {
	h.backend.progresses <- progressResult{progress: progress, err: err}
	h.apply()
}

// answerReads expects a status and a progress read and answers both.
func (h *harness) answerReads(status application.Status, progress collectorprogress.Snapshot) {
	h.t.Helper()
	h.expectCalls("status", "progress")
	h.answerStatus(status, nil)
	h.answerProgress(progress, nil)
}

func assertStatus(t *testing.T, w *Window, label string, state dashboardview.StatusState) {
	t.Helper()
	if got := w.StatusValue(); got.Label != label || got.State != state {
		t.Fatalf("status %+v, want %q %s", got, label, state)
	}
}

func TestPollReadsAfterInstanceEveryInterval(t *testing.T) {
	h := newHarness(t)
	w := h.window
	w.SetHidden(false) // a frame before the instance loaded
	h.advance(PollInterval)
	h.noCall()
	if _, ok := h.nextWake(); ok {
		t.Fatal("polling before the instance loaded")
	}
	assertStatus(t, w, "Loading…", dashboardview.StatusLoading)
	h.loaded(syncInstance)

	h.expectCalls("status", "progress")
	if wait, ok := h.nextWake(); !ok || wait <= PollInterval {
		t.Fatalf("a wake while both reads run: %v", wait)
	}
	h.advance(time.Second)
	h.noCall() // running reads coalesce
	assertStatus(t, w, "Loading…", dashboardview.StatusLoading)
	h.answerStatus(statusAt(1, 1), nil)
	h.answerProgress(progressOf(), nil)
	assertStatus(t, w, "Ready", dashboardview.StatusReady)
	if sync := w.SyncState(); sync.Identity != dashboardview.StatusIdentity(statusAt(1, 1)) || sync.Revision != 1 || !sync.AnalyticsEnabled {
		t.Fatalf("sync %+v", sync)
	}

	if wait, ok := h.nextWake(); !ok || wait != PollInterval-time.Second || h.armedCount() != 1 {
		t.Fatalf("wait %v, %v, %d timers", wait, ok, h.armedCount())
	}
	h.advance(PollInterval - 2*time.Second)
	h.noCall()
	h.advance(time.Second)
	h.answerReads(statusAt(1, 1), progressOf())
	if wait, ok := h.nextWake(); !ok || wait != PollInterval {
		t.Fatalf("wait after the second reads %v, %v", wait, ok)
	}
	h.noCall()
}

func TestPollPausesWhileHiddenAndReadsStatusOnShow(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(syncInstance)
	h.answerReads(statusAt(1, 1), progressOf())

	w.SetHidden(true)
	if h.armedCount() != 0 {
		t.Fatal("a hidden window kept its timer")
	}
	h.advance(3 * PollInterval)
	w.SetHidden(true) // a hidden window's later frames
	h.noCall()
	if h.armedCount() != 0 {
		t.Fatal("a hidden window armed a timer")
	}

	// Showing reads status at once; progress waits for its interval.
	h2 := newHarness(t)
	w2 := h2.window
	h2.loaded(syncInstance)
	h2.answerReads(statusAt(1, 1), progressOf())
	h2.advance(2 * time.Second)
	w2.SetHidden(true)
	h2.advance(time.Second)
	w2.SetHidden(false)
	h2.expectCall("status")
	h2.noCall()
	if wait, ok := h2.nextWake(); !ok || wait != 2*time.Second {
		t.Fatalf("wait for progress %v, %v", wait, ok)
	}
	h2.answerStatus(statusAt(1, 1), nil)
	if wait, ok := h2.nextWake(); !ok || wait != 2*time.Second || h2.armedCount() != 1 {
		t.Fatalf("wait for progress after status %v, %v", wait, ok)
	}

	// A window shown after its interval passed reads both at once.
	w.SetHidden(false)
	h.answerReads(statusAt(1, 1), progressOf())
	if wait, ok := h.nextWake(); !ok || wait != PollInterval {
		t.Fatalf("polling did not resume: %v, %v", wait, ok)
	}
}

func TestNoReadsAfterTheWindowCloses(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(syncInstance)
	h.answerReads(statusAt(1, 1), progressOf())
	h.cancel()
	h.apply() // closing stops the timer on the main thread
	if h.armedCount() != 0 {
		t.Fatal("a closed window kept its timer")
	}
	h.resetCounts()
	h.advance(3 * PollInterval)
	w.SetHidden(true)
	w.SetHidden(false)
	w.RefetchStatus()
	h.noCall()
	if h.armedCount() != 0 || h.invalidated != 0 {
		t.Fatalf("a closed window armed %d timers, redrew %d times", h.armedCount(), h.invalidated)
	}
}

func TestUnchangedPollsDrawNothing(t *testing.T) {
	h := newHarness(t)
	h.loaded(syncInstance)
	h.answerReads(statusAt(1, 1), progressOf(attempt(dashboardview.StageCapturing, 1)))
	if h.invalidated == 0 {
		t.Fatal("the first reads did not redraw")
	}
	h.resetCounts()
	for i := range 3 {
		h.advance(PollInterval)
		// Only the attempt's update time changes, which nothing shows.
		h.answerReads(statusAt(1, 1), progressOf(attempt(dashboardview.StageCapturing, int64(i+2))))
	}
	if h.updated != 0 || h.invalidated != 0 {
		t.Fatalf("unchanged polls built the view %d times, redrew %d times", h.updated, h.invalidated)
	}
	h.noCall()
}

func TestChangedReadsRedrawOnce(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(syncInstance)
	h.answerReads(statusAt(1, 1), progressOf())
	h.resetCounts()

	h.advance(PollInterval)
	pending := statusAt(1, 1)
	pending.Pending = 3
	h.answerReads(pending, progressOf())
	assertStatus(t, w, "Processing usage", dashboardview.StatusLoading)
	if h.updated != 0 || h.invalidated != 1 {
		t.Fatalf("a changed status built the view %d times, redrew %d times", h.updated, h.invalidated)
	}

	h.resetCounts()
	h.advance(PollInterval)
	pending.Pending = 2 // the note counts the entries pending
	h.answerReads(pending, progressOf())
	if h.invalidated != 1 {
		t.Fatalf("a changed pending count redrew %d times", h.invalidated)
	}

	h.resetCounts()
	h.advance(PollInterval)
	collected := attempt(dashboardview.StageCapturing, 1)
	collected.AcknowledgedEntries = 12
	h.answerReads(pending, progressOf(collected))
	if h.updated != 0 || h.invalidated != 1 {
		t.Fatalf("changed progress built the view %d times, redrew %d times", h.updated, h.invalidated)
	}
	h.resetCounts()
	h.advance(PollInterval)
	collected.AcknowledgedEntries = 40
	h.answerReads(pending, progressOf(collected))
	if h.invalidated != 1 {
		t.Fatalf("a changed attempt count redrew %d times", h.invalidated)
	}
}

func TestBackoffExpiryRedrawsOnceAtTheFirstReadAfterIt(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(syncInstance)
	backoff := statusAt(1, 1)
	backoff.Pending, backoff.Failed = 3, 1
	backoff.FailedRetryAtMs = clock.Add(PollInterval + 2*time.Second).UnixMilli()
	h.answerReads(backoff, progressOf())
	assertStatus(t, w, "Processing retry pending", dashboardview.StatusAttention)
	h.resetCounts()

	h.advance(PollInterval) // read before the retry time
	h.answerReads(backoff, progressOf())
	assertStatus(t, w, "Processing retry pending", dashboardview.StatusAttention)
	if h.invalidated != 0 {
		t.Fatalf("a backoff read before its retry time redrew %d times", h.invalidated)
	}

	h.advance(PollInterval) // read after it: the same status no longer waits
	h.answerReads(backoff, progressOf())
	assertStatus(t, w, "Processing usage", dashboardview.StatusLoading)
	if h.updated != 0 || h.invalidated != 1 {
		t.Fatalf("backoff expiry built the view %d times, redrew %d times", h.updated, h.invalidated)
	}
}

func TestServerDayChangeRedrawsAtMidnight(t *testing.T) {
	h := newHarness(t)
	instance := syncInstance
	instance.Capabilities = serverfeatures.Capabilities{}
	h.loaded(instance)
	h.expectCall("status") // left running: only the calendar's timer stays
	// clock is 09:00 in Europe/London, which keeps UTC in November.
	untilMidnight := 15 * time.Hour
	if wait, ok := h.nextWake(); !ok || wait != untilMidnight {
		t.Fatalf("wake %v, %v, want the server's midnight", wait, ok)
	}
	h.resetCounts()
	h.advance(untilMidnight - time.Second)
	if h.invalidated != 0 {
		t.Fatal("redrew before the day changed")
	}
	h.advance(time.Second)
	if h.invalidated != 1 || h.window.ServerToday() != "2026-11-14" {
		t.Fatalf("day change redrew %d times, today %s", h.invalidated, h.window.ServerToday())
	}
	if wait, ok := h.nextWake(); !ok || wait != 24*time.Hour {
		t.Fatalf("next day's wake %v, %v", wait, ok)
	}
	h.noCall()
}

func TestSnapshotChangesSignalAndReloadInstance(t *testing.T) {
	h := newHarness(t)
	w := h.window
	var changes []SnapshotChange
	w.OnSnapshotChange(func(change SnapshotChange) { changes = append(changes, change) })
	h.loaded(syncInstance)
	w.SelectTab(dashboardview.TabModels)
	h.answerReads(statusAt(1, 1), progressOf())
	if len(changes) != 1 || changes[0] != (SnapshotChange{IdentityChanged: true, RevisionChanged: true}) {
		t.Fatalf("first observation %+v", changes)
	}
	h.noCall() // the first observation keeps the instance just read

	reload := func(instance application.Instance) {
		t.Helper()
		h.expectCall("instance")
		h.backend.instances <- instanceResult{instance: instance}
		h.apply()
	}

	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(2, 1), nil)
	if len(changes) != 2 || changes[1] != (SnapshotChange{RevisionChanged: true}) {
		t.Fatalf("revision change %+v", changes)
	}
	renamed := syncInstance
	renamed.Hostname = "studio-2"
	reload(renamed)
	query, _ := w.Query()
	if w.Hostname() != "studio-2" || query.Tab != dashboardview.TabModels {
		t.Fatalf("reloaded instance reset the query: %s %+v", w.Hostname(), query)
	}

	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(2, 2), nil)
	if len(changes) != 3 || changes[2] != (SnapshotChange{IdentityChanged: true}) {
		t.Fatalf("identity change %+v", changes)
	}
	if w.SyncState().Identity != dashboardview.StatusIdentity(statusAt(2, 2)) {
		t.Fatal("identity not followed")
	}
	reload(syncInstance)

	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(2, 2), nil)
	if len(changes) != 3 {
		t.Fatalf("an unchanged snapshot signalled %+v", changes)
	}
	h.noCall()
}

func TestAcceptedAttemptRefetchesStatusOnce(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(syncInstance)
	h.answerReads(statusAt(1, 1), progressOf(attempt(dashboardview.StageCapturing, 1)))
	assertStatus(t, w, "Collecting usage", dashboardview.StatusLoading)
	if !w.RefreshPending() {
		t.Fatal("Refresh allowed while collecting")
	}

	h.advance(PollInterval)
	h.answerReads(statusAt(1, 1), progressOf(attempt(dashboardview.StageAccepted, 2)))
	if sync := w.SyncState(); !sync.CheckingAcceptance || !sync.CollectionPending {
		t.Fatalf("acceptance not checked: %+v", sync)
	}
	assertStatus(t, w, "Loading…", dashboardview.StatusLoading)
	h.expectCall("status")
	h.answerStatus(statusAt(2, 1), nil)
	if sync := w.SyncState(); sync.CheckingAcceptance || sync.CollectionPending || sync.Revision != 2 {
		t.Fatalf("acceptance check did not finish: %+v", sync)
	}
	h.expectCall("instance") // the revision changed
	h.backend.instances <- instanceResult{instance: syncInstance}
	h.apply()
	assertStatus(t, w, "Ready", dashboardview.StatusReady)

	h.advance(PollInterval)
	h.answerReads(statusAt(2, 1), progressOf(attempt(dashboardview.StageAccepted, 2)))
	h.noCall()
}

func TestAcceptanceSeenDuringAStatusReadReadsAgain(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(syncInstance)
	h.expectCalls("status", "progress")
	h.answerProgress(progressOf(attempt(dashboardview.StageAccepted, 2)), nil)
	h.answerStatus(statusAt(1, 1), nil) // started before the acceptance
	if !w.SyncState().CheckingAcceptance {
		t.Fatal("a read started before the acceptance checked it")
	}
	h.expectCall("status")
	h.answerStatus(statusAt(1, 1), nil)
	if w.SyncState().CheckingAcceptance {
		t.Fatal("the read after the acceptance did not check it")
	}
	h.noCall()
}

func TestReadFailuresNeedAttention(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(syncInstance)
	h.expectCalls("status", "progress")
	h.answerStatus(application.Status{}, errors.New("backend busy"))
	h.answerProgress(progressOf(attempt(dashboardview.StageCapturing, 1)), nil)
	assertStatus(t, w, "Connection unavailable", dashboardview.StatusAttention)
	if !w.ConnectionUnavailable() {
		t.Fatal("status failure kept the connection available")
	}

	h.advance(PollInterval)
	h.expectCalls("status", "progress")
	h.answerStatus(statusAt(1, 1), nil)
	h.answerProgress(collectorprogress.Snapshot{}, errors.New("progress unavailable"))
	assertStatus(t, w, "Collection status unavailable", dashboardview.StatusAttention)
	if sync := w.SyncState(); !sync.CollectionUnknown || sync.CollectionStage != "" || w.ConnectionUnavailable() {
		t.Fatalf("progress failure: %+v", sync)
	}
	if progress, ok := w.Progress(); !ok || len(progress.Attempts) != 1 {
		t.Fatal("a failed read dropped the last progress")
	}

	h.advance(PollInterval)
	h.expectCalls("status", "progress")
	h.answerStatus(statusAt(1, 1), nil)
	foreign := progressOf()
	foreign.InstanceID = "another-instance"
	h.answerProgress(foreign, nil)
	assertStatus(t, w, "Collection status unavailable", dashboardview.StatusAttention)

	h.advance(PollInterval)
	h.answerReads(statusAt(1, 1), progressOf(attempt(dashboardview.StageFailed, 3)))
	assertStatus(t, w, "Collection needs attention", dashboardview.StatusAttention)
}

func TestInstanceFailureIsAnUnavailableConnection(t *testing.T) {
	h := newHarness(t)
	w := h.window
	w.Load()
	h.expectCall("instance")
	h.backend.instances <- instanceResult{err: errors.New("backend busy")}
	h.apply()
	assertStatus(t, w, "Connection unavailable", dashboardview.StatusAttention)
}

func TestRefreshRereadsStatusAndProgress(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(syncInstance)
	h.answerReads(statusAt(1, 1), progressOf())
	w.Refresh()
	assertStatus(t, w, "Refreshing usage…", dashboardview.StatusLoading)
	h.expectCall("capture")
	h.backend.captures <- nil
	h.apply()
	h.answerReads(statusAt(1, 1), progressOf())
	assertStatus(t, w, "Ready", dashboardview.StatusReady)
	h.noCall()
}

func TestProcessingPendingAndBackoff(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(syncInstance)
	pending := statusAt(1, 1)
	pending.Pending = 3
	h.answerReads(pending, progressOf())
	assertStatus(t, w, "Processing usage", dashboardview.StatusLoading)

	h.advance(PollInterval)
	backoff := pending
	backoff.Failed, backoff.FailedRetryAtMs = 1, clock.Add(time.Minute).UnixMilli()
	h.answerReads(backoff, progressOf())
	assertStatus(t, w, "Processing retry pending", dashboardview.StatusAttention)
}

func TestProgressStaysHiddenWithoutItsCapability(t *testing.T) {
	h := newHarness(t)
	w := h.window
	instance := syncInstance
	instance.Capabilities = serverfeatures.Capabilities{}
	h.loaded(instance)
	h.expectCall("status")
	if wait, ok := h.nextWake(); !ok || wait <= PollInterval {
		t.Fatalf("a read wake while the only read runs: %v", wait)
	}
	h.answerStatus(statusAt(1, 1), nil)
	h.noCall()
	if _, ok := w.Progress(); ok || w.SyncState().CollectionPending {
		t.Fatal("progress without the capability")
	}
	assertStatus(t, w, "Ready", dashboardview.StatusReady)
	h.advance(PollInterval)
	h.expectCall("status")
	h.noCall()
}
