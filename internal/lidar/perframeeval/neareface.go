package perframeeval

import (
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// Near-face scoring: where an estimated body puts the face the sensor sees,
// against the returns a person labelled on it.
//
// The mask-position score (the rest of this package) says whether tracks keep
// their identity, and cannot say whether a body estimate is accurate: its
// reference is the centre of the visible returns, which sits toward the sensor
// as the medoid does. The physical references would say, and a person has to
// author them. This is the part of the answer the labels already give. Take
// the returns of a reviewed mask and the body an arm believed at that frame,
// put the returns in the body's own axes, and look at the faces the sensor can
// see. The returns are not tracker output, and where the sensor sees a face is
// where the face is, to the sensor's noise, whatever the body's far side does.
//
//   - Face normal. For an end face, the believed face sits at half the length
//     from the centre along the axis; the labelled returns' extreme on that
//     side is where the surface is. Their difference, positive when the
//     believed face is farther out toward the sensor than the returns, is the
//     residual. A centre biased toward the sensor, or a length believed too
//     long, shows as a positive residual. The same for a side face and the
//     half-width.
//   - Tangent. An end face seen square spans the width, and the midpoint of
//     that span is unbiased by what the sensor cannot see. Its offset from the
//     believed centre is the error along the face, which no normal residual
//     can show and the rank-one fix leaves unconstrained. Reported only where
//     the span covers enough of the believed width, and as an absolute value:
//     its sign follows the heading, which may be ambiguous.
//
// What this does not score: the centre along an axis whose face the sensor
// cannot see, the far bumper, yaw, and the identity of the track. A body whose
// extents are class priors is scored but kept apart, because its residual
// mostly measures the prior.

// NearFaceSchema identifies the report's format.
const (
	NearFaceSchema        = "velocity.report/near-face-comparison"
	NearFaceSchemaVersion = 1
)

// Reasons an instant is not scored, as the accounting names them.
const (
	NearFaceUnmatched       = "unmatched"          // no body within the gate at that frame
	NearFaceNotBodyCentre   = "not_body_centre"    // the matched row is referenced to the medoid
	NearFaceNoOrientation   = "no_orientation"     // no heading belief
	NearFaceNoExtent        = "no_extent"          // no length or no width belief
	NearFaceTooFewReturns   = "too_few_returns"    // too few labelled returns to place a face
	NearFaceNoVisibleFace   = "no_visible_face"    // the sensor is inside both slabs of the believed box
	nearFaceTangentPartial  = "tangent_partial"    // the end face's span covers too little of the width
	nearFaceTangentWide     = "tangent_wide"       // ... or far more than the width: not one face
	nearFaceTangentTooFew   = "tangent_too_few"    // too few returns in the face band
	nearFaceStratumAll      = "all"                // every scored instant
	nearFaceStratumEvidence = "extent_evidence"    // both extents learnt from this object
	nearFaceStratumPrior    = "extent_class_prior" // an extent that is a class prior
)

// NearFaceOptions chooses the reference and the scoring.
type NearFaceOptions struct {
	PackDir           string
	SplitManifestPath string
	Split             string
	// Episodes restricts scoring to the named episodes of Split. Empty scores
	// all of them.
	Episodes []string
	// AllowTuningSplit scores a split whose role is not held_out, as for the
	// per-frame evaluator.
	AllowTuningSplit bool
	Policy           annotation.ReferencePolicy

	// FrameToleranceNanos is how far a body's frame time may be from a
	// sample's.
	FrameToleranceNanos int64
	// GateSlackMetres plus half the mask's footprint diagonal is how far a
	// body's centre may be from the mask's footprint centre and still be the
	// same object.
	GateSlackMetres float64
	// MinReturns is how many labelled returns a mask needs to place a face.
	MinReturns int
	// FaceQuantile is the share of returns, from the outward end, ignored when
	// taking a face's extreme, as a whole count: 0.02 ignores the outermost
	// two of a hundred, so a stray return does not move the face.
	FaceQuantile float64
	// FaceBandMetres is how deep from the extreme an end face's returns are
	// taken when measuring its span.
	FaceBandMetres float64
	// MinSpanCoverage is the least share of the believed width an end face's
	// span must cover for its midpoint to count; MaxSpanCoverage the most.
	MinSpanCoverage, MaxSpanCoverage float64
	// SensorXM and SensorYM are where the sensor is in the pack's frame.
	SensorXM, SensorYM float64
	// IncludeInstants keeps every scored instant in the result.
	IncludeInstants bool
}

// DefaultNearFaceOptions are the defaults the tool uses.
func DefaultNearFaceOptions() NearFaceOptions {
	return NearFaceOptions{
		Policy:              annotation.DefaultReferencePolicy(),
		FrameToleranceNanos: 10_000_000,
		GateSlackMetres:     1.0,
		MinReturns:          8,
		FaceQuantile:        0.02,
		FaceBandMetres:      0.3,
		MinSpanCoverage:     0.7,
		MaxSpanCoverage:     1.5,
	}
}

// Validate refuses options that cannot score.
func (o NearFaceOptions) Validate() error {
	switch {
	case o.FrameToleranceNanos < 0:
		return fmt.Errorf("frame tolerance %d ns is negative", o.FrameToleranceNanos)
	case o.GateSlackMetres < 0:
		return fmt.Errorf("gate slack %g m is negative", o.GateSlackMetres)
	case o.MinReturns < 2:
		return fmt.Errorf("min returns %d: a face needs at least two", o.MinReturns)
	case !(o.FaceQuantile >= 0 && o.FaceQuantile < 0.5):
		return fmt.Errorf("face quantile %g: want at least 0 and below 0.5", o.FaceQuantile)
	case !(o.FaceBandMetres > 0):
		return fmt.Errorf("face band %g m: want a positive depth", o.FaceBandMetres)
	case !(o.MinSpanCoverage > 0) || o.MaxSpanCoverage < o.MinSpanCoverage:
		return fmt.Errorf("span coverage %g to %g: want 0 < min <= max", o.MinSpanCoverage, o.MaxSpanCoverage)
	}
	return o.Policy.Validate()
}

// FaceStats summarises signed residuals in metres.
type FaceStats struct {
	N        int     `json:"n"`
	Mean     float64 `json:"mean_m"`
	Median   float64 `json:"median_m"`
	RMS      float64 `json:"rms_m"`
	MeanAbs  float64 `json:"mean_abs_m"`
	P95Abs   float64 `json:"p95_abs_m"`
	P99Abs   float64 `json:"p99_abs_m"`
	MaxAbs   float64 `json:"max_abs_m"`
	Over10cm float64 `json:"share_over_10cm"`
}

func summariseResiduals(v []float64) FaceStats {
	var s FaceStats
	s.N = len(v)
	if s.N == 0 {
		return s
	}
	signed := append([]float64(nil), v...)
	abs := make([]float64, len(v))
	var sum, sumSq, sumAbs float64
	var over int
	for i, x := range v {
		a := math.Abs(x)
		abs[i] = a
		sum += x
		sumSq += x * x
		sumAbs += a
		if a > 0.10 {
			over++
		}
	}
	sort.Float64s(signed)
	sort.Float64s(abs)
	n := float64(s.N)
	s.Mean, s.RMS, s.MeanAbs = sum/n, math.Sqrt(sumSq/n), sumAbs/n
	s.Median = quantileSorted(signed, 0.5)
	s.P95Abs, s.P99Abs, s.MaxAbs = quantileSorted(abs, 0.95), quantileSorted(abs, 0.99), abs[len(abs)-1]
	s.Over10cm = float64(over) / n
	return s
}

// quantileSorted is the linear-interpolation quantile of an ascending slice.
func quantileSorted(v []float64, q float64) float64 {
	if len(v) == 1 {
		return v[0]
	}
	pos := q * float64(len(v)-1)
	lo := int(math.Floor(pos))
	hi := min(lo+1, len(v)-1)
	return v[lo] + (v[hi]-v[lo])*(pos-float64(lo))
}

// NearFaceInstant is one scored (sample, object) pair.
type NearFaceInstant struct {
	SampleID int     `json:"sample_id"`
	ObjectID string  `json:"object_id"`
	TrackKey string  `json:"track_key"`
	RangeM   float64 `json:"range_m"`
	// SensorAxial and SensorLateral are the sensor's coordinates in the body's
	// axes: which face it faces, and whether it is outside the slab.
	SensorAxialM   float64  `json:"sensor_axial_m"`
	SensorLateralM float64  `json:"sensor_lateral_m"`
	EndNormalM     *float64 `json:"end_normal_m,omitempty"`
	SideNormalM    *float64 `json:"side_normal_m,omitempty"`
	EndTangentM    *float64 `json:"end_tangent_m,omitempty"`
	// ExtentEvidence is true when both extents were learnt from this object.
	ExtentEvidence bool `json:"extent_evidence"`
	Returns        int  `json:"returns"`
}

// NearFaceStratum is the statistics of the instants in one stratum.
type NearFaceStratum struct {
	Name       string    `json:"name"`
	EndNormal  FaceStats `json:"end_normal"`
	SideNormal FaceStats `json:"side_normal"`
	// EndTangent is of the absolute offset: its sign follows the heading.
	EndTangent FaceStats `json:"end_tangent_abs"`
}

// NearFaceAccounting counts every labelled instant the scorer was asked to
// score and what became of it.
type NearFaceAccounting struct {
	// Masks is the reviewed masks inside the episodes that the policy scores.
	Masks int `json:"masks"`
	// Matched is the masks that had a body within the gate.
	Matched int `json:"matched"`
	// Scored is the matched instants that placed at least one face.
	Scored int `json:"scored"`
	// Unscored counts the reasons, per instant. Tangent reasons are counted
	// separately: the instant's normal residuals still count.
	Unscored map[string]int `json:"unscored"`
	// CentreDistance is the matched pairs' centre-to-footprint-centre
	// distance: a check that arm and pack share a frame.
	CentreDistance FaceStats `json:"centre_distance"`
}

// NearFacePair compares two arms on the instants both scored.
type NearFacePair struct {
	Against string `json:"against"`
	Face    string `json:"face"`
	Both    int    `json:"instants_scored_by_both"`
	// MeanAbs are the two arms' mean absolute residuals on those instants,
	// and Delta is this arm's minus the other's: negative is better.
	MeanAbsOther float64 `json:"mean_abs_other_m"`
	MeanAbsThis  float64 `json:"mean_abs_this_m"`
	Delta        float64 `json:"delta_m"`
	ThisLower    float64 `json:"share_this_lower"`
}

// NearFaceArmResult is one arm's score.
type NearFaceArmResult struct {
	Arm        ArmIdentity        `json:"arm"`
	Accounting NearFaceAccounting `json:"accounting"`
	Strata     []NearFaceStratum  `json:"strata"`
	// Paired compares this arm with the first, on the instants both scored.
	Paired   []NearFacePair    `json:"paired,omitempty"`
	Instants []NearFaceInstant `json:"instants,omitempty"`

	scored map[instantKey]NearFaceInstant
}

type instantKey struct {
	sample int
	object string
}

// NearFaceReport is the comparison of every arm.
type NearFaceReport struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schema_version"`
	// Reference is what the labels were: the pack, the revision, the split.
	Reference NearFaceReference   `json:"reference"`
	Options   NearFaceReportOpts  `json:"options"`
	Arms      []NearFaceArmResult `json:"arms"`
	Caveats   []string            `json:"caveats"`
}

