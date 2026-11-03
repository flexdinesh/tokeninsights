package application

import "github.com/flexdinesh/tokeninsights/packages/core/analytics"

const (
	defaultPageSize = 50
	MaxPageSize     = 200
)

// NormalizeQuery applies application defaults and validates semantic values.
// Zero-valued optional fields request defaults. Transport decoders reject
// explicitly invalid encodings before calling this operation.
func NormalizeQuery(q analytics.Query) (analytics.Query, error) {
	invalid := func(message string) (analytics.Query, error) { return q, &InvalidRequest{Message: message} }
	if q.Quality == "" {
		q.Quality = "confirmed"
	}
	if q.Quality != "confirmed" && q.Quality != "estimated" {
		return invalid("invalid quality")
	}
	if q.Selection.Period == "" {
		q.Selection.Period = "month"
	}
	if q.Selection.Bucket == "" {
		q.Selection.Bucket = "day"
	}
	if err := q.Selection.Validate(); err != nil {
		return invalid(err.Error())
	}
	if q.Tab == "" {
		q.Tab = "tokens"
	}
	switch q.Tab {
	case "tokens", "models", "providers", "harnesses", "sessions", "context", "repo":
	default:
		return invalid("invalid tab")
	}
	if (q.LocationGroup != "" || len(q.RepositoryKeys) > 0 || len(q.DirectoryKeys) > 0) && q.Tab != "repo" {
		return invalid("location options require repo tab")
	}
	if q.LocationGroup == "" && q.Tab == "repo" {
		q.LocationGroup = analytics.RepoGroupRepository
	}
	if q.LocationGroup != "" && q.LocationGroup != analytics.RepoGroupRepository && q.LocationGroup != analytics.RepoGroupDirectory {
		return invalid("invalid locationGroup")
	}
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
		return invalid("invalid sort for " + q.Tab)
	}
	if q.Direction == "" {
		q.Direction = "desc"
		if q.Sort == "name" || q.Sort == "harness" || q.Sort == "provider" || q.Sort == "model" {
			q.Direction = "asc"
		}
	}
	if q.Direction != "asc" && q.Direction != "desc" {
		return invalid("invalid sort direction")
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = defaultPageSize
	}
	if q.Page < 1 {
		return invalid("invalid page")
	}
	if q.PageSize < 1 {
		return invalid("invalid pageSize")
	}
	if q.PageSize > MaxPageSize {
		return invalid("pageSize must not exceed 200")
	}
	return q, nil
}
