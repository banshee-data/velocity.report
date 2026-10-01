import Foundation
import SwiftProtobuf
import Testing
import simd

@testable import VelocityVisualiser

enum FacetPoseFixture {
    static func bytes() throws -> Data {
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        return try Data(
            contentsOf: root.appendingPathComponent(
                "internal/lidar/annotation/testdata/facet-pose-proposal.json"))
    }
    static func proposal() throws -> FacetPoseProposal {
        try JSONDecoder().decode(FacetPoseProposal.self, from: bytes())
    }
    static func feature(_ result: FacetPoseProposal) -> FeatureCandidate {
        var feature = FeatureCandidate()
        feature.featureID = result.request.featureID
        feature.objectID = result.objectID
        feature.geometry = .protrusion
        feature.partRelation = "rigid_proposal"
        feature.anchor.bodyID = result.body.bodyID
        feature.anchor.partFrameID = result.body.bodyID + "_xy"
        feature.anchor.partFrameRevision = 1
        feature.anchor.coordinateDomain = "body_xy"
        feature.anchor.physicalRevision = result.sourcePhysicalRevision
        feature.anchor.physicalDigest = result.sourcePhysicalDigest
        feature.anchor.sourceSample = result.sourceSample
        feature.anchor.sourcePointIndex = 0
        feature.anchor.origin = result.sourceOrigin
        feature.anchor.method = "manual_named_return_v1"
        var observation = FeatureObservation()
        observation.sampleID = UInt32(result.request.sampleID)
        observation.timestampNs = result.request.timestampNs
        observation.membershipDigest = result.request.membershipDigest
        observation.membershipRevision = UInt64(result.request.membershipRevision)
        observation.pointIndices = [0, 1]
        observation.decision = .acceptedProposal
        feature.observations = [observation]
        return feature
    }
}

struct FacetPoseProposalTests {
    @Test func goContractPreservesPinsLargeTimestampAndTheFixedBody() throws {
        let result = try FacetPoseFixture.proposal()
        try result.validate(request: result.request, feature: FacetPoseFixture.feature(result))
        #expect(result.request.timestampNs == 9_007_199_254_740_993)
        #expect(result.request.pointIndex == 0 && result.request.endIndex == nil)
        #expect(result.status == "read_only_assisted_proposal")
        #expect(result.centre.xM == 10 && result.centre.yM == 5)
        #expect(result.centre.boundM >= result.request.prior.positionBoundM)
        #expect(result.box?.lengthM == 4.3 && abs((result.box?.widthM ?? 0) - 1.8) < 1e-10)
        #expect(result.body.review.origin == .independent, "rewrote the source body's origin")
        #expect(
            result.yaw.status == "prior_only" && result.unresolved.contains("height_not_measured"))
        let footprint = try #require(result.footprint)
        #expect(footprint.count == 4)
        #expect(abs(simd_distance(footprint[0], footprint[1]) - 4.3) < 1e-10)
        #expect(abs(simd_distance(footprint[1], footprint[2]) - 1.8) < 1e-10)
        let encoded = try JSONEncoder().encode(result.request)
        #expect(try JSONDecoder().decode(FacetPoseRequest.self, from: encoded) == result.request)
    }
    @Test func incompatibleResponsesCannotLosePinsBoundsOrFixedDimensions() throws {
        let original = try FacetPoseFixture.proposal()
        let feature = FacetPoseFixture.feature(original)
        var variants: [FacetPoseProposal] = []
        var result = original
        result.request.featureRevision += 1
        variants.append(result)
        result = original
        result.sourcePhysicalDigest = "other"
        variants.append(result)
        result = original
        result.centre.boundM = 0
        variants.append(result)
        result = original
        result.yaw.rad += 0.1
        variants.append(result)
        result = original
        result.yaw.status = "observed"
        variants.append(result)
        result = original
        result.box?.lengthM += 1
        variants.append(result)
        result = original
        result.matchedIndices = [99]
        variants.append(result)
        result = original
        result.matchedIndices = [0, 1]
        variants.append(result)
        result = original
        result.unresolved = []
        variants.append(result)
        result = original
        result.status = "reviewed"
        variants.append(result)
        for bad in variants {
            #expect(throws: (any Error).self) {
                try bad.validate(request: original.request, feature: feature)
            }
        }
        var uncertain = feature
        uncertain.observations[0].uncertainIndices = [0]
        #expect(throws: (any Error).self) {
            try original.validate(request: original.request, feature: uncertain)
        }
        var stale = feature
        stale.observations[0].membershipRevision += 1
        #expect(throws: (any Error).self) {
            try original.validate(request: original.request, feature: stale)
        }
    }
    @Test func anEdgeKeepsTangentialPositionAndYawExplicitlyPriorOnly() throws {
        var result = try FacetPoseFixture.proposal()
        var feature = FacetPoseFixture.feature(result)
        feature.geometry = .edge
        feature.anchor.clearSourcePointIndex()
        feature.anchor.line.normalX = 1
        feature.anchor.method = "manual_named_segment_v1"
        result.constraint = "normal_position_given_prior_yaw"
        result.normal = [1, 0]
        result.normalBoundM = 0.5
        result.tangentBoundM = result.request.prior.positionBoundM
        result.unresolved.append("tangent_from_prior_not_measured")
        result.request.endIndex = 1
        result.matchedIndices = [0, 1]
        try result.validate(request: result.request, feature: feature)
        var fake = result
        fake.tangentBoundM = 0
        #expect(throws: (any Error).self) {
            try fake.validate(request: result.request, feature: feature)
        }
        fake = result
        fake.normal = [1, 1]
        #expect(throws: (any Error).self) {
            try fake.validate(request: result.request, feature: feature)
        }
        fake = result
        fake.unresolved.removeAll { $0 == "tangent_from_prior_not_measured" }
        #expect(throws: (any Error).self) {
            try fake.validate(request: result.request, feature: feature)
        }
        fake = result
        fake.box = nil
        #expect(fake.footprint == nil)
    }
}
