package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// caseSummaryFile is written in each case's output directory as soon as the
// case finishes. phase0-summary.json is written only after the last case, so
// without it a case that fails late, or a disk that fills, loses every
// earlier case's summary, and with -discard-evidence the evidence it was
// read from too.
const caseSummaryFile = "case-summary.json"

// caseIdentity is what a case summary was made from, besides the case's own
// captures, which the source manifest holds. -resume reuses a summary only
// when all four match the current run.
type caseIdentity struct {
	// SourceManifestSHA256 is the run's source manifest; empty when the run
	// had none, and then never reused.
	SourceManifestSHA256 string `json:"source_manifest_sha256"`
	// ParametersSHA256 is runParametersSHA256's digest of the flags.
	ParametersSHA256 string   `json:"parameters_sha256"`
	Experiments      []string `json:"experiments"`
	// ExecutableSHA256 is the binary that ran. A summary made by other code
	// is not this run's, whatever its flags: phase0-summary.json records no
	// build, so a mixed one could not be told from a whole one.
	ExecutableSHA256 string `json:"executable_sha256"`
}

// caseRecord is case-summary.json: the case's entry in phase0-summary.json,
// byte for byte once re-encoded, and what it was made from.
type caseRecord struct {
	SchemaVersion int          `json:"schema_version"`
	Identity      caseIdentity `json:"identity"`
	Case          caseSummary  `json:"case"`
}

// placementFlags say where a run reads and writes, or how long it keeps its
// evidence, not what a case summary holds. The corpus, index and captures
// they locate are held to the source manifest instead.
var placementFlags = map[string]bool{
	"corpus": true, "index": true, "pcap-root": true, "pcap-subdir": true, "case": true,
	"out": true, "evidence-dir": true, "observations-db": true,
	"source-manifest": true, "existing-source-manifest": true, "source-manifest-only": true,
	"evidence-per-case": true, "discard-evidence": true, "evidence-profile": true, "cpuprofile-dir": true,
	"resume": true,
}

// contentFlags name a file whose content, not its path, is the parameter,
// and digest it. The tuning file is digested as the replay loads it, from
// config.DefaultConfigPath when none is named and from the embedded defaults
// when the file is absent.
var contentFlags = map[string]func(string) (string, error){
	"tuning": replayParametersSHA256, "uncertainty-calibration": namedFileSHA256,
	"continuity-coverage": namedFileSHA256, "split-manifest": namedFileSHA256,
}

// namedFileSHA256 digests a file a flag names, or nothing when it names none.
func namedFileSHA256(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	return fileSHA256(path)
}

