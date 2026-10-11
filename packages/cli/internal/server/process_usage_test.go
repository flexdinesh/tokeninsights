package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processusage"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

type resourceSampler struct{ calls int }

func (s *resourceSampler) Snapshot(context.Context) processusage.Sample {
	s.calls++
	cpu, memory := 125.5, int64(64*1024*1024)
	return processusage.Sample{CPUPercent: &cpu, ResidentMemoryBytes: &memory}
}

func TestPersonalStatusSamplesCommandResources(t *testing.T) {
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sampler := &resourceSampler{}
	policy, err := serverfeatures.New(serverfeatures.Personal, false)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewDataHandlerWithOptions(t.Context(), sqlanalytics.Source{Store: store}, io.Discard, DataHandlerOptions{Host: "127.0.0.1", Policy: policy, ProcessUsage: sampler})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v2/status", nil))
	var status api.StatusResponseV2
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if sampler.calls != 1 || status.ProcessUsage == nil || status.ProcessUsage.CpuPercent == nil || *status.ProcessUsage.CpuPercent != 125.5 || status.ProcessUsage.ResidentMemoryBytes == nil || *status.ProcessUsage.ResidentMemoryBytes != 64*1024*1024 {
		t.Fatalf("unexpected resources: calls %d, status %+v", sampler.calls, status.ProcessUsage)
	}
}

func TestHostedStatusNeverSamplesOrExposesProcessResources(t *testing.T) {
	fixture := newHostedContractFixture(t)
	policy, err := serverfeatures.New(serverfeatures.Hosted, false)
	if err != nil {
		t.Fatal(err)
	}
	sampler := &resourceSampler{}
	handler := NewDataHandlerWithOptions(t.Context(), sqlanalytics.Source{Store: fixture.store}, io.Discard, DataHandlerOptions{Policy: policy, Accounts: fixture.accounts, PublicURL: hostedTestOrigin, ProcessUsage: sampler})
	response := hostedRequest(t, handler, http.MethodGet, "/api/v2/status", fixture.aliceToken.Secret, nil, nil)
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body.String())
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if _, found := fields["processUsage"]; found || sampler.calls != 0 {
		t.Fatal("hosted process resources exposed or sampled", response.Body.String(), sampler.calls)
	}
}
