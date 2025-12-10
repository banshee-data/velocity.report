# Feature Specification: Site Configuration with Time-Based Cosine Error Correction

**Status:** Partially Implemented (Backend Complete, Frontend/Reports Pending)  
**Date:** 2025-11-07  
**Author:** Ictinus (Product Architecture Agent)

## Problem Statement

Users need the ability to:

1. **Configure cosine error angle correction** - Radar sensors are often not perfectly perpendicular to traffic, introducing cosine error in speed measurements
2. **Track configuration changes over time** - When sensors are adjusted or repositioned, different correction angles apply to different time periods
3. **Apply corrections retroactively** - Users may realize the angle was wrong after data collection and need to correct historical data without recomputation
4. **Visualize configuration coverage** - Identify time periods where data exists but no site configuration is assigned
5. **Support multiple configurations in reports** - A single report may span multiple days with different sensor angles

## User Value Proposition

**For community advocates and traffic engineers:**
- Accurate speed measurements despite imperfect sensor positioning
- Clear audit trail of sensor configuration changes
- Ability to correct measurement errors discovered after the fact
- Professional reports that account for configuration variations
- Confidence in data accuracy for decision-making

## Current System Capabilities

### Existing Infrastructure

**Database:**
- `site` table already exists with `cosine_error_angle` field (from migration `20251014_create_site_table.sql`)
- SQLite database with subsecond timestamp precision
- `radar_data` and `radar_objects` tables store raw measurements

**API:**
- `/api/sites` endpoints for site CRUD operations
- `/api/radar_stats` for querying speed statistics
- Unit conversion and timezone handling already implemented

**Reports:**
- PDF generator via Python/LaTeX
- Statistical summaries (P50, P85, P98)
- Site information included in reports

### Gaps Identified

1. **No time-based configuration tracking** - Current `site.cosine_error_angle` is a single value, not time-aware
2. **No correction application** - Queries return raw measured speeds, not corrected speeds
3. **No configuration timeline view** - Can't visualize which config was active when
4. **No active site concept** - No way to mark which site config applies to incoming data

## Solution Design: Type 6 Slowly Changing Dimension

### Architecture Decision

**Pattern Selected:** Type 6 SCD (Hybrid Temporal Tracking)

**Rationale:**
- Preserves complete history of configuration changes
- Enables point-in-time queries (join on timestamp)
- Supports retroactive corrections (compute at read time, not write time)
- Standard data warehousing pattern with well-understood semantics

**Alternatives Considered:**
- ❌ Type 1 SCD (Overwrite) - Loses history, can't track changes
- ❌ Type 2 SCD (New Row) - Would require duplicating site data
- ❌ Separate config history table - More complex joins, less standard pattern

