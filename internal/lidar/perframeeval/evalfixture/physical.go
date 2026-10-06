package evalfixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// The physical fixture: a straight pass of two cars whose physical truth is
// known exactly, for tests of physical-reference scoring.
//
// Ten samples 100 ms apart. Both cars drive along +x at ten metres a second,
// heading 0, on y = 0:
//
//	obj_follow  4.6 by 1.8 m, centre x = 10+i. Its front face returns at
//	            samples 0-2, all of it at 4-5, its rear face at 6-9. At
//	            sample 3 it is fully occluded. At sample 0 its mask also
//	            holds a stray return 5 m behind it: a contaminated mask.
//	obj_lead    4.4 by 1.8 m, centre x = 20+i, fully returned throughout.
//
// The bumper gap is therefore 5.5 m at every sample. Both objects are in one
// tuning split, scored by one episode over every sample.
//
// Physical references: the follower has keyframes at 0 (body centre), 2
// (front face with its offset), 3 (occluded, inferred), 5 (unresolved axis),
// 7 (rear face without an offset) and 8 (a tracker-assisted proposal); its
// height is a partial span. The leader has keyframes at 0, 2 and 5, and a
// full height of 1.4 to 1.6 m (its returns span 1.2 m, within the slack). The
// follower follows the leader over every sample, with gaps at 0 and 2 and an
// unknown gap at 5, where the follower's front cannot be named; the leader
// has no leader.
//
// Estimate versions, all solid bodies at stage online in one source:
//
//	params/exact              every truth exactly; the follower is missing
//	                          at sample 3
//	params/face-bias          the follower's centre 0.3 m towards whichever
//	                          face is visible, and a new track from sample 6
//	params/medoid             the cluster medoid, 1 m behind each centre
//	params/other-calibration  params/exact under another calibration
//
// and params/exact again as final point estimates from the visible box
// centre, which have no body.
const (
	PhysSamples     = 10
	PhysSensor      = "fixture-sensor"
	PhysCalibration = "calibration/v1/fixture"
	PhysSource      = "source/v1/physical-fixture"

	Follower = "obj_follow"
	Leader   = "obj_lead"

	PhysSplit   = "tune-physical"
	PhysEpisode = "tune-physical-ep1"

	OccludedFollowerSample = 3
	ContaminatedSample     = 0

	ParamsExact            = "params/exact"
	ParamsFaceBias         = "params/face-bias"
	ParamsMedoid           = "params/medoid"
	ParamsOtherCalibration = "params/other-calibration"
	OtherCalibration       = "calibration/v2/other"

	FollowerLength = 4.6
	LeaderLength   = 4.4
	CarWidth       = 1.8
	FaceBiasM      = 0.3
	MedoidOffsetM  = 1.0
	TrueGapM       = 5.5
)

// FollowerX and LeaderX are the true body-centre x at a sample.
func FollowerX(i int) float64 { return 10 + float64(i) }
func LeaderX(i int) float64   { return 20 + float64(i) }

// PhysicalFixture is where the files were written.
type PhysicalFixture struct {
	PackDir           string
	SplitManifestPath string
	DBPath            string
	PackDigest        string
	DatasetID         string
}

// WritePhysical creates the pack, sidecar, physical references, split
// manifest and database under dir.
func WritePhysical(dir string) (*PhysicalFixture, error) {
	f := &PhysicalFixture{
		PackDir:           filepath.Join(dir, "pack"),
		SplitManifestPath: filepath.Join(dir, "split.json"),
		DBPath:            filepath.Join(dir, "evidence.db"),
	}
	pack, err := writePhysicalPack(f.PackDir)
	if err != nil {
		return nil, err
	}
	f.PackDigest, f.DatasetID = pack.Manifest.PackDigest, pack.Manifest.DatasetID
	refs := PhysicalReferences(pack)
	if err := annotation.SavePhysicalReferences(pack, refs); err != nil {
		return nil, err
	}
	if err := WriteSplitManifest(f.SplitManifestPath, f.Manifest()); err != nil {
		return nil, err
	}
	if err := writePhysicalDB(f.DBPath); err != nil {
		return nil, err
	}
	return f, nil
}

