package server

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

const accountBodyMaxBytes = 16 << 10
const accountDatasetHeader = "X-TokenInsights-Dataset-ID"

func (a *app) authInfo(w http.ResponseWriter, _ *http.Request) {
	methods := []api.AuthInfoLoginMethods{}
	if a.policy.Kind == serverfeatures.Hosted {
		methods = append(methods, api.Password)
	}
	writeJSON(w, http.StatusOK, api.AuthInfo{ServerKind: api.AuthInfoServerKind(a.policy.Kind), LoginMethods: methods, SignupEnabled: a.policy.SignupEnabled})
}

func (a *app) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if a.publicURL != "" {
		return origin == a.publicURL
	}
	u, err := url.Parse(origin)
	return err == nil && u != nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host == r.Host && u.User == nil && u.Path == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""
}

func (a *app) requireOrigin(w http.ResponseWriter, r *http.Request) bool {
	if !a.sameOrigin(r) {
		apiError(w, http.StatusForbidden, api.ErrorCodeInvalidRequest, "Invalid origin.")
		return false
	}
	return true
}

func decodeAccountBody(w http.ResponseWriter, r *http.Request, input interface{}) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, accountBodyMaxBytes))
	if err != nil || r.Header.Get("Content-Type") != "application/json" || evidence.StrictDecode(body, input) != nil {
		apiError(w, http.StatusBadRequest, api.ErrorCodeInvalidRequest, "Invalid account request.")
		return false
	}
	return true
}

func (a *app) passwordAdmission(w http.ResponseWriter, r *http.Request) (func(), bool) {
	release, err := a.accounts.AdmitPassword(r.RemoteAddr)
	if err == nil {
		return release, true
	}
	var limit *accounts.AdmissionError
	if errors.As(err, &limit) {
		w.Header().Set("Retry-After", strconv.Itoa(limit.RetryAfter))
	}
	status := http.StatusTooManyRequests
	if limit != nil && limit.Code == "password_busy" {
		status = http.StatusServiceUnavailable
	}
	apiError(w, status, api.ErrorCodeUnavailable, "Authentication temporarily limited.")
	return nil, false
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	a.passwordSession(w, r, false)
}

func (a *app) signup(w http.ResponseWriter, r *http.Request) {
	a.passwordSession(w, r, true)
}

func (a *app) passwordSession(w http.ResponseWriter, r *http.Request, signup bool) {
	if !a.requireOrigin(w, r) {
		return
	}
	release, admitted := a.passwordAdmission(w, r)
	if !admitted {
		return
	}
	defer release()
	var input api.BrowserSessionRequest
	if !decodeAccountBody(w, r, &input) {
		return
	}
	if signup {
		if _, err := a.accounts.Signup(r.Context(), input.Username, input.Password); err != nil {
			accountError(w, err)
			return
		}
	}
	secret, expires, err := a.accounts.CreateSession(r.Context(), input.Username, input.Password)
	if err != nil {
		accountError(w, err)
		return
	}
	setSessionCookie(w, r, sessionCookie(secret, expires))
	w.WriteHeader(http.StatusNoContent)
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, cookie *http.Cookie) {
	cookie.Secure = strings.HasPrefix(r.Header.Get("Origin"), "https://")
	http.SetCookie(w, cookie)
	w.Header().Set("Cache-Control", "no-store")
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if !a.requireOrigin(w, r) {
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if err := a.accounts.Logout(r.Context(), cookie.Value); err != nil {
			accountError(w, err)
			return
		}
	}
	setSessionCookie(w, r, clearSessionCookie())
	w.WriteHeader(http.StatusNoContent)
}

