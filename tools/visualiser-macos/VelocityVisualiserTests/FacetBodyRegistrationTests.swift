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

extension FacetBodyRegistrationTests {
    @Test func sharedSegmentWireFixtureKeepsUnresolvedTangentAndOptionalZero() throws {
        let repo = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let bytes = try Data(
            contentsOf: repo.appendingPathComponent(
                "proto/velocity_recording/v1/testdata/segment-registration.pb"))
        let doc = try FeatureDocument(serializedBytes: bytes)
        let facet = try #require(doc.features.first)
        #expect(
            facet.geometry == .edge && facet.anchor.hasLine && !facet.anchor.hasSourcePointIndex)
        #expect(facet.anchor.xM == 0 && facet.anchor.yM == 0 && facet.anchor.zM == 0)
        #expect(facet.anchor.line.hasSourceStartIndex && facet.anchor.line.sourceStartIndex == 0)
        #expect(facet.anchor.line.sourceEndIndex == 2 && facet.anchor.line.offsetM == -0.875)
        #expect(
            facet.anchor.line.normalBoundRad == 0.1
                && facet.observations[0].timestampNs == 9_007_199_254_740_993)
        #expect(doc.features[1].inactive && doc.knownEditingContract)
        #expect(try doc.serializedData() == bytes)
    }

    private func segmentFixture() -> (FeatureCandidate, PackPoints, PhysicalPackState) {
        var (feature, _, physical) = fixture()
        feature.geometry = .edge
        for i in feature.observations.indices { feature.observations[i].pointIndices = [0, 1, 2] }
        return (
            feature, PackPoints(x: [8, 9, 10], y: [-0.9, -0.9, -0.9], z: [1, 1, 1]), physical
        )
    }

    @Test func segmentConstrainsNormalAndOrientationWhileLeavingTangentUnresolved() throws {
        let (feature, points, physical) = segmentFixture()
        let anchor = try FacetBodyRegistration.makeSegment(
            feature: feature, sampleID: 0, startIndex: 0, endIndex: 2, points: points,
            physical: physical, returnBoundM: 0.05,
            identityNote: "straight physical sill edge, checked in both frames")
        #expect(anchor.hasLine && !anchor.hasSourcePointIndex && anchor.xM == 0 && anchor.yM == 0)
        #expect(anchor.method == "manual_named_segment_v1" && anchor.coordinateDomain == "body_xy")
        #expect(anchor.line.hasSourceStartIndex && anchor.line.sourceStartIndex == 0)
        #expect(anchor.line.hasSourceEndIndex && anchor.line.sourceEndIndex == 2)
        #expect(abs(anchor.line.normalX - 1) < 1e-9 && abs(anchor.line.normalY) < 1e-9)
        #expect(abs(anchor.line.offsetM - Double(Float(-0.9))) < 1e-9)
        let angular = 0.05 + asin(0.05)
        let minimum = 0.25 + (hypot(1, Double(Float(-0.9))) + 0.25) * 2 * sin(angular / 2)
        #expect(
            abs(anchor.line.normalBoundRad - angular) < 1e-9 && abs(anchor.boundM - minimum) < 1e-9)
        let reversed = try FacetBodyRegistration.makeSegment(
            feature: feature, sampleID: 0, startIndex: 2, endIndex: 0, points: points,
            physical: physical, returnBoundM: 0.05, identityNote: "same physical edge")
        #expect(abs(reversed.line.normalX - anchor.line.normalX) < 1e-9)
        #expect(abs(reversed.line.offsetM - anchor.line.offsetM) < 1e-9)
        let shifted = PackPoints(x: [10, 11, 12], y: points.y, z: points.z)
        let laterSupport = try FacetBodyRegistration.makeSegment(
            feature: feature, sampleID: 0, startIndex: 0, endIndex: 2, points: shifted,
            physical: physical, returnBoundM: 0.05,
            identityNote: "fresh returns farther along the same physical edge")
        #expect(abs(laterSupport.line.offsetM - anchor.line.offsetM) < 1e-9)
        #expect(laterSupport.xM == 0 && laterSupport.yM == 0)
        let bytes = try anchor.serializedData()
        #expect(try FacetBodyAnchor(serializedBytes: bytes) == anchor)
    }

    @Test func segmentStopsForSparseCurvedVerticalAndUncertainSupport() {
        let (feature, points, physical) = segmentFixture()
        for name in [
            "same", "missing", "sparse", "duplicate", "nonfinite", "curve", "vertical", "short",
            "large bound", "wide yaw", "wrong type", "one frame",
        ] {
            var f = feature
            var p = points
            var state = physical
            var end: UInt32 = 2
            var bound = 0.05
            switch name {
            case "same": end = 0
            case "missing": end = 99
            case "sparse": f.observations[0].pointIndices = [0, 2]
            case "duplicate": p = PackPoints(x: [8, 8, 10], y: points.y, z: points.z)
            case "nonfinite": p = PackPoints(x: points.x, y: points.y, z: [1, .nan, 1])
            case "curve": p = PackPoints(x: points.x, y: [-0.9, 0, -0.9], z: points.z)
            case "vertical": p = PackPoints(x: [8, 8, 8], y: points.y, z: [1, 2, 3])
            case "short": p = PackPoints(x: [8, 8.02, 8.04], y: points.y, z: points.z)
            case "large bound": bound = 1
            case "wide yaw": state.document.objects[0].keyframes[0].yaw.boundRad = .pi / 2
            case "wrong type": f.geometry = .patch
            default: f.observations.removeLast()
            }
            #expect(throws: (any Error).self, Comment(rawValue: name)) {
                try FacetBodyRegistration.makeSegment(
                    feature: f, sampleID: 0, startIndex: 0, endIndex: end, points: p,
                    physical: state, returnBoundM: bound, identityNote: "physical edge")
            }
        }
    }

    @Test func segmentKeepsAssistedOriginAndDoesNotRescaleWithDimensions() throws {
        var (feature, points, physical) = segmentFixture()
        physical.document.objects[0].body?.review.origin = .trackerAssisted
        physical.document.objects[0].body?.review.trackerSource =
            "named report/estimator/physical seed"
        let first = try FacetBodyRegistration.makeSegment(
            feature: feature, sampleID: 0, startIndex: 0, endIndex: 2, points: points,
            physical: physical, returnBoundM: 0.05, identityNote: "same sill edge")
        #expect(first.origin == "tracker_seeded_proposal")
        physical.document.objects[0].body?.length.valueM = 100
        let second = try FacetBodyRegistration.makeSegment(
            feature: feature, sampleID: 0, startIndex: 0, endIndex: 2, points: points,
            physical: physical, returnBoundM: 0.05, identityNote: "same sill edge")
        #expect(second.line == first.line && second.boundM == first.boundM)
        var document = FeatureDocument()
        feature.partRelation = "rigid_proposal"
        feature.anchor = first
        document.features = [feature]
        #expect(document.knownEditingContract)
        document.features[0].anchor.line = try Velocity_Recording_V1_FeatureLineConstraint(
            serializedBytes: first.line.serializedData() + Data([0x78, 1]))
        #expect(!document.knownEditingContract)
        document.features[0].anchor.line = first.line
        // A future method must not be erased by editing an older contract.
        document.features[0].anchor.method = "future_segment_method"
        #expect(!document.knownEditingContract)
    }
}
