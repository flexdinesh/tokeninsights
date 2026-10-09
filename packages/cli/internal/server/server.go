// Package server serves the embedded web dashboard and canonical analytics API.
package server

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/networkprefs"
	serverapi "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

const DefaultPort = networkprefs.DefaultPort
const logIndent = "  "
const queryTimeout = 30 * time.Second

//go:embed static
var assets embed.FS

type Options struct {
	Defaults   viewer.Selection
	InstanceID string
	Hostname   string
}

type app struct {
	data           *datastore.Store
	queries        analytics.Repository
	allowIngestion bool
	policy         serverfeatures.Policy
	accounts       *accounts.Service
	publicURL      string
	progress       *collectorprogress.Registry
	options        Options
	ctx            context.Context
	log            io.Writer
}

func newApp(ctx context.Context, options Options, log io.Writer) *app {
	if options.InstanceID == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			panic(err)
		}
		options.InstanceID = hex.EncodeToString(id[:])
	}
	policy, _ := serverfeatures.New(serverfeatures.Personal, false)
	return &app{options: options, ctx: ctx, log: log, policy: policy}
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func apiError(w http.ResponseWriter, status int, code serverapi.ErrorCode, message string) {
	writeJSON(w, status, serverapi.ErrorResponse{Code: code, Message: message})
}

func apiNotFound(w http.ResponseWriter) {
	apiError(w, http.StatusNotFound, serverapi.ErrorCodeNotFound, "Unknown API route.")
}

func isDashboardRoute(path string) bool {
	switch path {
	case "/tokens", "/models", "/providers", "/harnesses", "/sessions", "/context", "/repo":
		return true
	default:
		return false
	}
}

func (a *app) instanceHostname(source string) string {
	if a.options.Hostname != "" {
		return a.options.Hostname
	}
	return source
}

func (a *app) queryError(w http.ResponseWriter, err error) {
	_, _ = fmt.Fprintf(a.log, "%sdashboard query failed: %v\n", logIndent, err)
	apiError(w, http.StatusServiceUnavailable, serverapi.ErrorCodeUnavailable, "Cannot read saved usage. Check server storage and reload.")
}

// Local compositions expose writes only through their private control socket.
func NewDataHandler(ctx context.Context, store *datastore.Store, log io.Writer, bindHost, instance string, allowIngestion bool) http.Handler {
	policy, _ := serverfeatures.New(serverfeatures.Personal, false)
	return NewDataHandlerWithOptions(ctx, store, log, DataHandlerOptions{Host: bindHost, InstanceID: instance, AllowIngestion: allowIngestion, Policy: policy})
}
func protectHandler(handler http.Handler, bindHost string) http.Handler {
	handler = http.NewCrossOriginProtection().Handler(handler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, err := netip.ParseAddr(bindHost)
		if bindHost == "" || err == nil && ip.IsLoopback() {
			host, _, err := net.SplitHostPort(r.Host)
			if err != nil {
				host = r.Host
			}
			address, ipErr := netip.ParseAddr(host)
			if host != "localhost" && (ipErr != nil || !address.IsLoopback()) {
				apiError(w, 403, serverapi.ErrorCodeInvalidRequest, "Invalid dashboard host.")
				return
			}
		}
		handler.ServeHTTP(w, r)
	})
}
