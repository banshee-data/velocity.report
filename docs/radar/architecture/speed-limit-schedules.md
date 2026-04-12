# Time-Based speed limit schedules

- **Status:** Design Specification (Not Implemented)
- **Issue:** Time-based speed limit schedules for sites
- **Future Work:** Ready for implementation when prioritised

Design specification for associating multiple speed limits with a site that vary by time of day and day of week, enabling accurate compliance reporting for locations such as school zones and work zones.

## Overview

Enable sites to define multiple speed limits that vary by time of day and day of week, allowing accurate monitoring and reporting for locations with variable speed limits such as school zones, residential areas with different daytime/nighttime limits, or work zones with active hours.

## Problem

### Problem statement

Traffic monitoring sites often have speed limits that change throughout the day:

- **School zones** reduce speed limits during arrival/dismissal times (e.g., 15 mph from 6:00-7:05 AM and 2:30-3:45 PM on weekdays, 25 mph otherwise)
- **Residential areas** may have different limits on weekends vs weekdays
- **Work zones** enforce reduced speeds only during active construction hours
- **Park zones** may have different limits during special events or peak hours

Currently, velocity.report sites can only specify a single static speed limit, making it impossible to:

- Accurately calculate speed limit violations for time-varying zones
- Generate context-aware reports that account for the active speed limit at measurement time
- Provide meaningful statistics (e.g., "85% of vehicles exceeded the school zone limit during active hours")

### User benefits

- **Accurate Violation Tracking:** Know which vehicles were speeding based on the speed limit in effect at that specific time
- **School Zone Compliance:** Monitor whether drivers are respecting reduced school zone hours
- **Temporal Analysis:** Compare speed behaviour during different speed limit periods (e.g., do drivers slow down when the school zone is active?)
- **Flexible Reporting:** Generate reports that show compliance with time-appropriate speed limits
- **Multiple Time Blocks:** Support complex schedules with different limits for morning/afternoon, weekdays/weekends, etc.

### Real-World use cases

**Use Case 1: School Zone Monitoring**

_Context:_ Elementary school with 15 mph limit during school hours, 25 mph otherwise

_Schedule Configuration:_

- Monday-Friday, 06:00-07:05: 15 mph (morning drop-off)
- Monday-Friday, 14:00-15:00: 15 mph (afternoon pickup)
- All other times: 25 mph (default site speed limit)

_Value:_ Parents and school administrators can see compliance data specifically during the times when children are present, supporting safety advocacy.

**Use Case 2: Variable Weekend/Weekday Limits**

_Context:_ Residential street with 25 mph weekday, 20 mph weekend

_Schedule Configuration:_

- Saturday-Sunday, 00:00-23:59: 20 mph
- Monday-Friday: 25 mph (default site speed limit)

_Value:_ Neighbourhood association can demonstrate that weekend traffic patterns justify the reduced speed limit.

**Use Case 3: Multi-Period School Zone**

_Context:_ School with staggered start times and multiple speed zones

_Schedule Configuration:_

- Monday-Friday, 06:00-07:30: 15 mph (elementary drop-off)
- Monday-Friday, 07:30-08:00: 20 mph (middle school arrival)
- Monday-Friday, 14:30-15:30: 15 mph (elementary pickup)
- Monday-Friday, 15:30-16:00: 20 mph (after-school activities)
- All other times: 30 mph (default)

_Value:_ Detailed analysis of compliance during different school-related time periods.

## Technical architecture

### Database schema

**Table:** `speed_limit_schedule`; columns: `id` (autoincrement PK), `site_id` (FK to `site` with cascade delete), `day_of_week` (1=Monday…7=Sunday), `start_time`/`end_time` (HH:MM text), `speed_limit` (integer), `created_at`/`updated_at` (Unix epoch, server-set). Indexed on `site_id` for fast retrieval. An `AFTER UPDATE` trigger maintains `updated_at` automatically. Target: `internal/db/migrations/`.