// runParametersSHA256 digests every flag but the placement flags, at its
// default when not given, so a flag added later is part of the identity
// unless it is declared a placement. A file-naming flag contributes its
// file's content. evidence says whether the first run writes an evidence
// database, which the summary's source identity and solid bodies come from.
func runParametersSHA256(flags *flag.FlagSet, evidence bool) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "evidence=%t\n", evidence)
	var err error
	flags.VisitAll(func(f *flag.Flag) {
		if err != nil || placementFlags[f.Name] {
			return
		}
		value := f.Value.String()
		if digest := contentFlags[f.Name]; digest != nil {
			if value, err = digest(value); err != nil {
				err = fmt.Errorf("-%s: %w", f.Name, err)
				return
			}
		}
		fmt.Fprintf(h, "%s=%q\n", f.Name, value)
	})
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// runIdentity is the identity this run's case summaries are written with.
func runIdentity(flags *flag.FlagSet, sourceManifestSHA256 string, experiments []string, evidence bool) (caseIdentity, error) {
	parameters, err := runParametersSHA256(flags, evidence)
	if err != nil {
		return caseIdentity{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return caseIdentity{}, fmt.Errorf("locate executable: %w", err)
	}
	executableSHA256, err := fileSHA256(executable)
	if err != nil {
		return caseIdentity{}, fmt.Errorf("hash executable: %w", err)
	}
	return caseIdentity{SourceManifestSHA256: sourceManifestSHA256, ParametersSHA256: parameters,
		Experiments: experiments, ExecutableSHA256: executableSHA256}, nil
}

// writeCaseSummary writes caseOut's case-summary.json whole or not at all:
// through a synced temporary file beside it, renamed over the name.
func writeCaseSummary(caseOut string, identity caseIdentity, summary caseSummary) (err error) {
	b, err := json.MarshalIndent(caseRecord{SchemaVersion: 1, Identity: identity, Case: summary}, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(caseOut, caseSummaryFile)
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(append(b, '\n')); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// reusableSummary returns the summary an earlier run left in caseOut when it
// was made from what this run would make it from, or else why not.
func reusableSummary(caseOut, caseID string, want caseIdentity) (*caseSummary, string) {
	b, err := os.ReadFile(filepath.Join(caseOut, caseSummaryFile))
	if os.IsNotExist(err) {
		return nil, "no " + caseSummaryFile
	}
	if err != nil {
		return nil, err.Error()
	}
	var record caseRecord
	if err := json.Unmarshal(b, &record); err != nil {
		return nil, fmt.Sprintf("unreadable %s: %v", caseSummaryFile, err)
	}
	got := record.Identity
	switch {
	case record.SchemaVersion != 1:
		return nil, fmt.Sprintf("%s schema version %d", caseSummaryFile, record.SchemaVersion)
	case record.Case.ID != caseID:
		return nil, fmt.Sprintf("a summary of case %q", record.Case.ID)
	case got.SourceManifestSHA256 == "" || got.SourceManifestSHA256 != want.SourceManifestSHA256:
		return nil, "made from another source manifest"
	case got.ParametersSHA256 != want.ParametersSHA256:
		return nil, "made with other parameters"
	case !slices.Equal(got.Experiments, want.Experiments):
		return nil, fmt.Sprintf("made with experiments %v, not %v", got.Experiments, want.Experiments)
	case got.ExecutableSHA256 != want.ExecutableSHA256:
		return nil, "made by another executable"
	}
	return &record.Case, ""
}

// resumable sorts the cases for -resume into those whose earlier summary
// stands, returned for reuse, and those that will replay. A case that will
// replay must have left nothing at its output or evidence paths, because the
// tool never writes over output; anything there, from an interrupted case or
// one summarised under other parameters, is named for the operator to move
// aside before any case replays, not when that case's turn comes. A run that
// wrote phase0-summary.json finished, and is kept as it is.
func (r corpusReplay) resumable(cases []resolvedCorpusCase) (map[string]caseSummary, error) {
	if _, err := os.Lstat(filepath.Join(r.outDir, phase0SummaryFile)); !os.IsNotExist(err) {
		return nil, fmt.Errorf("-resume finishes an interrupted run, and %s holds a finished one's %s", r.outDir, phase0SummaryFile)
	}
	reused := map[string]caseSummary{}
	var blocked []string
	for _, c := range cases {
		id := c.corpusCase.ID
		caseOut := filepath.Join(r.outDir, id)
		summary, why := reusableSummary(caseOut, id, r.identity)
		if summary != nil {
			reused[id] = *summary
			continue
		}
		paths := []string{caseOut}
		if db := r.evidencePath(id); db != "" {
			paths = append(paths, db, db+"-wal", db+"-shm")
		}
		for _, p := range paths {
			if _, err := os.Lstat(p); !os.IsNotExist(err) {
				blocked = append(blocked, fmt.Sprintf("%s (%s)", p, why))
			}
		}
	}
	if len(blocked) > 0 {
		return nil, fmt.Errorf("-resume replays a case only where it left nothing behind; move these aside first:\n  %s",
			strings.Join(blocked, "\n  "))
	}
	return reused, nil
}

// validateResume refuses -resume where a reused summary could not stand in
// for a replay, before any capture is hashed.
func validateResume(sourceManifest, evidence, evidencePerCase, uncertaintyReport, survey, cpuProfile bool) error {
	switch {
	case !sourceManifest:
		return fmt.Errorf("-resume needs -source-manifest or -existing-source-manifest: a summary is reused only for the captures it was made from")
	case evidence && !evidencePerCase:
		return fmt.Errorf("-resume with an evidence output needs -evidence-per-case: a shared database already holds the interrupted case's evidence")
	case uncertaintyReport:
		return fmt.Errorf("-resume does not combine with -uncertainty-report: the pooled report needs every case's samples, which a case summary does not keep")
	case survey:
		return fmt.Errorf("-resume does not combine with -survey-coverage: the declaration set already holds the finished cases")
	case cpuProfile:
		return fmt.Errorf("-resume does not combine with -cpuprofile-dir")
	}
	return nil
}
