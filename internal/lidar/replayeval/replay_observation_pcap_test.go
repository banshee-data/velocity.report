//go:build pcap

package replayeval

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
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
		DurationSeconds: 4,
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
	duplicate := with
	duplicate.OutDir = filepath.Join(dir, "duplicate")
	if _, err := Run(duplicate); err == nil || !strings.Contains(err.Error(), "immutable observation") {
		t.Fatalf("replaying into the same immutable source succeeded: %v", err)
	}
}
