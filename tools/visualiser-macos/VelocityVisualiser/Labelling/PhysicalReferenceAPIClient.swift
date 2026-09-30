// PhysicalReferenceAPIClient.swift
// Client for the local Go service that owns physical references:
//
//   GET  /api/annotations/physical?pack=<handle>&pack_digest=<d>[&revision=<n>]
//   POST /api/annotations/physical/validate
//   POST /api/annotations/physical/save
//   POST /api/annotations/physical/review
//
// The service is the one writer. It validates evidence, keeps the origin
// ledger, resets reviews an edit invalidates, and refuses a stale base or a
// changed membership under the same lock the sidecar's writers take. This
// client reports what it says, with its structured code, and never retries a
// write on its own.

import Foundation

/// Why a physical-reference request failed, in terms a pane can explain.
enum PhysicalReferenceAPIError: Error, LocalizedError, Equatable {
    /// The service answered and refused. `code` is the service's own:
    /// conflict, membership_changed, busy, invalid, pack_mismatch, and so on.
    case refused(status: Int, code: String, message: String)
    /// The edit was refused on its content, with the same diagnostics a
    /// validation returns.
    case invalid(PhysicalEditResult)
    /// No answer arrived. For a write, the commit may or may not have
    /// happened: the caller must re-read before writing again.
    case transport(String)
    case decoding(String)

    var errorDescription: String? {
        switch self {
        case .refused(let status, _, let message): return "\(message) (HTTP \(status))"
        case .invalid(let result):
            return result.invalid ?? result.linkProblems.first.map { "\($0.record): \($0.problem)" }
                ?? "The edit is invalid."
        case .transport(let detail): return "No answer from the annotation service: \(detail)"
        case .decoding(let detail): return "Could not read the service's answer: \(detail)"
        }
    }

    var code: String? {
        if case .refused(_, let code, _) = self { return code }
        return nil
    }
}

/// The body of a refusal: prose for a person, a code for the client.
private struct PhysicalServiceError: Decodable {
    var error: String?
    var code: String?
}

struct PhysicalReferenceAPIClient {
    /// Where the service is. The same default every other client here uses;
    /// `annotation.serviceURL` in the app's defaults overrides it.
    static var defaultBaseURL: URL {
        if let configured = UserDefaults.standard.string(forKey: "annotation.serviceURL"),
            let url = URL(string: configured)
        {
            return url
        }
        return URL(string: "http://localhost:8080")!
    }

    let baseURL: URL
    private let session: URLSession

    init(
        baseURL: URL = PhysicalReferenceAPIClient.defaultBaseURL,
        session: URLSession = APISession.shared
    ) {
        self.baseURL = baseURL
        self.session = session
    }

    /// The handle the service resolves a pack by: its directory relative to
    /// the service's annotation packs directory, which is "<name>" or, for a
    /// pack exported into a named folder, "<name>/pack".
    static func handle(for directory: URL) -> String {
        let last = directory.standardizedFileURL.lastPathComponent
        guard last == "pack" else { return last }
        return directory.standardizedFileURL.deletingLastPathComponent().lastPathComponent + "/pack"
    }

    func load(
        handle: String, packDigest: String, revision: Int? = nil
    ) async throws -> PhysicalPackState {
        var components = URLComponents(
            url: baseURL.appendingPathComponent("api/annotations/physical"),
            resolvingAgainstBaseURL: false)!
        var items = [
            URLQueryItem(name: "pack", value: handle),
            URLQueryItem(name: "pack_digest", value: packDigest),
        ]
        if let revision { items.append(URLQueryItem(name: "revision", value: String(revision))) }
        components.queryItems = items
        // A digest carries a colon and a plus sign never appears in one, but
        // be explicit: the query must reach the server byte for byte.
        components.percentEncodedQuery = components.percentEncodedQuery?.replacingOccurrences(
            of: "+", with: "%2B")
        var request = URLRequest(url: components.url!)
        request.httpMethod = "GET"
        return try await send(request, as: PhysicalPackState.self)
    }

