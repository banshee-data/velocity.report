package annotation

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

var pruneEpoch = time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

// pruneFixture is a pack directory holding only what a prune reads: a manifest,
// a head, and archived revisions one every step, oldest first.
type pruneFixture struct {
	dir   string
	steps []time.Time
}

func newPruneFixture(t *testing.T, revisions int, step time.Duration) *pruneFixture {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, revisionDir), 0o700); err != nil {
		t.Fatal(err)
	}
	writePruneFile(t, filepath.Join(dir, manifestFile), `{"pack_digest":"sha256:pack-a"}`, pruneEpoch)
	f := &pruneFixture{dir: dir}
	for i := 1; i <= revisions; i++ {
		at := pruneEpoch.Add(time.Duration(i-1) * step)
		f.steps = append(f.steps, at)
		writePruneFile(t, filepath.Join(dir, revisionName(i)), fmt.Sprintf(`{"revision":%d,"pad":%q}`, i, strings.Repeat("x", i)), at)
	}
	writePruneFile(t, filepath.Join(dir, sidecarFile), fmt.Sprintf(`{"revision":%d}`, revisions+1), pruneEpoch.Add(time.Duration(revisions)*step))
	return f
}

func writePruneFile(t *testing.T, path, body string, at time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func (f *pruneFixture) has(t *testing.T, rev int) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(f.dir, revisionName(rev)))
	return err == nil
}

func (f *pruneFixture) backup(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "backup.tar.gz")
}

func keptRevisions(p *PrunePlan) []int {
	var out []int
	for _, e := range p.Entries {
		if e.Keep {
			out = append(out, e.Revision)
		}
	}
	return out
}

func TestPlanPruneKeepsNewestOldestAndThinsTheRest(t *testing.T) {
	// Twelve revisions twenty minutes apart. Keeping the newest two and one per
	// hour of the rest: the oldest, then the first of each later hour.
	f := newPruneFixture(t, 12, 20*time.Minute)
	plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 2, KeepEvery: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	got := keptRevisions(plan)
	want := []int{1, 4, 7, 10, 11, 12} // 1 oldest; 4, 7, 10 open hours 1, 2, 3; 11, 12 newest
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("kept %v, want %v", got, want)
	}
	if plan.Kept != 6 || plan.Pruned != 6 {
		t.Fatalf("kept %d pruned %d", plan.Kept, plan.Pruned)
	}
	if plan.KeptByReason[PruneKeepNewest] != 2 || plan.KeptByReason[PruneKeepOldest] != 1 || plan.KeptByReason[PruneKeepThinned] != 3 {
		t.Fatalf("reasons %v", plan.KeptByReason)
	}
	var keptBytes, prunedBytes int64
	for _, e := range plan.Entries {
		if e.Keep {
			keptBytes += e.Bytes
		} else {
			prunedBytes += e.Bytes
			if e.Reason != "pruned" {
				t.Fatalf("revision %d reason %q", e.Revision, e.Reason)
			}
		}
	}
	if plan.KeptBytes != keptBytes || plan.PrunedBytes != prunedBytes || prunedBytes == 0 {
		t.Fatalf("bytes kept %d/%d pruned %d/%d", plan.KeptBytes, keptBytes, plan.PrunedBytes, prunedBytes)
	}
}

