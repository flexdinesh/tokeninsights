package accountsql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlutil"
)

const cleanupBatch = 1000

type Storage interface {
	SQL() *sql.DB
	WriteTransaction(context.Context, func(*sql.Tx) error) error
}
type Repository struct {
	store    Storage
	expired  func(string) string
	datasets accounts.Datasets
}

var _ accounts.Repository = (*Repository)(nil)

func New(store Storage, datasets accounts.Datasets, expired func(string) string) *Repository {
	return &Repository{store: store, datasets: datasets, expired: expired}
}
func (s *Repository) CreateUser(ctx context.Context, name string) (accounts.User, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > accounts.MaxDisplayNameBytes {
		return accounts.User{}, errors.New("invalid_display_name")
	}
	id, err := accounts.NewCredential()
	if err != nil {
		return accounts.User{}, err
	}
	dataset, err := accounts.NewCredential()
	if err != nil {
		return accounts.User{}, err
	}
	user := accounts.User{UserID: id, DatasetID: dataset, DisplayName: name}
	err = s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, sqlutil.Bind("INSERT INTO users VALUES(?,?,?,?,?,'pending')"), id, dataset, name, false, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return accounts.User{}, err
	}
	if err := s.activate(ctx, user); err != nil {
		return user, err
	}
	user.Enabled = true
	return user, nil
}

func (s *Repository) activate(ctx context.Context, user accounts.User) error {
	if err := s.datasets.EnsureDataset(ctx, user.DatasetID); err != nil {
		return err
	}
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, sqlutil.Bind("UPDATE users SET provisioning='ready',enabled=true WHERE user_id=? AND provisioning='pending'"), user.UserID)
		return err
	})
}

// Resume completes only pending provisioning; disabled ready users stay disabled.
func (s *Repository) Resume(ctx context.Context) error {
	rows, err := s.store.SQL().QueryContext(ctx, sqlutil.Bind("SELECT user_id,dataset_id,display_name FROM users WHERE provisioning='pending'"))
	if err != nil {
		return err
	}
	var users []accounts.User
	for rows.Next() {
		var u accounts.User
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
func (s *Repository) EnsureDefault(ctx context.Context) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, sqlutil.Bind("INSERT INTO users VALUES('default','default','Local user',true,?,'ready') ON CONFLICT(user_id) DO NOTHING"), time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

func (s *Repository) DisableUser(ctx context.Context, userID string) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT EXISTS(SELECT 1 FROM users WHERE user_id=?)"), userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return accounts.ErrNotFound
		}
		_, err := tx.ExecContext(ctx, sqlutil.Bind("UPDATE users SET enabled=false,provisioning='ready' WHERE user_id=?"), userID)
		return err
	})
}

func (s *Repository) CreateToken(ctx context.Context, userID string, scopes []string, expires *time.Time) (accounts.Token, error) {
	if err := accounts.ValidatePermissions(scopes); err != nil {
		return accounts.Token{}, err
	}
	encoded, err := json.Marshal(scopes)
	if err != nil {
		return accounts.Token{}, err
	}
	if expires != nil && !expires.After(time.Now()) {
		return accounts.Token{}, errors.New("invalid_expiry")
	}
	id, err := accounts.NewCredential()
	if err != nil {
		return accounts.Token{}, err
	}
	secret, err := accounts.NewCredential()
	if err != nil {
		return accounts.Token{}, err
	}
	var expiry interface{}
	if expires != nil {
		expiry = expires.UTC().Format(time.RFC3339Nano)
	}
	err = s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		var enabled bool
		if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT enabled FROM users WHERE user_id=?"), userID).Scan(&enabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return accounts.ErrNotFound
			}
			return err
		}
		if !enabled {
			return errors.New("user_disabled")
		}
		_, err := tx.ExecContext(ctx, sqlutil.Bind("INSERT INTO tokens VALUES(?,?,?,?,?,?,NULL)"), id, userID, accounts.CredentialDigest(secret), string(encoded), time.Now().UTC().Format(time.RFC3339Nano), expiry)
		return err
	})
	if err != nil {
		return accounts.Token{}, err
	}
	return accounts.Token{TokenID: id, Secret: secret, Permissions: append([]string(nil), scopes...), ExpiresAt: expires}, nil
}

func (s *Repository) RevokeToken(ctx context.Context, id string) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT EXISTS(SELECT 1 FROM tokens WHERE token_id=?)"), id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return accounts.ErrNotFound
		}
		_, err := tx.ExecContext(ctx, sqlutil.Bind("UPDATE tokens SET revoked_at=COALESCE(revoked_at,?) WHERE token_id=?"), time.Now().UTC().Format(time.RFC3339Nano), id)
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

