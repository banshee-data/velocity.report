import Foundation
import Testing

@testable import VelocityVisualiser

struct FacetAuthoringFeedbackTests {
    @Test func supportShareUsesOnlyDistinctDefiniteMembersOfThePinnedObject() {
        var observation = FeatureObservation()
        observation.pointIndices = [0, 1, 1, 2]
        observation.uncertainIndices = [2]
        let result = FacetSupportSummary.make(
            observation: observation, objectIndices: [-1, 0, 1, 2, 3, 3], currentPin: true)
        #expect(result.definiteCount == 2 && result.objectCount == 4 && result.fraction == 0.5)
        #expect(result.explanation.contains("not accuracy"))
        let stale = FacetSupportSummary.make(
            observation: observation, objectIndices: [0, 1], currentPin: false)
        #expect(stale.fraction == nil && stale.explanation.contains("older"))
        let outside = FacetSupportSummary.make(
            observation: observation, objectIndices: [0], currentPin: true)
        #expect(outside.fraction == nil && outside.explanation.contains("outside"))
        let empty = FacetSupportSummary.make(
            observation: observation, objectIndices: [], currentPin: true)
        #expect(empty.fraction == nil && empty.objectCount == 0)
        observation.pointIndices = []
        observation.decision = .occluded
        let absent = FacetSupportSummary.make(
            observation: observation, objectIndices: [0, 1], currentPin: true)
        #expect(absent.fraction == nil && absent.explanation.contains("No measured"))
    }

    private func fixture() -> (FeatureCandidate, PhysicalPackState) {
        var feature = FeatureCandidate()
        feature.objectID = "car"
        feature.geometry = .edge
        for sample: UInt32 in [0, 1] {
            var observation = FeatureObservation()
            observation.sampleID = sample
            observation.pointIndices = [0, 1, 2]
            observation.decision = .acceptedProposal
            observation.membershipRevision = 1
            observation.membershipDigest = "sha256:members"
            feature.observations.append(observation)
        }
        var body = PhysicalBody(bodyID: "body")
        body.review.status = .reviewed
        body.review.origin = .independent
        body.review.reviewedAgainst = PhysicalMembershipPin(revision: 1, digest: "sha256:members")
        var pose = PhysicalKeyframe(keyframeID: "pose", sampleID: 0, timestampNs: 1)
        pose.review = body.review
        pose.position = PhysicalPosition(status: .observed, xM: 0, yM: 0, boundM: 0.1)
        pose.yaw = PhysicalYaw(status: .observed, axis: .resolved, yawRad: 0, boundRad: 0.05)
        let fake = FakePhysicalService(packDir: "/tmp/pack")
        fake.digest = "sha256:physical"
        fake.objects = [PhysicalObject(objectID: "car", body: body, keyframes: [pose])]
        return (feature, fake.state())
    }

    @Test func registrationPrerequisitesCountFramesAndRefuseStaleOrIncompletePose() {
        var (feature, physical) = fixture()
        func inspect() -> FacetRegistrationRequirements {
            .inspect(
                feature: feature, sampleID: 0, physical: physical, membershipRevision: 1,
                membershipDigest: "sha256:members")
        }
        #expect(inspect().ready)
        feature.observations[1].sampleID = 0
        #expect(inspect().supportedFrames == 1 && !inspect().ready)
        feature.observations[1].sampleID = 1
        feature.observations[0].membershipDigest = "sha256:old"
        #expect(!inspect().currentMembership && !inspect().reviewedPose)
        feature.observations[0].membershipDigest = "sha256:members"
        physical.document.objects[0].keyframes[0].yaw.axis = .frontRearAmbiguous
        #expect(!inspect().reviewedPose)
        physical.document.objects[0].keyframes[0].yaw.axis = .resolved
        physical.document.objects[0].keyframes[0].position.boundM = nil
        #expect(!inspect().reviewedPose)
        physical.document.objects[0].keyframes[0].position.boundM = 0.1
        physical.document.objects[0].body?.review.origin = .trackerAssisted
        physical.document.objects[0].body?.review.trackerSource = " "
        #expect(!inspect().namedAssistance && !inspect().ready)
        physical.document.objects[0].body?.review.trackerSource = "named estimator/report"
        #expect(inspect().ready)
        feature.geometry = .patch
        #expect(!inspect().eligibleGeometry && !inspect().ready)
        feature.geometry = .corner
        feature.observations[0].decision = .missing
        feature.observations[0].pointIndices = []
        #expect(!inspect().sourceSupported && !inspect().ready)
    }
}

@MainActor struct FacetSupportCacheTests {
    @Test func shareChangesWithSubsetSavedMaskAndFrameAndNeverUsesUnsavedPoints() throws {
        let points: [SyntheticPack.Point] = [
            (0, 0, 1, 1), (1, 0, 1, 1), (0, 1, 1, 1), (1, 1, 1, 1),
        ]
        let dir = try SyntheticPack.write([points, points], sourceStride: 1)
        defer { try? FileManager.default.removeItem(at: dir) }
        let session = try AnnotationSession(pack: AnnotationPack.open(directory: dir))
        session.operatorName = "op"
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: SelectionPolygon(rectFrom: SIMD2(-2, -2), to: SIMD2(2, 2))))
        #expect(session.save())
        var observation = FeatureObservation()
        observation.sampleID = 0
        observation.pointIndices = [0, 1]
        observation.membershipDigest = session.membershipDigest
        observation.membershipRevision = UInt64(session.sidecar.revision)
        let first = session.facetSupportSummary(observation: observation)
        #expect(first.fraction == 0.5 && first.objectCount == 4)
        #expect(session.facetSupportSummary(observation: observation) == first)
        observation.pointIndices = [0]
        #expect(session.facetSupportSummary(observation: observation).fraction == 0.25)
        observation.pointIndices = [0, 1]
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: SIMD2(-0.1, -0.1), to: SIMD2(1.1, 0.1)),
                mode: .replace))
        let unsaved = session.facetSupportSummary(observation: observation)
        #expect(unsaved.fraction == nil && unsaved.explanation.contains("Save object points"))
        #expect(session.save())
        #expect(session.facetSupportSummary(observation: observation).fraction == nil)
        observation.membershipDigest = session.membershipDigest
        observation.membershipRevision = UInt64(session.sidecar.revision)
        #expect(session.facetSupportSummary(observation: observation).fraction == 1)
        #expect(session.stepForward() == nil)
        #expect(session.facetSupportSummary(observation: observation).fraction == nil)
        observation.sampleID = 1
        let next = session.facetSupportSummary(observation: observation)
        #expect(next.fraction == nil && next.objectCount == 0)
    }
}
