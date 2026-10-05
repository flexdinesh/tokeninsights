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
  service start|stop|restart|status   manage local web/API server
  sync                              collect all harnesses and publish normalized facts
  tui                               read committed data through the same REST API as Web

advanced:
  collector normalize|reset-canonical|reset-all
                                    manage host collector data
  service run                       run local server in foreground
  server run                        run canonical ingestion/query server

  tokeninsights service start --port 8765
  tokeninsights service status --json
  tokeninsights sync
  tokeninsights tui
  tokeninsights sync --publish-only
  tokeninsights sync --server-url https://example.test
  tokeninsights tui --sync
  tokeninsights collector --help

Deprecated aliases: view, normalize, reset-canonical, reset-all, serve.

Collector: --collector-db-path (collector.sqlite). Server: --server-db-path (server.sqlite).
Existing tokeninsights.sqlite is untouched; retained sources rebuild fresh databases.
Explicit --server-url skips local startup. Non-loopback serving requires a token.`
}
