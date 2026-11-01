package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRequiresKnownGroupAndAttempt(t *testing.T) {
	for _, group := range []string{"contracts", "javascript", "go", "benchmarks", "distribution", "browser", "deployment"} {
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
