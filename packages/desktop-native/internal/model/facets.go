package model

import (
	"context"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

// SessionSearchDelay is how long the session filter's search text rests
// before it is searched, as the web's SessionFilter debounce.
const SessionSearchDelay = 200 * time.Millisecond

// facetsKey is one facets read: the request sent and the snapshot it must
// answer from, the web's facets query key.
type facetsKey struct {
	request  application.FacetRequest
	identity string
	revision int64
}

func (k facetsKey) equal(other facetsKey) bool {
	return k.identity == other.identity && k.revision == other.revision &&
		k.request.Search == other.request.Search && sameRequest(k.request.Query, other.request.Query)
}

// sameSearch reports whether two keys ask the same of the same snapshot
// identity, whatever its revision.
func (k facetsKey) sameSearch(other facetsKey) bool {
	return k.identity == other.identity && k.request.Search == other.request.Search &&
		sameRequest(k.request.Query, other.request.Query)
}

// facetsState is one facets read of the window, the web's useFacets: one
// read at a time, numbered so a superseded answer is dropped.
type facetsState struct {
	seq    uint64
	cancel context.CancelFunc
	// running is whether the latest read runs; requested is its key.
	running   bool
	requested *facetsKey
	// data is the latest answer, kept while another read runs or fails,
	// as keeps allows; dataKey is the read it answered.
	data    *application.Facets
	dataKey facetsKey
	err     error
	// awaitingStatus is whether an answer from another snapshot waits for
	// the status read it requested; staleRetried whether the read was
	// already repeated once for that, as the web's single retry.
	awaitingStatus, staleRetried bool
	// anyRequest keeps data of the snapshot identity for any request
	// while the next answers; otherwise data stays only for its own
	// request.
	anyRequest bool
}

// keeps reports whether data of dataKey still shows while next reads.
func (f *facetsState) keeps(next facetsKey) bool {
	if f.anyRequest {
		return f.dataKey.identity == next.identity
	}
	return f.dataKey.sameSearch(next)
}

// pending reports whether the latest read has neither answered nor failed:
// the web's pending facets.
func (f *facetsState) pending() bool {
	return f.err == nil && (f.data == nil || f.requested == nil || !f.requested.equal(f.dataKey))
}

// drop forgets the read and its data, cancelling a running read, as the
// web removes facets of another snapshot identity.
func (f *facetsState) drop() {
	if f.cancel != nil {
		f.cancel()
	}
	*f = facetsState{seq: f.seq + 1, anyRequest: f.anyRequest}
}

// sessionSearch is the session filter's server search: the text typed,
// when it is due to be searched, and the facets read of the text searched.
type sessionSearch struct {
	text, searched string
	due            time.Time
	read           facetsState
}

// facetsWanted is the facets read of search the window should show, and
// whether it may read: the instance offers facets and the status permits
// analytics of its dataset.
func (w *Window) facetsWanted(search string) (facetsKey, bool) {
	if w.instance == nil || !w.instance.Capabilities.Has(serverfeatures.Facets) {
		return facetsKey{}, false
	}
	sync := w.SyncState()
	if !sync.AnalyticsEnabled {
		return facetsKey{}, false
	}
	return facetsKey{request: w.query.FacetRequest(search), identity: sync.Identity, revision: sync.Revision}, true
}

// syncFacets reads the query's filter values and, while a session search
// is due, its matches, when their wanted reads differ from the latest.
func (w *Window) syncFacets() {
	key, ok := w.facetsWanted("")
	w.syncFacetsRead(&w.facets, key, ok)
	if search := w.sessions.searched; search != "" {
		key, ok := w.facetsWanted(search)
		w.syncFacetsRead(&w.sessions.read, key, ok)
	}
}

// syncFacetsRead reads key into f when it differs from f's latest read,
// as syncUsage does: a failed read stays failed until retried, and an
// answer from another snapshot is read once more after status.
func (w *Window) syncFacetsRead(f *facetsState, key facetsKey, ok bool) {
	if !ok || w.rt.Context.Err() != nil {
		return
	}
	if f.awaitingStatus {
		if w.sync.statusRunning {
			return
		}
		f.awaitingStatus = false
		if f.requested != nil && f.requested.equal(key) {
			if f.staleRetried {
				f.err = ErrDataChanged
				return
			}
			f.staleRetried = true
			w.readFacets(f, key)
			return
		}
	}
	if f.requested != nil && f.requested.equal(key) {
		return
	}
	f.staleRetried = false
	w.readFacets(f, key)
}

// readFacets starts the read of key into f, superseding any running one.
func (w *Window) readFacets(f *facetsState, key facetsKey) {
	if f.cancel != nil {
		f.cancel()
	}
	f.seq++
	seq := f.seq
	ctx, cancel := context.WithCancel(w.rt.Context)
	f.cancel, f.running, f.requested, f.err = cancel, true, &key, nil
	if f.data != nil && !f.keeps(key) {
		f.data = nil
	}
	w.rt.Go(func() {
		facets, err := w.backend.Facets(ctx, key.request)
		w.rt.Update(func() { w.facetsRead(f, seq, key, facets, err) })
	})
}

// facetsRead applies the answer of read seq of f unless a later read
// superseded it, checking its snapshot as usageRead does.
func (w *Window) facetsRead(f *facetsState, seq uint64, key facetsKey, facets application.Facets, err error) {
	if seq != f.seq {
		return
	}
	f.running = false
	f.cancel()
	f.cancel = nil
	if err != nil {
		f.err = err
		return
	}
	if dashboardview.FacetsIdentity(facets) != key.identity || facets.Revision < key.revision {
		f.awaitingStatus = true
		w.RefetchStatus()
		return
	}
	if facets.Revision > key.revision {
		w.RefetchStatus()
	}
	f.data, f.dataKey, f.staleRetried = &facets, key, false
}

// retryFacets reads f's wanted key of search again.
func (w *Window) retryFacets(f *facetsState, search string) {
	key, ok := w.facetsWanted(search)
	if !ok || w.rt.Context.Err() != nil {
		return
	}
	f.awaitingStatus, f.staleRetried = false, false
	w.readFacets(f, key)
}

// facetsSnapshotChanged drops facets of another snapshot identity and
// their running reads, as the web removes its cached facets.
func (w *Window) facetsSnapshotChanged(change SnapshotChange) {
	if !change.IdentityChanged {
		return
	}
	w.facets.drop()
	w.sessions.read.drop()
}

// Facets are the filter values the window offers for the query: those of
// its latest answer, kept while a read for a changed query or revision of
// the same snapshot identity runs or fails; ok is false before one
// answered.
func (w *Window) Facets() (application.Facets, bool) {
	if w.facets.data == nil {
		return application.Facets{}, false
	}
	return *w.facets.data, true
}

// FacetsError is why the latest facets read failed, or nil.
func (w *Window) FacetsError() error { return w.facets.err }

// FacetsPending reports whether the facets of the query have neither
// answered nor failed yet.
func (w *Window) FacetsPending() bool { return w.facets.pending() }

// RetryFacets reads the query's facets again, the facets banner's Retry
// Request.
func (w *Window) RetryFacets() { w.retryFacets(&w.facets, "") }

// SessionSearch is the session filter's search text as typed.
func (w *Window) SessionSearch() string { return w.sessions.text }

// SearchSessions sets the session filter's search text, typed at now. The
// backend searches it once it rested SessionSearchDelay, as
// PollSessionSearch finds.
func (w *Window) SearchSessions(text string, now time.Time) {
	s := &w.sessions
	if text == s.text {
		return
	}
	s.text, s.due = text, now.Add(SessionSearchDelay)
}

// PollSessionSearch searches the session search text once it rested
// SessionSearchDelay at now, and returns how long until it will have; ok
// is false when no search is due. Views call it as they build, with the
// frame's time, and wake after wait.
func (w *Window) PollSessionSearch(now time.Time) (wait time.Duration, ok bool) {
	s := &w.sessions
	if s.due.IsZero() {
		return 0, false
	}
	if now.Before(s.due) {
		return s.due.Sub(now), true
	}
	s.searched, s.due = s.text, time.Time{}
	w.syncFacets()
	return 0, false
}

// SessionChoices are the sessions the session filter offers: the query's
// facets while nothing is searched, else the matches of the search text
// once it was searched.
type SessionChoices struct {
	Values []string
	// Searching is whether search text is typed: the choices are the
	// server's first matches of it.
	Searching bool
	// Loading is whether the matches of the text typed are on their way.
	Loading bool
	// Err is why the search of the text typed failed, or nil.
	Err error
}

// SessionChoices are the session filter's choices, as the web's
// SessionFilter: no matches while the text typed waits to be searched.
func (w *Window) SessionChoices() SessionChoices {
	s := &w.sessions
	if s.text == "" {
		choices := SessionChoices{}
		if w.facets.data != nil {
			choices.Values = w.facets.data.Sessions
		}
		return choices
	}
	if s.text != s.searched {
		return SessionChoices{Searching: true, Loading: true}
	}
	r := &s.read
	choices := SessionChoices{Searching: true, Loading: r.running || r.awaitingStatus, Err: r.err}
	if r.data != nil {
		choices.Values = r.data.Sessions
	}
	return choices
}

// RetrySessionSearch searches the session search text again.
func (w *Window) RetrySessionSearch() {
	if w.sessions.searched == "" {
		return
	}
	w.retryFacets(&w.sessions.read, w.sessions.searched)
}

// syncReads reads what the window shows whose wanted read changed with
// the query or the snapshot: usage, facets and the session search.
func (w *Window) syncReads() {
	w.syncUsage()
	w.syncFacets()
}
