package storagecontract

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
)

type Accounts struct {
	Repository accounts.Repository
	Datasets   accounts.Datasets
	Resume     func(context.Context) error
	Reopen     func() Accounts
}

// RunAccounts checks the same account semantics for SQLite and future adapters;
// account and token repositories remain separate even on a shared database.
func RunAccounts(t *testing.T, factory func(*testing.T) Accounts) {
	t.Helper()
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
