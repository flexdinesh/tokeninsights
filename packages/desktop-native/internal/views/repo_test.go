package views

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// locationRow is a Repo row of total tokens at a location, split as
// usageRow splits it, with the dimensions it summarizes.
func locationRow(key, name, repository string, sessions, total int64, providers, harnesses, models string) analytics.Row {
	row := usageRow(key+"\x00", name, sessions, total)
	row.LocationKey, row.LocationName, row.RepositoryName = key, name, repository
	row.RepositoryKey = repository
	row.Provider, row.Harness, row.Model = providers, harnesses, models
	return row
}

// repositoryRows are the web mocks' Repo rows grouped by repository: a
// repository and the unknown location with the directories recorded
// for it.
func repositoryRows() []analytics.Row {
	unknown := locationRow("unknown", "unknown", "unknown", 1, 87_000, "openai", "opencode", "gpt-5-mini")
	unknown.DirectoryNames = []string{"~/workspace/client-a", "~/workspace/client-b", "~/workspace/scratch", "~/workspace/tools"}
	unknown.HasUnknownDirectory = true
	return []analytics.Row{
		locationRow("repo-1", "tokeninsights", "tokeninsights", 3, 560_000, "anthropic, openai", "codex, pi", "claude-sonnet-4-5, gpt-5"),
		unknown,
	}
}

// directoryRows are the Repo rows grouped by directory: a checkout of a
// repository, directories outside any, and usage without a directory.
func directoryRows() []analytics.Row {
	unknown := locationRow("unknown", "unknown", "unknown", 1, 16_000, "openai", "opencode", "gpt-5-mini")
	unknown.HasUnknownDirectory = true
	return []analytics.Row{
		locationRow("directory-1", "~/workspace/tokeninsights/main", "tokeninsights", 3, 560_000, "anthropic, openai", "codex, pi", "claude-sonnet-4-5, gpt-5"),
		locationRow("directory-2", "~/workspace/client-a", "unknown", 1, 41_000, "openai", "opencode", "gpt-5-mini"),
		locationRow("directory-4", "~/workspace/scratch", "unknown", 1, 30_000, "openai", "opencode", "gpt-5-mini"),
		unknown,
	}
}

// chooseGroup chooses group in the Group by select.
func chooseGroup(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	press(t, tt, ui.RolePopUpButton, groupByLocationLabel)
	press(t, tt, ui.RoleOption, label)
}

func TestGroupBySelectReadsTheGroupingChosen(t *testing.T) {
	f := loaded()
	tt := f.onTab(t, dashboardview.TabTokens, false)
	if _, ok := tt.AccessByRole(ui.RoleGroup, repoOptionsLabel); ok {
		t.Fatal("Repo view options off Repo")
	}
	if err := tt.Click("Repo"); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if _, ok := tt.AccessByRole(ui.RoleGroup, repoOptionsLabel); !ok {
		t.Fatal("no Repo view options")
	}
	if a, ok := tt.AccessByRole(ui.RolePopUpButton, groupByLocationLabel); !ok || a.Value != "Repository" {
		t.Fatalf("group by select %+v", a)
	}
	assertTexts(t, tt, groupByLabel, "Usage by repository", "Top 12 location rows by total tokens · % of filtered total",
		"Repo totals", "Location", "Providers", "Harnesses", "Models", "tokeninsights", "unknown", "560K", "86.6%", "13.4%")
	if got := f.lastRequest(); got.Tab != string(dashboardview.TabRepo) || got.LocationGroup != analytics.RepoGroupRepository {
		t.Fatalf("repo request %+v", got)
	}

	facets := len(f.backend.facetRequests)
	chooseGroup(t, tt, "Directory")
	f.sync(tt)
	if got := f.lastRequest(); got.LocationGroup != analytics.RepoGroupDirectory || got.Page != 1 {
		t.Fatalf("directory request %+v", got)
	}
	if got := f.backend.excludedRequests[len(f.backend.excludedRequests)-1]; got.LocationGroup != analytics.RepoGroupDirectory {
		t.Fatalf("excluded request %+v", got)
	}
	if len(f.backend.facetRequests) != facets {
		t.Fatal("the grouping read facets again")
	}
	if a, _ := tt.AccessByRole(ui.RolePopUpButton, groupByLocationLabel); a.Value != "Directory" {
		t.Fatalf("group by select %+v", a)
	}
	// A directory of a repository names it under its path; unknown usage
	// without a directory says so.
	assertTexts(t, tt, "Usage by directory", "~/workspace/tokeninsights/main", "tokeninsights", "~/workspace/client-a", noRecordedDirectory)
	if _, ok := tt.AccessByRole(ui.RoleDisclosure, showDirectoriesLabel); ok {
		t.Fatal("unknown usage without directories offers them")
	}
}

