package main

import "testing"

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
