package pcapsplit

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WriteSegmentsJSON writes the segment metadata document to path.
func WriteSegmentsJSON(path string, r Report) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal segments json: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write segments json: %w", err)
	}
	return nil
}

// frameMetricsHeader is the column order for frame_metrics.csv.
var frameMetricsHeader = []string{
	"frame_id", "timestamp", "total_points", "foreground_points",
	"nonzero_cells", "settled_cells", "percent_settled",
	"drift_ratio", "moving", "state",
}

// WriteFrameMetricsCSV writes per-frame metrics to path.
func WriteFrameMetricsCSV(path string, frames []FrameMetrics) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create frame metrics csv: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write(frameMetricsHeader); err != nil {
		return err
	}
	for _, fm := range frames {
		row := []string{
			strconv.Itoa(fm.FrameID),
			fm.T.UTC().Format(time.RFC3339Nano),
			strconv.Itoa(fm.TotalPoints),
			strconv.Itoa(fm.ForegroundPoints),
			strconv.Itoa(fm.NonzeroCells),
			strconv.Itoa(fm.SettledCells),
			strconv.FormatFloat(fm.PercentSettled, 'f', 4, 64),
			strconv.FormatFloat(fm.DriftRatio, 'f', 3, 64),
			strconv.FormatBool(fm.Moving),
			fm.State,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// WriteSummary writes the human-readable summary to path.
func WriteSummary(path string, r Report) error {
	return os.WriteFile(path, []byte(FormatSummary(r)), 0o644)
}

// FormatSummary renders the human-readable run summary.
func FormatSummary(r Report) string {
	var b strings.Builder

	var motion, static int
	var motionDur, staticDur float64
	for _, s := range r.Segments {
		if s.Type == MotionLabel {
			motion++
			motionDur += s.DurationSecs
		} else {
			static++
			staticDur += s.DurationSecs
		}
	}

	fmt.Fprintln(&b, "PCAP Split Analysis Summary")
	fmt.Fprintln(&b, "===========================")
	fmt.Fprintf(&b, "Input File:      %s\n", r.InputFile)
	fmt.Fprintf(&b, "Processing Time: %s\n", time.Duration(r.ProcessingTimeMs)*time.Millisecond)
	fmt.Fprintf(&b, "Total Packets:   %d\n", r.TotalPackets)
	fmt.Fprintf(&b, "Total Frames:    %d\n", r.TotalFrames)
	fmt.Fprintf(&b, "Total Duration:  %s\n", minSec(r.TotalDurationSec))
	if r.Capture.TotalFrames > 0 {
		fmt.Fprint(&b, r.Capture.captureBlock())
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Configuration:")
	fmt.Fprintf(&b, "  Settling Duration:  %.0fs\n", r.Config.SettlingSec)
	fmt.Fprintf(&b, "  Motion Trigger:     %.0fs\n", r.Config.MotionTriggerSec)
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Segments: %d motion (%s), %d static (%s)\n",
		motion, minSec(motionDur), static, minSec(staticDur))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Detailed Breakdown:")
	for i, s := range r.Segments {
		fmt.Fprintf(&b, "  [%d] %-7s %s  (%s)  %d packets  %s\n",
			i, s.Type, segmentBounds(s, r.Config.TimelineUnits), minSec(s.DurationSecs), s.PacketCount, s.Filename)
	}
	return b.String()
}

// segmentBounds renders a segment's start→end columns in the requested units:
// "frames", "timestamp", or "seconds" (the default).
func segmentBounds(s Segment, units string) string {
	switch units {
	case "frames":
		return fmt.Sprintf("frame %7d → %7d", s.StartFrame, s.EndFrame)
	case "timestamp":
		return fmt.Sprintf("%s → %s",
			s.StartTime.Format("15:04:05.000"), s.EndTime.Format("15:04:05.000"))
	default: // seconds
		return fmt.Sprintf("%8.1fs → %8.1fs", s.StartSecs, s.EndSecs)
	}
}

// minSec formats a duration in seconds as "Nm SSs".
func minSec(secs float64) string {
	total := int(secs + 0.5)
	return fmt.Sprintf("%dm %02ds", total/60, total%60)
}

// MetadataPath joins the output dir and a metadata filename.
func (c SplitConfig) MetadataPath(name string) string {
	return filepath.Join(c.OutputDir, name)
}
