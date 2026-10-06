package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/jobs"
)

// Env is what an executor is given: where to write, the verified captures,
// and how to reach a tool. Nothing in it came from the request unchecked.
type Env struct {
	// BundleDir is the attempt's bundle directory, created and empty.
	BundleDir string
	// Captures are the manifest's files, found and verified, in order.
	Captures []Resolved
	// CaptureRoot is the worker's capture root, for tools that take one.
	CaptureRoot string
	// Tools stages and builds the kind's tool from the job's commit.
	Tools ToolStager
	// Log writes a line to the attempt's log.
	Log func(format string, args ...any)
	// Report sends advisory progress.
	Report func(jobs.Progress)
}

// Executor runs one kind. It returns the kind's summary, read from the
// bundle, and leaves every file it made under Env.BundleDir.
type Executor interface {
	Run(ctx context.Context, req jobs.JobRequest, env Env) (summary json.RawMessage, err error)
}

// ToolStager makes a named tool from a commit available as an executable.
type ToolStager interface {
	// Stage returns the path of the built tool and the directory of the
	// staged source tree it was built from, which is the working directory
	// the tool runs in: some tools look up config relative to it.
	Stage(ctx context.Context, tool string, code jobs.CodeIdentity, log func(string, ...any)) (bin, srcDir string, err error)
}

// Executors is the kind to executor table. Kinds with no executor on this
// build (a pcap-tagged kind in a build without pcap) are refused at
// submission with a message saying so.
var Executors = map[string]Executor{}

// ToolExecutor runs a cmd/tools program from a staged commit. The only
// things a request contributes to the command line are the typed parameters
// the kind validated; every path is one the worker chose.
type ToolExecutor struct {
	Tool string
	// Args builds the argument list from the request and the environment.
	Args func(req jobs.JobRequest, env Env, tuningPath string) ([]string, error)
	// Summary is the file under BundleDir to read the summary from.
	Summary string
}

// Run stages the tool, writes the tuning document beside the bundle, runs
// the tool with the bundle as its output and the staged tree as its working
// directory, and reads the summary back.
func (t ToolExecutor) Run(ctx context.Context, req jobs.JobRequest, env Env) (json.RawMessage, error) {
	if env.Tools == nil {
		return nil, fmt.Errorf("this worker has no tool stager configured; kind %s needs one", req.Kind)
	}
	bin, srcDir, err := env.Tools.Stage(ctx, t.Tool, req.Code, env.Log)
	if err != nil {
		return nil, fmt.Errorf("stage %s at %s: %w", t.Tool, req.Code.GitSHA[:12], err)
	}
	tuningPath := filepath.Join(env.BundleDir, "tuning.json")
	if err := os.WriteFile(tuningPath, append(canonicalIndent(req.Tuning), '\n'), 0o644); err != nil {
		return nil, err
	}
	args, err := t.Args(req, env, tuningPath)
	if err != nil {
		return nil, err
	}
	env.Log("run %s %s", filepath.Base(bin), strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = srcDir
	stdout, err := os.Create(filepath.Join(env.BundleDir, "tool-stdout.txt"))
	if err != nil {
		return nil, err
	}
	defer stdout.Close()
	stderr, err := os.Create(filepath.Join(env.BundleDir, "tool-stderr.txt"))
	if err != nil {
		return nil, err
	}
	defer stderr.Close()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// The tool gets a clean environment plus what Go and libpcap need. It
	// does not inherit the worker's token.
	cmd.Env = toolEnvironment()
	started := time.Now()
	runErr := cmd.Run()
	env.Log("%s exited after %s: %v", filepath.Base(bin), time.Since(started).Round(time.Second), exitDescription(runErr))
	if runErr != nil {
		return nil, fmt.Errorf("%s: %w (see tool-stderr.txt)", t.Tool, runErr)
	}
	summary, err := os.ReadFile(filepath.Join(env.BundleDir, t.Summary))
	if err != nil {
		return nil, fmt.Errorf("%s finished but wrote no %s: %w", t.Tool, t.Summary, err)
	}
	if !json.Valid(summary) {
		return nil, fmt.Errorf("%s is not valid JSON", t.Summary)
	}
	return summary, nil
}

func exitDescription(err error) string {
	if err == nil {
		return "exit 0"
	}
	return err.Error()
}

func toolEnvironment() []string {
	keep := []string{"PATH", "HOME", "GOPATH", "GOCACHE", "GOFLAGS", "GOTMPDIR", "TMPDIR", "LANG", "LC_ALL", "CGO_ENABLED", "PKG_CONFIG_PATH", "LD_LIBRARY_PATH"}
	var env []string
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

func canonicalIndent(raw json.RawMessage) []byte {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return raw
	}
	return out
}

// The state-estimation baseline, invoked as the 2026-09 campaign's
// run_scorecard_sweep.py did (archived; see its campaign plan). The corpus,
// its index and the case are how that tool names its input; the manifest is
// what this worker verified.
func baselineArgs(req jobs.JobRequest, env Env, tuningPath string) ([]string, error) {
	var p jobs.BaselineParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
	}
	corpus, index, err := writeCorpus(req, env, p.Case)
	if err != nil {
		return nil, err
	}
	args := []string{
		"-corpus", corpus, "-index", index, "-pcap-root", env.CaptureRoot,
		"-case", p.Case, "-out", filepath.Join(env.BundleDir, "run"),
		"-tuning", tuningPath,
		"-duration", strconv.FormatFloat(req.Replay.DurationSeconds, 'f', -1, 64),
		"-warmup", strconv.FormatFloat(req.Replay.WarmupSeconds, 'f', -1, 64),
		"-evidence-dir", filepath.Join(env.BundleDir, "evidence"),
		"-source-manifest", filepath.Join(env.BundleDir, "source-manifest.json"),
	}
	if len(req.Experiments) > 0 {
		args = append(args, "-experiment", strings.Join(req.Experiments, ","))
	}
	if p.SurfaceGround {
		args = append(args, "-surface-ground")
	}
	if req.Replay.MeasurementMode != "" {
		args = append(args, "-measurement-mode", req.Replay.MeasurementMode)
	}
	return args, nil
}

