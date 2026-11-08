package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// A sweep names a clip only when it replays that clip: the clip exists, has
// one capture, and the sweep's file and window are the clip's.
func TestDirectBackend_CheckSweepClip(t *testing.T) {
	dbWrapped, cleanupDB := setupTestDBWrapped(t)
	defer cleanupDB()
	store := sqlite.NewClipStore(dbWrapped.DB)

	start, duration := 12.0, 30.0
	windowed := &sqlite.Clip{SensorID: "s1", Files: []sqlite.ClipCapture{{PCAPFile: "site/a.pcap"}},
		PCAPStartSecs: &start, PCAPDurationSecs: &duration}
	whole := &sqlite.Clip{SensorID: "s1", Files: []sqlite.ClipCapture{{PCAPFile: "b.pcap"}}}
	multi := &sqlite.Clip{SensorID: "s1", Files: []sqlite.ClipCapture{
		{Ordinal: 0, PCAPFile: "c1.pcap"}, {Ordinal: 1, PCAPFile: "c2.pcap"}}}
	for _, c := range []*sqlite.Clip{windowed, whole, multi} {
		if err := store.InsertClip(c); err != nil {
			t.Fatalf("insert clip: %v", err)
		}
	}

	backend := NewDirectBackend("s1", &Server{db: dbWrapped})
	for _, tc := range []struct {
		name          string
		clipID, file  string
		start, dur    float64
		wantErrSubstr string
	}{
		{"the clip's file and window", windowed.ClipID, "site/a.pcap", 12, 30, ""},
		{"an uncleaned spelling of the clip's file", windowed.ClipID, "site/./a.pcap", 12, 30, ""},
		{"a whole-file clip, duration -1", whole.ClipID, "b.pcap", 0, -1, ""},
		{"a whole-file clip, duration 0", whole.ClipID, "b.pcap", 0, 0, ""},
		{"an unknown clip", "no-such-clip", "b.pcap", 0, -1, "no such clip"},
		{"a multi-file clip", multi.ClipID, "c1.pcap", 0, -1, "spans 2 capture files"},
		{"another capture", windowed.ClipID, "other.pcap", 12, 30, "the clip's capture is"},
		{"another start", windowed.ClipID, "site/a.pcap", 0, 30, "but the clip is 30s from 12s"},
		{"another duration", windowed.ClipID, "site/a.pcap", 12, -1, "but the clip is 30s from 12s"},
		{"a window on a whole-file clip", whole.ClipID, "b.pcap", 5, 10, "but the clip is from 0s to the end"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := backend.CheckSweepClip(tc.clipID, tc.file, tc.start, tc.dur)
			switch {
			case tc.wantErrSubstr == "" && err != nil:
				t.Fatalf("CheckSweepClip: %v, want accepted", err)
			case tc.wantErrSubstr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErrSubstr)):
				t.Fatalf("CheckSweepClip = %v, want an error containing %q", err, tc.wantErrSubstr)
			}
		})
	}
}

// A sweep for an unknown clip is refused when it starts, instead of starting
// and skipping every combination whose run cannot name the clip.
func TestSweepForUnknownClipIsRefusedAtStart(t *testing.T) {
	dbWrapped, cleanupDB := setupTestDBWrapped(t)
	defer cleanupDB()
	runner := sweep.NewRunner(NewDirectBackend("s1", &Server{db: dbWrapped}))

	err := runner.Start(context.Background(), sweep.SweepRequest{
		DataSource: "capture",
		PCAPFile:   "b.pcap",
		ClipID:     "deleted-clip",
		Params:     []sweep.SweepParam{{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01}}},
		Iterations: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "no such clip") {
		t.Fatalf("Start = %v, want refused for the unknown clip", err)
	}
	if status := runner.GetSweepState().Status; status != sweep.SweepStatusIdle {
		t.Fatalf("runner status = %q after a refused start, want idle", status)
	}
}
