//
//  AnnotationPackTests.swift
//  VelocityVisualiserTests
//
//  Cross-implementation tests for the annotation pack reader.
//
//  The fixture is internal/lidar/annotation/testdata/swift-pack-v2, the real
//  output of Go's internal/lidar/annotation.WritePack, read from the
//  repository. Go pins the same bytes (TestSwiftFixturePackBytesAreStable), so
//  a change to the writer fails there and names this file. Testing the decoder
//  against bytes this implementation did not produce is the whole point: a
//  self-consistent encoder/decoder pair can agree with each other and still
//  disagree with the writer that actually creates the packs an operator opens.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

// MARK: - Fixture

/// One Go-written pack: two samples of four and two points, each frame on a
/// packet of its own, foreground-only coverage.
enum PackFixture {
    static let directory = labelSetRepositoryRoot.appendingPathComponent(
        "internal/lidar/annotation/testdata/swift-pack-v2")
    static let fileNames = [
        "manifest.json", "samples.json", "points.bin", "packets.bin", "returns.bin",
    ]

    /// The digest Go's writer gave the fixture.
    static let packDigest =
        "sha256:e361672595fc28fb847b78dcd10b70aa9ecbb96fa44de2b9670bfd8af15e4cd3"

    /// One of the fixture's files, as Go wrote it.
    static func file(_ name: String) throws -> Data {
        try Data(contentsOf: directory.appendingPathComponent(name))
    }

    /// Copies the fixture into a folder of its own, or into `folder`, so a
    /// label set made from it lands beside it and in no other test's way.
    /// `mutate` can corrupt one file to exercise a rejection path. Remove the
    /// copy with `removePack`.
    static func write(
        in folder: URL? = nil, mutate: (inout [String: Data]) throws -> Void = { _ in }
    ) throws -> URL {
        let parent =
            folder
            ?? FileManager.default.temporaryDirectory.appendingPathComponent(
                "\(PackFixture.folderPrefix)\(UUID().uuidString)")
        let dir = parent.appendingPathComponent("pack-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        var files: [String: Data] = [:]
        for name in fileNames { files[name] = try file(name) }
        try mutate(&files)
        for (name, data) in files {
            try data.write(to: dir.appendingPathComponent(name))
        }
        return dir
    }

    /// The prefix of the folder a copy is written in, which `removePack`
    /// removes with it.
    static let folderPrefix = "pack-fixture-"

    /// The manifest as Go wrote it, with `edit` applied to its keys.
    static func manifest(_ edit: (inout [String: Any]) -> Void) throws -> Data {
        guard
            var object = try JSONSerialization.jsonObject(with: file("manifest.json"))
                as? [String: Any]
        else { throw CocoaError(.fileReadCorruptFile) }
        edit(&object)
        return try JSONSerialization.data(withJSONObject: object, options: [.sortedKeys])
    }
}

// MARK: - Tests

struct AnnotationPackTests {
    @Test func opensGoWrittenPackAndDecodesCanonicalArrays() throws {
        let dir = try PackFixture.write()
        defer { removePack(dir) }

        let pack = try AnnotationPack.open(directory: dir)

        #expect(pack.manifest.schemaVersion == 2)
        #expect(pack.manifest.datasetID == "ds_e361672595fc28fb")
        #expect(pack.manifest.coverage == .foregroundOnly)
        #expect(pack.manifest.sampleCount == 2)
        #expect(pack.manifest.pointCount == 6)
        #expect(pack.samples.count == 2)
        #expect(pack.manifest.packDigest == PackFixture.packDigest)
        #expect(AnnotationPack.packDigest(of: pack.manifest) == pack.manifest.packDigest)

        // Sample 0: a three-point cluster plus a far outlier.
        let first = try pack.points(sampleID: 0)
        #expect(first.count == 4)
        #expect(first.x == [1.0, 1.5, 2.0, 40.0])
        #expect(first.y == [1.0, 1.25, 1.5, -30.0])
        #expect(first.z == [0.5, 0.75, 1.0, 3.0])
        #expect(first.intensity == [10, 20, 30, 40])
        #expect(first.classification == [1, 1, 1, 2])

        // Sample 1 is read from a non-zero byte offset, so a decoder that
        // ignored ByteOffset would fail here rather than silently on sample 0.
        #expect(pack.samples[1].byteOffset == 56)
        let second = try pack.points(sampleID: 1)
        #expect(second.count == 2)
        #expect(second.x == [3.0, 3.5])
        #expect(second.y == [2.0, 2.5])
        #expect(second.z == [1.5, 1.75])
        #expect(second.intensity == [50, 60])
        #expect(second.x[1] == 3.5 && second.intensity[1] == 60)
    }

