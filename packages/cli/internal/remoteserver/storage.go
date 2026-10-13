package remoteserver

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/postgres"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlite"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqliteaccounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverruntime"
)

type accountLifecycle interface {
	accounts.Repository
	Resume(context.Context) error
}
type hostedStorage struct {
	ctx      context.Context
	source   server.DataSource
	worker   serverruntime.Worker
	accounts accountLifecycle
	socket   string
	close    func() error
	failure  func() error
}
type readyWorker struct {
	serverruntime.Worker
	ready func(context.Context) error
}

func (w readyWorker) Ready(ctx context.Context) error { return w.ready(ctx) }

func openStorage(ctx context.Context, settings Settings) (*hostedStorage, error) {
	if settings.Backend == "postgres" {
		s, err := postgres.Open(ctx, settings.PostgresDSN)
		if err != nil {
			return nil, err
		}
		return &hostedStorage{ctx: s.Owner.Context(), source: sqlanalytics.Source{Store: s.Tokens}, worker: readyWorker{Worker: s.Tokens, ready: s.Owner.Ready}, accounts: s.Accounts, socket: settings.AdminSocket, close: s.Owner.Close, failure: func() error {
			if ctx.Err() != nil {
				return nil
			}
			return context.Cause(s.Owner.Context())
		}}, nil
	}
	path, _, err := serverownership.Identify(settings.DBPath)
	if err != nil {
		return nil, err
	}
	appPath := settings.AppDBPath
	if appPath == "" {
		appPath = filepath.Join(filepath.Dir(path), "app.sqlite")
	}
	if err := collector.ValidatePaths(appPath, path); err != nil {
		return nil, err
	}
	if err := collector.ValidatePaths(appPath, path+".application.json"); err != nil {
		return nil, err
	}
	release, err := db.AcquireWriterLock(ctx, path+".service.op")
	if err != nil {
		return nil, err
	}
	defer release()
	owner, held, err := serverownership.Lifetime(path, true)
	if err != nil {
		return nil, err
	}
	if held {
		return nil, errors.New("server already owns database")
	}
	cleanup := func() error { return owner.Close() }
	complete := false
	defer func() {
		if !complete {
			_ = cleanup()
		}
	}()
	tokens, err := sqlite.Open(ctx, path, datastore.Options{Kind: datastore.KindHosted})
	if err != nil {
		return nil, err
	}
	cleanup = func() error { return errors.Join(tokens.Close(), owner.Close()) }
	id, err := tokens.DatabaseIdentity(ctx)
	if err != nil {
		return nil, err
	}
	app, err := appstore.OpenPaired(ctx, appPath, path, id, datastore.KindHosted)
	if err != nil {
		return nil, err
	}
	cleanup = func() error { return errors.Join(app.Close(), tokens.Close(), owner.Close()) }
	repository := sqliteaccounts.NewSQLite(app, tokens)
	socket := settings.AdminSocket
	if socket == "" {
		socket = path + ".admin.sock"
	}
	ready := func(ctx context.Context) error {
		if err := tokens.Ready(ctx); err != nil {
			return err
		}
		return app.Ready(ctx, id, datastore.KindHosted)
	}
	complete = true
	return &hostedStorage{ctx: ctx, source: sqlanalytics.Source{Store: tokens}, worker: readyWorker{Worker: tokens, ready: ready}, accounts: repository, socket: socket, close: cleanup, failure: func() error { return nil }}, nil
}
