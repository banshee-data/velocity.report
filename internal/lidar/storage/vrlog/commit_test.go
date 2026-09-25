package vrlog

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// chainOf reads every generation of dir in order.
func chainOf(t *testing.T, dir string) []*pb.CommitGeneration {
	t.Helper()
	r, err := openManifest(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var out []*pb.CommitGeneration
	for n := uint64(0); ; n++ {
		g, _, err := readGeneration(dir, n, r.tag)
		if isMissing(err) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, g)
	}
}

func testContext(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// eventually polls cond until it holds or the test's patience runs out.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// Group commit: an append is accepted, not durable; the batch commits at the
// age bound, or at the byte threshold, as its own generation; the policy and
// its crash-loss bound are declared in the manifest, and each generation
// records its batch age and synchronisation time.
func TestGroupCommitClosesBatchesByAgeAndBytes(t *testing.T) {
	m := testManifest(t)
	m.Commit.MaxBatchAge = 30 * time.Millisecond
	w, dir := createTest(t, m)
	start := time.Now()
	if err := w.AppendFrame(synthFrame(0, 20)); err != nil {
		t.Fatal(err)
	}
	if f := w.Frontier(); f.Records != 0 || f.AcceptedRecords != 1 || f.AcceptedEndSequence != 1 || f.State != CaptureOpen {
		t.Fatalf("an accepted frame was reported durable at once: %+v", f)
	}
	f, err := w.WaitFrontier(testContext(t, 5*time.Second), 0)
	if err != nil || f.Generation != 1 || f.Records != 1 || f.EndSequence != 1 {
		t.Fatalf("frontier = %+v, %v", f, err)
	}
	if waited := time.Since(start); waited < m.Commit.MaxBatchAge {
		t.Fatalf("the batch committed after %s, before its age bound", waited)
	}
	if _, err := w.Close(); err != nil {
		t.Fatal(err)
	}

	m = testManifest(t)
	m.Commit.MaxBatchAge = time.Hour
	m.Commit.MaxBatchBytes = 4 << 10
	w, bytesDir := createTest(t, m)
	for seq := range uint64(8) {
		if err := w.AppendFrame(synthFrame(seq, 60)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Close(); err != nil {
		t.Fatal(err)
	}

	triggers := map[pb.CommitTrigger]int{}
	for _, d := range []string{dir, bytesDir} {
		chain := chainOf(t, d)
		for i, g := range chain {
			triggers[g.Trigger]++
			if len(g.Chunks) > 0 && (g.BatchAgeNanos < 0 || g.SyncNanos <= 0 || g.CommittedUnixNanos == 0) {
				t.Fatalf("generation %d does not record its measurements: %+v", i, g)
			}
		}
		if last := chain[len(chain)-1]; last.Kind != pb.GenerationKind_GENERATION_KIND_CLOSE || chain[0].Kind != pb.GenerationKind_GENERATION_KIND_OPEN {
			t.Fatalf("chain runs %s to %s", chain[0].Kind, last.Kind)
		}
	}
	if triggers[pb.CommitTrigger_COMMIT_TRIGGER_BATCH_AGE] != 1 || triggers[pb.CommitTrigger_COMMIT_TRIGGER_BATCH_BYTES] < 2 {
		t.Fatalf("triggers = %v", triggers)
	}

	r, err := Open(bytesDir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	policy := r.Manifest().Commit
	if policy != w.Policy() || policy.MaxBatchBytes != 4<<10 || policy.CommitDeadline != DefaultCommitDeadline {
		t.Fatalf("declared policy %+v, writer used %+v", policy, w.Policy())
	}
	loss := policy.CrashLoss(r.Manifest().Limits)
	if loss.Interval != time.Hour+DefaultCommitDeadline || loss.Frames != uint64((time.Hour+DefaultCommitDeadline).Seconds()*DefaultMaxFrameRateHz)+1 {
		t.Fatalf("crash loss = %+v", loss)
	}
	if s, ok := r.Summary(); !ok || s.Commits == 0 || s.CommitLatency.Max <= 0 || s.CommitLatency.P50 > s.CommitLatency.Max {
		t.Fatalf("summary latency = %+v", s)
	}
}

// Strict mode: every append returns only once its record is durable.
func TestStrictModeMakesEachAppendDurable(t *testing.T) {
	m := testManifest(t)
	m.Commit.Strict = true
	w, dir := createTest(t, m)
	for seq := range uint64(4) {
		if err := w.AppendFrame(synthFrame(seq, 10)); err != nil {
			t.Fatal(err)
		}
		if f := w.Frontier(); f.Records != seq+1 || f.Generation != seq+1 {
			t.Fatalf("after strict append %d: %+v", seq, f)
		}
	}
	if _, err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for _, g := range chainOf(t, dir)[1:5] {
		if g.Trigger != pb.CommitTrigger_COMMIT_TRIGGER_STRICT || len(g.Chunks) != 1 || g.Chunks[0].RecordCount != 1 {
			t.Fatalf("strict generation = %+v", g)
		}
	}
}

// The frontier moves only after the last step of publication, the final
// directory sync, has returned: at every earlier point the generation being
// published is not announced.
func TestFrontierIsAnnouncedOnlyAfterEveryStep(t *testing.T) {
	m := testManifest(t)
	m.Commit.MaxBatchAge = time.Hour
	w, _ := createTest(t, m)
	var seen, early atomic.Int32
	w.hooks.Store(&writerHooks{kill: func(step commitStep, generation uint64) bool {
		seen.Add(1)
		if w.Frontier().Generation >= generation {
			early.Add(1)
		}
		return false
	}})
	if err := w.AppendFrame(synthFrame(0, 10)); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if seen.Load() != int32(stepPointerDurable) || early.Load() != 0 {
		t.Fatalf("%d steps seen, generation announced early at %d of them", seen.Load(), early.Load())
	}
	if f := w.Frontier(); f.Generation != 1 || f.Records != 1 {
		t.Fatalf("frontier after the flush = %+v", f)
	}
}

// A synchronisation that stalls beyond the declared bound puts the capture
// into failure and stops admission. The stalled commit may still complete;
// then the failure marker records the committed and accepted extents, and
// only then is the frontier final.
func TestCommitStallFailsTheCapture(t *testing.T) {
	m := testManifest(t)
	m.Commit.MaxBatchAge = 5 * time.Millisecond
	m.Commit.CommitDeadline = 40 * time.Millisecond
	w, dir := createTest(t, m)
	release := make(chan struct{})
	w.hooks.Store(&writerHooks{sync: func(name string) error {
		if strings.HasSuffix(name, chunkSuffix+openSuffix) {
			<-release
		}
		return nil
	}})
	if err := w.AppendFrame(synthFrame(0, 10)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the stall to fail the capture", func() bool { return w.Frontier().State == CaptureFailed })
	f := w.Frontier()
	if f.Failure == nil || f.Failure.Cause != FailureStall || f.Final || f.Records != 0 || f.AcceptedRecords != 1 {
		t.Fatalf("frontier during the stall = %+v", f)
	}
	if err := w.AppendFrame(synthFrame(1, 10)); !errors.Is(err, ErrCaptureFailed) {
		t.Fatalf("admission continued during a stall: %v", err)
	}
	close(release)
	f, err := w.WaitFrontier(testContext(t, 5*time.Second), f.Generation)
	for err == nil && !f.Final {
		f, err = w.WaitFrontier(testContext(t, 5*time.Second), f.Generation)
	}
	if err != nil || f.Records != 1 || !f.Failure.MarkerWritten || f.Failure.CommittedEndSequence != 1 {
		t.Fatalf("final frontier = %+v (%+v), %v", f, f.Failure, err)
	}
	if _, err := w.Close(); !errors.Is(err, ErrCaptureFailed) {
		t.Fatalf("Close after a stall = %v", err)
	}
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	st := r.Status()
	if st.State != CaptureFailed || st.Failure.Cause != FailureStall || st.EndSequence != 1 || st.Failure.AcceptedEndSequence != 1 {
		t.Fatalf("status = %+v, failure %+v", st, st.Failure)
	}
}

// On a full disk the writer stops admission, frees its reserve and uses it
// for the failure marker. When even that cannot be written, the container is
// left without one, and recovery says the tail extent is unknown.
func TestDiskFullStopsAdmissionAndSpendsTheReserve(t *testing.T) {
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
	reserve := filepath.Join(dir, reserveName)
	// Chunk writes find no space; small objects find space only once the
	// reserve is released.
	w.hooks.Store(&writerHooks{write: func(name string) error {
		if _, err := os.Stat(reserve); err == nil || strings.HasPrefix(name, chunksDir+"/") {
			return syscall.ENOSPC
		}
		return nil
	}})
	err := w.AppendFrame(synthFrame(4, 10))
	var failed *CaptureFailedError
	if !errors.As(err, &failed) || failed.Cause != FailureDiskFull || failed.AcceptedEndSequence != 4 || failed.CommittedEndSequence != 3 {
		t.Fatalf("disk full = %v", err)
	}
	if _, err := w.Close(); !errors.Is(err, ErrCaptureFailed) {
		t.Fatal(err)
	}
	if f := w.Frontier(); !f.Failure.MarkerWritten {
		t.Fatal("the failure marker was not written from the reserve")
	}
	if _, err := os.Stat(reserve); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("reserve still held: %v", err)
	}
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st := r.Status(); st.State != CaptureFailed || st.Failure.Cause != FailureDiskFull || st.Records != 3 || st.TailExtentUnknown {
		t.Fatalf("status = %+v", st)
	}

	// With no space at all, not even for the marker.
	w, dir = createTest(t, m)
	if err := w.AppendFrame(synthFrame(0, 10)); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	w.hooks.Store(&writerHooks{write: func(string) error { return syscall.ENOSPC }})
	if err := w.AppendFrame(synthFrame(1, 10)); !errors.Is(err, ErrCaptureFailed) {
		t.Fatalf("disk full = %v", err)
	}
	if _, err := w.Close(); !errors.Is(err, ErrCaptureFailed) {
		t.Fatal(err)
	}
	if w.Frontier().Failure.MarkerWritten {
		t.Fatal("a marker was reported written to a disk with no space")
	}
	if r, err = Open(dir, Options{}); err != nil {
		t.Fatal(err)
	}
	if st := r.Status(); st.State != CaptureOpen || !st.TailExtentUnknown || st.Records != 1 {
		t.Fatalf("status without a marker = %+v", st)
	}
	report, err := Recover(dir, Options{})
	if err != nil || report.State != CaptureIncomplete || report.Records != 1 {
		t.Fatalf("recover = %+v, %v", report, err)
	}
}

// Under the shed policy, a frame that cannot be admitted while a commit is
// pending becomes an explicit gap over its sequence: the loss is recorded,
// counted and returned, never silent, and the capture continues.
func TestShedFramesBecomeExplicitGaps(t *testing.T) {
	m := testManifest(t)
	m.Commit.MaxBatchAge = time.Hour
	m.Commit.MaxBatchBytes = 1 // every record fills its batch
	m.Commit.ShedAfter = 20 * time.Millisecond
	w, dir := createTest(t, m)
	release := make(chan struct{})
	var blocked atomic.Bool
	w.hooks.Store(&writerHooks{sync: func(name string) error {
		if name == chunkObject(0)+openSuffix {
			blocked.Store(true)
			<-release
		}
		return nil
	}})
	if err := w.AppendFrame(synthFrame(0, 10)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the first commit to block", blocked.Load)
	if err := w.AppendFrame(synthFrame(1, 10)); err != nil {
		t.Fatalf("the open batch should take one frame: %v", err)
	}
	for seq := uint64(2); seq < 4; seq++ {
		err := w.AppendFrame(synthFrame(seq, 10))
		var shed *FrameShedError
		if !errors.As(err, &shed) || shed.Sequence != seq || shed.Waited < m.Commit.ShedAfter {
			t.Fatalf("frame %d under backlog = %v", seq, err)
		}
	}
	close(release)
	if err := w.AppendFrame(synthFrame(4, 10)); err != nil {
		t.Fatal(err)
	}
	summary, err := w.Close()
	if err != nil || summary.ShedFrames != 2 || summary.Frames != 3 || summary.Gaps != 2 || summary.EndSequence != 5 {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
	got := readAll(t, dir, Options{})
	kinds := ""
	for _, rec := range got {
		kinds += rec.Kind.String()[:1]
		if rec.Kind == RecordGap && (!strings.HasPrefix(rec.Gap.Cause, "writer-backlog") || !rec.Gap.MissingFrames.Known) {
			t.Fatalf("shed gap = %+v", rec.Gap)
		}
	}
	if kinds != "ffggf" {
		t.Fatalf("stream = %s", kinds)
	}
	r, _ := Open(dir, Options{})
	if st := r.Status(); st.ShedFrames != 2 {
		t.Fatalf("status = %+v", st)
	}
}

// Without shedding, an append that finds the open batch full waits for the
// committer (backpressure, as a paused PCAP reader allows) and nothing is
// lost.
func TestBackpressureWaitsForTheCommitter(t *testing.T) {
	m := testManifest(t)
	m.Commit.MaxBatchAge = time.Hour
	m.Commit.MaxBatchBytes = 1
	w, dir := createTest(t, m)
	const stall = 60 * time.Millisecond
	w.hooks.Store(&writerHooks{sync: func(name string) error {
		if name == chunkObject(0)+openSuffix {
			time.Sleep(stall)
		}
		return nil
	}})
	start := time.Now()
	for seq := range uint64(3) {
		if err := w.AppendFrame(synthFrame(seq, 10)); err != nil {
			t.Fatal(err)
		}
	}
	if waited := time.Since(start); waited < stall/2 {
		t.Fatalf("the third append did not wait for the committer (%s)", waited)
	}
	summary, err := w.Close()
	if err != nil || summary.Frames != 3 || summary.Gaps != 0 || summary.ShedFrames != 0 {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
	if got := readAll(t, dir, Options{}); len(got) != 3 {
		t.Fatalf("read %d records", len(got))
	}
}

// Fail commits what was accepted, then records the owner's reason; Abandon
// commits and stops without a terminal generation, so the container reads
// as open until recovery.
func TestFailAndAbandonEndTheCaptureVisibly(t *testing.T) {
	m := testManifest(t)
	m.Commit.MaxBatchAge = time.Hour
	w, dir := createTest(t, m)
	for seq := range uint64(3) {
		if err := w.AppendFrame(synthFrame(seq, 10)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Fail("replay failed: source ended early"); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	st := r.Status()
	if st.State != CaptureFailed || st.Records != 3 || st.Failure.Cause != FailureStopped ||
		!strings.Contains(st.Failure.Detail, "source ended early") || st.Failure.AcceptedEndSequence != 3 {
		t.Fatalf("status = %+v, failure %+v", st, st.Failure)
	}

	w, dir = createTest(t, m)
	for seq := range uint64(3) {
		if err := w.AppendFrame(synthFrame(seq, 10)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Abandon(); err != nil {
		t.Fatal(err)
	}
	if f := w.Frontier(); f.State != CaptureIncomplete || !f.Final || f.Records != 3 {
		t.Fatalf("frontier after Abandon = %+v", f)
	}
	if r, err = Open(dir, Options{}); err != nil {
		t.Fatal(err)
	}
	if st := r.Status(); st.State != CaptureOpen || st.Records != 3 || !st.TailExtentUnknown || len(st.UncommittedTail) != 0 {
		t.Fatalf("status after Abandon = %+v", st)
	}
	if _, err := w.Close(); err == nil {
		t.Fatal("Close reported an abandoned writer closed")
	}
}

// A container's lock admits one writer or recovery at a time.
func TestTheLockAdmitsOneWriter(t *testing.T) {
	w, dir := createTest(t, testManifest(t))
	if _, err := Recover(dir, Options{}); !errors.Is(err, ErrContainerBusy) {
		t.Fatalf("recovery beside a live writer = %v", err)
	}
	if _, err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if report, err := Recover(dir, Options{}); err != nil || report.Wrote || report.PointerRewritten || report.State != CaptureClosed {
		t.Fatalf("recovery of a closed container = %+v, %v", report, err)
	}
}

func TestLatencyHistogramQuantiles(t *testing.T) {
	var h latencyHistogram
	for i := 1; i <= 100; i++ {
		h.add(time.Duration(i) * time.Millisecond)
	}
	s := h.summary()
	within := func(got, want time.Duration) bool { return got >= want && float64(got) <= float64(want)*1.1 }
	if s.Count != 100 || !within(s.P50, 50*time.Millisecond) || !within(s.P95, 95*time.Millisecond) || s.Max != 100*time.Millisecond || s.P99 > s.Max {
		t.Fatalf("summary = %+v", s)
	}
	var empty latencyHistogram
	if empty.summary() != (Latency{}) {
		t.Fatal("an empty histogram reports latency")
	}
}
