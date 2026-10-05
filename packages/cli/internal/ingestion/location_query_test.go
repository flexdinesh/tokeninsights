package ingestion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func TestIngestedUnknownRepositoryRetainsDirectoryDisclosure(t *testing.T) {
	s, _ := contractStore(t)
	server := httptest.NewServer(NewHandler(NewCore(s)))
	defer server.Close()
	one, two, missing := contractFact("fixture-request-A"), contractFact("fixture-request-B"), contractFact("fixture-request-C")
	one.Location = &publication.Location{DirectoryKey: "fixture-directory-one", DirectoryName: "one"}
	two.Location = &publication.Location{DirectoryKey: "fixture-directory-two", DirectoryName: "two"}
	publication.SetIDs(&one)
	publication.SetIDs(&two)
	contractPost(t, server, contractBatch(t, s, "stream", "locations", one, two, missing), http.StatusOK)
	ctx := context.Background()
	rows, err := db.ViewerRepoGroups(ctx, s.SQL(), db.Filter{}, db.RepoGroupRepository)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Key != db.UnknownLocationKey || rows[0].RepositoryKey != db.UnknownLocationKey || rows[0].Name != "unknown" || rows[0].TotalTokens != 300 || !rows[0].HasUnknownDirectory || !reflect.DeepEqual(rows[0].DirectoryNames, []string{"one", "two"}) {
		t.Fatalf("unknown repository disclosure: %+v", rows)
	}
	filtered, err := db.ViewerRepoGroups(ctx, s.SQL(), db.Filter{RepositoryKeys: []string{db.UnknownLocationKey}, DirectoryKeys: []string{"fixture-directory-one"}}, db.RepoGroupRepository)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Key != db.UnknownLocationKey || filtered[0].TotalTokens != 100 || filtered[0].Name != "unknown · one" || filtered[0].HasUnknownDirectory || !reflect.DeepEqual(filtered[0].DirectoryNames, []string{"one"}) {
		t.Fatalf("filtered directory disclosure: %+v", filtered)
	}
	options, err := db.AvailableLocations(ctx, s.SQL(), db.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(options.Repositories, []db.LocationOption{{Key: db.UnknownLocationKey, Name: "unknown"}}) {
		t.Fatalf("repository facets: %+v", options.Repositories)
	}
	var nullRows int
	if err := s.SQL().QueryRow(`SELECT COUNT(*) FROM usage_locations WHERE repository_key IS NULL AND repository_name IS NULL AND repository_source IS NULL AND directory_key IS NOT NULL AND directory_name IS NOT NULL`).Scan(&nullRows); err != nil {
		t.Fatal(err)
	}
	if nullRows != 2 {
		t.Fatalf("missing repository metadata stored as values: nullrows=%d", nullRows)
	}
	contractPost(t, server, contractBatch(t, s, "recreated-stream", "repeat-locations", one, two, missing), http.StatusOK)
	metadata, err := s.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Revision != 1 {
		t.Fatalf("NULL reference replay changed revision: %d", metadata.Revision)
	}
}

func TestIngestedKnownRepositoryPreservesNameWithMissingDirectory(t *testing.T) {
	s, _ := contractStore(t)
	server := httptest.NewServer(NewHandler(NewCore(s)))
	defer server.Close()
	fact := contractFact("fixture-request-A")
	fact.Location = &publication.Location{RepositoryKey: "fixture-repository-key", RepositoryName: "fixture-repository", RepositorySource: "git"}
	publication.SetIDs(&fact)
	contractPost(t, server, contractBatch(t, s, "stream", "repository-only", fact), http.StatusOK)
	ctx := context.Background()
	rows, err := db.ViewerRepoGroups(ctx, s.SQL(), db.Filter{}, db.RepoGroupRepository)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Key != "fixture-repository-key" || rows[0].Name != "fixture-repository" || rows[0].TotalTokens != 100 || !rows[0].HasUnknownDirectory || len(rows[0].DirectoryNames) != 0 {
		t.Fatalf("known repository missing directory: %+v", rows)
	}
	directories, err := db.ViewerRepoGroups(ctx, s.SQL(), db.Filter{DirectoryKeys: []string{db.UnknownLocationKey}}, db.RepoGroupDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if len(directories) != 1 || directories[0].Key != db.UnknownLocationKey || directories[0].Name != "unknown" || directories[0].TotalTokens != 100 {
		t.Fatalf("missing directory grouping: %+v", directories)
	}
	var missingDirectory bool
	if err := s.SQL().QueryRow(`SELECT directory_key IS NULL AND directory_name IS NULL FROM usage_locations`).Scan(&missingDirectory); err != nil {
		t.Fatal(err)
	}
	if !missingDirectory {
		t.Fatal("missing directory metadata stored as values")
	}
}
