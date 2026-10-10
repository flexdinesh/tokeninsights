// Package postgres composes token and account adapters for a PostgreSQL owner.
package postgres

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/accountsql"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	physical "github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/postgres"
)

type Storage struct {
	Owner    *physical.Store
	Tokens   *datastore.Store
	Accounts *accountsql.Repository
}

func Open(ctx context.Context, dsn string) (*Storage, error) {
	owner, err := physical.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	tokens, err := datastore.AttachPostgres(owner.Context(), owner.Reader, &owner.Writer, owner.BeginWrite, owner.Check)
	if err != nil {
		_ = owner.Close()
		return nil, err
	}
	accounts := accountsql.New(owner, tokens, func(column string) string { return column + "<=CAST(? AS TIMESTAMPTZ)" })
	return &Storage{Owner: owner, Tokens: tokens, Accounts: accounts}, nil
}
