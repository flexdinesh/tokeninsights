package deployment_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
)

// TestSSHEndpointDeployment runs against a real OpenSSH server whose remote
// user has the CLI on its default non-interactive PATH.
func TestSSHEndpointDeployment(t *testing.T) {
	target := os.Getenv("TOKENINSIGHTS_TEST_SSH_ENDPOINT")
	if target == "" {
		t.Skip("run isolated deployment validation")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	selected, err := endpoint.Parse(target)
	if err != nil || selected.IsLocal() {
		t.Fatal("invalid SSH fixture endpoint", target, err)
	}
	local, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	// Runner storage must stay untouched by remote sessions.
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("TOKENINSIGHTS_CONFIG_PATH", filepath.Join(root, "config.json"))
	session, err := localclient.ConnectRemote(ctx, selected.SSH.Dialer(""), localclient.RemoteOptions{Capture: true})
	if err != nil {
		t.Fatal("real SSH endpoint rejected", err)
	}
	defer func() { _ = session.Close() }()
	instance, err := session.Query.Instance(ctx)
	if err != nil || instance.Hostname == "" || instance.Hostname == local {
		t.Fatal("remote instance did not describe the remote machine", instance.Hostname, local, err)
	}
	if _, err := session.Collect(ctx, false, ""); err != nil {
		t.Fatal("remote capture failed", err)
	}
	if err := session.WaitVisible(ctx); err != nil {
		t.Fatal(err)
	}

	cli := filepath.Join(t.TempDir(), "tokeninsights")
	build := exec.CommandContext(ctx, "go", "build", "-o", cli, "../../cli/cmd/tokeninsights")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build cli: %v\n%s", err, output)
	}
	run := func(command string, args ...string) (string, error) {
		t.Helper()
		process := exec.CommandContext(ctx, cli, args...)
		process.Env = append(os.Environ(), "TOKENINSIGHTS_REMOTE_COMMAND="+command)
		var output bytes.Buffer
		process.Stdout, process.Stderr = &output, &output
		err := process.Run()
		return output.String(), err
	}
	// The attached session keeps the remote owner; finite commands share it.
	if output, err := run(endpoint.DefaultCommand, "data", "wait", "--endpoint", target); err != nil {
		t.Fatalf("remote data wait through PATH failed: %v\n%s", err, output)
	}
	if output, err := run("/opt/tokeninsights/tokeninsights", "data", "reprocess", "--endpoint", target); err != nil {
		t.Fatalf("remote data reprocess through remote.command failed: %v\n%s", err, output)
	}
	output, err := run("/opt/absent/tokeninsights", "data", "wait", "--endpoint", target)
	if err == nil || !strings.Contains(output, "/opt/absent/tokeninsights") {
		t.Fatalf("missing remote command lost diagnostics: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "tokeninsights")); !os.IsNotExist(err) {
		t.Fatal("remote endpoint created local storage", err)
	}
	if _, err := session.Query.Status(ctx); err != nil {
		t.Fatal("finite remote commands stopped the attached viewer", err)
	}
}
