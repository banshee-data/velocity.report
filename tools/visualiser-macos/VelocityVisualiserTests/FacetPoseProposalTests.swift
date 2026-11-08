import Foundation
import SwiftProtobuf
import Testing
import simd

@testable import VelocityVisualiser

enum FacetPoseFixture {
    static func bytes(_ name: String = "facet-pose-proposal.json") throws -> Data {
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        return try Data(
            contentsOf: root.appendingPathComponent("internal/lidar/annotation/testdata/" + name))
    }
    static func proposal() throws -> FacetPoseProposal {
        try JSONDecoder().decode(FacetPoseProposal.self, from: bytes())
    }
    /// The Go segment proposal: the body's left side seen off the prior yaw.
    static func segmentProposal() throws -> FacetPoseProposal {
        try JSONDecoder().decode(
            FacetPoseProposal.self, from: bytes("facet-pose-proposal-segment.json"))
    }
    /// The saved edge registration the Go segment fixture was solved from.
    static func segmentFeature(_ result: FacetPoseProposal) -> FeatureCandidate {
        var edge = Self.feature(result)
        edge.geometry = .edge
        edge.anchor.clearSourcePointIndex()
        edge.anchor.method = "manual_named_segment_v1"
        edge.anchor.line.normalY = 1
        edge.anchor.line.offsetM = 0.9
        edge.anchor.line.normalBoundRad = 0.06
        edge.observations[0].pointIndices = [0, 1, 2]
        return edge
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
        #expect(result.status == "read_only_assisted_proposal" && result.schemaVersion == 2)
        #expect(result.constraint == FacetPoseProposal.planarPositionGivenPriorYaw)
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
        // A version 1 result never measured yaw; do not read it as version 2.
        result = original
        result.schemaVersion = 1
        variants.append(result)
        result = original
        result.unresolved.removeAll { $0 == "yaw_from_prior_not_measured" }
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
    @Test func aSegmentReportsItsMeasuredYawAndKeepsTangentialPositionPriorOnly() throws {
        let result = try FacetPoseFixture.segmentProposal()
        let feature = FacetPoseFixture.segmentFeature(result)
        try result.validate(request: result.request, feature: feature)
        #expect(result.schemaVersion == 2 && result.request.schemaVersion == 1)
        #expect(result.constraint == FacetPoseProposal.normalPositionAndYaw)
        #expect(result.yaw.status == "observed" && result.yaw.axis == "resolved")
        // The fixture's side is seen 0.02 rad off the prior yaw; the bound is
        // the two normal bounds, not the prior's.
        #expect(abs(result.yaw.rad - (result.request.prior.yawRad + 0.02)) < 1e-4)
        #expect(
            result.yaw.boundRad > 0.06
                && result.yaw.boundRad != result.request.prior.yawBoundRad)
        #expect(!result.unresolved.contains("yaw_from_prior_not_measured"))
        #expect(
            result.unresolved.contains("tangent_from_prior_not_measured")
                && result.unresolved.contains("height_not_measured"))
        #expect(result.box?.yawRad == result.yaw.rad && result.matchedIndices == [0, 1, 2])
        #expect(result.tangentBoundM == result.request.prior.positionBoundM)
        var variants: [FacetPoseProposal] = []
        var fake = result
        fake.tangentBoundM = 0
        variants.append(fake)
        fake = result
        fake.normal = [1, 1]
        variants.append(fake)
        fake = result
        fake.unresolved.removeAll { $0 == "tangent_from_prior_not_measured" }
        variants.append(fake)
        // A measured yaw must say so, and may not also claim to be the prior.
        fake = result
        fake.yaw.status = "prior_only"
        variants.append(fake)
        fake = result
        fake.unresolved.append("yaw_from_prior_not_measured")
        variants.append(fake)
        // A yaw outside both bounds of the prior is a contradiction.
        fake = result
        fake.yaw.rad = result.request.prior.yawRad + 0.5
        fake.box?.yawRad = fake.yaw.rad
        variants.append(fake)
        fake = result
        fake.yaw.boundRad = .pi / 2
        variants.append(fake)
        // The box follows the measured yaw, not the prior's.
        fake = result
        fake.box?.yawRad = result.request.prior.yawRad
        variants.append(fake)
        // The version 1 name meant yaw was the prior's.
        fake = result
        fake.constraint = "normal_position_given_prior_yaw"
        variants.append(fake)
        for bad in variants {
            #expect(throws: (any Error).self) {
                try bad.validate(request: result.request, feature: feature)
            }
        }
        // A compact spot registration cannot accept a segment's result.
        #expect(throws: (any Error).self) {
            try result.validate(request: result.request, feature: FacetPoseFixture.feature(result))
        }
        fake = result
        fake.box = nil
        #expect(fake.footprint == nil)
    }
}
