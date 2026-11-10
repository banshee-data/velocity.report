package l5tracks

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// believedBox is a labelled box heading psi, updated on the frame at now,
// with a standard deviation of sigmaDeg.
func believedBox(psi, sigmaDeg float64, now int64) *BoxEstimator {
	s := sigmaDeg * math.Pi / 180
	return &BoxEstimator{seeded: true, labelled: true, psi: psi, p: [2][2]float64{{s * s, 0}, {0, 1e-4}}, lastNanos: now}
}

// anchoredTruck is the through-truck's body, moving along +X at speed, with
// its half-extents in the joint state and a cross-covariance that says the
// rear face is held: a change dh of the half-length moves the centre by dh
// along the axis (the regression P_x,hL / P_hL,hL is one).
func anchoredTruck(speed float32) *solidBodyTrack {
	sb := profiledBody(0, 0)
	sb.state[2] = speed
	sb.p[2*4+2], sb.p[3*4+3] = 0.01, 0.01
	sb.ext = solidBodyExtent{on: true, h: [2]float32{4.8, 1.25}, phh: [4]float32{0.04, 0, 0, 0.04}}
	sb.ext.pxh[0] = 0.04
	return sb
}

// throughTruckProfiles runs the through-truck pass through geom0019's profile
// and the anchored one, with the solid body's centre the true centre until
// the midpoint frame. There the extent state shortens the truck by shorten
// metres with its rear face held, as a face or bound would through the joint
// state: the half-length falls by shorten/2 and the centre moves back by the
// same along the axis. It returns each profile's reading per frame.
func throughTruckProfiles(t *testing.T, shorten float64) (plain, anchored []ExtentProfileReading, mid int) {
	t.Helper()
	frames := l4perception.GenerateSyntheticPass(throughTruckPass())
	mid = len(frames) / 2
	speed := float32(throughTruckPass().Vehicle.SpeedMps)
	plainBody, track := anchoredTruck(speed), &TrackedObject{}
	track.solidBody = *anchoredTruck(speed)
	pp, pa := &extentProfile{}, &extentProfile{}
	for i, f := range frames {
		if len(f.Points) == 0 {
			continue
		}
		now := f.Timestamp.UnixNano()
		x, y := float32(f.TrueCentreX), float32(f.TrueCentreY)
		h := float32(4.8)
		if i >= mid {
			x -= float32(shorten / 2)
			h -= float32(shorten / 2)
		}
		for _, sb := range []*solidBodyTrack{plainBody, &track.solidBody} {
			sb.state[0], sb.state[1] = x, y
			sb.ext.h[0] = h
		}
		track.box = believedBox(0, 1, now)
		cluster := WorldCluster{Members: f.Points, PointsCount: len(f.Points)}
		frame := newProfileFrame([]WorldCluster{cluster}, 0, 0)
		plain = append(plain, pp.step(plainBody, cluster, frame, now))
		anchored = append(anchored, pa.stepAnchored(track, cluster, frame, now))
	}
	return plain, anchored, mid
}

// The through-truck pass with the extent state shortening the truck by 3 m
// at its midpoint, abeam: geom0019's profile, whose origin is the solid
// body's centre, writes the centre's move as length, and the anchored
// profile, whose origin takes the move back, keeps the truck's 9.6 m: from
// the midpoint on its extents are those of the same pass without the change,
// frame for frame. Its reported pose is the origin it holds, the true centre.
// Within 0.2 m is inclusive: the cells are 0.1 m, and the profile reads this
// truck as 9.7 to 9.8 m with or without the change.
func TestExtentProfileAnchoredHoldsLengthWhenTheExtentStateShortens(t *testing.T) {
	const length, tolerance = 9.6, 0.2 + 1e-5
	plain, anchored, mid := throughTruckProfiles(t, 3)
	_, unchanged, _ := throughTruckProfiles(t, 0)
	locked, smear := 0, float32(0)
	for i := mid; i < len(anchored); i++ {
		r := anchored[i]
		smear = max(smear, plain[i].LengthMetres)
		if !r.Known {
			t.Errorf("anchored: frame %d not known", i)
			continue
		}
		locked++
		if math.Abs(float64(r.LengthMetres)-length) > tolerance {
			t.Errorf("anchored: frame %d length %.2f m, want %.1f within 0.2", i, r.LengthMetres, length)
		}
		u := unchanged[i]
		if r.AlongLo != u.AlongLo || r.AlongHi != u.AlongHi || r.AcrossLo != u.AcrossLo || r.AcrossHi != u.AcrossHi {
			t.Errorf("anchored: frame %d extents [%.2f, %.2f] x [%.2f, %.2f], without the change [%.2f, %.2f] x [%.2f, %.2f]",
				i, r.AlongLo, r.AlongHi, r.AcrossLo, r.AcrossHi, u.AlongLo, u.AlongHi, u.AcrossLo, u.AcrossHi)
		}
	}
	if locked == 0 {
		t.Fatal("anchored: no locked frame after the midpoint")
	}
	if smear < length+1 {
		t.Errorf("geom0019's profile: longest %.2f m after the midpoint; the scenario should smear it past %.1f m", smear, length+1)
	}
	frames := l4perception.GenerateSyntheticPass(throughTruckPass())
	if got, want := float64(anchored[len(anchored)-1].X), frames[len(frames)-1].TrueCentreX; math.Abs(got-want) > 1e-3 {
		t.Errorf("anchored origin x %.3f, want the true centre %.3f", got, want)
	}
}

