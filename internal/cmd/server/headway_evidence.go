package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitycli"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
	"github.com/banshee-data/velocity.report/internal/version"
)

// followingEvidenceBinding is hand-authored context. Source describes the exact
// ordered captures/extractor that formed the persisted observation source.
type followingEvidenceBinding struct {
	Context fieldrun.EvidenceContext `json:"context"`
	Source  l4bobserve.CaptureSource `json:"source"`
}

func runFollowingEvidence(dbPath string, spec fieldrun.Spec, contextPath, dir string, guard *eligibilitycli.CLIFlags, stdout, stderr io.Writer) int {
	err := exportFollowingEvidence(dbPath, spec, contextPath, dir, guard, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: following evidence: %v\n", err)
		return 1
	}
	return 0
}

func exportFollowingEvidence(dbPath string, spec fieldrun.Spec, contextPath, dir string, guard *eligibilitycli.CLIFlags, stdout io.Writer) error {
	f, err := os.Open(contextPath)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var b followingEvidenceBinding
	if err = dec.Decode(&b); err != nil {
		return err
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("binding must contain exactly one JSON object")
	}
	source, err := l4bobserve.SourceID(b.Source)
	if err != nil {
		return err
	}
	if source != spec.SourceID || source != b.Context.SourceID {
		return fmt.Errorf("binding capture identity differs from --source")
	}
	b.Context.SourceManifest = &b.Source
	b.Context.BuildGitSHA = version.GitSHA
	if err = b.Context.Validate(); err != nil {
		return err
	}
	database, err := sqlite.OpenReadOnly(dbPath)
	if err == nil {
		defer database.Close()
		_, err = database.Exec("PRAGMA query_only=ON")
	}
	if err != nil {
		return err
	}
	// Resolve metadata and read all rows within the same read-only snapshot.
	tx, err := sqlite.BeginReadOnly(context.Background(), database)
	var selected sqlite.EstimateVersion
	if err == nil {
		defer tx.Rollback()
		selected, err = fieldrun.SelectVersion(sqlite.ReadSnapshot{SQLTx: tx}, spec)
	}
	if err != nil {
		return err
	}
	spec.ParamHash = selected.ParamHash
	b.Context.BuildGitSHA = version.GitSHA
	req := eligibility.Request{Purpose: eligibility.PurposeDeveloperDiagnostic, ConfigDigest: spec.ParamHash}
	paths := b.Context.CaptureFiles
	if len(paths) == 0 {
		paths = b.Source.CapturePaths
	}
	if len(paths) != len(b.Source.CapturePaths) {
		return fmt.Errorf("capture_files must match the ordered source captures")
	}
	b.Context.CaptureFiles = append([]string(nil), paths...)
	for i, path := range paths {
		id, e := eligibility.NewHasher().Capture(path)
		if e != nil {
			return e
		}
		if strings.TrimPrefix(id.SHA256, "sha256:") != b.Source.CaptureSHA256s[i] {
			return fmt.Errorf("capture hash differs from binding: %s", path)
		}
		req.Inputs = append(req.Inputs, eligibility.Input{Identity: id})
	}
	auth, err := guard.Authorize("velocity report headway --evidence-context", req)
	if err != nil {
		return err
	}
	// Authorize returns nil only after the gated diagnostic use is allowed
	// and its required ledger record has succeeded.
	for _, in := range auth.Inputs {
		if in.Assignment.SiteID != b.Context.SiteID {
			return fmt.Errorf("site binding differs from cohort assignment")
		}
	}
	if b.Context.PolicyID != auth.PolicyDigest {
		return fmt.Errorf("binding policy differs from eligibility decision")
	}
	r, err := fieldrun.Analyse(sqlite.ReadSnapshot{SQLTx: tx}, spec)
	var doc fieldrun.EvidenceDocument
	if err == nil {
		doc, err = fieldrun.Evidence(r, b.Context)
	}
	if err != nil {
		return err
	}
	path, err := fieldrun.WriteEvidence(dir, doc)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Review-only evidence: %s\n", path)
	return err
}