// NearFaceReference identifies the labels the score used.
type NearFaceReference struct {
	PackDigest      string                     `json:"pack_digest"`
	DatasetID       string                     `json:"dataset_id"`
	SidecarRevision int                        `json:"sidecar_revision"`
	SplitDigest     string                     `json:"split_manifest_digest"`
	Split           string                     `json:"split"`
	SplitRole       annotation.SplitRole       `json:"split_role"`
	HeldOut         bool                       `json:"held_out"`
	FrozenDigest    string                     `json:"frozen_split_digest,omitempty"`
	Episodes        []string                   `json:"episodes"`
	Policy          annotation.ReferencePolicy `json:"policy"`
}

// NearFaceReportOpts is the scoring's own choices, recorded.
type NearFaceReportOpts struct {
	FrameToleranceNanos int64   `json:"frame_tolerance_ns"`
	GateSlackMetres     float64 `json:"gate_slack_m"`
	MinReturns          int     `json:"min_returns"`
	FaceQuantile        float64 `json:"face_quantile"`
	FaceBandMetres      float64 `json:"face_band_m"`
	MinSpanCoverage     float64 `json:"min_span_coverage"`
	MaxSpanCoverage     float64 `json:"max_span_coverage"`
	SensorXM            float64 `json:"sensor_x_m"`
	SensorYM            float64 `json:"sensor_y_m"`
}