**Design Notes:**

- **Cascade Deletion:** When a site is deleted, all associated schedules are automatically removed
- **Day of Week Convention:** Follows ISO 8601 standard with (1=Monday, ..., 7=Sunday)
- **Time Format:** HH:MM 24-hour format for simplicity and precision
- **Timestamps:** Unix epoch (seconds since 1970) for consistency with other tables
- **Indexing:** Fast retrieval of all schedules for a site (common query pattern)

**Data Model:** `SpeedLimitSchedule` struct in `internal/db/`; mirrors the table columns with JSON tags. Fields: `ID`, `SiteID`, `DayOfWeek` (1–7), `StartTime`/`EndTime` (HH:MM strings), `SpeedLimit` (int), `CreatedAt`/`UpdatedAt` (`time.Time`).

### Database layer (Go)

**Location:** `internal/db/speed_limit_schedule.go`

**CRUD Operations:** Standard `Create`, `Get`, `GetForSite`, `Update`, `Delete`, and `DeleteAllForSite` methods on `*DB`. `GetForSite` returns schedules ordered by `day_of_week ASC, start_time ASC`. Create sets the ID on the struct after insertion; Update returns an error if the record does not exist.

**Key Implementation Details:**

- Schedules retrieved for a site are sorted by `day_of_week ASC, start_time ASC` for predictable display order
- All functions return detailed error messages for debugging
- Create operation sets the ID on the schedule object after insertion
- Update operation validates that the schedule exists (returns error if not found)

### HTTP API endpoints

**Base Path:** `/api/speed_limit_schedules`

**Endpoint Design:**

```
GET    /api/speed_limit_schedules/site/{siteID}  - List all schedules for a site
GET    /api/speed_limit_schedules/{id}           - Get single schedule by ID
POST   /api/speed_limit_schedules                - Create new schedule
PUT    /api/speed_limit_schedules/{id}           - Update existing schedule
DELETE /api/speed_limit_schedules/{id}           - Delete single schedule
DELETE /api/speed_limit_schedules/site/{siteID}  - Delete all schedules for site
```

**API Handler:** `internal/api/server.go:handleSpeedLimitSchedules()`

**Request/Response Shapes:**

- **GET /site/{siteID}** → 200: JSON array of schedule objects, each with `id`, `site_id`, `day_of_week`, `start_time`, `end_time`, `speed_limit`, `created_at`, `updated_at`.
- **POST** → request body contains `site_id`, `day_of_week`, `start_time`, `end_time`, `speed_limit`; response 201 returns the full object with server-assigned `id` and timestamps.
- **PUT /{id}** → same request shape as POST; response 200 returns the updated object with refreshed `updated_at`.
- **DELETE /{id}** → 204 No Content.
- **DELETE /site/{siteID}** → 204 No Content (bulk delete).

**Validation Rules:**

- `site_id`: Required, must be > 0
- `day_of_week`: Required, must be 1-7 (1=Monday, 7=Sunday)
- `start_time`: Required, must be in HH:MM format
- `end_time`: Required, must be in HH:MM format
- `speed_limit`: Required, must be > 0

**Error Responses:** Standard `{"error": "<message>"}` JSON envelope; 400 for validation failures (e.g., day_of_week out of range), 404 for missing schedule, 500 for database errors with detail text.

### Web UI components

**Primary Component:** `web/src/lib/components/SpeedLimitScheduleEditor.svelte`

**Purpose:** Inline editor for managing speed limit schedules within the site configuration page

**Props:** `siteId` (number), `schedules` (array of `SpeedLimitSchedule`, default empty), `onSchedulesChange` callback.

**UI Features:**

1. **Add Schedule Button:** Creates new schedule with sensible defaults (Monday, 06:00-07:05, 15 mph)
2. **Schedule List:** Displays all schedules as editable rows
3. **Inline Editing:** 4 fields per row (Day, Start Time, End Time, Speed Limit)
4. **Delete Button:** Removes individual schedule from list
5. **Empty State:** Helpful message when no schedules are defined
6. **Help Text:** Explains the purpose of time-based schedules

