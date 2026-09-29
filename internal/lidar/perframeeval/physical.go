package perframeeval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// Physical-reference scoring.
//
// The mask-position policy (annotation.BuildReference, footprint centre or
// point mean) stays what it is: the place a visible mask puts an object, used
// to compare identities. It is not a body centre and nothing here scores it
// as one. This path scores one estimate version against the pack's physical
// references: body centre, yaw, dimensions, bumpers, box overlap and
// following gap, each only where a reviewed, independent reference and its
// bound exist, with the reference's bound reported beside the error.
//
// Matching is by source (the reference's sensor and calibration against every
// prediction row's), by capture instant (a prediction moves onto the pack
// sample within the frame tolerance), by reference frame (the pack's, which
// the reference was validated against), and by object: at each sample, truth
// keyframes and predictions are paired one to one, nearest first, within a
// gate. Every object of every episode is expected at every sample where it
// has a mask or a keyframe, and each expected instant is either scored or
// counted, component by component, as unknown geometry, unmatched or missing
// prediction, with a reason. A keyframe covers its own sample; the frames
// between keyframes are counted, not filled in.

// PhysicalScoreSchema names the physical score document.
const PhysicalScoreSchema = "velocity.report/physical-reference-score"

// PhysicalScoreSchemaVersion is its layout version.
const PhysicalScoreSchemaVersion = 1

// PhysicalMethodID versions the matching, derivation and accounting rules.
const PhysicalMethodID = "physical_reference_v1"

// DefaultPhysicalGateMetres is the default object gate: how far a
// prediction's point may be from the reference's centre, or its anchor where
// the centre is unknown, and still correspond to it. A medoid sits towards the
// faces the sensor saw, so a long vehicle may need more.
const DefaultPhysicalGateMetres = 3.0

// ErrPhysicalHeldOut refuses physical scoring of a held-out split. Its error
// limits, reference precision and coverage must be pinned on tuning data
// first, and nothing yet records them.
var ErrPhysicalHeldOut = errors.New("physical scoring of a held-out split is refused")

// PhysicalComponent is one scored quantity.
type PhysicalComponent string

const (
	ComponentCentre PhysicalComponent = "centre"
	ComponentYaw    PhysicalComponent = "yaw"
	ComponentLength PhysicalComponent = "length"
	ComponentWidth  PhysicalComponent = "width"
	ComponentHeight PhysicalComponent = "height"
	ComponentFront  PhysicalComponent = "front"
	ComponentRear   PhysicalComponent = "rear"
	// ComponentEnds compares both bumpers without saying which is which,
	// which is all an ambiguous axis allows.
	ComponentEnds PhysicalComponent = "ends_unsigned"
	ComponentBox  PhysicalComponent = "box"
)

// PhysicalComponents lists every per-instant component in report order.
func PhysicalComponents() []PhysicalComponent {
	return []PhysicalComponent{ComponentCentre, ComponentYaw, ComponentLength, ComponentWidth, ComponentHeight,
		ComponentFront, ComponentRear, ComponentEnds, ComponentBox}
}

// Outcome categories. Every expected instant lands in exactly one per
// component.
const (
	OutcomeScored            = "scored"
	OutcomeUnknownGeometry   = "unknown_geometry"
	OutcomeUnmatched         = "unmatched"
	OutcomeMissingPrediction = "missing_prediction"
	// OutcomeNotFollowing is a reviewed no_leader or ambiguous decision:
	// an answer, with no gap to score.
	OutcomeNotFollowing = "not_following"
)

// Reasons beyond the reference layer's own (annotation.Unavailable*).
const (
	ReasonNoKeyframe               = "no_keyframe"
	ReasonReferenceUnreviewed      = "reference_unreviewed"
	ReasonReferenceTrackerAssisted = "reference_tracker_assisted"
	ReasonNoReferencePosition      = "no_reference_position"
	ReasonNoPredictionAtInstant    = "no_prediction_at_instant"
	ReasonNoPredictionWithinGate   = "no_prediction_within_gate"
	ReasonPredictionTaken          = "prediction_matched_to_another_object"
	ReasonPredictionNotOnBody      = "prediction_reference_not_physical"
	ReasonPredictionNoHeading      = "prediction_heading_absent"
	ReasonPredictionAxisAmbiguous  = "prediction_axis_ambiguous"
	ReasonPredictionNoExtent       = "prediction_extent_absent"
	ReasonPredictionIncompleteBox  = "prediction_box_incomplete"
	ReasonNoGapReference           = "no_gap_reference"
	ReasonFollowerUnmatched        = "follower_unmatched"
	ReasonLeaderUnmatched          = "leader_unmatched"
	ReasonFollowerAxisUnavailable  = "follower_axis_unavailable"
	ReasonLeaderOutsideEpisode     = "leader_outside_episode"
	ReasonNoLeaderKeyframe         = "no_leader_keyframe"
	ReasonPredictionInstantsDiffer = "prediction_instants_differ"
)

