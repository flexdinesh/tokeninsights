package localruntime

import "github.com/flexdinesh/tokeninsights/packages/core/application"

// Application exposes semantic operations while retaining storage ownership.
// AllUsage delegates to one repository AllDashboard transaction.
func (r *Runtime) Application() application.Reader {
	return application.NewQueries(r.queries, r.Policy, r.InstanceID, r.Hostname)
}
