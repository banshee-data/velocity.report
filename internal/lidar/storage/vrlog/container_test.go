package vrlog

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// smallChunks forces several chunks from a handful of synthetic frames.
func smallChunks() Limits {
	l := DefaultLimits()
	l.TargetChunkBytes = 8 << 10
	return l
}

// stream is a synthetic extraction exercising every record shape: frames
// of several sizes, each disposition, a gap between received frames with
// unknown time, and a bounded gap over assigned sequences.
type stream struct {
	frames []l4bobserve.FrameRecord
	gaps   map[int]l4bobserve.GapRecord // written before frames[i]
}

func testStream() stream {
	s := stream{gaps: map[int]l4bobserve.GapRecord{}}
	for seq := range uint64(6) {
		s.frames = append(s.frames, synthFrame(seq, 40+int(seq)*30))
	}
	for _, f := range dispositionFrames(6) {
		s.frames = append(s.frames, f)
	}
	s.gaps[3] = l4bobserve.GapRecord{Cause: "l2-callback-queue-overflow"}
	// Sequences 10 and 11 were assigned and lost; frames resume at 12.
	s.gaps[10] = l4bobserve.GapRecord{HasSequenceRange: true, FirstSequence: 10, LastSequence: 11,
		MissingFrames: l4bobserve.KnownCount(2), Time: l4bobserve.GapTimeBounded,
		StartUnixNanos: testFrameStart + 10*100_000_000, EndUnixNanos: testFrameStart + 12*100_000_000 - 1, Cause: "salvage"}
	for seq := uint64(12); seq < 20; seq++ {
		s.frames = append(s.frames, synthFrame(seq, 25))
	}
	return s
}

// write appends the stream in order and returns the records a reader must
// yield, in order.
func (s stream) write(t *testing.T, w *Writer) []Record {
	t.Helper()
	var want []Record
	for i, f := range s.frames {
		if g, ok := s.gaps[i]; ok {
			if err := w.AppendGap(g); err != nil {
				t.Fatalf("gap before frame %d: %v", i, err)
			}
			want = append(want, Record{Kind: RecordGap, Gap: g})
		}
		if err := w.AppendFrame(f); err != nil {
			t.Fatalf("frame %d: %v", f.Sequence, err)
		}
		want = append(want, Record{Kind: RecordFrame, Frame: f})
	}
	return want
}

func writeTestContainer(t *testing.T, limits Limits) (string, []Record, Summary) {
	t.Helper()
	m := testManifest(t)
	m.Limits = limits
	w, dir := createTest(t, m)
	want := testStream().write(t, w)
	summary, err := w.Close()
	if err != nil {
		t.Fatal(err)
	}
	return dir, want, summary
}

func assertRecords(t *testing.T, want, got []Record) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("read %d records, wrote %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Kind != want[i].Kind {
			t.Fatalf("record %d is a %s, want %s", i, got[i].Kind, want[i].Kind)
		}
		switch want[i].Kind {
		case RecordFrame:
			if err := l4bobserve.DiffFrames(want[i].Frame, got[i].Frame); err != nil {
				t.Fatalf("frame %d: %v", want[i].Frame.Sequence, err)
			}
			if l4bobserve.FrameDigest(want[i].Frame) != l4bobserve.FrameDigest(got[i].Frame) {
				t.Fatalf("frame %d: semantic digests differ", want[i].Frame.Sequence)
			}
		case RecordGap:
			if err := l4bobserve.DiffGaps(want[i].Gap, got[i].Gap); err != nil {
				t.Fatalf("record %d: %v", i, err)
			}
		}
	}
}

