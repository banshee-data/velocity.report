// GLBReader.swift
// A minimal binary glTF reader for Gerardus's city tiles: one JSON chunk,
// one binary chunk, and at most three meshes (ground, buildings, streets)
// of one primitive each, with POSITION and indices only. There are no
// normals and no materials: buildings are shaded flat from each triangle's
// own normal, and colour is the client's (CITY-VIEW-TILES §3.4, §8).
//
// glTF is Y-up: glTF (x, y, z) = city (east, up, −north). Vertices are
// turned to the city's Z-up here, as Float32 metres from the tile's origin.
// Triangles wind counter-clockwise seen from outside, which the turn keeps.

import Foundation
import simd

/// A street-name label: the City's own name, for finding the way around.
/// Not an attribute of any feature.
struct StreetLabel: Equatable, Sendable {
    var name: String
    /// On the centreline, metres from the tile's origin, city ENU.
    var anchor: SIMD3<Float>
    /// The street's bearing at the anchor, degrees in [0, 180).
    var bearingDeg: Float
    /// DataSF classcode: 1–3 are arterials.
    var streetClass: String
}

/// One layer of a tile: positions (x, y, z per vertex, city ENU from the
/// tile's origin) and indices into them.
struct TileMesh: Equatable, Sendable {
    var positions: [Float] = []
    var indices: [UInt32] = []

    var vertexCount: Int { positions.count / 3 }
    var isEmpty: Bool { indices.isEmpty }
    var byteCount: Int { positions.count * 4 + indices.count * 4 }
}

/// What a tile holds.
struct CityTileContent: Sendable {
    /// Triangles.
    var ground = TileMesh()
    /// Triangles; shade flat.
    var buildings = TileMesh()
    /// Line segments, two indices each.
    var streets = TileMesh()
    var labels: [StreetLabel] = []
    var s2: String?
    var level: Int?
    var origin: SIMD3<Double>?

    var byteCount: Int { ground.byteCount + buildings.byteCount + streets.byteCount }
}

enum GLBReader {
    enum ReadError: Error, LocalizedError, Equatable {
        case notGLB
        case badChunk(String)
        case badAccessor(Int, String)

        var errorDescription: String? {
            switch self {
            case .notGLB: return "Not a binary glTF file"
            case .badChunk(let why): return "Bad glTF chunk: \(why)"
            case .badAccessor(let index, let why): return "Bad glTF accessor \(index): \(why)"
            }
        }
    }

    private struct Document: Decodable {
        struct Accessor: Decodable {
            var bufferView: Int?
            var byteOffset: Int?
            var componentType: Int
            var count: Int
            var type: String
        }
        struct BufferView: Decodable {
            var buffer: Int?
            var byteOffset: Int?
            var byteLength: Int
            var byteStride: Int?
        }
        struct Primitive: Decodable {
            var attributes: [String: Int]
            var indices: Int?
            var mode: Int?
        }
        struct Mesh: Decodable {
            var name: String?
            var primitives: [Primitive]
        }
        struct Label: Decodable {
            var name: String
            var anchorEnuM: [Double]
            var bearingDeg: Double?
            var `class`: String?
        }
        struct Extras: Decodable {
            var labels: [Label]?
            var level: Int?
            var s2: String?
            var originCityEnuM: [Double]?
        }
        var accessors: [Accessor]?
        var bufferViews: [BufferView]?
        var meshes: [Mesh]?
        var extras: Extras?
    }

