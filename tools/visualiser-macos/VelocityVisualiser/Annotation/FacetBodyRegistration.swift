import Foundation
import simd

typealias FacetBodyAnchor = Velocity_Recording_V1_FeatureAnchor

/// A proposal seeded by a reviewed body pose. Metric coordinates are stored
/// once in the pinned body frame; changing dimensions never rescales them.
/// Horizontal only: the current physical schema has no vertical uncertainty.
enum FacetBodyRegistration {
    static func make(
        feature: FeatureCandidate, sampleID: Int, pointIndex: UInt32, points: PackPoints,
        physical: PhysicalPackState, returnBoundM: Double, identityNote: String
    ) throws -> FacetBodyAnchor {
        guard sampleID >= 0, sampleID <= Int(UInt32.max), physical.revision > 0,
            physical.revision <= Int(Int32.max)
        else { throw FeatureError.message("Invalid source sample or physical revision") }
        guard feature.geometry == .corner || feature.geometry == .protrusion else {
            throw FeatureError.message(
                "Edges and surfaces need weak-direction constraints; do not anchor their visible centre"
            )
        }
        guard feature.observations.filter({ $0.decision == .acceptedProposal }).count >= 2,
            let observation = feature.observations.first(where: {
                $0.sampleID == UInt32(sampleID) && $0.decision == .acceptedProposal
            }), observation.pointIndices.contains(pointIndex),
            let point = points.point(at: Int(pointIndex))
        else {
            throw FeatureError.message(
                "Save definite support for the same feature in two frames, then choose one of its returns"
            )
        }
        guard returnBoundM.isFinite, returnBoundM > 0,
            !identityNote.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        else {
            throw FeatureError.message(
                "Name the repeatable physical spot and state its return uncertainty")
        }
        guard !physical.digest.isEmpty,
            let object = physical.document.objects.first(where: { $0.objectID == feature.objectID }
            ), let body = object.body, body.review.status == .reviewed,
            body.review.origin == .independent, let k = object.keyframe(sampleID: sampleID),
            k.review.status == .reviewed, k.review.origin == .independent, k.yaw.axis == .resolved,
            body.review.reviewedAgainst?.digest == observation.membershipDigest,
            k.review.reviewedAgainst?.digest == observation.membershipDigest
        else {
            throw FeatureError.message(
                "Review an independent body and resolved pose against this feature's saved membership first"
            )
        }
        let geometry = PhysicalGeometry.derive(object: object, keyframe: k)
        guard let centre = geometry.centre, let yaw = geometry.yaw, point.x.isFinite,
            point.y.isFinite
        else { throw FeatureError.message("The pinned pose needs a supported centre and yaw") }
        let dx = Double(point.x) - centre.x
        let dy = Double(point.y) - centre.y
        let c = cos(yaw.rad)
        let s = sin(yaw.rad)
        var anchor = FacetBodyAnchor()
        anchor.partFrameID = body.bodyID + "_xy"
        anchor.partFrameRevision = 1
        anchor.xM = c * dx + s * dy
        anchor.yM = -s * dx + c * dy
        anchor.physicalRevision = UInt64(physical.revision)
        anchor.physicalDigest = physical.digest
        anchor.bodyID = body.bodyID
        anchor.keyframeID = k.keyframeID
        anchor.coordinateDomain = "body_xy"
        anchor.sourceSample = UInt32(sampleID)
        anchor.sourcePointIndex = pointIndex
        anchor.returnBoundM = returnBoundM
        anchor.boundM =
            centre.bound + hypot(dx, dy) * PhysicalGeometry.swing(yaw.boundRad) + returnBoundM
        anchor.origin = "reference_seeded_proposal"
        anchor.method = "manual_named_return_v1"
        anchor.identityNote = identityNote
        return anchor
    }
}
