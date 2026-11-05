// Package eligibilitytest builds synthetic annotation packs, label sets,
// frozen splits, capture files and cohort manifests for tests. Nothing here
// reads a real capture or a reserved qualification dataset.
package eligibilitytest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
)

// PackOptions describes a synthetic pack.
type PackOptions struct {
	Capture      string // capture folder under the annotation root
	Run          string // pack name under <capture>/packs
	PCAPBasename string
	// StartNs is the first sample's time; samples are 100 ms apart.
	StartNs int64
	// Objects are the object IDs, each with a mask in every sample.
	Objects []string
	// Status is every object's and mask's review status.
	Status annotation.ReviewStatus
	// NoLabels leaves the pack without a label set, for Attach.
	NoLabels bool
}

// Pack writes a three-sample pack with a label set and returns it.
func Pack(t testing.TB, root string, o PackOptions) *annotation.Pack {
	t.Helper()
	if o.StartNs == 0 {
		o.StartNs = 1_000_000_000
	}
	if o.Status == "" {
		o.Status = annotation.StatusReviewed
	}
	if len(o.Objects) == 0 {
		o.Objects = []string{"car-1"}
	}
	var samples []annotation.Sample
	var blocks [][]byte
	for i := 0; i < 3; i++ {
		var pts annotation.Points
		for k := range o.Objects {
			for _, x := range []float32{float32(10+i+5*k) - 2, float32(10+i+5*k) + 2} {
				for _, z := range []float32{0.3, 1.5} {
					pts.X, pts.Y, pts.Z = append(pts.X, x), append(pts.Y, 0), append(pts.Z, z)
				}
			}
		}
		block, err := annotation.EncodePoints(pts)
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, annotation.Sample{SourceOrdinal: i, SourceFrameID: uint64(100 + i),
			TimestampNs: o.StartNs + int64(i)*100_000_000, SensorID: "hesai-test", PointCount: len(pts.X)})
		blocks = append(blocks, block)
	}
	m := annotation.Manifest{
		Coverage: annotation.CoverageForegroundOnly,
		Source: annotation.SourceProvenance{SensorID: "hesai-test", VRLOGHeaderSHA: "sha256:header-" + o.Run,
			VRLOGFramesSHA: "sha256:frames-" + o.Run, PCAPBasename: o.PCAPBasename},
		Coordinate: annotation.CoordinateContract{Units: "metres", FrameID: "sensor", ReferenceFrame: "sensor",
			Handedness: "right", OriginNote: "sensor origin"},
	}
	dir := filepath.Join(root, o.Capture, annotation.PacksDir, o.Run)
	if err := annotation.WritePack(dir, m, samples, blocks, annotation.FixtureReturns(samples)); err != nil {
		t.Fatal(err)
	}
	p, err := annotation.OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	if o.NoLabels {
		return p
	}
	s := annotation.NewSidecar(p)
	s.Change = annotation.Provenance{Author: "op", Operation: "label"}
	for k, id := range o.Objects {
		s.Objects = append(s.Objects, annotation.Object{ObjectID: id, Class: "car", Confidence: 1, Status: o.Status})
		for i := 0; i < 3; i++ {
			s.Masks = append(s.Masks, annotation.FrameMask{ObjectID: id, SampleID: i,
				PointIndices: []int{4 * k, 4*k + 1, 4*k + 2, 4*k + 3},
				Completeness: annotation.MaskComplete, Visibility: annotation.VisiblePresent, Status: o.Status})
		}
	}
	if _, err := labelset.SaveMembership(p, s, labelset.Change{Author: "op", Operation: "label"}); err != nil {
		t.Fatal(err)
	}
	return p
}

