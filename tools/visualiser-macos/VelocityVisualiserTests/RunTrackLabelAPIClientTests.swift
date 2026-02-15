//
//  RunTrackLabelAPIClientTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for RunTrackLabelAPIClient covering all API methods,
//  error handling, and response model decoding.
//

import Foundation
import Testing
import XCTest

@testable import VelocityVisualiser

// MARK: - Mock Session Helper

/// Helper to create a RunTrackLabelAPIClient with a mocked URLSession.
func makeMockRunTrackClient() -> RunTrackLabelAPIClient {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [MockURLProtocol.self]
    let session = URLSession(configuration: config)
    return RunTrackLabelAPIClient(
        baseURL: URL(string: "http://localhost:8080")!, session: session)
}

// MARK: - Initialisation Tests

struct RunTrackLabelAPIClientInitTests {
    @Test func defaultInitialisation() throws {
        let client = RunTrackLabelAPIClient()
        // Just verify creation succeeds
        _ = client
    }

    @Test func customBaseURL() throws {
        let url = URL(string: "http://192.168.1.100:9090")!
        let client = RunTrackLabelAPIClient(baseURL: url)
        _ = client
    }
}

// MARK: - APIError Tests

struct RunTrackLabelAPIClientErrorTests {
    @Test func requestFailedErrorDescription() throws {
        let response = HTTPURLResponse(
            url: URL(string: "http://localhost")!, statusCode: 500, httpVersion: nil,
            headerFields: nil)!
        let error = RunTrackLabelAPIClient.APIError.requestFailed(response)
        #expect(error.errorDescription?.contains("500") == true)
    }

    @Test func requestFailedWithNonHTTPResponse() throws {
        let response = URLResponse(
            url: URL(string: "http://localhost")!, mimeType: nil, expectedContentLength: 0,
            textEncodingName: nil)
        let error = RunTrackLabelAPIClient.APIError.requestFailed(response)
        #expect(error.errorDescription == "Request failed")
    }

    @Test func decodingFailedErrorDescription() throws {
        struct TestError: Error, LocalizedError {
            var errorDescription: String? { "bad data" }
        }
        let error = RunTrackLabelAPIClient.APIError.decodingFailed(TestError())
        #expect(error.errorDescription?.contains("bad data") == true)
    }
}

// MARK: - AnalysisRun Model Tests

struct AnalysisRunModelTests {
    @Test func hasVRLogWhenPathExists() throws {
        let run = makeRun(vrlogPath: "/path/to/file.vrlog")
        #expect(run.hasVRLog == true)
    }

    @Test func hasVRLogFalseWhenNil() throws {
        let run = makeRun(vrlogPath: nil)
        #expect(run.hasVRLog == false)
    }

    @Test func hasVRLogFalseWhenEmpty() throws {
        let run = makeRun(vrlogPath: "")
        #expect(run.hasVRLog == false)
    }

    @Test func formattedDateIsNonEmpty() throws {
        let run = makeRun()
        #expect(!run.formattedDate.isEmpty)
    }

    @Test func identifiableUsesRunId() throws {
        let run = makeRun()
        #expect(run.id == run.runId)
    }

    private func makeRun(
        vrlogPath: String? = "/some/path.vrlog"
    ) -> AnalysisRun {
        AnalysisRun(
            runId: "run-001", createdAt: Date(), sourceType: "vrlog",
            sourcePath: "/data/test.vrlog", sensorId: "hesai-01", durationSecs: 120.0,
            totalFrames: 1200, totalClusters: 500, totalTracks: 25, confirmedTracks: 20,
            status: "completed", errorMessage: nil, vrlogPath: vrlogPath, notes: nil)
    }
}

// MARK: - RunTrack Model Tests

struct RunTrackModelTests {
    @Test func isLabelledWithLabel() throws {
        let track = RunTrack(
            runId: "run-001", trackId: "track-001", sensorId: "hesai-01",
            userLabel: "car", qualityLabel: nil, labelConfidence: nil,
            labelerId: nil, startUnixNanos: nil, endUnixNanos: nil, totalObservations: nil,
            durationSecs: nil, avgSpeedMps: nil, peakSpeedMps: nil, isSplitCandidate: nil,
            isMergeCandidate: nil)
        #expect(track.isLabelled == true)
    }

