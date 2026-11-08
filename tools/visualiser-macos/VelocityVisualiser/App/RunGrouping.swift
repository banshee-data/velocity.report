// RunGrouping.swift
// The run browser's groups: one per clip, repeats folded, and the
// Annotation window's title. Pure, so the shapes are tested without a view.

import Foundation

/// A run shown as a row, with the runs folded under it: the consecutive runs
/// after it on the same capture with the same parameters.
struct RunFoldRow: Identifiable {
    let run: Run
    let folded: [Run]

    var id: String { run.runId }

    /// Whether a run is one of this row's folded repeats.
    func folds(_ runID: String?) -> Bool {
        guard let runID else { return false }
        return folded.contains { $0.runId == runID }
    }

    /// The fold's line under the row, or nil when nothing is folded.
    var foldLabel: String? { folded.isEmpty ? nil : "+ \(folded.count) more, identical parameters" }
}

/// The runs of one clip, or the runs with no clip.
struct RunClipGroup: Identifiable {
    /// Nil for the "No clip" group.
    let clipID: String?
    let title: String
    let site: String?
    let rows: [RunFoldRow]

    var id: String { clipID ?? "" }

    /// Every run in the group, folded ones included.
    var runCount: Int { rows.reduce(0) { $0 + 1 + $1.folded.count } }
}

enum RunGrouping {
    static let noClipTitle = "No clip"

    /// The clip a run was made from, or nil.
    static func clipID(of run: Run) -> String? {
        let clip = run.clipId?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return clip.isEmpty ? nil : clip
    }

    /// Group runs by clip, in the order the runs come (the server's is newest
    /// first, so the clip with the newest run leads), with "No clip" last.
    /// Within a group the runs keep their order and repeats fold.
    static func groups(_ runs: [Run], clips: [String: ClipSummary]) -> [RunClipGroup] {
        var order: [String] = []
        var byClip: [String: [Run]] = [:]
        var unclipped: [Run] = []
        for run in runs {
            guard let clip = clipID(of: run) else {
                unclipped.append(run)
                continue
            }
            if byClip[clip] == nil { order.append(clip) }
            byClip[clip, default: []].append(run)
        }
        var groups = order.map { clip in
            RunClipGroup(
                clipID: clip, title: clips[clip]?.name ?? "Clip \(clip.prefix(8))",
                site: clips[clip]?.site, rows: fold(byClip[clip] ?? []))
        }
        if !unclipped.isEmpty {
            groups.append(
                RunClipGroup(clipID: nil, title: noClipTitle, site: nil, rows: fold(unclipped)))
        }
        return groups
    }

    /// Fold each run with the consecutive runs after it that repeat it.
    static func fold(_ runs: [Run]) -> [RunFoldRow] {
        var rows: [RunFoldRow] = []
        var lead: Run?
        var folded: [Run] = []
        for run in runs {
            if let current = lead, repeats(run, current) {
                folded.append(run)
                continue
            }
            if let current = lead { rows.append(RunFoldRow(run: current, folded: folded)) }
            lead = run
            folded = []
        }
        if let current = lead { rows.append(RunFoldRow(run: current, folded: folded)) }
        return rows
    }

    /// Whether a run repeats another: the same capture read with the same
    /// parameters and the same build, ending the same way. A run without
    /// either digest repeats nothing. The status and the recording are
    /// compared so a failed run never hides an openable one behind its fold,
    /// and the configuration digest, which includes the build, so a rerun
    /// after a perception change is not called identical.
    static func repeats(_ run: Run, _ other: Run) -> Bool {
        guard let params = nonEmpty(run.paramsHash), params == nonEmpty(other.paramsHash),
            let config = nonEmpty(run.configHash), config == nonEmpty(other.configHash)
        else { return false }
        return clipID(of: run) == clipID(of: other) && run.sourcePath == other.sourcePath
            && run.sensorId == other.sensorId && run.status == other.status
            && run.hasVRLog == other.hasVRLog
    }

    private static func nonEmpty(_ value: String?) -> String? {
        guard let value, !value.isEmpty else { return nil }
        return value
    }
}

/// The Annotation window's title.
enum AnnotationWindowTitle {
    /// "Annotation: <clip> · <pack> · window i of n". Without a clip the run
    /// ID stands in for it, and is left out when the pack's name already
    /// starts with it. The window position is shown only when it is known.
    static func format(
        clip: String?, pack: String?, runID: String? = nil, window: (index: Int, count: Int)? = nil
    ) -> String {
        guard let pack, !pack.isEmpty else { return "Annotation" }
        var parts: [String] = []
        if let clip, !clip.isEmpty {
            parts.append(clip)
        } else if let runID, !runID.isEmpty, !pack.hasPrefix(runID) {
            parts.append(runID)
        }
        parts.append(pack)
        if let window, window.count > 0, (1...window.count).contains(window.index) {
            parts.append("window \(window.index) of \(window.count)")
        }
        return "Annotation: " + parts.joined(separator: " · ")
    }

    /// The run a pack was cut from: its VRLOG directory is named for the run.
    static func runID(ofVRLogPath path: String) -> String? {
        let name = (path as NSString).lastPathComponent
        return name.isEmpty ? nil : name
    }
}
