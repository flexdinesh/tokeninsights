package datastore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

const IngestionPrefix = "/api/v2/ingestion/"
const LegacyIngestionPrefix = "/api/v1/ingestion/"
const ProcessingPrefix = "/api/v2/processing/"

func respond(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (s *Store) Handler() http.Handler {
	mux := http.NewServeMux()
	s.mountLegacy(mux)
	mux.HandleFunc("POST "+ProcessingPrefix+"reprocess", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 3))
		if err != nil || len(body) != 0 && string(body) != "{}" {
			respond(w, 400, publication.ErrorResponse{Stage: "validation", Code: "invalid_request"})
			return
		}
		generation, err := s.Reprocess(r.Context())
		if err != nil {
			respond(w, 503, publication.ErrorResponse{Stage: "database", Code: "unavailable"})
			return
		}
		respond(w, 202, struct {
			Generation int64 `json:"generation"`
		}{generation})
	})
	mux.HandleFunc("GET "+IngestionPrefix+"capabilities", func(w http.ResponseWriter, r *http.Request) {
		metadata, err := s.Metadata(r.Context())
		if err != nil {
			respond(w, 503, publication.ErrorResponse{Stage: "database", Code: "unavailable"})
			return
		}
		respond(w, 200, evidence.Capabilities{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, Completion: "acceptance", MaxBodyBytes: evidence.MaxBodyBytes, MaxEntries: evidence.MaxEntries})
	})
	mux.HandleFunc("POST "+IngestionPrefix+"batches", func(w http.ResponseWriter, r *http.Request) {
		select {
		case s.admissions <- struct{}{}:
			defer func() { <-s.admissions }()
		default:
			respond(w, 503, publication.ErrorResponse{Stage: "admission", Code: "busy"})
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			respond(w, 400, publication.ErrorResponse{Stage: "validation", Code: "invalid_request"})
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, evidence.MaxBodyBytes+1))
		if err != nil {
			respond(w, 400, publication.ErrorResponse{Stage: "validation", Code: "body_read_failed"})
			return
		}
		if len(body) > evidence.MaxBodyBytes {
			respond(w, 413, publication.ErrorResponse{Stage: "validation", Code: "body_limit"})
			return
		}
		response, err := s.Accept(r.Context(), body)
		if err != nil {
			var admission *AdmissionError
			status := 503
			code := "transaction_failed"
			if errors.As(err, &admission) {
				code = admission.Code
				status = 422
				if strings.HasSuffix(code, "conflict") || code == "database_mismatch" {
					status = 409
				}
				if code == "invalid_request" {
					status = 400
				}
			}
			respond(w, status, publication.ErrorResponse{Stage: "admission", Code: code})
			return
		}
		status := 202
		if response.Processing.Pending == 0 {
			status = 200
		}
		respond(w, status, response)
	})
	mux.HandleFunc("GET "+IngestionPrefix+"batches/{stream}/{batch}", func(w http.ResponseWriter, r *http.Request) {
		response, err := s.Receipt(r.Context(), r.PathValue("stream"), r.PathValue("batch"))
		if err != nil {
			status := 503
			code := "unavailable"
			if errors.Is(err, sql.ErrNoRows) {
				status = 404
				code = "not_found"
			}
			respond(w, status, publication.ErrorResponse{Stage: "receipt", Code: code})
			return
		}
		status := 202
		if response.Processing.Pending == 0 {
			status = 200
		}
		respond(w, status, response)
	})
	return mux
}
