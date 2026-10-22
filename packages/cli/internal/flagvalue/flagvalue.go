// Package flagvalue holds command-line flag values shared by commands.
package flagvalue

import (
	"fmt"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

// List collects repeated or comma-separated values, dropping blank items.
type List []string

func (values *List) String() string { return strings.Join(*values, ",") }

func (values *List) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			*values = append(*values, trimmed)
		}
	}
	return nil
}

// HarnessChoices names every supported harness for help and errors.
func HarnessChoices() string {
	names := make([]string, len(pipeline.SupportedHarnesses))
	for i, harness := range pipeline.SupportedHarnesses {
		names[i] = string(harness)
	}
	last := len(names) - 1
	return strings.Join(names[:last], ", ") + ", or " + names[last]
}

// Harnesses parses --harness values in order.
func Harnesses(values List) ([]pipeline.Harness, error) {
	selected := make([]pipeline.Harness, 0, len(values))
	for _, value := range values {
		harness, ok := pipeline.ParseHarness(value)
		if !ok {
			return nil, fmt.Errorf("invalid --harness %q: must be %s", value, HarnessChoices())
		}
		selected = append(selected, harness)
	}
	return selected, nil
}
