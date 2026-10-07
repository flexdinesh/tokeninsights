package processor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

func Nested(record map[string]interface{}, key string) map[string]interface{} {
	value, ok := record[key]
	if !ok {
		return nil
	}
	nestedValue, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return nestedValue
}

func StringField(record map[string]interface{}, names ...string) *string {
	for _, name := range names {
		value, ok := record[name]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if ok && strings.TrimSpace(text) != "" {
			trimmed := strings.TrimSpace(text)
			return &trimmed
		}
	}
	return nil
}

func StringValue(record map[string]interface{}, fallback string, names ...string) string {
	value := StringField(record, names...)
	if value == nil {
		return fallback
	}
	return *value
}

func IntField(record map[string]interface{}, names ...string) *int64 {
	for _, name := range names {
		value, ok := record[name]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case json.Number:
			result, err := typed.Int64()
			if err == nil {
				return &result
			}
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed >= math.MaxInt64 || typed < math.MinInt64 {
				return nil
			}
			result := int64(typed)
			return &result
		case int64:
			result := typed
			return &result
		case string:
			result, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
			if err == nil {
				return &result
			}
		}
		return nil
	}
	return nil
}

func TokenComponentSum(values ...*int64) (int64, bool) {
	var total int64
	for _, value := range values {
		if value == nil {
			continue
		}
		if *value < 0 {
			return 0, false
		}
		if *value > math.MaxInt64-total {
			return 0, false
		}
		total += *value
	}
	return total, true
}

func HasInvalidIntegerField(record map[string]interface{}, names ...string) bool {
	for _, name := range names {
		value, ok := record[name]
		if ok && value != nil && IntField(record, name) == nil {
			return true
		}
	}
	return false
}

func StableHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// Native tuples use unambiguous encoding; source IDs may contain separators.
func NativeTupleHash(parts ...string) string {
	encoded, _ := json.Marshal(parts)
	return StableHash(string(encoded))
}

// Source-session provenance is allowlisted host metadata. Filenames remain a
// useful local fallback, but cannot identify portable published native sessions.
type sourceIdentityMetadata struct {
	SessionSource string  `json:"session_source,omitempty"`
	RequestID     *string `json:"request_id,omitempty"`
}

func SourceIdentityJSON(sessionSource string, requestID *string) *string {
	encoded, _ := json.Marshal(sourceIdentityMetadata{SessionSource: sessionSource, RequestID: requestID})
	metadata := string(encoded)
	return &metadata
}

func SourceSessionIdentity(metadata *string) string {
	if metadata == nil {
		return ""
	}
	var identity sourceIdentityMetadata
	if json.Unmarshal([]byte(*metadata), &identity) != nil {
		return ""
	}
	return identity.SessionSource
}
func stringValueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
