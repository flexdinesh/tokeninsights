package theme

import "testing"

func TestAppearanceCyclesLikeTheWebThemeButton(t *testing.T) {
	want := map[Appearance]Appearance{
		AppearanceSystem: AppearanceLight,
		AppearanceLight:  AppearanceDark,
		AppearanceDark:   AppearanceSystem,
	}
	for _, a := range Appearances() {
		if got := a.Next(); got != want[a] {
			t.Errorf("%s.Next() = %s, want %s", a, got, want[a])
		}
	}
	if Appearance("sepia").Valid() || !AppearanceDark.Valid() {
		t.Fatal("validity")
	}
}

func TestAppearanceChoiceOverridesSystem(t *testing.T) {
	for _, test := range []struct {
		choice     Appearance
		systemDark bool
		want       *Tokens
	}{
		{AppearanceSystem, false, For(false)},
		{AppearanceSystem, true, For(true)},
		{AppearanceLight, true, For(false)},
		{AppearanceDark, false, For(true)},
	} {
		if got := test.choice.Tokens(test.systemDark); got != test.want {
			t.Errorf("%s with system dark %v", test.choice, test.systemDark)
		}
	}
}