func TestContainerRoundTripsAStreamAcrossChunks(t *testing.T) {
	dir, want, summary := writeTestContainer(t, smallChunks())
	r, err := Open(dir, Options{Require: l4bobserve.ForegroundComplete().Capabilities.List()})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if len(r.Chunks()) < 3 {
		t.Fatalf("expected several chunks, got %d", len(r.Chunks()))
	}
	st := r.Status()
	if !st.Closed || len(st.UnsealedTail) != 0 || st.EndSequence != 20 || st.Frames != 18 || st.Gaps != 2 {
		t.Fatalf("status = %+v", st)
	}
	stored, ok := r.Summary()
	if !ok || stored != summary {
		t.Fatalf("summary read %+v, written %+v", stored, summary)
	}
	assertRecords(t, want, readAll(t, dir, Options{}))

	report, err := r.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if report.Semantic != summary.Semantic || report.AncillarySkipped != 0 {
		t.Fatalf("verify = %+v", report)
	}
	// The manifest reads back as written, identity allocated by the writer.
	m := r.Manifest()
	if m.Capture.UUID == "" || m.Capture.CreatedUnixNanos == 0 || m.Profile.Name != l4bobserve.ProfileForegroundComplete {
		t.Fatalf("manifest = %+v", m.Capture)
	}
	if o, ok := m.MetadataObject("tuning"); !ok || string(o.Content) != `{"l4":{}}` {
		t.Fatalf("metadata = %+v", m.Metadata)
	}
}

// The stream digest is a property of the records, not of their chunking.
func TestSemanticDigestIsIndependentOfChunking(t *testing.T) {
	_, _, small := writeTestContainer(t, smallChunks())
	_, _, large := writeTestContainer(t, DefaultLimits())
	if small.Chunks == large.Chunks || small.Semantic != large.Semantic {
		t.Fatalf("chunks %d vs %d, digests %s vs %s", small.Chunks, large.Chunks, small.Semantic, large.Semantic)
	}
}

func TestWriterRotatesBeforeTheChunkBound(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxRecordBytes = 16 << 10
	limits.MaxChunkBytes = uint64(limits.MaxRecordBytes) + envelopeSize + chunkOverhead
	limits.TargetChunkBytes = limits.MaxChunkBytes
	limits.MaxRecordsPerChunk = 5
	dir, want, _ := writeTestContainer(t, limits)
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range r.Chunks() {
		if c.Bytes > limits.MaxChunkBytes || c.Records > uint64(limits.MaxRecordsPerChunk) {
			t.Fatalf("chunk %d: %d bytes, %d records", c.Ordinal, c.Bytes, c.Records)
		}
	}
	assertRecords(t, want, readAll(t, dir, Options{}))
}

// An over-limit frame is never truncated: it is refused with a typed error,
// a gap over its sequence takes its place, and the capture is marked
// incomplete. A frame exactly at the limit is accepted.
func TestOversizedFrameBecomesAnExplicitGap(t *testing.T) {
	f := synthFrame(0, 300)
	payload, err := marshalOptions.Marshal(frameToProto(f))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		limits func(*Limits)
		reject bool
	}{
		{"exactly at the record limit", func(l *Limits) { l.MaxRecordBytes = uint32(len(payload)) }, false},
		{"one byte over the record limit", func(l *Limits) { l.MaxRecordBytes = uint32(len(payload)) - 1 }, true},
		{"over the point limit", func(l *Limits) { l.MaxPointsPerFrame = 299 }, true},
		{"over the cluster limit", func(l *Limits) { l.MaxClustersPerFrame = 1 }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testManifest(t)
			m.Limits = DefaultLimits()
			tc.limits(&m.Limits)
			w, dir := createTest(t, m)
			err := w.AppendFrame(f)
			if !tc.reject {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var tooLarge *FrameTooLargeError
			if !errors.As(err, &tooLarge) || !errors.Is(err, ErrFrameTooLarge) || tooLarge.Sequence != 0 {
				t.Fatalf("error = %v", err)
			}
			// The stream continues at the next sequence.
			if err := w.AppendFrame(synthFrame(1, 3)); err != nil {
				t.Fatal(err)
			}
			summary, err := w.Close()
			if err != nil {
				t.Fatal(err)
			}
			if summary.RejectedFrames != 1 || summary.Frames != 1 || summary.Gaps != 1 {
				t.Fatalf("summary = %+v", summary)
			}
			got := readAll(t, dir, Options{})
			if len(got) != 2 || got[0].Kind != RecordGap || !got[0].Gap.HasSequenceRange || got[0].Gap.FirstSequence != 0 ||
				!strings.HasPrefix(got[0].Gap.Cause, "writer-limit: ") {
				t.Fatalf("records = %+v", got)
			}
		})
	}
}

