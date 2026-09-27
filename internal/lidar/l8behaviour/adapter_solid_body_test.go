package l8behaviour

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// Readings from the tracker's near-edge solid body on the synthetic pass, as a
// consumer would read them live or back from lidar_track_solid_bodies.

type passReading struct {
	capture int64
	reading l5tracks.SolidBodyReading
}

// synthesisedPassReadings runs the default synthetic pass through a tracker
// with the solid body on and the vehicle classified as a car, and returns the
// longest-lived track's reading after every frame it was observed on.
func synthesisedPassReadings(t *testing.T) []passReading {
	t.Helper()
	cfg := l5tracks.DefaultTrackerConfig()
	cfg.SolidBody = l5tracks.SolidBodyOptions{Enabled: true, OriginSource: "test: synthetic pass in the sensor frame"}
	tracker := l5tracks.NewTracker(cfg)
	params := l4perception.DefaultDBSCANParams()
	params.Eps, params.MinPts, params.MaxSamplePoints = 0.6, 5, 512
	base := time.Unix(1_700_000_000, 0).Sub(time.Unix(0, 0))

	var out []passReading
	for _, f := range l4perception.GenerateSyntheticPass(l4perception.DefaultSyntheticPass()) {
		points := make([]l4perception.WorldPoint, len(f.Points))
		for i, p := range f.Points {
			p.Timestamp = p.Timestamp.Add(base)
			points[i] = p
		}
		at := f.Timestamp.Add(base)
		tracker.Update(l4perception.DBSCAN(points, params), at)
		var main *l5tracks.TrackedObject
		for _, tr := range tracker.GetActiveTracks() {
			tracker.UpdateClassification(tr.TrackID, "car", 0.9, "test")
			if main == nil || tr.ObservationCount > main.ObservationCount {
				main = tr
			}
		}
		if main == nil || main.Misses != 0 {
			continue
		}
		if r, ok := main.SolidBody(); ok {
			out = append(out, passReading{capture: at.UnixNano(), reading: r})
		}
	}
	if len(out) == 0 {
		t.Fatal("no solid-body readings from the synthetic pass")
	}
	return out
}

func TestSampleFromSolidBodyReadingAcceptsEveryObservedInstant(t *testing.T) {
	bounds := l5tracks.DefaultConvergenceBounds()
	var established, faces int
	var front, rear bool
	for i, pr := range synthesisedPassReadings(t) {
		s, err := SampleFromSolidBodyReading(pr.reading, pr.capture, bounds)
		if err != nil {
			t.Fatalf("reading %d (%s, %s): %v", i, pr.reading.Estimate.Reference, pr.reading.Estimate.Estimation, err)
		}
		if s.Estimation == EstimationEstablished {
			established++
		}
		if s.Faces.FrontObserved || s.Faces.RearObserved {
			faces++
			if !s.Heading.Resolved() || s.Support != SupportObserved {
				t.Fatalf("reading %d names a face without a resolved heading on an observed instant", i)
			}
		}
		front = front || s.Faces.FrontObserved
		rear = rear || s.Faces.RearObserved
	}
	if established == 0 {
		t.Error("no established sample on a clean pass")
	}
	// The vehicle approaches the sensor and then leaves it, so its front and
	// then its rear face the sensor.
	if faces == 0 || !front || !rear {
		t.Errorf("end faces claimed on %d instants (front %v, rear %v); the pass shows the front then the rear", faces, front, rear)
	}
}

