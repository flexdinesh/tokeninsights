package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestSelectTagStartsSeriesAtPatchZero(t *testing.T) {
	series := mustParseSeries(t, "0.1")
	if got, want := selectedTag(t, series, nil, nil), "packages/cli/v0.1.0"; got != want {
		t.Fatalf("selectTag() = %q, want %q", got, want)
	}
}

func TestSelectTagIncrementsHighestPatch(t *testing.T) {
	series := mustParseSeries(t, "0.1")
	tags := []string{
		"packages/cli/v0.1.1",
		"packages/cli/v0.1.9",
		"packages/cli/v0.1.3",
		"packages/cli/v0.2.20",
		"packages/cli/v0.1.10-beta.1",
		"v0.1.10",
	}
	if got, want := selectedTag(t, series, tags, nil), "packages/cli/v0.1.10"; got != want {
		t.Fatalf("selectTag() = %q, want %q", got, want)
	}
}

func TestSelectTagReusesReleaseAtHead(t *testing.T) {
	series := mustParseSeries(t, "0.1")
	tags := []string{"packages/cli/v0.1.0", "packages/cli/v0.1.1"}
	headTags := []string{"packages/cli/v0.1.1"}
	if got, want := selectedTag(t, series, tags, headTags), "packages/cli/v0.1.1"; got != want {
		t.Fatalf("selectTag() = %q, want %q", got, want)
	}
}

func TestSelectTagStartsChangedSeriesAtPatchZero(t *testing.T) {
	series := mustParseSeries(t, "1.0")
	tags := []string{"packages/cli/v0.1.8", "packages/cli/v0.2.4"}
	if got, want := selectedTag(t, series, tags, nil), "packages/cli/v1.0.0"; got != want {
		t.Fatalf("selectTag() = %q, want %q", got, want)
	}
}

func TestParseSeriesRejectsFullOrPaddedVersions(t *testing.T) {
	for _, value := range []string{"0.1.0", "v0.1", "01.2", "1.02", "next"} {
		if _, err := parseSeries(value); err == nil {
			t.Fatalf("parseSeries(%q) succeeded", value)
		}
	}
}

func TestGitSourceTagsReleaseOwnership(t *testing.T) {
	for _, test := range []struct {
		name       string
		parents    []string
		annotated  bool
		wantTag    string
		historical bool
	}{
		{name: "historical direct tag", historical: true, wantTag: "packages/cli/v0.1.7"},
		{name: "asset release", parents: []string{"source"}, wantTag: "packages/cli/v0.1.7"},
		{name: "annotated asset release", parents: []string{"source"}, annotated: true, wantTag: "packages/cli/v0.1.7"},
		{name: "source is second parent", parents: []string{"base", "source"}, wantTag: "packages/cli/v0.1.8"},
		{name: "older release", parents: []string{"base"}, wantTag: "packages/cli/v0.1.8"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := newGitRepository(t)
			gitCommand(t, root, "commit", "--allow-empty", "-m", "base")
			base := gitCommand(t, root, "rev-parse", "HEAD")
			gitCommand(t, root, "commit", "--allow-empty", "-m", "source")
			source := gitCommand(t, root, "rev-parse", "HEAD")
			target := source
			if !test.historical {
				arguments := []string{"commit-tree", "HEAD^{tree}", "-m", "release"}
				for _, parent := range test.parents {
					if parent == "source" {
						arguments = append(arguments, "-p", source)
					} else {
						arguments = append(arguments, "-p", base)
					}
				}
				target = gitCommand(t, root, arguments...)
			}
			arguments := []string{"tag"}
			if test.annotated {
				arguments = append(arguments, "-a", "-m", "release")
			}
			gitCommand(t, root, append(arguments, "packages/cli/v0.1.7", target)...)
			tags, err := gitTags(root, "--list")
			if err != nil {
				t.Fatal(err)
			}
			sourceTags, err := gitSourceTags(root, tags)
			if err != nil {
				t.Fatal(err)
			}
			if got := selectedTag(t, mustParseSeries(t, "0.1"), tags, sourceTags); got != test.wantTag {
				t.Fatalf("selected tag = %q, want %q", got, test.wantTag)
			}
		})
	}
}

func TestGitSourceTagsIgnoresOtherModulesAndUnstableVersions(t *testing.T) {
	root := newGitRepository(t)
	gitCommand(t, root, "commit", "--allow-empty", "-m", "source")
	for _, tag := range []string{"v0.1.9", "packages/other/v0.1.9", "packages/cli/v0.1.9-beta.1", "packages/cli/v0.1.09", "packages/cli/v0.1.9+build"} {
		gitCommand(t, root, "tag", tag)
	}
	tags, err := gitTags(root, "--list")
	if err != nil {
		t.Fatal(err)
	}
	sourceTags, err := gitSourceTags(root, tags)
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceTags) != 0 {
		t.Fatalf("source tags = %q, want none", sourceTags)
	}
	if got, want := selectedTag(t, mustParseSeries(t, "0.1"), tags, sourceTags), "packages/cli/v0.1.0"; got != want {
		t.Fatalf("selected tag = %q, want %q", got, want)
	}
}

func newGitRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitCommand(t, root, "init", "--initial-branch=main")
	gitCommand(t, root, "config", "user.name", "Release Test")
	gitCommand(t, root, "config", "user.email", "release@example.invalid")
	gitCommand(t, root, "config", "commit.gpgsign", "false")
	gitCommand(t, root, "config", "tag.gpgsign", "false")
	return root
}

func gitCommand(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", root}, arguments...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func mustParseSeries(t *testing.T, value string) releaseSeries {
	t.Helper()
	series, err := parseSeries(value)
	if err != nil {
		t.Fatal(err)
	}
	return series
}

func selectedTag(t *testing.T, series releaseSeries, tags, headTags []string) string {
	t.Helper()
	tag, err := selectTag(series, tags, headTags)
	if err != nil {
		t.Fatal(err)
	}
	return tag
}