// nearFaceMask is one scored labelled instant: its returns and where they are.
type nearFaceMask struct {
	sampleID  int
	timestamp int64
	object    string
	x, y      []float64
	// footprint centre and diagonal, for the gate
	cx, cy, diag float64
}

// loadNearFaceMasks binds the pack, split and sidecar as the per-frame
// evaluator does and returns the scored masks of the selected episodes.
func loadNearFaceMasks(opts NearFaceOptions) ([]nearFaceMask, NearFaceReference, error) {
	var ref NearFaceReference
	if opts.Split == "" {
		return nil, ref, fmt.Errorf("no split named: say which partition to score")
	}
	pack, err := annotation.OpenPack(opts.PackDir)
	if err != nil {
		return nil, ref, fmt.Errorf("open pack: %w", err)
	}
	manifest, frozen, err := annotation.LoadAnySplit(opts.SplitManifestPath)
	if err != nil {
		return nil, ref, err
	}
	var sidecar *annotation.Sidecar
	if frozen != nil {
		if manifest, sidecar, err = frozen.Bind(pack); err != nil {
			return nil, ref, err
		}
	} else {
		if manifest.SidecarRevision > 0 {
			sidecar, err = annotation.LoadSidecarRevision(pack, manifest.SidecarRevision)
		} else {
			sidecar, err = annotation.LoadSidecar(pack)
		}
		if err != nil {
			return nil, ref, fmt.Errorf("load annotation: %w", err)
		}
		if err := manifest.ValidateAgainst(pack, sidecar); err != nil {
			return nil, ref, err
		}
	}
	episodes, err := manifest.SelectEpisodes(opts.Split, opts.Episodes, !opts.AllowTuningSplit)
	if err != nil {
		return nil, ref, err
	}
	split, _ := manifest.SplitByName(opts.Split)
	points, _, err := annotation.BuildReference(pack, sidecar, opts.Policy)
	if err != nil {
		return nil, ref, err
	}
	scored := map[instantKey]annotation.ReferencePoint{}
	for _, p := range points {
		if p.Ignore == "" {
			scored[instantKey{p.SampleID, p.ObjectID}] = p
		}
	}

	ref = NearFaceReference{
		PackDigest: pack.Manifest.PackDigest, DatasetID: pack.Manifest.DatasetID, SidecarRevision: sidecar.Revision,
		SplitDigest: manifest.Digest, Split: split.Name, SplitRole: split.Role,
		HeldOut: split.Role == annotation.SplitRoleHeldOut, Policy: opts.Policy,
	}
	if frozen != nil {
		ref.FrozenDigest = frozen.SplitDigest
	}
	var masks []nearFaceMask
	seen := map[instantKey]bool{}
	for _, ep := range episodes {
		ref.Episodes = append(ref.Episodes, ep.EpisodeID)
		inEpisode := map[string]bool{}
		for _, id := range ep.ObjectIDs {
			inEpisode[id] = true
		}
		for _, m := range sidecar.Masks {
			k := instantKey{m.SampleID, m.ObjectID}
			rp, ok := scored[k]
			if !ok || !inEpisode[m.ObjectID] || !ep.ContainsSample(m.SampleID) || seen[k] {
				continue
			}
			seen[k] = true
			pts, err := pack.PointsAt(m.SampleID)
			if err != nil {
				return nil, ref, fmt.Errorf("mask %s sample %d: %w", m.ObjectID, m.SampleID, err)
			}
			nm := nearFaceMask{
				sampleID: m.SampleID, timestamp: pack.Samples[m.SampleID].TimestampNs, object: m.ObjectID,
				cx: rp.X, cy: rp.Y, diag: rp.FootprintDiagonal,
			}
			for _, i := range m.PointIndices {
				nm.x, nm.y = append(nm.x, float64(pts.X[i])), append(nm.y, float64(pts.Y[i]))
			}
			masks = append(masks, nm)
		}
	}
	sort.Slice(masks, func(i, j int) bool {
		if masks[i].sampleID != masks[j].sampleID {
			return masks[i].sampleID < masks[j].sampleID
		}
		return masks[i].object < masks[j].object
	})
	if len(masks) == 0 {
		return nil, ref, fmt.Errorf("the selected episodes hold no scored mask under the %s policy: review the objects first", opts.Policy.Status)
	}
	return masks, ref, nil
}

