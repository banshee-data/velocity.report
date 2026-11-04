-- A transit's direction from the sign of its readings
-- (docs/plans/platform-speed-by-direction-reporting-plan.md, Item 1). The
-- OPS243 reports approaching traffic as a positive speed and receding as
-- negative, and radar_data keeps the sign; the worker took the magnitude
-- only. Existing rows read unknown until the worker rebuilds them.
    ALTER TABLE radar_data_transits
      ADD COLUMN direction TEXT NOT NULL DEFAULT 'unknown' CHECK (direction IN ('inbound', 'outbound', 'mixed', 'unknown'));

-- The radar deployment's two directions (names, bearings, which sign is
-- inbound, and each direction's limits as signed with their effective
-- dates), as JSON on the config period until deployments exist.
    ALTER TABLE site_config_periods
      ADD COLUMN directions_json TEXT CHECK (
directions_json IS NULL
       OR JSON_VALID(directions_json)
          );