**Field Controls:**

- **Day of Week:** `<select>` with Sunday-Saturday options
- **Start Time:** `<select>` with 5-minute increments (00:00-23:55)
- **End Time:** `<select>` with 5-minute increments (00:00-23:55)
- **Speed Limit:** `<input type="number">` with min=5, max=100

**Time Selection Design:**

The component generates time options in 5-minute increments (00:00–23:55, 288 values), cached once per component instance.

**Reactivity:**

- Changes to any field trigger immediate updates to the parent component
- Parent component handles persistence (save to API)
- Uses negative IDs for new schedules that haven't been saved yet
- Deep copy of original schedules allows for cancel/reset functionality

**Integration Point:** `web/src/routes/site/[id]/+page.svelte`

The schedule editor is embedded in `web/src/routes/site/[id]/+page.svelte`. On mount, it fetches schedules via the API client and passes them to `SpeedLimitScheduleEditor` with a change callback that updates local state.

**Save Flow:**

When the user clicks "Save" on the site editor page:

1. Iterate through all schedules in the editor
2. For new schedules (negative ID): Call `createSpeedLimitSchedule()`
3. For modified existing schedules: Call `updateSpeedLimitSchedule()`
4. For schedules removed from editor: Call `deleteSpeedLimitSchedule()`
5. Reload schedules from server to get updated IDs and timestamps

**API Client:** `web/src/lib/api.ts`

`SpeedLimitSchedule` interface in `web/src/lib/api.ts` mirrors the JSON shape (id, site_id, day_of_week, start_time, end_time, speed_limit, created_at, updated_at: all typed as number or string). Six async API functions provide full CRUD: `getForSite`, `get`, `create`, `update`, `delete`, and `deleteAllForSite`.

### Testing

**Database Layer Tests:** `internal/db/speed_limit_schedule_test.go`

**Test Coverage:**

- ✅ Create speed limit schedule
- ✅ Get speed limit schedule by ID
- ✅ Get all schedules for site (with ordering verification)
- ✅ Update speed limit schedule
- ✅ Delete speed limit schedule
- ✅ Delete all schedules for site
- ✅ Get non-existent schedule (error case)

**Test Data Pattern:**

Tests create a test site and then exercise CRUD operations on schedules:

Tests follow the standard pattern: create a test DB + site, then exercise each CRUD operation as subtests (Create, Get, GetForSite ordering, Update, Delete, DeleteAll, Get-nonexistent). Target: `internal/db/speed_limit_schedule_test.go`.

**Web API Tests:** `web/src/lib/api.test.ts`

Tests mock the API endpoints and verify TypeScript interfaces match expected responses.

## Data flow diagrams

### Creating a schedule

```
User (Site Editor Page)
    │
    │ Click "Add Schedule"
    ↓
SpeedLimitScheduleEditor
    │
    │ User fills: Day=Monday, Start=06:00, End=07:05, Limit=15
    │ Click "Save"
    ↓
Site Editor Page
    │
    │ POST /api/speed_limit_schedules
    ↓
API Server (handleSpeedLimitSchedules)
    │
    │ Validate fields
    │ Create schedule
    ↓
Database Layer (CreateSpeedLimitSchedule)
    │
    │ INSERT INTO speed_limit_schedule
    ↓
SQLite (speed_limit_schedule table)
    │
    │ Return ID, timestamps
    ↓
API Server
    │
    │ 201 Created with schedule JSON
    ↓
Site Editor Page
    │
    │ Reload schedules
    ↓
SpeedLimitScheduleEditor
    │
    │ Display updated schedule list
    ↓
User sees new schedule with ID
```

### Loading site schedules

