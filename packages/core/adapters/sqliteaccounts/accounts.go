package sqliteaccounts

import (
	"github.com/flexdinesh/tokeninsights/packages/core/accounts"
	"github.com/flexdinesh/tokeninsights/packages/core/adapters/accountsql"
	"github.com/flexdinesh/tokeninsights/packages/core/appstore"
)

func NewSQLite(store *appstore.Store, datasets accounts.Datasets) *accountsql.Repository {
	return accountsql.New(store, datasets, func(column string) string { return "julianday(" + column + ")<=julianday(?)" })
}
