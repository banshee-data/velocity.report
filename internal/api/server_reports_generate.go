package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/report"
	"github.com/banshee-data/velocity.report/internal/security"
	"github.com/banshee-data/velocity.report/internal/units"
)

// ReportRequest represents the JSON payload for report generation
type ReportRequest struct {
	SiteID            *int    `json:"site_id"`            // Optional: use site configuration
	StartDate         string  `json:"start_date"`         // YYYY-MM-DD format
	EndDate           string  `json:"end_date"`           // YYYY-MM-DD format
	CompareStart      string  `json:"compare_start_date"` // Optional: comparison start date (YYYY-MM-DD)
	CompareEnd        string  `json:"compare_end_date"`   // Optional: comparison end date (YYYY-MM-DD)
	Timezone          string  `json:"timezone"`           // e.g., "US/Pacific"
	Units             string  `json:"units"`              // "mph" or "kph"
	Group             string  `json:"group"`              // e.g., "1h", "4h"
	Source            string  `json:"source"`             // "radar_objects", "radar_data", or "radar_data_transits"
	CompareSource     string  `json:"compare_source"`     // Optional: source for comparison period (defaults to Source)
	MinSpeed          float64 `json:"min_speed"`          // minimum speed filter
	BoundaryThreshold int     `json:"boundary_threshold"` // filter boundary hours with < N samples (default: 5)
	Histogram         bool    `json:"histogram"`          // whether to generate histogram
	HistBucketSize    float64 `json:"hist_bucket_size"`   // histogram bucket size
	HistMax           float64 `json:"hist_max"`           // histogram max value

	// Paper size for PDF output: "letter" (default) or "a4"
	PaperSize string `json:"paper_size"`

	// ExpandedChart preserves linear timestamp spacing in time-series charts.
	// Default false collapses sparse coverage gaps for consolidated charts.
	ExpandedChart bool `json:"expanded_chart"`

	// CompareCosineAngle overrides the cosine error angle for the comparison
	// period. Zero means use the same angle as the primary period.
	CompareCosineAngle float64 `json:"compare_cosine_angle"`

	// These can be overridden if site_id is not provided
	Location        string `json:"location"`         // site location
	Surveyor        string `json:"surveyor"`         // surveyor name
	Contact         string `json:"contact"`          // contact info
	SpeedLimit      int    `json:"speed_limit"`      // posted speed limit
	SiteDescription string `json:"site_description"` // site description
}

func isValidReportSource(source string) bool {
	switch source {
	case "radar_objects", "radar_data", "radar_data_transits":
		return true
	default:
		return false
	}
}

type reportCosineMetadata struct {
	angle float64
	label string
}

func reportDateUnixRange(startDate, endDate, timezone string) (int64, int64, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}
	startTime, err := time.ParseInLocation("2006-01-02", startDate, loc)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start date %q: %w", startDate, err)
	}
	endTime, err := time.ParseInLocation("2006-01-02", endDate, loc)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end date %q: %w", endDate, err)
	}
	return startTime.Unix(), inclusiveLocalDateEnd(endTime).Unix(), nil
}

func reportCosineMetadataForRange(periods []db.SiteConfigPeriod, startUnix, endUnix int64) reportCosineMetadata {
	angles := uniqueAnglesForRange(periods, float64(startUnix), float64(endUnix))
	if len(angles) == 0 {
		return reportCosineMetadata{}
	}
	if len(angles) == 1 {
		return reportCosineMetadata{angle: angles[0]}
	}
	return reportCosineMetadata{label: fmt.Sprintf("multiple periods: %s", formatReportCosineAngles(angles))}
}

func formatReportCosineAngles(angles []float64) string {
	parts := make([]string, 0, len(angles))
	for _, angle := range angles {
		parts = append(parts, fmt.Sprintf("%.1f°", angle))
	}
	return strings.Join(parts, ", ")
}

