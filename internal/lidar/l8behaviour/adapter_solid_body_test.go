package l8behaviour

import (
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
	cfg.SolidBody = l5tracks.SolidBodyOptions{Enabled: true}
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