// Handle is a pack's handle beneath the annotation root.
func Handle(t testing.TB, root string, p *annotation.Pack) string {
	t.Helper()
	rel, err := filepath.Rel(root, p.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}

// Freeze freezes one pack with every object in one partition of the role,
// writes the split as <root>/splits/<name> and returns it.
func Freeze(t testing.TB, root, name string, p *annotation.Pack, role annotation.SplitRole, supersedes *annotation.FrozenSplit) *annotation.FrozenSplit {
	t.Helper()
	s, err := annotation.LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, o := range s.Objects {
		ids = append(ids, o.ObjectID)
	}
	draft := &annotation.SplitDraft{Schema: annotation.SplitDraftSchema, SchemaVersion: annotation.SplitDraftSchemaVersion,
		Packs: []annotation.DraftPack{{
			Dir:    p.Dir,
			Splits: []annotation.Split{{Name: string(role), Role: role, ObjectIDs: ids}},
			Episodes: []annotation.Episode{{EpisodeID: "ep-1", Split: string(role), ObjectIDs: ids,
				FrameIntervals: []annotation.FrameInterval{{FirstSample: 0, LastSample: 2}}}},
		}}}
	f, err := annotation.FreezeSplit(annotation.FreezeOptions{Draft: draft, BaseDir: root, Author: "op",
		Now: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC), GuardSeconds: annotation.DefaultSplitGuardSeconds, Supersedes: supersedes,
		BuildVersion: "test", BuildGitSHA: "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "splits")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := annotation.WriteFrozenSplit(filepath.Join(dir, name), f); err != nil {
		t.Fatal(err)
	}
	return f
}

// Capture writes a synthetic capture file and returns its path and hash.
func Capture(t testing.TB, dir, name, content string) (string, string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := fileSHA(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, sum
}

// Manifest writes a cohort manifest and returns its path.
func Manifest(t testing.TB, dir string, m map[string]any) string {
	t.Helper()
	base := map[string]any{
		"schema": "velocity.report/cohort-manifest", "schema_version": 1, "revision": 1,
		"frozen_utc": "2026-11-05T00:00:00Z", "sites": []any{}, "captures": []any{}, "access": map[string]any{},
	}
	for k, v := range m {
		base[k] = v
	}
	b, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "cohort-manifest.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Site is a manifest site entry.
func Site(id, cohort string) map[string]any {
	return map[string]any{"site_id": id, "cohort": cohort, "rationale": "synthetic test site"}
}

// CaptureEntry is a manifest capture entry.
func CaptureEntry(id, site, sha, basename string) map[string]any {
	return map[string]any{"capture_id": id, "site_id": site, "raw_sha256": sha, "basename": basename}
}

func fileSHA(path string) (string, error) {
	return sha256File(path)
}

// LabelSetDir is the directory of the label set a pack links.
func LabelSetDir(t testing.TB, p *annotation.Pack) string {
	t.Helper()
	link, err := annotation.ReadLabelSetLink(p.Dir)
	if err != nil || link == nil {
		t.Fatalf("pack %s links no label set: %v", p.Dir, err)
	}
	return link.LabelSetDir(p.Dir)
}

// Attach adds another run's pack of the same capture to a label set and
// links it, as velocity lidar labels attach does.
func Attach(t testing.TB, p *annotation.Pack, labelSetDir string) {
	t.Helper()
	ls, err := labelset.Open(labelSetDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := labelset.AddPackDomain(ls.Manifest.Layout, &ls.State, p); err != nil {
		t.Fatal(err)
	}
	if err := ls.Save(labelset.Change{Author: "op", Operation: "attach_pack"}); err != nil {
		t.Fatal(err)
	}
	if err := annotation.WriteLabelSetLink(p, ls.Manifest.LabelSetID, labelSetDir); err != nil {
		t.Fatal(err)
	}
}

// Bind records a capture file in a label set's applies_to, as a successful
// velocity lidar labels bind would, without reading a capture.
func Bind(t testing.TB, labelSetDir, name, sha string) {
	t.Helper()
	path := filepath.Join(labelSetDir, labelset.ManifestFile)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m["applies_to"] = []any{map[string]any{"name": name, "sha256": sha, "bytes": 1, "udp_port": 2369, "packets_found": 1,
		"packets_missing": 0, "windows_complete": 3, "windows_partial": 0, "windows_missing": 0, "verified_utc": "2026-11-05T00:00:00Z"}}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
