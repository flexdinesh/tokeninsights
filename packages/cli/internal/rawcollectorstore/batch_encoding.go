package rawcollectorstore

import (
	"encoding/json"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

// encodeBatchPrefix accounts for the complete JSON envelope and each encoded
// entry once. Empty arrays contribute no entry bytes; commas contribute one each.
func encodeBatchPrefix(batch evidence.Batch, entries []evidence.Entry) (evidence.Batch, []byte, error) {
	if len(entries) > evidence.MaxEntries {
		entries = entries[:evidence.MaxEntries]
	}
	wire := struct {
		evidence.Batch
		Entries []json.RawMessage `json:"entries"`
	}{Batch: batch, Entries: []json.RawMessage{}}
	var encoded []json.RawMessage
	entriesBytes := 0
	for _, entry := range entries {
		if entry.Sequence != batch.FromSequence+int64(len(batch.Entries)) {
			return batch, nil, errors.New("outbox_gap")
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
		if candidateBytes > evidence.MaxBodyBytes {
			if len(encoded) == 0 {
				return batch, nil, errors.New("record_exceeds_batch_limit")
			}
			break
		}
		batch.Entries = append(batch.Entries, entry)
		batch.ToSequence = entry.Sequence
		encoded = append(encoded, entryJSON)
		entriesBytes += len(entryJSON)
	}
	wire.Batch = batch
	wire.Entries = encoded
	body, err := json.Marshal(wire)
	if err != nil {
		return batch, nil, err
	}
	if _, err := evidence.DecodeBatch(body); err != nil {
		return batch, nil, err
	}
	return batch, body, nil
}
