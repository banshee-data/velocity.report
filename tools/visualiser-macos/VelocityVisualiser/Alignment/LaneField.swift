// LaneField.swift
// The lane field the street painter draws under the marks
// (docs/plans/lidar-visualiser-lane-painter-plan.md, Item 1): what distinct
// tracks did in each 0.25 m cell, per class group, from one of three
// sources. Nothing here talks to a server or draws.
//
// - This recording: the open run's field from velocity.report's
//   GET /api/lidar/runs/{run_id}/lane-field, in the sensor frame, placed by
//   the pose the reference is drawn with, so a pose nudge moves the
//   reference and leaves the traffic where the sensor saw it.
// - The site: Gerardus's GetLaneField, every capture uploaded so far summed
//   in the site frame, heading bins clockwise from true north.
// - Live: accumulated from the stream as it plays, labelled "live, this
//   replay only", never saved. It keeps the same floor of five distinct
//   tracks a cell, so no view shows a thinner field than Gerardus receives.
//
// The field carries counts only, never a track id (Gerardus ADR-0011); the
// live accumulator holds ids only to count them and drops them with itself.

import Foundation
import SwiftProtobuf
import simd

/// The class groups a field carries.
enum LaneFieldClassGroup: String, CaseIterable, Identifiable, Sendable {
    case vehicle
    case cyclist
    case pedestrian

    var id: String { rawValue }

    var label: String {
        switch self {
        case .vehicle: return "Vehicles"
        case .cyclist: return "Cycles"
        case .pedestrian: return "People"
        }
    }

    /// The tracker's labels in each group (l8analytics.LaneFieldClassGroup).
    init?(classLabel: String) {
        switch classLabel {
        case "car", "truck", "bus", "motorcyclist": self = .vehicle
        case "cyclist": self = .cyclist
        case "pedestrian": self = .pedestrian
        default: return nil
        }
    }
}

/// One class group's cells, sparse, as Gerardus's LaneFieldCells: eight
/// heading counts a cell.
struct LaneFieldCellList: Equatable, Sendable {
    var ix: [UInt32] = []
    var iy: [UInt32] = []
    var tracks: [UInt32] = []
    var boxTracks: [UInt32] = []
    var heading: [UInt32] = []
    var speedSumMps: [Float] = []

    var count: Int { ix.count }

    /// Whether the arrays agree: one entry a cell, eight heading counts.
    var isWellFormed: Bool {
        let n = ix.count
        return iy.count == n && tracks.count == n && boxTracks.count == n && speedSumMps.count == n
            && heading.count == 8 * n
    }

    /// The bin holding most of the cell's heading counts, or nil when the
    /// cell names no direction (nothing moving at 2 m/s or more).
    func dominantBin(_ c: Int) -> Int? {
        var best = -1
        var bestCount: UInt32 = 0
        for b in 0..<8 where heading[8 * c + b] > bestCount {
            best = b
            bestCount = heading[8 * c + b]
        }
        return best < 0 ? nil : best
    }
}

/// A field as the painter draws it.
struct LaneFieldGrid: Equatable, Sendable {
    enum Frame: Equatable, Sendable {
        /// Cells and heading bins in the sensor frame: placed by the pose.
        case sensor
        /// Cells in the site frame, heading bins clockwise from true north.
        case site
    }

    enum Source: String, Equatable, Sendable {
        case run = "This recording"
        case site = "The site"
        case live = "Live, this replay only"
    }

    var source: Source
    var frame: Frame
    /// The south-west cell's centre.
    var originX: Double = 0
    var originY: Double = 0
    var cellM: Double = 0.25
    var groups: [LaneFieldClassGroup: LaneFieldCellList] = [:]
    /// Distinct tracks behind the field, all groups.
    var trackCount: Int = 0
    /// Captures summed, for the site's field.
    var captures: Int = 0
    var fieldIDs: [String] = []

    var cellCount: Int { groups.values.reduce(0) { $0 + $1.count } }

    /// A cell's centre in the field's own frame.
    func centre(_ ix: UInt32, _ iy: UInt32) -> SIMD2<Double> {
        SIMD2(originX + Double(ix) * cellM, originY + Double(iy) * cellM)
    }
}

