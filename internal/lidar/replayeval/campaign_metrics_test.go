package replayeval

import (
	"encoding/json"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type campaignFake struct {
	l5tracks.TrackerInterface
	active, confirmed []*l5tracks.TrackedObject
	calls             int
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
	if err := c.write(out); err != nil {
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
	if err := c.write(filepath.Join(t.TempDir(), "missing")); err == nil {
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
