package cli

import "fmt"

var serveCommand = commandSpec{name: "serve", run: runServe}

func runServe(invocation commandInvocation, args []string) error {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--no-sync" {
			_, _ = fmt.Fprintln(invocation.stderr, "serve --no-sync is obsolete; service startup never syncs")
			continue
		}
		filtered = append(filtered, arg)
	}
	_, _ = fmt.Fprintln(invocation.stderr, "serve is deprecated; use tokeninsights service run")
	return runService(invocation, append([]string{"run"}, filtered...))
}
