package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"io"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/browser"
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

func runService(invocation commandInvocation, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("service requires start, stop, restart, status, or run\n%w", ErrUsage)
	}
	action := args[0]
	if action == "--help" || action == "-h" {
		_, err := fmt.Fprintln(invocation.stdout, "usage: tokeninsights service <start|stop|restart|status|run> [options]")
		return err
	}
	switch action {
	case "start", "stop", "restart", "status", "run", "reprocess", "wait", "import":
	default:
		return fmt.Errorf("unknown service action %q\n%w", action, ErrUsage)
	}
	options, err := parseServiceOptionsWithDefaults(action, args[1:], invocation.stderr, invocation.defaults())
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if action == "restart" && invocation.configPath != "" {
		host, port, legacy, err := service.LegacyBind(options.DBPath)
		if err != nil {
			return err
		}
		if legacy {
			if err := config.Update(invocation.context, invocation.configPath, func(values *config.Values) error {
				if values.Host == nil {
					values.Host = &host
				}
				if values.Port == nil {
					values.Port = &port
				}
				return values.Validate()
			}); err != nil {
				return err
			}
			overrides, err := configurationOverrides(args[1:])
			if err != nil {
				return err
			}
			settings, err := config.ResolveWithOverrides(invocation.configPath, true, overrides)
			if err != nil {
				return err
			}
			options.DefaultHost, options.DefaultPort = &settings.Host, &settings.Port
		}
	}
	var state service.State
	switch action {
	case "import":
		if err := service.ImportLegacy(invocation.context, options.DBPath, options.legacyPath); err != nil {
			return err
		}
		_, err := fmt.Fprintln(invocation.stdout, "Legacy history imported. SQLite source preserved.")
		return err
	case "wait", "reprocess":
		state, err = service.Probe(invocation.context, options.DBPath)
		if err != nil {
			return err
		}
		if !state.Running || state.Record == nil {
			return service.ErrStopped
		}
		client := service.Client{Record: *state.Record}
		if action == "reprocess" {
			generation, err := client.Reprocess(invocation.context)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(invocation.stdout, "Reprocessing queued: generation=%d\n", generation)
			return err
		}
		ctx, cancel := context.WithTimeout(invocation.context, 30*time.Second)
		defer cancel()
		return client.WaitProcessing(ctx)
	case "start":
		state, err = service.Ensure(invocation.context, options.Options)
	case "restart":
		state, err = service.Restart(invocation.context, options.Options)
	case "stop":
		if err := service.Stop(invocation.context, options.DBPath); err != nil {
			return err
		}
		_, err := fmt.Fprintln(invocation.stdout, "Service stopped.")
		return err
	case "status":
		state, err = service.Probe(invocation.context, options.DBPath)
	case "run":
		return service.Run(invocation.context, options.Options, invocation.stdout, invocation.stderr)
	}
	if err != nil {
		return err
	}
	if action == "restart" {
		_, _ = fmt.Fprintln(invocation.stderr, "Local public API is unauthenticated after restart.")
	}
	if err := printServiceStatus(invocation.stdout, state, options.json); err != nil {
		return err
	}
	if action == "status" && !state.Running {
		return service.ErrStopped
	}
	if options.open && state.Record != nil {
		if err := browser.Open(state.Record.URL); err != nil {
			_, _ = fmt.Fprintf(invocation.stderr, "Could not open browser: %v; open %s\n", err, state.Record.URL)
		}
	}
	return nil
}
