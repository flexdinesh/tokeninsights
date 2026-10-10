package sqlanalytics

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecontract"
)

func TestTokenStorageContract(t *testing.T) {
	storagecontract.RunTokens(t, func(t *testing.T) storagecontract.Tokens { return sqliteFixture(t).Tokens })
}

func sqliteFixture(t testing.TB) storagecontract.BenchmarkFixture {
	path := filepath.Join(t.TempDir(), "tokens.sqlite")
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
	return storagecontract.BenchmarkFixture{Tokens: open(), SizeBytes: func() (int64, error) {
		var size int64
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
		return size, nil
	}}
}

func BenchmarkStorage(b *testing.B) {
	storagecontract.BenchmarkStorage(b, func(b *testing.B) storagecontract.BenchmarkFixture { return sqliteFixture(b) })
}