// MARK: - velocity.report's JSON

/// The JSON velocity.report's lane-field route serves and `velocity lidar
/// lane-field` writes: the shape `gerardus upload-lane-field` reads.
struct VelocityReportLaneField: Codable, Equatable, Sendable {
    struct Header: Codable, Equatable, Sendable {
        var l16Token: String
        var sensorID: String
        var recordID: String
        var captureRef: String
        var windowStart: String
        var windowEnd: String
        var windowPart: String
        var cellM: Double
        var origin: [Double]
        var nx: UInt32
        var ny: UInt32
        var k: UInt32
        var pipelineBuild: String
        var tuningFingerprint: String
        var trackCount: UInt32

        enum CodingKeys: String, CodingKey {
            case l16Token = "l16_token"
            case sensorID = "sensor_id"
            case recordID = "record_id"
            case captureRef = "capture_ref"
            case windowStart = "window_start"
            case windowEnd = "window_end"
            case windowPart = "window_part"
            case cellM = "cell_m"
            case origin, nx, ny, k
            case pipelineBuild = "pipeline_build"
            case tuningFingerprint = "tuning_fingerprint"
            case trackCount = "track_count"
        }
    }

    struct Group: Codable, Equatable, Sendable {
        var classGroup: String
        var ix: [UInt32]
        var iy: [UInt32]
        var tracks: [UInt32]
        var boxTracks: [UInt32]
        var heading: [UInt32]
        var speedSumMps: [Float]

        enum CodingKeys: String, CodingKey {
            case classGroup = "class_group"
            case ix, iy, tracks
            case boxTracks = "box_tracks"
            case heading
            case speedSumMps = "speed_sum_mps"
        }

        var cells: LaneFieldCellList {
            LaneFieldCellList(
                ix: ix, iy: iy, tracks: tracks, boxTracks: boxTracks, heading: heading,
                speedSumMps: speedSumMps)
        }
    }

    var header: Header
    var groups: [Group]

    /// The run's field as the painter draws it, in the sensor frame. Groups
    /// with arrays that disagree, or a class the painter does not know, are
    /// left out.
    var grid: LaneFieldGrid {
        var g = LaneFieldGrid(source: .run, frame: .sensor)
        g.originX = header.origin.first ?? 0
        g.originY = header.origin.count > 1 ? header.origin[1] : 0
        g.cellM = header.cellM > 0 ? header.cellM : 0.25
        g.trackCount = Int(header.trackCount)
        for group in groups {
            guard let cls = LaneFieldClassGroup(rawValue: group.classGroup) else { continue }
            let cells = group.cells
            if cells.isWellFormed { g.groups[cls] = cells }
        }
        return g
    }

    /// The upload's header, against the accepted record that places it.
    func uploadHeader(l16Token: String, recordID: String) -> Gerardus_Align_V1alpha1_LaneFieldHeader
    {
        var h = Gerardus_Align_V1alpha1_LaneFieldHeader()
        h.l16Token = l16Token
        h.sensorID = header.sensorID
        h.recordID = recordID
        h.captureRef = header.captureRef
        if let t = Self.time(header.windowStart) { h.windowStart = t }
        if let t = Self.time(header.windowEnd) { h.windowEnd = t }
        h.windowPart = header.windowPart
        h.cellM = Float(header.cellM)
        var origin = Gerardus_Align_V1alpha1_Vector3()
        origin.x = header.origin.first ?? 0
        origin.y = header.origin.count > 1 ? header.origin[1] : 0
        h.origin = origin
        h.nx = header.nx
        h.ny = header.ny
        h.k = header.k
        h.pipelineBuild = header.pipelineBuild
        h.tuningFingerprint = header.tuningFingerprint
        h.trackCount = header.trackCount
        return h
    }

