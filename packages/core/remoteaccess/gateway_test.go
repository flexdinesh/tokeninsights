package remoteaccess

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/http2"

	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardproxy"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/tcptransport"
)

// The fixture speaks the real private HTTP/2 session protocol. TLS, HTTP,
// reverse proxies and session leases all use real listeners and connections.
func backendSession(t *testing.T) (*localclient.Session, *atomic.Int64, func()) {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "backend.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	captures := new(atomic.Int64)
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{Protocols: protocols, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const instance = "fixed-backend"
		if r.URL.Path == "/api/local/v1/descriptor" {
			_ = json.NewEncoder(w).Encode(localprotocol.Descriptor{ProtocolVersion: localprotocol.ProtocolVersion, InstanceID: instance, Settings: localprotocol.Settings{CollectorDBPath: "/private/collector.sqlite", ServerDBPath: "/private/server.sqlite"}})
			return
		}
		if r.Header.Get("X-TokenInsights-Instance") != instance {
			http.Error(w, "instance_changed", http.StatusConflict)
			return
		}
		switch r.URL.Path {
		case "/api/local/v1/viewers":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, ": viewer\n\n")
			_ = http.NewResponseController(w).Flush()
			<-r.Context().Done()
		case "/api/local/v1/queries/instance", "/api/v2/instance":
			_ = json.NewEncoder(w).Encode(application.Instance{InstanceID: instance, Hostname: "serving-machine"})
		case "/api/local/v1/capture":
			if !localprotocol.DefaultCapture(r.Body) {
				t.Error("nondefault capture reached backend")
				http.Error(w, "invalid", 400)
				return
			}
			captures.Add(1)
			if r.URL.RawQuery == "wait=true" {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"accepted\":7}\n\n")
			} else {
				w.WriteHeader(http.StatusAccepted)
			}
		case "/api/local/v1/progress", "/api/local/v1/processing", "/api/local/v1/queries/status", "/api/v2/status":
			_, _ = io.WriteString(w, "{}")
		case "/":
			_, _ = io.WriteString(w, "dashboard")
		default:
			t.Error("unexpected forwarded operation", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	stop := func() { _ = server.Close() }
	t.Cleanup(stop)
	session, err := localclient.ConnectRemote(t.Context(), localclient.SocketDialer(socket), localclient.RemoteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, captures, stop
}

func startGateway(t *testing.T, session *localclient.Session) *Gateway {
	t.Helper()
	gateway, err := Start(t.Context(), session, Options{Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gateway.Close() })
	return gateway
}

func parseGateway(t *testing.T, gateway *Gateway) endpoint.TCP {
	t.Helper()
	selected, err := endpoint.Parse(gateway.ConnectionString())
	if err != nil || selected.TCP == nil || selected.IsLocal() {
		t.Fatal("invalid TCP connection string", err)
	}
	return *selected.TCP
}

func rawClient(target endpoint.TCP) *http.Client {
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{Protocols: protocols, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return tcptransport.Dialer(target)(ctx) }}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

func call(t *testing.T, client *http.Client, target endpoint.TCP, method, path, body string, authenticate bool) (int, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, "http://gateway"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-TokenInsights-Instance", "fixed-backend")
	request.Header.Set("Content-Type", "application/json")
	if authenticate {
		request.Header.Set("Authorization", "Bearer "+target.Token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(data)
}

func TestGatewayTLSAuthenticationScopeAndSafeDescriptor(t *testing.T) {
	local, captures, _ := backendSession(t)
	gateway := startGateway(t, local)
	target := parseGateway(t, gateway)
	client := rawClient(target)
	defer client.CloseIdleConnections()
	for _, path := range []string{"/api/local/v1/descriptor", "/api/local/v1/progress", "/api/v2/instance", "/api/local/v1/capture"} {
		status, _ := call(t, client, target, http.MethodGet, path, "", false)
		if status != http.StatusUnauthorized {
			t.Fatal("unauthenticated request reached route", path, status)
		}
	}
	status, body := call(t, client, target, http.MethodGet, "/api/local/v1/descriptor", "", true)
	if status != 200 || strings.Contains(body, "/private") || strings.Contains(body, target.Token) {
		t.Fatal("unsafe descriptor", status)
	}
	var descriptor localprotocol.Descriptor
	if err := json.Unmarshal([]byte(body), &descriptor); err != nil || descriptor.Settings != (localprotocol.Settings{}) || descriptor.InstanceID != local.InstanceID {
		t.Fatal("wrong descriptor", err)
	}

	// A TCP target served on this machine stays on the authenticated TLS path.
	remote, err := localclient.ConnectRemote(t.Context(), tcptransport.Dialer(target), localclient.RemoteOptions{Token: target.Token, Capture: true})
	if err != nil {
		t.Fatal("same-machine TCP failed", err)
	}
	defer func() { _ = remote.Close() }()
	instance, err := remote.Query.Instance(t.Context())
	if err != nil || instance.Hostname != "serving-machine" || remote.InstanceID != local.InstanceID {
		t.Fatal("remote identity mismatch", err)
	}
	if _, err := remote.Progress(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.ProcessingStatus(t.Context()); err != nil {
		t.Fatal(err)
	}
	result, err := remote.Collect(t.Context(), false, "")
	if err != nil || result.Accepted != 7 || captures.Load() != 2 {
		t.Fatal("default capture parity failed", result.Accepted, captures.Load(), err)
	}
	if err := remote.Reprocess(t.Context()); err == nil {
		t.Fatal("viewer credential authorized maintenance")
	}
	if err := remote.WaitVisible(t.Context()); err == nil {
		t.Fatal("viewer credential authorized private wait")
	}
	for _, scenario := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v3/ingestion/batches", "{}"},
		{http.MethodPost, "/api/local/v1/reprocess", "{}"},
		{http.MethodPost, "/api/local/v1/wait", "{}"},
		{http.MethodGet, "/api/local/v1/viewers?host=0.0.0.0&port=80", ""},
		{http.MethodPost, "/api/local/v1/capture", `{"sourceDir":"/private"}`},
		{http.MethodPost, "/api/local/v1/capture", `{"fullRefresh":true}`},
		{http.MethodPost, "/api/local/v1/capture", `{"fullRefresh":true,"fullRefresh":false}`},
		{http.MethodPost, "/api/local/v1/capture", `{"token":"override"}`},
		{http.MethodPost, "/api/local/v1/capture", "null"},
		{http.MethodGet, "/arbitrary", ""},
		{http.MethodGet, "/api/v2/account", ""},
		{http.MethodGet, "/assets/../api/v2/status", ""},
	} {
		status, _ := call(t, client, target, scenario.method, scenario.path, scenario.body, true)
		if status < 400 {
			t.Fatal("gateway allowed unsafe route", scenario.path)
		}
	}
	if captures.Load() != 2 {
		t.Fatal("rejected captures changed backend")
	}
	// A caller cannot select another backend on the gateway's connection.
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://gateway/api/v2/instance", nil)
	request.Header.Set("Authorization", "Bearer "+target.Token)
	request.Header.Set("X-TokenInsights-Instance", "other")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatal("gateway ignored backend pin")
	}
}

func TestGatewayInvalidPinTokenExpiryAndDisconnect(t *testing.T) {
	local, _, stopBackend := backendSession(t)
	gateway := startGateway(t, local)
	target := parseGateway(t, gateway)
	badPin := target
	badPin.Pin = strings.Repeat("00", 32)
	if session, err := localclient.ConnectRemote(t.Context(), tcptransport.Dialer(badPin), localclient.RemoteOptions{Token: target.Token}); err == nil {
		_ = session.Close()
		t.Fatal("invalid pin accepted")
	} else if strings.Contains(err.Error(), target.Token) {
		t.Fatal("pin error leaked token")
	}
	badToken := target
	badToken.Token = strings.Repeat("A", 43)
	if session, err := localclient.ConnectRemote(t.Context(), tcptransport.Dialer(target), localclient.RemoteOptions{Token: badToken.Token}); err == nil {
		_ = session.Close()
		t.Fatal("invalid token accepted")
	}
	session, err := localclient.ConnectRemote(t.Context(), tcptransport.Dialer(target), localclient.RemoteOptions{Token: target.Token})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if err := gateway.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gateway.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("gateway did not stop")
	}
	select {
	case <-session.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("gateway left accepted session alive")
	}
	if _, err := session.Query.Instance(t.Context()); err == nil {
		t.Fatal("expired session reconnected")
	}
	if expired, err := localclient.ConnectRemote(t.Context(), tcptransport.Dialer(target), localclient.RemoteOptions{Token: target.Token}); err == nil {
		_ = expired.Close()
		t.Fatal("expired connection accepted")
	}
	if _, err := local.Query.Instance(t.Context()); err != nil {
		t.Fatal("gateway close stopped local viewer", err)
	}
	replacement := startGateway(t, local)
	fresh := parseGateway(t, replacement)
	if fresh.Pin == target.Pin || fresh.Token == target.Token {
		t.Fatal("gateway reused identity or credential")
	}
	stopBackend()
	select {
	case <-replacement.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("gateway survived backend disconnect")
	}
}

func TestGatewayBridgeMetadataRefreshAndIndependentLifetime(t *testing.T) {
	local, captures, _ := backendSession(t)
	gateway := startGateway(t, local)
	target := parseGateway(t, gateway)
	session, err := localclient.ConnectRemote(t.Context(), tcptransport.Dialer(target), localclient.RemoteOptions{Token: target.Token})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	info := endpoint.Info{Transport: "tcp", Target: endpoint.Endpoint{TCP: &target}.String(), State: "connected", Hostname: "serving-machine"}
	bridge, err := dashboardproxy.Start(t.Context(), session.PrivateRoundTripper(), dashboardproxy.Options{Connection: info})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bridge.Close() }()
	response, err := http.Get(bridge.URL + "/api/viewer/connection")
	if err != nil {
		t.Fatal(err)
	}
	var result endpoint.Info
	err = json.NewDecoder(response.Body).Decode(&result)
	_ = response.Body.Close()
	if err != nil || result != info || strings.Contains(result.Target, target.Token) {
		t.Fatal("bridge metadata leaked or changed", err)
	}
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, bridge.URL+"/api/viewer/refresh", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", bridge.URL)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted || captures.Load() != 1 {
		t.Fatal("bridge did not refresh remote backend", response.StatusCode, captures.Load())
	}
	if err := bridge.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Query.Instance(t.Context()); err != nil {
		t.Fatal("bridge close owned caller session", err)
	}
}

