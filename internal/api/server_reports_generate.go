package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/report"
	"github.com/banshee-data/velocity.report/internal/security"
)

var runPythonPDFGenerator = func(pythonBin, pdfDir, configFile string) ([]byte, error) {
	cmd := exec.Command(
		pythonBin,
		"-m", "pdf_generator.cli.main",
		configFile,
	)
	cmd.Dir = pdfDir
	return cmd.CombinedOutput()
}

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
	Source            string  `json:"source"`             // "radar_objects" or "radar_data_transits"
	CompareSource     string  `json:"compare_source"`     // Optional: source for comparison period (defaults to Source)
	MinSpeed          float64 `json:"min_speed"`          // minimum speed filter
	BoundaryThreshold int     `json:"boundary_threshold"` // filter boundary hours with < N samples (default: 5)
	Histogram         bool    `json:"histogram"`          // whether to generate histogram
	HistBucketSize    float64 `json:"hist_bucket_size"`   // histogram bucket size
	HistMax           float64 `json:"hist_max"`           // histogram max value

	// Paper size for PDF output: "a4" (default) or "letter"
	PaperSize string `json:"paper_size"`

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
	var cosineErrorAngle float64
	if req.SiteID != nil {
		var err error
		site, err = s.db.GetSite(r.Context(), *req.SiteID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to load site: %v", err))
			return
		}

		// Get the active SCD period to retrieve cosine_error_angle
		activePeriod, err := s.db.GetActiveSiteConfigPeriod(*req.SiteID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to load site config period: %v. Please ensure site has an active configuration period.", err))
			return
		}
		cosineErrorAngle = activePeriod.CosineErrorAngle
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
	if req.HistBucketSize == 0 {
		req.HistBucketSize = 5.0
	}

	// Use site data if available, otherwise use request data or defaults
	location := req.Location
	surveyor := req.Surveyor
	contact := req.Contact
	speedLimit := req.SpeedLimit
	siteDescription := req.SiteDescription
	speedLimitNote := ""

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
		speedLimitNote,
	)

	backend := strings.ToLower(strings.TrimSpace(os.Getenv("VELOCITY_PDF_BACKEND")))
	if backend == "go" || backend == "both" {
		s.generateReportGo(w, r, req, site, cfg, backend == "both")
		return
	}

	// Create unique run ID for organized output folders
	// Include nanoseconds to ensure uniqueness under concurrent load
	now := time.Now()
	runID := fmt.Sprintf("%s-%d", now.Format("20060102-150405"), now.Nanosecond())
	outputDir := fmt.Sprintf("output/%s", runID)

	// Pre-create the output directory relative to the PDF generator dir.
	// The Python subprocess also calls os.makedirs, but creating it here
	// with the Go process's permissions avoids PermissionError when the
	// pdf-generator tree is owned by root on deployed systems.
	pdfDirEarly, err := getPDFGeneratorDir()
	if err == nil {
		absOutputDir := filepath.Join(pdfDirEarly, outputDir)
		if mkErr := os.MkdirAll(absOutputDir, 0755); mkErr != nil {
			log.Printf("Warning: could not pre-create output directory %s: %v", absOutputDir, mkErr)
		}
	}

	config := buildPythonReportConfig(cfg, site, outputDir, s.debugMode)
	configFile, cleanupConfigFile, err := writePythonConfigFile(config, cfg.SpeedLimitNote)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer cleanupConfigFile()

	// Get the PDF generator directory
	pdfDir, err := getPDFGeneratorDir()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to determine PDF generator directory: %v", err))
		return
	}

	artifacts := buildPythonReportArtifacts(pdfDir, outputDir, cfg.EndDate, cfg.Location)
	output, err := runPythonPDFGenerator(resolvePythonBinary(), pdfDir, configFile)
	if err != nil {
		log.Printf("PDF generation failed: %v\nOutput: %s", err, string(output))
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("PDF generation failed: %v", err))
		return
	}

	// Log Python output for debugging
	if len(output) > 0 {
		log.Printf("PDF generator output:\n%s", string(output))
	}

	if outputIndicatesReportFailure(string(output)) {
		log.Printf("PDF generation output contained failure markers despite zero exit status")
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, "PDF generation failed: report compiler indicated failure")
		return
	}

	// Verify PDF was actually created
	// Security: Validate path is within pdf-generator directory to prevent path traversal
	if err := security.ValidatePathWithinDirectory(artifacts.FullPDFPath, pdfDir); err != nil {
		log.Printf("Security: rejected PDF path %s: %v", artifacts.FullPDFPath, err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusForbidden, "Invalid file path")
		return
	}

	if _, err := os.Stat(artifacts.FullPDFPath); os.IsNotExist(err) {
		log.Printf("PDF generation completed but file not found at: %s", artifacts.FullPDFPath)
		log.Printf("This usually means no data was available for the specified date range.")
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, "PDF generation failed: No data available for the specified date range. Please check your dates and ensure data exists.")
		return
	}

	// Create report record in database
	siteID := 0
	if req.SiteID != nil {
		siteID = *req.SiteID
	}

	report := &db.SiteReport{
		SiteID:      siteID,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Filepath:    artifacts.RelativePDFPath,
		Filename:    artifacts.PDFFilename,
		ZipFilepath: &artifacts.RelativeZIPPath,
		ZipFilename: &artifacts.ZIPFilename,
		RunID:       runID,
		Timezone:    req.Timezone,
		Units:       req.Units,
		Source:      req.Source,
	}

	if err := s.db.CreateSiteReport(r.Context(), report); err != nil {
		log.Printf("Failed to create report record: %v", err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to create report record")
		return
	}

	// Return report ID and file paths
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success":   true,
		"report_id": report.ID,
		"message":   "Report generated successfully",
		"pdf_path":  artifacts.RelativePDFPath,
		"zip_path":  artifacts.RelativeZIPPath,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode response")
		return
	}

	log.Printf("Successfully generated PDF report (ID: %d): %s", report.ID, artifacts.PDFFilename)
}

