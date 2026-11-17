package followingreview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/google/uuid"
)

func TestPlaybackFieldBoundsAndEmptyLeader(t *testing.T) {
	f := newFixture(t)
	base := Record{ID: uuid.NewString(), Reviewer: "reviewer", SavedAt: time.Now().UTC(), FollowerTrackID: "follower", StartNanos: 1000, EndExclusiveNanos: 1100, IdentityContinuity: "unknown", LeaderAndPath: "unknown", VisibleEndpoints: "unknown", SpeedEvidence: "unknown", Conclusion: "insufficientEvidence", Notes: "uncertain"}
	mutations := map[string]func(*Record){
		"empty proposed leader": func(r *Record) { s := ""; r.ProposedLeaderTrackID = &s },
		"source path":           func(r *Record) { r.Playback.SourcePath = strings.Repeat("x", 4097) },
		"recording path":        func(r *Record) { r.Playback.RecordingPath = strings.Repeat("x", 4097) },
		"coordinate id":         func(r *Record) { r.Playback.CoordinateFrameID = strings.Repeat("x", 257) },
		"many source ids":       func(r *Record) { r.Playback.FrameSourceIDs = make([]string, 65) },
		"long source id":        func(r *Record) { r.Playback.FrameSourceIDs = []string{strings.Repeat("x", 513)} },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			r := base
			mutate(&r)
			if e := ValidateRecord(r, f.slice.Rows, "run", 1000, 1101); e == nil {
				t.Fatal("accepted unsupported audit field")
			}
		})
	}
}

func TestSliceCandidateAndDRACAdmission(t *testing.T) {
	for _, kind := range []string{"valid nominal", "duplicate candidate", "self candidate", "empty candidate", "missing selected leader", "suppressed value", "negative value"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			value := 0.25
			row := &f.slice.Rows[0]
			row.Decision.Candidates = []l8behaviour.LeaderCandidate{{TrackID: "leader", Disposition: l8behaviour.DispositionLeader}}
			row.Decision.LeaderTrackID = "leader"
			row.DRAC.Status = "supported"
			row.DRAC.Calculation = &l8behaviour.DRACResult{Status: l8behaviour.DRACSupported, ValueMPS2: &value}
			switch kind {
			case "duplicate candidate":
				row.Decision.Candidates = append(row.Decision.Candidates, row.Decision.Candidates[0])
			case "self candidate":
				row.Decision.Candidates[0].TrackID = "follower"
			case "empty candidate":
				row.Decision.Candidates[0].TrackID = ""
			case "missing selected leader":
				row.Decision.LeaderTrackID = "absent"
			case "suppressed value":
				row.DRAC.Status = "missing_input"
			case "negative value":
				value = -1
			}
			b := writeJSON(t, f.receipt.Slices[0].Path, f.slice)
			f.receipt.Slices[0].SHA256 = digest(b)
			writeJSON(t, filepath.Join(f.root, "complete.json"), f.receipt)
			_, err := new(Cache).Open(f.run)
			if (kind == "valid nominal") != (err == nil) {
				t.Fatal(kind, err)
			}
		})
	}
}

func TestMatchesCaptureUsesExplicitIdentityAndResolvedAliases(t *testing.T) {
	root := t.TempDir()
	capture := filepath.Join(root, "capture")
	if e := os.WriteFile(capture, []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	alias := filepath.Join(root, "alias")
	if e := os.Symlink(capture, alias); e != nil {
		t.Fatal(e)
	}
	if !matchesCapture(capture, []string{capture}) || !matchesCapture(alias, []string{capture}) || matchesCapture(filepath.Join(root, "unknown"), []string{capture}) || matchesCapture(capture, nil) {
		t.Fatal("capture match used a heuristic")
	}
}

func TestAuditBoundsRemainExactForNegativeCaptureInstants(t *testing.T) {
	f := newFixture(t)
	rows := f.slice.Rows
	rows[0].CaptureUnixNanos = -1100
	rows[1].CaptureUnixNanos = -1000
	rows[0].Decision.CaptureUnixNanos = -1100
	rows[1].Decision.CaptureUnixNanos = -1000
	r := Record{ID: uuid.NewString(), Reviewer: "operator", SavedAt: time.Now().UTC(), FollowerTrackID: "follower", StartNanos: -1100, EndExclusiveNanos: -1000, IdentityContinuity: "unknown", LeaderAndPath: "unknown", VisibleEndpoints: "unknown", SpeedEvidence: "unknown", Conclusion: "insufficientEvidence", Notes: "synthetic bounds"}
	if e := ValidateRecord(r, rows, "run", -1200, -500); e != nil {
		t.Fatal(e)
	}
	r.EndExclusiveNanos = -900
	if e := ValidateRecord(r, rows, "run", -1200, -500); e == nil {
		t.Fatal("period exceeds follower evidence")
	}
}
