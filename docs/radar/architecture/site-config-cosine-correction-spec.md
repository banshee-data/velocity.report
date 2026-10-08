# Site configuration with time-based cosine error correction

- **Status:** Implemented

Specifies how radar sites store and apply cosine angle correction factors to compensate for sensors that are not perpendicular to the traffic flow, including time-based configuration periods so corrections can change when a sensor is repositioned.

## Problem statement

Users need the ability to:

1. **Configure cosine error angle correction** - Radar sensors are often not perfectly perpendicular to traffic, introducing cosine error in speed measurements.
2. **Track configuration changes over time** - When sensors are adjusted or repositioned, different correction angles apply to different time periods.
3. **Apply corrections retroactively** - Users may realize the angle was wrong after data collection and need to correct historical data without recomputation.
4. **Visualise configuration coverage** - Identify time periods where data exists but no site configuration is assigned.
5. **Support multiple configurations in reports** - A single report may span multiple days with different sensor angles.
6. **Support consistent comparisons** - When comparing two different time periods (e.g., "This Week vs Last Week"), ensure that the correct angle correction is applied to each period independently to allow for accurate velocity comparisons.

## Problem

**For community advocates and traffic engineers:**

- Accurate speed measurements despite imperfect sensor positioning
- Clear audit trail of sensor configuration changes
- Ability to correct measurement errors discovered after the fact
- Professional reports that account for configuration variations
- Confidence in data accuracy for decision-making
- **Accurate Trend Analysis:** When comparing traffic data pre- and post-intervention, or week-over-week, ensures that sensor adjustments don't masquerade as changes in driver behaviour.

## Current system capabilities

### Existing infrastructure

**Database:**

- `site` table exists with `cosine_error_angle` field (single value, no history).
- SQLite database with subsecond timestamp precision.
- `radar_data` and `radar_objects` tables store raw measurements.

**API:**

- `/api/sites` endpoints for site CRUD operations.
- `/api/radar_stats` for querying speed statistics.
- Unit conversion and timezone handling implemented.

**iReports & Visualisation:**

- PDF generator (Go pipeline, `internal/report/`).
- Statistical summaries (P50, P85, P98).
- **Report Comparison:** The system supports generating reports that compare two data sets (e.g., "Main Period" t1 vs "Comparison Period" t2).

### Gaps identified

1. ~~**No time-based configuration tracking**~~: ✅ Resolved: `site_config_periods` table with SCD Type 6 pattern.
2. ~~**No correction application**~~: ✅ Resolved: `buildCosineSpeedExpr()` applies correction at query time.
3. ~~**No configuration timeline view**~~: ✅ Resolved: `GET /api/timeline` returns periods and unconfigured gaps.
4. ~~**No active site concept**~~: ✅ Resolved: `is_active` flag with single-active trigger constraint.
5. **Comparison Validity:** Currently, if a sensor is moved/adjusted, comparing data before and after the move is invalid because the cosine error changes.

## Solution design: type 6 slowly changing dimension

### Architecture decision

**Pattern Selected:** Type 6 SCD (Hybrid Temporal Tracking)

**Rationale:**

- Preserves complete history of configuration changes.
- Enables point-in-time queries (join on timestamp).
- Supports retroactive corrections (compute at read time, not write time).
- Standard data warehousing pattern with well-understood semantics.

### Proposed schema

The `site_config_periods` table uses a Type 6 SCD pattern:

| Column                 | Type    | Constraint / Default                                 |
| ---------------------- | ------- | ---------------------------------------------------- |
| `id`                   | INTEGER | PRIMARY KEY                                          |
| `site_id`              | INTEGER | NOT NULL, FOREIGN KEY → `site(id)` ON DELETE CASCADE |
| `effective_start_unix` | DOUBLE  | NOT NULL                                             |
| `effective_end_unix`   | DOUBLE  | NULL = currently active / open-ended                 |
| `is_active`            | INTEGER | NOT NULL DEFAULT 0 (1 = active for new data)         |
| `notes`                | TEXT    |                                                      |
| `created_at`           | DOUBLE  |                                                      |
| `updated_at`           | DOUBLE  |                                                      |
| `cosine_error_angle`   | DOUBLE  | NOT NULL DEFAULT 0                                   |

**Key Design Choices:**

- **DOUBLE timestamps** - Subsecond precision matches existing `radar_data.write_timestamp`.
- **Nullable end time** - Open-ended periods support ongoing data collection.
- **is_active flag** - Marks which period applies to new incoming data (only one can be active).
- **Snapshot Values**: Store the `cosine_error_angle` directly in the period row.

### Cosine correction formula

```
corrected_speed = measured_speed / cos(angle_in_radians)
```

Where:

- `angle_in_radians = cosine_error_angle × (π / 180)`
- Applied via SQL: `measured_speed / COS(angle * 0.0174533)`

**Implementation Location:** Query time (SELECT statements), not write time (INSERT statements).

## Proposed implementation plan

### 1. Database schema changes

- Create `site_config_periods` table.
- Migrate existing `site.cosine_error_angle` to an initial "forever" period (start=0, end=NULL) to preserve backward compatibility.
- Add triggers to enforce "Single Active Period" constraint.

### 2. Backend query refactoring (Go)

Modify all speed-related queries in [internal/db/](../../../internal/db) to join with `site_config_periods`:

