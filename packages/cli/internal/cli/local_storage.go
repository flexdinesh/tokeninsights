package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localstorage"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecompat"
)

const storageUnchanged = "TokenInsights did not change any files."

// storageMessage is complete user-facing guidance printed as the command error.
type storageMessage string

func (m storageMessage) Error() string { return string(m) }

// storagePromptInteractive reports whether the user can answer the reset prompt.
var storagePromptInteractive = func(input io.Reader) bool {
	file, ok := input.(*os.File)
	return ok && term.IsTerminal(file.Fd())
}

// prepareLocalStorage runs before an in-process command opens its stores.
func (invocation commandInvocation) prepareLocalStorage(settings config.LocalSettings) error {
	report, err := localstorage.InspectLocal(invocation.context, settings.CollectorDBPath, settings.ServerDBPath, settings.ApplicationPath())
	if err != nil {
		return err
	}
	return invocation.prepareStorage(report)
}

// prepareSyncStorage runs before a distributed sync client opens its stores.
func (invocation commandInvocation) prepareSyncStorage(collectorPath string) error {
	report, err := localstorage.InspectSync(invocation.context, collectorPath)
	if err != nil {
		return err
	}
	return invocation.prepareStorage(report)
}

// prepareStorage resets stores written by an older version after the user
// confirms. Every other incompatibility is reported without changes.
func (invocation commandInvocation) prepareStorage(report localstorage.Report) error {
	if blocked := report.Blocked(); len(blocked) > 0 {
		return storageMessage(blockedStorageMessage(blocked))
	}
	if !report.NeedsReset() {
		return nil
	}
	if !storagePromptInteractive(invocation.stdin) {
		return storageMessage(olderStorageMessage(report))
	}
	_, _ = io.WriteString(invocation.stderr, resetPrompt(report))
	answer, err := bufio.NewReader(invocation.stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "y" && answer != "yes" {
		return storageMessage("Reset declined. " + storageUnchanged)
	}
	moved, err := localstorage.Reset(invocation.context, report, invocation.now)
	for _, file := range moved {
		// SQLite companions follow their database; listing them adds noise.
		if !strings.HasSuffix(file.From, "-wal") && !strings.HasSuffix(file.From, "-shm") {
			_, _ = fmt.Fprintf(invocation.stderr, "Moved %s to %s\n", file.From, file.To)
		}
	}
	if errors.Is(err, localstorage.ErrBusy) {
		return fmt.Errorf("another TokenInsights process is using local storage. Close it, then rerun this command: %w", err)
	}
	if err != nil {
		return fmt.Errorf("reset local storage: %w", err)
	}
	_, _ = io.WriteString(invocation.stderr, "Local storage reset.\n\n")
	return nil
}

func resetPrompt(report localstorage.Report) string {
	var text strings.Builder
	text.WriteString("TokenInsights needs to reset local storage written by an older version.\n\n")
	text.WriteString(storageTable(report.Stores))
	text.WriteString("\nOld files move aside:\n")
	rebuilt, app, jobs := false, false, false
	for _, store := range report.Stores {
		if store.Reset && store.Result.Status != storagecompat.Missing {
			text.WriteString("  " + store.Backup() + "\n")
		}
		rebuilt = rebuilt || store.Reset && store.Role != localstorage.Jobs
		app = app || store.Reset && store.Role == localstorage.Application
		jobs = jobs || store.Reset && store.Role == localstorage.Jobs
	}
	text.WriteString("\n")
	if rebuilt {
		text.WriteString("Usage is rebuilt from saved usage and harness logs.\n")
	}
	if app {
		text.WriteString("Local accounts and credentials are recreated.\n")
	}
	if jobs {
		text.WriteString("Past sync job status is cleared.\n")
	}
	if report.LosesHistory() {
		text.WriteString("Usage whose harness logs were already deleted cannot be rebuilt.\n")
	}
	text.WriteString("\nReset now? [y/N] ")
	return text.String()
}

func olderStorageMessage(report localstorage.Report) string {
	var text strings.Builder
	text.WriteString("TokenInsights needs to reset local storage written by an older version. " + storageUnchanged + "\n\n")
	text.WriteString(storageTable(report.Stores))
	for _, store := range report.Stores {
		if store.Result.Status == storagecompat.Older {
			fmt.Fprintf(&text, "\n  %s", store.Path)
		}
	}
	text.WriteString("\n\nRun this command in a terminal to reset it.")
	return text.String()
}

func blockedStorageMessage(blocked []localstorage.Store) string {
	var text strings.Builder
	text.WriteString("TokenInsights cannot open local storage. " + storageUnchanged + "\n")
	for _, store := range blocked {
		cause, next := "", ""
		switch store.Result.Status {
		case storagecompat.Newer:
			cause = fmt.Sprintf("schema %d; this build supports %d", store.Result.Found, store.Result.Supported)
			next = "Upgrade TokenInsights, or move the file aside to start fresh."
		case storagecompat.Foreign:
			cause = "not a TokenInsights " + roleName(store.Role) + " database"
			next = "Check the configured path."
		default:
			cause = fmt.Sprintf("schema %d does not match this build", store.Result.Found)
			next = "Restore it from a backup, or move the file aside to start fresh."
		}
		fmt.Fprintf(&text, "\n  %s: %s\n  %s\n  %s\n", filepath.Base(store.Path), cause, store.Path, next)
	}
	return strings.TrimSuffix(text.String(), "\n")
}

// storageTable lists existing stores with their schema and planned action.
func storageTable(stores []localstorage.Store) string {
	var rows [][3]string
	width := [2]int{}
	for _, store := range stores {
		if store.Result.Status == storagecompat.Missing {
			continue
		}
		schema := fmt.Sprintf("schema %d", store.Result.Found)
		if store.Result.Status == storagecompat.Older {
			schema = fmt.Sprintf("schema %d -> %d", store.Result.Found, store.Result.Supported)
		}
		action := "keep"
		if store.Reset {
			action = "reset"
		}
		row := [3]string{filepath.Base(store.Path), schema, action}
		width[0], width[1] = max(width[0], len(row[0])), max(width[1], len(row[1]))
		rows = append(rows, row)
	}
	var text strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&text, "  %-*s   %-*s   %s\n", width[0], row[0], width[1], row[1], row[2])
	}
	return text.String()
}

func roleName(role localstorage.Role) string {
	switch role {
	case localstorage.Collector:
		return "collector"
	case localstorage.Server:
		return "token"
	case localstorage.Application:
		return "application"
	default:
		return "sync jobs"
	}
}
