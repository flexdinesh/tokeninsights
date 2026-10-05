package serverstore

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestInitializationFailureDoesNotClaimServerPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sqlite")
	originalSchema := Schema
	Schema += "\nINVALID INITIALIZATION SQL;"
	_, err := CreateIfMissing(path)
	Schema = originalSchema
	if err == nil {
		t.Fatal("expected initialization SQL failure")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed initialization claimed final server path: %v", err)
	}
	store, err := CreateIfMissing(path)
	if err != nil {
		t.Fatalf("manual retry after failed initialization: %v", err)
	}
	defer func() { _ = store.Close() }()
	if _, err := store.Metadata(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyExistingFileRemainsUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sqlite")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateIfMissing(path); err == nil {
		t.Fatal("preexisting untagged file must not be claimed")
	}
	body, err := os.ReadFile(path)
	if err != nil || len(body) != 0 {
		t.Fatalf("preexisting file changed: bytes=%d err=%v", len(body), err)
	}
}

func TestConcurrentInitializationPublishesOneCompleteDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sqlite")
	const callers = 12
	start := make(chan struct{})
	failures := make(chan error, callers)
	identities := make(chan string, callers)
	var workers sync.WaitGroup
	for range callers {
		workers.Go(func() {
			<-start
			store, err := CreateIfMissing(path)
			if err != nil {
				failures <- err
				return
			}
			defer func() { _ = store.Close() }()
			metadata, err := store.Metadata(context.Background())
			if err != nil {
				failures <- err
				return
			}
			identities <- metadata.DatabaseID
		})
	}
	close(start)
	workers.Wait()
	close(failures)
	close(identities)
	for err := range failures {
		t.Errorf("initialization exposed incomplete database: %v", err)
	}
	first := ""
	for id := range identities {
		if first == "" {
			first = id
		}
		if id != first {
			t.Error("concurrent callers opened different database identities")
		}
	}
}
