// SplitFreezeAPIClient.swift
// Client for freezing a split through the local service:
//
//   POST /api/annotations/split/preview
//   POST /api/annotations/split/freeze
//   GET  /api/annotations/splits
//
// A draft names packs by handle beneath the service's annotation packs
// directory, and the frozen split is written there too, write-once. The
// preview is the freeze without the write, so what the window shows is what
// the freeze would pin.

import Foundation

/// A frozen split as the listing describes it.
struct FrozenSplitListing: Decodable, Equatable, Identifiable {
    var name: String
    var revision: Int
    var splitDigest: String
    var frozenUTC: String
    var author: String
    var packs: Int
    var error: String?

    var id: String { name }

    enum CodingKeys: String, CodingKey {
        case name
        case revision
        case splitDigest = "split_digest"
        case frozenUTC = "frozen_utc"
        case author
        case packs
        case error
    }
}

/// A pack's physical-reference pin, as a frozen split records it: the
/// revision, its digests, what each object's records review, and how many
/// reviewed keyframes leave each component unavailable.
struct FrozenPhysicalPin: Decodable, Equatable {
    struct Object: Decodable, Equatable, Identifiable {
        struct Body: Decodable, Equatable {
            var status: String
            var independent: Bool
        }
        struct Keyframes: Decodable, Equatable {
            var total: Int
            var reviewed: Int
            var proposed: Int
            var trackerAssisted: Int
            enum CodingKeys: String, CodingKey {
                case total
                case reviewed
                case proposed
                case trackerAssisted = "tracker_assisted"
            }
        }
        var objectID: String
        var body: Body
        var keyframes: Keyframes
        var id: String { objectID }
        enum CodingKeys: String, CodingKey {
            case objectID = "object_id"
            case body
            case keyframes
        }
    }
    struct ComponentCoverage: Decodable, Equatable {
        var scorable: Int
        var unavailable: Int
    }
    struct Coverage: Decodable, Equatable {
        var position: ComponentCoverage
        var yaw: ComponentCoverage
        var length: ComponentCoverage
        var width: ComponentCoverage
        var height: ComponentCoverage
        var front: ComponentCoverage
        var rear: ComponentCoverage

        /// Components in reading order, with the count each leaves unavailable.
        var unavailableByComponent: [(name: String, coverage: ComponentCoverage)] {
            [
                ("position", position), ("yaw", yaw), ("length", length), ("width", width),
                ("height", height), ("front", front), ("rear", rear),
            ]
        }
    }
    var revision: Int
    var sha256: String
    var contentSHA256: String
    var objects: [Object]
    var coverage: Coverage
    enum CodingKeys: String, CodingKey {
        case revision
        case sha256
        case contentSHA256 = "content_sha256"
        case objects
        case coverage
    }
}

/// Facet proposals stay separate from physical reference review.
struct FrozenFacetPin: Decodable, Equatable {
    var revision: UInt64
    var sha256: String
    var contentSHA256: String
    var candidates: Int
    var active: Int
    var supportedObservations: Int
    var absenceDecisions: Int
    var registrations: Int
    var trackerSeededRegistrations: Int
    /// Registrations made against another physical revision than the split
    /// pins (or while it pins none). Recorded in the split, never a refusal.
    var physicalDivergences: [FacetPhysicalDivergence]? = nil
    enum CodingKeys: String, CodingKey {
        case revision, sha256, candidates, active, registrations
        case contentSHA256 = "content_sha256"
        case supportedObservations = "supported_observations"
        case absenceDecisions = "absence_decisions"
        case trackerSeededRegistrations = "tracker_seeded_registrations"
        case physicalDivergences = "physical_divergences"
    }

    /// One line for the freeze sheet, or nil when every registration names
    /// the pinned physical revision.
    var physicalDivergenceSummary: String? {
        guard let divergences = physicalDivergences, !divergences.isEmpty else { return nil }
        let revisions = Set(divergences.map(\.physicalRevision)).sorted().map(String.init).joined(
            separator: ", ")
        let noun = divergences.count == 1 ? "registration" : "registrations"
        return
            "\(divergences.count) body \(noun) made against physical revision \(revisions), not this split's pin · recorded, not refused"
    }
}

/// A body registration whose physical revision is not the split's pin.
struct FacetPhysicalDivergence: Decodable, Equatable {
    var featureID: String
    var objectID: String
    var physicalRevision: UInt64
    var physicalDigest: String
    enum CodingKeys: String, CodingKey {
        case featureID = "feature_id"
        case objectID = "object_id"
        case physicalRevision = "physical_revision"
        case physicalDigest = "physical_digest"
    }
}

