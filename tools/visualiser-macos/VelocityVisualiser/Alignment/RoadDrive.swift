// RoadDrive.swift
// The driving view's core, as pure values the tests can reach without a
// window: a height index over the road mesh, routes chained from the patches'
// centreline profiles (bridged across junction patches), and a driver that
// moves along a route and STOPS, with its reason, wherever the surface has no
// height.
//
// Rules kept here:
//  - The path comes from the profile stations (their x, y); the height comes
//    from the mesh under that point. A station or mesh sample with no height
//    stops the driver. A previous height is never carried across it.
//  - A junction has no profile of its own: the route bridges between the arms'
//    ends along a chord, and every bridge sample must lie on the junction's
//    mesh. A bridge that leaves the mesh, or a junction not loaded, stops.
//  - Two successors that both continue straight on at different heights are an
//    unresolved deck choice: the driver stops rather than guess.

import Foundation
import simd

// MARK: - Height index

/// Triangles of the road mesh by 4 m cell, for height queries along a route.
/// `height(at:)` answers nil where no triangle lies: a gap, a hole, a missing
/// patch.
struct RoadHeightIndex: Sendable {
    static let cell: Float = 4
    private struct Key: Hashable {
        var x: Int32
        var y: Int32
    }
    private struct Tri: Sendable {
        var a: SIMD3<Float>
        var b: SIMD3<Float>
        var c: SIMD3<Float>
        var patchID: String
    }
    private var cells: [Key: [Int]] = [:]
    private var tris: [Tri] = []
    /// Patches whose mesh is held: an evicted patch is not here.
    private(set) var patchIDs: Set<String> = []

    init(patches: [RoadPatch]) {
        for patch in patches where patch.isResident {
            patchIDs.insert(patch.patchID)
            for t in 0..<patch.triangleCount {
                let a = patch.vertices[Int(patch.indices[3 * t])]
                let b = patch.vertices[Int(patch.indices[3 * t + 1])]
                let c = patch.vertices[Int(patch.indices[3 * t + 2])]
                guard a.z.isFinite, b.z.isFinite, c.z.isFinite else { continue }
                let i = tris.count
                tris.append(Tri(a: a, b: b, c: c, patchID: patch.patchID))
                let lo = simd_min(simd_min(a, b), c)
                let hi = simd_max(simd_max(a, b), c)
                for x in Self.index(lo.x)...Self.index(hi.x) {
                    for y in Self.index(lo.y)...Self.index(hi.y) {
                        cells[Key(x: x, y: y), default: []].append(i)
                    }
                }
            }
        }
    }

    private static func index(_ v: Float) -> Int32 { Int32((v / cell).rounded(.down)) }

    /// Heights under (x, y), highest first: more than one is a stacked deck.
    func heights(at p: SIMD2<Float>, in only: String? = nil) -> [(z: Float, patchID: String)] {
        let key = Key(x: Self.index(p.x), y: Self.index(p.y))
        var out: [(Float, String)] = []
        for i in cells[key] ?? [] {
            let t = tris[i]
            if let only, t.patchID != only { continue }
            if let z = RoadPatch.interpolate(p, t.a, t.b, t.c) { out.append((z, t.patchID)) }
        }
        return out.sorted { $0.0 > $1.0 }.map { (z: $0.0, patchID: $0.1) }
    }

    func height(at p: SIMD2<Float>, in only: String? = nil) -> Float? {
        heights(at: p, in: only).first?.z
    }
}

// MARK: - Route

/// One point of a route.
struct RouteStation: Sendable, Equatable {
    /// Distance from the route's start, m.
    var s: Float
    /// Horizontal position and the mesh height under it; z NaN with no height.
    var position: SIMD3<Float>
    var support: RoadSupportClass
    var patchID: String

    var isMissing: Bool { !position.z.isFinite || support == .missing }
}

/// Why a driver is not moving.
enum RoadStop: Equatable, Sendable {
    case missingHeight(s: Float, detail: String)
    case unloadedGeometry(detail: String)
    case unresolvedChoice(detail: String)
    case endOfRoute

    var message: String {
        switch self {
        case .missingHeight(let s, let detail):
            return String(format: "Stopped at %.0f m: no road height (%@).", s, detail)
        case .unloadedGeometry(let detail): return "Stopped: geometry not loaded (\(detail))."
        case .unresolvedChoice(let detail): return "Stopped: unresolved choice (\(detail))."
        case .endOfRoute: return "End of the loaded route."
        }
    }
}

