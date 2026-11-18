// Joint confirmed correspondences use the existing offline proposal endpoint.
// They never write physical references or update the online tracker.
import Foundation
import simd

struct FacetJointMatch: Codable, Equatable {
    var featureID: String
    var pointIndex: UInt32
    var endIndex: UInt32?
    var returnBoundM: Double
    var confirmed: Bool
    var identityNote: String
    enum CodingKeys: String, CodingKey {
        case featureID = "feature_id"
        case pointIndex = "point_index"
        case endIndex = "end_index"
        case returnBoundM = "return_bound_m"
        case confirmed = "correspondence_confirmed"
        case identityNote = "identity_note"
    }
}

/// Pins the basket to one object, frame and saved evidence revision.
struct FacetJointContext: Equatable {
    var objectID: String
    var sampleID: Int
    var timestampNs: Int64
    var featureRevision: UInt64
    var featureDigest: String
    var membershipRevision: Int
    var membershipDigest: String
}

struct FacetJointPoseRequest: Codable, Equatable {
    var schemaVersion = 2
    var pack: String
    var packDigest: String
    var featureRevision: UInt64
    var featureDigest: String
    var membershipRevision: Int
    var membershipDigest: String
    var sampleID: Int
    var timestampNs: Int64
    var author: String
    var prior: FacetPosePrior
    var matches: [FacetJointMatch]
    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case pack
        case packDigest = "pack_digest"
        case featureRevision = "feature_revision"
        case featureDigest = "feature_digest"
        case membershipRevision = "membership_revision"
        case membershipDigest = "membership_digest"
        case sampleID = "sample_id"
        case timestampNs = "timestamp_ns"
        case author, prior, matches
    }
}

struct FacetJointContributor: Codable, Equatable {
    var featureID: String
    var sourcePhysicalRevision: UInt64
    var sourcePhysicalDigest: String
    var sourceSample: UInt32
    var sourceOrigin: String
    var targetOrigin: String
    var matchedIndices: [UInt32]
    enum CodingKeys: String, CodingKey {
        case featureID = "feature_id"
        case sourcePhysicalRevision = "source_physical_revision"
        case sourcePhysicalDigest = "source_physical_digest"
        case sourceSample = "source_sample"
        case sourceOrigin = "source_origin"
        case targetOrigin = "target_origin"
        case matchedIndices = "matched_indices"
    }
}

/// Evidence support states, never calibrated match probabilities.
struct FacetJointConfidence: Codable, Equatable {
    var state: String
    var position: String
    var yaw: String
    var dataRank: Int
    var reasons: [String]
    enum CodingKeys: String, CodingKey {
        case state, position, yaw, reasons
        case dataRank = "data_rank"
    }
}

struct FacetJointPoseProposal: Decodable, Equatable {
    var schema: String
    var schemaVersion: Int
    var status: String
    var request: FacetJointPoseRequest
    var objectID: String
    var body: PhysicalBody
    var centre: FacetPoseCentre
    var yaw: FacetPoseYaw
    var constraint: String
    var unresolved: [String]
    var matchedIndices: [UInt32]
    var contributors: [FacetJointContributor]
    var confidence: FacetJointConfidence
    var normal: [Double]?
    var normalBoundM: Double?
    var tangentBoundM: Double?
    var box: FacetPoseBox?
    var boxUnavailable: String?
    var frontPrediction: [Double]?
    var rearPrediction: [Double]?
    enum CodingKeys: String, CodingKey {
        case schema, status, request, body, centre, yaw, constraint, unresolved, contributors,
            confidence, box
        case schemaVersion = "schema_version"
        case objectID = "object_id"
        case matchedIndices = "matched_indices"
        case boxUnavailable = "box_unavailable"
        case frontPrediction = "front_prediction"
        case rearPrediction = "rear_prediction"
        case normal
        case normalBoundM = "normal_bound_m"
        case tangentBoundM = "tangent_bound_m"
    }

