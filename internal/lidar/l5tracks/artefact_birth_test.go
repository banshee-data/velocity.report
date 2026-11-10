package l5tracks

import (
	"hash/fnv"
	"math"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// artefactScene is a fixed scene built to reach every site the artefact
// options touch, at 10 Hz for 60 frames from a fixed capture time. Each
// object stands far enough from the others that a tentative track's wide
// first gate reaches no other object's cluster.
//
//   - vehicle: 4 x 1.8 m, 120 points, along y = 40 from x = -30 at 8 m/s;
//   - parked: a stopped car, 4.2 x 1.9 m, 150 points, at (10, 0) throughout;
//   - flicker: a small sparse artefact, 0.5 x 0.3 m, 10 points, at (-30, 0)
//     on frames 10 to 15 only;
//   - standing: a small sparse object that persists, 0.9 x 0.6 m, 14 points,
//     at (-20, -30) on frames 5 to 50, a person standing still;
//   - walker: 0.6 x 0.5 m, 20 points, from (30, -30) along +y at 1.3 m/s;
//   - dense: a small object with many returns, 1.0 x 0.8 m, 60 points, at
//     (0, -40) from frame 30;
//   - fragment: 0.6 x 0.4 m, 10 points, 2 m to the vehicle's side (1.1 m
//     beyond its 1.8 m width) on frames 20 to 27.
type artefactObject struct {
	name          string
	from, to      int
	x, y, vx, vy  float32
	length, width float32
	points        int
}

var artefactSceneObjects = []artefactObject{
	{name: "vehicle", from: 0, to: 59, x: -30, y: 40, vx: 8, length: 4, width: 1.8, points: 120},
	{name: "parked", from: 0, to: 59, x: 10, y: 0, length: 4.2, width: 1.9, points: 150},
	{name: "flicker", from: 10, to: 15, x: -30, y: 0, length: 0.5, width: 0.3, points: 10},
	{name: "standing", from: 5, to: 50, x: -20, y: -30, length: 0.9, width: 0.6, points: 14},
	{name: "walker", from: 0, to: 59, x: 30, y: -30, vy: 1.3, length: 0.6, width: 0.5, points: 20},
	{name: "dense", from: 30, to: 59, x: 0, y: -40, length: 1.0, width: 0.8, points: 60},
	{name: "fragment", from: 20, to: 27, x: -30, y: 42, vx: 8, length: 0.6, width: 0.4, points: 10},
}

const artefactSceneFrames = 60

var artefactSceneStart = time.Unix(1_700_000_000, 0)

func artefactSceneTime(i int) time.Time {
	return artefactSceneStart.Add(time.Duration(i) * 100 * time.Millisecond)
}

// artefactSceneClusters is frame i's clusters, in a fixed order. With obb
// each cluster also carries its PCA box along x, so a drawn box exists for
// the overlap tests and the box model to read.
func artefactSceneClusters(i int, obb bool) []WorldCluster {
	now := artefactSceneTime(i)
	ts := 0.1 * float32(i)
	var out []WorldCluster
	for _, o := range artefactSceneObjects {
		if i < o.from || i > o.to {
			continue
		}
		c := WorldCluster{
			CentroidX: o.x + o.vx*ts, CentroidY: o.y + o.vy*ts, CentroidZ: 0.8, SensorID: "test",
			BoundingBoxLength: o.length, BoundingBoxWidth: o.width, BoundingBoxHeight: 1.4,
			PointsCount: o.points, TSUnixNanos: now.UnixNano(),
		}
		if obb {
			c.OBB = &l4perception.OrientedBoundingBox{CenterX: c.CentroidX, CenterY: c.CentroidY, CenterZ: 0.8,
				Length: o.length, Width: o.width, Height: 1.4}
		}
		out = append(out, c)
	}
	return out
}

// artefactScene runs the scene and returns a digest of every live track's
// state, covariance, counts and lifecycle state after every frame, and the
// tracker. Tracks are visited in creation order, so the digest depends only
// on the arithmetic and the lifecycle decisions.
func artefactScene(t *testing.T, cfg TrackerConfig, obb bool) (uint64, *Tracker) {
	t.Helper()
	tr := NewTracker(cfg)
	h := fnv.New64a()
	put := func(v float32) {
		b := math.Float32bits(v)
		_, _ = h.Write([]byte{byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24)})
	}
	for i := 0; i < artefactSceneFrames; i++ {
		tr.Update(artefactSceneClusters(i, obb), artefactSceneTime(i))
		tracks := make([]*TrackedObject, 0, len(tr.Tracks))
		for _, track := range tr.Tracks {
			tracks = append(tracks, track)
		}
		sort.Slice(tracks, func(a, b int) bool { return tracks[a].CreationSequence < tracks[b].CreationSequence })
		for _, track := range tracks {
			put(float32(track.CreationSequence))
			put(float32(track.TrackState[0]))
			put(track.X)
			put(track.Y)
			put(track.VX)
			put(track.VY)
			for _, v := range track.P {
				put(v)
			}
			put(float32(track.Hits))
			put(float32(track.Misses))
			put(track.BoxX)
			put(track.BoxY)
			put(track.BoxLength)
			put(track.BoxWidth)
		}
	}
	return h.Sum64(), tr
}

