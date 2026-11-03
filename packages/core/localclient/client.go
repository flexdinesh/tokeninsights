// Package localclient attaches presentations to the shared local backend.
// Presentations never open its storage or terminate other viewers.
package localclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/core/serverownership"
	"github.com/flexdinesh/tokeninsights/packages/core/writerlock"
)

const StartupTimeout = 15 * time.Second
const operationTimeout = 35 * time.Second

type Options struct {
	CollectorDBPath, ServerDBPath, AppDBPath, Host string
	Port                                           int
	BackendPath                                    string
	Capture, FullRefresh                           bool
	SourceDir                                      string
}

type Session struct {
	Query      *queryclient.Client
	URL        string
	InstanceID string
	client     *http.Client
	transport  *http.Transport
	lease      io.ReadCloser
	cancel     context.CancelFunc
	done       chan struct{}
	once       sync.Once
}

func Connect(parent context.Context, options Options) (*Session, error) {
	return ConnectPrepared(parent, options, nil)
}

// ConnectPrepared runs legacy local-storage recovery only when starting an
// unowned backend, under startup serialization. Attached viewers never inspect
// or reopen the owner's stores. A nil preparation leaves recovery to the caller.
func ConnectPrepared(parent context.Context, options Options, prepare func() error) (*Session, error) {
	settings := localprotocol.Settings{CollectorDBPath: options.CollectorDBPath, ServerDBPath: options.ServerDBPath, AppDBPath: options.AppDBPath, Host: options.Host, Port: options.Port}
	if settings.AppDBPath == "" {
		server, _, err := serverownership.Identify(settings.ServerDBPath)
		if err != nil {
			return nil, err
		}
		settings.AppDBPath = filepath.Join(filepath.Dir(server), "app.sqlite")
	}
	settings, err := settings.Canonical()
	if err != nil {
		return nil, err
	}
	if options.FullRefresh && !options.Capture {
		return nil, errors.New("full refresh requires capture")
	}
	if options.SourceDir != "" {
		options.SourceDir, err = filepath.Abs(options.SourceDir)
		if err != nil {
			return nil, err
		}
	}
	socket, err := localprotocol.SocketPath(settings)
	if err != nil {
		return nil, err
	}
	startup, cancelStartup := context.WithTimeout(parent, StartupTimeout)
	defer cancelStartup()
	release, err := writerlock.Acquire(startup, socket+".startup")
	if err != nil {
		return nil, err
	}
	defer release()
	s := socketSession(socket)
	attached := false
	defer func() {
		if !attached {
			s.transport.CloseIdleConnections()
		}
	}()
	var descriptor localprotocol.Descriptor
	err = s.call(startup, http.MethodGet, "/descriptor", nil, &descriptor)
	if err != nil {
		// A reachable but incompatible or corrupt endpoint must never trigger
		// fallback. Only a failed Unix connection permits owner inspection.
		var connection *net.OpError
		if !errors.As(err, &connection) {
			return nil, err
		}
		owner, held, ownerErr := serverownership.Lifetime(settings.ServerDBPath, false)
		if ownerErr != nil {
			return nil, ownerErr
		}
		if owner != nil {
			_ = owner.Close()
		}
		if held {
			return nil, localprotocol.ErrOwned
		}
		if prepare != nil {
			if err := prepare(); err != nil {
				return nil, err
			}
		}
		if err := start(startup, options.BackendPath, settings); err != nil {
			return nil, err
		}
		if err := s.call(startup, http.MethodGet, "/descriptor", nil, &descriptor); err != nil {
			return nil, err
		}
	}
	if err := descriptor.Compatible(settings); err != nil {
		return nil, err
	}
	if err := s.attach(parent, startup, descriptor, options.Capture, options.FullRefresh, options.SourceDir); err != nil {
		return nil, err
	}
	attached = true
	return s, nil
}

func (s *Session) attach(parent, startup context.Context, descriptor localprotocol.Descriptor, capture, fullRefresh bool, sourceDir string) error {
	s.URL, s.InstanceID = descriptor.URL, descriptor.InstanceID
	s.Query = queryclient.NewDirect(privateReader{session: s})
	leaseCtx, cancelLease := context.WithCancel(parent)
	s.cancel = cancelLease
	request, err := s.request(leaseCtx, http.MethodGet, "/viewers", nil)
	if err != nil {
		cancelLease()
		return err
	}
	// Stream headers are bounded independently from its indefinite lifetime.
	stop := context.AfterFunc(startup, cancelLease)
	response, err := s.client.Do(request)
	stop()
	if err != nil {
		cancelLease()
		return err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		cancelLease()
		return errors.New("local backend viewer attachment rejected")
	}
	s.lease = response.Body
	go func() { _, _ = io.Copy(io.Discard, response.Body); close(s.done) }()
	if capture {
		if err := s.Capture(startup, fullRefresh, sourceDir); err != nil {
			_ = s.Close()
			return err
		}
	}
	return nil
}