    static func read(_ data: Data) throws -> CityTileContent {
        let bytes = [UInt8](data)
        func u32(_ at: Int) -> UInt32 {
            UInt32(bytes[at]) | UInt32(bytes[at + 1]) << 8 | UInt32(bytes[at + 2]) << 16 | UInt32(
                bytes[at + 3]) << 24
        }
        guard bytes.count >= 20, u32(0) == 0x4654_6C67, u32(4) == 2 else { throw ReadError.notGLB }
        let total = min(Int(u32(8)), bytes.count)
        var offset = 12
        var json: ArraySlice<UInt8>?
        var bin: ArraySlice<UInt8>?
        while offset + 8 <= total {
            let length = Int(u32(offset))
            let type = u32(offset + 4)
            let start = offset + 8
            guard start + length <= total else { throw ReadError.badChunk("runs past the file") }
            if type == 0x4E4F_534A, json == nil { json = bytes[start..<start + length] }
            if type == 0x004E_4942, bin == nil { bin = bytes[start..<start + length] }
            offset = start + ((length + 3) & ~3)
        }
        guard let json else { throw ReadError.badChunk("no JSON chunk") }
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let doc: Document
        do { doc = try decoder.decode(Document.self, from: Data(json)) } catch {
            throw ReadError.badChunk("JSON: \(error.localizedDescription)")
        }
        let binary = bin ?? []
        let base = binary.startIndex

        func view(
            _ index: Int
        ) throws -> (start: Int, length: Int, stride: Int?, accessor: Document.Accessor) {
            guard let accessor = doc.accessors?[safe: index] else {
                throw ReadError.badAccessor(index, "missing")
            }
            guard let viewIndex = accessor.bufferView,
                let bufferView = doc.bufferViews?[safe: viewIndex]
            else { throw ReadError.badAccessor(index, "no buffer view") }
            let start = (bufferView.byteOffset ?? 0) + (accessor.byteOffset ?? 0)
            return (
                start, bufferView.byteLength - (accessor.byteOffset ?? 0), bufferView.byteStride,
                accessor
            )
        }

        func positions(_ index: Int) throws -> [Float] {
            let v = try view(index)
            guard v.accessor.componentType == 5126, v.accessor.type == "VEC3" else {
                throw ReadError.badAccessor(index, "POSITION must be float VEC3")
            }
            let stride = v.stride ?? 12
            guard v.accessor.count == 0 || (v.accessor.count - 1) * stride + 12 <= v.length,
                base + v.start + max(0, (v.accessor.count - 1) * stride + 12) <= binary.endIndex
            else { throw ReadError.badAccessor(index, "runs past its view") }
            var out = [Float](repeating: 0, count: v.accessor.count * 3)
            binary.withUnsafeBytes { raw in
                for i in 0..<v.accessor.count {
                    let at = v.start + i * stride
                    let x = raw.loadUnaligned(fromByteOffset: at, as: Float.self)
                    let y = raw.loadUnaligned(fromByteOffset: at + 4, as: Float.self)
                    let z = raw.loadUnaligned(fromByteOffset: at + 8, as: Float.self)
                    // glTF (x, y, z) = city (east, up, −north).
                    out[i * 3] = x
                    out[i * 3 + 1] = -z
                    out[i * 3 + 2] = y
                }
            }
            return out
        }

        func indices(_ index: Int, vertexCount: Int) throws -> [UInt32] {
            let v = try view(index)
            let size: Int
            switch v.accessor.componentType {
            case 5121: size = 1
            case 5123: size = 2
            case 5125: size = 4
            default: throw ReadError.badAccessor(index, "index type \(v.accessor.componentType)")
            }
            guard v.accessor.type == "SCALAR", v.accessor.count * size <= v.length,
                base + v.start + v.accessor.count * size <= binary.endIndex
            else { throw ReadError.badAccessor(index, "runs past its view") }
            var out = [UInt32](repeating: 0, count: v.accessor.count)
            binary.withUnsafeBytes { raw in
                for i in 0..<v.accessor.count {
                    let at = v.start + i * size
                    switch size {
                    case 1: out[i] = UInt32(raw[at])
                    case 2: out[i] = UInt32(raw.loadUnaligned(fromByteOffset: at, as: UInt16.self))
                    default: out[i] = raw.loadUnaligned(fromByteOffset: at, as: UInt32.self)
                    }
                }
            }
            guard out.allSatisfy({ Int($0) < vertexCount }) else {
                throw ReadError.badAccessor(index, "an index past the last vertex")
            }
            return out
        }

        var content = CityTileContent()
        for mesh in doc.meshes ?? [] {
            guard let primitive = mesh.primitives.first, let p = primitive.attributes["POSITION"]
            else { continue }
            var layer = TileMesh()
            layer.positions = try positions(p)
            if let i = primitive.indices {
                layer.indices = try indices(i, vertexCount: layer.vertexCount)
            } else {
                layer.indices = (0..<UInt32(layer.vertexCount)).map { $0 }
            }
            let mode = primitive.mode ?? 4
            switch (mesh.name ?? "", mode) {
            case ("streets", _), (_, 1): content.streets = layer
            case ("buildings", 4): content.buildings = layer
            case ("ground", 4): content.ground = layer
            default: continue
            }
        }
        content.labels = (doc.extras?.labels ?? []).compactMap { label in
            guard label.anchorEnuM.count == 3 else { return nil }
            return StreetLabel(
                name: label.name,
                anchor: SIMD3(
                    Float(label.anchorEnuM[0]), Float(label.anchorEnuM[1]),
                    Float(label.anchorEnuM[2])), bearingDeg: Float(label.bearingDeg ?? 0),
                streetClass: label.class ?? "")
        }
        content.s2 = doc.extras?.s2
        content.level = doc.extras?.level
        if let o = doc.extras?.originCityEnuM, o.count == 3 {
            content.origin = SIMD3(o[0], o[1], o[2])
        }
        return content
    }
}

extension Array {
    subscript(safe index: Int) -> Element? { indices.contains(index) ? self[index] : nil }
}
