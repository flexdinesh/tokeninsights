package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
)

func TestRepoTabGroupsAndFiltersWithoutChangingOtherTabs(t *testing.T) {
	database, path := newLoadRowsTestDB(t)
	defer func() { _ = database.Close() }()
	now := time.Date(2026, 4, 20, 9, 0, 0, 0, time.Local)
	insertLoadRowsCanonicalToken(t, database, now.UnixMilli(), "codex", "one", "openai", "gpt")
	_, err := database.Exec("UPDATE analytics.facts SET directory_key='dir-one',directory_name='~/work/repo',repository_key='repo-one',repository_name='repo'")
	if err != nil {
		t.Fatal(err)
	}
	insertLoadRowsCanonicalToken(t, database, now.Add(time.Second).UnixMilli(), "pi", "two", "anthropic", "claude")
	options := tableOptions{serverURL: queryServerURL(t, path), period: periodAllTime, bucket: bucketDay, repoGroup: querymodel.RepoGroupRepository}
	rows, err := loadRows(context.Background(), options, now, groupByNone, tabRepo)
	if err != nil || len(rows) != 2 || (rows[0].location != "unknown" && rows[1].location != "unknown") {
		t.Fatalf("repo rows = %+v, err = %v", rows, err)
	}
	options.filters.repositories = stringList{"repo-one"}
	rows, err = loadRows(context.Background(), options, now, groupByNone, tabRepo)
	if err != nil || len(rows) != 1 || rows[0].location != "repo" || rows[0].providers != "openai" || rows[0].harnesses != "codex" || rows[0].models != "gpt" {
		t.Fatalf("filtered repo row = %+v, err = %v", rows, err)
	}
	table := ansi.Strip(renderTableWithReferenceRowsAndSortWidth(rows, rows, groupByNone, tabRepo, sortTokens, 0, 0, "", -1))
	for _, want := range []string{"providers", "harnesses", "models", "openai", "codex", "gpt"} {
		if !strings.Contains(table, want) {
			t.Fatalf("repo table missing %q: %s", want, table)
		}
	}
	options.repoGroup = querymodel.RepoGroupDirectory
	rows, err = loadRows(context.Background(), options, now, groupByNone, tabRepo)
	if err != nil || len(rows) != 1 || rows[0].location != "~/work/repo" {
		t.Fatalf("directory row = %+v, err = %v", rows, err)
	}
	buckets, err := loadRows(context.Background(), options, now, groupByNone, tabTokens)
	if err != nil || len(buckets) != 1 || buckets[0].totalValue != 272 {
		t.Fatalf("token tab changed by repo filter: rows = %+v, err = %v", buckets, err)
	}
}

func TestLocationFilterLabelsKeepDuplicateNamesSelectable(t *testing.T) {
	labels, keys := locationFilterLabels([]querymodel.LocationOption{{Key: "first", Name: "repo"}, {Key: "second", Name: "repo"}})
	if len(labels) != 2 || labels[0] != "repo" || labels[1] != "repo (2)" || keys[labels[0]] != "first" || keys[labels[1]] != "second" {
		t.Fatalf("location labels = %v, keys = %v", labels, keys)
	}
}

func TestRepoTabControlsOnlyRepositoryAndDirectory(t *testing.T) {
	if len(repoGroupOptions) != 2 || repoGroupOptions[0] != querymodel.RepoGroupRepository || repoGroupOptions[1] != querymodel.RepoGroupDirectory {
		t.Fatalf("repo group options = %v", repoGroupOptions)
	}
	m := newInteractiveModel(context.Background(), tableOptions{}, time.Now(), "host")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	m = updated.(interactiveModel)
	if m.activeTab != tabRepo {
		t.Fatalf("active tab = %v", m.activeTab)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = updated.(interactiveModel)
	if m.popup != popupNone {
		t.Fatalf("breakdown shortcut opened popup = %v", m.popup)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(interactiveModel)
	if m.popup != popupRepoGroup {
		t.Fatalf("group popup = %v", m.popup)
	}
	m.popupCursor = 1
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(interactiveModel)
	if m.options.repoGroup != querymodel.RepoGroupDirectory {
		t.Fatalf("repo group = %q", m.options.repoGroup)
	}
}
