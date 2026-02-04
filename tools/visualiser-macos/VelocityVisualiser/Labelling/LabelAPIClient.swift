// LabelAPIClient.swift
// REST API client for track labels.
//
// Labels are stored in the Go backend's SQLite database via REST API.
// This ensures a single source of truth shared across macOS visualiser and web UI.
//
// IMPORTANT: The visualiser does NOT maintain its own SQLite database for labels.
// All label persistence is handled by the Go backend at /api/lidar/labels.

import Foundation

/// REST API client for label CRUD operations against the Go backend.
class LabelAPIClient {

    // MARK: - Properties

    /// Base URL for the Go backend API.
    private let baseURL: URL

    /// URL session for HTTP requests.
    private let session: URLSession

    /// Current session ID for grouping labels.
    var sessionID: String = UUID().uuidString

    /// Source file being annotated (for replay mode).
    var sourceFile: String = ""

    // MARK: - Initialisation

    init(baseURL: URL = URL(string: "http://localhost:8080")!) {
        self.baseURL = baseURL
        self.session = URLSession.shared
    }

    // MARK: - Label Operations

    /// Create a new label for a track.
    func createLabel(
        trackID: String, classLabel: String, startFrameID: UInt64? = nil, endFrameID: UInt64? = nil,
        annotator: String = "", notes: String = ""
    ) async throws -> LabelEvent {
        let url = baseURL.appendingPathComponent("api/lidar/labels")

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")

        let payload: [String: Any] = [
            "session_id": sessionID, "source_file": sourceFile, "track_id": trackID,
            "class_label": classLabel, "start_frame_id": startFrameID ?? 0,
            "end_frame_id": endFrameID ?? 0, "annotator": annotator, "notes": notes,
        ]

        request.httpBody = try JSONSerialization.data(withJSONObject: payload)

        let (data, response) = try await session.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        return try JSONDecoder().decode(LabelEvent.self, from: data)
    }

    /// Get all labels for the current session.
    func getLabelsForSession() async throws -> [LabelEvent] {
        var components = URLComponents(
            url: baseURL.appendingPathComponent("api/lidar/labels"), resolvingAgainstBaseURL: false)!
        components.queryItems = [URLQueryItem(name: "session_id", value: sessionID)]

        let (data, response) = try await session.data(from: components.url!)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        return try JSONDecoder().decode([LabelEvent].self, from: data)
    }

    /// Get all labels for a specific track.
    func getLabelsForTrack(_ trackID: String) async throws -> [LabelEvent] {
        var components = URLComponents(
            url: baseURL.appendingPathComponent("api/lidar/labels"), resolvingAgainstBaseURL: false)!
        components.queryItems = [URLQueryItem(name: "track_id", value: trackID)]

        let (data, response) = try await session.data(from: components.url!)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        return try JSONDecoder().decode([LabelEvent].self, from: data)
    }

    /// Update an existing label.
    func updateLabel(_ label: LabelEvent) async throws -> LabelEvent {
        let url = baseURL.appendingPathComponent("api/lidar/labels/\(label.id)")

        var request = URLRequest(url: url)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder().encode(label)

        let (data, response) = try await session.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        return try JSONDecoder().decode(LabelEvent.self, from: data)
    }

    /// Delete a label by ID.
    func deleteLabel(_ labelID: String) async throws {
        let url = baseURL.appendingPathComponent("api/lidar/labels/\(labelID)")

        var request = URLRequest(url: url)
        request.httpMethod = "DELETE"

        let (_, response) = try await session.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }
    }

    /// Export labels as JSON (fetches from backend export endpoint).
    func exportLabels() async throws -> Data {
        var components = URLComponents(
            url: baseURL.appendingPathComponent("api/lidar/labels/export"),
            resolvingAgainstBaseURL: false)!
        components.queryItems = [URLQueryItem(name: "session_id", value: sessionID)]

        let (data, response) = try await session.data(from: components.url!)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        return data
    }

    /// Save exported labels to a local file.
    func exportToFile(_ url: URL) async throws {
        let data = try await exportLabels()
        try data.write(to: url)
    }

    // MARK: - Errors

    enum APIError: Error {
        case requestFailed(URLResponse)
        case decodingFailed(Error)
    }
}
