package labelset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// testFrame is one sample of a synthetic version 2 pack: the frame's packet
// range in the pack's table, and the returns its foreground points came from.
type testFrame struct {
	firstPacket, packets int
	returns              []uint64 // pack return numbers, increasing
}

func writeTestPack(t *testing.T, dir string, packets []uint64, frames []testFrame, run string) *annotation.Pack {
	t.Helper()
	per := uint64(Pandar40P.Blocks * Pandar40P.Channels)
	var samples []annotation.Sample
	var blocks [][]byte
	var returns [][]uint64
	for i, f := range frames {
		n := len(f.returns)
		pts := annotation.Points{X: make([]float32, n), Y: make([]float32, n), Z: make([]float32, n)}
		for j, r := range f.returns {
			pts.X[j], pts.Y[j], pts.Z[j] = float32(r), float32(i), 1
		}
		b, err := annotation.EncodePoints(pts)
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, annotation.Sample{
			SourceOrdinal: i, TimestampNs: int64(1000 + i), SensorID: "hesai-pandar40p", PointCount: n,
			ReturnStart: uint64(f.firstPacket) * per, ReturnEnd: uint64(f.firstPacket+f.packets) * per,
		})
		blocks = append(blocks, b)
		returns = append(returns, f.returns)
	}
	m := annotation.Manifest{
		Source:     annotation.SourceProvenance{VRLOGPath: run, SensorID: "hesai-pandar40p", ConfigHash: "sha256:cfg-" + run},
		Coordinate: annotation.CoordinateContract{Units: "metres"},
		Coverage:   annotation.CoverageForegroundOnly,
	}
	pr := annotation.PackReturns{
		Layout:  annotation.ReturnLayout{Model: Pandar40P.Model, Blocks: Pandar40P.Blocks, Channels: Pandar40P.Channels},
		Packets: packets, Returns: returns,
	}
	if err := annotation.WritePack(dir, m, samples, blocks, pr); err != nil {
		t.Fatal(err)
	}
	p, err := annotation.OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPackV2RoundTripAndTamper(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pack")
	p := writeTestPack(t, dir, []uint64{1, 2, 3}, []testFrame{{0, 2, []uint64{5, 6, 410}}, {2, 1, []uint64{800, 801}}}, "run-a")
	if p.Manifest.SchemaVersion != annotation.PackSchemaVersion || len(p.Packets) != 3 {
		t.Fatalf("version %d, %d packets", p.Manifest.SchemaVersion, len(p.Packets))
	}
	got, err := p.ReturnsAt(1)
	if err != nil || len(got) != 2 || got[0] != 800 {
		t.Fatalf("returns %v %v", got, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "returns.bin"))
	b[0] ^= 1
	os.WriteFile(filepath.Join(dir, "returns.bin"), b, 0o644)
	if _, err := annotation.OpenPack(dir); err == nil {
		t.Fatal("tampered returns opened")
	}
}

func TestPackV2RefusesDisorderedReturns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pack")
	pts := annotation.Points{X: []float32{0, 0}, Y: []float32{0, 0}, Z: []float32{0, 0}}
	b, _ := annotation.EncodePoints(pts)
	samples := []annotation.Sample{{PointCount: 2, ReturnStart: 0, ReturnEnd: 400}}
	pr := annotation.PackReturns{Layout: annotation.ReturnLayout{Model: "m", Blocks: 10, Channels: 40}, Packets: []uint64{9}, Returns: [][]uint64{{7, 3}}}
	if err := annotation.WritePack(dir, annotation.Manifest{}, samples, [][]byte{b}, pr); err == nil {
		t.Fatal("decreasing returns written")
	}
	pr.Returns = [][]uint64{{3, 400}}
	if err := annotation.WritePack(dir, annotation.Manifest{}, samples, [][]byte{b}, pr); err == nil {
		t.Fatal("a return outside its frame written")
	}
}

func TestViewMapAcrossRuns(t *testing.T) {
	base := t.TempDir()
	// Run A's pack: packets 10..13, two frames of two packets each.
	a := writeTestPack(t, filepath.Join(base, "a"), []uint64{10, 11, 12, 13},
		[]testFrame{{0, 2, []uint64{5, 6, 7, 450}}, {2, 2, []uint64{800, 900}}}, "run-a")
	layout, state, err := NewFromPack(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Windows) != 2 || len(state.Domains) != 1 {
		t.Fatalf("windows %d domains %d", len(state.Windows), len(state.Domains))
	}
	state.Records.Objects = []annotation.Object{{ObjectID: "obj_a", Class: "car", Status: annotation.StatusReviewed}}
	vmA, err := NewViewMap(layout, &state, a)
	if err != nil {
		t.Fatal(err)
	}
	w0, ok := vmA.SampleWindow(0)
	if !ok {
		t.Fatal("sample 0 has no window")
	}
	members, err := vmA.Offsets(0, []int{0, 1, 3})
	if err != nil {
		t.Fatal(err)
	}
	state.Masks = []Mask{{ObjectID: "obj_a", WindowID: w0, DomainID: state.Domains[0].DomainID, Members: members,
		Completeness: annotation.MaskPartial, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed}}
	state.Canonicalise()
	if err := state.Validate(layout); err != nil {
		t.Fatal(err)
	}

	// Run B framed the same packets but kept different foreground, and its
	// packet table lists them in the same order with one extra packet first.
	b := writeTestPack(t, filepath.Join(base, "b"), []uint64{9, 10, 11, 12, 13},
		[]testFrame{{1, 2, []uint64{405, 406, 450, 850}}, {3, 2, []uint64{1300}}}, "run-b")
	skipped, err := AddPackDomain(layout, &state, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped samples %v", skipped)
	}
	if len(state.Packets) != 5 || len(state.Windows) != 2 || len(state.Domains) != 2 {
		t.Fatalf("after run B: %d packets, %d windows, %d domains", len(state.Packets), len(state.Windows), len(state.Domains))
	}
	if err := state.Validate(layout); err != nil {
		t.Fatal(err)
	}
	vmB, err := NewViewMap(layout, &state, b)
	if err != nil {
		t.Fatal(err)
	}
	shown, hidden := vmB.Shown(0, state.Masks[0].Members)
	// Run B shows returns 5, 6 and 450 (as 405, 406 and 850); run A's mask holds those three.
	if len(shown) != 3 || hidden != 0 {
		t.Fatalf("run B shows %v of run A's mask, hides %d", shown, hidden)
	}
	// Editing through run B: dropping its point 0 removes return 5 only.
	edited, err := vmB.Edit(0, state.Masks[0].Members, []int{1, 3})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Count() != 2 || edited.Contains(5) || !edited.Contains(450) {
		t.Fatalf("edit through run B left %v", edited)
	}
	// A member run B never showed survives an edit made through run B.
	withHidden := Union(state.Masks[0].Members, Ranges{{7, 1}})
	kept, _ := vmB.Edit(0, withHidden, []int{0, 1, 3})
	if !kept.Contains(7) {
		t.Fatalf("hidden member dropped: %v", kept)
	}
}

func TestViewMapRefusesOtherLayout(t *testing.T) {
	a := writeTestPack(t, filepath.Join(t.TempDir(), "a"), []uint64{1}, []testFrame{{0, 1, []uint64{1}}}, "run-a")
	_, state, err := NewFromPack(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewViewMap(Layout{Model: "other", Blocks: 6, Channels: 32}, &state, a); err == nil {
		t.Fatal("mismatched layout mapped")
	}
}
