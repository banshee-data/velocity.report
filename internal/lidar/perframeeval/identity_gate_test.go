package perframeeval

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

func identityComparison() Comparison {
	s := Summary{NumGT: 200, Matches: 100, HOTA: .6, IDF1: .7, IDSwitches: 10, Fragmentations: 10}
	return Comparison{Reference: ReferenceIdentity{HeldOut: true, SplitRole: annotation.SplitRoleHeldOut, SplitDigest: "frozen", SplitRevision: 1, Digest: "reference", Policy: annotation.DefaultReferencePolicy()}, Total: PairedResult{A: s, B: s}}
}

func TestIdentityGateBoundariesAndMissingEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Comparison)
		want   string
	}{
		{"equal", func(*Comparison) {}, "pass"},
		{"at recall bound", func(c *Comparison) { c.Total.B.Matches = 98 }, "pass"},
		{"below recall bound", func(c *Comparison) { c.Total.B.Matches = 97 }, "fail"},
		{"identity loss", func(c *Comparison) { c.Total.B.IDF1 -= .006 }, "fail"},
		{"switch regression", func(c *Comparison) { c.Total.B.IDSwitches++ }, "fail"},
		{"fragment regression", func(c *Comparison) { c.Total.B.Fragmentations = 13 }, "fail"},
		{"tuning", func(c *Comparison) { c.Reference.HeldOut = false }, "insufficient_evidence"},
		{"unfrozen", func(c *Comparison) { c.Reference.SplitDigest = "" }, "insufficient_evidence"},
		{"proposed truth", func(c *Comparison) { c.Reference.Policy.Status = annotation.ReferenceIncludeProposed }, "insufficient_evidence"},
		{"empty", func(c *Comparison) { c.Total.A.NumGT = 0 }, "insufficient_evidence"},
		{"lost population", func(c *Comparison) { c.Total.B.NumGT-- }, "insufficient_evidence"},
		{"nan", func(c *Comparison) { c.Total.B.HOTA = math.NaN() }, "insufficient_evidence"},
		{"negative count", func(c *Comparison) { c.Total.B.IDSwitches = -1 }, "insufficient_evidence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := identityComparison()
			tc.mutate(&c)
			g := AssessIdentity(c)
			if g.Verdict != tc.want {
				t.Fatalf("got %+v, want %s", g, tc.want)
			}
		})
	}
}
