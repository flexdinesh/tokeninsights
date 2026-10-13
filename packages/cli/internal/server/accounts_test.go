package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

func accountJSON(t *testing.T, value interface{}) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func loginHostedUser(t *testing.T, fixture hostedContractFixture, username string) *http.Cookie {
	t.Helper()
	body := accountJSON(t, api.BrowserSessionRequest{Username: username, Password: hostedTestPassword})
	response := hostedRequest(t, fixture.handler, http.MethodPost, "/api/v2/auth/session", "", body, nil)
	requireHostedStatus(t, response, http.StatusNoContent)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing browser cookie", cookies)
	}
	return cookies[0]
}

func TestAuthenticationDiscoveryAndPersonalAccountPolicy(t *testing.T) {
	fixture := newHostedContractFixture(t)
	response := hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/auth", "", nil, nil)
	requireHostedStatus(t, response, http.StatusOK)
	auth := hostedDecode[api.AuthInfo](t, response)
	if auth.ServerKind != "hosted" || len(auth.LoginMethods) != 1 || auth.LoginMethods[0] != "password" || auth.SignupEnabled || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("hosted authentication policy", auth, response.Header())
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodPost, "/api/v2/auth/signup", "", []byte(`{"username":"charlie","password":"a-long-test-password"}`), nil), http.StatusNotFound)
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "personal.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	handler := NewDataHandler(t.Context(), sqlanalytics.Source{Store: store}, io.Discard, "0.0.0.0", "local", false)
	response = hostedRequest(t, handler, http.MethodGet, "/api/v2/auth", "", nil, nil)
	requireHostedStatus(t, response, http.StatusOK)
	auth = hostedDecode[api.AuthInfo](t, response)
	if auth.ServerKind != "personal" || len(auth.LoginMethods) != 0 || auth.SignupEnabled {
		t.Fatal("personal authentication policy", auth)
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/v2/auth/session"}, {http.MethodDelete, "/api/v2/auth/session"},
		{http.MethodPost, "/api/v2/auth/signup"}, {http.MethodGet, "/api/v2/account"},
		{http.MethodPut, "/api/v2/account/password"}, {http.MethodGet, "/api/v2/account/keys"},
		{http.MethodPost, "/api/v2/account/keys"}, {http.MethodDelete, "/api/v2/account/keys/key"},
	} {
		requireHostedStatus(t, hostedRequest(t, handler, route.method, route.path, "", []byte(`{}`), nil), http.StatusNotFound)
	}
}

func TestHostedSignupCreatesIndependentDatasetAndSession(t *testing.T) {
	fixture := newHostedContractFixture(t)
	policy, _ := serverfeatures.New(serverfeatures.Hosted, false)
	policy.SignupEnabled = true
	handler := NewDataHandlerWithOptions(t.Context(), sqlanalytics.Source{Store: fixture.store}, io.Discard, DataHandlerOptions{Policy: policy, Accounts: fixture.accounts, PublicURL: hostedTestOrigin})
	response := hostedRequest(t, handler, http.MethodPost, "/api/v2/auth/signup", "", []byte(`{"username":"charlie","password":"a-long-test-password"}`), nil)
	requireHostedStatus(t, response, http.StatusNoContent)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("signup did not sign in")
	}
	response = hostedRequest(t, handler, http.MethodGet, "/api/v2/account", "", nil, cookies[0])
	requireHostedStatus(t, response, http.StatusOK)
	user := hostedDecode[accounts.User](t, response)
	if user.Username != "charlie" || !user.Enabled || user.DatasetID == fixture.alice.DatasetID || user.DatasetID == fixture.bob.DatasetID {
		t.Fatal("signup identity", user)
	}
	response = hostedRequest(t, handler, http.MethodGet, "/api/v2/instance", "", nil, cookies[0])
	requireHostedStatus(t, response, http.StatusOK)
	if instance := hostedDecode[api.InstanceResponseV2](t, response); instance.DatasetId != user.DatasetID {
		t.Fatal("signup dataset not active", instance)
	}
	requireHostedStatus(t, hostedRequest(t, handler, http.MethodPost, "/api/v2/auth/signup", "", []byte(`{"username":"charlie","password":"another-long-password"}`), nil), http.StatusConflict)
}

