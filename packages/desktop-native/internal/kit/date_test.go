package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// galleryToday is the day the date inputs' Today chooses.
var galleryToday = time.Date(2026, time.November, 13, 0, 0, 0, 0, time.UTC)

// pressNamed clicks the element named name with role.
func pressNamed(t *testing.T, tt *ui.Tester, role ui.Role, name string) {
	t.Helper()
	a, ok := tt.AccessByRole(role, name)
	if !ok {
		t.Fatalf("no %q in %q", name, tt.Texts())
	}
	tt.ClickAt(a.Bounds.X+a.Bounds.W/2, a.Bounds.Y+a.Bounds.H/2)
	tt.Advance(settle)
}

func TestDateInputChoosesClearsAndPicksToday(t *testing.T) {
	var date time.Time
	changes := 0
	tt := newTester(func(c *ui.Context) {
		if DateInput(c.Key("from"), &date, DateInputOptions{Placeholder: "Beginning", Today: galleryToday}).Label("From date").Changed() {
			changes++
		}
	})
	field, ok := tt.AccessByRole(ui.RolePopUpButton, "From date")
	if !ok || field.Value != "Beginning" || field.Expanded {
		t.Fatalf("empty field %+v", field)
	}
	pressNamed(t, tt, ui.RolePopUpButton, "From date")
	if a, _ := tt.AccessByRole(ui.RolePopUpButton, "From date"); !a.Expanded {
		t.Fatal("calendar did not open")
	}
	if a, _ := tt.AccessByRole(ui.RoleButton, "Clear"); !a.Disabled {
		t.Fatal("Clear enabled while empty")
	}
	pressNamed(t, tt, ui.RoleButton, "November 20, 2026")
	if !date.Equal(time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC)) || changes != 1 {
		t.Fatalf("date %v, changes %d", date, changes)
	}
	if a, _ := tt.AccessByRole(ui.RolePopUpButton, "From date"); a.Expanded || a.Value != "11/20/2026" || !tt.Focused("From date") {
		t.Fatalf("chosen field %+v, focused %v", a, tt.Focused("From date"))
	}

	pressNamed(t, tt, ui.RolePopUpButton, "From date")
	pressNamed(t, tt, ui.RoleButton, "Clear")
	if !date.IsZero() || changes != 2 {
		t.Fatalf("cleared date %v, changes %d", date, changes)
	}

	// Enter opens from the keyboard; Today chooses o.Today.
	tt.Key(0, ui.KeyEnter)
	tt.Advance(settle)
	pressNamed(t, tt, ui.RoleButton, "Today")
	if !date.Equal(galleryToday) {
		t.Fatalf("today %v", date)
	}
	tt.Key(0, ui.KeySpace)
	tt.Advance(settle)
	if a, _ := tt.AccessByRole(ui.RolePopUpButton, "From date"); !a.Expanded {
		t.Fatal("Space did not open the calendar")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(settle)
	if a, _ := tt.AccessByRole(ui.RolePopUpButton, "From date"); a.Expanded || !tt.Focused("From date") {
		t.Fatalf("Escape left %+v", a)
	}
}

func TestDateInputMarksInvalid(t *testing.T) {
	date := galleryToday
	tt := newTester(func(c *ui.Context) {
		DateInput(c.Key("to"), &date, DateInputOptions{Placeholder: "Now", Today: galleryToday, Invalid: true}).Label("To date")
	})
	if a, ok := tt.AccessByRole(ui.RolePopUpButton, "To date"); !ok || !a.Invalid || a.Value != "11/13/2026" {
		t.Fatalf("invalid field %+v", a)
	}
}
