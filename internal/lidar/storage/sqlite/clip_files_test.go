package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupClipFilesDB builds the clip tables the way the test fixtures in
// this package do, plus the file list migration 041 adds.
func setupClipFilesDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile(filepath.Join("..", "..", "..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// splitSQLStatements strips comment lines and splits on statement boundaries.
// Comments are removed first because they carry no semicolons of their own and
// would otherwise be glued onto whichever statement followed them.
func splitSQLStatements(schema string) []string {
	var body strings.Builder
	for _, line := range strings.Split(schema, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}
	var out []string
	for _, stmt := range strings.Split(body.String(), ";") {
		if trimmed := strings.TrimSpace(stmt); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func insertCase(t *testing.T, store *ClipStore, id, pcap string) {
	t.Helper()
	c := &Clip{ClipID: id, SensorID: "hesai-pandar40p", Files: []ClipCapture{{PCAPFile: pcap}}}
	if err := store.InsertClip(c); err != nil {
		t.Fatalf("InsertClip: %v", err)
	}
}

func TestClipCapturesRoundTrip(t *testing.T) {
	store := NewClipStore(setupClipFilesDB(t))
	insertCase(t, store, "case-1", "file7.pcap")

	files := []ClipCapture{
		{Ordinal: 0, PCAPFile: "s2_sf_3_20260902134538_00007.pcap"},
		{Ordinal: 1, PCAPFile: "s2_sf_3_20260902135038_00008.pcap"},
	}
	if err := store.SetClipFiles("case-1", files); err != nil {
		t.Fatalf("SetClipFiles: %v", err)
	}

	got, err := store.ClipFiles("case-1")
	if err != nil {
		t.Fatalf("ClipFiles: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("stored %d files, want 2", len(got))
	}
	for i, want := range files {
		if got[i].PCAPFile != want.PCAPFile || got[i].Ordinal != i {
			t.Errorf("file %d = %+v, want %+v", i, got[i], want)
		}
	}

	paths, err := store.ClipPaths("case-1")
	if err != nil {
		t.Fatalf("ClipPaths: %v", err)
	}
	if len(paths) != 2 || paths[0] != files[0].PCAPFile {
		t.Errorf("ClipPaths = %v, want the ordered list", paths)
	}
}

func TestClipCapturesLoadTheCurrentAssociation(t *testing.T) {
	// Capture rows remain authoritative after replacing the association.
	store := NewClipStore(setupClipFilesDB(t))
	insertCase(t, store, "case-1", "old.pcap")

	if err := store.SetClipFiles("case-1", []ClipCapture{
		{Ordinal: 0, PCAPFile: "file7.pcap"},
		{Ordinal: 1, PCAPFile: "file8.pcap"},
	}); err != nil {
		t.Fatalf("SetClipFiles: %v", err)
	}
	got, err := store.GetClip("case-1")
	if err != nil {
		t.Fatalf("GetClip: %v", err)
	}
	if got.Files[0].PCAPFile != "file7.pcap" {
		t.Errorf("pcap_file = %q, want the first capture of the list", got.Files[0].PCAPFile)
	}
}

func TestClipCapturesReplaceWholesale(t *testing.T) {
	store := NewClipStore(setupClipFilesDB(t))
	insertCase(t, store, "case-1", "a.pcap")

	if err := store.SetClipFiles("case-1", []ClipCapture{
		{PCAPFile: "a.pcap"}, {PCAPFile: "b.pcap"}, {PCAPFile: "c.pcap"},
	}); err != nil {
		t.Fatalf("first SetClipFiles: %v", err)
	}
	if err := store.SetClipFiles("case-1", []ClipCapture{{PCAPFile: "z.pcap"}}); err != nil {
		t.Fatalf("second SetClipFiles: %v", err)
	}
	got, _ := store.ClipFiles("case-1")
	if len(got) != 1 || got[0].PCAPFile != "z.pcap" {
		t.Errorf("files = %+v, want just z.pcap", got)
	}
}

func TestClipCapturesRejectsEmptyInput(t *testing.T) {
	store := NewClipStore(setupClipFilesDB(t))
	insertCase(t, store, "case-1", "a.pcap")

	if err := store.SetClipFiles("case-1", nil); err == nil {
		t.Error("a case with no captures was accepted")
	}
	if err := store.SetClipFiles("", []ClipCapture{{PCAPFile: "a.pcap"}}); err == nil {
		t.Error("a file list with no case was accepted")
	}
	if err := store.SetClipFiles("case-1", []ClipCapture{{PCAPFile: "  "}}); err == nil {
		t.Error("a capture with no path was accepted")
	}
}

func TestClipCapturesPersistOnInsert(t *testing.T) {
	// Inserting a clip writes its capture association.
	store := NewClipStore(setupClipFilesDB(t))
	insertCase(t, store, "case-1", "legacy.pcap")

	got, err := store.ClipFiles("case-1")
	if err != nil {
		t.Fatalf("ClipFiles: %v", err)
	}
	if len(got) != 1 || got[0].PCAPFile != "legacy.pcap" {
		t.Errorf("files = %+v, want the legacy single file", got)
	}
}

func TestSetClipSession(t *testing.T) {
	store := NewClipStore(setupClipFilesDB(t))
	insertCase(t, store, "case-1", "a.pcap")

	if err := store.SetClipSession("case-1", "ses-1", "per-1"); err != nil {
		t.Fatalf("SetClipSession: %v", err)
	}
	if err := store.SetClipSession("case-nope", "ses-1", ""); err == nil {
		t.Error("linking an unknown case succeeded")
	}
}

// A case notes the session and motion period it was cut from, and the note is
// never a foreign key (data model review, finding 9). Re-deriving the index
// after the session's captures changed replaces the session and drops its
// periods; the case keeps its files, its window and its note, even when the
// note names a period that is gone. The platform vocabulary plan drops both
// columns in v0.6.7 (Item 18), and this test with them.
func TestACaseOutlivesTheSessionItWasCutFrom(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	captures := NewCaptureStore(db)
	sessionID := seedSession(t, captures)
	if err := captures.ReplaceSessionPeriods(sessionID, samplePeriods(captureBase)); err != nil {
		t.Fatalf("ReplaceSessionPeriods: %v", err)
	}
	periods, err := captures.ListSessionPeriods(sessionID)
	if err != nil || len(periods) != 3 {
		t.Fatalf("periods = %v (%v), want 3", periods, err)
	}
	cutFrom := periods[1]

	cases := NewClipStore(db)
	start, duration := cutFrom.StartSecs, cutFrom.EndSecs-cutFrom.StartSecs
	if err := cases.InsertClip(&Clip{ClipID: "case-1", SensorID: "hesai-pandar40p", Files: []ClipCapture{{PCAPFile: captureName(0)}}, PCAPStartSecs: &start, PCAPDurationSecs: &duration}); err != nil {
		t.Fatalf("InsertClip: %v", err)
	}
	files := []ClipCapture{{Ordinal: 0, PCAPFile: captureName(0)}, {Ordinal: 1, PCAPFile: captureName(1)}}
	if err := cases.SetClipFiles("case-1", files); err != nil {
		t.Fatalf("SetClipFiles: %v", err)
	}
	if err := cases.SetClipSession("case-1", sessionID, cutFrom.PeriodID); err != nil {
		t.Fatalf("SetClipSession: %v", err)
	}

	sessions, err := captures.ListSessions("")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %v (%v), want 1", sessions, err)
	}
	// A rescan of the same captures keeps the periods, so grow the session.
	extendRun(t, captures, sessions[0].RootID, 3)
	if _, err := captures.DeriveSessions(sessions[0].RootID); err != nil {
		t.Fatalf("re-derive: %v", err)
	}
	if left, err := captures.ListSessionPeriods(sessionID); err != nil || len(left) != 0 {
		t.Fatalf("periods after a re-derive = %v (%v), want none", left, err)
	}

	got, err := cases.GetClip("case-1")
	if err != nil {
		t.Fatalf("GetClip: %v", err)
	}
	if got.PCAPStartSecs == nil || got.PCAPDurationSecs == nil || *got.PCAPStartSecs != start || *got.PCAPDurationSecs != duration {
		t.Errorf("window = %v + %v, want %v + %v", got.PCAPStartSecs, got.PCAPDurationSecs, start, duration)
	}
	gotFiles, err := cases.ClipFiles("case-1")
	if err != nil || len(gotFiles) != 2 || gotFiles[0].PCAPFile != captureName(0) || gotFiles[1].PCAPFile != captureName(1) {
		t.Errorf("files = %+v (%v), want %+v", gotFiles, err, files)
	}
	var session, period string
	if err := db.QueryRow(`SELECT session_id, source_period_id FROM lidar_clips WHERE clip_id = 'case-1'`).Scan(&session, &period); err != nil {
		t.Fatal(err)
	}
	if session != sessionID || period != cutFrom.PeriodID {
		t.Errorf("note = %s / %s, want %s / %s", session, period, sessionID, cutFrom.PeriodID)
	}

	rows, err := db.Query(`SELECT "from" FROM pragma_foreign_key_list('lidar_clips')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var from string
		if err := rows.Scan(&from); err != nil {
			t.Fatal(err)
		}
		if from == "session_id" || from == "source_period_id" {
			t.Errorf("%s is a foreign key; the data model review keeps it a note", from)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

var caseBase = time.Date(2026, 9, 2, 13, 45, 38, 0, time.UTC)

func extentFor(name string, offset, dur time.Duration) ClipSequenceExtent {
	return ClipSequenceExtent{
		PCAPFile:    name,
		FirstPacket: caseBase.Add(offset),
		LastPacket:  caseBase.Add(offset + dur),
		PacketCount: 540000,
	}
}

func TestValidateClipSequenceAcceptsAContinuousRun(t *testing.T) {
	// The real shape: the last static stretch of broadway_columbus spans
	// files 7 and 8, which abut within a few hundred milliseconds.
	seq, err := ValidateClipSequence([]ClipSequenceExtent{
		extentFor("file7.pcap", 0, 5*time.Minute),
		extentFor("file8.pcap", 5*time.Minute+148*time.Millisecond, 5*time.Minute),
	})
	if err != nil {
		t.Fatalf("ValidateClipSequence: %v", err)
	}
	if len(seq.Segments) != 2 {
		t.Errorf("sequence holds %d segments, want 2", len(seq.Segments))
	}
	if !seq.Continuous() {
		t.Error("an abutting pair reports as not continuous")
	}
}

func TestValidateClipSequenceRejectsABrokenRun(t *testing.T) {
	_, err := ValidateClipSequence([]ClipSequenceExtent{
		extentFor("file7.pcap", 0, 5*time.Minute),
		extentFor("file9.pcap", 15*time.Minute, 5*time.Minute),
	})
	if err == nil {
		t.Fatal("a case skipping a capture was accepted")
	}
	for _, want := range []string{"file7.pcap", "file9.pcap", "continuous"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestValidateClipSequenceRejectsEmptyAndMalformed(t *testing.T) {
	if _, err := ValidateClipSequence(nil); err == nil {
		t.Error("an empty case was accepted")
	}
	if _, err := ValidateClipSequence([]ClipSequenceExtent{{PCAPFile: "a.pcap"}}); err == nil {
		t.Error("a capture with no extent was accepted")
	}
}

func TestValidateClipSequenceAcceptsASingleCapture(t *testing.T) {
	seq, err := ValidateClipSequence([]ClipSequenceExtent{extentFor("a.pcap", 0, 5*time.Minute)})
	if err != nil {
		t.Fatalf("ValidateClipSequence: %v", err)
	}
	if len(seq.Seams) != 0 {
		t.Errorf("a single capture produced %d joins, want none", len(seq.Seams))
	}
}

func TestClipCaptureAssociationRequiresItsClip(t *testing.T) {
	db := setupClipFilesDB(t)
	store := NewClipStore(db)
	if _, err := store.ClipFiles("missing"); err == nil {
		t.Fatal("missing clip accepted")
	}
	if _, err := store.ClipPaths("missing"); err == nil {
		t.Fatal("missing clip paths accepted")
	}
	if err := store.SetClipFiles("missing", []ClipCapture{{PCAPFile: "orphan.pcap"}}); err != ErrNotFound {
		t.Fatalf("missing parent: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_clip_captures WHERE clip_id='missing'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphan capture rows: %d %v", count, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.SetClipFiles("closed", []ClipCapture{{PCAPFile: "file.pcap"}}); err == nil {
		t.Fatal("closed write accepted")
	}
	if _, err := store.ClipFiles("closed"); err == nil {
		t.Fatal("closed read accepted")
	}
	if err := store.SetClipSession("closed", "session", "period"); err == nil {
		t.Fatal("closed session write accepted")
	}
}
