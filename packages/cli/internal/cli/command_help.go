package cli

import "fmt"

var helpCommand = commandSpec{name: "help", aliases: []string{"--help", "-h"}, run: runHelp}

func runHelp(invocation commandInvocation, _ []string) error {
	_, err := fmt.Fprintln(invocation.stdout, usageText())
	return err
}

func usageText() string {
	return `usage: tokeninsights <command> [options]

Single-process (default):
  tui                         collect, ingest and open terminal dashboard
  web                         collect, ingest and serve browser dashboard until exit
  web --host 0.0.0.0          bind all IPv4 interfaces (read-only dashboard)
  sync                        collect and ingest directly; hand off to an active viewer
  tui|web --sync=false         show saved data

Distributed:
  sync                        start finite background submission
  sync --print                also submit; stdout contains only remote URL
  sync --wait                 wait for acceptance
  sync --debug                show capture, acceptance and receipt processing
  sync status [--json]         latest durable job status
  web                         sync, then open remote browser login

Configuration:
  config set KEY VALUE|get KEY|remove KEY
  config set mode single-process|distributed
  config set server-url https://usage.example.com
  config set server-token     read bearer token securely from terminal/stdin

Maintenance:
  data import --legacy-server-db-path PATH --server-db-path NEW
  data reprocess|wait          finite local processing, no daemon
  collector normalize|reset-canonical|reset-all
  service stop|status          migrate an owner created by an older release
  tokeninsights-server         separate authenticated container/server executable

Paths: --collector-db-path, --server-db-path, --app-db-path.
Config: --config-file PATH / TOKENINSIGHTS_CONFIG_PATH; default XDG config.json.
Keys: mode, server-url, server-token, host, port, collector-db-path, server-db-path, app-db-path.
Precedence: flags > environment > file > defaults. get reads saved preferences.
Use TOKENINSIGHTS_MODE and TOKENINSIGHTS_ACCESS_TOKEN for environment overrides.
Legacy server-kind hosted maps to distributed. Remote requires URL and bearer token.
Distributed has no analytics TUI. Local commands need no daemon or HTTP ingestion.
Bare invocation prints help. Reload queries saved data. Plugins pass --wait --harness.`
}
