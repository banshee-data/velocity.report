// Package eligibilitycli gives an offline tool the eligibility guard's flags
// and records its decisions in the same use ledger as the server.
package eligibilitycli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/labelworkdb"
)

// CLIFlags are the eligibility flags an offline tool registers, so a
// development run on a workstation is held to the same guard as the server's
// entry points and recorded in the same ledger.
type CLIFlags struct {
	Manifest       *string
	AnnotationRoot *string
	LedgerDB       *string
	Actor          *string
	Measures       *string
}

// Environment variables the flags default from, so an unattended run's
// configuration lives in one place.
const (
	EnvManifest       = "VELOCITY_COHORT_MANIFEST"
	EnvAnnotationRoot = "VELOCITY_ANNOTATION_ROOT"
	EnvLedgerDB       = "VELOCITY_ELIGIBILITY_DB"
)

// RegisterFlags adds -cohort-manifest, -annotation-root, -eligibility-db and
// -actor to fs.
func RegisterFlags(fs *flag.FlagSet) *CLIFlags {
	return &CLIFlags{
		Manifest: fs.String("cohort-manifest", os.Getenv(EnvManifest),
			"site/capture cohort manifest ($"+EnvManifest+"); without one every capture is unassigned and development use is refused"),
		AnnotationRoot: fs.String("annotation-root", os.Getenv(EnvAnnotationRoot),
			"annotation root whose splits/ hold frozen splits ($"+EnvAnnotationRoot+"); required for development use"),
		LedgerDB: fs.String("eligibility-db", os.Getenv(EnvLedgerDB),
			"SQLite database holding the use ledger, migrated to version 70 or later ($"+EnvLedgerDB+"); required for development use"),
		Actor:    fs.String("actor", "", "who is running this, as recorded in the use ledger (default: the login name)"),
		Measures: fs.String("measures", "", "measure families this run reads, comma separated (identity, geometry, ground, settling); empty is every family, the strictest"),
	}
}

// Authorize decides and records a tool's request before it reads a sample.
// The tool names the purpose; the caller never chooses it from input.
func (f *CLIFlags) Authorize(tool string, req eligibility.Request) (eligibility.Decision, error) {
	cfg := eligibility.Config{ManifestPath: *f.Manifest}
	if *f.AnnotationRoot != "" {
		cfg.SplitsDir = filepath.Join(*f.AnnotationRoot, "splits")
	}
	if *f.LedgerDB != "" {
		ledger, closeLedger, err := labelworkdb.OpenUseLedger(*f.LedgerDB)
		if err != nil {
			return eligibility.Decision{}, fmt.Errorf("%w: -eligibility-db: %v", eligibility.ErrNotEligible, err)
		}
		defer closeLedger()
		cfg.Ledger = ledger
	}
	measures, err := eligibility.ParseMeasures(*f.Measures)
	if err != nil {
		return eligibility.Decision{}, fmt.Errorf("%w: %v", eligibility.ErrNotEligible, err)
	}
	req.Measures = measures
	req.EntryPoint = tool
	req.Actor = *f.Actor
	if req.Actor == "" {
		if u, err := user.Current(); err == nil {
			req.Actor = u.Username
		}
	}
	host, _ := os.Hostname()
	req.Actor = fmt.Sprintf("cli:%s@%s", req.Actor, host)
	d, err := eligibility.New(cfg).Authorize(req)
	if err != nil && !errors.Is(err, eligibility.ErrNotEligible) {
		return d, fmt.Errorf("%w: %v", eligibility.ErrNotEligible, err)
	}
	return d, err
}