    @Test func isLabelledFalseWhenNil() throws {
        let track = RunTrack(
            runId: "run-001", trackId: "track-001", sensorId: "hesai-01",
            userLabel: nil, qualityLabel: nil, labelConfidence: nil,
            labelerId: nil, startUnixNanos: nil, endUnixNanos: nil, totalObservations: nil,
            durationSecs: nil, avgSpeedMps: nil, peakSpeedMps: nil, isSplitCandidate: nil,
            isMergeCandidate: nil)
        #expect(track.isLabelled == false)
    }

    @Test func isLabelledFalseWhenEmpty() throws {
        let track = RunTrack(
            runId: "run-001", trackId: "track-001", sensorId: "hesai-01",
            userLabel: "", qualityLabel: nil, labelConfidence: nil,
            labelerId: nil, startUnixNanos: nil, endUnixNanos: nil, totalObservations: nil,
            durationSecs: nil, avgSpeedMps: nil, peakSpeedMps: nil, isSplitCandidate: nil,
            isMergeCandidate: nil)
        #expect(track.isLabelled == false)
    }

    @Test func identifiableUsesTrackId() throws {
        let track = RunTrack(
            runId: "run-001", trackId: "track-abc", sensorId: "hesai-01",
            userLabel: nil, qualityLabel: nil, labelConfidence: nil,
            labelerId: nil, startUnixNanos: nil, endUnixNanos: nil, totalObservations: nil,
            durationSecs: nil, avgSpeedMps: nil, peakSpeedMps: nil, isSplitCandidate: nil,
            isMergeCandidate: nil)
        #expect(track.id == "track-abc")
    }
}

// MARK: - HTTP Tests

final class RunTrackLabelAPIClientHTTPTests: XCTestCase {

    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    // MARK: - listRuns

