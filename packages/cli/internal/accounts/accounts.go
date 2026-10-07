// Package accounts owns hosted principals and credentials. Account writes share
// the data engine's coordinator; it never opens a second writable database.
package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
)

const (
	Read                = "read"
	Ingest              = "ingest"
	SessionCookieName   = "tokeninsights_session"
	SessionLifetime     = 24 * time.Hour
	credentialBytes     = 32
	maxDisplayNameBytes = 200
)

var ErrUnauthenticated = errors.New("unauthenticated")
var ErrNotFound = errors.New("account_not_found")

type Principal struct {
	UserID      string   `json:"userId"`
	DatasetID   string   `json:"datasetId"`
	Permissions []string `json:"permissions"`
}

func (p Principal) HasPermission(permission string) bool {
	for _, scope := range p.Permissions {
		if scope == permission {
			return true
		}
	}
	return false
}

type User struct {
	UserID      string `json:"userId"`
	DatasetID   string `json:"datasetId"`
	DisplayName string `json:"displayName"`
	Enabled     bool   `json:"enabled"`
}

// Token is returned once at creation. Only its digest is persisted.
type Token struct {
	TokenID     string     `json:"tokenId"`
	Secret      string     `json:"token"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

type Service struct {
	store     *datastore.Store
	admission *admission
	login     *loginAdmission
}

func New(store *datastore.Store) *Service {
	return &Service{store: store, admission: newAdmission(), login: newLoginAdmission()}
}

func opaque() (string, error) {
	var data [credentialBytes]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data[:]), nil
}

func digest(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func encodePermissions(scopes []string) (string, error) {
	if len(scopes) == 0 || len(scopes) > 2 {
		return "", errors.New("invalid_permissions")
	}
	seen := make(map[string]bool)
	for _, scope := range scopes {
		if (scope != Read && scope != Ingest) || seen[scope] {
			return "", errors.New("invalid_permissions")
		}
		seen[scope] = true
	}
	encoded, err := json.Marshal(scopes)
	return string(encoded), err
}

func (s *Service) CreateUser(ctx context.Context, name string) (User, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxDisplayNameBytes {
		return User{}, errors.New("invalid_display_name")
	}
	id, err := opaque()
	if err != nil {
		return User{}, err
	}
	dataset, err := opaque()
	if err != nil {
		return User{}, err
	}
	user := User{UserID: id, DatasetID: dataset, DisplayName: name, Enabled: true}
	err = s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		if err := s.store.CreateDatasetInTx(ctx, tx, dataset); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO accounts.users VALUES(?,?,?,?,?)", id, dataset, name, true, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	return user, err
}

func (s *Service) DisableUser(ctx context.Context, userID string) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM accounts.users WHERE user_id=?)", userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, "UPDATE accounts.users SET enabled=false WHERE user_id=?", userID)
		return err
	})
}

func (s *Service) CreateToken(ctx context.Context, userID string, scopes []string, expires *time.Time) (Token, error) {
	encoded, err := encodePermissions(scopes)
	if err != nil {
		return Token{}, err
	}
	if expires != nil && !expires.After(time.Now()) {
		return Token{}, errors.New("invalid_expiry")
	}
	id, err := opaque()
	if err != nil {
		return Token{}, err
	}
	secret, err := opaque()
	if err != nil {
		return Token{}, err
	}
	var expiry interface{}
	if expires != nil {
		expiry = expires.UTC().Format(time.RFC3339Nano)
	}
	err = s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		var enabled bool
		if err := tx.QueryRowContext(ctx, "SELECT enabled FROM accounts.users WHERE user_id=?", userID).Scan(&enabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if !enabled {
			return errors.New("user_disabled")
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO accounts.tokens VALUES(?,?,?,?,?,?,NULL)", id, userID, digest(secret), encoded, time.Now().UTC().Format(time.RFC3339Nano), expiry)
		return err
	})
	if err != nil {
		return Token{}, err
	}
	return Token{TokenID: id, Secret: secret, Permissions: append([]string(nil), scopes...), ExpiresAt: expires}, nil
}

func (s *Service) RevokeToken(ctx context.Context, id string) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM accounts.tokens WHERE token_id=?)", id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, "UPDATE accounts.tokens SET revoked_at=COALESCE(revoked_at,?) WHERE token_id=?", time.Now().UTC().Format(time.RFC3339Nano), id)
		return err
	})
}

func validExpiry(expiry sql.NullString, now time.Time) bool {
	if !expiry.Valid {
		return true
	}
	parsed, err := time.Parse(time.RFC3339Nano, expiry.String)
	return err == nil && now.Before(parsed)
}

type rowReader interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func authenticateToken(ctx context.Context, reader rowReader, secret string) (Principal, string, error) {
	if len(secret) != base64.RawURLEncoding.EncodedLen(credentialBytes) {
		return Principal{}, "", ErrUnauthenticated
	}
	var p Principal
	var tokenID, encoded string
	var enabled bool
	var expiry, revoked sql.NullString
	err := reader.QueryRowContext(ctx, `SELECT u.user_id,u.dataset_id,u.enabled,t.token_id,t.permissions,t.expires_at,t.revoked_at
	FROM accounts.tokens t JOIN accounts.users u ON u.user_id=t.user_id WHERE t.digest=?`, digest(secret)).Scan(&p.UserID, &p.DatasetID, &enabled, &tokenID, &encoded, &expiry, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, "", ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, "", err
	}
	if !enabled || revoked.Valid || !validExpiry(expiry, time.Now()) {
		return Principal{}, "", ErrUnauthenticated
	}
	if err := json.Unmarshal([]byte(encoded), &p.Permissions); err != nil {
		return Principal{}, "", fmt.Errorf("invalid_account_permissions: %w", err)
	}
	if _, err := encodePermissions(p.Permissions); err != nil {
		return Principal{}, "", err
	}
	return p, tokenID, nil
}

func (s *Service) AuthenticateBearer(ctx context.Context, secret string) (Principal, error) {
	p, _, err := authenticateToken(ctx, s.store.SQL(), secret)
	return p, err
}

func (s *Service) AuthenticateSession(ctx context.Context, secret string) (Principal, error) {
	if len(secret) != base64.RawURLEncoding.EncodedLen(credentialBytes) {
		return Principal{}, ErrUnauthenticated
	}
	var p Principal
	var enabled bool
	var expiry, revoked, tokenExpiry, tokenRevoked sql.NullString
	err := s.store.SQL().QueryRowContext(ctx, `SELECT u.user_id,u.dataset_id,u.enabled,s.expires_at,s.revoked_at,t.expires_at,t.revoked_at
	FROM accounts.sessions s JOIN accounts.users u ON u.user_id=s.user_id
	JOIN accounts.tokens t ON t.token_id=s.source_token_id AND t.user_id=s.user_id WHERE s.digest=?`, digest(secret)).Scan(&p.UserID, &p.DatasetID, &enabled, &expiry, &revoked, &tokenExpiry, &tokenRevoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, err
	}
	if !enabled || revoked.Valid || tokenRevoked.Valid || !expiry.Valid || !validExpiry(expiry, time.Now()) || !validExpiry(tokenExpiry, time.Now()) {
		return Principal{}, ErrUnauthenticated
	}
	p.Permissions = []string{Read}
	return p, nil
}

func (s *Service) AuthenticateRequest(ctx context.Context, r *http.Request) (Principal, error) {
	if header := r.Header.Get("Authorization"); header != "" {
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return Principal{}, ErrUnauthenticated
		}
		return s.AuthenticateBearer(ctx, parts[1])
	}
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	return s.AuthenticateSession(ctx, cookie.Value)
}

func (s *Service) CreateSession(ctx context.Context, token string) (string, time.Time, error) {
	secret, err := opaque()
	if err != nil {
		return "", time.Time{}, err
	}
	id, err := opaque()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().UTC().Add(SessionLifetime)
	err = s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		p, tokenID, err := authenticateToken(ctx, tx, token)
		if err != nil {
			return err
		}
		if !p.HasPermission(Read) {
			return ErrUnauthenticated
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO accounts.sessions VALUES(?,?,?,?,?,?,NULL)", id, p.UserID, tokenID, digest(secret), time.Now().UTC().Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return secret, expires, nil
}

func (s *Service) Logout(ctx context.Context, secret string) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE accounts.sessions SET revoked_at=COALESCE(revoked_at,?) WHERE digest=?", time.Now().UTC().Format(time.RFC3339Nano), digest(secret))
		return err
	})
}

func SessionCookie(secret string, expires time.Time) *http.Cookie {
	return &http.Cookie{Name: SessionCookieName, Value: secret, Path: "/", Expires: expires, MaxAge: int(SessionLifetime / time.Second), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}

func ClearSessionCookie() *http.Cookie {
	c := SessionCookie("", time.Unix(1, 0))
	c.MaxAge = -1
	return c
}
