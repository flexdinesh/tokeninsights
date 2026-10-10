package sqliteaccounts

import (
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/accountsql"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
)

func NewSQLite(store *appstore.Store, datasets accounts.Datasets) *accountsql.Repository {
	return accountsql.New(store, datasets, func(column string) string { return "julianday(" + column + ")<=julianday(?)" })
}
