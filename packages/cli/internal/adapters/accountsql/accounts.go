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
	"unicode/utf8"

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
	return s.createUser(ctx, name, "")
}
func (s *Repository) Signup(ctx context.Context, username, password string) (accounts.User, error) {
	normalized, err := accounts.NormalizeUsername(username)
	if err != nil {
		return accounts.User{}, err
	}
	if err := accounts.ValidatePassword(password); err != nil {
		return accounts.User{}, err
	}
	// Recover a previous interrupted signup only after proving the same password.
	// A ready account, including a disabled one, can never be claimed by signup.
	var pending accounts.User
	var encoded string
	err = s.store.SQL().QueryRowContext(ctx, sqlutil.Bind("SELECT user_id,dataset_id,display_name,username,password_hash FROM users WHERE username=? AND provisioning='pending' AND password_hash IS NOT NULL"), normalized).Scan(&pending.UserID, &pending.DatasetID, &pending.DisplayName, &pending.Username, &encoded)
	if err == nil {
		if !accounts.VerifyPassword(password, encoded) {
			return accounts.User{}, accounts.ErrUsernameTaken
		}
		if err := s.activate(ctx, pending); err != nil {
			return pending, err
		}
		return s.User(ctx, pending.UserID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return accounts.User{}, err
	}
	hash, err := accounts.HashPassword(password)
	if err != nil {
		return accounts.User{}, err
	}
	return s.createUser(ctx, username, hash)
}

func (s *Repository) createUser(ctx context.Context, name, passwordHash string) (accounts.User, error) {
	username, err := accounts.NormalizeUsername(name)
	if err != nil {
		return accounts.User{}, err
	}
	id, err := accounts.NewCredential()
	if err != nil {
		return accounts.User{}, err
	}
	dataset, err := accounts.NewCredential()
	if err != nil {
		return accounts.User{}, err
	}
	user := accounts.User{UserID: id, DatasetID: dataset, DisplayName: strings.TrimSpace(name), Username: username}
	var hash interface{}
	if passwordHash != "" {
		hash = passwordHash
	}
	err = s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, sqlutil.Bind("INSERT INTO users(user_id,dataset_id,display_name,enabled,created_at,provisioning,username,password_hash) VALUES(?,?,?,?,?,'pending',?,?) ON CONFLICT(username) DO NOTHING"), id, dataset, user.DisplayName, false, time.Now().UTC().Format(time.RFC3339Nano), username, hash)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return accounts.ErrUsernameTaken
		}
		return nil
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
		_, err := tx.ExecContext(ctx, sqlutil.Bind("INSERT INTO users(user_id,dataset_id,display_name,enabled,created_at,provisioning) VALUES('default','default','Local user',true,?,'ready') ON CONFLICT(user_id) DO NOTHING"), time.Now().UTC().Format(time.RFC3339Nano))
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
	return s.CreateNamedToken(ctx, userID, "API key", scopes, expires)
}
func (s *Repository) CreateNamedToken(ctx context.Context, userID, label string, scopes []string, expires *time.Time) (accounts.Token, error) {
	label = strings.TrimSpace(label)
	if label == "" || utf8.RuneCountInString(label) > accounts.MaxKeyLabelCharacters || !utf8.ValidString(label) {
		return accounts.Token{}, accounts.ErrInvalidLabel
	}
	if err := accounts.ValidatePermissions(scopes); err != nil {
		return accounts.Token{}, err
	}
	encoded, err := json.Marshal(scopes)
	if err != nil {
		return accounts.Token{}, err
	}
	if expires != nil && !expires.After(time.Now()) {
		return accounts.Token{}, accounts.ErrInvalidExpiry
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
		if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT enabled AND provisioning='ready' FROM users WHERE user_id=?"), userID).Scan(&enabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return accounts.ErrNotFound
			}
			return err
		}
		if !enabled {
			return errors.New("user_disabled")
		}
		_, err := tx.ExecContext(ctx, sqlutil.Bind("INSERT INTO tokens(token_id,user_id,digest,permissions,created_at,expires_at,revoked_at,label) VALUES(?,?,?,?,?,?,NULL,?)"), id, userID, accounts.CredentialDigest(secret), string(encoded), time.Now().UTC().Format(time.RFC3339Nano), expiry, label)
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
	if len(secret) == 0 || len(secret) > accounts.MaxTokenBytes {
		return accounts.Principal{}, "", accounts.ErrUnauthenticated
	}
	p := accounts.Principal{CredentialKind: accounts.BearerCredential}
	var tokenID, encoded string
	var enabled bool
	var expiry, revoked sql.NullString
	err := reader.QueryRowContext(ctx, sqlutil.Bind(`SELECT u.user_id,u.dataset_id,u.enabled AND u.provisioning='ready',t.token_id,t.permissions,t.expires_at,t.revoked_at
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
	p := accounts.Principal{CredentialKind: accounts.SessionCredential}
	var enabled bool
	var expiry, revoked sql.NullString
	err := s.store.SQL().QueryRowContext(ctx, sqlutil.Bind(`SELECT u.user_id,u.dataset_id,u.enabled AND u.provisioning='ready',s.expires_at,s.revoked_at FROM sessions s JOIN users u ON u.user_id=s.user_id WHERE s.digest=?`), accounts.CredentialDigest(secret)).Scan(&p.UserID, &p.DatasetID, &enabled, &expiry, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return accounts.Principal{}, accounts.ErrUnauthenticated
	}
	if err != nil {
		return accounts.Principal{}, err
	}
	if !enabled || revoked.Valid || !expiry.Valid || !validExpiry(expiry, time.Now()) {
		return accounts.Principal{}, accounts.ErrUnauthenticated
	}
	p.Permissions = []string{accounts.Read}
	return p, nil
}
func (s *Repository) CreateSession(ctx context.Context, username, password string) (string, time.Time, error) {
	normalized, err := accounts.NormalizeUsername(username)
	if err != nil || accounts.ValidatePassword(password) != nil {
		return "", time.Time{}, accounts.ErrUnauthenticated
	}
	var userID string
	var enabled bool
	var hash sql.NullString
	err = s.store.SQL().QueryRowContext(ctx, sqlutil.Bind("SELECT user_id,enabled AND provisioning='ready',password_hash FROM users WHERE username=?"), normalized).Scan(&userID, &enabled, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, err
	}
	storedHash := accounts.DummyPasswordHash
	if hash.Valid {
		storedHash = hash.String
	}
	verified := accounts.VerifyPassword(password, storedHash)
	if !verified || !enabled || !hash.Valid {
		return "", time.Time{}, accounts.ErrUnauthenticated
	}
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
		result, err := tx.ExecContext(ctx, sqlutil.Bind(`INSERT INTO sessions(session_id,user_id,digest,created_at,expires_at,revoked_at)
   SELECT ?,user_id,?,?,?,NULL FROM users WHERE user_id=? AND enabled=true AND provisioning='ready' AND password_hash=?`), id, accounts.CredentialDigest(secret), time.Now().UTC().Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano), userID, hash.String)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return accounts.ErrUnauthenticated
		}
		return nil
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

// Cleanup removes expired/revoked sessions in bounded transactions. API keys
// retain their lifecycle metadata for settings and never control session lifetime.
func (s *Repository) Cleanup(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, sqlutil.Bind(`DELETE FROM sessions WHERE session_id IN (
   SELECT s.session_id FROM sessions s JOIN users u ON u.user_id=s.user_id
   WHERE s.revoked_at IS NOT NULL OR `+s.expired("s.expires_at")+` OR u.enabled=false LIMIT ?)`), now, cleanupBatch)
		return err
	})
}

// BootstrapUser initializes an ordinary account once. Later starts never replace
// its password or enablement; changing the configured username fails explicitly.
func (s *Repository) BootstrapUser(ctx context.Context, username, password string) error {
	if err := accounts.ValidateBootstrap(username, password); err != nil {
		return err
	}
	normalized, _ := accounts.NormalizeUsername(username)
	const id = "bootstrap-user"
	var existingUsername string
	err := s.store.SQL().QueryRowContext(ctx, sqlutil.Bind("SELECT username FROM users WHERE user_id=?"), id).Scan(&existingUsername)
	if err == nil {
		if existingUsername != normalized {
			return errors.New("bootstrap_username_conflict")
		}
		return s.activate(ctx, accounts.User{UserID: id, DatasetID: id})
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	hash, err := accounts.HashPassword(password)
	if err != nil {
		return err
	}
	err = s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		var existingUsername string
		err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT username FROM users WHERE user_id=?"), id).Scan(&existingUsername)
		if err == nil {
			if existingUsername != normalized {
				return errors.New("bootstrap_username_conflict")
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var taken bool
		if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT EXISTS(SELECT 1 FROM users WHERE username=?)"), normalized).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return accounts.ErrUsernameTaken
		}
		_, err = tx.ExecContext(ctx, sqlutil.Bind("INSERT INTO users(user_id,dataset_id,display_name,enabled,created_at,provisioning,username,password_hash) VALUES(?,?,?,false,?,'pending',?,?)"), id, id, normalized, time.Now().UTC().Format(time.RFC3339Nano), normalized, hash)
		return err
	})
	if err != nil {
		return err
	}
	return s.activate(ctx, accounts.User{UserID: id, DatasetID: id})
}
