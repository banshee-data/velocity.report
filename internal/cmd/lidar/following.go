//go:build pcap

package lidar

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/followingreview"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/version"
)

// FollowingMain reduces an explicit corpus of completed, paired recordings.
// Packet replay remains the preparer's responsibility: this command never
// silently selects a source, new parameters, or a different estimator version.
func FollowingMain(args []string) int {
	return followingMain(args, analyseCorpusWithContext)
}

func followingMain(args []string, run func(string, []string, string, bool, string) error) int {
	fs := flag.NewFlagSet("velocity lidar following", flag.ContinueOnError)
	dbPath := fs.String("db", "", "Registered-run database (read only)")
	runs := fs.String("runs", "", "Explicit comma-separated recording run IDs")
	out := fs.String("output", "", "New local output directory (must not exist)")
	scenes := fs.Bool("scenes", false, "Also export each paired scene with exact-sample following distances")
	catalog := fs.String("context-catalog", "", "Site catalogue whose static_parts restrict review metrics and scene measurements; original evidence is unchanged")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *dbPath == "" || *runs == "" || *out == "" || fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "--db, --runs and --output are required")
		return 2
	}
	if err := run(*dbPath, strings.Split(*runs, ","), *out, *scenes, *catalog); err != nil {
		fmt.Fprintln(os.Stderr, "following analysis:", err)
		return 1
	}
	return 0
}

func analyseCorpus(dbPath string, ids []string, out string, scenes bool) error {
	return analyseCorpusWrite(dbPath, ids, out, scenes, os.WriteFile)
}

func analyseCorpusWithContext(dbPath string, ids []string, out string, scenes bool, catalog string) error {
	return analyseCorpusScopedWrite(dbPath, ids, out, scenes, catalog, os.WriteFile)
}

func analyseCorpusWrite(dbPath string, ids []string, out string, scenes bool, write func(string, []byte, os.FileMode) error) error {
	return analyseCorpusScopedWrite(dbPath, ids, out, scenes, "", write)
}

func analyseCorpusScopedWrite(dbPath string, ids []string, out string, scenes bool, catalog string, write func(string, []byte, os.FileMode) error) error {
	var catalogData []byte
	if catalog != "" {
		var err error
		catalogData, err = os.ReadFile(catalog)
		if err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSpace(id) != id || id == "" || seen[id] {
			return fmt.Errorf("empty, duplicate or padded run ID")
		}
		seen[id] = true
	}
	if err := os.Mkdir(out, 0755); err != nil {
		return err
	}
	db, err := sqlite.OpenReadOnly(dbPath)
	if err == nil {
		defer db.Close()
		err = db.Ping()
	}
	if err != nil {
		return err
	}
	var cache followingreview.Cache
	analyses := make([]followingreview.Analysis, 0, len(ids))
	for _, id := range ids {
		run, err := sqlite.NewRunStore(db).GetRun(id)
		if err != nil {
			return err
		}
		session, err := cache.Open(run)
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		var scope *followingreview.ReviewScope
		if catalog != "" {
			scope, err = followingreview.StaticReviewScope(catalogData, session.Metadata.Binding.SiteID, session.Metadata.Start, session.Metadata.End)
			if err != nil {
				return fmt.Errorf("%s review context: %w", id, err)
			}
		}
		a, err := session.AnalyseScoped(context.Background(), scope, nil)
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		analyses = append(analyses, a)
		if scenes {
			if err := exportFollowingScene(session, filepath.Join(out, fmt.Sprintf("recording-%03d", len(analyses)-1)), a); err != nil {
				return err
			}
		}
	}
	// Publish only once every requested recording has succeeded. No partial
	// corpus or failed run can masquerade as a complete site histogram.
	result := struct {
		Schema     string                     `json:"schema"`
		Build      string                     `json:"analysis_tool_git_sha"`
		Recordings []followingreview.Analysis `json:"recordings"`
	}{"following_corpus_v1", version.GitSHA, analyses}
	return writeFollowingCorpusJSON(out, result, write)
}

func writeFollowingCorpusJSON(out string, result any, write func(string, []byte, os.FileMode) error) error {
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return write(filepath.Join(out, "complete.json"), append(b, '\n'), 0644)
}