func TestPlanPruneWithoutThinningKeepsOnlyNewestOldestAndProtected(t *testing.T) {
	f := newPruneFixture(t, 9, 10*time.Minute)
	plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 3, Protect: map[int]string{5: "pinned by split.json", 77: "pinned by the head"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fmt.Sprint(keptRevisions(plan)), fmt.Sprint([]int{1, 5, 7, 8, 9}); got != want {
		t.Fatalf("kept %s, want %s", got, want)
	}
	for _, e := range plan.Entries {
		if e.Revision == 5 && e.Reason != "protected: pinned by split.json" {
			t.Fatalf("revision 5 reason %q", e.Reason)
		}
	}
	if plan.KeptByReason[PruneKeepProtected] != 1 {
		t.Fatalf("reasons %v", plan.KeptByReason)
	}
}

func TestPlanPruneRefusesWhatItCannotRead(t *testing.T) {
	f := newPruneFixture(t, 3, time.Minute)
	for name, policy := range map[string]PrunePolicy{
		"keep none":     {KeepLast: 0},
		"negative keep": {KeepLast: 1, KeepEvery: -time.Hour},
	} {
		if _, err := PlanPrune(f.dir, policy); err == nil {
			t.Fatalf("%s: planned", name)
		}
	}
	if _, err := PlanPrune(filepath.Join(f.dir, "missing"), PrunePolicy{KeepLast: 1}); err == nil {
		t.Fatal("a missing pack planned")
	}

	bare := t.TempDir()
	if _, err := PlanPrune(bare, PrunePolicy{KeepLast: 1}); err == nil || !strings.Contains(err.Error(), "annotation history") {
		t.Fatalf("a pack with no history: %v", err)
	}

	linked := t.TempDir()
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(linked, revisionDir)); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanPrune(linked, PrunePolicy{KeepLast: 1}); err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("a symlinked history directory: %v", err)
	}
}

func TestPlanPruneIgnoresWhatIsNotAnArchive(t *testing.T) {
	f := newPruneFixture(t, 3, time.Minute)
	hist := filepath.Join(f.dir, revisionDir)
	writePruneFile(t, filepath.Join(hist, "notes.txt"), "keep me", pruneEpoch)
	writePruneFile(t, filepath.Join(hist, ".0000000002.json.tmp"), "partial", pruneEpoch)
	writePruneFile(t, filepath.Join(hist, "0000000000.json"), "{}", pruneEpoch)
	if err := os.Mkdir(filepath.Join(hist, "0000000009.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.dir, sidecarFile), filepath.Join(hist, "0000000010.json")); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Entries) != 3 {
		t.Fatalf("entries %+v", plan.Entries)
	}
	sort.Strings(plan.Ignored)
	want := []string{".0000000002.json.tmp", "0000000000.json", "0000000009.json", "0000000010.json", "notes.txt"}
	if fmt.Sprint(plan.Ignored) != fmt.Sprint(want) {
		t.Fatalf("ignored %v, want %v", plan.Ignored, want)
	}
	// Executing the plan never touches an ignored name.
	if _, err := plan.Execute(PruneExecuteOptions{BackupPath: f.backup(t)}); err != nil {
		t.Fatal(err)
	}
	for _, n := range want {
		if _, err := os.Lstat(filepath.Join(hist, n)); err != nil {
			t.Fatalf("%s was removed: %v", n, err)
		}
	}
}

// untar reads a backup into memory.
func untar(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]string{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out[hdr.Name] = string(b)
	}
}

