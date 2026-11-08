//
//  RunGroupingTests.swift
//  VelocityVisualiserTests
//
//  The run browser's clip groups and folded repeats, the clip summary read
//  from /api/lidar/clips, and the Annotation window's title.
//

import Foundation
import XCTest

@testable import VelocityVisualiser

private func makeRun(
    _ id: String, clip: String? = nil, params: String? = "sha256:a",
    source: String = "/pcaps/kirk0.pcapng", sensor: String = "hesai-01"
) -> Run {
    Run(
        runId: id, createdAt: Date(), sourceType: "pcap", sourcePath: source, sensorId: sensor,
        durationSecs: 60, totalFrames: 600, totalClusters: 10, totalTracks: 5, confirmedTracks: 5,
        status: "completed", errorMessage: nil, vrlogPath: "/vrlog/\(id)", notes: nil,
        sourceName: nil, clipId: clip, paramsHash: params)
}

final class RunGroupingTests: XCTestCase {
    private func ids(_ group: RunClipGroup) -> [[String]] {
        group.rows.map { [$0.run.runId] + $0.folded.map(\.runId) }
    }

    func testGroupsByClipInRunOrderWithNoClipLast() {
        let runs = [
            makeRun("r1", clip: "c-b", params: "p1"), makeRun("r2", params: "p2"),
            makeRun("r3", clip: "c-a", params: "p3"), makeRun("r4", clip: "c-b", params: "p4"),
            makeRun("r5", clip: "  ", params: "p5"),
        ]
        let clips = [
            "c-a": ClipSummary(clipId: "c-a", description: "kirk static-0"),
            "c-b": ClipSummary(
                clipId: "c-b", description: "kirk2-1",
                location: .init(siteId: "89c25", s2L16Display: "Kirkham at 9th")),
        ]
        let groups = RunGrouping.groups(runs, clips: clips)
        XCTAssertEqual(groups.map(\.title), ["kirk2-1", "kirk static-0", "No clip"])
        XCTAssertEqual(groups.map(\.clipID), ["c-b", "c-a", nil])
        XCTAssertEqual(groups[0].site, "Kirkham at 9th")
        XCTAssertNil(groups[1].site)
        XCTAssertEqual(ids(groups[0]), [["r1"], ["r4"]])
        // A blank clip ID is no clip.
        XCTAssertEqual(ids(groups[2]), [["r2"], ["r5"]])
        XCTAssertEqual(groups.map(\.runCount), [2, 1, 2])
    }

    func testUnknownClipIsNamedByItsID() {
        let groups = RunGrouping.groups(
            [makeRun("r1", clip: "aed614a3-34de-46ab-b1c0-6fd89efc14a9")], clips: [:])
        XCTAssertEqual(groups.map(\.title), ["Clip aed614a3"])
    }

    func testNoClipGroupOnlyWhenNeeded() {
        let groups = RunGrouping.groups([makeRun("r1", clip: "c")], clips: [:])
        XCTAssertEqual(groups.count, 1)
        XCTAssertTrue(RunGrouping.groups([], clips: [:]).isEmpty)
    }

    func testFoldsConsecutiveRepeatsOnly() {
        let runs = [
            makeRun("r1"), makeRun("r2"), makeRun("r3"), makeRun("r4", params: "sha256:b"),
            makeRun("r5"),
        ]
        let rows = RunGrouping.fold(runs)
        XCTAssertEqual(rows.map { $0.run.runId }, ["r1", "r4", "r5"])
        XCTAssertEqual(rows[0].folded.map(\.runId), ["r2", "r3"])
        XCTAssertEqual(rows[0].foldLabel, "+ 2 more, identical parameters")
        XCTAssertNil(rows[1].foldLabel)
        XCTAssertNil(rows[2].foldLabel)
    }

    func testRepeatsNeedTheSameDigestCaptureAndSensor() {
        XCTAssertTrue(RunGrouping.repeats(makeRun("a"), makeRun("b")))
        XCTAssertFalse(RunGrouping.repeats(makeRun("a"), makeRun("b", params: "sha256:b")))
        XCTAssertFalse(RunGrouping.repeats(makeRun("a"), makeRun("b", source: "/pcaps/other")))
        XCTAssertFalse(RunGrouping.repeats(makeRun("a"), makeRun("b", sensor: "hesai-02")))
        XCTAssertFalse(RunGrouping.repeats(makeRun("a", clip: "c1"), makeRun("b", clip: "c2")))
        // A run with no digest, from an older server, repeats nothing.
        XCTAssertFalse(RunGrouping.repeats(makeRun("a", params: nil), makeRun("b", params: nil)))
        XCTAssertFalse(RunGrouping.repeats(makeRun("a", params: ""), makeRun("b", params: "")))
    }

    func testFoldedRunsCountInTheGroup() {
        let groups = RunGrouping.groups(
            [makeRun("r1", clip: "c"), makeRun("r2", clip: "c"), makeRun("r3", clip: "c")],
            clips: [:])
        XCTAssertEqual(ids(groups[0]), [["r1", "r2", "r3"]])
        XCTAssertEqual(groups[0].runCount, 3)
    }
}

