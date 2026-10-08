package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"io"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

var serviceCommand = commandSpec{name: "service", run: runService}

type serviceOptions struct {
	service.Options
	open, json bool
	legacyPath string
}

func parseServiceOptions(action string, args []string, output io.Writer) (serviceOptions, error) {
	return parseServiceOptionsWithDefaults(action, args, output, (commandInvocation{}).defaults())
}

func parseServiceOptionsWithDefaults(action string, args []string, output io.Writer, settings config.Settings) (serviceOptions, error) {
	options := serviceOptions{Options: localOptions(settings.ServerDBPath, settings)}
	flags := flag.NewFlagSet("tokeninsights service "+action, flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&options.DBPath, "server-db-path", settings.ServerDBPath, "path to server DuckDB")
	if action == "import" {
		flags.StringVar(&options.legacyPath, "legacy-server-db-path", "", "verified SQLite baseline to import read-only into a new DuckDB")
	}
	var host string
	var port int
	if action == "start" || action == "restart" || action == "run" {
		flags.StringVar(&host, "host", server.DefaultHost, "web/API bind IPv4 address")
		flags.IntVar(&port, "port", server.DefaultPort, "web/API port (0 chooses available)")
	}
	if action == "start" {
		flags.BoolVar(&options.open, "open", false, "open dashboard in browser")
	}
	if action == "status" {
		flags.BoolVar(&options.json, "json", false, "print JSON status")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options, err
		}
		return options, fmt.Errorf("%v; viewer flags require tokeninsights tui\n%w", err, ErrUsage)
	}
	if flags.NArg() != 0 {
		return options, fmt.Errorf("unexpected argument\n%w", ErrUsage)
	}
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "host" {
			options.Host = &host
		}
		if f.Name == "port" {
			options.Port = &port
		}

	})
	if err := server.ValidateHost(host); err != nil {
		return options, fmt.Errorf("%v\n%w", err, ErrUsage)
	}
	if port < 0 || port > 65535 {
		return options, fmt.Errorf("invalid --port\n%w", ErrUsage)
	}
	return options, nil
}

func printServiceStatus(out io.Writer, state service.State, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(out).Encode(state)
	}
	if !state.Running {
		_, err := fmt.Fprintln(out, "Service stopped.")
		return err
	}
	_, err := fmt.Fprintf(out, "Service running: %s (PID %d, bind %s)\n", state.Record.URL, state.Record.PID, state.Record.Address)
	return err
}

// service remains only to stop/probe owners created by older releases.
func runService(invocation commandInvocation, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use service stop|status for migration; web/tui own their runtime\n%w", ErrUsage)
	}
	action := args[0]
	if action == "--help" || action == "-h" {
		_, err := fmt.Fprintln(invocation.stdout, "usage: tokeninsights service <stop|status> [--server-db-path PATH] (legacy migration)")
		return err
	}
	if action != "stop" && action != "status" {
		return fmt.Errorf("managed services retired; use web, tui, or data maintenance\n%w", ErrUsage)
	}
	options, err := parseServiceOptionsWithDefaults(action, args[1:], invocation.stderr, invocation.defaults())
	if err != nil {
		return err
	}
	if action == "stop" {
		if err := service.Stop(invocation.context, options.DBPath); err != nil {
			return err
		}
		_, err := fmt.Fprintln(invocation.stdout, "Legacy service stopped.")
		return err
	}
	state, err := service.Probe(invocation.context, options.DBPath)
	if err != nil {
		return err
	}
	if err := printServiceStatus(invocation.stdout, state, options.json); err != nil {
		return err
	}
	if !state.Running {
		return service.ErrStopped
	}
	return nil
}
