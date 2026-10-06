import Foundation
import Testing
import simd

@testable import VelocityVisualiser

struct FacetBodyProjectionTests {
    private func fixture() -> (FeatureCandidate, PhysicalPackState) {
        var feature = FeatureCandidate()
        feature.featureID = "mirror"
        feature.objectID = "car"
        feature.geometry = .protrusion
        feature.partRelation = "rigid_proposal"
        var a = FacetBodyAnchor()
        a.bodyID = "body"
        a.partFrameID = "body_xy"
        a.partFrameRevision = 1
        a.coordinateDomain = "body_xy"
        a.origin = "reference_seeded_proposal"
        a.method = "manual_named_return_v1"
        a.sourcePointIndex = 0
        a.xM = 2
        a.yM = -1
        a.boundM = 0.3
        feature.anchor = a
        var body = PhysicalBody(bodyID: "body")
        body.review.status = .reviewed
        body.review.origin = .independent
        body.review.reviewedAgainst = PhysicalMembershipPin(revision: 1, digest: "sha256:members")
        var pose = PhysicalKeyframe(keyframeID: "pose", sampleID: 2, timestampNs: 300)
        pose.position = PhysicalPosition(status: .observed, xM: 10, yM: 5, boundM: 0.2)
        pose.yaw = PhysicalYaw(status: .observed, axis: .resolved, yawRad: .pi / 2, boundRad: 0.05)
        pose.review = body.review
        let service = FakePhysicalService(packDir: "/tmp/pack")
        service.digest = "sha256:physical"
        service.membershipDigest = "sha256:members"
        service.objects = [PhysicalObject(objectID: "car", body: body, keyframes: [pose])]
        return (feature, service.state())
    }
    private func project(
        _ feature: FeatureCandidate, _ physical: PhysicalPackState
    ) throws -> FacetBodyProjection {
        try FacetBodyProjection.make(
            feature: feature, sampleID: 2, timestampNs: 300, packDigest: "sha256:pack",
            physical: physical, membershipDigest: "sha256:members")
    }
    @Test func compactMappingRotatesAndTranslatesWithoutRefittingReturns() throws {
        let (feature, state) = fixture()
        let p = try project(feature, state)
        #expect(simd_distance(p.origin, SIMD2(11, 7)) < 1e-10)
        #expect(abs(p.boundM - (0.5 + sqrt(5) * PhysicalGeometry.swing(0.05))) < 1e-10)
        #expect(!p.assisted && !p.isLine)
        let residuals = p.residuals(
            points: PackPoints(x: [11, 11, .nan], y: [7, 8, 0], z: [0, 0, 0]),
            indices: [0, 1, 1, 2, 99])
        #expect(
            residuals.count == 2 && residuals.meanAbsoluteM == 0.5
                && residuals.maximumAbsoluteM == 1)
        var resized = state
        resized.document.objects[0].body?.length = PhysicalDimension(
            status: .inferred, span: .full, lowerM: 4, upperM: 6)
        #expect(try project(feature, resized) == p)
    }
    @Test func lineKeepsItsTangentWeakAndAddsTargetYawUncertainty() throws {
        var (feature, state) = fixture()
        feature.geometry = .edge
        feature.anchor.clearSourcePointIndex()
        feature.anchor.xM = 0
        feature.anchor.yM = 0
        feature.anchor.method = "manual_named_segment_v1"
        feature.anchor.line.normalX = 1
        feature.anchor.line.offsetM = 2
        feature.anchor.line.normalBoundRad = 0.1
        let p = try project(feature, state)
        #expect(p.isLine && abs((p.normalBoundRad ?? 0) - 0.15) < 1e-10)
        #expect(abs(p.residual(point: SIMD2(-100, 7))) < 1e-10)
        #expect(abs(p.residual(point: SIMD2(200, 8)) - 1) < 1e-10)
        feature.anchor.line.normalBoundRad = .pi / 2
        #expect(throws: (any Error).self) { try project(feature, state) }
    }
    @Test func staleWrongBodyUnsupportedPoseAndWrongFrameAreWithheld() throws {
        let (feature, original) = fixture()
        var variants: [PhysicalPackState] = []
        var state = original
        state.document.objects[0].body?.bodyID = "new-body"
        variants.append(state)
        state = original
        state.document.objects[0].keyframes[0].timestampNs = 301
        variants.append(state)
        state = original
        state.document.objects[0].keyframes[0].review.status = .proposed
        variants.append(state)
        state = original
        state.document.objects[0].keyframes[0].position.boundM = nil
        variants.append(state)
        state = original
        state.document.objects[0].keyframes[0].yaw.axis = .frontRearAmbiguous
        variants.append(state)
        state = original
        state.membershipDigest = "other"
        variants.append(state)
        state = original
        state.document.packDigest = "other"
        variants.append(state)
        state = original
        state.document.revision += 1
        variants.append(state)
        state = original
        state.document.objects[0].body?.review.reviewedAgainst?.revision = 2
        variants.append(state)
        state = original
        state.document.objects[0].body?.review.origin = .trackerAssisted
        variants.append(state)
        for invalid in variants {
            #expect(throws: (any Error).self) { try project(feature, invalid) }
        }
        var assisted = original
        assisted.document.objects[0].body?.review.origin = .trackerAssisted
        assisted.document.objects[0].body?.review.trackerSource = "online estimate"
        #expect(try project(feature, assisted).assisted)
        var bad = feature
        bad.anchor.xM = .infinity
        #expect(throws: (any Error).self) { try project(bad, original) }
        bad = feature
        bad.anchor.boundM = .nan
        #expect(throws: (any Error).self) { try project(bad, original) }
    }
}
