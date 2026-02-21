# Feature Specification: Site Configuration with Time-Based Cosine Error Correction

Status: Proposed
Target Directory: docs/features/

**Status:** Proposed / Design Phase
**Date:** 2025-11-07 (Revised 2026-01-19)
**Author:** Ictinus (Product Architecture Agent)

## Problem Statement

Users need the ability to:

1. **Configure cosine error angle correction** - Radar sensors are often not perfectly perpendicular to traffic, introducing cosine error in speed measurements.
2. **Track configuration changes over time** - When sensors are adjusted or repositioned, different correction angles apply to different time periods.
3. **Apply corrections retroactively** - Users may realize the angle was wrong after data collection and need to correct historical data without recomputation.
4. **Visualize configuration coverage** - Identify time periods where data exists but no site configuration is assigned.
5. **Support multiple configurations in reports** - A single report may span multiple days with different sensor angles.
6. **Support consistent comparisons** - When comparing two different time periods (e.g., "This Week vs Last Week"), ensure that the correct angle correction is applied to each period independently to allow for accurate velocity comparisons.

## User Value Proposition

**For community advocates and traffic engineers:**

- Accurate speed measurements despite imperfect sensor positioning
- Clear audit trail of sensor configuration changes
- Ability to correct measurement errors discovered after the fact
- Professional reports that account for configuration variations
- Confidence in data accuracy for decision-making
- **Accurate Trend Analysis:** When comparing traffic data pre- and post-intervention, or week-over-week, ensures that sensor adjustments don't masquerade as changes in driver behavior.

## Current System Capabilities

### Existing Infrastructure

**Database:**

- `site` table exists with `cosine_error_angle` field (single value, no history).
- SQLite database with subsecond timestamp precision.
- `radar_data` and `radar_objects` tables store raw measurements.

**API:**

- `/api/sites` endpoints for site CRUD operations.
- `/api/radar_stats` for querying speed statistics.
- Unit conversion and timezone handling implemented.

**iReports & Visualisation:**

- PDF generator via Python/LaTeX.
- Statistical summaries (P50, P85, P98).
- **Report Comparison:** The system supports generating reports that compare two data sets (e.g., "Main Period" t1 vs "Comparison Period" t2).

### Gaps Identified

1. **No time-based configuration tracking** - Current `site.cosine_error_angle` is a single value; changing it affects all historical data.
2. **No correction application** - Queries return raw measured speeds, not corrected speeds.
3. **No configuration timeline view** - Can't visualise which config was active when.
4. **No active site concept** - No way to mark which site config applies to incoming data.
5. **Comparison Validity:** Currently, if a sensor is moved/adjusted, comparing data before and after the move is invalid because the cosine error changes.

## Solution Design: Type 6 Slowly Changing Dimension

### Architecture Decision

**Pattern Selected:** Type 6 SCD (Hybrid Temporal Tracking)

**Rationale:**

- Preserves complete history of configuration changes.
- Enables point-in-time queries (join on timestamp).
- Supports retroactive corrections (compute at read time, not write time).
- Standard data warehousing pattern with well-understood semantics.

### Proposed Schema

```sql
CREATE TABLE site_config_periods (
    id INTEGER PRIMARY KEY,
    site_id INTEGER NOT NULL,
    effective_start_unix DOUBLE NOT NULL,
    effective_end_unix DOUBLE,           -- NULL = currently active/open-ended
    is_active INTEGER NOT NULL DEFAULT 0, -- 1 if active for new data
    notes TEXT,
    created_at DOUBLE,
    updated_at DOUBLE,
    cosine_error_angle DOUBLE NOT NULL DEFAULT 0, -- Store angle here for history
    FOREIGN KEY (site_id) REFERENCES site (id) ON DELETE CASCADE
);
```

**Key Design Choices:**

- **DOUBLE timestamps** - Subsecond precision matches existing `radar_data.write_timestamp`.
- **Nullable end time** - Open-ended periods support ongoing data collection.
- **is_active flag** - Marks which period applies to new incoming data (only one can be active).
- **Snapshot Values**: Store the `cosine_error_angle` directly in the period row.