// With no change of the extent state the two profiles are the same reading
// frame for frame, origin included: anchoring changes nothing it need not.
func TestExtentProfileAnchoredIsTheProfileWithoutAnExtentChange(t *testing.T) {
	plain, anchored, _ := throughTruckProfiles(t, 0)
	for i := range plain {
		if plain[i] != anchored[i] {
			t.Fatalf("frame %d: plain %+v, anchored %+v", i, plain[i], anchored[i])
		}
	}
}

// The anchored profile's axis is the box's heading filter: a car whose box
// heading has a standard deviation of 30 degrees is refused, as a heading
// refusal, however settled the solid body's own orientation is; at 1 degree
// it is admitted however unsettled the body's orientation is. An unlabelled
// box, or one not updated on this frame, is not a heading, and a body not
// referenced to its centre is refused for that reason, not the heading's.
func TestExtentProfileAnchoredAdmitsOnTheBoxHeading(t *testing.T) {
	at := int64(1_700_000_000_000_000_000)
	cluster := staticView(t, 4.5, 1.8, 1.5, -6, 8)
	track := &TrackedObject{}
	track.solidBody = *profiledBody(-6, 8)
	p := &extentProfile{}
	step := func() ExtentProfileReading {
		at += int64(100 * time.Millisecond)
		return p.stepAnchored(track, cluster, newProfileFrame([]WorldCluster{cluster}, 0, 0), at)
	}

	track.box = believedBox(0, 30, at+int64(100*time.Millisecond))
	if r := step(); r.Admitted || r.RefusedFrames != 1 || r.HeadingRefusedFrames != 1 {
		t.Errorf("box at 30 degrees: admitted %v, refused %d, heading refused %d; want a heading refusal", r.Admitted, r.RefusedFrames, r.HeadingRefusedFrames)
	}
	track.solidBody.orientation.VarianceRad2 = 1 // the body's own heading unsettled
	track.box = believedBox(0, 1, at+int64(100*time.Millisecond))
	if r := step(); !r.Admitted || r.PsiRad != 0 {
		t.Errorf("box at 1 degree: admitted %v along %.3f; want admitted along the box's 0", r.Admitted, r.PsiRad)
	}
	track.box = believedBox(0.2, 1, at+int64(100*time.Millisecond))
	track.box.labelled = false
	if r := step(); r.Admitted || r.HeadingRefusedFrames != 2 {
		t.Errorf("unlabelled box: admitted %v, heading refused %d", r.Admitted, r.HeadingRefusedFrames)
	}
	track.box = believedBox(0, 1, at) // last frame's
	if r := step(); r.Admitted || r.HeadingRefusedFrames != 3 {
		t.Errorf("box not updated this frame: admitted %v, heading refused %d", r.Admitted, r.HeadingRefusedFrames)
	}
	track.solidBody.reference = ReferenceClusterMedoid
	track.box = believedBox(0, 1, at+int64(100*time.Millisecond))
	if r := step(); r.Admitted || r.RefusedFrames != 4 || r.HeadingRefusedFrames != 3 {
		t.Errorf("medoid-referenced: admitted %v, refused %d, heading refused %d", r.Admitted, r.RefusedFrames, r.HeadingRefusedFrames)
	}
}

// A change of reference moves the centre by an amount no state accounts
// for: the origin follows the body's velocity over the interval and keeps
// the rest of the jump in its offset.
func TestExtentProfileAnchoredFollowsVelocityAcrossAReferenceChange(t *testing.T) {
	p := &extentProfile{axisSet: true}
	sb := profiledBody(10, 0)
	sb.state[2] = 5
	at := int64(1_700_000_000_000_000_000)
	if x, _ := p.follow(sb, 0, at); x != 10 {
		t.Fatalf("first step: origin %.3f, want the centre 10", x)
	}
	sb.state[0] = 10.5 + 2 // 0.1 s at 5 m/s, and a 2 m re-reference
	sb.reference = ReferenceClusterMedoid
	x, y := p.follow(sb, 0, at+int64(100*time.Millisecond))
	if math.Abs(float64(x)-10.5) > 1e-5 || y != 0 {
		t.Errorf("origin (%.3f, %.3f), want (10.5, 0)", x, y)
	}
	// The half-extent regression: P_ch P_hh^-1 dh, both dimensions.
	e := solidBodyExtent{on: true, phh: [4]float32{0.04, 0, 0, 0.01}, pxh: [8]float32{0.02, 0, 0, -0.01}}
	if sx, sy := extentShift(e, 1, 0.5); math.Abs(sx-0.5) > 1e-6 || math.Abs(sy+0.5) > 1e-6 {
		t.Errorf("shift (%.3f, %.3f), want (0.5, -0.5)", sx, sy)
	}
}

