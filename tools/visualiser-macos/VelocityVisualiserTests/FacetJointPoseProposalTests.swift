import Foundation
import Testing
import simd

@testable import VelocityVisualiser

enum FacetJointFixture {
    static func proposal(
        _ name: String = "facet-joint-pose-compact.json"
    ) throws -> FacetJointPoseProposal {
        try JSONDecoder().decode(FacetJointPoseProposal.self, from: FacetPoseFixture.bytes(name))
    }

    static func features(_ result: FacetJointPoseProposal) -> [FeatureCandidate] {
        result.contributors.enumerated().map { offset, contributor in
            var feature = FeatureCandidate()
            feature.featureID = contributor.featureID
            feature.objectID = result.objectID
            feature.partRelation = "rigid_proposal"
            feature.anchor.bodyID = result.body.bodyID
            feature.anchor.partFrameID = result.body.bodyID + "_xy"
            feature.anchor.partFrameRevision = 1
            feature.anchor.coordinateDomain = "body_xy"
            feature.anchor.physicalRevision = contributor.sourcePhysicalRevision
            feature.anchor.physicalDigest = contributor.sourcePhysicalDigest
            feature.anchor.sourceSample = contributor.sourceSample
            feature.anchor.origin = contributor.sourceOrigin
            let match = result.request.matches.first { $0.featureID == contributor.featureID }!
            if match.endIndex != nil {
                feature.geometry = .edge
                feature.anchor.line.normalX = 1
                feature.anchor.line.normalBoundRad = 0.01
                feature.anchor.method = "manual_named_segment_v1"
            } else {
                feature.geometry = .protrusion
                feature.anchor.sourcePointIndex = UInt32(offset)
                feature.anchor.method = "manual_named_return_v1"
            }
            var observation = FeatureObservation()
            observation.sampleID = UInt32(result.request.sampleID)
            observation.timestampNs = result.request.timestampNs
            observation.pointIndices = contributor.matchedIndices
            observation.decision = .acceptedProposal
            observation.membershipDigest = result.request.membershipDigest
            observation.membershipRevision = UInt64(result.request.membershipRevision)
            observation.origin = contributor.targetOrigin
            feature.observations = [observation]
            return feature
        }
    }
}

