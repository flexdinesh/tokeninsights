package version

import "testing"

func TestStringIncludesReleaseMetadata(t *testing.T) {
	oldVersion := Version
	oldCommit := Commit
	oldDate := Date
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		Date = oldDate
	})

	Version = "0.0.1"
	Commit = "abc123"
	Date = "2026-06-13T00:00:00Z"

	if got, want := String(), "tokeninsights 0.0.1 abc123 2026-06-13T00:00:00Z"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
