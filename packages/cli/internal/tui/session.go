package tui

import (
	"context"
	"errors"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
)

// Session is one viewer lease over shared application queries and progress.
// Collect cancels only the caller's wait; shared work belongs to the backend.
// Close releases this lease without closing resources held by other viewers.
type Session interface {
	Queries() *queryclient.Client
	Progress(context.Context) (collectorprogress.DetailSnapshot, error)
	ProcessingStatus(context.Context) (analytics.ProcessingStatus, error)
	Collect(context.Context, bool, string) (localprotocol.CollectionResult, error)
	Done() <-chan struct{}
	Close() error
}

type localSession struct{ *localclient.Session }

func (s localSession) Queries() *queryclient.Client { return s.Query }

func connectSession(ctx context.Context, settings config.LocalSettings, prepare func() error) (Session, error) {
	session, err := localclient.ConnectPrepared(ctx, localclient.Options{
		CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath,
		AppDBPath: settings.ApplicationPath(), Host: settings.Host, Port: settings.Port,
	}, prepare)
	if err != nil {
		return nil, err
	}
	return localSession{session}, nil
}

type collectionResult struct {
	Result localprotocol.CollectionResult
	Err    error
}

type sessionDisconnectedMsg struct{}

var errSessionDisconnected = errors.New("local backend disconnected")
