// AnnotationExportAPIClient.swift
// Client for POST /api/lidar/runs/{run_id}/annotation-export.
//
// This is what lets a pack be produced from a run inside the app, rather than
// requiring the operator to find the run's VRLOG directory on disk and run
// `velocity lidar annotation-export` separately first. The endpoint wraps the
// same Go export function the CLI calls; this client is the thin piece that
// reaches it.

import Foundation

/// One run's exported pack: where it landed, and enough of the manifest to
/// decide whether it is worth opening without a second round trip.
struct AnnotationExportResult: Decodable {
    let packDir: String
    let datasetID: String
    let sampleCount: Int
    let pointCount: Int64
    let coverage: String
    let framesWithoutPoints: Int
    let hasIntensity: Bool
    let hasClassification: Bool

    enum CodingKeys: String, CodingKey {
        case packDir = "pack_dir"
        case datasetID = "dataset_id"
        case sampleCount = "sample_count"
        case pointCount = "point_count"
        case coverage
        case framesWithoutPoints = "frames_without_points"
        case hasIntensity = "has_intensity"
        case hasClassification = "has_classification"
    }
}

/// The three coverage values the exporter accepts. Mirrors
/// internal/lidar/annotation.CaptureCoverage; kept as a Swift enum so the
/// picker cannot send anything the server would reject.
enum AnnotationCoverage: String, CaseIterable, Identifiable {
    case full
    case foregroundOnly = "foreground_only"
    case decimated

    var id: String { rawValue }

    var label: String {
        switch self {
        case .full: return "Full scene"
        case .foregroundOnly: return "Foreground only"
        case .decimated: return "Decimated"
        }
    }
}

/// REST client for the annotation-export endpoint.
///
/// Deliberately separate from RunTrackLabelAPIClient rather than added to it:
/// export is a write that creates a new artefact on disk, not a label
/// mutation on an existing run, and the error path needs the server's actual
/// message (a missing VRLOG, an empty window) rather than a bare status code.
struct AnnotationExportAPIClient {
    private let baseURL: URL
    private let session: URLSession

    init(
        baseURL: URL = URL(string: "http://localhost:8080")!,
        session: URLSession = APISession.shared
    ) {
        self.baseURL = baseURL
        self.session = session
    }

    /// Requests a pack for `runID`. `maxSamples` of 0 takes the server's own
    /// default (200): the cap exists so a UI button cannot produce a pack
    /// nobody will ever finish labelling, and 0 asking to disable it entirely
    /// would defeat that from this side instead.
    func exportPack(
        runID: String, coverage: AnnotationCoverage, coverageNote: String = "", maxSamples: Int = 0
    ) async throws -> AnnotationExportResult {
        let url = baseURL.appendingPathComponent("api/lidar/runs/\(runID)/annotation-export")

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")

        var body: [String: Any] = ["coverage": coverage.rawValue]
        if !coverageNote.isEmpty { body["coverage_note"] = coverageNote }
        if maxSamples > 0 { body["max_samples"] = maxSamples }
        request.httpBody = try JSONSerialization.data(withJSONObject: body)

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw AnnotationExportError.requestFailed(status: -1, message: "no HTTP response")
        }
        guard (200...299).contains(http.statusCode) else {
            throw AnnotationExportError.requestFailed(
                status: http.statusCode, message: Self.serverMessage(from: data))
        }

        // No .convertFromSnakeCase here: AnnotationExportResult already
        // declares explicit CodingKeys mapping camelCase properties to the
        // wire's snake_case names. That strategy transforms an incoming key
        // like "pack_dir" to "packDir" before matching it against a type's
        // CodingKeys, so combined with an explicit key whose raw value is
        // still "pack_dir" it makes every key look missing rather than
        // redundant — the two are alternatives, not composable.
        do { return try JSONDecoder().decode(AnnotationExportResult.self, from: data) } catch {
            throw AnnotationExportError.decodingFailed(error)
        }
    }

    /// Pulls the `"error"` field out of a writeJSONError body, falling back to
    /// a generic message when the body is not that shape — a proxy or a
    /// misconfigured base URL can return an HTML error page instead of JSON.
    private static func serverMessage(from data: Data) -> String {
        if let decoded = try? JSONDecoder().decode([String: String].self, from: data),
            let message = decoded["error"]
        {
            return message
        }
        return "request failed"
    }
}

enum AnnotationExportError: Error, LocalizedError {
    case requestFailed(status: Int, message: String)
    case decodingFailed(Error)

    var errorDescription: String? {
        switch self {
        case .requestFailed(let status, let message): return "\(message) (HTTP \(status))"
        case .decodingFailed(let error):
            return "Could not read the server's response: \(error.localizedDescription)"
        }
    }
}
