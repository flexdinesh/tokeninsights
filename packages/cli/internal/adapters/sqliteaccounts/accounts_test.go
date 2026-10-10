package sqliteaccounts

import (
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
)

func testAccounts(t *testing.T) (*accounts.Service, *accountTestStore) {
	t.Helper()
	store, err := datastore.OpenKind(t.Context(), filepath.Join(t.TempDir(), "hosted.duckdb"), "hosted")
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
	user, err := s.CreateUser(t.Context(), "user")
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.CreateToken(t.Context(), user.UserID, []string{accounts.Read}, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := s.CreateSession(t.Context(), token.Secret)
	if err != nil {
		t.Fatal(err)
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

func TestSessionCookieAndRequestAuthentication(t *testing.T) {
	s, _ := testAccounts(t)
	u, err := s.CreateUser(t.Context(), "user")
	if err != nil {
		t.Fatal(err)
	}
	read, err := s.CreateToken(t.Context(), u.UserID, []string{accounts.Read}, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, expiry, err := s.CreateSession(t.Context(), read.Secret)
	if err != nil {
		t.Fatal(err)
	}
	cookie := accounts.SessionCookie(session, expiry)
	if !cookie.Secure || !cookie.HttpOnly || cookie.Domain != "" {
		t.Fatal("unsafe session cookie", cookie)
	}
	request := httptest.NewRequest("GET", "https://usage.example/api/v2/instance", nil)
	request.AddCookie(cookie)
	p, err := s.AuthenticateRequest(t.Context(), request)
	if err != nil || p.UserID != u.UserID {
		t.Fatal(p, err)
	}
	request.Header.Set("Authorization", "Bearer invalid")
	if _, err := s.AuthenticateRequest(t.Context(), request); !errors.Is(err, accounts.ErrUnauthenticated) {
		t.Fatal("invalid bearer fell back to session", err)
	}
}

func TestExpiredCredentialsFailClosed(t *testing.T) {
	s, store := testAccounts(t)
	u, err := s.CreateUser(t.Context(), "user")
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

func TestCleanupRemovesExpiredCredentialsWithoutRemovingDatasets(t *testing.T) {
	s, store := testAccounts(t)
	u, err := s.CreateUser(t.Context(), "user")
	if err != nil {
		t.Fatal(err)
	}
	active, err := s.CreateToken(t.Context(), u.UserID, []string{accounts.Read, accounts.Ingest}, nil)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := s.CreateToken(t.Context(), u.UserID, []string{accounts.Read}, nil)
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := s.CreateToken(t.Context(), u.UserID, []string{accounts.Read}, nil)
	if err != nil {
		t.Fatal(err)
	}
	activeSession, _, err := s.CreateSession(t.Context(), active.Secret)
	if err != nil {
		t.Fatal(err)
	}
	expiredTokenSession, _, err := s.CreateSession(t.Context(), expired.Secret)
	if err != nil {
		t.Fatal(err)
	}
	revokedSession, _, err := s.CreateSession(t.Context(), revoked.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(t.Context(), revoked.TokenID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQL().ExecContext(t.Context(), "UPDATE tokens SET expires_at=? WHERE token_id=?", time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), expired.TokenID); err != nil {
		t.Fatal(err)
	}
	if err := s.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateSession(t.Context(), activeSession); err != nil {
		t.Fatal("active session removed", err)
	}
	for _, secret := range []string{expiredTokenSession, revokedSession} {
		if _, err := s.AuthenticateSession(t.Context(), secret); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("invalidated session", err)
		}
		var count int
		if err := store.SQL().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sessions WHERE digest=?", accounts.CredentialDigest(secret)).Scan(&count); err != nil || count != 0 {
			t.Fatal("invalidated session retained", count, err)
		}
	}
	var count int
	if err := store.SQL().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM tokens WHERE token_id=?", expired.TokenID).Scan(&count); err != nil || count != 0 {
		t.Fatal("expired token retained", count, err)
	}
	if _, err := store.ForDataset(u.DatasetID).Metadata(t.Context()); err != nil {
		t.Fatal("cleanup removed dataset", err)
	}
}

type accountTestStore struct {
	*appstore.Store
	data       *datastore.Store
	repository *SQLite
}

func (s *accountTestStore) ForDataset(id string) *datastore.Store { return s.data.ForDataset(id) }
