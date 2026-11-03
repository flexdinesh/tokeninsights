package localbackend

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/serverruntime"
)

// dashboard owns the on-demand public listener. The first dashboard lease
// binds it; equal requests share it; after the last dashboard lease and
// ReconnectGrace it drains. A request racing a drain waits, then rebinds.
// Viewer lifetime is tracked separately: dashboard leases are also viewers.
type dashboard struct {
	ctx     context.Context
	store   serverruntime.Readiness
	handler func(*dashboardBinding) http.Handler
	grace   time.Duration

	mu       sync.Mutex
	current  *dashboardBinding
	draining chan struct{}
	timer    *time.Timer
	closed   bool
}

type dashboardBinding struct {
	listen    localprotocol.Listen
	bindHost  string
	url       string
	leases    int
	listening *serverruntime.Listening
}

func newDashboard(ctx context.Context, store serverruntime.Readiness, handler func(*dashboardBinding) http.Handler) *dashboard {
	return &dashboard{ctx: ctx, store: store, handler: handler, grace: ReconnectGrace}
}

// descriptor returns the current binding without taking a lease.
func (d *dashboard) descriptor() (string, *localprotocol.Listen) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.current == nil {
		return "", nil
	}
	listen := d.current.listen
	return d.current.url, &listen
}

// acquire binds or shares the listener requested by one lease.
func (d *dashboard) acquire(ctx context.Context, listen localprotocol.Listen) (string, func(), error) {
	if err := listen.Validate(); err != nil {
		return "", nil, err
	}
	for {
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			return "", nil, localprotocol.ErrShuttingDown
		}
		if d.draining != nil {
			wait := d.draining
			d.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return "", nil, ctx.Err()
			}
		}
		defer d.mu.Unlock()
		if d.current == nil {
			binding, err := d.bind(listen)
			if err != nil {
				return "", nil, err
			}
			d.current = binding
		} else if d.current.listen != listen {
			return "", nil, localprotocol.ErrSettingsMismatch
		}
		return d.current.url, d.lease(d.current), nil
	}
}

// acquireBound leases the binding serving a public request, never another one.
func (d *dashboard) acquireBound(binding *dashboardBinding) (func(), bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.current != binding {
		return nil, false
	}
	return d.lease(binding), true
}

// bind runs under d.mu. Reserving the address fails fast and leaves an
// occupied port's owner untouched.
func (d *dashboard) bind(listen localprotocol.Listen) (*dashboardBinding, error) {
	listener, err := net.Listen("tcp4", net.JoinHostPort(listen.Host, strconv.Itoa(listen.Port)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", localprotocol.ErrListenUnavailable, err)
	}
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	binding := &dashboardBinding{listen: listen, bindHost: listen.Host, url: "http://" + net.JoinHostPort(host, port)}
	binding.listening, err = serverruntime.Start(d.ctx, d.store, serverruntime.Binding{Listener: listener, Handler: d.handler(binding)})
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	return binding, nil
}

// lease runs under d.mu.
func (d *dashboard) lease(binding *dashboardBinding) func() {
	binding.leases++
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			binding.leases--
			if binding.leases == 0 && d.current == binding && !d.closed {
				d.timer = time.AfterFunc(d.grace, func() { d.expire(binding) })
			}
		})
	}
}

func (d *dashboard) expire(binding *dashboardBinding) {
	d.mu.Lock()
	if d.closed || d.current != binding || binding.leases > 0 || d.draining != nil {
		d.mu.Unlock()
		return
	}
	d.current, d.timer = nil, nil
	done := make(chan struct{})
	d.draining = done
	d.mu.Unlock()
	d.shutdown(binding)
	d.mu.Lock()
	d.draining = nil
	close(done)
	d.mu.Unlock()
}

func (d *dashboard) shutdown(binding *dashboardBinding) {
	ctx, cancel := context.WithTimeout(context.Background(), serverruntime.ShutdownTimeout)
	defer cancel()
	_ = binding.listening.Shutdown(ctx)
}

// close stops admission and joins any bound or draining listener before the
// backend closes storage.
func (d *dashboard) close() {
	d.mu.Lock()
	d.closed = true
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	binding, draining := d.current, d.draining
	d.current = nil
	d.mu.Unlock()
	if binding != nil {
		d.shutdown(binding)
	}
	if draining != nil {
		<-draining
	}
}