// Outcome is one component's fate at one instant.
type Outcome struct {
	Category string `json:"category"`
	Reason   string `json:"reason,omitempty"`
}

// PhysicalOptions are the physical path's choices, recorded with its result.
type PhysicalOptions struct {
	// Revision pins the physical reference revision; zero scores the
	// current one. The revision and its content digest are recorded either
	// way.
	Revision            int
	GateMetres          float64
	FrameToleranceNanos int64
}

// DefaultPhysicalOptions are the current revision, the default gate and the
// per-frame harness's frame tolerance.
func DefaultPhysicalOptions() PhysicalOptions {
	return PhysicalOptions{GateMetres: DefaultPhysicalGateMetres, FrameToleranceNanos: DefaultScoreOptions().FrameToleranceNanos}
}

// Validate refuses options that cannot be applied.
func (o PhysicalOptions) Validate() error {
	if o.Revision < 0 {
		return fmt.Errorf("physical reference revision %d is negative", o.Revision)
	}
	if !(o.GateMetres > 0) || math.IsInf(o.GateMetres, 0) {
		return fmt.Errorf("physical gate %v m must be positive and finite", o.GateMetres)
	}
	if o.FrameToleranceNanos < 0 {
		return fmt.Errorf("frame tolerance %d ns is negative", o.FrameToleranceNanos)
	}
	return nil
}

// PhysicalReferenceIdentity is everything a physical score was computed
// against. Equal digests mean the same references at the same instants under
// the same rules.
type PhysicalReferenceIdentity struct {
	Method                 string               `json:"method"`
	PackDigest             string               `json:"pack_digest"`
	DatasetID              string               `json:"dataset_id"`
	SidecarRevision        int                  `json:"sidecar_revision"`
	PhysicalRevision       int                  `json:"physical_revision"`
	PhysicalRevisionDigest string               `json:"physical_revision_digest"`
	PhysicalContentDigest  string               `json:"physical_content_digest"`
	SplitManifestDigest    string               `json:"split_manifest_digest"`
	Split                  string               `json:"split"`
	SplitRole              annotation.SplitRole `json:"split_role"`
	Episodes               []string             `json:"episodes"`
	GateMetres             float64              `json:"gate_m"`
	FrameToleranceNanos    int64                `json:"frame_tolerance_ns"`
	GapDefinition          string               `json:"gap_definition"`
	ExpectedInstants       int                  `json:"expected_instants"`
	Digest                 string               `json:"digest"`
}

// expectedInstant is one object that should be accounted for at one sample.
type expectedInstant struct {
	episode string
	object  string
	sample  int
}

// expectedFollowing is one follower at one sample of an episode, with the
// following reference that speaks for it there.
type expectedFollowing struct {
	episode string
	ref     annotation.FollowingReference
	sample  int
	// leaderInEpisode says the episode scores the leader too; a leader it
	// does not score, perhaps one in another split, is not used.
	leaderInEpisode bool
}

// PhysicalReference is a split's physical truth: the reference layer every
// arm is scored against.
type PhysicalReference struct {
	Identity  PhysicalReferenceIdentity
	Source    annotation.PhysicalSource
	samples   []annotation.Sample
	geometry  map[string]map[int]annotation.PhysicalGeometry
	bodies    map[string]annotation.BodyGeometry
	instants  []expectedInstant
	following []expectedFollowing
	// otherSplits are objects the manifest puts in another split. They take
	// no part in matching, so another split's references never change this
	// one's outcomes.
	otherSplits map[string]bool
}

