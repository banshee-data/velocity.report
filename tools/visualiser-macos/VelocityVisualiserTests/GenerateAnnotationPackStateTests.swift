//
//  GenerateAnnotationPackStateTests.swift
//  VelocityVisualiserTests
//
//  The sheet's state talks to two API clients and does its own validation
//  before either is called (a run must be chosen, max samples must parse to
//  a positive number). These tests exercise that validation directly and the
//  success/failure paths through the real clients under
//  AnnotationMockURLProtocol, so the sheet's logic is covered without
//  mounting a view.
//

import Foundation
import Testing

@testable import VelocityVisualiser

/// Two independent sessions, one per client, each with its own host from
/// AnnotationMockURLProtocol.makeSession(). GenerateAnnotationPackState
/// issues its runs-list request and its export request one after the other
/// on the same instance, and reusing a single mock session for both proved
/// to corrupt the second response's body — sharing a session was buying
/// nothing here, so each call gets a session of its own instead.
@MainActor private func makeStateUnderTest(
    runsJSON: String, runsHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))? = nil,
    exportHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))? = nil
) -> GenerateAnnotationPackState {
    let (runsSession, runsBaseURL, registerRuns) = AnnotationMockURLProtocol.makeSession()
    registerRuns { request in
        if let runsHandler { return try runsHandler(request) }
        let response = HTTPURLResponse(
            url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
        return (response, Data(runsJSON.utf8))
    }

    let (exportSession, exportBaseURL, registerExport) = AnnotationMockURLProtocol.makeSession()
    registerExport { request in
        if let exportHandler { return try exportHandler(request) }
        throw NSError(
            domain: "test", code: 1, userInfo: [NSLocalizedDescriptionKey: "unexpected request"])
    }

    return GenerateAnnotationPackState(
        runsClient: RunTrackLabelAPIClient(baseURL: runsBaseURL, session: runsSession),
        exportClient: AnnotationExportAPIClient(baseURL: exportBaseURL, session: exportSession))
}

/// Counts how often a mock handler was reached. The handler runs on
/// whichever thread the URL Loading System calls `startLoading()` from, not
/// on the test's task, so the count is locked rather than a bare `var`.
private final class CallCounter: @unchecked Sendable {
    private let lock = NSLock()
    private var count = 0

    /// Records one call and returns the running total, so a handler can
    /// answer its first call differently from its second.
    @discardableResult func increment() -> Int {
        lock.lock()
        defer { lock.unlock() }
        count += 1
        return count
    }

    var value: Int {
        lock.lock()
        defer { lock.unlock() }
        return count
    }
}

private func okResponse(_ request: URLRequest, status: Int = 200) -> HTTPURLResponse {
    HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!
}

private let exportSucceededJSON = """
    {"pack_dir": "/x", "dataset_id": "d", "sample_count": 200, "point_count": 1,
     "coverage": "full", "frames_without_points": 0,
     "has_intensity": false, "has_classification": false}
    """

private let twoRunsOneWithoutVRLog = """
    {"count": 2, "runs": [
      {"run_id": "run-with-vrlog", "created_at": "2026-09-17T12:00:00Z", "source_type": "pcap",
       "sensor_id": "s", "duration_secs": 10, "total_frames": 100, "total_clusters": 10,
       "total_tracks": 2, "confirmed_tracks": 1, "status": "completed",
       "vrlog_path": "/data/vrlog/run-with-vrlog"},
      {"run_id": "run-without-vrlog", "created_at": "2026-09-17T11:00:00Z", "source_type": "live",
       "sensor_id": "s", "duration_secs": 5, "total_frames": 50, "total_clusters": 5,
       "total_tracks": 1, "confirmed_tracks": 0, "status": "completed"}
    ]}
    """

@MainActor struct GenerateAnnotationPackStateTests {

    @Test func loadRunsFiltersOutRunsWithoutAVRLog() async {
        let state = makeStateUnderTest(runsJSON: twoRunsOneWithoutVRLog)

        await state.loadRuns()

        #expect(state.runs.count == 2, "the full list is kept for reference")
        #expect(state.exportableRuns.count == 1)
        #expect(state.exportableRuns.first?.id == "run-with-vrlog")
    }

    @Test func loadRunsSelectsTheFirstExportableRunByDefault() async {
        let state = makeStateUnderTest(runsJSON: twoRunsOneWithoutVRLog)

        await state.loadRuns()

        #expect(state.selectedRunID == "run-with-vrlog")
    }

    @Test func generateWithoutASelectedRunFailsValidationWithoutANetworkCall() async {
        let state = makeStateUnderTest(runsJSON: "{\"runs\": []}")
        // Deliberately skip loadRuns(): selectedRunID stays nil.

        let result = await state.generate()

        #expect(result == nil)
        #expect(state.lastError != nil)
    }

    @Test func loadRunsClearsAStaleErrorOnceARefreshSucceeds() async {
        // First load fails, the operator presses refresh, the second load
        // succeeds: the first attempt's error must not stay on screen.
        let calls = CallCounter()
        let state = makeStateUnderTest(
            runsJSON: twoRunsOneWithoutVRLog,
            runsHandler: { request in
                if calls.increment() == 1 { return (okResponse(request, status: 500), Data()) }
                return (okResponse(request), Data(twoRunsOneWithoutVRLog.utf8))
            })

        await state.loadRuns()
        #expect(state.lastError != nil, "the first load is meant to fail")

        await state.loadRuns()

        #expect(state.exportableRuns.count == 1)
        #expect(state.lastError == nil, "a successful refresh must clear the earlier failure")
    }

    @Test func coverageStartsUnstated() {
        let state = makeStateUnderTest(runsJSON: twoRunsOneWithoutVRLog)
        #expect(state.coverage == nil, "a pre-selected coverage is a guess the operator never made")
    }

    @Test func generateRefusesToGuessCoverageAndMakesNoNetworkCall() async {
        // The handler would succeed, as in the max-samples test and for the
        // same reason: only a handler that succeeds, plus a count of whether
        // it was reached, separates "refused here" from "failed over there".
        let exportCalls = CallCounter()
        let state = makeStateUnderTest(
            runsJSON: twoRunsOneWithoutVRLog,
            exportHandler: { request in
                exportCalls.increment()
                return (okResponse(request), Data(exportSucceededJSON.utf8))
            })
        await state.loadRuns()

        let result = await state.generate()

        #expect(result == nil)
        #expect(state.lastError?.contains("what the recording could see") == true)
        #expect(exportCalls.value == 0, "an unstated coverage must never reach the server")
    }

    @Test func theStatedCoverageIsTheOneSent() async {
        let sent = CallCounter()
        let state = makeStateUnderTest(
            runsJSON: twoRunsOneWithoutVRLog,
            exportHandler: { request in
                let body = try requestJSONBody(request)
                if body["coverage"] as? String == "foreground_only" { sent.increment() }
                return (okResponse(request), Data(exportSucceededJSON.utf8))
            })
        await state.loadRuns()
        state.coverage = .foregroundOnly

        _ = await state.generate()

        #expect(sent.value == 1)
    }

    @Test func generateRejectsANonNumericMaxSamplesWithoutANetworkCall() async {
        // The export handler SUCCEEDS here on purpose. An earlier form of this
        // test left it unset, so the mock threw "unexpected request" — and
        // that failure set lastError and returned nil exactly as a validation
        // failure would. The test passed while "2oo" was in fact being sent to
        // the server as a request for its default. Only a handler that would
        // succeed, plus a count of whether it was reached, can tell the two
        // apart.
        let exportCalls = CallCounter()
        let state = makeStateUnderTest(
            runsJSON: twoRunsOneWithoutVRLog,
            exportHandler: { request in
                exportCalls.increment()
                return (okResponse(request), Data(exportSucceededJSON.utf8))
            })
        await state.loadRuns()
        state.coverage = .full

        for bad in ["2oo", "1.5", "abc"] {
            state.maxSamplesText = bad
            let result = await state.generate()
            #expect(result == nil, "max_samples=\(bad) should be rejected")
            #expect(state.lastError?.contains("Max samples") == true)
        }
        #expect(exportCalls.value == 0, "validation must fail before any request is made")
    }

    @Test func generateRejectsAZeroOrNegativeMaxSamples() async {
        let state = makeStateUnderTest(runsJSON: twoRunsOneWithoutVRLog)
        await state.loadRuns()
        state.coverage = .full

        for bad in ["0", "-5"] {
            state.maxSamplesText = bad
            let result = await state.generate()
            #expect(result == nil, "max_samples=\(bad) should be rejected")
        }
    }

    @Test func generateSucceedsAndReturnsTheReportedPackDirectory() async {
        let state = makeStateUnderTest(
            runsJSON: twoRunsOneWithoutVRLog,
            exportHandler: { request in
                let response = HTTPURLResponse(
                    url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
                let json = """
                    {"pack_dir": "/data/annotation-packs/run-with-vrlog-20260917",
                     "dataset_id": "ds_1", "sample_count": 20, "point_count": 1000,
                     "coverage": "full", "frames_without_points": 0,
                     "has_intensity": true, "has_classification": true}
                    """
                return (response, Data(json.utf8))
            })
        await state.loadRuns()
        state.coverage = .full

        let result = await state.generate()

        #expect(result?.path == "/data/annotation-packs/run-with-vrlog-20260917")
        #expect(state.lastError == nil)
    }

    @Test func generateSurfacesTheServersErrorMessage() async {
        let state = makeStateUnderTest(
            runsJSON: twoRunsOneWithoutVRLog,
            exportHandler: { request in
                let response = HTTPURLResponse(
                    url: request.url!, statusCode: 400, httpVersion: nil, headerFields: nil)!
                return (response, Data("{\"error\": \"coverage is required\"}".utf8))
            })
        await state.loadRuns()
        state.coverage = .full

        let result = await state.generate()

        #expect(result == nil)
        #expect(state.lastError?.contains("coverage is required") == true)
    }

    @Test func blankMaxSamplesUsesTheServerDefaultRatherThanFailing() async {
        // Blank text must parse to "no override" (0 passed through the
        // client), not be rejected as an invalid number.
        let state = makeStateUnderTest(
            runsJSON: twoRunsOneWithoutVRLog,
            exportHandler: { request in
                let response = HTTPURLResponse(
                    url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
                let json = """
                    {"pack_dir": "/x", "dataset_id": "d", "sample_count": 200, "point_count": 1,
                     "coverage": "full", "frames_without_points": 0,
                     "has_intensity": false, "has_classification": false}
                    """
                return (response, Data(json.utf8))
            })
        await state.loadRuns()
        state.coverage = .full
        state.maxSamplesText = "   "

        let result = await state.generate()

        #expect(result != nil)
        #expect(state.lastError == nil)
    }
}