// writeCorpus writes the one-case corpus and index the baseline tool reads,
// from the verified captures: their paths relative to the capture root, in
// manifest order.
func writeCorpus(req jobs.JobRequest, env Env, caseID string) (corpus, index string, err error) {
	rel := make([]string, 0, len(env.Captures))
	for _, c := range env.Captures {
		r, err := filepath.Rel(env.CaptureRoot, c.Path)
		if err != nil {
			return "", "", err
		}
		rel = append(rel, filepath.ToSlash(r))
	}
	corpusDoc := map[string]any{
		"schema_version": 1,
		"cases": []map[string]any{{
			"id": caseID, "source_id": caseID, "expected_capture_count": len(rel),
		}},
	}
	indexDoc := map[string]any{caseID: map[string]any{"captures": rel, "sensor_id": req.CaptureManifest.SensorID}}
	corpus = filepath.Join(env.BundleDir, "corpus.json")
	index = filepath.Join(env.BundleDir, "corpus-index.json")
	if err := writeJSON(corpus, corpusDoc); err != nil {
		return "", "", err
	}
	if err := writeJSON(index, indexDoc); err != nil {
		return "", "", err
	}
	return corpus, index, nil
}

func scorecardArgs(req jobs.JobRequest, env Env, _ string) ([]string, error) {
	var p jobs.ScorecardParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, err
	}
	// The evidence is another attempt's bundle on this worker. The id is
	// checked the way the store checks every id, and the file has to be
	// inside that attempt's bundle.
	if strings.ContainsAny(p.EvidenceJob, "/\\") || strings.Contains(p.EvidenceJob, "..") {
		return nil, fmt.Errorf("evidence_job %q is not an attempt id", p.EvidenceJob)
	}
	evidence := filepath.Join(filepath.Dir(env.BundleDir), "..", p.EvidenceJob, bundleDir, "evidence", "observations.db")
	if _, err := os.Stat(evidence); err != nil {
		return nil, fmt.Errorf("attempt %s has no evidence database on this worker: %w", p.EvidenceJob, err)
	}
	start := p.ScoringStartSeconds
	if start == 0 {
		start = req.Replay.WarmupSeconds
	}
	return []string{
		"-observations", evidence,
		"-scoring-start-seconds", strconv.FormatFloat(start, 'f', -1, 64),
		"-json", filepath.Join(env.BundleDir, "scorecard.json"),
	}, nil
}

