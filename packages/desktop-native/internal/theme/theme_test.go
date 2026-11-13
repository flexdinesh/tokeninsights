package theme

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// designColors reads the DESIGN.md frontmatter color roles, the browser and
// native visual contract, as name -> [light, dark].
func designColors(t *testing.T) map[string][2]string {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "..", "..", "DESIGN.md"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	values := map[string]string{}
	inColors := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "colors:" {
			inColors = true
			continue
		}
		if !inColors {
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			break
		}
		name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			t.Fatal("malformed color line", line)
		}
		values[name] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	pairs := map[string][2]string{}
	for name, value := range values {
		if strings.HasPrefix(name, "dark-") {
			continue
		}
		darkValue, ok := values["dark-"+name]
		if !ok {
			t.Fatal("DESIGN.md role has no dark value", name)
		}
		pairs[name] = [2]string{value, darkValue}
	}
	return pairs
}

func TestRolesMatchDesignContract(t *testing.T) {
	design := designColors(t)
	if len(design) == 0 {
		t.Fatal("DESIGN.md has no color roles")
	}
	seen := map[string]bool{}
	for _, r := range roles {
		want, ok := design[r.name]
		if !ok {
			t.Errorf("native role %s is not in DESIGN.md", r.name)
			continue
		}
		if seen[r.name] {
			t.Errorf("duplicate native role %s", r.name)
		}
		seen[r.name] = true
		if r.light != want[0] || r.dark != want[1] {
			t.Errorf("%s = %s/%s, DESIGN.md %s/%s", r.name, r.light, r.dark, want[0], want[1])
		}
	}
	for name := range design {
		if !seen[name] {
			t.Errorf("DESIGN.md role %s has no native token", name)
		}
	}
}

// sRGBHex is the one DESIGN.md color form.
var sRGBHex = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func TestDesignColorsAreSRGBHex(t *testing.T) {
	for name, values := range designColors(t) {
		for _, value := range values {
			if !sRGBHex.MatchString(value) {
				t.Errorf("DESIGN.md %s = %q, want sRGB #rrggbb", name, value)
			}
		}
	}
}

// A color outside sRGB makes MyGo switch a window to wide-gamut frame
// buffers, so every role must be an sRGB color.
func TestEveryRoleIsInsideSRGB(t *testing.T) {
	for _, isDark := range []bool{false, true} {
		tokens := For(isDark)
		for _, r := range roles {
			if c := *r.field(tokens); c.SRGB() != c {
				t.Errorf("%s (dark %v) is outside sRGB", r.name, isDark)
			}
		}
	}
}

func TestEveryRoleResolvesInBothAppearances(t *testing.T) {
	for _, isDark := range []bool{false, true} {
		tokens, err := build(isDark)
		if err != nil {
			t.Fatal(err)
		}
		if tokens.Surface == tokens.Ink || tokens.Action.A == 0 || tokens.Chart[ChartColors-1].A == 0 {
			t.Fatal("unresolved tokens", isDark)
		}
	}
	if For(false).Surface == For(true).Surface {
		t.Fatal("light and dark surfaces match")
	}
	if light, dark := For(false).Shadow, For(true).Shadow; light.A == 0 || dark.A <= light.A {
		t.Fatal("overlay shadow must be visible and deeper in the dark", light, dark)
	}
}

func TestColorParserRejectsUnsupportedForms(t *testing.T) {
	for _, spec := range []string{"", "#fff", "#gggggg", "#+12345", "#ffffff00", "rgb(0 0 0)", "oklch(0.5 0.1 20)"} {
		if _, err := parseColor(spec); err == nil {
			t.Error("accepted", spec)
		}
	}
}

func TestWidgetThemeIsSquareGeistAndQuietBlue(t *testing.T) {
	for _, isDark := range []bool{false, true} {
		tokens := For(isDark)
		widgets := tokens.Widgets()
		if widgets.Radius != 0 || widgets.Font != Font || widgets.FontSize != SizeBody || widgets.Accent != tokens.Action ||
			widgets.Focus != tokens.Focus || widgets.Background != tokens.Surface || widgets.Dark != isDark ||
			widgets.FocusRingWidth != FocusRing || widgets.FocusRingOffset != FocusOffset {
			t.Fatalf("%+v", widgets)
		}
	}
}

func TestBundledFontRegisters(t *testing.T) {
	if err := RegisterFonts(); err != nil {
		t.Fatal(err)
	}
}