// artefactTuningConfig is the tracker the replays run: the tuning defaults'
// L5 block, with the box model on.
func artefactTuningConfig() TrackerConfig {
	return TrackerConfigFromTuning(config.MustLoadDefaultConfig().L5.CvKfV1)
}

// The scene's digests as the tracker computed them before the artefact
// options existed (origin/main at 704b8e46). They pin the shipped arithmetic
// and lifecycle to the bit. They are amd64's: Go may fuse a multiply and add
// on arm64, which changes float results there, so elsewhere the test asserts
// only what does not depend on fusion.
const (
	artefactGoldenDefault = 0x91e3e613a40d371e // DefaultTrackerConfig, clusters without a PCA box
	artefactGoldenTuning  = 0xdde991eea69e568d // the tuning defaults, clusters with a PCA box
)

func TestArtefactOptionsOffKeepTheShippedTracker(t *testing.T) {
	plain, tr := artefactScene(t, DefaultTrackerConfig(), false)
	tuned, tuning := artefactScene(t, artefactTuningConfig(), true)
	t.Logf("digests: default=%#x (created %d, confirmed %d) tuning=%#x (created %d, confirmed %d)",
		plain, tr.TracksCreated, tr.TracksConfirmed, tuned, tuning.TracksCreated, tuning.TracksConfirmed)
	if runtime.GOARCH == "amd64" {
		if plain != artefactGoldenDefault {
			t.Errorf("default digest %#x, want the shipped %#x", plain, uint64(artefactGoldenDefault))
		}
		if tuned != artefactGoldenTuning {
			t.Errorf("tuning digest %#x, want the shipped %#x", tuned, uint64(artefactGoldenTuning))
		}
	}
	if n := len(artefactSceneObjects); tr.TracksCreated != n || tr.TracksConfirmed != n {
		t.Fatalf("the shipped tracker made %d tracks and confirmed %d, want %d of each: the scene no longer reaches the sites it is built for",
			tr.TracksCreated, tr.TracksConfirmed, n)
	}
}

// artefactFates runs the scene and returns, for each object, the frame its
// track first confirmed (absent: it never did) and whether a track was born
// from it. A track is the object's whose first point lies within a metre of
// the object's first position.
func artefactFates(t *testing.T, cfg TrackerConfig, obb bool) (map[string]int, map[string]bool, *Tracker) {
	t.Helper()
	tr := NewTracker(cfg)
	confirmed, born := map[string]int{}, map[string]bool{}
	owner := func(track *TrackedObject) string {
		if len(track.History) == 0 {
			return ""
		}
		p := track.History[0]
		for _, o := range artefactSceneObjects {
			ts := 0.1 * float32(o.from)
			if math.Hypot(float64(p.X-(o.x+o.vx*ts)), float64(p.Y-(o.y+o.vy*ts))) < 1 {
				return o.name
			}
		}
		return ""
	}
	for i := 0; i < artefactSceneFrames; i++ {
		tr.Update(artefactSceneClusters(i, obb), artefactSceneTime(i))
		for _, track := range tr.Tracks {
			name := owner(track)
			if name == "" {
				continue
			}
			born[name] = true
			if _, seen := confirmed[name]; !seen && track.TrackState == TrackConfirmed {
				confirmed[name] = i
			}
		}
	}
	return confirmed, born, tr
}

