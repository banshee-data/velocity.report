package l5tracks

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"math"
	"runtime"
	"sort"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The face-state options (ExtentState, CensoredExtent, SpanBoundsBelief,
// HeadingFaceNoise, AnisotropicReferenceBias) are default-off, and off they
// must leave the shipped arithmetic exactly as it was. This test pins it: it
// runs four existing solid-body scenarios, shadow and tracked, with a spread
// of the earlier options on, and hashes every bit the solid body and the
// tracked filter publish on every frame. The golden digests were taken on the
// code before the face-state options existed.
//
// They are pinned on amd64 only. The Go compiler fuses x*y+z into one
// rounding on arm64 and some other targets, so the same source gives other
// bits there; the property is the same, but its digest is not.

// solidBodyFingerprintScenario is one pinned run.
type solidBodyFingerprintScenario struct {
	name   string
	pass   func() l4perception.SyntheticPass
	config func() TrackerConfig
	golden string
}

func solidBodyFingerprintScenarios() []solidBodyFingerprintScenario {
	return []solidBodyFingerprintScenario{
		{
			name:   "shadow_default_pass",
			pass:   l4perception.DefaultSyntheticPass,
			config: solidBodyConfig,
			golden: "69fecbfeb7da6a56",
		},
		{
			name: "shadow_end_on_truck_containment",
			pass: endOnTruckPass,
			config: func() TrackerConfig {
				cfg := solidBodyConfig()
				cfg.SolidBody.CourseAlignedFaces = true
				cfg.SolidBody.CourseHeading = true
				cfg.SolidBody.ExtentGrowthAdmission = true
				cfg.SolidBody.Containment = true
				cfg.SolidBody.ReferenceTranslation = true
				cfg.SolidBody.FaceEntryConsider = true
				return cfg
			},
			golden: "f808e9e660ad9a75",
		},
		{
			name: "tracked_end_on_truck_remedies",
			pass: endOnTruckPass,
			config: func() TrackerConfig {
				cfg := nearEdgeTrackingConfig()
				cfg.SolidBody.FaceHysteresis = true
				cfg.SolidBody.CourseAlignedFaces = true
				cfg.SolidBody.RankOneMedoidScale = 1
				cfg.SolidBody.EndFaceCentring = true
				cfg.SolidBody.EndFaceCentringOpenPrior = true
				cfg.SolidBody.VehicleExtentFloor = true
				cfg.SolidBody.Containment = true
				return cfg
			},
			golden: "e012f3cecc556d23",
		},
		{
			name: "shadow_default_pass_rectangle",
			pass: l4perception.DefaultSyntheticPass,
			config: func() TrackerConfig {
				cfg := solidBodyConfig()
				cfg.SolidBody.ExtentPriorFloor = true
				cfg.SolidBody.FacePlaneSpans = true
				cfg.SolidBody.RectangleHeading = true
				cfg.SolidBody.RectangleCourseFusion = true
				cfg.SolidBody.RankOneMedoidScale = 0.25
				return cfg
			},
			golden: "70a3e339552f53da",
		},
	}
}

// solidBodyFingerprint hashes, frame by frame and track by track in creation
// order, the tracked filter's state and covariance and every number and name
// the solid body's reading publishes.
func solidBodyFingerprint(t *testing.T, cfg TrackerConfig, frames []syntheticPassFrame) string {
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
			writeFloats(h, tr.P[:]...)
			r, ok := tr.SolidBody()
			if !ok {
				writeInt(h, -1)
				continue
			}
			e, m := r.Estimate, r.Measurement
			writeInt(h, int64(e.Reference), int64(e.Estimation), int64(e.Support.CoastedFrames), int64(e.Support.Instant))
			writeFloats(h, e.X, e.Y, r.VX, r.VY)
			writeFloats(h, e.PositionCovariance[:]...)
			writeFloats(h, r.Covariance[:]...)
			writeFloats(h, e.Orientation.PsiRad, e.Orientation.VarianceRad2, e.Orientation.AmbiguousModeWeight)
			for _, d := range []DimensionBelief{e.Length, e.Width, e.Height} {
				writeFloats(h, d.Metres, d.SigmaMetres)
				writeInt(h, int64(d.AdmissibleFrames), int64(d.Provenance))
			}
			writeInt(h, int64(m.Rank), int64(m.Faces), int64(m.ReferenceChange), int64(m.ContainedPoints))
			writeFloats(h, m.NIS, m.AspectRad, m.ContainmentShare, m.ObservedSpanAlongMetres, m.ObservedSpanAcrossMetres,
				m.ContainmentShiftAlongMetres, m.ContainmentShiftAcrossMetres, m.RectangleAxisRad, m.RectangleSigmaRad)
			h.Write([]byte(string(m.Source) + "|" + m.FallbackReason + "|" + m.ExtentFloor + "|" + m.RectangleAbstain + "\n"))
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func writeFloats(h hash.Hash, v ...float32) {
	var b [4]byte
	for _, x := range v {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(x))
		h.Write(b[:])
	}
}

func writeInt(h hash.Hash, v ...int64) {
	var b [8]byte
	for _, x := range v {
		binary.LittleEndian.PutUint64(b[:], uint64(x))
		h.Write(b[:])
	}
}

func TestFaceStateOptionsOffLeaveTheShippedArithmetic(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skipf("digests are pinned on amd64; %s may fuse multiply-adds", runtime.GOARCH)
	}
	for _, s := range solidBodyFingerprintScenarios() {
		got := solidBodyFingerprint(t, s.config(), syntheticPassFrames(t, s.pass()))
		if got != s.golden {
			t.Errorf("%s: digest %s, want %s", s.name, got, s.golden)
		}
	}
}
