package server

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
)

func TestParsePortProcesses(t *testing.T) {
	output := "p123\ncnode\nn127.0.0.1:8765\np456\ncgo\nn10.0.1.151:8765\np789\ncwildcard\nn*:8765\n"
	got := parsePortProcesses(output, "")
	if len(got) != 2 || got[0] != (portProcess{pid: 123, name: "node"}) || got[1] != (portProcess{pid: 789, name: "wildcard"}) {
		t.Fatalf("processes = %+v", got)
	}
	if got := parsePortProcesses(output, "0.0.0.0"); len(got) != 3 {
		t.Fatalf("wildcard processes = %+v", got)
	}
}

func TestDefaultPortConflictCanTerminateAndRetry(t *testing.T) {
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(occupied.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		t.Fatal(err)
	}
	options := Options{Port: port, ResolvePortConflict: true, Input: strings.NewReader("yes\n")}
	var output bytes.Buffer
	listener, err := acquireListenerWith(context.Background(), options, &output, listen,
		func(context.Context, string, int) ([]portProcess, error) {
			return []portProcess{{pid: 42, name: "other-server"}}, nil
		},
		func(portProcess) error { return occupied.Close() },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	if !strings.Contains(output.String(), "port ") || !strings.Contains(output.String(), "other-server, pid 42") {
		t.Fatalf("missing conflict details: %q", output.String())
	}
}

func TestExplicitPortConflictDoesNotPrompt(t *testing.T) {
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = occupied.Close() }()
	_, portText, err := net.SplitHostPort(occupied.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	listener, err := acquireListener(context.Background(), Options{Port: port}, &output)
	if listener != nil || err == nil || output.Len() != 0 {
		t.Fatalf("listener=%v err=%v output=%q", listener, err, output.String())
	}
}
