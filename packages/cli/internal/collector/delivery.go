package collector

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

// Delivery transports immutable saved requests. Acceptance never means that
// processing has finished. Implementations must preserve exact receipt bindings.
type Delivery interface {
	Capabilities(context.Context) (evidence.Capabilities, error)
	Submit(context.Context, int, []byte) ([]byte, error)
	Receipt(context.Context, string, string) (evidence.Response, error)
}

type HTTPDelivery struct {
	URL    string
	Token  string
	Client *http.Client
}

func (d HTTPDelivery) call(ctx context.Context, route string, body []byte) ([]byte, error) {
	target, err := endpoint(d.URL)
	if err != nil {
		return nil, err
	}
	return request(ctx, deliveryClient(d.Client), target+route, d.Token, body)
}

func (d HTTPDelivery) Capabilities(ctx context.Context) (evidence.Capabilities, error) {
	var result evidence.Capabilities
	body, err := d.call(ctx, "/api/v3/ingestion/capabilities", nil)
	if err == nil {
		if decodeErr := evidence.StrictDecode(body, &result); decodeErr != nil {
			err = failure("capabilities", "incompatible_server", decodeErr)
		}
	}
	return result, err
}

func (d HTTPDelivery) Submit(ctx context.Context, protocol int, body []byte) ([]byte, error) {
	if protocol != evidence.ProtocolVersion {
		return nil, failure("validation", "incompatible", nil)
	}
	return d.call(ctx, "/api/v3/ingestion/batches", body)
}

func (d HTTPDelivery) Receipt(ctx context.Context, stream, batch string) (evidence.Response, error) {
	var result evidence.Response
	body, err := d.call(ctx, "/api/v3/ingestion/batches/"+url.PathEscape(stream)+"/"+url.PathEscape(batch), nil)
	if err == nil {
		err = evidence.StrictDecode(body, &result)
	}
	return result, err
}

// Receiver is an already authorized dataset. Its atomic operations are shared
// by the direct and HTTP adapters; no listener or synthetic HTTP request is used.
type Receiver interface {
	RawCapabilities(context.Context) (evidence.Capabilities, error)
	Accept(context.Context, []byte) (evidence.Response, error)
	Receipt(context.Context, string, string) (evidence.Response, error)
	AcquireAdmission() (func(), bool)
}

type DirectDelivery struct{ Receiver Receiver }

func (d DirectDelivery) Capabilities(ctx context.Context) (evidence.Capabilities, error) {
	return d.Receiver.RawCapabilities(ctx)
}

func (d DirectDelivery) Submit(ctx context.Context, protocol int, body []byte) ([]byte, error) {
	release, allowed := d.Receiver.AcquireAdmission()
	if !allowed {
		return nil, failure("admission", "busy", nil)
	}
	defer release()
	if len(body) > evidence.MaxBodyBytes {
		return nil, failure("validation", "body_limit", nil)
	}
	batch, err := evidence.DecodeBatch(body)
	if err != nil {
		if err.Error() == "incompatible" {
			return nil, failure("validation", "incompatible", err)
		}
		return nil, failure("validation", "invalid_request", err)
	}
	if batch.ProtocolVersion != protocol {
		return nil, failure("validation", "incompatible", nil)
	}
	result, err := d.Receiver.Accept(ctx, body)
	if err != nil {
		return nil, directFailure(err)
	}
	return json.Marshal(result)
}

func (d DirectDelivery) Receipt(ctx context.Context, stream, batch string) (evidence.Response, error) {
	return d.Receiver.Receipt(ctx, stream, batch)
}

func directFailure(err error) error {
	var rejected interface{ IngestionCode() string }
	if errors.As(err, &rejected) {
		return failure("admission", rejected.IngestionCode(), err)
	}
	return failure("admission", "transaction_failed", err)
}
