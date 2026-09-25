package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func TestRemoteIdentityNormalizesTransportAndPreservesPort(t *testing.T) {
	https, _ := remoteIdentity("https://user:secret@example.com/team/repo.git")
	ssh, _ := remoteIdentity("git@example.com:team/repo.git")
	if https != ssh {
		t.Fatalf("transport identities differ: %q, %q", https, ssh)
	}
	sshURL, _ := remoteIdentity("ssh://git@example.com:22/team/repo.git")
	if sshURL != ssh {
		t.Fatalf("default SSH port identity = %q, want %q", sshURL, ssh)
	}
	nondefault, _ := remoteIdentity("ssh://git@example.com:2222/team/repo.git")
	if nondefault == ssh {
		t.Fatal("nondefault SSH port merged with default remote")
	}
	local, _ := remoteIdentity("/tmp/git-source.git")
	if local != "local:/tmp/git-source" {
		t.Fatalf("local remote identity = %q", local)
	}
	relative, _ := remoteIdentity("../git-source.git", "/tmp/worktree")
	if relative != local {
		t.Fatalf("relative local remote identity = %q", relative)
	}
}

func TestLocationLabelsOmitKeySuffix(t *testing.T) {
	label := locationLabel(strings.Repeat("界", 60))
	if !utf8.ValidString(label) || label != strings.Repeat("界", locationLabelLimit) {
		t.Fatalf("invalid short label %q", label)
	}
}

func TestSanitizedLocationPathAcrossPlatforms(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ source, want string }{
		{filepath.Join(home, "work", "repo"), "~/work/repo"},
		{"/home/alice/work/repo", "~/work/repo"},
		{"/Users/Alice/work/repo", "~/work/repo"},
		{`C:\Users\Alice\work\repo`, "~/work/repo"},
		{`D:\Documents and Settings\Alice\work\repo`, "~/work/repo"},
		{"/opt/shared/repo", "/opt/shared/repo"},
		{`C:\Projects\repo`, "C:/Projects/repo"},
	} {
		_, display := recordedDirectoryPath(test.source)
		if got := sanitizedLocationPath(display); got != test.want {
			t.Errorf("path %q: got %q, want %q", test.source, got, test.want)
		}
	}
	if _, ok := locationPathSuffix("/home/alice-other/repo", "/home/alice"); ok {
		t.Fatal("home prefix matched another user")
	}
}

func TestLocationSourcePriorityIndependentOfOrder(t *testing.T) {
	key := stableHash("remote:example.com/team/project")
	fromGit := &Location{RepositoryKey: key, RepositoryName: locationLabel("project"), RepositorySource: "git-remote"}
	fromHarness := &Location{RepositoryKey: key, RepositoryName: locationLabel("project"), RepositorySource: "harness"}
	for _, pair := range [][2]*Location{{fromGit, fromHarness}, {fromHarness, fromGit}} {
		merged, conflicts, _, conflict := mergeLocations(pair[0], pair[1], "")
		if merged == nil || merged.RepositorySource != "harness" || conflicts != "" || conflict {
			t.Fatalf("source priority changed by order: %+v, %q", merged, conflicts)
		}
	}
}

func TestFallbackRepositoryUpgradesToRemoteInSameDirectory(t *testing.T) {
	remoteKey := stableHash("remote:example.com/team/shared")
	fallback := &Location{DirectoryKey: "shared", RepositoryKey: "fallback", RepositorySource: "git-common-dir"}
	remote := &Location{DirectoryKey: "shared", RepositoryKey: remoteKey, RepositorySource: "git-remote"}
	for _, pair := range [][2]*Location{{fallback, remote}, {remote, fallback}} {
		merged, conflicts, _, conflict := mergeLocations(pair[0], pair[1], "")
		if merged == nil || merged.RepositoryKey != remoteKey || merged.DirectoryKey != "shared" || conflict || conflicts != "" {
			t.Fatalf("fallback did not upgrade in shared directory: %+v, %q", merged, conflicts)
		}
	}
}

func TestGitLocationCombinesClonesAndKeepsDirectories(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	resolver := &locationResolver{}
	var locations [2]*Location
	for index, name := range []string{"first", "second"} {
		path := filepath.Join(root, name)
		for _, args := range [][]string{{"init", "-q", path}, {"-C", path, "remote", "add", "origin", "https://example.com/team/shared.git"}} {
			if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, output)
			}
		}
		locations[index], _ = resolveFactLocation(context.Background(), SyncOptions{locationResolver: resolver}, path, "", "")
	}
	if locations[0] == nil || locations[1] == nil || locations[0].RepositoryKey == "" || locations[0].RepositoryKey != locations[1].RepositoryKey || locations[0].DirectoryKey == locations[1].DirectoryKey {
		t.Fatalf("clone grouping wrong: %+v %+v", locations[0], locations[1])
	}
}

func TestRecordedRemoteIdentifiesRepositoryWithoutCheckoutRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	path := filepath.Join(t.TempDir(), "checkout")
	if output, err := exec.Command("git", "init", "-q", path).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	location, conflict := resolveFactLocation(context.Background(), SyncOptions{}, path, "https://example.com/team/project.git", "")
	if location == nil || location.RepositoryKey == "" || location.RepositorySource != "harness" || conflict {
		t.Fatalf("recorded remote attribution: %+v, conflict=%t", location, conflict)
	}
}

