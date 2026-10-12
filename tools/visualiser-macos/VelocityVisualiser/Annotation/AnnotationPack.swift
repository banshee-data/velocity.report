// AnnotationPack.swift
// Reader for the immutable point packs written by Go's internal/lidar/annotation.
//
// A pack is one run's view of a capture: manifest.json names the point domain
// and its digests, samples.json locates each frame's block, points.bin holds
// the canonical little-endian arrays, packets.bin is the packet table, and
// returns.bin is the return each point was decoded from. The returns are what
// let a label set made through one run's pack apply to another run's pack of
// the same capture. Nothing here writes to a pack. A pack whose bytes disagree
// with its manifest is refused rather than opened partially, because a point
// index only means anything under the exact digest it was recorded against.
//
// Only schema version 2 is read. A version 1 pack is refused by name
// (AnnotationPackError.version1Pack): its labels have been migrated into label
// sets, and the git tag annotation-v1-reader keeps the last build that reads it.

import CryptoKit
import Foundation
import simd

/// What the source recording could actually support, carried through so a
/// foreground-only or decimated pack never reads as full-scene truth.
enum CaptureCoverage: String, Codable {
    case full
    case foregroundOnly = "foreground_only"
    case decimated
}

/// The coordinate contract the pack's points are expressed in. Selection is
/// geometric, so an unread transform version would silently invalidate every
/// index; it is carried and surfaced rather than assumed.
struct CoordinateContract: Codable, Equatable {
    var units: String
    var frameID: String
    var referenceFrame: String
    var handedness: String
    var originNote: String
    var transformVersion: String

    enum CodingKeys: String, CodingKey {
        case units
        case frameID = "frame_id"
        case referenceFrame = "reference_frame"
        case handedness
        case originNote = "origin_note"
        case transformVersion = "transform_version"
    }
}

/// Where the pack came from. Retained for provenance display; an operator
/// reviewing a mask needs to know which recording it belongs to.
struct SourceProvenance: Codable, Equatable {
    var vrlogPath: String
    var vrlogHeaderSHA: String
    var vrlogFramesSHA: String
    var pcapBasename: String?
    var sensorID: String
    var runConfigID: String?
    var configHash: String?
    var paramsHash: String?
    var buildVersion: String?
    var buildGitSHA: String?
    /// The run's L4 height-band filter, when the exporter knew it. Absent
    /// from packs cut before it was recorded.
    var heightBand: HeightBand?

    enum CodingKeys: String, CodingKey {
        case vrlogPath = "vrlog_path"
        case vrlogHeaderSHA = "vrlog_header_sha256"
        case vrlogFramesSHA = "vrlog_frames_sha256"
        case pcapBasename = "pcap_basename"
        case sensorID = "sensor_id"
        case runConfigID = "run_config_id"
        case configHash = "config_hash"
        case paramsHash = "params_hash"
        case buildVersion = "build_version"
        case buildGitSHA = "build_git_sha"
        case heightBand = "height_band"
    }
}

/// The L4 height-band filter a run was made with: the rule Go's
/// `l4perception.HeightBandFilter` applies before clustering.
///
/// A pack holds points as recorded, which is before that filter ran. With the
/// band, the tool can show exactly which of them the clusterer never saw.
struct HeightBand: Codable, Equatable {
    var floorM: Double
    var ceilingM: Double
    var removeGround: Bool

    enum CodingKeys: String, CodingKey {
        case floorM = "floor_m"
        case ceilingM = "ceiling_m"
        case removeGround = "remove_ground"
    }

    /// The pipeline's defaults (`DefaultHeightBandFilter`), for a pack that
    /// does not record its own. Shown as assumed wherever it is used.
    static let pipelineDefault = HeightBand(floorM: -2.8, ceilingM: 1.5, removeGround: true)

    /// True when the filter dropped a return at this height. Both comparisons
    /// are strict and made in double precision, as the filter's are: a return
    /// exactly on the floor was kept.
    func removes(z: Float) -> Bool { removeGround && (Double(z) < floorM || Double(z) > ceilingM) }
}