func (s *Server) generateReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse the JSON request body
	var req ReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Validate required fields
	if req.StartDate == "" || req.EndDate == "" {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, "start_date and end_date are required")
		return
	}
	if (req.CompareStart == "") != (req.CompareEnd == "") {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(
			w,
			http.StatusBadRequest,
			"compare_start_date and compare_end_date are required together",
		)
		return
	}

	// Load site data if site_id is provided
	var site *db.Site
	var configPeriods []db.SiteConfigPeriod
	if req.SiteID != nil {
		var err error
		site, err = s.db.GetSite(r.Context(), *req.SiteID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to load site: %v", err))
			return
		}

		configPeriods, err = s.db.ListSiteConfigPeriods(req.SiteID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to load site config period: %v. Please ensure site has an active configuration period.", err))
			return
		}
		if len(configPeriods) == 0 {
			w.Header().Set("Content-Type", "application/json")
			s.writeJSONError(w, http.StatusBadRequest, "Failed to load site config period: no configuration periods found. Please ensure site has a configuration period.")
			return
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, "site_id is required for report generation")
		return
	}

	// Set defaults from site or fallback values
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	if req.Units == "" {
		req.Units = "mph"
	}
	if req.Group == "" {
		req.Group = "1h"
	}
	if req.Source == "" {
		req.Source = "radar_data_transits"
	}
	if !units.IsValid(req.Units) {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'units'. Must be one of: %s", units.GetValidUnitsString()))
		return
	}
	if !isValidReportSource(req.Source) {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'source'. Must be one of: radar_objects, radar_data, radar_data_transits")
		return
	}
	if req.CompareSource != "" && !isValidReportSource(req.CompareSource) {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'compare_source'. Must be one of: radar_objects, radar_data, radar_data_transits")
		return
	}
	if req.HistBucketSize == 0 {
		req.HistBucketSize = 5.0
	}

	startUnix, endUnix, err := reportDateUnixRange(req.StartDate, req.EndDate, req.Timezone)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid report date range: %v", err))
		return
	}
	primaryCosine := reportCosineMetadataForRange(configPeriods, startUnix, endUnix)
	cosineErrorAngle := primaryCosine.angle
	cosineCorrectionLabel := primaryCosine.label

	compareCosineCorrectionLabel := ""
	if req.CompareStart != "" {
		compareStartUnix, compareEndUnix, err := reportDateUnixRange(req.CompareStart, req.CompareEnd, req.Timezone)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid comparison date range: %v", err))
			return
		}
		compareCosine := reportCosineMetadataForRange(configPeriods, compareStartUnix, compareEndUnix)
		if req.CompareCosineAngle == 0 {
			req.CompareCosineAngle = compareCosine.angle
			compareCosineCorrectionLabel = compareCosine.label
		}
	}

	// Use site data if available, otherwise use request data or defaults
	location := req.Location
	surveyor := req.Surveyor
	contact := req.Contact
	speedLimit := req.SpeedLimit
	siteDescription := req.SiteDescription

	if site != nil {
		location = site.Location
		surveyor = site.Surveyor
		contact = site.Contact
		if site.SiteDescription != nil {
			siteDescription = *site.SiteDescription
		}
	}

	// Apply final defaults if still empty
	if location == "" {
		location = "Survey Location"
	}
	if surveyor == "" {
		surveyor = "Surveyor"
	}
	if contact == "" {
		contact = "contact@example.com"
	}
	if speedLimit == 0 {
		speedLimit = 25
	}

	cfg := buildReportConfig(
		req,
		site,
		cosineErrorAngle,
		location,
		surveyor,
		contact,
		speedLimit,
		siteDescription,
	)
	cfg.CosineCorrectionLabel = cosineCorrectionLabel
	cfg.CompareCosineCorrectionLabel = compareCosineCorrectionLabel

	s.generateReportGo(w, r, req, cfg)
}

