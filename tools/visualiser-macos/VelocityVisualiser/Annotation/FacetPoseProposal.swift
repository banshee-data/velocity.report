import Foundation
import simd

struct FacetPosePrior: Codable, Equatable {
    var xM: Double
    var yM: Double
    var yawRad: Double
    var positionBoundM: Double
    var yawBoundRad: Double
    var source: String
    enum CodingKeys: String, CodingKey {
        case xM = "x_m"
        case yM = "y_m"
        case yawRad = "yaw_rad"
        case positionBoundM = "position_bound_m"
        case yawBoundRad = "yaw_bound_rad"
        case source
    }
}

struct FacetPoseRequest: Codable, Equatable {
    var schemaVersion = 1
    var pack: String
    var packDigest: String
    var featureID: String
    var featureRevision: UInt64
    var featureDigest: String
    var membershipRevision: Int
    var membershipDigest: String
    var sampleID: Int
    var timestampNs: Int64
    var pointIndex: UInt32
    var endIndex: UInt32?
    var returnBoundM: Double
    var confirmed: Bool
    var author: String
    var identityNote: String
    var prior: FacetPosePrior
    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case pack
        case packDigest = "pack_digest"
        case featureID = "feature_id"
        case featureRevision = "feature_revision"
        case featureDigest = "feature_digest"
        case membershipRevision = "membership_revision"
        case membershipDigest = "membership_digest"
        case sampleID = "sample_id"
        case timestampNs = "timestamp_ns"
        case pointIndex = "point_index"
        case endIndex = "end_index"
        case returnBoundM = "return_bound_m"
        case confirmed = "correspondence_confirmed"
        case author
        case identityNote = "identity_note"
        case prior
    }
}

struct FacetPoseCentre: Codable, Equatable {
    var xM: Double
    var yM: Double
    var boundM: Double
    enum CodingKeys: String, CodingKey {
        case xM = "x_m"
        case yM = "y_m"
        case boundM = "bound_m"
    }
}
struct FacetPoseYaw: Codable, Equatable {
    var rad: Double
    var boundRad: Double
    var axis: String
    var status: String
    enum CodingKeys: String, CodingKey {
        case rad
        case boundRad = "bound_rad"
        case axis, status
    }
}
struct FacetPoseBox: Codable, Equatable {
    var centreXM: Double
    var centreYM: Double
    var yawRad: Double
    var lengthM: Double
    var widthM: Double
    enum CodingKeys: String, CodingKey {
        case centreXM = "centre_x_m"
        case centreYM = "centre_y_m"
        case yawRad = "yaw_rad"
        case lengthM = "length_m"
        case widthM = "width_m"
    }
}
struct FacetPoseProposal: Decodable, Equatable {
    var schema: String
    var schemaVersion: Int
    var status: String
    var request: FacetPoseRequest
    var objectID: String
    var sourcePhysicalRevision: UInt64
    var sourcePhysicalDigest: String
    var sourceSample: UInt32
    var sourceOrigin: String
    var targetOrigin: String
    var body: PhysicalBody
    var centre: FacetPoseCentre
    var yaw: FacetPoseYaw
    var constraint: String
    var normal: [Double]?
    var normalBoundM: Double?
    var tangentBoundM: Double?
    var unresolved: [String]
    var matchedIndices: [UInt32]
    var box: FacetPoseBox?
    var boxUnavailable: String?
    var frontPrediction: [Double]?
    var rearPrediction: [Double]?
    enum CodingKeys: String, CodingKey {
        case schema
        case schemaVersion = "schema_version"
        case status, request
        case objectID = "object_id"
        case sourcePhysicalRevision = "source_physical_revision"
        case sourcePhysicalDigest = "source_physical_digest"
        case sourceSample = "source_sample"
        case sourceOrigin = "source_origin"
        case targetOrigin = "target_origin"
        case body, centre, yaw, constraint, normal
        case normalBoundM = "normal_bound_m"
        case tangentBoundM = "tangent_bound_m"
        case unresolved
        case matchedIndices = "matched_indices"
        case box
        case boxUnavailable = "box_unavailable"
        case frontPrediction = "front_prediction"
        case rearPrediction = "rear_prediction"
    }

