package localprotocol

import (
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strings"
)

const MaxDefaultCaptureBytes = 1024

// ViewerRead permits only dashboard assets and the personal read API.
func ViewerRead(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	p := r.URL.Path
	if path.Clean(p) != p || strings.Contains(p, "\\") || r.URL.RawPath != "" {
		return false
	}
	switch p {
	case "/tokens", "/sessions", "/models", "/providers", "/harnesses", "/repo", "/context":
		return true
	case "/", "/index.html", "/favicon.ico", "/favicon.svg", "/api/v2/instance", "/api/v2/status", "/api/v2/usage", "/api/v2/usage/facets", "/api/v2/collector-progress":
		return true
	}
	return strings.HasPrefix(p, "/assets/") && len(p) > len("/assets/") || strings.HasPrefix(p, "/fonts/") && len(p) > len("/fonts/")
}

// DefaultCapture accepts only one JSON object requesting default capture.
// Recognized fields must hold defaults. Duplicate and unknown fields reject.
func DefaultCapture(body io.Reader) bool {
	decoder := json.NewDecoder(io.LimitReader(body, MaxDefaultCaptureBytes+1))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return false
		}
		seen[key] = true
		switch key {
		case "fullRefresh":
			value, err := decoder.Token()
			if err != nil || value != false {
				return false
			}
		case "sourceDir":
			value, err := decoder.Token()
			if err != nil || value != "" {
				return false
			}
		default:
			return false
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return false
	}
	return decoder.Decode(&struct{}{}) == io.EOF
}