func TestSuppressedDuplicateMergesLocationEvidenceInEitherOrder(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "forward", true: "reverse"}[reverse], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "usage.sqlite")
			database, _, err := db.CreateIfMissing(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = database.Close() }()
			ctx := context.Background()
			options := SyncOptions{Now: time.UnixMilli(1000)}
			runID, err := createIngestRun(ctx, database, "run", Source{Harness: HarnessOpenCode, ID: "source", Kind: "test"}, options)
			if err != nil {
				t.Fatal(err)
			}
			repoKey := stableHash("remote:example.com/team/shared")
			facts := []RawTokenFact{
				{Harness: HarnessOpenCode, SourceID: "one", SourceKind: "test", Collector: "test", Parser: "test", ObservedAtMs: 1000, UsageScope: "message", Quality: "exact", DedupeKey: "copy", Location: &Location{DirectoryKey: "a", DirectoryName: "a", RepositoryKey: repoKey, RepositoryName: "shared", RepositorySource: "harness"}},
				{Harness: HarnessOpenCode, SourceID: "two", SourceKind: "test", Collector: "test", Parser: "test", ObservedAtMs: 1000, UsageScope: "message", Quality: "exact", DedupeKey: "copy", Location: &Location{DirectoryKey: "b", DirectoryName: "b", RepositoryKey: repoKey, RepositoryName: "shared", RepositorySource: "harness"}},
			}
			if reverse {
				facts[0], facts[1] = facts[1], facts[0]
			}
			facts = append(facts, facts[0])
			if _, err := writeSourceIngest(ctx, database, runID, facts, nil, options, map[string]int64{}); err != nil {
				t.Fatal(err)
			}
			var directoryKey, repositoryKey, conflicts string
			if err := database.QueryRow(`SELECT COALESCE(l.directory_key, ''), COALESCE(l.repository_key, ''), COALESCE(r.location_conflicts, '') FROM raw_token_usage r LEFT JOIN usage_locations l ON l.id = r.location_id`).Scan(&directoryKey, &repositoryKey, &conflicts); err != nil {
				t.Fatal(err)
			}
			if directoryKey != "" || repositoryKey != repoKey || conflicts != "directory" {
				t.Fatalf("duplicate attribution = directory %q, repo %q, conflicts %q", directoryKey, repositoryKey, conflicts)
			}
		})
	}
}

func TestHarnessLocationsAndCanonicalPropagation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	path := filepath.Join(root, "sources", "session.jsonl")
	writeJSONL(t, path,
		fmt.Sprintf(`{"type":"session","id":"pi-session","cwd":%q}`, cwd),
		`{"type":"message","id":"pi-message","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","provider":"openai","model":"gpt","usage":{"input":3,"output":2,"totalTokens":5},"timestamp":1767225601000}}`,
	)
	outputPath := filepath.Join(root, "usage.sqlite")
	if _, err := Sync(ctx, SyncOptions{DBPath: outputPath, SourceDir: filepath.Dir(path), Harnesses: []Harness{HarnessPi}, Normalize: true}); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	var rawKey, canonicalKey, label string
	if err := database.QueryRow(`SELECT rloc.directory_key, cloc.directory_key, cloc.directory_name
		FROM raw_token_usage r JOIN usage_locations rloc ON rloc.id = r.location_id
		JOIN canonical_token_usage c ON c.primary_raw_fact_id = r.id
		JOIN usage_locations cloc ON cloc.id = c.location_id`).Scan(&rawKey, &canonicalKey, &label); err != nil {
		t.Fatal(err)
	}
	if rawKey != stableHash("directory:"+cwd) || canonicalKey != rawKey || label != sanitizedLocationPath(filepath.ToSlash(cwd)) {
		t.Fatalf("location propagation failed: raw=%q canonical=%q label=%q", rawKey, canonicalKey, label)
	}

	claudePath := filepath.Join(root, "claude.jsonl")
	writeJSONL(t, claudePath,
		fmt.Sprintf(`{"type":"assistant","uuid":"u","timestamp":"2026-01-01T00:00:02Z","sessionId":"claude-session","cwd":%q,"message":{"id":"m","role":"assistant","model":"claude","usage":{"input_tokens":3,"output_tokens":2}}}`, cwd),
	)
	claudeFacts, _, err := (claudeCodeJSONLAdapter{}).Parse(ctx, Source{Kind: claudeCodeJSONLSourceKind, Path: claudePath}, SyncOptions{})
	if err != nil || len(claudeFacts) != 1 || claudeFacts[0].Location == nil || claudeFacts[0].Location.DirectoryKey == "" {
		t.Fatalf("Claude directory attribution: facts=%+v err=%v", claudeFacts, err)
	}

	codexPath := filepath.Join(root, "rollout-2026-01-01T00-00-00-codex-session.jsonl")
	writeJSONL(t, codexPath,
		fmt.Sprintf(`{"timestamp":"2026-01-01T00:00:00Z","type":"session_meta","payload":{"id":"codex-session","cwd":%q,"model_provider":"openai","git":{"repository_url":"https://example.com/team/project.git"}}}`, cwd),
		`{"timestamp":"2026-01-01T00:00:01Z","type":"turn_context","payload":{"turn_id":"turn","model":"gpt"}}`,
		`{"timestamp":"2026-01-01T00:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":3,"cached_input_tokens":0,"output_tokens":2,"reasoning_output_tokens":0},"total_token_usage":{"input_tokens":3,"cached_input_tokens":0,"output_tokens":2,"reasoning_output_tokens":0}}}}`,
	)
	codex := &codexJSONLAdapter{}
	codexFacts, _, err := codex.Parse(ctx, Source{Kind: codexSessionJSONLSourceKind, Path: codexPath}, SyncOptions{})
	if err != nil || len(codexFacts) != 1 || codexFacts[0].Location == nil || codexFacts[0].Location.RepositorySource != "harness" {
		t.Fatalf("Codex repository attribution: facts=%+v err=%v", codexFacts, err)
	}
}
