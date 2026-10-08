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
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"strings"
	"time"
)

// Datasets is the account lifecycle's token-store contract.
type Datasets interface {
	EnsureDataset(context.Context, string) error
	DatasetExists(context.Context, string) (bool, error)
	ReprocessDataset(context.Context, string) (int64, error)
}
type SQLite struct {
	store    *appstore.Store
	datasets Datasets
}

func NewSQLite(store *appstore.Store, datasets Datasets) *SQLite {
	return &SQLite{store: store, datasets: datasets}
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

func (s *SQLite) CreateUser(ctx context.Context, name string) (User, error) {
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
	user := User{UserID: id, DatasetID: dataset, DisplayName: name}
	err = s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO users VALUES(?,?,?,?,?,'pending')", id, dataset, name, false, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return User{}, err
	}
	if err := s.activate(ctx, user); err != nil {
		return user, err
	}
	user.Enabled = true
	return user, nil
}

func (s *SQLite) activate(ctx context.Context, user User) error {
	if err := s.datasets.EnsureDataset(ctx, user.DatasetID); err != nil {
		return err
	}
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE users SET provisioning='ready',enabled=true WHERE user_id=? AND provisioning='pending'", user.UserID)
		return err
	})
}

// Resume completes only pending provisioning; disabled ready users stay disabled.
func (s *SQLite) Resume(ctx context.Context) error {
	rows, err := s.store.SQL().QueryContext(ctx, "SELECT user_id,dataset_id,display_name FROM users WHERE provisioning='pending'")
	if err != nil {
		return err
	}
	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.UserID, &u.DatasetID, &u.DisplayName); err != nil {
			_ = rows.Close()
			return err
		}
		users = append(users, u)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, u := range users {
		if err := s.activate(ctx, u); err != nil {
			return err
		}
	}
	return nil
}
func (s *SQLite) EnsureDefault(ctx context.Context) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO users VALUES('default','default','Local user',true,?,'ready') ON CONFLICT(user_id) DO NOTHING", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

func (s *SQLite) DisableUser(ctx context.Context, userID string) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE user_id=?)", userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, "UPDATE users SET enabled=false,provisioning='ready' WHERE user_id=?", userID)
		return err
	})
}

func (s *SQLite) CreateToken(ctx context.Context, userID string, scopes []string, expires *time.Time) (Token, error) {
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
		if err := tx.QueryRowContext(ctx, "SELECT enabled FROM users WHERE user_id=?", userID).Scan(&enabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if !enabled {
			return errors.New("user_disabled")
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO tokens VALUES(?,?,?,?,?,?,NULL)", id, userID, digest(secret), encoded, time.Now().UTC().Format(time.RFC3339Nano), expiry)
		return err
	})
	if err != nil {
		return Token{}, err
	}
	return Token{TokenID: id, Secret: secret, Permissions: append([]string(nil), scopes...), ExpiresAt: expires}, nil
}

func (s *SQLite) RevokeToken(ctx context.Context, id string) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM tokens WHERE token_id=?)", id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, "UPDATE tokens SET revoked_at=COALESCE(revoked_at,?) WHERE token_id=?", time.Now().UTC().Format(time.RFC3339Nano), id)
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
	FROM tokens t JOIN users u ON u.user_id=t.user_id WHERE t.digest=?`, digest(secret)).Scan(&p.UserID, &p.DatasetID, &enabled, &tokenID, &encoded, &expiry, &revoked)
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

func (s *SQLite) AuthenticateBearer(ctx context.Context, secret string) (Principal, error) {
	p, _, err := authenticateToken(ctx, s.store.SQL(), secret)
	return p, err
}

func (s *SQLite) AuthenticateSession(ctx context.Context, secret string) (Principal, error) {
	if len(secret) != base64.RawURLEncoding.EncodedLen(credentialBytes) {
		return Principal{}, ErrUnauthenticated
	}
	var p Principal
	var enabled bool
	var expiry, revoked, tokenExpiry, tokenRevoked sql.NullString
	err := s.store.SQL().QueryRowContext(ctx, `SELECT u.user_id,u.dataset_id,u.enabled,s.expires_at,s.revoked_at,t.expires_at,t.revoked_at
	FROM sessions s JOIN users u ON u.user_id=s.user_id
	JOIN tokens t ON t.token_id=s.source_token_id AND t.user_id=s.user_id WHERE s.digest=?`, digest(secret)).Scan(&p.UserID, &p.DatasetID, &enabled, &expiry, &revoked, &tokenExpiry, &tokenRevoked)
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

func (s *SQLite) CreateSession(ctx context.Context, token string) (string, time.Time, error) {
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
		_, err = tx.ExecContext(ctx, "INSERT INTO sessions VALUES(?,?,?,?,?,?,NULL)", id, p.UserID, tokenID, digest(secret), time.Now().UTC().Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return secret, expires, nil
}

func (s *SQLite) Logout(ctx context.Context, secret string) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE sessions SET revoked_at=COALESCE(revoked_at,?) WHERE digest=?", time.Now().UTC().Format(time.RFC3339Nano), digest(secret))
		return err
	})
}

// ReprocessUser resolves the operator's user ID to its server-owned dataset.
func (s *SQLite) ReprocessUser(ctx context.Context, userID string) (int64, error) {
	var dataset string
	err := s.store.SQL().QueryRowContext(ctx, "SELECT dataset_id FROM users WHERE user_id=?", userID).Scan(&dataset)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return s.datasets.ReprocessDataset(ctx, dataset)
}

// Cleanup removes expired credentials in bounded transactions. Revoked token
// IDs remain available to operators; evidence and datasets are never touched.
func (s *SQLite) Cleanup(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE session_id IN (
		 SELECT s.session_id FROM sessions s
		 LEFT JOIN tokens t ON t.token_id=s.source_token_id AND t.user_id=s.user_id
		 JOIN users u ON u.user_id=s.user_id
		 WHERE s.revoked_at IS NOT NULL OR julianday(s.expires_at)<=julianday(?)
		 OR t.token_id IS NULL OR t.revoked_at IS NOT NULL OR u.enabled=false
		 OR julianday(t.expires_at)<=julianday(?)
		 LIMIT ?
		)`, now, now, credentialCleanupBatch)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM tokens WHERE token_id IN (
		 SELECT token_id FROM tokens
		 WHERE julianday(expires_at)<=julianday(?) LIMIT ?
		)`, now, credentialCleanupBatch)
		return err
	})
}
