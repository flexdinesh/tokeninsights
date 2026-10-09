// Package serverfeatures defines deployment policy shared by servers and clients.
package serverfeatures

import "fmt"

type Kind string

const (
	Personal Kind = "personal"
	Hosted   Kind = "hosted"
)

func (k Kind) Validate() error {
	if k != Personal && k != Hosted {
		return fmt.Errorf("server-kind must be personal or hosted")
	}
	return nil
}

type Capability string

const (
	Usage             Capability = "usage"
	Facets            Capability = "facets"
	WebDashboard      Capability = "web-dashboard"
	DashboardReload   Capability = "dashboard-reload"
	RawIngestion      Capability = "raw-ingestion"
	TerminalDashboard Capability = "terminal-dashboard"
	CollectorProgress Capability = "collector-progress"
	Reprocess         Capability = "reprocess"
)

func (c Capability) Known() bool {
	switch c {
	case Usage, Facets, WebDashboard, DashboardReload, RawIngestion, TerminalDashboard, CollectorProgress, Reprocess:
		return true
	}
	return false
}

type Capabilities []Capability

func (cs Capabilities) Has(want Capability) bool {
	for _, c := range cs {
		if c == want {
			return true
		}
	}
	return false
}

// Known ignores additive wire capabilities without granting missing features.
func (cs Capabilities) Known() Capabilities {
	result := make(Capabilities, 0, len(cs))
	for _, c := range cs {
		if c.Known() && !result.Has(c) {
			result = append(result, c)
		}
	}
	return result
}

type Policy struct {
	Kind         Kind
	Capabilities Capabilities
}

func New(kind Kind, collectorProgress bool, disabled ...Capability) (Policy, error) {
	return newPolicy(kind, collectorProgress, false, disabled...)
}

// NewLocalViewer enables query-only Reload for a command-owned local viewer.
// Saved-data viewers retain Reload even when they skip initial collection.
func NewLocalViewer(collectorProgress bool, disabled ...Capability) (Policy, error) {
	return newPolicy(Personal, collectorProgress, true, disabled...)
}

func newPolicy(kind Kind, collectorProgress, dashboardReload bool, disabled ...Capability) (Policy, error) {
	p := Policy{Kind: kind, Capabilities: Capabilities{Usage, Facets, WebDashboard, RawIngestion, Reprocess}}
	if dashboardReload {
		p.Capabilities = append(p.Capabilities, DashboardReload)
	}
	if kind == Personal {
		p.Capabilities = append(p.Capabilities, TerminalDashboard)
		if collectorProgress {
			p.Capabilities = append(p.Capabilities, CollectorProgress)
		}
	}
	for _, c := range disabled {
		if !c.Known() {
			return Policy{}, fmt.Errorf("unknown disabled capability %q", c)
		}
		kept := make(Capabilities, 0, len(p.Capabilities))
		for _, existing := range p.Capabilities {
			if existing != c {
				kept = append(kept, existing)
			}
		}
		p.Capabilities = kept
	}
	return p, p.Validate()
}

func (p Policy) Validate() error {
	if err := p.Kind.Validate(); err != nil {
		return err
	}
	if p.Kind == Hosted && (p.Capabilities.Has(TerminalDashboard) || p.Capabilities.Has(CollectorProgress) || p.Capabilities.Has(DashboardReload)) {
		return fmt.Errorf("hosted server cannot support terminal-dashboard, collector-progress or dashboard-reload")
	}
	for _, c := range p.Capabilities {
		if !c.Known() {
			return fmt.Errorf("unknown capability %q", c)
		}
	}
	if (p.Capabilities.Has(TerminalDashboard) || p.Capabilities.Has(WebDashboard)) && (!p.Capabilities.Has(Usage) || !p.Capabilities.Has(Facets)) {
		return fmt.Errorf("dashboards require usage and facets")
	}
	if p.Capabilities.Has(CollectorProgress) && !p.Capabilities.Has(RawIngestion) {
		return fmt.Errorf("collector-progress requires raw-ingestion")
	}
	if p.Capabilities.Has(DashboardReload) && (!p.Capabilities.Has(WebDashboard) || !p.Capabilities.Has(Usage)) {
		return fmt.Errorf("dashboard-reload requires web-dashboard and usage")
	}
	return nil
}

type Permission string

const (
	Read   Permission = "read"
	Ingest Permission = "ingest"
)

type Permissions []Permission

func (ps Permissions) Has(want Permission) bool {
	for _, p := range ps {
		if p == want {
			return true
		}
	}
	return false
}
