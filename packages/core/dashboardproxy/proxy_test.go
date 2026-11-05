package dashboardproxy

import (
	"context"
	"encoding/json"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type recordingTransport struct {
	mu       sync.Mutex
	requests []*http.Request
}

func TestBridgeRefreshRequiresSameOriginDefaultJSON(t *testing.T) {
	for _, scenario := range []struct {
		origin, site, contentType, body, query string
		status                                 int
	}{
		{"http://127.0.0.1:9000", "same-origin", "application/json", "{}", "", http.StatusOK},
		{"http://127.0.0.1:9000", "", "application/json", `{"fullRefresh":false,"sourceDir":""}`, "", http.StatusOK},
		{"", "", "application/json", "{}", "", http.StatusForbidden},
		{"null", "", "application/json", "{}", "", http.StatusForbidden},
		{"http://evil.test", "cross-site", "application/json", "{}", "", http.StatusForbidden},
		{"http://127.0.0.1:9000", "cross-site", "application/json", "{}", "", http.StatusForbidden},
		{"http://127.0.0.1:9000", "", "text/plain", "{}", "", http.StatusUnsupportedMediaType},
		{"http://127.0.0.1:9000", "", "application/json", `{"sourceDir":"/private"}`, "", http.StatusBadRequest},
		{"http://127.0.0.1:9000", "", "application/json", `{"fullRefresh":true}`, "", http.StatusBadRequest},
		{"http://127.0.0.1:9000", "", "application/json", `{"fullRefresh":true,"fullRefresh":false}`, "", http.StatusBadRequest},
		{"http://127.0.0.1:9000", "", "application/json", "null", "", http.StatusBadRequest},
		{"http://127.0.0.1:9000", "", "application/json", "{} {}", "", http.StatusBadRequest},
		{"http://127.0.0.1:9000", "", "application/json", "{}", "?sourceDir=/private", http.StatusForbidden},
	} {
		upstream := &recordingTransport{}
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9000/api/viewer/refresh"+scenario.query, strings.NewReader(scenario.body))
		request.Header.Set("Origin", scenario.origin)
		request.Header.Set("Sec-Fetch-Site", scenario.site)
		request.Header.Set("Content-Type", scenario.contentType)
		response := httptest.NewRecorder()
		Handler(upstream, "127.0.0.1").ServeHTTP(response, request)
		if response.Code != scenario.status {
			t.Fatal("refresh policy differs", scenario, response.Code)
		}
		forwarded := upstream.last()
		if scenario.status == http.StatusOK {
			if forwarded == nil || forwarded.URL.Path != "/api/local/v1/capture" || forwarded.URL.RawQuery != "" {
				t.Fatal("refresh did not use default capture")
			}
			data, err := io.ReadAll(forwarded.Body)
			if err != nil || string(data) != "{}" {
				t.Fatal("capture body was not canonical default", err)
			}
		} else if forwarded != nil {
			t.Fatal("rejected mutation reached backend")
		}
	}
}

func TestStartLoopbackMetadataAndClose(t *testing.T) {
	upstream := &recordingTransport{}
	info := endpoint.Info{Transport: "ssh", Target: "ssh://box", State: "connected", Hostname: "verified-box"}
	bridge, err := Start(t.Context(), upstream, Options{Connection: info})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bridge.Close() }()
	response, err := http.Get(bridge.URL + "/api/viewer/connection")
	if err != nil {
		t.Fatal(err)
	}
	var metadata endpoint.Info
	err = json.NewDecoder(response.Body).Decode(&metadata)
	_ = response.Body.Close()
	if err != nil || metadata != info {
		t.Fatal("connection metadata differs", metadata, err)
	}
	if upstream.last() != nil {
		t.Fatal("connection metadata forwarded upstream")
	}
	if err := bridge.Close(); err != nil {
		t.Fatal(err)
	}
	if response, err := http.Get(bridge.URL); err == nil {
		_ = response.Body.Close()
		t.Fatal("closed bridge still serves")
	}
	for _, host := range []string{"0.0.0.0", "192.0.2.1", "example.test", "::"} {
		if invalid, err := Start(t.Context(), upstream, Options{Host: host}); err == nil {
			_ = invalid.Close()
			t.Fatal("nonloopback bridge accepted", host)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if invalid, err := Start(ctx, upstream, Options{}); err == nil {
		_ = invalid.Close()
		t.Fatal("cancelled bridge started")
	}
}

func (t *recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.requests = append(t.requests, r)
	t.mu.Unlock()
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/plain"}}, Body: io.NopCloser(strings.NewReader("ok")), Request: r}, nil
}

func (t *recordingTransport) last() *http.Request {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.requests) == 0 {
		return nil
	}
	return t.requests[len(t.requests)-1]
}

func TestProxyForwardsOnlyReadOnlyDashboardRequests(t *testing.T) {
	upstream := &recordingTransport{}
	handler := Handler(upstream, "127.0.0.1")
	for _, scenario := range []struct {
		method, path, host, origin, site string
		status                           int
		forwarded                        bool
	}{
		{http.MethodGet, "/", "127.0.0.1:9000", "", "", http.StatusOK, true},
		{http.MethodHead, "/api/v2/instance", "localhost:9000", "", "", http.StatusOK, true},
		{http.MethodGet, "/api/v2/usage?period=week", "127.0.0.1:9000", "", "", http.StatusOK, true},
		{http.MethodGet, "/api/local/v1/viewers", "127.0.0.1:9000", "http://127.0.0.1:9000", "same-origin", http.StatusOK, true},
		{http.MethodPost, "/api/v3/ingestion/batches", "127.0.0.1:9000", "", "", http.StatusMethodNotAllowed, false},
		{http.MethodPost, "/api/local/v1/capture", "127.0.0.1:9000", "", "", http.StatusMethodNotAllowed, false},
		{http.MethodGet, "/api/local/v1/progress", "127.0.0.1:9000", "", "", http.StatusNotFound, false},
		{http.MethodGet, "/api/local/v1/descriptor", "127.0.0.1:9000", "", "", http.StatusNotFound, false},
		{http.MethodGet, "/api/local/v1/viewers?host=0.0.0.0&port=80", "127.0.0.1:9000", "", "", http.StatusForbidden, false},
		{http.MethodGet, "/api/local/v1/viewers", "127.0.0.1:9000", "http://evil.test", "cross-site", http.StatusForbidden, false},
		{http.MethodGet, "/api/v2/instance", "evil.test:9000", "", "", http.StatusForbidden, false},
	} {
		t.Run(scenario.method+scenario.path, func(t *testing.T) {
			before := upstream.last()
			r := httptest.NewRequest(scenario.method, "http://"+scenario.host+scenario.path, nil)
			r.Host = scenario.host
			r.Header.Set("Origin", scenario.origin)
			r.Header.Set("Sec-Fetch-Site", scenario.site)
			r.Header.Set("Cookie", "private=1")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != scenario.status {
				t.Fatalf("status = %d, want %d", w.Code, scenario.status)
			}
			forwarded := upstream.last()
			if scenario.forwarded != (forwarded != before) {
				t.Fatal("forwarding differs", scenario.forwarded)
			}
			if !scenario.forwarded {
				return
			}
			if forwarded.Host != upstreamHost || forwarded.URL.Host != upstreamHost || forwarded.Header.Get("Cookie") != "" || forwarded.Header.Get("Origin") != "" || forwarded.URL.RequestURI() != scenario.path {
				t.Fatal("upstream request not sanitized", forwarded.Host, forwarded.URL, forwarded.Header)
			}
		})
	}
}

func TestProxyBroadBindAcceptsRemoteHostButKeepsOriginPolicy(t *testing.T) {
	upstream := &recordingTransport{}
	handler := Handler(upstream, "0.0.0.0")
	r := httptest.NewRequest(http.MethodGet, "http://my-host:9000/api/local/v1/viewers", nil)
	r.Header.Set("Origin", "http://my-host:9000")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatal("broad bind rejected its own origin", w.Code)
	}
	r = httptest.NewRequest(http.MethodGet, "http://my-host:9000/api/local/v1/viewers", nil)
	r.Header.Set("Origin", "http://evil.test")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("broad bind accepted foreign origin", w.Code)
	}
}
