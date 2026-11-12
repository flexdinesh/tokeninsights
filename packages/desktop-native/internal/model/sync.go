package model

import (
	"errors"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

// PollInterval is how often a showing window reads backend status and
// collector progress, as the web dashboard's dashboardPollIntervalMs.
const PollInterval = 5 * time.Second

// errForeignProgress rejects progress of another backend instance than
// the window's, as the web's collector progress read does.
var errForeignProgress = errors.New("collector progress belongs to another server instance")

// SnapshotChange is how a status read changed the published snapshot the
// window shows. Data read under another identity is stale and goes; data
// of an older revision is read again.
type SnapshotChange struct {
	IdentityChanged bool
	RevisionChanged bool
}

// syncState is the window's synchronization with backend status and
// collector progress, the web's useDashboardSync: the latest reads, which
// reads run and when the next are due.
type syncState struct {
	status       *application.Status
	statusReadAt time.Time
	statusErr    error
	// statusRunning coalesces reads; statusAgain reads once more after the
	// running one, for a request that must see a read started after it.
	statusRunning, statusAgain bool
	statusDue                  time.Time

	progress                       *collectorprogress.Snapshot
	progressErr                    error
	progressRunning, progressAgain bool
	progressDue                    time.Time

	// hidden is whether the window was hidden at the last poll.
	hidden bool

	// checkNext is the acceptance transition the next status read checks;
	// requested is the last one asked for and checked the last one whose
	// status read completed (dashboardview.SyncInput.CheckedAcceptance).
	checkNext, requested, checked string

	// observed is whether a status read succeeded; identity and revision
	// are the last one's.
	observed  bool
	identity  string
	revision  int64
	listeners []func(SnapshotChange)
}

// Poll starts the status and progress reads due at now while the window
// shows, and returns how long until the next is due; ok is false when
// none is: while hidden, before the instance loads, or while every read
// runs, whose result builds the view again. Reads start once the instance
// loaded, as the web's start after bootstrap. Showing a hidden window
// reads status at once, as the web's refetch on focus; progress keeps its
// own interval. Views call it as they build, with the frame's time.
func (w *Window) Poll(now time.Time, hidden bool) (wait time.Duration, ok bool) {
	s := &w.sync
	if hidden {
		s.hidden = true
		return 0, false
	}
	if s.hidden {
		s.hidden, s.statusDue = false, time.Time{}
	}
	if w.instance == nil || w.rt.Context.Err() != nil {
		return 0, false
	}
	if !s.statusRunning && !now.Before(s.statusDue) {
		s.statusDue = now.Add(PollInterval)
		w.readStatus()
	}
	if w.progressEnabled() && !s.progressRunning && !now.Before(s.progressDue) {
		s.progressDue = now.Add(PollInterval)
		w.readProgress()
	}
	var next time.Time
	if !s.statusRunning {
		next = s.statusDue
	}
	if w.progressEnabled() && !s.progressRunning && (next.IsZero() || s.progressDue.Before(next)) {
		next = s.progressDue
	}
	if next.IsZero() {
		return 0, false
	}
	return next.Sub(now), true
}

// RefetchStatus reads status now, whether the window shows or not. A
// request while a read runs reads once more after it, so the result
// reflects a read started after the request; requests then coalesce.
// Later reads (usage, facets) call it when they answered from a newer
// or another snapshot than the status, as the web's do.
func (w *Window) RefetchStatus() {
	if w.sync.statusRunning {
		w.sync.statusAgain = true
		return
	}
	w.readStatus()
}

func (w *Window) readStatus() {
	if w.instance == nil || w.rt.Context.Err() != nil {
		return
	}
	s := &w.sync
	s.statusRunning = true
	checking := s.checkNext
	s.checkNext = ""
	ctx := w.rt.Context
	w.rt.Go(func() {
		status, err := w.backend.Status(ctx)
		w.rt.Update(func() { w.statusRead(status, err, checking) })
	})
}

func (w *Window) statusRead(status application.Status, err error, checking string) {
	s := &w.sync
	s.statusRunning = false
	if checking != "" {
		s.checked = checking
	}
	if err != nil {
		s.statusErr = err
	} else {
		s.status, s.statusReadAt, s.statusErr = &status, w.rt.Now(), nil
		w.observe(status)
	}
	if s.statusAgain {
		s.statusAgain = false
		w.readStatus()
	}
	w.syncReads()
}

// observe tells listeners how status changed the snapshot. A change after
// the first observation reads the instance again, as the web's
// invalidation of its bootstrap does.
func (w *Window) observe(status application.Status) {
	s := &w.sync
	identity, revision := dashboardview.StatusIdentity(status), status.Metadata.Revision
	change := SnapshotChange{IdentityChanged: identity != s.identity, RevisionChanged: revision != s.revision}
	first := !s.observed
	s.observed, s.identity, s.revision = true, identity, revision
	if !change.IdentityChanged && !change.RevisionChanged {
		return
	}
	if !first {
		w.loadInstance()
	}
	for _, listener := range s.listeners {
		listener(change)
	}
}

// OnSnapshotChange calls fn on the main thread whenever a status read
// changes the snapshot's identity or revision, the first read included.
// Owners of usage and facets drop their data on an identity change and
// read it again on a revision change.
func (w *Window) OnSnapshotChange(fn func(SnapshotChange)) {
	w.sync.listeners = append(w.sync.listeners, fn)
}

// progressEnabled is whether the instance shows collector progress, as the
// web's showsCollectorProgress.
func (w *Window) progressEnabled() bool {
	return w.instance != nil && w.instance.Permissions.Has(serverfeatures.Read) &&
		w.instance.Capabilities.Has(serverfeatures.CollectorProgress)
}

// readProgress reads collector progress unless the instance hides it; a
// request while a read runs reads once more after it.
func (w *Window) readProgress() {
	if !w.progressEnabled() || w.rt.Context.Err() != nil {
		return
	}
	s := &w.sync
	if s.progressRunning {
		s.progressAgain = true
		return
	}
	s.progressRunning = true
	ctx, instanceID := w.rt.Context, w.instance.InstanceID
	w.rt.Go(func() {
		snapshot, err := w.backend.Progress(ctx)
		if err == nil && snapshot.InstanceID != instanceID {
			err = errForeignProgress
		}
		w.rt.Update(func() { w.progressRead(snapshot, err) })
	})
}

func (w *Window) progressRead(snapshot collectorprogress.Snapshot, err error) {
	s := &w.sync
	s.progressRunning = false
	if err != nil {
		s.progressErr = err
	} else {
		s.progress, s.progressErr = &snapshot, nil
	}
	w.checkAcceptance()
	if s.progressAgain {
		s.progressAgain = false
		w.readProgress()
	}
}

// checkAcceptance reads status once for each newly accepted attempt, so
// collection counts as finished only once status saw what it accepted.
func (w *Window) checkAcceptance() {
	s := &w.sync
	transition := dashboardview.AcceptedTransition(w.attempts())
	if transition == "" || transition == s.checked || transition == s.requested {
		return
	}
	s.requested, s.checkNext = transition, transition
	w.RefetchStatus()
}

func (w *Window) attempts() []collectorprogress.Attempt {
	if w.sync.progress == nil {
		return nil
	}
	return w.sync.progress.Attempts
}

// SyncState is the window's derived synchronization state.
func (w *Window) SyncState() dashboardview.SyncState {
	s := &w.sync
	input := dashboardview.SyncInput{
		Status: s.status, ProgressEnabled: w.progressEnabled(), Attempts: w.attempts(),
		ProgressPending: s.progress == nil && s.progressErr == nil, ProgressFailed: s.progressErr != nil,
		CheckedAcceptance: s.checked,
	}
	if s.status != nil {
		input.StatusReadAtMs = s.statusReadAt.UnixMilli()
	}
	if w.instance != nil {
		input.DatasetID = w.instance.DatasetID
	}
	return dashboardview.DeriveSync(input)
}

// StatusValue is the header status: the single most important state, in
// the web's priority. A failed instance or status read is an unavailable
// connection.
func (w *Window) StatusValue() dashboardview.StatusValue {
	input := dashboardview.StatusInput{
		Sync:             w.SyncState(),
		ConnectionFailed: w.instanceErr != nil || w.sync.statusErr != nil,
		RefreshFailed:    w.refreshErr != nil,
		RefreshPending:   w.refreshing,
	}
	if w.instance != nil {
		input.UsageCapability = w.instance.Capabilities.Has(serverfeatures.Usage)
		input.FacetsCapability = w.instance.Capabilities.Has(serverfeatures.Facets)
	}
	input.UsageFailed, input.FacetsFailed = w.usage.err != nil, w.facets.err != nil
	input.UsageFetching, input.UsageLoaded = w.UsageFetching(), w.usage.data != nil
	input.FacetsPending = w.FacetsPending()
	return dashboardview.DashboardStatus(input)
}

// ConnectionUnavailable reports whether the last status read failed.
func (w *Window) ConnectionUnavailable() bool { return w.sync.statusErr != nil }

// ConnectionError is why the backend is unavailable: the failed instance
// read, else the failed status read, or nil.
func (w *Window) ConnectionError() error {
	if w.instanceErr != nil {
		return w.instanceErr
	}
	return w.sync.statusErr
}

// RetryConnection reads again what made the connection unavailable: the
// instance when it failed, else the status, as the web's Retry Request.
func (w *Window) RetryConnection() {
	if w.instanceErr != nil {
		w.loadInstance()
		return
	}
	w.RefetchStatus()
}

// RetryProgress reads collector progress again, as the web's Retry
// Request when collection status could not load.
func (w *Window) RetryProgress() { w.readProgress() }

// Status is the latest status read, kept through a failed read; ok is
// false before one succeeded.
func (w *Window) Status() (application.Status, bool) {
	if w.sync.status == nil {
		return application.Status{}, false
	}
	return *w.sync.status, true
}

// RefreshPending reports whether Refresh should wait: a refresh or a
// collection runs, as the web's Refresh button.
func (w *Window) RefreshPending() bool { return w.refreshing || w.SyncState().CollectionPending }

// Progress is the latest collector progress while the instance shows it,
// kept through a failed read; ok is false before one succeeded.
func (w *Window) Progress() (collectorprogress.Snapshot, bool) {
	if !w.progressEnabled() || w.sync.progress == nil {
		return collectorprogress.Snapshot{}, false
	}
	return *w.sync.progress, true
}
