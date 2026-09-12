package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func needsRecovery(compatibility db.Compatibility) bool {
	return compatibility.ResetRequired || compatibility.RebuildPending
}

func recoveryAction(compatibility db.Compatibility) RecoveryAction {
	if compatibility.ResetRequired {
		return RecoveryReset
	}
	if compatibility.RebuildPending {
		return RecoveryResume
	}
	return RecoveryNone
}

func validateRecoveryScope(options SyncOptions, compatibility db.Compatibility) error {
	if !needsRecovery(compatibility) {
		return nil
	}
	if strings.TrimSpace(options.SourceDir) != "" && !selectsAllHarnesses(options.Harnesses) {
		return fmt.Errorf("automatic rebuild needs all harness sources; run `tokeninsights sync --all` or `tokeninsights sync --all --source-dir <all-harness-root>` using the same --db-path: %w", db.ErrRecoveryRequired)
	}
	if compatibility.RebuildPending {
		key, err := recoverySourceKey(options)
		if err != nil {
			return recoveryFailure(compatibility, err)
		}
		if key != compatibility.RebuildSourceKey {
			return fmt.Errorf("rebuild source scope differs; repeat the original --source-dir, source environment and --db-path: %w", db.ErrRebuildPending)
		}
	}
	return nil
}

func recoveryFailure(compatibility db.Compatibility, err error) error {
	if compatibility.ResetRequired {
		return errors.Join(db.ErrRecoveryRequired, err)
	}
	if compatibility.RebuildPending {
		return errors.Join(db.ErrRebuildPending, err)
	}
	return err
}

// The fingerprint represents configured roots, not their changing file sets.
// Normalize paths lexically so creating a previously absent source directory
// cannot change a pending rebuild's scope. Full paths never enter storage.
func recoverySourceKey(options SyncOptions) (string, error) {
	parts := []string{"rebuild-sources-v1"}
	if root := strings.TrimSpace(options.SourceDir); root != "" {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return "", err
		}
		parts = append(parts, "override", absolute)
	} else {
		home := strings.TrimSpace(os.Getenv("HOME"))
		dataHome := configuredRoot("XDG_DATA_HOME", home, ".local", "share")
		codexHome := configuredRoot("CODEX_HOME", home, ".codex")
		claudeHome := configuredRoot("CLAUDE_CONFIG_DIR", home, ".claude")
		parts = append(parts, "defaults")
		for _, root := range []string{
			rootChild(dataHome, "opencode"),
			rootChild(home, ".pi", "agent", "sessions"),
			rootChild(codexHome, "sessions"),
			rootChild(codexHome, "archived_sessions"),
			rootChild(claudeHome, "projects"),
		} {
			if root != "" {
				absolute, err := filepath.Abs(root)
				if err != nil {
					return "", err
				}
				root = absolute
			}
			parts = append(parts, root)
		}
	}
	encoded, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}
	return stableHash(string(encoded)), nil
}

func configuredRoot(environment, home string, suffix ...string) string {
	if root := strings.TrimSpace(os.Getenv(environment)); root != "" {
		return root
	}
	return rootChild(home, suffix...)
}

func rootChild(root string, suffix ...string) string {
	if root == "" {
		return ""
	}
	return filepath.Join(append([]string{root}, suffix...)...)
}

func selectsAllHarnesses(harnesses []Harness) bool {
	selected := make(map[Harness]bool, len(harnesses))
	for _, harness := range harnesses {
		selected[harness] = true
	}
	if len(selected) != len(SupportedHarnesses) {
		return false
	}
	for _, harness := range SupportedHarnesses {
		if !selected[harness] {
			return false
		}
	}
	return true
}

func normalizationRecoveryOptions(options NormalizeOptions) SyncOptions {
	return defaultSyncOptions(SyncOptions{
		DBPath: options.DBPath, Harnesses: SupportedHarnesses, DryRun: options.DryRun,
		Normalize: true, Now: options.Now, Progress: options.Progress,
	})
}

func previewSync(ctx context.Context, options SyncOptions, compatibility db.Compatibility) (Summary, error) {
	if err := validateRecoveryScope(options, compatibility); err != nil {
		return Summary{RequestedHarnesses: len(options.Harnesses)}, err
	}
	if needsRecovery(compatibility) {
		options.Harnesses = SupportedHarnesses
		// Incompatible data and continuity state cannot supply a recovery preview.
		options.FullRefresh = true
	}
	summary, err := dryRunSync(ctx, options)
	summary.Recovery = recoveryAction(compatibility)
	return summary, err
}

// recoverDatabase is called only while holding the database writer lock. The
// pending marker survives all ingest failures so a retry never erases progress.
func recoverDatabase(ctx context.Context, options SyncOptions, compatibility db.Compatibility) (Summary, error) {
	action := recoveryAction(compatibility)
	if err := validateRecoveryScope(options, compatibility); err != nil {
		return Summary{Recovery: action}, err
	}
	if compatibility.ResetRequired {
		sourceKey, err := recoverySourceKey(options)
		if err != nil {
			return Summary{Recovery: action}, recoveryFailure(compatibility, err)
		}
		reportSyncProgress(options, SyncProgressEvent{Status: SyncProgressResetting})
		if err := db.ResetForRecovery(ctx, options.DBPath, sourceKey); err != nil {
			return Summary{Recovery: action}, errors.Join(db.ErrRecoveryRequired, err)
		}
	}
	reportSyncProgress(options, SyncProgressEvent{Status: SyncProgressRebuilding})
	options.Harnesses = SupportedHarnesses
	options.Normalize = true
	summary, err := syncPrepared(ctx, options)
	summary.Recovery = action
	if err != nil {
		return summary, errors.Join(db.ErrRebuildPending, err)
	}
	database, err := db.OpenWritable(options.DBPath)
	if err != nil {
		return summary, errors.Join(db.ErrRebuildPending, err)
	}
	defer func() { _ = database.Close() }()
	if err := db.CompleteRecovery(ctx, database); err != nil {
		return summary, errors.Join(db.ErrRebuildPending, err)
	}
	return summary, nil
}