// LoadPhysicalReference opens the pack, its split manifest and annotation
// revision as LoadReference does, then the physical references, and lists
// every instant the split's episodes expect. A held-out split is refused.
func LoadPhysicalReference(ref ReferenceOptions, opts PhysicalOptions) (*PhysicalReference, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if ref.Split == "" {
		return nil, fmt.Errorf("no split named: say which partition to score")
	}
	pack, err := annotation.OpenPack(ref.PackDir)
	if err != nil {
		return nil, fmt.Errorf("open pack: %w", err)
	}
	manifest, err := annotation.LoadSplitManifest(ref.SplitManifestPath)
	if err != nil {
		return nil, err
	}
	var sidecar *annotation.Sidecar
	if manifest.SidecarRevision > 0 {
		sidecar, err = annotation.LoadSidecarRevision(pack, manifest.SidecarRevision)
	} else {
		sidecar, err = annotation.LoadSidecar(pack)
	}
	if err != nil {
		return nil, fmt.Errorf("load annotation: %w", err)
	}
	if err := manifest.ValidateAgainst(pack, sidecar); err != nil {
		return nil, err
	}
	episodes, err := manifest.SelectEpisodes(ref.Split, ref.Episodes, !ref.AllowTuningSplit)
	if err != nil {
		return nil, err
	}
	split, _ := manifest.SplitByName(ref.Split)
	if split.Role == annotation.SplitRoleHeldOut {
		return nil, fmt.Errorf("%w: split %q is held out, and its physical error limits, reference precision and "+
			"coverage have not been pinned on tuning data (docs/plans/lidar-physical-reference-review-plan.md); "+
			"score a tuning split with -allow-tuning-split", ErrPhysicalHeldOut, split.Name)
	}

	var doc *annotation.PhysicalReferenceSet
	if opts.Revision > 0 {
		doc, err = annotation.LoadPhysicalReferenceRevision(pack, opts.Revision)
	} else {
		doc, err = annotation.LoadPhysicalReferences(pack)
	}
	if err != nil {
		return nil, fmt.Errorf("load physical references: %w", err)
	}
	if doc.Digest() == "" {
		return nil, fmt.Errorf("pack %s has no physical references: author or import them first", pack.Manifest.PackDigest)
	}
	if err := doc.ValidateLinks(pack, sidecar); err != nil {
		return nil, fmt.Errorf("physical references revision %d against annotation revision %d: %w", doc.Revision, sidecar.Revision, err)
	}
	content, err := doc.ContentDigest()
	if err != nil {
		return nil, err
	}

	pr := &PhysicalReference{Source: doc.Source, samples: pack.Samples, geometry: doc.Geometries(),
		bodies: map[string]annotation.BodyGeometry{}, otherSplits: map[string]bool{}}
	for _, sp := range manifest.Splits {
		if sp.Name != split.Name {
			for _, obj := range sp.ObjectIDs {
				pr.otherSplits[obj] = true
			}
		}
	}
	for _, o := range doc.Objects {
		if o.Body != nil {
			pr.bodies[o.ObjectID] = *o.Body
		}
	}
	masked := map[string]map[int]bool{}
	for _, m := range sidecar.Masks {
		if m.Status == annotation.StatusRejected {
			continue
		}
		if masked[m.ObjectID] == nil {
			masked[m.ObjectID] = map[int]bool{}
		}
		masked[m.ObjectID][m.SampleID] = true
	}
	var ids []string
	for _, ep := range episodes {
		ids = append(ids, ep.EpisodeID)
		scored := map[string]bool{}
		for _, obj := range ep.ObjectIDs {
			scored[obj] = true
			for _, iv := range ep.FrameIntervals {
				for s := iv.FirstSample; s <= iv.LastSample; s++ {
					if _, keyed := pr.geometry[obj][s]; keyed || masked[obj][s] {
						pr.instants = append(pr.instants, expectedInstant{episode: ep.EpisodeID, object: obj, sample: s})
					}
				}
			}
		}
		pr.following = append(pr.following, expectedFollowings(ep, scored, doc.Following)...)
	}
	pr.Identity = PhysicalReferenceIdentity{
		Method: PhysicalMethodID, PackDigest: pack.Manifest.PackDigest, DatasetID: pack.Manifest.DatasetID,
		SidecarRevision: sidecar.Revision, PhysicalRevision: doc.Revision, PhysicalRevisionDigest: doc.Digest(),
		PhysicalContentDigest: content, SplitManifestDigest: manifest.Digest, Split: split.Name, SplitRole: split.Role,
		Episodes: ids, GateMetres: opts.GateMetres, FrameToleranceNanos: opts.FrameToleranceNanos,
		GapDefinition: annotation.GapAlongFollowerAxis, ExpectedInstants: len(pr.instants),
	}
	b, err := json.Marshal(struct {
		Identity  PhysicalReferenceIdentity `json:"identity"`
		Instants  [][3]string               `json:"instants"`
		Following [][3]string               `json:"following"`
	}{pr.Identity, pr.instantKeys(), pr.followingKeys()})
	if err != nil {
		return nil, fmt.Errorf("encode physical reference for digest: %w", err)
	}
	sum := sha256.Sum256(b)
	pr.Identity.Digest = "sha256:" + hex.EncodeToString(sum[:])
	return pr, nil
}

