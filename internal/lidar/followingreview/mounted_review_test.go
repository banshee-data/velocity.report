package followingreview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// This opt-in, read-only check exercises an existing preparation; it never
// replays packets, writes audit data or qualifies physical measurements.
func TestMountedReviewPreparation(t *testing.T) {
	path := os.Getenv("VELOCITY_FOLLOWING_REVIEW_SMOKE_VRLOG")
	if path == "" {
		t.Skip("set VELOCITY_FOLLOWING_REVIEW_SMOKE_VRLOG for an existing completed preparation")
	}
	b, err := os.ReadFile(filepath.Join(path, "header.json"))
	if err != nil {
		t.Fatal(err)
	}
	var h recorder.LogHeader
	if err = json.Unmarshal(b, &h); err != nil {
		t.Fatal(err)
	}
	run := &sqlite.Run{RunID: filepath.Base(filepath.Dir(path)), Status: "completed", VRLogPath: path, SensorID: h.SensorID, BuildGitSHA: h.BuildGitSHA, FrameStartNs: &h.StartNs, FrameEndNs: &h.EndNs}
	cache := new(Cache)
	session, err := cache.Open(run)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, file := range session.Metadata.Slices {
		b, rows, err := session.Evidence(file.Start)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) == 0 || len(rows) != file.Rows {
			t.Fatal("slice differs")
		}
		total += len(rows)
		if _, err = session.Geometry(file.Start, ""); err != nil {
			t.Fatal(err)
		}
	}
	if total != session.receipt.Rows {
		t.Fatal("receipt rows differ")
	}
	second, err := cache.Open(run)
	if err != nil || second != session {
		t.Fatal("verified preparation not cached", err)
	}
	t.Logf("read-only paired preparation verified: %d slices, %d rows, %d paths, canonical %s", len(session.Metadata.Slices), total, len(session.paths), session.Metadata.EvidenceSHA256)
}
