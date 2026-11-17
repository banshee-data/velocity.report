import Combine
import Foundation

/// Local operator observations bound to immutable offline evidence. These records
/// do not change pairing, DRAC applicability, actor IDs or physical qualification.
struct FollowingAudit: Codable, Equatable {
    struct Binding: Codable, Equatable {
        let evidenceSHA256: String
        let siteNamespace: String
        let siteID: String
        let sourceID: String
        let bindingID: String
        let policyID: String
        let analysisBuildGitSHA: String
        let estimateStage: String
        let estimatorID: String
        let observationModelID: String
        let methodID: String
        let parameterHash: String

        init(evidence: FollowingEvidence, sha256: String) {
            evidenceSHA256 = sha256
            siteNamespace = evidence.context.siteNamespace
            siteID = evidence.context.siteId
            sourceID = evidence.context.sourceId
            bindingID = evidence.context.bindingId
            policyID = evidence.context.policyId
            analysisBuildGitSHA = evidence.context.analysisBuildGitSha
            estimateStage = evidence.analysisVersion.estimateStage
            estimatorID = evidence.analysisVersion.estimatorId
            observationModelID = evidence.analysisVersion.obsModelId
            methodID = evidence.analysisVersion.methodId
            parameterHash = evidence.analysisVersion.paramHash
        }
    }

    enum Observation: String, Codable, CaseIterable {
        case unknown, consistent, disputed, notVisible
        var title: String {
            switch self {
            case .unknown: return "Unknown"
            case .consistent: return "Consistent with visible evidence"
            case .disputed: return "Disputed"
            case .notVisible: return "Not visible / insufficient evidence"
            }
        }
    }

    enum Conclusion: String, Codable, CaseIterable {
        case insufficientEvidence, consistent, disagreement
        var title: String {
            switch self {
            case .insufficientEvidence: return "Insufficient evidence"
            case .consistent: return "Consistent at reviewed scope"
            case .disagreement: return "Disagreement to investigate"
            }
        }
    }

    struct Playback: Codable, Equatable {
        var runID: String?
        var sourcePath: String?
        var recordingPath: String?
        var captureNanos: Int64 = 0
        var startNanos: Int64 = 0
        var endNanos: Int64 = 0
        var coordinateFrameID: String?
        var frameSourceIDs: [String] = []
        // This is viewing context, never an automatic source/version match.
    }

    struct Record: Codable, Equatable, Identifiable {
        var id = UUID()
        var reviewer = ""
        var savedAt = Date()
        var followerTrackID: String
        var proposedLeaderTrackID: String?
        var startNanos: Int64
        var endExclusiveNanos: Int64
        var identityContinuity: Observation = .unknown
        var leaderAndPath: Observation = .unknown
        var visibleEndpoints: Observation = .unknown
        var speedEvidence: Observation = .unknown
        var conclusion: Conclusion = .insufficientEvidence
        var notes = ""
        var playback: Playback

        init(row: FollowingEvidence.Row, playback: Playback) {
            followerTrackID = row.followerTrackId
            startNanos = row.captureUnixNanos
            let end = row.captureUnixNanos.addingReportingOverflow(row.intervalNanos)
            endExclusiveNanos = end.overflow ? startNanos : end.partialValue
            self.playback = playback
        }
    }

    var schema = "following_audit_v1"
    let binding: Binding
    var records: [Record] = []

    /// Periods are half-open; a zero-duration terminal sample is an instant.
    /// Gaps and suppressed rows remain in the immutable export. No interval is
    /// interpolated, joined to another track, or converted to observed exposure.
    static func validate(_ record: Record, evidence: FollowingEvidence) throws {
        let samples = evidence.rows.filter { $0.followerTrackId == record.followerTrackID }
        guard samples.contains(where: { $0.captureUnixNanos == record.startNanos }),
            record.endExclusiveNanos >= record.startNanos,
            let last = samples.max(by: { $0.captureUnixNanos < $1.captureUnixNanos })
        else {
            throw FollowingEvidence.ReviewError.invalid(
                "Audit must start at an exact follower sample and have an ordered period.")
        }
        let limit = last.captureUnixNanos.addingReportingOverflow(last.intervalNanos)
        guard !limit.overflow, record.endExclusiveNanos <= limit.partialValue else {
            throw FollowingEvidence.ReviewError.invalid(
                "Audit period exceeds this follower's exported evidence.")
        }
        if let proposed = record.proposedLeaderTrackID {
            guard
                samples.first(where: { $0.captureUnixNanos == record.startNanos })?.decision
                    .candidates?.contains(where: { $0.trackId == proposed }) == true,
                proposed != record.followerTrackID
            else {
                throw FollowingEvidence.ReviewError.invalid(
                    "The proposed leader must be a candidate at this exact sample.")
            }
        }
        try validateContent(record)
    }

