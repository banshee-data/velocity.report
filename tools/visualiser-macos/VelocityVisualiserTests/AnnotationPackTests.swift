//
//  AnnotationPackTests.swift
//  VelocityVisualiserTests
//
//  Cross-implementation tests for the annotation pack reader.
//
//  The fixture bytes below are the real output of Go's
//  internal/lidar/annotation.WritePack, captured verbatim. They are pinned on
//  the Go side too, by TestSwiftFixturePackBytesAreStable, so a change to the
//  writer fails there and names this file. Testing the decoder against bytes
//  this implementation did not produce is the whole point: a self-consistent
//  encoder/decoder pair can agree with each other and still disagree with the
//  writer that actually creates the packs an operator opens.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

// MARK: - Fixture

/// One Go-written pack: two samples, six points, foreground-only coverage.
enum PackFixture {
    /// points.bin, verbatim.
    static let pointsBase64 =
        "AACAPwAAwD8AAABAAAAgQgAAgD8AAKA/AADAPwAA8MEAAAA/AABAPwAAgD8AAEBAChQeKAEBAQIAAEBAAABgQAAAAEAAACBAAADAPwAA4D8yPAEB"

    static let pointsSHA =
        "sha256:f8bf067228e595dfee12c0cd3d974db11ebe5474f30e6e08fafef9da47211186"
    static let samplesSHA =
        "sha256:38e62501b09464832baa1bcdbcc21b3f58f2b77e58327df59cbda66109654410"
    static let packDigest =
        "sha256:41b9794747f16ebe2722ea9f75a631f38e35f9670e14dccc3636ed480320d84c"

    /// samples.json, verbatim including the writer's trailing newline: the
    /// digest is taken before that newline, which is exactly the boundary a
    /// naive reader gets wrong.
    static let samplesJSON = """
        [
          {
            "sample_id": 0,
            "source_ordinal": 0,
            "source_frame_id": 100,
            "timestamp_ns": 1000000000,
            "sensor_id": "hesai-pandar40p",
            "point_count": 4,
            "byte_offset": 0
          },
          {
            "sample_id": 1,
            "source_ordinal": 1,
            "source_frame_id": 101,
            "timestamp_ns": 1100000000,
            "sensor_id": "hesai-pandar40p",
            "point_count": 2,
            "byte_offset": 56
          }
        ]

        """

    static let manifestJSON = """
        {
          "schema_version": 1,
          "dataset_id": "ds_41b9794747f16ebe",
          "created_ns": 0,
          "source": {
            "vrlog_path": "fixture.vrlog",
            "vrlog_header_sha256": "sha256:aa",
            "vrlog_frames_sha256": "sha256:bb",
            "sensor_id": "hesai-pandar40p",
            "build_version": "fixture"
          },
          "coordinate": {
            "units": "metres",
            "frame_id": "sensor",
            "reference_frame": "site",
            "handedness": "right",
            "origin_note": "sensor origin",
            "transform_version": "v1"
          },
          "coverage": "foreground_only",
          "coverage_note": "fixture",
          "sample_count": 2,
          "point_count": 6,
          "points_sha256": "\(pointsSHA)",
          "samples_sha256": "\(samplesSHA)",
          "pack_digest": "\(packDigest)",
          "has_intensity": true,
          "has_classification": true,
          "completeness": {
            "requested_start_ns": 0,
            "requested_end_ns": 0,
            "actual_start_ns": 0,
            "actual_end_ns": 0,
            "frames_without_points": 0,
            "duplicate_timestamps": 0,
            "max_timestamp_gap_ns": 0
          }
        }

        """

    /// Writes the fixture to a fresh temporary directory.
    /// `mutate` can corrupt one file to exercise a rejection path.
    static func write(
        mutate: (inout [String: Data]) -> Void = { _ in }
    ) throws -> URL {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent("pack-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)

        var files: [String: Data] = [
            "manifest.json": Data(manifestJSON.utf8),
            "samples.json": Data(samplesJSON.utf8),
            "points.bin": Data(base64Encoded: pointsBase64)!,
        ]
        mutate(&files)
        for (name, data) in files {
            try data.write(to: dir.appendingPathComponent(name))
        }
        return dir
    }
}

// MARK: - Tests

struct AnnotationPackTests {
    @Test func opensGoWrittenPackAndDecodesCanonicalArrays() throws {
        let dir = try PackFixture.write()
        defer { try? FileManager.default.removeItem(at: dir) }

        let pack = try AnnotationPack.open(directory: dir)

        #expect(pack.manifest.schemaVersion == 1)
        #expect(pack.manifest.datasetID == "ds_41b9794747f16ebe")
        #expect(pack.manifest.coverage == .foregroundOnly)
        #expect(pack.manifest.sampleCount == 2)
        #expect(pack.manifest.pointCount == 6)
        #expect(pack.samples.count == 2)

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
        let second = try pack.points(sampleID: 1)
        #expect(second.count == 2)
        #expect(second.x == [3.0, 3.5])
        #expect(second.y == [2.0, 2.5])
        #expect(second.z == [1.5, 1.75])
        #expect(second.intensity == [50, 60])
    }

