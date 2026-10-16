package evalfixture

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
)

// WriteFate writes a pack, split, capture and container that read back as the
// scenario the package comment describes: nine frames (sample 6 has none),
// the unsettled, gapped and failed frames where it says, the decoy at sample
// 1 and sample 7's return straying into sample 8.
func TestWriteFateWritesTheScenarioItDescribes(t *testing.T) {
	f, err := WriteFate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{f.PackDir, f.SplitManifestPath, f.CapturePath, f.ObservationsDir} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("WriteFate did not write %s: %v", p, err)
		}
	}

	s, err := DefaultFateScenario(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Frames) != Samples-1 {
		t.Fatalf("%d scenario frames, want %d", len(s.Frames), Samples-1)
	}
	by := map[int]FateFrame{}
	for _, fr := range s.Frames {
		by[fr.Sample] = fr
	}
	if _, ok := by[6]; ok {
		t.Error("sample 6 has a frame; it should have none")
	}
	for sample, want := range map[int]l4bobserve.DispositionKind{
		0: l4bobserve.DispositionObserved, 4: l4bobserve.DispositionUnsettled, 9: l4bobserve.DispositionFailed,
	} {
		if got := by[sample].Disposition; got != want {
			t.Errorf("sample %d disposition %v, want %v", sample, got, want)
		}
	}
	for sample, fr := range by {
		if fr.GapBefore != (sample == 7) {
			t.Errorf("sample %d gap before = %v", sample, fr.GapBefore)
		}
	}

	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	pts, err := pack.PointsAt(1)
	if err != nil {
		t.Fatal(err)
	}
	decoy := by[1].Domain[3]
	if decoy.Block != 2 || decoy.Channel != 1 || decoy.X != float64(pts.X[3]) || decoy.Y != float64(pts.Y[3]) || decoy.Z != float64(pts.Z[3]) {
		t.Errorf("sample 1's decoy %+v: want block 2 channel 1 at return 3's coordinates", decoy)
	}
	if n := len(by[8].Domain); n != len(by[0].Domain)+1 {
		t.Errorf("sample 8's domain holds %d returns, want sample 0's %d plus sample 7's stray", n, len(by[0].Domain))
	}
	if FatePacketSequence(8, 1) != 117 {
		t.Errorf("FatePacketSequence(8, 1) = %d, want 117", FatePacketSequence(8, 1))
	}

	r, err := vrlog.Open(f.ObservationsDir, vrlog.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	frames, gaps := 0, 0
	for {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch rec.Kind {
		case vrlog.RecordFrame:
			frames++
		case vrlog.RecordGap:
			gaps++
		}
	}
	if frames != Samples-1 || gaps != 1 {
		t.Errorf("container holds %d frames and %d gaps, want %d and 1", frames, gaps, Samples-1)
	}
}

func TestWriteFateRefusesAnUnwritableDirectory(t *testing.T) {
	blocker := t.TempDir() + "/file"
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteFate(blocker); err == nil {
		t.Fatal("WriteFate wrote under a path that is a file")
	}
}