// The birth gate holds the small, static, sparse and young, and nothing
// else: the flicker never confirms, the standing object confirms once it has
// persisted a second, the walker once it has moved half a metre, and the
// vehicle, the stopped car, the dense object and the moving fragment confirm
// on the frame they always did.
func TestArtefactBirthGateHoldsOnlyTheSmallStaticSparseAndYoung(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  func() TrackerConfig
		obb  bool
	}{
		{"default", DefaultTrackerConfig, false},
		{"tuning", artefactTuningConfig, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shipped, _, _ := artefactFates(t, tc.cfg(), tc.obb)
			for _, o := range artefactSceneObjects {
				if want := o.from + tc.cfg().HitsToConfirm - 1; shipped[o.name] != want {
					t.Fatalf("shipped: %s confirmed on frame %d, want %d; the scene no longer reaches the gate's sites", o.name, shipped[o.name], want)
				}
			}
			cfg := tc.cfg()
			cfg.ArtefactBirthGate = true
			got, born, tr := artefactFates(t, cfg, tc.obb)
			// The fragment rides the vehicle at 8 m/s, so it is not static
			// and the gate does not reach it: that is absorb's case.
			for _, name := range []string{"vehicle", "parked", "dense", "fragment"} {
				if got[name] != shipped[name] {
					t.Errorf("%s confirmed on frame %d, want the shipped %d: the gate held what is not an artefact", name, got[name], shipped[name])
				}
			}
			if !born["flicker"] {
				t.Error("no track was born from the flicker: the gate must hold, not refuse a birth")
			}
			if f, ok := got["flicker"]; ok {
				t.Errorf("the flicker confirmed on frame %d; it lives under a second and should never confirm", f)
			}
			// The standing object is first seen on frame 5 and has persisted
			// a second on frame 15.
			if got["standing"] != 15 {
				t.Errorf("standing object confirmed on frame %d, want 15, when it has persisted a second", got["standing"])
			}
			// The walker passes 0.5 m from where it was first seen between
			// frames 3 (0.39 m) and 4 (0.52 m).
			if got["walker"] != 4 {
				t.Errorf("walker confirmed on frame %d, want 4, once it has moved half a metre", got["walker"])
			}
			s := tr.ContinuityStats()
			if s.ArtefactBirthsHeld != 3 || s.ArtefactBirthsReleased != 2 || s.ArtefactClustersAbsorbed != 0 {
				t.Errorf("held %d released %d absorbed %d, want 3, 2 and 0",
					s.ArtefactBirthsHeld, s.ArtefactBirthsReleased, s.ArtefactClustersAbsorbed)
			}
		})
	}
}

// Absorb seeds no track from the small sparse cluster beside a live
// confirmed vehicle, and every other birth is the shipped tracker's,
// including the small sparse objects that stand alone.
func TestArtefactAbsorbTakesOnlyTheClusterBesideALiveBody(t *testing.T) {
	shipped, _, _ := artefactFates(t, artefactTuningConfig(), true)
	cfg := artefactTuningConfig()
	cfg.ArtefactAbsorb = true
	got, born, tr := artefactFates(t, cfg, true)
	if born["fragment"] {
		t.Error("a track was born from the fragment beside the vehicle")
	}
	for _, o := range artefactSceneObjects {
		if o.name == "fragment" {
			continue
		}
		if got[o.name] != shipped[o.name] {
			t.Errorf("%s confirmed on frame %d, want the shipped %d", o.name, got[o.name], shipped[o.name])
		}
	}
	// Frames 20 to 27, each absorbed; the vehicle confirmed on frame 3.
	if n := tr.ContinuityStats().ArtefactClustersAbsorbed; n != 8 {
		t.Errorf("%d clusters absorbed, want 8", n)
	}
}

// absorbArtefact's refusals, one at a time: a cluster as large as a car on
// one side, one with many returns, one beyond the margin, and no live body.
func TestArtefactAbsorbRefusesTheLargeTheDenseAndTheDistant(t *testing.T) {
	tr := NewTracker(DefaultTrackerConfig())
	live := []drawnRect{{id: "car", x: 0, y: 0, length: 4, wd: 1.8}}
	beside := WorldCluster{CentroidX: 0, CentroidY: 0.9 + 1.2, BoundingBoxLength: 0.6, BoundingBoxWidth: 0.4, PointsCount: 10}
	if !tr.absorbArtefact(&beside, live) {
		t.Fatal("a small sparse cluster 1.2 m beyond the side was not absorbed")
	}
	for name, mutate := range map[string]func(*WorldCluster){
		"long":    func(c *WorldCluster) { c.BoundingBoxLength = 1.5 },
		"wide":    func(c *WorldCluster) { c.BoundingBoxWidth = 1.5 },
		"dense":   func(c *WorldCluster) { c.PointsCount = 24 },
		"distant": func(c *WorldCluster) { c.CentroidY = 0.9 + 1.3 },
	} {
		c := beside
		mutate(&c)
		if tr.absorbArtefact(&c, live) {
			t.Errorf("%s cluster was absorbed", name)
		}
	}
	if tr.absorbArtefact(&beside, nil) {
		t.Error("absorbed with no live body")
	}
}
