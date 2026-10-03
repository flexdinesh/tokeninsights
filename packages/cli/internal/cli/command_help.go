package cli

import "fmt"

var helpCommand = commandSpec{name: "help", aliases: []string{"--help", "-h"}, run: runHelp}

func runHelp(invocation commandInvocation, _ []string) error {
	_, err := fmt.Fprintln(invocation.stdout, usageText())
	return err
}

func usageText() string {
	return `usage: tokeninsights <command> [options]

Bare invocation ensures the background service and prints its URL. No startup sync.

commands:
  service start|stop|restart|status   manage background web/API service
  service run                       run service in foreground
  refresh [--wait]                   request all-harness refresh
  sync                              ingest local harness data
  normalize                         rebuild canonical facts
  reset-canonical                   delete canonical facts
  reset-all                         reset application tables
  view                              local TUI; refresh on opening
  serve                             deprecated alias for service run

  tokeninsights service start --host 0.0.0.0 --port 8765
  tokeninsights service status --json
  tokeninsights service restart --reload-sources
  tokeninsights view --no-sync

--host binds web/API only. Authentication and reboot autostart are not included.`
}