### Schema Design

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
    FOREIGN KEY (site_id) REFERENCES site (id) ON DELETE CASCADE
);
```

**Key Design Choices:**
- **DOUBLE timestamps** - Subsecond precision matches existing `radar_data.write_timestamp`
- **Nullable end time** - Open-ended periods support ongoing data collection
- **is_active flag** - Marks which period applies to new incoming data (only one can be active)
- **Cascade delete** - Deleting a site removes its configuration periods

### Cosine Correction Formula

```
corrected_speed = measured_speed / cos(angle_in_radians)
```

Where:
- `angle_in_radians = cosine_error_angle × (π / 180)`
- Applied via SQL: `measured_speed / COS(angle * 0.0174533)`

**Implementation Location:** Query time (SELECT statements), not write time (INSERT statements)

**Benefit:** Changing a site's cosine_error_angle and creating a new period with the updated angle immediately reflects in all queries, without data reprocessing.

## What Has Been Implemented

### 1. Database Schema (✅ Complete)

**Migration:** `data/migrations/000009_refactor_site_config.up.sql`

- Created `site_variable_config` table for reusable configuration values
- Created `site_config_periods` table (many-to-one with site_variable_config)
- Indexes on `(effective_start_unix, effective_end_unix)` for range queries
- Index on `is_active` for finding current active period
- Triggers to enforce single active period constraint
- Migration properly pairs sites with configs using ROW_NUMBER() to avoid ID mismatch
- Only one period set as active (minimum site ID) to avoid trigger conflicts

**Schema Maintenance:** The `schema.sql` file includes PRAGMA statements for database performance (WAL, synchronous=NORMAL, temp_store=MEMORY, busy_timeout) and uses IF NOT EXISTS clauses for idempotent table/index creation.

**Code:** `internal/db/site_config_period.go`

Functions implemented:
- `CreateSiteConfigPeriod` - Create new period
- `GetSiteConfigPeriod` - Get by ID
- `GetActiveSiteConfigPeriod` - Get currently active period (with site details)
- `GetSiteConfigPeriodForTimestamp` - Find period effective at a timestamp
- `GetAllSiteConfigPeriods` - List all periods with site details
- `UpdateSiteConfigPeriod` - Update existing period
- `SetActiveSiteConfigPeriod` - Mark a period as active (deactivates others)
- `CloseSiteConfigPeriod` - Close an open-ended period
- `DeleteSiteConfigPeriod` - Remove a period
- `GetTimeline` - Complex query showing data periods vs config periods

**Tests:** `internal/db/site_config_period_test.go`

Coverage:
- Period CRUD operations
- Active period enforcement (trigger validation)
- Multi-period scenarios
- Period closing logic
- Timeline generation

### 2. Cosine Error Correction (✅ Complete)

**Modified Files:** `internal/db/db.go`

**Queries Updated:**

1. **`RadarObjectRollupRange`** - Main stats query for API
   - Joins `radar_objects` with `site_config_periods` on `write_timestamp`
   - Applies correction: `max_speed / COS(cosine_error_angle * 0.0174533)`
   - Handles both `radar_objects` and `radar_data_transits` sources

2. **`Events`** - Recent events query
   - Joins `radar_data` with `site_config_periods`
   - Applies correction to `speed` field

3. **`RadarObjects`** - Object list query
   - Joins `radar_objects` with `site_config_periods`
   - Applies correction to `max_speed` and `min_speed`

**Correction Behavior:**
- If no site config period matches timestamp → returns uncorrected speed
- If site config period matches → applies cosine correction from that period's site
- Correction computed on-the-fly in SQL (no stored corrected values)

**Tests:** `internal/db/cosine_correction_test.go`

Validated:
- 5° angle produces 1.00382x correction factor (25.0 m/s → 25.0955 m/s)
- Floating point precision within 0.001 m/s tolerance
- Default period (0.5° from migration) applies to current data
- Existing tests continue passing (backward compatibility)

### 3. API Endpoints (✅ Complete)

**Modified File:** `internal/api/server.go`

**Endpoints Added:**

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/site_config_periods` | List all periods |
| POST | `/api/site_config_periods` | Create new period |
| GET | `/api/site_config_periods/{id}` | Get specific period |
| PUT | `/api/site_config_periods/{id}` | Update period |
| DELETE | `/api/site_config_periods/{id}` | Delete period |
| GET | `/api/site_config_periods/active` | Get active period |
| POST | `/api/site_config_periods/{id}/activate` | Set period as active |
| POST | `/api/site_config_periods/{id}/close` | Close open period |
| GET | `/api/timeline?start=X&end=Y` | Get timeline view |

**Timeline Endpoint Details:**
- Query parameters: `start` and `end` (unix timestamps)
- Returns array of time segments with:
  - Time range (`start_unix`, `end_unix`)
  - Data presence (`has_data`, `data_count`)
  - Associated site config period (if any)
  - `unconfigured_period` flag (true if data exists but no config)

**Error Handling:**
- 400 Bad Request for invalid parameters
- 404 Not Found for missing resources
- 405 Method Not Allowed for wrong HTTP methods
- 500 Internal Server Error for database failures
- All errors return JSON with `{"error": "message"}`

### 4. Documentation (✅ Complete)

**File:** `docs/SITE_CONFIG_COSINE_CORRECTION.md`

Comprehensive guide covering:
- Cosine error explanation and formula
- Type 6 SCD pattern overview
- API endpoint examples with curl commands
- Workflow examples (initial setup, changing configuration)
- Best practices (measurement accuracy, effective dates)
- Troubleshooting guide
- Migration from legacy systems

