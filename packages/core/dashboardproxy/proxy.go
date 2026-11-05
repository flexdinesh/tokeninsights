// Package dashboardproxy owns per-presentation loopback browser bridges.
package dashboardproxy

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"time"
)

const viewersPath = "/api/local/v1/viewers"
const upstreamHost = "localhost"
const readHeaderTimeout = 5 * time.Second

type Options struct {
	Host       string
	Port       int
	Connection endpoint.Info
}
type Bridge struct {
	URL    string
	server *http.Server
	once   sync.Once
	done   chan struct{}
}

func Start(ctx context.Context, private http.RoundTripper, options Options) (*Bridge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if private == nil {
		return nil, errors.New("dashboard bridge requires a backend transport")
	}
	host := options.Host
	if host == "" || host == "localhost" {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() || options.Port < 0 || options.Port > 65535 {
		return nil, errors.New("dashboard bridge requires a loopback host and valid port")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(options.Port)))
	if err != nil {
		return nil, errors.New("dashboard bridge address unavailable")
	}
	server := &http.Server{Handler: handler(private, host, options.Connection), ReadHeaderTimeout: readHeaderTimeout, MaxHeaderBytes: 1 << 20}
	bridge := &Bridge{URL: "http://" + listener.Addr().String(), server: server, done: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			_ = bridge.Close()
		case <-bridge.done:
		}
	}()
	go func() { _ = server.Serve(listener) }()
	return bridge, nil
}

func (b *Bridge) Close() error {
	var err error
	b.once.Do(func() { err = b.server.Close(); close(b.done) })
	return err
}

// Handler is useful to embed the same restricted bridge policy.
func Handler(private http.RoundTripper, bindHost string) http.Handler {
	return handler(private, bindHost, endpoint.Info{})
}

func handler(private http.RoundTripper, bindHost string, connection endpoint.Info) http.Handler {
	proxy := &httputil.ReverseProxy{
		Transport: private, FlushInterval: -1,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme, r.Out.URL.Host, r.Out.Host = "http", upstreamHost, upstreamHost
			for _, key := range []string{"Cookie", "Authorization", "Origin", "Referer", "X-TokenInsights-Instance", "Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
				r.Out.Header.Del(key)
			}
		},
		ModifyResponse: func(r *http.Response) error {
			r.Header.Del("Set-Cookie")
			r.Header.Del("Access-Control-Allow-Origin")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "backend_unavailable", http.StatusBadGateway)
		},
	}
	if parsed, err := endpoint.Parse(connection.Target); err == nil {
		connection.Target = parsed.String()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localprotocol.HostAllowed(r, bindHost) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/api/viewer/connection" {
			if r.Method != http.MethodGet {
				http.Error(w, "method_not_allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(connection)
			return
		}
		if r.URL.Path == "/api/viewer/refresh" {
			if r.Method != http.MethodPost {
				http.Error(w, "method_not_allowed", http.StatusMethodNotAllowed)
				return
			}
			if r.URL.RawQuery != "" || r.Header.Get("Origin") != "http://"+r.Host || !localprotocol.ViewerRequestAllowed(r, bindHost) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || media != "application/json" {
				http.Error(w, "json_required", http.StatusUnsupportedMediaType)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, localprotocol.MaxDefaultCaptureBytes)
			if !localprotocol.DefaultCapture(r.Body) {
				http.Error(w, "default_capture_required", http.StatusBadRequest)
				return
			}
			r = r.Clone(r.Context())
			r.URL.Path = "/api/local/v1/capture"
			r.URL.RawPath = ""
			r.Body = io.NopCloser(strings.NewReader("{}"))
			r.ContentLength = 2
			r.Header.Set("Content-Type", "application/json")
		} else if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method_not_allowed", http.StatusMethodNotAllowed)
			return
		} else if r.URL.Path == viewersPath {
			if r.URL.RawQuery != "" || !localprotocol.ViewerRequestAllowed(r, bindHost) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		} else if !localprotocol.ViewerRead(r) {
			http.NotFound(w, r)
			return
		}
		if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			http.Error(w, "stream_unavailable", http.StatusInternalServerError)
			return
		}
		proxy.ServeHTTP(w, r)
	})
}
