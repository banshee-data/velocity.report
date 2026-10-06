package annotation

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Every save archives the exact bytes it replaced as a full snapshot, so the
// history grows by one document per save: a pack with 3,700 masks writes about
// 25 MB each time, and 1,968 saves came to 38 GB. History is the recovery path,
// and a frozen split pins one revision by digest, but almost all of it is never
// read again. PlanPrune chooses what to keep; Execute moves the rest into a
// verified compressed backup and only then removes it.

// revisionFileName matches the archive files the store writes and nothing else.
var revisionFileName = regexp.MustCompile(`^(\d{10})\.json$`)

const (
	// pruneHeadMember is the current annotation snapshot, copied into the
	// backup so it holds the state the history ran up to. It is outside the
	// annotation-revisions/ path so extracting a backup into a pack cannot
	// overwrite the live head.
	pruneHeadMember = "head/annotations.json"
	// pruneManifestMember lists every member's SHA-256 in sha256sum format.
	pruneManifestMember = "MANIFEST.sha256"
	// DefaultPruneMinFreeRatio is how much of the pruned bytes the backup
	// volume must have free before a backup starts. JSON snapshots compress
	// several times over; this leaves a wide margin.
	DefaultPruneMinFreeRatio = 0.25
)

// Reasons a revision stays.
const (
	PruneKeepNewest    = "newest"
	PruneKeepOldest    = "oldest"
	PruneKeepThinned   = "thinned"
	PruneKeepProtected = "protected"
	prunePruned        = "pruned"
)

// PrunePolicy chooses which retained annotation revisions stay in the pack.
type PrunePolicy struct {
	// KeepLast keeps the newest this many retained revisions. At least one: the
	// newest archive is the cheapest recovery from the last edit.
	KeepLast int
	// KeepEvery thins what is older than the newest KeepLast: the earliest
	// revision in each interval stays, by the time its file was archived. Zero
	// keeps none of the older ones beyond the oldest and the protected.
	KeepEvery time.Duration
	// Protect names revisions that must stay and why, for instance because a
	// frozen split pins them. A name is for the report only.
	Protect map[int]string
}

// PruneEntry is one retained revision and the plan for it.
type PruneEntry struct {
	Revision int       `json:"revision"`
	Bytes    int64     `json:"bytes"`
	ModTime  time.Time `json:"mod_time"`
	Keep     bool      `json:"keep"`
	// Reason is why it stays (a Prune* constant, with the protector's name for
	// a protected one), or "pruned".
	Reason string `json:"reason"`
}

// PrunePlan is what a policy would do to one pack's retained history.
type PrunePlan struct {
	PackDir     string       `json:"pack_dir"`
	Policy      PrunePolicy  `json:"-"`
	Entries     []PruneEntry `json:"entries"`
	Kept        int          `json:"kept"`
	Pruned      int          `json:"pruned"`
	KeptBytes   int64        `json:"kept_bytes"`
	PrunedBytes int64        `json:"pruned_bytes"`
	// KeptByReason counts the kept revisions by why they stay.
	KeptByReason map[string]int `json:"kept_by_reason"`
	// Ignored are names in the history directory that are not archive files:
	// they are never touched.
	Ignored []string `json:"ignored,omitempty"`
}

