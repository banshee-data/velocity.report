import Foundation
import Testing

@testable import VelocityVisualiser

struct PhysicalTrackerSeedTests {
    private func fixture() throws -> (ReportInstant, ReportArmIdentity, AnnotationSample, String) {
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let bytes = try Data(
            contentsOf: root.appendingPathComponent(
                "internal/lidar/perframeeval/testdata/physical_report_fixture.json"))
        let envelope = try JSONSerialization.jsonObject(with: bytes) as! [String: Any]
        let physical = envelope["physical"] as! [String: Any]
        let reference = physical["reference"] as! [String: Any]
        let digest = reference["pack_digest"] as! String
        let report = try PhysicalComparisonReport.decode(bytes, packDigest: digest)
        var row = try #require(
            report.arms[0].instants.first { $0.prediction?.reference == "body_centre" })
        // The evaluator fixture deliberately tests nearest-time association.
        // Seed tests name an exact prediction; nearest matches are refused below.
        row.prediction!.timestampNs = row.timestampNs
        row.match!.offsetNs = 0
        let sample = AnnotationSample(
            sampleID: row.sampleID, sourceOrdinal: 10, sourceFrameID: 42,
            timestampNs: row.timestampNs, sensorID: "fixture", pointCount: 0, byteOffset: 0)
        return (row, report.arms[0].arm, sample, digest)
    }

    private func seed(
        _ row: ReportInstant, _ identity: ReportArmIdentity, _ sample: AnnotationSample,
        _ digest: String, reportDigest: String = "sha256:" + String(repeating: "a", count: 64),
        packDigest: String? = nil, objectID: String? = nil, author: String = "operator"
    ) throws -> PhysicalTrackerSeed {
        try PhysicalTrackerSeed.make(
            instant: row, identity: identity, reportDigest: reportDigest, reportPackDigest: digest,
            packDigest: packDigest ?? digest, objectID: objectID ?? row.objectID, sample: sample,
            author: author, session: "test")
    }

    private func eligible(
        _ row: ReportInstant, _ identity: ReportArmIdentity, _ sample: AnnotationSample,
        _ digest: String, reportDigest: String = "sha256:" + String(repeating: "a", count: 64),
        author: String = "operator"
    ) throws -> PhysicalTrackerSeed.Eligible {
        try PhysicalTrackerSeed.check(
            instant: row, identity: identity, reportDigest: reportDigest, reportPackDigest: digest,
            packDigest: digest, objectID: row.objectID, sample: sample, author: author)
    }

    @Test func theCheckAgreesWithTheSeedItWouldBuildAndMintsNothing() throws {
        let (row, identity, sample, digest) = try fixture()
        let checked = try eligible(row, identity, sample, digest)
        #expect(try eligible(row, identity, sample, digest) == checked, "a check minted something")
        let made = try seed(row, identity, sample, digest)
        #expect(checked.prediction == made.prediction && checked.sampleID == made.sampleID)
        #expect(
            try seed(row, identity, sample, digest).body.bodyID != made.body.bodyID,
            "make mints fresh records; the check must not need to")
        var variants: [ReportInstant] = []
        var v = row
        v.prediction!.reference = "cluster_medoid"
        variants.append(v)
        v = row
        v.prediction!.physical = false
        variants.append(v)
        v = row
        v.match!.candidates = 2
        variants.append(v)
        v = row
        v.prediction!.length!.metres = .nan
        variants.append(v)
        v = row
        v.prediction!.centreX = row.prediction!.x + 1
        variants.append(v)
        v = row
        v.prediction!.heading!.resolved = false
        variants.append(v)
        v = row
        v.prediction!.height = nil
        variants.append(v)
        for variant in variants {
            let offered = (try? eligible(variant, identity, sample, digest)) != nil
            let built = (try? seed(variant, identity, sample, digest)) != nil
            #expect(offered == built, "the pane and the import disagree")
        }
        #expect((try? eligible(row, identity, sample, digest, author: " ")) == nil)
        #expect((try? eligible(row, identity, sample, digest, reportDigest: "sha256:x")) == nil)
    }

    @Test func reportBodyIsAnAssistedSketchWithoutInventedBounds() throws {
        let (row, identity, sample, digest) = try fixture()
        let made = try seed(row, identity, sample, digest)
        #expect(made.keyframe.position.xM == row.prediction?.x)
        #expect(made.keyframe.position.yM == row.prediction?.y)
        #expect(made.body.length.valueM == row.prediction?.length?.metres)
        #expect(made.body.width.valueM == row.prediction?.width?.metres)
        #expect(made.body.length.status == .inferred && made.keyframe.position.status == .inferred)
        #expect(made.body.length.lowerM == nil && made.body.width.upperM == nil)
        #expect(made.keyframe.position.boundM == nil && made.keyframe.yaw.boundRad == nil)
        #expect(made.body.review.origin == .trackerAssisted)
        #expect(made.keyframe.review.status == .proposed)
        #expect(made.keyframe.review.trackerSource == made.source)
        #expect(made.source.contains("sha256:" + String(repeating: "a", count: 64)))
        #expect(made.source.contains("\(sample.timestampNs) ns"))
        let object = PhysicalObject(
            objectID: row.objectID, body: made.body, keyframes: [made.keyframe])
        #expect(
            PhysicalGeometry.derive(object: object, keyframe: made.keyframe, gate: .authoring).box
                != nil)
        #expect(
            PhysicalGeometry.derive(object: object, keyframe: made.keyframe, gate: .truth).box
                == nil)
        #expect(made.keyframe.front.status == .unknown && made.keyframe.rear.status == .unknown)
    }

    @Test func refusesWrongDomainAndUnavailablePointMeaning() throws {
        let (row, identity, sample, digest) = try fixture()
        #expect(throws: FeatureError.self) {
            try seed(row, identity, sample, digest, packDigest: "other")
        }
        #expect(throws: FeatureError.self) {
            try seed(row, identity, sample, digest, objectID: "other")
        }
        var otherSample = sample
        otherSample.timestampNs += 1
        #expect(throws: FeatureError.self) { try seed(row, identity, otherSample, digest) }
        otherSample = sample
        otherSample.sampleID += 1
        #expect(throws: FeatureError.self) { try seed(row, identity, otherSample, digest) }
        for meaning in [
            "cluster_medoid", "visible_obb_centre", "near_face_centre", "future_reference",
        ] {
            var other = row
            other.prediction!.reference = meaning
            #expect(throws: FeatureError.self) { try seed(other, identity, sample, digest) }
        }
        var other = row
        other.prediction!.physical = false
        #expect(throws: FeatureError.self) { try seed(other, identity, sample, digest) }
        other = row
        other.prediction!.timestampNs += 1
        #expect(throws: FeatureError.self) { try seed(other, identity, sample, digest) }
        other = row
        other.match!.offsetNs = 300_000
        #expect(throws: FeatureError.self) { try seed(other, identity, sample, digest) }
        other = row
        other.match!.candidates = 2
        #expect(throws: FeatureError.self) { try seed(other, identity, sample, digest) }
    }

    @Test func refusesIncompleteSourceAndInvalidGeometry() throws {
        let (row, identity, sample, digest) = try fixture()
        #expect(throws: FeatureError.self) {
            try seed(row, identity, sample, digest, reportDigest: "sha256:short")
        }
        #expect(throws: FeatureError.self) { try seed(row, identity, sample, digest, author: " ") }
        var unnamed = identity
        unnamed.estimatorID = nil
        #expect(throws: FeatureError.self) { try seed(row, unnamed, sample, digest) }
        for value in [Double.nan, .infinity, -1, 0] {
            var bad = row
            bad.prediction!.length!.metres = value
            #expect(throws: FeatureError.self) { try seed(bad, identity, sample, digest) }
        }
        var bad = row
        bad.prediction!.centreX = bad.prediction!.x + 1
        #expect(throws: FeatureError.self) { try seed(bad, identity, sample, digest) }
        bad = row
        bad.prediction!.heading!.resolved = false
        let made = try seed(bad, identity, sample, digest)
        #expect(made.keyframe.yaw.axis == .frontRearAmbiguous)
        bad = row
        bad.prediction!.height = nil
        #expect(try seed(bad, identity, sample, digest).body.height.status == .unknown)
    }
}
