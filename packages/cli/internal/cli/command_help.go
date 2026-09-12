package cli

import "fmt"

var helpCommand = commandSpec{name: "help", aliases: []string{"--help", "-h"}, run: runHelp}

func runHelp(invocation commandInvocation, _ []string) error {
	_, err := fmt.Fprintln(invocation.stdout, usageText())
	return err
}

func usageText() string {
	return `usage: tokeninsights <command> [options]

commands:
  sync              ingest local harness data
  normalize         rebuild canonical facts from raw facts
  reset-canonical   delete canonical facts and diagnostics
  reset-all         recreate the local database
  view              open the interactive TUI
  serve             serve the React dashboard over IPv4 (port 8765)

serve: viewer flags plus --host <ipv4> and --port <0-65535>; --no-sync skips startup sync
  tokeninsights serve --week
  tokeninsights serve --no-sync --port 8080`
}
