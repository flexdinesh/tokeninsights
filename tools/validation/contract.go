package main

import (
	"fmt"
	"strings"
)

func validateCheck(group, attempt string) error {
	if strings.TrimSpace(attempt) == "" {
		return fmt.Errorf("attempt is required")
	}
	switch group {
	case "contracts", "javascript", "go", "benchmarks", "distribution", "browser", "deployment":
		return nil
	}
	return fmt.Errorf("unknown validation group %q", group)
}

const captureScript = `mkdir -p /evidence/artifacts
mise run --no-deps --skip-tools "validation:internal:$1" > /evidence/check.log 2>&1
code=$?
for path in packages/web/test-results packages/web/playwright-report .check-logs; do
 if [ -d "$path" ]; then cp -R "$path" /evidence/artifacts/ || code=1; fi
done
git diff > /evidence/artifacts/generated.diff 2>> /evidence/check.log || code=1
printf '{"exitCode":%s}\n' "$code" > /evidence/status.json
`