struct FacetJointPoseProposalTests {
    @Test func sharedGoFixturesRetainSupportAndPartialDirections() throws {
        for name in ["facet-joint-pose-compact.json", "facet-joint-pose-parallel.json"] {
            let result = try FacetJointFixture.proposal(name)
            try result.validate(
                request: result.request, features: FacetJointFixture.features(result))
            let bytes = try JSONEncoder().encode(result.request)
            #expect(
                try JSONDecoder().decode(FacetJointPoseRequest.self, from: bytes) == result.request)
            let document = try #require(JSONSerialization.jsonObject(with: bytes) as? [String: Any])
            #expect(document["feature_id"] == nil && document["point_index"] == nil)
            #expect(result.request.matches.contains { $0.pointIndex == 0 })
            #expect(result.unresolved.contains("height_not_measured"))
            #expect(result.status == "read_only_assisted_proposal")
            if name.contains("parallel") {
                #expect(result.confidence.state == "partial" && result.confidence.dataRank == 2)
                #expect(result.unresolved.contains("tangent_from_prior_not_measured"))
                #expect(result.tangentBoundM == result.request.prior.positionBoundM)
            } else {
                #expect(result.confidence.state == "supported" && result.confidence.dataRank == 3)
                #expect(result.yaw.status == "observed")
            }
            let drawing = FacetPoseDrawing(
                centre: result.centre, matchedIndices: result.matchedIndices, box: result.box,
                label: "Joint proposal")
            #expect(drawing.footprint?.count == 4)
        }
    }

    @Test func incompatibleJointResultsCannotClaimAFullLockOrLosePins() throws {
        let original = try FacetJointFixture.proposal()
        let features = FacetJointFixture.features(original)
        var changes: [FacetJointPoseProposal] = []
        var altered = original
        altered.schemaVersion += 1
        changes.append(altered)
        altered = original
        altered.request.featureDigest = "other"
        changes.append(altered)
        altered = original
        altered.contributors[0].sourcePhysicalDigest = "other"
        changes.append(altered)
        altered = original
        altered.contributors[0].targetOrigin = "other"
        changes.append(altered)
        altered = original
        altered.confidence.dataRank = 2
        changes.append(altered)
        altered = original
        altered.confidence.state = "high_confidence"
        changes.append(altered)
        altered = original
        altered.centre.boundM = -1
        changes.append(altered)
        altered = original
        altered.yaw.boundRad = .infinity
        changes.append(altered)
        altered = original
        altered.unresolved = []
        changes.append(altered)
        altered = original
        altered.matchedIndices.append(999)
        changes.append(altered)
        altered = original
        altered.box?.lengthM += 1
        changes.append(altered)
        altered = original
        altered.confidence.yaw = "prior_only"
        changes.append(altered)
        for altered in changes {
            #expect(throws: (any Error).self) {
                try altered.validate(request: original.request, features: features)
            }
        }
        var uncertain = features
        uncertain[0].observations[0].uncertainIndices = uncertain[0].observations[0].pointIndices
        #expect(throws: (any Error).self) {
            try original.validate(request: original.request, features: uncertain)
        }
        var retired = features
        retired[0].inactive = true
        #expect(throws: (any Error).self) {
            try original.validate(request: original.request, features: retired)
        }
        var stale = features
        stale[0].anchor.partFrameRevision += 1
        #expect(throws: (any Error).self) {
            try original.validate(request: original.request, features: stale)
        }
    }

    @Test func partialGeometryCannotBeRelabelledAsFullySupported() throws {
        var result = try FacetJointFixture.proposal("facet-joint-pose-parallel.json")
        let request = result.request
        let features = FacetJointFixture.features(result)
        result.confidence.state = "supported"
        #expect(throws: (any Error).self) {
            try result.validate(request: request, features: features)
        }
    }
}

extension FacetJointPoseProposalTests {
    @Test func partialResultsRetainFiniteNormalAndPriorTangent() throws {
        let original = try FacetJointFixture.proposal("facet-joint-pose-parallel.json")
        let features = FacetJointFixture.features(original)
        var variants: [FacetJointPoseProposal] = []
        var altered = original
        altered.normal = nil
        variants.append(altered)
        altered = original
        altered.normal = [0, 0]
        variants.append(altered)
        altered = original
        altered.normalBoundM = -1
        variants.append(altered)
        altered = original
        altered.tangentBoundM = .infinity
        variants.append(altered)
        altered = original
        altered.confidence.position = "supported"
        variants.append(altered)
        for variant in variants {
            #expect(throws: (any Error).self) {
                try variant.validate(request: original.request, features: features)
            }
        }
    }

    @Test func sourceLandmarksAndSavedSupportMustBeDistinctAndCurrent() throws {
        let result = try FacetJointFixture.proposal()
        var features = FacetJointFixture.features(result)
        features[1].anchor.sourceSample = features[0].anchor.sourceSample
        features[1].anchor.sourcePointIndex = features[0].anchor.sourcePointIndex
        #expect(throws: (any Error).self) {
            try result.validate(request: result.request, features: features)
        }
        features = FacetJointFixture.features(result)
        features[0].observations[0].membershipRevision += 1
        #expect(throws: (any Error).self) {
            try result.validate(request: result.request, features: features)
        }
        var changed = result
        changed.contributors[1].matchedIndices = changed.contributors[0].matchedIndices
        #expect(throws: (any Error).self) {
            try changed.validate(
                request: result.request, features: FacetJointFixture.features(result))
        }
    }
}
