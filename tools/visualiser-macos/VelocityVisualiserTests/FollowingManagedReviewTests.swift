import CryptoKit
import Foundation
import XCTest

@testable import VelocityVisualiser

private final class FollowingReviewTestProtocol: URLProtocol, @unchecked Sendable {
    static let lock = NSLock()
    nonisolated(unsafe) static var handler: ((URLRequest) throws -> (Int, Data, [String: String]))?
    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        do {
            let handler = Self.lock.withLock { Self.handler }
            let (status, data, headers) = try XCTUnwrap(handler)(request)
            let response = HTTPURLResponse(
                url: request.url!, statusCode: status, httpVersion: nil, headerFields: headers)!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch { client?.urlProtocol(self, didFailWithError: error) }
    }
    override func stopLoading() {}
}

@MainActor final class FollowingManagedReviewTests: XCTestCase {
    private let canonical = "sha256:" + String(repeating: "a", count: 64)
    private let header = "sha256:" + String(repeating: "b", count: 64)
    private var transport: FollowingReviewTransport {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [FollowingReviewTestProtocol.self]
        return FollowingReviewTransport(
            baseURL: URL(string: "http://review.test")!, session: URLSession(configuration: config))
    }

    private func slice(_ at: Int64 = 1000) -> Data {
        let start = at < 2000 ? 1000 : 2000
        let endpoint =
            #"{"track_id":"f","arc_m":4,"sigma_m":0.1,"source":"observed","support":"observed","extent_converged":true}"#
        return Data(
            """
            {"schema":"following_review_slice_v1",
             "context":{"site_namespace":"test","site_id":"site","source_id":"source/v2/a","binding_id":"binding","analysis_build_git_sha":"build","policy_id":"policy"},
             "analysis_version":{"estimate_stage":"online","estimator_id":"est","obs_model_id":"obs","method_id":"method","param_hash":"params"},
             "review_slice":{"canonical_evidence_sha256":"\(canonical)","sample_start_nanos":\(start),"sample_end_exclusive_nanos":\(start+1000),"recording_header_sha256":"\(header)","recording_build_git_sha":"build","calibration_id":"calibration"},
             "analysis":{"timelines":[{"follower_track_id":"f","geometry_id":"path"}]},
             "rows":[{"follower_track_id":"f","capture_unix_nanos":\(start),"interval_nanos":1000,"bucket":"following_eligible",
              "decision":{"follower_track_id":"f","capture_unix_nanos":\(start),"leader_track_id":"l","candidates":[{"track_id":"l","disposition":"leader","established":true,"synchronised":true,"centre_arc_m":12,"lateral_m":0}]},
              "point":{"follower_track_id":"f","leader_track_id":"l","capture_unix_nanos":\(start),"supported_opportunity":true,"follower":{"leading":\(endpoint),"trailing":\(endpoint)}},
              "experimental_drac":{"status":"non_closing","calculation":{"status":"non_closing","closing_speed_mps":-1}}}]}
            """.utf8)
    }

