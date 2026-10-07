package accounts

import (
	"context"
	"database/sql"
	"time"
)

const (
	credentialCleanupInterval = time.Minute
	credentialCleanupBatch    = 1000
	credentialCleanupTimeout  = 10 * time.Second
)

// Cleanup removes expired credentials in bounded transactions. Revoked token
// IDs remain available to operators; evidence and datasets are never touched.
func (s *Service) Cleanup(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM accounts.sessions WHERE session_id IN (
		 SELECT s.session_id FROM accounts.sessions s
		 LEFT JOIN accounts.tokens t ON t.token_id=s.source_token_id AND t.user_id=s.user_id
		 JOIN accounts.users u ON u.user_id=s.user_id
		 WHERE s.revoked_at IS NOT NULL OR TRY_CAST(s.expires_at AS TIMESTAMPTZ)<=CAST(? AS TIMESTAMPTZ)
		 OR t.token_id IS NULL OR t.revoked_at IS NOT NULL OR u.enabled=false
		 OR TRY_CAST(t.expires_at AS TIMESTAMPTZ)<=CAST(? AS TIMESTAMPTZ)
		 LIMIT ?
		)`, now, now, credentialCleanupBatch)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM accounts.tokens WHERE token_id IN (
		 SELECT token_id FROM accounts.tokens
		 WHERE TRY_CAST(expires_at AS TIMESTAMPTZ)<=CAST(? AS TIMESTAMPTZ) LIMIT ?
		)`, now, credentialCleanupBatch)
		return err
	})
}

func (s *Service) RunCleanup(ctx context.Context, report func(error)) {
	run := func() {
		attempt, cancel := context.WithTimeout(ctx, credentialCleanupTimeout)
		defer cancel()
		if err := s.Cleanup(attempt); err != nil && ctx.Err() == nil && report != nil {
			report(err)
		}
	}
	run()
	ticker := time.NewTicker(credentialCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