// Done closes when this viewer loses its backend connection.
func (s *Session) Done() <-chan struct{} { return s.done }

func (s *Session) Close() error {
	var err error
	s.once.Do(func() { s.cancel(); err = s.lease.Close(); <-s.done; s.transport.CloseIdleConnections() })
	return err
}

func (s *Session) Progress(ctx context.Context) (collectorprogress.DetailSnapshot, error) {
	var result collectorprogress.DetailSnapshot
	err := s.call(ctx, http.MethodGet, "/progress", nil, &result)
	return result, err
}

func (s *Session) ProcessingStatus(ctx context.Context) (analytics.ProcessingStatus, error) {
	var result analytics.ProcessingStatus
	err := s.call(ctx, http.MethodGet, "/processing", nil, &result)
	return result, err
}

// Capture requests backend-owned collection. Cancellation stops this request,
// while accepted shared work remains owned by the backend.
func (s *Session) Capture(ctx context.Context, fullRefresh bool, sourceDir string) error {
	if sourceDir != "" {
		path, err := filepath.Abs(sourceDir)
		if err != nil {
			return err
		}
		sourceDir = path
	}
	return s.call(ctx, http.MethodPost, "/capture", localprotocol.CaptureRequest{FullRefresh: fullRefresh, SourceDir: sourceDir}, nil)
}

// Collect requests shared capture and waits for its sanitized completion. A
// disconnected caller stops waiting without cancelling work for other viewers.
func (s *Session) Collect(ctx context.Context, fullRefresh bool, sourceDir string) (localprotocol.CollectionResult, error) {
	var result localprotocol.CollectionResult
	if sourceDir != "" {
		path, err := filepath.Abs(sourceDir)
		if err != nil {
			return result, err
		}
		sourceDir = path
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(localprotocol.CaptureRequest{FullRefresh: fullRefresh, SourceDir: sourceDir}); err != nil {
		return result, err
	}
	request, err := s.request(ctx, http.MethodPost, "/capture?wait=true", &body)
	if err != nil {
		return result, err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return result, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusConflict {
		return result, localprotocol.ErrCaptureBusy
	}
	if response.StatusCode != http.StatusOK {
		return result, errors.New("local backend collection rejected")
	}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if line, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			if err := json.Unmarshal([]byte(line), &result); err != nil {
				return result, errors.New("invalid local collection result")
			}
			return result, nil
		}
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, errors.New("local backend collection disconnected")
}

func (s *Session) WaitVisible(ctx context.Context) error {
	return s.call(ctx, http.MethodPost, "/wait", nil, nil)
}
func (s *Session) Reprocess(ctx context.Context) error {
	return s.call(ctx, http.MethodPost, "/reprocess", nil, nil)
}

func (s *Session) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	r, err := http.NewRequestWithContext(ctx, method, "http://local/api/local/v1"+path, body)
	if err == nil {
		r.Header.Set("X-TokenInsights-Instance", s.InstanceID)
		r.Header.Set("Content-Type", "application/json")
	}
	return r, err
}

func (s *Session) call(parent context.Context, method, path string, body, target interface{}) error {
	ctx, cancel := context.WithTimeout(parent, operationTimeout)
	defer cancel()
	var data bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&data).Encode(body); err != nil {
			return err
		}
	}
	request, err := s.request(ctx, method, path, &data)
	if err != nil {
		return err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&failure)
		switch failure.Error {
		case "capture_busy":
			return localprotocol.ErrCaptureBusy
		case "processing_failed":
			return localprotocol.ErrProcessingFailed
		case "processing_timeout":
			return localprotocol.ErrProcessingTimeout
		}
		return fmt.Errorf("local backend operation rejected (HTTP %d)", response.StatusCode)
	}
	if target != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target)
	}
	return nil
}

// BundledBackend resolves the adjacent binary. No PATH or remote fallback.
func BundledBackend() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(executable), "tokeninsights-backend"), nil
}

func socketSession(socket string) *Session {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	return &Session{client: client, transport: transport, done: make(chan struct{})}
}
