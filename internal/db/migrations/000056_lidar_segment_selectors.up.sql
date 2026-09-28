-- Migration: name the selector that chose each segment
-- Date: 2026-09-27
-- Description: Windows are chosen by selectors defined in
-- config/segment-selectors.defaults.json: a finder at its parameters, the
-- measure that ranks what it finds, and the bounds a window must meet. A
-- selection records its selector as it ran, in selector_json, because the
-- file can say something else by the time anyone asks.
--
-- The column is added rather than the table rebuilt. Rows chosen before
-- selectors existed keep NULL; a trigger refuses a new row without one, and
-- another refuses erasing one. The CHECK ties the selector to the finder
-- that ranked the window, and holds the held-out rule: a held-out window is
-- chosen only by a selector the code found eligible when it ranked. It reads
-- the flag with IS 1, not = 1, so that a document without the flag is
-- refused: = would give NULL, and a CHECK accepts NULL.
    ALTER TABLE lidar_segment_selections
      ADD COLUMN selector_json TEXT CHECK (
selector_json IS NULL
       OR (
          JSON_VALID(selector_json)
      AND JSON_TYPE(selector_json) = 'object'
      AND TYPEOF(JSON_EXTRACT(selector_json, '$.id')) = 'text'
      AND JSON_EXTRACT(selector_json, '$.id') != ''
      AND JSON_EXTRACT(selector_json, '$.digest') LIKE 'sha256:_%'
      AND JSON_EXTRACT(selector_json, '$.finder') IS finder
      AND (
          role = 'tuning'
       OR JSON_EXTRACT(selector_json, '$.held_out_eligible') IS 1
          )
          )
          );

CREATE TRIGGER lidar_segment_selections_name_selector BEFORE INSERT ON lidar_segment_selections WHEN NEW.selector_json IS NULL BEGIN
   SELECT RAISE (ABORT, 'a segment selection names the selector that chose it');

END;

CREATE TRIGGER lidar_segment_selections_keep_selector BEFORE
   UPDATE OF selector_json ON lidar_segment_selections WHEN OLD.selector_json IS NOT NULL
      AND NEW.selector_json IS NULL BEGIN
             SELECT RAISE (ABORT, 'a segment selection keeps the selector that chose it');

END;