// ScoreNearFaces scores every arm against the reviewed masks of one split. An
// arm is a solid-body version: only a solid body carries the heading and the
// extents a face is placed from.
func ScoreNearFaces(opts NearFaceOptions, specs []ArmSpec) (*NearFaceReport, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("no arm to score")
	}
	masks, ref, err := loadNearFaceMasks(opts)
	if err != nil {
		return nil, err
	}
	report := &NearFaceReport{
		Schema: NearFaceSchema, SchemaVersion: NearFaceSchemaVersion, Reference: ref,
		Options: NearFaceReportOpts{
			FrameToleranceNanos: opts.FrameToleranceNanos, GateSlackMetres: opts.GateSlackMetres,
			MinReturns: opts.MinReturns, FaceQuantile: opts.FaceQuantile, FaceBandMetres: opts.FaceBandMetres,
			MinSpanCoverage: opts.MinSpanCoverage, MaxSpanCoverage: opts.MaxSpanCoverage,
			SensorXM: opts.SensorXM, SensorYM: opts.SensorYM,
		},
		Caveats: nearFaceCaveats(ref),
	}
	for _, spec := range specs {
		spec.SolidBodies = true
		arm, err := LoadPhysicalArm(spec)
		if err != nil {
			return nil, err
		}
		report.Arms = append(report.Arms, scoreArmNearFaces(masks, arm, opts))
	}
	for i := 1; i < len(report.Arms); i++ {
		report.Arms[i].Paired = pairNearFaces(report.Arms[i], report.Arms[0])
	}
	for i := range report.Arms {
		if opts.IncludeInstants {
			report.Arms[i].Instants = sortedInstants(report.Arms[i].scored)
		}
		report.Arms[i].scored = nil
	}
	return report, nil
}

