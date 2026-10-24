// StreetUseEditor.swift
// The street painter's marks and their unsaved changes. Nothing here talks
// to a server or draws.
//
// Ported from the web street editor (web/src/lib/streets/editor.ts). A mark
// is one surface, travel lane, parked-car space or driveway as the operator
// edits it: its vertices in the site frame, and its tags. The marks
// ListStreetUse returned are the base; the batch Save sends is the
// difference between the base and the marks now, so a mark moved and moved
// back sends nothing. Every change keeps the previous marks for Undo, and an
// undone change is kept for Redo until the next change.
//
// Gerardus snaps, drapes and checks every edit again (EditStreetUse), so a
// new vertex's height is only a guess for drawing: the ground under the
// cursor, or 0 where there is no ground mesh.

import Foundation
import SwiftProtobuf
import simd

/// The kinds the painter draws, and what each needs.
enum StreetUseKind: String, CaseIterable, Sendable {
    case surface
    case travelLane
    case parkingSpace
    case driveway

    init?(_ kind: Gerardus_Align_V1alpha1_FeatureKind) {
        switch kind {
        case .surface: self = .surface
        case .travelLane: self = .travelLane
        case .parkingSpace: self = .parkingSpace
        case .driveway: self = .driveway
        default: return nil
        }
    }

    var proto: Gerardus_Align_V1alpha1_FeatureKind {
        switch self {
        case .surface: return .surface
        case .travelLane: return .travelLane
        case .parkingSpace: return .parkingSpace
        case .driveway: return .driveway
        }
    }

    /// A surface and a space are closed polygons; a lane and a driveway are
    /// open lines.
    var closed: Bool { self == .surface || self == .parkingSpace }

    /// The fewest vertices the kind needs, and the most (a driveway is one
    /// span of kerb).
    var minVertices: Int { closed ? 3 : 2 }
    var maxVertices: Int { self == .driveway ? 2 : .max }

    var label: String {
        switch self {
        case .surface: return "Surface"
        case .travelLane: return "Travel lane"
        case .parkingSpace: return "Parking space"
        case .driveway: return "Driveway"
        }
    }
}

/// The tag values each kind offers, as Gerardus's proto comments list them.
enum StreetUseTags {
    static let surface = "surface"
    static let laneType = "lane_type"
    static let widthM = "width_m"
    static let spaceType = "space_type"
    static let count = "count"
    static let timeLimit = "time_limit"

    static let surfaces = ["carriageway", "sidewalk", "verge", "apron", "median"]
    static let laneTypes = ["general", "bus", "bike", "turn"]
    static let spaceTypes = ["metered", "permit", "loading", "disabled", "free"]

    /// What a new mark's tool fields hold.
    struct Fields: Equatable, Sendable {
        var surface = "carriageway"
        var laneType = "general"
        var widthM = 3.3
        var spaceType = "metered"
        var count = 1
        var timeLimit = ""
    }

    /// The tags a new mark of each kind starts with, from the tool's fields.
    static func tags(for kind: StreetUseKind, _ fields: Fields) -> [String: String] {
        switch kind {
        case .surface: return [surface: fields.surface]
        case .travelLane:
            var tags = [laneType: fields.laneType]
            if fields.widthM.isFinite, fields.widthM > 0 { tags[widthM] = format(fields.widthM) }
            return tags
        case .parkingSpace:
            var tags = [spaceType: fields.spaceType, count: String(max(1, fields.count))]
            let limit = fields.timeLimit.trimmingCharacters(in: .whitespaces)
            if !limit.isEmpty { tags[timeLimit] = limit }
            return tags
        case .driveway: return [:]
        }
    }

    /// A number as a tag value: "3.3", "3", never "3.3000000000000003".
    static func format(_ value: Double) -> String {
        let rounded = (value * 100).rounded() / 100
        if rounded == rounded.rounded() { return String(Int(rounded)) }
        return String(rounded)
    }

    /// Trimmed, with empty values dropped.
    static func cleaned(_ tags: [String: String]) -> [String: String] {
        var out = [String: String]()
        for (key, value) in tags {
            let v = value.trimmingCharacters(in: .whitespaces)
            if !v.isEmpty { out[key] = v }
        }
        return out
    }
}

/// One mark as the operator edits it.
struct StreetMark: Equatable, Identifiable, Sendable {
    /// Stable within the session: Gerardus's id, or a local key for a mark
    /// not saved yet.
    var key: String
    /// Gerardus's feature_id; empty for a new mark.
    var featureID: String
    var kind: StreetUseKind
    /// Vertices in the site frame. A closed mark's ring is open: its last
    /// vertex is not its first. z is the ground as last known (Gerardus's
    /// drape on a saved vertex, the ground mesh on a new one, or 0).
    var points: [SIMD3<Double>]
    var tags: [String: String]
    var author: String = ""
    var editedAt: Date?
    var rowID: String = ""

