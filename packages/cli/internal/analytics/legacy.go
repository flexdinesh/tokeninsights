package analytics

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
)

// LoadLegacyFacets serves the historical single-dataset SQLite reader.
func LoadLegacyFacets(ctx context.Context, path string, q Query, search string, now time.Time) (Facets, error) {
	result := Facets{Providers: []string{}, Models: []string{}, Harnesses: []string{}, Sessions: []string{}, Repositories: []db.LocationOption{}, Directories: []db.LocationOption{}, DatasetID: datastore.DatasetID}
	database, err := serverstore.Open(path)
	if err != nil {
		return result, err
	}
	defer func() { _ = database.Close() }()
	tx, err := database.BeginRead(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	f := queryFilter(q, now)
	result.Providers, err = db.AvailableProviders(ctx, tx, f)
	if err != nil {
		return result, err
	}
	result.Models, err = db.AvailableModels(ctx, tx, f)
	if err != nil {
		return result, err
	}
	result.Harnesses, err = db.AvailableHarnesses(ctx, tx, f)
	if err != nil {
		return result, err
	}
	result.Sessions, err = db.AvailableSessions(ctx, tx, f, search, sessionOptionLimit)
	if err != nil {
		return result, err
	}
	if q.Tab == "repo" {
		locations, err := db.AvailableLocations(ctx, tx, f)
		if err != nil {
			return result, err
		}
		result.Repositories, result.Directories = locations.Repositories, locations.Directories
	}
	metadata, err := serverstore.ReadMetadata(ctx, tx)
	if err != nil {
		return result, err
	}
	result.Revision, result.DatabaseID = metadata.Revision, metadata.DatabaseID
	return result, tx.Commit()
}

func LegacyStatus(ctx context.Context, path string) (ProcessingStatus, error) {
	result := ProcessingStatus{Hostname: "unknown"}
	store, err := serverstore.Open(path)
	if err != nil {
		return result, err
	}
	defer func() { _ = store.Close() }()
	tx, err := store.BeginRead(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	metadata, err := serverstore.ReadMetadata(ctx, tx)
	if err != nil {
		return result, err
	}
	result.Metadata = datastore.Metadata{DatabaseID: metadata.DatabaseID, DatasetID: datastore.DatasetID, Kind: datastore.KindPersonal, Revision: metadata.Revision, LastIngestionAtMs: metadata.LastIngestionAtMs}
	var count int
	var hostname string
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(DISTINCT hostname), COALESCE(MIN(hostname),'') FROM ingestion_producers WHERE hostname <> ''").Scan(&count, &hostname); err != nil {
		return result, err
	}
	if count > 1 {
		result.Hostname = "multiple machines"
	} else if count == 1 {
		result.Hostname = hostname
	}
	return result, tx.Commit()
}

func loadRows(ctx context.Context, reader db.Reader, f db.Filter, q Query) ([]Row, error) {
	result := []Row{}
	switch q.Tab {
	case "tokens":
		rows, err := db.ViewerTokenBuckets(ctx, reader, f, db.TimeBucket(q.Selection.Bucket))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			result = append(result, Row{Key: r.Bucket, Name: r.Bucket, Date: r.LatestAtMs, Sessions: r.SessionCount, Input: r.InputTokens, Output: r.OutputTokens, Reasoning: r.ReasoningTokens, CacheRead: r.CacheReadTokens, CacheWrite: r.CacheWriteTokens, Total: r.TotalTokens, Context: r.ContextUsedTokens})
		}
	case "sessions":
		rows, err := db.ViewerSessions(ctx, reader, f)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			result = append(result, Row{Key: r.Harness + ":" + r.SessionID, Name: r.SessionID, Date: r.LatestAtMs, Harness: r.Harness, Provider: r.Providers, Model: r.Models, Sessions: 1, Input: r.InputTokens, Output: r.OutputTokens, Reasoning: r.ReasoningTokens, CacheRead: r.CacheReadTokens, CacheWrite: r.CacheWriteTokens, Total: r.TotalTokens, Context: r.ContextUsedTokens})
		}
	case "context":
		rows, err := db.ViewerContext(ctx, reader, f)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			result = append(result, Row{Key: strings.Join([]string{r.Harness, r.Provider, r.Model}, "\x00"), Name: r.Model, Harness: r.Harness, Provider: r.Provider, Model: r.Model, Date: r.LatestAtMs, Sessions: r.SessionCount, AverageContext: r.AverageContextUsedTokens, MedianContext: r.MedianContextUsedTokens, MaxContext: r.MaxContextUsedTokens})
		}
	case "repo":
		rows, err := db.ViewerRepoGroups(ctx, reader, f, q.LocationGroup)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			name := db.LocationDisplayName(db.LocationOption{Key: r.Key, Name: r.Name})
			row := Row{Key: r.Key, Name: name, Harness: r.Harnesses, Provider: r.Providers, Model: r.Models, Date: r.LatestAtMs, Sessions: r.SessionCount, Input: r.InputTokens, Output: r.OutputTokens, Reasoning: r.ReasoningTokens, CacheRead: r.CacheReadTokens, CacheWrite: r.CacheWriteTokens, Total: r.TotalTokens, Context: r.ContextUsedTokens, LocationKey: r.Key, LocationName: name, DirectoryNames: r.DirectoryNames, HasUnknownDirectory: r.HasUnknownDirectory, RepositoryKey: r.RepositoryKey, RepositoryName: r.RepositoryName}
			result = append(result, row)
		}
	default:
		var rows []db.ViewerDimensionRow
		var err error
		switch q.Tab {
		case "models":
			rows, err = db.ViewerModels(ctx, reader, f)
		case "providers":
			rows, err = db.ViewerProviders(ctx, reader, f)
		case "harnesses":
			rows, err = db.ViewerHarnesses(ctx, reader, f)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			name := r.Model
			if q.Tab == "providers" {
				name = r.Provider
			}
			if q.Tab == "harnesses" {
				name = r.Harness
			}
			result = append(result, Row{Key: name, Name: name, Harness: r.Harnesses, Provider: r.Providers, Model: r.Models, Date: r.LatestAtMs, Sessions: r.SessionCount, Input: r.InputTokens, Output: r.OutputTokens, Reasoning: r.ReasoningTokens, CacheRead: r.CacheReadTokens, CacheWrite: r.CacheWriteTokens, Total: r.TotalTokens, Context: r.ContextUsedTokens})
		}
	}
	return result, nil
}

