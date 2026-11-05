// Package tcptransport dials temporary TLS gateways using their pinned public key.
package tcptransport

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"net"
	"time"
)

// Dialer verifies the pinned SPKI before returning a connection on which a
// caller may submit credentials. It never selects another transport.
func Dialer(target endpoint.TCP) func(context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) {
		if err := target.Validate(); err != nil {
			return nil, err
		}
		expected, _ := hex.DecodeString(target.Pin)
		config := &tls.Config{
			MinVersion: tls.VersionTLS13,
			NextProtos: []string{"h2"},
			// The self-signed per-run identity is authenticated by the exact SPKI pin.
			InsecureSkipVerify: true,
			VerifyConnection: func(state tls.ConnectionState) error {
				if len(state.PeerCertificates) != 1 || state.NegotiatedProtocol != "h2" {
					return errors.New("TCP gateway identity rejected")
				}
				certificate := state.PeerCertificates[0]
				now := time.Now()
				if now.Before(certificate.NotBefore) || now.After(certificate.NotAfter) {
					return errors.New("TCP gateway certificate expired")
				}
				pin := sha256.Sum256(certificate.RawSubjectPublicKeyInfo)
				if subtle.ConstantTimeCompare(pin[:], expected) != 1 {
					return errors.New("TCP gateway key pin mismatch")
				}
				return nil
			},
		}
		conn, err := (&tls.Dialer{Config: config}).DialContext(ctx, "tcp", target.Address)
		if err != nil {
			return nil, errors.New("TCP gateway unavailable or key pin rejected")
		}
		return conn, nil
	}
}
