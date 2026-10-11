package cli

import "fmt"

var helpCommand = commandSpec{name: "help", aliases: []string{"--help", "-h"}, run: runHelp}

func runHelp(invocation commandInvocation, _ []string) error {
	_, err := fmt.Fprintln(invocation.stdout, usageText())
	return err
}

func usageText() string {
	return `usage: tokeninsights <command> [options]

In-process:
  tui                         collect, ingest and open terminal dashboard
  web                         collect, ingest and serve browser dashboard until exit
  web --host 0.0.0.0          bind all IPv4 interfaces (read-only dashboard)
  tui|web --sync=false         show saved data; resume pending processing
  tui|web --full-refresh       reread sources/retry quarantine at startup

Distributed:
  sync                        start finite background submission
  sync --print                also submit; stdout contains only remote URL
  sync --wait                 wait for acceptance
  sync --debug                show capture, acceptance and receipt processing
  sync status [--json]         latest durable job status
  browse                      open configured hosted dashboard; no upload
  browse --open=false          print dashboard URL without launching browser

Configuration:
  config set KEY VALUE|get KEY|remove KEY
  config set distributed.server-url https://usage.example.com
  config set distributed.server-token  read bearer token securely from terminal/stdin

Maintenance:
  data reprocess|wait          finite local processing, no daemon
  tokeninsights-server         separate authenticated container/server executable

Config groups: collector.db-path; in-process.host, port, server-db-path, app-db-path;
distributed.server-url, server-token. No mode setting or flag.
Config: --config-file PATH / TOKENINSIGHTS_CONFIG_PATH; default XDG config.json.
Precedence: flags > environment > file > defaults. get reads saved preferences.
Local web defaults to 127.0.0.1:8765; hosted server defaults to 0.0.0.0:8766.
Commands select composition. Local viewers ignore distributed credentials.
Sync requires URL and bearer token. Browse requires only URL.
Bare invocation prints help. Reload queries saved data. Plugins pass --wait --harness.`
}
