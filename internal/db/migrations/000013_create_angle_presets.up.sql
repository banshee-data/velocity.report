-- Create angle_presets table for storing angle configurations with colors
CREATE TABLE IF NOT EXISTS angle_presets (
      id INTEGER PRIMARY KEY AUTOINCREMENT
    , angle REAL NOT NULL UNIQUE
    , color_hex TEXT NOT NULL
    , is_system INTEGER NOT NULL DEFAULT 0
    , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
    , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
);

-- Create index on angle for quick lookups
CREATE INDEX IF NOT EXISTS idx_angle_presets_angle ON angle_presets(angle);

-- Create index on is_system for filtering
CREATE INDEX IF NOT EXISTS idx_angle_presets_is_system ON angle_presets(is_system);

-- Create trigger to prevent deletion of system presets
CREATE TRIGGER IF NOT EXISTS prevent_system_preset_deletion
BEFORE DELETE ON angle_presets
FOR EACH ROW
WHEN OLD.is_system = 1
BEGIN
    SELECT RAISE (ABORT, 'Cannot delete system preset');
END;

-- Create trigger to update timestamp
CREATE TRIGGER IF NOT EXISTS update_angle_presets_timestamp
AFTER UPDATE ON angle_presets
BEGIN
    UPDATE angle_presets
       SET updated_at = STRFTIME('%s', 'now')
     WHERE id = NEW.id;
END;

-- Seed system presets with distinct colors
INSERT INTO angle_presets (angle, color_hex, is_system) VALUES
    (5.0,  '#3B82F6', 1),  -- Blue 500
    (15.0, '#10B981', 1),  -- Green 500
    (30.0, '#F59E0B', 1),  -- Amber 500
    (45.0, '#EF4444', 1),  -- Red 500
    (60.0, '#8B5CF6', 1);  -- Purple 500
