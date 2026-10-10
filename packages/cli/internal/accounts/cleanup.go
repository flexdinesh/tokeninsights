package accounts

import (
	"context"
	"time"
)

const (
	credentialCleanupInterval = time.Minute
	credentialCleanupTimeout  = 10 * time.Second
)

func (s *Service) RunCleanup(ctx context.Context, report func(error)) {
	run := func() {
		attempt, cancel := context.WithTimeout(ctx, credentialCleanupTimeout)
		defer cancel()
		if err := s.Cleanup(attempt); err != nil && ctx.Err() == nil && report != nil {
			report(err)
		}
	}
	run()
	ticker := time.NewTicker(credentialCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