// expectedFollowings is one expected item per follower and sample of an
// episode, however many following records cover it. Where several do, the
// one that speaks for the instant is a reviewed, independent record with a
// gap there, then one without, then any other, in document order; the
// validation already refuses reviewed records that disagree.
func expectedFollowings(ep annotation.Episode, scored map[string]bool, refs []annotation.FollowingReference) []expectedFollowing {
	type key struct {
		follower string
		sample   int
	}
	rank := func(f annotation.FollowingReference, s int) int {
		if f.Review.Status != annotation.StatusReviewed || f.Review.Origin != annotation.OriginIndependent {
			return 0
		}
		for _, g := range f.Gaps {
			if g.SampleID == s {
				return 2
			}
		}
		return 1
	}
	chosen := map[key]annotation.FollowingReference{}
	var keys []key
	for _, f := range refs {
		if !scored[f.FollowerObjectID] {
			continue
		}
		for s := f.Interval.FirstSample; s <= f.Interval.LastSample; s++ {
			if !ep.ContainsSample(s) {
				continue
			}
			k := key{f.FollowerObjectID, s}
			prev, seen := chosen[k]
			if !seen {
				keys = append(keys, k)
			}
			if !seen || rank(f, s) > rank(prev, s) {
				chosen[k] = f
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].follower != keys[j].follower {
			return keys[i].follower < keys[j].follower
		}
		return keys[i].sample < keys[j].sample
	})
	out := make([]expectedFollowing, len(keys))
	for i, k := range keys {
		f := chosen[k]
		out[i] = expectedFollowing{episode: ep.EpisodeID, ref: f, sample: k.sample, leaderInEpisode: scored[f.LeaderObjectID]}
	}
	return out
}

func (pr *PhysicalReference) instantKeys() [][3]string {
	out := make([][3]string, len(pr.instants))
	for i, e := range pr.instants {
		out[i] = [3]string{e.episode, e.object, fmt.Sprint(e.sample)}
	}
	return out
}

func (pr *PhysicalReference) followingKeys() [][3]string {
	out := make([][3]string, len(pr.following))
	for i, e := range pr.following {
		out[i] = [3]string{e.episode, e.ref.FollowingID, fmt.Sprint(e.sample)}
	}
	return out
}

// PhysicalMatch is the prediction an object's keyframe was paired with.
type PhysicalMatch struct {
	TrackKey  string  `json:"track_key"`
	DistanceM float64 `json:"distance_m"`
	// OffsetNanos is the prediction's time minus the sample's.
	OffsetNanos int64 `json:"offset_ns"`
	// Candidates counts predictions within the gate, taken or not, so an
	// operator can see a crowded match.
	Candidates int `json:"candidates"`
}

// alignedPrediction is a prediction moved onto a pack sample.
type alignedPrediction struct {
	body   PredictedBody
	offset int64
}

// sampleMatches is one sample's predictions and its object pairings.
type sampleMatches struct {
	predictions []alignedPrediction
	matched     map[string]int // object -> index into predictions
	candidates  map[string]int
}

// checkSource holds every prediction row to the reference's sensor and to
// one calibration: the reference's, when it names one.
func checkSource(src annotation.PhysicalSource, arm PhysicalArm) error {
	for _, s := range arm.SensorIDs {
		if s != src.SensorID {
			return fmt.Errorf("arm %s is observed by sensor %q; the physical references are for sensor %q",
				arm.Identity.Label, s, src.SensorID)
		}
	}
	if len(arm.CalibrationIDs) > 1 {
		return fmt.Errorf("arm %s mixes calibrations %q: score one calibration at a time",
			arm.Identity.Label, arm.CalibrationIDs)
	}
	if src.CalibrationID == "" {
		return nil
	}
	for _, c := range arm.CalibrationIDs {
		if c != src.CalibrationID {
			return fmt.Errorf("arm %s uses calibration %q; the physical references were measured under %q",
				arm.Identity.Label, c, src.CalibrationID)
		}
	}
	return nil
}