func authenticateToken(ctx context.Context, reader rowReader, secret string) (accounts.Principal, string, error) {
	if len(secret) != base64.RawURLEncoding.EncodedLen(accounts.CredentialBytes) {
		return accounts.Principal{}, "", accounts.ErrUnauthenticated
	}
	var p accounts.Principal
	var tokenID, encoded string
	var enabled bool
	var expiry, revoked sql.NullString
	err := reader.QueryRowContext(ctx, sqlutil.Bind(`SELECT u.user_id,u.dataset_id,u.enabled,t.token_id,t.permissions,t.expires_at,t.revoked_at
	FROM tokens t JOIN users u ON u.user_id=t.user_id WHERE t.digest=?`), accounts.CredentialDigest(secret)).Scan(&p.UserID, &p.DatasetID, &enabled, &tokenID, &encoded, &expiry, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return accounts.Principal{}, "", accounts.ErrUnauthenticated
	}
	if err != nil {
		return accounts.Principal{}, "", err
	}
	if !enabled || revoked.Valid || !validExpiry(expiry, time.Now()) {
		return accounts.Principal{}, "", accounts.ErrUnauthenticated
	}
	if err := json.Unmarshal([]byte(encoded), &p.Permissions); err != nil {
		return accounts.Principal{}, "", fmt.Errorf("invalid_account_permissions: %w", err)
	}
	if err := accounts.ValidatePermissions(p.Permissions); err != nil {
		return accounts.Principal{}, "", err
	}
	return p, tokenID, nil
}

func (s *Repository) AuthenticateBearer(ctx context.Context, secret string) (accounts.Principal, error) {
	p, _, err := authenticateToken(ctx, s.store.SQL(), secret)
	return p, err
}

func (s *Repository) AuthenticateSession(ctx context.Context, secret string) (accounts.Principal, error) {
	if len(secret) != base64.RawURLEncoding.EncodedLen(accounts.CredentialBytes) {
		return accounts.Principal{}, accounts.ErrUnauthenticated
	}
	var p accounts.Principal
	var enabled bool
	var expiry, revoked, tokenExpiry, tokenRevoked sql.NullString
	err := s.store.SQL().QueryRowContext(ctx, sqlutil.Bind(`SELECT u.user_id,u.dataset_id,u.enabled,s.expires_at,s.revoked_at,t.expires_at,t.revoked_at
	FROM sessions s JOIN users u ON u.user_id=s.user_id
	JOIN tokens t ON t.token_id=s.source_token_id AND t.user_id=s.user_id WHERE s.digest=?`), accounts.CredentialDigest(secret)).Scan(&p.UserID, &p.DatasetID, &enabled, &expiry, &revoked, &tokenExpiry, &tokenRevoked)
	if errors.Is(err, sql.ErrNoRows) {
		return accounts.Principal{}, accounts.ErrUnauthenticated
	}
	if err != nil {
		return accounts.Principal{}, err
	}
	if !enabled || revoked.Valid || tokenRevoked.Valid || !expiry.Valid || !validExpiry(expiry, time.Now()) || !validExpiry(tokenExpiry, time.Now()) {
		return accounts.Principal{}, accounts.ErrUnauthenticated
	}
	p.Permissions = []string{accounts.Read}
	return p, nil
}

func (s *Repository) CreateSession(ctx context.Context, token string) (string, time.Time, error) {
	secret, err := accounts.NewCredential()
	if err != nil {
		return "", time.Time{}, err
	}
	id, err := accounts.NewCredential()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().UTC().Add(accounts.SessionLifetime)
	err = s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		p, tokenID, err := authenticateToken(ctx, tx, token)
		if err != nil {
			return err
		}
		if !p.HasPermission(accounts.Read) {
			return accounts.ErrUnauthenticated
		}
		_, err = tx.ExecContext(ctx, sqlutil.Bind("INSERT INTO sessions VALUES(?,?,?,?,?,?,NULL)"), id, p.UserID, tokenID, accounts.CredentialDigest(secret), time.Now().UTC().Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return secret, expires, nil
}

func (s *Repository) Logout(ctx context.Context, secret string) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, sqlutil.Bind("UPDATE sessions SET revoked_at=COALESCE(revoked_at,?) WHERE digest=?"), time.Now().UTC().Format(time.RFC3339Nano), accounts.CredentialDigest(secret))
		return err
	})
}

// ReprocessUser resolves the operator's user ID to its server-owned dataset.
func (s *Repository) ReprocessUser(ctx context.Context, userID string) (int64, error) {
	var dataset string
	err := s.store.SQL().QueryRowContext(ctx, sqlutil.Bind("SELECT dataset_id FROM users WHERE user_id=?"), userID).Scan(&dataset)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, accounts.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return s.datasets.ReprocessDataset(ctx, dataset)
}

// Cleanup removes expired credentials in bounded transactions. Revoked token
// IDs remain available to operators; evidence and datasets are never touched.
func (s *Repository) Cleanup(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, sqlutil.Bind(`DELETE FROM sessions WHERE session_id IN (
		 SELECT s.session_id FROM sessions s
		 LEFT JOIN tokens t ON t.token_id=s.source_token_id AND t.user_id=s.user_id
		 JOIN users u ON u.user_id=s.user_id
		 WHERE s.revoked_at IS NOT NULL OR `+s.expired("s.expires_at")+`
		 OR t.token_id IS NULL OR t.revoked_at IS NOT NULL OR u.enabled=false
		 OR `+s.expired("t.expires_at")+`
		 LIMIT ?
		)`), now, now, cleanupBatch)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, sqlutil.Bind(`DELETE FROM tokens WHERE token_id IN (
		 SELECT token_id FROM tokens
		 WHERE `+s.expired("expires_at")+` LIMIT ?
		)`), now, cleanupBatch)
		return err
	})
}
