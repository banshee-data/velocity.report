// Read-only view of the authoritative offline following export. No arithmetic,
// lane attribution, actor joining or nearest-time interpolation happens here.
import CryptoKit
import Foundation

struct FollowingEvidence: Decodable, Sendable {
    struct ReviewSlice: Decodable, Sendable {
        let canonicalEvidenceSha256: String
        let sampleStartNanos: Int64
        let sampleEndExclusiveNanos: Int64
        let recordingHeaderSha256: String
        let recordingBuildGitSha: String
        let calibrationId: String
    }
    struct SourceManifest: Decodable, Sendable {
        struct Window: Decodable, Sendable {
            let startOffsetNanos: Int64
            let warmupNanos: Int64
            let durationNanos: Int64
            enum CodingKeys: String, CodingKey {
                case startOffsetNanos = "StartOffsetNanos"
                case warmupNanos = "WarmupNanos"
                case durationNanos = "DurationNanos"
            }
        }
        let capturePaths: [String]
        let captureSha256s: [String]
        let extractorId: String
        let window: Window?
        enum CodingKeys: String, CodingKey {
            case capturePaths = "CapturePaths"
            case captureSha256s = "CaptureSHA256s"
            case extractorId = "ExtractorID"
            case window = "Window"
        }
    }
    struct Context: Decodable, Sendable {
        let siteNamespace: String
        let siteId: String
        let sourceId: String
        let bindingId: String
        let analysisBuildGitSha: String
        let policyId: String
        let sourceManifest: SourceManifest?
    }
    struct Version: Decodable, Sendable {
        let estimateStage: String
        let estimatorId: String
        let obsModelId: String
        let methodId: String
        let paramHash: String
    }
    struct Candidate: Decodable, Sendable {
        let trackId: String
        let disposition: String
        let established: Bool
        let synchronised: Bool
        let centreArcM: Double?
        let lateralM: Double?
    }
    struct Decision: Decodable, Sendable {
        let followerTrackId: String
        let captureUnixNanos: Int64
        let leaderTrackId: String?
        let reason: String?
        let condition: String?
        let candidates: [Candidate]?
    }
    struct Endpoint: Decodable, Sendable {
        let trackId: String
        let arcM: Double
        let sigmaM: Double
        let source: String
        let support: String
        let extentConverged: Bool
        let lengthProvenance: String?
        let widthProvenance: String?
    }
    struct Body: Decodable, Sendable {
        let leading: Endpoint
        let trailing: Endpoint
    }
    struct Gap: Decodable, Sendable {
        let valueM: Double
        let sigmaM: Double
        let reason: String?
    }
    struct TimeGap: Decodable, Sendable {
        let valueS: Double?
        let sigmaS: Double?
        let reason: String?
    }
    struct Point: Decodable, Sendable {
        let followerTrackId: String
        let leaderTrackId: String
        let captureUnixNanos: Int64
        let gap: Gap?
        let timeGap: TimeGap?
        let leader: Body?
        let follower: Body?
        let reasons: [String]?
        let coastAgeNanos: Int64?
        let supportedOpportunity: Bool
    }
    struct Calculation: Decodable, Sendable {
        struct Bounds: Decodable, Sendable {
            let lower: Double
            let upper: Double
        }
        let status: String
        let valueMps2: Double?
        let closingSpeedMps: Double?
        let boundsMps2: Bounds?
    }
    struct DRAC: Decodable, Sendable {
        let status: String
        let evidenceReason: String?
        let calculation: Calculation?
    }
    struct Row: Decodable, Sendable {
        let followerTrackId: String
        let captureUnixNanos: Int64
        let intervalNanos: Int64
        let bucket: String
        let decision: Decision
        let point: Point?
        let followerSpeedMps: Double?
        let leaderSpeedMps: Double?
        let experimentalDrac: DRAC
    }
    struct Analysis: Decodable, Sendable {
        struct Timeline: Decodable, Sendable {
            let followerTrackId: String
            let geometryId: String?
        }
        let timelines: [Timeline]
    }
    let schema: String
    let context: Context
    let analysisVersion: Version
    let analysis: Analysis
    let rows: [Row]
    let reviewSlice: ReviewSlice?

