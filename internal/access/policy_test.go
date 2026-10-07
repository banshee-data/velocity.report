package access

import "testing"

func TestPrincipalAuthority(t *testing.T) {
	for _, tc := range []struct {
		name      string
		principal Principal
		operation Operation
		want      bool
	}{
		{"viewer aggregates", Viewer("anonymous", false), ViewAggregates, true},
		{"viewer report", Viewer("tailnet", true), ReadReports, true},
		{"viewer generation", Viewer("tailnet", true), CreateReports, false},
		{"viewer config", Viewer("tailnet", true), Configure, false},
		{"admin generation", Administrator("tailnet"), CreateReports, true},
		{"admin export", Administrator("tailnet"), ExportData, true},
		{"admin maintenance", Administrator("tailnet"), Maintenance, false},
		{"admin access", Administrator("tailnet"), ManageAccess, false},
		{"compat maintenance", Compatibility(), Maintenance, true},
		{"unknown operation", Principal{Permissions: []Operation{"unknown"}}, "unknown", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.principal.Allows(Request{tc.operation, "site:1"}); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			if tc.principal.Allows(Request{tc.operation, ""}) {
				t.Fatal("missing resource was admitted")
			}
		})
	}
}