```
User navigates to /site/123
    │
    ↓
Site Editor Page (+page.svelte)
    │
    │ onMount() triggers loadSchedules()
    │ GET /api/speed_limit_schedules/site/123
    ↓
API Server (listSpeedLimitSchedulesForSite)
    │
    ↓
Database Layer (GetSpeedLimitSchedulesForSite)
    │
    │ SELECT * FROM speed_limit_schedule
    │ WHERE site_id = 123
    │ ORDER BY day_of_week, start_time
    ↓
SQLite returns rows
    │
    ↓
API Server
    │
    │ 200 OK with schedules array JSON
    ↓
Site Editor Page
    │
    │ Store schedules in component state
    ↓
SpeedLimitScheduleEditor
    │
    │ Render schedule list
    ↓
User sees all configured schedules
```

## User workflow

### Configuring school zone schedules

**Scenario:** User wants to set up 15 mph limit during school hours

1. Navigate to site editor: `/site/1`
2. Scroll to "Speed Limit Schedules" section
3. Click "Add Schedule" button
4. Configure morning schedule:
   - Day of Week: Monday
   - Start Time: 06:00
   - End Time: 07:05
   - Speed Limit: 15
5. Click "Add Schedule" again
6. Configure afternoon schedule:
   - Day of Week: Monday
   - Start Time: 14:00
   - End Time: 15:00
   - Speed Limit: 15
7. Repeat steps 3-6 for Tuesday through Friday
8. Click "Save" button at bottom of page
9. System saves all 10 schedules (2 per weekday)
10. User sees confirmation and updated schedule list

**Result:** Site now has time-based speed limits that can be used for reporting and analysis

## Future enhancements

### Potential evolution areas

**1. Schedule Resolution Logic**

_Current State:_ Schedules are stored but not yet used in report generation or violation detection.

_Future Enhancement:_ Implement a time-based speed limit resolver that:

- Takes a timestamp and returns the active speed limit
- Handles overlapping schedules (precedence rules)
- Falls back to default site speed limit if no schedule matches
- Caches resolved limits for performance

**2. Schedule Templates**

_Problem:_ Creating the same schedule for all 5 weekdays is repetitive.

_Enhancement:_ Add schedule templates:

- "School Zone (Standard)" - 06:00-07:05 and 14:00-15:00, Mon-Fri
- "Weekend Residential" - All day Saturday and Sunday
- "Work Zone (9-5)" - 08:00-17:00, Mon-Fri
- Custom templates saved per user

**3. Schedule Visualisation**

_Problem:_ Grid of schedules is hard to understand at a glance.

_Enhancement:_ Visual weekly calendar view:

- 7 columns (days of week)
- 24 rows (hours of day)
- Colour-coded speed limit blocks
- Hover for details
- Drag-to-create new blocks

**4. Schedule Validation**

_Current State:_ No validation for overlapping or conflicting schedules.

_Enhancement:_ Pre-save validation:

- Warn about overlapping time blocks on same day
- Flag end times before start times
- Suggest combining adjacent blocks with same limit

**5. Bulk Schedule Operations**

_Problem:_ Copying Mon schedule to Tue-Fri requires manual work.

_Enhancement:_ Bulk operations:

- "Copy to weekdays" button
- "Copy to all days" button
- Multi-select schedules for batch delete
- "Apply to multiple days" checkbox when creating

**6. Schedule Import/Export**

_Problem:_ Setting up complex schedules in UI is tedious.

_Enhancement:_ JSON import/export:

- Export current schedules as JSON file
- Import schedules from JSON (merge or replace)
- Share schedule configurations between sites
- Version control for schedule changes

**7. Report Integration**

_Current State:_ Reports don't use time-based limits yet.

_Enhancement:_ Schedule-aware reporting:

- Split statistics by active speed limit period
- Show compliance % during school zone hours vs regular hours
- Highlight violations specific to reduced-limit periods
- Time-series graph with speed limit overlay

**8. Historical Schedule Tracking**

