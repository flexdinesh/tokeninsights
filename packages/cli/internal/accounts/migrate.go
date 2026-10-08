package accounts

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ImportLegacy copies once in one SQLite transaction while the composition owns
// DuckDB exclusively. The old account tables remain a read-only recovery source.
func (s *SQLite) ImportLegacy(ctx context.Context, source *sql.DB) error {
	return s.store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		var done bool
		if err := tx.QueryRowContext(ctx, "SELECT legacy_accounts_imported FROM application_metadata WHERE id=1").Scan(&done); err != nil {
			return err
		}
		if done {
			return nil
		}
		var existing int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&existing); err != nil {
			return err
		}
		if existing != 0 {
			return errors.New("application_migration_conflict")
		}
		for _, table := range []struct {
			name, columns string
			count         int
		}{
			{"users", "user_id,dataset_id,display_name,enabled,created_at", 5},
			{"tokens", "token_id,user_id,digest,permissions,created_at,expires_at,revoked_at", 7},
			{"sessions", "session_id,user_id,source_token_id,digest,created_at,expires_at,revoked_at", 7},
		} {
			rows, err := source.QueryContext(ctx, "SELECT "+table.columns+" FROM accounts."+table.name)
			if err != nil {
				return err
			}
			copied := 0
			for rows.Next() {
				values := make([]interface{}, table.count)
				pointers := make([]interface{}, table.count)
				for i := range values {
					pointers[i] = &values[i]
				}
				if err := rows.Scan(pointers...); err != nil {
					_ = rows.Close()
					return err
				}
				if table.name == "users" {
					dataset, ok := values[1].(string)
					if !ok {
						_ = rows.Close()
						return errors.New("invalid_legacy_dataset")
					}
					exists, err := s.datasets.DatasetExists(ctx, dataset)
					if err != nil {
						_ = rows.Close()
						return err
					}
					if !exists {
						_ = rows.Close()
						return errors.New("legacy_dataset_missing")
					}
				}
				suffix := ""
				if table.name == "users" {
					suffix = ",'ready'"
				}
				placeholders := strings.TrimSuffix(strings.Repeat("?,", table.count), ",")
				if _, err := tx.ExecContext(ctx, "INSERT INTO "+table.name+" VALUES("+placeholders+suffix+")", values...); err != nil {
					_ = rows.Close()
					return err
				}
				copied++
			}
			err = rows.Err()
			_ = rows.Close()
			if err != nil {
				return err
			}
			var count int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table.name).Scan(&count); err != nil {
				return err
			}
			if count != copied {
				return errors.New("application_migration_count_mismatch")
			}
		}
		_, err := tx.ExecContext(ctx, "UPDATE application_metadata SET legacy_accounts_imported=1 WHERE id=1")
		return err
	})
}