func TestAdvertiseSelectionAndWildcardRejection(t *testing.T) {
	bound := &net.TCPAddr{IP: net.IPv4zero, Port: 12345}
	value, err := advertise(bound, "", "192.0.2.10")
	if err != nil || value != "192.0.2.10:12345" {
		t.Fatal("SSH server address not advertised", value, err)
	}
	for override, want := range map[string]string{"example.test": "example.test:12345", "example.test:4321": "example.test:4321", "::1": "[::1]:12345"} {
		got, err := advertise(bound, override, "192.0.2.10")
		if err != nil || got != want {
			t.Fatal("advertise override failed", got, want, err)
		}
	}
	local, _, _ := backendSession(t)
	for _, override := range []string{"0.0.0.0", "[::]:12345", "*", "host:0", "host:bad"} {
		gateway, err := Start(t.Context(), local, Options{Listen: "127.0.0.1:0", Advertise: override})
		if err == nil {
			_ = gateway.Close()
			t.Fatal("invalid advertisement accepted", override)
		}
	}
}

func TestDefaultNetworkBindProducesDialableExplicitAdvertisement(t *testing.T) {
	local, _, _ := backendSession(t)
	gateway, err := Start(t.Context(), local, Options{Advertise: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gateway.Close() }()
	target := parseGateway(t, gateway)
	host, port, err := net.SplitHostPort(target.Address)
	if err != nil || host != "127.0.0.1" || port == "0" {
		t.Fatal("default bind emitted invalid advertisement", target.Address, err)
	}
	session, err := localclient.ConnectRemote(t.Context(), tcptransport.Dialer(target), localclient.RemoteOptions{Token: target.Token})
	if err != nil {
		t.Fatal("default network bind unavailable on serving machine", err)
	}
	_ = session.Close()
}

func TestIdleUnauthenticatedHTTP2ClosesWithoutInterruptingViewerLease(t *testing.T) {
	local, _, _ := backendSession(t)
	gateway, err := start(t.Context(), local, Options{Listen: "127.0.0.1:0"}, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gateway.Close() }()
	target := parseGateway(t, gateway)
	viewer, err := localclient.ConnectRemote(t.Context(), tcptransport.Dialer(target), localclient.RemoteOptions{Token: target.Token})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = viewer.Close() }()
	// This second peer completes TLS and HTTP/2 setup but never authenticates
	// or opens a stream. It must lose its idle connection while the viewer lives.
	idle, err := tcptransport.Dialer(target)(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = idle.Close() }()
	if err := idle.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(idle, http2.ClientPreface); err != nil {
		t.Fatal(err)
	}
	frames := http2.NewFramer(idle, idle)
	if err := frames.WriteSettings(); err != nil {
		t.Fatal(err)
	}
	graceful := false
	for {
		frame, err := frames.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal("idle peer was not closed gracefully before the deadline", err)
		}
		switch frame := frame.(type) {
		case *http2.SettingsFrame:
			if !frame.IsAck() {
				if err := frames.WriteSettingsAck(); err != nil {
					t.Fatal(err)
				}
			}
		case *http2.GoAwayFrame:
			if frame.ErrCode != http2.ErrCodeNo {
				t.Fatal("HTTP/2 ended for a reason other than idle expiry", frame.ErrCode)
			}
			graceful = true
		}
	}
	if !graceful {
		t.Fatal("idle connection closed without graceful HTTP/2 shutdown")
	}
	if _, err := viewer.Query.Instance(t.Context()); err != nil {
		t.Fatal("idle expiry interrupted an authenticated lease", err)
	}
}
