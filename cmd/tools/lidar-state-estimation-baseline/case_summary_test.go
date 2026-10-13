package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// enospc is the error that ended the 2026-09-28 corpus run on its second case.
var enospc = errors.New("close recording: failed to write header: no space left on device")

// stubCorpus is the two cases that run met, in its order, replayed by a
// stand-in. A first run with evidence makes the database the loop would
// discard; a case named in fail then fails, as columbus-broadway did, with
// its output already started.
type stubCorpus struct {
	replay   corpusReplay
	cases    []resolvedCorpusCase
	fail     map[string]error
	replayed []string
}

func newStubCorpus(t *testing.T, evidence bool) *stubCorpus {
	t.Helper()
	dir := t.TempDir()
	s := &stubCorpus{fail: map[string]error{}}
	for _, id := range []string{"marina-webster-beach", "columbus-broadway"} {
		s.cases = append(s.cases, resolvedCorpusCase{corpusCase: corpusCase{ID: id, ExpectedCaptureCount: 1},
			paths: []string{filepath.Join(dir, "pcaps", id+".pcapng")}})
	}
	s.replay = corpusReplay{
		base: replayeval.Config{SensorID: "hesai-pandar40p", UDPPort: 2369, StartSeconds: 70, WarmupSeconds: 70,
			RequireSettled: true, MeasurementSourceMode: "medoid_v0", Experiments: []string{}},
		outDir: filepath.Join(dir, "out"), samplePoints: 256,
		identity: caseIdentity{SourceManifestSHA256: "sha256:source", ParametersSHA256: "sha256:parameters",
			Experiments: []string{}, ExecutableSHA256: "sha256:executable"},
		prepare: func([]string, int) (*capseq.Sequence, error) { return nil, nil },
		replay:  s.run,
	}
	if evidence {
		s.replay.evidenceDir = filepath.Join(dir, "evidence")
		s.replay.observationDBPath = filepath.Join(s.replay.evidenceDir, "observations.db")
		s.replay.evidencePerCase, s.replay.discardEvidence = true, true
		if err := os.MkdirAll(s.replay.evidenceDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(s.replay.outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s *stubCorpus) run(cfg replayeval.Config) (*replayeval.Result, error) {
	caseID := filepath.Base(filepath.Dir(cfg.OutDir))
	s.replayed = append(s.replayed, caseID+"/"+filepath.Base(cfg.OutDir))
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return nil, err
	}
	result := &replayeval.Result{FramesRecorded: 100 * len(caseID)}
	if cfg.ObservationDBPath != "" {
		if err := os.WriteFile(cfg.ObservationDBPath, []byte("evidence"), 0o644); err != nil {
			return nil, err
		}
		result.ObservationSourceID = "source:" + caseID
	}
	if err := s.fail[caseID]; err != nil {
		return nil, err
	}
	return result, os.WriteFile(filepath.Join(cfg.OutDir, "tracking_baseline.json"), []byte(`{"case":"`+caseID+`"}`), 0o644)
}

func (s *stubCorpus) caseOut(caseID string) string { return filepath.Join(s.replay.outDir, caseID) }

func (s *stubCorpus) evidence(caseID string) string { return s.replay.evidencePath(caseID) }

func readCaseRecord(t *testing.T, caseOut string) caseRecord {
	t.Helper()
	var record caseRecord
	readJSON(t, filepath.Join(caseOut, caseSummaryFile), &record)
	return record
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return !os.IsNotExist(err)
}

// The 2026-09-28 run: marina-webster-beach finished both arms, then
// columbus-broadway failed on a full disk. marina's summary must survive on
// disk, though its evidence was discarded and phase0-summary.json never came.
func TestCaseSummaryOutlivesALaterCaseFailure(t *testing.T) {
	s := newStubCorpus(t, true)
	s.fail["columbus-broadway"] = enospc
	if _, _, err := s.replay.run(s.cases); err == nil || !strings.Contains(err.Error(), "first run columbus-broadway") {
		t.Fatalf("run error %v, want columbus-broadway's first run to fail", err)
	}
	marina := s.caseOut("marina-webster-beach")
	record := readCaseRecord(t, marina)
	if !reflect.DeepEqual(record.Identity, s.replay.identity) {
		t.Errorf("identity %+v, want %+v", record.Identity, s.replay.identity)
	}
	c := record.Case
	if c.ID != "marina-webster-beach" || !c.BaselineEqual || c.FirstRunFrames != 2000 || c.RepeatRunFrames != 2000 ||
		c.ObservationSourceID != "source:marina-webster-beach" {
		t.Errorf("marina's summary %+v", c)
	}
	if exists(s.evidence("marina-webster-beach")) {
		t.Error("-discard-evidence kept marina's database")
	}
	if exists(filepath.Join(marina, caseSummaryFile+".tmp")) {
		t.Error("the temporary summary was left behind")
	}
	if exists(filepath.Join(s.caseOut("columbus-broadway"), caseSummaryFile)) {
		t.Error("the failed case has a summary")
	}
}

// A summary that cannot be written keeps the evidence it was read from, and
// stops the run before the next case.
func TestCaseSummaryIsWrittenBeforeItsEvidenceIsDiscarded(t *testing.T) {
	s := newStubCorpus(t, true)
	marina := s.caseOut("marina-webster-beach")
	// A directory where the summary goes refuses the rename over it.
	if err := os.MkdirAll(filepath.Join(marina, caseSummaryFile, "occupied"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.replay.run(s.cases); err == nil || !strings.Contains(err.Error(), "case summary marina-webster-beach") {
		t.Fatalf("run error %v, want marina's summary to fail", err)
	}
	if !exists(s.evidence("marina-webster-beach")) {
		t.Error("marina's evidence was discarded although its summary was not written")
	}
	if exists(filepath.Join(marina, caseSummaryFile+".tmp")) {
		t.Error("the temporary summary was left behind")
	}
	if want := []string{"marina-webster-beach/first", "marina-webster-beach/repeat"}; !slices.Equal(s.replayed, want) {
		t.Errorf("replayed %v, want %v", s.replayed, want)
	}
}

// The default path's phase0-summary.json is the document it always was:
// schema version, source manifest, split and cases, nothing added, with
// each case as its case-summary.json holds it.
func TestPhase0SummaryIsTheCaseSummaries(t *testing.T) {
	s := newStubCorpus(t, false)
	summaries, _, err := s.replay.run(s.cases)
	if err != nil {
		t.Fatal(err)
	}
	split := &splitRecord{Digest: "sha256:split", Revision: 1, FileSHA256: "sha256:file"}
	if err := writePhase0Summary(s.replay.outDir, "sha256:source", split, summaries); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(s.replay.outDir, phase0SummaryFile))
	if err != nil {
		t.Fatal(err)
	}
	var cases []caseSummary
	for _, c := range s.cases {
		cases = append(cases, readCaseRecord(t, s.caseOut(c.corpusCase.ID)).Case)
	}
	want, err := json.MarshalIndent(struct {
		SchemaVersion        int           `json:"schema_version"`
		SourceManifestSHA256 string        `json:"source_manifest_sha256,omitempty"`
		Split                *splitRecord  `json:"split,omitempty"`
		Cases                []caseSummary `json:"cases"`
	}{SchemaVersion: 1, SourceManifestSHA256: "sha256:source", Split: split, Cases: cases}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, append(want, '\n')) {
		t.Fatalf("phase0-summary.json\n%s\nwant\n%s", got, want)
	}
}

// Resumed after the failure, the run refuses the interrupted case's leavings
// rather than write over them; once they are moved aside it replays that
// case alone and writes the phase0-summary.json an uninterrupted run does.
func TestResumeReplaysOnlyTheCaseThatDidNotFinish(t *testing.T) {
	whole := newStubCorpus(t, true)
	summaries, _, err := whole.replay.run(whole.cases)
	if err != nil {
		t.Fatal(err)
	}
	if err := writePhase0Summary(whole.replay.outDir, "sha256:source", nil, summaries); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(whole.replay.outDir, phase0SummaryFile))
	if err != nil {
		t.Fatal(err)
	}

	s := newStubCorpus(t, true)
	s.fail["columbus-broadway"] = enospc
	if _, _, err := s.replay.run(s.cases); err == nil {
		t.Fatal("the stand-in did not fail")
	}
	delete(s.fail, "columbus-broadway")
	s.replay.resume, s.replayed = true, nil
	columbus, columbusDB := s.caseOut("columbus-broadway"), s.evidence("columbus-broadway")
	_, _, err = s.replay.run(s.cases)
	if err == nil || !strings.Contains(err.Error(), columbus+" (no "+caseSummaryFile+")") || !strings.Contains(err.Error(), columbusDB) {
		t.Fatalf("resume error %v, want columbus-broadway's output and database named", err)
	}
	if len(s.replayed) != 0 {
		t.Fatalf("replayed %v before refusing", s.replayed)
	}

	if err := os.RemoveAll(columbus); err != nil {
		t.Fatal(err)
	}
	if err := removeDatabase(columbusDB); err != nil {
		t.Fatal(err)
	}
	summaries, _, err = s.replay.run(s.cases)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"columbus-broadway/first", "columbus-broadway/repeat"}; !slices.Equal(s.replayed, want) {
		t.Fatalf("replayed %v, want %v", s.replayed, want)
	}
	if err := writePhase0Summary(s.replay.outDir, "sha256:source", nil, summaries); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(s.replay.outDir, phase0SummaryFile))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("resumed phase0-summary.json\n%s\nuninterrupted\n%s", got, want)
	}
}

