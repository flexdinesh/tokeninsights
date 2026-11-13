package icons

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every embedded file has an accessor and every accessor its file, so an icon
// added on the web side and copied here cannot go unexposed.
func TestEveryEmbeddedIconHasAnAccessor(t *testing.T) {
	names, err := fs.Glob(files, "lucide/*.svg")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != len(byFile) {
		t.Fatalf("%d files, %d accessors", len(names), len(byFile))
	}
	for _, name := range names {
		get, ok := byFile[strings.TrimSuffix(strings.TrimPrefix(name, "lucide/"), ".svg")]
		if !ok {
			t.Errorf("%s has no accessor", name)
			continue
		}
		first, second := get(), get()
		if first == nil || first != second {
			t.Errorf("%s is not parsed once", name)
		}
	}
}

// lucideImport matches a named import from lucide-react, across lines.
var lucideImport = regexp.MustCompile(`import\s*(?:type\s*)?\{([^}]*)\}\s*from\s*['"]lucide-react['"]`)

// kebab turns a lucide-react export name into its file name: Layers3 is layers-3.
var kebabBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])|([A-Za-z])([0-9])`)

func kebab(name string) string {
	return strings.ToLower(kebabBoundary.ReplaceAllString(name, "$1$3-$2$4"))
}

// The native set covers every icon the web dashboard imports, so ported views
// find the same icon.
func TestCoversEveryWebDashboardIcon(t *testing.T) {
	root := filepath.Join("..", "..", "..", "dashboard", "src")
	seen := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".tsx") {
			return err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range lucideImport.FindAllStringSubmatch(string(source), -1) {
			for _, name := range strings.Split(match[1], ",") {
				name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "type "))
				name, _, _ = strings.Cut(name, " as ")
				if name == "" {
					continue
				}
				seen++
				if _, ok := byFile[kebab(name)]; !ok {
					t.Errorf("%s imports %s, which has no native icon", path, name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Fatal("found no lucide-react imports")
	}
}

func TestKebabMatchesLucideFileNames(t *testing.T) {
	for name, want := range map[string]string{"Layers3": "layers-3", "ChartNoAxesCombined": "chart-no-axes-combined", "X": "x", "RotateCw": "rotate-cw"} {
		if got := kebab(name); got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
}
