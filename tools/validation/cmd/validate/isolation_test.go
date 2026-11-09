package main

import (
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestComposeIsolation runs the real compose.yaml against a real Docker
// engine: concurrent projects stay isolated, host listeners stay unreachable,
// and tearing down one project leaves the other running. It uses slots 8 and
// 9, outside the admission range, and the pinned base image without building.
func TestComposeIsolation(t *testing.T) {
	if os.Getenv("TOKENINSIGHTS_VALIDATION_DOCKER_TEST") != "1" {
		t.Skip("set TOKENINSIGHTS_VALIDATION_DOCKER_TEST=1 (mise run check:isolation)")
	}
	tree, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	dockerfile, err := os.ReadFile(filepath.Join(tree, "tools", "validation", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^FROM (\S+) AS base$`).FindSubmatch(dockerfile)
	if match == nil {
		t.Fatal("base image not found")
	}
	base := string(match[1])
	r := runner{ctx: t.Context(), root: tree, home: t.TempDir(), docker: "docker", env: dockerEnvironment()}
	workdir := t.TempDir()
	docker := func(args ...string) (string, error) {
		out, err := r.command(workdir, r.docker, args...)
		return strings.TrimSpace(string(out)), err
	}
	if _, err := docker("image", "inspect", base); err != nil {
		if _, err := docker("pull", "--platform", platform, base); err != nil {
			t.Fatal(err)
		}
	}
	if err := ensureVolumes(r, workdir); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{}, 16)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			accepted <- struct{}{}
			conn.Close()
		}
	}()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	contexts := map[string]string{"empty": empty, groupContext: empty}
	for _, name := range sharedContexts {
		contexts[name] = empty
	}
	start := func(slot int) containerRun {
		run := containerRun{r: r, slot: slot, group: "probe", tree: tree, contexts: contexts, logs: t.TempDir(), workdir: workdir}
		t.Cleanup(func() { _ = run.teardown(t.Context()) })
		if _, err := docker("tag", base, imageTag(slot, "probe")); err != nil {
			t.Fatal(err)
		}
		if err := run.compose(t.Context(), "run.log", "run", "--detach", "-T", "--no-deps", "--name", run.runnerName(), "runner", "sleep", "600"); err != nil {
			t.Fatal(err)
		}
		return run
	}
	first, second := start(8), start(9)
	exec := func(run containerRun, script string) error {
		_, err := docker("exec", run.runnerName(), "bash", "-c", script)
		return err
	}
	// One-off run containers resolve by container name on the project network.
	if err := exec(first, "getent hosts "+first.runnerName()); err != nil {
		t.Fatal("project DNS unavailable, so later lookups prove nothing", err)
	}
	if err := exec(first, "getent hosts "+second.runnerName()); err == nil {
		t.Fatal("project resolved another project's runner")
	}
	networks := func(run containerRun) string {
		out, err := docker("inspect", "--format", "{{range $name, $_ := .NetworkSettings.Networks}}{{$name}} {{end}}", run.runnerName())
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if a, b := networks(first), networks(second); a == "" || a == b || strings.Contains(b, a) {
		t.Fatalf("projects share networks: %q %q", a, b)
	}
	subnet, err := docker("network", "inspect", "--format", "{{range .IPAM.Config}}{{.Subnet}}{{end}}", first.project()+"_default")
	if err != nil {
		t.Fatal(err)
	}
	// Every host address, the bridge's would-be gateway and loopback.
	hosts := []string{"127.0.0.1"}
	if prefix, err := netip.ParsePrefix(subnet); err == nil {
		hosts = append(hosts, prefix.Masked().Addr().Next().String())
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addrs {
		if prefix, err := netip.ParsePrefix(addr.String()); err == nil && prefix.Addr().Is4() && !prefix.Addr().IsLoopback() {
			hosts = append(hosts, prefix.Addr().String())
		}
	}
	for _, host := range hosts {
		if err := exec(first, "timeout 3 bash -c 'exec 3<>/dev/tcp/"+host+"/"+port+"'"); err == nil {
			t.Fatalf("container reached host listener via %s", host)
		}
	}
	select {
	case <-accepted:
		t.Fatal("host listener accepted a container connection")
	default:
	}
	if err := first.teardown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if out, err := docker("container", "ls", "--all", "--quiet", "--filter", "label="+composeLabel+"="+first.project()); err != nil || out != "" {
		t.Fatal("torn-down project left containers", out, err)
	}
	if out, err := docker("network", "ls", "--quiet", "--filter", "label="+composeLabel+"="+first.project()); err != nil || out != "" {
		t.Fatal("torn-down project left networks", out, err)
	}
	if out, err := docker("inspect", "--format", "{{.State.Running}}", second.runnerName()); err != nil || out != "true" {
		t.Fatal("teardown stopped the other project", out, err)
	}
	if err := exec(second, "getent hosts "+second.runnerName()); err != nil {
		t.Fatal("other project's network broke", err)
	}
}
