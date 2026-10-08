package api

import (
	"fmt"
	"net/url"
)

func setValue[T ~string | ~int](values url.Values, key string, value *T) {
	if value != nil {
		values.Set(key, fmt.Sprint(*value))
	}
}

func setValues[T ~string](values url.Values, key string, list *[]T) {
	if list != nil {
		for _, value := range *list {
			values.Add(key, string(value))
		}
	}
}

func selectionValues(period *PeriodFilter, bucket *BucketFilter, from, to *string, providers, models *[]string, harnesses *[]Harness, sessions, repositories, directories *[]string) url.Values {
	values := url.Values{}
	setValue(values, "period", period)
	setValue(values, "bucket", bucket)
	setValue(values, "from", from)
	setValue(values, "to", to)
	setValues(values, "provider", providers)
	setValues(values, "model", models)
	setValues(values, "harness", harnesses)
	setValues(values, "session", sessions)
	setValues(values, "repository", repositories)
	setValues(values, "directory", directories)
	return values
}

func UsageValues(params GetUsageParams) url.Values {
	values := selectionValues(params.Period, params.Bucket, params.From, params.To, params.Provider, params.Model, params.Harness, params.Session, params.Repository, params.Directory)
	setValue(values, "tab", params.Tab)
	setValue(values, "locationGroup", params.LocationGroup)
	setValue(values, "sort", params.Sort)
	setValue(values, "direction", params.Direction)
	setValue(values, "page", params.Page)
	setValue(values, "pageSize", params.PageSize)
	return values
}

func FacetValues(params GetUsageFacetsParams) url.Values {
	values := selectionValues(params.Period, params.Bucket, params.From, params.To, params.Provider, params.Model, params.Harness, params.Session, params.Repository, params.Directory)
	setValue(values, "tab", params.Tab)
	setValue(values, "search", params.Search)
	return values
}
