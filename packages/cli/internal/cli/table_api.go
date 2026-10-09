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
	if options.local != nil {
		return options.local.Query, nil
	}
	client, err := queryclient.New(options.serverURL, nil)
	if err != nil {
		return nil, err
	}
	client = client.WithToken(options.token)
	if options.datasetID != "" {
		client = client.WithDataset(options.datasetID)
	}
	return client, nil
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
	if instance.InstanceId != result.InstanceId || instance.DataEpoch != result.DataEpoch {
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
	var serverGeneration, inputRevision int64
	if result.Generation != nil {
		serverGeneration = *result.Generation
	}
	if result.InputRevision != nil {
		inputRevision = *result.InputRevision
	}
	return reloadMsg{instanceID: result.InstanceId, dataEpoch: result.DataEpoch, selection: m.selectionKey(), rows: rows, revision: result.Revision, serverGeneration: serverGeneration, inputRevision: inputRevision, lastSyncMs: result.LastSynced, processingPending: result.Pending, sessionCounts: db.SessionCounts{Shown: result.Summary.Sessions, Synced: result.Summary.SyncedSessions}, hostname: instance.Hostname, timezone: instance.Timezone}
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

func loadFacets(ctx context.Context, options tableOptions) (api.UsageFacetsResponse, error) {
	client, err := tableClient(options)
	if err != nil {
		return api.UsageFacetsResponse{}, err
	}
	return client.Facets(ctx, tableFacetsParams(options))
}

func (m interactiveModel) loadFacetValues(options tableOptions, dimension filterDimension) filterValuesMsg {
	response, err := loadFacets(m.ctx, options)
	msg := filterValuesMsg{requestID: m.requestID, generation: m.publicationGeneration, selection: m.selectionKey(), dimension: dimension, instanceID: response.InstanceId, dataEpoch: response.DataEpoch, revision: response.Revision, err: err}
	if err == nil {
		msg.values, msg.keys, msg.err = facetValues(response, dimension)
	}
	return msg
}

func facetValues(response api.UsageFacetsResponse, dimension filterDimension) ([]string, map[string]string, error) {
	switch dimension {
	case filterProvider:
		return response.Providers, nil, nil
	case filterModel:
		return response.Models, nil, nil
	case filterHarness:
		values := make([]string, 0, len(response.Harnesses))
		for _, harness := range response.Harnesses {
			values = append(values, string(harness))
		}
		return values, nil, nil
	case filterRepository, filterDirectory:
		source := response.Repositories
		if dimension == filterDirectory {
			source = response.Directories
		}
		locations := make([]db.LocationOption, 0, len(source))
		for _, value := range source {
			locations = append(locations, db.LocationOption{Key: value.Key, Name: value.Name})
		}
		values, keys := locationFilterLabels(locations)
		return values, keys, nil
	default:
		return nil, nil, errors.New("unsupported filter")
	}
}

func loadLocationFilterValues(ctx context.Context, options tableOptions, _ time.Time, dimension filterDimension) ([]string, map[string]string, error) {
	response, err := loadFacets(ctx, options)
	if err != nil {
		return nil, nil, err
	}
	return facetValues(response, dimension)
}