struct RoadRoute: Sendable {
    var stations: [RouteStation]
    /// Where the route ends early, and why; nil when it runs to a real end.
    var stop: RoadStop?
    /// Patches the route passes through, in order.
    var patchIDs: [String]
    /// What it takes to continue the route where it stopped.
    var chain: RoadRouteBuilder.Chain?
    /// How many of the chain's legs have stations.
    var assembledLegs = 0

    var length: Float { stations.last?.s ?? 0 }
    var isEmpty: Bool { stations.isEmpty }

    /// The station at or before `s`, by binary search.
    func index(at s: Float) -> Int {
        var lo = 0
        var hi = stations.count - 1
        while lo < hi {
            let mid = (lo + hi + 1) / 2
            if stations[mid].s <= s { lo = mid } else { hi = mid - 1 }
        }
        return lo
    }

    /// The first missing station at or after `s` within `ahead` m, if any.
    func firstMissing(from s: Float, within ahead: Float) -> RouteStation? {
        guard !stations.isEmpty else { return nil }
        var i = index(at: s)
        while i < stations.count, stations[i].s <= s + ahead {
            if stations[i].isMissing { return stations[i] }
            i += 1
        }
        return nil
    }
}

enum RoadRouteBuilder {
    /// How far apart two profile ends may be and still be one road, m.
    static let joinTolerance: Float = 2.5
    /// The longest bridge across a junction, m.
    static let maxBridge: Float = 40
    static let bridgeStep: Float = 1

    struct Leg {
        var patch: RoadPatch
        /// Profile stations in driving order.
        var stations: [RoadProfileStation]
        var start: SIMD2<Float> { SIMD2(stations[0].position.x, stations[0].position.y) }
        var end: SIMD2<Float> {
            let p = stations[stations.count - 1].position
            return SIMD2(p.x, p.y)
        }
        var reversed = false
        /// The City node at the end the driver leaves by.
        var endNode: String { reversed ? patch.fromNode : patch.toNode }
    }

    struct LegSpec: Sendable, Equatable {
        var patchID: String
        var reversed: Bool
    }

    /// The legs chosen so far and the bridges between them: what a route needs
    /// to continue where it stopped, without rebuilding from its start.
    struct Chain: Sendable {
        var legs: [LegSpec] = []
        /// After leg i: the bridge samples across a junction, and its patch.
        var bridges: [Int: [SIMD2<Float>]] = [:]
        var bridgePatch: [Int: String] = [:]
        var stop: RoadStop?
    }

    private static func leg(_ p: RoadPatch, reversed: Bool) -> Leg {
        Leg(patch: p, stations: reversed ? p.profile.reversed() : p.profile, reversed: reversed)
    }

    /// Route from the `startPatch`'s profile, forwards or backwards. At each end
    /// it looks for the next leg within `joinTolerance`, or across a junction
    /// patch within `maxBridge`, and takes the straightest; `choices` maps a
    /// junction patch id to the leg patch id to take there.
    static func route(
        from startPatch: String, reversed: Bool = false, patches: [RoadPatch],
        index: RoadHeightIndex, choices: [String: String] = [:]
    ) -> RoadRoute {
        guard
            patches.contains(where: {
                $0.patchID == startPatch && $0.kind == .segment && $0.profile.count >= 2
            })
        else {
            return RoadRoute(
                stations: [], stop: .unloadedGeometry(detail: "no profile for \(startPatch)"),
                patchIDs: [])
        }
        var chain = Chain(legs: [LegSpec(patchID: startPatch, reversed: reversed)])
        advance(&chain, patches: patches, choices: choices)
        return assemble(chain: chain, from: 0, existing: [], patches: patches, index: index)
    }

    /// Continues a route that stopped for lack of geometry, now that more is
    /// loaded. Stations already passed keep their heights, whether or not their
    /// meshes are still resident. A route that stopped for another reason, or
    /// ended, is returned as it is.
    static func extended(
        _ route: RoadRoute, patches: [RoadPatch], index: RoadHeightIndex,
        choices: [String: String] = [:]
    ) -> RoadRoute {
        guard var chain = route.chain, case .unloadedGeometry? = route.stop else { return route }
        chain.stop = nil
        advance(&chain, patches: patches, choices: choices)
        guard chain.legs.count > route.assembledLegs || chain.stop != route.stop else {
            return route
        }
        return assemble(
            chain: chain, from: route.assembledLegs, existing: route.stations, patches: patches,
            index: index)
    }

