import Foundation
import XCTest

@testable import VelocityVisualiser

final class FollowingAuditTests: XCTestCase {
    private let instant: Int64 = 1_788_380_529_986_405_330
    private let digest = "sha256:" + String(repeating: "a", count: 64)

    @MainActor func testPlaybackContextRetainsOnlyExactFrameSourceEvidence() {
        let state = AppState(autoConnectOnStartup: false)
        state.currentRunID = "review-run"
        state.currentTimestamp = instant
        state.logStartTimestamp = instant - 100
        state.logEndTimestamp = instant + 100
        var frame = FrameBundle()
        frame.timestampNanos = instant
        frame.coordinateFrame = CoordinateFrameInfo(frameID: "sensor-frame")
        frame.playbackInfo = PlaybackInfo(
            recordingPath: "/opened/recording", recordingHeaderSHA256: digest)
        var point = TrackPoint()
        point.timestampNanos = instant
        point.sourceID = "source/v2/b"
        var other = point
        other.sourceID = "source/v2/a"
        var old = point
        old.timestampNanos = instant - 1
        old.sourceID = "source/v2/old"
        var unknown = point
        unknown.sourceID = ""
        frame.tracks = TrackSet(
            tracks: [],
            trails: [TrackTrail(trackID: "f", points: [point, other, point, old, unknown])])
        state.currentFrame = frame
        let context = state.currentFollowingAuditPlayback
        XCTAssertEqual(context.runID, "review-run")
        XCTAssertEqual(context.captureNanos, instant)
        XCTAssertEqual(context.startNanos, instant - 100)
        XCTAssertEqual(context.endNanos, instant + 100)
        XCTAssertEqual(context.coordinateFrameID, "sensor-frame")
        XCTAssertEqual(context.frameSourceIDs, ["source/v2/a", "source/v2/b"])
        XCTAssertEqual(context.frameRecordingPath, "/opened/recording")
        XCTAssertEqual(context.frameRecordingHeaderSHA256, digest)

        state.currentTimestamp = instant + 1
        XCTAssertTrue(state.currentFollowingAuditPlayback.frameSourceIDs.isEmpty)
        XCTAssertNil(state.currentFollowingAuditPlayback.coordinateFrameID)
        XCTAssertNil(state.currentFollowingAuditPlayback.frameRecordingPath)
        XCTAssertNil(state.currentFollowingAuditPlayback.frameRecordingHeaderSHA256)
        state.currentFrame = nil
        XCTAssertTrue(state.currentFollowingAuditPlayback.frameSourceIDs.isEmpty)
    }

    private func evidence(source: String = "source/v2/a") throws -> FollowingEvidence {
        let rows: [[String: Any]] = (0..<3).map { index in
            let time = instant + Int64(index * 100)
            return [
                "follower_track_id": "f", "capture_unix_nanos": time,
                "interval_nanos": index == 2 ? 0 : 100, "bucket": "pairing_excluded",
                "decision": [
                    "follower_track_id": "f", "capture_unix_nanos": time,
                    "reason": index == 0 ? "ambiguous_leader" : "no_leader",
                ], "experimental_drac": ["status": "no_candidate"],
            ]
        }
        let value: [String: Any] = [
            "schema": "following_evidence_v1",
            "context": [
                "site_namespace": "test", "site_id": "site", "source_id": source,
                "binding_id": "binding", "analysis_build_git_sha": "build", "policy_id": "policy",
            ],
            "analysis_version": [
                "estimate_stage": "online", "estimator_id": "est", "obs_model_id": "obs",
                "method_id": "method", "param_hash": "params",
            ], "analysis": ["timelines": [["follower_track_id": "f", "geometry_id": "path"]]],
            "rows": rows,
        ]
        return try FollowingEvidence.decode(JSONSerialization.data(withJSONObject: value))
    }

    private func record(_ doc: FollowingEvidence) -> FollowingAudit.Record {
        var record = FollowingAudit.Record(row: doc.rows[0], playback: .init())
        record.reviewer = "reviewer-a"
        record.savedAt = Date(timeIntervalSince1970: 1_800_000_000)
        record.notes = "Two competing candidates; body continuity and endpoints are unknown."
        return record
    }

    @MainActor func testOnlyOneImportOwnsTheReviewSession() {
        let session = FollowingReviewSession()
        session.error = "previous failure"
        XCTAssertTrue(session.beginImport())
        XCTAssertNil(session.error)
        XCTAssertTrue(session.loading)
        // A recreated Inspector observes this same owner, rather than a fresh
        // view-local busy flag. It cannot overlap or clear the first import.
        XCTAssertFalse(session.beginImport())
        XCTAssertTrue(session.loading)
        session.error = "read failed"
        session.finishImport()
        XCTAssertFalse(session.loading)
        XCTAssertEqual(session.error, "read failed")
        XCTAssertTrue(session.beginImport())
        XCTAssertNil(session.error)
        session.finishImport()
    }

