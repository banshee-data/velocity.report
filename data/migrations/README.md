Local migrations for sensor data database
======================================

This folder contains SQL migration scripts intended to be applied directly
against the local SQLite sensor database (commonly `sensor_data.db`). These
migrations are intentionally simple SQL files so they can be applied without
Docker or other tooling.

Quick safety checklist
----------------------

- Always make a backup before running a migration:

  cp sensor_data.db sensor_data.db.bak

- Run the migration from the repository root:

  sqlite3 sensor_data.db < data/migrations/20250929_migrate_data_to_radar_data.sql

- Inspect the database after migration and before deleting backups.

About PRAGMA and transactional behavior
--------------------------------------

SQLite supports running DDL inside transactions, but behaviour can vary with
PRAGMA settings. The repository's canonical `internal/db/schema.sql` sets some
PRAGMA values (WAL, busy_timeout, etc.). When running migrations by hand you
may want to temporarily set WAL mode and a busy timeout to reduce contention.

Recommended sequence for a single-machine migration:

1. Ensure no other process is writing to the DB (stop services that use it).
2. Create a backup (see above).
3. Optionally enable WAL and a larger busy timeout for long migrations:

   sqlite3 sensor_data.db "PRAGMA journal_mode=WAL; PRAGMA busy_timeout=10000;"

4. Run the migration SQL file(s):

   sqlite3 sensor_data.db < data/migrations/20250929_migrate_data_to_radar_data.sql

5. Inspect results, then remove backup when satisfied.

Baselining and migrate CLI
--------------------------

If you later adopt a CLI migration tool (like golang-migrate) you can "baseline"
the DB at the current schema version so the CLI won't try to re-apply previous
changes. Baseline steps vary by tool; consult the tool's docs.

Notes about this repository's migrations
---------------------------------------

- Existing migration files in this folder follow the pattern `YYYYMMDD_...`.
- The migration `20250929_migrate_data_to_radar_data.sql` migrates the legacy
  `data` table into the newer `radar_data` schema by creating a new table,
  copying data across (converting textual timestamps where present), renaming
  the old table to `data_old`, and moving the new table into place.
- The migration intentionally leaves `data_old` in place so you can verify
  results before dropping it.
- The migration `20251014_create_site_table.sql` creates the `site` table for
  storing site-specific configurations including location, radar settings, and
  contact information. It also creates an index on site.name, a trigger to
  auto-update timestamps, and inserts a default site record.

If you want me to add a tiny shell helper (e.g. `scripts/run-data-migration.sh`)
that performs backup + migration + basic verification, tell me the desired
name and I'll add it.
