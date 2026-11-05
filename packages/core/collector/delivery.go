package collector

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
)

// Delivery transports immutable saved requests. Acceptance never means that
// processing has finished. Implementations must preserve exact receipt bindings.
type Delivery interface {
	Capabilities(context.Context) (evidence.Capabilities, error)
	Submit(context.Context, int, []byte) ([]byte, error)
	Receipt(context.Context, string, string) (evidence.Response, error)
}

// DirectDelivery uses an authorized receiver without HTTP or synthetic requests.
type DirectDelivery struct{ Receiver evidence.Receiver }

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
	result, err := d.Receiver.Accept(ctx, protocol, body)
	if err != nil {
		return nil, directFailure(err)
	}
	return json.Marshal(result)
}

func (d DirectDelivery) Receipt(ctx context.Context, stream, batch string) (evidence.Response, error) {
	return d.Receiver.Receipt(ctx, stream, batch)
}

func directFailure(err error) error {
	var invalid *evidence.ValidationError
	if errors.As(err, &invalid) {
		return failure("validation", invalid.Code, err)
	}
	var rejected evidence.Rejection
	if errors.As(err, &rejected) {
		return failure("admission", rejected.IngestionCode(), err)
	}
	return failure("admission", "transaction_failed", err)
}
