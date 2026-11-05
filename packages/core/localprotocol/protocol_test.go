package localprotocol

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalStorageIdentityAndShortSocket(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	settings := Settings{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite")}
	canonical, err := settings.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	aliased := settings
	aliased.CollectorDBPath, aliased.ServerDBPath = filepath.Join(alias, "collector.sqlite"), filepath.Join(alias, "server.sqlite")
	other, err := aliased.Canonical()
	if err != nil || other != canonical {
		t.Fatal("symlink identity differs", err)
	}
	socket, err := SocketPath(Settings{ServerDBPath: filepath.Join(root, strings.Repeat("long-directory/", 30), "server.sqlite")})
	if err != nil || len(socket) >= 104 {
		t.Fatal("runtime socket inherits long storage path", len(socket), err)
	}
	aliased.CollectorDBPath = aliased.ServerDBPath
	if _, err := aliased.Canonical(); err == nil {
		t.Fatal("role alias accepted")
	}
	if err := os.WriteFile(settings.ServerDBPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(settings.ServerDBPath, filepath.Join(root, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if err := settings.ValidateStorageIdentity(); err == nil {
		t.Fatal("hard-linked storage accepted")
	}
}

func TestEndpointOriginsAreDialableAndLoopbackWhenRequired(t *testing.T) {
	for _, target := range []string{"http://127.0.0.1:8765", "http://localhost:8765/"} {
		got, err := HTTPOrigin(target, true)
		if err != nil || got != "http://127.0.0.1:8765" {
			t.Fatal("loopback alias rejected", got, err)
		}
	}
	for _, target := range []string{"http://0.0.0.0:8765", "http://192.0.2.1:8765", "https://127.0.0.1:8765", "http://user@127.0.0.1:8765", "http://127.0.0.1:8765/?x=1", "http://127.0.0.1:8765/#fragment", "http://127.0.0.1:0"} {
		if _, err := HTTPOrigin(target, true); err == nil {
			t.Fatal("nonlocal or malformed endpoint accepted", target)
		}
	}
	if _, err := HTTPOrigin("http://192.0.2.1:8765", false); err != nil {
		t.Fatal("explicit broad host descriptor rejected", err)
	}
}
