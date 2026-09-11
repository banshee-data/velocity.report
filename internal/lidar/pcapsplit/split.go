package pcapsplit

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
)

// Default split parameters.
const (
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
	PCAPFile string `json:"pcap_file"`
	// PCAPFiles is an ordered set of captures analysed as one continuous
	// stream. Field capture rolls a new file every few minutes, so a static
	// period routinely spans several; analysing them separately restarts the
	// background model at each boundary and reports the settling as motion.
	//
	// Empty means the single PCAPFile. When set, PCAPFile names the first
	// entry so output naming and reporting have one file to refer to.
	PCAPFiles []string `json:"pcap_files,omitempty"`

	StartSeconds     float64 `json:"start_seconds"`
	DurationSeconds  float64 `json:"duration_seconds"`
	OutputDir        string  `json:"-"`
	OutputPrefix     string  `json:"output_prefix"`
	SettlingSec      float64 `json:"settling_sec"`
	MotionTriggerSec float64 `json:"motion_trigger_sec"`
	MaxMotionGapSec  float64 `json:"max_motion_gap_sec"`
	MinSegmentSec    float64 `json:"min_segment_sec"`
	SensorID         string  `json:"sensor_id"`
	UDPPort          int     `json:"udp_port"`
	ExportMetrics    bool    `json:"-"`
	ExportJSON       bool    `json:"-"`
	DryRun           bool    `json:"-"`
	Verbose          bool    `json:"-"`
	ProgressSecs     float64 `json:"-"` // Seconds between progress updates during the read (0 = off)
	Stats10s         bool    `json:"-"` // Print per-10s frame-rate buckets
	TimelineUnits    string  `json:"-"` // Breakdown time columns: seconds (default), frames, timestamp
	MotionJSONPath   string  `json:"-"` // Write the motion/static timeline to this JSON path
	// SelectSegments limits pass 2 to the named segments, matched against the
	// labels the summary prints ("static-5", "motion-0"). Empty writes them
	// all. Extracting one seven-minute stretch from a forty-minute session
	// should not mean writing the other thirty-three to disk.
	SelectSegments []string `json:"-"`

	// Tuning is the loaded tuning config (from -config); nil falls back to the
	// embedded defaults. The classifier's background model and thresholds come
	// from it so segmentation matches the live pipeline.
	Tuning *config.TuningConfig `json:"-"`
}

// DefaultSplitConfig returns a config with the standard defaults. Port 2369
// matches the project's capture conventions (settling-eval and the live
// capture pipeline).
func DefaultSplitConfig() SplitConfig {
	return SplitConfig{
		OutputDir:        ".",
		DurationSeconds:  -1,
		SettlingSec:      DefaultSettlingSec,
		MotionTriggerSec: DefaultMotionTriggerSec,
		MaxMotionGapSec:  DefaultMaxMotionGapSec,
		MinSegmentSec:    DefaultMinSegmentSec,
		SensorID:         "hesai-pandar40p",
		UDPPort:          2369,
	}
}

// EffectiveOutputPrefix returns the configured output prefix when set; when it
// is empty it derives the prefix from the input PCAP filename stem.
func (c SplitConfig) EffectiveOutputPrefix() string {
	if c.OutputPrefix != "" {
		return c.OutputPrefix
	}
	return deriveOutputPrefix(c.PCAPFile)
}

func deriveOutputPrefix(pcapFile string) string {
	base := filepath.Base(pcapFile)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "out"
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		return "out"
	}
	return stem
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
	DriftRatio         float64   `json:"drift_ratio"`
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
	StartFrame   int       `json:"start_frame"`
	EndFrame     int       `json:"end_frame"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	PacketCount  int       `json:"packet_count"`
}

// BuildSegments turns timeline periods into named output segments with
// sequential per-type IDs: <prefix>-<type>-<n>.pcap (e.g.
// soma2-motion-0.pcap, soma2-static-0.pcap, soma2-motion-1.pcap, ...).
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
			StartFrame:   p.StartFrame,
			EndFrame:     p.EndFrame,
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
	InputFile        string       `json:"input_file"`
	ProcessingTimeMs int64        `json:"processing_time_ms"`
	TotalPackets     int          `json:"total_packets"`
	TotalFrames      int          `json:"total_frames"`
	TotalDurationSec float64      `json:"total_duration_sec"`
	Config           SplitConfig  `json:"config"`
	Capture          CaptureStats `json:"capture"`
	Segments         []Segment    `json:"segments"`
}

// InputFiles is the ordered set of captures this run reads: the joined
// sequence when one was given, and the single capture otherwise.
func (c SplitConfig) InputFiles() []string {
	if len(c.PCAPFiles) > 0 {
		return c.PCAPFiles
	}
	if c.PCAPFile == "" {
		return nil
	}
	return []string{c.PCAPFile}
}

// SegmentLabel is the part of a segment's filename that identifies it within
// the run — "static-5" for "broadway_columbus-static-5.pcap". It is what the
// summary prints and what --segment matches against.
func (s Segment) SegmentLabel() string {
	name := strings.TrimSuffix(s.Filename, filepath.Ext(s.Filename))
	if i := strings.LastIndex(name, s.Type+"-"); i >= 0 {
		return name[i:]
	}
	return name
}

// SelectSegments filters segments down to the requested labels, preserving
// order. An unmatched request is returned so the caller can say which, rather
// than silently writing nothing.
func SelectSegments(segs []Segment, wanted []string) (selected []Segment, unmatched []string) {
	if len(wanted) == 0 {
		return segs, nil
	}
	want := make(map[string]bool, len(wanted))
	for _, w := range wanted {
		want[strings.TrimSpace(w)] = true
	}
	seen := map[string]bool{}
	for _, s := range segs {
		label := s.SegmentLabel()
		if want[label] {
			selected = append(selected, s)
			seen[label] = true
		}
	}
	for _, w := range wanted {
		w = strings.TrimSpace(w)
		if !seen[w] {
			unmatched = append(unmatched, w)
		}
	}
	return selected, unmatched
}

// selectionMask marks which segments are to be written, parallel to the full
// segment list. An empty selection writes them all.
//
// It is deliberately parallel rather than a filtered slice: segmentIndexForTime
// clamps out-of-range packets into the first or last segment so none are
// dropped, which is right for the complete timeline and catastrophic for a
// subset. Routing must see every segment; only writing is filtered.
func selectionMask(segs []Segment, wanted []string) []bool {
	mask := make([]bool, len(segs))
	if len(wanted) == 0 {
		for i := range mask {
			mask[i] = true
		}
		return mask
	}
	want := make(map[string]bool, len(wanted))
	for _, w := range wanted {
		want[strings.TrimSpace(w)] = true
	}
	for i, s := range segs {
		mask[i] = want[s.SegmentLabel()]
	}
	return mask
}
