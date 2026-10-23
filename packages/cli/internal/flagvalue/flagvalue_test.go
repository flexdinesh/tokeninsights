package flagvalue

import (
	"flag"
	"reflect"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

func TestListCollectsRepeatedAndCommaSeparatedValues(t *testing.T) {
	var values List
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.Var(&values, "item", "")
	if err := flags.Parse([]string{"--item", "a, b,,", "--item=c"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(values, List{"a", "b", "c"}) || values.String() != "a,b,c" {
		t.Fatalf("values = %q", values)
	}
}

func TestHarnessesAcceptsSupportedNamesInOrder(t *testing.T) {
	selected, err := Harnesses(List{"pi", "claude-code"})
	if err != nil || !reflect.DeepEqual(selected, []pipeline.Harness{pipeline.HarnessPi, pipeline.HarnessClaudeCode}) {
		t.Fatal(selected, err)
	}
	if _, err := Harnesses(List{"claude"}); err == nil || err.Error() != `invalid --harness "claude": must be opencode, pi, codex, or claude-code` {
		t.Fatal(err)
	}
}

func TestHarnessChoicesNamesEverySupportedHarness(t *testing.T) {
	choices := HarnessChoices()
	for _, harness := range pipeline.SupportedHarnesses {
		if !strings.Contains(choices, string(harness)) {
			t.Fatalf("%q missing %s", choices, harness)
		}
	}
}
