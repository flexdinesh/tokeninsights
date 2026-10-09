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
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

const (
	pageSize       = 200
	snapshotTries  = 3
	maxRows        = 100000
	maxBodyBytes   = 16 << 20
	requestTimeout = 30 * time.Second
	maxRevision    = 9007199254740991
)

var ErrSnapshotChanged = errors.New("server analytics changed during pagination; retry")

var ErrUnavailable = errors.New("server analytics unavailable")

// StatusError exposes only the HTTP status, never remote response text or URLs.
type StatusError struct{ StatusCode int }

func (e *StatusError) Error() string {
	return fmt.Sprintf("server query returned HTTP %d", e.StatusCode)
}

// Reader is the query contract shared by network and in-process compositions.
// Each response describes a consistent publication snapshot.
type Reader interface {
	Instance(context.Context) (api.InstanceResponseV2, error)
	Usage(context.Context, api.GetUsageParams) (api.UsageResponseV2, error)
	AllUsage(context.Context, api.GetUsageParams, int) (api.UsageResponseV2, error)
	Facets(context.Context, api.GetUsageFacetsParams) (api.UsageFacetsResponseV2, error)
	Status(context.Context) (api.StatusResponseV2, error)
}

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

func (c *Client) Instance(ctx context.Context) (api.InstanceResponseV2, error) {
	if c.direct != nil {
		return c.direct.Instance(ctx)
	}
	return c.Descriptor(ctx)
}

func (c *Client) Usage(ctx context.Context, params api.GetUsageParams) (api.UsageResponseV2, error) {
	if c.direct != nil {
		return c.direct.Usage(ctx, params)
	}
	var result api.UsageResponseV2
	err := c.get(ctx, "/api/v2/usage", api.UsageValues(params), &result)
	return result, err
}

func (c *Client) Facets(ctx context.Context, params api.GetUsageFacetsParams) (api.UsageFacetsResponseV2, error) {
	if c.direct != nil {
		return c.direct.Facets(ctx, params)
	}
	var result api.UsageFacetsResponseV2
	err := c.get(ctx, "/api/v2/usage/facets", api.FacetValues(params), &result)
	return result, err
}

// Status observes published data readiness and revision without requesting work.
func (c *Client) Status(ctx context.Context) (api.StatusResponseV2, error) {
	if c.direct != nil {
		return c.direct.Status(ctx)
	}
	var result api.StatusResponseV2
	err := c.get(ctx, "/api/v2/status", nil, &result)
	return result, err
}

// AllUsage returns every row from one publication revision. Summary, chart,
// coverage, and other full-filter metadata retain the first page's values.
// Page is 1 and PageSize remains the requested server page size (200).
func (c *Client) AllUsage(ctx context.Context, params api.GetUsageParams) (api.UsageResponseV2, error) {
	if c.direct != nil {
		page, size := 1, pageSize
		params.Page, params.PageSize = &page, &size
		return c.direct.AllUsage(ctx, params, maxRows)
	}
	for attempt := 0; attempt < snapshotTries; attempt++ {
		response, err := c.allUsage(ctx, params)
		if !errors.Is(err, ErrSnapshotChanged) {
			return response, err
		}
		if ctx.Err() != nil {
			return api.UsageResponseV2{}, ctx.Err()
		}
	}
	return api.UsageResponseV2{}, ErrSnapshotChanged
}

