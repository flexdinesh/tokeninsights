package theme

// Appearance is the app-wide theme choice: System follows the OS, Light and
// Dark pin one appearance. Its values are the web's theme names.
type Appearance string

const (
	AppearanceSystem Appearance = "system"
	AppearanceLight  Appearance = "light"
	AppearanceDark   Appearance = "dark"
)

// Appearances lists every choice in the header's cycle order.
func Appearances() []Appearance {
	return []Appearance{AppearanceSystem, AppearanceLight, AppearanceDark}
}

// Valid reports whether a is one of the Appearances.
func (a Appearance) Valid() bool {
	switch a {
	case AppearanceSystem, AppearanceLight, AppearanceDark:
		return true
	}
	return false
}

// Next is the choice after a in the header's cycle, as the web's theme
// button: system, light, dark, then system again.
func (a Appearance) Next() Appearance {
	switch a {
	case AppearanceSystem:
		return AppearanceLight
	case AppearanceLight:
		return AppearanceDark
	}
	return AppearanceSystem
}

// Tokens resolves a against the system's appearance.
func (a Appearance) Tokens(systemDark bool) *Tokens {
	switch a {
	case AppearanceLight:
		return For(false)
	case AppearanceDark:
		return For(true)
	}
	return For(systemDark)
}
