// Package capture defines source identities shared by capture and presentations.
package capture

type Harness string

const (
	HarnessOpenCode   Harness = "opencode"
	HarnessPi         Harness = "pi"
	HarnessCodex      Harness = "codex"
	HarnessClaudeCode Harness = "claude-code"
)

var SupportedHarnesses = []Harness{HarnessOpenCode, HarnessPi, HarnessCodex, HarnessClaudeCode}

// ParseHarness accepts exactly one supported harness identifier.
func ParseHarness(value string) (Harness, bool) {
	for _, harness := range SupportedHarnesses {
		if string(harness) == value {
			return harness, true
		}
	}
	return "", false
}
