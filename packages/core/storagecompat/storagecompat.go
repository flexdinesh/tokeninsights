// Package storagecompat classifies an existing database file against the role
// and schema version this binary supports. It holds no storage mechanics.
package storagecompat

type Status int

const (
	// Missing means no file exists; opening creates a current store.
	Missing Status = iota
	Compatible
	// Older means the role matches and an older release wrote a lower schema version.
	Older
	// Newer means a newer release wrote a higher schema version.
	Newer
	// Foreign means the file does not carry this store's role.
	Foreign
	// Mismatch means the supported version is recorded but schema objects differ.
	Mismatch
)

type Result struct {
	Status    Status
	Found     int
	Supported int
}

// Classify compares recorded identity with the expected identity. Matching
// identity still requires an object comparison, reported by objectsMatch.
func Classify(role, version, expectedRole, expectedVersion int, objectsMatch bool) Result {
	result := Result{Found: version, Supported: expectedVersion}
	switch {
	case role != expectedRole:
		result.Status = Foreign
	case version < expectedVersion:
		result.Status = Older
	case version > expectedVersion:
		result.Status = Newer
	case !objectsMatch:
		result.Status = Mismatch
	default:
		result.Status = Compatible
	}
	return result
}
