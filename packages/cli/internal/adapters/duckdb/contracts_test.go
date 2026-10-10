package duckdb

import (
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecontract"
)

func TestTokenStorageContract(t *testing.T) {
	storagecontract.RunTokens(t, func(t *testing.T) storagecontract.Tokens {
		path := filepath.Join(t.TempDir(), "tokens.duckdb")
		var open func() storagecontract.Tokens
		open = func() storagecontract.Tokens {
			store, err := datastore.OpenKind(t.Context(), path, datastore.KindHosted)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			return storagecontract.Tokens{
				EnsureDataset: store.EnsureDataset,
				Dataset: func(id string) storagecontract.Dataset {
					scoped := store.ForDataset(id)
					return storagecontract.Dataset{Receiver: scoped, Queries: Queries{Store: scoped}, Processing: scoped, Reprocess: scoped.Reprocess}
				},
				Reopen: func() storagecontract.Tokens {
					if err := store.Close(); err != nil {
						t.Fatal(err)
					}
					return open()
				},
			}
		}
		return open()
	})
}
