// Package ingestionhttp adapts durable raw acceptance to bounded HTTP requests.
package ingestionhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

const IngestionPrefix = "/api/v3/ingestion/"
const ProcessingPrefix = "/api/v2/processing/"

type Reprocessor interface {
	Reprocess(context.Context) (int64, error)
}

func respond(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, status int, stage, code string) {
	respond(w, status, publication.ErrorResponse{Stage: stage, Code: code})
}
func responseStatus(response evidence.Response) int {
	if response.Processing.Pending == 0 {
		return http.StatusOK
	}
	return http.StatusAccepted
}
func acceptanceError(w http.ResponseWriter, err error) {
	var invalid *evidence.ValidationError
	if errors.As(err, &invalid) {
		status := http.StatusBadRequest
		if invalid.Code == "incompatible" {
			status = http.StatusUnprocessableEntity
		}
		fail(w, status, "validation", invalid.Code)
		return
	}
	var refused evidence.Rejection
	status, code := http.StatusServiceUnavailable, "transaction_failed"
	if errors.As(err, &refused) {
		code = refused.IngestionCode()
		status = http.StatusUnprocessableEntity
		if strings.HasSuffix(code, "conflict") || code == "database_mismatch" || code == "dataset_mismatch" {
			status = http.StatusConflict
		}
		if code == "invalid_request" {
			status = http.StatusBadRequest
		}
	}
	fail(w, status, "admission", code)
}
func Handler(receiver evidence.Receiver, prefix string, protocol int) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+prefix+"capabilities", func(w http.ResponseWriter, r *http.Request) {
		caps, err := receiver.RawCapabilities(r.Context())
		if err != nil {
			fail(w, 503, "database", "unavailable")
			return
		}
		caps.ProtocolVersion = protocol
		respond(w, 200, caps)
	})
	mux.HandleFunc("POST "+prefix+"batches", func(w http.ResponseWriter, r *http.Request) {
		release, allowed := receiver.AcquireAdmission()
		if !allowed {
			fail(w, 503, "admission", "busy")
			return
		}
		defer release()
		if r.Header.Get("Content-Type") != "application/json" {
			fail(w, 400, "validation", "invalid_request")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, evidence.MaxBodyBytes+1))
		if err != nil {
			fail(w, 400, "validation", "body_read_failed")
			return
		}
		if len(body) > evidence.MaxBodyBytes {
			fail(w, 413, "validation", "body_limit")
			return
		}
		response, err := receiver.Accept(r.Context(), protocol, body)
		if err != nil {
			acceptanceError(w, err)
			return
		}
		respond(w, responseStatus(response), response)
	})
	mux.HandleFunc("GET "+prefix+"batches/{stream}/{batch}", func(w http.ResponseWriter, r *http.Request) {
		response, err := receiver.Receipt(r.Context(), r.PathValue("stream"), r.PathValue("batch"))
		if err != nil {
			if errors.Is(err, evidence.ErrReceiptNotFound) {
				fail(w, 404, "receipt", "not_found")
			} else {
				fail(w, 503, "receipt", "unavailable")
			}
			return
		}
		respond(w, responseStatus(response), response)
	})
	return mux
}
func AdminHandler(processor Reprocessor, prefix string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+prefix+"reprocess", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 3))
		if err != nil || len(body) != 0 && string(body) != "{}" {
			fail(w, 400, "validation", "invalid_request")
			return
		}
		generation, err := processor.Reprocess(r.Context())
		if err != nil {
			fail(w, 503, "database", "unavailable")
			return
		}
		respond(w, 202, struct {
			Generation int64 `json:"generation"`
		}{generation})
	})
	return mux
}
