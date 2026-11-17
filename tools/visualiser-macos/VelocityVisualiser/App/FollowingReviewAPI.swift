import CryptoKit
import Foundation

/// A recording owns the review. Files and minute boundaries remain server details.
struct FollowingReviewMetadata: Decodable, Sendable {
    struct Slice: Decodable, Sendable {
        let id: String
        let startNanos: Int64
        let endExclusiveNanos: Int64
        let rows: Int
    }
    let schema: String
    let runId: String
    let evidenceSha256: String
    let recordingHeaderSha256: String
    let sourceId: String
    let calibrationId: String
    let startNanos: Int64
    let endExclusiveNanos: Int64
    let slices: [Slice]
}

struct FollowingReviewGeometry: Decodable, Sendable {
    struct Vertex: Decodable, Sendable, Equatable {
        let x: Double
        let y: Double
        let arcM: Double
    }
    struct Path: Decodable, Sendable {
        let id: String
        let vertices: [Vertex]

        /// Projected path coordinates, never an invented box corner. Refuse
        /// outside support rather than extending the fit into unknown space.
        func point(at arc: Double, lateral: Double = 0) -> SIMD2<Double>? {
            guard arc.isFinite, lateral.isFinite, vertices.count >= 2, let first = vertices.first,
                let last = vertices.last, arc >= first.arcM, arc <= last.arcM
            else { return nil }
            for i in 1..<vertices.count where arc <= vertices[i].arcM {
                let a = vertices[i - 1]
                let b = vertices[i]
                let length = b.arcM - a.arcM
                let dx = b.x - a.x
                let dy = b.y - a.y
                let norm = hypot(dx, dy)
                guard length > 0, norm > 0 else { return nil }
                let t = (arc - a.arcM) / length
                return SIMD2(a.x + t * dx - lateral * dy / norm, a.y + t * dy + lateral * dx / norm)
            }
            return nil
        }
    }
    struct Result: Decodable, Sendable {
        let followerTrackId: String
        let path: Path?
        let conditions: [String]?
    }
    let schema: String
    let sourceId: String
    let calibrationId: String
    let canonicalEvidenceSha256: String
    let coordinateFrameId: String
    let recordingCoordinates: Bool
    let paths: [Result]
}

struct FollowingReviewTransport {
    let baseURL: URL
    let session: URLSession

    init(
        baseURL: URL = VelocityReportServer.currentHTTPURL, session: URLSession = APISession.shared
    ) {
        self.baseURL = baseURL
        self.session = session
    }

    func request(runID: String, action: String = "", at: Int64? = nil) -> URLRequest {
        var url = baseURL.appendingPathComponent("api/lidar/runs").appendingPathComponent(runID)
            .appendingPathComponent("following-review")
        if !action.isEmpty { url.appendPathComponent(action) }
        var components = URLComponents(url: url, resolvingAgainstBaseURL: false)!
        if let at { components.queryItems = [URLQueryItem(name: "at_ns", value: String(at))] }
        var request = URLRequest(url: components.url!)
        // First access verifies canonical evidence on directly attached storage.
        request.timeoutInterval = 300
        request.cachePolicy = .reloadIgnoringLocalCacheData
        return request
    }

    func data(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        let (data, response) = try await session.data(for: request)
        guard let response = response as? HTTPURLResponse else {
            throw FollowingEvidence.ReviewError.invalid(
                "The review server returned no HTTP response.")
        }
        guard (200..<300).contains(response.statusCode) else {
            let reason =
                (try? JSONDecoder().decode([String: String].self, from: data)["error"])
                ?? "Review request failed (HTTP \(response.statusCode))."
            throw FollowingEvidence.ReviewError.invalid(reason)
        }
        return (data, response)
    }

