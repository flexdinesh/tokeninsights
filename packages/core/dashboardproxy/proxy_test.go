package dashboardproxy

import (
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
