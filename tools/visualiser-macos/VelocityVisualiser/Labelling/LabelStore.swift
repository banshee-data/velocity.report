// LabelStore.swift
// Local storage and export for track labels.
//
// Labels are stored in a local SQLite database and can be exported to JSON
// for use in the ML training pipeline.

import Foundation
import SQLite3

/// Manages local storage of track labels.
class LabelStore {

    // MARK: - Properties

    private var db: OpaquePointer?
    private let dbPath: String

    /// Current session ID for grouping labels.
    var sessionID: String = UUID().uuidString

    /// Source file being annotated (for replay mode).
    var sourceFile: String = ""

    // MARK: - Initialisation

    init() {
        // Store in app support directory
        let appSupport = FileManager.default.urls(
            for: .applicationSupportDirectory, in: .userDomainMask
        ).first!
        let appDir = appSupport.appendingPathComponent("VelocityVisualiser", isDirectory: true)

        try? FileManager.default.createDirectory(at: appDir, withIntermediateDirectories: true)

        dbPath = appDir.appendingPathComponent("labels.sqlite").path
        openDatabase()
        createTables()
    }

    deinit { sqlite3_close(db) }

    // MARK: - Database Setup

    private func openDatabase() {
        if sqlite3_open(dbPath, &db) != SQLITE_OK {
            print("Failed to open label database: \(String(cString: sqlite3_errmsg(db)))")
        }
    }

    private func createTables() {
        let sql = """
            CREATE TABLE IF NOT EXISTS labels (
                label_id TEXT PRIMARY KEY,
                session_id TEXT NOT NULL,
                source_file TEXT,
                track_id TEXT NOT NULL,
                class_label TEXT NOT NULL,
                start_frame_id INTEGER,
                end_frame_id INTEGER,
                created_ns INTEGER NOT NULL,
                annotator TEXT,
                notes TEXT
            );

            CREATE INDEX IF NOT EXISTS idx_labels_session ON labels(session_id);
            CREATE INDEX IF NOT EXISTS idx_labels_track ON labels(track_id);
            """

        if sqlite3_exec(db, sql, nil, nil, nil) != SQLITE_OK {
            print("Failed to create labels table: \(String(cString: sqlite3_errmsg(db)))")
        }
    }

    // MARK: - Label Operations

    /// Add a new label for a track.
    func addLabel(
        trackID: String, classLabel: String, startFrameID: UInt64? = nil, endFrameID: UInt64? = nil,
        annotator: String = "", notes: String = ""
    ) -> LabelEvent {
        let label = LabelEvent(
            id: UUID().uuidString, trackID: trackID, classLabel: classLabel,
            startFrameID: startFrameID ?? 0, endFrameID: endFrameID ?? 0,
            createdNanos: Int64(Date().timeIntervalSince1970 * 1_000_000_000), annotator: annotator,
            notes: notes)

        insertLabel(label)
        return label
    }

    private func insertLabel(_ label: LabelEvent) {
        let sql = """
            INSERT OR REPLACE INTO labels
            (label_id, session_id, source_file, track_id, class_label,
             start_frame_id, end_frame_id, created_ns, annotator, notes)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """

        var stmt: OpaquePointer?
        if sqlite3_prepare_v2(db, sql, -1, &stmt, nil) == SQLITE_OK {
            sqlite3_bind_text(stmt, 1, label.id, -1, nil)
            sqlite3_bind_text(stmt, 2, sessionID, -1, nil)
            sqlite3_bind_text(stmt, 3, sourceFile, -1, nil)
            sqlite3_bind_text(stmt, 4, label.trackID, -1, nil)
            sqlite3_bind_text(stmt, 5, label.classLabel, -1, nil)
            sqlite3_bind_int64(stmt, 6, Int64(label.startFrameID))
            sqlite3_bind_int64(stmt, 7, Int64(label.endFrameID))
            sqlite3_bind_int64(stmt, 8, label.createdNanos)
            sqlite3_bind_text(stmt, 9, label.annotator, -1, nil)
            sqlite3_bind_text(stmt, 10, label.notes, -1, nil)

            if sqlite3_step(stmt) != SQLITE_DONE {
                print("Failed to insert label: \(String(cString: sqlite3_errmsg(db)))")
            }
        }
        sqlite3_finalize(stmt)
    }

