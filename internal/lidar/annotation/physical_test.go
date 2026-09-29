package annotation

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func fp(v float64) *float64 { return &v }

const physSamples = 6

// physCarReturns outlines a box: eight returns at x = cx+from and cx+to,
// y = ±halfW, z = 0.3 and 1.5, so every span is known by construction.
func physCarReturns(pts *Points, cx, from, to, halfW float32) {
	for _, x := range []float32{cx + from, cx + to} {
		for _, y := range []float32{-halfW, halfW} {
			for _, z := range []float32{0.3, 1.5} {
				pts.X, pts.Y, pts.Z = append(pts.X, x), append(pts.Y, y), append(pts.Z, z)
			}
		}
	}
}

// physPack writes six samples 100 ms apart of two cars driving along +x at
// ten metres a second: car-1, 4.5 by 1.8 m centred on (10+i, 0), returns 0-7;
// car-2, 4.2 by 1.8 m centred on (20+i, 0), returns 8-15. At sample 3 only
// car-1's rear 1.5 m returned.
func physPack(t *testing.T) *Pack {
	t.Helper()
	var samples []Sample
	var blocks [][]byte
	for i := 0; i < physSamples; i++ {
		var pts Points
		to := float32(2.25)
		if i == 3 {
			to = -0.75
		}
		physCarReturns(&pts, float32(10+i), -2.25, to, 0.9)
		physCarReturns(&pts, float32(20+i), -2.1, 2.1, 0.9)
		block, err := EncodePoints(pts)
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, Sample{
			SourceOrdinal: i, SourceFrameID: uint64(100 + i), TimestampNs: physTime(i),
			SensorID: "hesai-test", PointCount: len(pts.X),
		})
		blocks = append(blocks, block)
	}
	m := Manifest{
		Coverage: CoverageForegroundOnly,
		Source:   SourceProvenance{SensorID: "hesai-test", VRLOGHeaderSHA: "sha256:header", VRLOGFramesSHA: "sha256:frames"},
		Coordinate: CoordinateContract{Units: "metres", FrameID: "sensor", ReferenceFrame: "sensor",
			Handedness: "right", OriginNote: "sensor origin"},
	}
	dir := filepath.Join(t.TempDir(), "pack")
	if err := WritePack(dir, m, samples, blocks); err != nil {
		t.Fatal(err)
	}
	p, err := OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func physTime(i int) int64 { return 1_000_000_000 + int64(i)*100_000_000 }

// physSidecar saves the membership review: both cars reviewed with a mask in
// every sample, and a rejected object with none.
func physSidecar(t *testing.T, p *Pack) *Sidecar {
	t.Helper()
	s := NewSidecar(p)
	s.Change = Provenance{Author: "op", Operation: "label"}
	s.Objects = []Object{reviewedObject("car-1", "car"), reviewedObject("car-2", "car"),
		{ObjectID: "ghost", Class: "car", Status: StatusRejected}}
	for i := 0; i < physSamples; i++ {
		s.Masks = append(s.Masks, mask("car-1", i, 0, 1, 2, 3, 4, 5, 6, 7), mask("car-2", i, 8, 9, 10, 11, 12, 13, 14, 15))
	}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	return s
}

func independentReview() PhysicalReview {
	return PhysicalReview{
		Status: StatusReviewed, Origin: OriginIndependent, Method: "manual_box",
		UncertaintyAssumptions: "hard bounds read off the outline in the top and elevation views",
		Provenance:             Provenance{Author: "op"},
	}
}

func assistedReview() PhysicalReview {
	return PhysicalReview{
		Status: StatusProposed, Origin: OriginTrackerAssisted, Method: "tracker_copy",
		TrackerSource:          "lidar_track_solid_bodies cv_kf_v1 seq-000001",
		UncertaintyAssumptions: "the tracker's one-sigma, unreviewed",
		Provenance:             Provenance{Author: "op"},
	}
}

func frames(f ...int) EvidenceSupport { return EvidenceSupport{Frames: f} }

