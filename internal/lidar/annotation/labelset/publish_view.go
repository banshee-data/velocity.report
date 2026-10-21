package labelset

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// Publishing a view beside its label set.
//
// A label set holds no points, so nothing can be drawn or edited from it
// alone: that takes a pack, one run's view of the capture. A published view
// is that pack copied into the published label set's views/ folder, carrying
// its data and its physical references and linked to the set it sits in. Someone with the
// dataset opens the view, edits, saves, and the files they would commit are
// the label set's head files, labels.json and the view's records; the
// journals, locks and revision archives the editors write stay local.
//
// CheckView answers the question a committer needs answered: would a fresh
// copy of these tracked files open and hold, with no history behind them?

// lfsSuffixes are the files the dataset routes through LFS/Xet: the binary
// ones, which Hugging Face refuses in plain Git at any size. Everything else,
// the label set's text files included, is plain Git and must stay under
// MaxPlainGitBytes (tools/s2-archive/huggingface-dataset-layout.md).
var lfsSuffixes = []string{".bin", ".pb"}

// ViewReport describes a view and its label set as a fresh copy sees them.
type ViewReport struct {
	Pack       string   `json:"pack"`
	PackDigest string   `json:"pack_digest"`
	LabelSet   string   `json:"label_set"`
	LabelSetID string   `json:"label_set_id"`
	Revision   int      `json:"revision"`
	State      string   `json:"state"`
	Domain     string   `json:"domain"`
	Tracked    []string `json:"tracked"`
	Omitted    []string `json:"omitted,omitempty"`
	Problems   []string `json:"problems,omitempty"`
}

// PublishView copies a pack beside a published label set as its view. The
// pack must be linked to that label set, which must hold the same head as
// the pack's own label set and already know the pack's foreground as a
// domain, so that the first save from the view changes only what is edited.
// dest must be absent or empty and sit in the published set's views/ folder. Facets are
// left out: they pin label set revisions a published copy cannot rebuild.
// The copy is assembled beside dest and renamed into place, then checked as
// a fresh copy would be.
func PublishView(packDir, publishedSet, dest string) (*ViewReport, error) {
	var err error
	for _, p := range []*string{&packDir, &publishedSet, &dest} {
		if *p, err = filepath.Abs(*p); err != nil {
			return nil, err
		}
	}
	pack, err := annotation.OpenPack(packDir)
	if err != nil {
		return nil, err
	}
	link, err := annotation.ReadLabelSetLink(packDir)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, fmt.Errorf("%s is not linked to a label set", packDir)
	}
	src, err := Open(link.LabelSetDir(packDir))
	if err != nil {
		return nil, fmt.Errorf("the pack's label set: %w", err)
	}
	pub, err := Open(publishedSet)
	if err != nil {
		return nil, fmt.Errorf("published label set: %w", err)
	}
	switch {
	case link.LabelSetID != pub.Manifest.LabelSetID || src.Manifest.LabelSetID != pub.Manifest.LabelSetID:
		return nil, fmt.Errorf("the pack is linked to label set %s, %s holds %s", link.LabelSetID, publishedSet, pub.Manifest.LabelSetID)
	case src.Revision() != pub.Revision() || src.StateDigest() != pub.StateDigest():
		return nil, fmt.Errorf("the pack's label set is at revision %d (%s), the published copy at %d (%s): republish the label set first",
			src.Revision(), src.StateDigest(), pub.Revision(), pub.StateDigest())
	}
	if domain := DomainOf(pack).DomainID; !hasDomain(&pub.State, domain) {
		return nil, fmt.Errorf("the published label set does not know this pack's foreground %s; the first save from the view would add it whole: run 'velocity lidar labels attach' on the source and republish", domain)
	}
	if views := filepath.Join(publishedSet, annotation.ViewsDir); filepath.Clean(filepath.Dir(dest)) != views {
		return nil, fmt.Errorf("a view sits in its label set's %s/ folder: %s is not in %s", annotation.ViewsDir, dest, views)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, err
	}
	if entries, err := os.ReadDir(dest); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("refusing to publish into non-empty %s", dest)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	publish := map[string]bool{annotation.PhysicalReferencesFile: true}
	for _, name := range annotation.PackDataFiles() {
		publish[name] = true
	}
	entries, err := os.ReadDir(packDir)
	if err != nil {
		return nil, err
	}
	var copyNames, omitted []string
	for _, e := range entries {
		name := e.Name()
		switch {
		case publish[name] && e.Type().IsRegular():
			copyNames = append(copyNames, name)
		case name == annotation.LabelSetLinkFile:
		case name == annotation.FeatureProposalsFile || annotation.IsPackLocalEntry(name):
			omitted = append(omitted, name)
		default:
			return nil, fmt.Errorf("%s holds %s, which a view neither carries nor leaves behind knowingly", packDir, name)
		}
	}

	tmp, err := os.MkdirTemp(filepath.Dir(dest), ".publish-view-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	for _, name := range copyNames {
		if err := copyFile(filepath.Join(packDir, name), filepath.Join(tmp, name)); err != nil {
			return nil, err
		}
	}
	staged, err := annotation.OpenPack(tmp)
	if err != nil {
		return nil, fmt.Errorf("the copied pack: %w", err)
	}
	if staged.Manifest.PackDigest != pack.Manifest.PackDigest {
		return nil, fmt.Errorf("the copied pack is %s, the source %s", staged.Manifest.PackDigest, pack.Manifest.PackDigest)
	}
	if err := annotation.WriteLabelSetLink(staged, link.LabelSetID, publishedSet); err != nil {
		return nil, err
	}
	// Checked before it is put in place: a view that would not hold is not
	// published at all.
	report, err := CheckView(tmp)
	if report != nil {
		report.Pack = dest
		staging := path.Join(filepath.Base(publishedSet), annotation.ViewsDir, filepath.Base(tmp)) + "/"
		for i, name := range report.Tracked {
			if strings.HasPrefix(name, staging) {
				report.Tracked[i] = path.Join(filepath.Base(publishedSet), annotation.ViewsDir, filepath.Base(dest)) + "/" + strings.TrimPrefix(name, staging)
			}
		}
		sort.Strings(omitted)
		report.Omitted = omitted
	}
	if err != nil {
		return report, err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return nil, err
	}
	if err := os.Remove(dest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return nil, err
	}
	return report, nil
}

