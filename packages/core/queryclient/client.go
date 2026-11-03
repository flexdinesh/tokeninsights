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

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	api "github.com/flexdinesh/tokeninsights/packages/core/server/api"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

const (
	pageSize       = application.MaxPageSize
	snapshotTries  = 3
	maxRows        = 100000
	maxBodyBytes   = 16 << 20
	requestTimeout = 30 * time.Second
	maxRevision    = 9007199254740991
)

var ErrSnapshotChanged = application.ErrSnapshotChanged

var ErrUnavailable = application.ErrUnavailable

// StatusError exposes only the HTTP status, never remote response text or URLs.
type StatusError struct{ StatusCode int }

func (e *StatusError) Error() string {
	return fmt.Sprintf("server query returned HTTP %d", e.StatusCode)
}

// Reader is the transport-independent application query contract.
type Reader = application.Reader

func NewDirect(reader Reader) *Client { return &Client{direct: reader} }

type Client struct {
	direct  Reader
	base    url.URL
	http    http.Client
	token   string
	dataset string
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

func (c *Client) Instance(ctx context.Context) (application.Instance, error) {
	if c.direct != nil {
		result, err := c.direct.Instance(ctx)
		if err == nil {
			err = c.checkDataset(result.DatasetID)
		}
		if err != nil {
			return application.Instance{}, err
		}
		return result, nil
	}
	return c.Descriptor(ctx)
}

func (c *Client) Usage(ctx context.Context, params analytics.Query) (application.Usage, error) {
	if c.direct != nil {
		result, err := c.direct.Usage(ctx, params)
		if err == nil {
			err = c.checkDataset(result.DatasetID)
		}
		if err != nil {
			return application.Usage{}, err
		}
		return result, nil
	}
	var result api.UsageResponseV2
	if err := c.get(ctx, "/api/v2/usage", api.QueryValues(params), &result); err != nil {
		return application.Usage{}, err
	}
	return api.UsageValue(result)
}

func (c *Client) Facets(ctx context.Context, params application.FacetRequest) (application.Facets, error) {
	if c.direct != nil {
		result, err := c.direct.Facets(ctx, params)
		if err == nil {
			err = c.checkDataset(result.DatasetID)
		}
		if err != nil {
			return application.Facets{}, err
		}
		return result, nil
	}
	var result api.UsageFacetsResponseV2
	values := api.QueryValues(params.Query)
	if params.Search != "" {
		values.Set("search", params.Search)
	}
	if err := c.get(ctx, "/api/v2/usage/facets", values, &result); err != nil {
		return application.Facets{}, err
	}
	return api.FacetsValue(result), nil
}

// Status observes published data readiness and revision without requesting work.
func (c *Client) Status(ctx context.Context) (application.Status, error) {
	if c.direct != nil {
		result, err := c.direct.Status(ctx)
		if err == nil {
			err = c.checkDataset(result.Metadata.DatasetID)
		}
		if err != nil {
			return application.Status{}, err
		}
		return result, nil
	}
	var result api.StatusResponseV2
	if err := c.get(ctx, "/api/v2/status", nil, &result); err != nil {
		return application.Status{}, err
	}
	return api.StatusValue(result), nil
}

// AllUsage returns every row from one publication revision. Summary, chart,
// coverage, and other full-filter metadata retain the first page's values.
// Page is 1 and PageSize remains the requested server page size (200).
func (c *Client) AllUsage(ctx context.Context, params analytics.Query) (application.Usage, error) {
	if c.direct != nil {
		page, size := 1, pageSize
		params.Page, params.PageSize = page, size
		result, err := c.direct.AllUsage(ctx, params, maxRows)
		if err == nil {
			err = c.checkDataset(result.DatasetID)
		}
		if err != nil {
			return application.Usage{}, err
		}
		return result, nil
	}
	for attempt := 0; attempt < snapshotTries; attempt++ {
		response, err := c.allUsage(ctx, params)
		if !errors.Is(err, ErrSnapshotChanged) {
			return response, err
		}
		if ctx.Err() != nil {
			return application.Usage{}, ctx.Err()
		}
	}
	return application.Usage{}, ErrSnapshotChanged
}

func (c *Client) allUsage(ctx context.Context, params analytics.Query) (application.Usage, error) {
	instance, err := c.Instance(ctx)
	if err != nil {
		return application.Usage{}, err
	}
	if instance.Readiness != "ready" {
		return application.Usage{}, ErrUnavailable
	}
	page, size := 1, pageSize
	params.Page, params.PageSize = page, size
	result, err := c.Usage(ctx, params)
	if err != nil {
		return application.Usage{}, err
	}
	if instance.DatasetID != result.DatasetID || instance.InstanceID != result.InstanceID || instance.DatabaseID != result.DatabaseID {
		return application.Usage{}, ErrSnapshotChanged
	}
	if result.RowCount < 0 || result.RowCount > maxRows {
		return application.Usage{}, errors.New("server analytics count or revision is invalid")
	}
	seen := make(map[string]bool)
	if err := validatePage(result, page, size, seen); err != nil {
		return application.Usage{}, err
	}
	for len(result.Rows) < result.RowCount {
		page++
		params.Page = page
		next, err := c.Usage(ctx, params)
		if err != nil {
			return application.Usage{}, err
		}
		if result.DatasetID != next.DatasetID || result.Revision != next.Revision || result.InstanceID != next.InstanceID || result.DatabaseID != next.DatabaseID || next.RowCount != result.RowCount || result.Generation != next.Generation {
			return application.Usage{}, ErrSnapshotChanged
		}
		if err := validatePage(next, page, size, seen); err != nil {
			return application.Usage{}, err
		}
		result.Rows = append(result.Rows, next.Rows...)
	}
	latest, err := c.Instance(ctx)
	if err != nil {
		return application.Usage{}, err
	}
	if instance.DatasetID != latest.DatasetID || instance.InstanceID != latest.InstanceID || instance.DatabaseID != latest.DatabaseID || latest.Readiness != "ready" {
		return application.Usage{}, ErrSnapshotChanged
	}
	return result, nil
}

func validatePage(response application.Usage, page, size int, seen map[string]bool) error {
	expected := min(size, response.RowCount-(page-1)*size)
	if response.Page != page || response.PageSize != size || expected < 0 || len(response.Rows) != expected {
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

// Required generated values decode omitted fields as zero values. Decode the
// publication envelope separately to distinguish a missing revision from zero.
func validateEnvelope(body []byte, metadata, revisionRequired bool) error {
	var envelope struct {
		InstanceID *string `json:"instanceId"`
		DatabaseID *string `json:"dataEpoch"`
		Readiness  *string `json:"dataReadiness"`
		Revision   *int64  `json:"revision"`
	}
	invalid := errors.New("server query publication envelope is invalid")
	if json.Unmarshal(body, &envelope) != nil || envelope.InstanceID == nil || *envelope.InstanceID == "" || envelope.DatabaseID == nil {
		return invalid
	}
	if revisionRequired && (envelope.Revision == nil || *envelope.Revision < 0 || *envelope.Revision > maxRevision) {
		return invalid
	}
	if !metadata {
		if *envelope.DatabaseID == "" {
			return invalid
		}
		return nil
	}
	if envelope.Readiness == nil {
		return invalid
	}
	switch *envelope.Readiness {
	case "ready":
		if *envelope.DatabaseID == "" {
			return invalid
		}
	case "metadata", "recovery", "rebuild", "unavailable":
	default:
		return invalid
	}
	return nil
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
	metadata := strings.HasSuffix(endpoint, "/instance") || strings.HasSuffix(endpoint, "/sync") || strings.HasSuffix(endpoint, "/status")
	if err := validateEnvelope(body, metadata, !strings.HasSuffix(endpoint, "/instance")); err != nil {
		return err
	}
	if strings.HasPrefix(endpoint, "/api/v2/") {
		var envelope struct {
			DatasetID *string `json:"datasetId"`
		}
		if json.Unmarshal(body, &envelope) != nil || envelope.DatasetID == nil || *envelope.DatasetID == "" {
			return errors.New("server query dataset envelope is invalid")
		}
		if c.dataset != "" && c.dataset != *envelope.DatasetID {
			return ErrSnapshotChanged
		}
	}
	return nil
}

// WithDataset pins reads to one authenticated dataset for every read.
func (c *Client) WithDataset(dataset string) *Client {
	copy := *c
	copy.dataset = dataset
	return &copy
}

func (c *Client) Descriptor(ctx context.Context) (application.Instance, error) {
	if c.direct != nil {
		return c.Instance(ctx)
	}
	var wire api.InstanceResponseV2
	err := c.get(ctx, "/api/v2/instance", nil, &wire)
	var status *StatusError
	if errors.As(err, &status) && status.StatusCode == http.StatusNotFound {
		return application.Instance{}, errors.New("server upgrade required: read API v2 unavailable")
	}
	if err != nil {
		return application.Instance{}, err
	}
	if wire.ApiVersion != "v2" {
		return application.Instance{}, errors.New("unsupported server API version")
	}
	result := api.InstanceValue(wire)
	if err := result.Kind.Validate(); err != nil {
		return application.Instance{}, errors.New("unsupported server kind")
	}
	if err := (serverfeatures.Policy{Kind: result.Kind, Capabilities: DescriptorCapabilities(result)}).Validate(); err != nil {
		return application.Instance{}, errors.New("invalid server capability policy")
	}
	return result, nil
}

func DescriptorCapabilities(descriptor application.Instance) serverfeatures.Capabilities {
	return descriptor.Capabilities.Known()
}
func HasPermission(descriptor application.Instance, permission serverfeatures.Permission) bool {
	return descriptor.Permissions.Has(permission)
}

func (c *Client) checkDataset(dataset string) error {
	if c.dataset != "" && c.dataset != dataset {
		return ErrSnapshotChanged
	}
	return nil
}

// Unwrap retains HTTP diagnostics while exposing transport-independent outcomes.
func (e *StatusError) Unwrap() error {
	switch e.StatusCode {
	case http.StatusBadRequest:
		return &application.InvalidRequest{Message: "invalid query request"}
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return application.ErrForbidden
	case http.StatusServiceUnavailable:
		return application.ErrUnavailable
	}
	return nil
}
