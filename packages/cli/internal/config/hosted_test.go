package config

import (
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"path/filepath"
	"testing"
)

func TestHostedSetupAndSecretResolution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Update(t.Context(), path, func(v *Values) error { return v.Set("server-kind", "hosted", false) }); err != nil {
		t.Fatal(err)
	}
	settings, err := Resolve(path, false)
	if err != nil || settings.ServerKind != serverfeatures.Hosted || settings.ValidateDestination() == nil {
		t.Fatal("incomplete runtime setup", err)
	}
	for key, value := range map[string]string{"server-url": "https://hosted.test", "server-token": "stored-secret"} {
		if err := Update(t.Context(), path, func(v *Values) error { return v.Set(key, value, false) }); err != nil {
			t.Fatal(err)
		}
	}
	settings, err = Resolve(path, false)
	if err != nil || settings.ValidateDestination() != nil {
		t.Fatal(err)
	}
	values, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	masked, err := values.Get("server-token", filepath.Dir(path))
	if err != nil || masked != "configured" {
		t.Fatal(masked, err)
	}
	t.Setenv("TOKENINSIGHTS_SERVER_KIND", "personal")
	t.Setenv("TOKENINSIGHTS_ACCESS_TOKEN", "rotated-secret")
	settings, err = Resolve(path, true)
	if err != nil || settings.ServerKind != serverfeatures.Personal || settings.ServerToken != "rotated-secret" {
		t.Fatal("environment precedence", err)
	}
	settings.ServerURL = ""
	if settings.ValidateDestination() == nil {
		t.Fatal("credentials allowed into managed local service")
	}
}