    @Test func readsGosPacketTableAndReturns() throws {
        let dir = try PackFixture.write()
        defer { removePack(dir) }

        let pack = try AnnotationPack.open(directory: dir)

        #expect(pack.manifest.returnLayout == .pandar40P)
        #expect(pack.manifest.packetCount == 2)
        #expect(pack.packets == [0x0123_4567_89ab_cdef, 0xfedc_ba98_7654_3210])
        #expect(try pack.returns(sampleID: 0) == [0, 41, 82, 399])
        #expect(try pack.returns(sampleID: 1) == [400, 445])
        // Go writes the frame range with omitempty: sample 0's frame starts
        // at return 0, so its samples.json entry has no return_start.
        let samplesJSON = try #require(String(data: PackFixture.file("samples.json"), encoding: .utf8))
        #expect(samplesJSON.components(separatedBy: "\"return_start\"").count == 2)
        #expect(pack.samples[0].returnStart == 0 && pack.samples[0].returnEnd == 400)
        #expect(pack.samples[1].returnStart == 400 && pack.samples[1].returnEnd == 800)
    }

    @Test func blockSizeMatchesTheWritersLayout() {
        // 4 points: three float32 arrays plus two uint8 arrays.
        #expect(AnnotationPack.blockBytes(pointCount: 4) == 56)
        #expect(AnnotationPack.blockBytes(pointCount: 0) == 0)
        #expect(AnnotationPack.blockBytes(pointCount: 2) == 28)
    }

    @Test func samplesDigestIgnoresTheWritersTrailingNewline() throws {
        // The writer appends a newline but digests the JSON without it. If the
        // reader digested the file as-is, every real pack would look corrupt.
        let withNewline = try PackFixture.file("samples.json")
        #expect(withNewline.last == 0x0A)
        let manifest = try JSONDecoder().decode(
            AnnotationManifest.self, from: PackFixture.file("manifest.json"))
        let trimmed = AnnotationPack.trimTrailingNewline(withNewline)
        #expect(AnnotationPack.digest(trimmed) == manifest.samplesSHA256)
        #expect(AnnotationPack.digest(withNewline) != manifest.samplesSHA256)
    }

    @Test func rejectsAlteredPointBytes() throws {
        // Flipping one coordinate byte must fail closed: a point index
        // recorded against different bytes is meaningless, not approximate.
        let dir = try PackFixture.write { files in
            var points = files["points.bin"]!
            points[0] ^= 0xFF
            files["points.bin"] = points
        }
        defer { removePack(dir) }

        do {
            _ = try AnnotationPack.open(directory: dir)
            Issue.record("expected a digest mismatch")
        } catch let error as AnnotationPackError {
            guard case .digestMismatch(let field, _, _) = error else {
                Issue.record("expected digestMismatch, got \(error)")
                return
            }
            #expect(field == "points.bin")
        }
    }

    @Test func rejectsAlteredPacketsOrReturns() throws {
        // A label set cites points through these, so they are held to the
        // same standard as the points.
        for name in ["packets.bin", "returns.bin"] {
            let dir = try PackFixture.write { files in
                var bytes = files[name]!
                bytes[0] ^= 0x01
                files[name] = bytes
            }
            defer { removePack(dir) }

            do {
                _ = try AnnotationPack.open(directory: dir)
                Issue.record("expected a digest mismatch on \(name)")
            } catch let error as AnnotationPackError {
                guard case .digestMismatch(let field, _, _) = error else {
                    Issue.record("expected digestMismatch, got \(error)")
                    continue
                }
                #expect(field == name)
            }
        }
    }

    @Test func rejectsAlteredSampleTable() throws {
        let dir = try PackFixture.write { files in
            // A plausible edit: repoint sample 1 at sample 0's block.
            let samples = String(data: files["samples.json"]!, encoding: .utf8)!
            files["samples.json"] = Data(
                samples.replacingOccurrences(of: "\"byte_offset\": 56", with: "\"byte_offset\": 0")
                    .utf8)
        }
        defer { removePack(dir) }

        do {
            _ = try AnnotationPack.open(directory: dir)
            Issue.record("expected a digest mismatch")
        } catch let error as AnnotationPackError {
            guard case .digestMismatch(let field, _, _) = error else {
                Issue.record("expected digestMismatch, got \(error)")
                return
            }
            #expect(field == "samples.json")
        }
    }