func TestSampleFromSolidBodyReadingClaimsOnlyTheFacesTheFixUsed(t *testing.T) {
	var reading passReading
	for _, pr := range synthesisedPassReadings(t) {
		if pr.reading.Measurement.Source == l5tracks.MeasurementNearEdgeCandidateV1 &&
			pr.reading.Estimate.Orientation.IsResolved() {
			reading = pr
		}
	}
	if reading.capture == 0 {
		t.Fatal("no near-edge reading with a resolved heading")
	}
	bounds := l5tracks.DefaultConvergenceBounds()

	s, err := SampleFromSolidBodyReading(reading.reading, reading.capture, bounds)
	if err != nil {
		t.Fatal(err)
	}
	m := reading.reading.Measurement
	if s.Faces.FrontObserved != m.Faces.Has(l5tracks.FaceFront) || s.Faces.RearObserved != m.Faces.Has(l5tracks.FaceRear) {
		t.Fatalf("faces %+v from fix faces %q", s.Faces, m.Faces)
	}

	// The same instant from a medoid update names no face.
	medoid := reading.reading
	medoid.Measurement.Source = l5tracks.MeasurementMedoidV0
	if s, err := SampleFromSolidBodyReading(medoid, reading.capture, bounds); err != nil || s.Faces != (FaceVisibility{}) {
		t.Fatalf("a medoid update claimed faces %+v (%v)", s.Faces, err)
	}

	// An unresolved heading cannot tell front from rear.
	ambiguous := reading.reading
	ambiguous.Estimate.Orientation.AmbiguousModeWeight = 0.5
	ambiguous.Estimate.Estimation = l5tracks.EstimationGeometryConverging
	if s, err := SampleFromSolidBodyReading(ambiguous, reading.capture, bounds); err != nil || s.Faces != (FaceVisibility{}) {
		t.Fatalf("an unresolved heading claimed faces %+v (%v)", s.Faces, err)
	}
}

func TestSampleFromSolidBodyReadingRefusesADisagreeingCovariance(t *testing.T) {
	pr := synthesisedPassReadings(t)[0]
	pr.reading.Covariance[0] += 1
	if _, err := SampleFromSolidBodyReading(pr.reading, pr.capture, l5tracks.DefaultConvergenceBounds()); err == nil {
		t.Fatal("a reading whose covariance disagrees with its estimate was accepted")
	}
}

// persistedPass files the synthetic pass's readings as one online version of
// lidar_track_solid_bodies rows, as the online sink writes them.
func persistedPass(t *testing.T) []PersistedSolidBody {
	t.Helper()
	var rows []PersistedSolidBody
	for _, pr := range synthesisedPassReadings(t) {
		rows = append(rows, PersistedSolidBody{
			TrackID: "trk_pass", SensorID: "sensor_a", FrameUnixNanos: pr.capture,
			EstimatorID: "cv_kf_v1", ObsModelID: string(l5tracks.MeasurementNearEdgeCandidateV1),
			ParamHash: "sha256:online", Stage: "online", Reading: pr.reading,
		})
	}
	return rows
}