func TestWriterRefusesABrokenStreamWithoutWriting(t *testing.T) {
	w, dir := createTest(t, testManifest(t))
	if err := w.AppendFrame(synthFrame(1, 3)); err == nil {
		t.Fatal("a frame out of sequence was accepted")
	}
	bad := synthFrame(0, 3)
	bad.Points.Fields &^= l4bobserve.FieldChannel
	bad.Points.Channel = nil
	if err := w.AppendFrame(bad); err == nil || !strings.Contains(err.Error(), "requires capability") {
		t.Fatalf("a frame lacking a profile column was accepted: %v", err)
	}
	if err := w.AppendFrame(synthFrame(0, 3)); err != nil {
		t.Fatalf("the stream did not continue after refusals: %v", err)
	}
	if _, err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, dir, Options{}); len(got) != 1 || got[0].Frame.Sequence != 0 {
		t.Fatalf("records = %+v", got)
	}
	if _, err := Create(dir, testManifest(t)); err == nil {
		t.Fatal("Create wrote into an existing container")
	}
}

func TestSeekBySequenceAndTime(t *testing.T) {
	dir, want, _ := writeTestContainer(t, smallChunks())
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	next := func() Record {
		t.Helper()
		rec, err := r.Next()
		if err != nil {
			t.Fatal(err)
		}
		return rec
	}
	for _, seq := range []uint64{0, 5, 13, 19} {
		if err := r.SeekSequence(seq); err != nil {
			t.Fatal(err)
		}
		if rec := next(); rec.Kind != RecordFrame || rec.Frame.Sequence != seq {
			t.Fatalf("seek %d landed on %+v", seq, rec)
		}
	}
	// Sequence 11 was lost: the seek lands on the gap that says so.
	if err := r.SeekSequence(11); err != nil {
		t.Fatal(err)
	}
	if rec := next(); rec.Kind != RecordGap || rec.Gap.FirstSequence != 10 {
		t.Fatalf("seek 11 landed on %+v", rec)
	}
	if err := r.SeekSequence(20); !errors.Is(err, ErrSequenceNotFound) {
		t.Fatalf("seek past the end = %v", err)
	}

	frameStart := func(seq uint64) int64 { return testFrameStart + int64(seq)*100_000_000 }
	for _, tc := range []struct {
		name   string
		at     int64
		kind   RecordKind
		first  uint64
		cause  string
		atTime bool
	}{
		{"before the stream", frameStart(0) - 1e9, RecordFrame, 0, "", false},
		{"exactly a frame start", frameStart(5), RecordFrame, 5, "", false},
		{"inside a frame lands on it", frameStart(12) + 50_000_000, RecordFrame, 12, "", false},
		// Frame 3 is preceded by a gap of unknown time: the loss may be there.
		{"start of the frame after an unknown gap", frameStart(3), RecordGap, 3, "l2-callback-queue-overflow", false},
		{"inside a bounded gap lands on it", frameStart(11), RecordGap, 10, "salvage", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := r.SeekTime(0, tc.at); err != nil {
				t.Fatal(err)
			}
			rec := next()
			switch {
			case rec.Kind != tc.kind:
				t.Fatalf("landed on a %s: %+v", rec.Kind, rec)
			case rec.Kind == RecordFrame && rec.Frame.Sequence != tc.first:
				t.Fatalf("landed on frame %d", rec.Frame.Sequence)
			case rec.Kind == RecordGap && rec.Gap.Cause != tc.cause:
				t.Fatalf("landed on gap %+v", rec.Gap)
			}
		})
	}
	if err := r.SeekTime(0, frameStart(100)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("seek past the end then Next = %v", err)
	}
	if err := r.SeekTime(1, 0); err == nil {
		t.Fatal("an undeclared epoch was accepted")
	}
	r.Reset()
	if rec := next(); rec.Frame.Sequence != 0 || len(want) == 0 {
		t.Fatal("Reset did not return to the first record")
	}
}

