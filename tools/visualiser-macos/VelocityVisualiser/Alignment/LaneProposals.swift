// LaneProposals.swift
// Gerardus's proposed travel lanes as the street painter shows them
// (docs/plans/lidar-visualiser-lane-painter-plan.md, Item 2), and the limit
// a travel lane carries (Item 3). Nothing here talks to a server.
//
// A proposal is Gerardus's inference from the summed lane field, graded high
// or medium; approving one makes an ordinary travel-lane mark tagged with the
// proposal's id and grade, which the operator then edits as any mark. A
// rejection is Gerardus's append-only review, never a local state.

import Foundation
import simd

/// One proposal as drawn and listed.
struct LaneProposalView: Identifiable, Equatable, Sendable {
    var featureID: String
    var rowID: String
    /// The line in the direction of travel, site frame.
    var points: [SIMD3<Double>]
    var tags: [String: String]
    var profiles: [Gerardus_Align_V1alpha1_LateralProfile]
    /// The authored lane running along it, if any.
    var adoptedBy: String
    /// The latest review is a rejection.
    var rejected: Bool

    var id: String { featureID }
    var grade: String { tags["confidence"] ?? "" }
    var isHigh: Bool { grade == "high" }
    var plan: [PlanPoint] { points.map { PlanPoint($0.x, $0.y) } }
    var isAdopted: Bool { !adoptedBy.isEmpty }
    var widthM: Double? { Double(tags[StreetUseTags.widthM] ?? "").flatMap { $0 > 0 ? $0 : nil } }
    /// Bearing of travel, clockwise from true north: Gerardus's, else the line's.
    var bearingDeg: Double? {
        Double(tags["bearing_deg"] ?? "") ?? StreetGeometry.laneDirection(plan)
    }
    var bound: String { tags["bound"] ?? bearingDeg.map(LaneLimits.bound) ?? "" }
    var street: String { tags["street"] ?? "" }

    init(_ p: Gerardus_Align_V1alpha1_LaneProposal) {
        featureID = p.featureID
        rowID = p.rowID
        let xyz = p.line.xyz
        points = stride(from: 0, to: xyz.count - 2, by: 3).map {
            SIMD3(Double(xyz[$0]), Double(xyz[$0 + 1]), Double(xyz[$0 + 2]))
        }
        tags = p.line.tags
        profiles = p.profiles
        adoptedBy = p.adoptedBy
        rejected = p.hasReview && p.review.decision == .rejected
    }

    /// One line for the list: grade, street and bound, tracks and spread.
    var summary: String {
        var parts = [grade.isEmpty ? "proposal" : grade]
        let place = [LaneLimits.titleCase(street), bound].filter { !$0.isEmpty }.joined(
            separator: " ")
        if !place.isEmpty { parts.append(place) }
        if let t = tags["tracks"] { parts.append("\(t) tracks") }
        if let s = tags["spread_m"] { parts.append("spread \(s) m") }
        if let a = tags["agreement"] { parts.append("agreement \(a)") }
        if let w = tags["windows_agree_m"] { parts.append("halves \(w) m") }
        return parts.joined(separator: " · ")
    }

    /// The tags a mark approved from this proposal starts with.
    var approvedTags: [String: String] {
        var t: [String: String] = [
            StreetUseTags.laneType: tags[StreetUseTags.laneType] ?? "general"
        ]
        if let w = widthM { t[StreetUseTags.widthM] = StreetUseTags.format(w) }
        t[LaneLimits.proposal] = featureID
        if !grade.isEmpty { t[LaneLimits.confidence] = grade }
        return t
    }
}

/// The limit tags of a travel lane (Gerardus LANES-AND-LIMITS §3.1), and the
/// checks the painter shows before Save, which mirror the server's.
enum LaneLimits {
    static let speedLimit = "speed_limit"
    static let unit = "speed_limit_unit"
    static let source = "speed_limit_source"
    static let jurisdiction = "speed_limit_jurisdiction"
    static let note = "speed_limit_note"
    static let effectiveFrom = "speed_limit_effective_from"
    static let effectiveTo = "speed_limit_effective_to"
    static let proposal = "proposal"
    static let confidence = "confidence"
    static let bound = "bound"
    static let directionName = "direction_name"

    static let units = ["mph", "km/h"]
    static let sources = ["sign", "city", "statutory", "operator"]
    static let limitKeys = [
        speedLimit, unit, source, jurisdiction, note, effectiveFrom, effectiveTo,
    ]

    /// The statutory default where no sign governs, by jurisdiction: the
    /// prefill when Gerardus has no City record (its import is not done).
    static func statutoryDefault(_ jurisdiction: String) -> (value: Int, unit: String)? {
        switch jurisdiction.uppercased() {
        case "US-CA": return (25, "mph")
        default: return nil
        }
    }

