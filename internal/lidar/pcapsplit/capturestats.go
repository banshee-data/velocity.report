package pcapsplit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CaptureStats holds the concise capture-level "scan" metrics pcap-split reports
// from its classification pass: duration, frame/packet/point counts, frame-rate
// (derived from motor RPM), and optional per-10 s frame-rate buckets. It is the
// health view the removed pcap-analyse -stats produced; segmentation adds the
// motion/static timeline on top.
type CaptureStats struct {
	File              string            `json:"file"`
	DurationSecs      float64           `json:"duration_secs"`
	TotalFrames       int               `json:"total_frames"`
	TotalPackets      int               `json:"total_packets"`
	TotalPoints       int               `json:"total_points"`
	AvgFrameRateHz    float64           `json:"avg_frame_rate_hz"`
	MinFrameRateHz    float64           `json:"min_frame_rate_hz"`
	MaxFrameRateHz    float64           `json:"max_frame_rate_hz"`
	MinRPM            uint16            `json:"min_rpm"`
	MaxRPM            uint16            `json:"max_rpm"`
	RPMChanges        int               `json:"rpm_changes"`
	ForegroundPct     float64           `json:"foreground_pct"`
	AvgPointsPerFrame float64           `json:"avg_points_per_frame"`
	FrameRate10s      []FrameRateBucket `json:"frame_rate_10s,omitempty"`
}

// FrameRateBucket holds the frame rate over one 10-second capture window.
type FrameRateBucket struct {
	OffsetSecs float64 `json:"offset_secs"` // bucket start relative to capture start
	Frames     int     `json:"frames"`
	Hz         float64 `json:"hz"`
}

// rpmAccumulator collects motor-RPM statistics incrementally as the network
// layer reports them per packet, avoiding a per-packet slice on multi-GB
// captures. Frame rate is derived from RPM (RPM/60), which is more accurate than
// azimuth-wrap frame counting on multi-return sensors.
type rpmAccumulator struct {
	min, max, last uint16
	sum            float64
	count, changes int
}

func (r *rpmAccumulator) observe(rpm uint16) {
	if rpm == 0 {
		return
	}
	if r.last != 0 && rpm != r.last {
		r.changes++
	}
	if r.min == 0 || rpm < r.min {
		r.min = rpm
	}
	if rpm > r.max {
		r.max = rpm
	}
	r.sum += float64(rpm)
	r.count++
	r.last = rpm
}

// computeCaptureStats assembles the capture summary from the scan pass: the
// per-frame timestamps and point/foreground totals (from the frame records) plus
// the accumulated motor RPM and packet count.
func computeCaptureStats(file string, frameTimes []time.Time, totalPackets, totalPoints, foregroundPoints int, rpm rpmAccumulator) CaptureStats {
	stats := CaptureStats{
		File:         file,
		TotalFrames:  len(frameTimes),
		TotalPackets: totalPackets,
		TotalPoints:  totalPoints,
		MinRPM:       rpm.min,
		MaxRPM:       rpm.max,
		RPMChanges:   rpm.changes,
	}
	if len(frameTimes) > 1 {
		stats.DurationSecs = frameTimes[len(frameTimes)-1].Sub(frameTimes[0]).Seconds()
	}
	if totalPoints > 0 {
		stats.ForegroundPct = 100 * float64(foregroundPoints) / float64(totalPoints)
	}
	if len(frameTimes) > 0 {
		stats.AvgPointsPerFrame = float64(totalPoints) / float64(len(frameTimes))
	}
	if rpm.min > 0 {
		stats.MinFrameRateHz = float64(rpm.min) / 60.0
	}
	if rpm.max > 0 {
		stats.MaxFrameRateHz = float64(rpm.max) / 60.0
	}
	if rpm.count > 0 {
		stats.AvgFrameRateHz = (rpm.sum / float64(rpm.count)) / 60.0
	}
	stats.FrameRate10s = computeFrameRateBuckets(frameTimes)
	return stats
}

