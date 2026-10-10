package analytics

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

const defaultPageSize = 50
const maxPageSize = 200
const ChartLimit = 12
const SessionOptionLimit = 100

type Query struct {
	Quality        string
	Selection      viewer.Selection
	Tab            string
	LocationGroup  querymodel.RepoGroup
	RepositoryKeys []string
	DirectoryKeys  []string
	Sort           string
	Direction      string
	Page           int
	PageSize       int
}

func ParseQuery(values url.Values) (Query, error) {
	q := Query{Selection: viewer.Selection{Period: "month", Bucket: "day"}, Tab: "tokens", Page: 1, PageSize: defaultPageSize}
	q.Quality = values.Get("quality")
	if q.Quality == "" {
		q.Quality = "confirmed"
	}
	if q.Quality != "confirmed" && q.Quality != "estimated" {
		return q, fmt.Errorf("invalid quality")
	}
	if values.Has("period") {
		q.Selection.Period = values.Get("period")
	}
	if values.Has("bucket") {
		q.Selection.Bucket = values.Get("bucket")
	}
	q.Selection.From, q.Selection.To = values.Get("from"), values.Get("to")
	q.Selection.Providers, q.Selection.Models = values["provider"], values["model"]
	q.Selection.Harnesses, q.Selection.Sessions = values["harness"], values["session"]
	if err := q.Selection.Validate(); err != nil {
		return q, err
	}
	if values.Has("tab") {
		q.Tab = values.Get("tab")
	}
	switch q.Tab {
	case "tokens", "models", "providers", "harnesses", "sessions", "context", "repo":
	default:
		return q, fmt.Errorf("invalid tab")
	}
	q.LocationGroup = querymodel.RepoGroupRepository
	if values.Has("locationGroup") {
		q.LocationGroup = querymodel.RepoGroup(values.Get("locationGroup"))
	}
	if values.Has("breakdown") || values.Has("worktree") || values.Has("branch") {
		return q, fmt.Errorf("unsupported location option")
	}
	if values.Has("locationGroup") || values.Has("repository") || values.Has("directory") {
		if q.Tab != "repo" {
			return q, fmt.Errorf("location options require repo tab")
		}
	}
	switch q.LocationGroup {
	case querymodel.RepoGroupRepository, querymodel.RepoGroupDirectory:
	default:
		return q, fmt.Errorf("invalid locationGroup")
	}
	if q.Tab == "repo" {
		q.RepositoryKeys, q.DirectoryKeys = values["repository"], values["directory"]
	}
	q.Sort = values.Get("sort")
	if q.Sort == "" {
		q.Sort = "total"
		if q.Tab == "tokens" || q.Tab == "sessions" {
			q.Sort = "date"
		}
		if q.Tab == "context" {
			q.Sort = "averageContext"
		}
	}
	allowed := map[string]bool{"name": true, "date": true, "total": true, "input": true, "output": true, "reasoning": true, "cacheRead": true, "cacheWrite": true, "sessions": true, "context": true}
	if q.Tab == "repo" {
		allowed["harness"], allowed["provider"], allowed["model"] = true, true, true
	}
	if q.Tab == "context" {
		allowed = map[string]bool{"averageContext": true, "medianContext": true, "maxContext": true, "sessions": true, "harness": true, "provider": true, "model": true}
	}
	if !allowed[q.Sort] {
		return q, fmt.Errorf("invalid sort for %s", q.Tab)
	}
	q.Direction = values.Get("direction")
	if q.Direction == "" {
		q.Direction = "desc"
		if q.Sort == "name" || q.Sort == "harness" || q.Sort == "provider" || q.Sort == "model" {
			q.Direction = "asc"
		}
	}
	if q.Direction != "asc" && q.Direction != "desc" {
		return q, fmt.Errorf("invalid sort direction")
	}
	for key, target := range map[string]*int{"page": &q.Page, "pageSize": &q.PageSize} {
		if values.Has(key) {
			n, err := strconv.Atoi(values.Get(key))
			if err != nil || n < 1 {
				return q, fmt.Errorf("invalid %s", key)
			}
			*target = n
		}
	}
	if q.PageSize > maxPageSize {
		return q, fmt.Errorf("pageSize must not exceed %d", maxPageSize)
	}
	return q, nil
}

// Row is a numeric analytics result, independent of API transport.
type Row struct {
	Key                 string
	Name                string
	Harness             string
	Provider            string
	Model               string
	Date                int64
	Sessions            int64
	Input               int64
	Output              int64
	Reasoning           int64
	CacheRead           int64
	CacheWrite          int64
	Total               int64
	Context             int64
	AverageContext      int64
	MedianContext       int64
	MaxContext          int64
	LocationKey         string
	LocationName        string
	DirectoryNames      []string
	HasUnknownDirectory bool
	RepositoryKey       string
	RepositoryName      string
}

type Dashboard struct {
	FactCount     int64
	Generation    int64
	InputRevision int64
	Pending       int64
	Unresolved    int64
	Quality       string
	Revision      int64
	DatabaseID    string
	DatasetID     string
	Rows          []Row
	Chart         []Row
	RowCount      int
	Page          int
	PageSize      int
	Summary       querymodel.ViewerSummaryRow
	LastSynced    int64
	Range         string
}
