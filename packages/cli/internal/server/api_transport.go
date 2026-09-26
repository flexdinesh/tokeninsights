package server

import (
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	serverapi "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

func apiSelection(selection viewer.Selection) serverapi.Selection {
	return serverapi.Selection{
		Period:    serverapi.Period(selection.Period),
		Bucket:    serverapi.Bucket(selection.Bucket),
		From:      selection.From,
		To:        selection.To,
		Providers: nonNilStrings(selection.Providers),
		Models:    nonNilStrings(selection.Models),
		Harnesses: apiHarnesses(selection.Harnesses),
		Sessions:  nonNilStrings(selection.Sessions),
	}
}

func apiHarnesses(harnesses []string) []serverapi.Harness {
	result := make([]serverapi.Harness, 0, len(harnesses))
	for _, harness := range harnesses {
		result = append(result, serverapi.Harness(harness))
	}
	return result
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func apiSyncState(state syncState) serverapi.SyncResponse {
	var progress *serverapi.SyncProgress
	if p := state.Progress; p != nil {
		progress = &serverapi.SyncProgress{JobId: p.JobID, StartedAt: p.StartedAtMs, UpdatedAt: p.UpdatedAtMs, LastSuccessfulAt: p.LastSuccessfulAtMs, TotalSources: p.TotalSources, CheckedSources: p.CheckedSources, ReadySources: p.ReadySources, FailedSources: p.FailedSources, DiscoveryComplete: p.DiscoveryComplete}
	}
	harnesses := make(map[string]serverapi.HarnessSyncStatus, len(state.Harnesses))
	for harness, status := range state.Harnesses {
		harnesses[harness] = serverapi.HarnessSyncStatus(status)
	}
	return serverapi.SyncResponse{
		Progress:  progress,
		Running:   state.Running,
		Phase:     serverapi.SyncPhase(state.Phase),
		Harnesses: harnesses,
		Error:     state.Error,
		Revision:  int64(state.Revision),
	}
}

func apiDashboard(data dashboard) serverapi.UsageResponse {
	coverage := make([]serverapi.DayCoverage, 0, len(data.Coverage))
	for _, day := range data.Coverage {
		coverage = append(coverage, serverapi.DayCoverage{Day: day.Day, Status: serverapi.DayCoverageStatus(day.Status), CheckedAt: day.CheckedAtMs, PendingSources: day.PendingSources, FailedSources: day.FailedSources, HasUsage: day.HasUsage, Total: day.Total})
	}
	return serverapi.UsageResponse{
		Coverage:   &coverage,
		Rows:       apiUsageRows(data.Rows),
		Chart:      apiUsageRows(data.Chart),
		RowCount:   int64(data.RowCount),
		Page:       data.Page,
		PageSize:   data.PageSize,
		LastSynced: data.LastSynced,
		Range:      data.Range,
		Summary: serverapi.UsageSummary{
			Total:          data.Summary.TotalTokens,
			Input:          data.Summary.InputTokens,
			Output:         data.Summary.OutputTokens,
			Reasoning:      data.Summary.ReasoningTokens,
			CacheRead:      data.Summary.CacheReadTokens,
			CacheWrite:     data.Summary.CacheWriteTokens,
			Sessions:       data.Summary.SessionCount,
			SyncedSessions: data.Summary.SyncedSessions,
		},
	}
}

func apiUsageRows(rows []Row) []serverapi.UsageRow {
	result := make([]serverapi.UsageRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, serverapi.UsageRow{
			Key:                 row.Key,
			Name:                row.Name,
			Harness:             row.Harness,
			Provider:            row.Provider,
			Model:               row.Model,
			Date:                row.Date,
			Sessions:            row.Sessions,
			Input:               row.Input,
			Output:              row.Output,
			Reasoning:           row.Reasoning,
			CacheRead:           row.CacheRead,
			CacheWrite:          row.CacheWrite,
			Total:               row.Total,
			Context:             row.Context,
			AverageContext:      row.AverageContext,
			MedianContext:       row.MedianContext,
			MaxContext:          row.MaxContext,
			LocationKey:         row.LocationKey,
			LocationName:        row.LocationName,
			DirectoryNames:      nonNilStrings(row.DirectoryNames),
			HasUnknownDirectory: row.HasUnknownDirectory,
			RepositoryKey:       row.RepositoryKey,
			RepositoryName:      row.RepositoryName,
		})
	}
	return result
}

func apiLocationOptions(options []db.LocationOption) []serverapi.LocationOption {
	result := make([]serverapi.LocationOption, 0, len(options))
	for _, option := range options {
		result = append(result, serverapi.LocationOption{Key: option.Key, Name: db.LocationDisplayName(option)})
	}
	return result
}
