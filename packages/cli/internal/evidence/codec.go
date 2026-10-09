package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func StrictDecode(body []byte, target interface{}) error {
	if len(body) > MaxBodyBytes {
		return errors.New("body_limit")
	}
	if !utf8.Valid(body) {
		return errors.New("invalid_request")
	}
	check := json.NewDecoder(bytes.NewReader(body))
	check.UseNumber()
	if err := uniqueValue(check, 0); err != nil {
		return err
	}
	if _, err := check.Token(); err != io.EOF {
		return errors.New("invalid_request")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid_request")
	}
	if decoder.Decode(new(interface{})) != io.EOF {
		return errors.New("invalid_request")
	}
	return nil
}

func validID(value string) bool {
	return len(value) >= 1 && SafeMetadata(value) && !strings.Contains(value, "/") && !strings.Contains(value, " ")
}

// ValidDatasetID accepts opaque dataset bindings, never paths or display names.
func ValidDatasetID(value string) bool { return validID(value) }

func ValidateRecord(record Record) error {
	if !validID(record.SourceID) || !validID(record.Lineage) || record.Ordinal < 0 || record.Ordinal > publication.SafeInteger {
		return errors.New("invalid_identity")
	}
	clean, _, err := Sanitize(record.Harness, record.Format, record.Data)
	if err != nil {
		return err
	}
	left, err := DecodeData(clean)
	if err != nil {
		return err
	}
	right, err := DecodeData(record.Data)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(left, right) {
		return errors.New("private_or_unknown_field")
	}
	if len(record.Context) > 3 || len(record.Diagnostics) > 8 {
		return errors.New("body_limit")
	}
	for _, context := range record.Context {
		if context.Ordinal < 0 || context.Ordinal > record.Ordinal {
			return errors.New("invalid_context")
		}
		clone := record
		clone.Ordinal = context.Ordinal
		clone.Data = context.Data
		clone.Context = nil
		clone.Location = nil
		clone.Diagnostics = context.Diagnostics
		if err := ValidateRecord(clone); err != nil {
			return err
		}
		kind := String(context.Data, "type")
		valid := record.Harness == "pi" && kind == "session" || record.Harness == "codex" && (kind == "session_meta" || kind == "turn_context" || kind == "event_msg" && String(context.Data, "payload.type") == "task_started")
		if !valid {
			return errors.New("invalid_context")
		}
	}
	for _, code := range record.Diagnostics {
		switch code {
		case "invalid_field_type", "unsafe_metadata", "field_limit", "native_subagent_present":
		default:
			return errors.New("invalid_diagnostic")
		}
	}
	if record.Location != nil {
		l := record.Location
		if l.DirectoryKey == "" && l.RepositoryKey == "" {
			return errors.New("invalid_location")
		}
		for _, value := range []string{l.DirectoryKey, l.DirectoryName, l.RepositoryKey, l.RepositoryName} {
			if !SafeMetadata(value) || strings.Contains(value, "/") {
				return errors.New("invalid_location")
			}
		}
		if l.RepositorySource != "" && publication.RepositorySourceRank(l.RepositorySource) == 0 {
			return errors.New("invalid_location")
		}
	}
	return nil
}

func uniqueValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("invalid_request")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("invalid_request")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return errors.New("invalid_request")
			}
			key, ok := token.(string)
			if !ok || seen[key] {
				return errors.New("duplicate_field")
			}
			seen[key] = true
			if err := uniqueValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := uniqueValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid_request")
	}
	end, err := decoder.Token()
	if err != nil || delim == '{' && end != json.Delim('}') || delim == '[' && end != json.Delim(']') {
		return errors.New("invalid_request")
	}
	return nil
}

func DecodeBatch(body []byte) (Batch, error) {
	var batch Batch
	if err := StrictDecode(body, &batch); err != nil {
		return batch, err
	}
	if batch.ProtocolVersion != ProtocolVersion || batch.ExtractorVersion != ExtractorVersion {
		return batch, errors.New("incompatible")
	}
	if !ValidDatasetID(batch.DatasetID) {
		return batch, errors.New("invalid_dataset")
	}
	if !validID(batch.DatabaseID) || !validID(batch.StreamID) || !validID(batch.BatchID) || len(batch.Entries) == 0 || len(batch.Entries) > MaxEntries || batch.FromSequence <= 0 || batch.ToSequence > publication.SafeInteger || batch.ToSequence < batch.FromSequence || batch.ToSequence-batch.FromSequence+1 != int64(len(batch.Entries)) {
		return batch, errors.New("invalid_identity")
	}
	for index, entry := range batch.Entries {
		if entry.Sequence != batch.FromSequence+int64(index) {
			return batch, errors.New("invalid_sequence")
		}
		if err := ValidateRecord(entry.Record); err != nil {
			return batch, err
		}
	}
	return batch, nil
}

func ValidateReceipt(batch Batch, request []byte, response Response) error {
	r := response.Receipt
	if r.DatabaseID != batch.DatabaseID || r.DatasetID != batch.DatasetID || r.StreamID != batch.StreamID || r.BatchID != batch.BatchID || r.RequestHash != Hash(request) || r.FromSequence != batch.FromSequence || r.ToSequence != batch.ToSequence || r.Accepted != int64(len(batch.Entries)) || !publication.ValidTimestampMs(r.AcceptedAtMs) || r.InputRevision < 0 || r.InputRevision > publication.SafeInteger {
		return errors.New("receipt_mismatch")
	}
	return nil
}
