package sqliteaccounts

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/accountsql"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
)

func testAccounts(t *testing.T) (*accounts.Service, *accountTestStore) {
	t.Helper()
	store, err := datastore.OpenKind(t.Context(), filepath.Join(t.TempDir(), "hosted.sqlite"), "hosted")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	identity, err := store.DatabaseIdentity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	app, err := appstore.Open(t.Context(), filepath.Join(t.TempDir(), "app.sqlite"), identity, "hosted")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	repository := NewSQLite(app, store)
	return accounts.New(repository), &accountTestStore{Store: app, data: store, repository: repository}
}

func TestCredentialsPersistOnlyDigests(t *testing.T) {
	s, store := testAccounts(t)
	user, err := s.Signup(t.Context(), "user", "synthetic-password-one")
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.CreateToken(t.Context(), user.UserID, []string{accounts.Read}, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := s.CreateSession(t.Context(), "user", "synthetic-password-one")
	if err != nil {
		t.Fatal(err)
	}
	var passwordHash string
	if err := store.SQL().QueryRowContext(t.Context(), "SELECT password_hash FROM users WHERE user_id=?", user.UserID).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if passwordHash == "synthetic-password-one" || !accounts.VerifyPassword("synthetic-password-one", passwordHash) {
		t.Fatal("password persisted without valid salted hash")
	}
	for table, secret := range map[string]string{"tokens": token.Secret, "sessions": session} {
		var persisted string
		if err := store.SQL().QueryRowContext(t.Context(), "SELECT digest FROM "+table).Scan(&persisted); err != nil {
			t.Fatal(err)
		}
		if persisted != accounts.CredentialDigest(secret) || strings.Contains(persisted, secret) {
			t.Fatal("credential digest contract", table)
		}
	}
}

func TestSessionAuthentication(t *testing.T) {
	s, _ := testAccounts(t)
	u, err := s.Signup(t.Context(), "user", "synthetic-password-one")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := s.CreateSession(t.Context(), "user", "synthetic-password-one")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.AuthenticateSession(t.Context(), session)
	if err != nil || p.UserID != u.UserID || p.CredentialKind != accounts.SessionCredential {
		t.Fatal(p, err)
	}
	if _, err := s.AuthenticateBearer(t.Context(), session); !errors.Is(err, accounts.ErrUnauthenticated) {
		t.Fatal("session secret authenticated as bearer", err)
	}
}

func TestExpiredCredentialsFailClosed(t *testing.T) {
	s, store := testAccounts(t)
	u, err := s.Signup(t.Context(), "user", "synthetic-password-one")
	if err != nil {
		t.Fatal(err)
	}
	for _, scopes := range [][]string{nil, {accounts.Read, accounts.Read}, {"admin"}} {
		if _, err := s.CreateToken(t.Context(), u.UserID, scopes, nil); err == nil {
			t.Fatal("invalid permissions", scopes)
		}
	}
	expires := time.Now().Add(-time.Second)
	if _, err := s.CreateToken(t.Context(), u.UserID, []string{accounts.Read}, &expires); err == nil {
		t.Fatal("expired token created")
	}
	token, err := s.CreateToken(t.Context(), u.UserID, []string{accounts.Read}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQL().ExecContext(t.Context(), "UPDATE tokens SET expires_at=? WHERE token_id=?", expires.UTC().Format(time.RFC3339Nano), token.TokenID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateBearer(t.Context(), token.Secret); !errors.Is(err, accounts.ErrUnauthenticated) {
		t.Fatal("expired bearer", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateBearer(t.Context(), token.Secret); err == nil {
		t.Fatal("closed database authenticated")
	}
}

func TestCleanupPreservesKeyMetadataAndIndependentSessions(t *testing.T) {
	s, store := testAccounts(t)
	user, err := s.Signup(t.Context(), "user", "synthetic-password-one")
	if err != nil {
		t.Fatal(err)
	}
	expired, err := s.CreateToken(t.Context(), user.UserID, []string{accounts.Read}, nil)
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := s.CreateToken(t.Context(), user.UserID, []string{accounts.Read}, nil)
	if err != nil {
		t.Fatal(err)
	}
	activeSession, _, err := s.CreateSession(t.Context(), "user", "synthetic-password-one")
	if err != nil {
		t.Fatal(err)
	}
	expiredSession, _, err := s.CreateSession(t.Context(), "user", "synthetic-password-one")
	if err != nil {
		t.Fatal(err)
	}
	revokedSession, _, err := s.CreateSession(t.Context(), "user", "synthetic-password-one")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(t.Context(), revoked.TokenID); err != nil {
		t.Fatal(err)
	}
	if err := s.Logout(t.Context(), revokedSession); err != nil {
		t.Fatal(err)
	}
	for _, update := range []struct{ query, id string }{
		{"UPDATE tokens SET expires_at=? WHERE token_id=?", expired.TokenID},
		{"UPDATE sessions SET expires_at=? WHERE digest=?", accounts.CredentialDigest(expiredSession)},
	} {
		if _, err := store.SQL().ExecContext(t.Context(), update.query, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), update.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateSession(t.Context(), activeSession); err != nil {
		t.Fatal("key expiry/revocation removed session", err)
	}
	for _, secret := range []string{expiredSession, revokedSession} {
		if _, err := s.AuthenticateSession(t.Context(), secret); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("invalid session accepted", err)
		}
		var count int
		if err := store.SQL().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sessions WHERE digest=?", accounts.CredentialDigest(secret)).Scan(&count); err != nil || count != 0 {
			t.Fatal("expired/revoked session retained", count, err)
		}
	}
	keys, err := store.repository.ListTokens(t.Context(), user.UserID)
	if err != nil || len(keys) != 2 {
		t.Fatal("key metadata lost", keys, err)
	}
	if _, err := s.AuthenticateBearer(t.Context(), expired.Secret); !errors.Is(err, accounts.ErrUnauthenticated) {
		t.Fatal("expired key authenticated", err)
	}
	if _, err := store.ForDataset(user.DatasetID).Metadata(t.Context()); err != nil {
		t.Fatal("cleanup removed dataset", err)
	}
}

type accountTestStore struct {
	*appstore.Store
	data       *datastore.Store
	repository *accountsql.Repository
}

func (s *accountTestStore) ForDataset(id string) *datastore.Store { return s.data.ForDataset(id) }