    var id: String { key }
    var plan: [PlanPoint] { points.map { PlanPoint($0.x, $0.y) } }

    /// "metered · 3 spaces · 2hr", "bus · 3.3 m", "sidewalk"; "" for a
    /// driveway.
    var summary: String {
        switch kind {
        case .surface: return tags[StreetUseTags.surface] ?? ""
        case .travelLane:
            return [tags[StreetUseTags.laneType], tags[StreetUseTags.widthM].map { "\($0) m" }]
                .compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
        case .parkingSpace:
            let n = Int(tags[StreetUseTags.count] ?? "") ?? 1
            return [
                tags[StreetUseTags.spaceType], n > 1 ? "\(n) spaces" : "1 space",
                tags[StreetUseTags.timeLimit],
            ].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
        case .driveway: return ""
        }
    }
}

enum StreetUseEditing {
    /// The street-use features as marks. Features of another kind, or with
    /// too few vertices for their kind, are left out and counted.
    static func marks(
        from features: [Gerardus_Align_V1alpha1_StreetUseFeature]
    ) -> (marks: [StreetMark], skipped: Int) {
        var marks = [StreetMark]()
        var skipped = 0
        for feature in features {
            guard let kind = StreetUseKind(feature.kind) else {
                skipped += 1
                continue
            }
            var points = stride(from: 0, to: feature.xyz.count - 2, by: 3).map { i in
                SIMD3(
                    Double(feature.xyz[i]), Double(feature.xyz[i + 1]), Double(feature.xyz[i + 2]))
            }
            // A ring sent closed repeats its first vertex at the end.
            if kind.closed, points.count > 1, let first = points.first, let last = points.last,
                first.x == last.x, first.y == last.y
            {
                points.removeLast()
            }
            guard points.count >= kind.minVertices else {
                skipped += 1
                continue
            }
            let key = feature.featureID.isEmpty ? "unnamed-\(marks.count)" : feature.featureID
            marks.append(
                StreetMark(
                    key: key, featureID: feature.featureID, kind: kind, points: points,
                    tags: feature.tags, author: feature.author,
                    editedAt: feature.hasEditedAt ? feature.editedAt.date : nil,
                    rowID: feature.rowID))
        }
        return (marks, skipped)
    }

    /// A mark as the feature an upsert carries: its id (empty for a new mark),
    /// kind, vertices and non-empty tags. Who and when are the server's to
    /// fill in.
    static func feature(of mark: StreetMark) -> Gerardus_Align_V1alpha1_StreetUseFeature {
        var feature = Gerardus_Align_V1alpha1_StreetUseFeature()
        feature.featureID = mark.featureID
        feature.kind = mark.kind.proto
        feature.xyz = mark.points.flatMap { [Float($0.x), Float($0.y), Float($0.z)] }
        feature.closed = mark.kind.closed
        feature.tags = StreetUseTags.cleaned(mark.tags)
        return feature
    }

    /// Whether two marks would send the same feature: same kind, the same
    /// vertices across the ground, and the same tags. Heights are left out:
    /// Gerardus drapes every vertex again.
    static func same(_ a: StreetMark, _ b: StreetMark) -> Bool {
        a.kind == b.kind && a.plan == b.plan
            && StreetUseTags.cleaned(a.tags) == StreetUseTags.cleaned(b.tags)
    }

    /// The batch that turns base into marks: a remove for each saved mark no
    /// longer there, then an upsert for each new or changed mark.
    static func diff(
        base: [StreetMark], marks: [StreetMark]
    ) -> [Gerardus_Align_V1alpha1_StreetUseEdit] {
        var byID = [String: StreetMark]()
        for mark in base where !mark.featureID.isEmpty { byID[mark.featureID] = mark }
        let kept = Set(marks.map(\.featureID).filter { !$0.isEmpty })
        var edits = [Gerardus_Align_V1alpha1_StreetUseEdit]()
        for mark in base where !mark.featureID.isEmpty && !kept.contains(mark.featureID) {
            var edit = Gerardus_Align_V1alpha1_StreetUseEdit()
            edit.removeFeatureID = mark.featureID
            edits.append(edit)
        }
        for mark in marks {
            if let before = byID[mark.featureID], same(before, mark) { continue }
            var edit = Gerardus_Align_V1alpha1_StreetUseEdit()
            edit.upsert = feature(of: mark)
            edits.append(edit)
        }
        return edits
    }

