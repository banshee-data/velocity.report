package vrlog

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
)

// produce appends n frames, with a zero-width gap before every seventh,
// sleeping up to pause between appends, then closes the writer. It returns
// the records in order; errors go to the test.
func produce(t *testing.T, w *Writer, n int, pause time.Duration, seed uint64) []Record {
	var want []Record
	rng := rand.New(rand.NewPCG(seed, 1))
	for i := range uint64(n) {
		if i%7 == 3 {
			g := l4bobserve.GapRecord{Cause: "l2-callback-queue-overflow"}
			if err := w.AppendGap(g); err != nil {
				t.Error(err)
				return want
			}
			want = append(want, Record{Kind: RecordGap, Gap: g})
		}
		f := synthFrame(i, 5+int(i%40))
		if err := w.AppendFrame(f); err != nil {
			t.Error(err)
			return want
		}
		want = append(want, Record{Kind: RecordFrame, Frame: f})
		if pause > 0 {
			time.Sleep(time.Duration(rng.Int64N(int64(pause))))
		}
	}
	if _, err := w.Close(); err != nil {
		t.Error(err)
	}
	return want
}

func liveWriter(t *testing.T) (*Writer, string) {
	m := testManifest(t)
	m.Limits = smallChunks()
	m.Commit.MaxBatchAge = 2 * time.Millisecond
	return createTest(t, m)
}