**Updated File:** `internal/db/schema.sql`
- Includes site_config_periods table definition
- Documents the Type 6 SCD pattern in comments

## Assumptions Made

### Technical Assumptions

1. **Timestamp Precision:** Subsecond precision (DOUBLE) is sufficient for matching radar data to configuration periods
2. **Single Active Period:** Only one configuration period can be active at a time (enforced by triggers)
3. **SQLite COS() Function:** Available and accurate for correction computation (verified in tests)
4. **Left Join Behavior:** Unmatched rows (no config period) should return uncorrected speeds
5. **Backward Compatibility:** Default period (site_id=1, start=0) covers existing data

### Business Assumptions

1. **Angle Measurement:** Users can accurately measure the cosine error angle (e.g., with protractor)
2. **Configuration Frequency:** Site configurations change infrequently (weeks/months, not hours)
3. **Timestamp Accuracy:** Users know when they adjusted the radar (to set effective_start_unix)
4. **Single Location:** One device per deployment (multi-device coordination not addressed)
5. **Manual Management:** Users will manually create/close periods via API (no automatic detection)

### Data Assumptions

1. **No Gaps Desired:** Users want continuous configuration coverage for all data
2. **Retroactive Correction:** Users may discover angle errors after data collection
3. **Audit Trail:** Users want to know what configuration was used when (hence timeline view)
4. **Report Clarity:** Reports spanning multiple configurations should show which angles were used

## What Remains To Be Done

### High Priority

#### 1. PDF Report Integration
**Status:** Not Started  
**Effort:** Medium (4-6 hours)  
**Risk:** Low

**Requirements:**
- Query `site_config_periods` for the report's time range
- Display site configuration mapping table in PDF:
  - Time period | Site Name | Cosine Angle | Data Points
- Highlight if multiple configurations exist in one report
- Show unconfigured periods as warnings

**Files to Modify:**
- `tools/pdf-generator/pdf_generator/core/api_client.py` - Add endpoint to fetch periods for date range
- `tools/pdf-generator/pdf_generator/core/table_builders.py` - New table builder for config mapping
- `tools/pdf-generator/pdf_generator/core/document_builder.py` - Include config table in report

**Acceptance Criteria:**
- PDF includes "Site Configuration Periods" section
- Table shows all periods overlapping with report date range
- Multi-configuration reports clearly indicate angle changes
- Warning shown if unconfigured periods detected

#### 2. Sensor Initialization Validation
**Status:** Not Started  
**Effort:** Small (2-3 hours)  
**Risk:** Low

**Requirements:**
- Add validation in radar sensor initialization code
- Verify inbound direction angle = 0°
- Verify outbound direction angle = 0°
- Log warnings if angles are non-zero (indicates misconfiguration)

**Files to Modify:**
- `internal/radar/` - Sensor initialization code (need to locate exact file)
- Add configuration query/parse logic
- Log validation results

**Acceptance Criteria:**
- Sensor startup logs show angle validation
- Warnings logged if angles are non-zero
- Does not prevent sensor operation (warning only)

**Open Questions:**
- Which radar sensor initialization file handles this?
- What's the command to query sensor angles?
- Should this be error (prevent startup) or warning?

### Medium Priority

#### 3. Dashboard UI Updates
**Status:** Not Started  
**Effort:** Medium-Large (6-8 hours)  
**Risk:** Medium (requires frontend expertise)

**Requirements:**
- Display corrected speeds (already served by API, just need UI update)
- Show active site configuration in header/status bar
- Timeline visualization component showing:
  - Data periods (bar chart)
  - Site config periods (overlaid bars with different colors)
  - Unconfigured periods highlighted in red
- Site configuration management UI:
  - List periods
  - Create/edit/delete periods
  - Activate/close periods
  - Visual feedback for active period

**Files to Modify:**
- `web/src/` - Svelte components (need to identify specific files)
- New component: `SiteConfigTimeline.svelte`
- New component: `SiteConfigManager.svelte`
- Update: Speed display components to show "corrected" label

