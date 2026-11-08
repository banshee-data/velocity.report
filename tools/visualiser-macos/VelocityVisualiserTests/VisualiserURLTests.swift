//
//  VisualiserURLTests.swift
//  VelocityVisualiserTests
//
//  The velocityreport:// links: parsing, refusals, the inbox that holds
//  links until the app state is ready, and finding a linked run.
//

import Foundation
import XCTest

@testable import VelocityVisualiser

final class VisualiserURLParseTests: XCTestCase {
    private func parse(_ string: String) -> Result<VisualiserURL, VisualiserURL.ParseError> {
        VisualiserURL.parse(URL(string: string)!)
    }

    func testRunWithFrame() {
        XCTAssertEqual(
            parse("velocityreport://run/3f2a9c1e-7b4d-4e0a-9c55-0d1e2f3a4b5c?frame=42"),
            .success(.run(runID: "3f2a9c1e-7b4d-4e0a-9c55-0d1e2f3a4b5c", frame: 42)))
    }

    func testClipReplayRunIDAndFrameZero() {
        XCTAssertEqual(
            parse("velocityreport://run/replay-kirk0.a_1-1a2b3c4d?frame=0"),
            .success(.run(runID: "replay-kirk0.a_1-1a2b3c4d", frame: 0)))
    }

    func testMissingFrameOpensAtTheStart() {
        XCTAssertEqual(
            parse("velocityreport://run/run-001"), .success(.run(runID: "run-001", frame: nil)))
        XCTAssertEqual(
            parse("velocityreport://run/run-001/"), .success(.run(runID: "run-001", frame: nil)))
    }

    func testSchemeAndTargetAreCaseInsensitive() {
        XCTAssertEqual(
            parse("VelocityReport://RUN/run-001?frame=7"),
            .success(.run(runID: "run-001", frame: 7)))
    }

    func testOtherQueryItemsAreIgnored() {
        XCTAssertEqual(
            parse("velocityreport://run/run-001?track=t1&frame=5"),
            .success(.run(runID: "run-001", frame: 5)))
    }

    func testBadFrames() {
        for frame in ["-1", "abc", "", "1.5", "+3", "1e3", "99999999999999999999999"] {
            guard case .failure(.badFrame) = parse("velocityreport://run/run-001?frame=\(frame)")
            else { return XCTFail("frame=\(frame) was accepted") }
        }
        guard case .failure(.badFrame) = parse("velocityreport://run/run-001?frame=1&frame=2")
        else { return XCTFail("two frames were accepted") }
    }

    func testBadRunIDs() {
        for path in ["run/a/b", "run/..", "run/a%2Fb", "run/a%20b", "run/caf%C3%A9"] {
            guard case .failure(.badRunID) = parse("velocityreport://\(path)") else {
                return XCTFail("\(path) was accepted")
            }
        }
        let long = String(repeating: "a", count: VisualiserURL.maxRunIDLength + 1)
        guard case .failure(.badRunID) = parse("velocityreport://run/\(long)") else {
            return XCTFail("an over-long run ID was accepted")
        }
    }

    func testMissingRunID() {
        XCTAssertEqual(parse("velocityreport://run"), .failure(.missingRunID))
        XCTAssertEqual(parse("velocityreport://run/"), .failure(.missingRunID))
    }

    func testPackIsNotSupportedYet() {
        let result = parse("velocityreport://pack/kirk0-w3")
        XCTAssertEqual(result, .failure(.notSupportedYet("pack")))
        if case .failure(let error) = result {
            XCTAssertTrue(error.message.contains("not supported yet"), error.message)
        }
    }

    func testUnknownTargetAndScheme() {
        XCTAssertEqual(parse("velocityreport://view/abc"), .failure(.unknownTarget("view")))
        guard case .failure(.unknownTarget) = parse("velocityreport:run/run-001") else {
            return XCTFail("a link with no host was accepted")
        }
        XCTAssertEqual(parse("https://run/run-001"), .failure(.wrongScheme("https")))
    }

