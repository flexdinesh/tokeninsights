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
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
)

const StartupTimeout = 15 * time.Second
const operationTimeout = 35 * time.Second

// privateHost names the backend's private endpoint in request URLs. Every
// request uses it so transports share one connection, which single-use
// dialers such as SSH bridges require.
const privateHost = "local"

// maxLeaseEventBytes bounds the dashboard event preceding lease heartbeats.
const maxLeaseEventBytes = 4096

// Dialer opens one connection to a backend's private control endpoint. The
// launcher owns discovery; the Session owns HTTP and the viewer lease.
type Dialer func(context.Context) (net.Conn, error)

// Session is one viewer lease over private backend operations. URL is set only
// for sessions holding a dashboard lease.
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

// attachOptions describe one lease and its optional launch capture.
type attachOptions struct {
	Capture, FullRefresh bool
	SourceDir            string
	Listen               *localprotocol.Listen
}

func newSession(dial Dialer, unencryptedHTTP2 bool) *Session {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return dial(ctx) }}
	if unencryptedHTTP2 {
		protocols := new(http.Protocols)
		protocols.SetUnencryptedHTTP2(true)
		transport.Protocols = protocols
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	return &Session{client: client, transport: transport, done: make(chan struct{})}
}

func (s *Session) descriptor(ctx context.Context) (localprotocol.Descriptor, error) {
	var descriptor localprotocol.Descriptor
	err := s.call(ctx, http.MethodGet, "/descriptor", nil, &descriptor)
	return descriptor, err
}

func (s *Session) attach(parent, startup context.Context, descriptor localprotocol.Descriptor, options attachOptions) error {
	s.InstanceID = descriptor.InstanceID
	s.Query = queryclient.NewDirect(privateReader{session: s})
	leaseCtx, cancelLease := context.WithCancel(parent)
	s.cancel = cancelLease
	path := "/viewers"
	if options.Listen != nil {
		query := url.Values{"host": {options.Listen.Host}, "port": {strconv.Itoa(options.Listen.Port)}}
		path += "?" + query.Encode()
	}
	request, err := s.request(leaseCtx, http.MethodGet, path, nil)
	if err != nil {
		cancelLease()
		return err
	}
	// Stream headers and the dashboard event are bounded independently from
	// the lease's indefinite lifetime.
	stop := context.AfterFunc(startup, cancelLease)
	response, err := s.client.Do(request)
	if err != nil {
		stop()
		cancelLease()
		return err
	}
	if response.StatusCode != http.StatusOK {
		err := failure(response, "local backend viewer attachment rejected")
		_ = response.Body.Close()
		stop()
		cancelLease()
		return err
	}
	reader := bufio.NewReader(response.Body)
	if options.Listen != nil {
		dashboard, err := readDashboard(reader)
		if err != nil {
			_ = response.Body.Close()
			stop()
			cancelLease()
			return err
		}
		s.URL = dashboard.URL
	}
	stop()
	s.lease = response.Body
	go func() { _, _ = io.Copy(io.Discard, reader); close(s.done) }()
	if options.Capture {
		if err := s.Capture(startup, options.FullRefresh, options.SourceDir); err != nil {
			_ = s.Close()
			return err
		}
	}
	return nil
}

// readDashboard reads the first lease event, which names the bound listener.
func readDashboard(reader *bufio.Reader) (localprotocol.Dashboard, error) {
	var dashboard localprotocol.Dashboard
	limited := bufio.NewScanner(io.LimitReader(reader, maxLeaseEventBytes))
	for limited.Scan() {
		line := limited.Text()
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			if err := json.Unmarshal([]byte(data), &dashboard); err != nil {
				return dashboard, localprotocol.ErrIncompatible
			}
			if _, err := localprotocol.HTTPOrigin(dashboard.URL, false); err != nil {
				return dashboard, localprotocol.ErrIncompatible
			}
			return dashboard, nil
		}
	}
	return dashboard, errors.New("local backend dashboard lease disconnected")
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
	r, err := http.NewRequestWithContext(ctx, method, "http://"+privateHost+"/api/local/v1"+path, body)
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
		return failure(response, fmt.Sprintf("local backend operation rejected (HTTP %d)", response.StatusCode))
	}
	if target != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target)
	}
	return nil
}

// failure maps a sanitized backend error code to its protocol error.
func failure(response *http.Response, fallback string) error {
	var body struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&body)
	switch body.Error {
	case "capture_busy":
		return localprotocol.ErrCaptureBusy
	case "processing_failed":
		return localprotocol.ErrProcessingFailed
	case "processing_timeout":
		return localprotocol.ErrProcessingTimeout
	case "listen_conflict":
		return localprotocol.ErrSettingsMismatch
	case "listen_unavailable":
		return localprotocol.ErrListenUnavailable
	case "shutting_down":
		return localprotocol.ErrShuttingDown
	}
	return errors.New(fallback)
}

// PrivateRoundTripper sends requests over this session's private connection,
// pinned to its backend instance. Callers own routing policy and must forward
// only read-only dashboard requests and browser leases.
func (s *Session) PrivateRoundTripper() http.RoundTripper {
	return instanceTransport{session: s}
}

type instanceTransport struct{ session *Session }

func (t instanceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	// Keep the caller's Host header, but reuse the session's connection.
	if r.Host == "" {
		r.Host = r.URL.Host
	}
	r.URL.Scheme, r.URL.Host = "http", privateHost
	r.Header.Set("X-TokenInsights-Instance", t.session.InstanceID)
	return t.session.transport.RoundTrip(r)
}
