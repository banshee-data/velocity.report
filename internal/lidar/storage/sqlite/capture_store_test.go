package sqlite

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capindex"
	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	_ "modernc.org/sqlite"
)

// setupCaptureDB applies the real migration rather than a hand-copied schema, so
// a query that drifts from the shipped tables fails here rather than in
// production.
func setupCaptureDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	migration := filepath.Join("..", "..", "..", "db", "migrations",
		"000042_create_lidar_capture_index.up.sql")
	schema, err := os.ReadFile(migration)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return db
}

var captureBase = time.Date(2026, 9, 2, 13, 20, 0, 0, time.UTC)

func scanFile(path string, size int64, mod time.Time, tag string) capindex.File {
	return capindex.File{RelPath: path, SizeBytes: size, ModifiedAt: mod, ContentTag: tag}
}

func TestCaptureStoreUpsertRootIsIdempotent(t *testing.T) {
	store := NewCaptureStore(setupCaptureDB(t))

	first, err := store.UpsertRoot("/Volumes/lidar/lidar/s2", "field volume", true)
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}
	if first.RootID == "" {
		t.Fatal("UpsertRoot returned an empty root id")
	}
	if first.LastScanState != ScanStateNever {
		t.Errorf("LastScanState = %q, want %q", first.LastScanState, ScanStateNever)
	}

	// Restarting the process with the same configuration must address the same
	// row, not create a second one.
	second, err := store.UpsertRoot("/Volumes/lidar/lidar/s2", "renamed", true)
	if err != nil {
		t.Fatalf("UpsertRoot (repeat): %v", err)
	}
	if second.RootID != first.RootID {
		t.Errorf("root id changed between upserts: %q then %q", first.RootID, second.RootID)
	}
	if second.Label != "renamed" {
		t.Errorf("Label = %q, want the updated one", second.Label)
	}

	roots, err := store.ListRoots()
	if err != nil {
		t.Fatalf("ListRoots: %v", err)
	}
	if len(roots) != 1 {
		t.Errorf("ListRoots returned %d roots, want 1", len(roots))
	}
}

func TestCaptureStoreMarkScanned(t *testing.T) {
	store := NewCaptureStore(setupCaptureDB(t))
	root, err := store.UpsertRoot("/Volumes/lidar", "", true)
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}

	if err := store.MarkScanned(root.RootID, ScanStateUnreachable, "volume not mounted"); err != nil {
		t.Fatalf("MarkScanned: %v", err)
	}
	got, err := store.GetRoot(root.RootID)
	if err != nil {
		t.Fatalf("GetRoot: %v", err)
	}
	if got.LastScanState != ScanStateUnreachable {
		t.Errorf("LastScanState = %q, want %q", got.LastScanState, ScanStateUnreachable)
	}
	if got.LastScanError != "volume not mounted" {
		t.Errorf("LastScanError = %q, want the recorded reason", got.LastScanError)
	}
	if got.LastScanAtNs == nil {
		t.Error("LastScanAtNs is nil after a scan")
	}
}

func TestCaptureStoreApplyScanRoundTrips(t *testing.T) {
	store := NewCaptureStore(setupCaptureDB(t))
	root, err := store.UpsertRoot("/Volumes/lidar/lidar/s2", "", true)
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}

	found := []capindex.File{
		scanFile("s2_sf_3_00001.pcap", 724<<20, captureBase, "tag-1"),
		scanFile("s2_sf_3_00002.pcap", 724<<20, captureBase, "tag-2"),
	}
	drift := capindex.DiffScan(nil, found)
	if len(drift.Added) != 2 {
		t.Fatalf("first scan saw %d added, want 2", len(drift.Added))
	}
	if err := store.ApplyScan(root.RootID, found, drift); err != nil {
		t.Fatalf("ApplyScan: %v", err)
	}

	files, err := store.ListFiles(root.RootID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("ListFiles returned %d, want 2", len(files))
	}
	for _, f := range files {
		if f.ProbeState != ProbeStatePending {
			t.Errorf("%s probe state = %q, want %q", f.RelPath, f.ProbeState, ProbeStatePending)
		}
		if !f.Present {
			t.Errorf("%s is not marked present", f.RelPath)
		}
		if f.FirstPacketNs != nil {
			t.Errorf("%s has an extent before being probed", f.RelPath)
		}
	}

	// A second identical scan must report no drift and change nothing.
	indexedNow, err := store.IndexedFiles(root.RootID)
	if err != nil {
		t.Fatalf("IndexedFiles: %v", err)
	}
	if d := capindex.DiffScan(indexedNow, found); d.Any() {
		t.Errorf("re-scanning an unchanged root reported drift: %+v", d)
	}
	// Neither file has been probed yet, so a no-drift rescan must still leave
	// them selectable for probing — the bug where "Scan and probe" silently
	// probed nothing for an already-indexed, never-probed volume.
	for _, f := range indexedNow {
		if !f.NeedsProbe {
			t.Errorf("%s reports NeedsProbe=false before any probe ran", f.RelPath)
		}
	}
}