// generateReportGo handles report generation using the pure-Go report pipeline.
// When includePythonComparisonTeX is true, it also captures Python TeX and
// appends both TeX variants into the final sources ZIP for comparison.
func (s *Server) generateReportGo(
	w http.ResponseWriter, r *http.Request,
	req ReportRequest, site *db.Site,
	cfg report.Config,
	includePythonComparisonTeX bool,
) {
	// Determine output directory.
	pdfDir, err := getPDFGeneratorDir()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to determine output directory: %v", err))
		return
	}

	now := time.Now()
	runID := fmt.Sprintf("%s-%d", now.Format("20060102-150405"), now.Nanosecond())
	cfg.OutputDir = filepath.Join(pdfDir, "output", runID)

	result, err := report.Generate(r.Context(), s.db, cfg)
	if err != nil {
		log.Printf("Go PDF generation failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("PDF generation failed: %v", err))
		return
	}

	// Security: validate output paths are within the expected directory.
	if err := security.ValidatePathWithinDirectory(result.PDFPath, pdfDir); err != nil {
		log.Printf("Security: rejected Go PDF path %s: %v", result.PDFPath, err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusForbidden, "Invalid file path")
		return
	}
	if err := security.ValidatePathWithinDirectory(result.ZIPPath, pdfDir); err != nil {
		log.Printf("Security: rejected Go ZIP path %s: %v", result.ZIPPath, err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusForbidden, "Invalid file path")
		return
	}

	if includePythonComparisonTeX {
		if err := s.appendPythonComparisonTeX(cfg, site, pdfDir, runID, result.ZIPPath); err != nil {
			log.Printf("Failed to add Python comparison TeX to ZIP: %v", err)
			w.Header().Set("Content-Type", "application/json")
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("PDF generation failed: %v", err))
			return
		}
	}

	// Build relative paths matching the Python path convention.
	relativePdfPath, relativeZipPath, err := relativeReportPaths(pdfDir, result.PDFPath, result.ZIPPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pdfFilename := filepath.Base(result.PDFPath)
	zipFilename := filepath.Base(result.ZIPPath)

	// Create report record in database (same as Python path).
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

	// Return same JSON response shape as the Python path.
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
	siteDescription, speedLimitNote string,
) report.Config {
	cfg := report.Config{
		SiteID:             *req.SiteID,
		Location:           location,
		Surveyor:           surveyor,
		Contact:            contact,
		SpeedLimit:         speedLimit,
		SiteDescription:    siteDescription,
		SpeedLimitNote:     speedLimitNote,
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
	}

	if site != nil {
		cfg.IncludeMap = site.IncludeMap
		if site.MapSVGData != nil {
			cfg.MapSVG = *site.MapSVGData
		}
	}

	return cfg
}

