package replayeval

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

type campaignFake struct {
	l5tracks.TrackerInterface
	active, confirmed []*l5tracks.TrackedObject
	calls             int
	foreground        []int
}

func (f *campaignFake) RecordFrameStats(foreground, _ int) {
	f.foreground = append(f.foreground, foreground)
}

func (f *campaignFake) Update(_ []l5tracks.WorldCluster, _ time.Time) { f.calls++ }
func (f *campaignFake) GetActiveTracks() []*l5tracks.TrackedObject    { return f.active }
func (f *campaignFake) GetConfirmedTracks() []*l5tracks.TrackedObject { return f.confirmed }
func TestCampaignIntervalsCensorAndUseSupportTime(t *testing.T) {
	c := &campaignTracker{left: map[int64]bool{1: true}, intervals: map[int64]*confirmationInterval{}}
	a := &l5tracks.TrackedObject{CreationSequence: 1, LastObservedUnixNanos: 10e9}
	b := &l5tracks.TrackedObject{CreationSequence: 2, LastObservedUnixNanos: 10e9}
	d := &l5tracks.TrackedObject{CreationSequence: 3, LastObservedUnixNanos: 10e9}
	c.observe([]*l5tracks.TrackedObject{a, b, d}, 10e9)
	b.LastObservedUnixNanos = 14e9
	c.observe([]*l5tracks.TrackedObject{a, b, d}, 14e9)
	c.observe([]*l5tracks.TrackedObject{d}, 20e9)
	out := t.TempDir()
	if err := c.write(defaultRuntime(), out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "confirmed_duration.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Mean     float64 `json:"mean_confirmed_duration_seconds"`
		Complete int     `json:"complete_count"`
		Left     int     `json:"left_censored_count"`
		Right    int     `json:"right_censored_count"`
	}
	if err = json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Mean != 4 || report.Complete != 1 || report.Left != 1 || report.Right != 1 {
		t.Fatalf("wrong durations/censoring: %+v", report)
	}
	if c.intervals[2].LastSupportedAt != 14e9 {
		t.Fatal("coast time counted as support")
	}
}
func TestCampaignWrapperForwardsWarmupAndScoredUpdates(t *testing.T) {
	fake := &campaignFake{active: []*l5tracks.TrackedObject{{CreationSequence: 7}}}
	c := &campaignTracker{TrackerInterface: fake, start: 10e9, left: map[int64]bool{}, intervals: map[int64]*confirmationInterval{}}
	c.Update(nil, time.Unix(9, 0))
	c.Update(nil, time.Unix(10, 0))
	if fake.calls != 2 || len(c.costs) != 1 || !c.left[7] {
		t.Fatal("update forwarding, timing window or censor boundary failed")
	}
	if err := c.write(defaultRuntime(), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("write error hidden")
	}
}
func TestCampaignQuantileDoesNotMutateAndHandlesEmpty(t *testing.T) {
	v := []float64{9, 1, 3, 2}
	if campaignQuantile(v, .5) != 2 || campaignQuantile(v, .99) != 9 || v[0] != 9 || campaignQuantile(nil, .99) != 0 {
		t.Fatal("wrong nearest-rank quantile")
	}
}

func (f *campaignFake) AdvanceMisses(_ time.Time) { f.confirmed = nil }
func TestCampaignMissOnlyExpiryClosesInterval(t *testing.T) {
	fake := &campaignFake{confirmed: []*l5tracks.TrackedObject{{CreationSequence: 2, LastObservedUnixNanos: 12e9}}}
	c := &campaignTracker{TrackerInterface: fake, start: 10e9, left: map[int64]bool{}, intervals: map[int64]*confirmationInterval{}}
	c.observe(fake.confirmed, 10e9)
	c.AdvanceMisses(time.Unix(20, 0))
	if c.intervals[2].RightCensored || c.intervals[2].LastSupportedAt != 12e9 {
		t.Fatal("miss-only expiry was censored or extended support")
	}
}