// A run that wrote phase0-summary.json finished: -resume leaves it alone.
func TestResumeRefusesAFinishedRun(t *testing.T) {
	s := newStubCorpus(t, false)
	summaries, _, err := s.replay.run(s.cases)
	if err != nil {
		t.Fatal(err)
	}
	if err := writePhase0Summary(s.replay.outDir, "sha256:source", nil, summaries); err != nil {
		t.Fatal(err)
	}
	s.replay.resume, s.replayed = true, nil
	if _, _, err := s.replay.run(s.cases); err == nil || !strings.Contains(err.Error(), phase0SummaryFile) {
		t.Fatalf("resume error %v, want the finished run refused", err)
	}
	if len(s.replayed) != 0 {
		t.Fatalf("replayed %v", s.replayed)
	}
}

// A summary made from other captures, parameters, experiments or code is not
// reused: the case is refused until moved aside, then replays as any other.
func TestResumeDoesNotReuseAMismatchedSummary(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(*caseIdentity)
		why    string
	}{
		{"source manifest", func(id *caseIdentity) { id.SourceManifestSHA256 = "sha256:other" }, "made from another source manifest"},
		{"parameters", func(id *caseIdentity) { id.ParametersSHA256 = "sha256:other" }, "made with other parameters"},
		{"experiments", func(id *caseIdentity) { id.Experiments = []string{replayeval.ExperimentSolidBody} }, "made with experiments [], not [solid_body]"},
		{"executable", func(id *caseIdentity) { id.ExecutableSHA256 = "sha256:other" }, "made by another executable"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := newStubCorpus(t, false)
			if _, _, err := s.replay.run(s.cases); err != nil {
				t.Fatal(err)
			}
			c.change(&s.replay.identity)
			s.replay.resume, s.replayed = true, nil
			_, _, err := s.replay.run(s.cases)
			for _, cs := range s.cases {
				if want := s.caseOut(cs.corpusCase.ID) + " (" + c.why + ")"; err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("resume error %v, want %q", err, want)
				}
			}
			if len(s.replayed) != 0 {
				t.Fatalf("replayed %v before refusing", s.replayed)
			}
			for _, cs := range s.cases {
				if err := os.RemoveAll(s.caseOut(cs.corpusCase.ID)); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := s.replay.run(s.cases); err != nil {
				t.Fatal(err)
			}
			if len(s.replayed) != 4 {
				t.Fatalf("replayed %v, want both cases' two runs", s.replayed)
			}
			if got := readCaseRecord(t, s.caseOut("marina-webster-beach")).Identity; !reflect.DeepEqual(got, s.replay.identity) {
				t.Fatalf("replayed summary's identity %+v, want %+v", got, s.replay.identity)
			}
		})
	}
}

