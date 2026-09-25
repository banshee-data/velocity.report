package perframeeval

import (
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// Frame alignment.
//
// The matcher compares frames by exact timestamp, and a pack's samples carry
// the timestamps of the run that recorded its source, not of the run being
// scored. Two replays of one capture agree to within about a millisecond
// (1.2 ms over the whole of kirk0, measured for the annotation-scored sweep),
// and frame boundaries can move between builds. So each hypothesis point is
// moved onto the nearest reference frame within a tolerance, and everything
// that could not be is counted rather than dropped quietly: a point inside the
// episode's time span that lands on no frame is a hypothesis the score never
// saw, and past a small fraction of them the score is refused.

// AlignmentStats records what alignment did to one arm in one episode.
type AlignmentStats struct {
	// AlignedPoints were moved onto a reference frame and scored.
	AlignedPoints int `json:"aligned_points"`
	// OutsideEpisodePoints fell outside every interval of the episode. They
	// are not this episode's business.
	OutsideEpisodePoints int `json:"outside_episode_points"`
	// UnalignedPoints fell inside an interval's time span but further than
	// the tolerance from every frame in it. They are not scored.
	UnalignedPoints int `json:"unaligned_points"`
	// DuplicatePoints were a second point of one track on one frame; the
	// nearer in time was kept.
	DuplicatePoints int `json:"duplicate_points"`
	// MaxOffsetNanos is the largest time shift applied to a kept point.
	MaxOffsetNanos int64 `json:"max_offset_ns"`
	// Tracks is how many hypothesis tracks have a point in the episode.
	Tracks int `json:"tracks"`
}

// UnalignedFraction is the share of the points inside the episode that could
// not be scored.
func (a AlignmentStats) UnalignedFraction() float64 {
	inside := a.AlignedPoints + a.DuplicatePoints + a.UnalignedPoints
	if inside == 0 {
		return 0
	}
	return float64(a.UnalignedPoints) / float64(inside)
}

// alignToEpisode returns the hypothesis restricted to the episode, each point
// re-stamped with its reference frame's timestamp.
func alignToEpisode(hyp []l8analytics.TrackSeries, ep EpisodeReference, toleranceNanos int64) ([]l8analytics.TrackSeries, AlignmentStats) {
	var stats AlignmentStats
	frames := ep.Frames
	out := make([]l8analytics.TrackSeries, 0, len(hyp))

	for _, s := range hyp {
		type kept struct {
			point  l8analytics.SeriesPoint
			offset int64
		}
		byFrame := map[int64]kept{}
		for _, p := range s.Points {
			if !insideSpans(ep.spans, p.TimestampNanos, toleranceNanos) {
				stats.OutsideEpisodePoints++
				continue
			}
			frame, offset, ok := nearestFrame(frames, p.TimestampNanos)
			if !ok || abs64(offset) > toleranceNanos {
				stats.UnalignedPoints++
				continue
			}
			moved := p
			moved.TimestampNanos = frame
			if prev, dup := byFrame[frame]; dup {
				stats.DuplicatePoints++
				// Nearer in time wins; on a tie the earlier original point,
				// so the result does not depend on input order.
				if abs64(offset) > abs64(prev.offset) || (abs64(offset) == abs64(prev.offset) && offset > prev.offset) {
					continue
				}
			}
			byFrame[frame] = kept{point: moved, offset: offset}
		}
		if len(byFrame) == 0 {
			continue
		}
		aligned := l8analytics.TrackSeries{ID: s.ID, Points: make([]l8analytics.SeriesPoint, 0, len(byFrame))}
		for _, k := range byFrame {
			aligned.Points = append(aligned.Points, k.point)
			if o := abs64(k.offset); o > stats.MaxOffsetNanos {
				stats.MaxOffsetNanos = o
			}
		}
		sort.Slice(aligned.Points, func(i, j int) bool {
			return aligned.Points[i].TimestampNanos < aligned.Points[j].TimestampNanos
		})
		stats.AlignedPoints += len(aligned.Points)
		out = append(out, aligned)
	}
	stats.Tracks = len(out)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, stats
}

func insideSpans(spans []timeSpan, ts, tolerance int64) bool {
	for _, s := range spans {
		if ts >= s.first-tolerance && ts <= s.last+tolerance {
			return true
		}
	}
	return false
}

// nearestFrame returns the frame nearest ts in an ascending list, and ts's
// offset from it. On an exact tie the earlier frame wins.
func nearestFrame(frames []int64, ts int64) (int64, int64, bool) {
	if len(frames) == 0 {
		return 0, 0, false
	}
	i := sort.Search(len(frames), func(i int) bool { return frames[i] >= ts })
	best := -1
	if i < len(frames) {
		best = i
	}
	if i > 0 && (best < 0 || ts-frames[i-1] <= frames[best]-ts) {
		best = i - 1
	}
	return frames[best], ts - frames[best], true
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
