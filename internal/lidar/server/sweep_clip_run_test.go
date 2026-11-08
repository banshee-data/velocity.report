package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
)

// A sweep replay writes its run record exactly as a clip replay does: naming
// the clip when the sweep was started for one, and leaving it null otherwise.
func TestDirectBackend_SweepReplayRunRecordsClip(t *testing.T) {
	for _, tc := range []struct {
		name     string
		withClip bool
	}{
		{"for a clip", true},
		{"without a clip", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sensorID := "test-sweep-clip-run"
			cleanupBg := setupTestBackgroundManager(t, sensorID)
			defer cleanupBg()

			tmpDir := resolveSymlinks(t, t.TempDir())
			pcapHeader := []byte{
				0xd4, 0xc3, 0xb2, 0xa1, 0x02, 0x00, 0x04, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0xff, 0xff, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
			}
			if err := os.WriteFile(filepath.Join(tmpDir, "sweep.pcap"), pcapHeader, 0o644); err != nil {
				t.Fatal(err)
			}

			dbWrapped, cleanupDB := setupTestDBWrapped(t)
			defer cleanupDB()

			clipID := ""
			if tc.withClip {
				clip := &sqlite.Clip{SensorID: sensorID, Files: []sqlite.ClipCapture{{PCAPFile: "sweep.pcap"}}}
				if err := sqlite.NewClipStore(dbWrapped.DB).InsertClip(clip); err != nil {
					t.Fatalf("insert clip: %v", err)
				}
				clipID = clip.ClipID
			}

			ws := NewServer(Config{
				Stats:             NewPacketStats(),
				SensorID:          sensorID,
				PCAPSafeDir:       tmpDir,
				DB:                dbWrapped,
				UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
			})
			ws.setBaseContext(context.Background())

			backend := NewDirectBackend(sensorID, ws)
			err := backend.StartPCAPReplayWithConfig(sweep.PCAPReplayConfig{
				PCAPFile:         "sweep.pcap",
				MaxRetries:       1,
				AnalysisMode:     true,
				DisableRecording: true,
				ClipID:           clipID,
			})
			if err != nil {
				t.Fatalf("StartPCAPReplayWithConfig: %v", err)
			}
			if err := backend.WaitForPCAPComplete(10 * time.Second); err != nil {
				t.Fatalf("WaitForPCAPComplete: %v", err)
			}

			runID := ws.LastRunID()
			if runID == "" {
				t.Fatal("the analysis replay started no run")
			}
			run, err := sqlite.NewRunStore(dbWrapped.DB).GetRun(runID)
			if err != nil {
				t.Fatalf("GetRun(%s): %v", runID, err)
			}
			if run.ClipID != clipID {
				t.Fatalf("run %s clip = %q, want %q", runID, run.ClipID, clipID)
			}

			// The column itself, which the Runs page and the visualiser read.
			var stored *string
			if err := dbWrapped.DB.QueryRow(
				`SELECT clip_id FROM lidar_runs WHERE run_id = ?`, runID,
			).Scan(&stored); err != nil {
				t.Fatalf("read clip_id: %v", err)
			}
			switch {
			case tc.withClip && (stored == nil || *stored != clipID):
				t.Fatalf("clip_id = %v, want %q", stored, clipID)
			case !tc.withClip && stored != nil:
				t.Fatalf("clip_id = %q, want null", *stored)
			}
		})
	}
}