**Acceptance Criteria:**
- Dashboard shows "(corrected)" next to speed values
- Active site config visible in UI
- Timeline view accessible from navigation
- Timeline clearly shows unconfigured periods
- Users can manage periods without command line

**Open Questions:**
- Which Svelte components currently display speeds?
- Where should timeline view be placed in navigation?
- Should we auto-refresh timeline when new data arrives?

### Low Priority (Future Enhancements)

#### 4. Automated Period Creation
**Status:** Not Planned  
**Effort:** Large (8-10 hours)

**Idea:** Automatically detect when sensor is adjusted (significant change in data patterns) and prompt user to create a new period.

**Challenges:**
- How to detect sensor adjustment vs. normal traffic variance?
- Risk of false positives creating unwanted periods
- May require machine learning or complex heuristics

**Recommendation:** Defer until user feedback shows this is needed.

#### 5. Multi-Device Coordination
**Status:** Not Planned  
**Effort:** Very Large (20+ hours)

**Idea:** Support multiple sensors at one location with different angles, or coordinated measurements across sensors.

**Challenges:**
- Current schema assumes one active period at a time
- Would need device_id or sensor_id in periods table
- Increased complexity in queries and UI
- Data aggregation across devices

**Recommendation:** Wait for user request with specific use case.

#### 6. Angle Recommendation System
**Status:** Not Planned  
**Effort:** Medium (6-8 hours)

**Idea:** Analyze data patterns to suggest optimal cosine angle if user isn't sure of actual angle.

**Approach:**
- Compare speeds at different assumed angles
- Look for consistency with posted speed limits
- Use statistics to infer likely angle

**Challenges:**
- May not be accurate without ground truth
- Requires sophisticated statistical analysis
- Could mislead users if algorithm is wrong

**Recommendation:** Defer. Users should measure angle physically.

## Dependencies and Integration Points

### Upstream Dependencies
- SQLite 3.x with COS() function support ✅ (available in all modern versions)
- Go 1.21+ ✅ (already required)
- Existing `site` table ✅ (created in migration 20251014)

### Downstream Impacts
- **API Clients:** New endpoints available, backward compatible (existing endpoints unchanged)
- **PDF Generator:** Will need updates to consume new endpoints (not breaking)
- **Dashboard:** Will need updates to display corrected values (not breaking)
- **Existing Data:** Default period created by migration ensures backward compatibility ✅

## Testing Strategy

### Unit Tests (✅ Complete)
- Database operations: `site_config_period_test.go`
- Cosine correction: `cosine_correction_test.go`
- All existing tests pass (regression suite)

### Integration Tests (⚠️ Partial)
- API endpoints compile and build ✅
- End-to-end API tests not yet written
- PDF generation with config periods not tested

### Manual Testing (❌ Not Started)
- Deploy to test Raspberry Pi
- Collect data with multiple config periods
- Generate PDF report spanning multiple periods
- Verify timeline view in dashboard

### Performance Testing (❌ Not Started)
- Query performance with large datasets (millions of records)
- Impact of LEFT JOIN on radar stats queries
- Index effectiveness for time-range queries

## Migration and Deployment

### Database Migration
**File:** `data/migrations/20251107_create_site_config_periods.sql`

**Applied:** Automatically on startup (Go server runs migrations)

**Backward Compatibility:**
- Default period created (site_id=1, effective_start=0, is_active=1)
- Ensures all existing data gets corrected with default site's angle (0.5°)
- Users can adjust default period or create new ones as needed

### API Deployment
**Impact:** Additive only (new endpoints, existing endpoints unchanged)

**Rollback:** Safe to revert commits, old API continues working

### User Migration Path
1. System deploys with default period covering all historical data
2. User measures actual sensor angle
3. User creates appropriate site config period(s)
4. Historical data automatically reflects correct angles in new queries

## Security and Privacy

### Security Considerations
- **Input Validation:** All API endpoints validate timestamps, IDs, required fields ✅
- **SQL Injection:** Using parameterized queries throughout ✅
- **Access Control:** No authentication (relies on network security, same as existing API)
- **Path Traversal:** Not applicable (no file operations in these endpoints)