// CheckView checks a view and its label set as a fresh copy of their
// tracked files would hold them, with no journal, lock or revision archive
// behind them: the view opens to the same pack, its label set's head
// digests, membership projects, every physical reference links and no
// review drifts, facets (if any) load against the memberships they pin, the
// label set already knows the view's foreground, and no tracked file is too
// large for plain Git unless the dataset sends it through LFS/Xet. The view
// itself is only read.
func CheckView(packDir string) (*ViewReport, error) {
	// A missing link reads as an unlinked pack, so a mistyped path, or a
	// relative one from another directory, needs saying first.
	if _, err := os.Stat(packDir); errors.Is(err, fs.ErrNotExist) {
		abs, _ := filepath.Abs(packDir)
		return nil, fmt.Errorf("no pack at %s: %w", abs, fs.ErrNotExist)
	}
	link, err := annotation.ReadLabelSetLink(packDir)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, fmt.Errorf("%s is not linked to a label set", packDir)
	}
	setDir := link.LabelSetDir(packDir)
	// Two places a linked pack may be: in its set's views/, linked ../..,
	// or in its capture's packs/ beside the set, linked ../../<set>.
	inViews := link.Path == annotation.ViewLinkPath && filepath.Base(filepath.Dir(packDir)) == annotation.ViewsDir
	dir, name := path.Split(link.Path)
	inPacks := filepath.Base(filepath.Dir(packDir)) == annotation.PacksDir && dir == "../../" && name != "" && name != "." && name != ".."
	if !inViews && !inPacks {
		return nil, fmt.Errorf("a linked pack sits in its label set's %s/ (linking %q) or in its capture's %s/ beside the set (linking \"../../<set>\"); %s links %q (run 'velocity lidar labels nest')",
			annotation.ViewsDir, annotation.ViewLinkPath, annotation.PacksDir, packDir, link.Path)
	}

	fresh, err := os.MkdirTemp("", "check-view-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(fresh)
	// The fresh copy is the capture folder: the set, and the pack where it
	// sits relative to it.
	viewRel, err := filepath.Rel(filepath.Dir(setDir), packDir)
	if err != nil {
		return nil, err
	}
	setCopy := filepath.Join(fresh, filepath.Base(setDir))
	viewCopy := filepath.Join(fresh, viewRel)
	report := &ViewReport{Pack: packDir, LabelSet: setDir, LabelSetID: link.LabelSetID}
	problem := func(format string, args ...any) {
		report.Problems = append(report.Problems, fmt.Sprintf(format, args...))
	}

	// What the dataset tracks of a pack: its data, its physical references
	// and its link. Facets are working state the dataset ignores: they pin
	// label set revisions a copy without history cannot rebuild.
	tracked := map[string]bool{annotation.LabelSetLinkFile: true, annotation.PhysicalReferencesFile: true}
	for _, name := range annotation.PackDataFiles() {
		tracked[name] = true
	}
	if err := copySetFiles(setDir, setCopy, report); err != nil {
		return nil, err
	}
	viewPrefix := filepath.ToSlash(viewRel)
	if err := copyTracked(packDir, viewCopy, viewPrefix, func(name string) bool { return tracked[name] }, report); err != nil {
		return nil, err
	}
	for _, name := range report.Tracked {
		info, err := os.Stat(filepath.Join(fresh, filepath.FromSlash(name)))
		if err == nil && info.Size() > MaxPlainGitBytes && !hasLFSSuffix(name) {
			problem("%s is %d bytes, over Hugging Face's %d for plain Git, and the dataset keeps it in plain Git", name, info.Size(), MaxPlainGitBytes)
		}
	}
	before, err := listTree(fresh)
	if err != nil {
		return nil, err
	}

	pack, err := annotation.OpenPack(viewCopy)
	if err != nil {
		return nil, fmt.Errorf("a fresh copy of the view does not open: %w", err)
	}
	report.PackDigest = pack.Manifest.PackDigest
	ls, err := Open(setCopy)
	if err != nil {
		return nil, fmt.Errorf("a fresh copy of the label set does not open: %w", err)
	}
	report.Revision, report.State = ls.Revision(), ls.StateDigest()
	if ls.Manifest.LabelSetID != link.LabelSetID {
		return nil, fmt.Errorf("the view is linked to label set %s, %s holds %s", link.LabelSetID, setDir, ls.Manifest.LabelSetID)
	}
	report.Domain = DomainOf(pack).DomainID
	if !hasDomain(&ls.State, report.Domain) {
		problem("the label set does not know the view's foreground %s: the first save would add it whole", report.Domain)
	}
	sidecar, err := annotation.LoadSidecar(pack)
	if err != nil {
		return nil, fmt.Errorf("the view's membership does not project from a fresh copy: %w", err)
	}
	refs, err := annotation.LoadPhysicalReferences(pack)
	if err != nil {
		problem("physical references do not load: %v", err)
	} else {
		for _, s := range refs.Stale() {
			problem("physical reference %s", s)
		}
		for _, d := range refs.ReviewDrift(pack, sidecar) {
			problem("review drift: %s (run 'velocity lidar annotation-reference repin' where the edit was made)", d)
		}
	}
	after, err := listTree(fresh)
	if err != nil {
		return nil, err
	}
	if !equalStrings(before, after) {
		problem("reading a fresh copy wrote files: %v", diffStrings(before, after))
	}
	if len(report.Problems) > 0 {
		return report, fmt.Errorf("%s would not hold as a fresh copy: %d problem(s)", packDir, len(report.Problems))
	}
	return report, nil
}

