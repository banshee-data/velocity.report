package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func TestReplayL3OverrideSetsItsFiveValuesOnly(t *testing.T) {
	in := l3grid.BackgroundParams{
		BackgroundUpdateFraction:       0.02,
		ClosenessSensitivityMultiplier: 3,
		SafetyMarginMetres:             0.15,
		NoiseRelativeFraction:          0.05,
		NeighbourConfirmationCount:     3,
		SeedFromFirstObservation:       false,
		WarmupMinFrames:                50,
	}
	got := replayL3Override(in)
	want := in
	want.SeedFromFirstObservation = true
	want.ClosenessSensitivityMultiplier = 2.0
	want.NoiseRelativeFraction = 0.02
	want.NeighbourConfirmationCount = 5
	want.SafetyMarginMetres = 0.3
	if got != want {
		t.Fatalf("replayL3Override() = %+v, want %+v", got, want)
	}
}

// A paced replay runs the override's L3 unless KeepTuningL3 is set, and either
// way the tuning file's L3 is back once the replay ends.
func TestStartPCAPLocked_PacedReplayL3(t *testing.T) {
	for _, tc := range []struct {
		name          string
		keep          bool
		wantCloseness float32
	}{
		{"override", false, 2.0},
		{"keep tuning L3", true, 3.0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restore := restoreDatasourceHandlerSeams()
			t.Cleanup(restore)

			sensorID := "pcap-paced-l3-" + map[bool]string{false: "override", true: "keep"}[tc.keep]
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
				Address:           ":0",
				Stats:             NewPacketStats(),
				SensorID:          sensorID,
				PCAPSafeDir:       tmpDir,
				UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
			})
			defer stopLiveListenerIfRunning(ws)
			ws.setBaseContext(context.Background())

			if err := ws.startPCAPLockedWithConfig("paced.pcap", ReplayConfig{
				SpeedMode:    "scaled",
				SpeedRatio:   0.5,
				SensorID:     sensorID,
				KeepTuningL3: tc.keep,
			}); err != nil {
				t.Fatalf("startPCAPLockedWithConfig() error: %v", err)
			}
			waitForPCAPDone(t, ws)

			if during.ClosenessSensitivityMultiplier != tc.wantCloseness {
				t.Errorf("closeness during the replay = %v, want %v", during.ClosenessSensitivityMultiplier, tc.wantCloseness)
			}
			if after := mgr.GetParams(); after.ClosenessSensitivityMultiplier != 3 || after.SafetyMarginMetres != 0.15 {
				t.Errorf("the tuning file's L3 was not restored: %+v", after)
			}
		})
	}
}