func TestHostedAccountKeyOwnershipAndSessionIndependence(t *testing.T) {
	fixture := newHostedContractFixture(t)
	alice := loginHostedUser(t, fixture, "alice")
	bob := loginHostedUser(t, fixture, "bob")
	response := hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/account", "", nil, alice)
	requireHostedStatus(t, response, http.StatusOK)
	if user := hostedDecode[accounts.User](t, response); user.UserID != fixture.alice.UserID || user.Username != "alice" {
		t.Fatal("wrong account", user)
	}
	response = hostedRequest(t, fixture.handler, http.MethodPost, "/api/v2/account/keys", "", []byte(`{"label":"Work laptop","permissions":["read","ingest"]}`), alice)
	requireHostedStatus(t, response, http.StatusCreated)
	key := hostedDecode[accounts.Token](t, response)
	if key.Secret == "" {
		t.Fatal("created key", key)
	}
	response = hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/account/keys", "", nil, alice)
	requireHostedStatus(t, response, http.StatusOK)
	if bytes.Contains(response.Body.Bytes(), []byte(key.Secret)) || bytes.Contains(response.Body.Bytes(), []byte(`"token"`)) {
		t.Fatal("key listing exposed secret", response.Body.String())
	}
	listing := hostedDecode[struct {
		Keys []accounts.TokenInfo `json:"keys"`
	}](t, response)
	found := false
	for _, info := range listing.Keys {
		if info.TokenID == key.TokenID && info.Label == "Work laptop" {
			found = true
		}
	}
	if !found {
		t.Fatal("created key absent from listing", listing)
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodDelete, "/api/v2/account/keys/"+key.TokenID, "", nil, bob), http.StatusNotFound)
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/instance", key.Secret, nil, nil), http.StatusOK)
	for _, cookie := range []*http.Cookie{nil, alice} {
		for _, route := range []struct{ method, path string }{
			{http.MethodGet, "/api/v2/account"}, {http.MethodGet, "/api/v2/account/keys"},
			{http.MethodPost, "/api/v2/account/keys"}, {http.MethodPut, "/api/v2/account/password"},
			{http.MethodDelete, "/api/v2/account/keys/" + key.TokenID},
		} {
			requireHostedStatus(t, hostedRequest(t, fixture.handler, route.method, route.path, key.Secret, []byte(`{}`), cookie), http.StatusForbidden)
		}
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodDelete, "/api/v2/account/keys/"+key.TokenID, "", nil, alice), http.StatusNoContent)
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/instance", key.Secret, nil, nil), http.StatusUnauthorized)
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/account", "", nil, alice), http.StatusOK)
}

