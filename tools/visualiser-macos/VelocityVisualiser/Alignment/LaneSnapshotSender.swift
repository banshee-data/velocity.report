// LaneSnapshotSender.swift
// Send lanes to velocity.report (lane painter plan, Item 4): after a save,
// the street-use features exactly as Gerardus returned them, with the
// accepted record that places them, become velocity.report's append-only
// lane snapshot (PUT /api/lidar/sites/{l16}/lanes), which the speed reports
// attribute through. Gerardus is written first and is authoritative; this
// copy follows, and the pane says so until it is sent.

import Foundation
import SwiftProtobuf

/// velocity.report's lane snapshot route, abstracted so tests can fake it.
protocol LaneSnapshotSink: Sendable {
    /// PUTs the body; answers the status and the body returned.
    func putLanes(l16Token: String, body: Data) async throws -> (status: Int, body: Data)
}

struct VelocityReportLaneSnapshotClient: LaneSnapshotSink {
    var baseURL: @Sendable () -> URL = { VelocityReportServer.currentHTTPURL }
    var session: URLSession = APISession.shared

    func putLanes(l16Token: String, body: Data) async throws -> (status: Int, body: Data) {
        var request = URLRequest(
            url: baseURL().appendingPathComponent("api/lidar/sites/\(l16Token)/lanes"))
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = body
        let (data, response) = try await session.data(for: request)
        return ((response as? HTTPURLResponse)?.statusCode ?? 0, data)
    }
}

/// The request velocity.report's `lanes.Request` reads.
struct LaneSnapshotRequest: Codable, Equatable, Sendable {
    struct Pose: Codable, Equatable, Sendable {
        var translation: [Double]
        var rotation: [Double]
        enum CodingKeys: String, CodingKey {
            case translation = "translation_m"
            case rotation = "rotation_wxyz"
        }
    }

    struct Feature: Codable, Equatable, Sendable {
        var featureID: String
        var rowID: String
        var kind: String
        var xyz: [Double]
        var closed: Bool
        var tags: [String: String]
        var author: String
        var editedAt: String
        enum CodingKeys: String, CodingKey {
            case featureID = "feature_id"
            case rowID = "row_id"
            case kind, xyz, closed, tags, author
            case editedAt = "edited_at"
        }
    }

    var requestID: String
    var writer = "painter"
    var sensorID: String
    var recordID: String
    var pose: Pose
    var author: String
    var gerardusVersion: String
    var gerardusAddress: String
    var features: [Feature]

    enum CodingKeys: String, CodingKey {
        case requestID = "request_id"
        case writer
        case sensorID = "sensor_id"
        case recordID = "record_id"
        case pose, author
        case gerardusVersion = "gerardus_version"
        case gerardusAddress = "gerardus_address"
        case features
    }

    static func kindName(_ kind: Gerardus_Align_V1alpha1_FeatureKind) -> String {
        switch kind {
        case .travelLane: return "travel_lane"
        case .surface: return "surface"
        case .parkingSpace: return "parking_space"
        case .driveway: return "driveway"
        default: return "unspecified"
        }
    }

    /// The features as Gerardus returned them.
    static func features(_ features: [Gerardus_Align_V1alpha1_StreetUseFeature]) -> [Feature] {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return features.map {
            Feature(
                featureID: $0.featureID, rowID: $0.rowID, kind: kindName($0.kind),
                xyz: $0.xyz.map(Double.init), closed: $0.closed, tags: $0.tags, author: $0.author,
                editedAt: $0.hasEditedAt ? f.string(from: $0.editedAt.date) : "")
        }
    }

    static func pose(_ p: Gerardus_Align_V1alpha1_SensorPose) -> Pose {
        let q = p.rotation
        let zero = q.w == 0 && q.x == 0 && q.y == 0 && q.z == 0
        return Pose(
            translation: [p.translationM.x, p.translationM.y, p.translationM.z],
            rotation: zero ? [1, 0, 0, 0] : [q.w, q.x, q.y, q.z])
    }
}

/// What velocity.report answered a write with.
struct LaneSnapshotAnswer: Decodable, Equatable, Sendable {
    struct Direction: Decodable, Equatable, Sendable {
        var id: String
        var name: String
        var lanes: [String]
    }
    var snapshotID: String
    var repeated: Bool
    var lanes: Int
    var directions: [Direction]
    enum CodingKeys: String, CodingKey {
        case snapshotID = "snapshot_id"
        case repeated, lanes, directions
    }
}
