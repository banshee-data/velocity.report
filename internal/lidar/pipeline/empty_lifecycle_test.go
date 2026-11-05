package pipeline

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func seededLifecycleTracker(t *testing.T) (*l5tracks.Tracker, string, time.Time) {
	t.Helper()
	cfg := l5tracks.DefaultTrackerConfig()
	cfg.HitsToConfirm, cfg.MaxMissesConfirmed = 3, 4
	tk := l5tracks.NewTracker(cfg)
	base := time.Unix(1_700_000_000, 0)
	for i := 0; i < 3; i++ {
		tk.Update([]l5tracks.WorldCluster{{ClusterID: int64(i), SensorID: "empty-lifecycle", CentroidX: 10, PointsCount: 20}}, base.Add(time.Duration(i)*100*time.Millisecond))
	}
	tracks := tk.GetConfirmedTracks()
	if len(tracks) != 1 {
		t.Fatalf("seeded %d confirmed tracks, want 1", len(tracks))
	}
	id := tracks[0].TrackID
	tk.Tracks[id].VX = 2
	return tk, id, base.Add(200 * time.Millisecond)
}

// Empty frames must move the prediction at capture time and eventually expire
// it, with no observed point or detection added to the track.
func TestEmptyFrameLifecycleAcrossEarlyReturns(t *testing.T) {
	for _, kind := range []string{"zero-points", "missing-model", "no-foreground", "ground-only", "no-clusters"} {
		t.Run(kind, func(t *testing.T) {
			tk, id, last := seededLifecycleTracker(t)
			before := tk.GetTrack(id)
			cfg := &TrackingPipelineConfig{SensorID: "empty-" + t.Name(), Tracker: tk}
			if kind != "missing-model" && kind != "zero-points" {
				cfg.BackgroundManager = makeTestBgManager(t, cfg.SensorID)
				// Seed the model before handing it the already-confirmed tracker.
				seed := *cfg
				seed.Tracker = nil
				seed.NewFrameCallback()(makeStableFrame("seed", last, 20))
			}
			if kind == "ground-only" {
				cfg.RemoveGround, cfg.HeightBandFloor, cfg.HeightBandCeiling = true, 100, 200
			}
			if kind == "no-clusters" {
				params := cfg.BackgroundManager.GetParams()
				params.ForegroundMinClusterPoints = 1000
				if err := cfg.BackgroundManager.SetParams(params); err != nil {
					t.Fatal(err)
				}
			}
			cb := cfg.NewFrameCallback()
			for i := 1; i <= 4; i++ {
				ts := last.Add(time.Duration(i) * 100 * time.Millisecond)
				frame := makeStableFrame("empty", ts, 20)
				if kind == "zero-points" {
					frame = &l2frames.LiDARFrame{FrameID: "gap", StartTimestamp: ts}
				} else if kind == "ground-only" || kind == "no-clusters" {
					frame = makeForegroundFrame("rejected", ts, 20, 5)
				}
				cb(frame)
				got := tk.GetTrack(id)
				if got.ObservationCount != before.ObservationCount || got.LastObservedUnixNanos != before.LastObservedUnixNanos {
					t.Fatal("empty frame invented an observation")
				}
				if i < 4 {
					if got.Misses != i || got.StateUnixNanos != ts.UnixNano() || math.Abs(float64(got.X-(10+0.2*float32(i)))) > 1e-4 {
						t.Fatalf("prediction did not advance once at capture time: %+v", got)
					}
					want := l5tracks.SupportCoasted
					if kind == "zero-points" || kind == "missing-model" {
						want = l5tracks.SupportMissedUnknown
					}
					if got.LastSupport != want || got.History[len(got.History)-1].Support != want {
						t.Fatalf("support = %v, want %v", got.LastSupport, want)
					}
				} else if got.TrackState != l5tracks.TrackDeleted || got.ExpiryReason != l5tracks.ExpiryMisses {
					t.Fatalf("sustained empty input did not expire: %+v", got)
				}
			}
		})
	}
}

func TestEmptyLifecycleHonoursPipelineDepth(t *testing.T) {
	for _, profile := range []config.Profile{config.ProfileL3Only, config.ProfileDetect} {
		t.Run(string(profile), func(t *testing.T) {
			tk, id, last := seededLifecycleTracker(t)
			cfg := &TrackingPipelineConfig{Tracker: tk, Profile: profile}
			cfg.NewFrameCallback()(&l2frames.LiDARFrame{StartTimestamp: last.Add(time.Second)})
			if got := tk.GetTrack(id); got.Misses != 0 || got.StateUnixNanos != last.UnixNano() {
				t.Fatal("a profile without L5 advanced the tracker")
			}
		})
	}
}