// Committed tuning files, relative to this package.
const (
	tuningDefaults   = "../../../config/tuning.defaults.json"
	tuningExperiment = "../../../config/experiments/height-floor-road.json"
)

// Where a run reads and writes leaves the parameters digest alone; a
// parameter, a named file's content, or the evidence output moves it.
func TestRunParametersSHA256(t *testing.T) {
	dir := t.TempDir()
	defaults, err := os.ReadFile(tuningDefaults)
	if err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(dir, "tuning.json")
	coverage := filepath.Join(dir, "coverage.json")
	for path, content := range map[string][]byte{copied: defaults, coverage: []byte("{}\n")} {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	digest := func(evidence bool, args ...string) (string, error) {
		t.Helper()
		fs := flag.NewFlagSet("lidar-state-estimation-baseline", flag.ContinueOnError)
		fs.String("out", "", "")
		fs.String("tuning", "", "")
		fs.String("continuity-coverage", "", "")
		fs.Float64("warmup", 70, "")
		fs.Bool("resume", false, "")
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		return runParametersSHA256(fs, evidence)
	}
	must := func(d string, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	base := must(digest(true, "-out", "/srv/one", "-tuning", tuningDefaults))
	for name, got := range map[string]string{
		"another -out and -resume":         must(digest(true, "-out", "/srv/two", "-tuning", tuningDefaults, "-resume")),
		"the same tuning at another path":  must(digest(true, "-tuning", copied)),
		"an absent tuning file (embedded)": must(digest(true, "-tuning", filepath.Join(dir, "absent.json"))),
	} {
		if got != base {
			t.Errorf("%s moved the digest", name)
		}
	}
	for name, got := range map[string]string{
		"other tuning":       must(digest(true, "-tuning", tuningExperiment)),
		"-warmup":            must(digest(true, "-tuning", tuningDefaults, "-warmup", "20")),
		"a coverage file":    must(digest(true, "-tuning", tuningDefaults, "-continuity-coverage", coverage)),
		"no evidence output": must(digest(false, "-tuning", tuningDefaults)),
	} {
		if got == base {
			t.Errorf("%s did not move the digest", name)
		}
	}
	if _, err := digest(true, "-continuity-coverage", filepath.Join(dir, "absent.json")); err == nil ||
		!strings.Contains(err.Error(), "-continuity-coverage") {
		t.Errorf("an absent -continuity-coverage file gave %v", err)
	}
}

func TestValidateResume(t *testing.T) {
	for _, c := range []struct {
		name                                                               string
		sourceManifest, evidence, perCase, uncertainty, survey, cpuProfile bool
		want                                                               string
	}{
		{"no evidence", true, false, false, false, false, false, ""},
		{"per-case evidence", true, true, true, false, false, false, ""},
		{"no source manifest", false, false, false, false, false, false, "-source-manifest"},
		{"shared evidence database", true, true, false, false, false, false, "-evidence-per-case"},
		{"pooled uncertainty", true, false, false, true, false, false, "-uncertainty-report"},
		{"coverage survey", true, true, true, false, true, false, "-survey-coverage"},
		{"cpu profiles", true, false, false, false, false, true, "-cpuprofile-dir"},
	} {
		err := validateResume(c.sourceManifest, c.evidence, c.perCase, c.uncertainty, c.survey, c.cpuProfile)
		if c.want == "" && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: error %v, want one naming %s", c.name, err, c.want)
		}
	}
}
