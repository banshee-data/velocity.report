import Foundation

/// Counts describe selected evidence, never a quality/confidence score. An
/// older mask must not borrow today's denominator and imply the old subset fits.
struct FacetSupportSummary: Equatable {
    var definiteCount: Int
    var objectCount: Int
    var fraction: Double?
    var explanation: String

    static func make(
        observation: FeatureObservation, objectIndices: [Int], currentPin: Bool
    ) -> Self {
        let selected = Set(observation.pointIndices).subtracting(observation.uncertainIndices)
        let domain = Set(objectIndices.filter { $0 >= 0 }).compactMap { UInt32(exactly: $0) }
        let parent = Set(domain)
        var result = Self(
            definiteCount: selected.count, objectCount: parent.count, fraction: nil, explanation: ""
        )
        if selected.isEmpty {
            result.explanation = "No measured return support in this frame"
        } else if !currentPin {
            result.explanation = "Ratio unavailable: support pins an older object mask"
        } else if parent.isEmpty {
            result.explanation = "Ratio unavailable: no saved definite object returns"
        } else if !selected.isSubset(of: parent) {
            result.explanation =
                "Ratio unavailable: some support is outside the current definite object mask"
        } else {
            result.fraction = Double(selected.count) / Double(parent.count)
            result.explanation = "Support share, not accuracy. Facets may overlap."
        }
        return result
    }
}

/// Cheap source checks guide the operator before choosing a return. Geometry
/// and uncertainty of the actual chosen point/segment are validated on Register.
struct FacetRegistrationRequirements: Equatable {
    var supportedFrames: Int
    var sourceSupported: Bool
    var currentMembership: Bool
    var eligibleGeometry: Bool
    var reviewedPose: Bool
    var namedAssistance: Bool

    var ready: Bool {
        supportedFrames >= 2 && sourceSupported && currentMembership && eligibleGeometry
            && reviewedPose && namedAssistance
    }

    static func inspect(
        feature: FeatureCandidate, sampleID: Int, physical: PhysicalPackState?,
        membershipRevision: Int, membershipDigest: String
    ) -> Self {
        let support = feature.observations.filter {
            $0.decision == .acceptedProposal && !$0.pointIndices.isEmpty
        }
        let source = support.first { $0.sampleID == UInt32(exactly: sampleID) }
        var result = Self(
            supportedFrames: Set(support.map(\.sampleID)).count, sourceSupported: source != nil,
            currentMembership: source?.membershipDigest == membershipDigest
                && source?.membershipRevision == UInt64(exactly: membershipRevision)
                && !membershipDigest.isEmpty,
            eligibleGeometry: [.corner, .protrusion, .edge].contains(feature.geometry),
            reviewedPose: false, namedAssistance: true)
        guard let physical, physical.document.editable, physical.revision > 0,
            physical.revision <= Int(Int32.max), !physical.digest.isEmpty, let source,
            let object = physical.document.objects.first(where: { $0.objectID == feature.objectID }
            ), let body = object.body, let pose = object.keyframe(sampleID: sampleID)
        else { return result }
        let geometry = PhysicalGeometry.derive(object: object, keyframe: pose, gate: .preview)
        result.reviewedPose =
            body.review.status == .reviewed && pose.review.status == .reviewed
            && pose.yaw.axis == .resolved
            && body.review.reviewedAgainst?.digest == source.membershipDigest
            && pose.review.reviewedAgainst?.digest == source.membershipDigest
            && body.review.reviewedAgainst?.revision == Int(exactly: source.membershipRevision)
            && pose.review.reviewedAgainst?.revision == Int(exactly: source.membershipRevision)
            && geometry.centre != nil && geometry.yaw != nil
        for review in [body.review, pose.review] where review.origin == .trackerAssisted {
            if review.trackerSource?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                != false
            {
                result.namedAssistance = false
            }
        }
        return result
    }
}