func buildPythonReportConfig(cfg report.Config, site *db.Site, outputDir string, debugMode bool) map[string]interface{} {
	queryConfig := map[string]interface{}{
		"start_date":         cfg.StartDate,
		"end_date":           cfg.EndDate,
		"timezone":           cfg.Timezone,
		"group":              cfg.Group,
		"units":              cfg.Units,
		"source":             cfg.Source,
		"min_speed":          cfg.MinSpeed,
		"boundary_threshold": cfg.BoundaryThreshold,
		"histogram":          cfg.Histogram,
		"hist_bucket_size":   cfg.HistBucketSize,
		"hist_max":           cfg.HistMax,
		"site_id":            cfg.SiteID,
	}
	if cfg.CompareStart != "" && cfg.CompareEnd != "" {
		queryConfig["compare_start_date"] = cfg.CompareStart
		queryConfig["compare_end_date"] = cfg.CompareEnd
		compareSource := cfg.CompareSource
		if compareSource == "" {
			compareSource = cfg.Source
		}
		queryConfig["compare_source"] = compareSource
	}

	siteConfig := map[string]interface{}{
		"location":         cfg.Location,
		"surveyor":         cfg.Surveyor,
		"contact":          cfg.Contact,
		"speed_limit":      cfg.SpeedLimit,
		"site_description": cfg.SiteDescription,
		"speed_limit_note": cfg.SpeedLimitNote,
	}
	if site != nil {
		if site.Latitude != nil {
			siteConfig["latitude"] = *site.Latitude
		}
		if site.Longitude != nil {
			siteConfig["longitude"] = *site.Longitude
		}
		if site.MapAngle != nil {
			siteConfig["map_angle"] = *site.MapAngle
		}
		if site.BBoxNELat != nil {
			siteConfig["bbox_ne_lat"] = *site.BBoxNELat
		}
		if site.BBoxNELng != nil {
			siteConfig["bbox_ne_lng"] = *site.BBoxNELng
		}
		if site.BBoxSWLat != nil {
			siteConfig["bbox_sw_lat"] = *site.BBoxSWLat
		}
		if site.BBoxSWLng != nil {
			siteConfig["bbox_sw_lng"] = *site.BBoxSWLng
		}
	}

	return map[string]interface{}{
		"query": queryConfig,
		"site":  siteConfig,
		"radar": map[string]interface{}{
			"cosine_error_angle": cfg.CosineAngle,
		},
		"output": map[string]interface{}{
			"output_dir": outputDir,
			"debug":      debugMode,
			"map":        cfg.IncludeMap,
		},
	}
}

func writePythonConfigFile(config map[string]interface{}, speedLimitNote string) (string, func(), error) {
	configFile := filepath.Join(os.TempDir(), fmt.Sprintf("report_config_%d.json", time.Now().UnixNano()))
	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal config: %v", err)
	}

	if err := security.ValidateExportPath(configFile); err != nil {
		return "", nil, fmt.Errorf("invalid config file path: %v", err)
	}

	if err := os.WriteFile(configFile, configData, 0644); err != nil {
		return "", nil, fmt.Errorf("failed to write config file: %v", err)
	}

	log.Printf("Report config written: %s (site.speed_limit_note=%q)", configFile, speedLimitNote)
	cleanup := func() {
		if os.Getenv("PDF_GENERATOR_PYTHON") == "" {
			os.Remove(configFile)
			return
		}
		log.Printf("Preserving config file for test inspection: %s", configFile)
	}

	return configFile, cleanup, nil
}

func resolvePythonBinary() string {
	pythonBin := os.Getenv("PDF_GENERATOR_PYTHON")
	if pythonBin != "" {
		log.Printf("Using overridden PDF generator python: %s", pythonBin)
		return pythonBin
	}

	deployedPython := "/opt/velocity-report/.venv/bin/python"
	if _, err := os.Stat(deployedPython); err == nil {
		log.Printf("Using deployed PDF generator python: %s", deployedPython)
		return deployedPython
	}

	repoRoot, _ := os.Getwd()
	defaultPythonBin := filepath.Join(repoRoot, ".venv", "bin", "python")
	if _, err := os.Stat(defaultPythonBin); err == nil {
		log.Printf("Using development PDF generator python: %s", defaultPythonBin)
		return defaultPythonBin
	}

	log.Printf("PDF generator venv not found, using system python3")
	return "python3"
}

type pythonReportArtifacts struct {
	PDFFilename     string
	ZIPFilename     string
	RelativePDFPath string
	RelativeZIPPath string
	FullPDFPath     string
	FullZIPPath     string
}

func buildPythonReportArtifacts(pdfDir, outputDir, endDate, location string) pythonReportArtifacts {
	safeEndDate := security.SanitizeFilename(endDate)
	safeLocation := security.SanitizeFilename(location)
	pdfFilename := fmt.Sprintf("%s_velocity.report_%s_report.pdf", safeEndDate, safeLocation)
	zipFilename := fmt.Sprintf("%s_velocity.report_%s_sources.zip", safeEndDate, safeLocation)
	relativePDFPath := filepath.Join(outputDir, pdfFilename)
	relativeZIPPath := filepath.Join(outputDir, zipFilename)

	return pythonReportArtifacts{
		PDFFilename:     pdfFilename,
		ZIPFilename:     zipFilename,
		RelativePDFPath: relativePDFPath,
		RelativeZIPPath: relativeZIPPath,
		FullPDFPath:     filepath.Join(pdfDir, relativePDFPath),
		FullZIPPath:     filepath.Join(pdfDir, relativeZIPPath),
	}
}

