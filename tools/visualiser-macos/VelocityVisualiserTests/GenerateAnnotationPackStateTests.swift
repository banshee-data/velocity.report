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
    runsJSON: String, exportHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))? = nil
) -> GenerateAnnotationPackState {
    let (runsSession, runsBaseURL, registerRuns) = AnnotationMockURLProtocol.makeSession()
    registerRuns { request in
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

    @Test func generateRejectsANonNumericMaxSamples() async {
        let state = makeStateUnderTest(runsJSON: twoRunsOneWithoutVRLog)
        await state.loadRuns()
        state.maxSamplesText = "2oo"

        let result = await state.generate()

        #expect(result == nil)
        #expect(state.lastError != nil)
    }

    @Test func generateRejectsAZeroOrNegativeMaxSamples() async {
        let state = makeStateUnderTest(runsJSON: twoRunsOneWithoutVRLog)
        await state.loadRuns()

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
        state.maxSamplesText = "   "

        let result = await state.generate()

        #expect(result != nil)
        #expect(state.lastError == nil)
    }
}
