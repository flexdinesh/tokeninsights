package db

import (
	"context"
	"database/sql"
)

// Reader lets canonical queries share a read transaction for consistent dashboards.
type Reader interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}
