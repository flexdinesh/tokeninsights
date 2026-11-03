package api

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

// ParseQuery owns HTTP syntax; application owns defaults and semantic validation.
func ParseQuery(v url.Values) (analytics.Query, error) {
	q := analytics.Query{Quality: v.Get("quality"), Selection: analytics.Selection{Period: v.Get("period"), Bucket: v.Get("bucket"), From: v.Get("from"), To: v.Get("to"), Providers: v["provider"], Models: v["model"], Harnesses: v["harness"], Sessions: v["session"]}, Tab: v.Get("tab"), LocationGroup: analytics.RepoGroup(v.Get("locationGroup")), RepositoryKeys: v["repository"], DirectoryKeys: v["directory"], Sort: v.Get("sort"), Direction: v.Get("direction")}
	invalid := func(message string) (analytics.Query, error) { return q, &application.InvalidRequest{Message: message} }
	for _, key := range []string{"period", "bucket", "tab", "locationGroup"} {
		if v.Has(key) && v.Get(key) == "" {
			if key == "period" || key == "bucket" {
				return invalid(fmt.Sprintf("invalid %s %q", key, ""))
			}
			return invalid("invalid " + key)
		}
	}
	if v.Has("breakdown") || v.Has("worktree") || v.Has("branch") {
		return invalid("unsupported location option")
	}
	if (v.Has("locationGroup") || v.Has("repository") || v.Has("directory")) && q.Tab != "repo" {
		return invalid("location options require repo tab")
	}
	for key, target := range map[string]*int{"page": &q.Page, "pageSize": &q.PageSize} {
		if v.Has(key) {
			n, err := strconv.Atoi(v.Get(key))
			if err != nil || n < 1 {
				return invalid("invalid " + key)
			}
			*target = n
		}
	}
	return application.NormalizeQuery(q)
}

// QueryValues serializes semantic requests only at the HTTP client boundary.
func QueryValues(q analytics.Query) url.Values {
	v := url.Values{}
	for key, value := range map[string]string{"quality": q.Quality, "period": q.Selection.Period, "bucket": q.Selection.Bucket, "from": q.Selection.From, "to": q.Selection.To, "tab": q.Tab, "locationGroup": string(q.LocationGroup), "sort": q.Sort, "direction": q.Direction} {
		if value != "" {
			v.Set(key, value)
		}
	}
	for key, values := range map[string][]string{"provider": q.Selection.Providers, "model": q.Selection.Models, "harness": q.Selection.Harnesses, "session": q.Selection.Sessions, "repository": q.RepositoryKeys, "directory": q.DirectoryKeys} {
		for _, value := range values {
			v.Add(key, value)
		}
	}
	if q.Page != 0 {
		v.Set("page", strconv.Itoa(q.Page))
	}
	if q.PageSize != 0 {
		v.Set("pageSize", strconv.Itoa(q.PageSize))
	}
	return v
}

func InstanceResponse(value application.Instance) InstanceResponseV2 {
	capabilities := make([]string, 0, len(value.Capabilities))
	for _, capability := range value.Capabilities {
		capabilities = append(capabilities, string(capability))
	}
	permissions := make([]InstanceResponseV2Permissions, 0, len(value.Permissions))
	for _, permission := range value.Permissions {
		permissions = append(permissions, InstanceResponseV2Permissions(permission))
	}
	return InstanceResponseV2{ApiVersion: V2, InstanceId: value.InstanceID, DataEpoch: value.DatabaseID, DatasetId: value.DatasetID, DataReadiness: InstanceResponseV2DataReadiness(value.Readiness), ServerKind: InstanceResponseV2ServerKind(value.Kind), Capabilities: capabilities, Permissions: permissions, ServerVersion: value.ServerVersion, Hostname: value.Hostname, Timezone: value.Timezone, Defaults: apiSelection(value.Defaults)}
}

func InstanceValue(value InstanceResponseV2) application.Instance {
	capabilities := make(serverfeatures.Capabilities, 0, len(value.Capabilities))
	for _, capability := range value.Capabilities {
		capabilities = append(capabilities, serverfeatures.Capability(capability))
	}
	permissions := make(serverfeatures.Permissions, 0, len(value.Permissions))
	for _, permission := range value.Permissions {
		permissions = append(permissions, serverfeatures.Permission(permission))
	}
	return application.Instance{InstanceID: value.InstanceId, DatabaseID: value.DataEpoch, DatasetID: value.DatasetId, Readiness: string(value.DataReadiness), Kind: serverfeatures.Kind(value.ServerKind), Capabilities: capabilities, Permissions: permissions, ServerVersion: value.ServerVersion, Hostname: value.Hostname, Timezone: value.Timezone, Defaults: analytics.Selection{Period: string(value.Defaults.Period), Bucket: string(value.Defaults.Bucket), From: value.Defaults.From, To: value.Defaults.To, Providers: value.Defaults.Providers, Models: value.Defaults.Models, Harnesses: stringsFromHarnesses(value.Defaults.Harnesses), Sessions: value.Defaults.Sessions}}
}