    static func decode<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(type, from: data)
    }

    func metadata(runID: String) async throws -> FollowingReviewMetadata {
        let (bytes, _) = try await data(request(runID: runID))
        let metadata = try Self.decode(FollowingReviewMetadata.self, from: bytes)
        guard metadata.schema == "following_review_session_v1", metadata.runId == runID,
            metadata.startNanos > 0, metadata.startNanos < metadata.endExclusiveNanos,
            !metadata.sourceId.isEmpty, !metadata.calibrationId.isEmpty, !metadata.slices.isEmpty,
            Set(metadata.slices.map(\.id)).count == metadata.slices.count,
            metadata.slices.allSatisfy({
                !$0.id.isEmpty && $0.startNanos >= metadata.startNanos
                    && $0.endExclusiveNanos <= metadata.endExclusiveNanos
                    && $0.startNanos < $0.endExclusiveNanos && $0.rows > 0
            })
        else {
            throw FollowingEvidence.ReviewError.invalid("The review does not match this recording.")
        }
        return metadata
    }

    func evidence(
        runID: String, at: Int64, metadata: FollowingReviewMetadata
    ) async throws -> FollowingEvidence {
        let (bytes, response) = try await data(request(runID: runID, action: "evidence", at: at))
        let sha = "sha256:" + SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined()
        guard response.value(forHTTPHeaderField: "X-Evidence-SHA256") == sha,
            response.value(forHTTPHeaderField: "X-Canonical-Evidence-SHA256")
                == metadata.evidenceSha256
        else {
            throw FollowingEvidence.ReviewError.invalid(
                "Evidence verification failed; reload the review.")
        }
        let doc = try FollowingEvidence.decode(bytes)
        guard let slice = doc.reviewSlice, slice.canonicalEvidenceSha256 == metadata.evidenceSha256,
            slice.recordingHeaderSha256 == metadata.recordingHeaderSha256,
            slice.calibrationId == metadata.calibrationId,
            doc.context.sourceId == metadata.sourceId, at >= slice.sampleStartNanos,
            at < slice.sampleEndExclusiveNanos
        else {
            throw FollowingEvidence.ReviewError.invalid(
                "Evidence belongs to another recording or window.")
        }
        return doc
    }

    func geometry(
        runID: String, at: Int64, metadata: FollowingReviewMetadata
    ) async throws -> FollowingReviewGeometry {
        let (bytes, _) = try await data(request(runID: runID, action: "geometry", at: at))
        let geometry = try Self.decode(FollowingReviewGeometry.self, from: bytes)
        guard geometry.schema == "following_review_geometry_v1",
            geometry.sourceId == metadata.sourceId,
            geometry.calibrationId == metadata.calibrationId,
            geometry.canonicalEvidenceSha256 == metadata.evidenceSha256
        else {
            throw FollowingEvidence.ReviewError.invalid("Path geometry belongs to another review.")
        }
        for result in geometry.paths {
            guard let path = result.path else { continue }
            guard path.vertices.count >= 2, !path.id.isEmpty,
                path.vertices.allSatisfy({ $0.x.isFinite && $0.y.isFinite && $0.arcM.isFinite }),
                path.vertices.first?.arcM == 0,
                zip(path.vertices, path.vertices.dropFirst()).allSatisfy({ a, b in
                    let distance = hypot(b.x - a.x, b.y - a.y)
                    return a.arcM < b.arcM && distance > 0
                        && abs(b.arcM - a.arcM - distance) <= 1e-7 * max(1, b.arcM)
                })
            else { throw FollowingEvidence.ReviewError.invalid("Review path geometry is invalid.") }
        }
        return geometry
    }

    func audit(
        runID: String, evidence: FollowingEvidence, sha: String
    ) async throws -> FollowingAudit {
        let (bytes, _) = try await data(request(runID: runID, action: "audit"))
        return try FollowingAudit.decode(
            bytes, evidence: evidence, sha256: sha, validatePeriods: false)
    }

    func append(
        runID: String, binding: FollowingAudit.Binding, record: FollowingAudit.Record
    ) async throws {
        struct Append: Codable {
            let binding: FollowingAudit.Binding
            let record: FollowingAudit.Record
        }
        var request = request(runID: runID, action: "audit")
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        request.httpBody = try encoder.encode(Append(binding: binding, record: record))
        struct Receipt: Decodable {
            let created: Bool
            let record: FollowingAudit.Record
        }
        let expected = try FollowingAudit.jsonDecoder().decode(Append.self, from: request.httpBody!)
        let (bytes, _) = try await data(request)
        let receipt = try FollowingAudit.jsonDecoder().decode(Receipt.self, from: bytes)
        guard receipt.record == expected.record else {
            throw FollowingEvidence.ReviewError.invalid(
                "The server did not confirm the exact review record; retry without changing it.")
        }
    }
}
