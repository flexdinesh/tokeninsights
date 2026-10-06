package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/remoteserver"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("tokeninsights-server", flag.ContinueOnError)
	flags.SetOutput(stderr)
	settings := remoteserver.Settings{}
	flags.StringVar(&settings.Listen, "listen", "0.0.0.0:8765", "IPv4 listen address")
	flags.StringVar(&settings.DBPath, "server-db-path", "", "required server DuckDB database path")
	flags.StringVar(&settings.LegacyDBPath, "legacy-server-db-path", "", "read-only SQLite baseline import into a new DuckDB")
	showVersion := flags.Bool("version", false, "print version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		_, err := fmt.Fprintln(stdout, "tokeninsights-server "+version.Version)
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if os.Getenv("TOKENINSIGHTS_SERVER_TOKEN") != "" {
		return fmt.Errorf("TOKENINSIGHTS_SERVER_TOKEN removed; server is unauthenticated; unset it")
	}
	return remoteserver.Run(ctx, settings, stderr, func(url string) error { _, err := fmt.Fprintln(stdout, url); return err })
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
