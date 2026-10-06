//go:build pcap

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/jobs"
	"github.com/banshee-data/velocity.report/internal/lidar/lidarbench"
)

// benchmarkExecutor runs lidar-bench in-process over the first capture. It
// is the kind that proves the loop with nothing staged: it is on main, and
// its document already carries the identity fields a bundle summary wants.
type benchmarkExecutor struct{}

func (benchmarkExecutor) Run(ctx context.Context, req jobs.JobRequest, env Env) (json.RawMessage, error) {
	if len(env.Captures) != 1 {
		return nil, fmt.Errorf("benchmark replays one capture; the manifest lists %d", len(env.Captures))
	}
	var p jobs.BenchmarkParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
	}
	tuning, err := config.ParseTuningConfig(req.Tuning)
	if err != nil {
		return nil, fmt.Errorf("tuning: %w", err)
	}
	if p.Profile != "" && p.Profile != "full" {
		profile, err := config.ParseProfile(p.Profile)
		if err != nil {
			return nil, err
		}
		if err := tuning.ApplyProfile(profile); err != nil {
			return nil, err
		}
	}
	output := filepath.Join(env.BundleDir, "benchmark.json")
	cfg := lidarbench.Config{
		PCAPFile: env.Captures[0].Path, OutputDir: env.BundleDir, SensorID: req.Replay.SensorID,
		UDPPort: env.Captures[0].Capture.UDPPort, StartSeconds: req.Replay.StartSeconds,
		DurationSeconds: req.Replay.DurationSeconds, Tuning: tuning, BenchmarkOutput: output,
		Repeats: p.Repeats, Quiet: true, ProgressSecs: 0,
	}
	if cfg.DurationSeconds == 0 {
		cfg.DurationSeconds = -1
	}
	env.Log("benchmark %s profile=%q repeats=%d", filepath.Base(cfg.PCAPFile), p.Profile, p.Repeats)
	if code := lidarbench.Run(cfg); code != 0 {
		return nil, fmt.Errorf("lidar-bench exited %d", code)
	}
	summary, err := os.ReadFile(output)
	if err != nil {
		return nil, fmt.Errorf("benchmark wrote no document: %w", err)
	}
	return summary, ctx.Err()
}

func init() { Executors[jobs.KindBenchmark] = benchmarkExecutor{} }
