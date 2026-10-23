package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dbpath"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
)

type commandInvocation struct {
	context    context.Context
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
	now        time.Time
	configPath string
	// applicationPath records the resolved local application database for
	// incompatible-schema guidance after the command returns.
	applicationPath *string
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
	webCommand,
	browseCommand,
	syncCommand,
	dataCommand,
	configCommand,
}

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	invocation := commandInvocation{context: ctx, stdin: os.Stdin, stdout: stdout, stderr: stderr, now: now, applicationPath: new(string)}
	if len(args) == 5 && args[0] == "__prepare-dev-data" && args[1] == "--collector-db-path" && args[3] == "--server-db-path" {
		return localruntime.PrepareFixture(ctx, args[2], args[4])
	}
	if len(args) == 7 && args[0] == "__capture-dev-data" && args[1] == "--collector-db-path" && args[3] == "--server-db-path" && args[5] == "--source-dir" {
		return localruntime.CaptureFixture(ctx, args[2], args[4], args[6])
	}
	if len(args) == 1 && args[0] == "__sync-run" {
		return syncjob.Child(ctx)
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
	if len(args) > 0 {
		if _, ok := commandByName(args[0]); !ok {
			if strings.HasPrefix(args[0], "-") {
				return fmt.Errorf("choose tokeninsights tui or tokeninsights web\n%w", ErrUsage)
			}
			return fmt.Errorf("unknown command %q\n%w", args[0], ErrUsage)
		}
	}
	if len(args) == 0 {
		return runHelp(invocation, nil)
	}

	if command, ok := commandByName(args[0]); ok {
		err := command.run(invocation, args[1:])
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		if errors.Is(err, appstore.ErrIncompatibleDatabase) && *invocation.applicationPath != "" {
			path, pathErr := dbpath.Canonical(*invocation.applicationPath)
			if pathErr != nil {
				return err
			}
			return fmt.Errorf("%w: the local application database has an unsupported schema version or database role. "+
				"This binary supports application schema version %d; automatic migrations are unavailable.\n"+
				"Config: %s\nApplication database: %s\n"+
				"Close TokenInsights processes using this database, then delete the application database above and its -wal and -shm files if present. "+
				"Rerun the command to recreate it. This resets local accounts and credentials; token usage remains in the separate server database. "+
				"Do not delete the config file", err, appstore.SchemaVersion, invocation.configPath, path)
		}
		return err
	}
	if strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("choose tokeninsights tui or tokeninsights web\n%w", ErrUsage)
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

var ErrUsage = errors.New("usage: tokeninsights <sync|browse|tui|web|data|config> [options]")

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

// defaults supplies flag help values; resolveLocal applies the config file.
func (invocation commandInvocation) defaults() config.LocalSettings {
	return config.EnvironmentDefaults()
}

// resolveLocal resolves local settings after the command parsed its flags.
func (invocation commandInvocation) resolveLocal(flags *flag.FlagSet) (config.LocalSettings, error) {
	overrides, err := config.FlagOverrides(flags)
	if err != nil {
		return config.LocalSettings{}, err
	}
	return invocation.resolveLocalOverrides(overrides)
}

func (invocation commandInvocation) resolveLocalOverrides(overrides config.Overrides) (config.LocalSettings, error) {
	settings, err := config.ResolveLocal(invocation.configPath, true, overrides)
	if err != nil {
		return settings, fmt.Errorf("configuration: %w", err)
	}
	if invocation.applicationPath != nil {
		*invocation.applicationPath = settings.ApplicationPath()
	}
	return settings, nil
}

func (invocation commandInvocation) resolveSync(flags *flag.FlagSet) (config.SyncSettings, error) {
	overrides, err := config.FlagOverrides(flags)
	if err != nil {
		return config.SyncSettings{}, err
	}
	settings, err := config.ResolveSync(invocation.configPath, true, overrides)
	if err != nil {
		return settings, fmt.Errorf("configuration: %w", err)
	}
	return settings, nil
}

func (invocation commandInvocation) resolveBrowse(flags *flag.FlagSet) (config.BrowseSettings, error) {
	overrides, err := config.FlagOverrides(flags)
	if err != nil {
		return config.BrowseSettings{}, err
	}
	settings, err := config.ResolveBrowse(invocation.configPath, true, overrides)
	if err != nil {
		return settings, fmt.Errorf("configuration: %w", err)
	}
	return settings, nil
}

func (invocation commandInvocation) syncDefaults() config.SyncSettings {
	return config.SyncSettings{CollectorDBPath: config.EnvironmentDefaults().CollectorDBPath}
}
