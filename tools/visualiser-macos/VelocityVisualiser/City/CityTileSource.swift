// CityTileSource.swift
// Where the city tiles are read from: the `cityTiles.location` setting, a
// URL or a directory (lidar-city-view-plan Item 6).
//
// A bake writes `city-tiles/epochs/<epoch>/` and points `current` at the
// newest. Over SMB that symlink does not resolve, so a location may name an
// epoch directory directly, the `city-tiles` directory (its `current`, or
// else its newest epoch), or a `tileset.json`.

import Compression
import Foundation

enum CityTileSource {
    enum LocateError: Error, LocalizedError, Equatable {
        case noTileset(String)

        var errorDescription: String? {
            switch self {
            case .noTileset(let path): return "No tileset.json at \(path) or in its epochs"
            }
        }
    }

    /// The `tileset.json` a location names. A directory is searched for
    /// `tileset.json`, then `current/tileset.json`, then the newest
    /// `epochs/<epoch>/tileset.json`; a URL is used as it is, with
    /// `tileset.json` appended unless it names a JSON file.
    static func tilesetURL(for location: URL, fileManager: FileManager = .default) throws -> URL {
        if location.pathExtension == "json" { return location }
        guard location.isFileURL else { return location.appendingPathComponent("tileset.json") }
        let direct = location.appendingPathComponent("tileset.json")
        if fileManager.fileExists(atPath: direct.path) { return direct }
        let current = location.appendingPathComponent("current/tileset.json")
        if fileManager.fileExists(atPath: current.path) { return current }
        let epochs = location.appendingPathComponent("epochs")
        let names = (try? fileManager.contentsOfDirectory(atPath: epochs.path)) ?? []
        for name in names.sorted(by: >) {
            let candidate = epochs.appendingPathComponent(name).appendingPathComponent(
                "tileset.json")
            if fileManager.fileExists(atPath: candidate.path) { return candidate }
        }
        throw LocateError.noTileset(location.path)
    }

    /// The session tiles are fetched over HTTP with. An epoch's URLs never
    /// change (Gerardus serves them immutable), so unlike the API clients'
    /// this one keeps what it fetched on disk.
    static let session: URLSession = {
        let config = URLSessionConfiguration.default
        config.urlCache = URLCache(
            memoryCapacity: 32 * 1024 * 1024, diskCapacity: 1024 * 1024 * 1024,
            directory: FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first?
                .appendingPathComponent("city-tiles", isDirectory: true))
        config.requestCachePolicy = .useProtocolCachePolicy
        return URLSession(configuration: config)
    }()

    /// A tile's bytes, ungzipped when the file is gzip-compressed.
    static func read(_ url: URL, session: URLSession = CityTileSource.session) async throws -> Data
    {
        let data = try await StaticFiles.read(url, session: session)
        return try gunzipIfNeeded(data)
    }

    /// Strips a gzip wrapper (RFC 1952) and inflates its DEFLATE stream;
    /// other data is returned as it is.
    static func gunzipIfNeeded(_ data: Data) throws -> Data {
        let bytes = [UInt8](data)
        guard bytes.count > 18, bytes[0] == 0x1F, bytes[1] == 0x8B, bytes[2] == 8 else {
            return data
        }
        let flags = bytes[3]
        var offset = 10
        if flags & 0x04 != 0 {  // FEXTRA
            guard offset + 2 <= bytes.count else { throw CocoaError(.fileReadCorruptFile) }
            offset += 2 + Int(bytes[offset]) + Int(bytes[offset + 1]) << 8
        }
        if flags & 0x08 != 0 {  // FNAME
            while offset < bytes.count, bytes[offset] != 0 { offset += 1 }
            offset += 1
        }
        if flags & 0x10 != 0 {  // FCOMMENT
            while offset < bytes.count, bytes[offset] != 0 { offset += 1 }
            offset += 1
        }
        if flags & 0x02 != 0 { offset += 2 }  // FHCRC
        guard offset < bytes.count - 8 else { throw CocoaError(.fileReadCorruptFile) }
        let size =
            Int(bytes[bytes.count - 4]) | Int(bytes[bytes.count - 3]) << 8 | Int(
                bytes[bytes.count - 2]) << 16 | Int(bytes[bytes.count - 1]) << 24
        let deflated = Data(bytes[offset..<(bytes.count - 8)])
        // COMPRESSION_ZLIB is raw DEFLATE (RFC 1951), as gzip carries.
        let capacity = max(size, 64)
        var output = Data(count: capacity)
        let written = output.withUnsafeMutableBytes { out in
            deflated.withUnsafeBytes { input in
                compression_decode_buffer(
                    out.bindMemory(to: UInt8.self).baseAddress!, capacity,
                    input.bindMemory(to: UInt8.self).baseAddress!, deflated.count, nil,
                    COMPRESSION_ZLIB)
            }
        }
        guard written > 0 else { throw CocoaError(.fileReadCorruptFile) }
        return output.prefix(written)
    }
}