func TestExecuteBacksUpVerifiesAndThenRemoves(t *testing.T) {
	f := newPruneFixture(t, 8, time.Minute)
	plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 2})
	if err != nil {
		t.Fatal(err)
	}
	originals := map[int]string{}
	for r := 1; r <= 8; r++ {
		b, err := os.ReadFile(filepath.Join(f.dir, revisionName(r)))
		if err != nil {
			t.Fatal(err)
		}
		originals[r] = string(b)
	}
	head, _ := os.ReadFile(filepath.Join(f.dir, sidecarFile))
	backup := f.backup(t)
	var said []string
	report, err := plan.Execute(PruneExecuteOptions{BackupPath: backup, Progress: func(s string) { said = append(said, s) }})
	if err != nil {
		t.Fatal(err)
	}
	// Revisions 2 to 6 are pruned; 1 (oldest) and 7, 8 (newest) stay.
	for r := 1; r <= 8; r++ {
		if want := r == 1 || r >= 7; f.has(t, r) != want {
			t.Fatalf("revision %d present=%v, want %v", r, f.has(t, r), want)
		}
	}
	if report.Removed != 5 || report.BackedUp != 5 || report.RemovedBytes != plan.PrunedBytes || report.BackedBytes != plan.PrunedBytes || len(report.Skipped) != 0 {
		t.Fatalf("report %+v", report)
	}
	members := untar(t, backup)
	for r := 2; r <= 6; r++ {
		if got := members[fmt.Sprintf("annotation-revisions/%010d.json", r)]; got != originals[r] {
			t.Fatalf("revision %d in the backup reads %q, want %q", r, got, originals[r])
		}
	}
	if members[pruneHeadMember] != string(head) {
		t.Fatalf("head in the backup %q", members[pruneHeadMember])
	}
	manifest := members[pruneManifestMember]
	for r := 2; r <= 6; r++ {
		sum := sha256.Sum256([]byte(originals[r]))
		if line := hex.EncodeToString(sum[:]) + "  " + fmt.Sprintf("annotation-revisions/%010d.json", r); !strings.Contains(manifest, line) {
			t.Fatalf("manifest lacks %q", line)
		}
	}
	// The backup's own digest in the report is the file's.
	b, _ := os.ReadFile(backup)
	sum := sha256.Sum256(b)
	if report.BackupSHA256 != hex.EncodeToString(sum[:]) || report.BackupBytes != int64(len(b)) {
		t.Fatalf("backup digest %s / %d bytes, file %x / %d", report.BackupSHA256, report.BackupBytes, sum, len(b))
	}
	if len(said) == 0 || !strings.HasPrefix(said[len(said)-1], "backup verified") {
		t.Fatalf("progress %v", said)
	}
	// A partial file never survives.
	if _, err := os.Lstat(backup + ".partial"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial backup left: %v", err)
	}
}

func TestExecuteReportsProgressOnALongBackup(t *testing.T) {
	f := newPruneFixture(t, 120, time.Second)
	plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
	if err != nil {
		t.Fatal(err)
	}
	var said []string
	if _, err := plan.Execute(PruneExecuteOptions{BackupPath: f.backup(t), Progress: func(s string) { said = append(said, s) }}); err != nil {
		t.Fatal(err)
	}
	var steps int
	for _, s := range said {
		if strings.HasPrefix(s, "backed up through revision") {
			steps++
		}
	}
	if steps < 2 {
		t.Fatalf("progress lines %v", said)
	}
}

func TestExecuteWithNothingToPruneWritesNothing(t *testing.T) {
	f := newPruneFixture(t, 3, time.Minute)
	plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 5})
	if err != nil {
		t.Fatal(err)
	}
	backup := f.backup(t)
	report, err := plan.Execute(PruneExecuteOptions{BackupPath: backup})
	if err != nil || report.Removed != 0 || report.BackupPath != "" {
		t.Fatalf("report %+v, %v", report, err)
	}
	if _, err := os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a backup was written for nothing: %v", err)
	}
}

func TestExecuteRefusesAnUnsafeBackup(t *testing.T) {
	f := newPruneFixture(t, 6, time.Minute)
	plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
	if err != nil {
		t.Fatal(err)
	}
	existing := f.backup(t)
	writePruneFile(t, existing, "an earlier backup", pruneEpoch)
	cases := map[string]PruneExecuteOptions{
		"no backup path":      {},
		"not a tar.gz":        {BackupPath: filepath.Join(t.TempDir(), "backup.zip")},
		"inside the pack":     {BackupPath: filepath.Join(f.dir, "backup.tar.gz")},
		"inside the history":  {BackupPath: filepath.Join(f.dir, revisionDir, "backup.tar.gz")},
		"already exists":      {BackupPath: existing},
		"directory missing":   {BackupPath: filepath.Join(t.TempDir(), "missing", "backup.tar.gz")},
		"not enough free":     {BackupPath: f.backup(t), MinFreeRatio: 1e12},
		"partial file exists": {BackupPath: ""},
	}
	partial := f.backup(t)
	writePruneFile(t, partial+".partial", "debris", pruneEpoch)
	cases["partial file exists"] = PruneExecuteOptions{BackupPath: partial}
	for name, opts := range cases {
		if _, err := plan.Execute(opts); err == nil {
			t.Fatalf("%s: executed", name)
		}
		for r := 1; r <= 6; r++ {
			if !f.has(t, r) {
				t.Fatalf("%s: revision %d was removed", name, r)
			}
		}
	}
	if b, _ := os.ReadFile(existing); string(b) != "an earlier backup" {
		t.Fatalf("an earlier backup was overwritten: %q", b)
	}
	if b, _ := os.ReadFile(partial + ".partial"); string(b) != "debris" {
		t.Fatalf("someone else's partial file was removed: %q", b)
	}
}