    @Test func rejectsAMatchedPairFromADifferentPack() throws {
        // Every array digest is self-consistent but the pack digest that
        // binds them is not, so a swapped matched pair cannot pass.
        let dir = try PackFixture.write { files in
            files["manifest.json"] = try PackFixture.manifest {
                $0["pack_digest"] = "sha256:" + String(repeating: "0", count: 64)
            }
        }
        defer { removePack(dir) }

        do {
            _ = try AnnotationPack.open(directory: dir)
            Issue.record("expected a pack digest mismatch")
        } catch let error as AnnotationPackError {
            guard case .digestMismatch(let field, _, _) = error else {
                Issue.record("expected digestMismatch, got \(error)")
                return
            }
            #expect(field == "pack_digest")
        }
    }

    /// A version 1 pack as Go's old writer left it: no packet table, no
    /// returns, and a manifest without their fields.
    @Test func refusesAVersion1PackByName() throws {
        let dir = try PackFixture.write { files in
            files["manifest.json"] = try PackFixture.manifest {
                $0["schema_version"] = 1
                for key in [
                    "return_layout", "packet_key", "packet_count", "packets_sha256",
                    "returns_sha256",
                ] { $0.removeValue(forKey: key) }
            }
            files.removeValue(forKey: "packets.bin")
            files.removeValue(forKey: "returns.bin")
        }
        defer { removePack(dir) }

        do {
            _ = try AnnotationPack.open(directory: dir)
            Issue.record("expected a version 1 pack to be refused")
        } catch let error as AnnotationPackError {
            #expect(error == .version1Pack)
            let message = "\(error)"
            #expect(message.contains("git tag annotation-v1-reader"))
            #expect(message.contains("docs/lidar/operations/annotation-label-set-migration.md"))
        }
    }

    @Test func refusesAnUnsupportedSchema() throws {
        let dir = try PackFixture.write { files in
            files["manifest.json"] = try PackFixture.manifest { $0["schema_version"] = 99 }
        }
        defer { removePack(dir) }

        do {
            _ = try AnnotationPack.open(directory: dir)
            Issue.record("expected an unsupported schema error")
        } catch let error as AnnotationPackError {
            #expect(error == .unsupportedSchema(99))
        }
    }

    @Test func missingFileReportsWhichOne() throws {
        for name in ["points.bin", "returns.bin"] {
            let dir = try PackFixture.write { files in files.removeValue(forKey: name) }
            defer { removePack(dir) }

            do {
                _ = try AnnotationPack.open(directory: dir)
                Issue.record("expected an unreadable error for \(name)")
            } catch let error as AnnotationPackError {
                guard case .unreadable(let detail) = error else {
                    Issue.record("expected unreadable, got \(error)")
                    continue
                }
                #expect(detail.contains(name))
            }
        }
    }

    @Test func unknownSampleIsRejectedRatherThanClamped() throws {
        let dir = try PackFixture.write()
        defer { removePack(dir) }
        let pack = try AnnotationPack.open(directory: dir)

        do {
            _ = try pack.points(sampleID: 7)
            Issue.record("expected sampleOutOfRange")
        } catch let error as AnnotationPackError {
            #expect(error == .sampleOutOfRange(7))
        }
        #expect(throws: AnnotationPackError.sampleOutOfRange(7)) {
            try pack.returns(sampleID: 7)
        }
    }

    @Test func chronologicalOrderIsTotalAndReproducible() throws {
        let dir = try PackFixture.write()
        defer { removePack(dir) }
        let pack = try AnnotationPack.open(directory: dir)

        let ordered = pack.chronologicalSamples
        #expect(ordered.map(\.sampleID) == [0, 1])
        // Same input, same order: playback and review must not reshuffle.
        #expect(pack.chronologicalSamples.map(\.sampleID) == ordered.map(\.sampleID))
    }

    @Test func coverageCaveatNamesTheSourcesLimit() throws {
        let dir = try PackFixture.write()
        defer { removePack(dir) }
        let pack = try AnnotationPack.open(directory: dir)

        // A foreground-only pack cannot support whole-scene segmentation, and
        // the operator has to be told before labelling against it.
        #expect(pack.coverageCaveat.contains("Foreground only"))
    }

    @Test func pointAccessorRefusesOutOfRangeIndices() {
        let points = PackPoints(x: [1, 2], y: [3, 4], z: [5, 6])
        #expect(points.point(at: 0) == simd_float3(1, 3, 5))
        #expect(points.point(at: 1) == simd_float3(2, 4, 6))
        #expect(points.point(at: 2) == nil)
        #expect(points.point(at: -1) == nil)
    }
}