    /// The tags a new travel lane is prefilled with: the jurisdiction's
    /// statutory default, its source saying so, from the capture's date as a
    /// lower bound. Never silent: the source is `statutory`, not a sign.
    static func prefill(jurisdiction: String = "US-CA", captureDate: String) -> [String: String] {
        guard let d = statutoryDefault(jurisdiction) else {
            return [Self.jurisdiction: jurisdiction]
        }
        return [
            speedLimit: String(d.value), unit: d.unit, source: "statutory",
            Self.jurisdiction: jurisdiction.uppercased(), effectiveFrom: captureDate,
            note:
                "statutory default, not read from a sign; in force from the capture date at least",
        ]
    }

    /// One date as Gerardus writes it, in UTC.
    static func day(_ date: Date) -> String {
        let f = DateFormatter()
        f.calendar = Calendar(identifier: .iso8601)
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(identifier: "UTC")
        f.dateFormat = "yyyy-MM-dd"
        return f.string(from: date)
    }

    static func isDay(_ s: String) -> Bool {
        guard s.count == 10 else { return false }
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(identifier: "UTC")
        f.dateFormat = "yyyy-MM-dd"
        return f.date(from: s).map { day($0) == s } ?? false
    }

    /// The compass octant of a bearing, as Gerardus's `bound` tag.
    static func bound(_ bearing: Double) -> String {
        let names = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"]
        var b = bearing.truncatingRemainder(dividingBy: 360)
        if b < 0 { b += 360 }
        return names[Int(((b + 22.5) / 45).rounded(.down)) % 8]
    }

    static func boundWord(_ bound: String) -> String {
        switch bound {
        case "N": return "northbound"
        case "NE": return "northeastbound"
        case "E": return "eastbound"
        case "SE": return "southeastbound"
        case "S": return "southbound"
        case "SW": return "southwestbound"
        case "W": return "westbound"
        case "NW": return "northwestbound"
        default: return ""
        }
    }

    /// "COLUMBUS AVE" as "Columbus Ave".
    static func titleCase(_ s: String) -> String {
        s.split(separator: " ").map { $0.prefix(1).uppercased() + $0.dropFirst().lowercased() }
            .joined(separator: " ")
    }

    /// The lane's bound: Gerardus's after a save, else from its vertices.
    static func bound(of mark: StreetMark) -> String {
        if let b = mark.tags[bound], !b.isEmpty { return b }
        return StreetGeometry.laneDirection(mark.plan).map(bound) ?? ""
    }

    /// A direction's default name: "Columbus Ave northwestbound".
    static func defaultDirectionName(street: String, bound: String) -> String {
        [titleCase(street), boundWord(bound)].filter { !$0.isEmpty }.joined(separator: " ")
    }

    /// Problems with one lane's limit tags that Gerardus refuses a whole
    /// batch for (INVALID_ARGUMENT), shown before Save. Empty when none.
    static func refusals(_ tags: [String: String]) -> [String] {
        var out: [String] = []
        let value = tags[speedLimit] ?? ""
        let u = tags[unit] ?? ""
        if !value.isEmpty {
            if let n = Int(value) {
                let ceiling = u == "km/h" ? 150 : 90
                if n <= 0 || n > ceiling {
                    out.append("The limit must be 1 to \(ceiling) \(u.isEmpty ? "mph" : u).")
                }
            } else {
                out.append("The limit must be a whole number.")
            }
            if u.isEmpty { out.append("A limit needs its unit.") }
            if (tags[source] ?? "").isEmpty { out.append("A limit needs its source.") }
        } else if !u.isEmpty {
            out.append("A unit without a limit.")
        }
        if !u.isEmpty, !units.contains(u) { out.append("The unit must be mph or km/h.") }
        if let s = tags[source], !s.isEmpty, !sources.contains(s) {
            out.append("The source must be sign, city, statutory or operator.")
        }
        if let j = tags[jurisdiction], !j.isEmpty,
            j.range(of: "^[A-Z]{2}-[A-Z0-9]{1,3}$", options: .regularExpression) == nil
        {
            out.append("The jurisdiction must be ISO 3166-2, such as US-CA.")
        }
        for key in [effectiveFrom, effectiveTo] {
            if let d = tags[key], !d.isEmpty, !isDay(d) { out.append("\(key) must be YYYY-MM-DD.") }
        }
        if let a = tags[effectiveFrom], let b = tags[effectiveTo], isDay(a), isDay(b), b < a {
            out.append("The limit cannot end before it took effect.")
        }
        return out
    }

    /// Whether a new version changes the limit or its unit, which needs a
    /// new effective date (a limit that changes over time is a new version
    /// of the same lane).
    static func changesLimit(from old: [String: String], to new: [String: String]) -> Bool {
        (old[speedLimit] ?? "") != (new[speedLimit] ?? "") || (old[unit] ?? "") != (new[unit] ?? "")
    }

