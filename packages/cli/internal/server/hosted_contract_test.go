package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	serverapi "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

const hostedTestOrigin = "https://usage.example"

type hostedContractFixture struct {
	store                                        *datastore.Store
	accounts                                     *accounts.Service
	handler                                      http.Handler
	alice, bob                                   accounts.User
	aliceToken, bobToken, readToken, ingestToken accounts.Token
}

func newHostedContractFixture(t *testing.T) hostedContractFixture {
	t.Helper()
	store, err := datastore.OpenKind(t.Context(), filepath.Join(t.TempDir(), "hosted.duckdb"), datastore.KindHosted)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := newHostedAccounts(t, store)
	alice, err := service.CreateUser(t.Context(), "Alice")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := service.CreateUser(t.Context(), "Bob")
	if err != nil {
		t.Fatal(err)
	}
	token := func(user accounts.User, permissions ...string) accounts.Token {
		result, err := service.CreateToken(t.Context(), user.UserID, permissions, nil)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	policy, err := serverfeatures.New(serverfeatures.Hosted, false)
	if err != nil {
		t.Fatal(err)
	}
	return hostedContractFixture{store: store, accounts: service, handler: NewDataHandlerWithOptions(t.Context(), store, &bytes.Buffer{}, DataHandlerOptions{Host: "0.0.0.0", InstanceID: "hosted-fixture", AllowIngestion: true, Policy: policy, Accounts: service, PublicURL: hostedTestOrigin}), alice: alice, bob: bob, aliceToken: token(alice, accounts.Read, accounts.Ingest), bobToken: token(bob, accounts.Read, accounts.Ingest), readToken: token(alice, accounts.Read), ingestToken: token(alice, accounts.Ingest)}
}

func hostedRequest(t *testing.T, handler http.Handler, method, path, token string, body []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, hostedTestOrigin+path, bytes.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if method != http.MethodGet {
		request.Header.Set("Origin", hostedTestOrigin)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func requireHostedStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status %d, want %d: %s", response.Code, status, response.Body.String())
	}
}

func hostedDecode[T interface{}](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err, response.Body.String())
	}
	return value
}

func hostedBatch(t *testing.T, handler http.Handler, token string, input int, model string) ([]byte, evidence.Batch) {
	t.Helper()
	response := hostedRequest(t, handler, http.MethodGet, "/api/v3/ingestion/capabilities", token, nil, nil)
	requireHostedStatus(t, response, 200)
	capabilities := hostedDecode[evidence.Capabilities](t, response)
	batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: capabilities.DatabaseID, DatasetID: capabilities.DatasetID, StreamID: "same-stream", BatchID: "same-batch", FromSequence: 1, ToSequence: 1, Entries: []evidence.Entry{{Sequence: 1, Record: evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "same-source", Lineage: "same-lineage", Ordinal: 1, Context: []evidence.Context{{Ordinal: 0, Data: json.RawMessage(`{"type":"session","id":"same-session"}`)}}, Data: json.RawMessage(fmt.Sprintf(`{"type":"message","id":"same-message","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"%s","usage":{"input":%d,"output":20,"reasoning":5,"cacheRead":3,"cacheWrite":4}}}`, model, input))}}}}
	batch.Entries[0].Record.Location = &evidence.Location{DirectoryKey: model + "-directory", DirectoryName: model + "-directory", RepositoryKey: model + "-repo", RepositoryName: model + "-repo", RepositorySource: "harness"}
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	return body, batch
}

