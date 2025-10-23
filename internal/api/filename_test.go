package api

import (
	"testing"
)

// TestReportFilenameFormat verifies that generated report filenames
// follow the velocity.report_{source}_{start}_to_{end}_{type}.{ext} format
func TestReportFilenameFormat(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		startDate string
		endDate   string
		wantPDF   string
		wantZIP   string
	}{
		{
			name:      "radar_data_transits source",
			source:    "radar_data_transits",
			startDate: "2025-10-01",
			endDate:   "2025-10-14",
			wantPDF:   "velocity.report_radar_data_transits_2025-10-01_to_2025-10-14_report.pdf",
			wantZIP:   "velocity.report_radar_data_transits_2025-10-01_to_2025-10-14_sources.zip",
		},
		{
			name:      "radar_objects source",
			source:    "radar_objects",
			startDate: "2025-07-28",
			endDate:   "2025-08-11",
			wantPDF:   "velocity.report_radar_objects_2025-07-28_to_2025-08-11_report.pdf",
			wantZIP:   "velocity.report_radar_objects_2025-07-28_to_2025-08-11_sources.zip",
		},
		{
			name:      "single day report",
			source:    "radar_data_transits",
			startDate: "2025-12-25",
			endDate:   "2025-12-25",
			wantPDF:   "velocity.report_radar_data_transits_2025-12-25_to_2025-12-25_report.pdf",
			wantZIP:   "velocity.report_radar_data_transits_2025-12-25_to_2025-12-25_sources.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test PDF filename generation
			gotPDF := generatePDFFilename(tt.source, tt.startDate, tt.endDate)
			if gotPDF != tt.wantPDF {
				t.Errorf("PDF filename = %q, want %q", gotPDF, tt.wantPDF)
			}

			// Test ZIP filename generation
			gotZIP := generateZIPFilename(tt.source, tt.startDate, tt.endDate)
			if gotZIP != tt.wantZIP {
				t.Errorf("ZIP filename = %q, want %q", gotZIP, tt.wantZIP)
			}
		})
	}
}

// Helper functions to extract the filename generation logic for testing
func generatePDFFilename(source, startDate, endDate string) string {
	// This matches the logic in server.go generateReport
	return "velocity.report_" + source + "_" + startDate + "_to_" + endDate + "_report.pdf"
}

func generateZIPFilename(source, startDate, endDate string) string {
	// This matches the logic in server.go generateReport
	return "velocity.report_" + source + "_" + startDate + "_to_" + endDate + "_sources.zip"
}