    var footprint: [SIMD2<Double>]? {
        guard let box else { return nil }
        let c = cos(box.yawRad)
        let s = sin(box.yawRad)
        return [SIMD2(-1.0, -1.0), SIMD2(1.0, -1.0), SIMD2(1.0, 1.0), SIMD2(-1.0, 1.0)].map {
            corner in
            let x = corner.x * box.lengthM / 2
            let y = corner.y * box.widthM / 2
            return SIMD2(box.centreXM + c * x - s * y, box.centreYM + s * x + c * y)
        }
    }

    func validate(request expected: FacetPoseRequest, feature: FeatureCandidate) throws {
        guard schema == "velocity.report/facet-pose-proposal", schemaVersion == 1,
            status == "read_only_assisted_proposal", request == expected,
            objectID == feature.objectID, body.bodyID == feature.anchor.bodyID,
            sourcePhysicalRevision == feature.anchor.physicalRevision,
            sourcePhysicalDigest == feature.anchor.physicalDigest,
            sourceSample == feature.anchor.sourceSample, Int(sourceSample) != expected.sampleID,
            sourceOrigin == feature.anchor.origin,
            ["planar_position_given_prior_yaw", "normal_position_given_prior_yaw"].contains(
                constraint), centre.xM.isFinite, centre.yM.isFinite, centre.boundM.isFinite,
            centre.boundM >= expected.prior.positionBoundM, yaw.rad == expected.prior.yawRad,
            yaw.boundRad == expected.prior.yawBoundRad, yaw.axis == "resolved",
            yaw.status == "prior_only", unresolved.contains("yaw_from_prior_not_measured"),
            unresolved.contains("height_not_measured")
        else {
            throw FeatureError.message(
                "The service returned an incompatible or stale pose proposal")
        }
        if let box {
            guard
                [box.lengthM, box.widthM, box.centreXM, box.centreYM, box.yawRad].allSatisfy(
                    \.isFinite), box.lengthM > 0, box.widthM > 0, box.centreXM == centre.xM,
                box.centreYM == centre.yM, box.yawRad == yaw.rad,
                box.lengthM == body.length.best?.value, box.widthM == body.width.best?.value
            else { throw FeatureError.message("Invalid proposal footprint") }
        }
        guard feature.anchor.hasLine == (constraint == "normal_position_given_prior_yaw"),
            expected.confirmed, !expected.author.isEmpty,
            matchedIndices.contains(expected.pointIndex)
        else {
            throw FeatureError.message(
                "The returned constraint disagrees with the confirmed correspondence")
        }
        if constraint == "planar_position_given_prior_yaw" {
            guard expected.endIndex == nil, matchedIndices == [expected.pointIndex] else {
                throw FeatureError.message("A compact spot must keep its single named return")
            }
        }
        if constraint == "normal_position_given_prior_yaw" {
            guard let normal, normal.count == 2, normal.allSatisfy(\.isFinite),
                abs(hypot(normal[0], normal[1]) - 1) < 1e-6, let normalBoundM,
                normalBoundM.isFinite, normalBoundM >= 0,
                tangentBoundM == expected.prior.positionBoundM,
                expected.endIndex.map { matchedIndices.contains($0) } == true,
                unresolved.contains("tangent_from_prior_not_measured")
            else { throw FeatureError.message("The proposal lost the edge's unresolved direction") }
        }
        guard
            let support = feature.observations.first(where: {
                Int($0.sampleID) == expected.sampleID
            }), support.decision == .acceptedProposal, support.timestampNs == expected.timestampNs,
            support.membershipDigest == expected.membershipDigest,
            support.membershipRevision == UInt64(exactly: expected.membershipRevision),
            !matchedIndices.isEmpty,
            Set(matchedIndices).isSubset(
                of: Set(support.pointIndices).subtracting(support.uncertainIndices))
        else { throw FeatureError.message("The proposed match is outside saved definite support") }
    }
}

extension FeatureAPIClient {
    func poseProposal(_ request: FacetPoseRequest) async throws -> FacetPoseProposal {
        var urlRequest = URLRequest(
            url: baseURL.appendingPathComponent("api/annotations/features/pose-proposal"))
        urlRequest.httpMethod = "POST"
        urlRequest.setValue("application/json", forHTTPHeaderField: "Content-Type")
        urlRequest.httpBody = try JSONEncoder().encode(request)
        let (data, response) = try await session.data(for: urlRequest)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            let body = (try? JSONSerialization.jsonObject(with: data)) as? [String: String]
            throw FeatureError.message(
                body?["error"] ?? "The offline proposal service refused this request")
        }
        return try JSONDecoder().decode(FacetPoseProposal.self, from: data)
    }
}