func TestCheckFreeSpaceNamesAVolumeItCannotRead(t *testing.T) {
	if err := checkFreeSpace(filepath.Join(t.TempDir(), "missing"), 1); err == nil || !strings.Contains(err.Error(), "free space") {
		t.Fatalf("got %v", err)
	}
}

func TestExecuteKeepsTheBackupAndRemovesNothingWhileTheWriterHoldsTheLock(t *testing.T) {
	f := newPruneFixture(t, 6, time.Minute)
	plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	lock, err := lockAnnotations(root)
	if err != nil {
		t.Fatal(err)
	}
	backup := f.backup(t)
	_, err = plan.Execute(PruneExecuteOptions{BackupPath: backup})
	lock.Close()
	if !errors.Is(err, ErrSidecarBusy) || !strings.Contains(err.Error(), backup) {
		t.Fatalf("got %v", err)
	}
	for r := 1; r <= 6; r++ {
		if !f.has(t, r) {
			t.Fatalf("revision %d was removed under a held lock", r)
		}
	}
	if _, err := os.Lstat(backup); err != nil {
		t.Fatalf("the verified backup was not kept: %v", err)
	}
	// Run again with the lock free: the backup path is taken, so use a new one.
	report, err := plan.Execute(PruneExecuteOptions{BackupPath: f.backup(t)})
	if err != nil || report.Removed != 4 {
		t.Fatalf("second run: %+v, %v", report, err)
	}
}

func TestExecuteRefusesAnArchiveThatChangedSincePlanning(t *testing.T) {
	f := newPruneFixture(t, 6, time.Minute)
	plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
	if err != nil {
		t.Fatal(err)
	}
	writePruneFile(t, filepath.Join(f.dir, revisionName(3)), "a different length", f.steps[2])
	backup := f.backup(t)
	if _, err := plan.Execute(PruneExecuteOptions{BackupPath: backup}); err == nil || !strings.Contains(err.Error(), "changed since the plan") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a failed backup left a file: %v", err)
	}
	if _, err := os.Lstat(backup + ".partial"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a failed backup left a partial file: %v", err)
	}
	for r := 1; r <= 6; r++ {
		if !f.has(t, r) {
			t.Fatalf("revision %d was removed", r)
		}
	}
}

