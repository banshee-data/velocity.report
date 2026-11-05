// TileSelection.swift
// Which city tiles to draw for a view, by screen-space error, and a cache
// of the ones loaded (lidar-city-view-plan Item 6).
//
// A tile's screen-space error is its geometric error, in metres, as pixels
// at its distance: e · h / (2 · d · tan(fov / 2)). A tile whose error is
// under the threshold is drawn; otherwise its children are, the standard
// 3D Tiles REPLACE refinement. A loading child shows its parent until it
// arrives, so a refinement never opens a hole.

import Foundation
import simd

/// The view a selection is for.
struct TileView: Sendable {
    /// The eye, in the city frame.
    var eye: SIMD3<Double>
    /// The view's height in pixels and its vertical field of view, radians.
    var viewportHeight: Double
    var fovY: Double
    /// Pixels of error allowed before a tile is refined.
    var maximumError: Double = 16
    /// Whether a sphere in the city frame is in view; nil draws everything.
    var inFrustum: (@Sendable (SIMD3<Double>, Double) -> Bool)?

    /// A tile's error in pixels at its nearest point to the eye.
    func screenSpaceError(_ tile: CityTile) -> Double {
        let distance = max(simd_length(tile.cityCentre - eye) - tile.radius, 1)
        return tile.geometricError * viewportHeight / (2 * distance * tan(fovY / 2))
    }
}

struct TileSelection: Equatable, Sendable {
    /// Tiles to draw now, each covering its own ground once.
    var draw: [String] = []
    /// Tiles wanted and not loaded, most wanted first.
    var load: [String] = []
}

enum TileSelector {
    /// Selects the tiles for `view`. `isLoaded` says whether a tile's content
    /// is ready; a tile without content is always ready and draws nothing.
    static func select(
        _ tileset: CityTileset, view: TileView, isLoaded: (String) -> Bool
    ) -> TileSelection {
        var selection = TileSelection()
        var wanted: [(id: String, priority: Double)] = []

        func ready(_ tile: CityTile) -> Bool { !tile.hasContent || isLoaded(tile.id) }

        func want(_ tile: CityTile, error: Double) {
            guard tile.hasContent, !isLoaded(tile.id) else { return }
            wanted.append((tile.id, error))
        }

        /// The tiles that cover `tile`'s ground, or nil when nothing loaded
        /// can.
        func visit(_ tile: CityTile) -> [String]? {
            if let inFrustum = view.inFrustum, !inFrustum(tile.cityCentre, tile.radius) {
                return []
            }
            let error = view.screenSpaceError(tile)
            let refine = !tile.children.isEmpty && (error > view.maximumError || !tile.hasContent)
            if !refine {
                want(tile, error: error)
                return ready(tile) ? (tile.hasContent ? [tile.id] : []) : nil
            }
            var covering = [String]()
            var complete = true
            for child in tile.children {
                if let drawn = visit(child) { covering += drawn } else { complete = false }
            }
            if complete { return covering }
            // A child is still loading: draw this tile in its place, if it can.
            if tile.hasContent {
                want(tile, error: error)
                if isLoaded(tile.id) { return [tile.id] }
            }
            return covering.isEmpty ? nil : covering
        }

        selection.draw = visit(tileset.root) ?? []
        // Coarser tiles first, so a parent arrives before its children, then
        // by error.
        selection.load = wanted.sorted { $0.priority > $1.priority }.map(\.id).reduce(into: []) {
            if !$0.contains($1) { $0.append($1) }
        }
        return selection
    }
}

/// A view frustum as six planes, from a clip matrix with Metal's depth range
/// (reversed: near at w, far at 0). A point p is inside when n·p + d ≥ 0 for
/// every plane.
struct Frustum: Sendable {
    var planes: [SIMD4<Double>]

    init(clip m: simd_double4x4) {
        let r0 = SIMD4(m.columns.0.x, m.columns.1.x, m.columns.2.x, m.columns.3.x)
        let r1 = SIMD4(m.columns.0.y, m.columns.1.y, m.columns.2.y, m.columns.3.y)
        let r2 = SIMD4(m.columns.0.z, m.columns.1.z, m.columns.2.z, m.columns.3.z)
        let r3 = SIMD4(m.columns.0.w, m.columns.1.w, m.columns.2.w, m.columns.3.w)
        let raw = [r3 + r0, r3 - r0, r3 + r1, r3 - r1, r3 - r2, r2]
        planes = raw.map { p in
            let n = simd_length(SIMD3(p.x, p.y, p.z))
            return n > 0 ? p / n : p
        }
    }

    func contains(sphere centre: SIMD3<Double>, radius: Double) -> Bool {
        planes.allSatisfy { simd_dot(SIMD3($0.x, $0.y, $0.z), centre) + $0.w >= -radius }
    }
}

/// A least-recently-used cache bounded by bytes.
struct LRUCache<Value> {
    let capacityBytes: Int
    private(set) var totalBytes = 0
    private var entries: [String: (value: Value, bytes: Int, stamp: UInt64)] = [:]
    private var clock: UInt64 = 0

    init(capacityBytes: Int) { self.capacityBytes = capacityBytes }

    var count: Int { entries.count }
    var keys: [String] { Array(entries.keys) }

    func contains(_ key: String) -> Bool { entries[key] != nil }

    /// The value, marked as just used.
    mutating func get(_ key: String) -> Value? {
        guard var entry = entries[key] else { return nil }
        clock += 1
        entry.stamp = clock
        entries[key] = entry
        return entry.value
    }

    /// Inserts a value, then evicts the least recently used until the cache
    /// fits, never evicting a key in `keep` (the tiles drawn now). Returns
    /// the keys evicted.
    @discardableResult mutating func insert(
        _ value: Value, for key: String, bytes: Int, keep: Set<String> = []
    ) -> [String] {
        if let old = entries[key] { totalBytes -= old.bytes }
        clock += 1
        entries[key] = (value, bytes, clock)
        totalBytes += bytes
        var evicted = [String]()
        let protected = keep.union([key])
        while totalBytes > capacityBytes {
            guard
                let victim = entries.filter({ !protected.contains($0.key) }).min(by: {
                    $0.value.stamp < $1.value.stamp
                })?.key
            else { break }
            totalBytes -= entries[victim]?.bytes ?? 0
            entries[victim] = nil
            evicted.append(victim)
        }
        return evicted
    }

    mutating func removeAll() {
        entries = [:]
        totalBytes = 0
    }
}
