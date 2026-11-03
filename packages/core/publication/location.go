package publication

// RepositorySourceRank orders repository evidence consistently in collector and
// server reference merges. Provenance does not change location identity.
func RepositorySourceRank(source string) int {
	switch source {
	case "harness":
		return 4
	case "git-remote":
		return 3
	case "git-common-dir":
		return 2
	case "opencode-project":
		return 1
	default:
		return 0
	}
}