func TestExecuteFailsWhenAnArchiveOrTheHeadIsMissing(t *testing.T) {
	f := newPruneFixture(t, 4, time.Minute)
	plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.dir, revisionName(2))); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Execute(PruneExecuteOptions{BackupPath: f.backup(t)}); err == nil || !strings.Contains(err.Error(), "back up revision 2") {
		t.Fatalf("a missing archive: %v", err)
	}
	g := newPruneFixture(t, 4, time.Minute)
	plan, err = PlanPrune(g.dir, PrunePolicy{KeepLast: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(g.dir, sidecarFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Execute(PruneExecuteOptions{BackupPath: g.backup(t)}); err == nil || !strings.Contains(err.Error(), "current snapshot") {
		t.Fatalf("a missing head: %v", err)
	}
	h := newPruneFixture(t, 4, time.Minute)
	plan, err = PlanPrune(h.dir, PrunePolicy{KeepLast: 1})
	if err != nil {
		t.Fatal(err)
	}
	// A non-regular file in an archive's place is never copied.
	if err := os.Remove(filepath.Join(h.dir, revisionName(2))); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(h.dir, sidecarFile), filepath.Join(h.dir, revisionName(2))); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Execute(PruneExecuteOptions{BackupPath: h.backup(t)}); err == nil {
		t.Fatal("a symlink was backed up as an archive")
	}
}

func TestExecuteSkipsWhatChangesBetweenTheBackupAndTheRemoval(t *testing.T) {
	f := newPruneFixture(t, 6, time.Minute)
	plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
	if err != nil {
		t.Fatal(err)
	}
	pruneHook = func(stage, _ string) {
		if stage != "after-backup" {
			return
		}
		writePruneFile(t, filepath.Join(f.dir, revisionName(2)), "grown since it was backed up", f.steps[1])
		os.Remove(filepath.Join(f.dir, revisionName(3)))
		os.Remove(filepath.Join(f.dir, revisionName(4)))
		os.Symlink(filepath.Join(f.dir, sidecarFile), filepath.Join(f.dir, revisionName(4)))
	}
	defer func() { pruneHook = nil }()
	report, err := plan.Execute(PruneExecuteOptions{BackupPath: f.backup(t)})
	if err != nil {
		t.Fatal(err)
	}
	if report.Removed != 1 || len(report.Skipped) != 3 {
		t.Fatalf("report %+v", report)
	}
	joined := strings.Join(report.Skipped, "; ")
	for _, want := range []string{"0000000002.json: changed since the plan", "0000000003.json: already gone", "0000000004.json: changed since the plan"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("skipped %q lacks %q", joined, want)
		}
	}
	if !f.has(t, 2) || !f.has(t, 4) {
		t.Fatal("a changed archive was removed")
	}
}

func TestExecuteRemovesNothingWhenTheBackupDoesNotVerify(t *testing.T) {
	for name, damage := range map[string]func(string){
		"truncated": func(p string) {
			b, _ := os.ReadFile(p)
			os.WriteFile(p, b[:len(b)-20], 0o600)
		},
		"not gzip": func(p string) { os.WriteFile(p, []byte("not a gzip stream"), 0o600) },
		"missing":  func(p string) { os.Remove(p) },
	} {
		f := newPruneFixture(t, 6, time.Minute)
		plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
		if err != nil {
			t.Fatal(err)
		}
		pruneHook = func(stage, path string) {
			if stage == "before-verify" {
				damage(path)
			}
		}
		backup := f.backup(t)
		_, err = plan.Execute(PruneExecuteOptions{BackupPath: backup})
		pruneHook = nil
		if err == nil || !strings.Contains(err.Error(), "did not verify") {
			t.Fatalf("%s: got %v", name, err)
		}
		for r := 1; r <= 6; r++ {
			if !f.has(t, r) {
				t.Fatalf("%s: revision %d was removed", name, r)
			}
		}
	}
}

