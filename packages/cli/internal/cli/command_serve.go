package cli

import (
	"flag"
	"fmt"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
)

var serveCommand = commandSpec{name: "serve", run: runServe}

func runServe(invocation commandInvocation, args []string) error {
	port := server.DefaultPort
	var host string
	o, err := parseViewerOptions(args, invocation.stderr, false, periodMonth, func(f *flag.FlagSet) {
		f.IntVar(&port, "port", server.DefaultPort, "HTTP port (0 chooses an available port)")
		f.StringVar(&host, "host", server.DefaultHost, "bind IPv4 address")
	})
	if err != nil {
		return err
	}
	if port < 0 || port > 65535 {
		return fmt.Errorf("--port must be between 0 and 65535\n%w", ErrUsage)
	}
	if err := server.ValidateHost(host); err != nil {
		return fmt.Errorf("%v\n%w", err, ErrUsage)
	}
	return server.Run(invocation.context, server.Options{DBPath: o.dbPath, NoSync: o.noSync, Port: port, Host: host, Defaults: selectionFromOptions(o)}, invocation.stdout, invocation.stderr)
}