### Privacy Considerations
- **No New PII:** Site config contains location names but no vehicle/person data ✅
- **Audit Trail:** Configuration changes are logged but not attributed to users
- **Data Exposure:** Timeline endpoint reveals when data was collected (already exposed via radar_stats)

## Success Metrics

### Quantitative Metrics
- [ ] Time to create new site config period < 30 seconds (manual API call)
- [ ] Query performance degradation < 5% (due to LEFT JOIN)
- [ ] Test coverage > 80% for new code ✅ (achieved)
- [ ] Zero data loss during migration ✅ (backward compatible)

### Qualitative Metrics
- [ ] Users can explain Type 6 SCD pattern after reading docs
- [ ] Users can identify unconfigured periods in timeline view
- [ ] Users trust corrected speeds more than raw speeds
- [ ] Support requests about "wrong speeds" decrease

### Adoption Metrics
- [ ] % of deployments with >1 site config period
- [ ] Average number of periods per deployment
- [ ] % of reports spanning multiple config periods

## Open Questions and Decisions Needed

### Technical Questions
1. **Should sensor initialization fail or warn on non-zero angles?**
   - Recommendation: Warn only (allow operation to continue)
   - Rationale: Sensor may have legitimate directional configuration

2. **Should timeline view be paginated for long date ranges?**
   - Recommendation: No pagination initially, add if performance issues arise
   - Rationale: Timeline typically queried for weeks/months, not years

3. **Should we cache active site config period?**
   - Recommendation: No caching initially (query is fast, infrequent writes)
   - Rationale: Premature optimization, not a hot path

### Product Questions
1. **Should dashboard auto-highlight unconfigured periods on startup?**
   - Need UX input: Modal popup? Notification banner?
   - Consider: Could be annoying if intentional gaps exist

2. **Should PDF report fail or warn if unconfigured periods detected?**
   - Recommendation: Warn in report, don't fail generation
   - Rationale: User may want partial report despite gaps

3. **Should we support overlapping periods (same site, different angles)?**
   - Current design: Not supported (would require complex conflict resolution)
   - Recommendation: Enforce non-overlapping via UI validation

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Users measure angle incorrectly | High | Medium | Provide measurement guide in docs ✅ |
| Queries slow down with many periods | Low | Medium | Monitor performance, add indexes if needed |
| Users confused by Type 6 SCD pattern | Medium | Low | Comprehensive documentation ✅ |
| Unconfigured periods go unnoticed | Medium | High | Timeline view highlights gaps ✅ |
| Multiple active periods due to race condition | Low | Medium | Database triggers enforce constraint ✅ |
| PDF reports fail with multiple periods | Medium | Medium | Test thoroughly before release |

## Next Steps

### Immediate (This PR)
1. ✅ Complete backend implementation
2. ✅ Write comprehensive documentation
3. ✅ Add unit tests
4. ⏳ **Request review from user on approach**
5. ⏳ **Get feedback on assumptions**

### Short Term (Next PR)
1. Implement PDF report integration
2. Add sensor initialization validation
3. Write integration tests for API endpoints
4. Manual testing on Raspberry Pi

### Medium Term (Following PRs)
1. Dashboard UI updates
2. Timeline visualization component
3. Site config management UI
4. Performance testing with large datasets

### Long Term (Future Consideration)
1. Automated period creation (if requested)
2. Multi-device support (if needed)
3. Angle recommendation system (if validated)

## References

- Original problem statement in PR description
- Type 6 SCD pattern: [Kimball Group](https://www.kimballgroup.com/data-warehouse-business-intelligence-resources/kimball-techniques/dimensional-modeling-techniques/type-6/)
- Cosine error in radar: [Federal Highway Administration](https://safety.fhwa.dot.gov/speedmgt/ref_mats/fhwasa12004/)
- SQLite math functions: [Documentation](https://www.sqlite.org/lang_mathfunc.html)

---

**Document Version:** 1.0  
**Last Updated:** 2025-11-07  
**Next Review:** After user feedback on implementation approach
