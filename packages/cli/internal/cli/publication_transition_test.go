package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
)

func TestInitialFilterWaitsForValidatedPublication(t *testing.T) {
	m := newInteractiveModel(t.Context(), tableOptions{}, time.Now(), "unknown")
	defer m.cancelSync()
	updated, cmd := m.openFilterValues(filterProvider)
	m = updated.(interactiveModel)
	if cmd != nil || !m.filterLoading || len(m.filterValues) != 0 {
		t.Fatal("initial filter queried before publication identity was established")
	}
}

func TestSnapshotReplacementUsesTheSamePublicationTransition(t *testing.T) {
	m := newInteractiveModel(t.Context(), tableOptions{}, time.Now(), "unknown")
	defer m.cancelSync()
	m.instanceID, m.dataEpoch, m.observedRevision = "old", "old-epoch", 9
	m.syncing, m.snapshotAllowed = true, true
	_, sequence := m.queryContext(m.queries)
	updated, cmd := m.Update(snapshotMsg{reloadMsg{requestID: sequence, instanceID: "new", dataEpoch: "new-epoch", revision: 1, rows: []renderRow{{bucket: "new"}}}})
	m = updated.(interactiveModel)
	if cmd != nil || m.instanceID != "new" || m.observedRevision != 1 || !m.showingSnapshot || m.reloadInFlight || len(m.rows) != 1 || m.rows[0].bucket != "new" {
		t.Fatal("snapshot replacement bypassed publication transition or queued a retry")
	}
	updated, cmd = m.Update(snapshotMsg{reloadMsg{requestID: sequence, instanceID: "old", dataEpoch: "old-epoch", revision: 9}})
	m = updated.(interactiveModel)
	if cmd != nil || m.instanceID != "new" || len(m.rows) != 1 {
		t.Fatal("late snapshot reverted the publication")
	}
}

func TestReadOnlyHTTPReloadAcceptsRestartWithoutQueryLoop(t *testing.T) {
	database, path := newLoadRowsTestDB(t)
	defer func() { _ = database.Close() }()
	insertLoadRowsCanonicalToken(t, database, time.Now().UnixMilli(), "pi", "saved", "provider", "model")
	oldHandler := queryHandler(t, path, "old")
	newHandler := queryHandler(t, path, "new")
	var replaced atomic.Bool
	var requests atomic.Int64
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("viewer wrote server: %s", r.Method)
		}
		requests.Add(1)
		if replaced.Load() {
			newHandler.ServeHTTP(w, r)
		} else {
			oldHandler.ServeHTTP(w, r)
		}
	}))
	defer remote.Close()
	m := newInteractiveModel(t.Context(), tableOptions{serverURL: remote.URL, period: periodAllTime, bucket: bucketDay}, time.Now(), "unknown")
	defer m.cancelSync()
	updated, _ := m.Update(m.reloadCmd()())
	m = updated.(interactiveModel)
	if m.err != nil || m.instanceID != "old" || len(m.rows) != 1 {
		t.Fatalf("initial snapshot: %+v", m)
	}
	replaced.Store(true)
	before := requests.Load()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(interactiveModel)
	if cmd == nil {
		t.Fatal("missing explicit reload")
	}
	updated, followup := m.Update(cmd())
	m = updated.(interactiveModel)
	if m.err != nil || m.instanceID != "new" || len(m.rows) != 1 || followup != nil || requests.Load()-before != 4 {
		t.Fatalf("replacement retried valid snapshot: instance=%s err=%v followup=%v requests=%d", m.instanceID, m.err, followup != nil, requests.Load()-before)
	}
}

