package annotation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The annotation tree. One layout, locally under --lidar-annotation-dir and in
// the dataset under labels/:
//
//	<root>/
//	  <capture>/                 one folder per capture, named for its file's stem
//	    <set>/                   a label set: labels.json and its head files
//	      views/<view>/          a pack linked to the set: label-set-link.json -> ../..
//	    packs/<run>/             a pack not in a set yet (local only)
//	  clip-<job>-<n>/pack/       a Segments clip job's pack, in the job's own
//	                             folder beside its recording (local only)
//	  splits/                    frozen splits
//
// A pack's labels go in its capture folder: the first save from a pack in
// packs/ creates the set beside packs/ and links the pack where it is;
// 'velocity lidar labels nest' moves linked packs under their set's views/.
// A clip job's folder stays where the job wrote it, since its row names it;
// its pack's labels go in <root>/<capture>/ like any other.

const (
	// PacksDir holds a capture's packs that are not in a label set yet.
	PacksDir = "packs"
	// ViewsDir, inside a label set, holds the packs linked to it.
	ViewsDir = "views"
	// ViewLinkPath is the link of a view nested in its label set.
	ViewLinkPath = "../.."
	// clipPackDir is the pack inside a Segments clip job's attempt directory,
	// whose name starts with clipAttemptPrefix.
	clipPackDir       = "pack"
	clipAttemptPrefix = "clip-"
)

// reservedCaptureNames are root folders that are not captures.
var reservedCaptureNames = map[string]bool{"splits": true, "labels": true, PacksDir: true, ViewsDir: true}

// CaptureNameOf is the capture folder name for a capture file: its base name
// without the extension, with anything but letters, digits, dots, dashes and
// underscores replaced by a dash. A name the layout reserves, or none at all,
// becomes "capture".
func CaptureNameOf(captureFile string) string {
	base := filepath.Base(filepath.FromSlash(strings.ReplaceAll(captureFile, `\`, "/")))
	for _, ext := range []string{".gz", ".pcapng", ".pcap"} {
		base = strings.TrimSuffix(base, ext)
	}
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), ".")
	if name == "" || reservedCaptureNames[name] {
		return "capture"
	}
	return name
}

// CaptureName is the capture folder a pack belongs in.
func CaptureName(m Manifest) string { return CaptureNameOf(m.Source.PCAPBasename) }

// ViewName is the folder name a pack takes as a view: when its run was cut,
// to the second, and the first eight characters of its recording's name,
// which is the run id. The run id and digest stay in its manifest.
func ViewName(m Manifest) string {
	run := CaptureNameOf(m.Source.VRLOGPath)
	if len(run) > 8 {
		run = run[:8]
	}
	return time.Unix(0, m.CreatedNs).UTC().Format("20060102-150405") + "-" + run
}

// CaptureDirOf is the capture folder of a pack directory: the folder its
// labels belong in. A pack outside the layout gets one beside it, named for
// its capture.
func CaptureDirOf(packDir string, m Manifest) string {
	dir := filepath.Clean(packDir)
	parent := filepath.Dir(dir)
	switch {
	case filepath.Base(parent) == ViewsDir:
		return filepath.Dir(filepath.Dir(parent))
	case filepath.Base(parent) == PacksDir:
		return filepath.Dir(parent)
	case IsClipPackDir(dir):
		return filepath.Join(filepath.Dir(parent), CaptureName(m))
	}
	return filepath.Join(parent, CaptureName(m))
}

// NewPackDir is where a new pack of a capture is written: in the capture's
// packs/ folder, under name.
func NewPackDir(root, captureName, name string) string {
	return filepath.Join(root, captureName, PacksDir, name)
}

// IsClipPackDir reports whether dir is the pack in a clip job's folder.
func IsClipPackDir(dir string) bool {
	return filepath.Base(dir) == clipPackDir && strings.HasPrefix(filepath.Base(filepath.Dir(dir)), clipAttemptPrefix)
}

// ValidPackHandle reports whether a handle, a pack's path relative to the
// root with forward slashes, has one of the layout's pack shapes:
// <capture>/packs/<run>, <capture>/<set>/views/<view> or
// clip-<job>-<n>/pack.
func ValidPackHandle(handle string) bool {
	if handle == "" || strings.HasPrefix(handle, "/") || strings.Contains(handle, `\`) {
		return false
	}
	parts := strings.Split(handle, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return false
		}
	}
	if reservedCaptureNames[parts[0]] {
		return false
	}
	switch len(parts) {
	case 2:
		return strings.HasPrefix(parts[0], clipAttemptPrefix) && parts[1] == clipPackDir
	case 3:
		return parts[1] == PacksDir
	case 4:
		return parts[1] != PacksDir && parts[2] == ViewsDir
	}
	return false
}

// PackHandle is a pack directory's handle beneath root, or an error when the
// directory is not a pack position of the layout.
func PackHandle(root, packDir string) (string, error) {
	rel, err := filepath.Rel(root, packDir)
	if err != nil {
		return "", err
	}
	h := filepath.ToSlash(rel)
	if !ValidPackHandle(h) {
		return "", fmt.Errorf("%s is not a pack position under %s (<capture>/packs/<run>, <capture>/<set>/views/<view> or clip-<job>/pack)", packDir, root)
	}
	return h, nil
}

// ListPackDirs lists every directory under root in a pack position that
// holds a manifest, sorted. It reads directory names and stats manifests
// only; it opens nothing.
func ListPackDirs(root string) ([]string, error) {
	captures, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	has := func(dir string) bool {
		info, err := os.Stat(filepath.Join(dir, manifestFile))
		return err == nil && info.Mode().IsRegular()
	}
	subdirs := func(dir string) []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				names = append(names, filepath.Join(dir, e.Name()))
			}
		}
		return names
	}
	for _, c := range captures {
		if !c.IsDir() || strings.HasPrefix(c.Name(), ".") || reservedCaptureNames[c.Name()] {
			continue
		}
		capture := filepath.Join(root, c.Name())
		if strings.HasPrefix(c.Name(), clipAttemptPrefix) {
			if has(filepath.Join(capture, clipPackDir)) {
				out = append(out, filepath.Join(capture, clipPackDir))
			}
			continue
		}
		for _, child := range subdirs(capture) {
			if filepath.Base(child) == PacksDir {
				for _, run := range subdirs(child) {
					if has(run) {
						out = append(out, run)
					}
				}
				continue
			}
			for _, view := range subdirs(filepath.Join(child, ViewsDir)) {
				if has(view) {
					out = append(out, view)
				}
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// PackDigestAt reads the pack digest a pack directory's manifest declares,
// without verifying the pack. It is for finding a pack that moved, by the
// digest a record kept; open the pack to trust it.
func PackDigestAt(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		return "", err
	}
	var m struct {
		PackDigest string `json:"pack_digest"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return "", err
	}
	return m.PackDigest, nil
}

// FindPackByDigest returns the pack position under root whose manifest
// declares digest, or "" when none does.
func FindPackByDigest(root, digest string) (string, error) {
	dirs, err := ListPackDirs(root)
	if err != nil {
		return "", err
	}
	for _, dir := range dirs {
		if d, err := PackDigestAt(dir); err == nil && d == digest {
			return dir, nil
		}
	}
	return "", nil
}