    func validate(request expected: FacetJointPoseRequest, features: [FeatureCandidate]) throws {
        func refuse() throws {
            throw FeatureError.message(
                "The service returned an incompatible or stale joint pose proposal")
        }
        guard schema == "velocity.report/facet-joint-pose-proposal", schemaVersion == 1,
            status == "read_only_assisted_proposal", expected.schemaVersion == 2,
            request == expected, (2...4).contains(expected.matches.count),
            Set(expected.matches.map(\.featureID)).count == expected.matches.count,
            contributors.count == expected.matches.count,
            Set(contributors.map(\.featureID)) == Set(expected.matches.map(\.featureID)),
            centre.xM.isFinite, centre.yM.isFinite, centre.boundM.isFinite, centre.boundM >= 0,
            yaw.rad.isFinite, yaw.boundRad.isFinite, yaw.boundRad >= 0, yaw.boundRad <= .pi,
            yaw.axis == "resolved", unresolved.contains("height_not_measured"),
            [
                "joint_planar_pose", FacetPoseProposal.planarPositionGivenPriorYaw,
                FacetPoseProposal.normalPositionAndYaw,
            ].contains(constraint), ["supported", "partial"].contains(confidence.state),
            ["supported", "partial", "prior_only"].contains(confidence.position),
            ["supported", "prior_only"].contains(confidence.yaw),
            (1...3).contains(confidence.dataRank), !matchedIndices.isEmpty,
            Set(matchedIndices).count == matchedIndices.count
        else {
            try refuse()
            return
        }
        if confidence.state == "supported" {
            guard confidence.dataRank == 3, confidence.position == "supported",
                confidence.yaw == "supported", yaw.status == "observed",
                !unresolved.contains("yaw_from_prior_not_measured"),
                !unresolved.contains("tangent_from_prior_not_measured")
            else {
                try refuse()
                return
            }
        } else if confidence.dataRank == 3 {
            try refuse()
            return
        }
        if confidence.yaw == "prior_only" {
            guard yaw.status == "prior_only", yaw.rad == expected.prior.yawRad,
                yaw.boundRad == expected.prior.yawBoundRad,
                unresolved.contains("yaw_from_prior_not_measured")
            else {
                try refuse()
                return
            }
        } else {
            guard yaw.status == "observed", yaw.boundRad < .pi / 2,
                abs(yaw.rad - expected.prior.yawRad) <= yaw.boundRad + expected.prior.yawBoundRad
                    + 1e-9, !unresolved.contains("yaw_from_prior_not_measured")
            else {
                try refuse()
                return
            }
        }
        if let normal {
            guard normal.count == 2, normal.allSatisfy(\.isFinite),
                abs(hypot(normal[0], normal[1]) - 1) < 1e-6, let normalBoundM,
                normalBoundM.isFinite, normalBoundM >= 0, let tangentBoundM, tangentBoundM.isFinite,
                tangentBoundM == expected.prior.positionBoundM,
                unresolved.contains("tangent_from_prior_not_measured"),
                confidence.state == "partial"
            else {
                try refuse()
                return
            }
        }
        if constraint == FacetPoseProposal.normalPositionAndYaw && normal == nil {
            try refuse()
            return
        }
        if normal == nil && (normalBoundM != nil || tangentBoundM != nil) {
            try refuse()
            return
        }
        if constraint == "joint_planar_pose" {
            guard confidence.state == "supported", confidence.dataRank == 3 else {
                try refuse()
                return
            }
        } else {
            guard confidence.state == "partial", confidence.dataRank == 2,
                confidence.position == "partial"
            else {
                try refuse()
                return
            }
        }
        var all = Set<UInt32>()
        var physicalPin: String?
        var namedSources = Set<String>()
        for match in expected.matches {
            guard match.confirmed, match.returnBoundM.isFinite, match.returnBoundM > 0,
                !match.identityNote.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
                let feature = features.first(where: { $0.featureID == match.featureID }),
                !feature.inactive, feature.objectID == objectID, feature.hasAnchor,
                feature.anchor.bodyID == body.bodyID,
                let contributor = contributors.first(where: { $0.featureID == match.featureID }),
                contributor.sourcePhysicalRevision == feature.anchor.physicalRevision,
                contributor.sourcePhysicalDigest == feature.anchor.physicalDigest,
                contributor.sourceSample == feature.anchor.sourceSample,
                contributor.sourceOrigin == feature.anchor.origin,
                Int(contributor.sourceSample) != expected.sampleID,
                let observation = feature.observations.first(where: {
                    Int($0.sampleID) == expected.sampleID
                }), observation.decision == .acceptedProposal,
                observation.timestampNs == expected.timestampNs,
                observation.membershipDigest == expected.membershipDigest,
                observation.membershipRevision == UInt64(exactly: expected.membershipRevision),
                contributor.targetOrigin == observation.origin,
                contributor.matchedIndices.contains(match.pointIndex),
                Set(contributor.matchedIndices).isSubset(
                    of: Set(observation.pointIndices).subtracting(observation.uncertainIndices))
            else {
                try refuse()
                return
            }
            let pin =
                "\(contributor.sourcePhysicalRevision):\(contributor.sourcePhysicalDigest):\(feature.anchor.partFrameID):\(feature.anchor.partFrameRevision)"
            if let physicalPin, physicalPin != pin {
                try refuse()
                return
            }
            physicalPin = pin
            if feature.anchor.hasLine {
                guard let end = match.endIndex, contributor.matchedIndices.contains(end) else {
                    try refuse()
                    return
                }
            } else {
                guard match.endIndex == nil, contributor.matchedIndices == [match.pointIndex] else {
                    try refuse()
                    return
                }
                guard feature.anchor.hasSourcePointIndex,
                    namedSources.insert(
                        "\(feature.anchor.sourceSample):\(feature.anchor.sourcePointIndex)"
                    ).inserted
                else {
                    try refuse()
                    return
                }
            }
            guard all.isDisjoint(with: contributor.matchedIndices) else {
                try refuse()
                return
            }
            all.formUnion(contributor.matchedIndices)
        }
        guard all == Set(matchedIndices) else {
            try refuse()
            return
        }
        if let box {
            guard
                [box.centreXM, box.centreYM, box.yawRad, box.lengthM, box.widthM].allSatisfy(
                    \.isFinite), box.centreXM == centre.xM, box.centreYM == centre.yM,
                box.yawRad == yaw.rad, box.lengthM > 0, box.widthM > 0,
                box.lengthM == body.length.best?.value, box.widthM == body.width.best?.value
            else {
                try refuse()
                return
            }
        }
        for prediction in [frontPrediction, rearPrediction].compactMap({ $0 }) {
            guard prediction.count == 2, prediction.allSatisfy(\.isFinite) else {
                try refuse()
                return
            }
        }
    }
}