    private func metadata() throws -> FollowingReviewMetadata {
        try FollowingReviewTransport.decode(FollowingReviewMetadata.self, from: metadataBytes())
    }
    private func metadataBytes() -> Data {
        Data(
            """
            {"schema":"following_review_session_v1","run_id":"run","evidence_sha256":"\(canonical)","recording_header_sha256":"\(header)","source_id":"source/v2/a","calibration_id":"calibration","start_nanos":1000,"end_exclusive_nanos":3000,"slices":[{"id":"0","start_nanos":1000,"end_exclusive_nanos":2000,"rows":1},{"id":"1","start_nanos":2000,"end_exclusive_nanos":3000,"rows":1}]}
            """.utf8)
    }
    private func geometryBytes() -> Data {
        Data(
            """
            {"schema":"following_review_geometry_v1","source_id":"source/v2/a","calibration_id":"calibration","canonical_evidence_sha256":"\(canonical)","coordinate_frame_id":"sensor","recording_coordinates":true,"paths":[{"follower_track_id":"f","path":{"id":"path","vertices":[{"x":0,"y":0,"arc_m":0},{"x":20,"y":0,"arc_m":20}]}}]}
            """.utf8)
    }
    private func install(failPost: Bool = false, corruptDigest: Bool = false) throws {
        let first = slice()
        let second = slice(2000)
        let geometry = geometryBytes()
        let metadata = metadataBytes()
        let canonical = self.canonical
        let doc = try FollowingEvidence.decode(first)
        let emptyAudit = FollowingAudit(binding: .init(evidence: doc, sha256: canonical))
        let auditBytes = try emptyAudit.encoded()
        FollowingReviewTestProtocol.lock.withLock {
            FollowingReviewTestProtocol.handler = { request in
                if request.httpMethod == "POST" {
                    if failPost {
                        return (503, Data(#"{"error":"temporarily unavailable"}"#.utf8), [:])
                    }
                    var body = request.httpBody ?? Data()
                    if body.isEmpty, let stream = request.httpBodyStream {
                        stream.open()
                        defer { stream.close() }
                        var buffer = [UInt8](repeating: 0, count: 4096)
                        while stream.hasBytesAvailable {
                            let n = stream.read(&buffer, maxLength: buffer.count)
                            if n <= 0 { break }
                            body.append(contentsOf: buffer.prefix(n))
                        }
                    }
                    let payload = try XCTUnwrap(
                        JSONSerialization.jsonObject(with: body) as? [String: Any])
                    return (
                        201,
                        try JSONSerialization.data(withJSONObject: [
                            "created": true, "record": try XCTUnwrap(payload["record"]),
                        ]), [:]
                    )
                }
                let path = request.url!.lastPathComponent
                if path == "following-review" { return (200, metadata, [:]) }
                if path == "audit" { return (200, auditBytes, [:]) }
                if path == "geometry" { return (200, geometry, [:]) }
                let at = URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?
                    .queryItems?.first?.value
                let bytes = at == "2000" ? second : first
                let digest =
                    "sha256:" + SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined()
                return (
                    200, bytes,
                    [
                        "X-Evidence-SHA256": corruptDigest ? "wrong" : digest,
                        "X-Canonical-Evidence-SHA256": canonical,
                    ]
                )
            }
        }
    }

    func testOneRecordingLoadsEvidenceGeometryAndAuditThenCrossesSliceAutomatically() async throws {
        try install()
        let review = FollowingReviewSession()
        await review.openRecording(runID: "run", at: 1000, transport: transport)
        XCTAssertNil(review.error)
        XCTAssertEqual(review.managedRunID, "run")
        XCTAssertNil(review.evidenceURL)
        XCTAssertEqual(review.fileIdentity, canonical)
        XCTAssertEqual(review.selectedRow?.captureUnixNanos, 1000)
        XCTAssertEqual(review.selectedPath?.point(at: 4), SIMD2(4, 0))
        review.followPlayback(at: 2000, selectedTrackID: "f")
        await review.windowTask?.value
        XCTAssertNil(review.error)
        XCTAssertEqual(review.selectedRow?.captureUnixNanos, 2000)
        XCTAssertEqual(review.fileIdentity, canonical)
        review.recordingChanged(to: "another")
        XCTAssertNil(review.document)
        XCTAssertNil(review.managedGeometry)
        XCTAssertFalse(review.loadingWindow)
    }

    func testRecordingReviewDoesNotReplaceAnActiveFileImport() async {
        let review = FollowingReviewSession()
        XCTAssertTrue(review.beginImport())
        await review.openRecording(runID: "run", at: 1000, transport: transport)
        XCTAssertNil(review.managedRunID)
        XCTAssertTrue(review.loading)
        XCTAssertNotNil(review.error)
        review.finishImport()
    }

    func testTamperedEvidenceDoesNotEnterTheReview() async throws {
        try install(corruptDigest: true)
        let review = FollowingReviewSession()
        await review.openRecording(runID: "run", at: 1000, transport: transport)
        XCTAssertNotNil(review.error)
        XCTAssertNil(review.document)
        XCTAssertNil(review.managedGeometry)
    }

    func testFailedSaveRetainsSameRecordAndCanRetryAcrossRecordingChange() async throws {
        try install(failPost: true)
        let review = FollowingReviewSession()
        await review.openRecording(runID: "run", at: 1000, transport: transport)
        var record = FollowingAudit.Record(
            row: try XCTUnwrap(review.selectedRow), playback: .init(runID: "run"))
        record.reviewer = "operator"
        record.notes = "Leader remains uncertain"
        await review.saveManagedRecord(record)
        XCTAssertEqual(review.pendingSave?.record.id, record.id)
        XCTAssertNotNil(review.error)
        review.recordingChanged(to: nil)
        XCTAssertEqual(review.pendingSave?.runID, "run")
        try install()
        await review.retryPendingSave()
        XCTAssertNil(review.pendingSave)
        XCTAssertFalse(review.savingRecord)
    }

    func testDraftSurvivesSourceChangeAndSubmissionKeepsOriginalBinding() async throws {
        try install()
        let review = FollowingReviewSession()
        await review.openRecording(runID: "run", at: 1000, transport: transport)
        var record = FollowingAudit.Record(
            row: try XCTUnwrap(review.selectedRow), playback: .init(runID: "run"))
        record.reviewer = "operator"
        record.notes = "Uncertain through occlusion"
        review.startDraft(record)
        review.managedDraft?.endText = "0.000002000"
        let registry = UnsavedWorkRegistry()
        registry.register(review)
        XCTAssertNotNil(registry.unsavedSummary)
        review.recordingChanged(to: "another")
        XCTAssertEqual(review.managedDraft?.record.notes, record.notes)
        XCTAssertEqual(review.managedDraft?.endText, "0.000002000")
        try review.submitDraft(record)
        XCTAssertNil(review.managedDraft)
        XCTAssertEqual(review.pendingSave?.runID, "run")
        XCTAssertEqual(review.pendingSave?.binding.evidenceSHA256, canonical)
        XCTAssertEqual(review.pendingSave?.record.id, record.id)
        await review.retryPendingSave()
        XCTAssertNil(review.error)
        XCTAssertNil(review.pendingSave)
        XCTAssertNil(registry.unsavedSummary)
    }

    func testDRACDisplayDoesNotDependOnHeadwayBucket() throws {
        let bytes = String(decoding: slice(), as: UTF8.self).replacingOccurrences(
            of: "following_eligible", with: "speed_floor"
        ).replacingOccurrences(of: #""status":"non_closing""#, with: #""status":"supported""#)
            .replacingOccurrences(
                of: #""closing_speed_mps":-1"#, with: #""closing_speed_mps":2,"value_mps2":0.5"#)
        let doc = try FollowingEvidence.decode(Data(bytes.utf8))
        XCTAssertEqual(followingDRACValue(try XCTUnwrap(doc.rows.first)), 0.5)
        let nonClosing = try FollowingEvidence.decode(slice())
        XCTAssertNil(followingDRACValue(try XCTUnwrap(nonClosing.rows.first)))
    }

    func testPathProjectionRefusesUnobservedExtentAndSceneRetainsCandidateIdentity() async throws {
        try install()
        let geometry = try await transport.geometry(runID: "run", at: 1000, metadata: metadata())
        let path = try XCTUnwrap(geometry.paths.first?.path)
        XCTAssertNil(path.point(at: -1))
        XCTAssertNil(path.point(at: 21))
        XCTAssertEqual(path.point(at: 5, lateral: 2), SIMD2(5, 2))
        let row = try XCTUnwrap(FollowingEvidence.decode(slice()).rows.first)
        let scene = FollowingRelationshipScene(row: row, path: path)
        XCTAssertEqual(scene.actors.first?.id, "l")
        XCTAssertTrue(scene.actors.first?.leader == true)
        var record = FollowingAudit.Record(row: row, playback: .init())
        record.reviewer = "operator"
        record.notes = "disputed leader"
        record.proposedLeaderTrackID = "not-a-candidate"
        XCTAssertThrowsError(
            try FollowingAudit.validate(record, evidence: FollowingEvidence.decode(slice())))
    }

    func testUnboundPlaybackNeverProducesAnOverlay() throws {
        let row = try XCTUnwrap(FollowingEvidence.decode(slice()).rows.first)
        let overlay = FollowingReviewOverlay.make(
            frame: FrameBundle(), runID: "wrong", metadata: try metadata(), managedRunID: "run",
            geometry: nil, row: row, path: nil)
        XCTAssertNil(overlay.followerID)
        XCTAssertTrue(overlay.vertices.isEmpty)
    }

    func testAuditFromAnotherMinuteAndFractionalUTCDateRemainReadable() throws {
        let first = try FollowingEvidence.decode(slice())
        var record = FollowingAudit.Record(row: first.rows[0], playback: .init(runID: "run"))
        record.reviewer = "operator"
        record.notes = "Endpoints not visible"
        var audit = FollowingAudit(binding: .init(evidence: first, sha256: canonical))
        audit.records = [record]
        let data = try audit.encoded()
        XCTAssertFalse(
            String(decoding: data, as: UTF8.self).contains("\n"),
            "Keep the native byte budget consistent with compact backend storage")
        var object = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        var records = try XCTUnwrap(object["records"] as? [[String: Any]])
        records[0]["savedAt"] = "2026-11-16T18:00:00.123456Z"
        object["records"] = records
        let fromServer = try JSONSerialization.data(withJSONObject: object)
        let second = try FollowingEvidence.decode(slice(2000))
        let decoded = try FollowingAudit.decode(
            fromServer, evidence: second, sha256: canonical, validatePeriods: false)
        XCTAssertEqual(decoded.records[0].id, record.id)
        XCTAssertThrowsError(
            try FollowingAudit.decode(fromServer, evidence: second, sha256: canonical))
    }

    func testGapUsesSupportedPathSegmentsAndExactFrameSourceOnly() async throws {
        try install()
        let geometry = try await transport.geometry(runID: "run", at: 1000, metadata: metadata())
        let path = try XCTUnwrap(geometry.paths.first?.path)
        var object = try XCTUnwrap(JSONSerialization.jsonObject(with: slice()) as? [String: Any])
        var rows = try XCTUnwrap(object["rows"] as? [[String: Any]])
        var point = try XCTUnwrap(rows[0]["point"] as? [String: Any])
        let rear: [String: Any] = [
            "track_id": "l", "arc_m": 10, "sigma_m": 0.1, "source": "observed",
            "support": "observed", "extent_converged": true,
        ]
        point["leader"] = ["leading": rear, "trailing": rear]
        rows[0]["point"] = point
        object["rows"] = rows
        let row = try XCTUnwrap(
            FollowingEvidence.decode(JSONSerialization.data(withJSONObject: object)).rows.first)
        var frame = FrameBundle()
        frame.timestampNanos = 1000
        var f = Track()
        f.trackID = "f"
        f.z = 1
        var l = Track()
        l.trackID = "l"
        l.z = 1
        var provenance = TrackPoint()
        provenance.timestampNanos = 1000
        provenance.sourceID = "source/v2/a"
        frame.tracks = TrackSet(
            tracks: [f, l], trails: [TrackTrail(trackID: "f", points: [provenance])])
        let good = FollowingReviewOverlay.make(
            frame: frame, runID: "run", metadata: try metadata(), managedRunID: "run",
            geometry: geometry, row: row, path: path)
        XCTAssertEqual(good.followerID, "f")
        XCTAssertEqual(good.leaderID, "l")
        XCTAssertEqual(good.vertices.count, 14)
        XCTAssertEqual(good.vertices[0], 4)
        XCTAssertEqual(good.vertices[7], 10)
        frame.timestampNanos = 1001
        XCTAssertNil(
            FollowingReviewOverlay.make(
                frame: frame, runID: "run", metadata: try metadata(), managedRunID: "run",
                geometry: geometry, row: row, path: path
            ).followerID)
        frame.timestampNanos = 1000
        frame.tracks?.trails = []
        XCTAssertNil(
            FollowingReviewOverlay.make(
                frame: frame, runID: "run", metadata: try metadata(), managedRunID: "run",
                geometry: geometry, row: row, path: path
            ).followerID)
    }

    func testTimelineNeverMergesAcrossMissingTime() throws {
        let a = try FollowingEvidence.decode(slice()).rows[0]
        let b = try FollowingEvidence.decode(slice(2000)).rows[0]
        XCTAssertEqual(followingTimelineSpans([a, b], follower: "f").count, 1)
        XCTAssertEqual(followingTimelineSpans([a, b], follower: "another").count, 0)
        let late = String(decoding: slice(2000), as: UTF8.self).replacingOccurrences(
            of: "2000", with: "2500"
        ).replacingOccurrences(of: "3000", with: "3500")
        let c = try FollowingEvidence.decode(Data(late.utf8)).rows[0]
        XCTAssertEqual(followingTimelineSpans([a, c], follower: "f").count, 2)
    }

    func testMultiVehicleChainKeepsEachFollowersOwnGap() async throws {
        try install()
        let geometry = try await transport.geometry(runID: "run", at: 1000, metadata: metadata())
        let path = try XCTUnwrap(geometry.paths.first?.path)
        func endpoint(_ id: String, _ arc: Double) -> [String: Any] {
            [
                "track_id": id, "arc_m": arc, "sigma_m": 0.1, "source": "observed",
                "support": "observed", "extent_converged": true,
            ]
        }
        var object = try XCTUnwrap(JSONSerialization.jsonObject(with: slice()) as? [String: Any])
        var rows = try XCTUnwrap(object["rows"] as? [[String: Any]])
        var primary = rows[0]
        var point = try XCTUnwrap(primary["point"] as? [String: Any])
        let lbody = ["leading": endpoint("l", 14), "trailing": endpoint("l", 10)]
        point["leader"] = lbody
        point["gap"] = ["value_m": 6, "sigma_m": 0.1]
        primary["point"] = point
        var decision = try XCTUnwrap(primary["decision"] as? [String: Any])
        let a: [String: Any] = [
            "track_id": "a", "disposition": "blocked", "established": true, "synchronised": true,
            "centre_arc_m": 19, "lateral_m": 0,
        ]
        var candidates = try XCTUnwrap(decision["candidates"] as? [[String: Any]])
        candidates.append(a)
        decision["candidates"] = candidates
        primary["decision"] = decision
        var other = primary
        other["follower_track_id"] = "l"
        decision["follower_track_id"] = "l"
        decision["leader_track_id"] = "a"
        decision["candidates"] = [a]
        other["decision"] = decision
        point["follower_track_id"] = "l"
        point["leader_track_id"] = "a"
        point["follower"] = lbody
        point["leader"] = ["leading": endpoint("a", 20), "trailing": endpoint("a", 18)]
        point["gap"] = ["value_m": 4, "sigma_m": 0.1]
        other["point"] = point
        rows = [primary, other]
        object["rows"] = rows
        let doc = try FollowingEvidence.decode(JSONSerialization.data(withJSONObject: object))
        let scene = FollowingRelationshipScene(
            row: doc.rows[0], path: path, contextRows: doc.rows,
            paths: [.init(followerTrackId: "l", path: path, conditions: nil)])
        XCTAssertEqual(scene.gap.first, SIMD2(4, 0))
        XCTAssertEqual(scene.gap.last, SIMD2(10, 0))
        XCTAssertEqual(scene.otherGaps.count, 1)
        XCTAssertEqual(scene.otherGaps[0].followerID, "l")
        XCTAssertEqual(scene.otherGaps[0].leaderID, "a")
        XCTAssertEqual(scene.otherGaps[0].gapM, 4)
    }
}