/// The pack manifest. Only the fields this client uses are decoded; unknown
/// fields are ignored so a newer writer adding provenance does not break an
/// older reader that still understands the point domain.
struct AnnotationManifest: Codable, Equatable {
    var schemaVersion: Int
    var datasetID: String
    var createdNs: Int64
    var source: SourceProvenance
    var coordinate: CoordinateContract
    var coverage: CaptureCoverage
    var coverageNote: String?
    var sampleCount: Int
    var pointCount: Int64
    var pointsSHA256: String
    var samplesSHA256: String
    var packDigest: String
    var hasIntensity: Bool
    var hasClassification: Bool
    /// The settled background carried with the pack, when the recording had
    /// one. Context for the operator, outside the point domain and its digest.
    var backgroundCount: Int?
    var backgroundsSHA256: String?
    var backgroundPointsSHA256: String?
    /// How a packet divides into returns, how packets are keyed, and the
    /// digests of the packet table and the returns. Optional only because Go
    /// omits an empty value: `open` refuses a pack that lacks them.
    var returnLayout: ReturnLayout?
    var packetKey: String?
    var packetCount: Int?
    var packetsSHA256: String?
    var returnsSHA256: String?

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case datasetID = "dataset_id"
        case createdNs = "created_ns"
        case source
        case coordinate
        case coverage
        case coverageNote = "coverage_note"
        case sampleCount = "sample_count"
        case pointCount = "point_count"
        case pointsSHA256 = "points_sha256"
        case samplesSHA256 = "samples_sha256"
        case packDigest = "pack_digest"
        case hasIntensity = "has_intensity"
        case hasClassification = "has_classification"
        case backgroundCount = "background_count"
        case backgroundsSHA256 = "backgrounds_sha256"
        case backgroundPointsSHA256 = "background_points_sha256"
        case returnLayout = "return_layout"
        case packetKey = "packet_key"
        case packetCount = "packet_count"
        case packetsSHA256 = "packets_sha256"
        case returnsSHA256 = "returns_sha256"
    }
}

/// One settled-background snapshot carried with a pack.
///
/// The snapshot in force at a frame is the last one recorded at or before it,
/// by position in the recording. Not by timestamp or sequence number: a replay
/// settled ahead of time records its first snapshot stamped after every frame
/// that follows it, and the sequence number only moves when the grid resets.
struct AnnotationBackground: Codable, Equatable {
    var backgroundID: Int
    var sourceOrdinal: Int
    var timestampNs: Int64
    var sequenceNumber: UInt64
    var settlingComplete: Bool
    var pointCount: Int
    var byteOffset: Int64

    enum CodingKeys: String, CodingKey {
        case backgroundID = "background_id"
        case sourceOrdinal = "source_ordinal"
        case timestampNs = "timestamp_ns"
        case sequenceNumber = "sequence_number"
        case settlingComplete = "settling_complete"
        case pointCount = "point_count"
        case byteOffset = "byte_offset"
    }
}

/// A background snapshot's decoded coordinates. Not `PackPoints`: these have
/// no canonical index a mask could cite, and keeping the types apart is what
/// stops one being selected from.
struct BackgroundPoints: Equatable {
    var x: [Float] = []
    var y: [Float] = []
    var z: [Float] = []

    var count: Int { x.count }
}

extension AnnotationSample {
    /// What the window calls a frame: its sample ID, as every tool, record
    /// and report names it. The window once showed a 1-based position in one
    /// place and the recording's frame number in another, and sample 500 read
    /// as "frame 501" beside "frame 504": an off-by-one that was not one.
    var label: String { "sample \(sampleID)" }

    /// The recording's own frame number, second, for finding the frame in the
    /// main view's timeline.
    var recordingLabel: String { "recording frame \(sourceOrdinal)" }
}

/// One frame's point domain inside the pack.
///
/// `sampleID` is pack-local and dense; it is deliberately not the tracker's
/// frame number, because record order is not capture order and two source
/// frames may share a timestamp.
struct AnnotationSample: Codable, Equatable {
    var sampleID: Int
    var sourceOrdinal: Int
    var sourceFrameID: UInt64
    var timestampNs: Int64
    var sensorID: String
    var pointCount: Int
    var byteOffset: Int64
    /// Whether this sample's source frame carried an intensity column. The
    /// block zero-fills an absent column, so a zero byte cannot say which it
    /// is; only this can. Required: a sample without it is refused.
    var hasIntensity: Bool
    /// Every return of the frame the sample came from, foreground or not, as
    /// [returnStart, returnEnd) in the pack's return numbering. A label set's
    /// window is that range. Go omits a zero, so absent reads as 0: the first
    /// frame of a packet table starts at return 0 and carries no return_start.
    var returnStart: UInt64 = 0
    var returnEnd: UInt64 = 0