    private static func advance(
        _ chain: inout Chain, patches: [RoadPatch], choices: [String: String]
    ) {
        let segments = patches.filter { $0.kind == .segment && $0.profile.count >= 2 }
        let junctions = patches.filter { $0.kind == .junction }
        var used = Set(chain.legs.map(\.patchID))
        guard let last = chain.legs.last,
            let lastPatch = patches.first(where: { $0.patchID == last.patchID })
        else {
            chain.stop = .unloadedGeometry(detail: "the route's last road is not loaded")
            return
        }
        var current = leg(lastPatch, reversed: last.reversed)
        while true {
            let end = current.end
            let heading = direction(of: current)
            // Candidates: a segment piece starting (or ending) at `end`.
            var options: [(Leg, [SIMD2<Float>], String?)] = []
            for p in segments where !used.contains(p.patchID) {
                for rev in [false, true] {
                    let candidate = leg(p, reversed: rev)
                    let gap = simd_length(candidate.start - end)
                    if gap <= joinTolerance {
                        options.append((candidate, [], nil))
                    } else if gap <= maxBridge,
                        let j = junctions.first(where: {
                            junctionContains($0, end, candidate.start)
                        })
                    {
                        options.append((candidate, chord(end, candidate.start), j.patchID))
                    }
                }
            }
            if options.isEmpty {
                // The end of the profile with nothing beyond it. Segments that
                // share its node but could not be bridged mean the junction
                // patch is not loaded; a junction patch here with no road beyond
                // it means the road beyond is not loaded; else it is a real end.
                let node = current.endNode
                let sharing = segments.filter {
                    !used.contains($0.patchID) && !node.isEmpty
                        && ($0.fromNode == node || $0.toNode == node)
                }
                if !sharing.isEmpty {
                    chain.stop = .unloadedGeometry(
                        detail: "the junction patch at node \(node) is not loaded")
                } else if junctions.contains(where: { distance(end, to: $0) <= joinTolerance }) {
                    chain.stop = .unloadedGeometry(
                        detail: "the road beyond the junction is not loaded")
                } else {
                    chain.stop = .endOfRoute
                }
                return
            }
            // A chosen leg wins; else the straightest.
            var pick = options[0]
            if let j = options.first?.2, let want = choices[j],
                let chosen = options.first(where: { $0.0.patch.patchID == want })
            {
                pick = chosen
            } else {
                let scored = options.map { o -> (Float, (Leg, [SIMD2<Float>], String?)) in
                    let d = simd_normalize(o.0.start == end ? direction(of: o.0) : o.0.start - end)
                    return (simd_dot(heading, d), o)
                }.sorted { $0.0 > $1.0 }
                pick = scored[0].1
                if scored.count > 1, scored[0].0 > 0.98, scored[1].0 > 0.98 {
                    let z0 = height(of: scored[0].1.0.stations[0])
                    let z1 = height(of: scored[1].1.0.stations[0])
                    if let z0, let z1, abs(z0 - z1) > 2 {
                        chain.stop = .unresolvedChoice(
                            detail: "two roads continue straight on at different heights")
                        return
                    }
                }
            }
            if !pick.1.isEmpty {
                chain.bridges[chain.legs.count - 1] = pick.1
                chain.bridgePatch[chain.legs.count - 1] = pick.2
            }
            used.insert(pick.0.patch.patchID)
            chain.legs.append(LegSpec(patchID: pick.0.patch.patchID, reversed: pick.0.reversed))
            current = pick.0
        }
    }

    // MARK: Pieces

    private static func direction(of leg: Leg) -> SIMD2<Float> {
        let s = leg.stations
        let a = s[max(0, s.count - 4)].position
        let b = s[s.count - 1].position
        let d = SIMD2(b.x - a.x, b.y - a.y)
        return simd_length(d) > 1e-4 ? simd_normalize(d) : SIMD2(1, 0)
    }

    private static func height(of station: RoadProfileStation) -> Float? {
        station.position.z.isFinite ? station.position.z : nil
    }

    private static func chord(_ a: SIMD2<Float>, _ b: SIMD2<Float>) -> [SIMD2<Float>] {
        let n = max(1, Int((simd_length(b - a) / bridgeStep).rounded(.up)))
        return (1..<n).map { a + (b - a) * (Float($0) / Float(n)) }
    }

