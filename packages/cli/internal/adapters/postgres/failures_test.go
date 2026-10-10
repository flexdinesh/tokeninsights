package postgres

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/postgres/testdb"
)

func TestFutureProcessorRejectsBeforeAnyGenerationChanges(t *testing.T) {
	dsn := testdb.New(t)
	s := openTest(t, dsn)
	for _, id := range []string{"a-old", "z-future"} {
		if err := s.Tokens.EnsureDataset(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Owner.WriteTransaction(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), "UPDATE analytics_generations SET processor_version=CASE WHEN dataset_id='a-old' THEN 0 ELSE $1 END", evidence.ProcessorVersion+1)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Owner.Close(); err != nil {
		t.Fatal(err)
	}
	if other, err := Open(t.Context(), dsn); err == nil {
		_ = other.Owner.Close()
		t.Fatal("future processor accepted")
	}
	db := testdb.Open(t, dsn)
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM tokeninsights_data.analytics_generations").Scan(&count); err != nil || count != 2 {
		t.Fatal("rejection staged another dataset generation", count, err)
	}
}

func TestTimestampCredentialsAndCleanup(t *testing.T) {
	s := openTest(t, testdb.New(t))
	user, err := s.Accounts.CreateUser(t.Context(), "Alice")
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(time.Hour)
	token, err := s.Accounts.CreateToken(t.Context(), user.UserID, []string{accounts.Read}, &expiry)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := s.Accounts.CreateSession(t.Context(), token.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Accounts.AuthenticateSession(t.Context(), session); err != nil {
		t.Fatal("valid TIMESTAMPTZ rejected", err)
	}
	if err := s.Owner.WriteTransaction(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), "UPDATE tokens SET expires_at=now()-interval '1 second' WHERE token_id=$1", token.TokenID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Accounts.AuthenticateSession(t.Context(), session); !errors.Is(err, accounts.ErrUnauthenticated) {
		t.Fatal("expired source token authorized session", err)
	}
	if err := s.Accounts.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.Owner.Reader.QueryRowContext(t.Context(), "SELECT (SELECT COUNT(*) FROM tokens)+(SELECT COUNT(*) FROM sessions)").Scan(&count); err != nil || count != 0 {
		t.Fatal("expired credentials retained", count, err)
	}
	if exists, err := s.Tokens.DatasetExists(t.Context(), user.DatasetID); err != nil || !exists {
		t.Fatal("cleanup removed dataset", err)
	}
}