    enum CodingKeys: String, CodingKey {
        case sampleID = "sample_id"
        case sourceOrdinal = "source_ordinal"
        case sourceFrameID = "source_frame_id"
        case timestampNs = "timestamp_ns"
        case sensorID = "sensor_id"
        case pointCount = "point_count"
        case byteOffset = "byte_offset"
        case hasIntensity = "has_intensity"
        case returnStart = "return_start"
        case returnEnd = "return_end"
    }
}

extension AnnotationSample {
    /// Decodes Go's sample, whose return range is written with omitempty.
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        sampleID = try c.decode(Int.self, forKey: .sampleID)
        sourceOrdinal = try c.decode(Int.self, forKey: .sourceOrdinal)
        sourceFrameID = try c.decode(UInt64.self, forKey: .sourceFrameID)
        timestampNs = try c.decode(Int64.self, forKey: .timestampNs)
        sensorID = try c.decode(String.self, forKey: .sensorID)
        pointCount = try c.decode(Int.self, forKey: .pointCount)
        byteOffset = try c.decode(Int64.self, forKey: .byteOffset)
        guard let flag = try c.decodeIfPresent(Bool.self, forKey: .hasIntensity) else {
            throw DecodingError.keyNotFound(
                CodingKeys.hasIntensity,
                DecodingError.Context(
                    codingPath: c.codingPath,
                    debugDescription: "sample \(sampleID) does not say whether it has intensity"))
        }
        hasIntensity = flag
        returnStart = try c.decodeIfPresent(UInt64.self, forKey: .returnStart) ?? 0
        returnEnd = try c.decodeIfPresent(UInt64.self, forKey: .returnEnd) ?? 0
    }
}

/// One sample's decoded arrays. Index `i` is the canonical point reference
/// used by every mask: it is only valid under this pack's digest.
struct PackPoints: Equatable {
    var x: [Float]
    var y: [Float]
    var z: [Float]
    var intensity: [UInt8]
    var classification: [UInt8]

    var count: Int { x.count }

    init(
        x: [Float] = [], y: [Float] = [], z: [Float] = [], intensity: [UInt8] = [],
        classification: [UInt8] = []
    ) {
        self.x = x
        self.y = y
        self.z = z
        self.intensity = intensity
        self.classification = classification
    }

    /// Mean horizontal distance from the sensor of the returns at `indices`.
    func meanRange(of indices: Set<Int>) -> Float {
        var sum: Float = 0
        var found: Float = 0
        for index in indices where index >= 0 && index < count {
            sum += (x[index] * x[index] + y[index] * y[index]).squareRoot()
            found += 1
        }
        return found > 0 ? sum / found : 0
    }

    /// The point at a canonical index, or nil when the index is out of range.
    /// Callers validating operator input should prefer this to subscripting.
    func point(at index: Int) -> simd_float3? {
        guard index >= 0, index < count else { return nil }
        return simd_float3(x[index], y[index], z[index])
    }
}

enum AnnotationPackError: Error, Equatable, CustomStringConvertible {
    case unreadable(String)
    case malformed(String)
    case digestMismatch(field: String, expected: String, actual: String)
    case unsupportedSchema(Int)
    /// A version 1 pack: points with no returns. Refused by name, so the
    /// operator learns where its labels went and how to read it if they must.
    case version1Pack
    case sampleOutOfRange(Int)
    case blockOutOfBounds(sampleID: Int)

    /// What the window shows when a pack will not open.
    var description: String {
        switch self {
        case .unreadable(let detail): return "cannot read \(detail)"
        case .malformed(let detail): return detail
        case .digestMismatch(let field, let expected, let actual):
            return
                "\(field) digest mismatch: the manifest says \(expected), the bytes give \(actual)"
        case .unsupportedSchema(let version):
            return
                "pack schema version \(version); this build reads version \(AnnotationPack.schemaVersion)"
        case .version1Pack:
            return "This is a version 1 annotation pack. This build reads version 2 packs only. "
                + "Version 1 labels were migrated into label sets: see "
                + "docs/lidar/operations/annotation-label-set-migration.md. To read a version 1 "
                + "pack, build the git tag annotation-v1-reader."
        case .sampleOutOfRange(let id): return "no sample \(id) in this pack"
        case .blockOutOfBounds(let id): return "sample \(id) lies outside points.bin"
        }
    }
}

