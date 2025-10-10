# Go Server Integration - Quick Reference

## Overview

The Python report generator now supports **JSON configuration files**, making it easy for the Go webserver to generate reports programmatically.

## Basic Workflow

```
Go Server → JSON Config File → Python API → PDF Files → Go Server
```

## Step-by-Step Integration

### 1. Prepare Configuration JSON

From your Go server's form data, create a JSON file:

```go
type ReportConfig struct {
    Site   SiteConfig   `json:"site"`
    Query  QueryConfig  `json:"query"`
    Output OutputConfig `json:"output"`
}

type SiteConfig struct {
    Location       string   `json:"location"`
    Surveyor       string   `json:"surveyor"`
    Contact        string   `json:"contact"`
    SpeedLimit     int      `json:"speed_limit"`
    Latitude       *float64 `json:"latitude,omitempty"`
    Longitude      *float64 `json:"longitude,omitempty"`
}

type QueryConfig struct {
    StartDate       string   `json:"start_date"`        // "2025-06-01"
    EndDate         string   `json:"end_date"`          // "2025-06-07"
    Group           string   `json:"group"`             // "1h"
    Units           string   `json:"units"`             // "mph"
    Source          string   `json:"source"`            // "radar_data_transits"
    Timezone        string   `json:"timezone"`          // "US/Pacific"
    MinSpeed        *float64 `json:"min_speed,omitempty"`
    Histogram       bool     `json:"histogram"`
    HistBucketSize  float64  `json:"hist_bucket_size"`
    HistMax         *float64 `json:"hist_max,omitempty"`
}

type OutputConfig struct {
    FilePrefix string `json:"file_prefix"`
    OutputDir  string `json:"output_dir"`
    RunID      string `json:"run_id,omitempty"`
    Debug      bool   `json:"debug"`
}

// Example: Create config from form data
func CreateReportConfig(form FormData, runID string) *ReportConfig {
    return &ReportConfig{
        Site: SiteConfig{
            Location:   form.Location,
            Surveyor:   form.Surveyor,
            Contact:    form.Contact,
            SpeedLimit: form.SpeedLimit,
        },
        Query: QueryConfig{
            StartDate:      form.StartDate,
            EndDate:        form.EndDate,
            Group:          "1h",
            Units:          "mph",
            Source:         "radar_data_transits",
            Timezone:       "US/Pacific",
            MinSpeed:       &form.MinSpeed,
            Histogram:      form.Histogram,
            HistBucketSize: 5.0,
        },
        Output: OutputConfig{
            FilePrefix: fmt.Sprintf("%s-%s", form.Location, runID),
            OutputDir:  filepath.Join("/var/reports", runID),
            RunID:      runID,
            Debug:      false,
        },
    }
}
```

### 2. Save Config to File

```go
func SaveConfig(config *ReportConfig, path string) error {
    data, err := json.MarshalIndent(config, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(path, data, 0644)
}

// Usage
configPath := filepath.Join(outputDir, "config.json")
if err := SaveConfig(config, configPath); err != nil {
    return err
}
```

### 3. Call Python API

**Option A: Direct subprocess call (recommended)**

```go
type ReportResult struct {
    Success    bool                   `json:"success"`
    Files      []string               `json:"files"`
    Prefix     string                 `json:"prefix"`
    Errors     []string               `json:"errors"`
    ConfigUsed map[string]interface{} `json:"config_used"`
}

func GenerateReport(configPath string) (*ReportResult, error) {
    cmd := exec.Command(
        "python",
        "/path/to/generate_report_api.py",
        configPath,
        "--json",
    )
    
    output, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("python command failed: %w\nOutput: %s", err, output)
    }
    
    var result ReportResult
    if err := json.Unmarshal(output, &result); err != nil {
        return nil, fmt.Errorf("failed to parse result: %w", err)
    }
    
    if !result.Success {
        return &result, fmt.Errorf("report generation failed: %v", result.Errors)
    }
    
    return &result, nil
}
```

**Option B: Call via shell script (for complex environments)**

```bash
#!/bin/bash
# generate_report.sh

SCRIPT_DIR="/path/to/query_data"
CONFIG_FILE="$1"

cd "$SCRIPT_DIR"
source venv/bin/activate  # Activate virtual environment
python generate_report_api.py "$CONFIG_FILE" --json
```

```go
func GenerateReportViaScript(configPath string) (*ReportResult, error) {
    cmd := exec.Command("/path/to/generate_report.sh", configPath)
    output, err := cmd.CombinedOutput()
    // ... same parsing as Option A
}
```

### 4. Process Results