// writeRawManifest writes a container whose manifest is m exactly, bypassing
// the writer's own checks, to test what a reader refuses.
func writeRawManifest(t *testing.T, m *pb.RecordingManifest, preamble func([]byte)) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "raw.vrlog")
	if err := os.MkdirAll(filepath.Join(dir, chunksDir), 0o755); err != nil {
		t.Fatal(err)
	}
	payload, err := marshalOptions.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	object := appendRecord(appendPreamble(nil, objectManifest), envelope{kind: RecordManifest}, payload)
	if preamble != nil {
		preamble(object)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), object, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func validRawManifest(t *testing.T) *pb.RecordingManifest {
	t.Helper()
	m := testManifest(t)
	m.Profile = l4bobserve.ForegroundComplete()
	m.Limits = DefaultLimits()
	m.Capture.UUID = "00000000-0000-4000-8000-000000000000"
	return m.toProto()
}

func TestReaderRefusesWhatItCannotInterpret(t *testing.T) {
	if _, err := Open(writeRawManifest(t, validRawManifest(t), nil), Options{}); err != nil {
		t.Fatalf("the unmodified raw manifest was refused: %v", err)
	}
	for name, tc := range map[string]struct {
		mutate      func(*pb.RecordingManifest)
		preamble    func([]byte)
		unsupported bool
		want        string
	}{
		"newer major version":  {preamble: func(b []byte) { b[12] = 2 }, unsupported: true, want: "major version 2"},
		"required feature":     {mutate: func(m *pb.RecordingManifest) { m.RequiredFeatures = []string{"codec/zstd-blocks"} }, unsupported: true, want: "codec/zstd-blocks"},
		"unknown profile":      {mutate: func(m *pb.RecordingManifest) { m.Profile = "foreground-complete-v2" }, unsupported: true, want: "foreground-complete-v2"},
		"schema version":       {mutate: func(m *pb.RecordingManifest) { m.SchemaVersion = 2 }, unsupported: true, want: "schema version"},
		"stream kind":          {mutate: func(m *pb.RecordingManifest) { m.StreamKind = 7 }, unsupported: true, want: "stream kind"},
		"digest version":       {mutate: func(m *pb.RecordingManifest) { m.SemanticDigestVersion = "x/v9" }, unsupported: true, want: "semantic digest"},
		"mislabelled profile":  {mutate: func(m *pb.RecordingManifest) { m.Capabilities = m.Capabilities[1:] }, want: "which declares"},
		"limit over a ceiling": {mutate: func(m *pb.RecordingManifest) { m.Limits.MaxRecordBytes = HardMaxRecordBytes + 1 }, want: "record bound"},
		"source identity":      {mutate: func(m *pb.RecordingManifest) { m.Extraction.ExtractorId = "l4.other/v1" }, want: "source identity"},
		"calibration identity": {mutate: func(m *pb.RecordingManifest) { m.Calibration.Transform[3] = 1 }, want: "calibration identity"},
		"metadata digest":      {mutate: func(m *pb.RecordingManifest) { m.Metadata[0].Content = []byte("{}") }, want: "SHA-256"},
		"transform length":     {mutate: func(m *pb.RecordingManifest) { m.Calibration.Transform = m.Calibration.Transform[:15] }, want: "16"},
	} {
		t.Run(name, func(t *testing.T) {
			m := validRawManifest(t)
			if tc.mutate != nil {
				tc.mutate(m)
			}
			_, err := Open(writeRawManifest(t, m, tc.preamble), Options{})
			if err == nil || !strings.Contains(err.Error(), tc.want) || errors.Is(err, ErrUnsupported) != tc.unsupported {
				t.Fatalf("error = %v, want one containing %q (unsupported %v)", err, tc.want, tc.unsupported)
			}
		})
	}
}

