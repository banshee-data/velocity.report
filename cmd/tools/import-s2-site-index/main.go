// Command import-s2-site-index imports the archive's named, multi-file sites
// as replay cases. It is deliberately an operator tool rather than an HTTP
// endpoint: site-index.json is checked-in archive provenance, not data a web
// client may ask a running recorder to read from an arbitrary path.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

type siteIndexEntry struct {
	ID       string   `json:"id"`
	Where    string   `json:"where"`
	Minutes  float64  `json:"minutes"`
	Captures []string `json:"captures"`
	Lat      float64  `json:"lat"`
	Lon      float64  `json:"lon"`
}

func main() {
	var (
		indexPath = flag.String("index", "tools/s2-archive/site-index.json", "archive site index JSON")
		dbPath    = flag.String("db", "sensor_data.db", "SQLite database to update")
		pcapDir   = flag.String("pcap-subdir", "s2", "capture directory relative to the replay PCAP root")
		sensorID  = flag.String("sensor", "hesai-pandar40p", "sensor identity for imported cases")
		only      = flag.String("only", "", "comma-separated archive case IDs to import (default: all)")
		apply     = flag.Bool("apply", false, "write the import; without this flag, only report the plan")
	)
	flag.Parse()

	entries, err := readIndex(*indexPath)
	if err != nil {
		fatal(err)
	}
	entries, err = selectEntries(entries, *only)
	if err != nil {
		fatal(err)
	}
	if !*apply {
		for _, entry := range entries {
			fmt.Printf("would import %-28s %2d capture(s)  %s\n", entry.ID, len(entry.Captures), entry.Where)
		}
		fmt.Printf("%d replay cases; re-run with -apply to write them\n", len(entries))
		return
	}

	database, err := db.NewDBWithMigrationCheck(*dbPath, false)
	if err != nil {
		fatal(fmt.Errorf("open database: %w", err))
	}
	defer database.Close()
	store := sqlite.NewReplayCaseStore(database)

	created, updated := 0, 0
	for _, entry := range entries {
		files := prefixedPaths(*pcapDir, entry.Captures)
		if len(files) == 0 {
			fatal(fmt.Errorf("%s has no captures", entry.ID))
		}
		sourcePeriodID := "s2-archive:" + entry.ID
		caseID, err := findImportedCase(database, sourcePeriodID)
		if err != nil {
			fatal(err)
		}
		description := fmt.Sprintf("S2 archive: %s (%.1f min across %d captures)", entry.Where, entry.Minutes, len(files))
		wasCreated := caseID == ""
		if wasCreated {
			caseValue := &sqlite.ReplayCase{SensorID: *sensorID, PCAPFile: files[0], Description: description}
			if err := store.InsertScene(caseValue); err != nil {
				fatal(fmt.Errorf("create %s: %w", entry.ID, err))
			}
			caseID = caseValue.ReplayCaseID
			if err := store.SetCaseSession(caseID, "", sourcePeriodID); err != nil {
				fatal(fmt.Errorf("identify %s: %w", entry.ID, err))
			}
			created++
		} else {
			if _, err := database.Exec(`
				UPDATE lidar_replay_cases SET pcap_file = ?, description = ?, updated_at_ns = ?
				 WHERE replay_case_id = ?`, files[0], description, time.Now().UnixNano(), caseID); err != nil {
				fatal(fmt.Errorf("update %s: %w", entry.ID, err))
			}
			updated++
		}

		caseFiles := make([]sqlite.ReplayCaseFile, len(files))
		for i, file := range files {
			caseFiles[i] = sqlite.ReplayCaseFile{Ordinal: i, PCAPFile: file}
		}
		if err := store.SetCaseFiles(caseID, caseFiles); err != nil {
			fatal(fmt.Errorf("set captures for %s: %w", entry.ID, err))
		}
		if _, err := store.SetCaseLocation(caseID, entry.Lat, entry.Lon, sqlite.GeoSourceOperator); err != nil {
			fatal(fmt.Errorf("set location for %s: %w", entry.ID, err))
		}
		verb := "updated"
		if wasCreated {
			verb = "created"
		}
		fmt.Printf("%s  %s  %d capture(s)\n", verb, entry.ID, len(files))
	}
	fmt.Printf("imported %d S2 archive replay cases (%d created, %d updated)\n", len(entries), created, updated)
}

func readIndex(name string) ([]siteIndexEntry, error) {
	bytes, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read site index: %w", err)
	}
	var entries []siteIndexEntry
	if err := json.Unmarshal(bytes, &entries); err != nil {
		return nil, fmt.Errorf("decode site index: %w", err)
	}
	return entries, nil
}

func prefixedPaths(prefix string, captures []string) []string {
	prefix = strings.Trim(prefix, "/")
	paths := make([]string, 0, len(captures))
	for _, capture := range captures {
		if prefix == "" {
			paths = append(paths, capture)
		} else {
			paths = append(paths, path.Join(prefix, capture))
		}
	}
	return paths
}

// selectEntries keeps the archive's order rather than the flag's order. That
// order is the capture catalogue's provenance and is also the order in which
// a selected group is reported to an operator.
func selectEntries(entries []siteIndexEntry, only string) ([]siteIndexEntry, error) {
	if strings.TrimSpace(only) == "" {
		return entries, nil
	}
	wanted := map[string]bool{}
	for _, value := range strings.Split(only, ",") {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		if wanted[id] {
			return nil, fmt.Errorf("archive case %q was requested more than once", id)
		}
		wanted[id] = true
	}
	selected := make([]siteIndexEntry, 0, len(wanted))
	for _, entry := range entries {
		if wanted[entry.ID] {
			selected = append(selected, entry)
			delete(wanted, entry.ID)
		}
	}
	for missing := range wanted {
		return nil, fmt.Errorf("archive case %q is not in the site index", missing)
	}
	return selected, nil
}

func findImportedCase(database *db.DB, sourcePeriodID string) (string, error) {
	var caseID string
	err := database.QueryRow(`SELECT replay_case_id FROM lidar_replay_cases WHERE source_period_id = ?`, sourcePeriodID).Scan(&caseID)
	if err == sqlite.ErrNotFound {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find %s: %w", sourcePeriodID, err)
	}
	return caseID, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "import-s2-site-index:", err)
	os.Exit(1)
}
