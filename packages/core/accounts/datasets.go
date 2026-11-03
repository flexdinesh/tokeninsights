package accounts

import "context"

// Datasets is the account lifecycle's token-store contract.
type Datasets interface {
	// EnsureDataset is idempotent and must preserve any existing history.
	EnsureDataset(context.Context, string) error
	DatasetExists(context.Context, string) (bool, error)
	// ReprocessDataset retains published history while a replacement builds.
	ReprocessDataset(context.Context, string) (int64, error)
}
