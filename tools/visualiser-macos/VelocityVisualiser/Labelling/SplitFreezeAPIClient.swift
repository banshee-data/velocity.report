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

/// What freezing a draft would pin, and what stops it.
struct FreezePreview: Decodable, Equatable {
    struct PhysicalPin: Decodable, Equatable {
        struct Object: Decodable, Equatable, Identifiable {
            var objectID: String
            var body: String
            var bodyIndependent: Bool
            var keyframes: Int
            var reviewedKeyframes: Int
            var proposedKeyframes: Int
            var trackerAssistedKeyframes: Int
            var id: String { objectID }
            enum CodingKeys: String, CodingKey {
                case objectID = "object_id"
                case body
                case bodyIndependent = "body_independent"
                case keyframes
                case reviewedKeyframes = "reviewed_keyframes"
                case proposedKeyframes = "proposed_keyframes"
                case trackerAssistedKeyframes = "tracker_assisted_keyframes"
            }
        }
        var revision: Int
        var sha256: String
        var contentSHA256: String
        var objects: [Object]
        /// Per component, how many reviewed keyframes leave it unavailable.
        var coverage: [String: Int]
        enum CodingKeys: String, CodingKey {
            case revision
            case sha256
            case contentSHA256 = "content_sha256"
            case objects
            case coverage
        }
    }
    struct Pack: Decodable, Equatable, Identifiable {
        struct Object: Decodable, Equatable {
            var objectID: String
            var partition: String
            enum CodingKeys: String, CodingKey {
                case objectID = "object_id"
                case partition
            }
        }
        var packDigest: String
        var datasetID: String
        var sidecarRevision: Int
        var sidecarSHA256: String
        var physical: PhysicalPin?
        var objects: [Object]
        var id: String { packDigest }
        enum CodingKeys: String, CodingKey {
            case packDigest = "pack_digest"
            case datasetID = "dataset_id"
            case sidecarRevision = "sidecar_revision"
            case sidecarSHA256 = "sidecar_sha256"
            case physical
            case objects
        }
    }

    var packs: [Pack]
    var membershipProblems: [String]
    var physicalProblems: [String]
    var wouldFreeze: Bool
    var splitDigest: String?

    enum CodingKeys: String, CodingKey {
        case packs
        case membershipProblems = "membership_problems"
        case physicalProblems = "physical_problems"
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