extension FeatureAPIClient {
    func jointPoseProposal(_ request: FacetJointPoseRequest) async throws -> FacetJointPoseProposal
    { try await sendPoseProposal(request, as: FacetJointPoseProposal.self) }
}

/// Both wire formats feed the existing overlay through the same display model.
struct FacetPoseDrawing {
    var centre: FacetPoseCentre
    var matchedIndices: [UInt32]
    var box: FacetPoseBox?
    var label: String
    var footprint: [SIMD2<Double>]? {
        guard let box else { return nil }
        let c = cos(box.yawRad)
        let s = sin(box.yawRad)
        return [SIMD2(-1.0, -1.0), SIMD2(1.0, -1.0), SIMD2(1.0, 1.0), SIMD2(-1.0, 1.0)].map {
            let x = $0.x * box.lengthM / 2
            let y = $0.y * box.widthM / 2
            return SIMD2(box.centreXM + c * x - s * y, box.centreYM + s * x + c * y)
        }
    }
}

extension AnnotationSession {
    var currentJointFacetPoseProposal: FacetJointPoseProposal? {
        guard let result = features.jointPoseProposal, let sample = currentSample,
            result.request.sampleID == sample.sampleID,
            result.request.timestampNs == sample.timestampNs,
            result.request.membershipRevision == sidecar.revision,
            result.request.membershipDigest == membershipDigest,
            result.request.featureDigest == features.state?.digest,
            result.request.featureRevision == features.state?.document.revision,
            result.objectID == activeObjectID, !dirtySamples.contains(sample.sampleID),
            !physical.isDirty, !features.isDirty
        else { return nil }
        return result
    }

    var currentFacetPoseDrawing: FacetPoseDrawing? {
        if let result = currentJointFacetPoseProposal {
            return FacetPoseDrawing(
                centre: result.centre, matchedIndices: result.matchedIndices, box: result.box,
                label: "joint body proposal · \(result.confidence.state) support")
        }
        guard let result = currentFacetPoseProposal else { return nil }
        return FacetPoseDrawing(
            centre: result.centre, matchedIndices: result.matchedIndices, box: result.box,
            label: result.constraint == FacetPoseProposal.normalPositionAndYaw
                ? "body pose proposal · measured yaw" : "body pose proposal · prior yaw")
    }
}
