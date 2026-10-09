package pipeline

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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
