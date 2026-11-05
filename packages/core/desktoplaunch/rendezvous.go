package desktoplaunch

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

const acknowledgementTimeout = 30 * time.Second
const messageLimit = 16384
const rendezvousIDBytes = 16

type acknowledgement struct {
	Ready       bool         `json:"ready"`
	Application browser.Mode `json:"application"`
}

// Launch returns only after the desktop acknowledges ownership of the live
// requested session. An OS opener accepting a URL is insufficient.
func Launch(ctx context.Context, spec Spec) error {
	return launch(ctx, spec, browser.Launch)
}

func launch(ctx context.Context, spec Spec, open func(context.Context, string) error) error {
	ctx, cancel := context.WithTimeout(ctx, acknowledgementTimeout)
	defer cancel()
	directory, err := rendezvousDirectory()
	if err != nil {
		return err
	}
	random := make([]byte, rendezvousIDBytes)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	id := hex.EncodeToString(random)
	path := filepath.Join(directory, id+".sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("create private desktop rendezvous: %w", err)
	}
	defer func() { _ = listener.Close(); _ = os.Remove(path) }()
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	if err := open(ctx, browser.DesktopScheme+"://launch?id="+id); err != nil {
		return err
	}
	conn, err := listener.Accept()
	if err != nil {
		return errors.New("desktop did not acknowledge a live session; open or reinstall the desktop application and retry")
	}
	defer func() { _ = conn.Close() }()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	stopConn := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopConn()
	if err := json.NewEncoder(conn).Encode(spec); err != nil {
		return errors.New("desktop handoff failed; retry the desktop application")
	}
	var ack acknowledgement
	decoder := json.NewDecoder(io.LimitReader(conn, messageLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ack); err != nil || !ack.Ready || ack.Application != browser.ModeDesktop {
		return errors.New("desktop could not hold the requested session; check the desktop error and retry")
	}
	return nil
}

// Handoff holds a private launch request until the desktop confirms a live
// session. Close rejects an unacknowledged request and releases the socket.
type Handoff struct {
	Spec     Spec
	deadline time.Time
	conn     net.Conn
	once     sync.Once
}

func (h *Handoff) Acknowledge(application browser.Mode) error {
	return h.reply(acknowledgement{Ready: true, Application: application})
}

func (h *Handoff) reply(ack acknowledgement) error {
	var err error
	h.once.Do(func() { err = json.NewEncoder(h.conn).Encode(ack); _ = h.conn.Close() })
	return err
}
func (h *Handoff) Deadline() time.Time { return h.deadline }

func (h *Handoff) Close() error { return h.reply(acknowledgement{}) }

// Receive accepts only an opaque ID, never a connection string or URL target.
func Receive(ctx context.Context, raw string) (*Handoff, error) {
	id, err := ID(raw)
	if err != nil {
		return nil, err
	}
	directory, err := rendezvousDirectory()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, acknowledgementTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(directory, id+".sock"))
	if err != nil {
		return nil, errors.New("desktop launch expired; run tokeninsights again")
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	var spec Spec
	decoder := json.NewDecoder(io.LimitReader(conn, messageLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		_ = conn.Close()
		return nil, errors.New("invalid private desktop request")
	}
	return &Handoff{Spec: spec, conn: conn, deadline: deadline}, nil
}

func ID(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err == nil && u != nil && u.Scheme == browser.DesktopScheme && u.Host == "launch" && u.Path == "" && u.Fragment == "" && u.User == nil {
		values, queryErr := url.ParseQuery(u.RawQuery)
		id := values.Get("id")
		decoded, decodeErr := hex.DecodeString(id)
		if queryErr == nil && len(values) == 1 && len(values["id"]) == 1 && len(decoded) == rendezvousIDBytes && decodeErr == nil && id == strings.ToLower(id) {
			return id, nil
		}
	}
	return "", errors.New("invalid desktop launch link; run tokeninsights again")
}

func rendezvousDirectory() (string, error) { return localprotocol.RuntimeDirectory() }