- **Logic:** `LEFT JOIN site_config_periods ON ... AND timestamp >= start AND (timestamp < end OR end IS NULL)`
- **Correction:** Return `speed / COS(angle)` as the authoritative speed column.
- **Fallback:** If no period matches (shouldn't happen if migrated correctly), return raw speed (implying 0° angle).

### 3. API updates

**New Endpoints:**

- `GET /api/site_config_periods` (List)
- `POST /api/site_config_periods` (Create/Update)
- `GET /api/timeline` (Visualise data coverage vs config coverage)

**Modified Responses:**

- Stats and Speed APIs should transparently return corrected values.

### 4. Integration with report comparison

**Challenge:**
A comparison report involves two distinct time ranges (e.g., Range A: Jan 1-7, Range B: Jan 14-21). A sensor adjustment might occur on Jan 10.

**Requirement:**

1.  **Independent Correction:** Data from Range A must be corrected using the Jan 1-7 config. Data from Range B must be corrected using the Jan 14-21 config.
2.  **User Awareness:** The report must explicitly state if different angles were used.
    - _Good Scenario:_ "Comparing Week 1 (Angle 5°) vs Week 2 (Angle 5°)"
    - _Adjusted Scenario:_ "Comparing Week 1 (Angle 5°) vs Week 2 (Angle 12°)" - Note: "Speeds have been corrected for sensor angle change."
3.  **PDF Table:** The "Site Configuration" section in the PDF must list all configuration periods active during _any_ part of the report window (Main + Comparison).
4.  **Truthful statement:** A report may print the cosine angle and factor rows, and the "speeds have been corrected" note, only when the statistics it prints were fetched through the `site_config_periods` join. The join applies to every source the same way, `radar_data_transits` (the default report source) included. A report generated without a site prints an explicit "no sensor-angle correction was applied" note instead of staying silent, so a reader can always tell corrected figures from raw ones. The before-and-after model in the [crash-data integration plan](../../plans/platform-crash-data-integration-plan.md) refuses comparisons across differing corrections, and it relies on this statement being accurate.

### 5. Frontend (Svelte)

- **Timeline UI:** New view to show periods on a timeline.
- **Speed Displays:** Add indicator (e.g., tooltip or icon) showing "Corrected Speed".
- **Management UI:** Forms to add/edit historical periods.

## What remains to be done

### Completed (all layers)

- ✅ **Database schema & migration**: `site_config_periods` table created (migration 000013); legacy columns removed from `site` (migration 000014). SCD Type 6 pattern with overlap-prevention triggers and single-active-period constraint.
- ✅ **Core stats query updates**: `buildCosineSpeedExpr()` in `db.go` joins `site_config_periods` by timestamp range and applies `speed / COS(angle × π/180)` correction. Used in `RadarObjectRollupRange()` for all three data sources.
- ✅ **API endpoints**: `GET /api/site_config_periods` (list), `POST /api/site_config_periods` (upsert), `GET /api/timeline` (coverage gaps). Report generation passes active period's cosine angle to PDF generator.
- ✅ **PDF statistics on every source corrected (2026-10-08)**: between PR #481 and this change the report pipeline queried `radar_data_transits` with site ID 0, citing a "legacy Python baseline" of raw transit speeds that did not exist (the Python generator passed `site_id` on every stats call, and the transit join dates from PR #196). A transit-sourced PDF, the default, therefore printed uncorrected p50/p85/p98 beside the cosine rows and the "corrected" note; against the 21° period on site 1 the p85 read 20.78 mph where the corrected value is 22.26 mph. The carve-out is gone: all three sources use the site join, and the cosine rows and note are emitted from the same predicate the query uses (requirement 4 above), pinned by `TestLoadData_AllStatsQueriesUseSiteID` and `TestGenerateTypst_TransitReportClaimsOnlyAppliedCorrection` in `internal/report/`. Transit-sourced PDFs generated before this change carry raw speeds whatever their Survey Parameters say; do not compare them with corrected ones.
- ✅ **Frontend**: Site edit page shows Configuration Periods card with form (start, end, angle 0–80°, notes, active flag) and period listing table.
- ✅ **Test coverage**: CRUD tests, cosine correction integration tests, overlap validation, timeline boundary tests, and E2E report generation tests.

### Remaining

- [ ] **Delete endpoint**: API supports create and update but not period deletion.
- [ ] **Report angle annotation**: PDF comparison reports should note when different cosine angles apply to each period.
- [ ] **Speed limit fields**: `speed_limit` and `speed_limit_note` were removed from `site` in migration 000014. They will not return to `site` or move to `site_config_periods`: a street can change limit at a sign inside the sensor's view, so limits will attach to the vector scene's road geometry, per the [posted speed limits plan](../../plans/posted-speed-limits-plan.md). Time-of-day variation is the [speed-limit-schedules spec](speed-limit-schedules.md).

## Testing strategy

1.  **Unit Tests:**
    - Test cosine math logic.
    - Test period overlap prevention.
    - Test "No Config" fallback behaviour.
2.  **Comparison Tests:**
    - Create synthetic data with known speeds.
    - Simulate a sensor move (Change in angle).
    - Verify that "Pre-move" and "Post-move" data is corrected to the _same_ true speed.

## Assumptions

1.  Users manually record when they moved the sensor.
2.  The effective time of a configuration change is known to the second.
3.  For "Report Comparison", users want to compare _Corrected_ speeds (true velocity), not Raw speeds (what the sensor saw).