func TestHostedHTTPDatasetIsolationAndCapabilities(t *testing.T) {
	fixture := newHostedContractFixture(t)
	for _, path := range []string{"/api/v2/instance", "/api/v2/status", "/api/v2/usage", "/api/v2/usage/facets", "/api/v3/ingestion/capabilities"} {
		requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, path, "", nil, nil), 401)
	}
	for _, path := range []string{"/api/v1/instance", "/api/v1/sync", "/api/v1/usage", "/api/v1/usage/facets", "/api/v1/ingestion/capabilities", "/api/v2/ingestion/capabilities", "/api/v2/collector-progress", "/api/v2/processing/reprocess"} {
		requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, path, fixture.aliceToken.Secret, nil, nil), 404)
	}
	descriptorResponse := hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/instance", fixture.aliceToken.Secret, nil, nil)
	requireHostedStatus(t, descriptorResponse, 200)
	descriptor := hostedDecode[serverapi.InstanceResponseV2](t, descriptorResponse)
	if descriptor.ServerKind != "hosted" || descriptor.DatasetId != fixture.alice.DatasetID || slices.Contains(descriptor.Capabilities, "terminal-dashboard") || slices.Contains(descriptor.Capabilities, "collector-progress") || !slices.Contains(descriptor.Capabilities, "raw-ingestion") {
		t.Fatalf("wrong hosted descriptor: %+v", descriptor)
	}
	ingestDescriptor := hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/instance", fixture.ingestToken.Secret, nil, nil)
	requireHostedStatus(t, ingestDescriptor, 200)
	if got := hostedDecode[serverapi.InstanceResponseV2](t, ingestDescriptor); len(got.Permissions) != 1 || got.Permissions[0] != "ingest" {
		t.Fatalf("wrong ingest descriptor %+v", got)
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/usage", fixture.ingestToken.Secret, nil, nil), 403)
	aliceBody, aliceBatch := hostedBatch(t, fixture.handler, fixture.aliceToken.Secret, 100, "alice-model")
	bobBody, _ := hostedBatch(t, fixture.handler, fixture.bobToken.Secret, 900, "bob-model")
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodPost, "/api/v3/ingestion/batches", fixture.readToken.Secret, aliceBody, nil), 403)
	aliceResponse := hostedRequest(t, fixture.handler, http.MethodPost, "/api/v3/ingestion/batches", fixture.aliceToken.Secret, aliceBody, nil)
	requireHostedStatus(t, aliceResponse, 202)
	aliceReceipt := hostedDecode[evidence.Response](t, aliceResponse).Receipt
	for {
		worked, err := fixture.store.ProcessNext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	bobResponse := hostedRequest(t, fixture.handler, http.MethodPost, "/api/v3/ingestion/batches", fixture.bobToken.Secret, bobBody, nil)
	requireHostedStatus(t, bobResponse, 202)
	bobReceipt := hostedDecode[evidence.Response](t, bobResponse).Receipt
	for _, test := range []struct {
		token, dataset string
		pending        bool
	}{{fixture.aliceToken.Secret, fixture.alice.DatasetID, false}, {fixture.bobToken.Secret, fixture.bob.DatasetID, true}} {
		response := hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/status", test.token, nil, nil)
		requireHostedStatus(t, response, 200)
		status := hostedDecode[serverapi.StatusResponseV2](t, response)
		if status.DatasetId != test.dataset || (status.Pending > 0) != test.pending || status.InputRevision != 1 {
			t.Fatalf("status leaked: %+v", status)
		}
	}
	// Batch identity and native identities deliberately collide across users.
	for _, test := range []struct {
		token   string
		receipt evidence.Receipt
		body    []byte
	}{{fixture.aliceToken.Secret, aliceReceipt, aliceBody}, {fixture.bobToken.Secret, bobReceipt, bobBody}} {
		replay := hostedRequest(t, fixture.handler, http.MethodPost, "/api/v3/ingestion/batches", test.token, test.body, nil)
		expectedStatus := 200
		if test.token == fixture.bobToken.Secret {
			expectedStatus = 202
		}
		requireHostedStatus(t, replay, expectedStatus)
		lookup := hostedRequest(t, fixture.handler, http.MethodGet, "/api/v3/ingestion/batches/same-stream/same-batch", test.token, nil, nil)
		requireHostedStatus(t, lookup, expectedStatus)
		if got := hostedDecode[evidence.Response](t, lookup).Receipt; !reflect.DeepEqual(got, test.receipt) {
			t.Fatalf("receipt leaked: %+v want %+v", got, test.receipt)
		}
	}
	for {
		worked, err := fixture.store.ProcessNext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	before, err := fixture.store.ForDataset(fixture.alice.DatasetID).Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	bobBefore, err := fixture.store.ForDataset(fixture.bob.DatasetID).Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	spoof := aliceBatch
	spoof.DatasetID = fixture.bob.DatasetID
	spoof.BatchID = "spoof"
	spoofBody, err := json.Marshal(spoof)
	if err != nil {
		t.Fatal(err)
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodPost, "/api/v3/ingestion/batches", fixture.aliceToken.Secret, spoofBody, nil), 409)
	after, err := fixture.store.ForDataset(fixture.alice.DatasetID).Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("spoof mutated metadata: %+v %+v", before, after)
	}
	bobAfter, err := fixture.store.ForDataset(fixture.bob.DatasetID).Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if bobBefore != bobAfter {
		t.Fatalf("spoof mutated victim metadata: %+v %+v", bobBefore, bobAfter)
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v3/ingestion/batches/same-stream/spoof", fixture.aliceToken.Secret, nil, nil), 404)
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v3/ingestion/batches/same-stream/spoof", fixture.bobToken.Secret, nil, nil), 404)
	for _, test := range []struct {
		token, dataset, model string
		input, total          int64
	}{{fixture.aliceToken.Secret, fixture.alice.DatasetID, "alice-model", 100, 127}, {fixture.bobToken.Secret, fixture.bob.DatasetID, "bob-model", 900, 927}} {
		for _, tab := range []string{"tokens", "models", "providers", "harnesses", "sessions", "context", "repo"} {
			response := hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/usage?period=all&tab="+tab+"&page=999&pageSize=1", test.token, nil, nil)
			requireHostedStatus(t, response, 200)
			usage := hostedDecode[serverapi.UsageResponseV2](t, response)
			if usage.DatasetId != test.dataset || usage.Summary.Total != test.total || usage.Summary.Input != test.input || usage.Summary.Output != 15 || usage.Summary.Reasoning != 5 || usage.Summary.CacheRead != 3 || usage.Summary.CacheWrite != 4 || usage.Summary.Sessions != 1 || usage.Summary.SyncedSessions != 1 || len(usage.Rows) != 1 || usage.Rows[0].Model != test.model || usage.FactCount == nil || *usage.FactCount != 1 || usage.Page != 1 || usage.RowCount != 1 || len(usage.Chart) != 1 {
				t.Fatalf("%s usage leaked: %+v", tab, usage)
			}
			if tab == "context" {
				if usage.Rows[0].AverageContext != test.input+7 || usage.Rows[0].MedianContext != test.input+7 || usage.Rows[0].MaxContext != test.input+7 || usage.Chart[0].AverageContext != test.input+7 {
					t.Fatalf("context leaked: %+v", usage)
				}
			} else if usage.Rows[0].Total != test.total || usage.Chart[0].Total != test.total {
				t.Fatalf("%s chart/row leaked: %+v", tab, usage)
			}
			response = hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/usage/facets?period=all&tab="+tab+"&search=same-session", test.token, nil, nil)
			requireHostedStatus(t, response, 200)
			facets := hostedDecode[serverapi.UsageFacetsResponseV2](t, response)
			if facets.DatasetId != test.dataset || !reflect.DeepEqual(facets.Models, []string{test.model}) || !reflect.DeepEqual(facets.Sessions, []string{"same-session"}) {
				t.Fatalf("%s facets leaked: %+v", tab, facets)
			}
			if tab == "repo" && (len(facets.Repositories) != 1 || facets.Repositories[0].Key != test.model+"-repo" || len(facets.Directories) != 1 || facets.Directories[0].Key != test.model+"-directory") {
				t.Fatalf("location facets leaked: %+v", facets)
			}
		}
		response := hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/usage?period=all&tab=repo&locationGroup=directory", test.token, nil, nil)
		requireHostedStatus(t, response, 200)
		directories := hostedDecode[serverapi.UsageResponseV2](t, response)
		if len(directories.Rows) != 1 || directories.Rows[0].LocationKey != test.model+"-directory" || directories.Rows[0].Total != test.total {
			t.Fatalf("directory leaked: %+v", directories)
		}
		response = hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/usage/facets?period=all&tab=sessions&search=missing-session", test.token, nil, nil)
		requireHostedStatus(t, response, 200)
		if facets := hostedDecode[serverapi.UsageFacetsResponseV2](t, response); len(facets.Sessions) != 0 {
			t.Fatalf("session search leaked: %+v", facets)
		}

	}
}

