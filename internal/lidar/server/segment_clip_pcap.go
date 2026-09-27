//go:build pcap

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func (ws *Server) runSegmentClipJob(ctx context.Context, job capjobs.Job, report func(capjobs.Progress)) error {
	var caseID, segmentID, storedPack string
	if err := ws.db.QueryRow(`SELECT replay_case_id,segment_id,pack_dir FROM lidar_segment_clip_jobs WHERE job_id=?`, job.JobID).Scan(&caseID, &segmentID, &storedPack); err != nil {
		return fmt.Errorf("clip job has no selection: %w", err)
	}
	if storedPack != "" {
		if pack, err := annotation.OpenPack(storedPack); err == nil {
			var rec segments.Record
			if b, e := os.ReadFile(filepath.Join(storedPack, "segment.json")); e == nil && json.Unmarshal(b, &rec) == nil && rec.PackDigest == pack.Manifest.PackDigest {
				return nil
			}
		}
	}
	var role, finder, paramsJSON, windowJSON string
	if err := ws.db.QueryRow(`SELECT role,finder,parameters_json,window_json FROM lidar_segment_selections WHERE segment_id=? AND replay_case_id=?`, segmentID, caseID).Scan(&role, &finder, &paramsJSON, &windowJSON); err != nil {
		return err
	}
	var params segments.Params
	var chosen segments.Window
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(windowJSON), &chosen); err != nil {
		return err
	}
	store := sqlite.NewReplayCaseStore(ws.db)
	scene, err := store.GetScene(caseID)
	if err != nil {
		return err
	}
	paths, err := store.CasePaths(caseID)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("case has no captures")
	}
	resolved := make([]string, 0, len(paths))
	for _, p := range paths {
		path, err := ws.resolvePCAPPath(p)
		if err != nil {
			return err
		}
		resolved = append(resolved, path)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(ws.annotationPacksDir, 0755); err != nil {
		return err
	}
	out, err := os.MkdirTemp(ws.annotationPacksDir, "clip-"+job.JobID+"-")
	if err != nil {
		return err
	}
	port := ws.udpPort
	if port == 0 {
		port, err = network.DetectUDPPort(resolved[0])
		if err != nil {
			return err
		}
	}
	cfg := replayeval.Config{OutDir: filepath.Join(out, "vrlog"), TuningFile: config.DefaultConfigPath, SensorID: scene.SensorID, UDPPort: port, IncludePoints: true, RequireSettled: true, ProgressEvery: 200}
	if len(resolved) == 1 {
		cfg.PCAPFile = resolved[0]
	} else {
		cfg.PCAPFiles = resolved
	}
	if scene.PCAPStartSecs != nil {
		cfg.StartSeconds = *scene.PCAPStartSecs
	}
	if scene.PCAPDurationSecs != nil {
		cfg.DurationSeconds = *scene.PCAPDurationSecs
	}
	cfg.WarmupSeconds = cfg.StartSeconds
	if cfg.WarmupSeconds > 35 {
		cfg.WarmupSeconds = 35
	}
	report(capjobs.Progress{Current: 0, Total: 3, Detail: "replaying capture with points"})
	result, err := replayeval.Run(cfg)
	if err != nil {
		return fmt.Errorf("replay: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(result.VRLOGPath, "replay_manifest.json"))
	if err != nil {
		return err
	}
	var manifest struct {
		ScoringStartNs         int64   `json:"scoring_start_ns"`
		ScoringDurationSeconds float64 `json:"scoring_duration_seconds"`
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	if manifest.ScoringStartNs <= 0 || manifest.ScoringDurationSeconds <= 0 {
		return fmt.Errorf("replay manifest has no scoring bounds")
	}
	report(capjobs.Progress{Current: 1, Total: 3, Detail: "exporting annotation pack"})
	pack, err := annotation.Export(annotation.ExportConfig{VRLOGPath: result.VRLOGPath, OutDir: filepath.Join(out, "pack"), StartNs: manifest.ScoringStartNs, EndNs: manifest.ScoringStartNs + int64(manifest.ScoringDurationSeconds*1e9), MaxSamples: 0, Coverage: annotation.CoverageForegroundOnly, CoverageNote: "publisher records foreground points and periodic background snapshots"})
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	record := segments.Record{Schema: "velocity.report/annotation-segment", SchemaVersion: 1, PackDigest: pack.Manifest.PackDigest, Role: role, Finder: finder, FinderVersion: chosen.Version, Parameters: params, Segment: chosen}
	if err := segments.WriteRecord(pack.Dir, record); err != nil {
		return err
	}
	if _, err := ws.db.Exec(`UPDATE lidar_segment_clip_jobs SET pack_dir=? WHERE job_id=?`, pack.Dir, job.JobID); err != nil {
		return err
	}
	report(capjobs.Progress{Current: 3, Total: 3, Detail: "pack ready: " + pack.Dir})
	return nil
}
