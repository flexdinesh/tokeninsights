package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
)

var configCommand = commandSpec{name: "config", run: runConfig}

func runConfig(invocation commandInvocation, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := fmt.Fprintln(invocation.stdout, "usage: tokeninsights config set KEY VALUE | get KEY | remove KEY\nKeys: mode, server-url, server-token, host, port, collector-db-path, server-db-path, app-db-path\nSet server-token without VALUE to read securely from terminal/stdin. get never prints tokens.\nRuntime precedence: flags > environment > file > defaults.")
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
	secret := action == "set" && key == "server-token" && len(args) == 2
	if action != "set" && action != "remove" || action == "set" && len(args) != 3 && !secret || action == "remove" && len(args) != 2 {
		return fmt.Errorf("usage: tokeninsights config set KEY VALUE | get KEY | remove KEY\n%w", ErrUsage)
	}
	value := ""
	if secret {
		var err error
		value, err = readConfigToken(invocation)
		if err != nil {
			return err
		}
	} else if action == "set" {
		if key == "server-token" {
			return fmt.Errorf("set server-token without VALUE; token is read from terminal/stdin\n%w", ErrUsage)
		}
		value = args[2]
	}
	if err := config.Update(invocation.context, path, func(values *config.Values) error { return values.Set(key, value, action == "remove") }); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	return nil
}

func readConfigToken(invocation commandInvocation) (string, error) {
	input := invocation.stdin
	if input == nil {
		input = os.Stdin
	}
	if file, ok := input.(*os.File); ok && term.IsTerminal(file.Fd()) {
		_, _ = fmt.Fprint(invocation.stderr, "Server token: ")
		value, err := term.ReadPassword(file.Fd())
		_, _ = fmt.Fprintln(invocation.stderr)
		if err != nil {
			return "", fmt.Errorf("cannot read server token")
		}
		return string(value), nil
	}
	const maxTokenBytes = 4096
	line, err := bufio.NewReader(io.LimitReader(input, maxTokenBytes+1)).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("cannot read server token")
	}
	if len(line) > maxTokenBytes {
		return "", fmt.Errorf("server token too long")
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}
