package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/cli"
	_ "modernc.org/sqlite"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, cli.ErrUsage) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	return runWithTime(ctx, args, stdout, stderr, time.Now())
}

func runWithTime(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	return cli.Run(ctx, args, stdout, stderr, now)
}