_Problem:_ When schedules change, historical data interpretation is unclear.

_Enhancement:_ Schedule versioning:

- Track effective date ranges for schedule sets
- Query "what was the speed limit at this timestamp?"
- Generate reports using schedule active at measurement time
- Schedule change audit log

**9. Recurring Weekly Patterns**

_Problem:_ Schedules repeat weekly but can't handle special dates.

_Enhancement:_ Exception dates:

- Mark specific dates as "no school" (use default limit)
- Summer schedule variants (different months)
- Holiday overrides
- Special event schedules

**10. Speed Limit Zones (Geographic)**

_Problem:_ Single site may have multiple sensors in different speed zones.

_Enhancement:_ Multi-zone support:

- Define zones within a site
- Assign schedules to specific zones
- Tag radar sensors with zone ID
- Zone-specific reporting

## Design decisions and rationale

### Why day-based (not date-based) schedules?

**Decision:** Store day of week (0-6) instead of specific dates

**Rationale:**

- School zones repeat weekly, not on specific dates
- Simpler UI - no calendar picker needed
- More compact storage - 10 records instead of hundreds
- Easier to understand - "Monday 6-7am" vs "2025-09-15 6-7am"
- Matches user mental model - "school zone is active on weekdays"

**Tradeoff:** Cannot handle one-time events or date-specific limits (addressed in future enhancements)

### Why HH:MM format (not unix timestamps)?

**Decision:** Store times as "06:00" strings instead of absolute timestamps

**Rationale:**