// alignPredictions moves each prediction onto the nearest sample within the
// tolerance. A second prediction of one track on one sample keeps the nearer.
func (pr *PhysicalReference) alignPredictions(bodies []PredictedBody) map[int][]alignedPrediction {
	times := make([]int64, len(pr.samples))
	for i, s := range pr.samples {
		times[i] = s.TimestampNs
	}
	type key struct {
		sample int
		track  string
	}
	best := map[key]alignedPrediction{}
	for _, b := range bodies {
		i := sort.Search(len(times), func(i int) bool { return times[i] >= b.TimestampNs })
		sample := -1
		for _, j := range []int{i - 1, i} {
			if j >= 0 && j < len(times) && abs64(b.TimestampNs-times[j]) <= pr.Identity.FrameToleranceNanos &&
				(sample < 0 || abs64(b.TimestampNs-times[j]) < abs64(b.TimestampNs-times[sample])) {
				sample = j
			}
		}
		if sample < 0 {
			continue
		}
		k := key{sample, b.TrackKey}
		offset := b.TimestampNs - times[sample]
		if prev, ok := best[k]; ok && abs64(prev.offset) <= abs64(offset) {
			continue
		}
		best[k] = alignedPrediction{body: b, offset: offset}
	}
	out := map[int][]alignedPrediction{}
	for k, a := range best {
		out[k.sample] = append(out[k.sample], a)
	}
	for s := range out {
		sort.Slice(out[s], func(i, j int) bool { return out[s][i].body.TrackKey < out[s][j].body.TrackKey })
	}
	return out
}

// referencePosition is where an object's keyframe puts it for matching: its
// centre, or its anchor where the centre is unknown.
func referencePosition(g annotation.PhysicalGeometry) (x, y float64, ok bool) {
	switch {
	case g.Centre != nil:
		return g.Centre.XM, g.Centre.YM, true
	case g.AnchorPoint != nil:
		return g.AnchorPoint.XM, g.AnchorPoint.YM, true
	}
	return 0, 0, false
}

// predictionPosition is the prediction's centre when it names one, and its
// own point otherwise.
func predictionPosition(b PredictedBody) (x, y float64) {
	if b.Physical {
		return b.CentreXM, b.CentreYM
	}
	return b.XM, b.YM
}

// matchSample pairs every truth keyframe at a sample with a prediction, one
// to one, nearest pair first, within the gate. Every object with a truth
// keyframe takes part, scored by this episode or not, so a prediction of a
// neighbouring object is not handed to the one being scored; except objects
// in another split, whose references must not change this split's outcomes.
// A prediction of such an object, like one of an untracked neighbour, may
// then be matched to a scored object inside the gate.
func (pr *PhysicalReference) matchSample(sample int, predictions []alignedPrediction) sampleMatches {
	m := sampleMatches{predictions: predictions, matched: map[string]int{}, candidates: map[string]int{}}
	type pair struct {
		object string
		pred   int
		d      float64
	}
	var pairs []pair
	objects := make([]string, 0, len(pr.geometry))
	for obj := range pr.geometry {
		objects = append(objects, obj)
	}
	sort.Strings(objects)
	for _, obj := range objects {
		g, ok := pr.geometry[obj][sample]
		if !ok || !g.Truth || pr.otherSplits[obj] {
			continue
		}
		rx, ry, ok := referencePosition(g)
		if !ok {
			continue
		}
		for i, p := range predictions {
			px, py := predictionPosition(p.body)
			if d := math.Hypot(px-rx, py-ry); d <= pr.Identity.GateMetres {
				pairs = append(pairs, pair{obj, i, d})
				m.candidates[obj]++
			}
		}
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].d < pairs[j].d })
	taken := map[int]bool{}
	for _, p := range pairs {
		if _, done := m.matched[p.object]; done || taken[p.pred] {
			continue
		}
		m.matched[p.object], taken[p.pred] = p.pred, true
	}
	return m
}