```go
func HandleReportGeneration(ctx context.Context, runID string, formData FormData) error {
    // 1. Create output directory
    outputDir := filepath.Join("/var/reports", runID)
    if err := os.MkdirAll(outputDir, 0755); err != nil {
        return err
    }
    
    // 2. Build configuration
    config := CreateReportConfig(formData, runID)
    configPath := filepath.Join(outputDir, "config.json")
    
    // 3. Save to database
    if err := db.SaveReportConfig(ctx, runID, config); err != nil {
        return err
    }
    
    // 4. Save config file
    if err := SaveConfig(config, configPath); err != nil {
        return err
    }
    
    // 5. Generate report
    result, err := GenerateReport(configPath)
    if err != nil {
        db.UpdateReportStatus(ctx, runID, "failed", err.Error())
        return err
    }
    
    // 6. Save file paths to database
    for _, filePath := range result.Files {
        filename := filepath.Base(filePath)
        fileType := getFileType(filename)  // "report", "stats", "daily", "histogram"
        
        if err := db.SaveReportFile(ctx, runID, filename, fileType, filePath); err != nil {
            return err
        }
    }
    
    // 7. Update status
    db.UpdateReportStatus(ctx, runID, "completed", "")
    
    return nil
}

func getFileType(filename string) string {
    switch {
    case strings.Contains(filename, "_report.pdf"):
        return "report"
    case strings.Contains(filename, "_stats.pdf"):
        return "stats"
    case strings.Contains(filename, "_daily.pdf"):
        return "daily"
    case strings.Contains(filename, "_histogram.pdf"):
        return "histogram"
    default:
        return "other"
    }
}
```

## Database Schema Example

```sql
-- Reports table
CREATE TABLE reports (
    id TEXT PRIMARY KEY,
    run_id TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP,
    status TEXT NOT NULL, -- 'pending', 'generating', 'completed', 'failed'
    error_message TEXT,
    config JSON NOT NULL,
    
    -- Site info (denormalized for easy querying)
    location TEXT,
    start_date DATE,
    end_date DATE,
    
    -- User tracking
    user_id TEXT,
    deleted_at TIMESTAMP NULL
);

-- Report files table
CREATE TABLE report_files (
    id TEXT PRIMARY KEY,
    report_id TEXT REFERENCES reports(id),
    filename TEXT NOT NULL,
    file_type TEXT NOT NULL, -- 'report', 'stats', 'daily', 'histogram', 'config'
    file_path TEXT NOT NULL,
    file_size INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(report_id, file_type)
);

-- Index for cleanup job
CREATE INDEX idx_reports_deleted_at ON reports(deleted_at) WHERE deleted_at IS NOT NULL;
```

## Complete End-to-End Example

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "time"
)

// Example HTTP handler for report generation
func (h *Handler) GenerateReportHandler(w http.ResponseWriter, r *http.Request) {
    // 1. Parse form data
    var form FormData
    if err := json.NewDecoder(r.Body).Decode(&form); err != nil {
        http.Error(w, "Invalid form data", http.StatusBadRequest)
        return
    }
    
    // 2. Generate unique run ID
    runID := fmt.Sprintf("run-%s", time.Now().Format("20060102-150405"))
    
    // 3. Create output directory
    outputDir := filepath.Join("/var/reports", runID)
    if err := os.MkdirAll(outputDir, 0755); err != nil {
        http.Error(w, "Failed to create output directory", http.StatusInternalServerError)
        return
    }
    
    // 4. Build configuration
    config := &ReportConfig{
        Site: SiteConfig{
            Location:   form.Location,
            Surveyor:   form.Surveyor,
            Contact:    form.Contact,
            SpeedLimit: form.SpeedLimit,
        },
        Query: QueryConfig{
            StartDate:      form.StartDate,
            EndDate:        form.EndDate,
            Group:          "1h",
            Units:          "mph",
            Source:         "radar_data_transits",
            Timezone:       "US/Pacific",
            Histogram:      true,
            HistBucketSize: 5.0,
        },
        Output: OutputConfig{
            FilePrefix: runID,
            OutputDir:  outputDir,
            RunID:      runID,
        },
    }
    
    // 5. Save config
    configPath := filepath.Join(outputDir, "config.json")
    configData, _ := json.MarshalIndent(config, "", "  ")
    if err := os.WriteFile(configPath, configData, 0644); err != nil {
        http.Error(w, "Failed to save config", http.StatusInternalServerError)
        return
    }
    
    // 6. Save to database
    ctx := r.Context()
    if err := h.db.SaveReport(ctx, runID, config); err != nil {
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }
    
    // 7. Generate report (async)
    go func() {
        result, err := h.generateReport(configPath)
        if err != nil {
            h.db.UpdateReportStatus(context.Background(), runID, "failed", err.Error())
            return
        }
        
        // Save file references
        for _, filePath := range result.Files {
            h.db.SaveReportFile(context.Background(), runID, filePath)
        }
        
        h.db.UpdateReportStatus(context.Background(), runID, "completed", "")
    }()
    
    // 8. Return immediate response
    json.NewEncoder(w).Encode(map[string]interface{}{
        "run_id": runID,
        "status": "generating",
        "message": "Report generation started",
    })
}

