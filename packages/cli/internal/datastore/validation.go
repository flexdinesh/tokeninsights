package datastore

import (
	"context"
	"database/sql"
	"errors"
	"maps"
)

func inspectCurrent(ctx context.Context, database *sql.DB, expectedKind string) error {
	var role, id, kind string
	var version int
	if err := database.QueryRowContext(ctx, "SELECT role,schema_version,database_id,server_kind FROM ingestion.instance WHERE id=1").Scan(&role, &version, &id, &kind); err != nil {
		return err
	}
	if role != "server-data" || version != SchemaVersion || id == "" || (kind != KindPersonal && kind != KindHosted) {
		return errors.New("incompatible_server_data")
	}
	expected, err := currentSchemaContracts()
	if err != nil {
		return err
	}
	actual, err := schemaContracts(ctx, database)
	if err != nil {
		return err
	}
	if !maps.Equal(actual, expected) {
		return errors.New("incompatible_server_data")
	}
	if expectedKind != "" && kind != expectedKind {
		return errors.New("server_kind_mismatch")
	}
	rows, err := database.QueryContext(ctx, "SELECT dataset_id FROM ingestion.metadata")
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