func init() {
	Executors[jobs.KindStateEstimationBaseline] = ToolExecutor{
		Tool: "lidar-state-estimation-baseline", Args: baselineArgs, Summary: "phase0-summary.json",
	}
	Executors[jobs.KindTrackScorecard] = ToolExecutor{
		Tool: "lidar-track-scorecard", Args: scorecardArgs, Summary: "scorecard.json",
	}
}

// GitStager stages a commit from a local clone with `git worktree` and
// builds the tool with the Go toolchain, caching both by commit. The clone
// is the worker's, configured at start; a request never names a repository.
type GitStager struct {
	// Repo is a clone of the project on this worker. It is fetched from
	// before a commit it does not have is staged.
	Repo string
	// CacheDir holds staged trees and built binaries, by commit.
	CacheDir string
	// Remote is the remote to fetch a missing commit from, "origin" by default.
	Remote string
	// Timeout bounds a fetch or a build.
	Timeout time.Duration
}

// Stage implements ToolStager.
func (g GitStager) Stage(ctx context.Context, tool string, code jobs.CodeIdentity, log func(string, ...any)) (string, string, error) {
	if strings.ContainsAny(tool, "/\\ ") || strings.Contains(tool, "..") || tool == "" {
		return "", "", fmt.Errorf("tool %q is not a cmd/tools name", tool)
	}
	sha := strings.ToLower(code.GitSHA)
	timeout := g.Timeout
	if timeout == 0 {
		timeout = 20 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	srcDir := filepath.Join(g.CacheDir, "src", sha)
	bin := filepath.Join(g.CacheDir, "bin", sha, tool+"-"+runtime.GOOS+"-"+runtime.GOARCH)
	if _, err := os.Stat(bin); err == nil {
		if _, err := os.Stat(srcDir); err == nil {
			return bin, srcDir, nil
		}
	}

	if _, err := os.Stat(srcDir); err != nil {
		if err := g.git(ctx, log, "cat-file", "-e", sha+"^{commit}"); err != nil {
			remote := g.Remote
			if remote == "" {
				remote = "origin"
			}
			log("commit %s is not in %s; fetching from %s", sha[:12], g.Repo, remote)
			if err := g.git(ctx, log, "fetch", "--quiet", remote, sha); err != nil {
				return "", "", fmt.Errorf("fetch %s: %w", sha[:12], err)
			}
		}
		if err := os.MkdirAll(filepath.Dir(srcDir), 0o755); err != nil {
			return "", "", err
		}
		log("staging %s", sha[:12])
		if err := g.git(ctx, log, "worktree", "add", "--detach", srcDir, sha); err != nil {
			return "", "", fmt.Errorf("stage %s: %w", sha[:12], err)
		}
	}
	// The tool has to exist at that commit; a kind whose tool lives on a
	// branch says so when run from a commit that lacks it.
	if _, err := os.Stat(filepath.Join(srcDir, "cmd", "tools", tool)); err != nil {
		return "", "", fmt.Errorf("commit %s has no cmd/tools/%s", sha[:12], tool)
	}
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return "", "", err
	}
	tags := "pcap"
	if len(code.BuildTags) > 0 {
		tags = strings.Join(code.BuildTags, ",")
	}
	log("building cmd/tools/%s at %s with -tags %s", tool, sha[:12], tags)
	build := exec.CommandContext(ctx, "go", "build", "-tags", tags, "-o", bin, "./cmd/tools/"+tool)
	build.Dir = srcDir
	build.Env = toolEnvironment()
	if out, err := build.CombinedOutput(); err != nil {
		log("%s", strings.TrimSpace(string(out)))
		return "", "", fmt.Errorf("build %s: %w", tool, err)
	}
	return bin, srcDir, nil
}

func (g GitStager) git(ctx context.Context, log func(string, ...any), args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", g.Repo}, args...)...)
	cmd.Env = toolEnvironment()
	out, err := cmd.CombinedOutput()
	if err != nil && log != nil && len(out) > 0 {
		log("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return err
}