// validPhysical is a document exercising every record kind: car-1's body with
// an observed length, an inferred width and a partial height; a body-centre
// keyframe at sample 0; a rear-face keyframe at sample 3, where only the rear
// was seen; a tracker-assisted proposal at sample 5; car-2's body and centre;
// and car-1 following car-2 with a gap at sample 0.
func validPhysical(p *Pack) *PhysicalReferenceSet {
	r := NewPhysicalReferenceSet(p)
	r.Change = Provenance{Author: "op", Operation: "author"}
	r.Source.CalibrationID = "calibration/v1/test"
	car1 := PhysicalObject{
		ObjectID: "car-1",
		Body: &BodyGeometry{
			BodyID: "body-car-1", AxisConvention: BodyAxisConvention,
			Length: DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(4.3), UpperM: fp(4.7), ValueM: fp(4.5), Support: frames(0, 1)},
			Width: DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(1.7), UpperM: fp(1.9),
				Support: EvidenceSupport{External: "manufacturer's published width"}},
			Height: DimensionBound{Status: EvidenceObserved, Span: SpanPartial, LowerM: fp(1.1), Support: frames(0)},
			Review: independentReview(),
		},
		Keyframes: []PhysicalKeyframe{
			{
				KeyframeID: "kf-car-1-s0", SampleID: 0, TimestampNs: physTime(0),
				Anchor:   PhysicalAnchor{Kind: AnchorBodyCentre},
				Position: PositionBound{Status: EvidenceObserved, XM: fp(10), YM: fp(0), ZM: fp(0.9), BoundM: fp(0.2), Support: frames(0)},
				Yaw:      YawBound{Status: EvidenceObserved, Axis: AxisResolved, YawRad: fp(0), BoundRad: fp(0.05), Support: frames(0)},
				Front:    EndpointEvidence{Status: EvidenceObserved, Support: frames(0)},
				Rear:     EndpointEvidence{Status: EvidenceObserved, Support: frames(0)},
				SharedErrors: []SharedError{{Observation: "outline fit at sample 0",
					Components: []string{"length", "position", "yaw"}}},
				Review: independentReview(),
			},
			{
				KeyframeID: "kf-car-1-s3", SampleID: 3, TimestampNs: physTime(3),
				Anchor:   PhysicalAnchor{Kind: AnchorRearFace, OffsetM: fp(2.25), OffsetBoundM: fp(0.2)},
				Position: PositionBound{Status: EvidenceObserved, XM: fp(10.75), YM: fp(0), BoundM: fp(0.15), Support: frames(3)},
				Yaw:      YawBound{Status: EvidenceInferred, Axis: AxisResolved, YawRad: fp(0), BoundRad: fp(0.1), Support: frames(2, 3, 4)},
				Front:    EndpointEvidence{Status: EvidenceUnknown},
				Rear:     EndpointEvidence{Status: EvidenceObserved, Support: frames(3)},
				Review:   independentReview(),
			},
			{
				KeyframeID: "kf-car-1-s5-proposal", SampleID: 5, TimestampNs: physTime(5),
				Anchor:   PhysicalAnchor{Kind: AnchorBodyCentre},
				Position: PositionBound{Status: EvidenceObserved, XM: fp(15.1), YM: fp(0.05), BoundM: fp(0.3), Support: frames(5)},
				Yaw:      YawBound{Status: EvidenceInferred, Axis: AxisFrontRearAmbiguous, YawRad: fp(0.02), BoundRad: fp(0.1), Support: frames(5)},
				Front:    EndpointEvidence{Status: EvidenceUnknown},
				Rear:     EndpointEvidence{Status: EvidenceUnknown},
				Review:   assistedReview(),
			},
		},
	}
	car2 := PhysicalObject{
		ObjectID: "car-2",
		Body: &BodyGeometry{
			BodyID: "body-car-2", AxisConvention: BodyAxisConvention,
			Length: DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(4.0), UpperM: fp(4.4), Support: frames(0)},
			Width:  DimensionBound{Status: EvidenceUnknown},
			Height: DimensionBound{Status: EvidencePriorOnly, Span: SpanFull, LowerM: fp(1.3), UpperM: fp(1.7),
				Support: EvidenceSupport{External: "class prior: car height"}},
			Review: independentReview(),
		},
		Keyframes: []PhysicalKeyframe{{
			KeyframeID: "kf-car-2-s0", SampleID: 0, TimestampNs: physTime(0),
			Anchor:   PhysicalAnchor{Kind: AnchorBodyCentre},
			Position: PositionBound{Status: EvidenceObserved, XM: fp(20), YM: fp(0), BoundM: fp(0.2), Support: frames(0)},
			Yaw:      YawBound{Status: EvidenceObserved, Axis: AxisResolved, YawRad: fp(0), BoundRad: fp(0.05), Support: frames(0)},
			Front:    EndpointEvidence{Status: EvidenceUnknown},
			Rear:     EndpointEvidence{Status: EvidenceObserved, Support: frames(0)},
			Review:   independentReview(),
		}},
	}
	r.Objects = []PhysicalObject{car1, car2}
	r.Following = []FollowingReference{{
		FollowingID: "follow-car-1", FollowerObjectID: "car-1", Decision: FollowingLeader, LeaderObjectID: "car-2",
		Interval: FrameInterval{FirstSample: 0, LastSample: 5}, GapDefinition: GapAlongFollowerAxis,
		Gaps: []FollowingGap{{
			SampleID: 0, TimestampNs: physTime(0), Status: EvidenceObserved,
			LowerM: fp(5.3), UpperM: fp(6.0), ValueM: fp(5.65),
			FollowerFront: EndpointEvidence{Status: EvidenceObserved, Support: frames(0)},
			LeaderRear:    EndpointEvidence{Status: EvidenceObserved, Support: frames(0)},
			Support:       frames(0),
		}},
		Review: independentReview(),
	}}
	return r
}

