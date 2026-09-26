package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// LatestIngestHostname describes the latest completed source ingest, including
// unchanged-source checks. Older runs and failed hostname lookups remain unknown.
func LatestIngestHostname(ctx context.Context, reader Reader) (string, error) {
	var hostname sql.NullString
	err := reader.QueryRowContext(ctx, "SELECT hostname FROM ingest_runs WHERE status = 'completed' ORDER BY id DESC LIMIT 1").Scan(&hostname)
	if errors.Is(err, sql.ErrNoRows) {
		return "unknown", nil
	}
	if err != nil {
		return "", err
	}
	if value := strings.TrimSpace(hostname.String); value != "" {
		return value, nil
	}
	return "unknown", nil
}