func numberValue(r Row, key string) int64 {
	switch key {
	case "date":
		return r.Date
	case "sessions":
		return r.Sessions
	case "input":
		return r.Input
	case "output":
		return r.Output
	case "reasoning":
		return r.Reasoning
	case "cacheRead":
		return r.CacheRead
	case "cacheWrite":
		return r.CacheWrite
	case "context":
		return r.Context
	case "averageContext":
		return r.AverageContext
	case "medianContext":
		return r.MedianContext
	case "maxContext":
		return r.MaxContext
	default:
		return r.Total
	}
}

func sortRows(rows []Row, key, direction, tab string) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		comparison := 0
		switch key {
		case "name":
			comparison = strings.Compare(a.Name, b.Name)
		case "harness":
			comparison = strings.Compare(a.Harness, b.Harness)
		case "provider":
			comparison = strings.Compare(a.Provider, b.Provider)
		case "model":
			comparison = strings.Compare(a.Model, b.Model)
		default:
			if key == "date" && tab == "tokens" {
				comparison = strings.Compare(a.Name, b.Name)
			} else {
				x, y := numberValue(a, key), numberValue(b, key)
				if x < y {
					comparison = -1
				}
				if x > y {
					comparison = 1
				}
			}
		}
		if comparison == 0 {
			if a.Total != b.Total {
				return a.Total > b.Total
			}
			return a.Key < b.Key
		}
		if direction == "asc" {
			return comparison < 0
		}
		return comparison > 0
	})
}

func LoadLegacyDashboard(ctx context.Context, path string, q Query, now time.Time) (Dashboard, error) {
	result := Dashboard{Page: q.Page, PageSize: q.PageSize, DatasetID: "default"}
	database, err := serverstore.Open(path)
	if err != nil {
		return result, err
	}
	defer func() { _ = database.Close() }()
	tx, err := database.BeginRead(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	f := q.Selection.Filter(now)
	if q.Tab == "repo" {
		f.RepositoryKeys, f.DirectoryKeys = q.RepositoryKeys, q.DirectoryKeys
	}
	rows, err := loadRows(ctx, tx, f, q)
	if err != nil {
		return result, err
	}
	result.Summary, err = db.ViewerSummary(ctx, tx, f)
	if err != nil {
		return result, err
	}
	metadata, err := serverstore.ReadMetadata(ctx, tx)
	if err != nil {
		return result, err
	}
	result.LastSynced = metadata.LastIngestionAtMs
	result.Revision = metadata.Revision
	result.DatabaseID = metadata.DatabaseID
	result.RowCount = len(rows)
	result.Chart = append([]Row{}, rows...)
	if q.Tab == "sessions" {
		chartQuery := q
		chartQuery.Tab = "tokens"
		result.Chart, err = loadRows(ctx, tx, f, chartQuery)
		if err != nil {
			return result, err
		}
	}
	if q.Tab == "tokens" || q.Tab == "sessions" {
		sortRows(result.Chart, "date", "asc", "tokens")
	} else {
		metric := "total"
		if q.Tab == "context" {
			metric = "averageContext"
		}
		sortRows(result.Chart, metric, "desc", q.Tab)
		if len(result.Chart) > chartLimit {
			result.Chart = result.Chart[:chartLimit]
		}
	}
	sortRows(rows, q.Sort, q.Direction, q.Tab)
	pages := max(1, (len(rows)+q.PageSize-1)/q.PageSize)
	result.Page = min(q.Page, pages)
	start := (result.Page - 1) * q.PageSize
	result.Rows = rows[start:min(start+q.PageSize, len(rows))]
	result.Range = q.Selection.Period
	if result.Range == "all" {
		result.Range = "all time"
	}
	if q.Selection.From != "" || q.Selection.To != "" {
		result.Range = q.Selection.From + ".." + q.Selection.To
	}
	return result, tx.Commit()
}