// PlanPrune reads a pack's retained history and decides, without writing
// anything, which revisions a policy keeps. Only regular files named like the
// store's archives count; anything else in the directory is listed as ignored.
func PlanPrune(packDir string, policy PrunePolicy) (*PrunePlan, error) {
	if policy.KeepLast < 1 {
		return nil, fmt.Errorf("keep-last %d: keep at least the newest archived revision", policy.KeepLast)
	}
	if policy.KeepEvery < 0 {
		return nil, fmt.Errorf("keep-every %s is negative", policy.KeepEvery)
	}
	root, err := os.OpenRoot(packDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(revisionDir)
	if err != nil {
		return nil, fmt.Errorf("annotation history: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("history %s must be a real directory", revisionDir)
	}
	listing, err := fs.ReadDir(root.FS(), revisionDir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", revisionDir, err)
	}

	plan := &PrunePlan{PackDir: packDir, Policy: policy, KeptByReason: map[string]int{}}
	for _, entry := range listing {
		name := entry.Name()
		m := revisionFileName.FindStringSubmatch(name)
		rev := 0
		if m != nil {
			rev, _ = strconv.Atoi(m[1]) // ten digits always parse
		}
		fi, err := entry.Info()
		if rev < 1 || err != nil || !fi.Mode().IsRegular() {
			plan.Ignored = append(plan.Ignored, name)
			continue
		}
		plan.Entries = append(plan.Entries, PruneEntry{Revision: rev, Bytes: fi.Size(), ModTime: fi.ModTime().UTC()})
	}
	sort.Slice(plan.Entries, func(i, j int) bool { return plan.Entries[i].Revision < plan.Entries[j].Revision })

	n := len(plan.Entries)
	bucket := int64(-1)
	for i := range plan.Entries {
		e := &plan.Entries[i]
		switch {
		case i >= n-policy.KeepLast:
			e.Keep, e.Reason = true, PruneKeepNewest
		case policy.Protect[e.Revision] != "":
			e.Keep, e.Reason = true, PruneKeepProtected+": "+policy.Protect[e.Revision]
		case i == 0:
			e.Keep, e.Reason = true, PruneKeepOldest
			if policy.KeepEvery > 0 {
				// The oldest stands for its own interval.
				bucket = e.ModTime.UnixNano() / int64(policy.KeepEvery)
			}
		case policy.KeepEvery > 0:
			if b := e.ModTime.UnixNano() / int64(policy.KeepEvery); b != bucket {
				bucket = b
				e.Keep, e.Reason = true, PruneKeepThinned
			}
		}
		if !e.Keep {
			e.Reason = prunePruned
			plan.Pruned++
			plan.PrunedBytes += e.Bytes
			continue
		}
		plan.Kept++
		plan.KeptBytes += e.Bytes
		plan.KeptByReason[strings.SplitN(e.Reason, ":", 2)[0]]++
	}
	// A protected name with no archive file is not an error: the pinned
	// revision may be the head, which is not in the history directory.
	return plan, nil
}

// ProtectedBySplits returns the annotation revisions that split files pin for
// this pack, so a prune cannot remove the bytes a frozen split's digest names.
// A version 1 manifest pins one revision when it sets sidecar_revision; a frozen
// split pins one per pack. Entries for another pack are ignored.
func ProtectedBySplits(packDir string, splitPaths []string) (map[int]string, error) {
	b, err := os.ReadFile(filepath.Join(packDir, manifestFile))
	if err != nil {
		return nil, fmt.Errorf("read pack manifest: %w", err)
	}
	var man struct {
		PackDigest string `json:"pack_digest"`
	}
	if err := json.Unmarshal(b, &man); err != nil || man.PackDigest == "" {
		return nil, fmt.Errorf("pack manifest has no pack_digest")
	}
	out := map[int]string{}
	for _, path := range splitPaths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read split %s: %w", path, err)
		}
		var s struct {
			PackDigest      string `json:"pack_digest"`
			SidecarRevision int    `json:"sidecar_revision"`
			Packs           []struct {
				PackDigest      string `json:"pack_digest"`
				SidecarRevision int    `json:"sidecar_revision"`
			} `json:"packs"`
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("parse split %s: %w", path, err)
		}
		why := "pinned by " + filepath.Base(path)
		if s.PackDigest == man.PackDigest && s.SidecarRevision > 0 {
			out[s.SidecarRevision] = why
		}
		for _, p := range s.Packs {
			if p.PackDigest == man.PackDigest && p.SidecarRevision > 0 {
				out[p.SidecarRevision] = why
			}
		}
	}
	return out, nil
}

// PruneExecuteOptions controls a prune that writes.
type PruneExecuteOptions struct {
	// BackupPath receives every pruned revision, and the head, as a
	// gzip-compressed tar before anything is removed. It must not exist, must
	// end in .tar.gz and must be outside the pack.
	BackupPath string
	// MinFreeRatio is the fraction of the pruned bytes the backup volume must
	// have free. Zero means DefaultPruneMinFreeRatio.
	MinFreeRatio float64
	// Progress, when set, is told how far the backup has got.
	Progress func(string)
}

