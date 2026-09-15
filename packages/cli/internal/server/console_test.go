package server

import (
	"bytes"
	"strings"
	"testing"
)

func TestStartupOutput(t *testing.T) {
	var output bytes.Buffer
	if err := newConsole(&output).startup("workstation", "http://localhost:8765"); err != nil {
		t.Fatal(err)
	}
	want := "  tokeninsights dev\n  hostname:  workstation\n  url:  http://localhost:8765\n\n  ctrl-c to stop.\n"
	if output.String() != want {
		t.Fatalf("startup output = %q, want %q", output.String(), want)
	}
}

func TestTerminationConfirmation(t *testing.T) {
	for _, test := range []struct {
		answer string
		want   bool
	}{
		{"y\n", true},
		{"YES\n", true},
		{"n\n", false},
		{"\n", false},
	} {
		var output bytes.Buffer
		got, err := newConsole(&output).confirmTermination(strings.NewReader(test.answer))
		if err != nil || got != test.want {
			t.Errorf("answer %q: got %t, %v", test.answer, got, err)
		}
		if !strings.HasPrefix(output.String(), logIndent) {
			t.Errorf("prompt missing indent: %q", output.String())
		}
	}
}
