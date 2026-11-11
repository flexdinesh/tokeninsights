package cli

import (
	"fmt"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
)

var connectCommand = commandSpec{name: "connect", run: runConnect}
var launchDesktop = desktoplaunch.Launch

func runConnect(invocation commandInvocation, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("connect requires a connection URI\n%w", ErrUsage)
	}
	connection := args[0]
	return runDesktop(invocation, args[1:], &connection)
}

func runDesktop(invocation commandInvocation, args []string, connection *string) error {
	flags, capture, full, source := viewerFlags(invocation, "tokeninsights")
	appName := flags.String("app", string(browser.ModeDesktop), "desktop application: desktop or native")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() != 0 {
		return ErrUsage
	}
	app, err := browser.AppFor(browser.Mode(*appName))
	if err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if !*capture && *full {
		return fmt.Errorf("--full-refresh requires --capture\n%w", ErrUsage)
	}
	overrides, err := config.FlagOverrides(flags)
	if err != nil {
		return err
	}
	if connection != nil {
		if overrides.Connection != nil {
			return fmt.Errorf("connect URI conflicts with --connection\n%w", ErrUsage)
		}
		overrides.Connection = connection
	}
	settings, err := invocation.resolveLocalOverrides(overrides)
	if err != nil {
		return err
	}
	return launchDesktop(invocation.context, app, desktoplaunch.Specification(settings, *capture, *full, *source))
}
