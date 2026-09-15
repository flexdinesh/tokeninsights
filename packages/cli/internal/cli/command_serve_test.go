package cli

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"
)

func TestServeViewerFlagParity(t *testing.T) {
	args := []string{"--week", "--bucket", "week", "--provider", "openai,anthropic", "--model", "m1", "--harness", "pi", "--session-id", "s1", "--filter-day-from", "2026-01-01", "--no-sync"}
	view, err := parseTableOptions(args, io.Discard, false, periodMonth)
	if err != nil {
		t.Fatal(err)
	}
	serve, err := parseServeOptions(append(args, "--port", "8080"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view, serve.viewer) || serve.port != 8080 || serve.resolvePortConflict {
		t.Fatalf("flags diverged: %+v %+v", view, serve)
	}
	for _, port := range []string{"-1", "65536", "bad"} {
		if err := Run(context.Background(), []string{"serve", "--port", port}, io.Discard, io.Discard, time.Now()); err == nil {
			t.Fatalf("accepted port %s", port)
		}
	}
}

func TestServeNetworkDefaults(t *testing.T) {
	options, err := parseServeOptions(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if options.host != "" || options.port != 8765 || !options.resolvePortConflict {
		t.Fatalf("defaults = %+v", options)
	}
	explicit, err := parseServeOptions([]string{"--host", "10.0.1.151", "--port=8765"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if explicit.host != "10.0.1.151" || explicit.resolvePortConflict {
		t.Fatalf("explicit = %+v", explicit)
	}
}
