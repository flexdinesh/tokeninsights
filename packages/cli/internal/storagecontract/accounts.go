package storagecontract

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
)

type Accounts struct {
	Repository   accounts.Repository
	Datasets     accounts.Datasets
	Resume       func(context.Context) error
	Reopen       func() Accounts
	WithDatasets func(accounts.Datasets) accounts.Repository
}

const accountPassword = "synthetic-password-one"
const changedAccountPassword = "synthetic-password-two"

// RunAccounts checks identical identity, credential and provisioning semantics
// against real SQLite and PostgreSQL, including close/reopen boundaries.
func RunAccounts(t *testing.T, factory func(*testing.T) Accounts) {
	t.Helper()
	t.Run("bootstrap_initializes_once", func(t *testing.T) {
		store := factory(t)
		if err := store.Repository.BootstrapUser(t.Context(), "Admin", accountPassword); err != nil {
			t.Fatal(err)
		}
		session, _, err := store.Repository.CreateSession(t.Context(), "ADMIN", accountPassword)
		if err != nil {
			t.Fatal(err)
		}
		principal, err := store.Repository.AuthenticateSession(t.Context(), session)
		if err != nil || principal.UserID != "bootstrap-user" || principal.DatasetID != "bootstrap-user" {
			t.Fatal(principal, err)
		}
		if err := store.Repository.ChangePassword(t.Context(), principal.UserID, accountPassword, changedAccountPassword); err != nil {
			t.Fatal(err)
		}
		store = store.Reopen()
		if err := store.Repository.BootstrapUser(t.Context(), "admin", accountPassword); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Repository.CreateSession(t.Context(), "admin", accountPassword); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("bootstrap reset password", err)
		}
		if _, _, err := store.Repository.CreateSession(t.Context(), "admin", changedAccountPassword); err != nil {
			t.Fatal(err)
		}
		if err := store.Repository.BootstrapUser(t.Context(), "another", accountPassword); err == nil {
			t.Fatal("conflicting bootstrap username accepted")
		}
		if err := store.Repository.DisableUser(t.Context(), principal.UserID); err != nil {
			t.Fatal(err)
		}
		if err := store.Repository.BootstrapUser(t.Context(), "admin", accountPassword); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Repository.CreateSession(t.Context(), "admin", changedAccountPassword); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("bootstrap reenabled user", err)
		}
	})
	t.Run("bootstrap_username_conflict", func(t *testing.T) {
		store := factory(t)
		if _, err := store.Repository.Signup(t.Context(), "admin", accountPassword); err != nil {
			t.Fatal(err)
		}
		if err := store.Repository.BootstrapUser(t.Context(), "ADMIN", accountPassword); !errors.Is(err, accounts.ErrUsernameTaken) {
			t.Fatal("bootstrap claimed existing username", err)
		}
	})
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "bootstrap_interrupted_before_dataset", true: "bootstrap_interrupted_after_dataset"}[after], func(t *testing.T) {
			store := factory(t)
			interrupted := store.WithDatasets(interruptedDatasets{Datasets: store.Datasets, after: after})
			if err := interrupted.BootstrapUser(t.Context(), "admin", accountPassword); err == nil {
				t.Fatal("bootstrap interruption not exposed")
			}
			store = store.Reopen()
			if err := store.Resume(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := store.Repository.BootstrapUser(t.Context(), "admin", changedAccountPassword); err != nil {
				t.Fatal(err)
			}
			session, _, err := store.Repository.CreateSession(t.Context(), "admin", accountPassword)
			if err != nil {
				t.Fatal("initial password lost", err)
			}
			p, err := store.Repository.AuthenticateSession(t.Context(), session)
			if err != nil || p.UserID != "bootstrap-user" || p.DatasetID != "bootstrap-user" {
				t.Fatal("recovery changed identity", p, err)
			}
		})
		t.Run(map[bool]string{false: "signup_interrupted_before_dataset", true: "signup_interrupted_after_dataset"}[after], func(t *testing.T) {
			store := factory(t)
			interrupted := store.WithDatasets(interruptedDatasets{Datasets: store.Datasets, after: after})
			pending, err := interrupted.Signup(t.Context(), "Alice", accountPassword)
			if err == nil || pending.UserID == "" || pending.DatasetID == "" {
				t.Fatal("pending identity lost", pending, err)
			}
			if _, _, err := interrupted.CreateSession(t.Context(), "alice", accountPassword); !errors.Is(err, accounts.ErrUnauthenticated) {
				t.Fatal("pending user authenticated", err)
			}
			if _, err := store.Repository.Signup(t.Context(), "ALICE", changedAccountPassword); !errors.Is(err, accounts.ErrUsernameTaken) {
				t.Fatal("pending signup claimed by wrong password", err)
			}
			recovered, err := store.Repository.Signup(t.Context(), "ALICE", accountPassword)
			if err != nil || recovered.UserID != pending.UserID || recovered.DatasetID != pending.DatasetID {
				t.Fatal("retry changed identity", recovered, err)
			}
			store = store.Reopen()
			for range 2 {
				if err := store.Resume(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			session, _, err := store.Repository.CreateSession(t.Context(), "alice", accountPassword)
			if err != nil {
				t.Fatal(err)
			}
			p, err := store.Repository.AuthenticateSession(t.Context(), session)
			if err != nil || p.DatasetID != pending.DatasetID {
				t.Fatal(p, err)
			}
			if err := store.Repository.DisableUser(t.Context(), pending.UserID); err != nil {
				t.Fatal(err)
			}
			if err := store.Resume(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.Repository.CreateSession(t.Context(), "alice", accountPassword); !errors.Is(err, accounts.ErrUnauthenticated) {
				t.Fatal("resume reenabled user", err)
			}
		})
	}
	t.Run("signup_uniqueness_concurrent_case", func(t *testing.T) {
		store := factory(t)
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, candidate := range []struct{ name, password string }{{"Alice", accountPassword}, {" ALICE ", changedAccountPassword}} {
			wg.Go(func() {
				_, err := store.Repository.Signup(t.Context(), candidate.name, candidate.password)
				results <- err
			})
		}
		wg.Wait()
		close(results)
		succeeded, conflicted := 0, 0
		for err := range results {
			if err == nil {
				succeeded++
			} else if errors.Is(err, accounts.ErrUsernameTaken) {
				conflicted++
			} else {
				t.Fatal(err)
			}
		}
		if succeeded != 1 || conflicted != 1 {
			t.Fatal("duplicate usernames committed", succeeded, conflicted)
		}
		_, _, first := store.Repository.CreateSession(t.Context(), "ALICE", accountPassword)
		_, _, second := store.Repository.CreateSession(t.Context(), "ALICE", changedAccountPassword)
		if (first == nil) == (second == nil) {
			t.Fatal("concurrent signup credential ownership", first, second)
		}
		if _, _, err := store.Repository.CreateSession(t.Context(), "unknown", accountPassword); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal(err)
		}
	})
	t.Run("durable_session_key_independence_and_ownership", func(t *testing.T) {
		store := factory(t)
		a, err := store.Repository.Signup(t.Context(), "alice", accountPassword)
		if err != nil {
			t.Fatal(err)
		}
		b, err := store.Repository.Signup(t.Context(), "bob", accountPassword)
		if err != nil {
			t.Fatal(err)
		}
		if !a.Enabled || !b.Enabled || a.UserID == b.UserID || a.DatasetID == b.DatasetID {
			t.Fatal("user identity contract", a, b)
		}
		for _, u := range []accounts.User{a, b} {
			exists, err := store.Datasets.DatasetExists(t.Context(), u.DatasetID)
			if err != nil || !exists {
				t.Fatal("active user has no dataset", u, err)
			}
		}
		at, err := store.Repository.CreateNamedToken(t.Context(), a.UserID, "Work laptop", []string{accounts.Read, accounts.Ingest}, nil)
		if err != nil {
			t.Fatal(err)
		}
		bt, err := store.Repository.CreateNamedToken(t.Context(), b.UserID, "Personal laptop", []string{accounts.Read}, nil)
		if err != nil {
			t.Fatal(err)
		}
		session, expiry, err := store.Repository.CreateSession(t.Context(), "alice", accountPassword)
		if err != nil || expiry.Before(time.Now().Add(accounts.SessionLifetime-time.Minute)) {
			t.Fatal(session, expiry, err)
		}
		p, err := store.Repository.AuthenticateSession(t.Context(), session)
		if err != nil || p.DatasetID != a.DatasetID || !p.HasPermission(accounts.Read) || p.HasPermission(accounts.Ingest) || p.CredentialKind != accounts.SessionCredential {
			t.Fatal("session permission contract", p, err)
		}
		bearer, err := store.Repository.AuthenticateBearer(t.Context(), at.Secret)
		if err != nil || !bearer.HasPermission(accounts.Ingest) || bearer.CredentialKind != accounts.BearerCredential {
			t.Fatal(bearer, err)
		}
		service := accounts.New(store.Repository)
		if _, err := service.CurrentUser(t.Context(), bearer); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("bearer accessed settings", err)
		}
		if _, err := service.CreateOwnToken(t.Context(), bearer, "nested key", []string{accounts.Read}, nil); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("bearer created key", err)
		}
		if err := service.ChangeOwnPassword(t.Context(), bearer, accountPassword, changedAccountPassword); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("bearer changed password", err)
		}
		if _, err := service.ListOwnTokens(t.Context(), bearer); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("bearer listed keys", err)
		}
		if err := service.RevokeOwnToken(t.Context(), bearer, at.TokenID); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("bearer revoked key", err)
		}
		if err := store.Repository.RevokeUserToken(t.Context(), a.UserID, bt.TokenID); !errors.Is(err, accounts.ErrNotFound) {
			t.Fatal("cross-user revoke", err)
		}
		keys, err := store.Repository.ListTokens(t.Context(), a.UserID)
		if err != nil || len(keys) != 1 || keys[0].TokenID != at.TokenID || keys[0].Label != "Work laptop" || keys[0].CreatedAt.IsZero() {
			t.Fatal("key isolation/metadata", keys, err)
		}
		if err := store.Repository.RevokeUserToken(t.Context(), a.UserID, at.TokenID); err != nil {
			t.Fatal(err)
		}
		store = store.Reopen()
		if _, err := store.Repository.AuthenticateBearer(t.Context(), at.Secret); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("revocation lost", err)
		}
		if _, err := store.Repository.AuthenticateSession(t.Context(), session); err != nil {
			t.Fatal("API-key revocation invalidated password session", err)
		}
		if p, err := store.Repository.AuthenticateBearer(t.Context(), bt.Secret); err != nil || p.DatasetID != b.DatasetID {
			t.Fatal("other account affected", p, err)
		}
		keys, err = store.Repository.ListTokens(t.Context(), a.UserID)
		if err != nil || len(keys) != 1 || keys[0].RevokedAt == nil {
			t.Fatal("revoked metadata lost", keys, err)
		}
		if err := store.Repository.Logout(t.Context(), session); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Repository.AuthenticateSession(t.Context(), session); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("logout ignored", err)
		}
		rotated, err := store.Repository.CreateToken(t.Context(), a.UserID, []string{accounts.Read}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if p, err := store.Repository.AuthenticateBearer(t.Context(), rotated.Secret); err != nil || p.DatasetID != a.DatasetID {
			t.Fatal("rotation changed dataset", p, err)
		}
		liveSession, _, err := store.Repository.CreateSession(t.Context(), "alice", accountPassword)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Repository.DisableUser(t.Context(), a.UserID); err != nil {
			t.Fatal(err)
		}
		if err := store.Resume(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Repository.AuthenticateSession(t.Context(), liveSession); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("disabled user session accepted", err)
		}
		if _, err := store.Repository.AuthenticateBearer(t.Context(), rotated.Secret); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("disabled key works", err)
		}
		if err := store.Repository.SetPassword(t.Context(), a.UserID, changedAccountPassword); !errors.Is(err, accounts.ErrNotFound) {
			t.Fatal("reset changed disabled account", err)
		}
		if exists, err := store.Datasets.DatasetExists(t.Context(), a.DatasetID); err != nil || !exists {
			t.Fatal("disable deleted dataset", err)
		}
	})
	t.Run("password_change_and_operator_reset", func(t *testing.T) {
		store := factory(t)
		user, err := store.Repository.CreateUser(t.Context(), "alice")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Repository.CreateSession(t.Context(), "alice", accountPassword); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("passwordless login", err)
		}
		if err := store.Repository.SetPassword(t.Context(), user.UserID, accountPassword); err != nil {
			t.Fatal(err)
		}
		key, err := store.Repository.CreateToken(t.Context(), user.UserID, []string{accounts.Read}, nil)
		if err != nil {
			t.Fatal(err)
		}
		var sessions []string
		for range 2 {
			session, _, err := store.Repository.CreateSession(t.Context(), "alice", accountPassword)
			if err != nil {
				t.Fatal(err)
			}
			sessions = append(sessions, session)
		}
		if err := store.Repository.ChangePassword(t.Context(), user.UserID, changedAccountPassword, changedAccountPassword); !errors.Is(err, accounts.ErrCurrentPasswordMismatch) {
			t.Fatal("wrong current password accepted", err)
		}
		if _, err := store.Repository.AuthenticateSession(t.Context(), sessions[0]); err != nil {
			t.Fatal("rejected change invalidated session", err)
		}
		if err := store.Repository.ChangePassword(t.Context(), user.UserID, accountPassword, changedAccountPassword); err != nil {
			t.Fatal(err)
		}
		store = store.Reopen()
		for _, session := range sessions {
			if _, err := store.Repository.AuthenticateSession(t.Context(), session); !errors.Is(err, accounts.ErrUnauthenticated) {
				t.Fatal("password change retained session", err)
			}
		}
		if _, _, err := store.Repository.CreateSession(t.Context(), "alice", accountPassword); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("old password retained", err)
		}
		session, _, err := store.Repository.CreateSession(t.Context(), "alice", changedAccountPassword)
		if err != nil {
			t.Fatal(err)
		}
		p, err := store.Repository.AuthenticateSession(t.Context(), session)
		if err != nil || p.DatasetID != user.DatasetID {
			t.Fatal("password change altered dataset", p, err)
		}
		if _, err := store.Repository.AuthenticateBearer(t.Context(), key.Secret); err != nil {
			t.Fatal("password change revoked independent key", err)
		}
		if err := store.Repository.SetPassword(t.Context(), user.UserID, accountPassword); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Repository.AuthenticateSession(t.Context(), session); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("operator reset retained session", err)
		}
	})
	t.Run("password_reset_fences_concurrent_login", func(t *testing.T) {
		store := factory(t)
		user, err := store.Repository.Signup(t.Context(), "alice", accountPassword)
		if err != nil {
			t.Fatal(err)
		}
		type loginResult struct {
			secret string
			err    error
		}
		result := make(chan loginResult, 1)
		var wg sync.WaitGroup
		wg.Go(func() {
			secret, _, err := store.Repository.CreateSession(t.Context(), "alice", accountPassword)
			result <- loginResult{secret: secret, err: err}
		})
		if err := store.Repository.SetPassword(t.Context(), user.UserID, changedAccountPassword); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
		login := <-result
		if login.err != nil && !errors.Is(login.err, accounts.ErrUnauthenticated) {
			t.Fatal(login.err)
		}
		if login.err == nil {
			if _, err := store.Repository.AuthenticateSession(t.Context(), login.secret); !errors.Is(err, accounts.ErrUnauthenticated) {
				t.Fatal("concurrent old-password session survived reset", err)
			}
		}
		if _, _, err := store.Repository.CreateSession(t.Context(), "alice", accountPassword); !errors.Is(err, accounts.ErrUnauthenticated) {
			t.Fatal("old password survived reset", err)
		}
		if _, _, err := store.Repository.CreateSession(t.Context(), "alice", changedAccountPassword); err != nil {
			t.Fatal("new password unavailable", err)
		}
	})
	t.Run("permissions_expiry_and_invalid_inputs", func(t *testing.T) {
		store := factory(t)
		user, err := store.Repository.CreateUser(t.Context(), "alice")
		if err != nil {
			t.Fatal(err)
		}
		for _, scopes := range [][]string{nil, {accounts.Read, accounts.Read}, {"admin"}} {
			if _, err := store.Repository.CreateToken(t.Context(), user.UserID, scopes, nil); !errors.Is(err, accounts.ErrInvalidPermissions) {
				t.Fatal("invalid scopes accepted", scopes, err)
			}
		}
		expiry := time.Now().Add(-time.Second)
		if _, err := store.Repository.CreateToken(t.Context(), user.UserID, []string{accounts.Read}, &expiry); !errors.Is(err, accounts.ErrInvalidExpiry) {
			t.Fatal("expired key created", err)
		}
		if _, err := store.Repository.CreateNamedToken(t.Context(), user.UserID, " ", []string{accounts.Read}, nil); !errors.Is(err, accounts.ErrInvalidLabel) {
			t.Fatal("empty key label", err)
		}
		label := strings.Repeat("😀", accounts.MaxKeyLabelCharacters)
		if _, err := store.Repository.CreateNamedToken(t.Context(), user.UserID, label, []string{accounts.Read}, nil); err != nil {
			t.Fatal("Unicode label counted as bytes", err)
		}
		if _, err := store.Repository.CreateNamedToken(t.Context(), user.UserID, label+"😀", []string{accounts.Read}, nil); !errors.Is(err, accounts.ErrInvalidLabel) {
			t.Fatal("overlong label accepted", err)
		}
		if _, err := store.Repository.Signup(t.Context(), "valid-name", "short"); !errors.Is(err, accounts.ErrInvalidPassword) {
			t.Fatal("short password", err)
		}
	})
}

type interruptedDatasets struct {
	accounts.Datasets
	after bool
}

func (d interruptedDatasets) EnsureDataset(ctx context.Context, id string) error {
	if d.after {
		if err := d.Datasets.EnsureDataset(ctx, id); err != nil {
			return err
		}
	}
	return errors.New("synthetic provisioning interruption")
}