func TestHostedPasswordChangeInvalidatesSessionsAndPreservesKeys(t *testing.T) {
	fixture := newHostedContractFixture(t)
	first := loginHostedUser(t, fixture, "alice")
	second := loginHostedUser(t, fixture, "alice")
	response := hostedRequest(t, fixture.handler, http.MethodPut, "/api/v2/account/password", "", []byte(`{"currentPassword":"incorrect","password":"the-next-long-password"}`), first)
	requireHostedStatus(t, response, http.StatusBadRequest)
	if len(response.Result().Cookies()) != 0 {
		t.Fatal("incorrect password changed browser cookie")
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/account", "", nil, first), http.StatusOK)
	response = hostedRequest(t, fixture.handler, http.MethodPut, "/api/v2/account/password", "", []byte(`{"currentPassword":"a-long-test-password","password":"the-next-long-password"}`), first)
	requireHostedStatus(t, response, http.StatusNoContent)
	if cookies := response.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatal("password change did not clear cookie", cookies)
	}
	for _, cookie := range []*http.Cookie{first, second} {
		requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/account", "", nil, cookie), http.StatusUnauthorized)
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/instance", fixture.aliceToken.Secret, nil, nil), http.StatusOK)
	response = hostedRequest(t, fixture.handler, http.MethodPost, "/api/v2/auth/session", "", []byte(`{"username":"alice","password":"the-next-long-password"}`), nil)
	requireHostedStatus(t, response, http.StatusNoContent)
	response = hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/account", "", nil, response.Result().Cookies()[0])
	user := hostedDecode[accounts.User](t, response)
	if user.DatasetID != fixture.alice.DatasetID {
		t.Fatal("password rotation changed dataset", user)
	}
}

func TestHostedAccountMutationsRequireOriginAndStrictInput(t *testing.T) {
	fixture := newHostedContractFixture(t)
	cookie := loginHostedUser(t, fixture, "alice")
	for _, route := range []struct{ method, path string }{
		{http.MethodPut, "/api/v2/account/password"}, {http.MethodPost, "/api/v2/account/keys"},
		{http.MethodDelete, "/api/v2/account/keys/" + fixture.aliceToken.TokenID},
	} {
		request := httptest.NewRequest(route.method, hostedTestOrigin+route.path, bytes.NewBufferString(`{}`))
		request.Header.Set("Origin", "https://evil.example")
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		fixture.handler.ServeHTTP(response, request)
		requireHostedStatus(t, response, http.StatusForbidden)
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/instance", fixture.aliceToken.Secret, nil, nil), http.StatusOK)
	for _, body := range []string{
		`{"label":"Laptop","permissions":["read"],"userId":"` + fixture.bob.UserID + `"}`,
		`{"label":"Laptop","permissions":["read"]} {}`,
		`{"label":"Laptop","permissions":["admin"]}`,
		`{"label":"Laptop","permissions":["read","read"]}`,
		`{"label":"Laptop","permissions":["read"],"expiresAt":"2020-01-01T00:00:00Z"}`,
		`{"label":"","permissions":["read"]}`,
	} {
		requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodPost, "/api/v2/account/keys", "", []byte(body), cookie), http.StatusBadRequest)
	}
	response := hostedRequest(t, fixture.handler, http.MethodGet, "/settings", "", nil, nil)
	requireHostedStatus(t, response, http.StatusOK)
	if !bytes.Contains(response.Body.Bytes(), []byte("<html")) {
		t.Fatal("settings deep link did not render shell")
	}
}

func TestHostedPasswordAdmissionPrecedesBodyDecoding(t *testing.T) {
	fixture := newHostedContractFixture(t)
	releases := make([]func(), 0, accounts.MaxConcurrentPasswords)
	for i := range accounts.MaxConcurrentPasswords {
		release, err := fixture.accounts.AdmitPassword(fmt.Sprintf("198.51.100.%d:1234", i+1))
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
		defer release()
	}
	response := hostedRequest(t, fixture.handler, http.MethodPost, "/api/v2/auth/session", "", []byte(`invalid`), nil)
	requireHostedStatus(t, response, http.StatusServiceUnavailable)
	if response.Header().Get("Retry-After") != "1" {
		t.Fatal("password busy retry", response.Header())
	}
	for _, release := range releases {
		release()
	}
	response = hostedRequest(t, fixture.handler, http.MethodPost, "/api/v2/auth/session", "", []byte(`{"token":"`+fixture.aliceToken.Secret+`"}`), nil)
	requireHostedStatus(t, response, http.StatusBadRequest)
	for range accounts.LoginRequestsPerMinute - 2 {
		response = hostedRequest(t, fixture.handler, http.MethodPost, "/api/v2/auth/session", "", []byte(`invalid`), nil)
		requireHostedStatus(t, response, http.StatusBadRequest)
	}
	response = hostedRequest(t, fixture.handler, http.MethodPost, "/api/v2/auth/session", "", []byte(`invalid`), nil)
	requireHostedStatus(t, response, http.StatusTooManyRequests)
	if response.Header().Get("Retry-After") != "60" {
		t.Fatal("password rate retry", response.Header())
	}
}