// PruneReport is what Execute did.
type PruneReport struct {
	PackDir      string        `json:"pack_dir"`
	BackupPath   string        `json:"backup_path"`
	BackupBytes  int64         `json:"backup_bytes"`
	BackupSHA256 string        `json:"backup_sha256"`
	BackedUp     int           `json:"backed_up"`
	BackedBytes  int64         `json:"backed_up_bytes"`
	Removed      int           `json:"removed"`
	RemovedBytes int64         `json:"removed_bytes"`
	Skipped      []string      `json:"skipped,omitempty"`
	Elapsed      time.Duration `json:"elapsed_ns"`
}

// pruneHook is nil outside tests. It is told which stage Execute has reached
// and the path that stage concerns, so a test can change the pack or the backup
// at exactly the moment a race or a fault would.
var pruneHook func(stage, path string)

func pruneStage(stage, path string) {
	if pruneHook != nil {
		pruneHook(stage, path)
	}
}

// Execute backs up every pruned revision, proves the backup reads back with the
// digests the source had, and only then removes them. Order matters:
//
//  1. The backup is written and verified without the writer lock. Archives are
//     write-once, and a save made meanwhile only adds a newer file, so the set
//     being copied cannot change under it.
//  2. The lock is taken for the removal alone, and a busy lock aborts it with
//     the backup kept: the editor keeps working and the prune is run again.
//  3. A file whose size or time no longer matches the plan is not removed.
//
// A failure before step 3 leaves the pack exactly as it was.
func (pl *PrunePlan) Execute(opts PruneExecuteOptions) (*PruneReport, error) {
	start := time.Now()
	report := &PruneReport{PackDir: pl.PackDir}
	if pl.Pruned == 0 {
		return report, nil
	}
	root, err := os.OpenRoot(pl.PackDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := checkBackupPath(pl.PackDir, opts.BackupPath); err != nil {
		return nil, err
	}
	ratio := opts.MinFreeRatio
	if ratio == 0 {
		ratio = DefaultPruneMinFreeRatio
	}
	if err := checkFreeSpace(filepath.Dir(opts.BackupPath), int64(float64(pl.PrunedBytes)*ratio)); err != nil {
		return nil, err
	}
	say := opts.Progress
	if say == nil {
		say = func(string) {}
	}

	sums, err := pl.writeBackup(root, opts.BackupPath, say)
	if err != nil {
		return nil, err
	}
	report.BackupPath = opts.BackupPath
	pruneStage("before-verify", opts.BackupPath)
	backupSHA, backupBytes, err := verifyBackup(opts.BackupPath, sums)
	if err != nil {
		return nil, fmt.Errorf("backup %s did not verify, nothing was removed: %w", opts.BackupPath, err)
	}
	report.BackupSHA256, report.BackupBytes = backupSHA, backupBytes
	for _, e := range pl.Entries {
		if !e.Keep {
			report.BackedUp++
			report.BackedBytes += e.Bytes
		}
	}
	say(fmt.Sprintf("backup verified: %d revisions, %d bytes, sha256 %s", report.BackedUp, backupBytes, backupSHA))
	pruneStage("after-backup", opts.BackupPath)

	lock, err := lockAnnotations(root)
	if err != nil {
		return nil, fmt.Errorf("backup %s is kept, nothing was removed: %w", opts.BackupPath, err)
	}
	defer lock.Close()
	for _, e := range pl.Entries {
		if e.Keep {
			continue
		}
		name := revisionName(e.Revision)
		fi, err := root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			report.Skipped = append(report.Skipped, fmt.Sprintf("%s: already gone", filepath.Base(name)))
			continue
		}
		if err != nil || !fi.Mode().IsRegular() || fi.Size() != e.Bytes || !fi.ModTime().UTC().Equal(e.ModTime) {
			report.Skipped = append(report.Skipped, fmt.Sprintf("%s: changed since the plan", filepath.Base(name)))
			continue
		}
		pruneStage("before-remove", name)
		if err := root.Remove(name); err != nil {
			report.Skipped = append(report.Skipped, fmt.Sprintf("%s: could not be removed (%v)", filepath.Base(name), err))
			continue
		}
		report.Removed++
		report.RemovedBytes += e.Bytes
	}
	pruneStage("before-sync", pl.PackDir)
	if err := syncRootDir(root, revisionDir); err != nil {
		return report, fmt.Errorf("sync %s: %w", revisionDir, err)
	}
	report.Elapsed = time.Since(start)
	return report, nil
}

