// Command vrlog-analyse generates analysis reports from .vrlog recordings.
//
// Usage:
//
//	vrlog-analyse report <path.vrlog>              # generate analysis.json
//	vrlog-analyse compare <a.vrlog> <b.vrlog>      # compare two recordings
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/analysis"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  vrlog-analyse report <path.vrlog>\n")
		fmt.Fprintf(os.Stderr, "  vrlog-analyse compare <a.vrlog> <b.vrlog> [-o output.json]\n")
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "report":
		report, outPath, err := analysis.GenerateReport(os.Args[2])
		if err != nil {
			log.Fatalf("report failed: %v", err)
		}
		_ = report
		log.Printf("Wrote %s", outPath)

	case "compare":
		if len(os.Args) < 4 {
			log.Fatalf("compare requires two .vrlog paths")
		}
		outPath := ""
		if len(os.Args) >= 6 && os.Args[4] == "-o" {
			outPath = os.Args[5]
		}
		comparison, err := analysis.CompareReports(os.Args[2], os.Args[3], outPath)
		if err != nil {
			log.Fatalf("compare failed: %v", err)
		}
		if outPath == "" {
			data, err := json.MarshalIndent(comparison, "", "  ")
			if err != nil {
				log.Fatalf("marshal comparison: %v", err)
			}
			fmt.Println(string(data))
		} else {
			log.Printf("Wrote %s", outPath)
		}

	default:
		log.Fatalf("unknown command: %s (use 'report' or 'compare')", cmd)
	}
}
