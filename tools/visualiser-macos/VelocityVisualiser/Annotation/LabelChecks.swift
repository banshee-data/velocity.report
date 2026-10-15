// LabelChecks.swift
// Mistakes a labeller would want pointed out before a reviewer finds them,
// as Go's annotation/label_checks.go finds them: the same rules, limits,
// order and messages, held to Go's by internal/lidar/annotation/testdata/
// label_checks_fixture.json. They warn and never refuse: a save that trips
// one is still saved.
//
// The rules read a pack's membership in the pack's own terms: samples in
// order, the points of each, and masks by sample and point index.

import Foundation

/// One thing a check found.
struct LabelWarning: Equatable, Identifiable, Decodable {
    var rule: String
    var objectID: String
    /// The sample to look at.
    var sampleID: Int
    /// The second object, for shared-return and twin.
    var otherID: String?
    var message: String

    var id: String { "\(rule)/\(objectID)/\(sampleID)/\(otherID ?? "")" }

    enum CodingKeys: String, CodingKey {
        case rule
        case objectID = "object_id"
        case sampleID = "sample_id"
        case otherID = "other_id"
        case message
    }
}

/// One sample a check reads: its id, time and plan positions. Points are
/// needed only where a mask has members.
struct LabelCheckSample: Decodable {
    var sampleID: Int
    var timestampNs: Int64
    var x: [Float]
    var y: [Float]

    enum CodingKeys: String, CodingKey {
        case sampleID = "sample_id"
        case timestampNs = "timestamp_ns"
        case x
        case y
    }

    init(sampleID: Int, timestampNs: Int64, x: [Float] = [], y: [Float] = []) {
        self.sampleID = sampleID
        self.timestampNs = timestampNs
        self.x = x
        self.y = y
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        sampleID = try c.decode(Int.self, forKey: .sampleID)
        timestampNs = try c.decode(Int64.self, forKey: .timestampNs)
        x = try c.decodeIfPresent([Float].self, forKey: .x) ?? []
        y = try c.decodeIfPresent([Float].self, forKey: .y) ?? []
    }
}

enum LabelChecks {
    static let singleFrame = "single-frame"
    static let gap = "gap"
    static let implausibleSpeed = "implausible-speed"
    static let sharedReturn = "shared-return"
    static let tinyMask = "tiny-mask"
    static let twin = "twin"

    private static let order = [singleFrame, gap, implausibleSpeed, sharedReturn, tinyMask, twin]

    static let maxGapFrames = 10
    static let minMaskReturns = 3
    static let speedFrames = 3
    static let twinDistanceM = 0.5
    static let twinFrames = 3

    /// What each rule is called in the window.
    static func title(_ rule: String) -> String {
        switch rule {
        case singleFrame: return "One frame"
        case gap: return "Gap"
        case implausibleSpeed: return "Speed"
        case sharedReturn: return "Shared return"
        case tinyMask: return "Tiny mask"
        case twin: return "Twin"
        default: return rule
        }
    }

    /// The fastest an object of a class is taken to move.
    static func speedLimitMPS(_ objectClass: String) -> Double {
        switch objectClass.lowercased() {
        case "pedestrian", "person": return 4
        case "cyclist", "bicycle", "bike", "scooter": return 15
        default: return 45
        }
    }

    /// The mean plan position of a sample's points at indices, summed in
    /// index order, or nil when none is in the sample.
    static func centroid(_ s: LabelCheckSample, _ indices: [Int]) -> (x: Double, y: Double)? {
        var x = 0.0
        var y = 0.0
        var n = 0
        for i in indices.sorted() where i >= 0 && i < s.x.count && i < s.y.count {
            x += Double(s.x[i])
            y += Double(s.y[i])
            n += 1
        }
        guard n > 0 else { return nil }
        return (x / Double(n), y / Double(n))
    }

    private static func format(_ value: Double, _ digits: Int) -> String {
        String(format: "%.\(digits)f", value)
    }