// G-OBS-QUEUE: a follower reading while the writer commits sees only
// committed records (each one already inside the announced frontier when it
// is returned), whole, in order, each exactly once, across batch rotation.
func TestFollowerReadsOnlyCommittedWholeRecordsInOrder(t *testing.T) {
	w, dir := liveWriter(t)
	f, err := Follow(dir, w, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var want []Record
	done := make(chan struct{})
	go func() { defer close(done); want = produce(t, w, 150, time.Millisecond, 1) }()
	var got []Record
	ctx := testContext(t, 30*time.Second)
	for {
		rec, err := f.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ordinal := uint64(len(got)); ordinal >= w.Frontier().Records {
			t.Fatalf("record %d returned before it was committed (frontier %+v)", ordinal, w.Frontier())
		}
		got = append(got, rec)
	}
	<-done
	assertRecords(t, want, got)
	if st := f.Status(); !st.Closed || st.Chunks < 10 {
		t.Fatalf("the follower did not see a closed capture across many generations: %+v", st)
	}
}

// A paused follower never holds the writer back: the writer commits and
// closes while the follower waits, then the follower drains everything.
func TestPausedFollowerNeverBlocksTheWriter(t *testing.T) {
	w, dir := liveWriter(t)
	f, err := Follow(dir, w, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := testContext(t, 30*time.Second)
	want := produce(t, w, 300, 0, 2) // returns only once the writer has closed
	var got []Record
	for {
		rec, err := f.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, rec)
	}
	assertRecords(t, want, got)
}

// A stall in the writer's synchronisation holds the follower at the
// frontier: a bounded wait returns the context's error without moving, and
// reading resumes once the commit lands.
func TestFollowerWaitsThroughAWriterStall(t *testing.T) {
	m := testManifest(t)
	m.Commit.MaxBatchAge = time.Millisecond
	w, dir := createTest(t, m)
	f, err := Follow(dir, w, Options{})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	w.hooks.Store(&writerHooks{sync: func(name string) error {
		if name == chunkObject(0)+openSuffix {
			<-release
		}
		return nil
	}})
	if err := w.AppendFrame(synthFrame(0, 10)); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := f.Next(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("an uncommitted record was offered: %v", err)
	}
	if f.Cursor().Record != 0 {
		t.Fatal("a timed-out wait moved the cursor")
	}
	close(release)
	rec, err := f.Next(testContext(t, 5*time.Second))
	if err != nil || rec.Frame.Sequence != 0 {
		t.Fatalf("after the stall: %+v, %v", rec, err)
	}
}

// Cursors resume without skipping or repeating; a cursor from another
// container, from a generation not committed here, or beyond what its
// generation commits is refused as stale.
func TestCursorsResumeExactlyAndStaleOnesAreRefused(t *testing.T) {
	m := testManifest(t)
	m.Commit.Strict = true // a generation per record, so cursors can straddle them
	w, dir := createTest(t, m)
	for seq := range uint64(6) {
		if err := w.AppendFrame(synthFrame(seq, 10)); err != nil {
			t.Fatal(err)
		}
	}
	f, err := Follow(dir, w, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := testContext(t, 5*time.Second)
	for range 3 {
		if _, err := f.Next(ctx); err != nil {
			t.Fatal(err)
		}
	}
	checkpoint := f.Cursor()
	if checkpoint.Record != 3 || checkpoint.Generation != 6 {
		t.Fatalf("cursor = %+v", checkpoint)
	}
	resumed, err := Follow(dir, w, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := resumed.Seek(checkpoint); err != nil {
		t.Fatal(err)
	}
	if rec, err := resumed.Next(ctx); err != nil || rec.Frame.Sequence != 3 {
		t.Fatalf("resumed at %+v, %v", rec, err)
	}

	other, _ := createTest(t, testManifest(t))
	for name, c := range map[string]Cursor{
		"another container":     other.cursorForTest(0),
		"uncommitted":           {CaptureUUID: checkpoint.CaptureUUID, ContainerTag: checkpoint.ContainerTag, Record: 3, Generation: 9},
		"beyond its generation": {CaptureUUID: checkpoint.CaptureUUID, ContainerTag: checkpoint.ContainerTag, Record: 5, Generation: 2},
	} {
		if err := resumed.Seek(c); !errors.Is(err, ErrStaleCursor) {
			t.Fatalf("%s: seek = %v", name, err)
		}
	}
	if err := resumed.SeekSequence(6); !errors.Is(err, ErrNotCommitted) || !errors.Is(err, ErrSequenceNotFound) {
		t.Fatalf("seek past the frontier = %v", err)
	}
	if _, err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Offline: a container that lost its later generations cannot honour a
	// cursor taken from them.
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	late := r.Cursor()
	late.Generation, late.Record = 7, 6
	for _, n := range []uint64{7, 6, 5} {
		if err := removeFile(dir, generationObject(n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeFile(dir, currentName); err != nil {
		t.Fatal(err)
	}
	if r, err = Open(dir, Options{}); err != nil || r.Status().Generation != 4 {
		t.Fatalf("truncated chain: %v", err)
	}
	if err := r.SeekCursor(late); !errors.Is(err, ErrStaleCursor) {
		t.Fatalf("a cursor from lost generations = %v", err)
	}
	early := late
	early.Generation, early.Record = 2, 2
	if err := r.SeekCursor(early); err != nil {
		t.Fatalf("a cursor inside the surviving prefix = %v", err)
	}
}

func (w *Writer) cursorForTest(record uint64) Cursor {
	return Cursor{CaptureUUID: w.manifest.Capture.UUID, ContainerTag: hex.EncodeToString(w.tag[:]), Record: record}
}

func removeFile(dir, rel string) error { return os.Remove(filepath.Join(dir, rel)) }

// Concurrent seeks by several followers while the writer commits: every
// seek inside the committed prefix lands on the record covering it, and one
// past it is refused as not committed, never answered with partial data.
func TestConcurrentSeeksDuringCommits(t *testing.T) {
	w, dir := liveWriter(t)
	written := make(chan []Record, 1)
	go func() { written <- produce(t, w, 120, 500*time.Microsecond, 3) }()
	var wg sync.WaitGroup
	var seeks, beyond atomic.Int64
	for i := range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, err := Follow(dir, w, Options{})
			if err != nil {
				t.Error(err)
				return
			}
			defer f.Close()
			rng := rand.New(rand.NewPCG(uint64(i), 9))
			ctx := testContext(t, 30*time.Second)
			for mine := 0; w.Frontier().State == CaptureOpen || mine < 30; mine++ {
				end := w.Frontier().EndSequence + 2
				seq := rng.Uint64N(end + 1)
				err := f.SeekSequence(seq)
				seeks.Add(1)
				if errors.Is(err, ErrNotCommitted) {
					beyond.Add(1)
					continue
				}
				if err != nil {
					t.Error(err)
					return
				}
				rec, err := f.Next(ctx)
				if err != nil {
					t.Error(err)
					return
				}
				if rec.Kind != RecordFrame || rec.Frame.Sequence != seq {
					t.Errorf("seek to %d landed on %s %d", seq, rec.Kind, rec.Frame.Sequence)
					return
				}
			}
		}()
	}
	wg.Wait()
	<-written
	if seeks.Load() < 50 || beyond.Load() == 0 {
		t.Fatalf("%d seeks, %d beyond the frontier", seeks.Load(), beyond.Load())
	}
}

// The end of a stream says how the capture ended.
func TestFollowerReportsHowTheCaptureEnded(t *testing.T) {
	ctx := testContext(t, 5*time.Second)
	drain := func(f *Follower) error {
		for {
			if _, err := f.Next(ctx); err != nil {
				return err
			}
		}
	}
	for name, tc := range map[string]struct {
		end  func(*Writer)
		want func(error) bool
	}{
		"closed":    {func(w *Writer) { w.Close() }, func(err error) bool { return errors.Is(err, io.EOF) }},
		"failed":    {func(w *Writer) { w.Fail("replay failed") }, func(err error) bool { return errors.Is(err, ErrCaptureFailed) }},
		"abandoned": {func(w *Writer) { w.Abandon() }, func(err error) bool { return errors.Is(err, io.ErrUnexpectedEOF) }},
	} {
		t.Run(name, func(t *testing.T) {
			w, dir := liveWriter(t)
			f, err := Follow(dir, w, Options{})
			if err != nil {
				t.Fatal(err)
			}
			for seq := range uint64(3) {
				if err := w.AppendFrame(synthFrame(seq, 5)); err != nil {
					t.Fatal(err)
				}
			}
			tc.end(w)
			if err := drain(f); !tc.want(err) {
				t.Fatalf("end = %v", err)
			}
			if f.Cursor().Record != 3 {
				t.Fatalf("drained %d records", f.Cursor().Record)
			}
		})
	}
}

// G-OBS-TIME through durable commits: every frame state, gaps of each
// kind, duplicate capture times, irregular cadence and writer-limit gaps
// keep their time and completeness semantics however the stream is split
// into generations; seeks by sequence and time resolve the same; a clock
// reset, not yet representable without epoch records, is refused without
// writing anything, and the capture continues; the end of capture is
// explicit.
func TestTimeAndCompletenessSurviveCommitGenerations(t *testing.T) {
	type step struct {
		frame  *l4bobserve.FrameRecord
		gap    *l4bobserve.GapRecord
		refuse bool
	}
	var steps []step
	frame := func(f l4bobserve.FrameRecord) { steps = append(steps, step{frame: &f}) }
	gap := func(g l4bobserve.GapRecord) { steps = append(steps, step{gap: &g}) }
	at := func(f l4bobserve.FrameRecord, start int64) l4bobserve.FrameRecord {
		f.FrameUnixNanos, f.CaptureStartUnixNanos, f.CaptureEndUnixNanos = start, start, start+50_000_000
		for i := range f.Clusters {
			f.Clusters[i].Summary.FirstMemberUnixNanos = start
		}
		return f
	}
	t0 := testFrameStart
	frame(at(synthFrame(0, 30), t0))                   // observed, complete
	frame(at(dispositionFrames(1)[0], t0+100_000_000)) // observed, empty
	partial := at(synthFrame(2, 20), t0+200_000_000)
	partial.Completeness = l4bobserve.Completeness{State: l4bobserve.CompletenessPartial, LostPackets: l4bobserve.KnownCount(5)}
	frame(partial)
	gap(l4bobserve.GapRecord{Cause: "l2-callback-queue-overflow"}) // dropped between received frames, time unknown
	for i, f := range dispositionFrames(2)[1:] {                   // unsettled, suppressed, failed
		frame(at(f, t0+int64(3+i)*100_000_000))
	}
	frame(at(synthFrame(6, 8), t0+500_000_000))    // the same capture start as frame 5
	for i, start := range []int64{640, 890, 900} { // irregular cadence: 140, 250 and 10 ms apart
		frame(at(synthFrame(uint64(7+i), 6), t0+start*1_000_000))
	}
	gap(l4bobserve.GapRecord{HasSequenceRange: true, FirstSequence: 10, LastSequence: 11, MissingFrames: l4bobserve.KnownCount(2),
		Time: l4bobserve.GapTimeBounded, StartUnixNanos: t0 + 2_000_000_000, EndUnixNanos: t0 + 2_200_000_000, Cause: "salvage"})
	frame(at(synthFrame(12, 9), t0+2_200_000_000))
	reset := at(synthFrame(13, 9), t0) // the clock steps back
	steps = append(steps, step{frame: &reset, refuse: true})
	frame(at(synthFrame(13, 9), t0+2_300_000_000))
	huge := at(synthFrame(14, 40), t0+2_400_000_000) // over the point limit: a writer-limit gap
	frame(huge)
	frame(at(synthFrame(15, 3), t0+2_500_000_000))

	write := func(m Manifest) (string, []Record, Summary) {
		m.Limits.MaxPointsPerFrame = 35
		w, dir := createTest(t, m)
		var want []Record
		for _, s := range steps {
			switch {
			case s.refuse:
				before := w.Frontier()
				if err := w.AppendFrame(*s.frame); err == nil {
					t.Fatal("a backwards clock step was accepted")
				}
				if after := w.Frontier(); after.AcceptedRecords != before.AcceptedRecords {
					t.Fatal("a refused frame was written")
				}
			case s.gap != nil:
				if err := w.AppendGap(*s.gap); err != nil {
					t.Fatal(err)
				}
				want = append(want, Record{Kind: RecordGap, Gap: *s.gap})
			case s.frame.Points.Len() > 35:
				if err := w.AppendFrame(*s.frame); !errors.Is(err, ErrFrameTooLarge) {
					t.Fatalf("oversized frame = %v", err)
				}
				want = append(want, Record{Kind: RecordGap})
			default:
				if err := w.AppendFrame(*s.frame); err != nil {
					t.Fatalf("frame %d: %v", s.frame.Sequence, err)
				}
				want = append(want, Record{Kind: RecordFrame, Frame: *s.frame})
			}
		}
		summary, err := w.Close()
		if err != nil {
			t.Fatal(err)
		}
		return dir, want, summary
	}
	m := testManifest(t)
	m.Limits = DefaultLimits()
	oneBatch, _, whole := write(m)
	m.Commit.MaxBatchBytes = 1 // a generation per record
	dir, want, split := write(m)
	if whole.Semantic != split.Semantic || split.Commits+1 < uint64(len(want)) || whole.RejectedFrames != 1 {
		t.Fatalf("batching changed the evidence: %+v vs %+v", whole, split)
	}
	got := readAll(t, dir, Options{})
	for i := range want {
		if want[i].Kind == RecordGap && want[i].Gap.Cause == "" {
			want[i].Gap = got[i].Gap // the writer's own limit gap
			if !got[i].Gap.HasSequenceRange || got[i].Gap.FirstSequence != 14 || got[i].Gap.Time != l4bobserve.GapTimeBounded {
				t.Fatalf("limit gap = %+v", got[i].Gap)
			}
		}
	}
	assertRecords(t, want, got)
	assertRecords(t, want, readAll(t, oneBatch, Options{}))

	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		seek func() error
		want uint64 // sequence of the landing frame, or of a sequence gap's first
		gap  bool
	}{
		{"sequence of the failed frame", func() error { return r.SeekSequence(5) }, 5, false},
		{"sequence inside the salvaged gap", func() error { return r.SeekSequence(11) }, 10, true},
		{"duplicate capture time lands on the first", func() error { return r.SeekTime(0, t0+500_000_000) }, 5, false},
		{"irregular cadence", func() error { return r.SeekTime(0, t0+880_000_000) }, 8, false},
		{"inside the bounded gap", func() error { return r.SeekTime(0, t0+2_100_000_000) }, 10, true},
		{"the limit gap", func() error { return r.SeekSequence(14) }, 14, true},
	} {
		if err := tc.seek(); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		rec, err := r.Next()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if tc.gap != (rec.Kind == RecordGap) || (rec.Kind == RecordFrame && rec.Frame.Sequence != tc.want) ||
			(rec.Kind == RecordGap && rec.Gap.FirstSequence != tc.want) {
			t.Fatalf("%s landed on %+v", tc.name, rec)
		}
	}
	if err := r.SeekSequence(16); !errors.Is(err, ErrSequenceNotFound) {
		t.Fatalf("seek past the end of capture = %v", err)
	}
	if err := r.SeekTime(0, t0+10_000_000_000); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("after the end of capture: %v", err)
	}
	if st := r.Status(); !st.Closed || st.EndSequence != 16 || st.RejectedFrames != 1 || st.TailExtentUnknown {
		t.Fatalf("end of capture = %+v", st)
	}
}

// An offline reader of a container still being written sees the pointer's
// prefix; Refresh catalogues generations committed since, and a pointer that
// goes backwards is refused as stale rather than silently shrinking the
// prefix a consumer has read.
func TestRefreshCataloguesNewGenerations(t *testing.T) {
	m := testManifest(t)
	m.Commit.MaxBatchAge = time.Hour
	w, dir := createTest(t, m)
	for seq := range uint64(2) {
		if err := w.AppendFrame(synthFrame(seq, 8)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := r.Refresh(); err != nil || n != 0 {
		t.Fatalf("refresh with nothing new = %d, %v", n, err)
	}
	stale, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for seq := uint64(2); seq < 5; seq++ {
		if err := w.AppendFrame(synthFrame(seq, 8)); err != nil {
			t.Fatal(err)
		}
		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := r.Refresh(); err != nil || n != 3 || r.Status().Frames != 5 {
		t.Fatalf("refresh = %d, %v; status %+v", n, err, r.Status())
	}
	var got []uint64
	for {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, rec.Frame.Sequence)
	}
	if len(got) != 5 || got[4] != 4 {
		t.Fatalf("read %v after refresh", got)
	}
	// Point current back at generation 1, as a lying device might.
	c, err := encodeCurrent(r.tag, stale.chain)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, currentName), c, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Refresh(); !errors.Is(err, ErrStaleCursor) {
		t.Fatalf("a pointer that went backwards = %v", err)
	}
}
