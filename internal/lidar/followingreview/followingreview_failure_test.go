package followingreview

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
	"github.com/google/uuid"
)

type interruptedReviewReader struct{}

func (interruptedReviewReader) Read([]byte) (int, error) {
	return 0, errors.New("review read interrupted")
}

func TestCanonicalRejectsCorruptStreams(t *testing.T) {
	f := newFixture(t)
	valid, err := json.Marshal(f.doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, data string }{
		{"root array", `[]`},
		{"empty input", ``},
		{"truncated object", `{"schema":`},
		{"duplicate identity", `{"schema":"following_evidence_v1","schema":"following_evidence_v1"}`},
		{"wrong schema type", `{"schema":1}`},
		{"unsupported schema", `{"schema":"future"}`},
		{"context wrong type", `{"context":false}`},
		{"version wrong type", `{"analysis_version":"wrong"}`},
		{"analysis wrong type", `{"analysis":[]}`},
		{"paths wrong type", `{"analysis":{"paths":{}}}`},
		{"truncated paths", `{"analysis":{"paths":[`},
		{"missing paths value", `{"analysis":{"paths":`},
		{"wrong path type", `{"analysis":{"paths":[false]}}`},
		{"missing follower", `{"analysis":{"paths":[{}]}}`},
		{"duplicate follower", `{"analysis":{"paths":[{"follower_track_id":"f"},{"follower_track_id":"f"}]}}`},
		{"invalid geometry", `{"analysis":{"paths":[{"follower_track_id":"f","path":{"id":"p","estimate_stage":"online","vertices":[]}}]}}`},
		{"missing identity", `{"analysis":{}}`},
		{"malformed ignored value", `{"rows":[1,}`},
		{"truncated ignored object", `{"rows":{"a":`},
		{"nesting limit", `{"rows":` + strings.Repeat("[", 130) + `0` + strings.Repeat("]", 130) + `}`},
		{"trailing document", string(valid) + ` {}`},
		{"invalid version", strings.Replace(string(valid), `"method_id":"method"`, `"method_id":""`, 1)},
		{"missing estimator version", strings.Replace(string(valid), `"estimator_id":"estimator"`, `"estimator_id":""`, 1)},
		{"invalid object token", `{"schema":"following_evidence_v1",@}`},
		{"invalid ignored object token", `{"rows":{"a":1,@}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := readCanonical(strings.NewReader(tc.data)); err == nil {
				t.Fatal("accepted corrupt canonical stream")
			}
		})
	}
	for _, prefix := range []string{"", `{"schema":`, `{"rows":[1,`} {
		if _, err := readCanonical(io.MultiReader(strings.NewReader(prefix), interruptedReviewReader{})); err == nil {
			t.Fatal("accepted interrupted canonical stream")
		}
	}
}

func TestCanonicalGeometryCacheBoundary(t *testing.T) {
	f := newFixture(t)
	b, err := json.Marshal(f.doc)
	if err != nil {
		t.Fatal(err)
	}
	c, err := readCanonical(bytes.NewReader(b))
	if err != nil || len(c.Paths) != 1 {
		t.Fatalf("valid fixture: %v", err)
	}
	geometry, err := json.Marshal(c.Paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = readCanonicalLimit(bytes.NewReader(b), len(geometry)); err != nil {
		t.Fatalf("exact geometry limit refused: %v", err)
	}
	if _, err = readCanonicalLimit(bytes.NewReader(b), len(geometry)-1); err == nil {
		t.Fatal("geometry exceeded configured cache bound")
	}
}

func TestReviewFileAndContainmentFailures(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "file")
	if err := os.WriteFile(p, []byte("123456"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBounded(p, 5); err == nil {
		t.Fatal("accepted oversized file")
	}
	if b, err := readBounded(p, 6); err != nil || string(b) != "123456" {
		t.Fatalf("exact boundary rejected: %q %v", b, err)
	}
	for _, p := range []string{root, filepath.Join(root, "missing")} {
		if _, err := readBounded(p, 10); err == nil {
			t.Fatal("accepted unreadable file")
		}
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := contained(root, link); err == nil {
		t.Fatal("symlink escaped preparation")
	}
	if _, err := contained(root, filepath.Join(root, "missing")); err == nil {
		t.Fatal("accepted missing artifact")
	}
	if _, err := decodeForTest([]byte(`{} {}`)); err == nil {
		t.Fatal("accepted trailing receipt")
	}
}

func decodeForTest(b []byte) (receipt, error) { var r receipt; err := decode(b, &r); return r, err }

func TestSessionRejectsMissingOrMalformedArtifacts(t *testing.T) {
	for _, name := range []string{"receipt missing", "receipt malformed", "receipt trailing", "receipt oversized", "registered artifact missing", "header missing", "header malformed", "header directory", "canonical missing", "slice missing", "canonical directory"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			path := filepath.Join(f.root, "complete.json")
			switch name {
			case "receipt missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "receipt malformed":
				if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "receipt trailing":
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, append(b, []byte(" {}")...), 0600); err != nil {
					t.Fatal(err)
				}
			case "receipt oversized":
				if err := os.WriteFile(path, bytes.Repeat([]byte(" "), (1<<20)+1), 0600); err != nil {
					t.Fatal(err)
				}
			case "registered artifact missing":
				f.receipt.VRLOG = filepath.Join(f.root, "missing-recording")
				writeJSON(t, path, f.receipt)
			case "header missing":
				if err := os.Remove(filepath.Join(f.run.VRLogPath, "header.json")); err != nil {
					t.Fatal(err)
				}
			case "header malformed":
				if err := os.WriteFile(filepath.Join(f.run.VRLogPath, "header.json"), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "header directory":
				p := filepath.Join(f.run.VRLogPath, "header.json")
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			case "canonical missing":
				if err := os.Remove(f.receipt.Evidence); err != nil {
					t.Fatal(err)
				}
			case "slice missing":
				if err := os.Remove(f.receipt.Slices[0].Path); err != nil {
					t.Fatal(err)
				}
			case "canonical directory":
				if err := os.Remove(f.receipt.Evidence); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(f.receipt.Evidence, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := new(Cache).Open(f.run); err == nil {
				t.Fatal("opened invalid preparation")
			}
		})
	}
}

func TestSessionRechecksUnavailableSlicesAfterOpen(t *testing.T) {
	for _, directory := range []bool{false, true} {
		f := newFixture(t)
		s, err := new(Cache).Open(f.run)
		if err != nil {
			t.Fatal(err)
		}
		p := f.receipt.Slices[0].Path
		if err = os.Remove(p); err != nil {
			t.Fatal(err)
		}
		if directory {
			if err = os.Mkdir(p, 0700); err != nil {
				t.Fatal(err)
			}
		}
		if _, _, err = s.Evidence(1000); err == nil {
			t.Fatal("cached session ignored unavailable slice")
		}
	}
}

func TestCachedSessionCannotHideCanonicalReplacement(t *testing.T) {
	f := newFixture(t)
	var cache Cache
	first, err := cache.Open(f.run)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(f.receipt.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(f.receipt.Evidence, append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = cache.Open(f.run); err == nil {
		t.Fatal("cached metadata hid changed canonical digest")
	}
	if first.Metadata.EvidenceSHA256 == digest(append(b, '\n')) {
		t.Fatal("old session identity mutated")
	}
}

// Immutable preparation artifacts must not be replaceable through path aliases
// or a different inode hidden behind identical size and modification metadata.
func TestSessionRejectsReceiptAliasesAndInodeReplacement(t *testing.T) {
	t.Run("relative receipt artifact", func(t *testing.T) {
		f := newFixture(t)
		f.receipt.VRLOG = "vrlog"
		writeJSON(t, filepath.Join(f.root, "complete.json"), f.receipt)
		if _, err := new(Cache).Open(f.run); err == nil {
			t.Fatal("accepted relative receipt artifact")
		}
	})
	t.Run("receipt symlink outside preparation", func(t *testing.T) {
		f := newFixture(t)
		outside := filepath.Join(t.TempDir(), "complete.json")
		writeJSON(t, outside, f.receipt)
		path := filepath.Join(f.root, "complete.json")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, path); err != nil {
			t.Fatal(err)
		}
		if _, err := new(Cache).Open(f.run); err == nil {
			t.Fatal("accepted receipt from outside preparation")
		}
	})
	t.Run("replacement preserves size and mtime", func(t *testing.T) {
		f := newFixture(t)
		var cache Cache
		if _, err := cache.Open(f.run); err != nil {
			t.Fatal(err)
		}
		original, err := os.Stat(f.receipt.Evidence)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(f.receipt.Evidence)
		if err != nil {
			t.Fatal(err)
		}
		changed := bytes.Replace(b, []byte(`"method_id":"method"`), []byte(`"method_id":"edited"`), 1)
		if bytes.Equal(b, changed) || len(changed) != len(b) {
			t.Fatal("fixture did not change same-size version")
		}
		replacement := filepath.Join(f.root, "replacement.json")
		if err = os.WriteFile(replacement, changed, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.Chtimes(replacement, original.ModTime(), original.ModTime()); err != nil {
			t.Fatal(err)
		}
		if err = os.Rename(replacement, f.receipt.Evidence); err != nil {
			t.Fatal(err)
		}
		if _, err = cache.Open(f.run); err == nil {
			t.Fatal("cache hid changed canonical version behind preserved metadata")
		}
	})
}

func TestAuditRejectsUnsupportedLeaderAndIntervals(t *testing.T) {
	f := newFixture(t)
	r := Record{ID: uuid.NewString(), Reviewer: "operator", SavedAt: time.Now().UTC(), FollowerTrackID: "follower", StartNanos: 1000, EndExclusiveNanos: 1100, IdentityContinuity: "unknown", LeaderAndPath: "unknown", VisibleEndpoints: "notVisible", SpeedEvidence: "unknown", Conclusion: "insufficientEvidence", Notes: "bounded review"}
	leader := "leader"
	r.ProposedLeaderTrackID = &leader
	if err := ValidateRecord(r, f.slice.Rows, "run", 1000, 1101); err == nil {
		t.Fatal("accepted unlisted leader")
	}
	f.slice.Rows[0].Decision.Candidates = []l8behaviour.LeaderCandidate{{TrackID: leader, Disposition: l8behaviour.DispositionLeader}}
	if err := ValidateRecord(r, f.slice.Rows, "run", 1000, 1101); err != nil {
		t.Fatal(err)
	}
	self := "follower"
	r.ProposedLeaderTrackID = &self
	f.slice.Rows[0].Decision.Candidates = append(f.slice.Rows[0].Decision.Candidates, l8behaviour.LeaderCandidate{TrackID: self, Disposition: l8behaviour.DispositionCompeting})
	if err := ValidateRecord(r, f.slice.Rows, "run", 1000, 1101); err == nil {
		t.Fatal("accepted follower as its own leader")
	}
	r.ProposedLeaderTrackID = nil
	other := f.slice.Rows[0]
	other.FollowerTrackID = "another-follower"
	f.slice.Rows = append([]fieldrun.EvidenceRow{other}, f.slice.Rows...)
	if err := ValidateRecord(r, f.slice.Rows, "run", 1000, 1101); err != nil {
		t.Fatal(err)
	}
	f.slice.Rows[1].IntervalNanos = -1
	if err := ValidateRecord(r, f.slice.Rows, "run", 1000, 1101); err == nil {
		t.Fatal("accepted negative retained interval")
	}
}
