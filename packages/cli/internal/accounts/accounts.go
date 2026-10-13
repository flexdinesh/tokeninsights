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
	Read                                 = "read"
	Ingest                               = "ingest"
	SessionCookieName                    = "tokeninsights_session"
	SessionLifetime                      = 24 * time.Hour
	CredentialBytes                      = 32
	MaxTokenBytes                        = 4096
	MaxDisplayNameBytes                  = 200
	MaxKeyLabelCharacters                = 200
	BearerCredential      CredentialKind = "bearer"
	SessionCredential     CredentialKind = "session"
	LocalCredential       CredentialKind = "local"
)

var ErrUnauthenticated = errors.New("unauthenticated")
var ErrNotFound = errors.New("account_not_found")

type CredentialKind string

type Principal struct {
	UserID         string         `json:"userId"`
	DatasetID      string         `json:"datasetId"`
	Permissions    []string       `json:"permissions"`
	CredentialKind CredentialKind `json:"-"`
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
	Username    string `json:"username"`
	Enabled     bool   `json:"enabled"`
}

// Token is returned once at creation. Only its digest is persisted.
type Token struct {
	TokenID     string     `json:"tokenId"`
	Secret      string     `json:"token"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

type TokenInfo struct {
	TokenID     string     `json:"tokenId"`
	Label       string     `json:"label"`
	Permissions []string   `json:"permissions"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
}

// Repository owns credential transactions and recoverable user provisioning.
// Active users have a durable dataset; failed provisioning retains a fixed pending
// identity. Secrets persist only as digests. Authentication checks user enablement,
// expiry and revocation on every call. Password sessions are independent of API keys. Sessions
// grant read only; token rotation cannot change a user's dataset. Account and token
// stores need no shared transaction even when one database implements both.
type Repository interface {
	BootstrapUser(ctx context.Context, username, password string) error
	Signup(ctx context.Context, username, password string) (User, error)
	User(ctx context.Context, userID string) (User, error)
	SetPassword(ctx context.Context, userID, password string) error
	ChangePassword(ctx context.Context, userID, current, password string) error
	CreateNamedToken(ctx context.Context, userID, label string, scopes []string, expires *time.Time) (Token, error)
	ListTokens(ctx context.Context, userID string) ([]TokenInfo, error)
	RevokeUserToken(ctx context.Context, userID, tokenID string) error
	CreateUser(ctx context.Context, name string) (User, error)
	DisableUser(ctx context.Context, id string) error
	CreateToken(ctx context.Context, id string, scopes []string, expires *time.Time) (Token, error)
	RevokeToken(ctx context.Context, id string) error
	AuthenticateBearer(ctx context.Context, secret string) (Principal, error)
	AuthenticateSession(ctx context.Context, secret string) (Principal, error)
	CreateSession(ctx context.Context, username, password string) (string, time.Time, error)
	Logout(ctx context.Context, secret string) error
	Cleanup(ctx context.Context) error
	ReprocessUser(ctx context.Context, id string) (int64, error)
}
type Service struct {
	repository    Repository
	admission     *admission
	login         *loginAdmission
	passwordSlots chan struct{}
}

func New(repository Repository) *Service {
	return &Service{repository: repository, admission: newAdmission(), login: newLoginAdmission(), passwordSlots: make(chan struct{}, MaxConcurrentPasswords)}
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
func (s *Service) CreateSession(ctx context.Context, username, password string) (string, time.Time, error) {
	return s.repository.CreateSession(ctx, username, password)
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

func (s *Service) Signup(ctx context.Context, username, password string) (User, error) {
	return s.repository.Signup(ctx, username, password)
}
func (s *Service) SetPassword(ctx context.Context, userID, password string) error {
	return s.repository.SetPassword(ctx, userID, password)
}
func (s *Service) CurrentUser(ctx context.Context, p Principal) (User, error) {
	if p.CredentialKind != SessionCredential {
		return User{}, ErrUnauthenticated
	}
	return s.repository.User(ctx, p.UserID)
}
func (s *Service) ChangeOwnPassword(ctx context.Context, p Principal, current, password string) error {
	if p.CredentialKind != SessionCredential {
		return ErrUnauthenticated
	}
	return s.repository.ChangePassword(ctx, p.UserID, current, password)
}
func (s *Service) CreateOwnToken(ctx context.Context, p Principal, label string, scopes []string, expires *time.Time) (Token, error) {
	if p.CredentialKind != SessionCredential {
		return Token{}, ErrUnauthenticated
	}
	return s.repository.CreateNamedToken(ctx, p.UserID, label, scopes, expires)
}
func (s *Service) ListOwnTokens(ctx context.Context, p Principal) ([]TokenInfo, error) {
	if p.CredentialKind != SessionCredential {
		return nil, ErrUnauthenticated
	}
	return s.repository.ListTokens(ctx, p.UserID)
}
func (s *Service) RevokeOwnToken(ctx context.Context, p Principal, id string) error {
	if p.CredentialKind != SessionCredential {
		return ErrUnauthenticated
	}
	return s.repository.RevokeUserToken(ctx, p.UserID, id)
}
