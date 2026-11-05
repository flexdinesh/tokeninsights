package tcptransport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPinnedTLSRejectsBeforeCredentialSubmission(t *testing.T) {
	requests := new(atomic.Int64)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(200) }))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	pin := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	target := endpoint.TCP{Address: server.Listener.Addr().String(), Pin: hex.EncodeToString(pin[:]), Token: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{Protocols: protocols, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return Dialer(target)(ctx) }}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	target.Pin = strings.Repeat("00", 32)
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://gateway/api/local/v1/descriptor", nil)
	request.Header.Set("Authorization", "Bearer "+target.Token)
	if response, err := client.Do(request); err == nil {
		_ = response.Body.Close()
		t.Fatal("invalid pin accepted")
	}
	if requests.Load() != 0 {
		t.Fatal("credential submitted before verifying pin")
	}
	target.Pin = hex.EncodeToString(pin[:])
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("matching pin rejected", err)
	}
	_ = response.Body.Close()
	if requests.Load() != 1 {
		t.Fatal("verified TLS did not carry HTTP")
	}
}

func TestDialerValidatesTypedTargetAndCancellation(t *testing.T) {
	for _, target := range []endpoint.TCP{{}, {Address: "0.0.0.0:1234", Pin: strings.Repeat("00", 32), Token: strings.Repeat("A", 43)}} {
		if conn, err := Dialer(target)(t.Context()); err == nil {
			_ = conn.Close()
			t.Fatal("malformed target accepted")
		}
	}
	// A cancelled context cannot establish even a correctly pinned connection.
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	pin := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	target := endpoint.TCP{Address: server.Listener.Addr().String(), Pin: hex.EncodeToString(pin[:]), Token: strings.Repeat("A", 43)}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if conn, err := Dialer(target)(ctx); err == nil {
		_ = conn.Close()
		t.Fatal("cancelled TLS dial succeeded")
	}
}

func TestDialerRejectsExpiredPinnedIdentity(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-2 * time.Hour), NotAfter: time.Now().Add(-time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("expired identity carried HTTP") }))
	server.EnableHTTP2 = true
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS13}
	server.StartTLS()
	defer server.Close()
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(public)
	target := endpoint.TCP{Address: server.Listener.Addr().String(), Pin: hex.EncodeToString(pin[:]), Token: strings.Repeat("A", 43)}
	if conn, err := Dialer(target)(t.Context()); err == nil {
		_ = conn.Close()
		t.Fatal("expired pinned identity accepted")
	}
}
