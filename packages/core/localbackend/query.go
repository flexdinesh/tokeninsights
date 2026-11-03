package localbackend

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

// queries is an HTTP adapter over semantic application operations. It does not
// paginate AllUsage or split the repository transaction into separate reads.
func queryHandler(reader application.Reader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, localprotocol.MaxQueryRequestBytes))
		decoder.DisallowUnknownFields()
		decode := func(target interface{}) bool {
			if decoder.Decode(target) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
				respond(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
				return false
			}
			return true
		}
		var result interface{}
		var err error
		switch strings.TrimPrefix(r.URL.Path, "/api/local/v1/queries/") {
		case "instance":
			result, err = reader.Instance(r.Context())
		case "status":
			result, err = reader.Status(r.Context())
		case "usage":
			var request analytics.Query
			if !decode(&request) {
				return
			}
			result, err = reader.Usage(r.Context(), request)
		case "all":
			var request localprotocol.AllUsageRequest
			if !decode(&request) {
				return
			}
			if request.MaxRows < 1 || request.MaxRows > localprotocol.MaxQueryRows {
				respond(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
				return
			}
			result, err = reader.AllUsage(r.Context(), request.Query, request.MaxRows)
		case "facets":
			var request application.FacetRequest
			if !decode(&request) {
				return
			}
			result, err = reader.Facets(r.Context(), request)
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			var invalid *application.InvalidRequest
			switch {
			case errors.As(err, &invalid):
				respond(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
			case errors.Is(err, application.ErrUnavailable):
				respond(w, http.StatusServiceUnavailable, map[string]string{"error": "query_unavailable"})
			case errors.Is(err, application.ErrForbidden):
				respond(w, http.StatusForbidden, map[string]string{"error": "query_forbidden"})
			default:
				operationError(w, err)
			}
			return
		}
		respond(w, http.StatusOK, result)
	})
}
