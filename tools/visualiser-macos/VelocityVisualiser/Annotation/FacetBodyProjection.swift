import Foundation
import SwiftProtobuf
import simd

/// A read-only projection of a saved proposal using this frame's reviewed pose.
/// It is not a match or state update. A line retains an unconstrained tangent.
struct FacetBodyProjection: Equatable {
    var origin: SIMD2<Double>
    var normal: SIMD2<Double>?
    var boundM: Double
    var normalBoundRad: Double?
    var assisted: Bool
    var physicalRevision: Int
    var physicalDigest: String

    var isLine: Bool { normal != nil }

    func residual(point: SIMD2<Double>) -> Double {
        let delta = point - origin
        return normal.map { abs(simd_dot($0, delta)) } ?? simd_length(delta)
    }

    func residuals(points: PackPoints, indices: [UInt32]) -> FacetProjectionResiduals {
        let values = Set(indices).sorted().compactMap { index -> Double? in
            guard let point = points.point(at: Int(index)), point.x.isFinite, point.y.isFinite
            else { return nil }
            let value = residual(point: SIMD2(Double(point.x), Double(point.y)))
            return value.isFinite ? value : nil
        }
        return FacetProjectionResiduals(
            count: values.count,
            meanAbsoluteM: values.isEmpty ? nil : values.reduce(0, +) / Double(values.count),
            maximumAbsoluteM: values.max())
    }

    static func make(
        feature: FeatureCandidate, sampleID: Int, timestampNs: Int64, packDigest: String,
        physical: PhysicalPackState?, membershipDigest: String
    ) throws -> Self {
        guard feature.unknownFields.data.isEmpty, feature.hasAnchor,
            feature.partRelation == "rigid_proposal"
        else { throw FeatureError.message("Register this facet to a body first") }
        let anchor = feature.anchor
        guard anchor.unknownFields.data.isEmpty, anchor.coordinateDomain == "body_xy",
            anchor.partFrameRevision == 1, anchor.partFrameID == anchor.bodyID + "_xy",
            anchor.boundM.isFinite, anchor.boundM >= 0,
            ["reference_seeded_proposal", "tracker_seeded_proposal"].contains(anchor.origin)
        else { throw FeatureError.message("This saved relation cannot be projected by this build") }
        guard let physical, physical.document.editable, physical.revision > 0,
            !physical.digest.isEmpty, !membershipDigest.isEmpty,
            physical.membershipDigest == membershipDigest, physical.packDigest == packDigest,
            physical.document.packDigest == packDigest,
            physical.document.revision == physical.revision, physical.stale.isEmpty,
            physical.reviewDrift.isEmpty,
            let object = physical.document.objects.first(where: { $0.objectID == feature.objectID }
            ), let body = object.body, body.bodyID == anchor.bodyID,
            body.review.status == .reviewed, let pose = object.keyframe(sampleID: sampleID),
            pose.review.status == .reviewed, pose.yaw.axis == .resolved,
            pose.timestampNs == timestampNs
        else {
            throw FeatureError.message(
                "Reload/review Physical for this frame: its saved body, resolved pose and membership links must still hold"
            )
        }
        for review in [body.review, pose.review] {
            guard review.reviewedAgainst?.digest == membershipDigest,
                review.reviewedAgainst?.revision == physical.membershipRevision
            else {
                throw FeatureError.message(
                    "Review this frame against the current object membership")
            }
        }
        for review in [body.review, pose.review] where review.origin == .trackerAssisted {
            guard
                review.trackerSource?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                    == false
            else {
                throw FeatureError.message(
                    "The projected pose must retain its named tracker assistance")
            }
        }
        let geometry = PhysicalGeometry.derive(object: object, keyframe: pose, gate: .preview)
        guard let centre = geometry.centre, let yaw = geometry.yaw else {
            throw FeatureError.message("This frame needs supported centre and yaw bounds")
        }
        let c = cos(yaw.rad)
        let s = sin(yaw.rad)
        func rotate(_ p: SIMD2<Double>) -> SIMD2<Double> {
            SIMD2(c * p.x - s * p.y, s * p.x + c * p.y)
        }
        var local = SIMD2(anchor.xM, anchor.yM)
        var normal: SIMD2<Double>?
        var angular: Double?
        if anchor.hasLine {
            let line = anchor.line
            guard feature.geometry == .edge, anchor.method == "manual_named_segment_v1",
                line.unknownFields.data.isEmpty, line.normalX.isFinite, line.normalY.isFinite,
                abs(hypot(line.normalX, line.normalY) - 1) < 1e-6, line.offsetM.isFinite,
                line.normalBoundRad.isFinite, line.normalBoundRad >= 0, !anchor.hasSourcePointIndex,
                anchor.xM == 0, anchor.yM == 0, anchor.zM == 0
            else { throw FeatureError.message("Unsupported or malformed line relation") }
            let combined = line.normalBoundRad + yaw.boundRad
            angular = combined
            guard combined < .pi / 2 else {
                throw FeatureError.message(
                    "The combined normal uncertainty leaves this line unconstrained")
            }
            let n = SIMD2(line.normalX, line.normalY)
            normal = rotate(n)
            local = n * line.offsetM
        } else {
            guard [.corner, .protrusion].contains(feature.geometry),
                anchor.method == "manual_named_return_v1", anchor.hasSourcePointIndex,
                anchor.zM == 0, local.x.isFinite, local.y.isFinite
            else { throw FeatureError.message("Unsupported or malformed compact relation") }
        }
        let origin = SIMD2(centre.x, centre.y) + rotate(local)
        let bound =
            anchor.boundM + centre.bound + simd_length(local) * PhysicalGeometry.swing(yaw.boundRad)
        guard origin.x.isFinite, origin.y.isFinite, bound.isFinite,
            abs(origin.x) <= Double(Float.greatestFiniteMagnitude),
            abs(origin.y) <= Double(Float.greatestFiniteMagnitude)
        else {
            throw FeatureError.message(
                "The projected relation is outside the drawable numeric range")
        }
        return Self(
            origin: origin, normal: normal, boundM: bound, normalBoundRad: angular,
            assisted: anchor.origin == "tracker_seeded_proposal"
                || body.review.origin == .trackerAssisted || pose.review.origin == .trackerAssisted,
            physicalRevision: physical.revision, physicalDigest: physical.digest)
    }
}

struct FacetProjectionResiduals: Equatable {
    var count: Int
    var meanAbsoluteM: Double?
    var maximumAbsoluteM: Double?
}

struct FacetBodyPreview: Equatable {
    var featureID: String
    var sampleID: Int
    var membershipDigest: String
    var projection: FacetBodyProjection
}
