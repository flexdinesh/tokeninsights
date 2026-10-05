package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func locationReferenceFact(harness, source string) publication.Fact {
	f := contractFact("fixture-request-" + harness)
	f.Harness, f.Session.Harness = harness, harness
	f.Location = &publication.Location{DirectoryKey: "fixture-directory-key", DirectoryName: "fixture-directory",
		RepositoryKey: "fixture-repository-key", RepositoryName: "fixture-repository", RepositorySource: source}
	publication.SetIDs(&f)
	return f
}

func TestLocationReferencesMergeProvenanceAcrossHarnesses(t *testing.T) {
	for _, weaker := range []string{"", "opencode-project", "git-common-dir", "git-remote"} {
		for _, reverse := range []bool{false, true} {
			for _, sameBatch := range []bool{false, true} {
				name := weaker + "/git-first"
				if reverse {
					name = weaker + "/harness-first"
				}
				if sameBatch {
					name += "/same-batch"
				}
				t.Run(name, func(t *testing.T) {
					s, _ := contractStore(t)
					server := httptest.NewServer(NewHandler(NewCore(s)))
					defer server.Close()
					facts := []publication.Fact{locationReferenceFact("pi", weaker), locationReferenceFact("codex", "harness")}
					if reverse {
						facts[0], facts[1] = facts[1], facts[0]
					}
					var body, receipt []byte
					receipts := 1
					if sameBatch {
						body = contractBatch(t, s, "stream", "locations", facts...)
					} else {
						contractPost(t, server, contractBatch(t, s, "first-stream", "first", facts[0]), http.StatusOK)
						body = contractBatch(t, s, "second-stream", "second", facts[1])
						receipts++
					}
					receipt = contractPost(t, server, body, http.StatusOK)
					if replay := contractPost(t, server, body, http.StatusOK); !bytes.Equal(replay, receipt) {
						t.Fatal("saved batch replay changed its receipt")
					}
					metadata, err := s.Metadata(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					var replay publication.Receipt
					if err := json.Unmarshal(contractPost(t, server, contractBatch(t, s, "rebuilt-stream", "recollect", facts...), http.StatusOK), &replay); err != nil {
						t.Fatal(err)
					}
					if replay.Inserted != 0 || replay.Updated != 0 || replay.Noop != 2 || replay.Revision != metadata.Revision {
						t.Fatalf("recollection changed contributions or reference revision: %+v", replay)
					}
					contractAssert(t, s, receipts+1, facts...)
					var count int
					var source string
					if err := s.SQL().QueryRow(`SELECT COUNT(*),repository_source FROM usage_locations`).Scan(&count, &source); err != nil {
						t.Fatal(err)
					}
					if count != 1 || source != "harness" {
						t.Fatalf("shared location: count=%d source=%s", count, source)
					}
				})
			}
		}
	}
}

func TestLocationLabelConflictRollsBackProvenancePromotion(t *testing.T) {
	for _, field := range []string{"directory", "repository"} {
		t.Run(field, func(t *testing.T) {
			s, _ := contractStore(t)
			server := httptest.NewServer(NewHandler(NewCore(s)))
			defer server.Close()
			first := locationReferenceFact("pi", "git-remote")
			contractPost(t, server, contractBatch(t, s, "stream", "first", first), http.StatusOK)
			promotion := locationReferenceFact("codex", "harness")
			conflict := locationReferenceFact("opencode", "harness")
			if field == "directory" {
				conflict.Location.DirectoryName = "conflicting-directory"
			} else {
				conflict.Location.RepositoryName = "conflicting-repository"
			}
			var rejected publication.ErrorResponse
			if err := json.Unmarshal(contractPost(t, server, contractBatch(t, s, "stream", "conflict", promotion, conflict), http.StatusConflict), &rejected); err != nil {
				t.Fatal(err)
			}
			if rejected.Code != "reference_conflict" || rejected.Stage != "references" {
				t.Fatalf("unexpected rejection: %+v", rejected)
			}
			contractAssert(t, s, 1, first)
			var source string
			if err := s.SQL().QueryRow(`SELECT repository_source FROM usage_locations`).Scan(&source); err != nil {
				t.Fatal(err)
			}
			metadata, err := s.Metadata(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if source != "git-remote" || metadata.Revision != 1 {
				t.Fatalf("rejected batch changed reference/status: source=%s revision=%d", source, metadata.Revision)
			}
		})
	}
}
