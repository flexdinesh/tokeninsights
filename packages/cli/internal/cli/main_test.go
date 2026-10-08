package cli

import (
	"context"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "__sync-run" {
		if err := syncjob.Child(context.Background()); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
