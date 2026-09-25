package vrlog

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

var allSteps = []commitStep{stepSealed, stepChunkSynced, stepChunkPublished, stepIndexPublished, stepObjectsDurable,
	stepGenerationWritten, stepGenerationPublished, stepGenerationDurable, stepPointerWritten, stepPointerPublished, stepPointerDurable}

// killedAt writes frames 0-2 as generation 1, then frames 3-4, and kills the
// writer at step of generation 2's publication. It returns the frames
// written and the frontier announced before the kill.
func killedAt(t *testing.T, step commitStep) (string, []l4bobserve.FrameRecord, Frontier) {
	t.Helper()
	m := testManifest(t)
	m.Commit.MaxBatchAge = time.Hour
	w, dir := createTest(t, m)
	var frames []l4bobserve.FrameRecord
	for seq := range uint64(5) {
		frames = append(frames, synthFrame(seq, 12+int(seq)))
	}
	for _, f := range frames[:3] {
		if err := w.AppendFrame(f); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	for _, f := range frames[3:] {
		if err := w.AppendFrame(f); err != nil {
			t.Fatal(err)
		}
	}
	w.hooks.Store(&writerHooks{kill: func(s commitStep, generation uint64) bool { return s == step && generation == 2 }})
	if err := w.Flush(); !errors.Is(err, errWriterKilled) {
		t.Fatalf("flush through a kill = %v", err)
	}
	announced := w.Frontier()
	if announced.Generation != 1 || announced.Records != 3 {
		t.Fatalf("a killed publication was announced: %+v", announced)
	}
	return dir, frames, announced
}

// frameSequences returns the sequences of the frame records.
func frameSequences(records []Record) []uint64 {
	var out []uint64
	for _, r := range records {
		out = append(out, r.Frame.Sequence)
	}
	return out
}

func assertFrames(t *testing.T, want []l4bobserve.FrameRecord, got []Record) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("read frames %v, want %d", frameSequences(got), len(want))
	}
	for i := range want {
		if got[i].Kind != RecordFrame {
			t.Fatalf("record %d is a %s", i, got[i].Kind)
		}
		if err := l4bobserve.DiffFrames(want[i], got[i].Frame); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
}

// G-OBS-CRASH, process-kill paths: the writer is killed between every pair
// of publication steps. Open exposes exactly the acknowledged prefix;
// recovery exposes exactly the durable one (every generation whose object
// reached its name), promotes a published but unacknowledged generation with
// one recovery record, quarantines everything else, and never rewrites what
// was committed. A second recovery does nothing. Power loss, which can also
// undo unsynchronised renames, needs target hardware and is not tested here.
func TestCrashAtEveryPublicationStep(t *testing.T) {
	for _, step := range allSteps {
		t.Run(stepName(step), func(t *testing.T) {
			dir, frames, announced := killedAt(t, step)
			published := step >= stepGenerationPublished
			acknowledged := step >= stepPointerPublished

			r, err := Open(dir, Options{})
			if err != nil {
				t.Fatal(err)
			}
			st := r.Status()
			wantOpen := 3
			if acknowledged {
				wantOpen = 5
			}
			if st.Frames != uint64(wantOpen) || st.State != CaptureOpen || !st.TailExtentUnknown {
				t.Fatalf("open status = %+v", st)
			}
			if wantUnpromoted := published && !acknowledged; wantUnpromoted != slices.Equal(st.Unpromoted, []uint64{2}) {
				t.Fatalf("unpromoted = %v", st.Unpromoted)
			}
			if !published && len(st.UncommittedTail) == 0 {
				t.Fatalf("the unpublished batch is not reported: %+v", st)
			}
			assertFrames(t, frames[:wantOpen], readAll(t, dir, Options{}))
			committedBefore := committedDigests(t, dir, 2)

			report, err := Recover(dir, Options{})
			if err != nil {
				t.Fatal(err)
			}
			want := 3
			if published {
				want = 5
			}
			if !report.Wrote || report.State != CaptureIncomplete || report.Records != uint64(want) || report.Records < announced.Records {
				t.Fatalf("recovery = %+v", report)
			}
			if gotPromoted := slices.Equal(report.Promoted, []uint64{2}); gotPromoted != (published && !acknowledged) {
				t.Fatalf("promoted = %v", report.Promoted)
			}
			// Unpublished: the batch's chunk objects (and any temporary
			// generation) are quarantined. Published: nothing is left over.
			if published != (len(report.Quarantined) == 0) || (!published && !strings.HasPrefix(report.Quarantined[0], chunksDir+"/")) {
				t.Fatalf("quarantined %v", report.Quarantined)
			}
			assertFrames(t, frames[:want], readAll(t, dir, Options{}))
			if err := sameObjectsExceptQuarantined(committedBefore, committedDigests(t, dir, 2), report.Quarantined,
				generationObject(report.Generation)); err != nil {
				t.Fatalf("recovery changed committed objects: %v", err)
			}
			r, err = Open(dir, Options{})
			if err != nil {
				t.Fatal(err)
			}
			st = r.Status()
			if st.State != CaptureIncomplete || !st.TailExtentUnknown || st.Recovery == nil || !st.Recovery.SessionIncomplete ||
				len(st.UncommittedTail) != 0 || len(st.Unpromoted) != 0 || st.PointerFallback != "" {
				t.Fatalf("recovered status = %+v, record %+v", st, st.Recovery)
			}
			// Status is a copy: changing the returned marker changes nothing.
			st.Recovery.SessionIncomplete = false
			if len(st.Recovery.Promoted) > 0 {
				st.Recovery.Promoted[0] = 99
			}
			if len(st.Recovery.Quarantined) > 0 {
				st.Recovery.Quarantined[0] = "changed"
			}
			if fresh := r.Status(); !fresh.Recovery.SessionIncomplete || slices.Contains(fresh.Recovery.Promoted, 99) ||
				slices.Contains(fresh.Recovery.Quarantined, "changed") {
				t.Fatalf("Status shares the reader's recovery marker: %+v", fresh.Recovery)
			}
			if _, err := r.Verify(); err != nil {
				t.Fatal(err)
			}
			again, err := Recover(dir, Options{})
			if err != nil || again.Wrote || again.PointerRewritten || again.Generation != report.Generation {
				t.Fatalf("second recovery = %+v, %v", again, err)
			}
			recoveries := 0
			for _, g := range chainOf(t, dir) {
				if g.Kind == pb.GenerationKind_GENERATION_KIND_RECOVERY {
					recoveries++
				}
			}
			if recoveries != 1 {
				t.Fatalf("%d recovery records", recoveries)
			}
		})
	}
}

