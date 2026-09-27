package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// clipAttemptPrefix names the directories a job's attempts are written to.
func clipAttemptPrefix(jobID string) string { return "clip-" + jobID + "-" }

// segmentPackPath turns a stored pack directory, which is relative to the
// annotation packs directory, into the path an operator opens.
func (ws *Server) segmentPackPath(stored string) string {
	return filepath.Join(ws.annotationPacksDir, filepath.FromSlash(stored))
}

// storedPackDir makes a pack directory relative to the annotation packs
// directory, which is how the job row names it. A pack anywhere else is
// refused: the row could not find it again after the directory moved.
func (ws *Server) storedPackDir(dir string) (string, error) {
	rel, err := filepath.Rel(ws.annotationPacksDir, dir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("pack %s is not inside the annotation packs directory", dir)
	}
	return filepath.ToSlash(rel), nil
}

// segmentPackDigest returns the digest of the pack at dir if it is whole and
// was cut for this segment: the pack opens, and its segment record validates,
// is bound to the pack's own digest and names the segment.
func segmentPackDigest(dir, segmentID string) (string, bool) {
	pack, err := annotation.OpenPack(dir)
	if err != nil {
		return "", false
	}
	var rec segments.Record
	b, err := os.ReadFile(filepath.Join(dir, "segment.json"))
	if err != nil || json.Unmarshal(b, &rec) != nil || rec.Validate() != nil ||
		rec.PackDigest != pack.Manifest.PackDigest || rec.Segment.ID != segmentID {
		return "", false
	}
	return pack.Manifest.PackDigest, true
}

// clipPack is a whole pack that an attempt of a clip job left on disk.
type clipPack struct {
	// stored is the pack's directory as the job row names it: relative to
	// the annotation packs directory, with forward slashes.
	stored string
	// dir is the same directory as a path to open.
	dir    string
	digest string
}

// findClipPack returns the whole pack an attempt of this job left in the
// annotation packs directory, and the attempts that left none. An attempt is
// a directory of the packs directory, so its pack's stored name is known
// without asking where the packs directory is.
//
// A job can have left more than one whole pack: before a retry adopted what
// an earlier attempt had finished, it cut the pack again. The one an operator
// has worked in is the one to keep linked, so a pack that holds annotations
// is chosen before one that holds none, and the first in name order among
// equals. A whole pack is never reported as unfinished, whether it was chosen
// or not: what is reported is removed, and a pack may hold a person's review.
func (ws *Server) findClipPack(jobID, segmentID string, readDir func(string) ([]os.DirEntry, error)) (found *clipPack, unfinished []string, err error) {
	entries, err := readDir(ws.annotationPacksDir)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	reviewed := false
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), clipAttemptPrefix(jobID)) {
			continue
		}
		attempt := filepath.Join(ws.annotationPacksDir, entry.Name())
		dir := filepath.Join(attempt, "pack")
		digest, whole := segmentPackDigest(dir, segmentID)
		if !whole {
			unfinished = append(unfinished, attempt)
			continue
		}
		_, statErr := os.Stat(filepath.Join(dir, "annotations.json"))
		holdsReview := statErr == nil
		if found == nil || (holdsReview && !reviewed) {
			found = &clipPack{stored: entry.Name() + "/pack", dir: dir, digest: digest}
			reviewed = holdsReview
		}
	}
	return found, unfinished, nil
}

// relinkSegmentPacks gives a finished clip job its pack back. A job can be
// complete and unlinked because the link was cleared when the schema changed:
// the old row held an absolute path and no digest. The pack is still where
// the job wrote it, named for the job.
//
// It only reads. An attempt that left no whole pack is not removed here; the
// job that made it removes it if it runs again.
func (ws *Server) relinkSegmentPacks() (int, error) {
	if ws.db == nil || ws.annotationPacksDir == "" {
		return 0, nil
	}
	store := sqlite.NewSegmentStore(ws.db)
	jobs, err := store.UnlinkedFinishedClips()
	if err != nil {
		return 0, err
	}
	linked := 0
	for _, job := range jobs {
		pack, _, err := ws.findClipPack(job.JobID, job.SegmentID, os.ReadDir)
		if err != nil {
			return linked, err
		}
		if pack == nil {
			continue
		}
		if err := store.LinkPack(job.JobID, pack.stored, pack.digest); err != nil {
			return linked, err
		}
		linked++
	}
	return linked, nil
}
