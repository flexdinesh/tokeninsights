package cli

import (
	"context"
	"flag"
	"fmt"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
)

var dataCommand = commandSpec{name: "data", run: runData}

func runData(invocation commandInvocation, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("data requires reprocess|wait\n%w", ErrUsage)
	}
	action := args[0]
	if action != "reprocess" && action != "wait" {
		return ErrUsage
	}
	defaults := invocation.defaults()
	flags := flag.NewFlagSet("tokeninsights data "+action, flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	flags.String("server-db-path", defaults.ServerDBPath, "token database")
	flags.String("collector-db-path", defaults.CollectorDBPath, "collector database")
	flags.String("app-db-path", defaults.AppDBPath, "application database")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return ErrUsage
	}
	settings, err := invocation.resolveLocal(flags)
	if err != nil {
		return err
	}
	runtime, err := localruntime.OpenWithApp(invocation.context, settings.CollectorDBPath, settings.ServerDBPath, settings.ApplicationPath())
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	if action == "reprocess" {
		if _, err := runtime.Store.Reprocess(invocation.context); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(invocation.context, localVisibilityTimeout)
	defer cancel()
	return runtime.WaitVisible(ctx)
}
