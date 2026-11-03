// Package dashboardproxy serves a remote personal backend's read-only browser
// dashboard on a local listener. It forwards only safe dashboard reads and
// browser leases over an injected private round tripper; it never constructs
// storage, discovers backends or falls back to local data.
package dashboardproxy

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

const viewersPath = "/api/local/v1/viewers"

// upstreamHost satisfies the remote dashboard's loopback host policy; the
// proxy has already applied its own bind host policy to the browser request.
const upstreamHost = "localhost"

// Handler forwards GET/HEAD dashboard and API v2 reads plus browser lease
// streams to private. bindHost is the local listener's address for host and
// origin policy. Every other method or private control path is rejected.
func Handler(private http.RoundTripper, bindHost string) http.Handler {
	proxy := &httputil.ReverseProxy{
		Transport: private,
		// Lease streams and progress must reach the browser immediately.
		FlushInterval: -1,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme, r.Out.URL.Host, r.Out.Host = "http", upstreamHost, upstreamHost
			r.Out.Header.Del("Cookie")
			r.Out.Header.Del("Authorization")
			r.Out.Header.Del("Origin")
			r.Out.Header.Del("Referer")
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "remote_unavailable", http.StatusBadGateway)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method_not_allowed", http.StatusMethodNotAllowed)
			return
		}
		if !localprotocol.HostAllowed(r, bindHost) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		switch {
		case r.URL.Path == viewersPath:
			if r.URL.RawQuery != "" || !localprotocol.ViewerRequestAllowed(r, bindHost) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			// A browser lease outlives ordinary request deadlines. Writers
			// without deadline support impose none.
			if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
				http.Error(w, "stream_unavailable", http.StatusInternalServerError)
				return
			}
		case strings.HasPrefix(r.URL.Path, "/api/local/"):
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})
}