func TestLocationFiltersShowOnlyOnRepo(t *testing.T) {
	f := loaded()
	tt := f.onTab(t, dashboardview.TabTokens, false)
	for _, label := range []string{"Repository", "Directory"} {
		if _, ok := tt.AccessByRole(ui.RoleButton, label); ok {
			t.Fatalf("%s filter off Repo", label)
		}
	}
	tt = f.onTab(t, dashboardview.TabRepo, false)
	session, _ := tt.Find("Session:")
	previous := session
	for _, label := range []string{"Repository:", "Directory:"} {
		r, ok := tt.Find(label)
		if !ok || r.X <= previous.X && r.Y <= previous.Y {
			t.Fatalf("%s trigger at %v after %v", label, r, previous)
		}
		previous = r
	}

	openFilter(t, tt, "Directory", "Directory")
	assertTexts(t, tt, "~/workspace/tokeninsights/main", "~/workspace/tools", "unknown")
	press(t, tt, ui.RoleCheckBox, "~/workspace/client-a")
	f.sync(tt)
	if got := f.lastRequest(); !slices.Equal(got.DirectoryKeys, []string{"directory-2"}) || got.Tab != string(dashboardview.TabRepo) {
		t.Fatalf("directory request %+v", got)
	}
	if got := f.backend.facetRequests[len(f.backend.facetRequests)-1].Query; !slices.Equal(got.DirectoryKeys, []string{"directory-2"}) {
		t.Fatalf("facets request %+v", got)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(settle)
	if _, ok := tt.AccessByRole(ui.RoleButton, "Directory 1"); !ok {
		t.Fatalf("directory trigger in %q", tt.Texts())
	}

	// A repository the facets no longer offer is named unavailable.
	f.window.UpdateLocations(dashboardview.LocationsChange(dashboardview.LocationRepositories, []string{"repo-gone"}))
	f.sync(tt)
	assertTexts(t, tt, "Repository (unavailable)")
	press(t, tt, ui.RoleButton, "Remove repository Repository (unavailable)")
	f.sync(tt)
	if got := f.lastRequest(); len(got.RepositoryKeys) != 0 || !slices.Equal(got.DirectoryKeys, []string{"directory-2"}) {
		t.Fatalf("after removing the repository chip %+v", got)
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, "Remove directory ~/workspace/client-a"); !ok {
		t.Fatalf("no directory chip in %q", tt.Texts())
	}
	press(t, tt, ui.RoleButton, clearAllLabel)
	f.sync(tt)
	if got := f.lastRequest(); len(got.DirectoryKeys) != 0 {
		t.Fatalf("Clear All kept %+v", got)
	}
	if _, ok := tt.AccessByRole(ui.RoleGroup, activeFiltersLabel); ok {
		t.Fatal("active filters after Clear All")
	}

	// Leaving Repo drops its location filters.
	f.window.UpdateLocations(dashboardview.LocationsChange(dashboardview.LocationDirectories, []string{"directory-2"}))
	f.sync(tt)
	if err := tt.Click("Models"); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if got := f.lastRequest(); len(got.DirectoryKeys) != 0 || got.LocationGroup != "" {
		t.Fatalf("models request %+v", got)
	}
	if _, ok := tt.AccessByRole(ui.RoleGroup, activeFiltersLabel); ok || tt.HasText("~/workspace/client-a") {
		t.Fatal("location chips off Repo")
	}
	if query, _ := f.window.Query(); len(query.Directories) != 0 {
		t.Fatalf("query kept locations %+v", query.Directories)
	}
}

func TestUnknownLocationDisclosesItsDirectories(t *testing.T) {
	f := loaded()
	tt := f.onTab(t, dashboardview.TabRepo, false)
	a, ok := tt.AccessByRole(ui.RoleDisclosure, showDirectoriesLabel)
	if !ok || a.Expanded || tt.HasText("~/workspace/tools") || tt.HasText(someWithoutDirectory) {
		t.Fatalf("collapsed unknown location %+v, %q", a, tt.Texts())
	}
	collapsed, _ := tt.Find("openai")
	press(t, tt, ui.RoleDisclosure, showDirectoriesLabel)
	a, ok = tt.AccessByRole(ui.RoleDisclosure, hideDirectoriesLabel)
	if !ok || !a.Expanded {
		t.Fatalf("expanded unknown location %+v", a)
	}
	if _, ok := tt.AccessByRole(ui.RoleList, recordedDirectories); !ok {
		t.Fatal("no recorded directories list")
	}
	assertTexts(t, tt, "~/workspace/client-a", "~/workspace/client-b", "~/workspace/scratch", "~/workspace/tools", someWithoutDirectory)
	// The row grows to hold the directories, its other cells centered on it.
	if expanded, _ := tt.Find("opencode"); expanded.Y <= collapsed.Y {
		t.Fatalf("row cells at %v, collapsed %v", expanded, collapsed)
	}
	press(t, tt, ui.RoleDisclosure, hideDirectoriesLabel)
	if tt.HasText("~/workspace/tools") || tt.HasText(someWithoutDirectory) {
		t.Fatalf("collapsed again: %q", tt.Texts())
	}
}

// TestRepoImages renders the Repo tab grouped by repository and by
// directory, its unknown location expanded and a location filter's
// popover with location chips, in both appearances, and writes PNGs when
// TOKENINSIGHTS_NATIVE_GALLERY_DIR is set.
func TestRepoImages(t *testing.T) {
	dir := os.Getenv(galleryDirEnv)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, shot := range []struct {
		name    string
		prepare func(t *testing.T, f *fixture, tt *ui.Tester)
		texts   []string
	}{
		{"repo-repository", func(*testing.T, *fixture, *ui.Tester) {}, []string{"Usage by repository", showDirectoriesLabel}},
		{"repo-directory", func(t *testing.T, f *fixture, tt *ui.Tester) {
			chooseGroup(t, tt, "Directory")
			f.sync(tt)
		}, []string{"Usage by directory", noRecordedDirectory}},
		{"repo-unknown", func(t *testing.T, _ *fixture, tt *ui.Tester) {
			press(t, tt, ui.RoleDisclosure, showDirectoriesLabel)
		}, []string{hideDirectoriesLabel, someWithoutDirectory}},
		{"repo-location-filter", func(t *testing.T, f *fixture, tt *ui.Tester) {
			f.window.UpdateLocations(dashboardview.LocationsChange(dashboardview.LocationRepositories, []string{"repo-1"}))
			f.window.UpdateLocations(dashboardview.LocationsChange(dashboardview.LocationDirectories, []string{"directory-2"}))
			f.sync(tt)
			openFilter(t, tt, "Directory 1", "Directory")
		}, []string{"Remove repository tokeninsights", doneLabel, "~/workspace/tools"}},
	} {
		for _, dark := range []bool{false, true} {
			appearance := map[bool]string{false: "light", true: "dark"}[dark]
			t.Run(shot.name+"/"+appearance, func(t *testing.T) {
				f := loaded()
				tt := f.onTab(t, dashboardview.TabRepo, dark)
				shot.prepare(t, f, tt)
				tt.Advance(settle)
				for _, text := range shot.texts {
					if !tt.HasText(text) {
						if _, ok := tt.AccessByRole(ui.RoleButton, text); !ok {
							t.Errorf("missing %q in %q", text, tt.Texts())
						}
					}
				}
				if dir == "" {
					return
				}
				tt.SetScale(galleryScale)
				tt.Frame()
				writePNG(t, filepath.Join(dir, shot.name+"-"+appearance+".png"), tt)
			})
		}
	}
}
