// Package accounts owns principal policies behind a storage contract.
package accounts

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	Read                = "read"
	Ingest              = "ingest"
	SessionCookieName   = "tokeninsights_session"
	SessionLifetime     = 24 * time.Hour
	credentialBytes     = 32
	maxDisplayNameBytes = 200
)

var ErrUnauthenticated = errors.New("unauthenticated")
var ErrNotFound = errors.New("account_not_found")

type Principal struct {
	UserID      string   `json:"userId"`
	DatasetID   string   `json:"datasetId"`
	Permissions []string `json:"permissions"`
}

func (p Principal) HasPermission(permission string) bool {
	for _, scope := range p.Permissions {
		if scope == permission {
			return true
		}
	}
	return false
}

type User struct {
	UserID      string `json:"userId"`
	DatasetID   string `json:"datasetId"`
	DisplayName string `json:"displayName"`
	Enabled     bool   `json:"enabled"`
}

// Token is returned once at creation. Only its digest is persisted.
type Token struct {
	TokenID     string     `json:"tokenId"`
	Secret      string     `json:"token"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

// Repository owns credential transactions and recoverable user provisioning.
type Repository interface {
	CreateUser(ctx context.Context, name string) (User, error)
	DisableUser(ctx context.Context, id string) error
	CreateToken(ctx context.Context, id string, scopes []string, expires *time.Time) (Token, error)
	RevokeToken(ctx context.Context, id string) error
	AuthenticateBearer(ctx context.Context, secret string) (Principal, error)
	AuthenticateSession(ctx context.Context, secret string) (Principal, error)
	CreateSession(ctx context.Context, secret string) (string, time.Time, error)
	Logout(ctx context.Context, secret string) error
	Cleanup(ctx context.Context) error
	ReprocessUser(ctx context.Context, id string) (int64, error)
}
type Service struct {
	repository Repository
	admission  *admission
	login      *loginAdmission
}

func New(repository Repository) *Service {
	return &Service{repository: repository, admission: newAdmission(), login: newLoginAdmission()}
}
func (s *Service) CreateUser(ctx context.Context, name string) (User, error) {
	return s.repository.CreateUser(ctx, name)
}
func (s *Service) DisableUser(ctx context.Context, id string) error {
	return s.repository.DisableUser(ctx, id)
}
func (s *Service) CreateToken(ctx context.Context, id string, scopes []string, expires *time.Time) (Token, error) {
	return s.repository.CreateToken(ctx, id, scopes, expires)
}
func (s *Service) RevokeToken(ctx context.Context, id string) error {
	return s.repository.RevokeToken(ctx, id)
}
func (s *Service) AuthenticateBearer(ctx context.Context, secret string) (Principal, error) {
	return s.repository.AuthenticateBearer(ctx, secret)
}
func (s *Service) AuthenticateSession(ctx context.Context, secret string) (Principal, error) {
	return s.repository.AuthenticateSession(ctx, secret)
}
func (s *Service) CreateSession(ctx context.Context, secret string) (string, time.Time, error) {
	return s.repository.CreateSession(ctx, secret)
}
func (s *Service) Logout(ctx context.Context, secret string) error {
	return s.repository.Logout(ctx, secret)
}
func (s *Service) Cleanup(ctx context.Context) error { return s.repository.Cleanup(ctx) }
func (s *Service) ReprocessUser(ctx context.Context, id string) (int64, error) {
	return s.repository.ReprocessUser(ctx, id)
}
func (s *Service) AuthenticateRequest(ctx context.Context, r *http.Request) (Principal, error) {
	if header := r.Header.Get("Authorization"); header != "" {
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return Principal{}, ErrUnauthenticated
		}
		return s.AuthenticateBearer(ctx, parts[1])
	}
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	return s.AuthenticateSession(ctx, cookie.Value)
}

func SessionCookie(secret string, expires time.Time) *http.Cookie {
	return &http.Cookie{Name: SessionCookieName, Value: secret, Path: "/", Expires: expires, MaxAge: int(SessionLifetime / time.Second), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}

func ClearSessionCookie() *http.Cookie {
	c := SessionCookie("", time.Unix(1, 0))
	c.MaxAge = -1
	return c
}