/// An opened, digest-verified annotation pack.
final class AnnotationPack {
    /// The only version read: the points plus the packet table and each
    /// point's return.
    static let schemaVersion = 2
    /// The one packet key derivation version 2 packs use.
    static let packetKeyKind = "sha256-64:udp-payload"

    /// Encoded size of one sample's block: three float32 coordinate arrays
    /// then two uint8 attribute arrays. Absent attributes are written as
    /// zeros by the exporter, so the size depends only on the point count.
    static func blockBytes(pointCount n: Int) -> Int64 { Int64(n) * 12 + Int64(n) * 2 }

    let directory: URL
    let manifest: AnnotationManifest
    let samples: [AnnotationSample]
    /// In recording order. Empty for a pack cut before backgrounds were
    /// carried, or from a recording that had none.
    let backgrounds: [AnnotationBackground]

    /// The packet table, keys in table order.
    let packets: [UInt64]

    private let raw: Data
    private let rawBackground: Data
    /// returns.bin, verified, and where each sample's run of it starts.
    private let rawReturns: Data
    private let returnOffsets: [Int]

    private init(
        directory: URL, manifest: AnnotationManifest, samples: [AnnotationSample], raw: Data,
        backgrounds: [AnnotationBackground], rawBackground: Data, packets: [UInt64],
        rawReturns: Data, returnOffsets: [Int]
    ) {
        self.directory = directory
        self.manifest = manifest
        self.samples = samples
        self.raw = raw
        self.backgrounds = backgrounds
        self.rawBackground = rawBackground
        self.packets = packets
        self.rawReturns = rawReturns
        self.returnOffsets = returnOffsets
    }

    /// The directory a pack's label sets live under: the pack's parent, as
    /// `<annotation dir>/labels/<name>/`.
    var annotationDirectory: URL { directory.deletingLastPathComponent() }

    /// Opens a pack directory, verifying both digests before returning. A
    /// mismatch fails closed: point indices recorded against different bytes
    /// are not approximately valid, they are meaningless.
    static func open(directory: URL) throws -> AnnotationPack {
        let manifestData = try read(directory.appendingPathComponent("manifest.json"))
        let samplesData = try read(directory.appendingPathComponent("samples.json"))
        let pointsData = try read(directory.appendingPathComponent("points.bin"))

        let decoder = JSONDecoder()
        let manifest: AnnotationManifest
        do { manifest = try decoder.decode(AnnotationManifest.self, from: manifestData) } catch {
            throw AnnotationPackError.malformed("manifest.json: \(error)")
        }
        if manifest.schemaVersion == 1 { throw AnnotationPackError.version1Pack }
        guard manifest.schemaVersion == schemaVersion else {
            throw AnnotationPackError.unsupportedSchema(manifest.schemaVersion)
        }

        // The writer appends a newline to samples.json for readability but
        // digests the JSON without it. Trim before digesting or every pack
        // would look corrupt.
        let samplesDigest = digest(trimTrailingNewline(samplesData))
        guard samplesDigest == manifest.samplesSHA256 else {
            throw AnnotationPackError.digestMismatch(
                field: "samples.json", expected: manifest.samplesSHA256, actual: samplesDigest)
        }
        let pointsDigest = digest(pointsData)
        guard pointsDigest == manifest.pointsSHA256 else {
            throw AnnotationPackError.digestMismatch(
                field: "points.bin", expected: manifest.pointsSHA256, actual: pointsDigest)
        }
        // The pack digest binds the array digests together, so swapping a
        // matched pair of files cannot pass as the same pack. It covers the
        // packet table and the returns, since a label set cites points
        // through them.
        let packDigest = AnnotationPack.packDigest(of: manifest)
        guard packDigest == manifest.packDigest else {
            throw AnnotationPackError.digestMismatch(
                field: "pack_digest", expected: manifest.packDigest, actual: packDigest)
        }

        let samples: [AnnotationSample]
        do { samples = try decoder.decode([AnnotationSample].self, from: samplesData) } catch {
            throw AnnotationPackError.malformed("samples.json: \(error)")
        }
        guard samples.count == manifest.sampleCount else {
            throw AnnotationPackError.malformed(
                "manifest declares \(manifest.sampleCount) samples, samples.json has \(samples.count)"
            )
        }
        // The manifest's flag means "any sample was measured". It is outside
        // the pack digest, so an edit to it would pass every check above, and
        // a true flag over samples that all say false would send the reader
        // to zero bytes as if they were readings. Go's reader refuses it too.
        let anyIntensity = samples.contains { $0.hasIntensity }
        guard samples.isEmpty || anyIntensity == manifest.hasIntensity else {
            throw AnnotationPackError.malformed(
                "manifest has_intensity=\(manifest.hasIntensity) but the samples' own flags say \(anyIntensity)"
            )
        }

        let (backgrounds, backgroundData) = try openBackground(
            directory: directory, manifest: manifest, decoder: decoder)
        let (packets, returns, offsets) = try openReturns(
            directory: directory, manifest: manifest, samples: samples)
        return AnnotationPack(
            directory: directory, manifest: manifest, samples: samples, raw: pointsData,
            backgrounds: backgrounds, rawBackground: backgroundData, packets: packets,
            rawReturns: returns, returnOffsets: offsets)
    }