    // MARK: Changing one mark

    /// Moves one vertex.
    static func moveVertex(_ mark: StreetMark, _ index: Int, to point: SIMD3<Double>) -> StreetMark
    {
        guard mark.points.indices.contains(index) else { return mark }
        var out = mark
        out.points[index] = point
        return out
    }

    /// Removes one vertex, unless the mark would have fewer than its kind needs.
    static func removeVertex(_ mark: StreetMark, _ index: Int) -> StreetMark {
        guard mark.points.indices.contains(index), mark.points.count - 1 >= mark.kind.minVertices
        else { return mark }
        var out = mark
        out.points.remove(at: index)
        return out
    }

    /// Adds a vertex before `index` (at the end when `index` is the count),
    /// unless the kind holds no more.
    static func insertVertex(
        _ mark: StreetMark, at index: Int, _ point: SIMD3<Double>
    ) -> StreetMark {
        guard index >= 0, index <= mark.points.count, mark.points.count < mark.kind.maxVertices
        else { return mark }
        var out = mark
        out.points.insert(point, at: index)
        return out
    }

    /// A lane the other way round: its direction of travel is the order of
    /// its vertices.
    static func reversed(_ mark: StreetMark) -> StreetMark {
        var out = mark
        out.points.reverse()
        return out
    }

    static func settingTag(_ mark: StreetMark, _ key: String, _ value: String) -> StreetMark {
        var out = mark
        out.tags[key] = value
        return out
    }

    // MARK: Problems

    /// EditStreetUse's problems ("<id>: message") by the feature they name.
    /// Ids hold colons themselves ("gms:<ulid>"), so known ids are matched as
    /// prefixes, longest first, rather than split at the first colon. A
    /// problem naming no known feature is general.
    static func mapProblems(
        _ problems: [String], ids: [String]
    ) -> (byID: [String: [String]], general: [String]) {
        let known = Set(ids.filter { !$0.isEmpty }).sorted { $0.count > $1.count }
        var byID = [String: [String]]()
        var general = [String]()
        for problem in problems {
            let text = problem.trimmingCharacters(in: .whitespaces)
            if let id = known.first(where: { text.hasPrefix("\($0):") }) {
                let message = text.dropFirst(id.count + 1).trimmingCharacters(in: .whitespaces)
                byID[id, default: []].append(message.isEmpty ? text : message)
            } else {
                general.append(problem)
            }
        }
        return (byID, general)
    }
}

/// The marks as loaded, as they stand now, and the history between.
struct StreetEditState: Equatable, Sendable {
    /// How many changes Undo can step back through.
    static let undoDepth = 200

    private(set) var base: [StreetMark]
    private(set) var marks: [StreetMark]
    /// Earlier marks, most recent last.
    private(set) var past: [[StreetMark]] = []
    /// Marks undone, most recently undone last.
    private(set) var future: [[StreetMark]] = []

    init(base: [StreetMark] = []) {
        self.base = base
        marks = base
    }

    var canUndo: Bool { !past.isEmpty }
    var canRedo: Bool { !future.isEmpty }

    /// The batch Save would send now.
    var pendingEdits: [Gerardus_Align_V1alpha1_StreetUseEdit] {
        StreetUseEditing.diff(base: base, marks: marks)
    }

    var isDirty: Bool { !pendingEdits.isEmpty }

    func mark(_ key: String?) -> StreetMark? {
        guard let key else { return nil }
        return marks.first { $0.key == key }
    }

    /// Replaces the marks, keeping the previous ones for Undo. A change that
    /// changes nothing is not kept.
    mutating func change(_ next: [StreetMark]) {
        guard next != marks else { return }
        past.append(marks)
        if past.count > Self.undoDepth { past.removeFirst(past.count - Self.undoDepth) }
        future = []
        marks = next
    }

    mutating func undo() {
        guard let previous = past.popLast() else { return }
        future.append(marks)
        marks = previous
    }

    mutating func redo() {
        guard let next = future.popLast() else { return }
        past.append(marks)
        marks = next
    }

    mutating func add(_ mark: StreetMark) { change(marks + [mark]) }

    mutating func remove(_ key: String) {
        guard marks.contains(where: { $0.key == key }) else { return }
        change(marks.filter { $0.key != key })
    }

    /// Applies `transform` to the mark with `key`, as one undoable change.
    mutating func update(_ key: String, _ transform: (StreetMark) -> StreetMark) {
        guard marks.contains(where: { $0.key == key }) else { return }
        change(marks.map { $0.key == key ? transform($0) : $0 })
    }

    /// Drops every unsaved change.
    mutating func revert() { change(base) }
}
