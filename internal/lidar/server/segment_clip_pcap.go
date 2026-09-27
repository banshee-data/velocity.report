//go:build pcap

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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

type segmentClipOperations struct {
	mkdirAll    func(string, os.FileMode) error
	mkdirTemp   func(string, string) (string, error)
	removeAll   func(string) error
	readDir     func(string) ([]os.DirEntry, error)
	detectPort  func(string) (int, error)
	run         func(replayeval.Config) (*replayeval.Result, error)
	readFile    func(string) ([]byte, error)
	export      func(annotation.ExportConfig) (*annotation.Pack, error)
	writeRecord func(string, segments.Record) error
}

func (ws *Server) runSegmentClipJob(ctx context.Context, job capjobs.Job, report func(capjobs.Progress)) error {
	return ws.runSegmentClipJobWith(ctx, job, report, segmentClipOperations{os.MkdirAll, os.MkdirTemp, os.RemoveAll, os.ReadDir, network.DetectUDPPort, replayeval.Run, os.ReadFile, annotation.Export, segments.WriteRecord})
}

// adoptClipPack looks for a pack that an earlier attempt of this job finished
// and did not record: the process stopped between writing the pack and
// updating the row. A whole pack is linked; what an attempt left unfinished
// is removed, because it is this job's own and nothing else names it.
func (ws *Server) adoptClipPack(store *sqlite.SegmentStore, jobID, segmentID string, ops segmentClipOperations) (string, error) {
	pack, unfinished, err := ws.findClipPack(jobID, segmentID, ops.readDir)
	if err != nil {
		return "", err
	}
	for _, attempt := range unfinished {
		if err := ops.removeAll(attempt); err != nil {
			return "", err
		}
	}
	if pack == nil {
		return "", nil
	}
	if err := store.LinkPack(jobID, pack.stored, pack.digest); err != nil {
		return "", err
	}
	return pack.dir, nil
}

func (ws *Server) runSegmentClipJobWith(ctx context.Context, job capjobs.Job, report func(capjobs.Progress), ops segmentClipOperations) (err error) {
	clips := sqlite.NewSegmentStore(ws.db)
	clip, selection, err := clips.ClipJob(job.JobID)
	if err != nil {
		return fmt.Errorf("clip job has no selection: %w", err)
	}
	caseID, segmentID := selection.ReplayCaseID, selection.SegmentID
	role, finder := selection.Role, selection.Finder
	if clip.PackDir != "" {
		// A retry of a job whose pack is recorded, present and the one the
		// row names has nothing left to do.
		if digest, whole := segmentPackDigest(ws.segmentPackPath(clip.PackDir), segmentID); whole && digest == clip.PackDigest {
			return nil
		}
	}
	var params segments.Params
	var chosen segments.Window
	if err := json.Unmarshal([]byte(selection.ParametersJSON), &params); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(selection.WindowJSON), &chosen); err != nil {
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
	if chosen.ID != segmentID || chosen.ID != segments.Identity(finder, chosen.Source, role, params, chosen.StartNs) ||
		chosen.Finder != finder || chosen.Role != role ||
		filepath.Clean(chosen.Capture) != filepath.Clean(resolved[0]) ||
		scene.PCAPStartSecs == nil || scene.PCAPDurationSecs == nil ||
		math.Abs(*scene.PCAPStartSecs-chosen.OffsetSeconds) > 0.11 ||
		math.Abs(*scene.PCAPDurationSecs-float64(chosen.EndNs-chosen.StartNs)/1e9) > 0.11 {
		return fmt.Errorf("replay case has drifted from its selected segment")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ops.mkdirAll(ws.annotationPacksDir, 0755); err != nil {
		return err
	}
	adopted, err := ws.adoptClipPack(clips, job.JobID, segmentID, ops)
	if err != nil {
		return fmt.Errorf("recover an earlier attempt: %w", err)
	}
	if adopted != "" {
		report(capjobs.Progress{Current: 3, Total: 3, Detail: "pack ready: " + adopted})
		return nil
	}
	out, err := ops.mkdirTemp(ws.annotationPacksDir, clipAttemptPrefix(job.JobID))
	if err != nil {
		return err
	}
	// A recording with points is large. An attempt that fails or is cancelled
	// leaves nothing the job row points at, so nothing would ever reclaim it;
	// remove it here. A pack exists for the operator only once its job says so.
	defer func() {
		if err != nil {
			_ = ops.removeAll(out)
		}
	}()
	port := ws.udpPort
	if port == 0 {
		port, err = ops.detectPort(resolved[0])
		if err != nil {
			return err
		}
	}
	cfg := replayeval.Config{Context: ctx, OutDir: filepath.Join(out, "vrlog"), TuningFile: config.DefaultConfigPath, SensorID: scene.SensorID, UDPPort: port, IncludePoints: true, RequireSettled: true, ProgressEvery: 200}
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
	result, err := ops.run(cfg)
	if err != nil {
		return fmt.Errorf("replay: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b, err := ops.readFile(filepath.Join(result.VRLOGPath, "replay_manifest.json"))
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
	pack, err := ops.export(annotation.ExportConfig{VRLOGPath: result.VRLOGPath, OutDir: filepath.Join(out, "pack"), StartNs: manifest.ScoringStartNs, EndNs: manifest.ScoringStartNs + int64(manifest.ScoringDurationSeconds*1e9), MaxSamples: 0, Coverage: annotation.CoverageForegroundOnly, CoverageNote: "publisher records foreground points and periodic background snapshots"})
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	record := segments.Record{Schema: "velocity.report/annotation-segment", SchemaVersion: 1, PackDigest: pack.Manifest.PackDigest, Role: role, Finder: finder, FinderVersion: chosen.Version, Parameters: params, Segment: chosen}
	if err := ops.writeRecord(pack.Dir, record); err != nil {
		return err
	}
	stored, err := ws.storedPackDir(pack.Dir)
	if err != nil {
		return err
	}
	if err := clips.LinkPack(job.JobID, stored, pack.Manifest.PackDigest); err != nil {
		return err
	}
	report(capjobs.Progress{Current: 3, Total: 3, Detail: "pack ready: " + pack.Dir})
	return nil
}
