// Package capindex records what capture files are on a volume, what changed
// since the last look, and which of them form a continuous session.
//
// Scanning is deliberately split in two. Walking a root and stating its files
// is cheap and happens on every scan; probing a file's packet-time extent reads
// the whole file and happens separately, because a field volume holds hundreds
// of 700 MB captures and a scan that probed them all would take hours.
//
// The cheap half is enough to answer "what is here and what changed". The
// expensive half is what a file needs before it can join a sequence, and
// [Sessions] simply skips files that have not had it.
package capindex

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CaptureExtensions are the file suffixes treated as capture files.
var CaptureExtensions = []string{".pcap", ".pcapng"}

// TagChunkBytes is how much of each end of a file feeds the content tag.
const TagChunkBytes = 1 << 20 // 1 MiB

// ErrRootUnreachable is returned when a root's directory cannot be read, which
// on a field machine usually means the external volume is not mounted.
var ErrRootUnreachable = errors.New("capindex: capture root unreachable")

// File is one capture file as the cheap half of a scan sees it: what the
// filesystem says, plus a tag over its ends. Packet extents are not here
// because obtaining them is the expensive half.
type File struct {
	// RelPath is the path relative to its root, always slash-separated so the
	// same volume indexes identically on any host.
	RelPath string
	// SizeBytes and ModifiedAt come from the directory entry.
	SizeBytes  int64
	ModifiedAt time.Time
	// ContentTag is a digest over the file's first and last TagChunkBytes plus
	// its length. It is not a whole-file checksum, and does not pretend to be:
	// hashing a 700 MB capture on every scan would cost minutes per volume for
	// a guarantee that size and modification time already give in every case
	// but deliberate tampering. It does catch the failure that actually
	// happens, which is a capture truncated or rewritten in place.
	ContentTag string
}

// Scan walks a capture root and returns its capture files, ordered by relative
// path so two scans of an unchanged volume compare equal.
//
// Files that cannot be read are skipped rather than failing the scan: one
// unreadable capture on a volume of hundreds should not cost the operator the
// other 199 rows. A root that cannot be opened at all is a different matter and
// returns ErrRootUnreachable.
func Scan(root string) ([]File, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrRootUnreachable, root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrRootUnreachable, root)
	}

	var files []File
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subdirectory is skipped, not fatal.
			return nil //nolint:nilerr // deliberate: keep walking past unreadable entries
		}
		if d.IsDir() {
			// Analysis output lives beside the captures and is not capture data.
			if isExcludedDir(d.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !IsCaptureFile(path) {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		entryInfo, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		tag, tagErr := ContentTag(path, entryInfo.Size())
		if tagErr != nil {
			// A file we cannot read gets an empty tag rather than vanishing
			// from the index; it will simply always look changed.
			tag = ""
		}
		files = append(files, File{
			RelPath:    filepath.ToSlash(rel),
			SizeBytes:  entryInfo.Size(),
			ModifiedAt: entryInfo.ModTime(),
			ContentTag: tag,
		})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("capindex: walking %s: %w", root, walkErr)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	return files, nil
}

// excludedDirs are subdirectories a capture tool or this system writes beside
// the captures. Their contents are output, not capture data.
var excludedDirs = []string{"analysis", "segments", "vrlog", "plots"}

func isExcludedDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	for _, d := range excludedDirs {
		if name == d {
			return true
		}
	}
	// pcap-split writes timestamped analysis directories alongside the source.
	return strings.HasPrefix(name, "pcap_split_analysis_")
}

// IsCaptureFile reports whether a path looks like a capture file.
func IsCaptureFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, want := range CaptureExtensions {
		if ext == want {
			return true
		}
	}
	return false
}

// ContentTag digests a file's first and last TagChunkBytes together with its
// length. See File.ContentTag for what that does and does not cover.
func ContentTag(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	fmt.Fprintf(h, "%d\n", size)

	head := int64(TagChunkBytes)
	if size < head {
		head = size
	}
	if _, err := io.CopyN(h, f, head); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	// Only digest a tail when there is one the head did not already cover.
	if size > 2*TagChunkBytes {
		if _, err := f.Seek(size-TagChunkBytes, io.SeekStart); err != nil {
			return "", err
		}
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
