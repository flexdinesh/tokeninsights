package cli

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestNormalizeRejectsUnknownHarness(t *testing.T) {
	err := Run(context.Background(), []string{"normalize", "--harness", "nope"}, io.Discard, io.Discard, time.Now())
	if err == nil || !strings.Contains(err.Error(), "invalid --harness") {
		t.Fatalf("expected unsupported harness error, got %v", err)
	}
}
