package server

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/report/headway"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

// runHeadway implements "velocity report headway": the headway report of
// observed following exposure (behaviour plan Section 10.4), in one of two
// modes.
//
//	velocity report headway --oracle [--output DIR] [--paper letter|a4]
//	velocity report headway --db EVIDENCE.db --source ID [--stage final|fixed_lag|online]
//	    [--estimator ID] [--obs-model ID] [--param-hash HASH] [--output DIR] [--paper letter|a4]
//
// --oracle renders the synthetic oracle from the analytic encounter
// scenarios. --db runs the provisional field slice over one version of the
// persisted estimates in an evidence database: it stores the encounters it
// derives there, write-once, and renders the report labelled provisional.
func runHeadway(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("velocity report headway", flag.ContinueOnError)
	fs.SetOutput(stderr)
	oracle := fs.Bool("oracle", false,
		"Render the synthetic oracle from the analytic l8behaviour encounter scenarios")
	dbPath := fs.String("db", "", "Evidence database holding persisted estimates; the provisional field report "+
		"(its derived encounters are stored in it)")
	source := fs.String("source", "", "With --db: the estimates' source id (source/v1/...)")
	stage := fs.String("stage", l8behaviour.StageFinal.String(), "With --db: estimate stage, final, fixed_lag or online")
	estimator := fs.String("estimator", "", "With --db: estimator id, when the source holds several versions at the stage")
	obsModel := fs.String("obs-model", "", "With --db: observation model id, when the source holds several versions at the stage")
	paramHash := fs.String("param-hash", "", "With --db: estimator parameter hash, when the source holds several versions at the stage")
	outputDir := fs.String("output", "", "Output directory (default: the current directory)")
	paper := fs.String("paper", string(chart.PaperLetter), "Paper size: letter or a4")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "error: unexpected argument %q\n", fs.Arg(0))
		fs.Usage()
		return 2
	}
	if *oracle == (*dbPath != "") {
		fmt.Fprintln(stderr, "error: give exactly one of --oracle (the synthetic oracle) or --db with --source "+
			"(the provisional field report)")
		fs.Usage()
		return 2
	}
	if *oracle {
		for _, f := range []string{"source", "estimator", "obs-model", "param-hash"} {
			if v := fs.Lookup(f).Value.String(); v != "" {
				fmt.Fprintf(stderr, "error: --%s applies to --db, not --oracle\n", f)
				return 2
			}
		}
	}
	size := chart.PaperSize(*paper)
	if size != chart.PaperLetter && size != chart.PaperA4 {
		fmt.Fprintf(stderr, "error: --paper must be %s or %s, got %q\n", chart.PaperLetter, chart.PaperA4, *paper)
		return 2
	}
	dir, err := normalizePDFOutputDir(*outputDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to resolve output directory: %v\n", err)
		return 1
	}

	var r headway.Report
	if *oracle {
		if r, err = headway.Oracle(); err != nil {
			fmt.Fprintf(stderr, "error: build the oracle report: %v\n", err)
			return 1
		}
	} else {
		spec := fieldrun.Spec{SourceID: *source, EstimatorID: *estimator, ObsModelID: *obsModel, ParamHash: *paramHash}
		if spec.Stage, err = l8behaviour.ParseEstimateStage(*stage); err != nil {
			fmt.Fprintf(stderr, "error: --stage: %v; want final, fixed_lag or online\n", err)
			return 2
		}
		var code int
		if r, code = runFieldReport(*dbPath, spec, stdout, stderr); code != 0 {
			return code
		}
	}

	res, err := headway.Generate(r, headway.Options{Paper: size, OutputDir: dir})
	if err != nil {
		fmt.Fprintf(stderr, "error: report generation failed: %v\n", err)
		return 1
	}
	if *oracle {
		// The field run printed its status with its figures, before
		// rendering, so they survive a failed render.
		fmt.Fprintf(stdout, "Status: %s\n", r.StatusLabel)
	}
	fmt.Fprintf(stdout, "PDF: %s\n", res.PDFPath)
	fmt.Fprintf(stdout, "ZIP: %s\n", res.ZIPPath)
	return 0
}

// runFieldReport opens an existing evidence database, performs the field run
// and builds its provisional report, printing the status and the run's
// figures. A missing --source lists the sources the database holds.
func runFieldReport(path string, spec fieldrun.Spec, stdout, stderr io.Writer) (headway.Report, int) {
	// db.NewDB would create a fresh database at a mistyped path.
	if _, err := os.Stat(path); err != nil {
		fmt.Fprintf(stderr, "error: --db: %v\n", err)
		return headway.Report{}, 1
	}
	database, err := db.NewDB(path)
	if err != nil {
		fmt.Fprintf(stderr, "error: open %s: %v\n", path, err)
		return headway.Report{}, 1
	}
	defer database.Close()

	if spec.SourceID == "" {
		fmt.Fprintln(stderr, "error: --source is required with --db")
		versions, err := sqlite.NewStateEstimateStore(database).ListEstimateVersions()
		if err != nil {
			fmt.Fprintf(stderr, "error: list estimate versions: %v\n", err)
			return headway.Report{}, 1
		}
		stages := map[string]map[string]bool{}
		for _, v := range versions {
			if stages[v.SourceID] == nil {
				stages[v.SourceID] = map[string]bool{}
			}
			stages[v.SourceID][v.Stage] = true
		}
		if len(stages) == 0 {
			fmt.Fprintf(stderr, "%s holds no persisted estimates\n", path)
		}
		sources := make([]string, 0, len(stages))
		for s := range stages {
			sources = append(sources, s)
		}
		sort.Strings(sources)
		for _, s := range sources {
			var at []string
			for _, st := range l8behaviour.EstimateStages() {
				if stages[s][st.String()] {
					at = append(at, st.String())
				}
			}
			fmt.Fprintf(stderr, "  %s (stages: %v)\n", s, at)
		}
		return headway.Report{}, 2
	}

	run, err := fieldrun.Run(database, spec)
	if err != nil {
		fmt.Fprintf(stderr, "error: field run: %v\n", err)
		return headway.Report{}, 1
	}
	r, err := fieldrun.Report(run)
	if err != nil {
		fmt.Fprintf(stderr, "error: build the provisional report: %v\n", err)
		return headway.Report{}, 1
	}
	fmt.Fprintf(stdout, "Status: %s\n", r.StatusLabel)
	e := run.Estimates
	fmt.Fprintf(stdout, "Estimates: %s %s, %s, %s (%d rows)\n", e.Stage, e.EstimatorID, e.ObservationModelID, e.ParamHash, e.Estimates)
	for _, line := range run.Summary().Lines() {
		fmt.Fprintln(stdout, line)
	}
	return r, 0
}
