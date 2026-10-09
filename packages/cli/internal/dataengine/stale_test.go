package dataengine

import (
	"fmt"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

func TestStaleTrackingBoundsAndReleasesEvidence(t *testing.T) {
	now := time.Now()
	var delayed staleComponents
	work := Work{DatasetID: "user", Root: "root", Bytes: 64 << 20, Scopes: map[string]int64{"root": 1}, Records: []evidence.Stored{{ID: "evidence"}}}
	for index := range maxDelayedComponents + 1 {
		work.DatasetID = fmt.Sprint(index)
		delayed.add(work, now)
	}
	if len(delayed) != maxDelayedComponents {
		t.Fatal("component tracking not bounded", len(delayed))
	}
	for _, excluded := range delayed.excluding(nil) {
		if excluded.Records != nil || excluded.Bytes != 0 || len(excluded.Scopes) != 1 {
			t.Fatal("delay retained evidence or lost component binding")
		}
	}
	delayed.prune(now.Add(staleRetryDelay))
	if len(delayed) != 0 {
		t.Fatal("expired hints retained")
	}
	for _, entry := range delayed[:cap(delayed)] {
		if entry.work.Scopes != nil {
			t.Fatal("expired backing array retained scope bindings")
		}
	}
	work.Scopes = make(map[string]int64)
	for index := range maxDelayedScopes + 1 {
		work.Scopes[fmt.Sprint(index)] = 1
	}
	delayed.add(work, now)
	if len(delayed) != 0 {
		t.Fatal("oversized component consumed unbounded scheduling state")
	}
	delete(work.Scopes, fmt.Sprint(maxDelayedScopes))
	delayed.add(work, now)
	work.Scopes = map[string]int64{"another": 1}
	delayed.add(work, now)
	if len(delayed) != 1 {
		t.Fatal("combined scope tracking exceeded budget", len(delayed))
	}
}
