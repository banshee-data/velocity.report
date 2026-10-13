package labelset

import (
	"errors"
	"fmt"
	"io"
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
// is that pack copied beside the published label set, carrying its data and
// its physical references and linked to the set next to it. Someone with the
// dataset opens the view, edits, saves, and the files they would commit are
// the label set's head files, labels.json and the view's records; the
// journals, locks and revision archives the editors write stay local.
//
// CheckView answers the question a committer needs answered: would a fresh
// copy of these tracked files open and hold, with no history behind them?

// MaxPlainGitBytes is the largest file the dataset keeps in plain Git:
// Hugging Face refuses larger ones outside LFS/Xet. The dataset routes
// lfsSuffixes through LFS/Xet (tools/s2-archive/huggingface-dataset-layout.md).
const MaxPlainGitBytes = 10 << 20

var lfsSuffixes = []string{".bin", ".pb", ".ndjson"}

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
// dest must be absent or empty and sit beside the published set. Facets are
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
	if filepath.Clean(filepath.Dir(dest)) != filepath.Clean(filepath.Dir(publishedSet)) {
		return nil, fmt.Errorf("a view sits beside its label set: %s is not in %s", dest, filepath.Dir(publishedSet))
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
	link, err := annotation.ReadLabelSetLink(packDir)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, fmt.Errorf("%s is not linked to a label set", packDir)
	}
	setDir := link.LabelSetDir(packDir)
	if dir, name := path.Split(link.Path); dir != "../" || name == "" || strings.ContainsAny(name, `/\`) {
		return nil, fmt.Errorf("a view's label set sits beside it (../<name>); %s links %q", packDir, link.Path)
	}

	fresh, err := os.MkdirTemp("", "check-view-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(fresh)
	viewCopy := filepath.Join(fresh, filepath.Base(packDir))
	setCopy := filepath.Join(fresh, filepath.Base(setDir))
	report := &ViewReport{Pack: packDir, LabelSet: setDir, LabelSetID: link.LabelSetID}
	problem := func(format string, args ...any) {
		report.Problems = append(report.Problems, fmt.Sprintf(format, args...))
	}

	tracked := map[string]bool{annotation.LabelSetLinkFile: true}
	for _, name := range append(annotation.PackDataFiles(), annotation.PackRecordFiles()...) {
		tracked[name] = true
	}
	if err := copyTracked(packDir, viewCopy, func(name string) bool { return tracked[name] }, report); err != nil {
		return nil, err
	}
	setFiles := map[string]bool{ManifestFile: true}
	for _, name := range stateFiles {
		setFiles[name] = true
	}
	if err := copyTracked(setDir, setCopy, func(name string) bool { return setFiles[name] }, report); err != nil {
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
	if _, err := os.Stat(filepath.Join(viewCopy, annotation.FeatureProposalsFile)); err == nil {
		if _, err := annotation.LoadFeatures(pack); err != nil {
			problem("facets do not load from a fresh copy: %v", err)
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
// the report as <directory>/<file>.
func copyTracked(src, dest string, tracked func(string) bool, report *ViewReport) error {
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
		report.Tracked = append(report.Tracked, filepath.Base(dest)+"/"+e.Name())
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
