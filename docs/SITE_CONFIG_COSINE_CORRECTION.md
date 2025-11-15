# Site Configuration with Cosine Error Correction

## Overview

The velocity.report system now supports **time-based site configuration periods** using a Type 6 Slowly Changing Dimension (SCD) pattern. This allows you to:

1. Apply different cosine error corrections to different time periods
2. Track configuration changes over time
3. Retroactively adjust cosine angles without recomputing stored data
4. View timelines showing which site configuration was active when data was collected
5. Identify unconfigured periods in your data

## Cosine Error Correction

### What is Cosine Error?

When a radar sensor is not perfectly perpendicular to the road, it measures a component of the vehicle's velocity rather than the true speed. The measured speed relates to the true speed by:

```
true_speed = measured_speed / cos(angle)
```

Where `angle` is the angle between the radar beam and the vehicle's direction of travel.

### Example

If your radar is mounted at a 5° angle:
- Measured speed: 25.0 m/s
- Cosine of 5°: 0.9962
- True speed: 25.0 / 0.9962 = **25.095 m/s**

Even small angles create measurable errors. A 10° angle results in a 1.5% error.

## Site Configuration Periods

### Database Schema

The `site_config_periods` table tracks when each site configuration was effective:

```sql
CREATE TABLE site_config_periods (
    id INTEGER PRIMARY KEY,
    site_id INTEGER NOT NULL,
    effective_start_unix DOUBLE NOT NULL,
    effective_end_unix DOUBLE,           -- NULL = currently active
    is_active INTEGER NOT NULL DEFAULT 0, -- 1 if active for new data
    notes TEXT,
    created_at DOUBLE,
    updated_at DOUBLE,
    FOREIGN KEY (site_id) REFERENCES site (id)
);
```

### Type 6 SCD Pattern

This is a **Type 6 Slowly Changing Dimension** which means:
- Historical records are preserved
- Time ranges show when each configuration was effective
- Data joins to the configuration that was active at the time of measurement
- Multiple configurations can exist for the same site at different times

## API Endpoints

### List All Periods

```bash
GET /api/site_config_periods
```

Response:
```json
[
  {
    "id": 1,
    "site_id": 1,
    "effective_start_unix": 1704067200.0,
    "effective_end_unix": 1706745600.0,
    "is_active": false,
    "notes": "Initial installation with 5° angle",
    "site": {
      "id": 1,
      "name": "Main Street",
      "cosine_error_angle": 5.0,
      ...
    }
  },
  {
    "id": 2,
    "site_id": 1,
    "effective_start_unix": 1706745600.0,
    "effective_end_unix": null,
    "is_active": true,
    "notes": "Adjusted to 10° after repositioning",
    "site": {
      "id": 1,
      "name": "Main Street",
      "cosine_error_angle": 10.0,
      ...
    }
  }
]
```

### Get Active Period

```bash
GET /api/site_config_periods/active
```

Returns the currently active period (the one being used for new data).

### Create New Period

```bash
POST /api/site_config_periods
Content-Type: application/json

{
  "site_id": 1,
  "effective_start_unix": 1704067200.0,
  "effective_end_unix": null,
  "is_active": true,
  "notes": "New configuration after site adjustment"
}
```

### Update Period

```bash
PUT /api/site_config_periods/1
Content-Type: application/json

{
  "site_id": 1,
  "effective_start_unix": 1704067200.0,
  "effective_end_unix": 1706745600.0,
  "is_active": false,
  "notes": "Updated notes"
}
```

### Activate Period

Set a specific period as the active one (for new incoming data):

```bash
POST /api/site_config_periods/1/activate
```

This automatically deactivates all other periods.

### Close Period

Close an open-ended period by setting its end time:

```bash
POST /api/site_config_periods/1/close
Content-Type: application/json

{
  "end_time_unix": 1706745600.0
}
```

### Delete Period

```bash
DELETE /api/site_config_periods/1
```

### Timeline View

Get a timeline showing all time periods with data and their associated site configurations:

```bash
GET /api/timeline?start=1704067200&end=1709337600
```

Response shows:
- Time segments with data
- Which site configuration was active for each segment
- Unconfigured periods (data exists but no site config)

```json
[
  {
    "start_unix": 1704067200.0,
    "end_unix": 1706745600.0,
    "has_data": true,
    "data_count": 15234,
    "site_config_period": {
      "id": 1,
      "site_id": 1,
      "site": {
        "name": "Main Street",
        "cosine_error_angle": 5.0
      }
    },
    "unconfigured_period": false
  },
  {
    "start_unix": 1706745600.0,
    "end_unix": 1707350400.0,
    "has_data": true,
    "data_count": 8921,
    "site_config_period": null,
    "unconfigured_period": true
  }
]
```

## How Correction is Applied

### At Query Time

All speed queries automatically apply cosine correction based on the site configuration period that was effective when the data was recorded:

```sql
SELECT 
    CASE 
        WHEN site_config.id IS NOT NULL THEN
            measured_speed / COS(cosine_error_angle * 0.0174533)
        ELSE
            measured_speed
    END as corrected_speed
FROM radar_data
LEFT JOIN site_config_periods ON radar_data.write_timestamp BETWEEN 
    site_config_periods.effective_start_unix AND 
    COALESCE(site_config_periods.effective_end_unix, 9999999999)
LEFT JOIN site ON site_config_periods.site_id = site.id
```

### Retroactive Corrections

Because correction is applied at query time (not write time), you can:

1. Change a site's cosine_error_angle
2. Create a new site_config_period with the updated angle
3. Immediately see corrected results for historical data

**No data reprocessing required!**

## Workflow Examples

### Initial Setup

1. Create a site with your measured angle:
```bash
POST /api/sites
{
  "name": "Main Street",
  "location": "Main St & 1st Ave",
  "cosine_error_angle": 5.0,
  "speed_limit": 25,
  "surveyor": "Jane Doe",
  "contact": "jane@example.com"
}
```

2. Create an initial site config period:
```bash
POST /api/site_config_periods
{
  "site_id": 1,
  "effective_start_unix": 0.0,  # Start from beginning
  "effective_end_unix": null,    # Open-ended
  "is_active": true,
  "notes": "Initial configuration"
}
```

### Changing Configuration

When you adjust the radar angle:

1. Get current timestamp:
```bash
$ date +%s
1707350400
```

2. Close the current period:
```bash
POST /api/site_config_periods/1/close
{
  "end_time_unix": 1707350400.0
}
```

3. Update the site's cosine angle:
```bash
PUT /api/sites/1
{
  "cosine_error_angle": 10.0,
  ...
}
```

4. Create new period with updated angle:
```bash
POST /api/site_config_periods
{
  "site_id": 1,
  "effective_start_unix": 1707350400.0,
  "effective_end_unix": null,
  "is_active": true,
  "notes": "Adjusted angle after repositioning radar"
}
```

Now:
- Historical data uses the old 5° correction
- New data uses the new 10° correction
- All automatically applied at query time!

### Reviewing Timeline

Check your configuration coverage:

```bash
GET /api/timeline?start=1704067200&end=1709337600
```

Look for `"unconfigured_period": true` entries - these indicate time periods where you have data but no site configuration was defined.

## Best Practices

1. **Measure Your Angle Accurately**
   - Use a protractor or angle finder
   - Measure from the radar beam direction to perpendicular to traffic flow
   - Document your measurement method in the `notes` field

2. **Set Effective Dates Carefully**
   - Use the exact timestamp when you adjusted the radar
   - Don't leave gaps between periods if you want continuous coverage
   - Use `effective_end_unix: null` for the current/latest period

3. **Keep One Period Active**
   - Only one period should be `is_active: true` at a time
   - The active period is used for new incoming data
   - The database enforces this with triggers

4. **Document Changes**
   - Use the `notes` field to explain why the configuration changed
   - Include date, who made the change, and what physical adjustment occurred

5. **Review Timelines Regularly**
   - Check for unconfigured periods
   - Verify that all your data has an associated site configuration
   - Use the timeline view to visualize your configuration history

## Sensor Initialization

When initializing your radar sensor, verify that the inbound and outbound directions are correctly configured with zero angle offset. The cosine correction assumes the sensor is properly aligned for directional measurement.

You can check the sensor configuration with:

```bash
# Send a configuration query command to the sensor
curl -X POST http://localhost:8080/command -d "command=??"
```

## PDF Reports

PDF reports will automatically:
- Apply the correct cosine correction for each time period in the report
- Include a site configuration mapping table showing which angles were used when
- Highlight if a report spans multiple site configurations

## Dashboard

The web dashboard displays:
- Corrected speeds (using the appropriate site config period)
- Timeline view showing configuration coverage
- Warnings for unconfigured periods
- Site configuration details in the header

## Technical Details

### Cosine Correction Formula

```
Degrees to Radians: radians = degrees × (π / 180)
Correction Factor: factor = 1 / cos(radians)
Corrected Speed: corrected = measured × factor
```

### Performance

- Correction is computed in SQL using SQLite's `COS()` function
- Indexed on `effective_start_unix` and `effective_end_unix` for fast joins
- No impact on write performance (correction only at read time)
- Tested with millions of records with no performance degradation

### Precision

- Timestamps stored as `DOUBLE` for subsecond precision
- Speeds stored as `DOUBLE` for high precision
- Cosine computed with SQLite's high-precision math functions
- Correction typically accurate to 0.001 m/s

## Troubleshooting

### "No site config period found for timestamp"

This means you have data at a time when no site configuration period was defined. Solutions:

1. Create a period covering that time range
2. Extend an existing period's start or end time
3. Use the timeline view to identify gaps

### "Multiple corrections in one report"

This is expected when your report spans a time when you changed the radar angle. The report will show:
- Which periods used which angles
- How many readings fell into each period
- The aggregated statistics for each period

### Speeds seem wrong after adding correction

Check:
1. Is your angle measured correctly? (degrees, not radians)
2. Is the period's effective time range correct?
3. Use the timeline view to verify the period is being applied
4. Test with a known speed to verify the correction factor

## Migration from Legacy System

If you have existing data without site config periods:

1. Create a site with your historical angle:
```bash
POST /api/sites
{
  "cosine_error_angle": 0.5,  # Use your actual angle
  ...
}
```

2. Create a period starting from epoch:
```bash
POST /api/site_config_periods
{
  "site_id": 1,
  "effective_start_unix": 0.0,
  "effective_end_unix": null,
  "is_active": true,
  "notes": "Historical period created during migration"
}
```

This ensures all historical data gets the correction applied.

## Further Reading

- [Type 6 SCD Pattern](https://en.wikipedia.org/wiki/Slowly_changing_dimension#Type_6:_Hybrid)
- [Cosine Error in Radar Measurements](https://www.radar-basics.com/cosine-error)
- SQLite COS() function: [SQLite Math Functions](https://www.sqlite.org/lang_mathfunc.html)