// Manifest is the fixture's split manifest: one tuning split and episode.
func (f *PhysicalFixture) Manifest() annotation.SplitManifest {
	return annotation.SplitManifest{
		Schema: annotation.SplitSchema, SchemaVersion: annotation.SplitSchemaVersion,
		PackDigest: f.PackDigest, DatasetID: f.DatasetID, SidecarRevision: 1,
		Splits: []annotation.Split{{Name: PhysSplit, Role: annotation.SplitRoleTuning, ObjectIDs: []string{Follower, Leader}}},
		Episodes: []annotation.Episode{{EpisodeID: PhysEpisode, Split: PhysSplit, ObjectIDs: []string{Follower, Leader},
			FrameIntervals: []annotation.FrameInterval{{FirstSample: 0, LastSample: PhysSamples - 1}}}},
	}
}

// FrozenDraft is the fixture's split as a draft that can be frozen: the
// same partition and episode as Manifest, pinned to annotation revision 1.
func (f *PhysicalFixture) FrozenDraft() annotation.SplitDraft {
	m := f.Manifest()
	return annotation.SplitDraft{
		Schema: annotation.SplitDraftSchema, SchemaVersion: annotation.SplitDraftSchemaVersion,
		Packs: []annotation.DraftPack{{Dir: f.PackDir, SidecarRevision: m.SidecarRevision, Splits: m.Splits, Episodes: m.Episodes}},
	}
}

// FreezeOptions are the options WriteFrozen freezes FrozenDraft under: a
// fixed time and build, so that the split's digest is reproducible.
func (f *PhysicalFixture) FreezeOptions() annotation.FreezeOptions {
	draft := f.FrozenDraft()
	return annotation.FreezeOptions{
		Draft: &draft, Author: "fixture", Now: time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC),
		BuildVersion: "fixture", BuildGitSHA: "fixture", GuardSeconds: annotation.DefaultSplitGuardSeconds,
	}
}

// WriteFrozen freezes FrozenDraft under FreezeOptions and writes it to path.
// The pack has physical references, so the split pins their head revision.
func (f *PhysicalFixture) WriteFrozen(path string) (*annotation.FrozenSplit, error) {
	frozen, err := annotation.FreezeSplit(f.FreezeOptions())
	if err != nil {
		return nil, err
	}
	if err := annotation.WriteFrozenSplit(path, frozen); err != nil {
		return nil, err
	}
	return frozen, nil
}

// WriteFrozenMembershipOnly writes FrozenDraft frozen as a version 2 split,
// the layout frozen before physical pins existed: the same split with no
// pin and the version 2 number, digested as that layout was, since the
// encoding differs from version 3 only by the pin. It is what a split
// frozen by an earlier build looks like, for tests of the legacy reader.
func (f *PhysicalFixture) WriteFrozenMembershipOnly(path string) (*annotation.FrozenSplit, error) {
	frozen, err := annotation.FreezeSplit(f.FreezeOptions())
	if err != nil {
		return nil, err
	}
	frozen.SchemaVersion = annotation.FrozenSplitSchemaVersionMembershipOnly
	for i := range frozen.Packs {
		frozen.Packs[i].Physical = nil
	}
	frozen.SplitDigest = ""
	b, err := json.Marshal(frozen)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	frozen.SplitDigest = "sha256:" + hex.EncodeToString(sum[:])
	if err := annotation.WriteFrozenSplit(path, frozen); err != nil {
		return nil, err
	}
	return frozen, nil
}

// boxReturns outlines x in [cx+from, cx+to], y = ±0.9, z = 0.3 and 1.5.
func boxReturns(p *annotation.Points, cx, from, to float64) []int {
	var idx []int
	for _, x := range []float64{cx + from, cx + to} {
		for _, y := range []float64{-CarWidth / 2, CarWidth / 2} {
			for _, z := range []float32{0.3, 1.5} {
				idx = append(idx, len(p.X))
				p.X, p.Y, p.Z = append(p.X, float32(x)), append(p.Y, float32(y)), append(p.Z, z)
			}
		}
	}
	return idx
}

