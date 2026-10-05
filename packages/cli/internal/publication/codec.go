package publication

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

const maxJSONDepth = 32

func EncodeBatch(batch Batch) ([]byte, error) {
	if err := ValidateBatch(batch); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(batch)
	if err != nil {
		return nil, invalid("invalid_request", "encoding")
	}
	if len(encoded) > MaxBodyBytes {
		return nil, invalid("too_large", "body")
	}
	return encoded, nil
}

func DecodeBatch(body []byte) (Batch, error) {
	batch, err := decode[Batch](body)
	if err == nil {
		err = ValidateBatch(batch)
	}
	return batch, err
}

func DecodeFact(body []byte) (Fact, error) {
	fact, err := decode[Fact](body)
	if err == nil {
		err = ValidateFact(fact)
	}
	return fact, err
}

func DecodeReceipt(body []byte) (Receipt, error) {
	r, err := decode[Receipt](body)
	if err != nil {
		return r, err
	}
	if !validString(r.DatabaseID, true) || !validString(r.StreamID, true) || !validString(r.BatchID, true) ||
		len(r.RequestHash) != 64 || !validInteger(r.FromSequence) || r.FromSequence < 1 || !validInteger(r.ToSequence) || r.ToSequence < r.FromSequence ||
		!validInteger(r.Inserted) || !validInteger(r.Updated) || !validInteger(r.Noop) || !validInteger(r.CommittedAtMs) || !validInteger(r.Revision) {
		return r, invalid("invalid_request", "receipt")
	}
	return r, nil
}

func DecodeCapabilities(body []byte) (Capabilities, error) {
	c, err := decode[Capabilities](body)
	if err != nil {
		return c, err
	}
	if c.ProtocolVersion != ProtocolVersion || c.IdentityVersion != IdentityVersion || c.SemanticsVersion != SemanticsVersion {
		return c, invalid("incompatible", "version")
	}
	if !validString(c.DatabaseID, true) || c.MaxBodyBytes != MaxBodyBytes || c.MaxEntries != MaxEntries ||
		c.MaxStringBytes != MaxStringBytes || c.MaxInteger != SafeInteger {
		return c, invalid("incompatible", "limits")
	}
	return c, nil
}

// decode rejects duplicate keys before unmarshalling, so a later occurrence
// cannot silently replace an earlier identity or private field. Shape checking
// requires explicit counters: absent/null is not interpreted as canonical zero.
func decode[T any](body []byte) (T, error) {
	var value T
	if len(body) > MaxBodyBytes {
		return value, invalid("too_large", "body")
	}
	if len(body) == 0 || !utf8.Valid(body) {
		return value, invalid("invalid_request", "body")
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if err := uniqueJSONValue(d, 0); err != nil {
		return value, err
	}
	if _, err := d.Token(); err != io.EOF {
		return value, invalid("invalid_request", "trailingJSON")
	}
	if err := validateShape(body, reflect.TypeFor[T]()); err != nil {
		return value, err
	}
	if err := json.Unmarshal(body, &value); err != nil {
		return value, invalid("invalid_request", "jsonType")
	}
	return value, nil
}

func uniqueJSONValue(d *json.Decoder, depth int) error {
	if depth > maxJSONDepth {
		return invalid("invalid_request", "jsonDepth")
	}
	token, err := d.Token()
	if err != nil {
		return invalid("invalid_request", "json")
	}
	delim, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return invalid("invalid_request", "json")
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return invalid("invalid_request", "duplicateJSONField")
			}
			seen[key] = true
			if err := uniqueJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return invalid("invalid_request", "json")
	}
	end, err := d.Token()
	if err != nil || delim == '{' && end != json.Delim('}') || delim == '[' && end != json.Delim(']') {
		return invalid("invalid_request", "json")
	}
	return nil
}

func validateShape(body json.RawMessage, typ reflect.Type) error {
	if typ.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
			return nil
		}
		return validateShape(body, typ.Elem())
	}
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return invalid("invalid_request", "nullField")
	}
	switch typ.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if err := json.Unmarshal(body, &object); err != nil {
			return invalid("invalid_request", "jsonObject")
		}
		allowed := make(map[string]bool)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			tag := strings.Split(field.Tag.Get("json"), ",")
			name := tag[0]
			allowed[name] = true
			raw, exists := object[name]
			optional := len(tag) > 1 && tag[1] == "omitempty"
			if !exists {
				if !optional {
					return invalid("invalid_request", "requiredField")
				}
				continue
			}
			if err := validateShape(raw, field.Type); err != nil {
				return err
			}
		}
		for name := range object {
			if !allowed[name] {
				return invalid("invalid_request", "unknownField")
			}
		}
	case reflect.Slice:
		var values []json.RawMessage
		if err := json.Unmarshal(body, &values); err != nil {
			return invalid("invalid_request", "jsonArray")
		}
		if len(values) > MaxEntries {
			return invalid("too_large", "entries")
		}
		for _, raw := range values {
			if err := validateShape(raw, typ.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}
