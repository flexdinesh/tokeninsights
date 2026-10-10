package sqliteaccounts

import (
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecontract"
)

func TestAccountStorageContract(t *testing.T) {
	storagecontract.RunAccounts(t, func(t *testing.T) storagecontract.Accounts {
		root := t.TempDir()
		dataPath, appPath := filepath.Join(root, "tokens.duckdb"), filepath.Join(root, "app.sqlite")
		var open func() storagecontract.Accounts
		open = func() storagecontract.Accounts {
			data, err := datastore.OpenKind(t.Context(), dataPath, datastore.KindHosted)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = data.Close() })
			id, err := data.DatabaseIdentity(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			app, err := appstore.OpenPaired(t.Context(), appPath, dataPath, id, datastore.KindHosted)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = app.Close() })
			r := NewSQLite(app, data)
			return storagecontract.Accounts{Repository: r, Datasets: data, Resume: r.Resume, Reopen: func() storagecontract.Accounts {
				if err := app.Close(); err != nil {
					t.Fatal(err)
				}
				if err := data.Close(); err != nil {
					t.Fatal(err)
				}
				return open()
			}}
		}
		return open()
	})
}
