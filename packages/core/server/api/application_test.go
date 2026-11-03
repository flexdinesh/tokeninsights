package api_test

import (
	"errors"
	"net/url"
	"reflect"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/server/api"
)

func TestHTTPQuerySyntaxAndSemanticValidation(t *testing.T) {
	for _, encoded := range []string{"period=", "bucket=", "tab=", "tab=repo&locationGroup=", "page=0", "pageSize=0", "pageSize=201", "page=1.5", "breakdown=", "worktree=private", "branch=main", "repository=", "locationGroup=repository", "tab=context&sort=total", "quality=combined"} {
		values, err := url.ParseQuery(encoded)
		if err != nil {
			t.Fatal(err)
		}
		_, err = api.ParseQuery(values)
		var invalid *application.InvalidRequest
		if !errors.As(err, &invalid) {
			t.Fatalf("query %q: %v", encoded, err)
		}
	}
	request := analytics.Query{Selection: analytics.Selection{Period: "all", Providers: []string{"a+b", "a&b"}, Harnesses: []string{"pi", "codex"}}, Tab: "repo", RepositoryKeys: []string{"a/b", "c d"}, LocationGroup: analytics.RepoGroupDirectory, Page: 2, PageSize: 1, Quality: "estimated"}
	want, err := application.NormalizeQuery(request)
	if err != nil {
		t.Fatal(err)
	}
	got, err := api.ParseQuery(api.QueryValues(request))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("semantic query changed: %+v, want %+v, %v", got, want, err)
	}
	// A normalized request is still valid when encoded for another transport.
	want, err = application.NormalizeQuery(analytics.Query{})
	if err != nil {
		t.Fatal(err)
	}
	got, err = api.ParseQuery(api.QueryValues(want))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("defaults changed: %+v, want %+v, %v", got, want, err)
	}
}

func TestStatusMappingPreservesPublicationAndResources(t *testing.T) {
	cpu, memory := 12.5, int64(1234)
	value := application.Status{InstanceID: "instance", Readiness: "ready", Metadata: application.Publication{DatabaseID: "database", DatasetID: "dataset", Revision: 3, InputRevision: 4, Generation: 1, TargetGeneration: 2}, Pending: 5, Failed: 2, FailedRetryAtMs: 12345, ProcessUsage: &application.ProcessUsage{CPUPercent: &cpu, ResidentMemoryBytes: &memory}}
	if got := api.StatusValue(api.StatusResponse(value)); !reflect.DeepEqual(got, value) {
		t.Fatalf("status changed: %+v, want %+v", got, value)
	}
}
