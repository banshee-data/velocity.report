//go:build pcap

package pcapsplit

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFranklinStopAfterDrive covers a stop that the old continuous model missed:
// the locked baseline retained the preceding drive and reported all seven
// five-minute captures as motion. Set FRANKLIN_PCAP_DIR to the S2 capture
// directory to run this external-data regression.
func TestFranklinStopAfterDrive(t *testing.T) {
	dir := os.Getenv("FRANKLIN_PCAP_DIR")
	if dir == "" {
		t.Skip("set FRANKLIN_PCAP_DIR to run the Franklin capture regression")
	}
	stamps := []string{
		"20260903102551_00006", "20260903103052_00007",
		"20260903103552_00008", "20260903104053_00009",
		"20260903104553_00010", "20260903105054_00011",
		"20260903105554_00012",
	}
	files := make([]string, len(stamps))
	for i, stamp := range stamps {
		files[i] = filepath.Join(dir, fmt.Sprintf("s2_sf_6_%s.pcap", stamp))
	}
	cfg := DefaultSplitConfig()
	cfg.PCAPFile = files[0]
	cfg.PCAPFiles = files
	analysis, err := Analyse(cfg)
	if err != nil {
		t.Fatal(err)
	}
	periods := BuildTimeline(analysis.Samples, cfg.TimelineConfig())
	if len(periods) != 3 || periods[0].Type != MotionLabel ||
		periods[1].Type != StaticLabel || periods[2].Type != MotionLabel {
		t.Fatalf("want drive, one Franklin stop, drive; got %+v", periods)
	}
	static := periods[1]
	if static.DurationSecs < 19*60+45 || static.DurationSecs > 20*60+30 {
		t.Fatalf("Franklin static duration = %.1fs, want about 20 minutes", static.DurationSecs)
	}
	start := time.Date(2026, 9, 3, 10, 35, 52, 0, time.FixedZone("PDT", -7*3600))
	end := time.Date(2026, 9, 3, 10, 55, 55, 0, time.FixedZone("PDT", -7*3600))
	if static.StartTime.Sub(start) < -10*time.Second || static.StartTime.Sub(start) > 30*time.Second ||
		static.EndTime.Sub(end) < -10*time.Second || static.EndTime.Sub(end) > 30*time.Second {
		t.Fatalf("Franklin transition times = %s–%s, want near %s–%s",
			static.StartTime, static.EndTime, start, end)
	}
}