    /// The pack digest the manifest should carry: over the points, samples,
    /// packets and returns digests, each the "sha256:…" string,
    /// newline-separated.
    static func packDigest(of manifest: AnnotationManifest) -> String {
        let parts = [
            manifest.pointsSHA256, manifest.samplesSHA256, manifest.packetsSHA256 ?? "",
            manifest.returnsSHA256 ?? "",
        ]
        return digest(Data(parts.joined(separator: "\n").utf8))
    }

    /// Reads and verifies the pack's packet table and returns, with
    /// the invariants Go's reader holds: no key listed twice, one return per
    /// point, returns increasing within a sample and inside its frame, and
    /// every frame inside the packet table.
    private static func openReturns(
        directory: URL, manifest: AnnotationManifest, samples: [AnnotationSample]
    ) throws -> ([UInt64], Data, [Int]) {
        guard let layout = manifest.returnLayout else {
            throw AnnotationPackError.malformed("manifest.json has no return layout")
        }
        do { try layout.validate() } catch {
            throw AnnotationPackError.malformed("return layout: \(error)")
        }
        guard manifest.packetKey == packetKeyKind else {
            throw AnnotationPackError.malformed(
                "packet key \"\(manifest.packetKey ?? "")\", this build reads \"\(packetKeyKind)\"")
        }
        let packetData = try read(directory.appendingPathComponent("packets.bin"))
        let returnData = try read(directory.appendingPathComponent("returns.bin"))
        let packetsDigest = digest(packetData)
        guard packetsDigest == manifest.packetsSHA256 else {
            throw AnnotationPackError.digestMismatch(
                field: "packets.bin", expected: manifest.packetsSHA256 ?? "", actual: packetsDigest)
        }
        let returnsDigest = digest(returnData)
        guard returnsDigest == manifest.returnsSHA256 else {
            throw AnnotationPackError.digestMismatch(
                field: "returns.bin", expected: manifest.returnsSHA256 ?? "", actual: returnsDigest)
        }
        let packetCount = manifest.packetCount ?? 0
        guard packetData.count % 8 == 0, packetData.count / 8 == packetCount else {
            throw AnnotationPackError.malformed(
                "packets.bin is \(packetData.count) bytes for \(packetCount) declared packets")
        }
        guard Int64(returnData.count) == 8 * manifest.pointCount else {
            throw AnnotationPackError.malformed(
                "returns.bin is \(returnData.count) bytes for \(manifest.pointCount) points")
        }
        let packets: [UInt64]
        do { packets = try PacketKeys.decode(packetData, name: "packets.bin") } catch {
            throw AnnotationPackError.malformed("\(error)")
        }
        var seen = Set<UInt64>()
        seen.reserveCapacity(packets.count)
        for key in packets where !seen.insert(key).inserted {
            throw AnnotationPackError.malformed(
                "packet table lists key \(PacketKeys.hex(key)) twice")
        }
        let limit = UInt64(packetCount) * layout.perPacket
        var offsets: [Int] = []
        offsets.reserveCapacity(samples.count)
        var offset = 0
        try returnData.withUnsafeBytes { buffer in
            for (i, sample) in samples.enumerated() {
                guard sample.sampleID == i else {
                    throw AnnotationPackError.malformed(
                        "sample \(i) carries id \(sample.sampleID): pack-local ids must be dense and ordered"
                    )
                }
                let start = sample.returnStart
                let end = sample.returnEnd
                guard start < end, end <= limit else {
                    throw AnnotationPackError.malformed(
                        "sample \(i) frame range [\(start),\(end)) is empty or outside the \(limit) returns the packet table numbers"
                    )
                }
                offsets.append(offset)
                var previous: UInt64?
                for j in 0..<sample.pointCount {
                    let r = UInt64(
                        littleEndian: buffer.loadUnaligned(
                            fromByteOffset: 8 * (offset + j), as: UInt64.self))
                    guard r >= start, r < end else {
                        throw AnnotationPackError.malformed(
                            "sample \(i) point \(j) is return \(r), outside its frame [\(start),\(end))"
                        )
                    }
                    if let previous, r <= previous {
                        throw AnnotationPackError.malformed(
                            "sample \(i) point \(j): returns must increase with point order, \(r) follows \(previous)"
                        )
                    }
                    previous = r
                }
                offset += sample.pointCount
            }
        }
        return (packets, returnData, offsets)
    }

