package runner

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/jobs"
	"github.com/banshee-data/velocity.report/internal/security"
)

// Captures finds a manifest's files under the worker's capture root and
// verifies their bytes before a job runs.
//
// A manifest's path is a hint. The file is looked for there first, then
// anywhere under the root by size, and whichever is found is hashed. A
// digest is cached against the file's size, mtime and inode, so the second
// job over a four-gigabyte capture does not hash it again, and a file that
// has been replaced is.
type Captures struct {
	root      string
	cachePath string
	mu        sync.Mutex
	cache     map[string]captureCacheEntry
}

type captureCacheEntry struct {
	Size   int64       `json:"size"`
	MTime  int64       `json:"mtime_ns"`
	Inode  uint64      `json:"inode"`
	Device uint64      `json:"device"`
	SHA256 jobs.Digest `json:"sha256"`
}

// NewCaptures opens the cache under the work directory.
func NewCaptures(root, workDir string) *Captures {
	c := &Captures{root: root, cachePath: filepath.Join(workDir, "captures.json"), cache: map[string]captureCacheEntry{}}
	_ = readJSON(c.cachePath, &c.cache)
	return c
}

// Root is the capture root.
func (c *Captures) Root() string { return c.root }

// Resolved is one manifest capture, found and verified.
type Resolved struct {
	Capture jobs.Capture
	Path    string
}

// Resolve finds and verifies every capture in the manifest, failing closed on
// the first that cannot be found or whose bytes differ.
func (c *Captures) Resolve(m jobs.CaptureManifest, log func(string, ...any)) ([]Resolved, error) {
	out := make([]Resolved, 0, len(m.Captures))
	for _, cap := range m.Captures {
		path, err := c.locate(cap)
		if err != nil {
			return nil, err
		}
		digest, err := c.digest(path, log)
		if err != nil {
			return nil, err
		}
		if digest != cap.SHA256 {
			return nil, fmt.Errorf("capture %s at %s has digest %s, manifest says %s: not the same bytes",
				cap.LogicalID, path, digest.Short(), cap.SHA256.Short())
		}
		out = append(out, Resolved{Capture: cap, Path: path})
	}
	return out, nil
}

// Verified reports which of the digests this worker holds a verified copy of,
// from the cache alone: what it advertises without hashing anything.
func (c *Captures) Verified() []jobs.Digest {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[jobs.Digest]bool{}
	var out []jobs.Digest
	for path, e := range c.cache {
		info, err := os.Stat(path)
		if err != nil || !c.entryMatches(e, info) || seen[e.SHA256] {
			continue
		}
		seen[e.SHA256] = true
		out = append(out, e.SHA256)
	}
	return out
}

func (c *Captures) locate(cap jobs.Capture) (string, error) {
	hint := filepath.Join(c.root, filepath.FromSlash(cap.RelativePath))
	if err := security.ValidatePathWithinDirectory(hint, c.root); err != nil {
		return "", fmt.Errorf("capture %s: %w", cap.LogicalID, err)
	}
	if info, err := os.Stat(hint); err == nil && info.Mode().IsRegular() && info.Size() == cap.ByteSize {
		return hint, nil
	}
	// Not where the hint says. A file of the right name and size anywhere
	// under the root is a candidate; the hash decides.
	base := filepath.Base(cap.RelativePath)
	var found string
	err := filepath.WalkDir(c.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !d.IsDir() && d.Name() == base {
			if info, err := d.Info(); err == nil && info.Size() == cap.ByteSize {
				found = path
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("capture %s (%s, %d bytes) is not under %s", cap.LogicalID, cap.RelativePath, cap.ByteSize, c.root)
	}
	return found, nil
}

func (c *Captures) digest(path string, log func(string, ...any)) (jobs.Digest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	entry, ok := c.cache[path]
	c.mu.Unlock()
	if ok && c.entryMatches(entry, info) {
		return entry.SHA256, nil
	}
	if log != nil {
		log("hashing %s (%d bytes)", path, info.Size())
	}
	started := time.Now()
	digest, _, err := jobs.DigestFile(path)
	if err != nil {
		return "", err
	}
	if log != nil {
		log("hashed %s in %s: %s", filepath.Base(path), time.Since(started).Round(time.Millisecond), digest.Short())
	}
	entry = captureCacheEntry{Size: info.Size(), MTime: info.ModTime().UnixNano(), SHA256: digest}
	entry.Inode, entry.Device = inode(info)
	c.mu.Lock()
	c.cache[path] = entry
	// A cache write failure costs a hash next time, not a job: the digest
	// just computed is returned regardless.
	_ = writeJSON(c.cachePath, c.cache)
	c.mu.Unlock()
	return digest, nil
}

func (c *Captures) entryMatches(e captureCacheEntry, info os.FileInfo) bool {
	ino, dev := inode(info)
	return e.Size == info.Size() && e.MTime == info.ModTime().UnixNano() && e.Inode == ino && e.Device == dev
}

func inode(info os.FileInfo) (uint64, uint64) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Ino), uint64(st.Dev)
	}
	return 0, 0
}

// MarshalJSON keeps the cache file readable by hand.
func (e captureCacheEntry) MarshalJSON() ([]byte, error) {
	type plain captureCacheEntry
	return json.Marshal(plain(e))
}
