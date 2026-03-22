package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/security"
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
	Source            string  `json:"source"`             // "radar_objects" or "radar_data_transits"
	CompareSource     string  `json:"compare_source"`     // Optional: source for comparison period (defaults to Source)
	MinSpeed          float64 `json:"min_speed"`          // minimum speed filter
	BoundaryThreshold int     `json:"boundary_threshold"` // filter boundary hours with < N samples (default: 5)
	Histogram         bool    `json:"histogram"`          // whether to generate histogram
	HistBucketSize    float64 `json:"hist_bucket_size"`   // histogram bucket size
	HistMax           float64 `json:"hist_max"`           // histogram max value

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

	// Create unique run ID for organized output folders
	// Include nanoseconds to ensure uniqueness under concurrent load
	now := time.Now()
	runID := fmt.Sprintf("%s-%d", now.Format("20060102-150405"), now.Nanosecond())
	outputDir := fmt.Sprintf("output/%s", runID)

	// Create a config JSON for the PDF generator
	// Note: Not setting file_prefix - let Python auto-generate from source + date range
	queryConfig := map[string]interface{}{
		"start_date":         req.StartDate,
		"end_date":           req.EndDate,
		"timezone":           req.Timezone,
		"group":              req.Group,
		"units":              req.Units,
		"source":             req.Source,
		"min_speed":          req.MinSpeed,
		"boundary_threshold": req.BoundaryThreshold,
		"histogram":          req.Histogram,
		"hist_bucket_size":   req.HistBucketSize,
		"hist_max":           req.HistMax,
	}
	if req.SiteID != nil {
		queryConfig["site_id"] = *req.SiteID
	}
	if req.CompareStart != "" && req.CompareEnd != "" {
		queryConfig["compare_start_date"] = req.CompareStart
		queryConfig["compare_end_date"] = req.CompareEnd
		// Use compare_source if specified, otherwise default to the primary source
		compareSource := req.CompareSource
		if compareSource == "" {
			compareSource = req.Source
		}
		queryConfig["compare_source"] = compareSource
	}

	// Build site config with map positioning data if available
	siteConfig := map[string]interface{}{
		"location":         location,
		"surveyor":         surveyor,
		"contact":          contact,
		"speed_limit":      speedLimit,
		"site_description": siteDescription,
		"speed_limit_note": speedLimitNote,
	}
	// Add map positioning fields if site has them
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

	config := map[string]interface{}{
		"query": queryConfig,
		"site":  siteConfig,
		"radar": map[string]interface{}{
			"cosine_error_angle": cosineErrorAngle,
		},
		"output": map[string]interface{}{
			"output_dir": outputDir,
			"debug":      s.debugMode,
			"map":        site != nil && site.IncludeMap,
		},
	}

	// Write config to a temporary file
	// Use nanoseconds to ensure unique filename under concurrent requests
	configFile := filepath.Join(os.TempDir(), fmt.Sprintf("report_config_%d.json", now.UnixNano()))
	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to marshal config: %v", err))
		return
	}

	// Validate temp file path (should always pass since we control the temp dir)
	if err := security.ValidateExportPath(configFile); err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Invalid config file path: %v", err))
		return
	}

	if err := os.WriteFile(configFile, configData, 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write config file: %v", err))
		return
	}
	// Log the config file and speed_limit_note so we can inspect what is passed to the
	// Python generator in production/debug runs. For tests we preserve the file when
	// PDF_GENERATOR_PYTHON is set so the test can inspect the JSON the server wrote.
	log.Printf("Report config written: %s (site.speed_limit_note=%q)", configFile, speedLimitNote)
	if os.Getenv("PDF_GENERATOR_PYTHON") == "" {
		defer os.Remove(configFile) // Clean up after execution in normal runs
	} else {
		log.Printf("Preserving config file for test inspection: %s", configFile)
	}

	// Get the PDF generator directory
	pdfDir, err := getPDFGeneratorDir()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to determine PDF generator directory: %v", err))
		return
	}

	// Path to Python binary - allow overriding via PDF_GENERATOR_PYTHON
	// Check locations in priority order:
	// 1. Deployed venv: /opt/velocity-report/.venv/bin/python
	// 2. Development venv: ./.venv/bin/python
	// 3. System python3
	pythonBin := os.Getenv("PDF_GENERATOR_PYTHON")
	if pythonBin == "" {
		deployedPython := "/opt/velocity-report/.venv/bin/python"
		if _, err := os.Stat(deployedPython); err == nil {
			pythonBin = deployedPython
			log.Printf("Using deployed PDF generator python: %s", pythonBin)
		} else {
			repoRoot, _ := os.Getwd()
			defaultPythonBin := filepath.Join(repoRoot, ".venv", "bin", "python")
			if _, err := os.Stat(defaultPythonBin); err == nil {
				pythonBin = defaultPythonBin
				log.Printf("Using development PDF generator python: %s", pythonBin)
			} else {
				pythonBin = "python3"
				log.Printf("PDF generator venv not found, using system python3")
			}
		}
	} else {
		log.Printf("Using overridden PDF generator python: %s", pythonBin)
	}

	// Execute the PDF generator
	cmd := exec.Command(
		pythonBin,
		"-m", "pdf_generator.cli.main",
		configFile,
	)
	cmd.Dir = pdfDir

	output, err := cmd.CombinedOutput()
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

	// Sanitize user-provided values to prevent path traversal attacks
	safeEndDate := security.SanitizeFilename(req.EndDate)
	safeLocation := security.SanitizeFilename(location)

	// Generate filename as: {endDate}_velocity.report_{location}.pdf
	// Python PDF generator adds _report suffix, so final name is:
	// {endDate}_velocity.report_{location}_report.pdf
	pdfFilename := fmt.Sprintf("%s_velocity.report_%s_report.pdf", safeEndDate, safeLocation)

	// Generate ZIP filename with same format (no _report suffix for ZIP)
	zipFilename := fmt.Sprintf("%s_velocity.report_%s_sources.zip", safeEndDate, safeLocation)

	// Store relative paths from pdf-generator directory
	relativePdfPath := filepath.Join(outputDir, pdfFilename)
	relativeZipPath := filepath.Join(outputDir, zipFilename)

	// Verify PDF was actually created
	fullPdfPath := filepath.Join(pdfDir, relativePdfPath)

	// Security: Validate path is within pdf-generator directory to prevent path traversal
	if err := security.ValidatePathWithinDirectory(fullPdfPath, pdfDir); err != nil {
		log.Printf("Security: rejected PDF path %s: %v", fullPdfPath, err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusForbidden, "Invalid file path")
		return
	}

	if _, err := os.Stat(fullPdfPath); os.IsNotExist(err) {
		log.Printf("PDF generation completed but file not found at: %s", fullPdfPath)
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
		Filepath:    relativePdfPath,
		Filename:    pdfFilename,
		ZipFilepath: &relativeZipPath,
		ZipFilename: &zipFilename,
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
		"pdf_path":  relativePdfPath,
		"zip_path":  relativeZipPath,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode response")
		return
	}

	log.Printf("Successfully generated PDF report (ID: %d): %s", report.ID, pdfFilename)
}