func (h *Handler) generateReport(configPath string) (*ReportResult, error) {
    cmd := exec.Command(
        "python",
        "/opt/velocity-report/generate_report_api.py",
        configPath,
        "--json",
    )
    
    output, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("python failed: %w, output: %s", err, output)
    }
    
    var result ReportResult
    if err := json.Unmarshal(output, &result); err != nil {
        return nil, err
    }
    
    if !result.Success {
        return &result, fmt.Errorf("generation failed: %v", result.Errors)
    }
    
    return &result, nil
}

// Get report status
func (h *Handler) GetReportStatusHandler(w http.ResponseWriter, r *http.Request) {
    runID := r.URL.Query().Get("run_id")
    
    report, err := h.db.GetReport(r.Context(), runID)
    if err != nil {
        http.Error(w, "Report not found", http.StatusNotFound)
        return
    }
    
    files, _ := h.db.GetReportFiles(r.Context(), runID)
    
    json.NewEncoder(w).Encode(map[string]interface{}{
        "run_id":  runID,
        "status":  report.Status,
        "files":   files,
        "created": report.CreatedAt,
    })
}
```

## Configuration Field Reference

### Required Fields

```json
{
  "query": {
    "start_date": "2025-06-01",  // REQUIRED
    "end_date": "2025-06-07"     // REQUIRED
  }
}
```

### Common Fields

```json
{
  "site": {
    "location": "Main Street, Springfield",
    "surveyor": "City Traffic Department",
    "contact": "traffic@springfield.gov",
    "speed_limit": 30
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "group": "1h",               // 15m, 30m, 1h, 2h, 6h, 12h, 24h
    "units": "mph",              // mph or kph
    "timezone": "US/Pacific",
    "min_speed": 5.0,            // Filter out speeds below this
    "histogram": true,
    "hist_bucket_size": 5.0
  },
  "output": {
    "file_prefix": "main-st-report",
    "output_dir": "/var/reports/run-123",
    "run_id": "run-123"
  }
}
```

### All Available Fields

See `CONFIG_SYSTEM.md` for complete field reference.

## Error Handling

```go
func (h *Handler) generateReportSafe(configPath string) (*ReportResult, error) {
    // Set timeout
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, "python", "generate_report_api.py", configPath, "--json")
    
    output, err := cmd.CombinedOutput()
    if err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            return nil, fmt.Errorf("report generation timed out after 5 minutes")
        }
        return nil, fmt.Errorf("python failed: %w\nOutput: %s", err, output)
    }
    
    var result ReportResult
    if err := json.Unmarshal(output, &result); err != nil {
        return nil, fmt.Errorf("invalid JSON response: %w\nOutput: %s", err, output)
    }
    
    if !result.Success {
        return &result, fmt.Errorf("report generation failed: %v", result.Errors)
    }
    
    return &result, nil
}
```

## Cleanup Job

```go
// Scheduled job to clean up old deleted reports
func (h *Handler) CleanupOldReports(ctx context.Context) error {
    // Find reports marked for deletion > 30 days ago
    cutoff := time.Now().AddDate(0, 0, -30)
    
    reports, err := h.db.GetDeletedReportsBefore(ctx, cutoff)
    if err != nil {
        return err
    }
    
    for _, report := range reports {
        // Delete files
        files, _ := h.db.GetReportFiles(ctx, report.RunID)
        for _, file := range files {
            os.Remove(file.FilePath)
        }
        
        // Delete output directory
        outputDir := filepath.Join("/var/reports", report.RunID)
        os.RemoveAll(outputDir)
        
        // Delete from database
        h.db.DeleteReport(ctx, report.RunID)
    }
    
    return nil
}
```

## Testing

```bash
# 1. Create test config
cat > /tmp/test_config.json << 'EOF'
{
  "site": {
    "location": "Test Location"
  },
  "query": {
    "start_date": "2025-06-02",
    "end_date": "2025-06-04",
    "histogram": true,
    "hist_bucket_size": 5.0
  },
  "output": {
    "output_dir": "/tmp/test-report"
  }
}
EOF

# 2. Test Python API directly
python generate_report_api.py /tmp/test_config.json --json

# 3. Verify output
ls -lh /tmp/test-report/
```

## Deployment

```bash
# Install Python dependencies
cd /opt/velocity-report/internal/report/query_data
python -m venv venv
source venv/bin/activate
pip install -r requirements.txt

# Make scripts executable
chmod +x generate_report_api.py

# Test from Go server user
sudo -u go-server python generate_report_api.py /tmp/test_config.json --json
```

## Support

For questions or issues:
- See `CONFIG_SYSTEM.md` for detailed documentation
- Check example config: `report_config_example.json`
- Test CLI: `python get_stats.py --help`
- Test API: `python generate_report_api.py --help`
