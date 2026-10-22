package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// A PCAP replay neither restores a region map an earlier run persisted nor
// persists its own, except for a settling pass, which persists the settle its
// recorded pass reloads; returning to live gives the store both uses back.
// Without this the same capture replayed twice against one database ran the
// first replay's regions the second time (l3bg0020).
func TestReplayRegionStoreUse(t *testing.T) {
	for _, tc := range []struct {
		name        string
		settle      bool
		wantPersist bool
	}{{"analysis replay", false, false}, {"settling pass", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			restore := restoreDatasourceHandlerSeams()
			t.Cleanup(restore)

			sensorID := "pcap-region-store-" + map[bool]string{false: "plain", true: "settle"}[tc.settle]
			mgr := l3grid.NewBackgroundManagerDI(sensorID, 2, 4, l3grid.BackgroundParams{}, nil)
			l3grid.RegisterBackgroundManager(sensorID, mgr)
			t.Cleanup(func() { l3grid.RegisterBackgroundManager(sensorID, nil) })

			tmpDir := resolveSymlinks(t, t.TempDir())
			if err := os.WriteFile(filepath.Join(tmpDir, "site.pcap"), testPCAPHeader, 0o644); err != nil {
				t.Fatalf("WriteFile(): %v", err)
			}
			countPCAPPackets = func(string, int) (network.PCAPCountResult, error) {
				return network.PCAPCountResult{Count: 3}, nil
			}
			type use struct{ restore, persist bool }
			var during []use
			readPCAPFile = func(
				_ context.Context, _ string, _ int, _ network.Parser, _ network.FrameBuilder,
				_ network.PacketStatsInterface, _ *network.PacketForwarder, _ float64, _ float64,
				_ uint64, _ uint64, _ func(current, total uint64),
			) error {
				r, p := mgr.RegionStoreUse()
				during = append(during, use{r, p})
				return nil
			}
			prepareSettledPCAPReplay = func(*Server, string, string) error { return nil }

			ws := NewServer(Config{
				Stats:             NewPacketStats(),
				SensorID:          sensorID,
				PCAPSafeDir:       tmpDir,
				UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
			})
			defer stopLiveListenerIfRunning(ws)
			ws.setBaseContext(context.Background())

			if err := ws.startPCAPLockedWithConfig("site.pcap", ReplayConfig{
				SpeedMode:             "analysis",
				AnalysisMode:          true,
				SettleBeforeRecording: tc.settle,
				SensorID:              sensorID,
			}); err != nil {
				t.Fatalf("startPCAPLockedWithConfig() error: %v", err)
			}
			waitForPCAPDone(t, ws)

			if len(during) == 0 {
				t.Fatal("the replay read no packets")
			}
			for i, u := range during {
				if u.restore || u.persist != tc.wantPersist {
					t.Errorf("pass %d: restore=%v persist=%v, want restore=false persist=%v", i+1, u.restore, u.persist, tc.wantPersist)
				}
			}

			if err := ws.ReturnToLive("test"); err != nil {
				t.Fatalf("ReturnToLive(): %v", err)
			}
			if r, p := mgr.RegionStoreUse(); !r || !p {
				t.Errorf("after returning to live: restore=%v persist=%v, want both", r, p)
			}
		})
	}
}
