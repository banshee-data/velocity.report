package pcapsplit

import (
	"fmt"
	"time"
)

// Default split parameters.
const (
	// DefaultSettledThreshold is the minimum TimesSeenCount for a grid cell to
	// be considered settled when computing per-frame metrics.
	DefaultSettledThreshold uint32 = 50
	// DefaultMaxMotionGapSec bridges short stops (e.g. intersection waits)
	// adjacent to motion into the motion segment.
	DefaultMaxMotionGapSec = 30.0
	// DefaultMinSegmentSec merges any segment shorter than this into a
	// neighbour, preventing micro-segments.
	DefaultMinSegmentSec = 5.0
)

// SplitConfig holds the parameters for a pcap-split run. JSON tags mirror the
// segments.json `config` block.
type SplitConfig struct {
	PCAPFile         string  `json:"pcap_file"`
	OutputDir        string  `json:"-"`
	OutputPrefix     string  `json:"output_prefix"`
	SettlingSec      float64 `json:"settling_sec"`
	MotionTriggerSec float64 `json:"motion_trigger_sec"`
	MaxMotionGapSec  float64 `json:"max_motion_gap_sec"`
	MinSegmentSec    float64 `json:"min_segment_sec"`
	SettledThreshold uint32  `json:"settled_threshold"`
	SensorID         string  `json:"sensor_id"`
	UDPPort          int     `json:"udp_port"`
	ExportMetrics    bool    `json:"-"`
	ExportJSON       bool    `json:"-"`
	DryRun           bool    `json:"-"`
	Verbose          bool    `json:"-"`
	ProgressSecs     float64 `json:"-"` // Seconds between progress updates during the read (0 = off)
}

// DefaultSplitConfig returns a config with the standard defaults. Port 2369
// matches the project's capture conventions (pcap-analyse / settling-eval).
func DefaultSplitConfig() SplitConfig {
	return SplitConfig{
		OutputDir:        ".",
		OutputPrefix:     "out",
		SettlingSec:      DefaultSettlingSec,
		MotionTriggerSec: DefaultMotionTriggerSec,
		MaxMotionGapSec:  DefaultMaxMotionGapSec,
		MinSegmentSec:    DefaultMinSegmentSec,
		SettledThreshold: DefaultSettledThreshold,
		SensorID:         "hesai-pandar40p",
		UDPPort:          2369,
	}
}

// TimelineConfig returns the hysteresis and refinement thresholds derived from
// this config.
func (c SplitConfig) TimelineConfig() TimelineConfig {
	return TimelineConfig{
		SettlingSec:      c.SettlingSec,
		MotionTriggerSec: c.MotionTriggerSec,
		MaxMotionGapSec:  c.MaxMotionGapSec,
		MinSegmentSec:    c.MinSegmentSec,
	}
}

// FrameMetrics is the per-frame record exported to frame_metrics.csv. State is
// back-filled from the final timeline (see AssignFrameStates); Moving is the raw
// per-frame detector result before hysteresis.
type FrameMetrics struct {
	FrameID            int       `json:"frame_id"`
	T                  time.Time `json:"timestamp"`
	TotalPoints        int       `json:"total_points"`
	ForegroundPoints   int       `json:"foreground_points"`
	ForegroundFraction float64   `json:"foreground_fraction"`
	NonzeroCells       int       `json:"nonzero_cells"`
	SettledCells       int       `json:"settled_cells"`
	PercentSettled     float64   `json:"percent_settled"`
	DeviationFromNoise float64   `json:"deviation_from_noise"`
	WithinNoiseBounds  bool      `json:"within_noise_bounds"`
	Stable             bool      `json:"stable"`
	Moving             bool      `json:"moving"`
	State              string    `json:"state"`
}

// Segment is one output PCAP file: a single motion or static period with a
// sequential per-type filename and the packet count written into it.
type Segment struct {
	Type         string    `json:"type"`
	ID           int       `json:"id"`
	Filename     string    `json:"filename"`
	StartSecs    float64   `json:"start_secs"`
	EndSecs      float64   `json:"end_secs"`
	DurationSecs float64   `json:"duration_secs"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	PacketCount  int       `json:"packet_count"`
}

// BuildSegments turns timeline periods into named output segments with
// sequential per-type IDs: <prefix>-<type>-<n>.pcap (e.g. out-motion-0.pcap,
// out-static-0.pcap, out-motion-1.pcap, ...).
func BuildSegments(periods []MotionPeriod, prefix string) []Segment {
	if prefix == "" {
		prefix = "out"
	}
	counts := make(map[string]int, 2)
	segs := make([]Segment, 0, len(periods))
	for _, p := range periods {
		id := counts[p.Type]
		counts[p.Type]++
		segs = append(segs, Segment{
			Type:         p.Type,
			ID:           id,
			Filename:     fmt.Sprintf("%s-%s-%d.pcap", prefix, p.Type, id),
			StartSecs:    p.StartSecs,
			EndSecs:      p.EndSecs,
			DurationSecs: p.DurationSecs,
			StartTime:    p.StartTime,
			EndTime:      p.EndTime,
		})
	}
	return segs
}

// segmentIndexForTime returns the index of the segment that a packet captured
// at time t belongs to. Segments are contiguous and time-ordered, so the rule
// is "the first segment whose EndTime is after t". Packets before the first
// segment map to segment 0 and packets at/after the final boundary map to the
// last segment, so no packet is dropped. Returns -1 only for an empty list.
func segmentIndexForTime(segs []Segment, t time.Time) int {
	if len(segs) == 0 {
		return -1
	}
	for i := range segs {
		if t.Before(segs[i].EndTime) {
			return i
		}
	}
	return len(segs) - 1
}

// AssignFrameStates back-fills each frame's State with the type of the segment
// that contains its timestamp, so the exported frame metrics agree with the
// final (post-hysteresis) timeline.
func AssignFrameStates(frames []FrameMetrics, segs []Segment) {
	for i := range frames {
		if idx := segmentIndexForTime(segs, frames[i].T); idx >= 0 {
			frames[i].State = segs[idx].Type
		}
	}
}

// Report is the top-level result bundle for metadata export and the summary.
type Report struct {
	InputFile        string      `json:"input_file"`
	ProcessingTimeMs int64       `json:"processing_time_ms"`
	TotalPackets     int         `json:"total_packets"`
	TotalFrames      int         `json:"total_frames"`
	TotalDurationSec float64     `json:"total_duration_sec"`
	Config           SplitConfig `json:"config"`
	Segments         []Segment   `json:"segments"`
}
