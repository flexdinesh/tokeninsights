package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
)

type stubAuthenticator struct{ bearer, session string }

func (s stubAuthenticator) AuthenticateBearer(_ context.Context, secret string) (accounts.Principal, error) {
	if secret != s.bearer {
		return accounts.Principal{}, accounts.ErrUnauthenticated
	}
	return accounts.Principal{UserID: "bearer", CredentialKind: accounts.BearerCredential}, nil
}

func (s stubAuthenticator) AuthenticateSession(_ context.Context, secret string) (accounts.Principal, error) {
	if secret != s.session {
		return accounts.Principal{}, accounts.ErrUnauthenticated
	}
	return accounts.Principal{UserID: "session", CredentialKind: accounts.SessionCredential}, nil
}

func TestSessionCookieAndRequestAuthentication(t *testing.T) {
	auth := stubAuthenticator{bearer: "key", session: "cookie"}
	cookie := sessionCookie("cookie", time.Now().Add(time.Hour))
	if !cookie.Secure || !cookie.HttpOnly || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe session cookie", cookie)
	}
	request := httptest.NewRequest("GET", "https://usage.example/api/v2/instance", nil)
	if _, err := authenticateRequest(t.Context(), auth, request); !errors.Is(err, accounts.ErrUnauthenticated) {
		t.Fatal("anonymous request authenticated", err)
	}
	request.AddCookie(cookie)
	if p, err := authenticateRequest(t.Context(), auth, request); err != nil || p.UserID != "session" {
		t.Fatal(p, err)
	}
	request.Header.Set("Authorization", "Bearer key")
	if p, err := authenticateRequest(t.Context(), auth, request); err != nil || p.UserID != "bearer" {
		t.Fatal("bearer did not take precedence", p, err)
	}
	for _, header := range []string{"Bearer invalid", "Basic key", "Bearer key extra"} {
		request.Header.Set("Authorization", header)
		if _, err := authenticateRequest(t.Context(), auth, request); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatalf("%q fell back to session: %v", header, err)
		}
	}
	if cleared := clearSessionCookie(); cleared.MaxAge >= 0 || cleared.Value != "" {
		t.Fatal("session cookie not cleared", cleared)
	}
}