// generateReportGo handles report generation using the pure-Go report pipeline.
func (s *Server) generateReportGo(
	w http.ResponseWriter, r *http.Request,
	req ReportRequest,
	cfg report.Config,
) {
	// Determine output directory.
	reportRoot, err := getReportOutputRoot()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to determine output directory: %v", err))
		return
	}

	now := time.Now()
	runID := fmt.Sprintf("%s-%d", now.Format("20060102-150405"), now.Nanosecond())
	cfg.OutputDir = filepath.Join(reportRoot, "output", runID)

	result, err := report.Generate(r.Context(), s.db, cfg)
	if err != nil {
		log.Printf("Go PDF generation failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, report.ErrInvalidConfig) {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid report request: %v", err))
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("PDF generation failed: %v", err))
		}
		return
	}

	// Security: validate output paths are within the expected directory.
	if err := security.ValidatePathWithinDirectory(result.PDFPath, reportRoot); err != nil {
		log.Printf("Security: rejected Go PDF path %s: %v", result.PDFPath, err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusForbidden, "Invalid file path")
		return
	}
	if err := security.ValidatePathWithinDirectory(result.ZIPPath, reportRoot); err != nil {
		log.Printf("Security: rejected Go ZIP path %s: %v", result.ZIPPath, err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusForbidden, "Invalid file path")
		return
	}

	// Store paths relative to the report output root.
	relativePdfPath, relativeZipPath, err := relativeReportPaths(reportRoot, result.PDFPath, result.ZIPPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pdfFilename := filepath.Base(result.PDFPath)
	zipFilename := filepath.Base(result.ZIPPath)

	// Create report record in database.
	siteReport := &db.SiteReport{
		SiteID:      *req.SiteID,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Filepath:    relativePdfPath,
		Filename:    pdfFilename,
		ZipFilepath: &relativeZipPath,
		ZipFilename: &zipFilename,
		RunID:       runID,
		Timezone:    req.Timezone,
		Units:       req.Units,
		Source:      req.Source,
	}

	if err := s.db.CreateSiteReport(r.Context(), siteReport); err != nil {
		log.Printf("Failed to create report record: %v", err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to create report record")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success":   true,
		"report_id": siteReport.ID,
		"message":   "Report generated successfully",
		"pdf_path":  relativePdfPath,
		"zip_path":  relativeZipPath,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode response")
		return
	}

	log.Printf("Successfully generated Go PDF report (ID: %d): %s", siteReport.ID, pdfFilename)
}

func buildReportConfig(
	req ReportRequest,
	site *db.Site,
	cosineErrorAngle float64,
	location, surveyor, contact string,
	speedLimit int,
	siteDescription string,
) report.Config {
	cfg := report.Config{
		SiteID:             *req.SiteID,
		Location:           location,
		Surveyor:           surveyor,
		Contact:            contact,
		SpeedLimit:         speedLimit,
		SiteDescription:    siteDescription,
		StartDate:          req.StartDate,
		EndDate:            req.EndDate,
		Timezone:           req.Timezone,
		Units:              req.Units,
		Group:              req.Group,
		Source:             req.Source,
		MinSpeed:           req.MinSpeed,
		BoundaryThreshold:  req.BoundaryThreshold,
		Histogram:          req.Histogram,
		HistBucketSize:     req.HistBucketSize,
		HistMax:            req.HistMax,
		CompareStart:       req.CompareStart,
		CompareEnd:         req.CompareEnd,
		CompareSource:      req.CompareSource,
		CosineAngle:        cosineErrorAngle,
		CompareCosineAngle: req.CompareCosineAngle,
		PaperSize:          req.PaperSize,
		ExpandedChart:      req.ExpandedChart,
	}

	if site != nil {
		cfg.IncludeMap = site.IncludeMap
		if site.MapSVGData != nil {
			cfg.MapSVG = *site.MapSVGData
		}
	}

	return cfg
}

const (
	reportOutputDirEnv       = "VELOCITY_REPORT_OUTPUT_DIR"
	deployedReportOutputRoot = "/var/lib/velocity-report/reports"
)

// getReportOutputRoot determines where generated report artifacts are stored.
// Can be overridden via VELOCITY_REPORT_OUTPUT_DIR.
func getReportOutputRoot() (string, error) {
	reportRoot := os.Getenv(reportOutputDirEnv)
	if reportRoot != "" {
		return reportRoot, nil
	}

	if _, err := os.Stat(deployedReportOutputRoot); err == nil {
		return deployedReportOutputRoot, nil
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return "", fmt.Errorf("failed to determine repository root: %w", err)
	}
	return filepath.Join(repoRoot, ".tmp", "reports"), nil
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd, nil
		}
	}
}

func relativeReportPaths(reportRoot, pdfPath, zipPath string) (string, string, error) {
	relativePDFPath, err := filepath.Rel(reportRoot, pdfPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to compute relative PDF path")
	}
	if strings.HasPrefix(relativePDFPath, "..") {
		return "", "", fmt.Errorf("failed to compute relative PDF path")
	}

	relativeZIPPath, err := filepath.Rel(reportRoot, zipPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to compute relative ZIP path")
	}
	if strings.HasPrefix(relativeZIPPath, "..") {
		return "", "", fmt.Errorf("failed to compute relative ZIP path")
	}

	return relativePDFPath, relativeZIPPath, nil
}