### Cosine Correction Formula

```
corrected_speed = measured_speed / cos(angle_in_radians)
```

Where:

- `angle_in_radians = cosine_error_angle × (π / 180)`
- Applied via SQL: `measured_speed / COS(angle * 0.0174533)`

**Implementation Location:** Query time (SELECT statements), not write time (INSERT statements).

## Proposed Implementation Plan

### 1. Database Schema Changes

- Create `site_config_periods` table.
- Migrate existing `site.cosine_error_angle` to an initial "forever" period (start=0, end=NULL) to preserve backward compatibility.
- Add triggers to enforce "Single Active Period" constraint.

### 2. Backend Query Refactoring (Go)

Modify all speed-related queries in `internal/db/` to join with `site_config_periods`:

- **Logic:** `LEFT JOIN site_config_periods ON ... AND timestamp >= start AND (timestamp < end OR end IS NULL)`
- **Correction:** Return `speed / COS(angle)` as the authoritative speed column.
- **Fallback:** If no period matches (shouldn't happen if migrated correctly), return raw speed (implying 0° angle).

### 3. API Updates

**New Endpoints:**

- `GET /api/site_config_periods` (List)
- `POST /api/site_config_periods` (Create/Update)
- `GET /api/timeline` (Visualize data coverage vs config coverage)

**Modified Responses:**

- Stats and Speed APIs should transparently return corrected values.

### 4. Integration with Report Comparison

**Challenge:**
A comparison report involves two distinct time ranges (e.g., Range A: Jan 1-7, Range B: Jan 14-21). A sensor adjustment might occur on Jan 10.

**Requirement:**

1.  **Independent Correction:** Data from Range A must be corrected using the Jan 1-7 config. Data from Range B must be corrected using the Jan 14-21 config.
2.  **User Awareness:** The report must explicitly state if different angles were used.
    - _Good Scenario:_ "Comparing Week 1 (Angle 5°) vs Week 2 (Angle 5°)"
    - _Adjusted Scenario:_ "Comparing Week 1 (Angle 5°) vs Week 2 (Angle 12°)" - Note: "Speeds have been corrected for sensor angle change."
3.  **PDF Table:** The "Site Configuration" section in the PDF must list all configuration periods active during _any_ part of the report window (Main + Comparison).

### 5. Frontend (Svelte)

- **Timeline UI:** New view to show periods on a timeline.
- **Speed Displays:** Add indicator (e.g., tooltip or icon) showing "Corrected Speed".
- **Management UI:** Forms to add/edit historical periods.

## What Remains To Be Done

### High Priority

#### 1. Database Schema & Migration

- Write `up.sql` migration for `site_config_periods`.
- Implementation of `db.go` lookup logic.

#### 2. Core Stats Query Updates

- Update `GetRadarStats`, `GetSpeedPercentiles` to join on config periods.
- Ensure consistent behavior for raw measurements queries.

#### 3. PDF Generator Updates for Comparison

- Update Python PDF generator to fetch config history.
- Logic to map periods to the "Main" and "Comparison" date ranges.
- Render config table with date ranges.

### Medium Priority

#### 4. Frontend Timeline Component

- Visualise when the sensor was moved vs when data was recorded.
- Highlight "Unconfigured" gaps (if any).

## Testing Strategy

1.  **Unit Tests:**
    - Test cosine math logic.
    - Test period overlap prevention.
    - Test "No Config" fallback behavior.
2.  **Comparison Tests:**
    - Create synthetic data with known speeds.
    - Simulate a sensor move (Change in angle).
    - Verify that "Pre-move" and "Post-move" data is corrected to the _same_ true speed.

## Assumptions

1.  Users manually record when they moved the sensor.
2.  The effective time of a configuration change is known to the second.
3.  For "Report Comparison", users want to compare _Corrected_ speeds (true velocity), not Raw speeds (what the sensor saw).

---

**Document Version:** 2.0 (Draft)
**Next Step:** Implementation
