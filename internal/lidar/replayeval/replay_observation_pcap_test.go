//go:build pcap

package replayeval

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func identityReplayCalibration(sensorID string) l4bobserve.Calibration {
	return l4bobserve.Calibration{
		SensorID: sensorID, FromFrame: "sensor", ToFrame: "site",
		Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
	}
}

// Immutable L4 storage is intentionally an observer of the existing tracker,
// not a new measurement path. This proves the replay scorer agrees byte for
// byte with and without it, while also proving that frozen evidence is present
// under the source identity returned by the run.
func TestObservationStoragePreservesReplayBaseline(t *testing.T) {
	pcapPath := requireKirk0(t)
	dir := t.TempDir()
	base := Config{
		PCAPFile: pcapPath, SensorID: "test-replay", UDPPort: 2369,
		// kirk0 starts with a static prefix. Process it as an unrecorded
		// capture-time warm-up so the analysis window has a settled grid and
		// actual foreground clusters, rather than treating the empty prefix as
		// a four-second evidence run.
		StartSeconds: 20, WarmupSeconds: 20, DurationSeconds: 4, RequireSettled: true,
	}
	without := base
	without.OutDir = filepath.Join(dir, "without")
	if _, err := Run(without); err != nil {
		t.Fatalf("replay without observation storage: %v", err)
	}
	with := base
	with.OutDir = filepath.Join(dir, "with")
	with.ObservationDBPath = filepath.Join(dir, "observations.db")
	with.ReplayCaseID = "kirk0-test-case"
	with.ObservationCalibration = identityReplayCalibration(with.SensorID)
	with.ObservationMaxSamplePoints = 64
	result, err := Run(with)
	if err != nil {
		t.Fatalf("replay with observation storage: %v", err)
	}
	if result.ObservationSourceID == "" {
		t.Fatal("stored replay did not report its observation source identity")
	}
	plain, err := os.ReadFile(filepath.Join(without.OutDir, "tracking_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(filepath.Join(with.OutDir, "tracking_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, stored) {
		t.Fatalf("observation storage changed replay baseline:\n%s\n%s", plain, stored)
	}
	database, err := db.NewDB(with.ObservationDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	observations, err := observationsqlite.NewObservationStore(database).ListBySource(result.ObservationSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) == 0 {
		t.Fatal("replay produced no immutable L4 observations")
	}
	for _, observation := range observations {
		record := observation.Snapshot()
		if record.SourceID != result.ObservationSourceID || record.CalibrationID == "" || record.Cluster.FrameID == "" {
			t.Fatalf("incomplete immutable observation: %+v", record)
		}
	}
	// Every online estimate has its track's observation filed beside it at
	// the same frame, carrying the published box the default box model drew.
	store := observationsqlite.NewStateEstimateStore(database)
	boxVersions, err := store.ListBoxVersions()
	if err != nil {
		t.Fatal(err)
	}
	if len(boxVersions) != 1 || boxVersions[0].SourceID != result.ObservationSourceID {
		t.Fatalf("box versions: %+v", boxVersions)
	}
	v := boxVersions[0]
	boxes, err := store.ListVersionBoxes(observationsqlite.EstimateVersionKey{SourceID: v.SourceID,
		EstimatorID: v.EstimatorID, ObservationModelID: v.ObservationModelID, ParamHash: v.ParamHash, Stage: v.Stage})
	if err != nil {
		t.Fatalf("read the boxes filed beside the estimates: %v", err)
	}
	withBox := 0
	for _, b := range boxes {
		if b.HasBox {
			withBox++
		}
	}
	if withBox == 0 || withBox != v.Estimates {
		t.Fatalf("%d of %d estimates carry a box; the version lists %d", withBox, len(boxes), v.Estimates)
	}
	// The L3 region map is in the same evidence: the map as identified and
	// the map the replay ended with, each linked to its grid snapshot. Like
	// the observations, it left the baseline above untouched.
	regionSets, err := database.ListRegionSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(regionSets) != 2 || regionSets[0].SnapshotReason != l3grid.SnapshotReasonRegionsIdentified ||
		regionSets[1].SnapshotReason != l3grid.SnapshotReasonReplayEnd {
		t.Fatalf("region sets: %+v", regionSets)
	}
	for _, set := range regionSets {
		if set.SourcePath != pcapPath || set.RegionCount == 0 {
			t.Fatalf("region set: %+v", set)
		}
	}
	regions, err := database.GetRegionSnapshotByID(regionSets[0].RegionSetID)
	if err != nil {
		t.Fatal(err)
	}
	grid, err := database.GetBgSnapshotByID(regions.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	regionMap, err := l3grid.BuildRegionMap(grid, regions)
	if err != nil {
		t.Fatal(err)
	}
	if regionMap.RegionCount != regionSets[0].RegionCount || len(regionMap.TercileThresholds) != 2 {
		t.Fatalf("identified region map: %d regions, thresholds %v", regionMap.RegionCount, regionMap.TercileThresholds)
	}
	duplicate := with
	duplicate.OutDir = filepath.Join(dir, "duplicate")
	if _, err := Run(duplicate); err == nil || !strings.Contains(err.Error(), "immutable observation") {
		t.Fatalf("replaying into the same immutable source succeeded: %v", err)
	}
	// A failed replay writes no region map.
	if after, err := database.ListRegionSnapshots(); err != nil || len(after) != len(regionSets) {
		t.Fatalf("failed replay changed the region sets: %d, %v", len(after), err)
	}
}
