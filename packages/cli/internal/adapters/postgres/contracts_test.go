package postgres

import (
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/accountsql"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/postgres/testdb"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecontract"
)

func openTest(t testing.TB, dsn string) *Storage {
	t.Helper()
	s, err := Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Owner.Close() })
	return s
}
func TestTokenContract(t *testing.T) {
	storagecontract.RunTokens(t, func(t *testing.T) storagecontract.Tokens {
		dsn := testdb.New(t)
		var reopen func() storagecontract.Tokens
		reopen = func() storagecontract.Tokens {
			s := openTest(t, dsn)
			return storagecontract.Tokens{EnsureDataset: s.Tokens.EnsureDataset, Dataset: func(id string) storagecontract.Dataset {
				d := s.Tokens.ForDataset(id)
				return storagecontract.Dataset{Receiver: d, Queries: sqlanalytics.Queries{Store: d}, Processing: d, Reprocess: d.Reprocess}
			}, Reopen: func() storagecontract.Tokens {
				if err := s.Owner.Close(); err != nil {
					t.Fatal(err)
				}
				return reopen()
			}}
		}
		return reopen()
	})
}
func TestAccountContract(t *testing.T) {
	storagecontract.RunAccounts(t, func(t *testing.T) storagecontract.Accounts {
		dsn := testdb.New(t)
		var reopen func() storagecontract.Accounts
		reopen = func() storagecontract.Accounts {
			s := openTest(t, dsn)
			return storagecontract.Accounts{WithDatasets: func(d accounts.Datasets) accounts.Repository {
				return accountsql.New(s.Owner, d, func(column string) string { return column + "<=CAST(? AS TIMESTAMPTZ)" })
			}, Repository: s.Accounts, Datasets: s.Tokens, Resume: s.Accounts.Resume, Reopen: func() storagecontract.Accounts {
				if err := s.Owner.Close(); err != nil {
					t.Fatal(err)
				}
				return reopen()
			}}
		}
		return reopen()
	})
}

func BenchmarkQueries(b *testing.B) {
	s := openTest(b, testdb.New(b))
	if err := s.Tokens.EnsureDataset(b.Context(), "benchmark"); err != nil {
		b.Fatal(err)
	}
	d := s.Tokens.ForDataset("benchmark")
	storagecontract.BenchmarkQueries(b, storagecontract.Dataset{Receiver: d, Processing: d, Queries: sqlanalytics.Queries{Store: d}})
}