    /// Whether `a` and `b` both lie on (or within tolerance of) the junction's outer ring.
    private static func junctionContains(
        _ j: RoadPatch, _ a: SIMD2<Float>, _ b: SIMD2<Float>
    ) -> Bool {
        guard let outer = j.rings.first(where: { !$0.hole }) else { return false }
        return RoadSurfaceStyle.distance(a, to: outer) <= joinTolerance
            && RoadSurfaceStyle.distance(b, to: outer) <= joinTolerance
    }

    private static func distance(_ p: SIMD2<Float>, to patch: RoadPatch) -> Float {
        guard let outer = patch.rings.first(where: { !$0.hole }) else { return .infinity }
        return RoadSurfaceStyle.distance(p, to: outer)
    }

    /// Stations for `chain`'s legs from `k` on, appended to `existing`. A
    /// station's height is the mesh's under it when the patch is resident; a
    /// patch whose mesh has been evicted lends its profile's height, which the
    /// server sent; no height anywhere makes it a missing station.
    private static func assemble(
        chain: Chain, from k: Int, existing: [RouteStation], patches: [RoadPatch],
        index: RoadHeightIndex
    ) -> RoadRoute {
        var out = existing
        var s = existing.last?.s ?? 0
        let byID = Dictionary(patches.map { ($0.patchID, $0) }, uniquingKeysWith: { a, _ in a })
        func add(_ xy: SIMD2<Float>, support: RoadSupportClass, patch: String, profileZ: Float?) {
            if let last = out.last {
                let step = simd_length(xy - SIMD2(last.position.x, last.position.y))
                // A shared mouth is one place.
                if step < 1e-3 { return }
                s += step
            }
            var z = index.height(at: xy, in: patch)
            if z == nil, !index.patchIDs.contains(patch), let profileZ, profileZ.isFinite {
                z = profileZ
            }
            out.append(
                RouteStation(
                    s: s, position: SIMD3(xy.x, xy.y, z ?? .nan),
                    support: z == nil ? .missing : support, patchID: patch))
        }
        func bridge(after i: Int) {
            guard let samples = chain.bridges[i], let jp = chain.bridgePatch[i] else { return }
            for xy in samples { add(xy, support: .observed, patch: jp, profileZ: nil) }
        }
        var ids = chain.legs.map(\.patchID)
        for i in k..<chain.legs.count {
            if i == k, k > 0 { bridge(after: k - 1) }
            let spec = chain.legs[i]
            guard let patch = byID[spec.patchID] else { continue }
            for st in leg(patch, reversed: spec.reversed).stations {
                let support = st.support == .unspecified ? .observed : st.support
                add(
                    SIMD2(st.position.x, st.position.y), support: support, patch: patch.patchID,
                    profileZ: st.position.z)
                // A profile station with no height stays without one.
                if st.isMissing {
                    out[out.count - 1].position.z = .nan
                    out[out.count - 1].support = .missing
                }
            }
            bridge(after: i)
        }
        // Junction patches the bridges crossed are part of the route.
        for (i, jp) in chain.bridgePatch.sorted(by: { $0.key < $1.key }) {
            if let at = ids.firstIndex(of: chain.legs[i].patchID), !ids.contains(jp) {
                ids.insert(jp, at: at + 1)
            }
        }
        return RoadRoute(
            stations: out, stop: chain.stop, patchIDs: ids, chain: chain,
            assembledLegs: chain.legs.count)
    }
}

// MARK: - Driver

/// The camera pose for a driver now.
struct DrivePose: Equatable, Sendable {
    var position: SIMD3<Float>
    var target: SIMD3<Float>
}

/// Moves along a route at a speed, at an eye height. Pure: `advance(by:)` is
/// the whole clock.
struct RoadDriver: Sendable {
    static let lookAhead: Float = 12
    /// How far ahead of the eye a missing height stops the driver, m.
    static let stopMargin: Float = 1.5

    var route: RoadRoute
    var s: Float = 0
    var speed: Float = 8
    var eyeHeight: Float = 1.5
    var playing = false
    private(set) var stopped: RoadStop?

    static let speedRange: ClosedRange<Float> = 0...30
    static let eyeRange: ClosedRange<Float> = 0.5...3

    init(route: RoadRoute) {
        self.route = route
        refreshStop()
    }

    var length: Float { route.length }

    /// Replaces the route (more loaded, a different choice), keeping the place
    /// as near as possible and the pause or play it was in.
    mutating func replace(route: RoadRoute) {
        self.route = route
        s = min(s, route.length)
        refreshStop()
    }