func readPhysical(t *testing.T, p *Pack) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(p.Dir, physicalReferenceFile))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Completion evidence for P0: component bounds, ambiguity, support, source
// and revisions round-trip without changing masks.
func TestPhysicalReferencesRoundTripWithoutTouchingMasks(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	sidecarBefore := readSaved(t, p)

	r := validPhysical(p)
	want := validPhysical(p)
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatalf("save: %v", err)
	}
	if r.Revision != 1 || r.Change.Revision != 1 || r.Change.ParentRev != 0 || r.Change.Operation != "author" || r.Digest() == "" {
		t.Fatalf("first save metadata: revision %d change %+v digest %q", r.Revision, r.Change, r.Digest())
	}
	if !bytes.Equal(sidecarBefore, readSaved(t, p)) {
		t.Fatal("saving physical references changed the sidecar's bytes")
	}
	if s, err := LoadSidecar(p); err != nil || len(s.ReviewedMasks()) != 2*physSamples {
		t.Fatalf("sidecar no longer loads unchanged: %v", err)
	}

	got, err := LoadPhysicalReferences(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(got.Objects, want.Objects) || !reflect.DeepEqual(got.Following, want.Following) || got.Source != want.Source {
		t.Fatalf("round trip changed the references:\n got %+v\nwant %+v", got.Objects, want.Objects)
	}
	if got.Digest() != r.Digest() {
		t.Fatalf("loaded token %s, saved %s", got.Digest(), r.Digest())
	}
	wantOrigins := map[string]ReferenceOrigin{
		"body/body-car-1": OriginIndependent, "body/body-car-2": OriginIndependent,
		"keyframe/kf-car-1-s0": OriginIndependent, "keyframe/kf-car-1-s3": OriginIndependent,
		"keyframe/kf-car-1-s5-proposal": OriginTrackerAssisted, "keyframe/kf-car-2-s0": OriginIndependent,
		"following/follow-car-1": OriginIndependent,
	}
	if !reflect.DeepEqual(got.RecordOrigins, wantOrigins) {
		t.Fatalf("origin ledger %v", got.RecordOrigins)
	}
	kf, ok := got.Keyframe("car-1", 5)
	if !ok || kf.Yaw.Axis != AxisFrontRearAmbiguous || kf.Review.ScoredAsTruth() {
		t.Fatalf("ambiguity or proposal status lost: %+v", kf)
	}
	if _, ok := got.Keyframe("car-1", 4); ok {
		t.Fatal("a keyframe appeared at a sample nobody referenced")
	}
	if _, ok := got.Keyframe("car-9", 0); ok {
		t.Fatal("a keyframe appeared for an object with no reference")
	}
	if o, ok := got.Object("car-2"); !ok || o.Body.Width.Status != EvidenceUnknown || o.Body.Height.Status != EvidencePriorOnly {
		t.Fatalf("unknown and prior-only components did not keep their status: %+v", o.Body)
	}
	if _, ok := got.Object("car-9"); ok {
		t.Fatal("an object appeared with no reference")
	}

	first, err := got.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	// An unchanged save is a new revision with the same content.
	got.Change = Provenance{Author: "op", Operation: "reload"}
	if err := SavePhysicalReferences(p, got); err != nil {
		t.Fatal(err)
	}
	second, _ := got.ContentDigest()
	if got.Revision != 2 || second != first {
		t.Fatalf("revision %d, content digest %s then %s", got.Revision, first, second)
	}
	archived, err := os.ReadFile(filepath.Join(p.Dir, physicalRevisionName(1)))
	if err != nil || sha256Hex(archived) != r.Digest() {
		t.Fatalf("revision 1 was not archived byte for byte: %v", err)
	}
	if !bytes.Equal(sidecarBefore, readSaved(t, p)) {
		t.Fatal("a second physical save changed the sidecar")
	}
}