    static func validateContent(_ record: Record) throws {
        guard !record.followerTrackID.isEmpty, record.startNanos <= record.endExclusiveNanos else {
            throw FollowingEvidence.ReviewError.invalid("Invalid review identity or period.")
        }
        guard !record.reviewer.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
            !record.notes.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
            record.reviewer.utf8.count <= 256, record.notes.utf8.count <= 16_384,
            record.savedAt.timeIntervalSince1970.isFinite
        else {
            throw FollowingEvidence.ReviewError.invalid(
                "Supply a reviewer label and bounded notes describing observations and uncertainty."
            )
        }
        if record.conclusion == .consistent {
            guard
                [
                    record.identityContinuity, record.leaderAndPath, record.visibleEndpoints,
                    record.speedEvidence,
                ].allSatisfy({ $0 == .consistent })
            else {
                throw FollowingEvidence.ReviewError.invalid(
                    "A consistent conclusion requires consistent observations in every field; retain unknowns otherwise."
                )
            }
        }
    }

    mutating func append(_ record: Record, evidence: FollowingEvidence) throws {
        guard binding == Binding(evidence: evidence, sha256: binding.evidenceSHA256) else {
            throw FollowingEvidence.ReviewError.invalid(
                "Audit belongs to another source or analysis version.")
        }
        try Self.validate(record, evidence: evidence)
        guard records.count < 10_000, !records.contains(where: { $0.id == record.id }) else {
            throw FollowingEvidence.ReviewError.invalid(
                "Duplicate audit record or audit limit reached.")
        }
        var updated = self
        updated.records.append(record)
        _ = try updated.encoded()  // Every saved audit must remain reopenable.
        self = updated
    }

    func encoded() throws -> Data {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
        let data = try encoder.encode(self)
        guard data.count <= 16 * 1024 * 1024 else {
            throw FollowingEvidence.ReviewError.invalid(
                "Audit exceeds the 16 MiB limit; shorten the new review notes or start another audit."
            )
        }
        return data
    }

    static func decode(
        _ data: Data, evidence: FollowingEvidence, sha256: String, validatePeriods: Bool = true
    ) throws -> Self {
        guard data.count <= 16 * 1024 * 1024 else {
            throw FollowingEvidence.ReviewError.invalid("Audit exceeds the 16 MiB limit.")
        }
        let decoder = jsonDecoder()
        let audit = try decoder.decode(Self.self, from: data)
        guard audit.schema == "following_audit_v1",
            audit.binding == Binding(evidence: evidence, sha256: sha256),
            audit.records.count <= 10_000, Set(audit.records.map(\.id)).count == audit.records.count
        else {
            throw FollowingEvidence.ReviewError.invalid(
                "Audit belongs to different evidence bytes, source or analysis version, or has conflicting records."
            )
        }
        for record in audit.records {
            if validatePeriods {
                try validate(record, evidence: evidence)
            } else {
                try validateContent(record)
            }
        }
        return audit
    }
    static func jsonDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let value = try decoder.singleValueContainer().decode(String.self)
            let fractional = ISO8601DateFormatter()
            fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let date = fractional.date(from: value) ?? ISO8601DateFormatter().date(from: value) {
                return date
            }
            throw FollowingEvidence.ReviewError.invalid("Invalid UTC review timestamp.")
        }
        return decoder
    }

}

/// Owned by AppState so hiding the Inspector preserves evidence and unsaved
/// observations. Explicit file replacement asks before discarding them.
@MainActor final class FollowingReviewSession: ObservableObject {
    @Published private(set) var loading = false
    @Published var error: String?
    @Published var document: FollowingEvidence?
    @Published var fileIdentity = ""
    @Published var evidenceURL: URL?
    @Published var timestamp = ""
    @Published var follower = ""
    @Published var times: [Int64] = []
    @Published var audit: FollowingAudit?
    @Published var auditUnsaved = false
    @Published var managedMetadata: FollowingReviewMetadata?
    @Published var managedGeometry: FollowingReviewGeometry?
    @Published var managedRunID: String?
    @Published var loadingWindow = false
    @Published var savingRecord = false
    @Published var pendingSave: FollowingPendingSave?
    @Published var managedDraft: FollowingReviewDraft?
    var managedTransport: FollowingReviewTransport?
    var managedGeneration = 0
    var windowTask: Task<Void, Never>?
    var sampleIndices: [String: [Int64: Int]] = [:]
    var geometryIndex: [String: FollowingReviewGeometry.Path] = [:]
    @Published var followerChoices: [String] = []

    init() { UnsavedWorkRegistry.shared.register(self) }

    /// Import ownership survives Inspector removal/recreation. Only its owner
    /// finishes the import; another view cannot start an overlapping load.
    func beginImport() -> Bool {
        guard !loading else { return false }
        loading = true
        error = nil
        return true
    }

    func finishImport() { loading = false }
}