    @MainActor func testUnsavedAuditParticipatesInQuitGuard() {
        let session = FollowingReviewSession()
        session.auditUnsaved = true
        XCTAssertTrue(
            UnsavedWorkRegistry.shared.unsavedSummary?.contains("following/DRAC audit") == true)
        session.auditUnsaved = false
        XCTAssertFalse(
            UnsavedWorkRegistry.shared.unsavedSummary?.contains("following/DRAC audit") == true)
    }

    func testNegativePeriodRoundTripsWithoutQualifyingOrChangingEvidence() throws {
        let doc = try evidence()
        var audit = FollowingAudit(binding: .init(evidence: doc, sha256: digest))
        var item = record(doc)
        item.endExclusiveNanos = instant + 200
        try audit.append(item, evidence: doc)
        let data = try audit.encoded()
        XCTAssertEqual(try FollowingAudit.decode(data, evidence: doc, sha256: digest), audit)
        XCTAssertEqual(audit.records[0].startNanos, instant)
        XCTAssertTrue(String(decoding: data, as: UTF8.self).contains(String(instant)))
        XCTAssertTrue(String(decoding: data, as: UTF8.self).contains("2027-01-15T08:00:00Z"))
        XCTAssertEqual(audit.records[0].conclusion, .insufficientEvidence)
        XCTAssertEqual(audit.records[0].leaderAndPath, .unknown)
        XCTAssertTrue(audit.records[0].playback.frameSourceIDs.isEmpty)
        XCTAssertEqual(doc.rows[0].experimentalDrac.status, "no_candidate")
        XCTAssertNil(doc.rows[0].experimentalDrac.calculation)
    }

    func testRejectsWrongBytesVersionSourceSchemaAndDuplicateRecords() throws {
        let doc = try evidence()
        var audit = FollowingAudit(binding: .init(evidence: doc, sha256: digest))
        let item = record(doc)
        try audit.append(item, evidence: doc)
        XCTAssertThrowsError(try audit.append(item, evidence: doc))
        var other = record(try evidence(source: "source/v2/b"))
        other.id = UUID()
        XCTAssertThrowsError(try audit.append(other, evidence: evidence(source: "source/v2/b")))
        XCTAssertThrowsError(
            try FollowingAudit.decode(audit.encoded(), evidence: doc, sha256: "other"))
        let encoded = String(decoding: try audit.encoded(), as: UTF8.self)
        for (original, changed) in [
            ("source/v2/a", "source/v2/b"), ("params", "other"),
            ("following_audit_v1", "following_audit_v2"),
        ] {
            XCTAssertTrue(
                encoded.contains(original), "The mutation must change the encoded binding.")
            let data = Data(encoded.replacingOccurrences(of: original, with: changed).utf8)
            XCTAssertThrowsError(try FollowingAudit.decode(data, evidence: doc, sha256: digest))
        }
        audit.records.append(item)
        XCTAssertThrowsError(
            try FollowingAudit.decode(audit.encoded(), evidence: doc, sha256: digest))
    }

    func testRejectsUnboundPeriodsAndUnsubstantiatedConsistentConclusion() throws {
        let doc = try evidence()
        for mutation: (inout FollowingAudit.Record) -> Void in [
            { $0.startNanos += 1 }, { $0.endExclusiveNanos = $0.startNanos - 1 },
            { $0.endExclusiveNanos = Int64.max }, { $0.followerTrackID = "different-track" },
            { $0.reviewer = " " }, { $0.notes = "\n" }, { $0.conclusion = .consistent },
        ] {
            var item = record(doc)
            mutation(&item)
            XCTAssertThrowsError(try FollowingAudit.validate(item, evidence: doc))
        }
        var item = record(doc)
        item.conclusion = .consistent
        item.identityContinuity = .consistent
        item.leaderAndPath = .consistent
        item.visibleEndpoints = .consistent
        item.speedEvidence = .consistent
        XCTAssertNoThrow(try FollowingAudit.validate(item, evidence: doc))
        item.conclusion = .disagreement
        item.visibleEndpoints = .disputed
        XCTAssertNoThrow(try FollowingAudit.validate(item, evidence: doc))
        var terminal = FollowingAudit.Record(row: doc.rows[2], playback: .init())
        terminal.reviewer = "reviewer-a"
        terminal.notes = "Terminal instant only."
        XCTAssertEqual(terminal.startNanos, terminal.endExclusiveNanos)
        XCTAssertNoThrow(try FollowingAudit.validate(terminal, evidence: doc))
    }
}
