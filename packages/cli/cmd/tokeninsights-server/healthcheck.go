package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Probe only HTTP readiness; never open storage or read deployment secrets.
func healthcheck(ctx context.Context, args []string, stderr io.Writer) error {
	flags := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	listen := flags.String("listen", envDefault("TOKENINSIGHTS_LISTEN", defaultListen), "server listen address (TOKENINSIGHTS_LISTEN)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected healthcheck arguments")
	}
	host, port, err := net.SplitHostPort(*listen)
	if err != nil {
		return fmt.Errorf("invalid healthcheck listen address")
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/readyz", nil)
	if err != nil {
		return fmt.Errorf("invalid healthcheck address")
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("server not ready")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("server not ready")
	}
	return nil
}
