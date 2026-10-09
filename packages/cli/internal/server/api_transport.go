package server

import (
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
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

func apiDashboard(data dashboard) serverapi.UsageResponseV2 {
	response := serverapi.UsageResponseV2{
		Revision:   data.Revision,
		DatasetId:  data.DatasetID,
		DataEpoch:  data.DatabaseID,
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
	if data.Generation > 0 {
		response.FactCount = &data.FactCount
		response.Generation = &data.Generation
		response.InputRevision = &data.InputRevision
		response.Pending = &data.Pending
		response.Unresolved = &data.Unresolved
		quality := serverapi.UsageQuality(data.Quality)
		response.Quality = &quality
	}
	return response
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

func apiLocationOptions(options []querymodel.LocationOption) []serverapi.LocationOption {
	result := make([]serverapi.LocationOption, 0, len(options))
	for _, option := range options {
		result = append(result, serverapi.LocationOption{Key: option.Key, Name: querymodel.LocationDisplayName(option)})
	}
	return result
}

func apiFacets(data analytics.Facets) serverapi.UsageFacetsResponseV2 {
	response := serverapi.UsageFacetsResponseV2{
		Providers: nonNilStrings(data.Providers), Models: nonNilStrings(data.Models),
		Harnesses: apiHarnesses(data.Harnesses), Sessions: nonNilStrings(data.Sessions),
		Repositories: apiLocationOptions(data.Repositories), Directories: apiLocationOptions(data.Directories),
		Revision: data.Revision, DataEpoch: data.DatabaseID, DatasetId: data.DatasetID,
	}
	if data.Generation > 0 {
		response.Generation = &data.Generation
		response.InputRevision = &data.InputRevision
	}
	return response
}