// A reader asking for foreground-complete evidence refuses a reduced source,
// naming every missing capability; a reader asking for less accepts it.
func TestCapabilityRequirementRefusesAReducedSource(t *testing.T) {
	m := validRawManifest(t)
	reduced := l4bobserve.ReducedClusterSample()
	m.Profile = string(reduced.Name)
	m.Capabilities = nil
	for _, c := range reduced.Capabilities.List() {
		m.Capabilities = append(m.Capabilities, string(c))
	}
	dir := writeRawManifest(t, m, nil)
	_, err := Open(dir, Options{Require: l4bobserve.ForegroundComplete().Capabilities.List()})
	var missing *l4bobserve.MissingCapabilitiesError
	if !errors.As(err, &missing) || !errors.Is(err, l4bobserve.ErrMissingCapability) || missing.Profile != reduced.Name {
		t.Fatalf("error = %v", err)
	}
	for _, c := range []l4bobserve.Capability{l4bobserve.CapabilityCompleteForeground, l4bobserve.CapabilityClusterMembership, l4bobserve.CapabilityFrameRecords} {
		if !strings.Contains(err.Error(), string(c)) {
			t.Fatalf("%v does not name %s", err, c)
		}
	}
	if _, err := Open(dir, Options{Require: []l4bobserve.Capability{l4bobserve.CapabilityClusterSummaries}}); err != nil {
		t.Fatalf("a request the reduced source satisfies was refused: %v", err)
	}
	if _, err := Create(filepath.Join(t.TempDir(), "x"), Manifest{Profile: reduced}); err == nil {
		t.Fatal("the writer accepted a profile its records do not satisfy")
	}
}

