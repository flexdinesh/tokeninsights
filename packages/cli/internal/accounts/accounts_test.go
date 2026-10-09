package accounts

import (
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
)

func testAccounts(t *testing.T) (*Service, *accountTestStore) {
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
	return New(repository), &accountTestStore{Store: app, data: store}
}

func TestUsersCredentialsAndSessionRevocation(t *testing.T) {
	s, store := testAccounts(t)
	a, err := s.CreateUser(t.Context(), "Alice")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateUser(t.Context(), "Bob")
	if err != nil {
		t.Fatal(err)
	}
	if a.UserID == b.UserID || a.DatasetID == b.DatasetID {
		t.Fatal("users share identity")
	}
	for _, u := range []User{a, b} {
		m, err := store.ForDataset(u.DatasetID).Metadata(t.Context())
		if err != nil || m.DatasetID != u.DatasetID {
			t.Fatal("user dataset missing", m, err)
		}
	}
	aToken, err := s.CreateToken(t.Context(), a.UserID, []string{Read, Ingest}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bToken, err := s.CreateToken(t.Context(), b.UserID, []string{Read, Ingest}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		user  User
		token Token
	}{{a, aToken}, {b, bToken}} {
		p, err := s.AuthenticateBearer(t.Context(), item.token.Secret)
		if err != nil || p.DatasetID != item.user.DatasetID || !p.HasPermission(Ingest) {
			t.Fatal(p, err)
		}
		var persisted string
		if err := store.SQL().QueryRowContext(t.Context(), "SELECT digest FROM tokens WHERE token_id=?", item.token.TokenID).Scan(&persisted); err != nil {
			t.Fatal(err)
		}
		if persisted == item.token.Secret || strings.Contains(persisted, item.token.Secret) {
			t.Fatal("plaintext token persisted")
		}
	}
	session, expiry, err := s.CreateSession(t.Context(), aToken.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if expiry.Before(time.Now().Add(SessionLifetime - time.Minute)) {
		t.Fatal("short session")
	}
	p, err := s.AuthenticateSession(t.Context(), session)
	if err != nil || p.DatasetID != a.DatasetID || !p.HasPermission(Read) || p.HasPermission(Ingest) {
		t.Fatal("session scope", p, err)
	}
	if err := s.RevokeToken(t.Context(), aToken.TokenID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateBearer(t.Context(), aToken.Secret); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("revoked bearer", err)
	}
	if _, err := s.AuthenticateSession(t.Context(), session); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("source revoked session", err)
	}
	if _, err := s.AuthenticateBearer(t.Context(), bToken.Secret); err != nil {
		t.Fatal("other user affected", err)
	}
	rotated, err := s.CreateToken(t.Context(), a.UserID, []string{Read, Ingest}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.AuthenticateBearer(t.Context(), rotated.Secret)
	if err != nil || p.DatasetID != a.DatasetID {
		t.Fatal("rotation changed dataset", p, err)
	}
}

func TestReadOnlyLoginLogoutAndDisabledUser(t *testing.T) {
	s, _ := testAccounts(t)
	u, err := s.CreateUser(t.Context(), "user")
	if err != nil {
		t.Fatal(err)
	}
	ingest, err := s.CreateToken(t.Context(), u.UserID, []string{Ingest}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateSession(t.Context(), ingest.Secret); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("ingest-only browser login", err)
	}
	read, err := s.CreateToken(t.Context(), u.UserID, []string{Read}, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, expiry, err := s.CreateSession(t.Context(), read.Secret)
	if err != nil {
		t.Fatal(err)
	}
	cookie := SessionCookie(session, expiry)
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
	if _, err := s.AuthenticateRequest(t.Context(), request); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("invalid bearer fell back to session", err)
	}
	if err := s.Logout(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateSession(t.Context(), session); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("logged out session", err)
	}
	session, _, err = s.CreateSession(t.Context(), read.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DisableUser(t.Context(), u.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateBearer(t.Context(), read.Secret); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("disabled bearer", err)
	}
	if _, err := s.AuthenticateSession(t.Context(), session); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("disabled session", err)
	}
}

func TestExpiredCredentialsFailClosed(t *testing.T) {
	s, store := testAccounts(t)
	u, err := s.CreateUser(t.Context(), "user")
	if err != nil {
		t.Fatal(err)
	}
	for _, scopes := range [][]string{nil, {Read, Read}, {"admin"}} {
		if _, err := s.CreateToken(t.Context(), u.UserID, scopes, nil); err == nil {
			t.Fatal("invalid permissions", scopes)
		}
	}
	expires := time.Now().Add(-time.Second)
	if _, err := s.CreateToken(t.Context(), u.UserID, []string{Read}, &expires); err == nil {
		t.Fatal("expired token created")
	}
	token, err := s.CreateToken(t.Context(), u.UserID, []string{Read}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQL().ExecContext(t.Context(), "UPDATE tokens SET expires_at=? WHERE token_id=?", expires.UTC().Format(time.RFC3339Nano), token.TokenID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateBearer(t.Context(), token.Secret); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("expired bearer", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateBearer(t.Context(), token.Secret); err == nil {
		t.Fatal("closed database authenticated")
	}
}

func TestAdmissionDoesNotStarveAnotherUser(t *testing.T) {
	s := &Service{admission: newAdmission(), login: newLoginAdmission()}
	a := Principal{UserID: "a"}
	b := Principal{UserID: "b"}
	release, err := s.Admit(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Admit(a); err == nil {
		t.Fatal("parallel same-user admission")
	}
	other, err := s.Admit(b)
	if err != nil {
		t.Fatal("one user blocks another", err)
	}
	other()
	release()
	release()
	for range BatchBurst - 1 {
		release, err := s.Admit(a)
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if _, err := s.Admit(a); err == nil {
		t.Fatal("unbounded user burst")
	}
	for range LoginRequestsPerMinute {
		if err := s.AdmitLogin("127.0.0.1:1234"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AdmitLogin("127.0.0.1:5678"); err == nil {
		t.Fatal("login source port bypass")
	}
	if err := s.AdmitLogin("127.0.0.2:1234"); err != nil {
		t.Fatal("other login peer blocked", err)
	}
}

func TestCleanupRemovesExpiredCredentialsWithoutRemovingDatasets(t *testing.T) {
	s, store := testAccounts(t)
	u, err := s.CreateUser(t.Context(), "user")
	if err != nil {
		t.Fatal(err)
	}
	active, err := s.CreateToken(t.Context(), u.UserID, []string{Read, Ingest}, nil)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := s.CreateToken(t.Context(), u.UserID, []string{Read}, nil)
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := s.CreateToken(t.Context(), u.UserID, []string{Read}, nil)
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
		if _, err := s.AuthenticateSession(t.Context(), secret); !errors.Is(err, ErrUnauthenticated) {
			t.Fatal("invalidated session", err)
		}
		var count int
		if err := store.SQL().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sessions WHERE digest=?", digest(secret)).Scan(&count); err != nil || count != 0 {
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
	data *datastore.Store
}

func (s *accountTestStore) ForDataset(id string) *datastore.Store { return s.data.ForDataset(id) }