    /// One sample's return numbers in the pack's own packet table, one per
    /// point in point order.
    func returns(sampleID: Int) throws -> [UInt64] {
        guard sampleID >= 0, sampleID < samples.count else {
            throw AnnotationPackError.sampleOutOfRange(sampleID)
        }
        let n = samples[sampleID].pointCount
        let base = returnOffsets[sampleID]
        return rawReturns.withUnsafeBytes { buffer in
            (0..<n).map {
                UInt64(
                    littleEndian: buffer.loadUnaligned(
                        fromByteOffset: 8 * (base + $0), as: UInt64.self))
            }
        }
    }

    /// Reads and verifies the background context. A pack that declares one and
    /// cannot produce it is refused like any other mismatch: an operator shown
    /// the wrong background would label against a scene that was not there.
    private static func openBackground(
        directory: URL, manifest: AnnotationManifest, decoder: JSONDecoder
    ) throws -> ([AnnotationBackground], Data) {
        guard let declared = manifest.backgroundCount, declared > 0 else { return ([], Data()) }
        let indexData = try read(directory.appendingPathComponent("backgrounds.json"))
        let pointsData = try read(directory.appendingPathComponent("background.bin"))

        let indexDigest = digest(trimTrailingNewline(indexData))
        guard indexDigest == manifest.backgroundsSHA256 else {
            throw AnnotationPackError.digestMismatch(
                field: "backgrounds.json", expected: manifest.backgroundsSHA256 ?? "",
                actual: indexDigest)
        }
        let pointsDigest = digest(pointsData)
        guard pointsDigest == manifest.backgroundPointsSHA256 else {
            throw AnnotationPackError.digestMismatch(
                field: "background.bin", expected: manifest.backgroundPointsSHA256 ?? "",
                actual: pointsDigest)
        }

        let backgrounds: [AnnotationBackground]
        do { backgrounds = try decoder.decode([AnnotationBackground].self, from: indexData) } catch
        { throw AnnotationPackError.malformed("backgrounds.json: \(error)") }
        guard backgrounds.count == declared else {
            throw AnnotationPackError.malformed(
                "manifest declares \(declared) backgrounds, backgrounds.json has \(backgrounds.count)"
            )
        }
        for background in backgrounds {
            let end = background.byteOffset + Int64(background.pointCount) * 12
            guard background.byteOffset >= 0, background.pointCount >= 0,
                end <= Int64(pointsData.count)
            else {
                throw AnnotationPackError.malformed(
                    "background \(background.backgroundID) lies outside background.bin")
            }
        }
        return (backgrounds, pointsData)
    }

