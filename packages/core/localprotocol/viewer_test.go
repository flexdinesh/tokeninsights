package localprotocol

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDefaultCaptureRejectsOverridesDuplicateUnknownAndMalformedJSON(t *testing.T) {
	for _, body := range []string{"{}", `{"fullRefresh":false,"sourceDir":""}`, "  { } \n"} {
		if !DefaultCapture(strings.NewReader(body)) {
			t.Fatal("default capture rejected", body)
		}
	}
	for _, body := range []string{"", "null", "[]", "{} {}", `{"fullRefresh":true}`, `{"sourceDir":"/private"}`, `{"sourceDir":null}`, `{"fullRefresh":null}`, `{"fullRefresh":true,"fullRefresh":false}`, `{"unknown":false}`, `{"sourceDir":""`} {
		if DefaultCapture(strings.NewReader(body)) {
			t.Fatal("nondefault capture accepted", body)
		}
	}
}
func TestViewerReadHasExplicitRoutesAndCanonicalAssets(t *testing.T) {
	for _, path := range []string{"/", "/tokens", "/sessions", "/models", "/providers", "/harnesses", "/repo", "/context", "/index.html", "/assets/app.js", "/fonts/geist-latin-variable.woff2", "/api/v2/usage?period=week", "/api/v2/status", "/api/v2/collector-progress"} {
		if !ViewerRead(httptest.NewRequest(http.MethodGet, path, nil)) {
			t.Fatal("viewer read rejected", path)
		}
	}
	for _, path := range []string{"/arbitrary", "/api/v2/account", "/api/v3/ingestion/batches", "/api/local/v1/descriptor", "/assets/../api/v2/status", "/assets//app.js", "/assets/%2e%2e/api/v2/status"} {
		if ViewerRead(httptest.NewRequest(http.MethodGet, path, nil)) {
			t.Fatal("unsafe read allowed", path)
		}
	}
	if ViewerRead(httptest.NewRequest(http.MethodPost, "/api/v2/usage", nil)) {
		t.Fatal("mutation admitted as read")
	}
}
