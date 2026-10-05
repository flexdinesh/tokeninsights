package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func TestLocationPathRejectionIsAtomicAndDoesNotPersistPrivateLabels(t *testing.T) {
	for _, field := range []string{"directory", "repository"} {
		for _, name := range []string{"/synthetic/PRIVATE_LOCATION_MARKER/project", "C:/synthetic/PRIVATE_LOCATION_MARKER/project", `C:\synthetic\PRIVATE_LOCATION_MARKER\project`, `\\synthetic-server\PRIVATE_LOCATION_MARKER\project`, "relative/PRIVATE_LOCATION_MARKER/project", `relative\PRIVATE_LOCATION_MARKER\project`, "~/PRIVATE_LOCATION_MARKER/project"} {
			t.Run(field+" "+name, func(t *testing.T) {
				store, path := contractStore(t)
				server := httptest.NewServer(NewHandler(NewCore(store)))
				defer server.Close()
				first, second := contractFact("fixture-request-A"), contractFact("fixture-request-B")
				second.Location = &publication.Location{DirectoryKey: "fixture-directory-key", DirectoryName: "fixture-directory", RepositoryKey: "fixture-repository-key", RepositoryName: "fixture-repository"}
				publication.SetIDs(&second)
				var batch publication.Batch
				if err := json.Unmarshal(contractBatch(t, store, "stream", "private-location", first, second), &batch); err != nil {
					t.Fatal(err)
				}
				if field == "directory" {
					batch.Entries[1].Fact.Location.DirectoryName = name
				} else {
					batch.Entries[1].Fact.Location.RepositoryName = name
				}
				body, err := json.Marshal(batch)
				if err != nil {
					t.Fatal(err)
				}
				response := contractPost(t, server, body, http.StatusBadRequest)
				contractAssert(t, store, 0)
				metadata, err := store.Metadata(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if metadata.Revision != 0 || metadata.LastIngestionAtMs != 0 {
					t.Fatal("rejected batch advanced ingestion status", metadata)
				}
				var rejected publication.ErrorResponse
				if err := json.Unmarshal(response, &rejected); err != nil {
					t.Fatal(err)
				}
				if rejected.Code != "invalid_request" || rejected.Stage != "validation" {
					t.Fatalf("unexpected rejection stage: %+v", rejected)
				}
				if bytes.Contains(response, []byte("PRIVATE_LOCATION_MARKER")) {
					t.Fatal("rejected private label echoed")
				}
				for _, file := range []string{path, path + "-wal"} {
					data, err := os.ReadFile(file)
					if err != nil && !os.IsNotExist(err) {
						t.Fatal(err)
					}
					if bytes.Contains(data, []byte("PRIVATE_LOCATION_MARKER")) {
						t.Fatal("rejected private label persisted")
					}
				}
			})
		}
	}
}
