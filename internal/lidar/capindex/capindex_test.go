package capindex

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
)

var scanBase = time.Date(2026, 9, 2, 13, 20, 0, 0, time.UTC)

// writeCapture creates a file of the given size under dir, filled so its ends
// differ between calls unless seed matches.
func writeCapture(t *testing.T, dir, name string, size int, seed byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data := make([]byte, size)
	for i := range data {
		data[i] = seed + byte(i%7)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
	return path
}

func TestScanFindsCapturesAndIgnoresTheRest(t *testing.T) {
	dir := t.TempDir()
	writeCapture(t, dir, "s2_sf_3_00001.pcap", 512, 1)
	writeCapture(t, dir, "s2_sf_3_00002.pcapng", 512, 2)
	writeCapture(t, dir, "notes.txt", 32, 3)
	writeCapture(t, dir, "sub/s2_sf_3_00003.pcap", 512, 4)
	// Output directories beside the captures are not capture data.
	writeCapture(t, dir, "analysis/s2_sf_3_00001/motion.pcap", 512, 5)
	writeCapture(t, dir, "pcap_split_analysis_20260903T150630Z/x.pcap", 512, 6)
	writeCapture(t, dir, ".hidden/x.pcap", 512, 7)

	files, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got := make([]string, len(files))
	for i, f := range files {
		got[i] = f.RelPath
	}
	want := []string{"s2_sf_3_00001.pcap", "s2_sf_3_00002.pcapng", "sub/s2_sf_3_00003.pcap"}
	if len(got) != len(want) {
		t.Fatalf("Scan found %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("file %d = %q, want %q", i, got[i], want[i])
		}
	}
	for _, f := range files {
		if f.SizeBytes != 512 {
			t.Errorf("%s size = %d, want 512", f.RelPath, f.SizeBytes)
		}
		if f.ContentTag == "" {
			t.Errorf("%s has no content tag", f.RelPath)
		}
	}
}

func TestScanIsStableAndUsesSlashPaths(t *testing.T) {
	dir := t.TempDir()
	writeCapture(t, dir, "b/second.pcap", 256, 1)
	writeCapture(t, dir, "a/first.pcap", 256, 2)

	first, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	second, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan (repeat): %v", err)
	}
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("scans returned %d and %d files, want 2 each", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("scan %d differs between runs: %+v vs %+v", i, first[i], second[i])
		}
	}
	if first[0].RelPath != "a/first.pcap" {
		t.Errorf("first path = %q, want a/first.pcap", first[0].RelPath)
	}
}

func TestScanReportsAnUnreachableRoot(t *testing.T) {
	_, err := Scan(filepath.Join(t.TempDir(), "not-mounted"))
	if !errors.Is(err, ErrRootUnreachable) {
		t.Fatalf("Scan of a missing root: %v, want ErrRootUnreachable", err)
	}

	// A file where a directory is expected is equally unusable.
	file := writeCapture(t, t.TempDir(), "a.pcap", 16, 1)
	if _, err := Scan(file); !errors.Is(err, ErrRootUnreachable) {
		t.Fatalf("Scan of a file: %v, want ErrRootUnreachable", err)
	}
}

func TestContentTagDistinguishesRewrites(t *testing.T) {
	dir := t.TempDir()
	a := writeCapture(t, dir, "a.pcap", 4096, 1)
	b := writeCapture(t, dir, "b.pcap", 4096, 9)

	tagA, err := ContentTag(a, 4096)
	if err != nil {
		t.Fatalf("ContentTag: %v", err)
	}
	tagB, err := ContentTag(b, 4096)
	if err != nil {
		t.Fatalf("ContentTag: %v", err)
	}
	if tagA == tagB {
		t.Error("files with different content share a tag")
	}

	// Same bytes, same tag: a scan of an unchanged volume must not report drift.
	again, err := ContentTag(a, 4096)
	if err != nil {
		t.Fatalf("ContentTag (repeat): %v", err)
	}
	if again != tagA {
		t.Error("content tag is not stable across calls")
	}

	// The length is part of the tag, so a truncation shows even when the
	// surviving bytes are identical.
	truncated := writeCapture(t, dir, "c.pcap", 2048, 1)
	tagC, err := ContentTag(truncated, 2048)
	if err != nil {
		t.Fatalf("ContentTag: %v", err)
	}
	if tagC == tagA {
		t.Error("truncating a file did not change its tag")
	}
}

func indexed(path string, size int64, mod time.Time, tag string) Indexed {
	return Indexed{RelPath: path, SizeBytes: size, ModifiedAt: mod, ContentTag: tag, Present: true}
}

func scanned(path string, size int64, mod time.Time, tag string) File {
	return File{RelPath: path, SizeBytes: size, ModifiedAt: mod, ContentTag: tag}
}