    mutating func play() { if stopped == nil { playing = true } }
    mutating func pause() { playing = false }
    mutating func setSpeed(_ v: Float) {
        speed = min(max(v, Self.speedRange.lowerBound), Self.speedRange.upperBound)
    }
    mutating func setEyeHeight(_ v: Float) {
        eyeHeight = min(max(v, Self.eyeRange.lowerBound), Self.eyeRange.upperBound)
    }

    /// Moves to distance `to` (scrubbing). A scrub past a missing height stops
    /// at it, as driving would.
    mutating func scrub(to target: Float) {
        var place = min(max(0, target), route.length)
        // Scrubbing across a gap stops at its start, as driving would.
        if let gap = route.stations.first(where: { $0.isMissing && $0.s <= place }) {
            place = max(0, gap.s - Self.stopMargin)
        }
        s = place
        refreshStop()
    }

    /// Advances by `dt` seconds if playing; stops at the first missing height,
    /// the end of the route, or the route's own stop.
    mutating func advance(by dt: Float) {
        guard playing, stopped == nil else { return }
        let next = s + speed * dt
        // The first missing station between here and there (and a margin).
        if let miss = route.firstMissing(from: s, within: speed * dt + Self.stopMargin) {
            s = max(s, miss.s - Self.stopMargin)
            stopped = .missingHeight(s: miss.s, detail: describe(miss))
            playing = false
            return
        }
        s = min(next, route.length)
        refreshStop()
    }

    private func describe(_ station: RouteStation) -> String {
        station.support == .missing ? "missing ground" : "no mesh under the line"
    }

    private mutating func refreshStop() {
        stopped = nil
        if route.isEmpty {
            stopped = route.stop ?? .unloadedGeometry(detail: "no route")
            playing = false
            return
        }
        if let miss = route.firstMissing(from: s, within: Self.stopMargin) {
            stopped = .missingHeight(s: miss.s, detail: describe(miss))
            playing = false
        } else if s >= route.length - 1e-3 {
            stopped = route.stop ?? .endOfRoute
            playing = false
        }
    }

    /// The camera now, or nil while the route has no height under the eye.
    func pose() -> DrivePose? {
        guard !route.isEmpty, let here = point(at: s) else { return nil }
        let eye = SIMD3(here.x, here.y, here.z + eyeHeight)
        // Look along the road, but never at a point with no height: stop the
        // look-ahead short of the first missing station.
        var reach = Self.lookAhead
        if let gap = route.firstMissing(from: s, within: Self.lookAhead) {
            reach = max(0, gap.s - s - 0.01)
        }
        if reach > 1, let ahead = point(at: s + reach) {
            return DrivePose(position: eye, target: SIMD3(ahead.x, ahead.y, ahead.z + eyeHeight))
        }
        // Too close to the end or a gap to look at it: keep the road's heading.
        let i = route.index(at: s)
        var heading = SIMD3<Float>(0, 1, 0)
        for k in stride(from: min(i + 1, route.stations.count - 1), to: 0, by: -1) {
            let a = route.stations[k - 1]
            let b = route.stations[k]
            if !a.isMissing, !b.isMissing {
                let d = SIMD3(b.position.x - a.position.x, b.position.y - a.position.y, 0)
                if simd_length(d) > 1e-4 {
                    heading = simd_normalize(d)
                    break
                }
            }
        }
        return DrivePose(position: eye, target: eye + heading * 5)
    }

    /// The road surface point at distance `d`, linearly between stations; nil
    /// where either neighbour has no height. The last supported point serves
    /// the look-ahead at the very end of the route.
    func point(at d: Float) -> SIMD3<Float>? {
        guard !route.isEmpty else { return nil }
        let clamped = min(max(0, d), route.length)
        let i = route.index(at: clamped)
        let a = route.stations[i]
        guard !a.isMissing else { return nil }
        guard i + 1 < route.stations.count else { return a.position }
        let b = route.stations[i + 1]
        guard !b.isMissing else { return a.position }
        let span = b.s - a.s
        let t = span > 1e-6 ? (clamped - a.s) / span : 0
        return a.position + (b.position - a.position) * t
    }

    /// Grade (dz/ds) at the eye, nil where there is none.
    func grade() -> Float? {
        guard let a = point(at: s), let b = point(at: s + 2) else { return nil }
        let run = simd_length(SIMD2(b.x - a.x, b.y - a.y))
        return run > 1e-3 ? (b.z - a.z) / run : nil
    }

    /// The support under the eye.
    func supportHere() -> RoadSupportClass {
        guard !route.isEmpty else { return .unspecified }
        return route.stations[route.index(at: s)].support
    }
}
