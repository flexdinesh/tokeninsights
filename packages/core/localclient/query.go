package localclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

type privateReader struct{ session *Session }

var _ application.Reader = privateReader{}

func (p privateReader) Instance(ctx context.Context) (application.Instance, error) {
	var result application.Instance
	err := p.read(ctx, "instance", nil, &result)
	return result, err
}
func (p privateReader) Status(ctx context.Context) (application.Status, error) {
	var result application.Status
	err := p.read(ctx, "status", nil, &result)
	return result, err
}
func (p privateReader) Usage(ctx context.Context, request analytics.Query) (application.Usage, error) {
	var result application.Usage
	err := p.read(ctx, "usage", request, &result)
	return result, err
}
func (p privateReader) AllUsage(ctx context.Context, request analytics.Query, maxRows int) (application.Usage, error) {
	var result application.Usage
	err := p.read(ctx, "all", localprotocol.AllUsageRequest{Query: request, MaxRows: maxRows}, &result)
	if err == nil && (len(result.Rows) > maxRows || len(result.Rows) != result.RowCount) {
		return application.Usage{}, errors.New("invalid local analytics snapshot")
	}
	return result, err
}
func (p privateReader) Facets(ctx context.Context, request application.FacetRequest) (application.Facets, error) {
	var result application.Facets
	err := p.read(ctx, "facets", request, &result)
	return result, err
}

func (p privateReader) read(parent context.Context, operation string, value, target interface{}) error {
	ctx, cancel := context.WithTimeout(parent, operationTimeout)
	defer cancel()
	var body bytes.Buffer
	if value != nil {
		if err := json.NewEncoder(&body).Encode(value); err != nil {
			return err
		}
	}
	r, err := p.session.request(ctx, http.MethodPost, "/queries/"+operation, &body)
	if err != nil {
		return err
	}
	response, err := p.session.client.Do(r)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case http.StatusBadRequest:
			return &application.InvalidRequest{Message: "invalid local analytics query"}
		case http.StatusForbidden:
			return application.ErrForbidden
		case http.StatusServiceUnavailable:
			return application.ErrUnavailable
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("local analytics query failed")
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, localprotocol.MaxQueryResponseBytes+1))
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid local analytics response")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("invalid local analytics response tail")
	}
	return nil
}