// The canonical encoding does not depend on the order records were given in.
func TestPhysicalContentDigestIsOrderFreeAndContentBound(t *testing.T) {
	p := physPack(t)
	a := validPhysical(p)
	b := validPhysical(p)
	b.Objects[0], b.Objects[1] = b.Objects[1], b.Objects[0]
	ks := b.Objects[1].Keyframes
	ks[0], ks[2] = ks[2], ks[0]
	b.Objects[1].Keyframes[1].SharedErrors = nil
	a.Objects[0].Keyframes[1].SharedErrors = nil
	da, err := a.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	db, _ := b.ContentDigest()
	if da != db {
		t.Fatal("reordering records changed the content digest")
	}
	if b.Objects[0].ObjectID != "car-2" {
		t.Fatal("computing a digest reordered the caller's document")
	}
	b.Revision, b.UpdatedUTC, b.Change = 7, "2026-01-01T00:00:00Z", Provenance{Author: "someone"}
	if dc, _ := b.ContentDigest(); dc != da {
		t.Fatal("revision metadata entered the content digest")
	}
	*b.Objects[1].Body.Length.LowerM = 4.4
	if dd, _ := b.ContentDigest(); dd == da {
		t.Fatal("a changed bound kept the content digest")
	}
}

// Gate: reviewing a mask does not review a pose. The membership review and
// the physical review are separate records with separate statuses, and the
// sidecar's legacy pose never becomes a physical reference.
func TestReviewedMaskDoesNotBecomeReviewedPose(t *testing.T) {
	p := physPack(t)
	s := physSidecar(t, p)
	// A reviewed mask of a reviewed object, carrying a legacy pose the
	// client may have written, reviewed as well.
	s.Masks[0].Pose = &Pose{CenterX: 10, Length: 4.5, Width: 1.8, Height: 1.5, Confidence: 0.99, Status: StatusReviewed}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	refs, err := LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs.Objects) != 0 {
		t.Fatalf("a reviewed mask produced physical references: %+v", refs.Objects)
	}

	// A proposed keyframe stays proposed when its masks are reviewed, and
	// reviewing masks does not touch the physical file.
	r := validPhysical(p)
	r.Objects[0].Keyframes[0].Review.Status = StatusProposed
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	physicalBefore := readPhysical(t, p)
	s, err = LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	for i := range s.Masks {
		s.Masks[i].Status = StatusReviewed
		s.Masks[i].Completeness = MaskComplete
	}
	s.Change.Operation = "review_all"
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(physicalBefore, readPhysical(t, p)) {
		t.Fatal("a membership review rewrote the physical references")
	}
	reloaded, err := LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	kf, _ := reloaded.Keyframe("car-1", 0)
	if kf.Review.Status != StatusProposed || kf.Review.ScoredAsTruth() {
		t.Fatalf("a mask review reviewed the pose: %+v", kf.Review)
	}
}

