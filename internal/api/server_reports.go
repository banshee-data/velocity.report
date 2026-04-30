package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/security"
)

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

	// Get the report output root.
	reportRoot, err := getReportOutputRoot()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to determine report output directory: %v", err))
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
		filePath = filepath.Join(reportRoot, *report.ZipFilepath)
		filename = *report.ZipFilename
		contentType = "application/zip"
	} else {
		// Default to PDF
		filePath = filepath.Join(reportRoot, report.Filepath)
		filename = report.Filename
		contentType = "application/pdf"
	}

	// Validate path is within the report output root.
	if err := security.ValidatePathWithinDirectory(filePath, reportRoot); err != nil {
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
