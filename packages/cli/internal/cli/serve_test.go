package cli

import (
	"context"
	"flag"
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
	port := 0
	serve, err := parseViewerOptions(append(args, "--port", "8080"), io.Discard, false, periodMonth, func(f *flag.FlagSet) { f.IntVar(&port, "port", 8765, "port") })
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view, serve) || port != 8080 {
		t.Fatalf("flags diverged: %+v %+v %d", view, serve, port)
	}
	for _, port := range []string{"-1", "65536", "bad"} {
		if err := Run(context.Background(), []string{"serve", "--port", port}, io.Discard, io.Discard, time.Now()); err == nil {
			t.Fatalf("accepted port %s", port)
		}
	}
}
