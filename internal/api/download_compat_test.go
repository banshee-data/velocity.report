package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
)

// TestDownloadReport_FilenameFormat verifies that the download endpoint
// correctly serves files with the velocity.report_ prefix
func TestDownloadReport_FilenameFormat(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Create temporary output directory for test files
	tmpDir := t.TempDir()
	runID := "test-20251022-100000"
	outputDir := filepath.Join(tmpDir, "output", runID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("failed to create output dir: %v", err)
	}

	// Save original working directory
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	// Change to a temporary directory that will act as our repo root
	testRepoRoot := t.TempDir()
	pdfDir := filepath.Join(testRepoRoot, "tools", "pdf-generator")
	if err := os.MkdirAll(filepath.Join(pdfDir, "output", runID), 0755); err != nil {
		t.Fatalf("failed to create pdf dir structure: %v", err)
	}
	if err := os.Chdir(testRepoRoot); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	tests := []struct {
		name          string
		dbFilename    string // What's in the database
		dbZipFilename string
		diskFilename  string // What actually exists on disk (should match database)
		diskZipname   string
		shouldFindPDF bool
		shouldFindZIP bool
	}{
		{
			name:          "correct format - both database and disk use velocity.report_ prefix",
			dbFilename:    "velocity.report_radar_data_transits_2025-10-01_to_2025-10-14_report.pdf",
			dbZipFilename: "velocity.report_radar_data_transits_2025-10-01_to_2025-10-14_sources.zip",
			diskFilename:  "velocity.report_radar_data_transits_2025-10-01_to_2025-10-14_report.pdf",
			diskZipname:   "velocity.report_radar_data_transits_2025-10-01_to_2025-10-14_sources.zip",
			shouldFindPDF: true,
			shouldFindZIP: true,
		},
		{
			name:          "another valid report with correct prefix",
			dbFilename:    "velocity.report_radar_objects_2025-09-01_to_2025-09-15_report.pdf",
			dbZipFilename: "velocity.report_radar_objects_2025-09-01_to_2025-09-15_sources.zip",
			diskFilename:  "velocity.report_radar_objects_2025-09-01_to_2025-09-15_report.pdf",
			diskZipname:   "velocity.report_radar_objects_2025-09-01_to_2025-09-15_sources.zip",
			shouldFindPDF: true,
			shouldFindZIP: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the actual files on disk with the disk filename
			diskPDFPath := filepath.Join(pdfDir, "output", runID, tt.diskFilename)
			diskZIPPath := filepath.Join(pdfDir, "output", runID, tt.diskZipname)

			if err := os.WriteFile(diskPDFPath, []byte("fake pdf content"), 0644); err != nil {
				t.Fatalf("failed to create test PDF: %v", err)
			}
			if err := os.WriteFile(diskZIPPath, []byte("fake zip content"), 0644); err != nil {
				t.Fatalf("failed to create test ZIP: %v", err)
			}
			defer os.Remove(diskPDFPath)
			defer os.Remove(diskZIPPath)

			// Create a report record in the database with the database filename
			relativePdfPath := filepath.Join("output", runID, tt.dbFilename)
			relativeZipPath := filepath.Join("output", runID, tt.dbZipFilename)

			report := &db.SiteReport{
				SiteID:      1,
				StartDate:   "2025-10-01",
				EndDate:     "2025-10-14",
				Filepath:    relativePdfPath,
				Filename:    tt.dbFilename,
				ZipFilepath: &relativeZipPath,
				ZipFilename: &tt.dbZipFilename,
				RunID:       runID,
				Timezone:    "UTC",
				Units:       "mph",
				Source:      "radar_data_transits",
			}

			if err := dbInst.CreateSiteReport(report); err != nil {
				t.Fatalf("failed to create report: %v", err)
			}

			// Test PDF download
			t.Run("pdf", func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download?file_type=pdf", report.ID), nil)
				w := httptest.NewRecorder()

				server.downloadReport(w, req, report.ID, "pdf")

				if tt.shouldFindPDF {
					if w.Code != http.StatusOK {
						t.Errorf("PDF download failed: status=%d, body=%s", w.Code, w.Body.String())
					}
					if w.Header().Get("Content-Type") != "application/pdf" {
						t.Errorf("wrong content type: got %s", w.Header().Get("Content-Type"))
					}
				} else {
					if w.Code != http.StatusNotFound {
						t.Errorf("expected 404 for PDF, got %d", w.Code)
					}
				}
			})

			// Test ZIP download
			t.Run("zip", func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download?file_type=zip", report.ID), nil)
				w := httptest.NewRecorder()

				server.downloadReport(w, req, report.ID, "zip")

				if tt.shouldFindZIP {
					if w.Code != http.StatusOK {
						t.Errorf("ZIP download failed: status=%d, body=%s", w.Code, w.Body.String())
					}
					if w.Header().Get("Content-Type") != "application/zip" {
						t.Errorf("wrong content type: got %s", w.Header().Get("Content-Type"))
					}
				} else {
					if w.Code != http.StatusNotFound {
						t.Errorf("expected 404 for ZIP, got %d", w.Code)
					}
				}
			})

			// Test new URL format with filename in path (PDF)
			t.Run("pdf_with_filename_in_path", func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download/%s", report.ID, tt.dbFilename), nil)
				w := httptest.NewRecorder()

				server.handleReports(w, req)

				if tt.shouldFindPDF {
					if w.Code != http.StatusOK {
						t.Errorf("PDF download via filename path failed: status=%d, body=%s", w.Code, w.Body.String())
					}
					if w.Header().Get("Content-Type") != "application/pdf" {
						t.Errorf("wrong content type: got %s", w.Header().Get("Content-Type"))
					}
				} else {
					if w.Code != http.StatusNotFound {
						t.Errorf("expected 404 for PDF, got %d", w.Code)
					}
				}
			})

			// Test new URL format with filename in path (ZIP)
			t.Run("zip_with_filename_in_path", func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/reports/%d/download/%s", report.ID, tt.dbZipFilename), nil)
				w := httptest.NewRecorder()

				server.handleReports(w, req)

				if tt.shouldFindZIP {
					if w.Code != http.StatusOK {
						t.Errorf("ZIP download via filename path failed: status=%d, body=%s", w.Code, w.Body.String())
					}
					if w.Header().Get("Content-Type") != "application/zip" {
						t.Errorf("wrong content type: got %s", w.Header().Get("Content-Type"))
					}
				} else {
					if w.Code != http.StatusNotFound {
						t.Errorf("expected 404 for ZIP, got %d", w.Code)
					}
				}
			})
		})
	}
}
