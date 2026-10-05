package queryclient

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
)

func TestQueryResponsesRequirePublicationEnvelope(t *testing.T) {
	for _, endpoint := range []string{"instance", "sync", "usage", "facets"} {
		for _, invalid := range []string{"missing-instance", "null-instance", "empty-instance", "missing-epoch", "null-epoch", "empty-epoch", "missing-revision", "null-revision", "negative-revision", "fractional-revision", "missing-readiness", "unknown-readiness"} {
			if endpoint == "instance" && invalid == "missing-revision" || endpoint == "instance" && invalid == "null-revision" || endpoint == "instance" && invalid == "negative-revision" || endpoint == "instance" && invalid == "fractional-revision" {
				continue
			}
			if (endpoint == "usage" || endpoint == "facets") && (invalid == "missing-readiness" || invalid == "unknown-readiness") {
				continue
			}
			t.Run(endpoint+"/"+invalid, func(t *testing.T) {
				body := map[string]any{"apiVersion": "v1", "instanceId": "instance", "dataEpoch": "epoch", "revision": 0, "dataReadiness": "ready"}
				switch invalid {
				case "missing-instance":
					delete(body, "instanceId")
				case "null-instance":
					body["instanceId"] = nil
				case "empty-instance":
					body["instanceId"] = ""
				case "missing-epoch":
					delete(body, "dataEpoch")
				case "null-epoch":
					body["dataEpoch"] = nil
				case "empty-epoch":
					body["dataEpoch"] = ""
				case "missing-revision":
					delete(body, "revision")
				case "null-revision":
					body["revision"] = nil
				case "negative-revision":
					body["revision"] = -1
				case "fractional-revision":
					body["revision"] = 0.5
				case "missing-readiness":
					delete(body, "dataReadiness")
				case "unknown-readiness":
					body["dataReadiness"] = "unknown"
				}
				c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(body)
				}))
				if err := readEnvelopeEndpoint(t, c, endpoint); err == nil {
					t.Fatal("unsupported publication envelope accepted")
				}
			})
		}
	}
}

func TestQueryEnvelopeAcceptsZeroRevisionAndExplicitUnavailableMetadata(t *testing.T) {
	for _, endpoint := range []string{"instance", "sync", "usage", "facets"} {
		for _, unavailable := range []bool{false, true} {
			if unavailable && (endpoint == "usage" || endpoint == "facets") {
				continue
			}
			body := map[string]any{"apiVersion": "v1", "instanceId": "instance", "dataEpoch": "epoch", "revision": 0, "dataReadiness": "ready"}
			if unavailable {
				body["dataEpoch"], body["dataReadiness"] = "", "unavailable"
			}
			c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { serveJSON(w, body) }))
			if err := readEnvelopeEndpoint(t, c, endpoint); err != nil {
				t.Fatalf("%s unavailable=%v: %v", endpoint, unavailable, err)
			}
		}
	}
}

func TestAllUsageDoesNotReadUnavailableMetadata(t *testing.T) {
	requests := 0
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/v1/instance" {
			t.Errorf("unavailable metadata authorized %s", r.URL.Path)
		}
		serveJSON(w, map[string]any{"apiVersion": "v1", "instanceId": "instance", "dataEpoch": "", "dataReadiness": "unavailable"})
	}))
	result, err := c.AllUsage(t.Context(), api.GetUsageParams{})
	if !errors.Is(err, ErrUnavailable) || requests != 1 || len(result.Rows) != 0 {
		t.Fatalf("unavailable queries=%d rows=%d error=%v", requests, len(result.Rows), err)
	}
}

func readEnvelopeEndpoint(t *testing.T, c *Client, endpoint string) error {
	t.Helper()
	switch endpoint {
	case "instance":
		_, err := c.Instance(t.Context())
		return err
	case "sync":
		_, err := c.Status(t.Context())
		return err
	case "usage":
		_, err := c.Usage(t.Context(), api.GetUsageParams{})
		return err
	default:
		_, err := c.Facets(t.Context(), api.GetUsageFacetsParams{})
		return err
	}
}