    /// The cells as upload chunks: no message over Gerardus's 4 MiB, a group
    /// spanning chunks where it must. A cell is at most about 70 bytes on
    /// the wire (four varints, eight more, a float), so 50,000 a chunk stays
    /// under 4 MiB.
    func uploadChunks(cellsPerChunk: Int = 50_000) -> [Gerardus_Align_V1alpha1_LaneFieldCells] {
        let step = max(1, cellsPerChunk)
        var out: [Gerardus_Align_V1alpha1_LaneFieldCells] = []
        for group in groups {
            let n = group.ix.count
            var start = 0
            while start < n {
                let end = min(n, start + step)
                var c = Gerardus_Align_V1alpha1_LaneFieldCells()
                c.classGroup = group.classGroup
                c.ix = Array(group.ix[start..<end])
                c.iy = Array(group.iy[start..<end])
                c.tracks = Array(group.tracks[start..<end])
                c.boxTracks = Array(group.boxTracks[start..<end])
                c.heading = Array(group.heading[(8 * start)..<(8 * end)])
                c.speedSumMps = Array(group.speedSumMps[start..<end])
                out.append(c)
                start = end
            }
        }
        return out
    }

    var cellCount: Int { groups.reduce(0) { $0 + $1.ix.count } }

    static func time(_ s: String) -> SwiftProtobuf.Google_Protobuf_Timestamp? {
        guard !s.isEmpty else { return nil }
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = f.date(from: s) { return Google_Protobuf_Timestamp(date: d) }
        f.formatOptions = [.withInternetDateTime]
        return f.date(from: s).map { Google_Protobuf_Timestamp(date: $0) }
    }
}

extension LaneFieldGrid {
    /// Gerardus's summed field for one class group, in the site frame.
    static func site(
        _ responses: [LaneFieldClassGroup: Gerardus_Align_V1alpha1_GetLaneFieldResponse]
    ) -> LaneFieldGrid {
        var g = LaneFieldGrid(source: .site, frame: .site)
        var origin: SIMD2<Double>?
        var ids = Set<String>()
        for (cls, r) in responses {
            let o = SIMD2(r.origin.x, r.origin.y)
            let cell = r.cellM > 0 ? Double(r.cellM) : 0.25
            // Groups may come on different grids: shift each onto the first.
            let base = origin ?? o
            origin = base
            g.cellM = cell
            var cells = LaneFieldCellList(
                ix: r.cells.ix, iy: r.cells.iy, tracks: r.cells.tracks,
                boxTracks: r.cells.boxTracks, heading: r.cells.heading,
                speedSumMps: r.cells.speedSumMps)
            guard cells.isWellFormed else { continue }
            let dx = Int(((o.x - base.x) / cell).rounded())
            let dy = Int(((o.y - base.y) / cell).rounded())
            if dx != 0 || dy != 0 {
                cells.ix = cells.ix.map { UInt32(max(0, Int($0) + dx)) }
                cells.iy = cells.iy.map { UInt32(max(0, Int($0) + dy)) }
            }
            g.groups[cls] = cells
            g.trackCount = max(g.trackCount, Int(r.trackCount))
            g.captures = max(g.captures, Int(r.captures))
            ids.formUnion(r.fieldIds)
        }
        g.originX = origin?.x ?? 0
        g.originY = origin?.y ?? 0
        g.fieldIDs = ids.sorted()
        return g
    }
}

// MARK: - Live accumulation

/// Distinct tracks per cell from the stream, while it plays: the labelled
/// fallback when the server has no field for the open capture. Observed
/// instants only (no miss), the same class groups, speed floor and k as
/// velocity.report's field; ids are held only to count them.
struct LaneFieldLiveAccumulator: Sendable {
    static let k = 5
    static let headingMinSpeedMps: Float = 2

    private struct Key: Hashable {
        var group: LaneFieldClassGroup
        var ix: Int
        var iy: Int
    }
    private struct Cell {
        var tracks = Set<String>()
        var boxTracks = Set<String>()
        var heading = [Set<String>](repeating: [], count: 8)
        var speed: [String: (sum: Double, n: Int)] = [:]
    }
    private var cells: [Key: Cell] = [:]
    private var seen = Set<String>()
    let cellM: Double

    init(cellM: Double = 0.25) { self.cellM = cellM }

    var isEmpty: Bool { cells.isEmpty }