func writePhysicalPack(dir string) (*annotation.Pack, error) {
	var samples []annotation.Sample
	var blocks [][]byte
	followerMasks, leaderMasks := make([][]int, PhysSamples), make([][]int, PhysSamples)
	for i := 0; i < PhysSamples; i++ {
		var p annotation.Points
		half := FollowerLength / 2
		switch {
		case i <= 2:
			followerMasks[i] = boxReturns(&p, FollowerX(i), 0, half)
		case i == OccludedFollowerSample:
		case i <= 5:
			followerMasks[i] = boxReturns(&p, FollowerX(i), -half, half)
		default:
			followerMasks[i] = boxReturns(&p, FollowerX(i), -half, 0)
		}
		leaderMasks[i] = boxReturns(&p, LeaderX(i), -LeaderLength/2, LeaderLength/2)
		if i == ContaminatedSample {
			followerMasks[i] = append(followerMasks[i], len(p.X))
			p.X, p.Y, p.Z = append(p.X, float32(FollowerX(i)-5)), append(p.Y, 0), append(p.Z, 0.3)
		}
		block, err := annotation.EncodePoints(p)
		if err != nil {
			return nil, err
		}
		samples = append(samples, annotation.Sample{
			SourceOrdinal: i, SourceFrameID: uint64(i), TimestampNs: SampleTime(i), SensorID: PhysSensor, PointCount: len(p.X),
		})
		blocks = append(blocks, block)
	}
	m := annotation.Manifest{
		Coverage: annotation.CoverageForegroundOnly,
		Source:   annotation.SourceProvenance{SensorID: PhysSensor, VRLOGHeaderSHA: "sha256:fixture-header", VRLOGFramesSHA: "sha256:fixture-frames"},
		Coordinate: annotation.CoordinateContract{Units: "metres", FrameID: "sensor", ReferenceFrame: "sensor",
			Handedness: "right", OriginNote: "fixture"},
	}
	if err := annotation.WritePack(dir, m, samples, blocks); err != nil {
		return nil, err
	}
	pack, err := annotation.OpenPack(dir)
	if err != nil {
		return nil, err
	}
	s := annotation.NewSidecar(pack)
	s.Change = annotation.Provenance{Author: "fixture", Operation: "fixture"}
	for _, id := range []string{Follower, Leader} {
		s.Objects = append(s.Objects, annotation.Object{ObjectID: id, Class: "car", Confidence: 1, Status: annotation.StatusReviewed})
	}
	// Examined and found to be nothing: in no split, and never expected.
	s.Objects = append(s.Objects, annotation.Object{ObjectID: "obj_rejected", Class: "car", Status: annotation.StatusRejected})
	s.Masks = append(s.Masks, annotation.FrameMask{ObjectID: "obj_rejected", SampleID: 1, PointIndices: []int{},
		Completeness: annotation.MaskPartial, Visibility: annotation.VisibleUnknown, Status: annotation.StatusRejected})
	for i := 0; i < PhysSamples; i++ {
		follow := annotation.FrameMask{ObjectID: Follower, SampleID: i, PointIndices: followerMasks[i],
			Completeness: annotation.MaskPartial, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed}
		if i == OccludedFollowerSample {
			follow.PointIndices, follow.Visibility = []int{}, annotation.VisibleFullyOccluded
		}
		s.Masks = append(s.Masks, follow, annotation.FrameMask{ObjectID: Leader, SampleID: i, PointIndices: leaderMasks[i],
			Completeness: annotation.MaskComplete, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed})
	}
	if err := annotation.SaveSidecar(pack, s); err != nil {
		return nil, err
	}
	return pack, nil
}

func f64(v float64) *float64 { return &v }

func physicalReview(origin annotation.ReferenceOrigin, status annotation.ReviewStatus) annotation.PhysicalReview {
	r := annotation.PhysicalReview{
		Status: status, Origin: origin, Method: "manual_box",
		UncertaintyAssumptions: "hard bounds read off the outline", Provenance: annotation.Provenance{Author: "fixture"},
	}
	if origin == annotation.OriginTrackerAssisted {
		r.Method, r.TrackerSource = "tracker_copy", "lidar_track_solid_bodies params/exact seq-000001"
	}
	return r
}

func support(frames ...int) annotation.EvidenceSupport {
	return annotation.EvidenceSupport{Frames: frames}
}

func external(what string) annotation.EvidenceSupport {
	return annotation.EvidenceSupport{External: what}
}

func bumper(status annotation.EvidenceStatus, s annotation.EvidenceSupport) annotation.EndpointEvidence {
	return annotation.EndpointEvidence{Status: status, Support: s}
}

func resolvedYaw(status annotation.EvidenceStatus, s annotation.EvidenceSupport) annotation.YawBound {
	return annotation.YawBound{Status: status, Axis: annotation.AxisResolved, YawRad: f64(0), BoundRad: f64(0.05), Support: s}
}

