package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	_ "modernc.org/sqlite"
	"tokeninsights-cli/internal/cli"
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