    /// Adds one frame's tracks, in the sensor frame.
    mutating func add(_ tracks: [Track]) {
        for t in tracks where t.misses == 0 {
            guard let group = LaneFieldClassGroup(classLabel: t.classLabel) else { continue }
            let id = t.trackID
            seen.insert(id)
            let key = Key(group: group, ix: cellIndex(t.x), iy: cellIndex(t.y))
            var cell = cells[key] ?? Cell()
            cell.tracks.insert(id)
            var s = cell.speed[id] ?? (0, 0)
            s.sum += Double(t.speedMps)
            s.n += 1
            cell.speed[id] = s
            if t.speedMps >= Self.headingMinSpeedMps, t.vx != 0 || t.vy != 0 {
                cell.heading[Self.bin(vx: Double(t.vx), vy: Double(t.vy))].insert(id)
            }
            cells[key] = cell
            if t.bboxLength > 0, t.bboxWidth > 0 {
                for (ix, iy) in boxCells(t) {
                    let bk = Key(group: group, ix: ix, iy: iy)
                    var c = cells[bk] ?? Cell()
                    c.boxTracks.insert(id)
                    cells[bk] = c
                }
            }
        }
    }

    /// The bin of a velocity, clockwise from +Y: bin b covers [45b, 45b + 45).
    static func bin(vx: Double, vy: Double) -> Int {
        var deg = atan2(vx, vy) * 180 / .pi
        if deg < 0 { deg += 360 }
        return min(7, Int(deg / 45))
    }

    private func cellIndex(_ v: Float) -> Int { Int((Double(v) / cellM).rounded(.down)) }

    private func boxCells(_ t: Track) -> [(Int, Int)] {
        let cx = Double(t.boxCentreX)
        let cy = Double(t.boxCentreY)
        let h = Double(t.bboxHeadingRad)
        let ux = cos(h)
        let uy = sin(h)
        let hl = Double(t.bboxLength) / 2
        let hw = Double(t.bboxWidth) / 2
        let r = hl + hw
        var out: [(Int, Int)] = []
        let x0 = Int(((cx - r) / cellM).rounded(.down))
        let x1 = Int(((cx + r) / cellM).rounded(.down))
        let y0 = Int(((cy - r) / cellM).rounded(.down))
        let y1 = Int(((cy + r) / cellM).rounded(.down))
        guard x1 - x0 < 400, y1 - y0 < 400 else { return [] }
        for ix in x0...x1 {
            for iy in y0...y1 {
                let dx = (Double(ix) + 0.5) * cellM - cx
                let dy = (Double(iy) + 0.5) * cellM - cy
                if abs(dx * ux + dy * uy) <= hl + 1e-9, abs(-dx * uy + dy * ux) <= hw + 1e-9 {
                    out.append((ix, iy))
                }
            }
        }
        return out
    }

    /// The field so far, cells under k left out, in the sensor frame.
    var grid: LaneFieldGrid {
        var g = LaneFieldGrid(source: .live, frame: .sensor)
        g.cellM = cellM
        let kept = cells.filter {
            $0.value.tracks.count >= Self.k || $0.value.boxTracks.count >= Self.k
        }
        guard let minX = kept.keys.map(\.ix).min(), let minY = kept.keys.map(\.iy).min() else {
            return g
        }
        g.originX = (Double(minX) + 0.5) * cellM
        g.originY = (Double(minY) + 0.5) * cellM
        var contributing = Set<String>()
        let ordered = kept.sorted {
            ($0.key.group.rawValue, $0.key.ix, $0.key.iy) < (
                $1.key.group.rawValue, $1.key.ix, $1.key.iy
            )
        }
        for (key, cell) in ordered {
            var list = g.groups[key.group] ?? LaneFieldCellList()
            list.ix.append(UInt32(key.ix - minX))
            list.iy.append(UInt32(key.iy - minY))
            list.tracks.append(UInt32(cell.tracks.count))
            list.boxTracks.append(UInt32(cell.boxTracks.count))
            list.heading.append(contentsOf: cell.heading.map { UInt32($0.count) })
            list.speedSumMps.append(
                Float(cell.speed.values.reduce(0) { $0 + $1.sum / Double(max(1, $1.n)) }))
            g.groups[key.group] = list
            contributing.formUnion(cell.tracks)
            contributing.formUnion(cell.boxTracks)
        }
        g.trackCount = contributing.count
        return g
    }
}
