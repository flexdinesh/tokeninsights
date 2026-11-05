package cli

import "fmt"

var helpCommand = commandSpec{name: "help", aliases: []string{"--help", "-h"}, run: runHelp}

func runHelp(invocation commandInvocation, _ []string) error {
	_, err := fmt.Fprintln(invocation.stdout, usageText())
	return err
}
func usageText() string {
	return `usage: tokeninsights [command] [options]

  tokeninsights               open local desktop (MyGo by default)
  --open=mygo|electron        select installed desktop
  connect URI                 open exact local, ssh://host or tokeninsights://connect/v1 connection
  remote                      foreground temporary TCP gateway; print connect command
  remote --listen HOST:PORT --advertise HOST
  tui                         local terminal dashboard, also inside SSH
  web                         browser dashboard through a loopback bridge
  web --open=false            print URL without opening a browser
  tui|web --connection URI    explicitly choose another backend
  --capture=false             read saved usage; resume pending processing
  --full-refresh              reread sources and retry quarantine at launch
  data reprocess|wait          finite local/SSH maintenance; --connection supported
  config set KEY VALUE|get KEY|remove KEY

Config: collector.db-path; in-process.server-db-path, host, port; viewer.open; remote.command.
Connections are explicit. No implicit remote default. No transport or browser fallback.
Config: --config-file PATH / TOKENINSIGHTS_CONFIG_PATH; default XDG config.json.
Precedence: flags > environment > file > defaults. Reload/filter query saved data.`
}
