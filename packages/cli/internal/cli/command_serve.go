package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
)

var serveCommand = commandSpec{name: "serve", run: runServe}

type serveOptions struct {
	viewer              tableOptions
	host                string
	port                int
	resolvePortConflict bool
}

func parseServeOptions(args []string, stderr io.Writer) (serveOptions, error) {
	options := serveOptions{port: server.DefaultPort, resolvePortConflict: true}
	var flags *flag.FlagSet
	viewer, err := parseViewerOptions(args, stderr, false, periodMonth, func(f *flag.FlagSet) {
		flags = f
		f.IntVar(&options.port, "port", server.DefaultPort, "HTTP port (0 chooses an available port)")
		f.StringVar(&options.host, "host", "", "bind IPv4 address (default localhost)")
	})
	if err != nil {
		return serveOptions{}, err
	}
	if options.port < 0 || options.port > 65535 {
		return serveOptions{}, fmt.Errorf("--port must be between 0 and 65535\n%w", ErrUsage)
	}
	if err := server.ValidateHost(options.host); err != nil {
		return serveOptions{}, fmt.Errorf("%v\n%w", err, ErrUsage)
	}
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "port" {
			options.resolvePortConflict = false
		}
	})
	options.viewer = viewer
	return options, nil
}

func runServe(invocation commandInvocation, args []string) error {
	options, err := parseServeOptions(args, invocation.stderr)
	if err != nil {
		return err
	}
	return server.Run(invocation.context, server.Options{
		DBPath:              options.viewer.dbPath,
		NoSync:              options.viewer.noSync,
		Port:                options.port,
		Host:                options.host,
		ResolvePortConflict: options.resolvePortConflict,
		Input:               invocation.stdin,
		Defaults:            selectionFromOptions(options.viewer),
	}, invocation.stdout, invocation.stderr)
}
