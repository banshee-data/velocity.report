# Histogram & Percentile queries — radar_data_transits vs radar_objects

Purpose
- Document the SQL queries used to generate histograms and nearest-rank percentiles for comparison between `radar_data_transits` (sessionized) and `radar_objects` (raw objects).
- Record the most recent results (computed Sept 30, 2025) and provide runnable sqlite3 commands you can reuse.

Notes
- Data lives in `sensor_data.db`.
- `radar_data_transits` uses `model_version` tags; the commands below use `model_version = 'rebuild-full'` (change as needed).
- We filter out very-low-speed rows to compare meaningful moving events. The filter used here is >= 2.2352 m/s (≈5 mpg) as requested.
- Avoid multi-statement CTE batches in sqlite3 (they caused "no such table/column" prepare errors). Run the single-statement queries sequentially or wrap logic in your application code.

1) Compute numeric overlap bounds (transit start vs object write_timestamp)

```sql
-- returns overlap_start, overlap_end as unix seconds
SELECT
  (CASE WHEN (SELECT MIN(transit_start_unix) FROM radar_data_transits WHERE model_version='rebuild-full')
        > (SELECT MIN(write_timestamp) FROM radar_objects)
    THEN (SELECT MIN(transit_start_unix) FROM radar_data_transits WHERE model_version='rebuild-full')
    ELSE (SELECT MIN(write_timestamp) FROM radar_objects) END) AS overlap_start,
  (CASE WHEN (SELECT MAX(transit_start_unix) FROM radar_data_transits WHERE model_version='rebuild-full')
        < (SELECT MAX(write_timestamp) FROM radar_objects)
    THEN (SELECT MAX(transit_start_unix) FROM radar_data_transits WHERE model_version='rebuild-full')
    ELSE (SELECT MAX(write_timestamp) FROM radar_objects) END) AS overlap_end;
```

2) Counts inside overlap with speed filter (>= 2.2352 m/s)

```sql
-- transits count
SELECT COUNT(*)
FROM radar_data_transits
WHERE model_version='rebuild-full'
  AND transit_start_unix BETWEEN <overlap_start> AND <overlap_end>
  AND transit_max_speed >= 2.2352;

-- objects count
SELECT COUNT(*)
FROM radar_objects
WHERE write_timestamp BETWEEN <overlap_start> AND <overlap_end>
  AND CAST(COALESCE(json_extract(raw_event, '$.max_speed_mps'), json_extract(raw_event, '$.max_speed')) AS DOUBLE) >= 2.2352;
```

Replace `<overlap_start>` / `<overlap_end>` with the numeric values returned by the bounds query.

3) Histogram (bucket speeds into integer m/s bins)

```sql
-- Transits histogram (example: bucket by floor(transit_max_speed))
SELECT CAST(transit_max_speed AS INTEGER) AS bucket,
       COUNT(*) AS cnt
FROM radar_data_transits
WHERE model_version='rebuild-full'
  AND transit_start_unix BETWEEN <overlap_start> AND <overlap_end>
  AND transit_max_speed >= 2.2352
GROUP BY bucket
ORDER BY bucket;

-- Objects histogram (extract numeric max speed from JSON)
SELECT CAST(COALESCE(json_extract(raw_event, '$.max_speed_mps'), json_extract(raw_event, '$.max_speed')) AS DOUBLE) AS speed,
       CAST(COALESCE(json_extract(raw_event, '$.max_speed_mps'), json_extract(raw_event, '$.max_speed')) AS INTEGER) AS bucket,
       COUNT(*) AS cnt
FROM radar_objects
WHERE write_timestamp BETWEEN <overlap_start> AND <overlap_end>
  AND CAST(COALESCE(json_extract(raw_event, '$.max_speed_mps'), json_extract(raw_event, '$.max_speed')) AS DOUBLE) >= 2.2352
GROUP BY bucket
ORDER BY bucket;
```

If desired, you can normalize counts to percentages by adding `CAST(100.0 * COUNT(*) / (SELECT COUNT(*) FROM ... ) AS DOUBLE)`.

4) Nearest-rank percentile queries (p50/p85/p98) — run sequentially
- Steps: 1) get `n` = count of filtered rows; 2) compute offset = floor(n * p) - 1; 3) select ordered value LIMIT 1 OFFSET offset.

