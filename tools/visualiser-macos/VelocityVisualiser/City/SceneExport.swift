// SceneExport.swift
// A reader for velocity.report's published scene exports (lidar-city-view-
// plan Item 8; the format is lidar-web-scene-export-plan's, and
// public_html/src/js/scene-reader.js reads the same files for the web).
//
// A scene is a static directory: `manifest.json` lists its parts; each part
// has `header.json`, `index.json` (one entry per chunk with its frame count
// and time span), `timeline.json`, and `frames/chunk_NNNN.ndjson.gz`, one
// frame per line. Frame times are microsecond offsets from the part's first
// frame; `header.start_ns` is the absolute instant, as a string.
//
// The city view plays nearby sites at street range: boxes and headings only.
// Points in a frame are skipped. Track ids are export-local, and they are
// namespaced by site wherever they are drawn, so nothing joins two sites.

import Foundation

enum SceneError: Error, LocalizedError, Equatable {
    case malformed(String)
    case unavailable(String)

    var errorDescription: String? {
        switch self {
        case .malformed(let why): return "The scene is malformed: \(why)"
        case .unavailable(let why): return "The scene cannot be read: \(why)"
        }
    }
}

/// One box in a frame, in the scene's sensor frame (metres, radians).
struct SceneBox: Equatable, Sendable {
    var id: String
    /// The box's centre.
    var x: Float
    var y: Float
    var z: Float
    var length: Float
    var width: Float
    var height: Float
    /// The box's heading, and the track's heading of travel.
    var boxHeading: Float
    var heading: Float
    var speed: Float
    var classLabel: String
}

struct SceneFrame: Equatable, Sendable {
    /// Microseconds from the part's first frame.
    var t: Int64
    var boxes: [SceneBox]
}

struct SceneChunk: Equatable, Sendable {
    var c: Int
    var n: Int
    var t0: Int64
    var t1: Int64
}

/// A part's header, index and timeline.
struct ScenePartInfo: Equatable, Sendable {
    var site: String
    var title: String
    var sensorID: String
    var startNs: Int64
    var durationSec: Double
    var frameCount: Int
    var chunks: [SceneChunk]
    /// The timeline's buckets: their start in seconds and track count.
    var bucketSeconds: Double
    var buckets: [(t: Double, n: Int)]

    static func == (a: ScenePartInfo, b: ScenePartInfo) -> Bool {
        a.site == b.site && a.title == b.title && a.sensorID == b.sensorID && a.startNs == b.startNs
            && a.durationSec == b.durationSec && a.frameCount == b.frameCount
            && a.chunks == b.chunks && a.bucketSeconds == b.bucketSeconds
            && a.buckets.map(\.t) == b.buckets.map(\.t) && a.buckets.map(\.n) == b.buckets.map(\.n)
    }

    var startUs: Int64 { chunks.first?.t0 ?? 0 }
    var endUs: Int64 { chunks.last?.t1 ?? 0 }
    var spanSec: Double { Double(endUs - startUs) / 1e6 }

    /// The chunk whose span holds `us`; between two chunks, the earlier.
    func chunkIndex(forOffset us: Int64) -> Int {
        guard let first = chunks.first, let last = chunks.last else { return 0 }
        if us <= first.t0 { return 0 }
        if us >= last.t1 { return chunks.count - 1 }
        var lo = 0
        var hi = chunks.count - 1
        while lo <= hi {
            let mid = (lo + hi) / 2
            if us < chunks[mid].t0 {
                hi = mid - 1
            } else if us > chunks[mid].t1 {
                lo = mid + 1
            } else {
                return mid
            }
        }
        return max(0, min(chunks.count - 1, hi))
    }
}

enum SceneDecoding {
    private struct Manifest: Decodable {
        struct Site: Decodable {
            var id: String?
            var title: String?
        }
        struct Part: Decodable { var url: String }
        var site: Site?
        var parts: [Part]
    }

    /// The manifest's part directories, resolved against its own.
    static func partURLs(manifest data: Data, at url: URL) throws -> (title: String?, parts: [URL])
    {
        let manifest: Manifest
        do { manifest = try JSONDecoder().decode(Manifest.self, from: data) } catch {
            throw SceneError.malformed("manifest: \(error.localizedDescription)")
        }
        guard !manifest.parts.isEmpty else { throw SceneError.malformed("manifest lists no parts") }
        let base = url.deletingLastPathComponent()
        let parts = manifest.parts.map { part -> URL in
            var path = part.url
            if path.hasPrefix("./") { path.removeFirst(2) }
            return base.appendingPathComponent(path, isDirectory: true)
        }
        return (manifest.site?.title, parts)
    }

