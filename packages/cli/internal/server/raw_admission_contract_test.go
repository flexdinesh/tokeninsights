package server

import (
	"net/http"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func TestHostedRawAdmissionErrorsUseBoundedProtocolEnvelope(t *testing.T) {
	fixture := newHostedContractFixture(t)
	for _, test := range []struct {
		token, code string
		status      int
	}{
		{"", "unauthorized", 401},
		{fixture.readToken.Secret, "forbidden", 403},
	} {
		response := hostedRequest(t, fixture.handler, http.MethodPost, "/api/v3/ingestion/batches", test.token, []byte(`{}`), nil)
		requireHostedStatus(t, response, test.status)
		var body publication.ErrorResponse
		if err := evidence.StrictDecode(response.Body.Bytes(), &body); err != nil || body.Stage != "admission" || body.Code != test.code {
			t.Fatal(body, err)
		}
	}
	release, err := fixture.accounts.Admit(accounts.Principal{UserID: fixture.alice.UserID})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	// Invalid body deliberately demonstrates that admission precedes decoding.
	response := hostedRequest(t, fixture.handler, http.MethodPost, "/api/v3/ingestion/batches", fixture.aliceToken.Secret, []byte(`invalid`), nil)
	requireHostedStatus(t, response, 429)
	var body publication.ErrorResponse
	if err := evidence.StrictDecode(response.Body.Bytes(), &body); err != nil || body.Stage != "admission" || body.Code != "user_busy" {
		t.Fatal(body, err)
	}
	if response.Header().Get("Retry-After") != "1" {
		t.Fatal(response.Header())
	}
}
