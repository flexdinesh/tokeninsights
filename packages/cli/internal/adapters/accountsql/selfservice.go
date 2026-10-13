package accountsql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlutil"
)

func (s *Repository) User(ctx context.Context, userID string) (accounts.User, error) {
	var user accounts.User
	err := s.store.SQL().QueryRowContext(ctx, sqlutil.Bind("SELECT user_id,dataset_id,display_name,username,enabled FROM users WHERE user_id=? AND enabled=true AND provisioning='ready'"), userID).Scan(&user.UserID, &user.DatasetID, &user.DisplayName, &user.Username, &user.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return accounts.User{}, accounts.ErrNotFound
	}
	return user, err
}

func (s *Repository) SetPassword(ctx context.Context, userID, password string) error {
	hash, err := accounts.HashPassword(password)
	if err != nil {
		return err
	}
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, sqlutil.Bind("UPDATE users SET password_hash=? WHERE user_id=? AND enabled=true AND provisioning='ready'"), hash, userID)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return accounts.ErrNotFound
		}
		return revokeSessions(ctx, tx, userID)
	})
}

func (s *Repository) ChangePassword(ctx context.Context, userID, current, password string) error {
	if err := accounts.ValidatePassword(password); err != nil {
		return err
	}
	var savedHash sql.NullString
	err := s.store.SQL().QueryRowContext(ctx, sqlutil.Bind("SELECT password_hash FROM users WHERE user_id=? AND enabled=true AND provisioning='ready'"), userID).Scan(&savedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return accounts.ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	if accounts.ValidatePassword(current) != nil {
		return accounts.ErrCurrentPasswordMismatch
	}
	encoded := accounts.DummyPasswordHash
	if savedHash.Valid {
		encoded = savedHash.String
	}
	verified := accounts.VerifyPassword(current, encoded)
	if !savedHash.Valid {
		return accounts.ErrUnauthenticated
	}
	if !verified {
		return accounts.ErrCurrentPasswordMismatch
	}
	hash, err := accounts.HashPassword(password)
	if err != nil {
		return err
	}
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, sqlutil.Bind("UPDATE users SET password_hash=? WHERE user_id=? AND enabled=true AND provisioning='ready' AND password_hash=?"), hash, userID, savedHash.String)
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
		return revokeSessions(ctx, tx, userID)
	})
}

func revokeSessions(ctx context.Context, tx *sql.Tx, userID string) error {
	_, err := tx.ExecContext(ctx, sqlutil.Bind("UPDATE sessions SET revoked_at=COALESCE(revoked_at,?) WHERE user_id=?"), time.Now().UTC().Format(time.RFC3339Nano), userID)
	return err
}

func (s *Repository) RevokeUserToken(ctx context.Context, userID, tokenID string) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, sqlutil.Bind(`UPDATE tokens SET revoked_at=COALESCE(revoked_at,?) WHERE token_id=? AND user_id=?
   AND EXISTS(SELECT 1 FROM users WHERE user_id=? AND enabled=true AND provisioning='ready')`), time.Now().UTC().Format(time.RFC3339Nano), tokenID, userID, userID)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return accounts.ErrNotFound
		}
		return nil
	})
}

func (s *Repository) ListTokens(ctx context.Context, userID string) ([]accounts.TokenInfo, error) {
	rows, err := s.store.SQL().QueryContext(ctx, sqlutil.Bind(`SELECT t.token_id,t.label,t.permissions,t.created_at,t.expires_at,t.revoked_at
  FROM tokens t JOIN users u ON u.user_id=t.user_id WHERE t.user_id=? AND u.enabled=true AND u.provisioning='ready'
  ORDER BY t.created_at DESC,t.token_id`), userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tokens := make([]accounts.TokenInfo, 0)
	for rows.Next() {
		var token accounts.TokenInfo
		var encoded, created string
		var expires, revoked sql.NullString
		if err := rows.Scan(&token.TokenID, &token.Label, &encoded, &created, &expires, &revoked); err != nil {
			return nil, err
		}
		if err := decodeTokenInfo(&token, encoded, created, expires, revoked); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func decodeTokenInfo(token *accounts.TokenInfo, encoded, created string, expires, revoked sql.NullString) error {
	if err := json.Unmarshal([]byte(encoded), &token.Permissions); err != nil {
		return err
	}
	if err := accounts.ValidatePermissions(token.Permissions); err != nil {
		return err
	}
	var err error
	token.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return err
	}
	if expires.Valid {
		value, err := time.Parse(time.RFC3339Nano, expires.String)
		if err != nil {
			return err
		}
		token.ExpiresAt = &value
	}
	if revoked.Valid {
		value, err := time.Parse(time.RFC3339Nano, revoked.String)
		if err != nil {
			return err
		}
		token.RevokedAt = &value
	}
	return nil
}