func (s *Server) appendPythonComparisonTeX(cfg report.Config, site *db.Site, pdfDir, runID, goZipPath string) error {
	comparisonRelativeOutputDir := filepath.Join("output", runID, "python-compare")
	comparisonAbsOutputDir := filepath.Join(pdfDir, comparisonRelativeOutputDir)
	if err := os.MkdirAll(comparisonAbsOutputDir, 0755); err != nil {
		return fmt.Errorf("create Python comparison output dir: %w", err)
	}
	defer os.RemoveAll(comparisonAbsOutputDir)

	config := buildPythonReportConfig(cfg, site, comparisonRelativeOutputDir, s.debugMode)
	configFile, cleanupConfigFile, err := writePythonConfigFile(config, cfg.SpeedLimitNote)
	if err != nil {
		return err
	}
	defer cleanupConfigFile()

	output, err := runPythonPDFGenerator(resolvePythonBinary(), pdfDir, configFile)
	if len(output) > 0 {
		log.Printf("PDF generator comparison output:\n%s", string(output))
	}
	if err != nil {
		return fmt.Errorf("python comparison generation failed: %w", err)
	}
	if outputIndicatesReportFailure(string(output)) {
		return fmt.Errorf("python comparison generation failed: report compiler indicated failure")
	}

	goTex, err := readZipEntry(goZipPath, "report.tex")
	if err != nil {
		return fmt.Errorf("read Go report.tex from ZIP: %w", err)
	}

	artifacts := buildPythonReportArtifacts(pdfDir, comparisonRelativeOutputDir, cfg.EndDate, cfg.Location)
	pythonTex, err := readPythonPortableReportTeX(artifacts.FullZIPPath)
	if err != nil {
		return err
	}

	if err := report.AppendFilesToZip(goZipPath, map[string][]byte{
		"comparison/go/report.tex":     goTex,
		"comparison/python/report.tex": pythonTex,
	}); err != nil {
		return fmt.Errorf("append comparison TeX to ZIP: %w", err)
	}

	return nil
}

func readZipEntry(zipPath, entryName string) ([]byte, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	for _, entry := range reader.File {
		if entry.Name != entryName {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}

	return nil, fmt.Errorf("zip entry %q not found", entryName)
}

func readPythonPortableReportTeX(zipPath string) ([]byte, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open Python sources ZIP: %w", err)
	}
	defer reader.Close()

	for _, entry := range reader.File {
		if !strings.HasSuffix(entry.Name, "_report.tex") || strings.HasSuffix(entry.Name, "_report_fonts.tex") {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, err
		}
		return data, nil
	}

	return nil, fmt.Errorf("python sources ZIP does not contain portable report.tex")
}

// getPDFGeneratorDir determines the PDF generator directory.
// Can be overridden via PDF_GENERATOR_DIR env var.
// Default to /opt/velocity-report/tools/pdf-generator for deployed systems,
// or tools/pdf-generator relative to current directory for development.
func getPDFGeneratorDir() (string, error) {
	pdfDir := os.Getenv("PDF_GENERATOR_DIR")
	if pdfDir != "" {
		return pdfDir, nil
	}

	// Check if deployed location exists
	deployedPath := "/opt/velocity-report/tools/pdf-generator"
	if _, err := os.Stat(deployedPath); err == nil {
		return deployedPath, nil
	}

	// Fall back to development location
	repoRoot, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return filepath.Join(repoRoot, "tools", "pdf-generator"), nil
}

func outputIndicatesReportFailure(output string) bool {
	if output == "" {
		return false
	}

	lowerOutput := strings.ToLower(output)
	failureMarkers := []string{
		"failed to complete report for",
		"error: failed to generate pdf report",
		"one or more date ranges failed to generate a complete pdf report",
	}

	for _, marker := range failureMarkers {
		if strings.Contains(lowerOutput, marker) {
			return true
		}
	}

	return false
}

func relativeReportPaths(pdfDir, pdfPath, zipPath string) (string, string, error) {
	relativePDFPath, err := filepath.Rel(pdfDir, pdfPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to compute relative PDF path")
	}
	if strings.HasPrefix(relativePDFPath, "..") {
		return "", "", fmt.Errorf("failed to compute relative PDF path")
	}

	relativeZIPPath, err := filepath.Rel(pdfDir, zipPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to compute relative ZIP path")
	}
	if strings.HasPrefix(relativeZIPPath, "..") {
		return "", "", fmt.Errorf("failed to compute relative ZIP path")
	}

	return relativePDFPath, relativeZIPPath, nil
}
