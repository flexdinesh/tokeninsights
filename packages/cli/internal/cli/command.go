package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type commandInvocation struct {
	context    context.Context
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
	now        time.Time
	settings   *config.Settings
	configPath string
}

type commandHandler func(commandInvocation, []string) error

type commandSpec struct {
	name    string
	aliases []string
	run     commandHandler
}

var commands = []commandSpec{
	helpCommand,
	versionCommand,
	tuiCommand,
	serviceCommand,
	serverCommand,
	syncCommand,
	collectorCommand,
	configCommand,
}

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	invocation := commandInvocation{context: ctx, stdin: os.Stdin, stdout: stdout, stderr: stderr, now: now}
	if len(args) == 5 && args[0] == "__prepare-dev-data" && args[1] == "--collector-db-path" && args[3] == "--server-db-path" {
		return service.PrepareFixture(ctx, args[2], args[4])
	}
	if len(args) > 0 && args[0] == "__service-run" {
		return service.Child(ctx)
	}
	var selectedPath string
	var err error
	args, selectedPath, err = configFileArgument(args)
	if err != nil {
		return err
	}
	invocation.configPath, err = config.Path(selectedPath)
	if err != nil {
		return err
	}
	help := len(args) > 0 && (args[0] == "help" || args[0] == "version")
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--help" || arg == "-h" || arg == "--version" {
			help = true
		}
	}
	if !help && (len(args) == 0 || args[0] != "config") {
		overrides, err := configurationOverrides(args)
		if err != nil {
			return fmt.Errorf("%w\n%w", err, ErrUsage)
		}
		settings, err := config.ResolveWithOverrides(invocation.configPath, true, overrides)
		if err != nil {
			return fmt.Errorf("configuration: %w", err)
		}
		invocation.settings = &settings
		lifecycleRecovery := len(args) > 1 && args[0] == "service" && (args[1] == "stop" || args[1] == "status")
		if os.Getenv("TOKENINSIGHTS_SERVER_TOKEN") != "" && !lifecycleRecovery {
			return fmt.Errorf("TOKENINSIGHTS_SERVER_TOKEN removed; servers are unauthenticated; unset it\n%w", ErrUsage)
		}
	}
	if len(args) == 0 {
		if invocation.defaults().ServerURL != "" {
			_, err := fmt.Fprintln(stdout, "Server: "+invocation.defaults().ServerURL)
			return err
		}
		return runService(invocation, []string{"start"})
	}

	if command, ok := commandByName(args[0]); ok {
		err := command.run(invocation, args[1:])
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.HasPrefix(args[0], "-") {
		return runService(invocation, append([]string{"start"}, args...))
	}
	return fmt.Errorf("unknown command %q\n%w", args[0], ErrUsage)
}

func commandByName(name string) (commandSpec, bool) {
	for _, command := range commands {
		if name == command.name {
			return command, true
		}
		for _, alias := range command.aliases {
			if name == alias {
				return command, true
			}
		}
	}
	return commandSpec{}, false
}

var ErrUsage = errors.New("usage: tokeninsights <service|sync|tui|collector|config> [options]")

func configFileArgument(args []string) ([]string, string, error) {
	result := make([]string, 0, len(args))
	path := ""
	seen := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			result = append(result, args[i:]...)
			break
		}
		if arg == "--config-file" || strings.HasPrefix(arg, "--config-file=") {
			if seen {
				return nil, "", fmt.Errorf("duplicate --config-file\n%w", ErrUsage)
			}
			seen = true
			if arg == "--config-file" {
				i++
				if i >= len(args) {
					return nil, "", fmt.Errorf("--config-file requires a path\n%w", ErrUsage)
				}
				path = args[i]
			} else {
				path = strings.TrimPrefix(arg, "--config-file=")
			}
			if path == "" {
				return nil, "", fmt.Errorf("--config-file requires a path\n%w", ErrUsage)
			}
		} else {
			result = append(result, arg)
		}
	}
	return result, path, nil
}

func (invocation commandInvocation) defaults() config.Settings {
	if invocation.settings != nil {
		return *invocation.settings
	}
	settings := config.Defaults()
	settings.CollectorDBPath, settings.ServerDBPath, settings.ServerURL = defaultCollectorDBPath(), defaultServerDBPath(), defaultServerURL()
	return settings
}

func configurationOverrides(args []string) (config.Values, error) {
	var values config.Values
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			break
		}
		key, value, equals := strings.Cut(args[i], "=")
		switch key {
		case "--server-url", "--host", "--port", "--collector-db-path", "--server-db-path":
			if !equals {
				i++
				if i >= len(args) {
					return values, fmt.Errorf("%s requires a value", key)
				}
				value = args[i]
			}
			switch key {
			case "--server-url":
				values.ServerURL = &value
			case "--host":
				if value == "" {
					value = config.Defaults().Host
				}
				values.Host = &value
			case "--port":
				port, err := strconv.Atoi(value)
				if err != nil {
					return values, fmt.Errorf("invalid port")
				}
				values.Port = &port
			case "--collector-db-path":
				values.CollectorDBPath = &value
			case "--server-db-path":
				values.ServerDBPath = &value
			}
		}
	}
	return values, nil
}
