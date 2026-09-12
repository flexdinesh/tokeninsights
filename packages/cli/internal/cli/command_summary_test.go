package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

func TestRecoverySummaryPreview(t *testing.T) {
	for _, test := range []struct {
		action pipeline.RecoveryAction
		want   string
	}{{pipeline.RecoveryNone, ""}, {pipeline.RecoveryReset, "would reset"}, {pipeline.RecoveryResume, "would resume"}} {
		var output bytes.Buffer
		printSummary(&output, "sync", pipeline.Summary{Recovery: test.action}, true)
		if !strings.Contains(output.String(), "sync dry-run: requested=") {
			t.Fatalf("missing normal summary: %s", output.String())
		}
		if test.want == "" && strings.Contains(output.String(), "Recovery preview") || test.want != "" && !strings.Contains(output.String(), test.want) {
			t.Fatalf("unexpected preview: %s", output.String())
		}
		output.Reset()
		printSummary(&output, "sync", pipeline.Summary{Recovery: test.action}, false)
		if strings.Contains(output.String(), "Recovery preview") {
			t.Fatalf("non-dry summary included preview: %s", output.String())
		}
	}
}
