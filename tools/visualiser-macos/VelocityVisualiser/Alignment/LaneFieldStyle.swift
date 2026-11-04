// LaneFieldStyle.swift
// The traffic layer's triangles: a lane field drawn as one quad per cell in
// the site frame, under the marks, in the street overlay's vertex layout.
// The plan named a texture per class group; quads in the overlay the marks
// already use keep the Metal work where it is and draw the same picture.
//
// A cell's opacity follows the log of its count, its hue the direction of
// its dominant heading bin (vehicles; cycles and people have their own
// hues), grey where nothing moved fast enough to name a direction.

import Foundation
import simd

/// What the traffic layer shows of a field.
struct LaneFieldDisplay: Equatable, Sendable {
    enum Show: String, CaseIterable, Identifiable, Sendable {
        /// Distinct tracks whose centre fell in the cell.
        case centres = "Centres"
        /// Distinct tracks whose box covered the cell's centre.
        case extents = "Extents"
        var id: String { rawValue }
    }

    var show: Show = .extents
    var classes: Set<LaneFieldClassGroup> = [.vehicle]
    /// Cells under this many distinct tracks are left undrawn; the field
    /// itself holds none under five.
    var floor: Int = 5
    /// The quad's opacity at the densest cell.
    var opacity: Double = 0.55
}

enum LaneFieldStyle {
    /// Drawn this far above the ground, under the marks (lifted 0.15 m) and
    /// over the reference's draped lines (0.10 m).
    static let lift: Double = 0.05
    /// Eight direction hues, one per 45° sector of site-frame bearing from
    /// true north (0–45°, 45–90°, …), the sectors Gerardus's bins cover,
    /// chosen to stay apart in light and dark mode.
    static let directionHues: [SIMD3<Float>] = [
        SIMD3(0.26, 0.52, 0.96),  // NNE blue
        SIMD3(0.00, 0.70, 0.75),  // ENE teal
        SIMD3(0.20, 0.72, 0.30),  // ESE green
        SIMD3(0.62, 0.80, 0.12),  // SSE lime
        SIMD3(0.96, 0.62, 0.04),  // SSW amber
        SIMD3(0.93, 0.33, 0.20),  // WSW vermilion
        SIMD3(0.86, 0.26, 0.62),  // WNW magenta
        SIMD3(0.55, 0.36, 0.92),  // NNW violet
    ]
    static let noDirection = SIMD3<Float>(0.62, 0.64, 0.68)
    static let cyclist = SIMD3<Float>(0.98, 0.80, 0.18)
    static let pedestrian = SIMD3<Float>(0.80, 0.55, 0.98)
    /// Each sector by the compass point at its centre.
    static let compass = ["NNE", "ENE", "ESE", "SSE", "SSW", "WSW", "WNW", "NNW"]

    /// The bearing, clockwise from true north, of the sensor's +Y in the site
    /// frame: what turns a sensor-frame heading bin into a compass one.
    static func sensorYBearing(_ pose: SitePose) -> Double {
        let y = pose.rotation.act(SIMD3<Double>(0, 1, 0))
        var deg = atan2(y.x, y.y) * 180 / .pi
        if deg < 0 { deg += 360 }
        return deg
    }

    /// The site-frame sector a heading bin's centre falls in, given the
    /// frame's turn, as Gerardus places bins: a site-frame bin is itself.
    static func octant(bin: Int, turnDeg: Double) -> Int {
        var deg = (45 * Double(bin) + 22.5 + turnDeg).truncatingRemainder(dividingBy: 360)
        if deg < 0 { deg += 360 }
        return Int(deg / 45) % 8
    }

    static func hue(_ group: LaneFieldClassGroup, octant: Int?) -> SIMD3<Float> {
        switch group {
        case .cyclist: return cyclist
        case .pedestrian: return pedestrian
        case .vehicle: return octant.map { directionHues[$0] } ?? noDirection
        }
    }

    /// Fill triangles, [x, y, z, r, g, b, a] per vertex, in the site frame.
    /// A sensor-frame field is placed by `pose`; a site-frame one is not.
    /// `groundZ` gives the height to draw a site-frame point at.
    static func fills(
        _ grid: LaneFieldGrid, display: LaneFieldDisplay, pose: SitePose?,
        groundZ: (SIMD2<Double>) -> Double
    ) -> [Float] {
        if grid.frame == .sensor && pose == nil { return [] }
        let turn = grid.frame == .sensor ? sensorYBearing(pose!) : 0
        let half = grid.cellM / 2
        var out: [Float] = []
        for group in LaneFieldClassGroup.allCases where display.classes.contains(group) {
            guard let cells = grid.groups[group], cells.isWellFormed else { continue }
            let counts = display.show == .centres ? cells.tracks : cells.boxTracks
            let floor = UInt32(max(1, display.floor))
            guard let top = counts.max(), top >= floor else { continue }
            let logTop = log(Double(top) + 1)
            out.reserveCapacity(out.count + cells.count * 42)
            for c in 0..<cells.count {
                let n = counts[c]
                guard n >= floor else { continue }
                let rgb = hue(
                    group, octant: cells.dominantBin(c).map { octant(bin: $0, turnDeg: turn) })
                let a = Float(display.opacity * (0.25 + 0.75 * log(Double(n) + 1) / logTop))
                let p = grid.centre(cells.ix[c], cells.iy[c])
                let corners = [
                    SIMD2(p.x - half, p.y - half), SIMD2(p.x + half, p.y - half),
                    SIMD2(p.x + half, p.y + half), SIMD2(p.x - half, p.y + half),
                ].map { q -> SIMD3<Double> in
                    let s = place(q, grid.frame, pose)
                    return SIMD3(s.x, s.y, groundZ(s) + lift)
                }
                for i in [0, 1, 2, 0, 2, 3] {
                    let v = corners[i]
                    out.append(contentsOf: [
                        Float(v.x), Float(v.y), Float(v.z), rgb.x, rgb.y, rgb.z, a,
                    ])
                }
            }
        }
        return out
    }

    /// A field-frame point in the site frame's plane.
    static func place(
        _ q: SIMD2<Double>, _ frame: LaneFieldGrid.Frame, _ pose: SitePose?
    ) -> SIMD2<Double> {
        guard frame == .sensor, let pose else { return q }
        let s = pose.rotation.act(SIMD3(q.x, q.y, 0)) + pose.translation
        return SIMD2(s.x, s.y)
    }

    /// The legend: each compass octant the vehicle field holds, with its
    /// share of cells, largest first.
    static func legend(_ grid: LaneFieldGrid, pose: SitePose?) -> [(bound: String, cells: Int)] {
        guard let cells = grid.groups[.vehicle] else { return [] }
        if grid.frame == .sensor && pose == nil { return [] }
        let turn = grid.frame == .sensor ? sensorYBearing(pose!) : 0
        var counts = [Int](repeating: 0, count: 8)
        for c in 0..<cells.count {
            if let b = cells.dominantBin(c) { counts[octant(bin: b, turnDeg: turn)] += 1 }
        }
        return counts.enumerated().filter { $0.element > 0 }.sorted { $0.element > $1.element }.map
        { (compass[$0.offset], $0.element) }
    }
}
