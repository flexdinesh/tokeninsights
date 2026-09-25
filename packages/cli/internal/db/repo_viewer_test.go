package db

import (
	"context"
	"database/sql"
	"slices"
	"testing"
)

func attachTestLocation(t *testing.T, database *sql.DB, semantic, directoryKey, repositoryKey string) {
	t.Helper()
	_, err := database.Exec(`INSERT INTO usage_locations (
		semantic_key, directory_key, directory_name, repository_key, repository_name
	) VALUES (?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))`,
		semantic, directoryKey, directoryKey, repositoryKey, repositoryKey)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`UPDATE canonical_token_usage SET location_id =
		(SELECT id FROM usage_locations WHERE semantic_key = ?)
		WHERE id = (SELECT MAX(id) FROM canonical_token_usage)`, semantic)
	if err != nil {
		t.Fatal(err)
	}
}

func TestViewerRepoGroupsCombineClonesAndKeepUnknown(t *testing.T) {
	database, _ := newTestDB(t)
	defer func() { _ = database.Close() }()
	insertCanonicalToken(t, database, 1000, "codex", "one", "openai", "gpt", 10, 0, 0, 0, 0, 10)
	attachTestLocation(t, database, "loc1", "dir1", "remote1")
	insertCanonicalToken(t, database, 2000, "pi", "two", "anthropic", "claude", 20, 0, 0, 0, 0, 20)
	attachTestLocation(t, database, "loc2", "dir2", "remote1")
	insertCanonicalToken(t, database, 3000, "opencode", "three", "openai", "gpt", 30, 0, 0, 0, 0, 30)
	rows, err := ViewerRepoGroups(context.Background(), database, Filter{}, RepoGroupRepository)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Key != "remote1" || rows[0].TotalTokens != 30 || rows[0].SessionCount != 2 || rows[0].Providers != "anthropic, openai" || rows[0].Harnesses != "codex, pi" || rows[0].Models != "claude, gpt" || !slices.Equal(rows[0].DirectoryNames, []string{"dir1", "dir2"}) || rows[1].Key != UnknownLocationKey || rows[1].TotalTokens != 30 {
		t.Fatalf("repository rows = %+v", rows)
	}
	directories, err := ViewerRepoGroups(context.Background(), database, Filter{RepositoryKeys: []string{"remote1"}}, RepoGroupDirectory)
	if err != nil || len(directories) != 2 || directories[0].Key != "dir2" || directories[1].Key != "dir1" {
		t.Fatalf("directory groups = %+v, %v", directories, err)
	}
	unknown, err := ViewerRepoGroups(context.Background(), database, Filter{RepositoryKeys: []string{UnknownLocationKey}}, RepoGroupRepository)
	if err != nil || len(unknown) != 1 || unknown[0].TotalTokens != 30 || !unknown[0].HasUnknownDirectory {
		t.Fatalf("unknown filter rows=%+v err=%v", unknown, err)
	}
	counts, err := ViewerSessionCounts(context.Background(), database, Filter{RepositoryKeys: []string{"remote1"}})
	if err != nil || counts.Shown != 2 || counts.Synced != 3 {
		t.Fatalf("counts=%+v err=%v", counts, err)
	}
}

func TestUnknownRepositoryShowsContributingDirectories(t *testing.T) {
	database, _ := newTestDB(t)
	defer func() { _ = database.Close() }()
	insertCanonicalToken(t, database, 1000, "pi", "one", "openai", "gpt", 10, 0, 0, 0, 0, 10)
	attachTestLocation(t, database, "loc1", "~/work/one", "")
	rows, err := ViewerRepoGroups(context.Background(), database, Filter{}, RepoGroupRepository)
	if err != nil || len(rows) != 1 || rows[0].Name != "unknown · ~/work/one" || !slices.Equal(rows[0].DirectoryNames, []string{"~/work/one"}) || rows[0].HasUnknownDirectory {
		t.Fatalf("single-directory fallback = %+v, %v", rows, err)
	}
	facets, err := AvailableLocations(context.Background(), database, Filter{})
	if err != nil || len(facets.Repositories) != 1 || facets.Repositories[0].Name != rows[0].Name {
		t.Fatalf("repository facet = %+v, %v", facets.Repositories, err)
	}
	insertCanonicalToken(t, database, 2000, "pi", "two", "openai", "gpt", 10, 0, 0, 0, 0, 10)
	attachTestLocation(t, database, "loc2", "~/work/two", "")
	insertCanonicalToken(t, database, 3000, "pi", "three", "openai", "other", 10, 0, 0, 0, 0, 10)
	attachTestLocation(t, database, "loc3", "~/work/a,b", "")
	insertCanonicalToken(t, database, 4000, "pi", "four", "openai", "gpt", 10, 0, 0, 0, 0, 10)
	rows, err = ViewerRepoGroups(context.Background(), database, Filter{}, RepoGroupRepository)
	if err != nil || len(rows) != 1 || rows[0].Name != "unknown" || !slices.Equal(rows[0].DirectoryNames, []string{"~/work/a,b", "~/work/one", "~/work/two"}) || !rows[0].HasUnknownDirectory {
		t.Fatalf("mixed-directory attribution = %+v, %v", rows, err)
	}
	filtered, err := ViewerRepoGroups(context.Background(), database, Filter{Models: []string{"other"}}, RepoGroupRepository)
	if err != nil || len(filtered) != 1 || !slices.Equal(filtered[0].DirectoryNames, []string{"~/work/a,b"}) || filtered[0].HasUnknownDirectory {
		t.Fatalf("filtered directories = %+v, %v", filtered, err)
	}
}

func TestLocationDisplayNameOmitsKey(t *testing.T) {
	if got := LocationDisplayName(LocationOption{Key: "abcdef012345", Name: "repo"}); got != "repo" {
		t.Fatalf("display name = %q", got)
	}
}
