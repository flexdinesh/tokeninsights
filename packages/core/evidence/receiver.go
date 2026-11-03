package evidence

import (
	"context"
	"errors"
)

const PersonalDatasetID = "default"

var ErrReceiptNotFound = errors.New("receipt_not_found")

// Rejection reports a safe ingestion code independently of driver errors.
// Invalid bytes/protocol use ValidationError; identity conflicts use this contract.
type Rejection interface {
	error
	IngestionCode() string
}

// Receiver is bound to one authorized dataset. Accept validates exact bytes and
// commits evidence, immutable receipt and pending work atomically. Exact retries
// return the original receipt; conflicting identity bindings reject without writes.
// Receipt never searches another dataset and returns ErrReceiptNotFound if absent.
// Cancellation may race commit: callers retry identical bytes to resolve uncertainty.
type Receiver interface {
	RawCapabilities(context.Context) (Capabilities, error)
	Accept(context.Context, int, []byte) (Response, error)
	Receipt(context.Context, string, string) (Response, error)
	// Successful admission supplies a release function; all dataset views share
	// the adapter's bounded admission pool. This is not a distributed lease.
	AcquireAdmission() (release func(), allowed bool)
}