func TestHostedHTTPBrowserSessionReadOnlyAndLogout(t *testing.T) {
	fixture := newHostedContractFixture(t)
	body, err := json.Marshal(serverapi.BrowserSessionRequest{Token: fixture.aliceToken.Secret})
	if err != nil {
		t.Fatal(err)
	}
	invalid := httptest.NewRequest(http.MethodPost, hostedTestOrigin+"/api/v2/auth/session", bytes.NewReader(body))
	invalid.Header.Set("Origin", "https://evil.example")
	invalid.Header.Set("Content-Type", "application/json")
	rejected := httptest.NewRecorder()
	fixture.handler.ServeHTTP(rejected, invalid)
	requireHostedStatus(t, rejected, 403)
	login := hostedRequest(t, fixture.handler, http.MethodPost, "/api/v2/auth/session", "", body, nil)
	requireHostedStatus(t, login, 204)
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies: %+v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != accounts.SessionCookieName || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("cookie policy: %+v", cookie)
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/usage?period=all", "", nil, cookie), 200)
	batch, _ := hostedBatch(t, fixture.handler, fixture.aliceToken.Secret, 100, "model")
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodPost, "/api/v3/ingestion/batches", "", batch, cookie), 403)
	logout := hostedRequest(t, fixture.handler, http.MethodDelete, "/api/v2/auth/session", "", nil, cookie)
	requireHostedStatus(t, logout, 204)
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/usage", "", nil, cookie), 401)
	ingestLoginBody, err := json.Marshal(serverapi.BrowserSessionRequest{Token: fixture.ingestToken.Secret})
	if err != nil {
		t.Fatal(err)
	}
	requireHostedStatus(t, hostedRequest(t, fixture.handler, http.MethodPost, "/api/v2/auth/session", "", ingestLoginBody, nil), 401)
}

