// ReaderLevel.swift
// The reader level this build has, as Go's annotation.ReaderLevel.
//
// A pack's manifest.json, a label set's labels.json and
// physical-references.json say in min_reader the lowest level that reads them
// correctly. schema_version changes when a format breaks; min_reader rises
// when a writer uses something an older reader would misread or drop, so an
// older build refuses the file and says which build reads it. A file with no
// min_reader is level 1.

import Foundation

enum ReaderLevel {
    static let current = 1

    /// Why this build cannot read a file that needs `minReader`, or nil.
    static func refusal(_ what: String, minReader: Int?) -> String? {
        guard let minReader, minReader > current else { return nil }
        return "\(what) needs reader level \(minReader), and this build reads level \(current): "
            + "update velocity.report to read it"
    }
}
