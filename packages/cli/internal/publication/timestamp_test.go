package publication

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

const timestampBoundary int64 = 253402214399999

func TestTimestampDomainAcrossNormalizedFields(t *testing.T) {
	if actual := time.UnixMilli(timestampBoundary).UTC().Format(time.RFC3339Nano); actual != "9999-12-30T23:59:59.999Z" {
		t.Fatalf("timestamp boundary date=%s", actual)
	}
	for _, value := range []int64{-1, 0, 1, timestampBoundary - 1, timestampBoundary, timestampBoundary + 1, 1767225600000000, SafeInteger} {
		for _, field := range []string{"snapshot", "session-range", "message", "claude-revision"} {
			t.Run(fmt.Sprintf("%s/%d", field, value), func(t *testing.T) {
				fact := fixtureFact()
				switch field {
				case "snapshot", "claude-revision":
					fact.Session.FirstOccurredAtMs, fact.Session.LastOccurredAtMs = value, value
					fact.OccurredAtMs, fact.Message.OccurredAtMs = value, value
					if field == "claude-revision" {
						fact.Harness, fact.Session.Harness, fact.NativeRequestID = "claude-code", "claude-code", "request"
						fact.Revision = &SourceRevision{Rule: ClaudeRevisionRule, Value: value}
					}
				case "session-range":
					// Keep ordering valid so this exercises the timestamp domain.
					fact.Session.FirstOccurredAtMs = min(value, fact.OccurredAtMs)
					fact.Session.LastOccurredAtMs = max(value, fact.OccurredAtMs)
				case "message":
					fact.Message.OccurredAtMs = value
				}
				SetIDs(&fact)
				wantValid := value >= 0 && value <= timestampBoundary
				if err := ValidateFact(fact); (err == nil) != wantValid {
					t.Fatalf("ValidateFact error=%v wantValid=%v", err, wantValid)
				}
				body, err := json.Marshal(fact)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := DecodeFact(body); (err == nil) != wantValid {
					t.Fatalf("DecodeFact error=%v wantValid=%v", err, wantValid)
				}
				batch := fixtureBatch()
				batch.Entries[0].Fact = fact
				if _, err := EncodeBatch(batch); (err == nil) != wantValid {
					t.Fatalf("EncodeBatch error=%v wantValid=%v", err, wantValid)
				}
				body, err = json.Marshal(batch)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := DecodeBatch(body); (err == nil) != wantValid {
					t.Fatalf("DecodeBatch error=%v wantValid=%v", err, wantValid)
				}
			})
		}
	}
}

func TestTimestampDomainDoesNotNarrowCounters(t *testing.T) {
	fact := fixtureFact()
	fact.InputTokens, fact.OutputTokens, fact.TotalTokens = SafeInteger, 0, SafeInteger
	if err := ValidateFact(fact); err != nil {
		t.Fatalf("safe integer token counter rejected: %v", err)
	}
}
