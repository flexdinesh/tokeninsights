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
	storagecontract.RunTokens(t, func(t *testing.T) storagecontract.Tokens { return postgresFixture(t).Tokens })
}

func postgresFixture(t testing.TB) storagecontract.BenchmarkFixture {
	dsn := testdb.New(t)
	var active *Storage
	var reopen func() storagecontract.Tokens
	reopen = func() storagecontract.Tokens {
		s := openTest(t, dsn)
		active = s
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
	return storagecontract.BenchmarkFixture{Tokens: reopen(), SizeBytes: func() (int64, error) {
		var size int64
		err := active.Owner.Reader.QueryRowContext(t.Context(), `SELECT COALESCE(SUM(pg_total_relation_size(c.oid)),0)::bigint
   FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname='tokeninsights_data' AND c.relkind='r'`).Scan(&size)
		return size, err
	}}
}

func BenchmarkStorage(b *testing.B) {
	storagecontract.BenchmarkStorage(b, func(b *testing.B) storagecontract.BenchmarkFixture { return postgresFixture(b) })
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
