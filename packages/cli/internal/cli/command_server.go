package cli

import (
	"flag"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
	"os"
	"strings"
)

var serverCommand = commandSpec{name: "server", run: runServer}

func runServer(invocation commandInvocation, args []string) error {
	if len(args) == 0 || args[0] != "run" {
		return fmt.Errorf("usage: tokeninsights server run [--server-db-path PATH] [--host IPV4] [--port PORT] [--token TOKEN]\n%w", ErrUsage)
	}
	flags := flag.NewFlagSet("tokeninsights server run", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	var path, host, token string
	var port int
	flags.StringVar(&path, "server-db-path", defaultServerDBPath(), "path to server sqlite db")
	flags.StringVar(&host, "host", server.DefaultHost, "web/API bind IPv4 address")
	flags.IntVar(&port, "port", server.DefaultPort, "web/API port (0 chooses available)")
	flags.StringVar(&token, "token", strings.TrimSpace(os.Getenv("TOKENINSIGHTS_SERVER_TOKEN")), "authentication token; required for non-loopback remote server")
	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() != 0 {
		return ErrUsage
	}
	if err := server.ValidateHost(host); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if port < 0 || port > 65535 {
		return fmt.Errorf("invalid --port\n%w", ErrUsage)
	}
	return service.Run(invocation.context, service.Options{DBPath: path, Host: &host, Port: &port, Remote: true, Token: &token}, invocation.stdout, invocation.stderr)
}