func TestUnavailableStatusClearsSavedDataAndRecoversSameRevision(t *testing.T) {
	m := newInteractiveModel(t.Context(), tableOptions{filters: filters{models: stringList{"selected"}}}, time.Now(), "unknown")
	defer m.cancelSync()
	updated, _ := m.Update(reloadMsg{instanceID: "instance", dataEpoch: "epoch", revision: 3, rows: []renderRow{{bucket: "saved"}}})
	m = updated.(interactiveModel)
	_, statusSequence := m.queryContext(m.statusQueries)
	_, usageSequence := m.queryContext(m.queries)
	oldGeneration := m.publicationGeneration
	updated, _ = m.Update(sharedSyncMsg{requestID: statusSequence, generation: oldGeneration, instanceID: "instance", dataEpoch: "", readiness: "unavailable"})
	m = updated.(interactiveModel)
	if len(m.rows) != 0 || m.err != queryclient.ErrUnavailable || m.serviceReadiness != "unavailable" || m.instanceID != "instance" || m.dataEpoch != "epoch" || !reflect.DeepEqual(m.options.filters.models, stringList{"selected"}) {
		t.Fatal("unavailable metadata established an empty identity or retained saved data")
	}
	updated, _ = m.Update(reloadMsg{requestID: usageSequence, generation: oldGeneration, instanceID: "instance", dataEpoch: "epoch", revision: 3, rows: []renderRow{{bucket: "late"}}})
	m = updated.(interactiveModel)
	if len(m.rows) != 0 {
		t.Fatal("late successful read restored unavailable data")
	}
	updated, cmd := m.Update(sharedSyncMsg{generation: m.publicationGeneration, instanceID: "instance", dataEpoch: "epoch", readiness: "ready", status: querymodel.SyncStatus{Revision: 3}})
	m = updated.(interactiveModel)
	if cmd == nil || !m.reloadInFlight || m.serviceReadiness != "ready" {
		t.Fatal("storage recovery at the same revision did not schedule a read")
	}
}

func TestNewerStatusDuringReadRetriesOnNextPollWithoutHotLoop(t *testing.T) {
	m := newInteractiveModel(t.Context(), tableOptions{}, time.Now(), "unknown")
	defer m.cancelSync()
	m.instanceID, m.dataEpoch, m.serviceReadiness, m.observedRevision = "instance", "epoch", "ready", 1
	m.sharedSync.Revision = 1
	m.reloadInFlight = true
	_, sequence := m.queryContext(m.queries)
	updated, _ := m.Update(sharedSyncMsg{instanceID: "instance", dataEpoch: "epoch", readiness: "ready", status: querymodel.SyncStatus{Revision: 2}})
	m = updated.(interactiveModel)
	if !m.reloadInFlight {
		t.Fatal("status discarded the in-flight read")
	}
	updated, immediate := m.Update(reloadMsg{requestID: sequence, instanceID: "instance", dataEpoch: "epoch", revision: 1})
	m = updated.(interactiveModel)
	if immediate != nil || m.err != queryclient.ErrSnapshotChanged {
		t.Fatal("old revision retried immediately or was accepted")
	}
	updated, cmd := m.Update(sharedSyncMsg{instanceID: "instance", dataEpoch: "epoch", readiness: "ready", status: querymodel.SyncStatus{Revision: 2}})
	m = updated.(interactiveModel)
	if cmd == nil || !m.reloadInFlight {
		t.Fatal("next status poll did not retry the pending revision")
	}
}

func TestDashboardReplacementAcceptsSnapshotAndInvalidatesOldReaders(t *testing.T) {
	m := newInteractiveModel(t.Context(), tableOptions{filters: filters{providers: stringList{"selected"}}}, time.Now(), "unknown")
	defer m.cancelSync()
	m.instanceID, m.dataEpoch, m.observedRevision = "old", "old-epoch", 9
	m.popup, m.filterDimension = popupFilterValues, filterProvider
	m.filterValues = []string{"old-provider"}
	oldFacetContext, oldFacetSequence := m.queryContext(m.filterQueries)
	oldStatusContext, oldStatusSequence := m.queryContext(m.statusQueries)
	_, sequence := m.queryContext(m.queries)
	oldGeneration := m.publicationGeneration
	updated, _ := m.Update(reloadMsg{requestID: sequence, generation: oldGeneration, instanceID: "new", dataEpoch: "new-epoch", revision: 1, rows: []renderRow{{bucket: "new"}}})
	m = updated.(interactiveModel)
	if m.instanceID != "new" || m.dataEpoch != "new-epoch" || m.observedRevision != 1 || len(m.rows) != 1 || m.rows[0].bucket != "new" || m.reloadInFlight {
		t.Fatal("valid replacement snapshot was rejected or queued another dashboard read")
	}
	if oldFacetContext.Err() != context.Canceled || oldStatusContext.Err() != context.Canceled || len(m.filterValues) != 0 || !reflect.DeepEqual(m.options.filters.providers, stringList{"selected"}) {
		t.Fatal("replacement did not cancel obsolete reads and preserve selections")
	}
	updated, cmd := m.Update(sharedSyncMsg{requestID: oldStatusSequence, generation: oldGeneration, instanceID: "old", dataEpoch: "old-epoch", readiness: "ready", status: querymodel.SyncStatus{Revision: 99}})
	m = updated.(interactiveModel)
	if cmd != nil || m.instanceID != "new" || m.observedRevision != 1 || len(m.rows) != 1 {
		t.Fatal("late status reverted the new publication")
	}
	updated, _ = m.Update(filterValuesMsg{requestID: oldFacetSequence, generation: oldGeneration, instanceID: "old", dataEpoch: "old-epoch", revision: 9, dimension: filterProvider, values: []string{"old-provider"}})
	m = updated.(interactiveModel)
	if len(m.filterValues) != 0 {
		t.Fatal("late facets populated the replacement publication")
	}
}