func centreKeyframe(id string, i int, x float64) annotation.PhysicalKeyframe {
	return annotation.PhysicalKeyframe{
		KeyframeID: id, SampleID: i, TimestampNs: SampleTime(i),
		Anchor:   annotation.PhysicalAnchor{Kind: annotation.AnchorBodyCentre},
		Position: annotation.PositionBound{Status: annotation.EvidenceObserved, XM: f64(x), YM: f64(0), BoundM: f64(0.2), Support: support(i)},
		Yaw:      resolvedYaw(annotation.EvidenceObserved, support(i)),
		Front:    bumper(annotation.EvidenceUnknown, annotation.EvidenceSupport{}),
		Rear:     bumper(annotation.EvidenceUnknown, annotation.EvidenceSupport{}),
		Review:   physicalReview(annotation.OriginIndependent, annotation.StatusReviewed),
	}
}

// PhysicalReferences is the fixture's reference document, for tests to
// revise.
func PhysicalReferences(p *annotation.Pack) *annotation.PhysicalReferenceSet {
	r := annotation.NewPhysicalReferenceSet(p)
	r.Change = annotation.Provenance{Author: "fixture", Operation: "fixture"}
	r.Source.CalibrationID = PhysCalibration
	indep := physicalReview(annotation.OriginIndependent, annotation.StatusReviewed)
	half := FollowerLength / 2

	f0 := centreKeyframe("kf-follow-0", 0, FollowerX(0))
	f0.Front, f0.Rear = bumper(annotation.EvidenceObserved, support(0)), bumper(annotation.EvidenceInferred, support(0, 4))
	f0.SharedErrors = []annotation.SharedError{{Observation: "outline fit at sample 0", Components: []string{"length", "position", "yaw"}}}
	f2 := centreKeyframe("kf-follow-2", 2, FollowerX(2)+half)
	f2.Anchor = annotation.PhysicalAnchor{Kind: annotation.AnchorFrontFace, OffsetM: f64(half), OffsetBoundM: f64(0.1)}
	f2.Position.BoundM = f64(0.1)
	f2.Front, f2.Rear = bumper(annotation.EvidenceObserved, support(2)), bumper(annotation.EvidenceInferred, support(2, 4))
	f3 := centreKeyframe("kf-follow-3", OccludedFollowerSample, FollowerX(3))
	f3.Position = annotation.PositionBound{Status: annotation.EvidenceInferred, XM: f64(FollowerX(3)), YM: f64(0), BoundM: f64(0.5),
		Support: external("constant speed between the reviewed keyframes at 2 and 5")}
	f3.Yaw = resolvedYaw(annotation.EvidenceInferred, external("constant heading between the reviewed keyframes"))
	f5 := centreKeyframe("kf-follow-5", 5, FollowerX(5))
	f5.Yaw = annotation.YawBound{Status: annotation.EvidenceInferred, Axis: annotation.AxisFrontRearAmbiguous,
		YawRad: f64(0), BoundRad: f64(0.1), Support: support(5)}
	f7 := centreKeyframe("kf-follow-7", 7, FollowerX(7)-half)
	f7.Anchor = annotation.PhysicalAnchor{Kind: annotation.AnchorRearFace}
	f7.Position.BoundM = f64(0.1)
	f7.Rear = bumper(annotation.EvidenceObserved, support(7))
	f8 := centreKeyframe("kf-follow-8-proposal", 8, FollowerX(8))
	f8.Review = physicalReview(annotation.OriginTrackerAssisted, annotation.StatusProposed)

	follower := annotation.PhysicalObject{
		ObjectID: Follower,
		Body: &annotation.BodyGeometry{
			BodyID: "body-follow", AxisConvention: annotation.BodyAxisConvention,
			Length: annotation.DimensionBound{Status: annotation.EvidenceObserved, Span: annotation.SpanFull,
				LowerM: f64(4.5), UpperM: f64(4.7), ValueM: f64(FollowerLength), Support: support(4, 5)},
			Width: annotation.DimensionBound{Status: annotation.EvidenceInferred, Span: annotation.SpanFull,
				LowerM: f64(1.7), UpperM: f64(1.9), Support: external("registration record")},
			Height: annotation.DimensionBound{Status: annotation.EvidenceObserved, Span: annotation.SpanPartial,
				LowerM: f64(1.1), Support: support(5)},
			Review: indep,
		},
		Keyframes: []annotation.PhysicalKeyframe{f0, f2, f3, f5, f7, f8},
	}
	var leaderFrames []annotation.PhysicalKeyframe
	for _, i := range []int{0, 2, 5} {
		k := centreKeyframe(fmt.Sprintf("kf-lead-%d", i), i, LeaderX(i))
		k.Rear = bumper(annotation.EvidenceObserved, support(i))
		leaderFrames = append(leaderFrames, k)
	}
	leader := annotation.PhysicalObject{
		ObjectID: Leader,
		Body: &annotation.BodyGeometry{
			BodyID: "body-lead", AxisConvention: annotation.BodyAxisConvention,
			Length: annotation.DimensionBound{Status: annotation.EvidenceInferred, Span: annotation.SpanFull,
				LowerM: f64(4.3), UpperM: f64(4.5), Support: external("registration record")},
			Width: annotation.DimensionBound{Status: annotation.EvidenceInferred, Span: annotation.SpanFull,
				LowerM: f64(1.7), UpperM: f64(1.9), Support: external("registration record")},
			Height: annotation.DimensionBound{Status: annotation.EvidenceObserved, Span: annotation.SpanFull,
				LowerM: f64(1.4), UpperM: f64(1.6), Support: support(0)},
			Review: indep,
		},
		Keyframes: leaderFrames,
	}
	gap := func(i int, status annotation.EvidenceStatus, front annotation.EndpointEvidence, lo, hi float64) annotation.FollowingGap {
		return annotation.FollowingGap{SampleID: i, TimestampNs: SampleTime(i), Status: status,
			LowerM: f64(lo), UpperM: f64(hi), ValueM: f64(TrueGapM),
			FollowerFront: front, LeaderRear: bumper(annotation.EvidenceObserved, support(i)), Support: support(i)}
	}
	r.Objects = []annotation.PhysicalObject{follower, leader}
	r.Following = []annotation.FollowingReference{
		{
			FollowingID: "follow-lead", FollowerObjectID: Follower, Decision: annotation.FollowingLeader, LeaderObjectID: Leader,
			Interval: annotation.FrameInterval{FirstSample: 0, LastSample: PhysSamples - 1}, GapDefinition: annotation.GapAlongFollowerAxis,
			Gaps: []annotation.FollowingGap{
				gap(0, annotation.EvidenceObserved, bumper(annotation.EvidenceObserved, support(0)), 5.3, 5.7),
				gap(2, annotation.EvidenceObserved, bumper(annotation.EvidenceObserved, support(2)), 5.3, 5.7),
				// The follower's axis is unresolved at 5, so its front has no
				// name there, and the gap is unknown.
				{SampleID: 5, TimestampNs: SampleTime(5), Status: annotation.EvidenceUnknown,
					FollowerFront: bumper(annotation.EvidenceUnknown, annotation.EvidenceSupport{}),
					LeaderRear:    bumper(annotation.EvidenceObserved, support(5))},
			},
			Review: indep,
		},
		{
			FollowingID: "lead-free", FollowerObjectID: Leader, Decision: annotation.FollowingNoLeader,
			Interval: annotation.FrameInterval{FirstSample: 0, LastSample: PhysSamples - 1}, Review: indep,
		},
	}
	return r
}