// handleReports routes report-related requests to appropriate handlers
func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse the path to extract ID and action
	// URL formats:
	//   /api/reports - list all recent reports
	//   /api/reports/123 - get report metadata
	//   /api/reports/123/download/filename.pdf - download file with filename in URL
	//   /api/reports/site/456 - list reports for site 456
	path := strings.TrimPrefix(r.URL.Path, "/api/reports")
	path = strings.Trim(path, "/")

	// List all recent reports
	if path == "" && r.Method == http.MethodGet {
		s.listAllReports(w, r)
		return
	}

	// Handle /api/reports/site/{siteID}
	if strings.HasPrefix(path, "site/") {
		siteIDStr := strings.TrimPrefix(path, "site/")
		siteID, err := strconv.Atoi(siteIDStr)
		if err != nil {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid site ID")
			return
		}
		if r.Method == http.MethodGet {
			s.listSiteReports(w, r, siteID)
		} else {
			s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	// Parse report ID and action
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid request path")
		return
	}

	reportID, err := strconv.Atoi(parts[0])
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid report ID")
		return
	}

	// Handle download action: /api/reports/123/download/filename.pdf
	if len(parts) >= 2 && parts[1] == "download" {
		if r.Method == http.MethodGet {
			if len(parts) == 3 {
				// Extract file type from filename extension
				filename := parts[2]
				fileFormat := "pdf"
				if strings.HasSuffix(filename, ".zip") {
					fileFormat = "zip"
				}
				s.downloadReport(w, r, reportID, fileFormat)
				return
			}
			// No filename in path — reject
			s.writeJSONError(w, http.StatusBadRequest, "filename required in download path; use /api/reports/{id}/download/{filename}")
		} else {
			s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	// Get report metadata or delete
	switch r.Method {
	case http.MethodGet:
		s.getReport(w, r, reportID)
	case http.MethodDelete:
		s.deleteReport(w, r, reportID)
	default:
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) listAllReports(w http.ResponseWriter, r *http.Request) {
	reports, err := s.db.GetRecentReportsAllSites(r.Context(), 15)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve reports: %v", err))
		return
	}

	if err := json.NewEncoder(w).Encode(reports); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode reports")
		return
	}
}