func TestDiffScanClassifiesEveryOutcome(t *testing.T) {
	mod := scanBase
	known := []Indexed{
		indexed("same.pcap", 100, mod, "tag-same"),
		indexed("resized.pcap", 100, mod, "tag-resized"),
		indexed("rewritten.pcap", 100, mod, "tag-old"),
		indexed("touched.pcap", 100, mod, "tag-touched"),
		indexed("gone.pcap", 100, mod, "tag-gone"),
	}
	found := []File{
		scanned("same.pcap", 100, mod, "tag-same"),
		scanned("resized.pcap", 250, mod, "tag-resized"),
		scanned("rewritten.pcap", 100, mod, "tag-new"),
		scanned("touched.pcap", 100, mod.Add(time.Hour), "tag-touched"),
		scanned("new.pcap", 100, mod, "tag-new-file"),
	}

	drift := DiffScan(known, found)

	if len(drift.Added) != 1 || drift.Added[0].RelPath != "new.pcap" {
		t.Errorf("Added = %+v, want just new.pcap", drift.Added)
	}
	if len(drift.Missing) != 1 || drift.Missing[0].RelPath != "gone.pcap" {
		t.Errorf("Missing = %+v, want just gone.pcap", drift.Missing)
	}
	if drift.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", drift.Unchanged)
	}
	wantKinds := map[string]ChangeKind{
		"resized.pcap":   ChangeResized,
		"rewritten.pcap": ChangeRewritten,
		"touched.pcap":   ChangeTouched,
	}
	if len(drift.Changed) != len(wantKinds) {
		t.Fatalf("Changed = %+v, want %d entries", drift.Changed, len(wantKinds))
	}
	for _, c := range drift.Changed {
		if want, ok := wantKinds[c.RelPath]; !ok || c.Kind != want {
			t.Errorf("%s classified %q, want %q", c.RelPath, c.Kind, want)
		}
	}
	if !drift.Any() {
		t.Error("Any() is false with added, missing and changed files")
	}
	if drift.Total() != 5 {
		t.Errorf("Total() = %d, want 5 files seen", drift.Total())
	}
}

func TestDiffScanReportsNoDriftForAnUnchangedVolume(t *testing.T) {
	mod := scanBase
	known := []Indexed{indexed("a.pcap", 100, mod, "tag-a"), indexed("b.pcap", 200, mod, "tag-b")}
	found := []File{scanned("a.pcap", 100, mod, "tag-a"), scanned("b.pcap", 200, mod, "tag-b")}

	drift := DiffScan(known, found)
	if drift.Any() {
		t.Errorf("unchanged volume reported drift: %+v", drift)
	}
	if drift.Unchanged != 2 {
		t.Errorf("Unchanged = %d, want 2", drift.Unchanged)
	}
}

func TestDiffScanTreatsAReturningFileAsAdded(t *testing.T) {
	// Anything probed about a file that left the volume cannot be trusted when
	// something with the same name comes back.
	mod := scanBase
	known := []Indexed{{RelPath: "a.pcap", SizeBytes: 100, ModifiedAt: mod, ContentTag: "tag-a", Present: false}}
	found := []File{scanned("a.pcap", 100, mod, "tag-a")}

	drift := DiffScan(known, found)
	if len(drift.Added) != 1 {
		t.Errorf("Added = %+v, want the returning file", drift.Added)
	}
	if len(drift.Missing) != 0 {
		t.Errorf("Missing = %+v, want none; the file was already absent", drift.Missing)
	}
}

func TestDiffScanIgnoresAMissingContentTag(t *testing.T) {
	// An unreadable file gets no tag. That must not by itself read as a rewrite.
	mod := scanBase
	known := []Indexed{indexed("a.pcap", 100, mod, "")}
	found := []File{scanned("a.pcap", 100, mod, "tag-a")}

	drift := DiffScan(known, found)
	if drift.Unchanged != 1 {
		t.Errorf("a newly obtained tag was reported as a change: %+v", drift.Changed)
	}
}

func TestChangeNeedsProbe(t *testing.T) {
	for _, tc := range []struct {
		kind ChangeKind
		want bool
	}{
		{ChangeResized, true},
		{ChangeRewritten, true},
		{ChangeTouched, false},
	} {
		if got := (Change{Kind: tc.kind}).NeedsProbe(); got != tc.want {
			t.Errorf("%q.NeedsProbe() = %v, want %v", tc.kind, got, tc.want)
		}
	}
}

// probed builds a file running for dur from scanBase+offset.
func probed(name string, offset, dur time.Duration) Probed {
	return Probed{
		RelPath:     name,
		FirstPacket: scanBase.Add(offset),
		LastPacket:  scanBase.Add(offset + dur),
		PacketCount: 500000,
		SizeBytes:   724 << 20,
	}
}

const rollFile = 5 * time.Minute

