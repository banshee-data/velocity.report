package perframeeval

import (
	"encoding/json"
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// ComparisonSchema names the comparison document.
const ComparisonSchema = "velocity.report/perframe-comparison"

// ComparisonSchemaVersion is its layout version.
const ComparisonSchemaVersion = 1

// Summary is the headline of one per-frame result: the numbers a reader
// compares, flat, so a delta is one subtraction per field.
type Summary struct {
	MOTA           float64 `json:"mota"`
	MOTP           float64 `json:"motp_metres"`
	IDSwitches     int     `json:"id_switches"`
	Fragmentations int     `json:"fragmentations"`
	FN             int     `json:"fn"`
	FP             int     `json:"fp"`
	Matches        int     `json:"matches"`
	NumGT          int     `json:"num_gt"`
	HOTA           float64 `json:"hota"`
	DetA           float64 `json:"det_a"`
	AssA           float64 `json:"ass_a"`
	IDF1           float64 `json:"idf1"`
	IDP            float64 `json:"idp"`
	IDR            float64 `json:"idr"`
}

func summarise(r l8analytics.PerFrameResult) Summary {
	return Summary{
		MOTA: r.CLEARMOT.MOTA, MOTP: r.CLEARMOT.MOTP,
		IDSwitches: r.CLEARMOT.IDSwitches, Fragmentations: r.CLEARMOT.Fragmentations,
		FN: r.CLEARMOT.FN, FP: r.CLEARMOT.FP, Matches: r.CLEARMOT.Matches, NumGT: r.CLEARMOT.NumGT,
		HOTA: r.HOTA.HOTA, DetA: r.HOTA.DetA, AssA: r.HOTA.AssA,
		IDF1: r.Identity.IDF1, IDP: r.Identity.IDP, IDR: r.Identity.IDR,
	}
}

// minus is b - a, field by field.
func (b Summary) minus(a Summary) Summary {
	return Summary{
		MOTA: b.MOTA - a.MOTA, MOTP: b.MOTP - a.MOTP,
		IDSwitches: b.IDSwitches - a.IDSwitches, Fragmentations: b.Fragmentations - a.Fragmentations,
		FN: b.FN - a.FN, FP: b.FP - a.FP, Matches: b.Matches - a.Matches, NumGT: b.NumGT - a.NumGT,
		HOTA: b.HOTA - a.HOTA, DetA: b.DetA - a.DetA, AssA: b.AssA - a.AssA,
		IDF1: b.IDF1 - a.IDF1, IDP: b.IDP - a.IDP, IDR: b.IDR - a.IDR,
	}
}

// PairedResult is both arms on one episode, or pooled over all of them.
type PairedResult struct {
	EpisodeID string  `json:"episode_id"`
	A         Summary `json:"a"`
	B         Summary `json:"b"`
	Delta     Summary `json:"delta_b_minus_a"`
}

// Comparison is two arms scored against one reference.
type Comparison struct {
	Schema              string                `json:"schema"`
	SchemaVersion       int                   `json:"schema_version"`
	Reference           ReferenceIdentity     `json:"reference"`
	Gate                l8analytics.MatchGate `json:"gate"`
	FrameToleranceNanos int64                 `json:"frame_tolerance_ns"`
	Paired              []PairedResult        `json:"paired"`
	Total               PairedResult          `json:"total"`
	// Caveats are the limits a reader must know before quoting a number.
	Caveats []string  `json:"caveats"`
	A       ArmResult `json:"arm_a"`
	B       ArmResult `json:"arm_b"`
}

// CompareArms pairs two arms episode by episode. It refuses when anything that
// decides the numbers differs between them: the reference and its policy, the
// episodes, the gate, the frame tolerance, or the kind of hypothesis.
func CompareArms(a, b ArmResult) (Comparison, error) {
	if !samePolicy(a.Reference.Policy, b.Reference.Policy) {
		return Comparison{}, fmt.Errorf("arms were scored under different reference policies (%+v and %+v)",
			a.Reference.Policy, b.Reference.Policy)
	}
	if !sameStrings(a.Reference.Episodes, b.Reference.Episodes) || len(a.Episodes) != len(b.Episodes) {
		return Comparison{}, fmt.Errorf("arms were scored on different episodes (%v and %v)", a.Reference.Episodes, b.Reference.Episodes)
	}
	for i := range a.Episodes {
		if a.Episodes[i].EpisodeID != b.Episodes[i].EpisodeID {
			return Comparison{}, fmt.Errorf("arms were scored on different episodes at position %d (%s and %s)",
				i, a.Episodes[i].EpisodeID, b.Episodes[i].EpisodeID)
		}
	}
	if a.Gate != b.Gate {
		return Comparison{}, fmt.Errorf("arms were scored under different gates (%s and %s)", a.Gate, b.Gate)
	}
	if a.FrameToleranceNanos != b.FrameToleranceNanos {
		return Comparison{}, fmt.Errorf("arms were aligned with different frame tolerances (%d and %d ns)",
			a.FrameToleranceNanos, b.FrameToleranceNanos)
	}
	if a.Reference.Digest == "" || a.Reference.Digest != b.Reference.Digest {
		return Comparison{}, fmt.Errorf("arms were scored against different references (digest %q and %q)",
			a.Reference.Digest, b.Reference.Digest)
	}
	if a.Arm.Kind != b.Arm.Kind {
		return Comparison{}, fmt.Errorf("arm %s is %s and arm %s is %s: their positions come from different write paths, so a difference between them is not a difference between estimators",
			a.Arm.Label, a.Arm.Kind, b.Arm.Label, b.Arm.Kind)
	}

	c := Comparison{
		Schema: ComparisonSchema, SchemaVersion: ComparisonSchemaVersion,
		Reference: a.Reference, Gate: a.Gate, FrameToleranceNanos: a.FrameToleranceNanos,
		A: a, B: b,
	}
	for i := range a.Episodes {
		sa, sb := summarise(a.Episodes[i].Metrics), summarise(b.Episodes[i].Metrics)
		c.Paired = append(c.Paired, PairedResult{EpisodeID: a.Episodes[i].EpisodeID, A: sa, B: sb, Delta: sb.minus(sa)})
	}
	ta, tb := summarise(a.Total), summarise(b.Total)
	c.Total = PairedResult{EpisodeID: "all", A: ta, B: tb, Delta: tb.minus(ta)}
	c.Caveats = caveats(c)
	return c, nil
}

func caveats(c Comparison) []string {
	var out []string
	if !c.Reference.HeldOut {
		out = append(out, fmt.Sprintf("Split %q has role %q: this is not a held-out result and must not be quoted as one.",
			c.Reference.Split, c.Reference.SplitRole))
	}
	if c.Reference.Policy.Status != annotation.ReferenceReviewedOnly {
		out = append(out, fmt.Sprintf("The reference policy is %q: proposed masks nobody reviewed are scored as truth.",
			c.Reference.Policy.Status))
	}
	for _, arm := range []ArmIdentity{c.A.Arm, c.B.Arm} {
		if arm.Stage != StageFinal {
			out = append(out, fmt.Sprintf("Arm %s scores %s-stage positions as a declared baseline, not final estimates.",
				arm.Label, arm.Stage))
		}
	}
	out = append(out, "False positives include hypotheses on returns nobody labelled. They are an upper bound "+
		"unless every road user in the episodes' frames has a mask; the deltas are sounder than either arm's count.")
	return out
}

func samePolicy(a, b annotation.ReferencePolicy) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ja) == string(jb)
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
