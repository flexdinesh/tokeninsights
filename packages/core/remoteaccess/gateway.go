// Package remoteaccess exposes one temporary, scoped TLS viewer gateway.
package remoteaccess

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"io"
	"math/big"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Process closure revokes this ephemeral identity. Avoid an unrelated short
// certificate expiry interrupting a foreground session left open for days.
const certificateValidityYears = 10
const certificateClockSkew = time.Minute
const serialBits = 128
const readHeaderTimeout = 5 * time.Second
const connectionIdleTimeout = time.Minute

type Options struct{ Listen, Advertise, AddressHint string }
type Gateway struct {
	server *http.Server
	target endpoint.Endpoint
	done   chan struct{}
	once   sync.Once
}

func Start(ctx context.Context, session *localclient.Session, options Options) (*Gateway, error) {
	return start(ctx, session, options, connectionIdleTimeout)
}

func start(ctx context.Context, session *localclient.Session, options Options, idleTimeout time.Duration) (*Gateway, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if session == nil || session.Query == nil || session.InstanceID == "" {
		return nil, errors.New("remote gateway requires a live backend session")
	}
	instance, err := session.Query.Instance(ctx)
	if err != nil || instance.InstanceID != session.InstanceID {
		return nil, errors.New("remote gateway backend unavailable")
	}
	certificate, pin, err := identity()
	if err != nil {
		return nil, err
	}
	secret := make([]byte, endpoint.SecretBytes)
	if _, err = rand.Read(secret); err != nil {
		return nil, err
	}
	listen := options.Listen
	if listen == "" {
		listen = "0.0.0.0:0"
	}
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, errors.New("remote gateway listen address unavailable")
	}
	address, err := advertise(listener.Addr(), options.Advertise, options.AddressHint)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	target := endpoint.TCP{Address: address, Pin: pin, Token: base64.RawURLEncoding.EncodeToString(secret)}
	if err = target.Validate(); err != nil {
		_ = listener.Close()
		return nil, err
	}
	gateway := &Gateway{target: endpoint.Endpoint{TCP: &target}, done: make(chan struct{})}
	protocols := new(http.Protocols)
	protocols.SetHTTP2(true)
	gateway.server = &http.Server{
		Handler:   gatewayHandler(session, target.Token),
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, NextProtos: []string{"h2"}},
		Protocols: protocols, ReadHeaderTimeout: readHeaderTimeout, IdleTimeout: idleTimeout, MaxHeaderBytes: 1 << 20,
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = gateway.Close()
		case <-gateway.done:
		}
	}()
	go func() {
		defer close(gateway.done)
		_ = gateway.server.ServeTLS(listener, "", "")
	}()
	go func() {
		select {
		case <-session.Done():
			_ = gateway.Close()
		case <-gateway.done:
		}
	}()
	return gateway, nil
}

// ConnectionString intentionally returns the temporary secret for sharing.
func (g *Gateway) ConnectionString() string { return g.target.ConnectionString() }
func (g *Gateway) Done() <-chan struct{}    { return g.done }
func (g *Gateway) Close() error {
	var err error
	g.once.Do(func() { err = g.server.Close() })
	return err
}

func identity() (tls.Certificate, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), serialBits))
	if err != nil {
		return tls.Certificate{}, "", err
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "TokenInsights temporary gateway"}, NotBefore: time.Now().Add(-certificateClockSkew), NotAfter: time.Now().AddDate(certificateValidityYears, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	pin := sha256.Sum256(public)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, hex.EncodeToString(pin[:]), nil
}

func advertise(bound net.Addr, override, hint string) (string, error) {
	host, port, err := net.SplitHostPort(bound.String())
	if err != nil {
		return "", errors.New("remote gateway has no TCP address")
	}
	if override != "" {
		if h, p, e := net.SplitHostPort(override); e == nil {
			host, port = h, p
		} else {
			host = override
		}
	} else if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		if candidate := net.ParseIP(hint); candidate != nil && !candidate.IsUnspecified() {
			host = candidate.String()
		} else {
			host = advertiseHost()
		}
	}
	ip := net.ParseIP(host)
	if host == "" || host == "*" || ip != nil && ip.IsUnspecified() {
		return "", errors.New("remote advertise must name a dialable host")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("remote advertise requires a valid port")
	}
	return net.JoinHostPort(host, port), nil
}

func advertiseHost() string {
	addresses, err := net.InterfaceAddrs()
	if err == nil {
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.IsGlobalUnicast() && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}

func gatewayHandler(session *localclient.Session, token string) http.Handler {
	proxy := &httputil.ReverseProxy{
		Transport: session.PrivateRoundTripper(), FlushInterval: -1,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme, r.Out.URL.Host, r.Out.Host = "http", "localhost", "localhost"
			for _, header := range []string{"Authorization", "Cookie", "Origin", "Referer", "Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
				r.Out.Header.Del(header)
			}
		},
		ModifyResponse: func(r *http.Response) error { r.Header.Del("Set-Cookie"); return nil },
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "backend_unavailable", http.StatusBadGateway)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.RawPath != "" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/api/local/v1/descriptor" && r.Method == http.MethodGet && r.URL.RawQuery == "" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(struct {
				ProtocolVersion int    `json:"protocolVersion"`
				InstanceID      string `json:"instanceId"`
			}{localprotocol.ProtocolVersion, session.InstanceID})
			return
		}
		if r.Header.Get("X-TokenInsights-Instance") != session.InstanceID {
			http.Error(w, "instance_mismatch", http.StatusConflict)
			return
		}
		allowed := localprotocol.ViewerRead(r)
		switch r.URL.Path {
		case "/api/local/v1/viewers", "/api/local/v1/progress", "/api/local/v1/processing":
			allowed = r.Method == http.MethodGet && r.URL.RawQuery == ""
		case "/api/local/v1/queries/instance", "/api/local/v1/queries/status", "/api/local/v1/queries/usage", "/api/local/v1/queries/all", "/api/local/v1/queries/facets":
			allowed = r.Method == http.MethodPost && r.URL.RawQuery == ""
			r.Body = http.MaxBytesReader(w, r.Body, localprotocol.MaxQueryRequestBytes)
		case "/api/local/v1/capture":
			allowed = r.Method == http.MethodPost && (r.URL.RawQuery == "" || r.URL.RawQuery == "wait=true")
			media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if !allowed || err != nil || media != "application/json" {
				http.Error(w, "default_capture_required", http.StatusBadRequest)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, localprotocol.MaxDefaultCaptureBytes)
			if !localprotocol.DefaultCapture(r.Body) {
				http.Error(w, "default_capture_required", http.StatusBadRequest)
				return
			}
			r = r.Clone(r.Context())
			r.Body = io.NopCloser(strings.NewReader("{}"))
			r.ContentLength = 2
		}
		if !allowed {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})
}
