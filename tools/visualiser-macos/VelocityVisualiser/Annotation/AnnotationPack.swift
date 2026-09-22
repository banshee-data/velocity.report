// AnnotationPack.swift
// Reader for the immutable point packs written by Go's internal/lidar/annotation.
//
// A pack is the annotation source of truth: manifest.json names the point
// domain and its digests, samples.json locates each frame's block, and
// points.bin holds the canonical little-endian arrays. Nothing here writes to
// a pack. A pack whose bytes disagree with its manifest is refused rather than
// opened partially, because a point index only means anything under the exact
// digest it was recorded against.

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
    var buildVersion: String?
    /// The run's L4 height-band filter, when the exporter knew it. Absent
    /// from packs cut before it was recorded.
    var heightBand: HeightBand?

    enum CodingKeys: String, CodingKey {
        case vrlogPath = "vrlog_path"
        case vrlogHeaderSHA = "vrlog_header_sha256"
        case vrlogFramesSHA = "vrlog_frames_sha256"
        case pcapBasename = "pcap_basename"
        case sensorID = "sensor_id"
        case buildVersion = "build_version"
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

    enum CodingKeys: String, CodingKey {
        case sampleID = "sample_id"
        case sourceOrdinal = "source_ordinal"
        case sourceFrameID = "source_frame_id"
        case timestampNs = "timestamp_ns"
        case sensorID = "sensor_id"
        case pointCount = "point_count"
        case byteOffset = "byte_offset"
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

enum AnnotationPackError: Error, Equatable {
    case unreadable(String)
    case malformed(String)
    case digestMismatch(field: String, expected: String, actual: String)
    case unsupportedSchema(Int)
    case sampleOutOfRange(Int)
    case blockOutOfBounds(sampleID: Int)
}

/// An opened, digest-verified annotation pack.
final class AnnotationPack {
    static let supportedSchemaVersion = 1

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

    private let raw: Data
    private let rawBackground: Data

    private init(
        directory: URL, manifest: AnnotationManifest, samples: [AnnotationSample], raw: Data,
        backgrounds: [AnnotationBackground], rawBackground: Data
    ) {
        self.directory = directory
        self.manifest = manifest
        self.samples = samples
        self.raw = raw
        self.backgrounds = backgrounds
        self.rawBackground = rawBackground
    }

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
        guard manifest.schemaVersion == supportedSchemaVersion else {
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
        // The pack digest binds the two array digests together, so swapping a
        // matched pair of files cannot pass as the same pack.
        let packDigest = digest(Data((manifest.pointsSHA256 + "\n" + manifest.samplesSHA256).utf8))
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

        let (backgrounds, backgroundData) = try openBackground(
            directory: directory, manifest: manifest, decoder: decoder)
        return AnnotationPack(
            directory: directory, manifest: manifest, samples: samples, raw: pointsData,
            backgrounds: backgrounds, rawBackground: backgroundData)
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
