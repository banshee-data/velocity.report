# Dates and timezones

Canonical reference for how velocity.report represents time across the database,
the HTTP API, and the web frontend. The guiding rule is **store UTC, interpret
at the edge**: the database holds absolute instants, the client decides which
calendar window it wants, and timezone is a presentation concern.

## Storage (database)

All timestamps are stored as **Unix epoch seconds in UTC**. There is no
local-time or wall-clock storage anywhere in the schema.

| Column                                        | Table(s)                      | Type                                           | Meaning                       |
| --------------------------------------------- | ----------------------------- | ---------------------------------------------- | ----------------------------- |
| `write_timestamp`                             | `radar_data`, `radar_objects` | `DOUBLE` (subsecond via `UNIXEPOCH('subsec')`) | Instant the row was recorded  |
| `transit_start_unix` / `transit_end_unix`     | `radar_data_transits`         | `INTEGER`/`DOUBLE`                             | Transit session bounds        |
| `effective_start_unix` / `effective_end_unix` | `site_config_periods`         | `INTEGER`                                      | Configuration validity window |

Unix epoch seconds are timezone-agnostic by definition (they count from
`1970-01-01T00:00:00Z`), so the database never needs a timezone column. Range
queries are plain integer comparisons: `WHERE write_timestamp BETWEEN ? AND ?`.

## API inputs

There are two input shapes, each an ISO 8601 profile chosen to match what the
field actually means. **No other date string format is accepted.**

### Query endpoints — ISO 8601 instants

`GET /api/radar_stats` and `GET /api/charts/{timeseries,histogram,comparison}`
take an absolute window:

| Parameter                      | Format                                 | Notes                                                                                                      |
| ------------------------------ | -------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `start`, `end`                 | ISO 8601 / RFC3339 instant with offset | e.g. `2026-06-09T00:00:00-07:00` or `2026-06-09T23:59:59Z`                                                 |
| `compare_start`, `compare_end` | ISO 8601 / RFC3339 instant with offset | comparison chart only                                                                                      |
| `tz`                           | IANA zone (e.g. `America/Los_Angeles`) | **display only** — labels chart axes and formats response timestamps; does **not** affect which rows match |

The instant fully specifies the query window, so the server does **no**
calendar-day interpretation — it parses each value with `time.Parse(time.RFC3339, …)`
and compares unix seconds. A bare `YYYY-MM-DD`, a naive datetime without an
offset, or a raw unix integer is rejected (`400`).

Because both the summary card and the chart send the _same_ instants, they
always resolve the same window. See [the worked example](#worked-example-the-5pm-pst-gap).

### Report generation — calendar dates

`POST /api/generate_report` (and the `velocity report pdf` CLI) take a calendar
period, not an instant:

| Field (JSON body)                        | Format                               | Notes                                                              |
| ---------------------------------------- | ------------------------------------ | ------------------------------------------------------------------ |
| `start_date`, `end_date`                 | `YYYY-MM-DD` (ISO 8601 date profile) | the reporting period                                               |
| `compare_start_date`, `compare_end_date` | `YYYY-MM-DD`                         | optional comparison period                                         |
| `timezone`                               | IANA zone                            | interprets the dates into a UTC window **and** sets report display |

A report is a human document: `start_date`/`end_date` are stored on the
`site_report` row, printed in the PDF, and embedded in the report **filename**
(e.g. `2025-12-03_velocity.report_…_report.pdf`). A colon-bearing instant cannot
live in a filename, so the report deliberately stays on the date profile. The
server expands `end_date` to an inclusive end-of-day in `timezone`.

### Other endpoints

`GET /api/events` accepts a `timezone` parameter for display only; it does not
take a date range.

## API outputs

Outputs are **not** unified — each surface returns whatever is most useful to
its consumer, and clients parse accordingly:

- `radar_stats` returns `start_time` as an RFC3339 string converted into the
  requested `tz` (the frontend reads it with `new Date(...)`).
- `timeline` returns raw unix seconds (`start_unix`, `end_unix`, `data_range`).

The "single date string format" rule governs what clients **send**, not what the
server serializes back.

## Frontend

The date picker (`svelte-ux` `DateRangeField`) operates on browser-local
calendar days. When building query parameters the frontend converts a picked day
into the exact instant for that day's start/end **in the selected display
timezone**, via `isoStartOfDay` / `isoEndOfDay` ([web/src/lib/dateUtils.ts](../../../web/src/lib/dateUtils.ts)):

```ts
start = isoStartOfDay(dateRange.from, $displayTimezone); // 00:00:00 of the day in tz
end = isoEndOfDay(dateRange.to, $displayTimezone); //   23:59:59 of the day in tz
```

These use `Intl.DateTimeFormat` offset math so they are correct across DST. The
display timezone therefore **shifts** the queried window (not just the labels):
choosing `UTC` vs `America/Los_Angeles` asks for different days.

Report generation instead sends `YYYY-MM-DD` via `isoDate`, matching the report
endpoint's calendar-date contract.

`displayTimezone` defaults to the server's configured zone, falling back to the
browser's IANA zone when the server default is `UTC` and the user has no stored
preference ([web/src/lib/timezone.ts](../../../web/src/lib/timezone.ts)).

## Worked example: the 5pm-PST gap

The design exists to kill a specific class of bug. Before unification, the
Vehicle Count card sent raw unix timestamps while the chart sent `YYYY-MM-DD`
re-interpreted server-side in a (often `UTC`) timezone. Between 5pm and midnight
Pacific the local date and the UTC date differ, so the two endpoints queried
different windows: the card counted a transit the chart had excluded, leaving a
populated card above an empty graph.

With instants, the frontend computes one window — `isoStartOfDay`/`isoEndOfDay`
in the display zone — and sends the identical `start`/`end` to both endpoints.
There is no server-side day interpretation left to disagree about, so the card
and chart cannot diverge.

## Summary

| Concern                                     | Representation               |
| ------------------------------------------- | ---------------------------- |
| DB storage                                  | Unix epoch seconds, UTC      |
| Query API input (`start`/`end`/`compare_*`) | ISO 8601 instant with offset |
| Query API `tz`                              | IANA zone, display only      |
| Report API input (`*_date`)                 | `YYYY-MM-DD` (ISO 8601 date) |
| `radar_stats` output time                   | RFC3339 in display tz        |
| `timeline` output time                      | Unix seconds                 |