// solidBody is one fixture solid-body row.
func solidBody(params, calibration string, seq int64, i int, x float64, ref l5tracks.ReferencePoint, length float32) sqlite.TrackSolidBody {
	cov := [16]float32{0.01, 0, 0, 0, 0, 0.01, 0, 0, 0, 0, 0.1, 0, 0, 0, 0, 0.1}
	frame := SampleTime(i) + EstimateOffsetNs
	id := fmt.Sprintf("%s/%d/%d", params, seq, i)
	return sqlite.TrackSolidBody{
		EstimateID: "solid_body/" + id, TrackID: fmt.Sprintf("uuid-%s-%d", params, seq), ObservationID: "observation/" + id,
		SourceID: PhysSource, CalibrationID: calibration, FrameUnixNanos: frame, MeasurementUnixNanos: frame,
		EstimatorID: EstimatorID, ObservationModelID: string(l5tracks.MeasurementNearEdgeCandidateV1), ParamHash: params,
		Stage: "online", CreationSequence: seq,
		Reading: l5tracks.SolidBodyReading{
			Estimate: l5tracks.SolidBodyEstimate{
				StateModel: l5tracks.StateModelCVCartesianV1, Reference: ref, X: float32(x), Y: 0,
				PositionCovariance:    [4]float32{cov[0], cov[1], cov[4], cov[5]},
				Orientation:           l5tracks.OrientationBelief{PsiRad: 0, VarianceRad2: 0.0025, Provenance: l5tracks.ProvenanceObserved},
				Length:                l5tracks.DimensionBelief{Metres: length, SigmaMetres: 0.1, AdmissibleFrames: 6, Provenance: l5tracks.ProvenanceAccumulated},
				Width:                 l5tracks.DimensionBelief{Metres: CarWidth, SigmaMetres: 0.1, AdmissibleFrames: 6, Provenance: l5tracks.ProvenanceAccumulated},
				Height:                l5tracks.DimensionBelief{Metres: 1.5, SigmaMetres: 0.3, Provenance: l5tracks.ProvenanceClassPrior},
				Motion:                l5tracks.MotionClassBelief{Class: l5tracks.MotionRigidVehicle, Posterior: 0.9},
				Estimation:            l5tracks.EstimationGeometryConverging,
				Stage:                 l5tracks.StageLive,
				LastObservedUnixNanos: frame,
				Support:               l5tracks.SupportState{PointCount: 8},
			},
			VX: 10, Covariance: cov,
			Measurement: l5tracks.SolidBodyMeasurement{Source: l5tracks.MeasurementNearEdgeCandidateV1, Rank: 2},
		},
	}
}