func stepName(s commitStep) string {
	return [...]string{"", "sealed", "chunk-synced", "chunk-published", "index-published", "objects-durable",
		"generation-written", "generation-published", "generation-durable", "pointer-written", "pointer-published", "pointer-durable"}[s]
}

// committedDigests hashes the objects of generations 0 to through, which
// recovery must leave byte for byte.
func committedDigests(t *testing.T, dir string, through uint64) map[string][sha256.Size]byte {
	t.Helper()
	out := map[string][sha256.Size]byte{}
	add := func(rel string) {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err == nil {
			out[rel] = sha256.Sum256(data)
		}
	}
	add(manifestName)
	for n := uint64(0); n <= through; n++ {
		add(generationObject(n))
	}
	entries, _ := os.ReadDir(filepath.Join(dir, chunksDir))
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), openSuffix) {
			add(chunksDir + "/" + e.Name())
		}
	}
	return out
}

// Recovery quarantines only this format's own debris: a file in the
// container's root that merely ends in the open suffix is left where it is.
func TestRecoverLeavesForeignOpenFiles(t *testing.T) {
	dir, _, _ := killedAt(t, stepSealed)
	for _, name := range []string{"notes.open", summaryName + openSuffix} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	report, err := Recover(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(report.Quarantined, "notes.open") {
		t.Fatalf("a foreign file was quarantined: %v", report.Quarantined)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.open")); err != nil {
		t.Fatalf("a foreign file was moved: %v", err)
	}
	if !slices.Contains(report.Quarantined, summaryName+openSuffix) {
		t.Fatalf("the writer's own open summary was not quarantined: %v", report.Quarantined)
	}
}

// sameObjectsExceptQuarantined requires recovery to leave every object it
// found byte for byte, except those it names as quarantined, which must be
// gone from their place, and to add nothing but its own recovery generation.
func sameObjectsExceptQuarantined(before, after map[string][sha256.Size]byte, quarantined []string, recovery string) error {
	moved := map[string]bool{}
	for _, name := range quarantined {
		moved[name] = true
	}
	for name, digest := range before {
		got, ok := after[name]
		switch {
		case moved[name] && ok:
			return fmt.Errorf("%s is quarantined but still in place", name)
		case moved[name]:
		case !ok:
			return fmt.Errorf("%s was removed", name)
		case got != digest:
			return fmt.Errorf("%s was rewritten", name)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok && name != recovery {
			return fmt.Errorf("%s appeared", name)
		}
	}
	return nil
}

// A writer killed with an open batch leaves it as an uncommitted tail, torn
// or not; recovery quarantines it rather than deleting it or reading it.
func TestKilledWithAnOpenBatch(t *testing.T) {
	m := testManifest(t)
	m.Commit.MaxBatchAge = time.Hour
	w, dir := createTest(t, m)
	for seq := range uint64(4) {
		if err := w.AppendFrame(synthFrame(seq, 20)); err != nil {
			t.Fatal(err)
		}
	}
	w.die()
	open := filepath.Join(dir, chunkObject(0)+openSuffix)
	info, err := os.Stat(open)
	if err != nil {
		t.Fatal(err)
	}
	Truncate(t, open, info.Size()-7) // a torn final record
	report, err := Recover(dir, Options{})
	if err != nil || report.Records != 0 || !report.Wrote || !slices.Equal(report.Quarantined, []string{chunkObject(0) + openSuffix}) {
		t.Fatalf("recovery = %+v, %v", report, err)
	}
	if _, err := os.Stat(filepath.Join(dir, quarantineDir, chunkBase(1), chunkObject(0)+openSuffix)); err != nil {
		t.Fatalf("the torn batch was not kept in quarantine: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, reserveName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("recovery left the dead writer's reserve")
	}
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st := r.Status(); st.Recovery == nil || st.Recovery.QuarantinedCount != 1 || !st.Recovery.TailExtentUnknown {
		t.Fatalf("status = %+v", st)
	}
}

// A torn pointer falls back to the longest validated chain; recovery
// records that the pointer was unusable and replaces it.
func TestTornPointerFallsBackToTheValidatedChain(t *testing.T) {
	dir, want, _ := writeTestContainer(t, smallChunks())
	chain := chainOf(t, dir)
	for name, tear := range map[string]func(string){
		"truncated": func(p string) { Truncate(t, p, 30) },
		"bit flip":  func(p string) { FlipBit(t, p, preambleSize+envelopeSize+2, 1) },
		"missing":   func(p string) { os.Remove(p) },
	} {
		t.Run(name, func(t *testing.T) {
			d := filepath.Join(t.TempDir(), "copy.vrlog")
			copyDir(t, dir, d)
			tear(filepath.Join(d, currentName))
			r, err := Open(d, Options{})
			if err != nil {
				t.Fatal(err)
			}
			st := r.Status()
			if st.PointerFallback == "" || st.Generation != uint64(len(chain)-1) || !st.Closed {
				t.Fatalf("status = %+v", st)
			}
			assertRecords(t, want, readAll(t, d, Options{}))
			report, err := Recover(d, Options{})
			if err != nil || !report.Wrote || report.PointerDetail == "" || len(report.Promoted) != 0 || report.State != CaptureClosed {
				t.Fatalf("recovery = %+v, %v", report, err)
			}
			if r, err = Open(d, Options{}); err != nil || r.Status().PointerFallback != "" || r.Status().Recovery.PointerValid {
				t.Fatalf("after recovery: %+v, %v", r.Status(), err)
			}
		})
	}
}

// Damage inside the acknowledged prefix fails normal reads, located, and
// recovery refuses to touch it: it never rewrites committed evidence. A
// reordered chain is refused the same way.
func TestDamageInTheCommittedPrefixIsNeverRecovered(t *testing.T) {
	dir, _, _ := writeTestContainer(t, smallChunks())
	for name, tc := range map[string]struct {
		damage func(string)
		kind   CorruptionKind
	}{
		"generation bit flip": {func(d string) { FlipBit(t, filepath.Join(d, generationObject(2)), preambleSize+envelopeSize+5, 4) }, CorruptChecksum},
		"generation truncated": {func(d string) {
			Truncate(t, filepath.Join(d, generationObject(2)), preambleSize+envelopeSize+3)
		}, CorruptTruncated},
		"generation missing": {func(d string) { os.Remove(filepath.Join(d, generationObject(2))) }, CorruptMissing},
		"generations reordered": {func(d string) {
			SwapFiles(t, filepath.Join(d, generationObject(1)), filepath.Join(d, generationObject(2)))
		}, CorruptSequence},
		"generation from another capture": {func(d string) {
			other, _, _ := writeTestContainer(t, smallChunks())
			data, err := os.ReadFile(filepath.Join(other, generationObject(2)))
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(d, generationObject(2)), data, 0o644)
		}, CorruptDisagreement},
	} {
		t.Run(name, func(t *testing.T) {
			d := filepath.Join(t.TempDir(), "copy.vrlog")
			copyDir(t, dir, d)
			tc.damage(d)
			before := listing(t, d)
			_, err := Open(d, Options{})
			var c *CorruptionError
			if !errors.As(err, &c) || c.Kind != tc.kind || !c.HaveSequences || !strings.Contains(c.Detail, "committed prefix") {
				t.Fatalf("open = %v, want %s located in the committed prefix", err, tc.kind)
			}
			if _, err := Recover(d, Options{}); !errors.As(err, &c) {
				t.Fatalf("recovery of a damaged committed prefix = %v", err)
			}
			if after := listing(t, d); !slices.Equal(before, after) {
				t.Fatalf("recovery changed a damaged container:\n%v\n%v", before, after)
			}
		})
	}
}

// listing names and hashes every file under dir.
func listing(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, _ := os.ReadFile(path)
		rel, _ := filepath.Rel(dir, path)
		out = append(out, fmt.Sprintf("%s %x", rel, sha256.Sum256(data)))
		return nil
	})
	return out
}

