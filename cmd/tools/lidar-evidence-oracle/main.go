// Command lidar-evidence-oracle creates or verifies the compact semantic
// oracle for an external, immutable LiDAR evidence database.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	_ "modernc.org/sqlite"

	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

type sourceManifest struct {
	Cases []struct {
		ID       string `json:"id"`
		SourceID string `json:"source_id"`
	} `json:"cases"`
}

func main() {
	var (
		dbPath     = flag.String("db", "", "immutable observations SQLite database (required)")
		sourcePath = flag.String("source-manifest", "", "immutable source-PCAP manifest (required)")
		outPath    = flag.String("out", "", "new oracle JSON path; refuses an existing file")
		verifyPath = flag.String("verify", "", "existing oracle JSON to verify instead of writing")
	)
	flag.Parse()
	if *dbPath == "" || *sourcePath == "" {
		fatal(fmt.Errorf("-db and -source-manifest are required"))
	}
	if (*outPath == "") == (*verifyPath == "") {
		fatal(fmt.Errorf("provide exactly one of -out or -verify"))
	}
	sourceDigest, sourceIDs, err := loadSourceManifest(*sourcePath)
	if err != nil {
		fatal(err)
	}
	db, err := observationsqlite.OpenReadOnly(*dbPath)
	if err != nil {
		fatal(fmt.Errorf("open observation database: %w", err))
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	oracle, err := observationsqlite.BuildEvidenceOracle(db, sourceIDs)
	if err != nil {
		fatal(err)
	}
	oracle.SourceManifestSHA256 = sourceDigest
	payload, err := json.MarshalIndent(oracle, "", "  ")
	if err != nil {
		fatal(fmt.Errorf("marshal oracle: %w", err))
	}
	payload = append(payload, '\n')
	if *verifyPath != "" {
		expected, err := os.ReadFile(*verifyPath)
		if err != nil {
			fatal(fmt.Errorf("read oracle %s: %w", *verifyPath, err))
		}
		if string(expected) != string(payload) {
			fatal(fmt.Errorf("semantic oracle differs: %s", *verifyPath))
		}
		fmt.Printf("verified semantic oracle %s (%s)\n", *verifyPath, sha256Text(payload))
		return
	}
	file, err := os.OpenFile(*outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		fatal(fmt.Errorf("create oracle %s: %w", *outPath, err))
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		fatal(fmt.Errorf("write oracle %s: %w", *outPath, err))
	}
	if err := file.Close(); err != nil {
		fatal(fmt.Errorf("close oracle %s: %w", *outPath, err))
	}
	fmt.Printf("wrote semantic oracle %s (%s)\n", *outPath, sha256Text(payload))
}

func loadSourceManifest(path string) (string, map[string]struct{}, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read source manifest: %w", err)
	}
	var manifest sourceManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return "", nil, fmt.Errorf("decode source manifest: %w", err)
	}
	if len(manifest.Cases) == 0 {
		return "", nil, fmt.Errorf("source manifest has no cases")
	}
	ids := make(map[string]struct{}, len(manifest.Cases))
	for _, capture := range manifest.Cases {
		if capture.ID == "" || capture.SourceID == "" {
			return "", nil, fmt.Errorf("source manifest contains incomplete case identity")
		}
		if _, exists := ids[capture.SourceID]; exists {
			return "", nil, fmt.Errorf("source manifest duplicates source ID %q", capture.SourceID)
		}
		ids[capture.SourceID] = struct{}{}
	}
	return sha256Text(payload), ids, nil
}

func sha256Text(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "lidar-evidence-oracle:", err); os.Exit(1) }