    /// The checks before Save that do not refuse it (shown, not blocking),
    /// by mark key, and those for the whole site.
    static func warnings(
        marks: [StreetMark], base: [StreetMark]
    ) -> (byKey: [String: [String]], general: [String]) {
        var byKey: [String: [String]] = [:]
        let lanes = marks.filter { $0.kind == .travelLane }
        let baseByID = Dictionary(
            base.filter { !$0.featureID.isEmpty }.map { ($0.featureID, $0) },
            uniquingKeysWith: { a, _ in a })
        for lane in lanes {
            var w = refusals(lane.tags)
            if (lane.tags[speedLimit] ?? "").isEmpty { w.append("No limit.") }
            if let old = baseByID[lane.featureID], changesLimit(from: old.tags, to: lane.tags),
                (lane.tags[effectiveFrom] ?? "") == (old.tags[effectiveFrom] ?? "")
            {
                w.append("The limit changed: give the date the new one took effect.")
            }
            if !w.isEmpty { byKey[lane.key] = w }
        }
        // Two lanes in one direction with different limits and no split
        // between them (they share no end vertex).
        var general: [String] = []
        let units = Set(lanes.compactMap { $0.tags[unit] }.filter { !$0.isEmpty })
        if units.count > 1 {
            general.append(
                "Limits here are in more than one unit: \(units.sorted().joined(separator: ", ")).")
        }
        for (i, a) in lanes.enumerated() {
            for b in lanes[(i + 1)...] {
                guard bound(of: a) == bound(of: b), !bound(of: a).isEmpty,
                    (a.tags["street"] ?? "") == (b.tags["street"] ?? ""),
                    let la = a.tags[speedLimit], let lb = b.tags[speedLimit], !la.isEmpty,
                    !lb.isEmpty, la != lb || (a.tags[unit] ?? "") != (b.tags[unit] ?? "")
                else { continue }
                let touch = [a.points.first, a.points.last].contains { p in
                    [b.points.first, b.points.last].contains { q in
                        guard let p, let q else { return false }
                        return simd_distance(SIMD2(p.x, p.y), SIMD2(q.x, q.y)) < 0.05
                    }
                }
                if !touch {
                    byKey[a.key, default: []].append(
                        "Another \(bound(of: a))-bound lane carries \(lb) without a split between them."
                    )
                }
            }
        }
        return (byKey, general)
    }
}

extension StreetUseEditing {
    /// Splits a lane at a point on it: two lanes sharing that vertex, both
    /// keeping the direction, type and width. The first keeps the lane's id;
    /// the second is new, and its limit is cleared but for its unit and
    /// jurisdiction, so the operator gives the limit beyond the sign. Nil
    /// when the point is not within `tolerance` of the lane or lies at an end.
    static func split(
        _ lane: StreetMark, at point: PlanPoint, newKey: String, tolerance: Double = 1.5
    ) -> (StreetMark, StreetMark)? {
        guard lane.kind == .travelLane, lane.points.count >= 2 else { return nil }
        var best: (segment: Int, t: Double, d: Double)?
        for i in 0..<(lane.points.count - 1) {
            let a = SIMD2(lane.points[i].x, lane.points[i].y)
            let b = SIMD2(lane.points[i + 1].x, lane.points[i + 1].y)
            let ab = b - a
            let len2 = simd_dot(ab, ab)
            guard len2 > 1e-12 else { continue }
            let t = max(0, min(1, simd_dot(SIMD2(point.x, point.y) - a, ab) / len2))
            let d = simd_distance(a + t * ab, SIMD2(point.x, point.y))
            if best == nil || d < best!.d { best = (i, t, d) }
        }
        guard let best, best.d <= tolerance else { return nil }
        let a = lane.points[best.segment]
        let b = lane.points[best.segment + 1]
        let cut = a + best.t * (b - a)
        // Not at an end: each half needs a length.
        let first = Array(lane.points[...best.segment]) + [cut]
        let second = [cut] + Array(lane.points[(best.segment + 1)...])
        func length(_ ps: [SIMD3<Double>]) -> Double {
            zip(ps, ps.dropFirst()).reduce(0) {
                $0 + simd_distance(SIMD2($1.0.x, $1.0.y), SIMD2($1.1.x, $1.1.y))
            }
        }
        guard length(first) > 0.5, length(second) > 0.5 else { return nil }
        var up = lane
        up.points = first
        var down = lane
        down.key = newKey
        down.featureID = ""
        down.rowID = ""
        down.points = second
        for key in [
            LaneLimits.speedLimit, LaneLimits.source, LaneLimits.note, LaneLimits.effectiveFrom,
            LaneLimits.effectiveTo, LaneLimits.proposal, LaneLimits.confidence,
        ] { down.tags[key] = nil }
        return (up, down)
    }
}