func TestSessionsGroupsAContiguousRun(t *testing.T) {
	// Nine rolling files with a clean roll-over, as the field volume holds.
	var files []Probed
	for i := range 9 {
		files = append(files, probed(fileNameFor(i), time.Duration(i)*(rollFile+300*time.Millisecond), rollFile))
	}

	sessions := Sessions(files, capseq.DefaultTolerances())
	if len(sessions) != 1 {
		t.Fatalf("derived %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	if len(s.Files) != 9 {
		t.Errorf("session holds %d files, want 9", len(s.Files))
	}
	if s.Worst != capseq.SeamAcceptable {
		t.Errorf("Worst = %q, want %q for 300ms roll-overs", s.Worst, capseq.SeamAcceptable)
	}
	if want := 8 * 300 * time.Millisecond; s.Lost != want {
		t.Errorf("Lost = %v, want %v", s.Lost, want)
	}
	if want := 9 * rollFile; s.Covered != want {
		t.Errorf("Covered = %v, want %v", s.Covered, want)
	}
	if want := int64(9 * (724 << 20)); s.SizeBytes != want {
		t.Errorf("SizeBytes = %d, want %d", s.SizeBytes, want)
	}
}

func TestSessionsSplitsOnAnUncrossableGap(t *testing.T) {
	// Two visits to a site on the same day: three files, a long break, four more.
	var files []Probed
	for i := range 3 {
		files = append(files, probed(fileNameFor(i), time.Duration(i)*rollFile, rollFile))
	}
	breakStart := 3*rollFile + 2*time.Hour
	for i := range 4 {
		files = append(files, probed(fileNameFor(10+i), breakStart+time.Duration(i)*rollFile, rollFile))
	}

	sessions := Sessions(files, capseq.DefaultTolerances())
	if len(sessions) != 2 {
		t.Fatalf("derived %d sessions, want 2", len(sessions))
	}
	if len(sessions[0].Files) != 3 || len(sessions[1].Files) != 4 {
		t.Errorf("sessions hold %d and %d files, want 3 and 4",
			len(sessions[0].Files), len(sessions[1].Files))
	}
	if !sessions[0].End.Before(sessions[1].Start) {
		t.Error("sessions are not in chronological order")
	}
	// Neither session may contain the break.
	for i, s := range sessions {
		if s.Lost > time.Second {
			t.Errorf("session %d lost %v internally; the break should have split it", i, s.Lost)
		}
	}
}

func TestSessionsSplitsOnOverlap(t *testing.T) {
	files := []Probed{
		probed("a.pcap", 0, rollFile),
		probed("b.pcap", rollFile-time.Minute, rollFile),
	}
	sessions := Sessions(files, capseq.DefaultTolerances())
	if len(sessions) != 2 {
		t.Fatalf("derived %d sessions, want 2 for overlapping files", len(sessions))
	}
}

func TestSessionsOrdersUnsortedInput(t *testing.T) {
	files := []Probed{
		probed("c.pcap", 2*rollFile, rollFile),
		probed("a.pcap", 0, rollFile),
		probed("b.pcap", rollFile, rollFile),
	}
	sessions := Sessions(files, capseq.DefaultTolerances())
	if len(sessions) != 1 {
		t.Fatalf("derived %d sessions, want 1", len(sessions))
	}
	for i, want := range []string{"a.pcap", "b.pcap", "c.pcap"} {
		if got := sessions[0].Files[i].RelPath; got != want {
			t.Errorf("file %d = %q, want %q", i, got, want)
		}
	}
	if got := sessions[0].Duration(); got != 3*rollFile {
		t.Errorf("Duration() = %v, want %v", got, 3*rollFile)
	}
}

func TestSessionsSkipsUnprobedFiles(t *testing.T) {
	// A file with no extent cannot be placed on the clock, so it is not
	// sessioned rather than being guessed at.
	files := []Probed{
		probed("a.pcap", 0, rollFile),
		{RelPath: "unprobed.pcap"},
		probed("b.pcap", rollFile, rollFile),
		{RelPath: "inverted.pcap", FirstPacket: scanBase.Add(time.Hour), LastPacket: scanBase},
	}
	sessions := Sessions(files, capseq.DefaultTolerances())
	if len(sessions) != 1 {
		t.Fatalf("derived %d sessions, want 1", len(sessions))
	}
	if len(sessions[0].Files) != 2 {
		t.Errorf("session holds %d files, want the 2 probed ones", len(sessions[0].Files))
	}
}

func TestSessionsOnNoUsableFiles(t *testing.T) {
	if got := Sessions(nil, capseq.DefaultTolerances()); got != nil {
		t.Errorf("Sessions(nil) = %+v, want nil", got)
	}
	if got := Sessions([]Probed{{RelPath: "x.pcap"}}, capseq.DefaultTolerances()); got != nil {
		t.Errorf("Sessions of one unprobed file = %+v, want nil", got)
	}
}

func fileNameFor(i int) string {
	return "s2_sf_3_" + string(rune('a'+i)) + ".pcap"
}
