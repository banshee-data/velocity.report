package api

import "testing"

func TestOutputIndicatesReportFailure(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name:   "successful output",
			output: "Completed 2026-01-01 -> 2026-01-02. PDF and charts use prefix 'x'.",
			want:   false,
		},
		{
			name:   "explicit range failure marker",
			output: "Failed to complete report for 2026-01-01 -> 2026-01-02. See errors above.",
			want:   true,
		},
		{
			name:   "explicit generator failure marker",
			output: "Error: failed to generate PDF report. Ensure XeLaTeX is installed.",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outputIndicatesReportFailure(tt.output)
			if got != tt.want {
				t.Fatalf("outputIndicatesReportFailure() = %v, want %v", got, tt.want)
			}
		})
	}
}
