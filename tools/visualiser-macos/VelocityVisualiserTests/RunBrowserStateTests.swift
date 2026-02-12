//
//  RunBrowserStateTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for RunBrowserState covering fetchRuns, loadRunForReplay,
//  stopReplay, and refresh.
//

import XCTest

@testable import VelocityVisualiser

// MARK: - RunBrowserState Tests

@available(macOS 15.0, *) @MainActor final class RunBrowserStateTests: XCTestCase {

    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    // MARK: - Default State

    func testDefaultState() throws {
        let state = RunBrowserState()
        XCTAssertTrue(state.runs.isEmpty)
        XCTAssertFalse(state.isLoading)
        XCTAssertNil(state.error)
        XCTAssertNil(state.selectedRunID)
        XCTAssertFalse(state.isLoadingReplay)
    }

    // MARK: - fetchRuns

    func testFetchRunsSuccess() async throws {
        let client = makeMockRunTrackClient()
        let state = RunBrowserState(apiClient: client)

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
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        await state.fetchRuns()

        XCTAssertFalse(state.isLoading)
        XCTAssertNil(state.error)
        XCTAssertEqual(state.runs.count, 1)
        XCTAssertEqual(state.runs[0].runId, "run-001")
    }

    func testFetchRunsSetsLoadingState() async throws {
        let client = makeMockRunTrackClient()
        let state = RunBrowserState(apiClient: client)

        MockURLProtocol.requestHandler = { request in
            let responseJSON = """
                {"runs": [], "count": 0}
                """
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        await state.fetchRuns()
        // After completion, isLoading should be false
        XCTAssertFalse(state.isLoading)
    }

    func testFetchRunsError() async throws {
        let client = makeMockRunTrackClient()
        let state = RunBrowserState(apiClient: client)

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 500, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        await state.fetchRuns()

        XCTAssertFalse(state.isLoading)
        XCTAssertNotNil(state.error)
        XCTAssertTrue(state.error!.contains("Failed to fetch runs"))
        XCTAssertTrue(state.runs.isEmpty)
    }

    // MARK: - loadRunForReplay

    func testLoadRunForReplaySuccess() async throws {
        let client = makeMockRunTrackClient()
        let state = RunBrowserState(apiClient: client)

        // First populate runs list
        let listJSON = """
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
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, listJSON.data(using: .utf8)!)
        }
        await state.fetchRuns()

        // Now load for replay
        MockURLProtocol.requestHandler = { request in
            XCTAssertTrue(request.url!.path.contains("api/lidar/vrlog/load"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        let success = await state.loadRunForReplay("run-001")
        XCTAssertTrue(success)
        XCTAssertEqual(state.selectedRunID, "run-001")
        XCTAssertFalse(state.isLoadingReplay)
        XCTAssertNil(state.error)
    }

    func testLoadRunForReplayRunNotFound() async throws {
        let client = makeMockRunTrackClient()
        let state = RunBrowserState(apiClient: client)
        // Runs list is empty — run won't be found

        let success = await state.loadRunForReplay("nonexistent-run")
        XCTAssertFalse(success)
        XCTAssertEqual(state.error, "Run not found")
        XCTAssertNil(state.selectedRunID)
    }

    func testLoadRunForReplayServerError() async throws {
        let client = makeMockRunTrackClient()
        let state = RunBrowserState(apiClient: client)

        // Populate runs list
        let listJSON = """
            {
                "runs": [{
                    "run_id": "run-002",
                    "created_at": "2026-02-10T20:05:09-08:00",
                    "source_type": "vrlog",
                    "source_path": null,
                    "sensor_id": "hesai-01",
                    "duration_secs": 60.0,
                    "total_frames": 600,
                    "total_clusters": 200,
                    "total_tracks": 10,
                    "confirmed_tracks": 8,
                    "status": "completed",
                    "error_message": null,
                    "vrlog_path": "/data/run2.vrlog",
                    "notes": null
                }],
                "count": 1
            }
            """
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, listJSON.data(using: .utf8)!)
        }
        await state.fetchRuns()

        // Now loadVRLog fails
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 500, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        let success = await state.loadRunForReplay("run-002")
        XCTAssertFalse(success)
        XCTAssertNotNil(state.error)
        XCTAssertTrue(state.error!.contains("Failed to load VRLOG"))
        XCTAssertFalse(state.isLoadingReplay)
        XCTAssertNil(state.selectedRunID)
    }

    // MARK: - stopReplay

    func testStopReplaySuccess() async throws {
        let client = makeMockRunTrackClient()
        let state = RunBrowserState(apiClient: client)
        state.selectedRunID = "run-001"

        MockURLProtocol.requestHandler = { request in
            XCTAssertTrue(request.url!.path.contains("api/lidar/vrlog/stop"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        await state.stopReplay()
        XCTAssertNil(state.selectedRunID)
    }

    func testStopReplayServerError() async throws {
        let client = makeMockRunTrackClient()
        let state = RunBrowserState(apiClient: client)
        state.selectedRunID = "run-001"

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 500, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        await state.stopReplay()
        // On error, selectedRunID is NOT cleared (only cleared on success)
        XCTAssertEqual(state.selectedRunID, "run-001")
    }

    // MARK: - refresh

    func testRefreshCallsFetchRuns() async throws {
        let client = makeMockRunTrackClient()
        let state = RunBrowserState(apiClient: client)

        let responseJSON = """
            {
                "runs": [{
                    "run_id": "run-refresh",
                    "created_at": "2026-02-10T20:05:09-08:00",
                    "source_type": "live",
                    "source_path": null,
                    "sensor_id": "hesai-01",
                    "duration_secs": 30.0,
                    "total_frames": 300,
                    "total_clusters": 100,
                    "total_tracks": 5,
                    "confirmed_tracks": 3,
                    "status": "completed",
                    "error_message": null,
                    "vrlog_path": null,
                    "notes": null
                }],
                "count": 1
            }
            """
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        await state.refresh()
        XCTAssertEqual(state.runs.count, 1)
        XCTAssertEqual(state.runs[0].runId, "run-refresh")
    }
}
