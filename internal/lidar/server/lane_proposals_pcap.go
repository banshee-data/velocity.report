//go:build pcap

package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/lanefieldrun"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func (ws *Server) runLaneProposals(ctx context.Context, job capjobs.Job, report func(capjobs.Progress)) error {
	return ws.runLaneProposalsWith(ctx, job, report, laneJobOps{replay: ws.replayClipHeadless, read: lanefieldrun.FromHeadlessVRLOG})
}

// replayClipHeadless replays a clip through replayeval to a recording with no
// points, under the server's current tuning, as the VRLOG record job does but
// without the replay slot the visualiser and the web share.
func (ws *Server) replayClipHeadless(ctx context.Context, clip *sqlite.Clip, paths []string, report func(string)) (string, string, func(), error) {
	dir, err := os.MkdirTemp("", "lane-proposals-")
	if err != nil {
		return "", "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	tuning, fingerprint, err := ws.writeLaneTuning(dir)
	if err != nil {
		return "", "", cleanup, err
	}
	port := ws.udpPort
	if port == 0 {
		if port, err = network.DetectUDPPort(paths[0]); err != nil {
			return "", "", cleanup, err
		}
	}
	cfg := replayeval.Config{Context: ctx, OutDir: filepath.Join(dir, "vrlog"), TuningFile: tuning, SensorID: clip.SensorID, UDPPort: port, ProgressEvery: 0}
	if len(paths) == 1 {
		cfg.PCAPFile = paths[0]
	} else {
		cfg.PCAPFiles = paths
	}
	if clip.PCAPStartSecs != nil {
		cfg.StartSeconds = *clip.PCAPStartSecs
	}
	if clip.PCAPDurationSecs != nil {
		cfg.DurationSeconds = *clip.PCAPDurationSecs
	}
	cfg.WarmupSeconds = min(cfg.StartSeconds, 35)

	// replayeval reports no progress of its own, so the job's detail says how
	// long the replay has run.
	start := time.Now()
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				report(fmt.Sprintf("replaying the clip: %s", time.Since(start).Round(time.Second)))
			}
		}
	}()
	result, err := replayeval.Run(cfg)
	close(stop)
	if err != nil {
		return "", "", cleanup, err
	}
	return result.VRLOGPath, fingerprint, cleanup, nil
}