    /// The snapshot in force at a sample: the last recorded at or before it.
    func backgroundInForce(at sample: AnnotationSample) -> AnnotationBackground? {
        backgrounds.last { $0.sourceOrdinal <= sample.sourceOrdinal }
    }

    /// Decodes one snapshot's coordinates.
    func backgroundPoints(_ background: AnnotationBackground) -> BackgroundPoints {
        let n = background.pointCount
        var points = BackgroundPoints(
            x: [Float](repeating: 0, count: n), y: [Float](repeating: 0, count: n),
            z: [Float](repeating: 0, count: n))
        rawBackground.withUnsafeBytes { buffer in
            var offset = Int(background.byteOffset)
            func readFloats(into array: inout [Float]) {
                for i in 0..<n {
                    let bits = buffer.loadUnaligned(fromByteOffset: offset, as: UInt32.self)
                    array[i] = Float(bitPattern: UInt32(littleEndian: bits))
                    offset += 4
                }
            }
            readFloats(into: &points.x)
            readFloats(into: &points.y)
            readFloats(into: &points.z)
        }
        return points
    }

    /// The canonical "sha256:" + lower hex form used throughout the pack, so a
    /// bare hash can never be mistaken for another algorithm's.
    static func digest(_ data: Data) -> String {
        let sum = SHA256.hash(data: data)
        return "sha256:" + sum.map { String(format: "%02x", $0) }.joined()
    }

    /// Drops one trailing newline, matching the writer's digest boundary.
    static func trimTrailingNewline(_ data: Data) -> Data {
        guard data.last == 0x0A else { return data }
        return data.dropLast()
    }

    private static func read(_ url: URL) throws -> Data {
        do { return try Data(contentsOf: url) } catch {
            throw AnnotationPackError.unreadable("\(url.lastPathComponent): \(error)")
        }
    }

    /// Decodes one sample's canonical arrays.
    func points(sampleID: Int) throws -> PackPoints {
        guard let sample = samples.first(where: { $0.sampleID == sampleID }) else {
            throw AnnotationPackError.sampleOutOfRange(sampleID)
        }
        let n = sample.pointCount
        let need = AnnotationPack.blockBytes(pointCount: n)
        let start = Int(sample.byteOffset)
        guard start >= 0, Int64(start) + need <= Int64(raw.count) else {
            throw AnnotationPackError.blockOutOfBounds(sampleID: sampleID)
        }

        var points = PackPoints(
            x: [Float](repeating: 0, count: n), y: [Float](repeating: 0, count: n),
            z: [Float](repeating: 0, count: n), intensity: [UInt8](repeating: 0, count: n),
            classification: [UInt8](repeating: 0, count: n))

        raw.withUnsafeBytes { buffer in
            var offset = start
            func readFloats(into array: inout [Float]) {
                for i in 0..<n {
                    let bits = buffer.loadUnaligned(fromByteOffset: offset, as: UInt32.self)
                    array[i] = Float(bitPattern: UInt32(littleEndian: bits))
                    offset += 4
                }
            }
            readFloats(into: &points.x)
            readFloats(into: &points.y)
            readFloats(into: &points.z)
            for i in 0..<n {
                points.intensity[i] = buffer.load(fromByteOffset: offset, as: UInt8.self)
                offset += 1
            }
            for i in 0..<n {
                points.classification[i] = buffer.load(fromByteOffset: offset, as: UInt8.self)
                offset += 1
            }
        }
        return points
    }

    /// Samples in display order: chronological, with the pack-local sample ID
    /// breaking a shared timestamp so the order is total and reproducible.
    var chronologicalSamples: [AnnotationSample] {
        samples.sorted {
            $0.timestampNs == $1.timestampNs
                ? $0.sampleID < $1.sampleID : $0.timestampNs < $1.timestampNs
        }
    }

    /// Human-readable coverage caveat for the annotation UI. A pack that only
    /// retained foreground returns cannot support whole-scene segmentation,
    /// and the operator has to see that before labelling against it.
    var coverageCaveat: String {
        switch manifest.coverage {
        case .full: return "Full scene: masks may claim every visible return."
        case .foregroundOnly:
            return "Foreground only: masks cover recorded foreground, not the whole scene."
        case .decimated: return "Decimated: masks cover the retained sample, not discarded returns."
        }
    }
}