// A generation published beyond the pointer but torn is not promoted: it
// fails validation, so recovery quarantines it and keeps the pointer's
// prefix.
func TestTornGenerationBeyondThePointerIsQuarantined(t *testing.T) {
	dir, frames, _ := killedAt(t, stepGenerationDurable)
	Truncate(t, filepath.Join(dir, generationObject(2)), 40)
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st := r.Status(); len(st.Unpromoted) != 0 || !slices.Contains(st.UncommittedTail, generationObject(2)) {
		t.Fatalf("status = %+v", st)
	}
	report, err := Recover(dir, Options{})
	if err != nil || len(report.Promoted) != 0 || report.Records != 3 || !slices.Contains(report.Quarantined, generationObject(2)) {
		t.Fatalf("recovery = %+v, %v", report, err)
	}
	assertFrames(t, frames[:3], readAll(t, dir, Options{}))
}

// A recovery interrupted after publishing its record is completed by the
// next one, which only moves the pointer: the promotion is recorded once.
func TestInterruptedRecoveryCompletesOnce(t *testing.T) {
	for _, step := range []commitStep{stepGenerationWritten, stepGenerationPublished, stepPointerWritten, stepPointerPublished} {
		t.Run(stepName(step), func(t *testing.T) {
			dir, frames, _ := killedAt(t, stepGenerationDurable)
			recoveryHooks.Store(&writerHooks{kill: func(s commitStep, _ uint64) bool { return s == step }})
			_, err := Recover(dir, Options{})
			recoveryHooks.Store(nil)
			if !errors.Is(err, errWriterKilled) {
				t.Fatalf("interrupted recovery = %v", err)
			}
			report, err := Recover(dir, Options{})
			if err != nil || report.Records != 5 {
				t.Fatalf("second recovery = %+v, %v", report, err)
			}
			published := step >= stepGenerationPublished
			if report.Wrote == published || (published && step < stepPointerPublished) != report.PointerRewritten {
				t.Fatalf("second recovery wrote %v, rewrote the pointer %v", report.Wrote, report.PointerRewritten)
			}
			var records []*pb.RecoveryRecord
			for _, g := range chainOf(t, dir) {
				if g.Recovery != nil {
					records = append(records, g.Recovery)
				}
			}
			if len(records) != 1 || !slices.Equal(records[0].PromotedGenerations, []uint64{2}) {
				t.Fatalf("recovery records = %v", records)
			}
			assertFrames(t, frames, readAll(t, dir, Options{}))
			if again, err := Recover(dir, Options{}); err != nil || again.Wrote || again.PointerRewritten {
				t.Fatalf("third recovery = %+v, %v", again, err)
			}
		})
	}
}