// Gate: a tracker-assisted record cannot acquire independent provenance, by
// review, by editing its origin, or by deleting it and adding it back.
func TestTrackerAssistedNeverBecomesIndependent(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	r := validPhysical(p)
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	assisted := &r.Objects[0].Keyframes[2]

	// Reviewing it is allowed and leaves it tracker-assisted.
	assisted.Review.Status = StatusReviewed
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	if kf, _ := r.Keyframe("car-1", 5); kf.Review.ScoredAsTruth() {
		t.Fatal("a reviewed tracker-assisted keyframe counts as truth")
	}

	flip := func(mutate func(*PhysicalKeyframe)) error {
		cur, err := LoadPhysicalReferences(p)
		if err != nil {
			t.Fatal(err)
		}
		mutate(&cur.Objects[0].Keyframes[2])
		return SavePhysicalReferences(p, cur)
	}
	if err := flip(func(k *PhysicalKeyframe) { k.Review = independentReview() }); err == nil {
		t.Fatal("a tracker-assisted keyframe became independent by editing its origin")
	}
	if err := flip(func(k *PhysicalKeyframe) { k.Review.Origin = OriginIndependent }); err == nil {
		t.Fatal("an independent claim over tracker provenance was saved")
	}

	// Deleted in one revision, added back under its old ID in the next.
	cur, _ := LoadPhysicalReferences(p)
	removed := cur.Objects[0].Keyframes[2]
	cur.Objects[0].Keyframes = cur.Objects[0].Keyframes[:2]
	if err := SavePhysicalReferences(p, cur); err != nil {
		t.Fatal(err)
	}
	removed.Review = independentReview()
	cur.Objects[0].Keyframes = append(cur.Objects[0].Keyframes, removed)
	if err := SavePhysicalReferences(p, cur); err == nil || !strings.Contains(err.Error(), "cannot become independent") {
		t.Fatalf("a deleted tracker-assisted ID came back independent: %v", err)
	}
	// Even with the caller's ledger emptied: the store's copy is the one
	// that counts.
	cur.RecordOrigins = map[string]ReferenceOrigin{}
	if err := SavePhysicalReferences(p, cur); err == nil {
		t.Fatal("dropping the caller's ledger laundered the origin")
	}
	// Re-authoring from raw evidence is a new record.
	removed.KeyframeID = "kf-car-1-s5-reauthored"
	cur.Objects[0].Keyframes[2] = removed
	if err := SavePhysicalReferences(p, cur); err != nil {
		t.Fatalf("a new independent record was refused: %v", err)
	}
	// Restoring the revision that held the proposal keeps it assisted, and
	// the ledger keeps the new record's origin.
	if err := RestorePhysicalReferenceRevision(p, cur, 2); err != nil {
		t.Fatal(err)
	}
	kf, _ := cur.Keyframe("car-1", 5)
	if kf.Review.Origin != OriginTrackerAssisted || cur.RecordOrigins["keyframe/kf-car-1-s5-reauthored"] != OriginIndependent {
		t.Fatalf("restore changed an origin: %+v, ledger %v", kf.Review, cur.RecordOrigins)
	}
}

