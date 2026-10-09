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

func TestReloadRequiresLocalViewerComposition(t *testing.T) {
	for _, kind := range []Kind{Personal, Hosted} {
		policy, err := New(kind, false)
		if err != nil || policy.Capabilities.Has(DashboardReload) {
			t.Fatalf("%s server granted local Reload: %+v, %v", kind, policy, err)
		}
	}
	for _, progress := range []bool{false, true} {
		policy, err := NewLocalViewer(progress)
		if err != nil || !policy.Capabilities.Has(DashboardReload) || policy.Capabilities.Has(CollectorProgress) != progress {
			t.Fatalf("local viewer progress=%t: %+v, %v", progress, policy, err)
		}
	}
	policy, err := NewLocalViewer(true, DashboardReload)
	if err != nil || policy.Capabilities.Has(DashboardReload) || !policy.Capabilities.Has(CollectorProgress) {
		t.Fatalf("disabled Reload affected collection progress: %+v, %v", policy, err)
	}
	if !DashboardReload.Known() || len((Capabilities{"future", DashboardReload, DashboardReload}).Known()) != 1 {
		t.Fatal("Reload capability negotiation failed")
	}
}

func TestReloadPolicyDependencies(t *testing.T) {
	for _, test := range []struct {
		name         string
		policy       Policy
		wantAccepted bool
	}{
		{name: "local query Reload", policy: Policy{Kind: Personal, Capabilities: Capabilities{Usage, Facets, WebDashboard, DashboardReload}}, wantAccepted: true},
		{name: "hosted rejects Reload", policy: Policy{Kind: Hosted, Capabilities: Capabilities{Usage, Facets, WebDashboard, DashboardReload}}},
		{name: "Reload requires dashboard", policy: Policy{Kind: Personal, Capabilities: Capabilities{Usage, DashboardReload}}},
		{name: "Reload requires usage", policy: Policy{Kind: Personal, Capabilities: Capabilities{DashboardReload}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.policy.Validate(); (err == nil) != test.wantAccepted {
				t.Fatalf("Validate() = %v; want accepted %t", err, test.wantAccepted)
			}
		})
	}
	if _, err := NewLocalViewer(false, WebDashboard); err == nil {
		t.Fatal("local Reload without web dashboard accepted")
	}
	if _, err := NewLocalViewer(false, DashboardReload, WebDashboard); err != nil {
		t.Fatalf("disabling Reload and web dashboard failed: %v", err)
	}
}