    /// Get all labels for the current session.
    func getLabelsForSession() -> [LabelEvent] {
        var labels: [LabelEvent] = []

        let sql = "SELECT * FROM labels WHERE session_id = ? ORDER BY created_ns"
        var stmt: OpaquePointer?

        if sqlite3_prepare_v2(db, sql, -1, &stmt, nil) == SQLITE_OK {
            sqlite3_bind_text(stmt, 1, sessionID, -1, nil)

            while sqlite3_step(stmt) == SQLITE_ROW {
                let label = LabelEvent(
                    id: String(cString: sqlite3_column_text(stmt, 0)),
                    trackID: String(cString: sqlite3_column_text(stmt, 3)),
                    classLabel: String(cString: sqlite3_column_text(stmt, 4)),
                    startFrameID: UInt64(sqlite3_column_int64(stmt, 5)),
                    endFrameID: UInt64(sqlite3_column_int64(stmt, 6)),
                    createdNanos: sqlite3_column_int64(stmt, 7),
                    annotator: sqlite3_column_text(stmt, 8).map { String(cString: $0) } ?? "",
                    notes: sqlite3_column_text(stmt, 9).map { String(cString: $0) } ?? "")
                labels.append(label)
            }
        }
        sqlite3_finalize(stmt)

        return labels
    }

    /// Get all labels for a specific track.
    func getLabelsForTrack(_ trackID: String) -> [LabelEvent] {
        var labels: [LabelEvent] = []

        let sql = "SELECT * FROM labels WHERE track_id = ? ORDER BY created_ns"
        var stmt: OpaquePointer?

        if sqlite3_prepare_v2(db, sql, -1, &stmt, nil) == SQLITE_OK {
            sqlite3_bind_text(stmt, 1, trackID, -1, nil)

            while sqlite3_step(stmt) == SQLITE_ROW {
                let label = LabelEvent(
                    id: String(cString: sqlite3_column_text(stmt, 0)),
                    trackID: String(cString: sqlite3_column_text(stmt, 3)),
                    classLabel: String(cString: sqlite3_column_text(stmt, 4)),
                    startFrameID: UInt64(sqlite3_column_int64(stmt, 5)),
                    endFrameID: UInt64(sqlite3_column_int64(stmt, 6)),
                    createdNanos: sqlite3_column_int64(stmt, 7),
                    annotator: sqlite3_column_text(stmt, 8).map { String(cString: $0) } ?? "",
                    notes: sqlite3_column_text(stmt, 9).map { String(cString: $0) } ?? "")
                labels.append(label)
            }
        }
        sqlite3_finalize(stmt)

        return labels
    }

    /// Delete a label by ID.
    func deleteLabel(_ labelID: String) {
        let sql = "DELETE FROM labels WHERE label_id = ?"
        var stmt: OpaquePointer?

        if sqlite3_prepare_v2(db, sql, -1, &stmt, nil) == SQLITE_OK {
            sqlite3_bind_text(stmt, 1, labelID, -1, nil)
            sqlite3_step(stmt)
        }
        sqlite3_finalize(stmt)
    }

    /// Clear all labels for the current session.
    func clearSession() {
        let sql = "DELETE FROM labels WHERE session_id = ?"
        var stmt: OpaquePointer?

        if sqlite3_prepare_v2(db, sql, -1, &stmt, nil) == SQLITE_OK {
            sqlite3_bind_text(stmt, 1, sessionID, -1, nil)
            sqlite3_step(stmt)
        }
        sqlite3_finalize(stmt)
    }

    // MARK: - Export

    /// Export labels to JSON.
    func exportToJSON() -> Data? {
        let labels = getLabelsForSession()
        let labelSet = LabelSet(sessionID: sessionID, sourceFile: sourceFile, labels: labels)

        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]

        return try? encoder.encode(labelSet)
    }

    /// Export labels to a file.
    func exportToFile(_ url: URL) throws {
        guard let data = exportToJSON() else { throw ExportError.encodingFailed }
        try data.write(to: url)
    }

    enum ExportError: Error { case encodingFailed }
}
