package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
)

func TestReloadCancelsObsoleteSameSelectionAndRejectsLateError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/instance" {
			_ = json.NewEncoder(w).Encode(api.InstanceResponse{ApiVersion: api.V1, InstanceId: "instance", DataEpoch: "epoch", DataReadiness: api.InstanceResponseDataReadinessReady, Hostname: "producer", Timezone: "UTC +00:00"})
			return
		}
		_ = json.NewEncoder(w).Encode(api.UsageResponse{InstanceId: "instance", DataEpoch: "epoch", Revision: 1, Page: 1, PageSize: 200, RowCount: 1, Rows: []api.UsageRow{{Key: "one", Name: "2026-01-01", Total: 42}}, Summary: api.UsageSummary{Total: 42, Sessions: 1, SyncedSessions: 1}})
	}))
	defer server.Close()
	m := newInteractiveModel(t.Context(), tableOptions{serverURL: server.URL, dbPath: "/unusable/local/path", period: periodAllTime, bucket: bucketDay}, time.Now(), "unknown")
	defer m.cancelSync()
	oldRequest := m.reloadCmd()
	newRequest := m.reloadCmd()
	oldMessage, ok := oldRequest().(reloadMsg)
	if !ok || !errors.Is(oldMessage.err, context.Canceled) {
		t.Fatalf("obsolete query error = %v", oldMessage.err)
	}
	updated, _ := m.Update(newRequest())
	m = updated.(interactiveModel)
	if m.err != nil || len(m.rows) != 1 || m.rows[0].totalValue != 42 {
		t.Fatalf("new snapshot rows=%v err=%v", m.rows, m.err)
	}
	updated, cmd := m.Update(oldMessage)
	m = updated.(interactiveModel)
	if cmd != nil || m.err != nil || len(m.rows) != 1 {
		t.Fatalf("late cancellation overwrote current snapshot rows=%v err=%v", m.rows, m.err)
	}
}

func TestServerIngestionTimeUsesServingOffset(t *testing.T) {
	instant := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	if got := formatServerIngestion(instant, "AEST +10:00"); got != "2026-01-01 10:00" {
		t.Fatalf("ingestion time=%q", got)
	}
	if got := formatServerIngestion(instant, "UTC+05:30"); got != "2026-01-01 05:30" {
		t.Fatalf("fixed serving timezone=%q", got)
	}
	if got := formatServerIngestion(0, "AEST +10:00"); got != "never" {
		t.Fatalf("empty ingestion=%q", got)
	}
	for _, test := range []struct {
		month time.Month
		want  string
	}{{time.January, "2025-12-31 19:00"}, {time.June, "2026-05-31 20:00"}} {
		stamp := time.Date(2026, test.month, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		if got := formatServerIngestion(stamp, "America/New_York"); got != test.want {
			t.Fatalf("historical ingestion=%q want%q", got, test.want)
		}
	}
}

func TestServerReplacementCancelsOldFacetsAndPreservesSelectedFilters(t *testing.T) {
	m := newInteractiveModel(t.Context(), tableOptions{serverURL: "http://127.0.0.1:8765", period: periodAllTime, bucket: bucketDay, filters: filters{providers: stringList{"selected"}}}, time.Now(), "unknown")
	defer m.cancelSync()
	m.instanceID, m.dataEpoch = "old", "old-epoch"
	m.popup, m.filterDimension = popupFilterValues, filterProvider
	m.filterValues = []string{"old-provider"}
	oldContext, oldSequence := m.queryContext(m.filterQueries)
	oldMessage := filterValuesMsg{requestID: oldSequence, selection: m.selectionKey(), dimension: filterProvider, values: []string{"old-provider"}}
	updated, _ := m.Update(sharedSyncMsg{instanceID: "new", dataEpoch: "new-epoch", readiness: "ready"})
	m = updated.(interactiveModel)
	if !errors.Is(oldContext.Err(), context.Canceled) || len(m.filterValues) != 0 || !m.filterLoading || !reflect.DeepEqual(m.options.filters.providers, stringList{"selected"}) {
		t.Fatalf("replacement retained stale facets: values=%v loading=%v selected=%v", m.filterValues, m.filterLoading, m.options.filters.providers)
	}
	updated, _ = m.Update(oldMessage)
	m = updated.(interactiveModel)
	if len(m.filterValues) != 0 || !m.filterLoading {
		t.Fatalf("old facets published after replacement: %v", m.filterValues)
	}
}

func TestLocationFacetsUseRepoAPIAndInactiveFiltersDoNotHideOtherTabs(t *testing.T) {
	database, path := newLoadRowsTestDB(t)
	defer func() { _ = database.Close() }()
	now := time.Now()
	insertLoadRowsCanonicalToken(t, database, now.UnixMilli(), "pi", "located", "provider-one", "model-one")
	if _, err := database.Exec(`INSERT INTO usage_locations (semantic_key,directory_key,directory_name,repository_key,repository_name) VALUES ('fixture-location','fixture-directory','fixture/dir','fixture-repository','fixture-repo')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE canonical_token_usage SET location_id=(SELECT id FROM usage_locations WHERE semantic_key='fixture-location')`); err != nil {
		t.Fatal(err)
	}
	insertLoadRowsCanonicalToken(t, database, now.Add(time.Second).UnixMilli(), "codex", "unlocated", "provider-two", "model-two")
	options := tableOptions{serverURL: queryServerURL(t, path), period: periodAllTime, bucket: bucketDay}
	values, keys, err := loadLocationFilterValues(t.Context(), options, now, filterRepository)
	if err != nil || !reflect.DeepEqual(values, []string{"fixture-repo", "unknown"}) || keys["fixture-repo"] != "fixture-repository" {
		t.Fatalf("repo facets=%v keys=%v error=%v", values, keys, err)
	}
	options.filters.repositories = stringList{"fixture-repository"}
	values, keys, err = loadLocationFilterValues(t.Context(), options, now, filterDirectory)
	if err != nil || !reflect.DeepEqual(values, []string{"fixture/dir"}) || keys["fixture/dir"] != "fixture-directory" {
		t.Fatalf("directory facets=%v keys=%v error=%v", values, keys, err)
	}
	options.filters.directories = stringList{"fixture-directory"}
	m := newInteractiveModel(t.Context(), options, now, "unknown")
	defer m.cancelSync()
	loaded, _ := m.Update(m.loadDashboard())
	m = loaded.(interactiveModel)
	m.activeTab = tabRepo
	repoMessage, ok := m.filterValuesCmd(filterProvider)().(filterValuesMsg)
	if !ok || repoMessage.err != nil || !reflect.DeepEqual(repoMessage.values, []string{"provider-one"}) {
		t.Fatalf("selected repo providers=%v error=%v", repoMessage.values, repoMessage.err)
	}
	m.activeTab = tabModels
	otherMessage, ok := m.filterValuesCmd(filterProvider)().(filterValuesMsg)
	if !ok || otherMessage.err != nil || !reflect.DeepEqual(otherMessage.values, []string{"provider-one", "provider-two"}) {
		t.Fatalf("other tab providers=%v error=%v", otherMessage.values, otherMessage.err)
	}
}
