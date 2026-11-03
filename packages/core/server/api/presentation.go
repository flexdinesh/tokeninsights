package api

import (
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
)

func apiSelection(selection analytics.Selection) Selection {
	return Selection{
		Period:    Period(selection.Period),
		Bucket:    Bucket(selection.Bucket),
		From:      selection.From,
		To:        selection.To,
		Providers: nonNilStrings(selection.Providers),
		Models:    nonNilStrings(selection.Models),
		Harnesses: apiHarnesses(selection.Harnesses),
		Sessions:  nonNilStrings(selection.Sessions),
	}
}

func apiHarnesses(harnesses []string) []Harness {
	result := make([]Harness, 0, len(harnesses))
	for _, harness := range harnesses {
		result = append(result, Harness(harness))
	}
	return result
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func dashboardResponse(data analytics.Dashboard) UsageResponseV2 {
	response := UsageResponseV2{
		Revision:   data.Revision,
		DatasetId:  data.DatasetID,
		DataEpoch:  data.DatabaseID,
		Rows:       UsageRows(data.Rows),
		Chart:      UsageRows(data.Chart),
		RowCount:   int64(data.RowCount),
		Page:       data.Page,
		PageSize:   data.PageSize,
		LastSynced: data.LastSynced,
		Range:      data.Range,
		Summary: UsageSummary{
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
		quality := UsageQuality(data.Quality)
		response.Quality = &quality
	}
	return response
}

func UsageRows(rows []analytics.Row) []UsageRow {
	result := make([]UsageRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, UsageRow{
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

func apiLocationOptions(options []analytics.LocationOption) []LocationOption {
	result := make([]LocationOption, 0, len(options))
	for _, option := range options {
		result = append(result, LocationOption{Key: option.Key, Name: analytics.LocationDisplayName(option)})
	}
	return result
}

func facetsResponse(data analytics.Facets) UsageFacetsResponseV2 {
	response := UsageFacetsResponseV2{
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