func nearFaceCaveats(ref NearFaceReference) []string {
	c := []string{
		"The reference is a person's reviewed returns, not a body: they say where the sensor-facing surface is, " +
			"and nothing about the side it cannot see. A face residual is the believed face against that surface.",
		"A positive normal residual means the believed face is farther out toward the sensor than the labelled returns: " +
			"a centre biased toward the sensor, or an extent believed too long. It cannot tell the two apart.",
		"The face is the outermost labelled return (less a small trimmed share), so the sensor's range noise, a few centimetres, puts a floor under every residual.",
		"Every mask is saved as partial by default; a partial mask still certifies that its returns belong to the object, and the extreme of a partly seen face is a lower bound on the face.",
	}
	if !ref.HeldOut {
		c = append(c, fmt.Sprintf("Split %q is role %q, not held out: this is a tuning score and may not be quoted as held out.", ref.Split, ref.SplitRole))
	}
	return c
}

// scoreArmNearFaces places every mask against the arm's bodies, one to one in
// each frame, and scores the faces of each pair.
func scoreArmNearFaces(masks []nearFaceMask, arm PhysicalArm, opts NearFaceOptions) NearFaceArmResult {
	res := NearFaceArmResult{
		Arm:        arm.Identity,
		Accounting: NearFaceAccounting{Masks: len(masks), Unscored: map[string]int{}},
		scored:     map[instantKey]NearFaceInstant{},
	}
	pairs := matchNearFaces(masks, arm.Bodies, opts)
	var distances []float64
	accum := map[string]*stratumAccum{}
	get := func(name string) *stratumAccum {
		if accum[name] == nil {
			accum[name] = &stratumAccum{}
		}
		return accum[name]
	}
	for mi, m := range masks {
		bi, ok := pairs[mi]
		if !ok {
			res.Accounting.Unscored[NearFaceUnmatched]++
			continue
		}
		res.Accounting.Matched++
		body := arm.Bodies[bi]
		cx, cy := bodyCentre(body)
		distances = append(distances, math.Hypot(cx-m.cx, cy-m.cy))
		inst, reason, tangentReasons := scoreBodyFaces(body, m, opts)
		for _, r := range tangentReasons {
			res.Accounting.Unscored[r]++
		}
		if reason != "" {
			res.Accounting.Unscored[reason]++
			continue
		}
		res.Accounting.Scored++
		res.scored[instantKey{m.sampleID, m.object}] = inst
		stratum := nearFaceStratumPrior
		if inst.ExtentEvidence {
			stratum = nearFaceStratumEvidence
		}
		for _, name := range []string{nearFaceStratumAll, stratum, rangeStratum(inst.RangeM)} {
			get(name).add(inst)
		}
	}
	res.Accounting.CentreDistance = summariseResiduals(distances)
	for _, name := range orderedStrata(accum) {
		res.Strata = append(res.Strata, accum[name].stratum(name))
	}
	return res
}

