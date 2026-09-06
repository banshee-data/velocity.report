package analysis

import (
	"encoding/json"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"os"
	"testing"
)

// This fixture freezes recorded output, not ideal headings or new-path targets.
// The raw PCAP is identified by hash but is not needed for this regression test.
func TestBaf20f02NamedCarHeadingFixture(t *testing.T) {
	var f struct {
		Schema    int               `json:"schema"`
		PCAPHash  string            `json:"pcap_sha256"`
		PoseTruth bool              `json:"pose_truth"`
		Tracks    map[string]string `json:"tracks"`
		Frames    []struct {
			ID        uint64 `json:"id"`
			Timestamp int64  `json:"timestamp_ns"`
			Samples   []struct {
				Track   string  `json:"track"`
				Source  int     `json:"source"`
				State   int     `json:"state"`
				Speed   float32 `json:"speed"`
				Course  float32 `json:"course"`
				Heading float32 `json:"heading"`
				Length  float32 `json:"length"`
				Width   float32 `json:"width"`
			} `json:"samples"`
		} `json:"frames"`
	}
	data, err := os.ReadFile("testdata/baf20f02-heading.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if f.Schema != 1 || len(f.Frames) != 200 || len(f.Tracks) != 2 || f.PoseTruth {
		t.Fatal("fixture contract changed")
	}
	if f.PCAPHash != "sha256:6d1270ccd6a9aa239b831f560cfb1db6615c28ffe0fa871e1134b8b0d651dd7b" {
		t.Fatal("source changed")
	}
	var sources []int
	var live []bool
	var obb, course, speed []float32
	coPublished := 0
	observedCollapse := false
	for i, fr := range f.Frames {
		if fr.ID != uint64(1000+i) || (i > 0 && fr.Timestamp <= f.Frames[i-1].Timestamp) {
			t.Fatal("frame identity/order changed")
		}
		if len(fr.Samples) == 2 {
			coPublished++
		}
		for _, s := range fr.Samples {
			if s.Track == "trk_18952226" {
				sources = append(sources, s.Source)
				live = append(live, s.State != int(l9endpoints.TrackStateDeleted))
				obb = append(obb, s.Heading)
				course = append(course, s.Course)
				speed = append(speed, s.Speed)
				if fr.ID == 1026 && s.Length < .12 && s.Width < .09 {
					observedCollapse = true
				}
			}
		}
	}
	locks := computeLockStats(sources, live)
	_, _, n := courseAlignmentMetrics(obb, course, speed, live, CourseAlignmentMinSpeedMps)
	if coPublished != 123 || len(sources) != 124 || locks.lockedFrames != 63 || !observedCollapse || n == 0 {
		t.Fatalf("case lost: pairs=%d samples=%d locked=%d collapse=%v course_n=%d", coPublished, len(sources), locks.lockedFrames, observedCollapse, n)
	}
}
