package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

var ErrStopped = errors.New("service stopped")

type State struct {
	Running bool    `json:"running"`
	Record  *Record `json:"service,omitempty"`
	Status  *Status `json:"status,omitempty"`
}

type Client struct{ Record Record }

type instanceTransport struct {
	transport *http.Transport
	instance  string
}

func (t instanceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("X-TokenInsights-Instance", t.instance)
	return t.transport.RoundTrip(clone)
}
func (c Client) IngestionClient() *http.Client {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", c.Record.Socket)
	}, DisableKeepAlives: true}
	return &http.Client{Transport: instanceTransport{transport: transport, instance: c.Record.InstanceID}, Timeout: callTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (c Client) call(ctx context.Context, method, path string, input, output interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var body bytes.Buffer
	if input != nil {
		if err := json.NewEncoder(&body).Encode(input); err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://local/control/v1"+path, &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-TokenInsights-Instance", c.Record.InstanceID)
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", c.Record.Socket)
	}, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 400 {
		var failure struct {
			Error string `json:"error"`
		}
		if err := decode(response.Body, &failure); err != nil {
			return err
		}
		return fmt.Errorf("control request: %s", failure.Error)
	}
	return decode(response.Body, output)
}

func Probe(ctx context.Context, path string) (State, error) {
	state, err := probeOwner(ctx, path)
	if err != nil || !state.Running {
		return state, err
	}
	client := Client{Record: *state.Record}
	status, err := client.Status(ctx)
	if err != nil {
		return state, err
	}
	state.Status = &status
	return state, nil
}

// probeOwner verifies the held lifetime lock and private runtime identity without
// requiring healthy analytics storage. Only this verified owner can be stopped.
func probeOwner(ctx context.Context, path string) (State, error) {
	canonical, key, err := identify(path)
	if err != nil {
		return State{}, err
	}
	f, held, err := lifetime(canonical, false)
	if f != nil {
		_ = f.Close()
	}
	if err != nil {
		return State{}, err
	}
	if !held {
		return State{}, nil
	}
	p, err := servicePaths(key, false)
	if err != nil {
		return State{}, err
	}
	for _, recordPath := range []string{p.record, p.fallbackRecord} {
		var record Record
		if err := readFile(recordPath, &record); err != nil {
			continue
		}
		if record.Config.DatabaseKey != key || record.Config.DBPath != canonical || record.Socket != p.socket && recordPath != p.fallbackRecord {
			continue
		}
		if record.Protocol != protocolVersion {
			return State{Running: true, Record: &record}, fmt.Errorf("incompatible service protocol; stop service with its installed executable")
		}
		client := Client{Record: record}
		var instance Record
		if err := client.call(ctx, "GET", "/instance", nil, &instance); err != nil {
			continue
		}
		if instance.InstanceID != record.InstanceID || instance.Config != record.Config || instance.Socket != record.Socket || instance.PID != record.PID || instance.SchemaVersion != record.SchemaVersion || instance.Protocol != protocolVersion {
			continue
		}
		return State{Running: true, Record: &record}, nil
	}
	return State{Running: true}, fmt.Errorf("service owns database but control socket is unreachable; inspect service logs")
}

type Status struct {
	InstanceID        string `json:"instanceId"`
	DataEpoch         string `json:"dataEpoch"`
	DataReadiness     string `json:"dataReadiness"`
	Revision          int64  `json:"revision"`
	LastIngestionAtMS int64  `json:"lastIngestionAtMs"`
	PendingProcessing int64  `json:"pendingProcessing"`
}

func (c Client) Status(ctx context.Context) (Status, error) {
	var s Status
	err := c.call(ctx, "GET", "/status", nil, &s)
	return s, err
}

// WaitProcessing is maintenance-only. Sync never waits for projection completion.
func (c Client) WaitProcessing(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := c.Status(ctx)
		if err != nil {
			return err
		}
		if status.PendingProcessing == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c Client) Reprocess(ctx context.Context) (int64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://local/api/v2/processing/reprocess", nil)
	if err != nil {
		return 0, err
	}
	response, err := c.IngestionClient().Do(request)
	if err != nil {
		return 0, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusAccepted {
		return 0, fmt.Errorf("reprocess status %d", response.StatusCode)
	}
	var result struct {
		Generation int64 `json:"generation"`
	}
	err = decode(response.Body, &result)
	return result.Generation, err
}