Example for transits (using computed n):

```sql
-- Suppose n = 21466 (replace with the actual count returned earlier)
-- p50
SELECT transit_max_speed
FROM radar_data_transits
WHERE model_version='rebuild-full'
  AND transit_start_unix BETWEEN <overlap_start> AND <overlap_end>
  AND transit_max_speed >= 2.2352
ORDER BY transit_max_speed
LIMIT 1 OFFSET (CAST((21466 * 0.50) AS INTEGER) - 1);

-- p85
LIMIT 1 OFFSET (CAST((21466 * 0.85) AS INTEGER) - 1);

-- p98
LIMIT 1 OFFSET (CAST((21466 * 0.98) AS INTEGER) - 1);

-- max
SELECT MAX(transit_max_speed)
FROM radar_data_transits
WHERE model_version='rebuild-full'
  AND transit_start_unix BETWEEN <overlap_start> AND <overlap_end>
  AND transit_max_speed >= 2.2352;
```

Example for `radar_objects` — replace the LIMIT/OFFSET ordering expression to order by the extracted numeric speed:

```sql
-- Suppose n = 22693
-- p50
SELECT CAST(COALESCE(json_extract(raw_event, '$.max_speed_mps'), json_extract(raw_event, '$.max_speed')) AS DOUBLE) AS max_speed
FROM radar_objects
WHERE write_timestamp BETWEEN <overlap_start> AND <overlap_end>
  AND CAST(COALESCE(json_extract(raw_event, '$.max_speed_mps'), json_extract(raw_event, '$.max_speed')) AS DOUBLE) >= 2.2352
ORDER BY max_speed
LIMIT 1 OFFSET (CAST((22693 * 0.50) AS INTEGER) - 1);

-- p85 / p98 analogous

-- max
SELECT MAX(CAST(COALESCE(json_extract(raw_event, '$.max_speed_mps'), json_extract(raw_event, '$.max_speed')) AS DOUBLE))
FROM radar_objects
WHERE write_timestamp BETWEEN <overlap_start> AND <overlap_end>
  AND CAST(COALESCE(json_extract(raw_event, '$.max_speed_mps'), json_extract(raw_event, '$.max_speed')) AS DOUBLE) >= 2.2352;
```

5) sqlite3 quick-run examples (run these sequentially in a shell)

```bash
# get overlap bounds
sqlite3 ./sensor_data.db "<bounds query from section (1)>"

# using the returned numeric overlap bounds, get counts
sqlite3 ./sensor_data.db "<counts query from section (2) with numeric bounds substituted>"

# run percentiles for transits (replace n in offset expression with transit count)
sqlite3 ./sensor_data.db "<p50/p85/p98/max transit queries from section (4)>"

# run percentiles for objects (replace n with objects count)
sqlite3 ./sensor_data.db "<p50/p85/p98/max object queries from section (4)>":
```

Recent results (computed sequentially Sept 30, 2025)
- Overlap bounds: 1751007797.0 → 1757781705.0

- Counts (speed >= 2.2352 m/s)
  - radar_data_transits (model_version='rebuild-full'): 21,466
  - radar_objects: 22,693

- radar_data_transits (filtered)
  - p50 = 7.65 m/s
  - p85 = 9.47 m/s
  - p98 = 11.23 m/s
  - max  = 19.79 m/s

- radar_objects (filtered)
  - p50 = 7.71 m/s
  - p85 = 9.59 m/s
  - p98 = 11.47 m/s
  - max  = 21.79 m/s

Notes & recommendations
- The two sets of percentiles align closely once low-speed rows are filtered; objects are slightly higher at the extreme end (max and p98).
- To automate: add a small Go tool or a SQL script that runs these queries and writes CSV/JSON for dashboard ingestion.
- If you want pairwise per-transit vs nearest-object comparisons, I can add example JOIN queries or a small Go program that performs matching using `radar_transit_links` and `radar_objects` timestamps.

If you'd like, I can also:
- Commit this file and add a tiny shell script under `internal/db/` to run and output CSV.
- Add a periodic check to monitoring that computes and stores these percentiles for trend analysis.


---
Document created on 2025-09-30
