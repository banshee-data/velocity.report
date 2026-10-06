package capindex

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// Extent is a capture file's packet-time bounds, the expensive half of a scan.
type Extent struct {
	FirstPacketNs int64
	LastPacketNs  int64
	PacketCount   uint64
	UDPPort       int
}

// Prober obtains a file's extent. It is an interface point because doing so
// needs libpcap, which the default build does not have, and because probing is
// the only part of indexing that reads whole files.
type Prober func(absPath string, udpPort int) (Extent, error)

// Store is the persistence an Indexer needs. The sqlite CaptureStore satisfies
// it; the narrow shape keeps this package free of any storage dependency.
type Store interface {
	IndexedFiles(rootID string) ([]Indexed, error)
	ApplyScan(rootID string, found []File, drift Drift) error
	MarkScanned(rootID, state, scanErr string) error
	RecordProbe(rootID, relPath string, firstNs, lastNs, count int64, udpPort int) error
	RecordProbeFailure(rootID, relPath, reason string) error
	ProbedFiles(rootID string) ([]Probed, error)
}

// Scan states an Indexer reports. They mirror the store's vocabulary without
// importing it.
const (
	StateOK          = "ok"
	StateUnreachable = "unreachable"
	StateError       = "error"
)

// Result is what one indexing pass did.
type Result struct {
	Drift Drift
	// Probed and ProbeFailed count the files whose extents this pass obtained
	// and failed to obtain.
	Probed      int
	ProbeFailed int
	// State is the scan state recorded against the root.
	State string
	// Err is the reason a scan did not complete, if it did not.
	Err error
}

// Indexer keeps one capture root's index up to date.
type Indexer struct {
	// RootID and RootPath identify the volume.
	RootID   string
	RootPath string
	// Store persists what the passes find.
	Store Store
	// Probe obtains packet extents. A nil Probe indexes metadata only, which is
	// the fast path and all a listing needs.
	Probe Prober
	// UDPPort is passed to the prober. Zero leaves the choice to it.
	UDPPort int
	// OnProgress, when set, is called before each file is probed so a long pass
	// can report where it is.
	OnProgress func(done, total int, relPath string)
}

// Refresh scans the root, records what changed, and probes the files that need
// it: those the scan found new or materially changed, plus any already
// indexed that have never been successfully probed (see stillPending).
//
// Probing is where the time goes — it reads every byte of every new capture —
// so a volume that has not moved since the last look, and was fully probed
// then, costs one walk and no reads on a later pass.
func (ix *Indexer) Refresh(ctx context.Context) (Result, error) {
	var result Result

	// A routine index pass must not read prefixes and suffixes from every
	// multi-hundred-megabyte capture.  That made the Captures page's "Quick
	// scan" outlast the browser connection before it could report anything.
	// Metadata is enough to identify files that require probing; the store
	// retains any previous content tag when this pass does not supply one.
	found, err := ScanMetadata(ix.RootPath)
	if err != nil {
		state := StateError
		if errors.Is(err, ErrRootUnreachable) {
			state = StateUnreachable
		}
		result.State = state
		result.Err = err
		// Recording the failure matters more than the failure itself: an
		// operator needs to see that the volume is not mounted, not an empty
		// list that looks like an empty volume.
		if markErr := ix.Store.MarkScanned(ix.RootID, state, err.Error()); markErr != nil {
			return result, fmt.Errorf("%w (and recording it failed: %v)", err, markErr)
		}
		return result, err
	}

	known, err := ix.Store.IndexedFiles(ix.RootID)
	if err != nil {
		return result, err
	}
	result.Drift = DiffScan(known, found)

	if err := ix.Store.ApplyScan(ix.RootID, found, result.Drift); err != nil {
		return result, err
	}

	if ix.Probe != nil {
		probed, failed, probeErr := ix.probeChanged(ctx, result.Drift, stillPending(known, found))
		result.Probed, result.ProbeFailed = probed, failed
		if probeErr != nil {
			result.State = StateError
			result.Err = probeErr
			_ = ix.Store.MarkScanned(ix.RootID, StateError, probeErr.Error())
			return result, probeErr
		}
	}

	result.State = StateOK
	if err := ix.Store.MarkScanned(ix.RootID, StateOK, ""); err != nil {
		return result, err
	}
	return result, nil
}

