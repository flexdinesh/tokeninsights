package cli

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
	"github.com/flexdinesh/tokeninsights/packages/core/remoteaccess"
)

var remoteCommand = commandSpec{name: "remote", run: runRemote}

func runRemote(invocation commandInvocation, args []string) error {
	flags := flag.NewFlagSet("tokeninsights remote", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	listen := flags.String("listen", "0.0.0.0:0", "TCP listen address; 0 chooses an available port")
	advertise := flags.String("advertise", "", "reachable advertised host or host:port")
	defaults := invocation.defaults()
	flags.String("collector-db-path", defaults.CollectorDBPath, "collector database")
	flags.String("server-db-path", defaults.ServerDBPath, "token database")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return ErrUsage
	}
	settings, err := invocation.resolveLocal(flags)
	if err != nil {
		return err
	}
	session, err := connectBackendSession(invocation.context, desktoplaunch.Specification(settings, false, false, ""), func() error { return invocation.prepareLocalStorage(settings) })
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()
	gateway, err := remoteaccess.Start(invocation.context, session, remoteaccess.Options{Listen: *listen, Advertise: *advertise, AddressHint: sshAddressHint()})
	if err != nil {
		return err
	}
	defer func() { _ = gateway.Close() }()
	if _, err := fmt.Fprintf(invocation.stdout, "%s %s %s\n", shellQuote("tokeninsights"), shellQuote("connect"), shellQuote(gateway.ConnectionString())); err != nil {
		return err
	}
	select {
	case <-invocation.context.Done():
		return nil
	case <-gateway.Done():
		if invocation.context.Err() != nil {
			return nil
		}
		return errors.New("remote gateway stopped")
	case <-session.Done():
		if invocation.context.Err() != nil {
			return nil
		}
		return errors.New("backend disconnected")
	}
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func sshAddressHint() string {
	fields := strings.Fields(os.Getenv("SSH_CONNECTION"))
	if len(fields) == 4 {
		if address := net.ParseIP(fields[2]); address != nil && !address.IsUnspecified() {
			return address.String()
		}
	}
	return ""
}