// A revision made after a reference was scored is a new revision: the old
// one stays readable, byte for byte, for the result that cited it.
func TestPhysicalRevisionAfterScoringIsNewRevision(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	r := validPhysical(p)
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	scoredDigest, _ := r.ContentDigest()
	scoredBytes := readPhysical(t, p)

	*r.Objects[0].Keyframes[0].Position.XM = 10.1
	r.Change = Provenance{Author: "op", Operation: "revise"}
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	revised, _ := r.ContentDigest()
	if r.Revision != 2 || revised == scoredDigest {
		t.Fatalf("revision %d; content %s then %s", r.Revision, scoredDigest, revised)
	}
	old, err := LoadPhysicalReferenceRevision(p, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := old.ContentDigest(); d != scoredDigest || old.Digest() != sha256Hex(scoredBytes) {
		t.Fatal("the scored revision no longer reads as it was scored")
	}
	if cur, _ := LoadPhysicalReferenceRevision(p, 2); cur.Digest() != r.Digest() {
		t.Fatal("the current revision did not load through the revision API")
	}
	// A historical revision cannot overwrite the head directly.
	if err := SavePhysicalReferences(p, old); !errors.Is(err, ErrSidecarConflict) {
		t.Fatalf("historical snapshot overwrote head: %v", err)
	}
	if err := RestorePhysicalReferenceRevision(p, r, 1); err != nil {
		t.Fatal(err)
	}
	if d, _ := r.ContentDigest(); r.Revision != 3 || r.RestoredFrom != 1 || r.Change.Operation != "restore" || d != scoredDigest {
		t.Fatalf("restore: revision %d from %d, %q", r.Revision, r.RestoredFrom, r.Change.Operation)
	}
	r.Change = Provenance{Author: "op", Operation: "restore"}
	if err := SavePhysicalReferences(p, r); err != nil || r.RestoredFrom != 0 || r.Change.Operation != "save" {
		t.Fatalf("an ordinary save kept the restore marker: %v %+v", err, r.Change)
	}
}

func TestPhysicalStoreConflictsAndLocks(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	r := validPhysical(p)
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	stale, err := LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	r.Change.Operation = "edit"
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	head := readPhysical(t, p)
	before, _ := json.Marshal(stale)
	if err := SavePhysicalReferences(p, stale); !errors.Is(err, ErrSidecarConflict) {
		t.Fatalf("stale save: %v", err)
	}
	after, _ := json.Marshal(stale)
	if !bytes.Equal(before, after) || !bytes.Equal(head, readPhysical(t, p)) {
		t.Fatal("a refused save changed the caller or the head")
	}
	// A fresh document cannot overwrite an existing one.
	if err := SavePhysicalReferences(p, NewPhysicalReferenceSet(p)); !errors.Is(err, ErrSidecarConflict) {
		t.Fatalf("fresh document replaced existing references: %v", err)
	}
	// Another writer holding the pack's annotation lock, whichever file it
	// is saving, makes this one wait.
	f, err := os.OpenFile(filepath.Join(p.Dir, annotationLock), os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	if err := SavePhysicalReferences(p, r); !errors.Is(err, ErrSidecarBusy) {
		t.Fatalf("competing writer: %v", err)
	}
	f.Close()
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
}

func TestPhysicalStoreRefusesDamagedState(t *testing.T) {
	t.Run("missing head with history", func(t *testing.T) {
		p := physPack(t)
		physSidecar(t, p)
		r := validPhysical(p)
		if err := SavePhysicalReferences(p, r); err != nil {
			t.Fatal(err)
		}
		if err := SavePhysicalReferences(p, r); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(p.Dir, physicalReferenceFile)); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadPhysicalReferences(p); err == nil {
			t.Fatal("missing head became an empty document")
		}
		if err := SavePhysicalReferences(p, r); !errors.Is(err, ErrSidecarConflict) {
			t.Fatalf("deleted head: %v", err)
		}
		if err := SavePhysicalReferences(p, NewPhysicalReferenceSet(p)); err == nil {
			t.Fatal("fresh document bypassed history")
		}
		if _, err := LoadPhysicalReferenceRevision(p, 1); err != nil {
			t.Fatalf("history recovery: %v", err)
		}
	})
	t.Run("corrupt head", func(t *testing.T) {
		p := physPack(t)
		physSidecar(t, p)
		r := validPhysical(p)
		if err := SavePhysicalReferences(p, r); err != nil {
			t.Fatal(err)
		}
		for _, data := range []string{"{not json", `{"schema":"velocity.report/physical-reference","unknown":1}`, "{}\n{}"} {
			if err := os.WriteFile(filepath.Join(p.Dir, physicalReferenceFile), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadPhysicalReferences(p); err == nil {
				t.Fatalf("corrupt head %q loaded", data)
			}
			if err := SavePhysicalReferences(p, r); err == nil {
				t.Fatal("overwrote a corrupt head")
			}
		}
	})
	t.Run("sidecar missing with history", func(t *testing.T) {
		p := physPack(t)
		s := physSidecar(t, p)
		if err := SaveSidecar(p, s); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(p.Dir, sidecarFile)); err != nil {
			t.Fatal(err)
		}
		if err := SavePhysicalReferences(p, validPhysical(p)); err == nil {
			t.Fatal("saved against a damaged membership head")
		}
	})
	t.Run("sidecar unreadable", func(t *testing.T) {
		p := physPack(t)
		if err := os.Mkdir(filepath.Join(p.Dir, sidecarFile), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := SavePhysicalReferences(p, validPhysical(p)); err == nil {
			t.Fatal("saved against an unreadable sidecar")
		}
	})
	t.Run("no sidecar at all", func(t *testing.T) {
		p := physPack(t)
		err := SavePhysicalReferences(p, validPhysical(p))
		if err == nil || !strings.Contains(err.Error(), "declare it there first") {
			t.Fatalf("references to objects nobody declared: %v", err)
		}
		empty := NewPhysicalReferenceSet(p)
		if err := SavePhysicalReferences(p, empty); err != nil || empty.Revision != 1 {
			t.Fatalf("an empty document needs no objects: %v", err)
		}
	})
	t.Run("unreadable head", func(t *testing.T) {
		p := physPack(t)
		physSidecar(t, p)
		if err := os.Mkdir(filepath.Join(p.Dir, physicalReferenceFile), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadPhysicalReferences(p); err == nil {
			t.Fatal("directory read as references")
		}
		if err := SavePhysicalReferences(p, validPhysical(p)); err == nil {
			t.Fatal("wrote over an unreadable head")
		}
	})
	t.Run("archive mismatch", func(t *testing.T) {
		p := physPack(t)
		physSidecar(t, p)
		r := validPhysical(p)
		if err := SavePhysicalReferences(p, r); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(p.Dir, physicalRevisionDir), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p.Dir, physicalRevisionName(1)), []byte("other"), 0o600); err != nil {
			t.Fatal(err)
		}
		head := readPhysical(t, p)
		if err := SavePhysicalReferences(p, r); err == nil || r.Revision != 1 || !bytes.Equal(head, readPhysical(t, p)) {
			t.Fatalf("overwrote a differing archive: %v", err)
		}
	})
	t.Run("absent pack", func(t *testing.T) {
		p := physPack(t)
		p.Dir = filepath.Join(p.Dir, "missing")
		if _, err := LoadPhysicalReferences(p); err == nil {
			t.Fatal("absent pack loaded")
		}
		if err := SavePhysicalReferences(p, NewPhysicalReferenceSet(p)); err == nil {
			t.Fatal("absent pack saved")
		}
		if _, err := LoadPhysicalReferenceRevision(p, 1); err == nil {
			t.Fatal("absent pack revision loaded")
		}
	})
	t.Run("exhausted revisions", func(t *testing.T) {
		p := physPack(t)
		physSidecar(t, p)
		r := NewPhysicalReferenceSet(p)
		r.Revision = math.MaxInt - 1
		b, _ := json.Marshal(r)
		if err := os.WriteFile(filepath.Join(p.Dir, physicalReferenceFile), b, 0o600); err != nil {
			t.Fatal(err)
		}
		cur, err := LoadPhysicalReferences(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := SavePhysicalReferences(p, cur); err == nil {
			t.Fatal("exhausted revision saved")
		}
	})
	t.Run("oversized", func(t *testing.T) {
		p := physPack(t)
		physSidecar(t, p)
		r := validPhysical(p)
		r.Objects[0].Body.Review.UncertaintyAssumptions = strings.Repeat("x", MaxSidecarBytes)
		if err := SavePhysicalReferences(p, r); err == nil {
			t.Fatal("oversized document saved")
		}
	})
}

func TestPhysicalRevisionReadFailures(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	for _, revision := range []int{0, -1, 9} {
		if _, err := LoadPhysicalReferenceRevision(p, revision); err == nil {
			t.Fatalf("revision %d loaded", revision)
		}
		if err := RestorePhysicalReferenceRevision(p, NewPhysicalReferenceSet(p), revision); err == nil {
			t.Fatalf("restore of revision %d succeeded", revision)
		}
	}
	r := validPhysical(p)
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p.Dir, physicalRevisionName(1))
	for _, data := range [][]byte{[]byte("bad"), readPhysical(t, p)} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadPhysicalReferenceRevision(p, 1); err == nil {
			t.Fatal("damaged or mislabelled archive loaded")
		}
	}
	// A restore whose result no longer links to the sidecar is refused.
	stale := validPhysical(p)
	stale.Revision = 1
	stale.RecordOrigins = stale.currentOrigins()
	b, _ := json.MarshalIndent(stale, "", "  ")
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := LoadSidecar(p)
	s.Objects = []Object{reviewedObject("car-2", "car")}
	s.Masks = s.Masks[:0]
	for i := 0; i < physSamples; i++ {
		s.Masks = append(s.Masks, mask("car-2", i, 8, 9, 10, 11, 12, 13, 14, 15))
	}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	if err := RestorePhysicalReferenceRevision(p, r, 1); err == nil {
		t.Fatal("restored references to an object the sidecar no longer has")
	}
}