    private struct Header: Decodable {
        var site: String?
        var title: String?
        var sensorId: String?
        var startNs: String?
        var durationSec: Double?
        var frameCount: Int?
    }
    private struct Index: Decodable {
        struct Chunk: Decodable {
            var c: Int
            var n: Int
            var t0: Double
            var t1: Double
        }
        var chunks: [Chunk]
    }
    private struct Timeline: Decodable {
        struct Bucket: Decodable {
            var t: Double
            var n: Int?
        }
        var bucketSeconds: Double?
        var buckets: [Bucket]?
    }

    static func partInfo(header: Data, index: Data, timeline: Data?) throws -> ScenePartInfo {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let h: Header
        let i: Index
        do {
            h = try decoder.decode(Header.self, from: header)
            i = try decoder.decode(Index.self, from: index)
        } catch { throw SceneError.malformed("header or index: \(error.localizedDescription)") }
        guard !i.chunks.isEmpty else { throw SceneError.malformed("the index lists no chunks") }
        let chunks = i.chunks.map {
            SceneChunk(c: $0.c, n: $0.n, t0: Int64($0.t0), t1: Int64($0.t1))
        }
        for c in chunks where c.t1 < c.t0 {
            throw SceneError.malformed("chunk \(c.c) ends before it starts")
        }
        let t = timeline.flatMap { try? decoder.decode(Timeline.self, from: $0) }
        return ScenePartInfo(
            site: h.site ?? "", title: h.title ?? h.site ?? "", sensorID: h.sensorId ?? "",
            startNs: Int64(h.startNs ?? "") ?? 0,
            durationSec: h.durationSec ?? Double(chunks.last!.t1 - chunks.first!.t0) / 1e6,
            frameCount: h.frameCount ?? chunks.reduce(0) { $0 + $1.n }, chunks: chunks,
            bucketSeconds: t?.bucketSeconds ?? 0,
            buckets: (t?.buckets ?? []).map { ($0.t, $0.n ?? 0) })
    }

    private struct Frame: Decodable {
        struct Track: Decodable {
            var id: String
            var x: Float
            var y: Float
            var z: Float?
            var l: Float?
            var w: Float?
            var h: Float?
            var hdg: Float?
            var bh: Float?
            var bx: Float?
            var by: Float?
            var spd: Float?
            var c: String?
        }
        var t: Double?
        var tr: [Track]?
    }

    /// A chunk's frames: boxes and headings only. A box is drawn at its own
    /// centre (`bx`, `by`) when the export gives one.
    static func frames(chunk data: Data) throws -> [SceneFrame] {
        let text = try CityTileSource.gunzipIfNeeded(data)
        var out = [SceneFrame]()
        let decoder = JSONDecoder()
        for line in text.split(separator: UInt8(ascii: "\n")) where !line.isEmpty {
            let f: Frame
            do { f = try decoder.decode(Frame.self, from: Data(line)) } catch {
                throw SceneError.malformed("a frame: \(error.localizedDescription)")
            }
            guard let t = f.t else { throw SceneError.malformed("a frame with no timestamp") }
            out.append(
                SceneFrame(
                    t: Int64(t),
                    boxes: (f.tr ?? []).map { r in
                        SceneBox(
                            id: r.id, x: r.bx ?? r.x, y: r.by ?? r.y, z: r.z ?? 0, length: r.l ?? 1,
                            width: r.w ?? 1, height: r.h ?? 1, boxHeading: r.bh ?? r.hdg ?? 0,
                            heading: r.hdg ?? 0, speed: r.spd ?? 0, classLabel: r.c ?? "")
                    }))
        }
        guard !out.isEmpty else { throw SceneError.malformed("a chunk with no frames") }
        return out
    }

    /// The frame at or just before `us`, by binary search.
    static func frame(in frames: [SceneFrame], atOffset us: Int64) -> SceneFrame? {
        guard !frames.isEmpty else { return nil }
        var lo = 0
        var hi = frames.count - 1
        var best = 0
        while lo <= hi {
            let mid = (lo + hi) / 2
            if frames[mid].t <= us {
                best = mid
                lo = mid + 1
            } else {
                hi = mid - 1
            }
        }
        return frames[best]
    }
}

