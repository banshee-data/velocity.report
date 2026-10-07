package server

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

// The Segments page reads two things on every load that are large on disk and
// rarely change: each annotation pack's review sidecar, which runs to tens of
// megabytes, and a replayed run's recording, which is decoded frame by frame
// to rank it. The caches here keep what was read from them against the files
// it was read from, and read again only when one of those files changes.

// settleTime is how long every file behind an entry must have gone unwritten
// before the entry is kept. Some filesystems hold modification times to a
// second or two (HFS+, exFAT on a USB disk), so a file rewritten within the
// same tick at the same size would otherwise keep a stale entry.
const settleTime = 2 * time.Second

// fileStamp is a file as a stat sees it.
type fileStamp struct {
	path   string
	exists bool
	dir    bool
	size   int64
	modNs  int64
}

// stamps describe the files an entry was read from. An entry is reused only
// while they are unchanged: nothing written, created, removed or replaced.
type stamps []fileStamp

func (s stamps) equal(other stamps) bool { return slices.Equal(s, other) }

// settled reports whether every file was last written settleTime before now.
func (s stamps) settled(now time.Time) bool {
	for _, f := range s {
		if f.exists && now.Sub(time.Unix(0, f.modNs)) < settleTime {
			return false
		}
	}
	return true
}

func stampOf(path string) (fileStamp, bool) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fileStamp{path: path}, true
	}
	if err != nil {
		return fileStamp{}, false
	}
	return fileStamp{path: path, exists: true, dir: info.IsDir(), size: info.Size(), modNs: info.ModTime().UnixNano()}, true
}

// stampPaths stamps the named files, absent ones included. It reports false
// when a file cannot be stat'ed, and then nothing may be cached.
func stampPaths(paths ...string) (stamps, bool) {
	out := make(stamps, 0, len(paths))
	for _, path := range paths {
		stamp, ok := stampOf(path)
		if !ok {
			return nil, false
		}
		out = append(out, stamp)
	}
	return out, true
}

// stampTree stamps a directory and everything in it. A tree holding a link
// reports false: the walk does not follow one, so a file changed behind it
// would go unseen.
func stampTree(root string) (stamps, bool) {
	out := stamps{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fs.ErrInvalid
		}
		stamp, ok := stampOf(path)
		if !ok || !stamp.exists {
			return fs.ErrNotExist
		}
		out = append(out, stamp)
		return nil
	})
	return out, err == nil
}

// packListingCache keeps each pack's listing against the files
// readPackListing reads.
type packListingCache struct {
	mu      sync.Mutex
	entries map[string]cachedPackListing
}

type cachedPackListing struct {
	stamps  stamps
	listing packListing
}

// packListingInputs are the files a pack's listing is read from, and the
// proposals directory whose entries it counts. A directory's modification
// time moves whenever an entry is added, removed or renamed.
func packListingInputs(dir string) []string {
	return []string{
		filepath.Join(dir, "manifest.json"),
		filepath.Join(dir, "segment.json"),
		filepath.Join(dir, "annotations.json"),
		filepath.Join(dir, "proposals"),
	}
}

// list returns the listing of each pack directory, in order, reading only the
// packs whose files changed since they were last read. Packs no longer listed
// are forgotten.
func (c *packListingCache) list(dirs []string, now time.Time) []packListing {
	c.mu.Lock()
	previous := c.entries
	c.mu.Unlock()
	kept := make(map[string]cachedPackListing, len(dirs))
	listed := make([]packListing, 0, len(dirs))
	for _, dir := range dirs {
		stamp, ok := stampPaths(packListingInputs(dir)...)
		if entry, found := previous[dir]; ok && found && entry.stamps.equal(stamp) {
			kept[dir] = entry
			listed = append(listed, entry.listing)
			continue
		}
		listing := readPackListing(dir)
		if ok && stamp.settled(now) {
			kept[dir] = cachedPackListing{stamps: stamp, listing: listing}
		}
		listed = append(listed, listing)
	}
	c.mu.Lock()
	c.entries = kept
	c.mu.Unlock()
	return listed
}

// recordingSeriesCache keeps the series last read from a recording. One is
// kept, the run being worked on: the page ranks it again for every selector,
// role and refresh, and for each clip made from it.
type recordingSeriesCache struct {
	mu     sync.Mutex
	path   string
	stamps stamps
	// points are as the recording holds them, in frame order and without
	// the run's split and merge marks, which live in the database and can
	// change while the recording does not.
	points []segments.Point
}

// load reads the recording's series and marks the flagged tracks. The caller
// gets its own copy, since ranking sorts a series in place.
func (c *recordingSeriesCache) load(path string, flagged map[string]bool, now time.Time) ([]segments.Point, error) {
	stamp, ok := stampTree(path)
	var points []segments.Point
	found := false
	if ok {
		c.mu.Lock()
		if c.path == path && c.stamps.equal(stamp) {
			points, found = c.points, true
		}
		c.mu.Unlock()
	}
	if !found {
		read, err := segments.LoadRecording(path, nil)
		if err != nil {
			return nil, err
		}
		points = read
		if ok && stamp.settled(now) {
			c.mu.Lock()
			c.path, c.stamps, c.points = path, stamp, read
			c.mu.Unlock()
		}
	}
	marked := slices.Clone(points)
	for i := range marked {
		marked[i].SplitFlag = flagged[marked[i].Track]
	}
	return marked, nil
}