    /// Every rule over a membership: warnings in rule order, then by object,
    /// sample position and other object.
    static func run(
        samples: [LabelCheckSample], objects: [AnnotationObject], masks: [FrameMask]
    ) -> [LabelWarning] {
        var position: [Int: Int] = [:]
        var byID: [Int: LabelCheckSample] = [:]
        for (i, s) in samples.enumerated() {
            position[s.sampleID] = i
            byID[s.sampleID] = s
        }
        var objectClass: [String: String] = [:]
        for o in objects { objectClass[o.objectID] = o.objectClass }

        struct Held {
            var mask: FrameMask
            var pos: Int
            var centroid: (x: Double, y: Double)?
        }
        var byObject: [String: [Held]] = [:]
        for m in masks {
            guard let pos = position[m.sampleID], !m.pointIndices.isEmpty, let s = byID[m.sampleID]
            else { continue }
            byObject[m.objectID, default: []].append(
                Held(mask: m, pos: pos, centroid: centroid(s, m.pointIndices)))
        }
        var out: [LabelWarning] = []
        func warn(
            _ rule: String, _ object: String, _ sample: Int, _ other: String?, _ message: String
        ) {
            out.append(
                LabelWarning(
                    rule: rule, objectID: object, sampleID: sample, otherID: other, message: message
                ))
        }
        for (object, unsorted) in byObject {
            let held = unsorted.sorted { $0.pos < $1.pos }
            if held.count == 1 {
                warn(
                    singleFrame, object, held[0].mask.sampleID, nil,
                    "\(object) has a mask in one frame only")
            }
            let cls = objectClass[object] ?? ""
            let limit = speedLimitMPS(cls)
            for i in held.indices.dropFirst() {
                let a = held[i - 1]
                let b = held[i]
                let missing = b.pos - a.pos - 1
                if missing > maxGapFrames {
                    warn(
                        gap, object, b.mask.sampleID, nil,
                        "\(object) has no mask for \(missing) frames between samples \(a.mask.sampleID) and "
                            + "\(b.mask.sampleID) (more than \(maxGapFrames))")
                }
                guard b.pos - a.pos <= speedFrames, let ca = a.centroid, let cb = b.centroid,
                    let ta = byID[a.mask.sampleID]?.timestampNs,
                    let tb = byID[b.mask.sampleID]?.timestampNs
                else { continue }
                let dt = Double(tb - ta) / 1e9
                guard dt > 0 else { continue }
                let speed = hypot(cb.x - ca.x, cb.y - ca.y) / dt
                if speed > limit {
                    warn(
                        implausibleSpeed, object, b.mask.sampleID, nil,
                        "\(object) moves \(format(speed, 1)) m/s between samples \(a.mask.sampleID) and "
                            + "\(b.mask.sampleID), faster than a \(cls.isEmpty ? "object" : cls)'s "
                            + "\(format(limit, 0)) m/s")
                }
            }
        }

        var bySample: [Int: [FrameMask]] = [:]
        for m in masks where position[m.sampleID] != nil {
            bySample[m.sampleID, default: []].append(m)
        }
        struct Pair: Hashable {
            var a: String
            var b: String
        }
        var near: [Pair: [Int]] = [:]
        for (sample, unsorted) in bySample {
            let sampleMasks = unsorted.sorted { $0.objectID < $1.objectID }
            var owner: [Int: String] = [:]
            var shared: [Pair: Int] = [:]
            for m in sampleMasks {
                let n = m.pointIndices.count
                if n > 0 && n < minMaskReturns {
                    warn(
                        tinyMask, m.objectID, sample, nil,
                        "\(m.objectID)'s mask in sample \(sample) has \(n) return(s) (fewer than \(minMaskReturns))"
                    )
                }
                for idx in m.pointIndices {
                    if let first = owner[idx], first != m.objectID {
                        shared[Pair(a: first, b: m.objectID), default: 0] += 1
                    } else {
                        owner[idx] = m.objectID
                    }
                }
            }
            for (pair, n) in shared {
                warn(
                    sharedReturn, pair.a, sample, pair.b,
                    "\(pair.a) and \(pair.b) share \(n) return(s) in sample \(sample)")
            }
            guard let s = byID[sample] else { continue }
            for i in sampleMasks.indices {
                for j in sampleMasks.indices where j > i {
                    if let ca = centroid(s, sampleMasks[i].pointIndices),
                        let cb = centroid(s, sampleMasks[j].pointIndices),
                        hypot(ca.x - cb.x, ca.y - cb.y) < twinDistanceM
                    {
                        near[
                            Pair(a: sampleMasks[i].objectID, b: sampleMasks[j].objectID),
                            default: []
                        ].append(sample)
                    }
                }
            }
        }
        for (pair, frames) in near where frames.count >= twinFrames {
            let first = frames.min { (position[$0] ?? 0) < (position[$1] ?? 0) } ?? frames[0]
            warn(
                twin, pair.a, first, pair.b,
                "\(pair.a) and \(pair.b) are within \(format(twinDistanceM, 1)) m of each other in "
                    + "\(frames.count) frames: one object labelled twice?")
        }
        let rank = Dictionary(uniqueKeysWithValues: order.enumerated().map { ($1, $0) })
        return out.sorted { a, b in
            if rank[a.rule] != rank[b.rule] { return (rank[a.rule] ?? 99) < (rank[b.rule] ?? 99) }
            if a.objectID != b.objectID { return a.objectID < b.objectID }
            let pa = position[a.sampleID] ?? 0
            let pb = position[b.sampleID] ?? 0
            if pa != pb { return pa < pb }
            return (a.otherID ?? "") < (b.otherID ?? "")
        }
    }
}