    struct EditRequest: Encodable {
        var pack: String
        var packDigest: String
        var baseRevision: Int
        var baseDigest: String
        var membershipDigest: String
        var author: String
        var session: String
        var objects: [PhysicalObject]

        enum CodingKeys: String, CodingKey {
            case pack
            case packDigest = "pack_digest"
            case baseRevision = "base_revision"
            case baseDigest = "base_digest"
            case membershipDigest = "membership_digest"
            case author
            case session
            case objects
        }
    }

    func validate(_ edit: EditRequest) async throws -> PhysicalEditResult {
        try await post("api/annotations/physical/validate", edit, as: PhysicalEditResult.self)
    }

    func save(_ edit: EditRequest) async throws -> PhysicalEditResult {
        try await post("api/annotations/physical/save", edit, as: PhysicalEditResult.self)
    }

    struct ReviewRequest: Encodable {
        var pack: String
        var packDigest: String
        var baseRevision: Int
        var baseDigest: String
        var membershipDigest: String
        var kind: PhysicalRecordKind
        var objectID: String
        var recordID: String
        var reviewer: String
        var session: String

        enum CodingKeys: String, CodingKey {
            case pack
            case packDigest = "pack_digest"
            case baseRevision = "base_revision"
            case baseDigest = "base_digest"
            case membershipDigest = "membership_digest"
            case kind
            case objectID = "object_id"
            case recordID = "record_id"
            case reviewer
            case session
        }
    }

    func review(_ review: ReviewRequest) async throws -> PhysicalPackState {
        try await post("api/annotations/physical/review", review, as: PhysicalPackState.self)
    }

    func history(handle: String, packDigest: String) async throws -> [PhysicalRevisionSummary] {
        var components = URLComponents(
            url: baseURL.appendingPathComponent("api/annotations/physical/history"),
            resolvingAgainstBaseURL: false)!
        components.queryItems = [
            URLQueryItem(name: "pack", value: handle),
            URLQueryItem(name: "pack_digest", value: packDigest),
        ]
        var request = URLRequest(url: components.url!)
        request.httpMethod = "GET"
        struct History: Decodable { var revisions: [PhysicalRevisionSummary] }
        return try await send(request, as: History.self).revisions
    }

    struct RestoreRequest: Encodable {
        var pack: String
        var packDigest: String
        var baseRevision: Int
        var baseDigest: String
        var membershipDigest: String
        var revision: Int
        var author: String
        var session: String

        enum CodingKeys: String, CodingKey {
            case pack
            case packDigest = "pack_digest"
            case baseRevision = "base_revision"
            case baseDigest = "base_digest"
            case membershipDigest = "membership_digest"
            case revision
            case author
            case session
        }
    }

    func restore(_ restore: RestoreRequest) async throws -> PhysicalPackState {
        try await post("api/annotations/physical/restore", restore, as: PhysicalPackState.self)
    }

    // MARK: Transport

    private func post<Body: Encodable, Result: Decodable>(
        _ path: String, _ body: Body, as: Result.Type
    ) async throws -> Result {
        var request = URLRequest(url: baseURL.appendingPathComponent(path))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        do { request.httpBody = try JSONEncoder().encode(body) } catch {
            // Only a non-finite number fails to encode, and Go would refuse
            // it anyway; say so rather than send something else.
            throw PhysicalReferenceAPIError.decoding("could not encode the edit: \(error)")
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
        let decoder = JSONDecoder()
        if http.statusCode == 422,
            let result = try? decoder.decode(PhysicalEditResult.self, from: data)
        {
            throw PhysicalReferenceAPIError.invalid(result)
        }
        guard (200...299).contains(http.statusCode) else {
            let body = try? decoder.decode(PhysicalServiceError.self, from: data)
            throw PhysicalReferenceAPIError.refused(
                status: http.statusCode, code: body?.code ?? "unknown",
                message: body?.error ?? "request failed")
        }
        do { return try decoder.decode(Result.self, from: data) } catch {
            throw PhysicalReferenceAPIError.decoding(String(describing: error))
        }
    }
}
