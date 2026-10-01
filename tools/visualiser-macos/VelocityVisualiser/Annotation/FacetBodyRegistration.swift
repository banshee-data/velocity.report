import Foundation
import simd

typealias FacetBodyAnchor = Velocity_Recording_V1_FeatureAnchor

/// A proposal seeded by a reviewed body pose. Metric coordinates are stored
/// once in the pinned body frame; changing dimensions never rescales them.
/// Horizontal only: the current physical schema has no vertical uncertainty.
enum FacetBodyRegistration {
    /// A straight segment is a line relation, never a point at its visible
    /// centre. End returns define direction only; tangent position is weak.
    static func makeSegment(
        feature: FeatureCandidate, sampleID: Int, startIndex: UInt32, endIndex: UInt32,
        points: PackPoints, physical: PhysicalPackState, returnBoundM: Double, identityNote: String
    ) throws -> FacetBodyAnchor {
        guard feature.geometry == .edge, startIndex != endIndex,
            let source = feature.observations.first(where: {
                $0.sampleID == UInt32(exactly: sampleID) && $0.decision == .acceptedProposal
            }), source.pointIndices.contains(endIndex),
            let start = points.point(at: Int(startIndex)), let end = points.point(at: Int(endIndex))
        else {
            throw FeatureError.message("Choose two distinct returns in this edge's saved support")
        }
        // The compact seed validates the shared physical/origin/membership
        // contract. Its point coordinates are replaced by a line below.
        var compact = feature
        compact.geometry = .corner
        var anchor = try make(
            feature: compact, sampleID: sampleID, pointIndex: startIndex, points: points,
            physical: physical, returnBoundM: returnBoundM, identityNote: identityNote)
        let values = source.pointIndices.compactMap { points.point(at: Int($0)) }
        guard values.count == source.pointIndices.count,
            values.allSatisfy({ $0.x.isFinite && $0.y.isFinite && $0.z.isFinite }),
            Set(values).count >= 3
        else {
            throw FeatureError.message("A segment needs at least three distinct finite returns")
        }
        let a = SIMD3<Double>(Double(start.x), Double(start.y), Double(start.z))
        let b = SIMD3<Double>(Double(end.x), Double(end.y), Double(end.z))
        let vector = b - a
        let span = hypot(vector.x, vector.y)
        guard span >= 0.1, returnBoundM * 2 < span,
            values.allSatisfy({ value in
                let p = SIMD3<Double>(Double(value.x), Double(value.y), Double(value.z))
                return simd_length(simd_cross(p - a, vector)) / simd_length(vector) <= returnBoundM
            })
        else {
            throw FeatureError.message(
                "Choose a longer straight edge: every selected return must fit within the stated return bound"
            )
        }
        let object = physical.document.objects.first { $0.objectID == feature.objectID }!
        let pose = PhysicalGeometry.derive(
            object: object, keyframe: object.keyframe(sampleID: sampleID)!, gate: .preview)
        let yaw = pose.yaw!
        let c = cos(yaw.rad)
        let s = sin(yaw.rad)
        let vx = c * vector.x + s * vector.y
        let vy = -s * vector.x + c * vector.y
        var normal = SIMD2<Double>(-vy / span, vx / span)
        let major = abs(normal.x) >= abs(normal.y) ? normal.x : normal.y
        if major < 0 { normal = -normal }
        let angularBound = yaw.boundRad + asin(2 * returnBoundM / span)
        guard angularBound.isFinite, angularBound < .pi / 2 else {
            throw FeatureError.message(
                "This segment's orientation uncertainty is too broad to constrain a body relation")
        }
        let midpoint = SIMD2<Double>(anchor.xM + vx / 2, anchor.yM + vy / 2)
        var line = Velocity_Recording_V1_FeatureLineConstraint()
        line.normalX = normal.x
        line.normalY = normal.y
        line.offsetM = simd_dot(normal, midpoint)
        line.normalBoundRad = angularBound
        line.sourceStartIndex = startIndex
        line.sourceEndIndex = endIndex
        anchor.xM = 0
        anchor.yM = 0
        anchor.clearSourcePointIndex()
        anchor.line = line
        anchor.boundM =
            pose.centre!.bound + returnBoundM
            + (simd_length(midpoint) + pose.centre!.bound + returnBoundM)
            * PhysicalGeometry.swing(angularBound)
        anchor.method = "manual_named_segment_v1"
        guard anchor.boundM.isFinite else {
            throw FeatureError.message("The segment bound is not finite")
        }
        return anchor
    }

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
        guard
            Set(feature.observations.filter({ $0.decision == .acceptedProposal }).map(\.sampleID))
                .count >= 2,
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
        guard physical.document.editable, !physical.digest.isEmpty,
            let object = physical.document.objects.first(where: { $0.objectID == feature.objectID }
            ), let body = object.body, body.review.status == .reviewed,
            let k = object.keyframe(sampleID: sampleID), k.review.status == .reviewed,
            k.yaw.axis == .resolved,
            body.review.reviewedAgainst?.digest == observation.membershipDigest,
            k.review.reviewedAgainst?.digest == observation.membershipDigest,
            body.review.reviewedAgainst?.revision == Int(exactly: observation.membershipRevision),
            k.review.reviewedAgainst?.revision == Int(exactly: observation.membershipRevision)
        else {
            throw FeatureError.message(
                "Review the body and resolved pose against this feature's saved membership first")
        }
        let assisted = body.review.origin == .trackerAssisted || k.review.origin == .trackerAssisted
        for review in [body.review, k.review] where review.origin == .trackerAssisted {
            guard let source = review.trackerSource,
                !source.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            else {
                throw FeatureError.message(
                    "An assisted reference must retain its named tracker source")
            }
        }
        // Preview keeps explicit supported bounds while allowing assisted size.
        // The mapping is always a proposal and never becomes independent truth.
        let geometry = PhysicalGeometry.derive(object: object, keyframe: k, gate: .preview)
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
        anchor.origin = assisted ? "tracker_seeded_proposal" : "reference_seeded_proposal"
        anchor.method = "manual_named_return_v1"
        anchor.identityNote = identityNote
        guard anchor.boundM.isFinite else {
            throw FeatureError.message("The registration bound is not finite")
        }
        return anchor
    }
}