// computeFrameRateBuckets groups per-frame timestamps into 10-second windows and
// reports the frame rate in each.
func computeFrameRateBuckets(frameTimes []time.Time) []FrameRateBucket {
	const bucket = 10 * time.Second
	if len(frameTimes) <= 1 {
		return nil
	}
	t0 := frameTimes[0]
	var buckets []FrameRateBucket
	bucketStart := t0
	count := 0
	for _, ts := range frameTimes {
		for ts.Sub(bucketStart) >= bucket {
			buckets = append(buckets, FrameRateBucket{
				OffsetSecs: bucketStart.Sub(t0).Seconds(),
				Frames:     count,
				Hz:         float64(count) / bucket.Seconds(),
			})
			bucketStart = bucketStart.Add(bucket)
			count = 0
		}
		count++
	}
	if count > 0 {
		elapsed := frameTimes[len(frameTimes)-1].Sub(bucketStart).Seconds()
		if elapsed < 0.001 {
			elapsed = bucket.Seconds() // single-frame trailing bucket
		}
		buckets = append(buckets, FrameRateBucket{
			OffsetSecs: bucketStart.Sub(t0).Seconds(),
			Frames:     count,
			Hz:         float64(count) / elapsed,
		})
	}
	return buckets
}

// captureBlock renders the capture-stats lines included in the run summary.
func (s CaptureStats) captureBlock() string {
	var b strings.Builder
	fmt.Fprintf(&b, "  Frame rate:  avg %.1f Hz, min %.1f Hz, max %.1f Hz\n",
		s.AvgFrameRateHz, s.MinFrameRateHz, s.MaxFrameRateHz)
	fmt.Fprintf(&b, "  RPM:         %d–%d", s.MinRPM, s.MaxRPM)
	if s.RPMChanges > 0 {
		fmt.Fprintf(&b, " (%d changes)", s.RPMChanges)
	}
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "  Points:      %d (%.0f/frame, %.1f%% foreground)\n",
		s.TotalPoints, s.AvgPointsPerFrame, s.ForegroundPct)
	return b.String()
}

// FormatStats10s renders one grep-friendly line per 10-second frame-rate bucket:
// [file] (mmm:ss) frame_rate: XX.X Hz
func FormatStats10s(s CaptureStats) string {
	base := filepath.Base(s.File)
	var b strings.Builder
	for _, bk := range s.FrameRate10s {
		min := int(bk.OffsetSecs) / 60
		sec := int(bk.OffsetSecs) % 60
		fmt.Fprintf(&b, "[%s] (%03d:%02d) frame_rate: %.1f Hz\n", base, min, sec, bk.Hz)
	}
	return b.String()
}

// FormatMotionTimeline renders the motion/static timeline. units selects the
// boundary columns: "frames" (default), "seconds" (offset from the first frame),
// or "timestamp" (absolute capture time).
func FormatMotionTimeline(periods []MotionPeriod, units string) string {
	if len(periods) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintln(&b, "Motion Timeline:")
	var staticSecs, motionSecs float64
	for _, p := range periods {
		switch units {
		case "seconds":
			fmt.Fprintf(&b, "  [%8.1fs – %8.1fs]  %-7s (%s)\n",
				p.StartSecs, p.EndSecs, p.Type, minSec(p.DurationSecs))
		case "timestamp":
			fmt.Fprintf(&b, "  [%s – %s]  %-7s (%s)\n",
				p.StartTime.Format("15:04:05.000"), p.EndTime.Format("15:04:05.000"),
				p.Type, minSec(p.DurationSecs))
		default: // frames
			fmt.Fprintf(&b, "  [frame %7d – %7d]  %-7s (%d frames)\n",
				p.StartFrame, p.EndFrame, p.Type, p.EndFrame-p.StartFrame)
		}
		switch p.Type {
		case StaticLabel:
			staticSecs += p.DurationSecs
		case MotionLabel:
			motionSecs += p.DurationSecs
		}
	}
	if total := staticSecs + motionSecs; total > 0 {
		fmt.Fprintf(&b, "  ── %.0f%% static, %.0f%% motion (%s static, %s motion)\n",
			100*staticSecs/total, 100*motionSecs/total, minSec(staticSecs), minSec(motionSecs))
	}
	return b.String()
}

// motionTimelineReport is the JSON document written by --motion-json.
type motionTimelineReport struct {
	File           string         `json:"file"`
	DurationSecs   float64        `json:"duration_secs"`
	MotionTimeline []MotionPeriod `json:"motion_timeline"`
}

// WriteMotionTimelineJSON writes the motion/static timeline to path.
func WriteMotionTimelineJSON(path, file string, durationSecs float64, periods []MotionPeriod) error {
	rep := motionTimelineReport{
		File:           filepath.Base(file),
		DurationSecs:   durationSecs,
		MotionTimeline: periods,
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