func TestCampaignWritePropagatesExportFailures(t *testing.T) {
	for _, name := range []string{"confirmed_duration.json", "tracker_timing.json", "frame_timing.json", "dbscan_input.json"} {
		t.Run(name, func(t *testing.T) {
			failure := errors.New("disk full")
			out := t.TempDir()
			runtime := defaultRuntime()
			runtime.writeFile = func(path string, data []byte, mode os.FileMode) error {
				if filepath.Dir(path) != out || mode != 0644 || !json.Valid(data) || data[len(data)-1] != '\n' {
					t.Fatalf("invalid export: path=%q mode=%v data=%q", path, mode, data)
				}
				if filepath.Base(path) == name {
					return failure
				}
				return nil
			}
			if err := (&campaignTracker{}).write(runtime, out); !errors.Is(err, failure) {
				t.Fatalf("write failure not propagated: %v", err)
			}
		})
	}
}

func TestCampaignWriteRejectsNonFiniteTiming(t *testing.T) {
	c := &campaignTracker{costs: []float64{math.NaN()}}
	var unsupported *json.UnsupportedValueError
	if err := c.write(defaultRuntime(), t.TempDir()); !errors.As(err, &unsupported) {
		t.Fatalf("invalid timing was not rejected: %v", err)
	}
}

// Only frames starting inside the scored window are timed.
func TestCampaignRecordsScoredFramesOnly(t *testing.T) {
	c := &campaignTracker{start: 10e9}
	c.recordFrame(9e9, 0.5)
	c.recordFrame(10e9, 0.01)
	c.recordFrame(11e9, 0.02)
	if len(c.frameCosts) != 2 || c.frameCosts[0] != 0.01 || c.frameCosts[1] != 0.02 {
		t.Fatalf("frame costs %v, want the two scored frames", c.frameCosts)
	}
}

// A frame padded past the cap is counted once, with the fraction of it DBSCAN
// kept; a frame under the cap and a warm-up frame over it are not, and the
// tracker still sees every frame's statistics.
func TestCampaignCountsFramesOverTheDBSCANCap(t *testing.T) {
	fake := &campaignFake{}
	c := &campaignTracker{TrackerInterface: fake, start: 10e9, inputCap: 8000}
	for _, f := range []struct {
		at    int64
		input int
	}{{9e9, 20000}, {10e9, 12000}, {11e9, 5000}, {12e9, 16000}, {12.5e9, 10000}} {
		c.RecordFrameStats(f.input, 0)
		c.recordFrame(f.at, 0.01)
	}
	// A frame with no foreground never calls RecordFrameStats.
	c.recordFrame(13e9, 0.01)
	if len(fake.foreground) != 5 {
		t.Fatalf("the tracker saw %d frames' statistics, want 5", len(fake.foreground))
	}
	out := t.TempDir()
	if err := c.write(defaultRuntime(), out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "dbscan_input.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Cap        int     `json:"cap"`
		Frames     int     `json:"frames"`
		Over       int     `json:"frames_over_cap"`
		Share      float64 `json:"share_over_cap"`
		KeptMedian float64 `json:"kept_fraction_median"`
		KeptMin    float64 `json:"kept_fraction_min"`
		InputMax   float64 `json:"input_points_max"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	if r.Cap != 8000 || r.Frames != 5 || r.Over != 3 || math.Abs(r.Share-0.6) > 1e-12 || r.InputMax != 16000 {
		t.Fatalf("report %+v: want cap 8000, 5 scored frames, 3 over the cap, share 0.6, largest input 16000", r)
	}
	// f is 2/3, 1/2 and 4/5 for the three frames over the cap.
	if math.Abs(r.KeptMin-0.5) > 1e-12 || math.Abs(r.KeptMedian-8000.0/12000) > 1e-12 {
		t.Fatalf("kept fractions median %v min %v, want 2/3 and 1/2", r.KeptMedian, r.KeptMin)
	}
}

// With no frame over the cap the kept fraction is absent, not zero.
func TestCampaignDBSCANInputWithoutCappedFrames(t *testing.T) {
	c := &campaignTracker{TrackerInterface: &campaignFake{}, inputCap: 8000}
	c.RecordFrameStats(100, 0)
	c.recordFrame(0, 0.01)
	r := c.inputReport()
	if r["frames_over_cap"] != 0 || r["kept_fraction_median"] != (*float64)(nil) {
		t.Fatalf("report %v, want no capped frames and no kept fraction", r)
	}
}
