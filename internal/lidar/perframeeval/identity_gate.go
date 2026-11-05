package perframeeval

import (
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"math"
)

// IdentityGate is the labelled per-frame component of l5id0004's rule. It
// does not certify the corpus duplicate, coverage, geometry or timing gates.
type IdentityGate struct {
	Verdict         string           `json:"verdict"`
	ReferenceDigest string           `json:"reference_digest"`
	SplitDigest     string           `json:"split_digest"`
	Clauses         []IdentityClause `json:"clauses"`
	Reason          string           `json:"reason,omitempty"`
}

type IdentityClause struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Bound float64 `json:"bound"`
	Pass  bool    `json:"pass"`
}

// AssessIdentity applies the predeclared per-frame limits, while requiring
// reviewed, frozen held-out truth for a pass. A tuning score can fail, but
// can never pass this acceptance component. Empty populations fail closed.
func AssessIdentity(c Comparison) IdentityGate {
	g := IdentityGate{Verdict: "insufficient_evidence", ReferenceDigest: c.Reference.Digest, SplitDigest: c.Reference.SplitDigest}
	a, b := c.Total.A, c.Total.B
	if a.NumGT <= 0 || a.Matches <= 0 || a.NumGT != b.NumGT {
		g.Reason = "requires the same non-empty reference population and baseline matches"
		return g
	}
	for _, v := range []float64{a.HOTA, b.HOTA, a.IDF1, b.IDF1} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			g.Reason = "non-finite or invalid identity metric"
			return g
		}
	}
	if a.IDSwitches < 0 || b.IDSwitches < 0 || a.Fragmentations < 0 || b.Fragmentations < 0 || b.Matches < 0 {
		g.Reason = "negative count"
		return g
	}
	g.Clauses = []IdentityClause{
		{"matches", float64(b.Matches), .98 * float64(a.Matches), float64(b.Matches) >= .98*float64(a.Matches)},
		{"hota", b.HOTA, a.HOTA - .005, b.HOTA >= a.HOTA-.005},
		{"idf1", b.IDF1, a.IDF1 - .005, b.IDF1 >= a.IDF1-.005},
		{"id_switches", float64(b.IDSwitches), float64(a.IDSwitches), b.IDSwitches <= a.IDSwitches},
		{"fragmentations", float64(b.Fragmentations), 1.1*float64(a.Fragmentations) + 1, float64(b.Fragmentations) <= 1.1*float64(a.Fragmentations)+1},
	}
	for _, clause := range g.Clauses {
		if !clause.Pass {
			g.Verdict = "fail"
		}
	}
	if g.Verdict == "fail" {
		g.Reason = "one or more predeclared labelled identity limits failed"
		return g
	}
	if !c.Reference.HeldOut || c.Reference.SplitRole != annotation.SplitRoleHeldOut || c.Reference.SplitDigest == "" || c.Reference.SplitRevision <= 0 || c.Reference.Digest == "" || c.Reference.Policy.Status != annotation.ReferenceReviewedOnly {
		g.Reason = "acceptance requires a frozen held-out split with reviewed truth; tuning or proposed labels cannot qualify"
		return g
	}
	g.Verdict = "pass"
	return g
}