func TestInitialDashboardIdentityRejectsEarlierFacetsAndStatus(t *testing.T) {
	m := newInteractiveModel(t.Context(), tableOptions{}, time.Now(), "unknown")
	defer m.cancelSync()
	m.popup, m.filterDimension = popupFilterValues, filterProvider
	_, facetSequence := m.queryContext(m.filterQueries)
	_, statusSequence := m.queryContext(m.statusQueries)
	_, usageSequence := m.queryContext(m.queries)
	updated, _ := m.Update(reloadMsg{requestID: usageSequence, instanceID: "new", dataEpoch: "new-epoch", revision: 4, rows: []renderRow{{bucket: "new"}}})
	m = updated.(interactiveModel)
	updated, _ = m.Update(sharedSyncMsg{requestID: statusSequence, instanceID: "old", dataEpoch: "old-epoch", readiness: "ready", status: querymodel.SyncStatus{Revision: 100}})
	m = updated.(interactiveModel)
	updated, _ = m.Update(filterValuesMsg{requestID: facetSequence, instanceID: "old", dataEpoch: "old-epoch", revision: 100, dimension: filterProvider, values: []string{"old-provider"}})
	m = updated.(interactiveModel)
	if m.instanceID != "new" || m.observedRevision != 4 || len(m.filterValues) != 0 || len(m.rows) != 1 {
		t.Fatal("initial dashboard identity was contaminated by earlier reads")
	}
}

func TestSamePublicationRevisionCannotMoveBackwards(t *testing.T) {
	m := newInteractiveModel(t.Context(), tableOptions{}, time.Now(), "unknown")
	defer m.cancelSync()
	m.instanceID, m.dataEpoch, m.observedRevision = "instance", "epoch", 7
	m.sharedSync.Revision = 7
	m.popup, m.filterDimension = popupFilterValues, filterProvider
	_, sequence := m.queryContext(m.filterQueries)
	updated, cmd := m.Update(filterValuesMsg{requestID: sequence, instanceID: "instance", dataEpoch: "epoch", revision: 6, dimension: filterProvider, values: []string{"stale"}})
	m = updated.(interactiveModel)
	if cmd != nil || len(m.filterValues) != 0 || m.filterErr == nil {
		t.Fatal("older facet publication was accepted or retried immediately")
	}
	updated, _ = m.Update(sharedSyncMsg{instanceID: "instance", dataEpoch: "epoch", readiness: "ready", status: querymodel.SyncStatus{Revision: 6}})
	m = updated.(interactiveModel)
	if m.sharedSync.Revision != 7 || m.observedRevision != 7 {
		t.Fatal("older status lowered the observed revision")
	}
	_, sequence = m.queryContext(m.queries)
	updated, cmd = m.Update(reloadMsg{requestID: sequence, instanceID: "instance", dataEpoch: "epoch", revision: 6, rows: []renderRow{{bucket: "stale"}}})
	m = updated.(interactiveModel)
	if cmd != nil || len(m.rows) != 0 || m.observedRevision != 7 || m.err == nil {
		t.Fatal("older dashboard revision was accepted or retried immediately")
	}
}
