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
	const prefix = "github.com/flexdinesh/tokeninsights/packages/cli/internal/"
	imports := map[string][]string{}
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		packageName := filepath.ToSlash(filepath.Dir(path))
		packageName = strings.TrimPrefix(strings.TrimPrefix(packageName, "../../"), "../")
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
	for _, root := range []string{"..", "../../cmd"} {
		if err := filepath.WalkDir(root, walk); err != nil {
			t.Fatal(err)
		}
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
	hostCapture := []string{prefix + "collector", prefix + "pipeline", prefix + "rawcollectorstore", prefix + "collectordb", prefix + "syncjob", prefix + "localruntime", prefix + "clientworkflow"}
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
		{"analytics", []string{"database/sql", "net/http", prefix + "datastore", prefix + "appstore", prefix + "collectordb", prefix + "adapters", prefix + "server"}},
		{"clientaddress", []string{"os", "database/sql", prefix + "accounts", prefix + "datastore", prefix + "server"}},
		{"accounts", []string{"database/sql", "net/http", prefix + "datastore", prefix + "appstore", prefix + "collectordb", prefix + "adapters", prefix + "clientaddress"}},
		{"server", []string{"database/sql", prefix + "datastore", prefix + "appstore", prefix + "adapters", prefix + "pipeline", prefix + "collector", prefix + "localruntime", prefix + "remoteserver"}},
		{"localruntime", []string{prefix + "syncjob", prefix + "clientworkflow"}},
		{"serverruntime", []string{"database/sql", prefix + "datastore", prefix + "appstore", prefix + "collectordb", prefix + "adapters"}},
		// Remote servers accept and process evidence; they never capture host sources.
		{"remoteserver", hostCapture},
		{"cmd/tokeninsights-server", hostCapture},
		{"collector", []string{prefix + "processor", prefix + "dataengine", prefix + "datastore", prefix + "analytics", prefix + "accounts", prefix + "server", prefix + "localruntime", prefix + "remoteserver"}},
		{"evidence", pureContract},
		{"publication", pureContract},
		{"collectordb", []string{"net/http"}},
		{"operator", []string{"database/sql", prefix + "datastore", prefix + "appstore", prefix + "server", prefix + "ingestionhttp"}},
		{"writerlock", []string{"database/sql", "net/http"}},
		{"dbpath", []string{"database/sql", "net/http"}},
	}
	persistenceOwners := map[string]bool{"datastore": true, "pipeline": true, "localruntime": true, "collector": true, "collectordb": true, "remoteserver": true, "cmd/tokeninsights-server": true}
	// Command packages compose through localruntime; they never construct
	// storage adapters or HTTP handlers themselves.
	direct := []struct {
		name      string
		forbidden []string
		allowed   []string
	}{
		{"tui", []string{"net/http", "database/sql", prefix + "server", prefix + "adapters", prefix + "datastore", prefix + "persistence", prefix + "syncjob", prefix + "remoteserver"}, []string{prefix + "server/api"}},
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
					for _, forbidden := range rule.forbidden {
						if dependency == forbidden || strings.HasPrefix(dependency, forbidden+"/") {
							t.Errorf("%s imports forbidden dependency %s (root %s)", name, dependency, rule.name)
						}
					}
					if strings.HasPrefix(dependency, prefix) {
						visit(strings.TrimPrefix(dependency, prefix))
					}
				}
			}
			visit(rule.name)
		})
	}
}
