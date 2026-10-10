package datastore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlitecore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlutil"
)

func inspectCurrent(ctx context.Context, database *sql.DB, expectedKind string) error {
	var role, id, kind string
	var version int
	if err := database.QueryRowContext(ctx, sqlutil.Bind("SELECT role,schema_version,database_id,server_kind FROM ingestion_instance WHERE id=1")).Scan(&role, &version, &id, &kind); err != nil {
		return err
	}
	if role != "server-data" || version != SchemaVersion || id == "" || (kind != KindPersonal && kind != KindHosted) {
		return errors.New("incompatible_server_data")
	}
	var applicationID, versionID int
	if err := database.QueryRowContext(ctx, sqlutil.Bind("PRAGMA application_id")).Scan(&applicationID); err != nil {
		return err
	}
	if err := database.QueryRowContext(ctx, sqlutil.Bind("PRAGMA user_version")).Scan(&versionID); err != nil {
		return err
	}
	if applicationID != ApplicationID || versionID != SchemaVersion {
		return errors.New("incompatible_server_data")
	}
	if err := sqlitecore.Validate(ctx, database, Schema); err != nil {
		return err
	}
	if expectedKind != "" && kind != expectedKind {
		return errors.New("server_kind_mismatch")
	}
	return inspectDatasets(ctx, database, id, kind)
}

func inspectDatasets(ctx context.Context, database *sql.DB, id, kind string) error {
	rows, err := database.QueryContext(ctx, sqlutil.Bind("SELECT dataset_id FROM ingestion_metadata"))
	if err != nil {
		return err
	}
	var datasets []string
	for rows.Next() {
		var dataset string
		if err := rows.Scan(&dataset); err != nil {
			_ = rows.Close()
			return err
		}
		datasets = append(datasets, dataset)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if kind == KindPersonal && (len(datasets) != 1 || datasets[0] != DatasetID) {
		return errors.New("incompatible_server_data")
	}
	for _, dataset := range datasets {
		m, err := ReadMetadataForDataset(ctx, database, dataset)
		if err != nil {
			return err
		}
		if m.DatabaseID != id || m.Kind != kind {
			return errors.New("incompatible_server_data")
		}
	}
	return nil
}

func inspectPath(ctx context.Context, path, kind string) error {
	database, err := connect(path, true)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	return inspectCurrent(ctx, database, kind)
}
