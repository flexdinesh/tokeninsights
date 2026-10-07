package serverfeatures

import "testing"

func TestCompositionPolicy(t *testing.T) {
	personal, err := New(Personal, true)
	if err != nil || !personal.Capabilities.Has(TerminalDashboard) || !personal.Capabilities.Has(CollectorProgress) {
		t.Fatal(personal, err)
	}
	foreground, err := New(Personal, false)
	if err != nil || foreground.Capabilities.Has(CollectorProgress) {
		t.Fatal(foreground, err)
	}
	hosted, err := New(Hosted, false)
	if err != nil || hosted.Capabilities.Has(TerminalDashboard) || hosted.Capabilities.Has(CollectorProgress) {
		t.Fatal(hosted, err)
	}
	if err := (Policy{Kind: Hosted, Capabilities: Capabilities{TerminalDashboard}}).Validate(); err == nil {
		t.Fatal("hosted terminal feature enabled")
	}
	if _, err := New(Personal, false, Usage); err == nil {
		t.Fatal("dashboard without usage enabled")
	}
	disabled, err := New(Personal, false, Usage, WebDashboard, TerminalDashboard)
	if err != nil || disabled.Capabilities.Has(Usage) {
		t.Fatal(disabled, err)
	}
	if _, err := New(Personal, false, "unknown"); err == nil {
		t.Fatal("unknown policy accepted")
	}
	known := (Capabilities{Usage, "future", Usage}).Known()
	if len(known) != 1 || known[0] != Usage {
		t.Fatal(known)
	}
}