func TestCaptureStoreApplyScanResetsProbeOnlyWhenBytesMoved(t *testing.T) {
	store := NewCaptureStore(setupCaptureDB(t))
	root, _ := store.UpsertRoot("/Volumes/lidar/lidar/s2", "", true)

	found := []capindex.File{
		scanFile("touched.pcap", 100, captureBase, "tag-a"),
		scanFile("resized.pcap", 100, captureBase, "tag-b"),
	}
	if err := store.ApplyScan(root.RootID, found, capindex.DiffScan(nil, found)); err != nil {
		t.Fatalf("ApplyScan: %v", err)
	}
	for _, f := range found {
		if err := store.RecordProbe(root.RootID, f.RelPath,
			captureBase.UnixNano(), captureBase.Add(5*time.Minute).UnixNano(), 500000, 2368); err != nil {
			t.Fatalf("RecordProbe %s: %v", f.RelPath, err)
		}
	}

	// One file is merely touched; the other is truncated.
	rescan := []capindex.File{
		scanFile("touched.pcap", 100, captureBase.Add(time.Hour), "tag-a"),
		scanFile("resized.pcap", 40, captureBase, "tag-b"),
	}
	indexedNow, err := store.IndexedFiles(root.RootID)
	if err != nil {
		t.Fatalf("IndexedFiles: %v", err)
	}
	drift := capindex.DiffScan(indexedNow, rescan)
	if err := store.ApplyScan(root.RootID, rescan, drift); err != nil {
		t.Fatalf("ApplyScan (rescan): %v", err)
	}

	files, err := store.ListFiles(root.RootID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	byPath := map[string]CaptureFile{}
	for _, f := range files {
		byPath[f.RelPath] = f
	}
	if got := byPath["touched.pcap"]; got.ProbeState != ProbeStateOK || got.FirstPacketNs == nil {
		t.Errorf("a touched file lost its extent: state=%q first=%v", got.ProbeState, got.FirstPacketNs)
	}
	if got := byPath["resized.pcap"]; got.ProbeState != ProbeStatePending || got.FirstPacketNs != nil {
		t.Errorf("a resized file kept a stale extent: state=%q first=%v", got.ProbeState, got.FirstPacketNs)
	}

	afterRescan, err := store.IndexedFiles(root.RootID)
	if err != nil {
		t.Fatalf("IndexedFiles (after rescan): %v", err)
	}
	needsProbe := map[string]bool{}
	for _, f := range afterRescan {
		needsProbe[f.RelPath] = f.NeedsProbe
	}
	if needsProbe["touched.pcap"] {
		t.Error("touched.pcap kept its extent and should not need reprobing")
	}
	if !needsProbe["resized.pcap"] {
		t.Error("resized.pcap lost its extent and should need reprobing")
	}
}

func TestCaptureStoreMarksMissingFilesAbsent(t *testing.T) {
	store := NewCaptureStore(setupCaptureDB(t))
	root, _ := store.UpsertRoot("/Volumes/lidar", "", true)

	found := []capindex.File{
		scanFile("a.pcap", 100, captureBase, "tag-a"),
		scanFile("b.pcap", 100, captureBase, "tag-b"),
	}
	if err := store.ApplyScan(root.RootID, found, capindex.DiffScan(nil, found)); err != nil {
		t.Fatalf("ApplyScan: %v", err)
	}

	// The volume comes back with one file gone.
	remaining := found[:1]
	indexedNow, _ := store.IndexedFiles(root.RootID)
	drift := capindex.DiffScan(indexedNow, remaining)
	if len(drift.Missing) != 1 || drift.Missing[0].RelPath != "b.pcap" {
		t.Fatalf("Missing = %+v, want b.pcap", drift.Missing)
	}
	if err := store.ApplyScan(root.RootID, remaining, drift); err != nil {
		t.Fatalf("ApplyScan (rescan): %v", err)
	}

	files, _ := store.ListFiles(root.RootID)
	var absent int
	for _, f := range files {
		if !f.Present {
			absent++
			if f.RelPath != "b.pcap" {
				t.Errorf("%s marked absent, want b.pcap", f.RelPath)
			}
		}
	}
	if absent != 1 {
		t.Errorf("%d files absent, want 1", absent)
	}
	// The row is kept, not deleted: a case referencing it must still be able to
	// say what it lost.
	if len(files) != 2 {
		t.Errorf("index holds %d rows, want 2 with one absent", len(files))
	}
}

func TestCaptureStoreRecordProbeFailure(t *testing.T) {
	store := NewCaptureStore(setupCaptureDB(t))
	root, _ := store.UpsertRoot("/Volumes/lidar", "", true)
	found := []capindex.File{scanFile("a.pcap", 100, captureBase, "tag-a")}
	if err := store.ApplyScan(root.RootID, found, capindex.DiffScan(nil, found)); err != nil {
		t.Fatalf("ApplyScan: %v", err)
	}

	if err := store.RecordProbeFailure(root.RootID, "a.pcap", "truncated capture"); err != nil {
		t.Fatalf("RecordProbeFailure: %v", err)
	}
	files, _ := store.ListFiles(root.RootID)
	if files[0].ProbeState != ProbeStateFailed {
		t.Errorf("probe state = %q, want %q", files[0].ProbeState, ProbeStateFailed)
	}
	if files[0].ProbeError != "truncated capture" {
		t.Errorf("probe error = %q, want the recorded reason", files[0].ProbeError)
	}
	// A failed probe must not make the file eligible for sessioning.
	probedFiles, err := store.ProbedFiles(root.RootID)
	if err != nil {
		t.Fatalf("ProbedFiles: %v", err)
	}
	if len(probedFiles) != 0 {
		t.Errorf("ProbedFiles returned %d, want 0 after a failed probe", len(probedFiles))
	}
}

// seedProbedRun indexes and probes n rolling files with a clean roll-over,
// returning the root id.
func seedProbedRun(t *testing.T, store *CaptureStore, rootPath string, n int, gap time.Duration) string {
	t.Helper()
	root, err := store.UpsertRoot(rootPath, "", true)
	if err != nil {
		t.Fatalf("UpsertRoot: %v", err)
	}
	var found []capindex.File
	for i := range n {
		found = append(found, scanFile(captureName(i), 724<<20, captureBase, "tag-"+captureName(i)))
	}
	if err := store.ApplyScan(root.RootID, found, capindex.DiffScan(nil, found)); err != nil {
		t.Fatalf("ApplyScan: %v", err)
	}
	for i := range n {
		start := captureBase.Add(time.Duration(i) * (5*time.Minute + gap))
		if err := store.RecordProbe(root.RootID, captureName(i),
			start.UnixNano(), start.Add(5*time.Minute).UnixNano(), 540000, 2368); err != nil {
			t.Fatalf("RecordProbe: %v", err)
		}
	}
	return root.RootID
}

func captureName(i int) string {
	return "s2_sf_3_0000" + string(rune('1'+i)) + ".pcap"
}

func TestCaptureStoreDerivesSessionsAndLinksFiles(t *testing.T) {
	store := NewCaptureStore(setupCaptureDB(t))
	rootID := seedProbedRun(t, store, "/Volumes/lidar/lidar/s2", 9, 300*time.Millisecond)

	sessions, err := store.DeriveSessions(rootID)
	if err != nil {
		t.Fatalf("DeriveSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("derived %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	if s.FileCount != 9 {
		t.Errorf("FileCount = %d, want 9", s.FileCount)
	}
	if s.WorstSeam != string(capseq.SeamAcceptable) {
		t.Errorf("WorstSeam = %q, want %q", s.WorstSeam, capseq.SeamAcceptable)
	}
	if want := int64(8 * 300 * time.Millisecond); s.LostNs != want {
		t.Errorf("LostNs = %d, want %d", s.LostNs, want)
	}

	files, err := store.SessionFiles(s.SessionID)
	if err != nil {
		t.Fatalf("SessionFiles: %v", err)
	}
	if len(files) != 9 {
		t.Fatalf("SessionFiles returned %d, want 9", len(files))
	}
	for i := 1; i < len(files); i++ {
		if *files[i-1].FirstPacketNs > *files[i].FirstPacketNs {
			t.Errorf("session files are not in packet-time order at %d", i)
		}
	}
}

func TestCaptureStoreSessionLabelSurvivesRederivation(t *testing.T) {
	// The site name is the one thing about a session an operator supplies, and
	// re-deriving must not throw it away.
	store := NewCaptureStore(setupCaptureDB(t))
	rootID := seedProbedRun(t, store, "/Volumes/lidar/lidar/s2", 5, 300*time.Millisecond)

	sessions, err := store.DeriveSessions(rootID)
	if err != nil {
		t.Fatalf("DeriveSessions: %v", err)
	}
	if err := store.SetSessionLabel(sessions[0].SessionID, "broadway_columbus", "hesai-pandar40p"); err != nil {
		t.Fatalf("SetSessionLabel: %v", err)
	}

	again, err := store.DeriveSessions(rootID)
	if err != nil {
		t.Fatalf("DeriveSessions (repeat): %v", err)
	}
	if len(again) != 1 {
		t.Fatalf("derived %d sessions, want 1", len(again))
	}
	if again[0].SessionID != sessions[0].SessionID {
		t.Errorf("session identity changed on re-derivation: %q then %q",
			sessions[0].SessionID, again[0].SessionID)
	}
	if again[0].Label != "broadway_columbus" {
		t.Errorf("Label = %q, want it to survive re-derivation", again[0].Label)
	}
}

func TestCaptureStoreSetSessionLabelOnUnknownSession(t *testing.T) {
	store := NewCaptureStore(setupCaptureDB(t))
	if err := store.SetSessionLabel("ses-nope", "x", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetSessionLabel on an unknown session: %v, want ErrNotFound", err)
	}
}

func TestCaptureStoreSplitsSessionsOnALongBreak(t *testing.T) {
	store := NewCaptureStore(setupCaptureDB(t))
	root, _ := store.UpsertRoot("/Volumes/lidar/lidar/s2", "", true)

	found := []capindex.File{
		scanFile("a.pcap", 100, captureBase, "tag-a"),
		scanFile("b.pcap", 100, captureBase, "tag-b"),
		scanFile("c.pcap", 100, captureBase, "tag-c"),
	}
	if err := store.ApplyScan(root.RootID, found, capindex.DiffScan(nil, found)); err != nil {
		t.Fatalf("ApplyScan: %v", err)
	}
	// a and b abut; c is two hours later.
	offsets := []time.Duration{0, 5 * time.Minute, 3 * time.Hour}
	for i, f := range found {
		start := captureBase.Add(offsets[i])
		if err := store.RecordProbe(root.RootID, f.RelPath,
			start.UnixNano(), start.Add(5*time.Minute).UnixNano(), 1000, 2368); err != nil {
			t.Fatalf("RecordProbe: %v", err)
		}
	}

	sessions, err := store.DeriveSessions(root.RootID)
	if err != nil {
		t.Fatalf("DeriveSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("derived %d sessions, want 2 across a two-hour break", len(sessions))
	}
	// Newest first.
	if sessions[0].StartNs < sessions[1].StartNs {
		t.Error("ListSessions is not newest-first")
	}
	if sessions[0].FileCount != 1 || sessions[1].FileCount != 2 {
		t.Errorf("sessions hold %d and %d files, want 1 and 2",
			sessions[0].FileCount, sessions[1].FileCount)
	}
}

func TestCaptureStoreListsAcrossRoots(t *testing.T) {
	store := NewCaptureStore(setupCaptureDB(t))
	a := seedProbedRun(t, store, "/Volumes/lidar/lidar/s2", 2, 0)
	seedProbedRun(t, store, "/Volumes/other/captures", 3, 0)

	if _, err := store.DeriveSessions(a); err != nil {
		t.Fatalf("DeriveSessions: %v", err)
	}

	all, err := store.ListFiles("")
	if err != nil {
		t.Fatalf("ListFiles(all): %v", err)
	}
	if len(all) != 5 {
		t.Errorf("ListFiles(all) returned %d, want 5", len(all))
	}
	scoped, err := store.ListFiles(a)
	if err != nil {
		t.Fatalf("ListFiles(root): %v", err)
	}
	if len(scoped) != 2 {
		t.Errorf("ListFiles(root) returned %d, want 2", len(scoped))
	}
}