func TestDisabledHTTPCapabilitiesFailClosed(t *testing.T) {
	for _, kind := range []serverfeatures.Kind{serverfeatures.Personal, serverfeatures.Hosted} {
		t.Run(string(kind), func(t *testing.T) {
			store, err := datastore.OpenKind(t.Context(), filepath.Join(t.TempDir(), "server.duckdb"), string(kind))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()
			policy, err := serverfeatures.New(kind, false, serverfeatures.Usage, serverfeatures.Facets, serverfeatures.WebDashboard, serverfeatures.TerminalDashboard, serverfeatures.RawIngestion, serverfeatures.Reprocess)
			if err != nil {
				t.Fatal(err)
			}
			var service *accounts.Service
			token := ""
			if kind == serverfeatures.Hosted {
				service = newHostedAccounts(t, store)
				user, err := service.CreateUser(t.Context(), "User")
				if err != nil {
					t.Fatal(err)
				}
				credential, err := service.CreateToken(t.Context(), user.UserID, []string{accounts.Read, accounts.Ingest}, nil)
				if err != nil {
					t.Fatal(err)
				}
				token = credential.Secret
			}
			handler := NewDataHandlerWithOptions(t.Context(), store, &bytes.Buffer{}, DataHandlerOptions{Host: "0.0.0.0", InstanceID: "disabled-fixture", AllowIngestion: true, Policy: policy, Accounts: service, PublicURL: hostedTestOrigin})
			for _, path := range []string{"/api/v2/usage", "/api/v2/usage/facets", "/api/v2/collector-progress", "/api/v3/ingestion/capabilities", "/api/v3/ingestion/batches", "/api/v2/processing/reprocess", "/tokens", "/"} {
				requireHostedStatus(t, hostedRequest(t, handler, http.MethodGet, path, token, nil, nil), 404)
			}
			if kind == serverfeatures.Personal {
				for _, path := range []string{"/api/v1/usage", "/api/v1/usage/facets", "/api/v1/ingestion/capabilities", "/api/v2/ingestion/capabilities"} {
					requireHostedStatus(t, hostedRequest(t, handler, http.MethodGet, path, token, nil, nil), 404)
				}
			}
		})
	}
}

func newHostedAccounts(t *testing.T, store *datastore.Store) *accounts.Service {
	t.Helper()
	identity, err := store.DatabaseIdentity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	app, err := appstore.Open(t.Context(), filepath.Join(t.TempDir(), "app.sqlite"), identity, "hosted")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	repository := accounts.NewSQLite(app, store)
	return accounts.New(repository)
}
