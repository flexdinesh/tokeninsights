package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

func selectionFromOptions(o tableOptions) viewer.Selection {
	return viewer.Selection{Period: string(o.period), Bucket: string(o.bucket), From: o.filters.dayFrom, To: o.filters.dayTo,
		Providers: append([]string{}, o.filters.providers...), Models: append([]string{}, o.filters.models...),
		Harnesses: append([]string{}, o.filters.harnesses...), Sessions: append([]string{}, o.filters.sessionIDs...)}
}

func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	port := server.DefaultPort
	var host string
	o, err := parseViewerOptions(args, stderr, false, periodMonth, func(f *flag.FlagSet) {
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
	return server.Run(ctx, server.Options{DBPath: o.dbPath, NoSync: o.noSync, Port: port, Host: host, Defaults: selectionFromOptions(o)}, stdout, stderr)
}
