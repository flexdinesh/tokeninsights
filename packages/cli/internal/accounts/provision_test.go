package accounts

import (
	"context"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"path/filepath"
	"testing"
	"time"
)

type interruptedDatasets struct {
	*datastore.Store
	after bool
}

func (d interruptedDatasets) EnsureDataset(ctx context.Context, id string) error {
	if d.after {
		if err := d.Store.EnsureDataset(ctx, id); err != nil {
			return err
		}
	}
	return errors.New("simulated interruption")
}
func TestProvisioningResumesSameIdentityBeforeAndAfterDatasetCreation(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "after"}[after], func(t *testing.T) {
			s, store := testAccounts(t)
			repository := s.repository.(*SQLite)
			repository.datasets = interruptedDatasets{Store: store.data, after: after}
			pending, err := s.CreateUser(t.Context(), "Alice")
			if err == nil || pending.UserID == "" || pending.DatasetID == "" {
				t.Fatal("lost pending identity", pending, err)
			}
			if _, err := s.CreateToken(t.Context(), pending.UserID, []string{Read}, nil); err == nil {
				t.Fatal("pending user minted token")
			}
			repository.datasets = store.data
			if err := repository.Resume(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := repository.Resume(t.Context()); err != nil {
				t.Fatal("resume not idempotent", err)
			}
			token, err := s.CreateToken(t.Context(), pending.UserID, []string{Read}, nil)
			if err != nil {
				t.Fatal(err)
			}
			principal, err := s.AuthenticateBearer(t.Context(), token.Secret)
			if err != nil || principal.DatasetID != pending.DatasetID {
				t.Fatal("provision changed identity", principal, err)
			}
			if err := s.DisableUser(t.Context(), pending.UserID); err != nil {
				t.Fatal(err)
			}
			if err := repository.Resume(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AuthenticateBearer(t.Context(), token.Secret); !errors.Is(err, ErrUnauthenticated) {
				t.Fatal("resume reenabled user", err)
			}
		})
	}
}
func TestLegacyMigrationIsAtomicAndNeverResurrectsRevokedToken(t *testing.T) {
	data, err := datastore.OpenKind(t.Context(), filepath.Join(t.TempDir(), "data.duckdb"), "hosted")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = data.Close() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	secret, err := opaque()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.SQL().ExecContext(t.Context(), "INSERT INTO accounts.users VALUES('user','dataset','Alice',true,?)", now); err != nil {
		t.Fatal(err)
	}
	if _, err := data.SQL().ExecContext(t.Context(), "INSERT INTO accounts.tokens VALUES('token','user',?,'[\"read\"]',?,NULL,NULL)", digest(secret), now); err != nil {
		t.Fatal(err)
	}
	identity, err := data.DatabaseIdentity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "app.sqlite")
	app, err := appstore.Open(t.Context(), path, identity, "hosted")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = app.Close() }()
	repository := NewSQLite(app, data)
	if err := repository.ImportLegacy(t.Context(), data.SQL()); err == nil {
		t.Fatal("missing dataset imported")
	}
	var count int
	if err := app.SQL().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM users").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial migration", count, err)
	}
	if err := data.EnsureDataset(t.Context(), "dataset"); err != nil {
		t.Fatal(err)
	}
	if err := repository.ImportLegacy(t.Context(), data.SQL()); err != nil {
		t.Fatal(err)
	}
	service := New(repository)
	if _, err := service.AuthenticateBearer(t.Context(), secret); err != nil {
		t.Fatal("migrated token invalid", err)
	}
	if err := service.RevokeToken(t.Context(), "token"); err != nil {
		t.Fatal(err)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	app, err = appstore.Open(t.Context(), path, identity, "hosted")
	if err != nil {
		t.Fatal(err)
	}
	repository = NewSQLite(app, data)
	if err := repository.ImportLegacy(t.Context(), data.SQL()); err != nil {
		t.Fatal(err)
	}
	if _, err := New(repository).AuthenticateBearer(t.Context(), secret); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("stale legacy token resurrected", err)
	}
}
