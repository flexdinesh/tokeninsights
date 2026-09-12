package cli

import (
	"fmt"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
)

var versionCommand = commandSpec{name: "version", aliases: []string{"--version"}, run: runVersion}

func runVersion(invocation commandInvocation, _ []string) error {
	_, err := fmt.Fprintln(invocation.stdout, version.String())
	return err
}
