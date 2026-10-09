package accounts

import (
	"context"
	"errors"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
)

type interruptedDatasets struct {
	*datastore.Store
	after bool
}

func (d interruptedDatasets) EnsureDataset(ctx context.Context, id string) error {
	if d.after {
		if err := d.Store.EnsureDataset(ctx, id); err != nil {
			return err
		}
	}
	return errors.New("simulated interruption")
}
func TestProvisioningResumesSameIdentityBeforeAndAfterDatasetCreation(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "after"}[after], func(t *testing.T) {
			s, store := testAccounts(t)
			repository := s.repository.(*SQLite)
			repository.datasets = interruptedDatasets{Store: store.data, after: after}
			pending, err := s.CreateUser(t.Context(), "Alice")
			if err == nil || pending.UserID == "" || pending.DatasetID == "" {
				t.Fatal("lost pending identity", pending, err)
			}
			if _, err := s.CreateToken(t.Context(), pending.UserID, []string{Read}, nil); err == nil {
				t.Fatal("pending user minted token")
			}
			repository.datasets = store.data
			if err := repository.Resume(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := repository.Resume(t.Context()); err != nil {
				t.Fatal("resume not idempotent", err)
			}
			token, err := s.CreateToken(t.Context(), pending.UserID, []string{Read}, nil)
			if err != nil {
				t.Fatal(err)
			}
			principal, err := s.AuthenticateBearer(t.Context(), token.Secret)
			if err != nil || principal.DatasetID != pending.DatasetID {
				t.Fatal("provision changed identity", principal, err)
			}
			if err := s.DisableUser(t.Context(), pending.UserID); err != nil {
				t.Fatal(err)
			}
			if err := repository.Resume(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AuthenticateBearer(t.Context(), token.Secret); !errors.Is(err, ErrUnauthenticated) {
				t.Fatal("resume reenabled user", err)
			}
		})
	}
}
