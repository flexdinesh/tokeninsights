package server

import (
	"net/http"
	"testing"

	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

func TestHostedIdentityNeverUsesRuntimeHostname(t *testing.T) {
	fixture := newHostedContractFixture(t)
	policy, err := serverfeatures.New(serverfeatures.Hosted, false)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewDataHandlerWithOptions(t.Context(), fixture.store, nil, DataHandlerOptions{Host: "0.0.0.0", Hostname: "private-deployment-host", Policy: policy, Accounts: fixture.accounts, PublicURL: hostedTestOrigin})
	response := hostedRequest(t, handler, http.MethodGet, "/api/v2/instance", fixture.aliceToken.Secret, nil, nil)
	requireHostedStatus(t, response, http.StatusOK)
	instance := hostedDecode[api.InstanceResponseV2](t, response)
	if instance.Hostname != "unknown" {
		t.Fatal("hosted runtime hostname leaked", instance.Hostname)
	}
}

func TestHTTPStatusReportsOnlyDatasetFailedPendingScopes(t *testing.T) {
	fixture := newHostedContractFixture(t)
	if _, err := fixture.store.SQL().Exec(`INSERT INTO processing.scopes(dataset_id,scope,revision,processed_revision,generation,error_code,retry_at_ms) VALUES
		(?, 'pi:session:complete',1,1,1,'processing_failed',1000),
		(?, 'pi:session:failed',1,0,0,'processing_failed',1234),
		(?, 'pi:session:pending',1,0,0,'',0),
		(?, 'pi:session:other',1,0,0,'processing_failed',5678)`, fixture.alice.DatasetID, fixture.alice.DatasetID, fixture.alice.DatasetID, fixture.bob.DatasetID); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		token, dataset           string
		pending, failed, retryAt int64
	}{
		{fixture.aliceToken.Secret, fixture.alice.DatasetID, 2, 1, 1234},
		{fixture.bobToken.Secret, fixture.bob.DatasetID, 1, 1, 5678},
	} {
		response := hostedRequest(t, fixture.handler, http.MethodGet, "/api/v2/status", test.token, nil, nil)
		requireHostedStatus(t, response, http.StatusOK)
		status := hostedDecode[api.StatusResponseV2](t, response)
		if status.DatasetId != test.dataset || status.Pending != test.pending || status.Failed == nil || *status.Failed != test.failed || status.FailedRetryAtMs == nil || *status.FailedRetryAtMs != test.retryAt {
			t.Fatal("status failure isolation wrong", status)
		}
	}
}