// TestTrajectoriesFromSolidBodies: persisted readings become the samples a
// live reading does, with the class from the latest row, the reference each
// row named, and each measurement's acquisition time.
func TestTrajectoriesFromSolidBodies(t *testing.T) {
	rows := persistedPass(t)
	// Out of order, as a query need not return them.
	rows[0], rows[len(rows)-1] = rows[len(rows)-1], rows[0]
	trajectories, err := TrajectoriesFromSolidBodies(rows, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	if len(trajectories) != 1 || len(trajectories[0].Samples) != len(rows) {
		t.Fatalf("%d trajectories", len(trajectories))
	}
	tr := trajectories[0]
	want := EstimateIdentity{EstimatorID: "cv_kf_v1", ObsModelID: "near_edge_candidate_v1", ParamHash: "sha256:online"}
	if tr.Estimate != want || tr.Passage.SensorID != "sensor_a" || tr.Passage.MotionClass != MotionRigidVehicle ||
		tr.Passage.ClassConfidence == nil {
		t.Fatalf("passage %+v, estimate %+v", tr.Passage, tr.Estimate)
	}
	centred := 0
	for i, s := range tr.Samples {
		if i > 0 && s.CaptureUnixNanos <= tr.Samples[i-1].CaptureUnixNanos {
			t.Fatal("samples are not in frame order")
		}
		if s.Stage != StageOnline || s.Support != SupportObserved || s.LastObservedUnixNanos != s.CaptureUnixNanos {
			t.Fatalf("sample %d: %+v", i, s)
		}
		if s.AcquisitionUnixNanos <= 0 {
			t.Fatalf("sample %d lost its acquisition time", i)
		}
		switch s.Reference {
		case ReferenceBodyCentre:
			centred++
		case ReferenceClusterMedoid:
			if s.Faces != (FaceVisibility{}) {
				t.Fatalf("sample %d claims a face from the medoid", i)
			}
		default:
			t.Fatalf("sample %d refers to %s", i, s.Reference)
		}
	}
	if centred == 0 {
		t.Fatal("no near-edge fix placed the body centre")
	}
	if out, err := TrajectoriesFromSolidBodies(nil, l5tracks.DefaultConvergenceBounds()); err != nil || out != nil {
		t.Errorf("no rows = %v, %v; want nothing and no error", out, err)
	}
}

// TestTrajectoriesFromSolidBodiesRefusesWhatARowCannotSay: a mixed version,
// a stage its reading does not state, an unknown stage, two sensors, the
// fixture's identity and a reading that is not a sample are errors.
func TestTrajectoriesFromSolidBodiesRefusesWhatARowCannotSay(t *testing.T) {
	for name, mutate := range map[string]func([]PersistedSolidBody){
		"mixed parameters": func(r []PersistedSolidBody) { r[1].ParamHash = "sha256:other" },
		"relabelled final": func(r []PersistedSolidBody) {
			for i := range r {
				r[i].Stage = "final"
			}
		},
		"unknown stage": func(r []PersistedSolidBody) {
			for i := range r {
				r[i].Stage = "smoothed"
			}
		},
		"two sensors":       func(r []PersistedSolidBody) { r[len(r)-1].SensorID = "sensor_b" },
		"no parameter hash": func(r []PersistedSolidBody) { r[0].ParamHash = "" },
		"fixture identity": func(r []PersistedSolidBody) {
			for i := range r {
				f := FixtureEstimate()
				r[i].EstimatorID, r[i].ObsModelID, r[i].ParamHash = f.EstimatorID, f.ObsModelID, f.ParamHash
			}
		},
		"unknown reference": func(r []PersistedSolidBody) { r[1].Reading.Estimate.Reference = l5tracks.ReferenceUnknown },
		"repeated frame":    func(r []PersistedSolidBody) { r[1].FrameUnixNanos = r[0].FrameUnixNanos },
	} {
		t.Run(name, func(t *testing.T) {
			rows := persistedPass(t)
			mutate(rows)
			if _, err := TrajectoriesFromSolidBodies(rows, l5tracks.DefaultConvergenceBounds()); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

// TestSampleFromSolidBodyReadingAveragesRoundOff: the shadow filter's float32
// covariance, whose pairs differ in their last places as kirk0's persisted
// solid bodies do, is averaged into one symmetric matrix; a pair further
// apart than round-off is refused.
func TestSampleFromSolidBodyReadingAveragesRoundOff(t *testing.T) {
	pr := synthesisedPassReadings(t)[0]
	r := pr.reading
	r.Covariance[1] = 0.01
	r.Covariance[4] = math.Nextafter32(math.Nextafter32(0.01, 1), 1)
	r.Estimate.PositionCovariance = [4]float32{r.Covariance[0], r.Covariance[1], r.Covariance[4], r.Covariance[5]}
	s, err := SampleFromSolidBodyReading(r, pr.capture, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatalf("round-off refused: %v", err)
	}
	if s.Covariance[1] != s.Covariance[4] || s.Covariance[1] != float64(math.Nextafter32(0.01, 1)) {
		t.Errorf("pair = %v and %v, want both the float32 between them", s.Covariance[1], s.Covariance[4])
	}
	r.Covariance[4] = 0.02
	r.Estimate.PositionCovariance[2] = 0.02
	if _, err := SampleFromSolidBodyReading(r, pr.capture, l5tracks.DefaultConvergenceBounds()); err == nil {
		t.Error("an asymmetric covariance was accepted")
	}
}
