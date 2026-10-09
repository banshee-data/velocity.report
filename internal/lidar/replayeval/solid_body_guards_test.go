package replayeval

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// guardRowsFixture is one moving track of 40 frames at 10 Hz and 8 m/s along
// +X, on the body centre from frame 4. Its heading starts 60 degrees off the
// course and settles to 5 degrees off from frame 20; its box holds all its
// points from frame 10 and a third of them before; its observed length
// exceeds the believed one by a metre on even frames. A second, parked track
// has no velocity, no containment and no spans.
func guardRowsFixture() []observationsqlite.TrackSolidBody {
	const base, period = int64(1_750_000_000_000_000_000), int64(100_000_000)
	var bodies []observationsqlite.TrackSolidBody
	for f := int64(0); f < 40; f++ {
		psi := float32(60 * math.Pi / 180)
		if f >= 20 {
			psi = float32(5 * math.Pi / 180)
		}
		e := l5tracks.SolidBodyEstimate{
			X: float32(f) * 0.8, Reference: l5tracks.ReferenceClusterMedoid,
			Orientation: l5tracks.OrientationBelief{PsiRad: psi, Provenance: l5tracks.ProvenanceObserved},
			Length:      l5tracks.DimensionBelief{Metres: 4.0, Provenance: l5tracks.ProvenanceAccumulated},
			Width:       l5tracks.DimensionBelief{Metres: 1.8, Provenance: l5tracks.ProvenanceAccumulated},
		}
		if f >= 4 {
			e.Reference = l5tracks.ReferenceBodyCentre
		}
		m := l5tracks.SolidBodyMeasurement{
			Source: l5tracks.MeasurementNearEdgeCandidateV1, Rank: 1,
			ContainmentKnown: true, ContainedPoints: 100, ContainmentShare: 1,
			ObservedSpanAlongMetres: 4.0, ObservedSpanAcrossMetres: 1.7,
		}
		if f < 10 {
			m.ContainmentShare = 1.0 / 3
		}
		if f%2 == 0 {
			m.ObservedSpanAlongMetres = 5.0
		}
		if f >= 30 {
			m.ExtentFloor, m.ContainmentShiftAcrossMetres = "length", 0.2
		}
		bodies = append(bodies, observationsqlite.TrackSolidBody{
			CreationSequence: 1, FrameUnixNanos: base + f*period, EstimatorID: "cv_kf_v1", ParamHash: "p", Stage: "online",
			Reading: l5tracks.SolidBodyReading{Estimate: e, VX: 8, Measurement: m},
		})
	}
	for f := int64(0); f < 5; f++ {
		bodies = append(bodies, observationsqlite.TrackSolidBody{
			CreationSequence: 2, FrameUnixNanos: base + f*period, EstimatorID: "cv_kf_v1", ParamHash: "p", Stage: "online",
			Reading: l5tracks.SolidBodyReading{Estimate: l5tracks.SolidBodyEstimate{
				X: 30, Reference: l5tracks.ReferenceClusterMedoid,
				Orientation: l5tracks.OrientationBelief{Provenance: l5tracks.ProvenanceObserved},
				Length:      l5tracks.DimensionBelief{Metres: 4.5, Provenance: l5tracks.ProvenanceClassPrior},
				Width:       l5tracks.DimensionBelief{Metres: 2.0, Provenance: l5tracks.ProvenanceClassPrior},
			}, Measurement: l5tracks.SolidBodyMeasurement{Source: l5tracks.MeasurementMedoidV0, Rank: 2}},
		})
	}
	return bodies
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestSummaryGuardsReadContainmentShortfallAndAxisByAge(t *testing.T) {
	s, err := SummariseSolidBodies(nil, guardRowsFixture(), l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	c := s.Containment
	if c == nil || c.Rows != 40 {
		t.Fatalf("containment %+v, want 40 rows: the parked track carries none", c)
	}
	// 30 of 40 rows hold their points; the median row is one of them.
	if !near(c.HeldShare, 0.75, 1e-9) || c.MedianShare != 1 {
		t.Errorf("containment held %.3f median %.3f, want 0.75 and 1", c.HeldShare, c.MedianShare)
	}
	if len(c.ByReference) != 2 || c.ByReference[0].Reference != "body_centre" || c.ByReference[1].Reference != "cluster_medoid" {
		t.Fatalf("by reference %+v, want body_centre then cluster_medoid", c.ByReference)
	}
	// Frames 4 to 9 are on the body centre and short of their points; 30 of 36 hold.
	if bc := c.ByReference[0]; bc.Rows != 36 || !near(bc.HeldShare, 30.0/36, 1e-9) {
		t.Errorf("body-centre containment %+v, want 36 rows with 30 held", bc)
	}
	if md := c.ByReference[1]; md.Rows != 4 || md.HeldShare != 0 {
		t.Errorf("medoid containment %+v, want 4 rows with none held", md)
	}
	if c.FlooredRows != 10 || c.ShiftedRows != 10 || !near(c.ShiftMedianMetres, 0.2, 1e-6) {
		t.Errorf("floor and shift counts %+v, want 10 floored, 10 shifted by 0.2 m", c)
	}

	x := s.ExtentShortfall
	if x == nil || x.Rows != 36 {
		t.Fatalf("extent shortfall %+v, want the 36 body-centre rows with spans", x)
	}
	// Even frames from 4 to 39 are 18 rows a metre over the believed length; the width never is.
	if x.LengthShortRows != 18 || !near(x.LengthShortShare, 0.5, 1e-9) || !near(x.LengthShortMedianM, 1.0, 1e-6) {
		t.Errorf("length shortfall %+v, want 18 rows, half, a metre", x)
	}
	if x.WidthShortRows != 0 || x.WidthShortShare != 0 {
		t.Errorf("width shortfall %+v, want none", x)
	}

	// 40 rows at 8 m/s: frames 0-9 in 0-1s, 10-19 in 1-2s, 20-39 in 2-4s. The
	// first two buckets are 60 degrees off, the third 5 degrees.
	if len(s.AxisByAge) != 3 {
		t.Fatalf("axis by age %+v, want three buckets", s.AxisByAge)
	}
	for i, want := range []struct {
		bucket string
		rows   int
		axis   float64
		over   float64
	}{{"0-1s", 10, 60, 1}, {"1-2s", 10, 60, 1}, {"2-4s", 20, 5, 0}} {
		got := s.AxisByAge[i]
		if got.Bucket != want.bucket || got.Rows != want.rows || !near(got.AxisMedianDeg, want.axis, 1e-3) ||
			!near(got.AxisOver30Share, want.over, 1e-9) || !near(got.DirectedMedianDeg, want.axis, 1e-3) {
			t.Errorf("bucket %d %+v, want %+v", i, got, want)
		}
	}
}

// Rows from before containment was recorded, or a parked body, leave the
// guards empty rather than reading zero as a share.
func TestSummaryGuardsAreAbsentWithoutTheirRows(t *testing.T) {
	bodies := guardRowsFixture()[40:]
	s, err := SummariseSolidBodies(nil, bodies, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	if s.Containment != nil || s.ExtentShortfall != nil || len(s.AxisByAge) != 0 {
		t.Fatalf("guards present for a parked body with no containment: %+v %+v %+v", s.Containment, s.ExtentShortfall, s.AxisByAge)
	}
	g := s.Guards()
	if g.SolidBodies != 5 || g.Containment != nil {
		t.Fatalf("guards %+v, want five bodies and no containment", g)
	}
}
