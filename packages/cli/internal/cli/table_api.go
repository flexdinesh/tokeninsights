package cli

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
)

type tableRequests struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	sequence uint64
}

func (requests *tableRequests) current(sequence uint64) bool {
	if requests == nil || sequence == 0 {
		return true
	}
	requests.mu.Lock()
	defer requests.mu.Unlock()
	return requests.sequence == sequence
}

func (requests *tableRequests) invalidate() {
	if requests == nil {
		return
	}
	requests.mu.Lock()
	defer requests.mu.Unlock()
	if requests.cancel != nil {
		requests.cancel()
	}
	requests.sequence++
}

func (m interactiveModel) queryContext(requests *tableRequests) (context.Context, uint64) {
	if requests == nil {
		return m.ctx, 0
	}
	requests.mu.Lock()
	defer requests.mu.Unlock()
	if requests.cancel != nil {
		requests.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	requests.cancel = cancel
	requests.sequence++
	return ctx, requests.sequence
}

func tableClient(options tableOptions) (*queryclient.Client, error) {
	client, err := queryclient.New(options.serverURL, nil)
	if err != nil {
		return nil, err
	}
	return client.WithToken(options.token), nil
}

func apiPointer[T any](value T) *T { return &value }

func tableUsageParams(options tableOptions, tab tabMode) api.GetUsageParams {
	params := api.GetUsageParams{Period: apiPointer(api.Period(options.period)), Bucket: apiPointer(api.Bucket(options.bucket)), From: apiPointer(options.filters.dayFrom), To: apiPointer(options.filters.dayTo), Provider: apiPointer([]string(options.filters.providers)), Model: apiPointer([]string(options.filters.models)), Session: apiPointer([]string(options.filters.sessionIDs)), Tab: apiPointer(api.UsageTab(tab.String()))}
	harnesses := make([]api.Harness, 0, len(options.filters.harnesses))
	for _, value := range options.filters.harnesses {
		harnesses = append(harnesses, api.Harness(value))
	}
	params.Harness = &harnesses
	if tab == tabRepo {
		group := options.repoGroup
		if group == "" {
			group = db.RepoGroupRepository
		}
		params.LocationGroup = apiPointer(api.LocationGroup(group))
		params.Repository = apiPointer([]string(options.filters.repositories))
		params.Directory = apiPointer([]string(options.filters.directories))
	}
	return params
}

func tableFacetsParams(options tableOptions) api.GetUsageFacetsParams {
	params := tableUsageParams(options, tabRepo)
	return api.GetUsageFacetsParams{Tab: params.Tab, Period: params.Period, Bucket: params.Bucket, From: params.From, To: params.To, Provider: params.Provider, Model: params.Model, Harness: params.Harness, Session: params.Session, Repository: params.Repository, Directory: params.Directory}
}

func apiText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatServerIngestion(value int64, timezone string) string {
	if value <= 0 {
		return "never"
	}
	if zone, err := servingTimezone(timezone); err == nil {
		return time.UnixMilli(value).In(zone).Format("2006-01-02 15:04")
	}
	return formatLastSync(value)
}

func servingTimezone(timezone string) (*time.Location, error) {
	if timezone == "" || timezone == "Local" {
		return nil, errors.New("server timezone is unspecified")
	}
	if zone, err := time.LoadLocation(timezone); err == nil {
		return zone, nil
	}
	_, err := time.Parse("MST -07:00", timezone)
	if err != nil {
		_, err = time.Parse("MST-07:00", timezone)
	}
	if err != nil {
		return nil, err
	}
	// Parsing a named UTC zone can discard a conflicting numeric offset. Parse
	// that suffix independently so UTC+05:30 remains an actual fixed offset.
	const offsetFormat = "-07:00"
	zone, err := time.Parse(offsetFormat, timezone[len(timezone)-len(offsetFormat):])
	if err != nil {
		return nil, err
	}
	_, offset := zone.Zone()
	return time.FixedZone(timezone, offset), nil
}

func (m interactiveModel) loadServerDashboard() reloadMsg {
	client, err := tableClient(m.options)
	if err != nil {
		return reloadMsg{selection: m.selectionKey(), err: err}
	}
	instance, err := client.Instance(m.ctx)
	if err != nil {
		return reloadMsg{selection: m.selectionKey(), err: err}
	}
	result, err := client.AllUsage(m.ctx, tableUsageParams(m.options, m.activeTab))
	if err != nil {
		return reloadMsg{selection: m.selectionKey(), err: err}
	}
	if apiText(instance.InstanceId) != apiText(result.InstanceId) || apiText(instance.DataEpoch) != apiText(result.DataEpoch) {
		return reloadMsg{selection: m.selectionKey(), err: queryclient.ErrSnapshotChanged}
	}
	rows := apiRenderRows(result.Rows, m.activeTab)
	if zone, err := servingTimezone(instance.Timezone); err == nil {
		for index, row := range result.Rows {
			if row.Date > 0 {
				rows[index].latest = time.UnixMilli(row.Date).In(zone).Format("2006-01-02 15:04")
			}
		}
	}
	sortRenderRows(rows, m.activeTab, m.options.sort)
	revision := int64(0)
	if result.Revision != nil {
		revision = *result.Revision
	}
	return reloadMsg{instanceID: apiText(result.InstanceId), dataEpoch: apiText(result.DataEpoch), selection: m.selectionKey(), rows: rows, revision: revision, lastSyncMs: result.LastSynced, sessionCounts: db.SessionCounts{Shown: result.Summary.Sessions, Synced: result.Summary.SyncedSessions}, hostname: instance.Hostname, timezone: instance.Timezone}
}

func apiRenderRows(source []api.UsageRow, tab tabMode) []renderRow {
	result := make([]renderRow, 0, len(source))
	for _, row := range source {
		value := renderRow{bucket: row.Name, latest: formatLatest(row.Date), latestValue: row.Date, sessions: formatTokens(row.Sessions), sessionsValue: row.Sessions, harness: row.Harness, provider: row.Provider, model: row.Model, providers: row.Provider, harnesses: row.Harness, models: row.Model, location: row.LocationName, inputTokens: formatTokens(row.Input), inputValue: row.Input, outputTokens: formatTokens(row.Output), outputValue: row.Output, reasoningTokens: formatTokens(row.Reasoning), reasoningValue: row.Reasoning, cacheReadTokens: formatTokens(row.CacheRead), cacheReadValue: row.CacheRead, cacheWriteTokens: formatTokens(row.CacheWrite), cacheWriteValue: row.CacheWrite, totalTokens: formatTokens(row.Total), totalValue: row.Total, contextUsedTokens: formatContextTokens(row.Context), contextUsedValue: row.Context, averageContextUsedTokens: formatContextTokens(row.AverageContext), averageContextUsedValue: row.AverageContext, medianContextUsedTokens: formatContextTokens(row.MedianContext), medianContextUsedValue: row.MedianContext, maxContextUsedTokens: formatContextTokens(row.MaxContext), maxContextUsedValue: row.MaxContext}
		if tab == tabSessions {
			value.sessionID = row.Name
		}
		result = append(result, value)
	}
	return result
}

func loadRows(ctx context.Context, options tableOptions, _ time.Time, _ groupByMode, tab tabMode) ([]renderRow, error) {
	client, err := tableClient(options)
	if err != nil {
		return nil, err
	}
	response, err := client.AllUsage(ctx, tableUsageParams(options, tab))
	if err != nil {
		return nil, err
	}
	rows := apiRenderRows(response.Rows, tab)
	sortRenderRows(rows, tab, options.sort)
	return rows, nil
}

func loadFilterValues(ctx context.Context, options tableOptions, _ time.Time, dimension filterDimension) ([]string, error) {
	client, err := tableClient(options)
	if err != nil {
		return nil, err
	}
	response, err := client.Facets(ctx, tableFacetsParams(options))
	if err != nil {
		return nil, err
	}
	switch dimension {
	case filterProvider:
		return response.Providers, nil
	case filterModel:
		return response.Models, nil
	case filterHarness:
		values := make([]string, 0, len(response.Harnesses))
		for _, harness := range response.Harnesses {
			values = append(values, string(harness))
		}
		return values, nil
	default:
		return nil, errors.New("unsupported filter")
	}
}

func loadLocationFilterValues(ctx context.Context, options tableOptions, _ time.Time, dimension filterDimension) ([]string, map[string]string, error) {
	client, err := tableClient(options)
	if err != nil {
		return nil, nil, err
	}
	response, err := client.Facets(ctx, tableFacetsParams(options))
	if err != nil {
		return nil, nil, err
	}
	var source []api.LocationOption
	if dimension == filterRepository {
		source = response.Repositories
	} else {
		source = response.Directories
	}
	locations := make([]db.LocationOption, 0, len(source))
	for _, value := range source {
		locations = append(locations, db.LocationOption{Key: value.Key, Name: value.Name})
	}
	values, keys := locationFilterLabels(locations)
	return values, keys, nil
}