// G-OBS-COMPAT, new-reader side: a legacy 0.5 recording is refused by name,
// never parsed as observations.
func TestReaderRefusesLegacyRecordings(t *testing.T) {
	legacy := t.TempDir()
	for _, name := range []string{"header.json", "index.bin"} {
		if err := os.WriteFile(filepath.Join(legacy, name), []byte(`{"version":"0.5"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := Open(legacy, Options{})
	if !errors.Is(err, ErrLegacyRecording) || !errors.Is(err, ErrNotContainer) || !strings.Contains(err.Error(), "FrameBundle") {
		t.Fatalf("legacy recording: %v", err)
	}
	if IsContainer(legacy) {
		t.Fatal("IsContainer claimed a legacy recording")
	}
	if _, err := Open(t.TempDir(), Options{}); !errors.Is(err, ErrNotContainer) || errors.Is(err, ErrLegacyRecording) {
		t.Fatalf("empty directory: %v", err)
	}
	// A manifest-named file that is not a manifest object is not a container.
	impostor := t.TempDir()
	if err := os.WriteFile(filepath.Join(impostor, manifestName), []byte(`{"version":"1.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(impostor, Options{}); !errors.Is(err, ErrNotContainer) {
		t.Fatalf("impostor manifest: %v", err)
	}
	dir, _, _ := writeTestContainer(t, DefaultLimits())
	if !IsContainer(dir) {
		t.Fatal("IsContainer missed a container")
	}
}

// injectRecord appends a record of an arbitrary kind at the writer's
// current position, as a newer writer might.
func injectRecord(t *testing.T, w *Writer, kind RecordKind) {
	t.Helper()
	record := appendRecord(nil, envelope{kind: kind, tag: w.tag, sequence: w.stream.NextSequence()}, []byte("future"))
	if err := w.append(record, entry{kind: kind, first: w.stream.NextSequence()}, l4bobserve.Digest{}); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownRecordKinds(t *testing.T) {
	ancillary := RecordKind(0x8001)
	w, dir := createTest(t, testManifest(t))
	if err := w.AppendFrame(synthFrame(0, 5)); err != nil {
		t.Fatal(err)
	}
	injectRecord(t, w, ancillary)
	if err := w.AppendFrame(synthFrame(1, 5)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, dir, Options{}); len(got) != 2 || got[1].Frame.Sequence != 1 {
		t.Fatalf("an ancillary record was not skipped: %+v", got)
	}
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report, err := r.Verify(); err != nil || report.AncillarySkipped != 1 {
		t.Fatalf("verify = %+v, %v", report, err)
	}

	w, dir = createTest(t, testManifest(t))
	if err := w.AppendFrame(synthFrame(0, 5)); err != nil {
		t.Fatal(err)
	}
	injectRecord(t, w, RecordKind(0x0042))
	if _, err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, Options{}); !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "critical record kind") {
		t.Fatalf("an unknown critical kind was accepted: %v", err)
	}
}

// An interrupted writer leaves at most an unsealed tail, which is reported
// and never read; Abandon seals what a healthy writer holds.
func TestUnsealedTailIsNeverEvidence(t *testing.T) {
	w, dir := createTest(t, testManifest(t))
	for seq := range uint64(3) {
		if err := w.AppendFrame(synthFrame(seq, 10)); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate a crash: the open chunk is neither sealed nor summarised.
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	st := r.Status()
	if st.Closed || len(st.UnsealedTail) != 1 || st.Records != 0 {
		t.Fatalf("status = %+v", st)
	}
	if _, err := r.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("an unsealed record was read: %v", err)
	}
	if err := w.Abandon(); err != nil {
		t.Fatal(err)
	}
	r, err = Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st := r.Status(); st.Closed || len(st.UnsealedTail) != 0 || st.Frames != 3 {
		t.Fatalf("after Abandon: %+v", st)
	}
	if err := w.AppendFrame(synthFrame(3, 1)); err == nil {
		t.Fatal("an abandoned writer accepted a frame")
	}
}

// After a failed write the open chunk's last record may be partial, so the
// writer stops admitting evidence, never seals that chunk, and writes no
// summary: the container reads as interrupted, with its sealed prefix intact.
func TestWriteFailureIsStickyAndNeverSealed(t *testing.T) {
	m := testManifest(t)
	m.Limits = smallChunks()
	w, dir := createTest(t, m)
	for seq := range uint64(6) {
		if err := w.AppendFrame(synthFrame(seq, 40)); err != nil {
			t.Fatal(err)
		}
	}
	sealed := w.summary.Chunks
	if sealed == 0 || w.chunk == nil {
		t.Fatalf("fixture needs sealed chunks and an open one: %d sealed, open %v", sealed, w.chunk != nil)
	}
	w.chunk.file.Close() // the next write fails, as on a lost device
	if err := w.AppendFrame(synthFrame(6, 40)); err == nil {
		t.Fatal("a write to a failed chunk succeeded")
	}
	if err := w.AppendFrame(synthFrame(7, 40)); err == nil || !strings.Contains(err.Error(), "failed earlier") {
		t.Fatalf("the failure was not sticky: %v", err)
	}
	if _, err := w.Close(); err == nil {
		t.Fatal("Close reported success after a failed write")
	}
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	st := r.Status()
	if st.Closed || len(st.UnsealedTail) != 1 || uint64(st.Chunks) != sealed {
		t.Fatalf("status = %+v", st)
	}
	if _, err := r.Verify(); err != nil {
		t.Fatalf("the sealed prefix does not verify: %v", err)
	}
}

func TestEmptyCaptureCloses(t *testing.T) {
	w, dir := createTest(t, testManifest(t))
	summary, err := w.Close()
	if err != nil || summary.Chunks != 0 || summary.Records != 0 {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
	if got := readAll(t, dir, Options{}); len(got) != 0 {
		t.Fatalf("records = %+v", got)
	}
}