func writePhysicalDB(path string) error {
	database, err := db.NewDB(path)
	if err != nil {
		return err
	}
	defer database.Close()
	store := sqlite.NewStateEstimateStore(database.DB)
	// Every row is attempted and every failure kept: a fixture that cannot
	// be written reports all of what went wrong at once.
	var errs []error
	observe := func(id, calibration string, frame int64) {
		_, err := database.Exec(`INSERT INTO lidar_observations
			(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos,
			 cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
			VALUES (?, 1, ?, ?, ?, 'fixture', ?, ?, 1, '{}', 1)`, id, PhysSource, calibration, PhysSensor, frame, frame)
		errs = append(errs, err)
	}
	write := func(sb sqlite.TrackSolidBody) {
		observe(sb.ObservationID, sb.CalibrationID, sb.FrameUnixNanos)
		errs = append(errs, store.InsertSolidBody(sb))
	}
	for i := 0; i < PhysSamples; i++ {
		for _, v := range []struct{ params, calibration string }{
			{ParamsExact, PhysCalibration}, {ParamsFaceBias, PhysCalibration}, {ParamsOtherCalibration, OtherCalibration},
		} {
			write(solidBody(v.params, v.calibration, 2, i, LeaderX(i), l5tracks.ReferenceBodyCentre, LeaderLength))
			if i == OccludedFollowerSample {
				continue
			}
			x, seq := FollowerX(i), int64(1)
			if v.params == ParamsFaceBias {
				switch {
				case i <= 2:
					x += FaceBiasM
				case i >= 6:
					x, seq = x-FaceBiasM, 3
				}
			}
			write(solidBody(v.params, v.calibration, seq, i, x, l5tracks.ReferenceBodyCentre, FollowerLength))
		}
		for seq, x := range map[int64]float64{1: FollowerX(i), 2: LeaderX(i)} {
			if seq == 1 && i == OccludedFollowerSample {
				continue
			}
			write(solidBody(ParamsMedoid, PhysCalibration, seq, i, x-MedoidOffsetM, l5tracks.ReferenceClusterMedoid, 4.0))
			id := fmt.Sprintf("estimate/%s/%d/%d", ParamsExact, seq, i)
			frame := SampleTime(i) + EstimateOffsetNs
			e := sqlite.TrackEstimate{
				EstimateID: id, TrackID: fmt.Sprintf("uuid-estimate-%d", seq), ObservationID: "observation/" + id,
				SourceID: PhysSource, CalibrationID: PhysCalibration, FrameUnixNanos: frame, MeasurementUnixNanos: frame,
				EstimatorID: EstimatorID, ObservationModelID: string(l5tracks.MeasurementOBBCentreV1), ParamHash: ParamsExact,
				Stage: "final", MeasurementSource: string(l5tracks.MeasurementOBBCentreV1), CreationSequence: seq,
				Reference: l5tracks.ReferenceVisibleOBBCentre, Support: l5tracks.SupportObserved,
				X: float32(x), Y: 0, VX: 10, Covariance: [16]float32{0.01, 0, 0, 0, 0, 0.01, 0, 0, 0, 0, 0.1, 0, 0, 0, 0, 0.1},
			}
			observe(e.ObservationID, PhysCalibration, frame)
			errs = append(errs, store.Insert(e, sqlite.TrackResidual{EstimateID: id, ObservationID: e.ObservationID,
				Disposition: "accepted", Reason: "fixture"}))
		}
	}
	return errors.Join(errs...)
}
