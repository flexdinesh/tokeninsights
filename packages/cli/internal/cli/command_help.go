package cli

import "fmt"

var helpCommand = commandSpec{name: "help", aliases: []string{"--help", "-h"}, run: runHelp}

func runHelp(invocation commandInvocation, _ []string) error {
	_, err := fmt.Fprintln(invocation.stdout, usageText())
	return err
}

func usageText() string {
	return `usage: tokeninsights <command> [options]

Bare invocation ensures local or prints configured remote URL. No startup sync.

commands:
  service start|stop|restart|status   manage local web/API server
  sync                              collect all harnesses and submit sanitized raw evidence
  tui                               sync with progress, then open terminal dashboard
  config set KEY VALUE|get KEY|remove KEY
                                    manage client preferences

advanced:
  collector normalize|reset-canonical|reset-all
                                    manage host collector data
  service run                       run local server in foreground
  service reprocess|wait             rebuild evidence projection; wait for processing
  service import --legacy-server-db-path PATH
                                    import verified SQLite history into a new DuckDB
  tokeninsights-server              separate foreground remote executable

  tokeninsights service start --port 8765
  tokeninsights service status --json
  tokeninsights sync
  tokeninsights tui
  tokeninsights sync --publish-only
  tokeninsights sync --server-url https://example.test
  tokeninsights tui --sync=false
  tokeninsights collector --help
  tokeninsights config set server-url http://remote-machine:8765
  tokeninsights config set host 0.0.0.0

Collector: --collector-db-path (collector.sqlite). Server: --server-db-path (server.duckdb).
Existing tokeninsights.sqlite is untouched; retained sources rebuild fresh databases.
Config: --config-file PATH / TOKENINSIGHTS_CONFIG_PATH (default XDG config.json).
Keys: server-url, host, port, collector-db-path, server-db-path.
Runtime precedence: flags > environment > file > defaults; get reads preferences.
Empty server-url selects local. Remote failures never start local.
Public servers are unauthenticated, including 0.0.0.0. --token/server run removed.`
}