/// One part, read from its directory, with the chunks near the playhead.
actor ScenePart {
    let url: URL
    let info: ScenePartInfo
    private var cache: [Int: [SceneFrame]] = [:]
    private var order: [Int] = []
    private var unavailable: Set<Int> = []
    private let read: @Sendable (URL) async throws -> Data
    static let cacheLimit = 3

    init(url: URL, info: ScenePartInfo, read: @escaping @Sendable (URL) async throws -> Data) {
        self.url = url
        self.info = info
        self.read = read
    }

    static func open(
        _ url: URL,
        read: @escaping @Sendable (URL) async throws -> Data = { try await StaticFiles.read($0) }
    ) async throws -> ScenePart {
        let header = try await read(url.appendingPathComponent("header.json"))
        let index = try await read(url.appendingPathComponent("index.json"))
        let timeline = try? await read(url.appendingPathComponent("timeline.json"))
        let info = try SceneDecoding.partInfo(header: header, index: index, timeline: timeline)
        return ScenePart(url: url, info: info, read: read)
    }

    func chunk(_ id: Int) async throws -> [SceneFrame] {
        if let frames = cache[id] {
            order.removeAll { $0 == id }
            order.append(id)
            return frames
        }
        guard !unavailable.contains(id) else { throw SceneError.unavailable("chunk \(id)") }
        let name = String(format: "chunk_%04d.ndjson.gz", id)
        let data: Data
        do {
            data = try await read(url.appendingPathComponent("frames").appendingPathComponent(name))
        } catch {
            unavailable.insert(id)
            throw SceneError.unavailable(name)
        }
        let frames = try SceneDecoding.frames(chunk: data)
        cache[id] = frames
        order.append(id)
        while order.count > Self.cacheLimit { cache[order.removeFirst()] = nil }
        return frames
    }

    /// The frame to show at `us` from the part's start.
    func frame(atOffset us: Int64) async throws -> SceneFrame? {
        let index = info.chunkIndex(forOffset: us)
        let frames = try await chunk(info.chunks[index].c)
        return SceneDecoding.frame(in: frames, atOffset: us)
    }

    var cachedChunks: [Int] { order }
}

/// A scene's parts on one timeline, played on the scene's own clock.
struct SceneSession: Sendable {
    let title: String
    let parts: [ScenePart]
    let infos: [ScenePartInfo]
    /// Each part's start on the scene's timeline, seconds.
    let offsets: [Double]
    let duration: Double

    static func open(
        manifest url: URL,
        read: @escaping @Sendable (URL) async throws -> Data = { try await StaticFiles.read($0) }
    ) async throws -> SceneSession {
        let (title, urls) = try SceneDecoding.partURLs(manifest: try await read(url), at: url)
        var parts = [ScenePart]()
        for partURL in urls { parts.append(try await ScenePart.open(partURL, read: read)) }
        let infos = parts.map(\.info)
        var offsets = [Double]()
        var total = 0.0
        for info in infos {
            offsets.append(total)
            total += info.spanSec
        }
        return SceneSession(
            title: title ?? infos.first?.title ?? "Scene", parts: parts, infos: infos,
            offsets: offsets, duration: total)
    }

    /// The part and offset in microseconds for `seconds` of scene time.
    func locate(_ seconds: Double) -> (part: Int, us: Int64) {
        let t = min(max(seconds, 0), duration)
        var index = offsets.count - 1
        for i in offsets.indices where t < offsets[i] + infos[i].spanSec {
            index = i
            break
        }
        let us = infos[index].startUs + Int64(((t - offsets[index]) * 1e6).rounded())
        return (index, us)
    }

    func frame(at seconds: Double) async throws -> (part: Int, frame: SceneFrame?) {
        let (part, us) = locate(seconds)
        return (part, try await parts[part].frame(atOffset: us))
    }

    /// The absolute capture instant of `seconds` of scene time, nanoseconds.
    func captureNanos(at seconds: Double) -> Int64 {
        let (part, us) = locate(seconds)
        return infos[part].startNs + us * 1000
    }
}

/// A site's own clock: scene time from when it started playing, looped. Each
/// site has its own, started when it came into view; sites recorded at
/// different times are never put on one clock.
struct SceneClock: Equatable, Sendable {
    var startedAt: Double
    var rate: Double = 1
    var duration: Double

    func sceneSeconds(at now: Double) -> Double {
        guard duration > 0 else { return 0 }
        let elapsed = max(0, now - startedAt) * rate
        return elapsed.truncatingRemainder(dividingBy: duration)
    }
}
