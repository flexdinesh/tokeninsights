package localclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
)

func TestPrivateAllUsagePreserves100kRowsBeyondPublicBodyBudget(t *testing.T) {
	const totalRows = 100000
	rows := make([]analytics.Row, totalRows)
	for i := range rows {
		rows[i] = analytics.Row{Key: fmt.Sprint(i), Name: "model", Input: 1, Total: 1}
	}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/local/v1/queries/all" || r.Header.Get("X-TokenInsights-Instance") != "instance" {
			t.Error("query used public route or lost owner guard")
		}
		var request localprotocol.AllUsageRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.MaxRows != totalRows {
			t.Error("private row bound changed", request.MaxRows, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(application.Usage{InstanceID: "instance", Dashboard: analytics.Dashboard{Rows: rows, RowCount: totalRows, Revision: 17, InputRevision: 23, DatabaseID: "database", DatasetID: "dataset", Summary: analytics.ViewerSummaryRow{TotalTokens: totalRows}}})
	}))
	defer server.Close()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	session := &Session{InstanceID: "instance", client: &http.Client{Transport: transport}}
	client := queryclient.NewDirect(privateReader{session: session})
	result, err := client.AllUsage(t.Context(), analytics.Query{Selection: analytics.Selection{Period: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(result.Rows) != totalRows || result.RowCount != totalRows || result.Summary.TotalTokens != totalRows || result.Revision != 17 || result.InputRevision != 23 {
		t.Fatal("snapshot contract reduced", calls.Load(), len(result.Rows), result.Summary.TotalTokens, result.Revision)
	}
	if result.Rows[totalRows-1].Key != "99999" {
		t.Fatal("complete snapshot truncated")
	}
}
