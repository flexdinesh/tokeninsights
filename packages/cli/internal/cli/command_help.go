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
  server run                        run canonical ingestion/query server
  sync                              collect all harnesses and publish normalized facts
  normalize                         normalize retained collector raw facts
  reset-canonical                   reset collector canonical facts only
  reset-all                         reset collector database only
  view                              read committed data through server API
  serve                             deprecated alias for service run

  tokeninsights service start --port 8765
  tokeninsights service status --json
  tokeninsights sync --publish-only
  tokeninsights sync --server-url https://example.test
  tokeninsights view --sync

Collector: --collector-db-path (collector.sqlite). Server: --server-db-path (server.sqlite).
Existing tokeninsights.sqlite is untouched; retained sources rebuild fresh databases.
Explicit --server-url skips local startup. Non-loopback serving requires a token.`
}
