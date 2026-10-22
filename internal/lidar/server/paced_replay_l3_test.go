package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// A paced replay runs the tuning file's L3 as written, during the replay and
// after it. Until October 2026 the scaled and realtime paths substituted five
// L3 values for the replay's duration (closeness 2.0 among them), so a paced
// replay ran an L3 its tuning hash did not name; this pins the removal.
func TestStartPCAPLocked_PacedReplayRunsTheTuningFilesL3(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	sensorID := "pcap-paced-l3-unchanged"
	mgr := l3grid.NewBackgroundManagerDI(sensorID, 2, 4, l3grid.BackgroundParams{
		ClosenessSensitivityMultiplier: 3,
		SafetyMarginMetres:             0.15,
		NeighbourConfirmationCount:     3,
	}, nil)
	l3grid.RegisterBackgroundManager(sensorID, mgr)
	t.Cleanup(func() { l3grid.RegisterBackgroundManager(sensorID, nil) })

	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "paced.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	countPCAPPackets = func(string, int) (network.PCAPCountResult, error) {
		return network.PCAPCountResult{Count: 3, FirstTimestampNs: 1, LastTimestampNs: 2}, nil
	}
	var during l3grid.BackgroundParams
	readPCAPFileRealtime = func(
		_ context.Context, _ string, _ int, _ network.Parser, _ network.FrameBuilder,
		_ network.PacketStatsInterface, _ network.RealtimeReplayConfig,
	) error {
		during = l3grid.GetBackgroundManager(sensorID).GetParams()
		return nil
	}

	ws := NewServer(Config{
		Stats:             NewPacketStats(),
		SensorID:          sensorID,
		PCAPSafeDir:       tmpDir,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	})
	defer stopLiveListenerIfRunning(ws)
	ws.setBaseContext(context.Background())

	if err := ws.startPCAPLockedWithConfig("paced.pcap", ReplayConfig{
		SpeedMode:  "scaled",
		SpeedRatio: 0.5,
		SensorID:   sensorID,
	}); err != nil {
		t.Fatalf("startPCAPLockedWithConfig() error: %v", err)
	}
	waitForPCAPDone(t, ws)

	for name, got := range map[string]l3grid.BackgroundParams{"during": during, "after": mgr.GetParams()} {
		if got.ClosenessSensitivityMultiplier != 3 || got.SafetyMarginMetres != 0.15 || got.NeighbourConfirmationCount != 3 {
			t.Errorf("%s the replay the L3 is not the tuning file's: closeness %v, margin %v, neighbours %d",
				name, got.ClosenessSensitivityMultiplier, got.SafetyMarginMetres, got.NeighbourConfirmationCount)
		}
	}
}
