// Package queryclient reads canonical analytics from a TokenInsights server.
package queryclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
)

const (
	pageSize       = 200
	snapshotTries  = 3
	maxRows        = 100000
	maxBodyBytes   = 16 << 20
	requestTimeout = 30 * time.Second
)

var ErrSnapshotChanged = errors.New("server analytics changed during pagination; retry")

// StatusError exposes only the HTTP status, never remote response text or URLs.
type StatusError struct{ StatusCode int }

func (e *StatusError) Error() string {
	return fmt.Sprintf("server query returned HTTP %d", e.StatusCode)
}

type Client struct {
	base  url.URL
	http  http.Client
	token string
}

// WithToken returns a client copy authenticated by a bearer token.
func (c *Client) WithToken(token string) *Client {
	copy := *c
	copy.token = token
	return &copy
}

// New accepts an HTTP(S) origin or path prefix without URL credentials or queries.
// A copied client gets a bounded timeout and never follows redirects.
func New(baseURL string, httpClient *http.Client) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u == nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("server URL must be HTTP(S), without credentials, query, or fragment")
	}
	if u.Port() != "" {
		port, portErr := strconv.Atoi(u.Port())
		if portErr != nil || port < 1 || port > 65535 {
			return nil, errors.New("server URL has invalid port")
		}
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	client := http.Client{}
	if httpClient != nil {
		client = *httpClient
	}
	if client.Timeout <= 0 || client.Timeout > requestTimeout {
		client.Timeout = requestTimeout
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: *u, http: client}, nil
}

func (c *Client) Instance(ctx context.Context) (api.InstanceResponse, error) {
	var response api.InstanceResponse
	err := c.get(ctx, "/api/v1/instance", nil, &response)
	if err == nil && response.ApiVersion != api.V1 {
		err = errors.New("unsupported server API version")
	}
	return response, err
}

func (c *Client) Usage(ctx context.Context, params api.GetUsageParams) (api.UsageResponse, error) {
	var response api.UsageResponse
	err := c.get(ctx, "/api/v1/usage", usageValues(params), &response)
	return response, err
}

func (c *Client) Facets(ctx context.Context, params api.GetUsageFacetsParams) (api.UsageFacetsResponse, error) {
	var response api.UsageFacetsResponse
	values := selectionValues(params.Period, params.Bucket, params.From, params.To, params.Provider, params.Model, params.Harness, params.Session, params.Repository, params.Directory)
	setValue(values, "tab", params.Tab)
	setValue(values, "search", params.Search)
	err := c.get(ctx, "/api/v1/usage/facets", values, &response)
	return response, err
}

// Status observes published data readiness and revision without requesting work.
func (c *Client) Status(ctx context.Context) (api.SyncResponse, error) {
	var response api.SyncResponse
	err := c.get(ctx, "/api/v1/sync", nil, &response)
	return response, err
}

// AllUsage returns every row from one publication revision. Summary, chart,
// coverage, and other full-filter metadata retain the first page's values.
// Page is 1 and PageSize remains the requested server page size (200).
func (c *Client) AllUsage(ctx context.Context, params api.GetUsageParams) (api.UsageResponse, error) {
	for attempt := 0; attempt < snapshotTries; attempt++ {
		response, err := c.allUsage(ctx, params)
		if !errors.Is(err, ErrSnapshotChanged) {
			return response, err
		}
		if ctx.Err() != nil {
			return api.UsageResponse{}, ctx.Err()
		}
	}
	return api.UsageResponse{}, ErrSnapshotChanged
}

func (c *Client) allUsage(ctx context.Context, params api.GetUsageParams) (api.UsageResponse, error) {
	instance, err := c.Instance(ctx)
	if err != nil {
		return api.UsageResponse{}, err
	}
	page, size := 1, pageSize
	params.Page, params.PageSize = &page, &size
	result, err := c.Usage(ctx, params)
	if err != nil {
		return api.UsageResponse{}, err
	}
	if !equalIdentity(instance.InstanceId, result.InstanceId) || !equalIdentity(instance.DataEpoch, result.DataEpoch) {
		return api.UsageResponse{}, ErrSnapshotChanged
	}
	if result.Revision == nil || result.RowCount < 0 || result.RowCount > maxRows {
		return api.UsageResponse{}, errors.New("server analytics count or revision is invalid")
	}
	seen := make(map[string]bool)
	if err := validatePage(result, page, size, seen); err != nil {
		return api.UsageResponse{}, err
	}
	for int64(len(result.Rows)) < result.RowCount {
		page++
		next, err := c.Usage(ctx, params)
		if err != nil {
			return api.UsageResponse{}, err
		}
		if !equal(result.Revision, next.Revision) || !equal(result.InstanceId, next.InstanceId) || !equal(result.DataEpoch, next.DataEpoch) || next.RowCount != result.RowCount {
			return api.UsageResponse{}, ErrSnapshotChanged
		}
		if err := validatePage(next, page, size, seen); err != nil {
			return api.UsageResponse{}, err
		}
		result.Rows = append(result.Rows, next.Rows...)
	}
	latest, err := c.Instance(ctx)
	if err != nil {
		return api.UsageResponse{}, err
	}
	if !equal(instance.InstanceId, latest.InstanceId) || !equal(instance.DataEpoch, latest.DataEpoch) {
		return api.UsageResponse{}, ErrSnapshotChanged
	}
	return result, nil
}

func validatePage(response api.UsageResponse, page, size int, seen map[string]bool) error {
	expected := min(int64(size), response.RowCount-int64((page-1)*size))
	if response.Page != page || response.PageSize != size || expected < 0 || int64(len(response.Rows)) != expected {
		return errors.New("server analytics pagination is inconsistent")
	}
	for _, row := range response.Rows {
		if row.Key == "" || seen[row.Key] {
			return errors.New("server analytics row identity is missing or duplicated")
		}
		seen[row.Key] = true
	}
	return nil
}

func equal[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Legacy handlers expose empty instance/epoch values on /instance and omit
// those optional fields on /usage. A nonempty identity must still match.
func equalIdentity(a, b *string) bool {
	if a == nil || *a == "" {
		return b == nil || *b == ""
	}
	return b != nil && *a == *b
}

func (c *Client) get(ctx context.Context, endpoint string, values url.Values, target interface{}) error {
	u := c.base
	u.Path += endpoint
	u.RawQuery = values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return errors.New("cannot create server query")
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("cannot reach server")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return &StatusError{StatusCode: response.StatusCode}
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("server query did not return JSON")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("cannot read server query response")
	}
	if len(body) > maxBodyBytes {
		return errors.New("server query response exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if decoder.Decode(target) != nil {
		return errors.New("server query returned invalid JSON")
	}
	var trailing interface{}
	if decoder.Decode(&trailing) != io.EOF {
		return errors.New("server query returned trailing data")
	}
	return nil
}

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

func selectionValues(period *api.PeriodFilter, bucket *api.BucketFilter, from, to *string, providers, models *[]string, harnesses *[]api.Harness, sessions, repositories, directories *[]string) url.Values {
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

func usageValues(params api.GetUsageParams) url.Values {
	values := selectionValues(params.Period, params.Bucket, params.From, params.To, params.Provider, params.Model, params.Harness, params.Session, params.Repository, params.Directory)
	setValue(values, "tab", params.Tab)
	setValue(values, "locationGroup", params.LocationGroup)
	setValue(values, "sort", params.Sort)
	setValue(values, "direction", params.Direction)
	setValue(values, "page", params.Page)
	setValue(values, "pageSize", params.PageSize)
	return values
}