// Account management uses browser credentials only. Bearer scopes authorize data
// access and cannot acquire account-management authority through a mixed request.
func (a *app) accountAccess(handler func(accounts.Principal, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.Header.Values("Authorization")) != 0 {
			apiError(w, http.StatusForbidden, api.ErrorCodeInvalidRequest, "Browser session required.")
			return
		}
		if r.Method != http.MethodGet && !a.requireOrigin(w, r) {
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			accountError(w, accounts.ErrUnauthenticated)
			return
		}
		p, err := a.accounts.AuthenticateSession(r.Context(), cookie.Value)
		if err != nil {
			accountError(w, err)
			return
		}
		expected := r.Header.Values(accountDatasetHeader)
		if len(expected) > 1 || len(expected) == 1 && strings.Contains(expected[0], ",") {
			apiError(w, http.StatusBadRequest, api.ErrorCodeInvalidRequest, "Invalid account binding.")
			return
		}
		if len(expected) == 1 && expected[0] != p.DatasetID {
			apiError(w, http.StatusConflict, api.ErrorCodeInvalidRequest, "Account changed. Sign in again.")
			return
		}
		handler(p, w, r)
	}
}

func (a *app) currentUser(p accounts.Principal, w http.ResponseWriter, r *http.Request) {
	user, err := a.accounts.CurrentUser(r.Context(), p)
	if err != nil {
		accountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (a *app) changePassword(p accounts.Principal, w http.ResponseWriter, r *http.Request) {
	release, admitted := a.passwordAdmission(w, r)
	if !admitted {
		return
	}
	defer release()
	var input api.ChangePasswordRequest
	if !decodeAccountBody(w, r, &input) {
		return
	}
	if err := a.accounts.ChangeOwnPassword(r.Context(), p, input.CurrentPassword, input.Password); err != nil {
		accountError(w, err)
		return
	}
	setSessionCookie(w, r, clearSessionCookie())
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) listKeys(p accounts.Principal, w http.ResponseWriter, r *http.Request) {
	keys, err := a.accounts.ListOwnTokens(r.Context(), p)
	if err != nil {
		accountError(w, err)
		return
	}
	if keys == nil {
		keys = []accounts.TokenInfo{}
	}
	writeJSON(w, http.StatusOK, struct {
		Keys []accounts.TokenInfo `json:"keys"`
	}{Keys: keys})
}

func (a *app) createKey(p accounts.Principal, w http.ResponseWriter, r *http.Request) {
	var input api.CreateAPIKeyRequest
	if !decodeAccountBody(w, r, &input) {
		return
	}
	permissions := make([]string, len(input.Permissions))
	for i, permission := range input.Permissions {
		permissions[i] = string(permission)
	}
	key, err := a.accounts.CreateOwnToken(r.Context(), p, input.Label, permissions, input.ExpiresAt)
	if err != nil {
		accountError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

func (a *app) revokeKey(p accounts.Principal, w http.ResponseWriter, r *http.Request) {
	if err := a.accounts.RevokeOwnToken(r.Context(), p, r.PathValue("keyId")); err != nil {
		accountError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func accountError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, accounts.ErrCurrentPasswordMismatch):
		apiError(w, http.StatusBadRequest, api.ErrorCodeInvalidRequest, "Current password is incorrect.")
	case errors.Is(err, accounts.ErrUnauthenticated):
		apiError(w, http.StatusUnauthorized, api.ErrorCodeInvalidRequest, "Authentication failed.")
	case errors.Is(err, accounts.ErrNotFound):
		apiError(w, http.StatusNotFound, api.ErrorCodeNotFound, "Account resource unavailable.")
	case errors.Is(err, accounts.ErrUsernameTaken):
		apiError(w, http.StatusConflict, api.ErrorCodeInvalidRequest, "Username unavailable.")
	case errors.Is(err, accounts.ErrInvalidPassword), errors.Is(err, accounts.ErrInvalidUsername), errors.Is(err, accounts.ErrInvalidLabel), errors.Is(err, accounts.ErrInvalidPermissions), errors.Is(err, accounts.ErrInvalidExpiry):
		apiError(w, http.StatusBadRequest, api.ErrorCodeInvalidRequest, "Invalid account request.")
	default:
		apiError(w, http.StatusServiceUnavailable, api.ErrorCodeUnavailable, "Account operation unavailable.")
	}
}
