package cli

import (
	"errors"
	"fmt"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/tui"
)

var tuiCommand = commandSpec{name: "tui", run: runView}

var runTerminalDashboard = tui.Run

func runView(invocation commandInvocation, args []string) error {
	err := runTerminalDashboard(tui.Command{Context: invocation.context, Args: args, Settings: invocation.defaults(), Now: invocation.now, Stdout: invocation.stdout, Stderr: invocation.stderr})
	var usage *tui.UsageError
	if errors.As(err, &usage) {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	return err
}
