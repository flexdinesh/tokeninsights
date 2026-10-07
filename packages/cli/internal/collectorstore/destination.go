package collectorstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

// ResolveDeliveryDestination preserves retained protocol-1 requests and reuses
// the furthest acknowledged matching binding after all saved requests finish.
// The private alias is eligible only for the verified local database. Protocol 1
// has no hosted dataset and callers must restrict this bridge to personal data.
func (s Store) ResolveDeliveryDestination(ctx context.Context, endpoint, databaseID string, local bool) (string, error) {
	query := `SELECT d.destination_id,d.database_id,d.acknowledged_sequence,b.batch_id,b.request_bytes,b.request_hash
		FROM publication_destinations d LEFT JOIN publication_batches b ON b.destination_id=d.destination_id AND b.receipt_bytes IS NULL
		WHERE rtrim(d.endpoint,'/')=?`
	args := []interface{}{endpoint}
	if local {
		query = `SELECT d.destination_id,d.database_id,d.acknowledged_sequence,b.batch_id,b.request_bytes,b.request_hash
			FROM publication_destinations d LEFT JOIN publication_batches b ON b.destination_id=d.destination_id AND b.receipt_bytes IS NULL
			WHERE (rtrim(d.endpoint,'/')=? OR rtrim(d.endpoint,'/')='http://local') AND d.database_id=?`
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
		var batchID, hash sql.NullString
		var body []byte
		if err := rows.Scan(&id, &previous, &cursor, &batchID, &body, &hash); err != nil {
			return "", err
		}
		if previous != databaseID {
			return "", errors.New("server_database_changed")
		}
		if batchID.Valid {
			var batch publication.Batch
			if json.Unmarshal(body, &batch) != nil || publication.ValidateBatch(batch) != nil || publication.RequestHash(body) != hash.String || batch.BatchID != batchID.String || batch.DatabaseID != databaseID || batch.FromSequence != cursor+1 {
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
	if err := rows.Close(); err != nil {
		return "", err
	}
	identity := endpoint
	if local {
		identity += "\x00" + databaseID
	}
	sum := sha256.Sum256([]byte(identity))
	id := hex.EncodeToString(sum[:])
	return id, s.BindDestination(ctx, id, endpoint, databaseID)
}