type stratumAccum struct{ end, side, tangent []float64 }

func (a *stratumAccum) add(i NearFaceInstant) {
	if i.EndNormalM != nil {
		a.end = append(a.end, *i.EndNormalM)
	}
	if i.SideNormalM != nil {
		a.side = append(a.side, *i.SideNormalM)
	}
	if i.EndTangentM != nil {
		a.tangent = append(a.tangent, math.Abs(*i.EndTangentM))
	}
}

func (a *stratumAccum) stratum(name string) NearFaceStratum {
	return NearFaceStratum{Name: name, EndNormal: summariseResiduals(a.end), SideNormal: summariseResiduals(a.side), EndTangent: summariseResiduals(a.tangent)}
}

func rangeStratum(r float64) string {
	switch {
	case r < 15:
		return "range_0_15m"
	case r < 30:
		return "range_15_30m"
	case r < 60:
		return "range_30_60m"
	}
	return "range_60m_plus"
}

// orderedStrata lists "all" first, then the extent strata, then ranges.
func orderedStrata(m map[string]*stratumAccum) []string {
	var out []string
	for _, name := range []string{nearFaceStratumAll, nearFaceStratumEvidence, nearFaceStratumPrior,
		"range_0_15m", "range_15_30m", "range_30_60m", "range_60m_plus"} {
		if m[name] != nil {
			out = append(out, name)
		}
	}
	return out
}

func bodyCentre(b PredictedBody) (float64, float64) {
	if b.Physical {
		return b.CentreXM, b.CentreYM
	}
	return b.XM, b.YM
}

// matchNearFaces pairs masks with bodies one to one in each frame, nearest
// first, within the gate: the same rule the per-frame evaluator's matcher and
// the physical scorer use. It returns each paired mask's body index.
func matchNearFaces(masks []nearFaceMask, bodies []PredictedBody, opts NearFaceOptions) map[int]int {
	type candidate struct {
		mask, body int
		dist       float64
	}
	var cands []candidate
	for mi, m := range masks {
		lo := sort.Search(len(bodies), func(i int) bool { return bodies[i].TimestampNs >= m.timestamp-opts.FrameToleranceNanos })
		for bi := lo; bi < len(bodies) && bodies[bi].TimestampNs <= m.timestamp+opts.FrameToleranceNanos; bi++ {
			cx, cy := bodyCentre(bodies[bi])
			d := math.Hypot(cx-m.cx, cy-m.cy)
			if d <= opts.GateSlackMetres+m.diag/2 {
				cands = append(cands, candidate{mi, bi, d})
			}
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		if cands[i].mask != cands[j].mask {
			return cands[i].mask < cands[j].mask
		}
		return cands[i].body < cands[j].body
	})
	out := map[int]int{}
	usedBody := map[int]bool{}
	for _, c := range cands {
		if _, taken := out[c.mask]; taken || usedBody[c.body] {
			continue
		}
		out[c.mask], usedBody[c.body] = c.body, true
	}
	return out
}

// scoreBodyFaces places the visible faces of one body against one mask's
// returns. A non-empty reason means the instant was not scored; the tangent
// reasons are for a scored instant whose tangent residual was withheld.
func scoreBodyFaces(b PredictedBody, m nearFaceMask, opts NearFaceOptions) (NearFaceInstant, string, []string) {
	inst := NearFaceInstant{SampleID: m.sampleID, ObjectID: m.object, TrackKey: b.TrackKey, Returns: len(m.x)}
	switch {
	case !b.Physical:
		return inst, NearFaceNotBodyCentre, nil
	case b.Heading == nil:
		return inst, NearFaceNoOrientation, nil
	case b.Length == nil || b.Width == nil:
		return inst, NearFaceNoExtent, nil
	case len(m.x) < opts.MinReturns:
		return inst, NearFaceTooFewReturns, nil
	}
	cx, cy := bodyCentre(b)
	sinT, cosT := math.Sincos(b.Heading.Rad)
	// Body axes: u along the heading, v to its left.
	toBody := func(x, y float64) (float64, float64) {
		dx, dy := x-cx, y-cy
		return dx*cosT + dy*sinT, -dx*sinT + dy*cosT
	}
	sa, sb := toBody(opts.SensorXM, opts.SensorYM)
	halfL, halfW := b.Length.Metres/2, b.Width.Metres/2
	inst.RangeM = math.Hypot(cx-opts.SensorXM, cy-opts.SensorYM)
	inst.SensorAxialM, inst.SensorLateralM = sa, sb
	inst.ExtentEvidence = b.Length.Evidence && b.Width.Evidence

	endSeen, sideSeen := math.Abs(sa) > halfL, math.Abs(sb) > halfW
	if !endSeen && !sideSeen {
		return inst, NearFaceNoVisibleFace, nil
	}
	as, bs := make([]float64, len(m.x)), make([]float64, len(m.x))
	for i := range m.x {
		as[i], bs[i] = toBody(m.x[i], m.y[i])
	}
	var tangentReasons []string
	if endSeen {
		n := math.Copysign(1, sa)
		extreme := outwardExtreme(as, n, opts.FaceQuantile)
		r := halfL - n*extreme
		inst.EndNormalM = &r
		t, reason := endFaceTangent(as, bs, n, extreme, b.Width.Metres, opts)
		if reason != "" {
			tangentReasons = append(tangentReasons, reason)
		} else {
			inst.EndTangentM = &t
		}
	}
	if sideSeen {
		n := math.Copysign(1, sb)
		r := halfW - n*outwardExtreme(bs, n, opts.FaceQuantile)
		inst.SideNormalM = &r
	}
	return inst, "", tangentReasons
}

