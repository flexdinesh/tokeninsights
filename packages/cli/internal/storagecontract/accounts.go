package storagecontract

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
)

type Accounts struct {
	Repository   accounts.Repository
	Datasets     accounts.Datasets
	Resume       func(context.Context) error
	Reopen       func() Accounts
	WithDatasets func(accounts.Datasets) accounts.Repository
}

// RunAccounts checks the same account semantics for SQLite and PostgreSQL;
// account and token repositories remain separate even on a shared database.
func RunAccounts(t *testing.T, factory func(*testing.T) Accounts) {
	t.Helper()
	t.Run("bootstrap_restart_and_revocation", func(t *testing.T) {
		store := factory(t)
		const token = "synthetic-bootstrap-token"
		if err := store.Repository.BootstrapAdmin(t.Context(), "admin", token); err != nil {
			t.Fatal(err)
		}
		principal, err := store.Repository.AuthenticateBearer(t.Context(), token)
		if err != nil || !principal.HasPermission(accounts.Read) || !principal.HasPermission(accounts.Ingest) {
			t.Fatal(principal, err)
		}
		session, _, err := store.Repository.CreateSession(t.Context(), token)
		if err != nil {
			t.Fatal(err)
		}
		store = store.Reopen()
		if err := store.Repository.BootstrapAdmin(t.Context(), "admin", token); err != nil {
			t.Fatal(err)
		}
		again, err := store.Repository.AuthenticateBearer(t.Context(), token)
		if err != nil || again.UserID != principal.UserID || again.DatasetID != principal.DatasetID {
			t.Fatal("restart changed identity", again, err)
		}
		if _, err := store.Repository.AuthenticateSession(t.Context(), session); err != nil {
			t.Fatal("restart lost session", err)
		}
		if err := store.Repository.RevokeToken(t.Context(), "bootstrap-admin"); err != nil {
			t.Fatal(err)
		}
		if err := store.Repository.BootstrapAdmin(t.Context(), "admin", token); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Repository.AuthenticateBearer(t.Context(), token); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("bootstrap undid revocation", err)
		}
		if err := store.Repository.DisableUser(t.Context(), principal.UserID); err != nil {
			t.Fatal(err)
		}
		if err := store.Repository.BootstrapAdmin(t.Context(), "admin", token); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Repository.CreateToken(t.Context(), principal.UserID, []string{accounts.Read}, nil); err == nil {
			t.Fatal("bootstrap reenabled user")
		}
	})
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "bootstrap_interrupted_before_dataset", true: "bootstrap_interrupted_after_dataset"}[after], func(t *testing.T) {
			store := factory(t)
			interrupted := store.WithDatasets(interruptedDatasets{Datasets: store.Datasets, after: after})
			const token = "synthetic-bootstrap-token"
			if err := interrupted.BootstrapAdmin(t.Context(), "admin", token); err == nil {
				t.Fatal("bootstrap interruption not exposed")
			}
			store = store.Reopen()
			if err := store.Resume(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := store.Repository.BootstrapAdmin(t.Context(), "admin", token); err != nil {
				t.Fatal(err)
			}
			principal, err := store.Repository.AuthenticateBearer(t.Context(), token)
			if err != nil || principal.UserID != "bootstrap-admin" || principal.DatasetID != "bootstrap-admin" {
				t.Fatal("bootstrap recovery changed identity", principal, err)
			}
		})
		t.Run(map[bool]string{false: "provision_interrupted_before_dataset", true: "provision_interrupted_after_dataset"}[after], func(t *testing.T) {
			store := factory(t)
			r := store.WithDatasets(interruptedDatasets{Datasets: store.Datasets, after: after})
			pending, err := r.CreateUser(t.Context(), "Alice")
			if err == nil || pending.UserID == "" || pending.DatasetID == "" {
				t.Fatal("lost pending identity", pending, err)
			}
			if _, err := r.CreateToken(t.Context(), pending.UserID, []string{accounts.Read}, nil); err == nil {
				t.Fatal("pending account authorized")
			}
			store = store.Reopen()
			for range 2 {
				if err := store.Resume(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			token, err := store.Repository.CreateToken(t.Context(), pending.UserID, []string{accounts.Read}, nil)
			if err != nil {
				t.Fatal(err)
			}
			principal, err := store.Repository.AuthenticateBearer(t.Context(), token.Secret)
			if err != nil || principal.DatasetID != pending.DatasetID {
				t.Fatal("resume changed identity", principal, err)
			}
			if err := store.Repository.DisableUser(t.Context(), pending.UserID); err != nil {
				t.Fatal(err)
			}
			if err := store.Resume(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Repository.AuthenticateBearer(t.Context(), token.Secret); !errors.Is(err, accounts.ErrUnauthenticated) {
				t.Fatal("resume reenabled account", err)
			}
		})
	}
	t.Run("durable_identity_revocation_and_isolation", func(t *testing.T) {
		store := factory(t)
		r := store.Repository
		a, err := r.CreateUser(t.Context(), "Alice")
		if err != nil {
			t.Fatal(err)
		}
		b, err := r.CreateUser(t.Context(), "Bob")
		if err != nil {
			t.Fatal(err)
		}
		if !a.Enabled || !b.Enabled || a.UserID == b.UserID || a.DatasetID == b.DatasetID || a.DatasetID == "" {
			t.Fatal("provisioning identity contract", a, b)
		}
		for _, u := range []accounts.User{a, b} {
			if exists, err := store.Datasets.DatasetExists(t.Context(), u.DatasetID); err != nil || !exists {
				t.Fatal("active user has no dataset", u, err)
			}
		}
		at, err := r.CreateToken(t.Context(), a.UserID, []string{accounts.Read, accounts.Ingest}, nil)
		if err != nil {
			t.Fatal(err)
		}
		bt, err := r.CreateToken(t.Context(), b.UserID, []string{accounts.Read}, nil)
		if err != nil {
			t.Fatal(err)
		}
		session, expiry, err := r.CreateSession(t.Context(), at.Secret)
		if err != nil {
			t.Fatal(err)
		}
		if expiry.Before(time.Now().Add(accounts.SessionLifetime - time.Minute)) {
			t.Fatal("short session lifetime")
		}
		p, err := r.AuthenticateSession(t.Context(), session)
		if err != nil || p.DatasetID != a.DatasetID || !p.HasPermission(accounts.Read) || p.HasPermission(accounts.Ingest) {
			t.Fatal("session permission contract", p, err)
		}
		store = store.Reopen()
		r = store.Repository
		p, err = r.AuthenticateBearer(t.Context(), at.Secret)
		if err != nil || p.DatasetID != a.DatasetID || !p.HasPermission(accounts.Ingest) {
			t.Fatal("reopen changed identity/permissions", p, err)
		}
		if _, err := r.AuthenticateSession(t.Context(), session); err != nil {
			t.Fatal("reopen lost session", err)
		}
		if err := r.RevokeToken(t.Context(), at.TokenID); err != nil {
			t.Fatal(err)
		}
		store = store.Reopen()
		r = store.Repository
		if _, err := r.AuthenticateBearer(t.Context(), at.Secret); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("revocation lost after reopen", err)
		}
		if _, err := r.AuthenticateSession(t.Context(), session); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("source token revocation ignored", err)
		}
		if p, err := r.AuthenticateBearer(t.Context(), bt.Secret); err != nil || p.DatasetID != b.DatasetID {
			t.Fatal("revocation affected other user", p, err)
		}
		rotated, err := r.CreateToken(t.Context(), a.UserID, []string{accounts.Read}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if p, err := r.AuthenticateBearer(t.Context(), rotated.Secret); err != nil || p.DatasetID != a.DatasetID {
			t.Fatal("rotation changed dataset", p, err)
		}
		rotatedSession, _, err := r.CreateSession(t.Context(), rotated.Secret)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.DisableUser(t.Context(), a.UserID); err != nil {
			t.Fatal(err)
		}
		if err := store.Resume(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := r.AuthenticateBearer(t.Context(), rotated.Secret); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("resume reenabled disabled user", err)
		}
		if _, err := r.AuthenticateSession(t.Context(), rotatedSession); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("disabled user session remained active", err)
		}
		if exists, err := store.Datasets.DatasetExists(t.Context(), a.DatasetID); err != nil || !exists {
			t.Fatal("disable deleted evidence dataset", err)
		}
	})
	t.Run("permissions_and_logout", func(t *testing.T) {
		store := factory(t)
		r := store.Repository
		u, err := r.CreateUser(t.Context(), "user")
		if err != nil {
			t.Fatal(err)
		}
		for _, scopes := range [][]string{nil, {accounts.Read, accounts.Read}, {"admin"}} {
			if _, err := r.CreateToken(t.Context(), u.UserID, scopes, nil); err == nil {
				t.Fatal("invalid permissions accepted", scopes)
			}
		}
		token, err := r.CreateToken(t.Context(), u.UserID, []string{accounts.Ingest}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := r.CreateSession(t.Context(), token.Secret); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("ingest-only login accepted", err)
		}
		read, err := r.CreateToken(t.Context(), u.UserID, []string{accounts.Read}, nil)
		if err != nil {
			t.Fatal(err)
		}
		session, _, err := r.CreateSession(t.Context(), read.Secret)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Logout(t.Context(), session); err != nil {
			t.Fatal(err)
		}
		if _, err := r.AuthenticateSession(t.Context(), session); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("logout ignored", err)
		}
		if _, err := r.AuthenticateBearer(t.Context(), read.Secret); err != nil {
			t.Fatal("logout revoked bearer", err)
		}
	})
}

type interruptedDatasets struct {
	accounts.Datasets
	after bool
}

func (d interruptedDatasets) EnsureDataset(ctx context.Context, id string) error {
	if d.after {
		if err := d.Datasets.EnsureDataset(ctx, id); err != nil {
			return err
		}
	}
	return errors.New("simulated provisioning interruption")
}
