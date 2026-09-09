package db

import (
	"context"
	"database/sql"
)

// SessionCounts compares sessions matching the view with all countable sessions
// in this database. Canonical IDs distinguish sessions across harnesses.
type SessionCounts struct {
	Shown  int64
	Synced int64
}

func ViewerSessionCounts(ctx context.Context, database *sql.DB, f Filter) (SessionCounts, error) {
	where, args := canonicalWhereClause(f)
	var counts SessionCounts
	err := database.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(DISTINCT ctu.session_id)
			 FROM canonical_token_usage ctu
			 INNER JOIN canonical_sessions cs ON cs.id = ctu.session_id
			 `+where+`),
			(SELECT COUNT(DISTINCT ctu.session_id)
			 FROM canonical_token_usage ctu
			 INNER JOIN canonical_sessions cs ON cs.id = ctu.session_id
			 WHERE ctu.is_countable = 1)
	`, args...).Scan(&counts.Shown, &counts.Synced)
	return counts, err
}
