package cli

import "fmt"

var serverCommand = commandSpec{name: "server", run: runServer}

func runServer(_ commandInvocation, _ []string) error {
	return fmt.Errorf("server run removed; use tokeninsights-server --listen IPv4:port --server-db-path PATH\n%w", ErrUsage)
}