// copyTracked copies a directory's tracked files into dest and names them in
// the report as <prefix>/<file>, prefix being dest's path in the fresh copy.
func copyTracked(src, dest, prefix string, tracked func(string) bool, report *ViewReport) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !tracked(e.Name()) || !e.Type().IsRegular() {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dest, e.Name())); err != nil {
			return err
		}
		report.Tracked = append(report.Tracked, prefix+"/"+e.Name())
	}
	return nil
}

// copySetFiles copies a label set's tracked files, labels.json and its head
// in the storage layout, into dest and names them in the report.
func copySetFiles(src, dest string, report *ViewReport) error {
	tracked := map[string]bool{ManifestFile: true}
	for _, name := range wholeFiles {
		tracked[name] = true
	}
	if err := copyTracked(src, dest, filepath.Base(dest), func(name string) bool { return tracked[name] }, report); err != nil {
		return err
	}
	root, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, dir := range chunkDirs {
		names, err := chunkFiles(root, dir)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dest, dir), 0o755); err != nil {
			return err
		}
		for _, name := range names {
			if err := copyFile(filepath.Join(src, dir, name), filepath.Join(dest, dir, name)); err != nil {
				return err
			}
			report.Tracked = append(report.Tracked, filepath.Base(dest)+"/"+dir+"/"+name)
		}
	}
	return nil
}

func hasLFSSuffix(name string) bool {
	for _, s := range lfsSuffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func listTree(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, rel)
		return nil
	})
	sort.Strings(out)
	return out, err
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func diffStrings(before, after []string) []string {
	had := map[string]bool{}
	for _, s := range before {
		had[s] = true
	}
	var out []string
	for _, s := range after {
		if !had[s] {
			out = append(out, s)
		}
	}
	return out
}