    func testListRunsSuccess() async throws {
        let client = makeMockRunTrackClient()

        let responseJSON = """
            {
                "runs": [{
                    "run_id": "run-001",
                    "created_at": "2026-02-10T20:05:09.283745-08:00",
                    "source_type": "vrlog",
                    "source_path": "/data/test.vrlog",
                    "sensor_id": "hesai-01",
                    "duration_secs": 120.0,
                    "total_frames": 1200,
                    "total_clusters": 500,
                    "total_tracks": 25,
                    "confirmed_tracks": 20,
                    "status": "completed",
                    "error_message": null,
                    "vrlog_path": "/data/test.vrlog",
                    "notes": null
                }],
                "count": 1
            }
            """
        MockURLProtocol.requestHandler = { request in
            XCTAssertTrue(request.url!.path.contains("api/lidar/runs"))
            XCTAssertTrue(request.url!.absoluteString.contains("limit=50"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let runs = try await client.listRuns(limit: 50)
        XCTAssertEqual(runs.count, 1)
        XCTAssertEqual(runs[0].runId, "run-001")
        XCTAssertEqual(runs[0].status, "completed")
        XCTAssertEqual(runs[0].totalTracks, 25)
    }

    func testListRunsServerError() async throws {
        let client = makeMockRunTrackClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 500, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            _ = try await client.listRuns()
            XCTFail("Expected requestFailed error")
        } catch is RunTrackLabelAPIClient.APIError {
            // Expected
        }
    }

    // MARK: - getRun

    func testGetRunSuccess() async throws {
        let client = makeMockRunTrackClient()

        let responseJSON = """
            {
                "run_id": "run-002",
                "created_at": "2026-02-10T20:05:09-08:00",
                "source_type": "live",
                "source_path": null,
                "sensor_id": "hesai-01",
                "duration_secs": 60.0,
                "total_frames": 600,
                "total_clusters": 200,
                "total_tracks": 10,
                "confirmed_tracks": 8,
                "status": "running",
                "error_message": null,
                "vrlog_path": null,
                "notes": "Test run"
            }
            """
        MockURLProtocol.requestHandler = { request in
            XCTAssertTrue(request.url!.path.contains("api/lidar/runs/run-002"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let run = try await client.getRun("run-002")
        XCTAssertEqual(run.runId, "run-002")
        XCTAssertEqual(run.status, "running")
        XCTAssertEqual(run.notes, "Test run")
    }

    func testGetRunServerError() async throws {
        let client = makeMockRunTrackClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 404, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            _ = try await client.getRun("nonexistent")
            XCTFail("Expected requestFailed error")
        } catch is RunTrackLabelAPIClient.APIError {
            // Expected
        }
    }

    // MARK: - listTracks

    func testListTracksSuccess() async throws {
        let client = makeMockRunTrackClient()

        let responseJSON = """
            {
                "run_id": "run-001",
                "tracks": [{
                    "run_id": "run-001",
                    "track_id": "track-001",
                    "sensor_id": "hesai-01",
                    "user_label": "car",
                    "quality_label": "good",
                    "label_confidence": 0.95,
                    "labeler_id": "david",
                    "start_unix_nanos": 1000000000,
                    "end_unix_nanos": 5000000000,
                    "total_observations": 40,
                    "duration_secs": 4.0,
                    "avg_speed_mps": 8.5,
                    "peak_speed_mps": 12.0,
                    "is_split_candidate": false,
                    "is_merge_candidate": false
                }],
                "count": 1
            }
            """
        MockURLProtocol.requestHandler = { request in
            XCTAssertTrue(request.url!.path.contains("api/lidar/runs/run-001/tracks"))
            XCTAssertTrue(request.url!.absoluteString.contains("limit=100"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let tracks = try await client.listTracks(runID: "run-001", limit: 100)
        XCTAssertEqual(tracks.count, 1)
        XCTAssertEqual(tracks[0].trackId, "track-001")
        XCTAssertEqual(tracks[0].userLabel, "car")
        XCTAssertEqual(tracks[0].qualityLabel, "good")
        XCTAssertTrue(tracks[0].isLabelled)
    }

    func testListTracksServerError() async throws {
        let client = makeMockRunTrackClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 500, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            _ = try await client.listTracks(runID: "run-001")
            XCTFail("Expected requestFailed error")
        } catch is RunTrackLabelAPIClient.APIError {
            // Expected
        }
    }

    // MARK: - getTrack

    func testGetTrackSuccess() async throws {
        let client = makeMockRunTrackClient()

        let responseJSON = """
            {
                "run_id": "run-001",
                "track_id": "track-abc",
                "sensor_id": "hesai-01",
                "user_label": null,
                "quality_label": null,
                "label_confidence": null,
                "labeler_id": null,
                "start_unix_nanos": null,
                "end_unix_nanos": null,
                "total_observations": 30,
                "duration_secs": 3.0,
                "avg_speed_mps": 5.0,
                "peak_speed_mps": 8.0,
                "is_split_candidate": true,
                "is_merge_candidate": false
            }
            """
        MockURLProtocol.requestHandler = { request in
            XCTAssertTrue(request.url!.path.contains("api/lidar/runs/run-001/tracks/track-abc"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let track = try await client.getTrack(runID: "run-001", trackID: "track-abc")
        XCTAssertEqual(track.trackId, "track-abc")
        XCTAssertFalse(track.isLabelled)
        XCTAssertEqual(track.isSplitCandidate, true)
    }

    func testGetTrackServerError() async throws {
        let client = makeMockRunTrackClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 404, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            _ = try await client.getTrack(runID: "run-001", trackID: "missing")
            XCTFail("Expected requestFailed error")
        } catch is RunTrackLabelAPIClient.APIError {
            // Expected
        }
    }

    // MARK: - updateLabel

    func testUpdateLabelSuccess() async throws {
        let client = makeMockRunTrackClient()

        let responseJSON = """
            {
                "status": "updated",
                "run_id": "run-001",
                "track_id": "track-001",
                "user_label": "car",
                "quality_label": "good",
                "label_confidence": 0.9,
                "labeler_id": "david"
            }
            """
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "PUT")
            XCTAssertTrue(request.url!.path.contains("api/lidar/runs/run-001/tracks/track-001/label"))

            // Verify request body
            if let body = request.httpBody,
                let json = try? JSONSerialization.jsonObject(with: body) as? [String: Any]
            {
                XCTAssertEqual(json["user_label"] as? String, "car")
                XCTAssertEqual(json["quality_label"] as? String, "good")
            }

            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let result = try await client.updateLabel(
            runID: "run-001", trackID: "track-001",
            userLabel: "car", qualityLabel: "good")
        XCTAssertEqual(result.status, "updated")
        XCTAssertEqual(result.userLabel, "car")
    }

    func testUpdateLabelOnlyUserLabel() async throws {
        let client = makeMockRunTrackClient()

        let responseJSON = """
            {
                "status": "updated",
                "run_id": "run-001",
                "track_id": "track-001",
                "user_label": "noise",
                "quality_label": null,
                "label_confidence": null,
                "labeler_id": null
            }
            """
        MockURLProtocol.requestHandler = { request in
            if let body = request.httpBody,
                let json = try? JSONSerialization.jsonObject(with: body) as? [String: Any]
            {
                XCTAssertNotNil(json["user_label"])
                XCTAssertNil(json["quality_label"])
                XCTAssertNil(json["label_confidence"])
            }
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let result = try await client.updateLabel(
            runID: "run-001", trackID: "track-001", userLabel: "noise")
        XCTAssertEqual(result.userLabel, "noise")
    }

    func testUpdateLabelServerError() async throws {
        let client = makeMockRunTrackClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 400, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            _ = try await client.updateLabel(
                runID: "run-001", trackID: "track-001", userLabel: "bad")
            XCTFail("Expected requestFailed error")
        } catch is RunTrackLabelAPIClient.APIError {
            // Expected
        }
    }

    // MARK: - getLabellingProgress

    func testGetLabellingProgressSuccess() async throws {
        let client = makeMockRunTrackClient()

        let responseJSON = """
            {
                "run_id": "run-001",
                "total": 25,
                "labelled": 15,
                "by_class": {"car": 10, "noise": 5},
                "progress_pct": 60.0
            }
            """
        MockURLProtocol.requestHandler = { request in
            XCTAssertTrue(request.url!.path.contains("api/lidar/runs/run-001/labelling-progress"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let progress = try await client.getLabellingProgress(runID: "run-001")
        XCTAssertEqual(progress.total, 25)
        XCTAssertEqual(progress.labelled, 15)
        XCTAssertEqual(progress.progressPct, 60.0)
        XCTAssertEqual(progress.byClass?["car"], 10)
    }

    func testGetLabellingProgressServerError() async throws {
        let client = makeMockRunTrackClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 500, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            _ = try await client.getLabellingProgress(runID: "run-001")
            XCTFail("Expected requestFailed error")
        } catch is RunTrackLabelAPIClient.APIError {
            // Expected
        }
    }

    // MARK: - loadVRLog

    func testLoadVRLogSuccess() async throws {
        let client = makeMockRunTrackClient()

        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertTrue(request.url!.path.contains("api/lidar/vrlog/load"))

            if let body = request.httpBody,
                let json = try? JSONSerialization.jsonObject(with: body) as? [String: Any]
            {
                XCTAssertEqual(json["run_id"] as? String, "run-001")
            }
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        try await client.loadVRLog(runID: "run-001")
    }

    func testLoadVRLogServerError() async throws {
        let client = makeMockRunTrackClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 404, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            try await client.loadVRLog(runID: "no-such-run")
            XCTFail("Expected requestFailed error")
        } catch is RunTrackLabelAPIClient.APIError {
            // Expected
        }
    }

    // MARK: - stopVRLog

    func testStopVRLogSuccess() async throws {
        let client = makeMockRunTrackClient()

        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertTrue(request.url!.path.contains("api/lidar/vrlog/stop"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        try await client.stopVRLog()
    }

    func testStopVRLogServerError() async throws {
        let client = makeMockRunTrackClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 500, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            try await client.stopVRLog()
            XCTFail("Expected requestFailed error")
        } catch is RunTrackLabelAPIClient.APIError {
            // Expected
        }
    }

    // MARK: - getPlaybackStatus

    func testGetPlaybackStatusSuccess() async throws {
        let client = makeMockRunTrackClient()

        let responseJSON = """
            {
                "mode": "replay",
                "paused": false,
                "rate": 2.0,
                "seekable": true,
                "current_frame": 500,
                "total_frames": 1200,
                "timestamp_ns": 5000000000,
                "log_start_ns": 1000000000,
                "log_end_ns": 12000000000,
                "vrlog_path": "/data/test.vrlog"
            }
            """
        MockURLProtocol.requestHandler = { request in
            XCTAssertTrue(request.url!.path.contains("api/lidar/playback/status"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let status = try await client.getPlaybackStatus()
        XCTAssertEqual(status.mode, "replay")
        XCTAssertFalse(status.paused)
        XCTAssertEqual(status.rate, 2.0)
        XCTAssertTrue(status.seekable)
        XCTAssertEqual(status.totalFrames, 1200)
    }

    func testGetPlaybackStatusServerError() async throws {
        let client = makeMockRunTrackClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 503, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            _ = try await client.getPlaybackStatus()
            XCTFail("Expected requestFailed error")
        } catch is RunTrackLabelAPIClient.APIError {
            // Expected
        }
    }
}

// MARK: - Model Decoding Tests

struct RunTrackLabelModelDecodingTests {
    @Test func analysisRunDecodesFractionalSeconds() throws {
        let json = """
            {
                "run_id": "run-frac",
                "created_at": "2026-02-10T20:05:09.283745-08:00",
                "source_type": "vrlog",
                "source_path": null,
                "sensor_id": "hesai-01",
                "duration_secs": 30.5,
                "total_frames": 305,
                "total_clusters": 100,
                "total_tracks": 5,
                "confirmed_tracks": 3,
                "status": "completed",
                "error_message": null,
                "vrlog_path": "/data/frac.vrlog",
                "notes": null
            }
            """
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let isoFrac = ISO8601DateFormatter()
        isoFrac.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let iso = ISO8601DateFormatter()
        iso.formatOptions = [.withInternetDateTime]
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let dateString = try container.decode(String.self)
            if let date = isoFrac.date(from: dateString) { return date }
            if let date = iso.date(from: dateString) { return date }
            throw DecodingError.dataCorruptedError(
                in: container, debugDescription: "Cannot decode date: \(dateString)")
        }

        let run = try decoder.decode(AnalysisRun.self, from: json.data(using: .utf8)!)
        #expect(run.runId == "run-frac")
        #expect(run.durationSecs == 30.5)
    }

    @Test func analysisRunDecodesWithoutFractionalSeconds() throws {
        let json = """
            {
                "run_id": "run-nofrac",
                "created_at": "2026-02-10T20:05:09-08:00",
                "source_type": "live",
                "source_path": null,
                "sensor_id": "hesai-01",
                "duration_secs": 10.0,
                "total_frames": 100,
                "total_clusters": 50,
                "total_tracks": 3,
                "confirmed_tracks": 2,
                "status": "completed",
                "error_message": null,
                "vrlog_path": null,
                "notes": null
            }
            """
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let isoFrac = ISO8601DateFormatter()
        isoFrac.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let iso = ISO8601DateFormatter()
        iso.formatOptions = [.withInternetDateTime]
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let dateString = try container.decode(String.self)
            if let date = isoFrac.date(from: dateString) { return date }
            if let date = iso.date(from: dateString) { return date }
            throw DecodingError.dataCorruptedError(
                in: container, debugDescription: "Cannot decode date: \(dateString)")
        }

        let run = try decoder.decode(AnalysisRun.self, from: json.data(using: .utf8)!)
        #expect(run.runId == "run-nofrac")
        #expect(run.hasVRLog == false)
    }

    @Test func labelUpdateResponseDecodes() throws {
        let json = """
            {
                "status": "updated",
                "run_id": "run-001",
                "track_id": "track-001",
                "user_label": "noise",
                "quality_label": null,
                "label_confidence": 0.85,
                "labeler_id": "tester"
            }
            """
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let resp = try decoder.decode(LabelUpdateResponse.self, from: json.data(using: .utf8)!)
        #expect(resp.status == "updated")
        #expect(resp.userLabel == "noise")
        #expect(resp.labelConfidence == 0.85)
        #expect(resp.labelerId == "tester")
    }

    @Test func labellingProgressDecodes() throws {
        let json = """
            {
                "run_id": "run-001",
                "total": 50,
                "labelled": 30,
                "by_class": {"car": 20, "noise": 10},
                "progress_pct": 60.0
            }
            """
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let progress = try decoder.decode(LabellingProgress.self, from: json.data(using: .utf8)!)
        #expect(progress.total == 50)
        #expect(progress.labelled == 30)
        #expect(progress.progressPct == 60.0)
        #expect(progress.byClass?["car"] == 20)
    }

    @Test func playbackStatusDecodes() throws {
        let json = """
            {
                "mode": "live",
                "paused": true,
                "rate": 1.0,
                "seekable": false,
                "current_frame": 0,
                "total_frames": 0,
                "timestamp_ns": 0,
                "log_start_ns": 0,
                "log_end_ns": 0,
                "vrlog_path": null
            }
            """
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let status = try decoder.decode(PlaybackStatus.self, from: json.data(using: .utf8)!)
        #expect(status.mode == "live")
        #expect(status.paused == true)
        #expect(status.seekable == false)
    }

    @Test func runsResponseDecodes() throws {
        let json = """
            {
                "runs": [],
                "count": 0
            }
            """
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let resp = try decoder.decode(RunsResponse.self, from: json.data(using: .utf8)!)
        #expect(resp.runs.isEmpty)
        #expect(resp.count == 0)
    }

    @Test func tracksResponseDecodes() throws {
        let json = """
            {
                "run_id": "run-001",
                "tracks": [],
                "count": 0
            }
            """
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let resp = try decoder.decode(TracksResponse.self, from: json.data(using: .utf8)!)
        #expect(resp.runId == "run-001")
        #expect(resp.tracks.isEmpty)
    }
}
