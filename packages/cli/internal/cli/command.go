package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
	"io"
	"os"
	"strings"
	"time"
)

type commandInvocation struct {
	context context.Context
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	now     time.Time
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
	serveCommand,
	serviceCommand,
	serverCommand,
	syncCommand,
	collectorCommand,
	normalizeCommand,
	resetCanonicalCommand,
	resetAllCommand,
}

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	invocation := commandInvocation{context: ctx, stdin: os.Stdin, stdout: stdout, stderr: stderr, now: now}
	if len(args) == 5 && args[0] == "__prepare-dev-data" && args[1] == "--collector-db-path" && args[3] == "--server-db-path" {
		return service.PrepareFixture(ctx, args[2], args[4])
	}
	if len(args) > 0 && args[0] == "__service-run" {
		return service.Child(ctx)
	}
	if len(args) == 0 {
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

var ErrUsage = errors.New("usage: tokeninsights <service|sync|tui|collector|server> [options]")
