---
layout: doc.njk
title: Generate Your First PDF Report
description: Step-by-step guide to creating your first professional citizen radar report
section: guides
date: 2025-10-21
---

## Prerequisites

Before generating your first report, ensure you have:

- ✅ velocity.report server running
- ✅ At least 24 hours of traffic data collected
- ✅ Python PDF generator environment set up

If you haven't set up the PDF generator yet, see the [Software Installation Guide](/getting-started/software-installation/).

## Step 1: Access the Web Dashboard

Navigate to your dashboard at `http://localhost:8080/app`

The dashboard provides real-time visualization of your citizen radar data and access to all site management features.

## Step 2: Navigate to Site Management

1. Click on **"Sites"** in the navigation menu
2. Select the site you want to generate a report for
3. You'll see the site configuration page with report generation options

## Step 3: Configure Report Parameters

Fill in the report generation form with these key parameters:

### Date Range

Select the start and end dates for your analysis:

```bash
Start Date: 2025-10-01
End Date: 2025-10-14
```

**Tip**: For meaningful statistics, collect at least 3-7 days of data. Longer periods provide better trend analysis.

### Timezone

Choose your local timezone to ensure timestamps are correctly displayed:

```bash
Timezone: US/Pacific
```

Available timezones include all standard IANA timezone identifiers (e.g., `US/Eastern`, `US/Central`, `US/Mountain`, `UTC`).

### Units

Select your preferred speed units:

- **mph** - Miles per hour (United States)
- **kph** - Kilometers per hour (International)

### Data Grouping

Choose how to aggregate your data:

| Grouping | Best For                     |
| -------- | ---------------------------- |
| `1h`     | Detailed hourly analysis     |
| `4h`     | Daily pattern identification |
| `24h`    | Day-by-day comparison        |

### Advanced Options

**Data Source**:

- `radar_objects` - Raw radar detections
- `radar_data_transits` - Processed vehicle transits (recommended)

**Minimum Speed**: Filter out slow-moving objects (default: 5 mph)

**Histogram**: Enable to include speed distribution visualization

## Step 4: Generate the Report

Click the **"Generate Report"** button and wait for processing.

⏱️ **Processing Time**: Typically 30-60 seconds depending on data volume.

You'll see a success message when the report is ready.

## Step 5: Download Your Report

Once complete:

1. Click **"Download PDF"** to save your report
2. Optionally, download the **ZIP file** containing source data and LaTeX files

### Report Filename Format

Reports are automatically named with the `velocity.report_` prefix:

```
velocity.report_radar_data_transits_2025-10-01_to_2025-10-14_report.pdf
```

## Understanding Your Report

Your PDF report includes several key sections:

### 1. Summary Statistics

- **Total Vehicle Count**: Number of vehicles detected
- **Average Speed**: Mean speed of all vehicles
- **85th Percentile Speed (V85)**: Speed at or below which 85% of vehicles travel
- **Speed Limit Compliance**: Percentage of vehicles within posted limits

### 2. Speed Distribution Histogram

Visual representation showing:

- Number of vehicles in each speed bin
- Posted speed limit overlay
- Normal distribution curve

### 3. Time-Series Analysis

Charts showing:

- Speed trends over time
- Peak hour patterns
- Day-of-week variations

### 4. Professional Formatting

- Customizable header with your organization name
- Date range and site information
- Data source citation and methodology notes
- Cosine error correction details

## Example Report Output

Here's what a typical report section looks like:

```latex
Site: Main Street & Oak Avenue
Survey Period: October 1-14, 2025
Total Vehicles: 12,547
Average Speed: 28.3 mph
85th Percentile: 32.1 mph
Posted Speed Limit: 25 mph
```

## Next Steps

Now that you've generated your first report, explore these advanced features:

- [Customize Report Appearance](/guides/report-customization/) - Add logos, change colors, modify layout
- [Compare Multiple Sites](/guides/multi-site-analysis/) - Generate comparative reports
- [Schedule Automated Reports](/guides/automation/) - Set up recurring report generation
- [Understanding Traffic Statistics](/guides/traffic-statistics/) - Deep dive into metrics

## Troubleshooting

### Report Generation Fails

**Symptom**: "Python environment not found" error

**Solution**: Ensure PDF generator virtual environment is activated:

```bash
cd tools/pdf-generator
source .venv/bin/activate
python --version  # Should show Python 3.13+
```

### Missing Data in Report

**Symptom**: "No data for selected date range"

**Possible causes**:

1. Data collection wasn't running during selected period
2. Database file path is incorrect
3. Date range is in the future

**Solution**: Verify data exists:

```bash
# Check database for data
sqlite3 sensor_data.db
SELECT COUNT(*) FROM radar_data_transits
WHERE timestamp >= '2025-10-01' AND timestamp <= '2025-10-14';
```

### LaTeX Compilation Errors

**Symptom**: PDF generation succeeds but file is empty or corrupted

**Solution**: Check LaTeX installation:

```bash
which pdflatex
pdflatex --version
```

Install required packages:

```bash
# Ubuntu/Debian
sudo apt-get install texlive-full

# macOS
brew install --cask mactex
```

## Tips for Better Reports

1. **Collect longer time periods** - 7-14 days provides statistically significant data
2. **Include multiple time periods** - Compare weekdays vs weekends
3. **Document weather conditions** - Note unusual conditions in report annotations
4. **Calibrate radar angle** - Accurate cosine error correction is critical
5. **Review raw data first** - Use dashboard to identify any data quality issues

## Need Help?

- **Discord Community**: [Join our Discord](https://discord.gg/XXh6jXVFkt) for real-time help
- **GitHub Issues**: [Report bugs or request features](https://github.com/banshee-data/velocity.report/issues)
- **Email Support**: For sensitive questions about your deployment

---

**Last Updated**: October 21, 2025
**Applies to**: velocity.report v0.5.0+