    func testEveryRefusalSaysSomething() {
        let errors: [VisualiserURL.ParseError] = [
            .wrongScheme("https"), .notSupportedYet("pack"), .unknownTarget("view"), .missingRunID,
            .badRunID("a/b"), .badFrame("x"),
        ]
        for error in errors { XCTAssertFalse(error.message.isEmpty) }
    }

    func testLinkRoundTrips() {
        let url = VisualiserURL.link(runID: "run-001", frame: 12)
        XCTAssertEqual(url?.absoluteString, "velocityreport://run/run-001?frame=12")
        XCTAssertEqual(url.map(VisualiserURL.parse), .success(.run(runID: "run-001", frame: 12)))
        XCTAssertEqual(
            VisualiserURL.link(runID: "run-001", frame: nil)?.absoluteString,
            "velocityreport://run/run-001")
        XCTAssertNil(VisualiserURL.link(runID: "a/b", frame: 1))
    }
}

@MainActor final class VisualiserURLInboxTests: XCTestCase {
    func testLinksWaitForTheHandler() {
        let inbox = VisualiserURLInbox()
        let first = URL(string: "velocityreport://run/a")!
        let second = URL(string: "velocityreport://run/b")!
        inbox.receive([first])
        var seen: [URL] = []
        inbox.setHandler { seen.append($0) }
        XCTAssertEqual(seen, [first])
        inbox.receive([second])
        XCTAssertEqual(seen, [first, second])
    }
}

@available(macOS 15.0, *) @MainActor final class VisualiserLinkOpeningTests: XCTestCase {
    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    func testRefusedLinkSaysWhyAndOpensNothing() async {
        let appState = AppState()
        await appState.openLink(URL(string: "velocityreport://pack/kirk0-w3")!)
        XCTAssertTrue(appState.linkMessage?.contains("not supported yet") == true)
        XCTAssertNil(appState.currentRunID)
        XCTAssertNil(appState.pendingLinkFrame)
    }

    private static let runJSON = """
        {"run_id": "old-run", "created_at": "2026-02-10T20:05:09-08:00", "source_type": "vrlog",
         "source_path": null, "sensor_id": "hesai-01", "duration_secs": 1.0, "total_frames": 10,
         "total_clusters": 0, "total_tracks": 0, "confirmed_tracks": 0, "status": "completed",
         "error_message": null, "vrlog_path": "/v/old-run", "notes": null}
        """

    private static func respond(
        _ request: URLRequest, _ status: Int, _ body: String
    ) -> (HTTPURLResponse, Data) {
        (
            HTTPURLResponse(
                url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!,
            Data(body.utf8)
        )
    }

    func testFindRunAsksTheServerForARunOutsideTheList() async {
        let state = RunBrowserState(apiClient: makeMockRunTrackClient())
        MockURLProtocol.requestHandler = { request in
            if request.url!.path == "/api/lidar/runs" {
                return Self.respond(request, 200, #"{"runs": [], "count": 0}"#)
            }
            XCTAssertEqual(request.url!.path, "/api/lidar/runs/old-run")
            return Self.respond(request, 200, Self.runJSON)
        }
        let result = await state.findRun("old-run")
        guard case .success(let run) = result else { return XCTFail("\(result)") }
        XCTAssertEqual(run.runId, "old-run")
        XCTAssertEqual(state.runs.map(\.runId), ["old-run"])
    }

    func testFindRunReportsAnUnknownRun() async {
        let state = RunBrowserState(apiClient: makeMockRunTrackClient())
        MockURLProtocol.requestHandler = { request in
            if request.url!.path == "/api/lidar/runs" {
                return Self.respond(request, 200, #"{"runs": [], "count": 0}"#)
            }
            return Self.respond(request, 404, "not found")
        }
        let result = await state.findRun("missing")
        XCTAssertEqual(result.failureValue, .notFound)
        XCTAssertTrue(state.runs.isEmpty)
    }
}

extension Result {
    fileprivate var failureValue: Failure? {
        if case .failure(let error) = self { return error }
        return nil
    }
}