func TestIncorrectCurrentPasswordPreservesAuthentication(t *testing.T) {
	fixture := newHostedContractFixture(t)
	cookie := loginHostedUser(t, fixture, "alice")
	response := hostedRequest(t, fixture.handler, http.MethodPut, "/api/v2/account/password", "", []byte(`{"currentPassword":"incorrect-password","password":"the-next-long-password"}`), cookie)
	requireHostedStatus(t, response, http.StatusBadRequest)
	if len(response.Result().Cookies()) != 0 {
		t.Fatal("incorrect current password cleared session")
	}
	if errorResponse := hostedDecode[api.ErrorResponse](t, response); errorResponse.Message != "Current password is incorrect." {
		t.Fatal("incorrect password response", errorResponse)
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/account", "", nil, cookie), http.StatusOK)
	newCookie := loginHostedUser(t, fixture, "alice")
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/account", "", nil, newCookie), http.StatusOK)
}

func TestAccountExpectedDatasetCannotSelectIdentityOrMutate(t *testing.T) {
	fixture := newHostedContractFixture(t)
	cookie := loginHostedUser(t, fixture, "alice")
	request := func(method, path string, body []byte, datasets ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, hostedTestOrigin+path, bytes.NewReader(body))
		r.AddCookie(cookie)
		r.Header.Set("Origin", hostedTestOrigin)
		r.Header.Set("Content-Type", "application/json")
		for _, dataset := range datasets {
			r.Header.Add(accountDatasetHeader, dataset)
		}
		response := httptest.NewRecorder()
		fixture.handler.ServeHTTP(response, r)
		return response
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v2/account"}, {http.MethodGet, "/api/v2/account/keys"},
		{http.MethodPost, "/api/v2/account/keys"}, {http.MethodPut, "/api/v2/account/password"},
		{http.MethodDelete, "/api/v2/account/keys/" + fixture.aliceToken.TokenID},
	} {
		response := request(route.method, route.path, []byte(`invalid`), fixture.bob.DatasetID)
		requireHostedStatus(t, response, http.StatusConflict)
		if errorResponse := hostedDecode[api.ErrorResponse](t, response); errorResponse.Code != api.ErrorCodeInvalidRequest || errorResponse.Message != "Account changed. Sign in again." {
			t.Fatal("account mismatch response", errorResponse)
		}
	}
	requireHostedStatus(t, request(http.MethodPost, "/api/v2/account/keys", []byte(`{"label":"Rejected laptop","permissions":["read"]}`), fixture.bob.DatasetID), http.StatusConflict)
	requireHostedStatus(t, request(http.MethodPut, "/api/v2/account/password", []byte(`{"currentPassword":"a-long-test-password","password":"the-next-long-password"}`), fixture.bob.DatasetID), http.StatusConflict)
	requireHostedStatus(t, request(http.MethodGet, "/api/v2/account", nil, fixture.alice.DatasetID, fixture.alice.DatasetID), http.StatusBadRequest)
	requireHostedStatus(t, request(http.MethodGet, "/api/v2/account", nil, fixture.alice.DatasetID+","+fixture.alice.DatasetID), http.StatusBadRequest)
	response := request(http.MethodGet, "/api/v2/account", nil, fixture.alice.DatasetID)
	requireHostedStatus(t, response, http.StatusOK)
	if user := hostedDecode[accounts.User](t, response); user.UserID != fixture.alice.UserID || user.DatasetID != fixture.alice.DatasetID {
		t.Fatal("dataset header selected account", user)
	}
	response = request(http.MethodGet, "/api/v2/account/keys", nil, fixture.alice.DatasetID)
	requireHostedStatus(t, response, http.StatusOK)
	before := hostedDecode[struct {
		Keys []accounts.TokenInfo `json:"keys"`
	}](t, response)
	response = request(http.MethodPost, "/api/v2/account/keys", []byte(`{"label":"Bound laptop","permissions":["read"]}`), fixture.alice.DatasetID)
	requireHostedStatus(t, response, http.StatusCreated)
	response = request(http.MethodGet, "/api/v2/account/keys", nil, fixture.alice.DatasetID)
	requireHostedStatus(t, response, http.StatusOK)
	after := hostedDecode[struct {
		Keys []accounts.TokenInfo `json:"keys"`
	}](t, response)
	if len(after.Keys) != len(before.Keys)+1 {
		t.Fatal("rejected key mutation persisted", before, after)
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/instance", fixture.aliceToken.Secret, nil, nil), http.StatusOK)
	// Five mismatched account operations must consume no password-admission slots.
	for range accounts.LoginRequestsPerMinute - 1 {
		loginHostedUser(t, fixture, "alice")
	}
}
