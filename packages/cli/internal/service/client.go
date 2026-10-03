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

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/app"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

var ErrStopped = errors.New("service stopped")

type State struct {
	Running bool        `json:"running"`
	Record  *Record     `json:"service,omitempty"`
	Status  *app.Status `json:"status,omitempty"`
}

type Client struct{ Record Record }

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
		if instance.InstanceID != record.InstanceID || instance.Config.DatabaseKey != key || instance.Protocol != protocolVersion {
			continue
		}
		status, err := client.Status(ctx)
		if err != nil {
			return State{Running: true, Record: &record}, err
		}
		return State{Running: true, Record: &record, Status: &status}, nil
	}
	return State{Running: true}, fmt.Errorf("service owns database but control socket is unreachable; inspect service logs")
}

func (c Client) Status(ctx context.Context) (app.Status, error) {
	var s app.Status
	err := c.call(ctx, "GET", "/status", nil, &s)
	return s, err
}
func (c Client) Refresh(ctx context.Context) (app.Operation, error) {
	id := app.ID()
	var o app.Operation
	err := c.call(ctx, "POST", "/refresh", struct {
		ID string `json:"id"`
	}{id}, &o)
	if err != nil && ctx.Err() == nil {
		lookupErr := c.call(ctx, "GET", "/requests/"+id, nil, &o)
		if lookupErr == nil {
			return o, nil
		}
	}
	return o, err
}

func (c Client) Submit(ctx context.Context, id string, action app.Action) (app.Operation, error) {
	if c.Record.ActionVersion != 1 || c.Record.SchemaVersion != db.SupportedSchemaVersion || c.Record.DataGeneration != db.CurrentDataGeneration {
		return app.Operation{}, fmt.Errorf("service action contract incompatible; restart service")
	}
	var o app.Operation
	err := c.call(ctx, "POST", "/operations", struct {
		ID     string     `json:"id"`
		Action app.Action `json:"action"`
	}{id, action}, &o)
	return o, err
}
func (c Client) Operation(ctx context.Context, id string) (app.Operation, error) {
	var o app.Operation
	err := c.call(ctx, "GET", "/operations/"+id, nil, &o)
	return o, err
}
func (c Client) Wait(ctx context.Context, id string) (app.Operation, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		o, err := c.Operation(ctx, id)
		if err != nil {
			return o, err
		}
		switch o.State {
		case "succeeded":
			return o, nil
		case "failed", "cancelled":
			err := errors.New(o.Error)
			if o.ErrorCode == "recovery" {
				err = errors.Join(err, db.ErrRebuildPending)
			}
			return o, err
		}
		select {
		case <-ctx.Done():
			return o, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Mutate forwards exactly the caller's action or holds admission for its entire
// standalone execution. An unreachable owner never permits bypass.
func Mutate(ctx context.Context, path string, action app.Action, progress func(pipeline.SyncProgressEvent)) (pipeline.Summary, error) {
	canonical, _, err := identify(path)
	if err != nil {
		return pipeline.Summary{}, err
	}
	release, err := admission(ctx, canonical)
	if err != nil {
		return pipeline.Summary{}, err
	}
	state, err := Probe(ctx, canonical)
	if err != nil {
		release()
		return pipeline.Summary{}, err
	}
	if !state.Running {
		defer release()
		return app.Execute(ctx, canonical, action, progress, nil)
	}
	c := Client{Record: *state.Record}
	id := app.ID()
	o, err := c.Submit(ctx, id, action)
	release()
	if err != nil { // Lost response: query this ID, never blindly resubmit.
		if ctx.Err() != nil {
			c.cancelExclusive(id)
			return pipeline.Summary{}, ctx.Err()
		}
		lookup, lookupErr := c.Operation(ctx, id)
		if lookupErr != nil {
			return pipeline.Summary{}, fmt.Errorf("submission outcome unknown: %w", err)
		}
		o = lookup
	}
	o, err = c.Wait(ctx, o.ID)
	if ctx.Err() != nil {
		// A cancelled status HTTP call can return a zero Operation. Use the
		// original request ID, which also identifies this exclusive action.
		c.cancelExclusive(id)
	}
	return o.Summary, err
}

func (c Client) cancelExclusive(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var result app.Operation
	if c.call(ctx, "POST", "/operations/"+id+"/cancel", nil, &result) == nil {
		_, _ = c.Wait(ctx, id)
	}
}
