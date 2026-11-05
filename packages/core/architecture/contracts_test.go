package architecture_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// These rules protect capability and ownership boundaries, not directory shape.
// Behavioral contracts (replay, atomicity, isolation and direct/HTTP parity) live
// with their consumers and use real stores/adapters.
func TestLayerDependencies(t *testing.T) {
	const module = "github.com/flexdinesh/tokeninsights/"
	const prefix = module + "packages/core/"
	packageKey := func(path string) string {
		path = strings.TrimPrefix(path, "packages/core/")
		path = strings.TrimPrefix(path, "packages/cli/internal/")
		return path
	}
	imports := map[string][]string{}
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == "node_modules" || entry.Name() == ".git") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel("../../..", filepath.Dir(path))
		if err != nil {
			return err
		}
		packageName := packageKey(filepath.ToSlash(relative))
		if _, found := imports[packageName]; !found {
			imports[packageName] = nil
		}
		for _, spec := range file.Imports {
			name, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			imports[packageName] = append(imports[packageName], name)
		}
		return nil
	}
	if err := filepath.WalkDir("../../../packages", walk); err != nil {
		t.Fatal(err)
	}
	for name, dependencies := range imports {
		if name == "storagecontract" {
			continue
		}
		for _, dependency := range dependencies {
			if dependency == prefix+"storagecontract" || dependency == prefix+"persistence/postgres/testdb" {
				t.Errorf("production package %s imports test contracts", name)
			}
		}
	}
	// Presentation dependencies stay inside their independently removable shell.
	for name, dependencies := range imports {
		if strings.HasPrefix(name, "packages/desktop-mygo") {
			continue
		}
		for _, dependency := range dependencies {
			if dependency == "github.com/egoist/mygo" || strings.HasPrefix(dependency, "github.com/egoist/mygo/") || strings.HasPrefix(dependency, "github.com/flexdinesh/tokeninsights/packages/desktop") {
				t.Errorf("%s imports desktop viewer dependency %s", name, dependency)
			}
		}
	}
	for name, dependencies := range imports {
		if strings.HasPrefix(name, "packages/") || name == "tui" || name == "cli" {
			continue
		}
		for _, dependency := range dependencies {
			if strings.HasPrefix(dependency, module+"packages/") && !strings.HasPrefix(dependency, prefix) {
				t.Errorf("core package %s imports product shell %s", name, dependency)
			}
		}
	}
	// Storage location rule (docs/design.md): composition roots select adapters;
	// role packages and adapters share engine mechanics from persistence.
	compositionRoots := []string{"localruntime", "packages/cli/cmd/tokeninsights", "packages/backend/cmd/tokeninsights-backend"}
	storageRoles := []string{"rawcollectorstore", "collectordb", "datastore"}
	for name, dependencies := range imports {
		adapterOwner := strings.HasPrefix(name, "adapters/")
		for _, dependency := range dependencies {
			if strings.HasPrefix(dependency, prefix+"adapters/") && !adapterOwner && !slices.Contains(compositionRoots, name) {
				t.Errorf("%s imports adapter %s; only composition roots and adapters select adapters", name, dependency)
			}
			if strings.HasPrefix(dependency, prefix+"persistence/") && !adapterOwner && !strings.HasPrefix(name, "persistence/") && !slices.Contains(storageRoles, name) {
				t.Errorf("%s imports engine mechanics %s; only storage roles and adapters use persistence", name, dependency)
			}
		}
	}
	pureContract := []string{"database/sql", "net/http", "os", prefix + "persistence", prefix + "adapters", prefix + "datastore", prefix + "server", prefix + "collector", prefix + "pipeline"}
	rules := []struct {
		name      string
		forbidden []string
	}{
		{"processor", []string{"database/sql", "net/http", "os", prefix + "pipeline", prefix + "datastore"}},
		{"dataengine", []string{"database/sql", "net/http", "os", prefix + "pipeline", prefix + "datastore", prefix + "server"}},
		{"pipeline", []string{prefix + "processor", prefix + "datastore", prefix + "server", prefix + "accounts", prefix + "collectorprogress"}},
		{"processusage", []string{"net/http", "database/sql", prefix + "collector", prefix + "pipeline", prefix + "datastore", prefix + "server", prefix + "localruntime"}},
		{"collectorprogress", []string{prefix + "pipeline", prefix + "collector", prefix + "localruntime", prefix + "datastore"}},
		{"datastore", []string{"net/http", prefix + "ingestionhttp", prefix + "collector", prefix + "pipeline", prefix + "server"}},
		{"application", []string{"database/sql", "net/http", "net/url", prefix + "server", prefix + "queryclient", prefix + "localruntime", prefix + "localclient", prefix + "datastore", prefix + "appstore", prefix + "collector", prefix + "pipeline"}},
		{"analytics", []string{"database/sql", "net/http", prefix + "datastore", prefix + "appstore", prefix + "collectordb", prefix + "adapters", prefix + "server"}},
		{"server", []string{"database/sql", prefix + "datastore", prefix + "appstore", prefix + "adapters", prefix + "pipeline", prefix + "collector", prefix + "localruntime", prefix + "remoteserver"}},
		{"localruntime", []string{prefix + "endpoint", prefix + "sshtransport", prefix + "tcptransport", prefix + "remoteaccess"}},
		{"tui", []string{"database/sql", prefix + "datastore", prefix + "appstore", prefix + "collectordb", prefix + "rawcollectorstore", prefix + "collector", prefix + "localruntime", prefix + "localbackend", prefix + "pipeline", prefix + "syncjob", prefix + "localstorage"}},
		{"capture", pureContract},
		{"localclient", []string{"database/sql", prefix + "datastore", prefix + "appstore", prefix + "collectordb", prefix + "localruntime", prefix + "localbackend", prefix + "pipeline", prefix + "adapters", prefix + "remoteserver"}},
		{"localprotocol", []string{"database/sql", prefix + "datastore", prefix + "appstore", prefix + "collectordb", prefix + "localruntime", prefix + "localbackend", prefix + "pipeline", prefix + "adapters", prefix + "remoteserver"}},
		{"serverruntime", []string{"database/sql", prefix + "datastore", prefix + "appstore", prefix + "collectordb", prefix + "adapters"}},
		// Connection values are pure. Transport adapters never select storage.
		// Bridges expose viewer operations, not backend composition.
		{"dashboardproxy", []string{"database/sql", prefix + "datastore", prefix + "appstore", prefix + "collectordb", prefix + "localruntime", prefix + "localbackend", prefix + "localclient", prefix + "server", prefix + "pipeline", prefix + "remoteserver", prefix + "collector"}},
		{"endpoint", []string{"database/sql", "net/http", "os", prefix + "datastore", prefix + "collectordb", prefix + "localruntime", prefix + "localbackend", prefix + "localclient", prefix + "server", prefix + "pipeline", prefix + "sshtransport", prefix + "tcptransport"}},
		{"sshtransport", []string{"database/sql", "net/http", prefix + "datastore", prefix + "collectordb", prefix + "localruntime", prefix + "localbackend", prefix + "localclient", prefix + "server", prefix + "pipeline", prefix + "collector"}},
		{"tcptransport", []string{"database/sql", prefix + "datastore", prefix + "collectordb", prefix + "localruntime", prefix + "localbackend", prefix + "localclient", prefix + "server", prefix + "pipeline", prefix + "collector"}},
		{"remoteaccess", []string{"database/sql", prefix + "datastore", prefix + "collectordb", prefix + "localruntime", prefix + "localbackend", prefix + "server", prefix + "pipeline", prefix + "collector", prefix + "config"}},
		{"desktoplaunch", []string{"database/sql", prefix + "datastore", prefix + "collectordb", prefix + "localruntime", prefix + "localbackend", prefix + "server", prefix + "pipeline", prefix + "collector"}},
		{"collector", []string{prefix + "processor", prefix + "dataengine", prefix + "datastore", prefix + "analytics", prefix + "accounts", prefix + "server", prefix + "localruntime", prefix + "remoteserver"}},
		{"evidence", pureContract},
		{"publication", pureContract},
		{"collectordb", []string{"net/http"}},
		{"writerlock", []string{"database/sql", "net/http"}},
		{"dbpath", []string{"database/sql", "net/http"}},
		{"storagecompat", pureContract},
	}
	persistenceOwners := map[string]bool{"datastore": true, "pipeline": true, "localruntime": true, "collector": true, "collectordb": true}
	// Command packages compose through localruntime; they never construct
	// storage adapters or HTTP handlers themselves.
	direct := []struct {
		name      string
		forbidden []string
		allowed   []string
	}{
		{"application", []string{"os", "net/url"}, nil},
		{"tui", []string{"net/http", "database/sql", prefix + "server", prefix + "adapters", prefix + "datastore", prefix + "persistence", prefix + "syncjob", prefix + "remoteserver"}, nil},
		{"cli", []string{"database/sql", prefix + "adapters", prefix + "datastore", prefix + "persistence", prefix + "remoteserver"}, nil},
	}
	for _, rule := range direct {
		t.Run("direct/"+rule.name, func(t *testing.T) {
			if _, found := imports[rule.name]; !found {
				t.Fatalf("boundary package %s is missing; update its contract", rule.name)
			}
			for _, dependency := range imports[rule.name] {
				if slices.Contains(rule.allowed, dependency) {
					continue
				}
				for _, forbidden := range rule.forbidden {
					if dependency == forbidden || strings.HasPrefix(dependency, forbidden+"/") {
						t.Errorf("%s directly imports forbidden dependency %s", rule.name, dependency)
					}
				}
			}
		})
	}
	for _, rule := range rules {
		if !persistenceOwners[rule.name] {
			rule.forbidden = append(rule.forbidden, prefix+"persistence", prefix+"adapters")
		}
		t.Run(rule.name, func(t *testing.T) {
			if _, found := imports[rule.name]; !found {
				t.Fatalf("boundary package %s is missing; update its contract", rule.name)
			}
			seen := map[string]bool{}
			var visit func(string)
			visit = func(name string) {
				if seen[name] {
					return
				}
				seen[name] = true
				for _, dependency := range imports[name] {
					// Viewer compositions use the read-wire adapter through
					// queryclient, without importing the server implementation.
					wireContract := dependency == prefix+"server/api" && (rule.name == "remoteaccess" || rule.name == "desktoplaunch")
					for _, forbidden := range rule.forbidden {
						if !wireContract && (dependency == forbidden || strings.HasPrefix(dependency, forbidden+"/")) {
							t.Errorf("%s imports forbidden dependency %s (root %s)", name, dependency, rule.name)
						}
					}
					if strings.HasPrefix(dependency, module) {
						visit(packageKey(strings.TrimPrefix(dependency, module)))
					}
				}
			}
			visit(rule.name)
		})
	}
}