func (s *Server) listSiteReports(w http.ResponseWriter, r *http.Request, siteID int) {
	reports, err := s.db.GetRecentReportsForSite(r.Context(), siteID, 5)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve reports: %v", err))
		return
	}

	if err := json.NewEncoder(w).Encode(reports); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode reports")
		return
	}
}

func (s *Server) getReport(w http.ResponseWriter, r *http.Request, reportID int) {
	report, err := s.db.GetSiteReport(r.Context(), reportID)
	if err != nil {
		if err.Error() == "report not found" {
			s.writeJSONError(w, http.StatusNotFound, "Report not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve report: %v", err))
		}
		return
	}

	if err := json.NewEncoder(w).Encode(report); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode report")
		return
	}
}

func (s *Server) downloadReport(w http.ResponseWriter, r *http.Request, reportID int, fileFormat string) {
	// Validate file format
	if fileFormat != "pdf" && fileFormat != "zip" {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusBadRequest, "Invalid file format. Must be 'pdf' or 'zip'")
		return
	}

	// Get report metadata from database
	report, err := s.db.GetSiteReport(r.Context(), reportID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err.Error() == "report not found" {
			s.writeJSONError(w, http.StatusNotFound, "Report not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve report: %v", err))
		}
		return
	}

	// Get the PDF generator directory
	pdfDir, err := getPDFGeneratorDir()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to determine PDF generator directory: %v", err))
		return
	}

	// Determine which file to serve based on file format
	var filePath, filename, contentType string

	if fileFormat == "zip" {
		// Check if ZIP file exists
		if report.ZipFilepath == nil || *report.ZipFilepath == "" {
			w.Header().Set("Content-Type", "application/json")
			s.writeJSONError(w, http.StatusNotFound, "ZIP file not available for this report")
			return
		}
		filePath = filepath.Join(pdfDir, *report.ZipFilepath)
		filename = *report.ZipFilename
		contentType = "application/zip"
	} else {
		// Default to PDF
		filePath = filepath.Join(pdfDir, report.Filepath)
		filename = report.Filename
		contentType = "application/pdf"
	}

	// Validate path is within pdf-generator directory (security check)
	if err := security.ValidatePathWithinDirectory(filePath, pdfDir); err != nil {
		log.Printf("Security: rejected download path %s: %v", filePath, err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusForbidden, "Invalid file path")
		return
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Printf("File not found at path: %s", filePath)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("%s file not found", strings.ToUpper(fileFormat)))
		return
	}

	// Read the file
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("Failed to read file: %v", err)
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to read %s file", fileFormat))
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileData)))

	// Stream the file to the client
	if _, err := w.Write(fileData); err != nil {
		log.Printf("Failed to write file to response: %v", err)
		return
	}

	log.Printf("Successfully downloaded %s file (ID: %d): %s", strings.ToUpper(fileFormat), reportID, filename)
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

func (s *Server) deleteReport(w http.ResponseWriter, r *http.Request, reportID int) {
	if err := s.db.DeleteSiteReport(r.Context(), reportID); err != nil {
		if err.Error() == "report not found" {
			s.writeJSONError(w, http.StatusNotFound, "Report not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete report: %v", err))
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
