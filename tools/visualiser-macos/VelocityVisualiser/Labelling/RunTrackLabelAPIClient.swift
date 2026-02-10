// RunTrackLabelAPIClient.swift
// REST API client for run-track labels (Phase 4.2).
//
// This client interacts with the run-track label endpoints for analysis runs.
// Unlike LabelAPIClient (which uses /api/lidar/labels for free-form events),
// this uses /api/lidar/runs/{run_id}/tracks/{track_id}/label for canonical
// track labelling within analysis runs.

import Foundation

/// REST API client for run-track label operations against the Go backend.
class RunTrackLabelAPIClient {

    // MARK: - Properties

    /// Base URL for the Go backend API.
    private let baseURL: URL

    /// URL session for HTTP requests.
    private let session: URLSession

    // MARK: - Initialisation

    init(baseURL: URL = URL(string: "http://localhost:8080")!) {
        self.baseURL = baseURL
        self.session = URLSession.shared
    }

    // MARK: - Run Operations

    /// List recent analysis runs.
    func listRuns(limit: Int = 50) async throws -> [AnalysisRun] {
        var components = URLComponents(
            url: baseURL.appendingPathComponent("api/lidar/runs"),
            resolvingAgainstBaseURL: false)!
        components.queryItems = [URLQueryItem(name: "limit", value: "\(limit)")]

        let (data, response) = try await session.data(from: components.url!)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode([AnalysisRun].self, from: data)
    }

    /// Get a single analysis run by ID.
    func getRun(_ runID: String) async throws -> AnalysisRun {
        let url = baseURL.appendingPathComponent("api/lidar/runs/\(runID)")

        let (data, response) = try await session.data(from: url)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(AnalysisRun.self, from: data)
    }

    // MARK: - Track Operations

    /// List tracks for a run.
    func listTracks(runID: String, limit: Int = 100) async throws -> [RunTrack] {
        var components = URLComponents(
            url: baseURL.appendingPathComponent("api/lidar/runs/\(runID)/tracks"),
            resolvingAgainstBaseURL: false)!
        components.queryItems = [URLQueryItem(name: "limit", value: "\(limit)")]

        let (data, response) = try await session.data(from: components.url!)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode([RunTrack].self, from: data)
    }

    /// Get a single track by ID.
    func getTrack(runID: String, trackID: String) async throws -> RunTrack {
        let url = baseURL.appendingPathComponent("api/lidar/runs/\(runID)/tracks/\(trackID)")

        let (data, response) = try await session.data(from: url)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(RunTrack.self, from: data)
    }

    // MARK: - Label Operations

    /// Update the label for a track in a run.
    /// Only fields that are provided (non-nil) will be sent to the API.
    /// Use empty string to explicitly clear a field.
    func updateLabel(
        runID: String, trackID: String, userLabel: String? = nil, qualityLabel: String? = nil,
        labelConfidence: Float? = nil, labelerID: String? = nil
    ) async throws -> RunTrack {
        let url = baseURL.appendingPathComponent("api/lidar/runs/\(runID)/tracks/\(trackID)/label")

        var request = URLRequest(url: url)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")

        // Only include fields that are explicitly provided
        var payload: [String: Any] = [:]
        if let userLabel = userLabel { payload["user_label"] = userLabel }
        if let qualityLabel = qualityLabel { payload["quality_label"] = qualityLabel }
        if let labelConfidence = labelConfidence { payload["label_confidence"] = labelConfidence }
        if let labelerID = labelerID { payload["labeler_id"] = labelerID }

        request.httpBody = try JSONSerialization.data(withJSONObject: payload)

        let (data, response) = try await session.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(RunTrack.self, from: data)
    }

    /// Get labelling progress for a run.
    func getLabellingProgress(runID: String) async throws -> LabellingProgress {
        let url = baseURL.appendingPathComponent("api/lidar/runs/\(runID)/labelling-progress")

        let (data, response) = try await session.data(from: url)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(LabellingProgress.self, from: data)
    }

    // MARK: - VRLOG Playback Operations

    /// Load a VRLOG for replay by run ID.
    func loadVRLog(runID: String) async throws {
        let url = baseURL.appendingPathComponent("api/lidar/vrlog/load")

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONSerialization.data(withJSONObject: ["run_id": runID])

        let (_, response) = try await session.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }
    }

    /// Stop VRLOG replay.
    func stopVRLog() async throws {
        let url = baseURL.appendingPathComponent("api/lidar/vrlog/stop")

        var request = URLRequest(url: url)
        request.httpMethod = "POST"

        let (_, response) = try await session.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }
    }

    /// Get playback status.
    func getPlaybackStatus() async throws -> PlaybackStatus {
        let url = baseURL.appendingPathComponent("api/lidar/playback/status")

        let (data, response) = try await session.data(from: url)

        guard let httpResponse = response as? HTTPURLResponse,
            (200...299).contains(httpResponse.statusCode)
        else { throw APIError.requestFailed(response) }

        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(PlaybackStatus.self, from: data)
    }

    // MARK: - Errors

    enum APIError: Error, LocalizedError {
        case requestFailed(URLResponse)
        case decodingFailed(Error)

        var errorDescription: String? {
            switch self {
            case .requestFailed(let response):
                if let http = response as? HTTPURLResponse {
                    return "Request failed with status \(http.statusCode)"
                }
                return "Request failed"
            case .decodingFailed(let error):
                return "Decoding failed: \(error.localizedDescription)"
            }
        }
    }
}

// MARK: - Data Models

/// Analysis run from the backend.
struct AnalysisRun: Codable, Identifiable {
    let runId: String
    let createdAt: Int64
    let sourceType: String
    let sourcePath: String?
    let sensorId: String
    let durationSecs: Double
    let totalFrames: Int
    let totalClusters: Int
    let totalTracks: Int
    let confirmedTracks: Int
    let status: String
    let errorMessage: String?
    let vrlogPath: String?
    let notes: String?

    var id: String { runId }

    /// Whether this run has a VRLOG available for replay.
    var hasVRLog: Bool { vrlogPath != nil && !vrlogPath!.isEmpty }

    /// Formatted creation date.
    var formattedDate: String {
        let date = Date(timeIntervalSince1970: Double(createdAt) / 1_000_000_000)
        let formatter = DateFormatter()
        formatter.dateStyle = .short
        formatter.timeStyle = .medium
        return formatter.string(from: date)
    }
}

/// Track within an analysis run.
struct RunTrack: Codable, Identifiable {
    let runId: String
    let trackId: String
    let sensorId: String
    let classLabel: String
    let quality: String
    let isSplit: Bool
    let isMerge: Bool
    let firstSeenNs: Int64
    let lastSeenNs: Int64
    let totalObservations: Int
    let durationSecs: Double
    let avgSpeedMps: Double
    let peakSpeedMps: Double

    var id: String { trackId }

    /// Whether this track has been labelled.
    var isLabelled: Bool { !classLabel.isEmpty }
}

/// Labelling progress for a run.
struct LabellingProgress: Codable {
    let runId: String
    let totalTracks: Int
    let labelledTracks: Int
    let labelledPercent: Double
    let labelCounts: [String: Int]
}

/// Playback status from the backend.
struct PlaybackStatus: Codable {
    let mode: String
    let paused: Bool
    let rate: Float
    let seekable: Bool
    let currentFrame: UInt64
    let totalFrames: UInt64
    let timestampNs: Int64
    let logStartNs: Int64
    let logEndNs: Int64
    let vrlogPath: String?
}
