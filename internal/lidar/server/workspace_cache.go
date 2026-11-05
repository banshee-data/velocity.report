package server

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
	"github.com/banshee-data/velocity.report/internal/lidar/labelwork"
)

// The workspace reads every label set's state and every pack's physical
// references. Both run to megabytes, and both change only when someone
// saves, so each is kept against the files it was read from (as the pack
// listings are, segment_read_cache.go) and read again only when one changes.

// labelSetSummary is what the workspace keeps of one label set.
type labelSetSummary struct {
	Dir         string
	ID          string
	Name        string
	Revision    int
	StateDigest string
	AppliesTo   []labelset.Binding
	Progress    labelwork.LabelSetProgress
	Error       string
}

// physicalSummary is what the workspace keeps of one pack's physical
// references.
type physicalSummary struct {
	Progress labelwork.PhysicalProgress
	Error    string
}

type workspaceCache struct {
	mu       sync.Mutex
	sets     map[string]cachedSet
	physical map[string]cachedPhysical
}

type cachedSet struct {
	stamps  stamps
	summary labelSetSummary
}

type cachedPhysical struct {
	stamps  stamps
	summary physicalSummary
}

// labelSet reads the label set at dir, or returns the copy read when its
// manifest was last as it is now. Every save rewrites labels.json.
func (c *workspaceCache) labelSet(dir string, now time.Time) labelSetSummary {
	stamp, ok := stampPaths(filepath.Join(dir, labelset.ManifestFile), filepath.Join(dir, labelset.JournalFile))
	c.mu.Lock()
	entry, found := c.sets[dir]
	c.mu.Unlock()
	if ok && found && entry.stamps.equal(stamp) {
		return entry.summary
	}
	sum := labelSetSummary{Dir: dir}
	ls, err := labelset.Open(dir)
	if err != nil {
		sum.Error = err.Error()
	} else {
		sum.ID, sum.Name, sum.Revision, sum.StateDigest = ls.Manifest.LabelSetID, ls.Manifest.Name, ls.Revision(), ls.StateDigest()
		sum.AppliesTo = ls.Manifest.AppliesTo
		sum.Progress = labelwork.MembershipProgress(&ls.State)
	}
	if ok && stamp.settled(now) {
		c.mu.Lock()
		if c.sets == nil {
			c.sets = map[string]cachedSet{}
		}
		c.sets[dir] = cachedSet{stamps: stamp, summary: sum}
		c.mu.Unlock()
	}
	return sum
}

// physicalOf reads a pack's physical references. Their staleness depends on
// the label set's membership, so the label set's manifest is an input too.
func (c *workspaceCache) physicalOf(packDir, labelSetDir string, reviewedObjects int, now time.Time) physicalSummary {
	inputs := []string{filepath.Join(packDir, annotation.PhysicalReferencesFile), filepath.Join(packDir, "manifest.json")}
	if labelSetDir != "" {
		inputs = append(inputs, filepath.Join(labelSetDir, labelset.ManifestFile))
	}
	stamp, ok := stampPaths(inputs...)
	c.mu.Lock()
	entry, found := c.physical[packDir]
	c.mu.Unlock()
	if ok && found && entry.stamps.equal(stamp) {
		return entry.summary
	}
	var sum physicalSummary
	pack, err := annotation.OpenPack(packDir)
	if err != nil {
		sum.Error = err.Error()
	} else if !stamp[0].exists {
		sum.Progress = labelwork.PhysicalReferenceProgress(nil, reviewedObjects)
	} else if doc, err := annotation.LoadPhysicalReferences(pack); err != nil {
		sum.Error = err.Error()
	} else {
		sum.Progress = labelwork.PhysicalReferenceProgress(doc, reviewedObjects)
	}
	if ok && stamp.settled(now) {
		c.mu.Lock()
		if c.physical == nil {
			c.physical = map[string]cachedPhysical{}
		}
		c.physical[packDir] = cachedPhysical{stamps: stamp, summary: sum}
		c.mu.Unlock()
	}
	return sum
}