func (c *Client) allUsage(ctx context.Context, params api.GetUsageParams) (api.UsageResponseV2, error) {
	instance, err := c.Instance(ctx)
	if err != nil {
		return api.UsageResponseV2{}, err
	}
	if instance.DataReadiness != api.InstanceResponseV2DataReadinessReady {
		return api.UsageResponseV2{}, ErrUnavailable
	}
	page, size := 1, pageSize
	params.Page, params.PageSize = &page, &size
	result, err := c.Usage(ctx, params)
	if err != nil {
		return api.UsageResponseV2{}, err
	}
	if instance.DatasetId != result.DatasetId || instance.InstanceId != result.InstanceId || instance.DataEpoch != result.DataEpoch {
		return api.UsageResponseV2{}, ErrSnapshotChanged
	}
	if result.RowCount < 0 || result.RowCount > maxRows {
		return api.UsageResponseV2{}, errors.New("server analytics count or revision is invalid")
	}
	seen := make(map[string]bool)
	if err := validatePage(result, page, size, seen); err != nil {
		return api.UsageResponseV2{}, err
	}
	for int64(len(result.Rows)) < result.RowCount {
		page++
		next, err := c.Usage(ctx, params)
		if err != nil {
			return api.UsageResponseV2{}, err
		}
		if result.DatasetId != next.DatasetId || result.Revision != next.Revision || result.InstanceId != next.InstanceId || result.DataEpoch != next.DataEpoch || next.RowCount != result.RowCount || !sameGeneration(result.Generation, next.Generation) {
			return api.UsageResponseV2{}, ErrSnapshotChanged
		}
		if err := validatePage(next, page, size, seen); err != nil {
			return api.UsageResponseV2{}, err
		}
		result.Rows = append(result.Rows, next.Rows...)
	}
	latest, err := c.Instance(ctx)
	if err != nil {
		return api.UsageResponseV2{}, err
	}
	if instance.DatasetId != latest.DatasetId || instance.InstanceId != latest.InstanceId || instance.DataEpoch != latest.DataEpoch || latest.DataReadiness != api.InstanceResponseV2DataReadinessReady {
		return api.UsageResponseV2{}, ErrSnapshotChanged
	}
	return result, nil
}

func sameGeneration(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func validatePage(response api.UsageResponseV2, page, size int, seen map[string]bool) error {
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

// Required generated values decode omitted fields as zero values. Decode the
// publication envelope separately to distinguish a missing revision from zero.
func validateEnvelope(body []byte, metadata, revisionRequired bool) error {
	var envelope struct {
		InstanceID *string `json:"instanceId"`
		DataEpoch  *string `json:"dataEpoch"`
		Readiness  *string `json:"dataReadiness"`
		Revision   *int64  `json:"revision"`
	}
	invalid := errors.New("server query publication envelope is invalid")
	if json.Unmarshal(body, &envelope) != nil || envelope.InstanceID == nil || *envelope.InstanceID == "" || envelope.DataEpoch == nil {
		return invalid
	}
	if revisionRequired && (envelope.Revision == nil || *envelope.Revision < 0 || *envelope.Revision > maxRevision) {
		return invalid
	}
	if !metadata {
		if *envelope.DataEpoch == "" {
			return invalid
		}
		return nil
	}
	if envelope.Readiness == nil {
		return invalid
	}
	switch *envelope.Readiness {
	case "ready":
		if *envelope.DataEpoch == "" {
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

func (c *Client) Descriptor(ctx context.Context) (api.InstanceResponseV2, error) {
	var result api.InstanceResponseV2
	err := c.get(ctx, "/api/v2/instance", nil, &result)
	var status *StatusError
	if errors.As(err, &status) && status.StatusCode == http.StatusNotFound {
		return result, errors.New("server upgrade required: read API v2 unavailable")
	}
	if err != nil {
		return result, err
	}
	if result.ApiVersion != "v2" {
		return result, errors.New("unsupported server API version")
	}
	if err := serverfeatures.Kind(result.ServerKind).Validate(); err != nil {
		return result, errors.New("unsupported server kind")
	}
	if err := (serverfeatures.Policy{Kind: serverfeatures.Kind(result.ServerKind), Capabilities: DescriptorCapabilities(result)}).Validate(); err != nil {
		return result, errors.New("invalid server capability policy")
	}
	return result, nil
}

func DescriptorCapabilities(descriptor api.InstanceResponseV2) serverfeatures.Capabilities {
	result := make(serverfeatures.Capabilities, 0, len(descriptor.Capabilities))
	for _, value := range descriptor.Capabilities {
		result = append(result, serverfeatures.Capability(value))
	}
	return result.Known()
}

func HasPermission(descriptor api.InstanceResponseV2, permission serverfeatures.Permission) bool {
	for _, value := range descriptor.Permissions {
		if string(value) == string(permission) {
			return true
		}
	}
	return false
}