// stillPending is the files a scan found no drift for that nonetheless have
// no successful probe on record — a metadata-only pass indexed them, or an
// earlier probe attempt failed, and neither leaves any trace in Drift, whose
// Added/Changed/Missing describe how the filesystem moved, not what the store
// already knows. Left out of probeChanged's target list, such a file would
// stay pending forever: nothing about it ever "changes" again once the
// filesystem stops moving.
func stillPending(known []Indexed, found []File) []string {
	needsProbe := make(map[string]bool, len(known))
	for _, k := range known {
		if k.Present && k.NeedsProbe {
			needsProbe[k.RelPath] = true
		}
	}
	var stale []string
	for _, f := range found {
		if needsProbe[f.RelPath] {
			stale = append(stale, f.RelPath)
		}
	}
	return stale
}

// probeChanged obtains extents for the files a scan found new or materially
// changed, plus any stillPending files this pass rediscovered unchanged. A
// file that cannot be probed records why and does not stop the pass: one
// corrupt capture on a volume of two hundred should not cost the rest.
func (ix *Indexer) probeChanged(ctx context.Context, drift Drift, pending []string) (probed, failed int, err error) {
	seen := make(map[string]bool, len(drift.Added)+len(drift.Changed)+len(pending))
	targets := make([]string, 0, len(drift.Added)+len(drift.Changed)+len(pending))
	add := func(rel string) {
		if !seen[rel] {
			seen[rel] = true
			targets = append(targets, rel)
		}
	}
	for _, a := range drift.Added {
		add(a.RelPath)
	}
	for _, c := range drift.Changed {
		if c.NeedsProbe() {
			add(c.RelPath)
		}
	}
	for _, rel := range pending {
		add(rel)
	}

	for i, rel := range targets {
		if err := ctx.Err(); err != nil {
			return probed, failed, err
		}
		if ix.OnProgress != nil {
			ix.OnProgress(i, len(targets), rel)
		}
		abs := filepath.Join(ix.RootPath, filepath.FromSlash(rel))
		extent, probeErr := ix.Probe(abs, ix.UDPPort)
		if probeErr != nil {
			failed++
			if recErr := ix.Store.RecordProbeFailure(ix.RootID, rel, probeErr.Error()); recErr != nil {
				return probed, failed, recErr
			}
			continue
		}
		if extent.PacketCount == 0 {
			failed++
			if recErr := ix.Store.RecordProbeFailure(ix.RootID, rel,
				"no packets matched the LiDAR port"); recErr != nil {
				return probed, failed, recErr
			}
			continue
		}
		port := extent.UDPPort
		if port == 0 {
			port = ix.UDPPort
		}
		if recErr := ix.Store.RecordProbe(ix.RootID, rel, extent.FirstPacketNs,
			extent.LastPacketNs, int64(extent.PacketCount), port); recErr != nil {
			return probed, failed, recErr
		}
		probed++
	}
	if ix.OnProgress != nil && len(targets) > 0 {
		ix.OnProgress(len(targets), len(targets), "")
	}
	return probed, failed, nil
}

// Summary renders a one-line drift report, the shape an operator reads after a
// scan.
func (d Drift) Summary() string {
	if !d.Any() {
		return fmt.Sprintf("no drift (%d files)", d.Unchanged)
	}
	return fmt.Sprintf("%d new · %d missing · %d changed · %d unchanged",
		len(d.Added), len(d.Missing), len(d.Changed), d.Unchanged)
}

// ExtentDuration is the span an extent covers, for reporting.
func (e Extent) ExtentDuration() time.Duration {
	return time.Duration(e.LastPacketNs - e.FirstPacketNs)
}
