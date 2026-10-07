package cli

import "fmt"

var collectorCommand = commandSpec{name: "collector", run: runCollector}

func runCollector(invocation commandInvocation, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(invocation.stdout, collectorUsageText())
		return err
	}
	for _, command := range []commandSpec{normalizeCommand, resetCanonicalCommand, resetAllCommand} {
		if command.name == args[0] {
			return command.run(invocation, args[1:])
		}
	}
	return fmt.Errorf("unknown collector command %q\n%s\n%w", args[0], collectorUsageText(), ErrUsage)
}

func collectorUsageText() string {
	return `usage: tokeninsights collector <command> [options]

Advanced host collector operations:
  normalize         normalize legacy local facts; publish on the next sync
  reset-canonical   reset rebuildable collector canonical facts
  reset-all         reset collector storage and delivery state

  tokeninsights collector normalize --collector-db-path collector.sqlite
  tokeninsights collector reset-canonical --confirm
  tokeninsights collector reset-all --confirm

New evidence is processed on server; use service reprocess.\nAll operations affect collector storage. Server history remains available.`
}
