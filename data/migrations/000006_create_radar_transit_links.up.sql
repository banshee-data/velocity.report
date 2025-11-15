-- Migration: Create radar_transit_links table
-- Date: 2025-11-15
-- Description: Add join table linking radar_data_transits to radar_data

CREATE TABLE IF NOT EXISTS radar_transit_links (
    link_id INTEGER PRIMARY KEY AUTOINCREMENT,
    transit_id INTEGER NOT NULL REFERENCES radar_data_transits (transit_id) ON DELETE CASCADE,
    data_rowid INTEGER NOT NULL REFERENCES radar_data (rowid) ON DELETE CASCADE,
    link_score DOUBLE,
    created_at DOUBLE DEFAULT (UNIXEPOCH('subsec')),
    UNIQUE (transit_id, data_rowid)
);

CREATE INDEX IF NOT EXISTS idx_transit_links_transit ON radar_transit_links (transit_id);

CREATE INDEX IF NOT EXISTS idx_transit_links_data ON radar_transit_links (data_rowid);
