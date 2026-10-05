package cli

import (
	"fmt"
	"path/filepath"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
)

var configCommand = commandSpec{name: "config", run: runConfig}

func runConfig(invocation commandInvocation, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := fmt.Fprintln(invocation.stdout, "usage: tokeninsights config set KEY VALUE | get KEY | remove KEY\nKeys: server-url, host, port, collector-db-path, server-db-path\nget reads stored preferences/defaults. Runtime precedence: flags > environment > file > defaults.")
		return err
	}
	if len(args) < 2 {
		return fmt.Errorf("usage: tokeninsights config set KEY VALUE | get KEY | remove KEY\n%w", ErrUsage)
	}
	path := invocation.configPath
	if path == "" {
		var err error
		path, err = config.Path("")
		if err != nil {
			return err
		}
	}
	action, key := args[0], args[1]
	if action == "get" && len(args) == 2 {
		values, err := config.Read(path)
		if err != nil {
			return err
		}
		value, err := values.Get(key, filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("%w\n%w", err, ErrUsage)
		}
		_, err = fmt.Fprintln(invocation.stdout, value)
		return err
	}
	if action != "set" && action != "remove" || action == "set" && len(args) != 3 || action == "remove" && len(args) != 2 {
		return fmt.Errorf("usage: tokeninsights config set KEY VALUE | get KEY | remove KEY\n%w", ErrUsage)
	}
	value := ""
	if action == "set" {
		value = args[2]
	}
	if err := config.Update(invocation.context, path, func(values *config.Values) error { return values.Set(key, value, action == "remove") }); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	return nil
}
