package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// writeEvidence builds a small evidence database the way a replay does: one
// track followed for 3 s, then its object continuing unassociated.
// trackID stands in for the tracker's random UUID.
func writeEvidence(t *testing.T, path, trackID string) {
	t.Helper()
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	observations := observationsqlite.NewObservationStore(database)
	states := observationsqlite.NewStateEstimateStore(database)
	const source, frame = "source/v1/tool-test", int64(100_000_000)
	base := time.Date(2026, 9, 2, 13, 0, 0, 0, time.UTC).UnixNano()

	for f := int64(0); f < 60; f++ {
		ts := base + f*frame
		x := float32(f) * 0.5
		id := fmt.Sprintf("observation/v1/%04d", f)
		record := l4bobserve.Record{SchemaVersion: 1, ObservationID: id, SourceID: source,
			CalibrationID: "calibration/v1/test", FrameUnixNanos: ts,
			Cluster: l4perception.WorldCluster{ClusterID: 1, SensorID: "hesai-pandar40p", FrameID: "site/test",
				TSUnixNanos: ts + 3, CentroidX: x, CentroidY: 12, PointsCount: 60,
				RetainedPoints: []l4perception.WorldPoint{{X: float64(x), Y: 12, Z: -2, Timestamp: time.Unix(0, ts+3).UTC(), SensorID: "hesai-pandar40p"}}},
		}
		observation, err := l4bobserve.New(record)
		if err != nil {
			t.Fatal(err)
		}
		if err := observations.Insert(observation); err != nil {
			t.Fatal(err)
		}
		if f >= 30 {
			continue // the object keeps producing clusters; the track has ended
		}
		estimateID := fmt.Sprintf("estimate/%s/%04d", trackID, f)
		estimate := observationsqlite.TrackEstimate{EstimateID: estimateID, TrackID: trackID, ObservationID: id,
			SourceID: source, CalibrationID: "calibration/v1/test", FrameUnixNanos: ts, MeasurementUnixNanos: ts + 3,
			EstimatorID: "cv_kf_v1", ObservationModelID: "obb_centre_v1", ParamHash: "params/test", Stage: "online",
			MeasurementSource: "obb_centre_v1", CreationSequence: 1, X: x, Y: 12, VX: 5, VY: 0}
		residual := observationsqlite.TrackResidual{EstimateID: estimateID, ObservationID: id,
			MeasurementX: x, MeasurementY: 12, InnovationX: 0.1, InnovationY: 0.05, NIS: 1.5,
			Disposition: "accepted", Reason: "association_accepted"}
		if err := states.Insert(estimate, residual); err != nil {
			t.Fatal(err)
		}
	}
}

// Two databases holding the same evidence differ byte for byte (insert
// timestamps, the random track id), and must still give identical output.
func TestScorecardIsIdenticalAcrossIndependentlyWrittenEvidence(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.db"), filepath.Join(dir, "b.db")
	writeEvidence(t, a, "trk_11111111-aaaa")
	writeEvidence(t, b, "trk_99999999-zzzz")

	encode := func(path string) []byte {
		doc, err := score(path, 1.0, "", 2.0)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	first, second := encode(a), encode(b)
	if !bytes.Equal(first, second) {
		t.Fatalf("scorecards differ across independently written evidence:\n%s\n---\n%s", first, second)
	}
	if bytes.Contains(first, []byte("trk_")) {
		t.Error("the random track id leaked into the scorecard")
	}
	if !bytes.Equal(first, encode(a)) {
		t.Error("scoring the same database twice gave different output")
	}
}

func TestScorecardReadsTheEvidenceItWasGiven(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.db")
	writeEvidence(t, path, "trk_test")
	doc, err := score(path, 0, "", 2.0)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(doc.Sources))
	}
	sc := doc.Sources[0].Scorecard
	if sc.Population.Tracks != 1 || sc.Population.Estimates != 30 || sc.Population.Clusters != 60 {
		t.Errorf("population = %+v, want 1 track, 30 estimates, 60 clusters", sc.Population)
	}
	if sc.Population.ClustersUnassigned != 30 {
		t.Errorf("unassigned clusters = %d, want 30", sc.Population.ClustersUnassigned)
	}
	// The track ends at frame 29 and its object carries on, unassociated, on
	// its predicted path.
	for _, c := range sc.Termination.ByClass {
		want := 0
		if c.Class == "unassigned_nearby" {
			want = 1
		}
		if c.Count != want {
			t.Errorf("termination %s = %d, want %d", c.Class, c.Count, want)
		}
	}
}

// Scoring a run against an independently written copy of the same evidence
// must report perfect agreement. This is the noise floor for every reference
// comparison: a difference anywhere else is a real difference, not the
// metric's own variance.
func TestReferenceComparisonOfIdenticalEvidenceIsPerfect(t *testing.T) {
	dir := t.TempDir()
	candidate := filepath.Join(dir, "candidate.db")
	reference := filepath.Join(dir, "reference.db")
	writeEvidence(t, candidate, "trk_aaaa")
	writeEvidence(t, reference, "trk_zzzz")

	doc, err := score(candidate, 0, reference, 2.0)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sources) != 1 || len(doc.Sources[0].Reference) != 1 {
		t.Fatalf("got %d sources with %d reference comparisons, want 1 and 1",
			len(doc.Sources), len(doc.Sources[0].Reference))
	}
	ref := doc.Sources[0].Reference[0]
	m := ref.Metrics
	if m.MOTA != 1.0 || m.IDSwitches != 0 || m.Fragmentations != 0 || m.FN != 0 || m.FP != 0 {
		t.Errorf("identical evidence scored %+v; want MOTA 1.0 with no errors", m)
	}
	if ref.Kind != "reference_run" || ref.Note == "" {
		t.Error("the comparison must say what the reference is, so agreement is not read as accuracy")
	}
	if ref.ReferenceTracks != ref.CandidateTracks {
		t.Errorf("track counts differ: %d reference, %d candidate", ref.ReferenceTracks, ref.CandidateTracks)
	}
}

// A reference section only appears when one was asked for, so an existing
// scorecard's bytes do not change.
func TestReferenceSectionIsAbsentWithoutAReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.db")
	writeEvidence(t, path, "trk_test")
	doc, err := score(path, 0, "", 2.0)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sources[0].Reference) != 0 {
		t.Fatal("reference section present without -reference")
	}
	payload, err := marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte(`"reference"`)) {
		t.Error("empty reference section was serialised; omitempty should drop it")
	}
}
