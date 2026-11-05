package sqlanalytics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/datastore"
	"github.com/flexdinesh/tokeninsights/packages/core/storagecontract"
)

func TestTokenStorageContract(t *testing.T) {
	storagecontract.RunTokens(t, func(t *testing.T) storagecontract.Tokens { return sqliteFixture(t).Tokens })
}

func sqliteFixture(t testing.TB) storagecontract.BenchmarkFixture {
	root := t.TempDir()
	paths := make(map[string]string)
	var stores map[string]*datastore.Store
	var open func() storagecontract.Tokens
	open = func() storagecontract.Tokens {
		stores = make(map[string]*datastore.Store)
		ensure := func(ctx context.Context, id string) error {
			if stores[id] != nil {
				return nil
			}
			path := paths[id]
			if path == "" {
				path = filepath.Join(root, fmt.Sprintf("tokens-%d.sqlite", len(paths)))
				paths[id] = path
			}
			store, err := datastore.Open(ctx, path)
			if err != nil {
				return err
			}
			stores[id] = store
			t.Cleanup(func() { _ = store.Close() })
			return nil
		}
		for id := range paths {
			if err := ensure(t.Context(), id); err != nil {
				t.Fatal(err)
			}
		}
		return storagecontract.Tokens{
			EnsureDataset: ensure,
			Dataset: func(id string) storagecontract.Dataset {
				store := stores[id]
				if store == nil {
					// A missing identity must fail rather than select another store.
					for _, known := range stores {
						store = known.ForDataset("missing")
						break
					}
				}
				return storagecontract.Dataset{Receiver: store, Queries: Queries{Store: store}, Processing: store, Reprocess: store.Reprocess}
			},
			Reopen: func() storagecontract.Tokens {
				for _, store := range stores {
					if err := store.Close(); err != nil {
						t.Fatal(err)
					}
				}
				return open()
			},
		}
	}
	return storagecontract.BenchmarkFixture{Tokens: open(), SizeBytes: func() (int64, error) {
		var size int64
		for _, path := range paths {
			for _, suffix := range []string{"", "-wal", "-shm"} {
				info, err := os.Stat(path + suffix)
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err != nil {
					return 0, err
				}
				size += info.Size()
			}
		}
		return size, nil
	}}
}

func BenchmarkStorage(b *testing.B) {
	storagecontract.BenchmarkStorage(b, func(b *testing.B) storagecontract.BenchmarkFixture { return sqliteFixture(b) })
}
