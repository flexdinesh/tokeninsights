package dataengine

import "time"

const staleRetryDelay = 250 * time.Millisecond

// Scheduling hints are expendable: cap both components and scope bindings.
// At capacity, work stays immediately eligible in the durable queue.
const maxDelayedComponents = 128
const maxDelayedScopes = 4096

type staleComponent struct {
	work  Work
	until time.Time
}

type staleComponents []staleComponent

func (s *staleComponents) add(work Work, now time.Time) {
	if len(*s) >= maxDelayedComponents || len(work.Scopes) == 0 {
		return
	}
	scopes := len(work.Scopes)
	for _, entry := range *s {
		scopes += len(entry.work.Scopes)
	}
	if scopes > maxDelayedScopes {
		return
	}
	// Never retain evidence or charge delayed work against active byte admission.
	work.Records = nil
	work.Bytes = 0
	*s = append(*s, staleComponent{work: work, until: now.Add(staleRetryDelay)})
}

func (s *staleComponents) prune(now time.Time) {
	retained := (*s)[:0]
	for _, entry := range *s {
		if now.Before(entry.until) {
			retained = append(retained, entry)
		}
	}
	clear((*s)[len(retained):])
	*s = retained
}

func (s staleComponents) excluding(claims []Work) []Work {
	if len(s) == 0 {
		return claims
	}
	excluded := make([]Work, 0, len(claims)+len(s))
	excluded = append(excluded, claims...)
	for _, entry := range s {
		excluded = append(excluded, entry.work)
	}
	return excluded
}

func (s staleComponents) next() (time.Time, bool) {
	var next time.Time
	for _, entry := range s {
		if next.IsZero() || entry.until.Before(next) {
			next = entry.until
		}
	}
	return next, !next.IsZero()
}
