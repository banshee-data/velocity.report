package l5tracks

import (
	"crypto/sha256"
	"encoding/hex"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The options that join the published box to the solid body
// (BoxModelConfig.BoxExtentFromSolidBody, SolidBodyOptions.SharedOrientation)
// are default-off, and off they must leave the shipped arithmetic exactly as
// it was: the box's own extent belief, centre and heading filter, and the
// solid body's own orientation. This test pins it. It runs the box model with
// and without the solid body, the face-state options on, and hashes every bit
// the box and the solid body publish on every frame. The golden digests were
// taken on the code before either option existed.
//
// They are pinned on amd64 only, for the reason the face-state digest is
// (solid_body_face_state_off_test.go): other targets fuse multiply-adds.

// throughTruckPass is a 9.6 by 2.5 m truck passing 4 m from the sensor at
// 7 m/s, from 35 m before it to 35 m after: its front face on approach, its
// near side abeam, its rear face on departure. The end-on truck pass stops
// short of the sensor; this one goes through, so both end faces are seen.
func throughTruckPass() l4perception.SyntheticPass {
	sensor := l4perception.DefaultSyntheticSensor()
	sensor.HeightMetres = 2.3
	return l4perception.SyntheticPass{
		Sensor: sensor,
		Vehicle: l4perception.SyntheticVehicle{
			Length: 9.6, Width: 2.5, Height: 3.2, LateralOffsetMetres: 4,
			SpeedMps: 7, StartXMetres: -35,
		},
		Frames: 100, Interval: 100 * time.Millisecond, SensorID: "synthetic-pandar40p",
	}
}

// faceStateConfig is the solid body with all five face-state options.
func faceStateConfig() TrackerConfig {
	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentState = true
	cfg.SolidBody.CensoredExtent = true
	cfg.SolidBody.SpanBoundsBelief = true
	cfg.SolidBody.HeadingFaceNoise = true
	cfg.SolidBody.AnisotropicReferenceBias = true
	return cfg
}

func boxFingerprintScenarios() []solidBodyFingerprintScenario {
	return []solidBodyFingerprintScenario{
		{
			name:   "box_default_pass",
			pass:   l4perception.DefaultSyntheticPass,
			config: DefaultTrackerConfig,
			golden: "91eab0d1e27e3483",
		},
		{
			name:   "box_truck_face_state",
			pass:   throughTruckPass,
			config: faceStateConfig,
			// Span bounds as a projection (#989): faceStateConfig applies
			// bounds, so this pass moved with that fix and nothing else; with
			// SpanBoundsBelief and CensoredExtent off it hashes as before.
			golden: "dab1c76a63b6fc6c",
		},
		{
			name: "box_truck_face_state_rectangle_held",
			pass: throughTruckPass,
			config: func() TrackerConfig {
				cfg := faceStateConfig()
				cfg.SolidBody.RectangleHeading = true
				cfg.SolidBody.RectangleCourseFusion = true
				cfg.SolidBody.Containment = true
				cfg.BoxModel.AdmitMergeSpans = true
				cfg.BoxModel.YieldFrames = 3
				cfg.BoxModel.ExclusiveMarginMetres = 0.3
				cfg.BoxModel.DisplacementLabelMetres = 2
				return cfg
			},
			// Moved with the span-bound projection (#989), as above.
			golden: "164ca110ef6a341a",
		},
	}
}

// boxFingerprint hashes, frame by frame and track by track in creation order,
// the published box, its estimator's heading state and counters, and the
// solid body's position, orientation and dimensions.
func boxFingerprint(t *testing.T, cfg TrackerConfig, frames []syntheticPassFrame) string {
	t.Helper()
	h := sha256.New()
	tracker := NewTracker(cfg)
	for _, f := range frames {
		tracker.Update(f.clusters, f.at)
		tracks := tracker.GetAllTracks()
		sort.Slice(tracks, func(i, j int) bool { return tracks[i].CreationSequence < tracks[j].CreationSequence })
		for _, tr := range tracks {
			writeInt(h, tr.CreationSequence)
			writeFloats(h, tr.X, tr.Y, tr.VX, tr.VY)
			writeFloats(h, tr.BoxX, tr.BoxY, tr.BoxHeadingRad, tr.BoxLength, tr.BoxWidth, tr.BoxZ, tr.BoxHeight)
			if b := tr.box; b != nil {
				s := b.State()
				writeFloats(h, float32(s.X), float32(s.Y), float32(s.HeadingRad), float32(s.Length), float32(s.Width))
				writeFloats(h, float32(b.psi), float32(b.rate), float32(b.p[0][0]), float32(b.p[0][1]), float32(b.p[1][1]))
				writeInt(h, int64(b.Stats.Relabels), int64(b.Stats.Clamps), int64(b.Stats.Restarts),
					int64(b.Stats.DisplacementLabels), int64(b.Stats.Yields))
			} else {
				writeInt(h, -2)
			}
			r, ok := tr.SolidBody()
			if !ok {
				writeInt(h, -1)
				continue
			}
			e, m := r.Estimate, r.Measurement
			writeInt(h, int64(e.Reference), int64(e.Estimation))
			writeFloats(h, e.X, e.Y, r.VX, r.VY)
			writeFloats(h, r.Covariance[:]...)
			writeFloats(h, e.Orientation.PsiRad, e.Orientation.VarianceRad2, e.Orientation.AmbiguousModeWeight)
			writeInt(h, int64(e.Orientation.Provenance))
			for _, d := range []DimensionBelief{e.Length, e.Width} {
				writeFloats(h, d.Metres, d.SigmaMetres)
				writeInt(h, int64(d.AdmissibleFrames), int64(d.Provenance))
			}
			writeInt(h, int64(m.Rank), int64(m.Faces))
			writeFloats(h, m.NIS, m.AspectRad)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func TestBoxSolidBodyOptionsOffLeaveTheShippedArithmetic(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skipf("digests are pinned on amd64; %s may fuse multiply-adds", runtime.GOARCH)
	}
	for _, s := range boxFingerprintScenarios() {
		got := boxFingerprint(t, s.config(), syntheticPassFrames(t, s.pass()))
		if got != s.golden {
			t.Errorf("%s: digest %s, want %s", s.name, got, s.golden)
		}
	}
}