/// What freezing a draft would pin, and what stops it.
struct FreezePreview: Decodable, Equatable {
    struct Pack: Decodable, Equatable, Identifiable {
        struct Object: Decodable, Equatable {
            var objectID: String
            var partition: String
            var objectClass: String
            var reviewedMasks: Int
            enum CodingKeys: String, CodingKey {
                case objectID = "object_id"
                case partition
                case objectClass = "class"
                case reviewedMasks = "reviewed_masks"
            }
        }
        var packDigest: String
        var datasetID: String
        var caseID: String?
        var sidecarRevision: Int
        var sidecarSHA256: String
        var physical: FrozenPhysicalPin?
        var features: FrozenFacetPin? = nil
        var objects: [Object]
        var episodes: Int
        var id: String { packDigest }
        enum CodingKeys: String, CodingKey {
            case packDigest = "pack_digest"
            case datasetID = "dataset_id"
            case caseID = "case_id"
            case sidecarRevision = "sidecar_revision"
            case sidecarSHA256 = "sidecar_sha256"
            case physical
            case features
            case objects
            case episodes
        }
    }

    var packs: [Pack]
    var membershipProblems: [String]
    var physicalProblems: [String]
    var facetProblems: [String]? = nil
    /// A refusal the freeze would give after review passes, such as a
    /// lineage rule; reported here rather than returned as an error.
    var refusal: String?
    var wouldFreeze: Bool
    var splitDigest: String?

    enum CodingKeys: String, CodingKey {
        case packs
        case membershipProblems = "membership_problems"
        case physicalProblems = "physical_problems"
        case facetProblems = "facet_problems"
        case refusal
        case wouldFreeze = "would_freeze"
        case splitDigest = "split_digest"
    }
}

/// What a freeze wrote.
struct FreezeResult: Decodable, Equatable {
    var name: String
    var path: String
    var splitDigest: String
    var revision: Int
    enum CodingKeys: String, CodingKey {
        case name
        case path
        case splitDigest = "split_digest"
        case revision
    }
}

/// The body of a refusal: prose for a person, a code for the client.
private struct SplitServiceError: Decodable {
    var error: String?
    var code: String?
}

struct SplitFreezeAPIClient {
    let baseURL: URL
    private let session: URLSession

    init(
        baseURL: URL = PhysicalReferenceAPIClient.defaultBaseURL,
        session: URLSession = APISession.shared
    ) {
        self.baseURL = baseURL
        self.session = session
    }

    /// The draft as the operator's file holds it, sent through untouched:
    /// the service resolves its pack handles and applies every freeze rule.
    typealias Draft = [String: Any]

    func preview(draft: Draft, supersedes: String?) async throws -> FreezePreview {
        var body: [String: Any] = ["draft": draft]
        if let supersedes { body["supersedes"] = supersedes }
        return try await post("api/annotations/split/preview", body, as: FreezePreview.self)
    }

    func freeze(
        draft: Draft, author: String, output: String, supersedes: String?
    ) async throws -> FreezeResult {
        var body: [String: Any] = ["draft": draft, "author": author, "output": output]
        if let supersedes { body["supersedes"] = supersedes }
        return try await post("api/annotations/split/freeze", body, as: FreezeResult.self)
    }

    func splits() async throws -> [FrozenSplitListing] {
        var request = URLRequest(url: baseURL.appendingPathComponent("api/annotations/splits"))
        request.httpMethod = "GET"
        struct Listing: Decodable { var splits: [FrozenSplitListing] }
        return try await send(request, as: Listing.self).splits
    }

    /// Reads a draft file as the service will see it: an object, or nothing.
    static func loadDraft(at url: URL) throws -> Draft {
        let data = try Data(contentsOf: url)
        guard let object = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw PhysicalReferenceAPIError.decoding("the draft is not a JSON object")
        }
        return object
    }

    private func post<Result: Decodable>(
        _ path: String, _ body: [String: Any], as: Result.Type
    ) async throws -> Result {
        var request = URLRequest(url: baseURL.appendingPathComponent(path))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        do { request.httpBody = try JSONSerialization.data(withJSONObject: body) } catch {
            throw PhysicalReferenceAPIError.decoding("could not encode the request: \(error)")
        }
        return try await send(request, as: Result.self)
    }

    private func send<Result: Decodable>(
        _ request: URLRequest, as: Result.Type
    ) async throws -> Result {
        let data: Data
        let response: URLResponse
        do { (data, response) = try await session.data(for: request) } catch {
            throw PhysicalReferenceAPIError.transport(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else {
            throw PhysicalReferenceAPIError.transport("no HTTP response")
        }
        guard (200...299).contains(http.statusCode) else {
            let body = try? JSONDecoder().decode(SplitServiceError.self, from: data)
            throw PhysicalReferenceAPIError.refused(
                status: http.statusCode, code: body?.code ?? "unknown",
                message: body?.error ?? "request failed")
        }
        do { return try JSONDecoder().decode(Result.self, from: data) } catch {
            throw PhysicalReferenceAPIError.decoding(String(describing: error))
        }
    }
}
