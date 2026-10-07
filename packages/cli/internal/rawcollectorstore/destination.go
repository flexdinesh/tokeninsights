package rawcollectorstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

// ResolveDeliveryDestination also recognizes the historical private-transport
// alias for a verified local database/dataset. Retained requests finish on their
// original binding before the most advanced acknowledged binding is reused.
// Nothing rewrites saved bytes or copies a cursor between bindings.
func (s *Store) ResolveDeliveryDestination(ctx context.Context, endpoint, databaseID, datasetID string, local bool) (string, error) {
	if databaseID == "" || !evidence.ValidDatasetID(datasetID) {
		return "", errors.New("invalid_destination")
	}
	query := `SELECT d.destination_id,d.database_id,d.acknowledged_sequence,b.batch_id,b.request_bytes,b.request_hash,b.dataset_id,b.protocol_version
		FROM evidence_destinations d LEFT JOIN evidence_batches b ON b.destination_id=d.destination_id AND b.receipt_bytes IS NULL
		WHERE d.dataset_id=? AND rtrim(d.endpoint,'/')=?`
	args := []interface{}{datasetID, endpoint}
	if local {
		query = `SELECT d.destination_id,d.database_id,d.acknowledged_sequence,b.batch_id,b.request_bytes,b.request_hash,b.dataset_id,b.protocol_version
			FROM evidence_destinations d LEFT JOIN evidence_batches b ON b.destination_id=d.destination_id AND b.receipt_bytes IS NULL
			WHERE d.dataset_id=? AND (rtrim(d.endpoint,'/')=? OR rtrim(d.endpoint,'/')='http://local') AND d.database_id=?`
		args = append(args, databaseID)
	}
	query += ` ORDER BY (b.batch_id IS NOT NULL) DESC,d.acknowledged_sequence DESC,(d.endpoint=?) DESC,d.destination_id`
	args = append(args, endpoint)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	var selected string
	for rows.Next() {
		var id, previous string
		var cursor int64
		var batchID, hash, savedDataset sql.NullString
		var protocol sql.NullInt64
		var body []byte
		if err := rows.Scan(&id, &previous, &cursor, &batchID, &body, &hash, &savedDataset, &protocol); err != nil {
			return "", err
		}
		if previous != databaseID {
			return "", errors.New("server_database_changed")
		}
		if batchID.Valid {
			batch, err := evidence.DecodeBatch(body)
			if err != nil || evidence.Hash(body) != hash.String || batch.BatchID != batchID.String || batch.DatabaseID != databaseID || batch.EffectiveDatasetID() != datasetID || savedDataset.String != datasetID || int64(batch.ProtocolVersion) != protocol.Int64 || batch.FromSequence != cursor+1 {
				return "", errors.New("saved_request_binding_corrupt")
			}
		}
		if selected == "" {
			selected = id
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if selected != "" {
		return selected, nil
	}
	// Close before binding: the collector deliberately uses a single connection.
	if err := rows.Close(); err != nil {
		return "", err
	}
	return s.ResolveDestination(ctx, endpoint, databaseID, datasetID, local)
}