// checkBackupPath refuses a backup that could harm the pack or an earlier backup.
func checkBackupPath(packDir, backup string) error {
	if backup == "" {
		return fmt.Errorf("a prune that removes files needs a backup path: no revision is removed before a backup is verified")
	}
	if !strings.HasSuffix(backup, ".tar.gz") {
		return fmt.Errorf("backup %q must end in .tar.gz", backup)
	}
	if _, err := os.Stat(filepath.Dir(backup)); err != nil {
		return fmt.Errorf("backup directory: %w", err)
	}
	// Both resolve: the pack was opened a moment ago and the directory just
	// answered a stat, so an error here is a path that does not matter.
	abs, _ := filepath.Abs(backup)
	dir, _ := filepath.EvalSymlinks(filepath.Dir(abs))
	pack, _ := filepath.EvalSymlinks(packDir)
	if rel, err := filepath.Rel(pack, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("backup %s is inside the pack: put it on another path", backup)
	}
	if _, err := os.Lstat(abs); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("backup %s already exists or cannot be checked (%v): it is never overwritten", backup, err)
	}
	return nil
}

// checkFreeSpace refuses a backup the volume plainly cannot hold.
func checkFreeSpace(dir string, need int64) error {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return fmt.Errorf("free space on %s: %w", dir, err)
	}
	free := int64(uint64(st.Bavail) * uint64(st.Bsize))
	if free < need {
		return fmt.Errorf("%s has %d bytes free and the backup needs about %d: nothing was written", dir, free, need)
	}
	return nil
}

// writeBackup streams the pruned revisions and the head into a compressed tar
// beside its SHA-256 manifest, and returns the digest of each member as it was
// read from the pack. The tar appears under its final name only once complete.
func (pl *PrunePlan) writeBackup(root *os.Root, backup string, say func(string)) (map[string]string, error) {
	partial := backup + ".partial"
	f, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create backup: %w", err)
	}
	done := false
	defer func() {
		if !done {
			f.Close()
			os.Remove(partial)
		}
	}()
	gz, _ := gzip.NewWriterLevel(f, gzip.BestSpeed) // the level is valid
	tw := tar.NewWriter(gz)
	sums := map[string]string{}
	var order []string
	add := func(member, source string, want int64, modTime time.Time) error {
		src, err := root.OpenFile(source, os.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer src.Close()
		fi, err := src.Stat()
		if err != nil || !fi.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file (%v)", source, err)
		}
		if want >= 0 && (fi.Size() != want || !fi.ModTime().UTC().Equal(modTime)) {
			return fmt.Errorf("%s changed since the plan: archives are write-once", source)
		}
		hdr := &tar.Header{Name: member, Mode: 0o644, Size: fi.Size(), ModTime: fi.ModTime(), Typeflag: tar.TypeReg, Format: tar.FormatPAX}
		pruneStage("before-copy", source)
		h := sha256.New()
		n, err := copyMember(tw, hdr, io.TeeReader(src, h))
		if err != nil || n != fi.Size() {
			return fmt.Errorf("%s changed while it was read: %d of %d bytes (%v)", source, n, fi.Size(), err)
		}
		sums[member] = hex.EncodeToString(h.Sum(nil))
		order = append(order, member)
		return nil
	}

	var written int64
	for i, e := range pl.Entries {
		if e.Keep {
			continue
		}
		if err := add(fmt.Sprintf("%s/%010d.json", revisionDir, e.Revision), revisionName(e.Revision), e.Bytes, e.ModTime); err != nil {
			return nil, fmt.Errorf("back up revision %d: %w", e.Revision, err)
		}
		written += e.Bytes
		if i%50 == 49 {
			say(fmt.Sprintf("backed up through revision %d (%d bytes read)", e.Revision, written))
		}
	}
	// The head can be replaced by a save while it is copied; the rename makes
	// either the old or the new file whole, and the digest records which.
	if err := add(pruneHeadMember, sidecarFile, -1, time.Time{}); err != nil {
		return nil, fmt.Errorf("back up the current snapshot: %w", err)
	}
	var manifest strings.Builder
	for _, m := range order {
		fmt.Fprintf(&manifest, "%s  %s\n", sums[m], m)
	}
	body := manifest.String()
	_, merr := copyMember(tw, &tar.Header{Name: pruneManifestMember, Mode: 0o644, Size: int64(len(body)), ModTime: time.Now(), Typeflag: tar.TypeReg}, strings.NewReader(body))
	if err := errors.Join(merr, tw.Close(), gz.Close(), f.Sync(), f.Close()); err != nil {
		return nil, fmt.Errorf("finish backup: %w", err)
	}
	pruneStage("before-rename", backup)
	if err := os.Rename(partial, backup); err != nil {
		return nil, err
	}
	done = true
	if d, err := os.Open(filepath.Dir(backup)); err == nil {
		d.Sync()
		d.Close()
	}
	return sums, nil
}

