package version

import "strings"

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

func String() string {
	parts := []string{"tokeninsights", Version}
	if strings.TrimSpace(Commit) != "" {
		parts = append(parts, Commit)
	}
	if strings.TrimSpace(Date) != "" {
		parts = append(parts, Date)
	}
	return strings.Join(parts, " ")
}