    @Test func blockSizeMatchesTheWritersLayout() {
        // 4 points: three float32 arrays plus two uint8 arrays.
        #expect(AnnotationPack.blockBytes(pointCount: 4) == 56)
        #expect(AnnotationPack.blockBytes(pointCount: 0) == 0)
        #expect(AnnotationPack.blockBytes(pointCount: 2) == 28)
    }

    @Test func samplesDigestIgnoresTheWritersTrailingNewline() {
        // The writer appends a newline but digests the JSON without it. If the
        // reader digested the file as-is, every real pack would look corrupt.
        let withNewline = Data(PackFixture.samplesJSON.utf8)
        #expect(withNewline.last == 0x0A)
        let trimmed = AnnotationPack.trimTrailingNewline(withNewline)
        #expect(AnnotationPack.digest(trimmed) == PackFixture.samplesSHA)
        #expect(AnnotationPack.digest(withNewline) != PackFixture.samplesSHA)
    }

    @Test func rejectsAlteredPointBytes() throws {
        // Flipping one coordinate byte must fail closed: a point index
        // recorded against different bytes is meaningless, not approximate.
        let dir = try PackFixture.write { files in
            var points = files["points.bin"]!
            points[0] ^= 0xFF
            files["points.bin"] = points
        }
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(throws: AnnotationPackError.self) {
            _ = try AnnotationPack.open(directory: dir)
        }
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

    @Test func rejectsAlteredSampleTable() throws {
        let dir = try PackFixture.write { files in
            // A plausible edit: repoint sample 1 at sample 0's block.
            let tampered = PackFixture.samplesJSON.replacingOccurrences(
                of: "\"byte_offset\": 56", with: "\"byte_offset\": 0")
            files["samples.json"] = Data(tampered.utf8)
        }
        defer { try? FileManager.default.removeItem(at: dir) }

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
        // Both array digests are self-consistent but the pack digest that
        // binds them is not, so a swapped matched pair cannot pass.
        let dir = try PackFixture.write { files in
            let tampered = PackFixture.manifestJSON.replacingOccurrences(
                of: PackFixture.packDigest, with: "sha256:" + String(repeating: "0", count: 64))
            files["manifest.json"] = Data(tampered.utf8)
        }
        defer { try? FileManager.default.removeItem(at: dir) }

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

    @Test func refusesAnUnsupportedSchema() throws {
        let dir = try PackFixture.write { files in
            let tampered = PackFixture.manifestJSON.replacingOccurrences(
                of: "\"schema_version\": 1", with: "\"schema_version\": 99")
            files["manifest.json"] = Data(tampered.utf8)
        }
        defer { try? FileManager.default.removeItem(at: dir) }

        do {
            _ = try AnnotationPack.open(directory: dir)
            Issue.record("expected an unsupported schema error")
        } catch let error as AnnotationPackError {
            #expect(error == .unsupportedSchema(99))
        }
    }

    @Test func missingFileReportsWhichOne() throws {
        let dir = try PackFixture.write { files in
            files.removeValue(forKey: "points.bin")
        }
        defer { try? FileManager.default.removeItem(at: dir) }

        do {
            _ = try AnnotationPack.open(directory: dir)
            Issue.record("expected an unreadable error")
        } catch let error as AnnotationPackError {
            guard case .unreadable(let detail) = error else {
                Issue.record("expected unreadable, got \(error)")
                return
            }
            #expect(detail.contains("points.bin"))
        }
    }

    @Test func unknownSampleIsRejectedRatherThanClamped() throws {
        let dir = try PackFixture.write()
        defer { try? FileManager.default.removeItem(at: dir) }
        let pack = try AnnotationPack.open(directory: dir)

        do {
            _ = try pack.points(sampleID: 7)
            Issue.record("expected sampleOutOfRange")
        } catch let error as AnnotationPackError {
            #expect(error == .sampleOutOfRange(7))
        }
    }

    @Test func chronologicalOrderIsTotalAndReproducible() throws {
        let dir = try PackFixture.write()
        defer { try? FileManager.default.removeItem(at: dir) }
        let pack = try AnnotationPack.open(directory: dir)

        let ordered = pack.chronologicalSamples
        #expect(ordered.map(\.sampleID) == [0, 1])
        // Same input, same order: playback and review must not reshuffle.
        #expect(pack.chronologicalSamples.map(\.sampleID) == ordered.map(\.sampleID))
    }

    @Test func coverageCaveatNamesTheSourcesLimit() throws {
        let dir = try PackFixture.write()
        defer { try? FileManager.default.removeItem(at: dir) }
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
