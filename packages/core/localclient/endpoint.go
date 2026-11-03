package localclient

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

const endpointProbeTimeout = 250 * time.Millisecond

var ErrEndpointMissing = errors.New("local dashboard endpoint has no matching private owner")

// ConnectEndpoint attaches an existing local owner identified by URL and public
// instance. It never resolves default settings or starts another backend. A
// dashboard session also leases that listener, keeping the URL serving.
func ConnectEndpoint(parent context.Context, targetURL string, capture, dashboard bool) (*Session, error) {
	target, err := localprotocol.HTTPOrigin(targetURL, true)
	if err != nil {
		return nil, errors.New("desktop endpoint must be a loopback HTTP origin")
	}
	ctx, cancel := context.WithTimeout(parent, StartupTimeout)
	defer cancel()
	public, err := queryclient.New(target, nil)
	if err != nil {
		return nil, err
	}
	instance, err := public.Instance(ctx)
	if err != nil {
		return nil, err
	}
	if instance.Kind != serverfeatures.Personal || instance.InstanceID == "" {
		return nil, ErrEndpointMissing
	}
	dir, err := localprotocol.RuntimeDirectory()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !strings.HasSuffix(entry.Name(), ".sock") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 || int(stat.Uid) != os.Getuid() {
			continue
		}
		session := socketSession(filepath.Join(dir, entry.Name()))
		probe, cancelProbe := context.WithTimeout(ctx, endpointProbeTimeout)
		var descriptor localprotocol.Descriptor
		err = session.call(probe, http.MethodGet, "/descriptor", nil, &descriptor)
		cancelProbe()
		if err != nil || descriptor.URL != target || descriptor.InstanceID != instance.InstanceID {
			session.transport.CloseIdleConnections()
			continue
		}
		if err := descriptor.Compatible(descriptor.Settings); err != nil {
			session.transport.CloseIdleConnections()
			return nil, err
		}
		options := attachOptions{Capture: capture}
		if dashboard {
			// Share the bound listener; a different binding is not this endpoint.
			options.Listen = descriptor.Listen
		}
		if err := session.attach(parent, ctx, descriptor, options); err != nil {
			session.transport.CloseIdleConnections()
			return nil, err
		}
		if dashboard && session.URL != target {
			_ = session.Close()
			return nil, ErrEndpointMissing
		}
		return session, nil
	}
	return nil, ErrEndpointMissing
}