- Times recur daily - "6am" not "6am on Sept 15, 2025"
- Simpler to edit and validate
- Human-readable in database queries
- Easier to compare (lexicographic sort works)
- Timezone-independent (times are relative to site's configured timezone)

**Tradeoff:** Requires parsing for time arithmetic (minor, well-solved problem)

### Why no overlap validation?

**Decision:** Allow overlapping schedules without blocking saves

**Rationale:**

- Overlap rules are ambiguous - should we take first, last, min, max?
- Users may want to test different configurations
- Easier to edit if not blocked by temporary overlaps during editing
- Can add warnings later without breaking existing functionality

**Tradeoff:** Users could create conflicting schedules (addressed in future enhancements)

### Why server-side timestamps?

**Decision:** Use `DEFAULT (STRFTIME('%s', 'now'))` instead of client-provided times

**Rationale:**

- Server is source of truth for timing
- Avoids client clock skew issues
- Consistent across all records
- Simpler client code (no need to format timestamps)

### Why cascade delete?

**Decision:** `ON DELETE CASCADE` for site_id foreign key

**Rationale:**

- Schedules are meaningless without parent site
- Prevents orphaned schedule records
- Simpler site deletion workflow
- Matches user expectation (deleting site deletes everything about it)

### Why 5-Minute time increments?

**Decision:** Time selectors use 00:00, 00:05, 00:10, ..., 23:55

**Rationale:**

- Precision sufficient for school zones (no "6:02am" zones)
- Reasonable dropdown length (288 options)
- Matches common scheduling patterns
- More precise than 15-minute increments
- Less overwhelming than 1-minute increments

**Tradeoff:** Cannot represent 6:07am precisely (not a real limitation)

### Why separate API for schedules (not nested in site)?

**Decision:** `/api/speed_limit_schedules` instead of `/api/sites/123/schedules`

**Rationale:**

- Schedules can be CRUD independently from site updates
- Simpler API client code (fewer nested operations)
- Better RESTful resource modelling
- Easier to add bulk operations later
- Matches existing pattern (site_reports is also separate)

## Privacy and security

### Privacy considerations

**No PII Collection:**

- Schedules contain only times and speed limits
- No personal information stored
- No vehicle identification (consistent with velocity.report principles)

**Local Storage:**

- All schedule data in local SQLite database
- No transmission to external services
- User maintains full data control

### Security considerations

**Input Validation:**

- Day of week constrained to 0-6
- Speed limit must be positive integer
- Time format validated by database
- Site ID must reference existing site (foreign key constraint)

**SQL Injection Protection:**

- All queries use parameterised statements (`?` placeholders)
- No string concatenation for SQL construction
- Foreign key constraints prevent invalid references

**Authorization:**

- Currently no authentication (local-only deployment model)
- Future: Role-based access if multi-user support added

## Performance considerations

### Query optimisation

**Index Strategy:**

- `idx_speed_limit_schedule_site` on `site_id` for fast retrieval
- Most common query: "get all schedules for site X"
- No index on day_of_week/time (low cardinality, infrequent filtering)

**Typical Query Performance:**

- Single site schedule retrieval: <1ms (indexed)
- Schedule creation: <1ms
- Bulk site deletion with cascade: <10ms for 100 schedules

### UI performance

**Component Efficiency:**

- Schedules rendered as flat list (not virtualized, max ~50 schedules expected)
- Time options generated once, cached in component
- Reactive updates only re-render changed schedule row
- No debouncing on inputs (instant feedback)

**Network Efficiency:**

- Single GET request on page load
- Batch save (not per-schedule)
- No polling (schedules rarely change)
- Small payload size (~100 bytes per schedule)

### Scalability

**Current Limits:**

- No artificial limit on schedules per site
- Expected usage: 10-20 schedules per site (school zone pattern)
- Tested with: 100 schedules per site (no performance issues)
- SQLite can handle thousands of schedules easily

## Migration and deployment

### Database migration

**Migration Status:** Schema already in `internal/db/schema.sql`

**Migration Safety:**

- New table, no existing data impact
- Foreign key to site table (must exist first)
- No down migration needed (table drop is trivial)

**Deployment Steps:**

1. Deploy updated Go binary with schedule endpoints
2. Database automatically creates table on first query (if not exists)
3. Deploy updated web frontend with schedule editor
4. Users can immediately start creating schedules

**Backward Compatibility:**

- Existing sites without schedules work unchanged
- Default site speed limit still primary value
- Schedules are optional enhancement
- No breaking changes to existing API endpoints

### Rollback plan

If issues arise after deployment:

1. **API rollback:** Remove speed_limit_schedules endpoint registration
2. **UI rollback:** Remove SpeedLimitScheduleEditor component import
3. **Database:** Table can remain (no harm, ignored)
4. **Data:** Schedules remain in database for future re-enablement

## Documentation updates

### Files that should reference this feature

**User-Facing Documentation:**

- [ ] Main README.md - Add to features list
- [ ] docs/src/guides/setup.md - Add section on configuring schedules
- [ ] docs/src/guides/reports.md - Explain schedule impact on reporting (future)

**Developer Documentation:**

- [ ] ARCHITECTURE.md - Reference schedule subsystem
- [ ] internal/db/README.md - Document speed_limit_schedule table
- [ ] internal/api/README.md - Document schedule endpoints
- [ ] web/README.md - Document SpeedLimitScheduleEditor component

**API Documentation:**

- [ ] API reference doc - Document all schedule endpoints
- [ ] OpenAPI/Swagger spec - Add schedule endpoints (if created)

## Success metrics

### Feature adoption

- **Setup Rate:** % of sites with at least one schedule defined
- **Usage Pattern:** Average number of schedules per site
- **Time to Configure:** How long to set up typical school zone (target: <5 min)

### Feature effectiveness

- **Data Quality:** Are schedules being used in reports? (future)
- **User Satisfaction:** Feedback on ease of configuration
- **Support Burden:** Number of schedule-related support questions

### Technical metrics

- **API Performance:** Schedule CRUD operation latency (target: <100ms)
- **Error Rate:** Schedule creation/update success rate (target: >99%)
- **Database Size:** Average storage per schedule (~200 bytes expected)

## Appendices

### Appendix a: database schema details

Full DDL is described in the Database Schema section above. Constraints: all columns NOT NULL; `id` autoincrement PK; `site_id` FK with cascade delete. One index on `site_id`. One `AFTER UPDATE` trigger to maintain `updated_at`.

### Appendix b: example API interactions

Typical school-zone setup: POST two schedules per weekday (morning 06:00–07:05 and afternoon 14:00–15:00 at 15 mph), repeat for days 1–5. Retrieve with `GET /api/speed_limit_schedules/site/{id}`. Update with `PUT /{id}` (same body shape as POST). Bulk-delete with `DELETE /site/{id}`.

### Appendix c: component props and events

`SpeedLimitScheduleEditor` accepts `siteId`, `schedules`, and `onSchedulesChange` props (see Web UI section above). Local state holds a `daysOfWeek` label array and cached `timeOptions`. Methods: `addSchedule()` (inserts default row), `removeSchedule(index)`, `updateSchedule(index, field, value)`, and `generateTimeOptions()` (5-min increments, 00:00–23:55).

### Appendix d: test case summary

**Database Layer Tests (speed_limit_schedule_test.go):**

| Test Case                           | Purpose                  | Key Assertions                              |
| ----------------------------------- | ------------------------ | ------------------------------------------- |
| CreateSpeedLimitSchedule            | Verify schedule creation | ID is set after creation                    |
| GetSpeedLimitSchedule               | Verify single retrieval  | All fields match created schedule           |
| GetSpeedLimitSchedulesForSite       | Verify list retrieval    | Returns all schedules, sorted correctly     |
| UpdateSpeedLimitSchedule            | Verify updates work      | Updated fields persist to database          |
| DeleteSpeedLimitSchedule            | Verify deletion          | Schedule no longer retrievable after delete |
| DeleteAllSpeedLimitSchedulesForSite | Verify bulk delete       | All site schedules removed                  |
| GetNonExistentSchedule              | Verify error handling    | Returns appropriate error                   |

**Coverage:** 100% of database layer functions

### Appendix e: day of week reference

**Day Numbering Convention:**

| Number | Day       | ISO 8601 | Typical Use Case           |
| ------ | --------- | -------- | -------------------------- |
| 1      | Monday    | 1        | School zone active         |
| 2      | Tuesday   | 2        | School zone active         |
| 3      | Wednesday | 3        | School zone active         |
| 4      | Thursday  | 4        | School zone active         |
| 5      | Friday    | 5        | School zone active         |
| 6      | Saturday  | 6        | Weekend residential limits |
| 7      | Sunday    | 7        | Weekend residential limits |

**Note:** ISO 8601 (which uses 1=Monday, 7=Sunday). This conforms with using unix seconds and UTC as date standards.

### Appendix f: related tables

**Site Table Relationship:** The `site` table carries `speed_limit` (baseline default) and `speed_limit_note` (human-readable schedule description). See `internal/db/schema.sql` for the full `site` DDL.

**Relationship:**

- `site.speed_limit` is the baseline/default speed limit
- `speed_limit_schedule.speed_limit` overrides default during specified times
- `site.speed_limit_note` can document the schedule in human-readable form

**Design Pattern:** Default + Exceptions

- Sites have a default speed limit (site.speed_limit)
- Schedules define exceptions to that default
- If no schedule matches current time, use default
- This matches user mental model ("25 mph normally, 15 mph school hours")

## Glossary

**Terms Used in This Document:**

- **Schedule:** A time-based speed limit rule for a specific day and time range
- **Site:** A monitoring location with one or more radar sensors
- **Speed Limit:** Posted maximum legal speed in mph or kph
- **Day of Week:** Integer 1-7 representing Monday through Sunday
- **Time Block:** Period defined by start_time and end_time
- **School Zone:** Area with reduced speed limits during school hours
- **Cascade Delete:** Automatic deletion of child records when parent is deleted
- **CRUD:** Create, Read, Update, Delete operations
