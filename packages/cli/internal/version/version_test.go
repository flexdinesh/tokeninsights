package version

import (
	"runtime/debug"
	"testing"
)

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

func TestBuildInfoFallback(t *testing.T) {
	for _, test := range []struct {
		name    string
		command string
		version string
		commit  string
		date    string
		info    *debug.BuildInfo
		want    string
	}{
		{name: "missing build info", version: "dev", want: "tokeninsights dev"},
		{name: "stable module", version: "dev", info: buildInfo("v0.1.7", false), want: "tokeninsights v0.1.7 abc123 2026-10-18T00:00:00Z"},
		{name: "edge pseudo version", version: "dev", info: buildInfo("v0.1.8-0.20261018000000-abcdefabcdef", false), want: "tokeninsights v0.1.8-0.20261018000000-abcdefabcdef abc123 2026-10-18T00:00:00Z"},
		{name: "local checkout", version: "dev", info: buildInfo("(devel)", false), want: "tokeninsights dev abc123 2026-10-18T00:00:00Z"},
		{name: "dirty checkout", version: "dev", info: buildInfo("(devel)", true), want: "tokeninsights dev abc123+dirty 2026-10-18T00:00:00Z"},
		{name: "no module metadata", version: "dev", info: &debug.BuildInfo{}, want: "tokeninsights dev"},
		{name: "explicit release version", version: "0.1.7", info: buildInfo("v0.1.8", true), want: "tokeninsights 0.1.7"},
		{name: "explicit linker metadata", version: "dev", commit: "override", date: "today", info: buildInfo("v0.1.7", true), want: "tokeninsights v0.1.7 override today"},
		{name: "server source install", command: "tokeninsights-server", version: "dev", info: buildInfo("v0.1.7", false), want: "tokeninsights-server v0.1.7 abc123 2026-10-18T00:00:00Z"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := test.command
			if command == "" {
				command = "tokeninsights"
			}
			if got := format(command, test.version, test.commit, test.date, test.info); got != test.want {
				t.Fatalf("format() = %q, want %q", got, test.want)
			}
		})
	}
}

func buildInfo(version string, modified bool) *debug.BuildInfo {
	modifiedText := "false"
	if modified {
		modifiedText = "true"
	}
	return &debug.BuildInfo{
		Main: debug.Module{Version: version},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123"},
			{Key: "vcs.time", Value: "2026-10-18T00:00:00Z"},
			{Key: "vcs.modified", Value: modifiedText},
		},
	}
}
