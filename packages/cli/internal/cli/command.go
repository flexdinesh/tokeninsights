package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type commandInvocation struct {
	context context.Context
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
	viewCommand,
	serveCommand,
	syncCommand,
	normalizeCommand,
	resetCanonicalCommand,
	resetAllCommand,
}

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	invocation := commandInvocation{context: ctx, stdout: stdout, stderr: stderr, now: now}
	if len(args) == 0 {
		return viewCommand.run(invocation, nil)
	}

	if command, ok := commandByName(args[0]); ok {
		return command.run(invocation, args[1:])
	}
	if strings.HasPrefix(args[0], "-") {
		return viewCommand.run(invocation, args)
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

var ErrUsage = errors.New("usage: tokeninsights <sync|normalize|reset-canonical|reset-all|view|serve> [options]")
