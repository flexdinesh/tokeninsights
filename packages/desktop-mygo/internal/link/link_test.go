package link

import "testing"

func TestDashboardNavigationOrigins(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:8765/models", "https://example.test/", "http://[::1]:8765/"} {
		target, err := Dashboard(raw)
		if err != nil || !SameOrigin(raw, target.Origin) {
			t.Fatal(raw, target, err)
		}
	}
	for _, raw := range []string{"javascript:alert(1)", "file:///etc/passwd", "http://user:secret@localhost/", "/relative"} {
		if _, err := Dashboard(raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	if SameOrigin("http://127.0.0.1:9999/", "http://127.0.0.1:8765") {
		t.Fatal("different bridge origins matched")
	}
}