// outwardExtreme is the coordinate, along an axis, of the face the returns
// reach on the side n points to (+1 for the high end), with the outermost
// floor(quantile x N) returns ignored so a stray one does not move the face.
// The count is whole: below fifty returns at a 2 % quantile nothing is ignored,
// and the extreme is the outermost return itself.
func outwardExtreme(v []float64, n, quantile float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	k := int(quantile * float64(len(s)))
	if n > 0 {
		return s[len(s)-1-k]
	}
	return s[k]
}

// endFaceTangent is the offset of the believed centre from the midpoint of the
// end face's lateral span, taken over the returns within the face band of the
// extreme. The reason is non-empty when the span cannot stand for the face.
func endFaceTangent(as, bs []float64, n, extreme, width float64, opts NearFaceOptions) (float64, string) {
	var lat []float64
	for i, a := range as {
		if n*(extreme-a) >= -1e-9 && n*(extreme-a) <= opts.FaceBandMetres {
			lat = append(lat, bs[i])
		}
	}
	if len(lat) < opts.MinReturns {
		return 0, nearFaceTangentTooFew
	}
	lo, hi := outwardExtreme(lat, -1, opts.FaceQuantile), outwardExtreme(lat, 1, opts.FaceQuantile)
	coverage := (hi - lo) / width
	switch {
	case coverage < opts.MinSpanCoverage:
		return 0, nearFaceTangentPartial
	case coverage > opts.MaxSpanCoverage:
		return 0, nearFaceTangentWide
	}
	return -(lo + hi) / 2, ""
}

func sortedInstants(m map[instantKey]NearFaceInstant) []NearFaceInstant {
	out := make([]NearFaceInstant, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SampleID != out[j].SampleID {
			return out[i].SampleID < out[j].SampleID
		}
		return out[i].ObjectID < out[j].ObjectID
	})
	return out
}

// pairNearFaces compares an arm with the first arm on the instants both scored.
func pairNearFaces(this, other NearFaceArmResult) []NearFacePair {
	var out []NearFacePair
	for _, face := range []struct {
		name string
		get  func(NearFaceInstant) *float64
	}{
		{"end_normal", func(i NearFaceInstant) *float64 { return i.EndNormalM }},
		{"side_normal", func(i NearFaceInstant) *float64 { return i.SideNormalM }},
	} {
		p := NearFacePair{Against: other.Arm.Label, Face: face.name}
		var sumThis, sumOther float64
		var lower int
		for k, a := range this.scored {
			b, ok := other.scored[k]
			ra, rb := face.get(a), face.get(b)
			if !ok || ra == nil || rb == nil {
				continue
			}
			p.Both++
			sumThis += math.Abs(*ra)
			sumOther += math.Abs(*rb)
			if math.Abs(*ra) < math.Abs(*rb) {
				lower++
			}
		}
		if p.Both > 0 {
			n := float64(p.Both)
			p.MeanAbsThis, p.MeanAbsOther = sumThis/n, sumOther/n
			p.Delta = p.MeanAbsThis - p.MeanAbsOther
			p.ThisLower = float64(lower) / n
		}
		out = append(out, p)
	}
	return out
}
