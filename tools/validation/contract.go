package main

import (
	"fmt"
	"strings"
)

// Timings exclude host queueing, evidence export and service teardown. A failed
// phase records elapsed time; execution stays zero if preparation did not finish.
type checkTiming struct {
	PreparationMS int64
	ExecutionMS   int64
}

func (timing checkTiming) JSON() string {
	return fmt.Sprintf("{\"PreparationMS\":%d,\"ExecutionMS\":%d}\n", timing.PreparationMS, timing.ExecutionMS)
}

func needsWebAssets(group string) bool {
	return group != "web" && group != "tooling"
}

func dependencyManifestInputs() []string {
	return []string{"package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "packages/*/package.json", "tools/build/package.json", "docs-website/package.json"}
}

func webBuildInputs() []string {
	return append(dependencyManifestInputs(),
		"packages/web/**", "packages/dashboard/**", "packages/contracts/**",
		"tools/build/src/build-web.ts", "tools/build/src/sync-web-assets.ts",
		"tools/build/src/files.ts", "tools/build/src/workspace.ts",
	)
}

func webBuildExcludes() []string {
	// Mock and design modules stay: main.tsx imports them, including for tsc.
	return []string{
		"**/*.test.*", "**/*.spec.*", "**/test-results/**", "**/playwright-report/**",
		"packages/web/e2e/**", "packages/web/playwright.config.ts", "packages/web/scripts/**",
		"packages/web/src/test-setup.ts", "**/node_modules/**", "**/dist/**",
	}
}

func goBuildInputs() []string {
	return append(dependencyManifestInputs(),
		"go.mod", "go.sum", "packages/cli/**", "packages/backend/**", "packages/core/**",
	)
}

func goBuildExcludes() []string {
	// Keep embedded SQL and other production resources alongside Go source.
	return []string{
		"**/*_test.go", "**/testdata/**", "**/fixtures/**", "**/*.md",
		"**/node_modules/**", "**/bin/**", "packages/cli/.golangci.yml",
	}
}

func validateCheck(group, attempt string) error {
	if strings.TrimSpace(attempt) == "" {
		return fmt.Errorf("attempt is required")
	}
	switch group {
	case "static", "web", "tooling", "go", "benchmarks", "distribution", "browser", "deployment":
		return nil
	}
	return fmt.Errorf("unknown validation group %q", group)
}

const captureScript = `mkdir -p /evidence/artifacts
mise run --no-deps --skip-tools "check:internal:$1" > /evidence/check.log 2>&1
code=$?
for path in packages/web/test-results packages/web/playwright-report .check-logs; do
 if [ -d "$path" ]; then cp -R "$path" /evidence/artifacts/ || code=1; fi
done
git diff > /evidence/artifacts/generated.diff 2>> /evidence/check.log || code=1
printf '{"exitCode":%s}\n' "$code" > /evidence/status.json
`
