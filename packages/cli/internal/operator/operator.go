// Package operator owns the private owner-socket protocol for hosted account
// administration. It must never be mounted on a public listener.
package operator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
)

const (
	maxBody      = 4096
	accountsPath = "/control/v1/accounts"
)

// Request is one administrative operation sent to the running owner process.
type Request struct {
	Operation   string     `json:"operation"`
	Password    string     `json:"password,omitempty"`
	DisplayName string     `json:"displayName,omitempty"`
	UserID      string     `json:"userId,omitempty"`
	TokenID     string     `json:"tokenId,omitempty"`
	Permissions []string   `json:"permissions,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

// Handler must only be mounted on the private mode-0600 operator socket.
func Handler(s *accounts.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+accountsPath, func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
		decoder.DisallowUnknownFields()
		var request Request
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid_request", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(interface{})); err != io.EOF {
			http.Error(w, "invalid_request", http.StatusBadRequest)
			return
		}
		var result interface{} = struct{}{}
		var err error
		switch request.Operation {
		case "create-user":
			result, err = s.CreateUser(r.Context(), request.DisplayName)
		case "reset-password":
			err = s.SetPassword(r.Context(), request.UserID, request.Password)
		case "disable-user":
			err = s.DisableUser(r.Context(), request.UserID)
		case "create-token":
			result, err = s.CreateToken(r.Context(), request.UserID, request.Permissions, request.ExpiresAt)
		case "revoke-token":
			err = s.RevokeToken(r.Context(), request.TokenID)
		case "reprocess-user":
			var generation int64
			generation, err = s.ReprocessUser(r.Context(), request.UserID)
			result = struct {
				Generation int64 `json:"generation"`
			}{generation}
		default:
			err = errors.New("invalid_operation")
		}
		if err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, accounts.ErrNotFound) {
				code = http.StatusNotFound
			}
			http.Error(w, "account_operation_failed", code)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(result)
	})
	return mux
}

// Call talks to the owner process; provisioning never reopens its DB.
func Call(ctx context.Context, socket string, request Request, output io.Writer) error {
	if socket == "" {
		return errors.New("--admin-socket required")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://operator"+accountsPath, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	response, err := client.Do(r)
	if err != nil {
		return fmt.Errorf("admin server unavailable: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("account operation failed (%d)", response.StatusCode)
	}
	_, err = io.Copy(output, io.LimitReader(response.Body, maxBody))
	return err
}
