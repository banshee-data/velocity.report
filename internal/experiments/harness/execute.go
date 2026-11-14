package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/banshee-data/velocity.report/internal/experiments"
	"github.com/banshee-data/velocity.report/internal/lidar/jobs/toolargs"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	_ "modernc.org/sqlite"
)

func trim(s string) string            { return strings.TrimSpace(s) }
func jsonBytes(v any) ([]byte, error) { return json.Marshal(v) }
func supported(k string) bool {
	switch k {
	case "state_estimation_baseline", "box_eval", "ground_truth_eval", "corpus_screen":
		return true
	}
	return false
}

type Parameters struct {
	GroundReport  bool     `json:"ground_report"`
	SurfaceGround bool     `json:"surface_ground"`
	LowerBody     bool     `json:"lower_body"`
	KeepEvidence  bool     `json:"keep_evidence"`
	Retention     bool     `json:"retention"`
	SplitRole     string   `json:"split_role"`
	Warmup        *float64 `json:"warmup"`
	Duration      *float64 `json:"duration"`
	Measures      []string `json:"measures"`
}

func params(st experiments.Step) (Parameters, error) {
	var p Parameters
	b, e := json.Marshal(st.Params)
	if e != nil {
		return p, e
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e = d.Decode(&p); e != nil {
		return p, fmt.Errorf("%s params: %w", st.ID, e)
	}
	allowed := map[string]bool{}
	switch st.Kind {
	case "state_estimation_baseline":
		for _, k := range []string{"ground_report", "surface_ground", "keep_evidence", "warmup", "duration"} {
			allowed[k] = true
		}
	case "box_eval":
		for _, k := range []string{"lower_body", "warmup", "duration"} {
			allowed[k] = true
		}
	case "ground_truth_eval":
		for _, k := range []string{"retention", "split_role"} {
			allowed[k] = true
		}
	case "corpus_screen":
		allowed["measures"] = true
	}
	for key := range st.Params {
		if !allowed[key] {
			return p, fmt.Errorf("%s: %s does not accept param %s", st.ID, st.Kind, key)
		}
	}
	for _, v := range []*float64{p.Warmup, p.Duration} {
		if v != nil && (*v < 0 || math.IsNaN(*v) || math.IsInf(*v, 0)) {
			return p, fmt.Errorf("%s: warmup/duration must be finite and nonnegative", st.ID)
		}
	}
	if p.SplitRole != "" && p.SplitRole != "tuning" && p.SplitRole != "held_out" {
		return p, fmt.Errorf("%s: unknown split_role %s", st.ID, p.SplitRole)
	}
	for _, m := range p.Measures {
		if _, ok := screenMeasures[m]; !ok {
			return p, fmt.Errorf("unknown corpus_screen measure %q", m)
		}
	}
	return p, nil
}

// command runs a fixed compiled tool with typed arguments, in a process group.
// Cancellation kills its descendants too; credentials never enter tool env.
func command(ctx context.Context, src, tool, out string, args []string) error {
	log, e := os.OpenFile(filepath.Join(out, "tool.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	defer log.Close()
	c := toolCommand(ctx, src, tool, args)
	c.Stdout = log
	c.Stderr = log

	if e = c.Run(); e != nil {
		return fmt.Errorf("%s: %w; %s", filepath.Base(tool), e, lastLine(log.Name()))
	}
	return nil
}
func toolCommand(ctx context.Context, src, tool string, args []string) *exec.Cmd {
	c := exec.CommandContext(ctx, tool, args...)
	if lock, ok := ctx.Value(lockContextKey{}).(*os.File); ok {
		c.ExtraFiles = []*os.File{lock}
	}
	c.Dir = src
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
	c.WaitDelay = 5 * time.Second
	// The eligibility gate's configuration (cohort manifest, annotation root and
	// use ledger) is how an unattended run is authorised; without it every
	// development replay is refused. See eligibilitycli.
	for _, k := range []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "LD_LIBRARY_PATH",
		"VELOCITY_COHORT_MANIFEST", "VELOCITY_ANNOTATION_ROOT", "VELOCITY_ELIGIBILITY_DB"} {
		if v, ok := os.LookupEnv(k); ok {
			c.Env = append(c.Env, k+"="+v)
		}
	}
	return c
}

func lastLine(p string) string {
	f, e := os.Open(p)
	if e != nil {
		return "log unavailable"
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return "log unavailable"
	}
	if info.Size() > 2048 {
		f.Seek(-2048, io.SeekEnd)
	}
	b, _ := io.ReadAll(f)
	lines := strings.Split(trim(string(b)), "\n")
	s := lines[len(lines)-1]
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

func execute(ctx context.Context, src, bin string, h Host, sp *experiments.Spec, st experiments.Step, u experiments.Unit, out string, recs map[string]experiments.UnitRecord, r *Run, save func() error) (namedMeasures map[string]float64, payload []byte, runErr error) {
	p, e := params(st)
	if e != nil {
		return nil, nil, e
	}
	if st.Kind == "corpus_screen" {
		return screen(ctx, sp, st, u, p, out, recs)
	}
	tuning := ""
	var replay []string
	for _, a := range sp.Arms {
		if a.Name == u.Arm || u.Arm == "" && len(sp.Arms) == 1 {
			actual, e := experiments.TuningFingerprint(src, a.Tuning)
			if e != nil {
				return nil, nil, e
			}
			if actual != a.Fingerprint {
				return nil, nil, fmt.Errorf("pinned tuning changed: %s", a.Tuning)
			}
			tuning = filepath.Join(src, a.Tuning)
			replay = a.Replay
			break
		}
	}
	if tuning == "" && st.Kind != "ground_truth_eval" {
		return nil, nil, fmt.Errorf("%s needs exactly one arm or over: [arms]", u.ID())
	}
	var tool, summary string
	var args []string
	switch st.Kind {
	case "state_estimation_baseline", "box_eval":
		corpus, ok := h.Corpora[sp.CasesName()]
		if !ok {
			return nil, nil, fmt.Errorf("host has no corpus mapping for %s", sp.CasesName())
		}
		h = captureHost(h, corpus)
		caseID := u.Case
		if caseID == "" {
			ids := sp.CaseIDs()
			if len(ids) != 1 {
				return nil, nil, fmt.Errorf("%s needs an explicit single case", u.ID())
			}
			caseID = ids[0]
		}
		captures, e := caseCaptures(src, corpus, h, caseID)
		if e != nil {
			return nil, nil, e
		}
		for _, path := range captures {
			sum, e := hashFile(path)
			if e != nil {
				return nil, nil, e
			}
			if old, ok := r.Sources[path]; ok && old != sum {
				return nil, nil, fmt.Errorf("capture changed since pin: %s", path)
			}
			r.Sources[path] = sum
		}
		if e = save(); e != nil {
			return nil, nil, e
		}
		if e = atomicJSON(filepath.Join(out, "source-manifest.json"), r.Sources); e != nil {
			return nil, nil, e
		}
		args = []string{"-tuning", tuning, "-case", caseID}
		if len(replay) > 0 {
			args = append(args, "-experiment", strings.Join(replay, ","))
		}
		if p.Warmup != nil {
			args = append(args, "-warmup", strconv.FormatFloat(*p.Warmup, 'f', -1, 64))
		}
		if p.Duration != nil {
			args = append(args, "-duration", strconv.FormatFloat(*p.Duration, 'f', -1, 64))
		}
		if st.Kind == "state_estimation_baseline" {
			tool = "lidar-state-estimation-baseline"
			summary = filepath.Join(out, "run", "phase0-summary.json")
			warmup, duration := 70.0, 0.0
			if p.Warmup != nil {
				warmup = *p.Warmup
			}
			if p.Duration != nil {
				duration = *p.Duration
			}
			stage := filepath.Join(h.StageDir, r.ID, filepath.FromSlash(u.ID()), filepath.Base(out))
			if e = os.MkdirAll(stage, 0755); e != nil {
				return nil, nil, e
			}
			defer func() {
				if e := archiveStage(stage, filepath.Join(out, "staging")); e != nil {
					runErr = errors.Join(runErr, e)
				}
			}()

			evidenceDir := filepath.Join(stage, "evidence")
			if p.KeepEvidence {
				evidenceDir = filepath.Join(out, "evidence")
			}
			args = (toolargs.Baseline{Corpus: filepath.Join(src, corpus.Corpus), Index: filepath.Join(src, corpus.Index), CaptureRoot: h.CaptureRoot, Case: caseID, Out: filepath.Join(out, "run"), Tuning: tuning, Duration: duration, Warmup: warmup, EvidenceDir: evidenceDir, SourceManifest: filepath.Join(out, "tool-source-manifest.json")}).Args()
			args = append(args, "-pcap-subdir", h.CaptureSubdir, "-campaign-metrics", "-spurious-clusters", "-evidence-per-case")
			if len(replay) > 0 {
				args = append(args, "-experiment", strings.Join(replay, ","))
			}
			if !p.KeepEvidence {
				args = append(args, "-discard-evidence")
			}

			if p.GroundReport {
				args = append(args, "-ground-report")
			}
			if p.SurfaceGround {
				args = append(args, "-surface-ground")
			}

		} else {
			tool = "lidar-box-eval"
			summary = filepath.Join(out, "box-summary.json")
			args = append(args, "-pcap", strings.Join(captures, ","), "-out", summary, "-evidence-db", filepath.Join(out, "observations.db"))
			if p.LowerBody {
				args = append(args, "-lower-body")
			}
		}
	case "ground_truth_eval":
		ref, ok := h.References[sp.ID+"/"+st.ID+"/"+u.Arm]
		if !ok {
			ref, ok = h.References[sp.ID+"/"+st.ID]
		}
		if !ok {
			return nil, nil, fmt.Errorf("no frozen reference mapping for %s/%s/%s", sp.ID, st.ID, u.Arm)
		}
		for _, path := range []string{ref.Manifest, ref.ADB, ref.BDB} {
			if !filepath.IsAbs(path) {
				return nil, nil, fmt.Errorf("reference paths must be absolute")
			}
			sum, e := hashFile(path)
			if e != nil {
				return nil, nil, e
			}
			if old, ok := r.Sources[path]; ok && old != sum {
				return nil, nil, fmt.Errorf("reference input changed: %s", path)
			}
			r.Sources[path] = sum
		}
		if e = save(); e != nil {
			return nil, nil, e
		}
		tool = "lidar-ground-truth-eval"
		summary = filepath.Join(out, "perframe.json")
		args = []string{"perframe", "-pack", ref.Pack, "-split-manifest", ref.Manifest, "-split", ref.Split, "-a-db", ref.ADB, "-b-db", ref.BDB, "-json", summary}
		if p.SplitRole == "tuning" {
			args = append(args, "-allow-tuning-split")
		}
	default:
		return nil, nil, fmt.Errorf("unsupported executor %s", st.Kind)
	}
	if e = command(ctx, src, filepath.Join(bin, tool), out, args); e != nil {
		return nil, nil, e
	}
	if p.Retention && st.Kind == "ground_truth_eval" {
		ref, ok := h.References[sp.ID+"/"+st.ID+"/"+u.Arm]
		if !ok {
			ref = h.References[sp.ID+"/"+st.ID]
		}
		args = []string{"retention", "-pack", ref.Pack, "-split-manifest", ref.Manifest, "-split", ref.Split, "-db", ref.BDB, "-json", filepath.Join(out, "retention.json")}
		if p.SplitRole == "tuning" {
			args = append(args, "-allow-tuning-split")
		}
		if e = command(ctx, src, filepath.Join(bin, tool), out, args); e != nil {
			return nil, nil, e
		}
	}
	raw, e := os.ReadFile(summary)
	if e != nil {
		return nil, nil, e
	}
	if !json.Valid(raw) {
		return nil, nil, fmt.Errorf("invalid tool summary %s", summary)
	}
	measures, e := headlines(st.Kind, raw)
	if e != nil {
		return nil, nil, e
	}
	if p.Retention && st.Kind == "ground_truth_eval" {
		retentionRaw, e := os.ReadFile(filepath.Join(out, "retention.json"))
		if e != nil {
			return nil, nil, e
		}
		var retention struct {
			Total struct {
				Retention       float64 `json:"retention"`
				FragmentedShare float64 `json:"fragmented_share"`
				MergedShare     float64 `json:"merged_share"`
				Instants        int     `json:"instants"`
			} `json:"total"`
		}
		if e = json.Unmarshal(retentionRaw, &retention); e != nil {
			return nil, nil, e
		}
		if retention.Total.Instants == 0 {
			return nil, nil, fmt.Errorf("retention contains no scored instants")
		}
		measures["retention"] = retention.Total.Retention
		measures["fragmented_share"] = retention.Total.FragmentedShare
		measures["merged_share"] = retention.Total.MergedShare
		raw, e = json.Marshal(struct {
			Perframe  json.RawMessage `json:"perframe"`
			Retention json.RawMessage `json:"retention"`
		}{raw, retentionRaw})
		if e != nil {
			return nil, nil, e
		}
	}
	if e = atomicJSON(filepath.Join(out, "summary.json"), measures); e != nil {
		return nil, nil, e
	}
	return measures, raw, nil
}

func hashFile(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func caseCaptures(src string, c Corpus, h Host, id string) ([]string, error) {
	h = captureHost(h, c)
	var corpus struct {
		Cases []struct {
			ID    string `json:"id"`
			Count int    `json:"expected_capture_count"`
		} `json:"cases"`
	}
	if e := readJSON(filepath.Join(src, c.Corpus), &corpus); e != nil {
		return nil, e
	}
	count := 0
	for _, x := range corpus.Cases {
		if x.ID == id {
			count = x.Count
		}
	}
	if count < 1 {
		return nil, fmt.Errorf("case %s absent from corpus", id)
	}
	var entries []struct {
		ID       string   `json:"id"`
		Captures []string `json:"captures"`
	}
	if e := readJSON(filepath.Join(src, c.Index), &entries); e != nil {
		return nil, e
	}
	var paths []string
	for _, entry := range entries {
		if entry.ID != id {
			continue
		}
		if len(entry.Captures) != count {
			return nil, fmt.Errorf("case %s: corpus expects %d captures, index has %d", id, count, len(entry.Captures))
		}
		for _, p := range entry.Captures {
			base := filepath.Join(h.CaptureRoot, h.CaptureSubdir)
			path := filepath.Join(base, p)
			if filepath.IsAbs(p) || outside(base, path) || path == base {
				return nil, fmt.Errorf("capture path escapes configured root: %s", p)
			}
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("case %s absent from index", id)
	}
	return paths, nil
}

func headlines(kind string, raw []byte) (map[string]float64, error) {
	var data map[string]any
	if e := json.Unmarshal(raw, &data); e != nil {
		return nil, e
	}
	out := map[string]float64{}
	switch kind {
	case "state_estimation_baseline":
		var doc struct {
			Cases []struct {
				Frames     int     `json:"first_run_frames"`
				Duration   float64 `json:"duration_seconds"`
				Continuity struct {
					Confirmed int `json:"tracks_confirmed"`
				} `json:"continuity"`
			} `json:"cases"`
		}
		// Baseline writes an object with cases, never infer missing data as zero.
		if e := json.Unmarshal(raw, &doc); e != nil {
			return nil, e
		}
		if len(doc.Cases) != 1 {
			return nil, fmt.Errorf("baseline summary must contain one case")
		}
		c := doc.Cases[0]
		if c.Frames <= 0 {
			return nil, fmt.Errorf("baseline contains no scored frames")
		}
		out["frames"] = float64(c.Frames)
		out["duration_seconds"] = c.Duration
		out["tracks_confirmed"] = float64(c.Continuity.Confirmed)
	case "box_eval":
		for _, k := range []string{"scored_frames", "confirmed_tracks", "wall_seconds"} {
			v, ok := data[k].(float64)
			if ok {
				out[k] = v
			}
		}
		if _, ok := out["scored_frames"]; !ok {
			return nil, fmt.Errorf("box summary lacks frames")
		}
	case "ground_truth_eval":
		var c struct {
			Total struct {
				B map[string]float64 `json:"b"`
			} `json:"total"`
		}
		if e := json.Unmarshal(raw, &c); e != nil {
			return nil, e
		}
		for _, k := range []string{"hota", "idf1", "mota", "id_switches", "fragmentations"} {
			v, ok := c.Total.B[k]
			if !ok {
				return nil, fmt.Errorf("perframe summary lacks %s", k)
			}
			out[k] = v
		}
	}
	return out, nil
}

// Named screen measures are deliberately explicit; observed frames exclude
// empty frames, and estimated tracks do not imply confirmed vehicles.
var screenMeasures = map[string]string{
	"observed_clusters":        "SELECT COUNT(*) FROM lidar_observations",
	"frames_with_observations": "SELECT COUNT(DISTINCT frame_unix_nanos) FROM lidar_observations",
	"estimated_tracks":         "SELECT COUNT(DISTINCT track_id) FROM lidar_track_estimates WHERE stage = 'online'",
}

func screen(ctx context.Context, sp *experiments.Spec, st experiments.Step, u experiments.Unit, p Parameters, out string, recs map[string]experiments.UnitRecord) (map[string]float64, []byte, error) {
	var input string
	for _, prior := range sp.Steps {
		if prior.ID == st.ID {
			break
		}
		for _, v := range sp.Units(prior) {
			if (v.Arm == u.Arm || v.Arm == "") && (v.Case == u.Case || v.Case == "") && recs[v.ID()].State == experiments.UnitDone && (prior.Kind == "state_estimation_baseline" || prior.Kind == "box_eval") {
				input = recs[v.ID()].Out
			}
		}
	}
	if input == "" {
		return nil, nil, fmt.Errorf("screen has no completed matching replay")
	}
	m := map[string]float64{}
	if len(p.Measures) == 0 {
		path := filepath.Join(input, "run", u.Case, "spurious_clusters.json")
		var d struct {
			Lasting float64 `json:"lasting_tracks"`
			Steady  struct {
				Frames   int     `json:"frames"`
				Complete bool    `json:"complete"`
				Spurious float64 `json:"spurious_per_frame"`
			} `json:"steady"`
		}
		if e := readJSON(path, &d); e != nil {
			return nil, nil, fmt.Errorf("default screen requires baseline spurious-clusters evidence: %w; box screens must name measures", e)
		}
		if !d.Steady.Complete || d.Steady.Frames == 0 {
			return nil, nil, fmt.Errorf("steady screen window has insufficient evidence")
		}
		m["lasting_tracks"] = d.Lasting
		m["steady_spurious_per_frame"] = d.Steady.Spurious
	} else {
		dbpath := filepath.Join(input, "observations.db")
		if _, e := os.Stat(dbpath); e != nil {
			return nil, nil, e
		}
		db, e := sqlite.OpenReadOnly(dbpath)
		if e != nil {
			return nil, nil, e
		}
		defer db.Close()
		for _, name := range p.Measures {
			var v float64
			if e := db.QueryRowContext(ctx, screenMeasures[name]).Scan(&v); e != nil {
				return nil, nil, e
			}
			m[name] = v
		}
	}
	raw, e := json.Marshal(m)
	if e == nil {
		e = atomicJSON(filepath.Join(out, "summary.json"), m)
	}
	return m, raw, e
}
