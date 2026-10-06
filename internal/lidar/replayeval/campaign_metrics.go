package replayeval

import (
	"encoding/json"
	"math"
	"path/filepath"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// campaignTracker observes update cost and capture-time confirmation intervals.
// It forwards the estimator interface unchanged; diagnostics never feed back.
type campaignTracker struct {
	l5tracks.TrackerInterface
	start     int64
	begun     bool
	left      map[int64]bool
	intervals map[int64]*confirmationInterval
	costs     []float64
}
type confirmationInterval struct {
	Sequence        int64 `json:"creation_sequence"`
	ConfirmedAt     int64 `json:"confirmed_at_ns"`
	LastSupportedAt int64 `json:"last_supported_at_ns"`
	LeftCensored    bool  `json:"left_censored"`
	RightCensored   bool  `json:"right_censored"`
}

func (c *campaignTracker) RecordBaselineEmptyFrame() {
	if baseline, ok := c.TrackerInterface.(interface{ RecordBaselineEmptyFrame() }); ok {
		baseline.RecordBaselineEmptyFrame()
	}
}

func (c *campaignTracker) Update(clusters []l5tracks.WorldCluster, at time.Time) {
	scored := at.UnixNano() >= c.start
	if scored && !c.begun {
		c.begun = true
		for _, t := range c.GetActiveTracks() {
			c.left[t.CreationSequence] = true
		}
	}
	started := time.Now()
	c.TrackerInterface.Update(clusters, at)
	elapsed := time.Since(started).Seconds()
	if !scored {
		return
	}
	c.costs = append(c.costs, elapsed)
	c.observe(c.GetConfirmedTracks(), at.UnixNano())
}
func (c *campaignTracker) AdvanceMisses(at time.Time) {
	c.TrackerInterface.AdvanceMisses(at)
	if at.UnixNano() >= c.start {
		c.observe(c.GetConfirmedTracks(), at.UnixNano())
	}
}

func (c *campaignTracker) observe(tracks []*l5tracks.TrackedObject, at int64) {
	active := map[int64]bool{}
	for _, t := range tracks {
		seq := t.CreationSequence
		active[seq] = true
		item := c.intervals[seq]
		if item == nil {
			item = &confirmationInterval{Sequence: seq, ConfirmedAt: at, LastSupportedAt: at, LeftCensored: c.left[seq], RightCensored: true}
			c.intervals[seq] = item
		}
		if t.LastObservedUnixNanos > item.LastSupportedAt {
			item.LastSupportedAt = t.LastObservedUnixNanos
		}
	}
	for seq, item := range c.intervals {
		if !active[seq] {
			item.RightCensored = false
		}
	}
}
func campaignQuantile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	return v[int(math.Ceil(q*float64(len(v))))-1]
}
func (c *campaignTracker) write(runtime replayRuntime, out string) error {
	intervals := make([]confirmationInterval, 0, len(c.intervals))
	sum := 0.0
	count := 0
	left, right := 0, 0
	for _, item := range c.intervals {
		intervals = append(intervals, *item)
		if item.LeftCensored {
			left++
		}
		if item.RightCensored {
			right++
		}
		if !item.LeftCensored && !item.RightCensored {
			sum += float64(item.LastSupportedAt-item.ConfirmedAt) / 1e9
			count++
		}
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].Sequence < intervals[j].Sequence })
	var mean *float64
	if count > 0 {
		v := sum / float64(count)
		mean = &v
	}
	duration := map[string]any{"definition": "first confirmed scored frame to last supported observation; complete intervals exclude warmup-born and active-at-end tracks", "complete_count": count, "left_censored_count": left, "right_censored_count": right, "mean_confirmed_duration_seconds": mean, "intervals": intervals}
	timing := map[string]any{"definition": "wall time inside Tracker.Update only; excludes parsing, clustering, persistence and diagnostic collection", "samples": len(c.costs), "p50_seconds": campaignQuantile(c.costs, .5), "p99_seconds": campaignQuantile(c.costs, .99)}
	for name, value := range map[string]any{"confirmed_duration.json": duration, "tracker_timing.json": timing} {
		b, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		if err = runtime.writeFile(filepath.Join(out, name), append(b, '\n'), 0644); err != nil {
			return err
		}
	}
	return nil
}
