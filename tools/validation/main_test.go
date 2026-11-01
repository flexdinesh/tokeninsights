package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRequiresKnownGroupAndAttempt(t *testing.T) {
	for _, group := range []string{"static", "web", "tooling", "go", "benchmarks", "distribution", "browser", "deployment"} {
		if err := validateCheck(group, "attempt"); err != nil {
			t.Fatal(err)
		}
		if err := validateCheck(group, ""); err == nil {
			t.Fatalf("%s accepted missing attempt", group)
		}
	}
	if err := validateCheck("unexpected", "attempt"); err == nil {
		t.Fatal("accepted unknown group")
	}
}

func TestCapturePreservesFailureAndArtifacts(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "evidence")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "mise"), []byte("#!/bin/sh\necho diagnostic\nexit 23\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\necho generated-diff\n"), 0755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(root, "packages/web/test-results")
	if err := os.MkdirAll(artifact, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifact, "trace.zip"), []byte("trace"), 0644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", "-c", strings.ReplaceAll(captureScript, "/evidence", evidence), "capture", "browser")
	command.Dir = root
	command.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", output, err)
	}
	for path, want := range map[string]string{"status.json": "{\"exitCode\":23}\n", "check.log": "diagnostic\n", "artifacts/test-results/trace.zip": "trace", "artifacts/generated.diff": "generated-diff\n"} {
		data, err := os.ReadFile(filepath.Join(evidence, path))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Fatalf("%s: %q", path, data)
		}
	}
}

func TestWhitespaceAttemptRejected(t *testing.T) {
	// A missing invocation identity must never start a mutable service.
	if err := validateCheck("go", " "); err == nil {
		t.Fatal("whitespace attempt accepted")
	}
}

func TestGroupManifestCoverage(t *testing.T) {
	data, err := os.ReadFile("groups.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Common []string
		Groups map[string]struct {
			Inputs   []string
			Executor string
			Class    string
		}
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Groups) != 9 {
		t.Fatalf("expected nine groups: %v", manifest.Groups)
	}
	matches := func(path string, patterns []string) bool {
		for _, pattern := range patterns {
			if strings.HasSuffix(pattern, "/") && strings.HasPrefix(path, pattern) {
				return true
			}
			if ok, _ := filepath.Match(pattern, path); ok {
				return true
			}
		}
		return false
	}
	for name, group := range manifest.Groups {
		patterns := append(append([]string{}, manifest.Common...), group.Inputs...)
		if !matches("tools/validation/contract.go", patterns) {
			t.Errorf("%s misses implementation", name)
		}
		if name == "format" {
			if group.Executor != "host" {
				t.Fatal("format must run on host")
			}
			for _, path := range []string{"packages/cli/internal/server/server.go", "tools/build/src/build-web.ts", "docs/design.md", ".github/workflows/ci.yml"} {
				if !matches(path, patterns) {
					t.Errorf("format misses %s", path)
				}
			}
		} else {
			if err := validateCheck(name, "attempt"); err != nil {
				t.Fatal(err)
			}
			if group.Executor != "dagger" {
				t.Errorf("%s must run in Dagger", name)
			}
			if matches("README.md", patterns) {
				t.Errorf("%s invalidated by root README", name)
			}
		}
	}
}
