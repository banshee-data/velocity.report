import Foundation
import SwiftProtobuf
import Testing
import simd

@testable import VelocityVisualiser

struct FacetBodyRegistrationTests {
    private func fixture() -> (FeatureCandidate, PackPoints, PhysicalPackState) {
        var feature = FeatureCandidate()
        feature.featureID = "mirror"
        feature.objectID = "car"
        feature.geometry = .protrusion
        for sample: UInt32 in [0, 1] {
            var o = FeatureObservation()
            o.sampleID = sample
            o.decision = .acceptedProposal
            o.pointIndices = [0]
            o.membershipDigest = "sha256:members"
            feature.observations.append(o)
        }
        let points = PackPoints(x: [8], y: [-0.9], z: [1])
        var body = PhysicalBody(bodyID: "body")
        body.review.status = .reviewed
        body.review.origin = .independent
        body.review.reviewedAgainst = PhysicalMembershipPin(revision: 1, digest: "sha256:members")
        var k = PhysicalKeyframe(keyframeID: "pose", sampleID: 0, timestampNs: 10)
        k.position = PhysicalPosition(status: .observed, xM: 10, yM: 0, boundM: 0.2)
        k.yaw = PhysicalYaw(status: .observed, axis: .resolved, yawRad: .pi / 2, boundRad: 0.05)
        k.review = body.review
        let fake = FakePhysicalService(packDir: "/tmp/pack")
        fake.digest = "sha256:physical"
        fake.objects = [PhysicalObject(objectID: "car", body: body, keyframes: [k])]
        return (feature, points, fake.state())
    }

    @Test func horizontalCoordinatesUseThePinnedMetricBodyFrameWithConservativeBounds() throws {
        let (feature, points, physical) = fixture()
        let anchor = try FacetBodyRegistration.make(
            feature: feature, sampleID: 0, pointIndex: 0, points: points, physical: physical,
            returnBoundM: 0.05, identityNote: "outer mirror tip")
        #expect(abs(anchor.xM + 0.9) < 1e-6 && abs(anchor.yM - 2) < 1e-6)
        #expect(abs(anchor.boundM - (0.25 + hypot(2, 0.9) * 2 * sin(0.025))) < 1e-6)
        #expect(anchor.hasSourcePointIndex && anchor.sourcePointIndex == 0)
        #expect(anchor.coordinateDomain == "body_xy" && anchor.zM == 0)
        #expect(anchor.partFrameID == "body_xy" && anchor.physicalDigest == "sha256:physical")
        #expect(anchor.origin == "reference_seeded_proposal")
        var changed = physical
        changed.document.objects[0].body?.length = PhysicalDimension(
            status: .inferred, span: .full, lowerM: 4, upperM: 6)
        let again = try FacetBodyRegistration.make(
            feature: feature, sampleID: 0, pointIndex: 0, points: points, physical: changed,
            returnBoundM: 0.05, identityNote: "outer mirror tip")
        #expect(
            again.xM == anchor.xM && again.yM == anchor.yM, "size edit rescaled the metric offset")
        #expect(try FacetBodyAnchor(serializedBytes: anchor.serializedData()) == anchor)
    }

    @Test func aPatchASparseFeatureAnUnreviewedPoseAndMissingBoundsCannotRegister() throws {
        let (original, points, physical) = fixture()
        var patch = original
        patch.geometry = .patch
        var sparse = original
        sparse.observations.removeLast()
        for f in [patch, sparse] {
            #expect(throws: (any Error).self) {
                try FacetBodyRegistration.make(
                    feature: f, sampleID: 0, pointIndex: 0, points: points, physical: physical,
                    returnBoundM: 0.05, identityNote: "tip")
            }
        }
        for bound in [0, -1, Double.nan, Double.infinity] {
            #expect(throws: (any Error).self) {
                try FacetBodyRegistration.make(
                    feature: original, sampleID: 0, pointIndex: 0, points: points,
                    physical: physical, returnBoundM: bound, identityNote: "tip")
            }
        }
        var unreviewed = physical
        unreviewed.document.objects[0].keyframes[0].review.status = .proposed
        var ambiguous = physical
        ambiguous.document.objects[0].keyframes[0].yaw.axis = .frontRearAmbiguous
        var assisted = physical
        assisted.document.objects[0].body?.review.origin = .trackerAssisted
        var stale = physical
        stale.document.objects[0].keyframes[0].review.reviewedAgainst?.digest = "other"
        for state in [unreviewed, ambiguous, assisted, stale] {
            #expect(throws: (any Error).self) {
                try FacetBodyRegistration.make(
                    feature: original, sampleID: 0, pointIndex: 0, points: points, physical: state,
                    returnBoundM: 0.05, identityNote: "tip")
            }
        }
        // A reviewed assisted relation is usable only with its assistance
        // named and carried into the registration proposal's origin.
        assisted.document.objects[0].body?.review.trackerSource = "online seed"
        let assistedAnchor = try FacetBodyRegistration.make(
            feature: original, sampleID: 0, pointIndex: 0, points: points, physical: assisted,
            returnBoundM: 0.05, identityNote: "tip")
        #expect(assistedAnchor.origin == "tracker_seeded_proposal")
        #expect(abs(assistedAnchor.xM + 0.9) < 1e-6 && abs(assistedAnchor.yM - 2) < 1e-6)
        #expect(throws: (any Error).self) {
            try FacetBodyRegistration.make(
                feature: original, sampleID: -1, pointIndex: 0, points: points, physical: physical,
                returnBoundM: 0.05, identityNote: "tip")
        }
    }
}

struct FacetBodyRegistrationWireTests {
    @Test func theGoSharedFixturePinsMetricCoordinatesAndOptionalZero() throws {
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let bytes = try Data(
            contentsOf: root.appendingPathComponent(
                "proto/velocity_recording/v1/testdata/body-registration.pb"))
        let doc = try FeatureDocument(serializedBytes: bytes)
        let anchor = try #require(doc.features.first?.anchor)
        #expect(anchor.hasSourcePointIndex && anchor.sourcePointIndex == 0)
        #expect(anchor.xM == 1.25 && anchor.yM == -0.875 && anchor.coordinateDomain == "body_xy")
        #expect(anchor.boundM == 0.35 && anchor.returnBoundM == 0.05 && anchor.zM == 0)
        #expect(anchor.physicalRevision == 7 && anchor.partFrameRevision == 1)
        #expect(anchor.origin == "reference_seeded_proposal" && doc.features[1].inactive)
        #expect(doc.features[0].observations[0].timestampNs == 9_007_199_254_740_993)
        #expect(doc.knownEditingContract)
        #expect(try doc.serializedData() == bytes)
    }
}