// With the anchored option on, the tracks and every bit the solid body
// publishes are the off digests, and the profile admits frames on the
// through-truck pass under the face state, where the box labels and settles.
func TestExtentProfileAnchoredChangesNoTrack(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skipf("digests are pinned on amd64; %s may fuse multiply-adds", runtime.GOARCH)
	}
	scenarios := append(extentProfileFingerprintScenarios(), solidBodyFingerprintScenario{
		name: "profile_course_heading_through_truck",
		pass: throughTruckPass,
		config: func() TrackerConfig {
			cfg := faceStateConfig()
			cfg.SolidBody.CourseHeading = true
			return cfg
		},
		golden: profileCourseHeadingGolden,
	})
	admitted, known := 0, 0
	for _, s := range scenarios {
		cfg := s.config()
		cfg.SolidBody.ExtentProfileAnchored = true
		frames := syntheticPassFrames(t, s.pass())
		if got := solidBodyFingerprint(t, cfg, frames); got != s.golden {
			t.Errorf("%s with the anchored profile on: digest %s, want %s", s.name, got, s.golden)
		}
		tracker := NewTracker(cfg)
		for _, f := range frames {
			tracker.Update(f.clusters, f.at)
			for _, tr := range tracker.GetActiveTracks() {
				if r, ok := tr.SolidBody(); ok && r.Profile.Admitted {
					admitted++
					if r.Profile.Known {
						known++
					}
				}
			}
		}
	}
	if admitted == 0 || known == 0 {
		t.Errorf("admitted %d frames, %d known: the anchored profile never ran", admitted, known)
	}
}

// profileReadingFingerprint hashes every track's Profile reading on every
// frame: what solid_body_extent_profile reports, which the solid-body digest
// leaves out.
func profileReadingFingerprint(t *testing.T, cfg TrackerConfig, frames []syntheticPassFrame) string {
	t.Helper()
	h := sha256.New()
	tracker := NewTracker(cfg)
	for _, f := range frames {
		tracker.Update(f.clusters, f.at)
		tracks := tracker.GetAllTracks()
		sort.Slice(tracks, func(i, j int) bool { return tracks[i].CreationSequence < tracks[j].CreationSequence })
		for _, tr := range tracks {
			r, ok := tr.SolidBody()
			if !ok {
				continue
			}
			p := r.Profile
			writeInt(h, tr.CreationSequence, p.FrameUnixNanos, p.LockedUnixNanos, int64(p.AdmittedFrames), int64(p.RefusedFrames),
				int64(p.WithheldFrames), int64(p.HeadingRefusedFrames))
			writeFloats(h, p.AlongLo, p.AlongHi, p.AcrossLo, p.AcrossHi, p.LengthMetres, p.WidthMetres, p.X, p.Y, p.PsiRad)
			for _, b := range []bool{p.Known, p.RearObserved, p.FrontObserved, p.RightObserved, p.LeftObserved, p.Admitted, p.Withheld} {
				if b {
					writeInt(h, 1)
				} else {
					writeInt(h, 0)
				}
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// geom0019's profile reads as it did before the anchored option shared its
// code: these digests of its readings were taken at 39705e4, before the
// change, on the scenarios in which it admits frames.
func TestExtentProfileReadingsAreUnchangedByTheAnchoredOption(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skipf("digests are pinned on amd64; %s may fuse multiply-adds", runtime.GOARCH)
	}
	for _, s := range []struct {
		name   string
		config func() TrackerConfig
		golden string
	}{
		{"course_heading_through_truck", func() TrackerConfig {
			cfg := faceStateConfig()
			cfg.SolidBody.CourseHeading = true
			return cfg
		}, profileReadingsCourseHeadingGolden},
		{"face_state_through_truck", faceStateConfig, profileReadingsFaceStateGolden},
	} {
		cfg := s.config()
		cfg.SolidBody.ExtentProfile = true
		if got := profileReadingFingerprint(t, cfg, syntheticPassFrames(t, throughTruckPass())); got != s.golden {
			t.Errorf("%s: profile readings digest %s, want %s", s.name, got, s.golden)
		}
	}
}

const (
	profileReadingsCourseHeadingGolden = "e7ef89824229c9c1"
	profileReadingsFaceStateGolden     = "c0ad2ca374f4ae4d"
)
