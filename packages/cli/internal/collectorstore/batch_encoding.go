package collectorstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

// encodeBatchPrefix accounts for the complete JSON envelope and each encoded
// entry once. Empty arrays contribute no entry bytes; commas contribute one each.
func encodeBatchPrefix(batch publication.Batch, entries []publication.Entry) (publication.Batch, []byte, error) {
	if len(entries) > publication.MaxEntries {
		entries = entries[:publication.MaxEntries]
	}
	wire := struct {
		publication.Batch
		Entries []json.RawMessage `json:"entries"`
	}{Batch: batch, Entries: []json.RawMessage{}}
	var encoded []json.RawMessage
	entriesBytes := 0
	for _, entry := range entries {
		if entry.Sequence != batch.FromSequence+int64(len(batch.Entries)) {
			return batch, nil, errors.New("publication journal gap")
		}
		entryJSON, err := json.Marshal(entry)
		if err != nil {
			return batch, nil, err
		}
		wire.ToSequence = entry.Sequence
		envelope, err := json.Marshal(wire)
		if err != nil {
			return batch, nil, err
		}
		candidateBytes := len(envelope) + entriesBytes + len(entryJSON) + len(encoded)
		if candidateBytes > publication.MaxBodyBytes {
			if len(encoded) == 0 {
				return batch, nil, fmt.Errorf("publication entry cannot fit batch: %w", &publication.ValidationError{Code: "too_large", Field: "body"})
			}
			break
		}
		batch.Entries = append(batch.Entries, entry)
		batch.ToSequence = entry.Sequence
		encoded = append(encoded, entryJSON)
		entriesBytes += len(entryJSON)
	}
	if err := publication.ValidateBatch(batch); err != nil {
		return batch, nil, err
	}
	wire.Batch = batch
	wire.Entries = encoded
	body, err := json.Marshal(wire)
	if err != nil {
		return batch, nil, err
	}
	return batch, body, nil
}