    static func decode(_ data: Data) throws -> FollowingEvidence {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let doc = try decoder.decode(Self.self, from: data)
        guard ["following_evidence_v1", "following_review_slice_v1"].contains(doc.schema) else {
            throw ReviewError.invalid("Unsupported evidence schema")
        }
        if doc.schema == "following_review_slice_v1" {
            func isSHA(_ value: String) -> Bool {
                value.hasPrefix("sha256:") && value.count == 71
                    && value.dropFirst(7).allSatisfy { $0.isHexDigit }
            }
            guard let slice = doc.reviewSlice, isSHA(slice.canonicalEvidenceSha256),
                isSHA(slice.recordingHeaderSha256), !slice.recordingBuildGitSha.isEmpty,
                !slice.calibrationId.isEmpty,
                slice.sampleStartNanos < slice.sampleEndExclusiveNanos,
                doc.rows.allSatisfy({
                    $0.captureUnixNanos >= slice.sampleStartNanos
                        && $0.captureUnixNanos < slice.sampleEndExclusiveNanos
                })
            else {
                throw ReviewError.invalid(
                    "Missing slice provenance or samples outside its declared window")
            }
        } else if doc.reviewSlice != nil {
            throw ReviewError.invalid("Slice metadata requires the review-slice schema")
        }
        guard
            [
                doc.context.siteNamespace, doc.context.siteId, doc.context.sourceId,
                doc.context.bindingId, doc.context.analysisBuildGitSha, doc.context.policyId,
                doc.analysisVersion.estimateStage, doc.analysisVersion.estimatorId,
                doc.analysisVersion.obsModelId, doc.analysisVersion.methodId,
                doc.analysisVersion.paramHash,
            ].allSatisfy({ !$0.isEmpty }), !doc.rows.isEmpty
        else { throw ReviewError.invalid("Missing evidence identity or timeline") }
        var seen: [String: Set<Int64>] = [:]
        for row in doc.rows {
            let candidates = row.decision.candidates ?? []
            guard !row.followerTrackId.isEmpty, row.intervalNanos >= 0,
                row.decision.followerTrackId == row.followerTrackId,
                row.decision.captureUnixNanos == row.captureUnixNanos,
                seen[row.followerTrackId, default: []].insert(row.captureUnixNanos).inserted,
                Set(candidates.map(\.trackId)).count == candidates.count,
                candidates.allSatisfy({ !$0.trackId.isEmpty && $0.trackId != row.followerTrackId })
            else { throw ReviewError.invalid("Conflicting follower, time or candidate identities") }
            if let leader = row.decision.leaderTrackId, !leader.isEmpty,
                !candidates.contains(where: { $0.trackId == leader })
            {
                throw ReviewError.invalid("Selected leader is absent from candidate identities")
            }
            if let point = row.point {
                guard point.followerTrackId == row.followerTrackId,
                    point.leaderTrackId == row.decision.leaderTrackId,
                    point.captureUnixNanos == row.captureUnixNanos
                else { throw ReviewError.invalid("Point differs from pairing identity") }
            }
            if let value = row.experimentalDrac.calculation?.valueMps2 {
                guard row.experimentalDrac.status == "supported",
                    row.experimentalDrac.calculation?.status == "supported", value >= 0
                else { throw ReviewError.invalid("Suppressed DRAC carries a value") }
            }
        }
        return doc
    }

    enum ReviewError: LocalizedError {
        case invalid(String)
        var errorDescription: String? {
            if case .invalid(let reason) = self { return reason }
            return nil
        }
    }

    func rows(at nanos: Int64, follower: String) -> [Row] {
        rows.filter {
            $0.captureUnixNanos == nanos && (follower.isEmpty || $0.followerTrackId == follower)
        }.sorted { $0.followerTrackId < $1.followerTrackId }
    }

    /// Source metadata alone cannot establish estimator/version agreement.
    func frameSourceNote(_ sources: Set<String>) -> String {
        if sources.isEmpty {
            return "Recording source identity unavailable; evidence is not bound to playback."
        }
        if sources != [context.sourceId] {
            return "Recording source differs; evidence is not bound to playback."
        }
        return
            "Frame source matches. Recording and estimator/version agreement still require verification."
    }
}
