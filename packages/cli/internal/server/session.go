package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
)

const sessionCookieName = "tokeninsights_session"

type credentialAuthenticator interface {
	AuthenticateBearer(context.Context, string) (accounts.Principal, error)
	AuthenticateSession(context.Context, string) (accounts.Principal, error)
}

// authenticateRequest prefers an Authorization header; an invalid bearer never
// falls back to a session cookie.
func authenticateRequest(ctx context.Context, authenticator credentialAuthenticator, r *http.Request) (accounts.Principal, error) {
	if header := r.Header.Get("Authorization"); header != "" {
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return accounts.Principal{}, accounts.ErrUnauthenticated
		}
		return authenticator.AuthenticateBearer(ctx, parts[1])
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return accounts.Principal{}, accounts.ErrUnauthenticated
	}
	return authenticator.AuthenticateSession(ctx, cookie.Value)
}

func sessionCookie(secret string, expires time.Time) *http.Cookie {
	return &http.Cookie{Name: sessionCookieName, Value: secret, Path: "/", Expires: expires, MaxAge: int(accounts.SessionLifetime / time.Second), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}

func clearSessionCookie() *http.Cookie {
	c := sessionCookie("", time.Unix(1, 0))
	c.MaxAge = -1
	return c
}
