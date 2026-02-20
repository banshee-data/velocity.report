package l3grid

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// serializeGrid compresses the grid cells using gob encoding and gzip compression.
func serializeGrid(cells []BackgroundCell) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(gz)
	if err := enc.Encode(cells); err != nil {
		gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// deserializeGrid decompresses and decodes grid cells from a gob+gzip blob.
func deserializeGrid(blob []byte) ([]BackgroundCell, error) {
	if len(blob) == 0 {
		return nil, fmt.Errorf("empty grid blob")
	}
	gz, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	var cells []BackgroundCell
	dec := gob.NewDecoder(gz)
	if err := dec.Decode(&cells); err != nil {
		return nil, fmt.Errorf("failed to decode grid cells: %w", err)
	}
	return cells, nil
}

// BgStore is an interface required to persist BgSnapshot records. Implemented by lidardb.LidarDB.
type BgStore interface {
	InsertBgSnapshot(s *BgSnapshot) (int64, error)
}

// RegionStore is an optional interface for persisting region snapshots.
// Implementations that support region persistence should implement this interface
// in addition to BgStore.
type RegionStore interface {
	InsertRegionSnapshot(s *RegionSnapshot) (int64, error)
	GetRegionSnapshotBySceneHash(sensorID, sceneHash string) (*RegionSnapshot, error)
	GetRegionSnapshotBySourcePath(sensorID, sourcePath string) (*RegionSnapshot, error)
	GetLatestRegionSnapshot(sensorID string) (*RegionSnapshot, error)
	InsertBgSnapshot(s *BgSnapshot) (int64, error)
	GetBgSnapshotByID(snapshotID int64) (*BgSnapshot, error)
}

// Persist serializes the BackgroundGrid and writes a BgSnapshot via the provided store.
// It updates grid snapshot metadata on success.
// If the store also implements RegionStore and regions have been identified,
// the region data is also persisted with a scene hash for future restoration.
func (bm *BackgroundManager) Persist(store BgStore, reason string) error {
	if bm == nil || bm.Grid == nil || store == nil {
		return nil
	}
	g := bm.Grid

	// Copy cells and snapshot metadata under read lock to avoid racing with
	// concurrent writers in ProcessFramePolar. We only hold the RLock briefly
	// while copying small fields.
	g.mu.RLock()
	cellsCopy := make([]BackgroundCell, len(g.Cells))
	copy(cellsCopy, g.Cells)
	changesSince := g.ChangesSinceSnapshot
	var ringElevCopy []float64
	if len(g.RingElevations) == g.Rings {
		ringElevCopy = make([]float64, len(g.RingElevations))
		copy(ringElevCopy, g.RingElevations)
	}
	g.mu.RUnlock()

	// Serialize and compress grid cells
	blob, err := serializeGrid(cellsCopy)
	if err != nil {
		return err
	}

	snap := &BgSnapshot{
		SensorID:          g.SensorID,
		TakenUnixNanos:    time.Now().UnixNano(),
		Rings:             g.Rings,
		AzimuthBins:       g.AzimuthBins,
		ParamsJSON:        "{}",
		GridBlob:          blob,
		ChangedCellsCount: changesSince,
		SnapshotReason:    reason,
	}

	// If ring elevations were present at the time of copy, serialize the copied slice.
	if len(ringElevCopy) == snap.Rings {
		if b, err := json.Marshal(ringElevCopy); err == nil {
			snap.RingElevationsJSON = string(b)
		}
	}

	id, err := store.InsertBgSnapshot(snap)
	if err != nil {
		return err
	}

	// Persist region data if store supports it and regions have been identified
	if regionStore, ok := store.(RegionStore); ok && g.RegionMgr != nil && g.RegionMgr.IdentificationComplete {
		regionSnap := g.RegionMgr.ToSnapshot(g.SensorID, id)
		if regionSnap != nil {
			// Add scene hash for future matching
			regionSnap.SceneHash = g.SceneSignature()
			if _, err := regionStore.InsertRegionSnapshot(regionSnap); err != nil {
				log.Printf("[BackgroundManager] Failed to persist region snapshot: %v", err)
				// Don't fail the main snapshot persist for region errors
			} else {
				log.Printf("[BackgroundManager] Persisted region snapshot: sensor=%s, regions=%d, scene_hash=%s",
					g.SensorID, regionSnap.RegionCount, regionSnap.SceneHash)
			}
		}
	}

	// Diagnostic logging: count nonzero cells using the copy we made earlier to avoid
	// racing with concurrent ProcessFramePolar writers. cellsCopy was created under RLock.
	nonzero := 0
	for i := range cellsCopy {
		c := cellsCopy[i]
		if c.AverageRangeMeters != 0 || c.RangeSpreadMeters != 0 || c.TimesSeenCount != 0 {
			nonzero++
		}
	}
	percent := 0.0
	if len(cellsCopy) > 0 {
		percent = (float64(nonzero) / float64(len(cellsCopy))) * 100.0
	}
	log.Printf("[BackgroundManager] Persisted snapshot: sensor=%s, reason=%s, nonzero_cells=%d/%d (%.2f%%), grid_blob_size=%d bytes", g.SensorID, reason, nonzero, len(cellsCopy), percent, len(blob))

	// Update grid metadata under write lock. We subtract the value we copied
	// earlier (changesSince) from the current counter so that changes which
	// occurred while we were writing the snapshot are preserved. This avoids
	// losing increments made by ProcessFramePolar between the RLock copy and
	// this write lock.
	g.mu.Lock()
	now := time.Now()
	// compute remaining changes that occurred after the snapshot copy
	if g.ChangesSinceSnapshot >= changesSince {
		g.ChangesSinceSnapshot = g.ChangesSinceSnapshot - changesSince
	} else {
		// defensive: shouldn't happen, but guard against negative counts
		g.ChangesSinceSnapshot = 0
	}
	g.SnapshotID = &id
	g.LastSnapshotTime = now
	bm.LastPersistTime = now
	g.mu.Unlock()
	return nil
}

// RestoreRegions restores the region manager from a previously saved snapshot.
// This allows skipping the settling period when the scene hash matches.
// On success, SettlingComplete is set to true.
// Caller must NOT hold g.mu — this method acquires the lock internally.
func (bm *BackgroundManager) RestoreRegions(snap *RegionSnapshot) error {
	if bm == nil || bm.Grid == nil {
		return fmt.Errorf("background manager or grid nil")
	}
	if snap == nil {
		return fmt.Errorf("nil region snapshot")
	}

	g := bm.Grid
	g.mu.Lock()
	defer g.mu.Unlock()
	return bm.restoreRegionsLocked(snap)
}

// restoreRegionsLocked is the lock-free implementation of RestoreRegions.
// Caller must hold g.mu (write lock).
func (bm *BackgroundManager) restoreRegionsLocked(snap *RegionSnapshot) error {
	g := bm.Grid

	// Create or reset the region manager
	totalCells := g.Rings * g.AzimuthBins
	if g.RegionMgr == nil {
		g.RegionMgr = NewRegionManager(g.Rings, g.AzimuthBins)
	}

	if err := g.RegionMgr.RestoreFromSnapshot(snap, totalCells); err != nil {
		return fmt.Errorf("failed to restore regions: %w", err)
	}

	// Mark settling complete since we restored from a valid snapshot
	g.SettlingComplete = true
	g.WarmupFramesRemaining = 0
	g.regionRestoreAttempted = true

	log.Printf("[BackgroundManager] Regions restored from snapshot: settling_complete=true, regions=%d",
		snap.RegionCount)
	return nil
}

// TryRestoreRegionsBySceneHash attempts to restore regions from the database if the current
// scene signature matches a previously saved region snapshot. This is called early in PCAP
// processing after collecting enough frames to compute a scene signature.
// Also restores the linked grid snapshot if available.
// Caller must NOT hold g.mu — this method acquires the lock internally.
// Returns true if regions were successfully restored.
func (bm *BackgroundManager) TryRestoreRegionsBySceneHash(store RegionStore) bool {
	if bm == nil || bm.Grid == nil || store == nil {
		return false
	}

	// Compute current scene signature
	sceneHash := bm.Grid.SceneSignature()
	if sceneHash == "" {
		log.Printf("[BackgroundManager] Cannot restore regions: scene hash is empty (not enough data)")
		return false
	}

	// Try to find a matching region snapshot
	snap, err := store.GetRegionSnapshotBySceneHash(bm.Grid.SensorID, sceneHash)
	if err != nil {
		log.Printf("[BackgroundManager] Error looking up region snapshot: %v", err)
		return false
	}
	if snap == nil {
		log.Printf("[BackgroundManager] No region snapshot found for scene_hash=%s", sceneHash)
		return false
	}

	// Restore the grid and regions
	g := bm.Grid
	g.mu.Lock()
	err = bm.restoreFromSnapshotLocked(store, snap)
	g.mu.Unlock()

	if err != nil {
		log.Printf("[BackgroundManager] Failed to restore: %v", err)
		return false
	}

	log.Printf("[BackgroundManager] Successfully restored regions from database: scene_hash=%s, regions=%d",
		sceneHash, snap.RegionCount)
	return true
}

// tryRestoreRegionsFromStoreLocked attempts region restoration from the database
// while g.mu is already held. First tries matching by source path (e.g., PCAP filename),
// then falls back to scene hash matching. Returns true if regions were restored and
// settling was skipped.
// Caller must hold g.mu (write lock).
func (bm *BackgroundManager) tryRestoreRegionsFromStoreLocked() bool {
	g := bm.Grid

	// Mark attempted so we only try once per settling cycle
	g.regionRestoreAttempted = true

	regionStore, ok := bm.store.(RegionStore)
	if !ok || regionStore == nil {
		return false
	}

	// Try source path matching first (more reliable for PCAP replay)
	sourcePath := bm.GetSourcePath()
	if sourcePath != "" {
		// Release lock for DB I/O, then re-acquire
		g.mu.Unlock()
		snap, err := regionStore.GetRegionSnapshotBySourcePath(g.SensorID, sourcePath)
		g.mu.Lock()

		if err != nil {
			log.Printf("[BackgroundManager] Error looking up region snapshot by source_path: %v", err)
		} else if snap != nil {
			// Restore the grid and regions (already holding lock)
			if err := bm.restoreFromSnapshotLocked(regionStore, snap); err != nil {
				log.Printf("[BackgroundManager] Failed to restore from DB: %v", err)
				return false
			}
			log.Printf("[BackgroundManager] Restored regions from DB by source_path=%s, skipping remaining settling: regions=%d",
				sourcePath, snap.RegionCount)
			return true
		} else {
			log.Printf("[BackgroundManager] No region snapshot found for source_path=%s, trying scene_hash", sourcePath)
		}
	}

	// Fall back to scene hash matching
	sceneHash := g.sceneSignatureUnlocked()
	if sceneHash == "" {
		return false
	}

	// Release lock for DB I/O, then re-acquire
	g.mu.Unlock()
	snap, err := regionStore.GetRegionSnapshotBySceneHash(g.SensorID, sceneHash)
	g.mu.Lock()

	if err != nil {
		log.Printf("[BackgroundManager] Error looking up region snapshot: %v", err)
		return false
	}
	if snap == nil {
		log.Printf("[BackgroundManager] No region snapshot found for scene_hash=%s", sceneHash)
		return false
	}

	// Restore the grid and regions (already holding lock)
	if err := bm.restoreFromSnapshotLocked(regionStore, snap); err != nil {
		log.Printf("[BackgroundManager] Failed to restore from DB: %v", err)
		return false
	}

	log.Printf("[BackgroundManager] Restored regions from DB, skipping remaining settling: scene_hash=%s, regions=%d",
		sceneHash, snap.RegionCount)
	return true
}

// restoreFromSnapshotLocked restores both grid cells and regions from a snapshot.
// If the region snapshot has a linked grid snapshot (snapshot_id > 0), the grid cells
// are restored first. This ensures foreground extraction has EMA-converged values.
// Caller must hold g.mu (write lock).
func (bm *BackgroundManager) restoreFromSnapshotLocked(regionStore RegionStore, regionSnap *RegionSnapshot) error {
	g := bm.Grid

	// If there's a linked grid snapshot, restore grid cells first
	if regionSnap.SnapshotID > 0 {
		// Release lock for DB I/O
		g.mu.Unlock()
		bgSnap, err := regionStore.GetBgSnapshotByID(regionSnap.SnapshotID)
		g.mu.Lock()

		if err != nil {
			log.Printf("[BackgroundManager] Error fetching linked grid snapshot %d: %v", regionSnap.SnapshotID, err)
			// Continue without grid restoration - regions can still be restored
		} else if bgSnap != nil {
			// Restore grid cells
			cells, err := deserializeGrid(bgSnap.GridBlob)
			if err != nil {
				log.Printf("[BackgroundManager] Error deserializing grid blob: %v", err)
			} else if len(cells) == len(g.Cells) {
				copy(g.Cells, cells)
				// Update nonzero cell count
				nonzero := 0
				for i := range cells {
					if cells[i].AverageRangeMeters != 0 || cells[i].RangeSpreadMeters != 0 || cells[i].TimesSeenCount != 0 {
						nonzero++
					}
				}
				g.nonzeroCellCount = nonzero
				log.Printf("[BackgroundManager] Restored %d grid cells from snapshot_id=%d (nonzero=%d)",
					len(cells), regionSnap.SnapshotID, nonzero)
			} else {
				log.Printf("[BackgroundManager] Grid cell count mismatch: snapshot=%d, current=%d",
					len(cells), len(g.Cells))
			}
		} else {
			log.Printf("[BackgroundManager] Linked grid snapshot %d not found", regionSnap.SnapshotID)
		}
	}

	// Restore regions
	return bm.restoreRegionsLocked(regionSnap)
}

// persistRegionsOnSettleLocked persists regions to the database when settling
// completes naturally (not via restoration). Called while g.mu is held.
// Also persists a linked grid snapshot so grid cells can be restored alongside regions.
// Releases and re-acquires the lock for DB I/O.
func (bm *BackgroundManager) persistRegionsOnSettleLocked() {
	g := bm.Grid
	if bm.store == nil || g.RegionMgr == nil || !g.RegionMgr.IdentificationComplete {
		return
	}
	regionStore, ok := bm.store.(RegionStore)
	if !ok {
		return
	}

	// Copy cells and ring elevations while holding the lock
	cellsCopy := make([]BackgroundCell, len(g.Cells))
	copy(cellsCopy, g.Cells)
	var ringElevCopy []float64
	if len(g.RingElevations) == g.Rings {
		ringElevCopy = make([]float64, len(g.RingElevations))
		copy(ringElevCopy, g.RingElevations)
	}

	// Build region snapshot while holding the lock
	sceneHash := g.sceneSignatureUnlocked()
	regionSnap := g.RegionMgr.ToSnapshot(g.SensorID, 0) // will update snapshot_id after grid insert
	if regionSnap == nil {
		return
	}
	regionSnap.SceneHash = sceneHash
	regionSnap.SourcePath = bm.GetSourcePath()

	// Release lock for DB I/O, then re-acquire
	g.mu.Unlock()
	defer func() { g.mu.Lock() }()

	// Serialize grid cells
	blob, err := serializeGrid(cellsCopy)
	if err != nil {
		log.Printf("[BackgroundManager] Failed to serialize grid for region persist: %v", err)
		return
	}

	// Create and insert grid snapshot
	bgSnap := &BgSnapshot{
		SensorID:          g.SensorID,
		TakenUnixNanos:    time.Now().UnixNano(),
		Rings:             g.Rings,
		AzimuthBins:       g.AzimuthBins,
		ParamsJSON:        "{}",
		GridBlob:          blob,
		ChangedCellsCount: 0,
		SnapshotReason:    "region_settle",
	}
	if len(ringElevCopy) == g.Rings {
		if b, err := json.Marshal(ringElevCopy); err == nil {
			bgSnap.RingElevationsJSON = string(b)
		}
	}

	snapshotID, err := regionStore.InsertBgSnapshot(bgSnap)
	if err != nil {
		log.Printf("[BackgroundManager] Failed to persist grid snapshot for regions: %v", err)
		return
	}

	// Link region snapshot to grid snapshot
	regionSnap.SnapshotID = snapshotID

	if _, err := regionStore.InsertRegionSnapshot(regionSnap); err != nil {
		log.Printf("[BackgroundManager] Failed to persist regions on settle: %v", err)
	} else {
		log.Printf("[BackgroundManager] Persisted regions on settling complete: sensor=%s, regions=%d, scene_hash=%s, source_path=%s, snapshot_id=%d",
			g.SensorID, regionSnap.RegionCount, regionSnap.SceneHash, regionSnap.SourcePath, snapshotID)
	}
}
