// Package access defines transport-independent operation permissions. Authentication
// adapters supply principals; HTTP and future native/gRPC adapters share this policy.
package access

// Operation describes an action and its disclosure or side effects.
type Operation string

const (
	ViewAggregates    Operation = "aggregate:view"
	ReadReports       Operation = "reports:read"
	CreateReports     Operation = "reports:create"
	ReadConfiguration Operation = "configuration:read"
	Configure         Operation = "configuration:write"
	ExportData        Operation = "data:export"
	Maintenance       Operation = "maintenance:use"
	ManageAccess      Operation = "access:manage"
)

// Principal is application identity, independent of an address or transport.
// Subject is empty for anonymous and capability-only proxy principals. It must
// never be populated with an invented human identity.
type Principal struct {
	Mechanism     string      `json:"mechanism"`
	Subject       string      `json:"-"`
	Authenticated bool        `json:"authenticated"`
	Permissions   []Operation `json:"permissions"`
}

// Request carries the resource even while v0.5.1 grants are device-wide.
// Future account/group policies can narrow resources at this boundary.
type Request struct {
	Operation Operation
	Resource  string
}

// Allows refuses unknown operations and missing resource context.
func (p Principal) Allows(r Request) bool {
	if r.Resource == "" {
		return false
	}
	switch r.Operation {
	case ViewAggregates, ReadReports, CreateReports, ReadConfiguration, Configure, ExportData, Maintenance, ManageAccess:
	default:
		return false
	}
	for _, permission := range p.Permissions {
		if permission == r.Operation {
			return true
		}
	}
	return false
}

// Viewer grants deliberately disclosed aggregate and ordinary report reading.
func Viewer(mechanism string, authenticated bool) Principal {
	return Principal{Mechanism: mechanism, Authenticated: authenticated,
		Permissions: []Operation{ViewAggregates, ReadReports}}
}

// Administrator explicitly bundles routine operations, excluding maintenance
// and access management. Those authorities remain OS-local in hardened v0.5.1.
func Administrator(mechanism string) Principal {
	p := Viewer(mechanism, true)
	p.Permissions = append(p.Permissions, ReadConfiguration, Configure, CreateReports, ExportData)
	return p
}

// Compatibility describes the legacy authority for permission-aware clients.
func Compatibility() Principal {
	p := Administrator("compatibility")
	p.Permissions = append(p.Permissions, Maintenance, ManageAccess)
	return p
}