// copyMember writes one tar member and returns how many bytes of it were copied.
func copyMember(tw *tar.Writer, hdr *tar.Header, from io.Reader) (int64, error) {
	if err := tw.WriteHeader(hdr); err != nil {
		return 0, err
	}
	return io.Copy(tw, from)
}

// countingHash is a SHA-256 that also counts the bytes written to it.
type countingHash struct {
	sum interface {
		io.Writer
		Sum([]byte) []byte
	}
	n int64
}

func (c *countingHash) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return c.sum.Write(p)
}

// verifyBackup reads the finished tar back and compares every member with the
// digest taken from the pack, and the manifest with both. It returns the
// SHA-256 and size of the backup file itself.
func verifyBackup(backup string, want map[string]string) (string, int64, error) {
	f, err := os.Open(backup)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	whole := &countingHash{sum: sha256.New()}
	gz, err := gzip.NewReader(io.TeeReader(f, whole))
	if err != nil {
		return "", 0, err
	}
	tr := tar.NewReader(gz)
	seen := map[string]string{}
	var manifest strings.Builder
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", 0, err
		}
		h := sha256.New()
		var dst io.Writer = h
		if hdr.Name == pruneManifestMember {
			dst = io.MultiWriter(h, &manifest)
		}
		if n, err := io.Copy(dst, tr); err != nil || n != hdr.Size {
			return "", 0, fmt.Errorf("read %s: %d of %d bytes: %v", hdr.Name, n, hdr.Size, err)
		}
		if hdr.Name == pruneManifestMember {
			continue
		}
		if _, dup := seen[hdr.Name]; dup {
			return "", 0, fmt.Errorf("member %s appears twice", hdr.Name)
		}
		seen[hdr.Name] = hex.EncodeToString(h.Sum(nil))
	}
	// Reading the gzip stream to its end checks its own CRC, and draws the
	// whole file through the tee, so the digest below covers every byte.
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return "", 0, err
	}
	if len(seen) != len(want) {
		return "", 0, fmt.Errorf("backup holds %d members, expected %d", len(seen), len(want))
	}
	var lines []string
	for name, sum := range want {
		if seen[name] != sum {
			return "", 0, fmt.Errorf("member %s reads back as %s, the pack had %s", name, seen[name], sum)
		}
		lines = append(lines, sum+"  "+name)
	}
	have := strings.Split(strings.TrimSpace(manifest.String()), "\n")
	sort.Strings(lines)
	sort.Strings(have)
	if strings.Join(lines, "\n") != strings.Join(have, "\n") {
		return "", 0, fmt.Errorf("the backup's own manifest disagrees with its members")
	}
	return hex.EncodeToString(whole.sum.Sum(nil)), whole.n, nil
}

// syncRootDir makes the removals in a directory durable.
func syncRootDir(root *os.Root, dir string) error {
	d, err := root.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
