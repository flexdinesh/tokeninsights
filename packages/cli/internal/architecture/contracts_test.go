package architecture_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
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
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
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
		packageName := strings.TrimPrefix(filepath.ToSlash(filepath.Dir(path)), "../")
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
	})
	if err != nil {
		t.Fatal(err)
	}
	rules := []struct {
		name      string
		forbidden []string
	}{
		{"processor", []string{"database/sql", "net/http", "os", prefix + "pipeline", prefix + "datastore"}},
		{"dataengine", []string{"database/sql", "net/http", "os", prefix + "pipeline", prefix + "datastore", prefix + "server"}},
		{"pipeline", []string{prefix + "processor", prefix + "datastore", prefix + "server", prefix + "accounts"}},
		{"datastore", []string{"net/http", prefix + "ingestionhttp", prefix + "collector", prefix + "pipeline", prefix + "server"}},
		{"querymodel", []string{"database/sql", "net/http", prefix + "db", prefix + "datastore", prefix + "server"}},
		{"server", []string{prefix + "pipeline", prefix + "collector", prefix + "localruntime", prefix + "remoteserver"}},
	}
	for _, rule := range rules {
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