func UsageResponse(value application.Usage) UsageResponseV2 {
	response := dashboardResponse(value.Dashboard)
	response.InstanceId = value.InstanceID
	return response
}

func count(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func UsageValue(value UsageResponseV2) (application.Usage, error) {
	rowCount := int(value.RowCount)
	if int64(rowCount) != value.RowCount {
		return application.Usage{}, fmt.Errorf("server analytics row count exceeds integer range")
	}
	data := analytics.Dashboard{DatabaseID: value.DataEpoch, DatasetID: value.DatasetId, Revision: value.Revision, FactCount: count(value.FactCount), Generation: count(value.Generation), InputRevision: count(value.InputRevision), Pending: count(value.Pending), Unresolved: count(value.Unresolved), Rows: rowValues(value.Rows), Chart: rowValues(value.Chart), RowCount: rowCount, Page: value.Page, PageSize: value.PageSize, LastSynced: value.LastSynced, Range: value.Range, Summary: analytics.ViewerSummaryRow{TotalTokens: value.Summary.Total, InputTokens: value.Summary.Input, OutputTokens: value.Summary.Output, ReasoningTokens: value.Summary.Reasoning, CacheReadTokens: value.Summary.CacheRead, CacheWriteTokens: value.Summary.CacheWrite, SessionCount: value.Summary.Sessions, SyncedSessions: value.Summary.SyncedSessions}}
	if value.Quality != nil {
		data.Quality = string(*value.Quality)
	}
	return application.Usage{Dashboard: data, InstanceID: value.InstanceId}, nil
}

func rowValues(rows []UsageRow) []analytics.Row {
	result := make([]analytics.Row, 0, len(rows))
	for _, row := range rows {
		result = append(result, analytics.Row{Key: row.Key, Name: row.Name, Harness: row.Harness, Provider: row.Provider, Model: row.Model, Date: row.Date, Sessions: row.Sessions, Input: row.Input, Output: row.Output, Reasoning: row.Reasoning, CacheRead: row.CacheRead, CacheWrite: row.CacheWrite, Total: row.Total, Context: row.Context, AverageContext: row.AverageContext, MedianContext: row.MedianContext, MaxContext: row.MaxContext, LocationKey: row.LocationKey, LocationName: row.LocationName, DirectoryNames: row.DirectoryNames, HasUnknownDirectory: row.HasUnknownDirectory, RepositoryKey: row.RepositoryKey, RepositoryName: row.RepositoryName})
	}
	return result
}

func FacetsResponse(value application.Facets) UsageFacetsResponseV2 {
	response := facetsResponse(value.Facets)
	response.InstanceId = value.InstanceID
	return response
}

func stringsFromHarnesses(values []Harness) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func locationValues(values []LocationOption) []analytics.LocationOption {
	result := make([]analytics.LocationOption, 0, len(values))
	for _, value := range values {
		result = append(result, analytics.LocationOption{Key: value.Key, Name: value.Name})
	}
	return result
}

func FacetsValue(value UsageFacetsResponseV2) application.Facets {
	return application.Facets{InstanceID: value.InstanceId, Facets: analytics.Facets{DatabaseID: value.DataEpoch, DatasetID: value.DatasetId, Revision: value.Revision, Generation: count(value.Generation), InputRevision: count(value.InputRevision), Providers: value.Providers, Models: value.Models, Harnesses: stringsFromHarnesses(value.Harnesses), Sessions: value.Sessions, Repositories: locationValues(value.Repositories), Directories: locationValues(value.Directories)}}
}

func StatusResponse(value application.Status) StatusResponseV2 {
	m := value.Metadata
	result := StatusResponseV2{InstanceId: value.InstanceID, DataEpoch: m.DatabaseID, DatasetId: m.DatasetID, DataReadiness: StatusResponseV2DataReadiness(value.Readiness), Revision: m.Revision, Generation: m.Generation, TargetGeneration: m.TargetGeneration, InputRevision: m.InputRevision, Pending: value.Pending, Failed: &value.Failed, FailedRetryAtMs: &value.FailedRetryAtMs}
	if value.ProcessUsage != nil {
		result.ProcessUsage = &ProcessUsage{CpuPercent: value.ProcessUsage.CPUPercent, ResidentMemoryBytes: value.ProcessUsage.ResidentMemoryBytes}
	}
	return result
}

func StatusValue(value StatusResponseV2) application.Status {
	result := application.Status{InstanceID: value.InstanceId, Readiness: string(value.DataReadiness), Metadata: application.Publication{DatabaseID: value.DataEpoch, DatasetID: value.DatasetId, Revision: value.Revision, Generation: value.Generation, TargetGeneration: value.TargetGeneration, InputRevision: value.InputRevision}, Pending: value.Pending, Failed: count(value.Failed), FailedRetryAtMs: count(value.FailedRetryAtMs)}
	if value.ProcessUsage != nil {
		result.ProcessUsage = &application.ProcessUsage{CPUPercent: value.ProcessUsage.CpuPercent, ResidentMemoryBytes: value.ProcessUsage.ResidentMemoryBytes}
	}
	return result
}
