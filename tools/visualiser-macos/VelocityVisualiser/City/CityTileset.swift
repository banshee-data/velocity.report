// CityTileset.swift
// Gerardus's city tiles (CITY-VIEW-TILES §3, §8; lidar-city-view-plan
// Item 6): a 3D Tiles 1.1 explicit `tileset.json`, root → L10 → L13 → L16,
// one binary glTF per S2 cell.
//
// The tiles are measured spatial data: the City's footprints with heights
// from its 2010 aerial survey, its street centrelines, and 3DEP ground, each
// at its own acquisition time. They are drawn as context for the traffic.
//
// Frames. The city frame is ENU at lat 37.76, lon −122.44, NAVD88 0, metres,
// Z up. The root's transform maps it to ECEF, which this reader skips: each
// tile's `extras.origin_city_enu_m` gives its origin in the city frame
// directly, and without it the child transforms (translations from the
// parent's origin) are composed below the root.

import Foundation
import simd

/// One tile of the tree.
struct CityTile: Identifiable, Sendable {
    /// The content's path relative to the tileset, or a synthetic id for a
    /// tile without content (the root).
    var id: String
    var contentURI: String?
    var s2: String?
    /// The S2 level (10, 13, 16) from the token's length, or nil.
    var level: Int?
    var geometricError: Double
    /// The tile's origin in the city frame, metres.
    var origin: SIMD3<Double>
    /// The bounding box's centre and half-axes, in the tile's own frame
    /// (Z up, metres from its origin).
    var boxCentre: SIMD3<Double>
    var boxHalfAxes: simd_double3x3
    var children: [CityTile]

    /// The bounding box's centre in the city frame.
    var cityCentre: SIMD3<Double> { origin + boxCentre }

    /// The radius of the box's bounding sphere.
    var radius: Double {
        simd_length(
            simd_abs(boxHalfAxes.columns.0) + simd_abs(boxHalfAxes.columns.1)
                + simd_abs(boxHalfAxes.columns.2))
    }

    var hasContent: Bool { contentURI != nil }

    /// Every tile in the subtree, depth first.
    var all: [CityTile] { [self] + children.flatMap(\.all) }
}

struct CityTileset: Sendable {
    var root: CityTile
    var version: String
    /// The tileset's epoch or version, as its asset block names it.
    var tilesetVersion: String?

    enum ReadError: Error, LocalizedError, Equatable {
        case notATileset(String)

        var errorDescription: String? {
            switch self {
            case .notATileset(let why): return "Not a city tileset: \(why)"
            }
        }
    }

    private struct File: Decodable {
        struct Asset: Decodable {
            var version: String
            var tilesetVersion: String?
        }
        struct Volume: Decodable { var box: [Double]? }
        struct Content: Decodable { var uri: String? }
        struct Extras: Decodable {
            var originCityEnuM: [Double]?
            var s2: String?
        }
        final class Tile: Decodable {
            var boundingVolume: Volume?
            var geometricError: Double?
            var transform: [Double]?
            var content: Content?
            var extras: Extras?
            var children: [Tile]?
        }
        var asset: Asset
        var root: Tile
    }

    static func decode(_ data: Data) throws -> CityTileset {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let file: File
        do { file = try decoder.decode(File.self, from: data) } catch {
            throw ReadError.notATileset(error.localizedDescription)
        }
        guard file.asset.version.hasPrefix("1.") else {
            throw ReadError.notATileset("3D Tiles version \(file.asset.version)")
        }
        var counter = 0
        func build(_ t: File.Tile, parentOrigin: SIMD3<Double>, isRoot: Bool) -> CityTile {
            // The root's transform is city → ECEF: the city frame is its own.
            var origin = parentOrigin
            if !isRoot, let m = t.transform, m.count == 16 { origin += SIMD3(m[12], m[13], m[14]) }
            if let o = t.extras?.originCityEnuM, o.count == 3 { origin = SIMD3(o[0], o[1], o[2]) }
            let box = t.boundingVolume?.box ?? []
            let centre = box.count == 12 ? SIMD3(box[0], box[1], box[2]) : .zero
            let axes =
                box.count == 12
                ? simd_double3x3(
                    SIMD3(box[3], box[4], box[5]), SIMD3(box[6], box[7], box[8]),
                    SIMD3(box[9], box[10], box[11])) : simd_double3x3(0)
            let uri = t.content?.uri
            counter += 1
            let s2 = t.extras?.s2
            return CityTile(
                id: uri ?? "tile-\(counter)", contentURI: uri, s2: s2,
                level: s2.map { CityTileset.level(ofToken: $0) },
                geometricError: t.geometricError ?? 0, origin: origin, boxCentre: centre,
                boxHalfAxes: axes,
                children: (t.children ?? []).map { build($0, parentOrigin: origin, isRoot: false) })
        }
        var root = build(file.root, parentOrigin: .zero, isRoot: true)
        // The root's box is in the city frame itself (its transform goes to
        // ECEF), so its origin is the city origin.
        root.origin = .zero
        return CityTileset(
            root: root, version: file.asset.version, tilesetVersion: file.asset.tilesetVersion)
    }

    /// An S2 cell token's level. A cell id is 3 face bits, 2 bits per level
    /// and a terminating 1 bit; the token is its hex with trailing zeros cut.
    /// The city tiles use 6, 8 and 9 hex digits for L10, L13 and L16.
    static func level(ofToken token: String) -> Int {
        guard let last = token.last, let value = Int(String(last), radix: 16), value != 0 else {
            return 0
        }
        let bits = token.count * 4 - value.trailingZeroBitCount - 1  // before the terminator
        return (bits - 3) / 2
    }

    /// Tiles with content, by URI.
    var contentTiles: [String: CityTile] {
        Dictionary(
            root.all.compactMap { tile in tile.contentURI.map { ($0, tile) } },
            uniquingKeysWith: { a, _ in a })
    }
}