// writeTarGz writes members, then the manifest member when given.
func writeTarGz(t *testing.T, members [][2]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		if err := tw.WriteHeader(&tar.Header{Name: m[0], Mode: 0o644, Size: int64(len(m[1])), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(m[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := errors.Join(tw.Close(), gz.Close(), f.Close()); err != nil {
		t.Fatal(err)
	}
	return path
}

func sum(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func TestVerifyBackupRejectsEveryWayABackupCanDisagree(t *testing.T) {
	want := map[string]string{"a": sum("alpha"), "b": sum("beta")}
	manifest := sum("alpha") + "  a\n" + sum("beta") + "  b\n"
	good := [][2]string{{"a", "alpha"}, {"b", "beta"}, {pruneManifestMember, manifest}}
	path := writeTarGz(t, good)
	if _, n, err := verifyBackup(path, want); err != nil || n == 0 {
		t.Fatalf("a good backup: %d, %v", n, err)
	}
	cases := map[string]struct {
		members [][2]string
		want    string
	}{
		"wrong content":        {[][2]string{{"a", "alpha"}, {"b", "BETA"}, {pruneManifestMember, manifest}}, "reads back as"},
		"member missing":       {[][2]string{{"a", "alpha"}, {pruneManifestMember, manifest}}, "expected 2"},
		"unexpected member":    {[][2]string{{"a", "alpha"}, {"b", "beta"}, {"c", "gamma"}, {pruneManifestMember, manifest}}, "expected 2"},
		"duplicate member":     {[][2]string{{"a", "alpha"}, {"a", "alpha"}, {pruneManifestMember, manifest}}, "twice"},
		"manifest disagrees":   {[][2]string{{"a", "alpha"}, {"b", "beta"}, {pruneManifestMember, sum("alpha") + "  a\n"}}, "manifest disagrees"},
		"manifest missing":     {[][2]string{{"a", "alpha"}, {"b", "beta"}}, "manifest disagrees"},
		"same count other key": {[][2]string{{"a", "alpha"}, {"z", "beta"}, {pruneManifestMember, manifest}}, "reads back as"},
	}
	for name, c := range cases {
		if _, _, err := verifyBackup(writeTarGz(t, c.members), want); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: got %v, want %q", name, err, c.want)
		}
	}
	if _, _, err := verifyBackup(filepath.Join(t.TempDir(), "absent.tar.gz"), want); err == nil {
		t.Fatal("an absent backup verified")
	}
	// A tar cut short inside a member.
	b, _ := os.ReadFile(path)
	short := filepath.Join(t.TempDir(), "short.tar.gz")
	var cut bytes.Buffer
	gz := gzip.NewWriter(&cut)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "a", Mode: 0o644, Size: 100, Typeflag: tar.TypeReg})
	tw.Write([]byte("only a little"))
	gz.Close()
	os.WriteFile(short, cut.Bytes(), 0o600)
	if _, _, err := verifyBackup(short, want); err == nil {
		t.Fatalf("a short member verified (%d byte reference)", len(b))
	}
}

func TestProtectedBySplitsFindsThePinsForThisPack(t *testing.T) {
	f := newPruneFixture(t, 3, time.Minute)
	dir := t.TempDir()
	v1 := filepath.Join(dir, "v1.json")
	writePruneFile(t, v1, `{"pack_digest":"sha256:pack-a","sidecar_revision":2}`, pruneEpoch)
	v1Other := filepath.Join(dir, "v1-other.json")
	writePruneFile(t, v1Other, `{"pack_digest":"sha256:pack-b","sidecar_revision":9}`, pruneEpoch)
	v1Unpinned := filepath.Join(dir, "v1-unpinned.json")
	writePruneFile(t, v1Unpinned, `{"pack_digest":"sha256:pack-a"}`, pruneEpoch)
	frozen := filepath.Join(dir, "frozen.json")
	writePruneFile(t, frozen, `{"packs":[{"pack_digest":"sha256:pack-b","sidecar_revision":5},{"pack_digest":"sha256:pack-a","sidecar_revision":3},{"pack_digest":"sha256:pack-a"}]}`, pruneEpoch)
	got, err := ProtectedBySplits(f.dir, []string{v1, v1Other, v1Unpinned, frozen})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[2] != "pinned by v1.json" || got[3] != "pinned by frozen.json" {
		t.Fatalf("got %v", got)
	}
	if got, err := ProtectedBySplits(f.dir, nil); err != nil || len(got) != 0 {
		t.Fatalf("no splits: %v, %v", got, err)
	}
	if _, err := ProtectedBySplits(f.dir, []string{filepath.Join(dir, "absent.json")}); err == nil {
		t.Fatal("an absent split was read")
	}
	bad := filepath.Join(dir, "bad.json")
	writePruneFile(t, bad, `{not json`, pruneEpoch)
	if _, err := ProtectedBySplits(f.dir, []string{bad}); err == nil {
		t.Fatal("a malformed split was read")
	}
	if _, err := ProtectedBySplits(t.TempDir(), nil); err == nil {
		t.Fatal("a pack with no manifest")
	}
	writePruneFile(t, filepath.Join(f.dir, manifestFile), `{"pack_digest":""}`, pruneEpoch)
	if _, err := ProtectedBySplits(f.dir, nil); err == nil {
		t.Fatal("a manifest with no digest")
	}
}

func TestAPrunedPackStillLoadsItsRetainedRevisions(t *testing.T) {
	// The store reads retained revisions by number and refuses a missing one by
	// name; neither depends on the history being contiguous.
	p := synthPack(t)
	s := saveReference(t, p)
	for i := 0; i < 6; i++ {
		s.Change = Provenance{Author: "tester", Session: "s", Operation: "save_mask"}
		if err := SaveSidecar(p, s); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := PlanPrune(p.Dir, PrunePolicy{KeepLast: 1})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Pruned == 0 {
		t.Fatalf("plan %+v", plan)
	}
	if _, err := plan.Execute(PruneExecuteOptions{BackupPath: filepath.Join(t.TempDir(), "b.tar.gz")}); err != nil {
		t.Fatal(err)
	}
	cur, err := LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	if kept, err := LoadSidecarRevision(p, 1); err != nil || kept.Revision != 1 {
		t.Fatalf("the oldest revision: %v", err)
	}
	if _, err := LoadSidecarRevision(p, 3); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a pruned revision should be absent, got %v", err)
	}
	// And the head still saves.
	cur.Change = Provenance{Author: "tester", Session: "s", Operation: "save_mask"}
	if err := SaveSidecar(p, cur); err != nil {
		t.Fatalf("a save after a prune: %v", err)
	}
}

// withHook runs fn with pruneHook set and clears it afterwards.
func withHook(hook func(stage, path string), fn func()) {
	pruneHook = hook
	defer func() { pruneHook = nil }()
	fn()
}

func TestExecuteSurvivesFaultsAtEachStage(t *testing.T) {
	t.Run("pack gone before the run", func(t *testing.T) {
		f := newPruneFixture(t, 5, time.Minute)
		plan, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
		if err != nil {
			t.Fatal(err)
		}
		os.RemoveAll(f.dir)
		if _, err := plan.Execute(PruneExecuteOptions{BackupPath: f.backup(t)}); err == nil {
			t.Fatal("executed against a missing pack")
		}
	})

	t.Run("backup path taken before the rename", func(t *testing.T) {
		f := newPruneFixture(t, 5, time.Minute)
		plan, _ := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
		backup := f.backup(t)
		withHook(func(stage, path string) {
			if stage == "before-rename" {
				os.Mkdir(path, 0o700)
				os.WriteFile(filepath.Join(path, "x"), []byte("x"), 0o600)
			}
		}, func() {
			if _, err := plan.Execute(PruneExecuteOptions{BackupPath: backup}); err == nil {
				t.Fatal("renamed a backup over a directory")
			}
		})
		if _, err := os.Lstat(backup + ".partial"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("partial left: %v", err)
		}
		for r := 1; r <= 5; r++ {
			if !f.has(t, r) {
				t.Fatalf("revision %d removed", r)
			}
		}
	})

	t.Run("an archive that cannot be removed is skipped and reported", func(t *testing.T) {
		f := newPruneFixture(t, 5, time.Minute)
		plan, _ := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
		var swapped string
		withHook(func(stage, name string) {
			if stage != "before-remove" || swapped != "" {
				return
			}
			// Replace the file with a non-empty directory just before its removal.
			swapped = filepath.Join(f.dir, name)
			os.Remove(swapped)
			os.Mkdir(swapped, 0o700)
			os.WriteFile(filepath.Join(swapped, "x"), []byte("x"), 0o600)
		}, func() {
			report, err := plan.Execute(PruneExecuteOptions{BackupPath: f.backup(t)})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Skipped) != 1 || !strings.Contains(report.Skipped[0], "could not be removed") || report.Removed != 2 {
				t.Fatalf("report %+v", report)
			}
		})
	})

	t.Run("history directory gone before the final sync", func(t *testing.T) {
		f := newPruneFixture(t, 5, time.Minute)
		plan, _ := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
		withHook(func(stage, _ string) {
			if stage == "before-sync" {
				os.Rename(filepath.Join(f.dir, revisionDir), filepath.Join(f.dir, revisionDir+".moved"))
			}
		}, func() {
			report, err := plan.Execute(PruneExecuteOptions{BackupPath: f.backup(t)})
			if err == nil || !strings.Contains(err.Error(), "sync") || report == nil || report.Removed != 3 {
				t.Fatalf("report %+v, %v", report, err)
			}
		})
	})

	t.Run("an archive replaced by a directory is not backed up", func(t *testing.T) {
		f := newPruneFixture(t, 5, time.Minute)
		plan, _ := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
		path := filepath.Join(f.dir, revisionName(2))
		os.Remove(path)
		os.Mkdir(path, 0o700)
		if _, err := plan.Execute(PruneExecuteOptions{BackupPath: f.backup(t)}); err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestVerifyBackupRejectsABrokenTarAndAFlippedChecksum(t *testing.T) {
	want := map[string]string{"a": sum("alpha")}
	// A gzip stream that holds no tar.
	var junk bytes.Buffer
	gz := gzip.NewWriter(&junk)
	gz.Write(bytes.Repeat([]byte{0xff}, 1024))
	gz.Close()
	bad := filepath.Join(t.TempDir(), "junk.tar.gz")
	os.WriteFile(bad, junk.Bytes(), 0o600)
	if _, _, err := verifyBackup(bad, want); err == nil || !strings.Contains(err.Error(), "tar") {
		t.Fatalf("junk tar: %v", err)
	}
	// A good backup whose gzip checksum is damaged reads as damaged even though
	// every tar member still parses.
	good := writeTarGz(t, [][2]string{{"a", "alpha"}, {pruneManifestMember, sum("alpha") + "  a\n"}})
	b, _ := os.ReadFile(good)
	b[len(b)-5] ^= 0xff
	os.WriteFile(good, b, 0o600)
	if _, _, err := verifyBackup(good, want); err == nil {
		t.Fatal("a damaged gzip checksum verified")
	}
}

func TestExecuteRefusesAnArchiveThatShrinksWhileItIsCopied(t *testing.T) {
	f := newPruneFixture(t, 5, time.Minute)
	plan, _ := PlanPrune(f.dir, PrunePolicy{KeepLast: 1})
	withHook(func(stage, source string) {
		if stage == "before-copy" && source == revisionName(3) {
			os.Truncate(filepath.Join(f.dir, source), 2)
		}
	}, func() {
		if _, err := plan.Execute(PruneExecuteOptions{BackupPath: f.backup(t)}); err == nil || !strings.Contains(err.Error(), "changed while it was read") {
			t.Fatalf("got %v", err)
		}
	})
	for r := 1; r <= 5; r++ {
		if !f.has(t, r) {
			t.Fatalf("revision %d removed", r)
		}
	}
}

func TestCopyMemberRefusesAHeaderTheArchiveCannotHold(t *testing.T) {
	var buf bytes.Buffer
	if _, err := copyMember(tar.NewWriter(&buf), &tar.Header{Name: "x", Size: -1, Typeflag: tar.TypeReg}, strings.NewReader("x")); err == nil {
		t.Fatal("a negative size was written")
	}
}

func TestPlanPruneNamesAnUnreadableHistory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any directory")
	}
	f := newPruneFixture(t, 3, time.Minute)
	hist := filepath.Join(f.dir, revisionDir)
	if err := os.Chmod(hist, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(hist, 0o700)
	if _, err := PlanPrune(f.dir, PrunePolicy{KeepLast: 1}); err == nil || !strings.Contains(err.Error(), "list") {
		t.Fatalf("got %v", err)
	}
}
