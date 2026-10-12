// LabelSetCaptures.swift
// What the annotation window says about the capture files a label set has
// been verified against, and about the version 1 pack it was migrated from.
//
// A label set cites returns by packet key, so it applies to any file that
// holds its packets. `velocity lidar labels bind` reads one file and records
// in labels.json how many of the set's packets and frames that file holds.
// These lines say which files those are, so an operator knows which
// recording to replay beside the labels. The window reads them; only Go's
// `bind` writes them.

import Foundation

enum LabelSetCaptures {
    /// How many verified files are listed before the rest are only counted.
    static let listed = 3

    /// The lines shown under the label set's summary: its verified capture
    /// files, or the command that verifies one, then where it was migrated
    /// from. `packets` and `windows` are the label set's own counts, which
    /// each file's are read against.
    static func lines(
        manifest: LabelSetManifest, packets: Int, windows: Int, directory: URL
    ) -> [String] {
        var out: [String] = []
        let bindings = manifest.appliesTo
        if bindings.isEmpty {
            out.append("Not verified against any capture file. Verify one with:")
            out.append(bindCommand(directory: directory))
        } else {
            out.append("Verified capture files:")
            for binding in bindings.prefix(listed) {
                out.append(line(binding, packets: packets, windows: windows))
            }
            if bindings.count > listed { out.append("and \(bindings.count - listed) more") }
        }
        if let migration = manifest.migratedFrom { out.append(migrationLine(migration)) }
        return out
    }

    /// One verified file: its name, the start of its SHA-256, and how much
    /// of the label set it holds. Frames are counted whole; frames the file
    /// holds only some packets of are counted after.
    static func line(_ binding: LabelSetBinding, packets: Int, windows: Int) -> String {
        var text =
            "\(binding.name) · \(shortDigest(binding.sha256)) · "
            + "\(binding.packetsFound) of \(packets) packets · "
            + "\(binding.windowsComplete) of \(windows) frames"
        if binding.windowsPartial > 0 { text += ", \(binding.windowsPartial) partly" }
        return text
    }

    /// The first eight hex digits of a `sha256:` digest.
    static func shortDigest(_ digest: String) -> String {
        let hex = digest.hasPrefix("sha256:") ? digest.dropFirst(7) : Substring(digest)
        return String(hex.prefix(8))
    }

    /// The command that checks a capture file holds the label set's packets
    /// and records it.
    static func bindCommand(directory: URL) -> String {
        "velocity lidar labels bind --labels \(directory.path) --pcap <file>"
    }

    /// The version 1 pack and sidecar revision a migrated label set came
    /// from. A migration that did not record the pack's directory is named by
    /// its dataset.
    static func migrationLine(_ migration: LabelSetMigration) -> String {
        let pack = migration.packDir.flatMap { $0.isEmpty ? nil : $0 } ?? migration.datasetID
        return "Migrated from version 1 pack \(pack), revision \(migration.sidecarRevision)"
    }
}