final class ClipSummaryTests: XCTestCase {
    func testDecodesTheClipsList() throws {
        let json = """
            {"clips": [
              {"clip_id": "aed614a3-34de", "sensor_id": "hesai", "description": "kirk static-0",
               "location": {"origin_lat": 1, "origin_lon": 2, "site_id": "89c25",
                            "s2_l16_display": "Kirkham at 9th", "geographic_status": "ok"},
               "file_count": 1, "storage": "sources", "created_at_ns": 1},
              {"clip_id": "4ca6a3f8-8903", "sensor_id": "hesai", "file_count": 0,
               "storage": "sources", "created_at_ns": 2}
            ]}
            """
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let clips = try decoder.decode(ClipsResponse.self, from: Data(json.utf8)).clips
        XCTAssertEqual(clips.map(\.name), ["kirk static-0", "Clip 4ca6a3f8"])
        XCTAssertEqual(clips.map(\.site), ["Kirkham at 9th", nil])
    }

    func testSiteFallsBackToTheSiteID() {
        let clip = ClipSummary(clipId: "c", location: .init(siteId: "89c25", s2L16Display: ""))
        XCTAssertEqual(clip.site, "89c25")
    }
}

final class AnnotationWindowTitleTests: XCTestCase {
    func testClipPackAndWindow() {
        XCTAssertEqual(
            AnnotationWindowTitle.format(
                clip: "kirk static-0", pack: "run1-20261014-215113", runID: "run1", window: (2, 5)),
            "Annotation: kirk static-0 · run1-20261014-215113 · window 2 of 5")
    }

    func testClipWithoutWindow() {
        XCTAssertEqual(
            AnnotationWindowTitle.format(clip: "kirk2-1", pack: "pack", runID: "run1"),
            "Annotation: kirk2-1 · pack")
    }

    func testRunIDStandsInForAMissingClip() {
        XCTAssertEqual(
            AnnotationWindowTitle.format(clip: nil, pack: "pack", runID: "run1"),
            "Annotation: run1 · pack")
        // Not repeated when the pack's name already begins with it.
        XCTAssertEqual(
            AnnotationWindowTitle.format(clip: "", pack: "run1-20261014", runID: "run1"),
            "Annotation: run1-20261014")
    }

    func testNoPackAndImpossibleWindows() {
        XCTAssertEqual(AnnotationWindowTitle.format(clip: "c", pack: nil), "Annotation")
        XCTAssertEqual(
            AnnotationWindowTitle.format(clip: "c", pack: "p", window: (0, 5)), "Annotation: c · p")
        XCTAssertEqual(
            AnnotationWindowTitle.format(clip: "c", pack: "p", window: (6, 5)), "Annotation: c · p")
        XCTAssertEqual(
            AnnotationWindowTitle.format(clip: "c", pack: "p", window: (1, 0)), "Annotation: c · p")
    }

    func testRunIDOfAVRLogPath() {
        XCTAssertEqual(
            AnnotationWindowTitle.runID(ofVRLogPath: "/sensor_data/lidar/vrlog/b4ca577a-7f05"),
            "b4ca577a-7f05")
        XCTAssertEqual(AnnotationWindowTitle.runID(ofVRLogPath: "/vrlog/r1/"), "r1")
        XCTAssertNil(AnnotationWindowTitle.runID(ofVRLogPath: ""))
    }
}

@available(macOS 15.0, *) @MainActor final class RunBrowserClipNameTests: XCTestCase {
    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    func testClipNameForARunReadsItsClip() async {
        let state = RunBrowserState(apiClient: makeMockRunTrackClient())
        MockURLProtocol.requestHandler = { request in
            let body: String
            switch request.url!.path {
            case "/api/lidar/runs":
                body = """
                    {"runs": [{"run_id": "r1", "created_at": "2026-02-10T20:05:09-08:00",
                      "source_type": "pcap", "source_path": "/p", "sensor_id": "s",
                      "duration_secs": 1, "total_frames": 1, "total_clusters": 0,
                      "total_tracks": 0, "confirmed_tracks": 0, "status": "completed",
                      "error_message": null, "vrlog_path": "/v/r1", "notes": null,
                      "clip_id": "c1", "params_hash": "sha256:a"}], "count": 1}
                    """
            case "/api/lidar/clips":
                body = #"{"clips": [{"clip_id": "c1", "description": "kirk2-1"}]}"#
            default: body = "{}"
            }
            return (
                HTTPURLResponse(
                    url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!,
                Data(body.utf8)
            )
        }
        let name = await state.clipName(forRun: "r1")
        XCTAssertEqual(name, "kirk2-1")
        XCTAssertEqual(state.runs.first?.paramsHash, "sha256:a")
        XCTAssertEqual(state.clips["c1"]?.name, "kirk2-1")
    }
}
