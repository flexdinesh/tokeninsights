package version

import (
	"runtime/debug"
	"strings"
)

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

func String() string {
	return StringFor("tokeninsights")
}

func StringFor(command string) string {
	info, _ := debug.ReadBuildInfo()
	return format(command, Version, Commit, Date, info)
}

func format(command, version, commit, date string, info *debug.BuildInfo) string {
	if version == "dev" && info != nil {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		var revision, timestamp string
		var modified bool
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.time":
				timestamp = setting.Value
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
		if commit == "" {
			commit = revision
			if modified && commit != "" {
				commit += "+dirty"
			}
		}
		if date == "" {
			date = timestamp
		}
	}
	parts := []string{command, version}
	if strings.TrimSpace(commit) != "" {
		parts = append(parts, commit)
	}
	if strings.TrimSpace(date) != "" {
		parts = append(parts, date)
	}
	return strings.Join(parts, " ")
}
