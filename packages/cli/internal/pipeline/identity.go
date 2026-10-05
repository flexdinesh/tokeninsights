package pipeline

import "encoding/json"

// Native tuples use unambiguous encoding; source IDs may contain separators.
func nativeTupleHash(parts ...string) string {
	encoded, _ := json.Marshal(parts)
	return stableHash(string(encoded))
}