// A failure marker survives; recovery quarantines the abandoned batch
// behind it and the tail extent stays known.
func TestRecoveryAfterAFailureMarker(t *testing.T) {
	m := testManifest(t)
	m.Commit.MaxBatchAge = time.Hour
	w, dir := createTest(t, m)
	for seq := range uint64(3) {
		if err := w.AppendFrame(synthFrame(seq, 10)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := w.AppendFrame(synthFrame(3, 10)); err != nil {
		t.Fatal(err)
	}
	w.hooks.Store(&writerHooks{sync: func(name string) error {
		if strings.HasPrefix(name, chunksDir+"/") {
			return errors.New("injected sync failure")
		}
		return nil
	}})
	if err := w.Flush(); !errors.Is(err, ErrCaptureFailed) {
		t.Fatalf("flush = %v", err)
	}
	w.Close()
	report, err := Recover(dir, Options{})
	if err != nil || !report.Wrote || report.State != CaptureFailed || report.Records != 3 || len(report.Quarantined) == 0 {
		t.Fatalf("recovery = %+v, %v", report, err)
	}
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	st := r.Status()
	if st.State != CaptureFailed || st.TailExtentUnknown || st.Failure.AcceptedEndSequence != 4 || st.Recovery.TailExtentUnknown {
		t.Fatalf("status = %+v, failure %+v, recovery %+v", st, st.Failure, st.Recovery)
	}
}

func copyDir(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
