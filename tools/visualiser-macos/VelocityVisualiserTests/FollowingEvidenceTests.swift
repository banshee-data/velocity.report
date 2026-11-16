import Foundation
import XCTest

@testable import VelocityVisualiser

final class FollowingEvidenceTests: XCTestCase {
    private var fixture: String {
        """
        {"schema":"following_evidence_v1",
         "context":{"site_namespace":"test","site_id":"site","source_id":"source/v2/a",
                    "binding_id":"binding","analysis_build_git_sha":"build","policy_id":"policy"},
         "analysis_version":{"estimate_stage":"online","estimator_id":"est","obs_model_id":"obs",
                             "method_id":"method","param_hash":"params"},
         "analysis":{"timelines":[{"follower_track_id":"f","geometry_id":"path"}]},
         "rows":[{"follower_track_id":"f","capture_unix_nanos":1788380529986405330,
                  "interval_nanos":0,"bucket":"pairing_excluded",
                  "decision":{"follower_track_id":"f","capture_unix_nanos":1788380529986405330,
                              "reason":"ambiguous_leader","candidates":[
                              {"track_id":"a","disposition":"competing","established":true,"synchronised":true,"lateral_m":0.3},
                              {"track_id":"b","disposition":"competing","established":true,"synchronised":false}]},
                  "experimental_drac":{"status":"no_candidate","calculation":null}}]}
        """
    }

    func testExactNanosecondsAndDistinctCandidatesRemainUncalculated() throws {
        let doc = try FollowingEvidence.decode(Data(fixture.utf8))
        let instant: Int64 = 1_788_380_529_986_405_330
        XCTAssertEqual(doc.rows(at: instant, follower: "f").count, 1)
        XCTAssertTrue(doc.rows(at: instant + 1, follower: "f").isEmpty)
        XCTAssertTrue(doc.rows(at: instant, follower: "another-version-f").isEmpty)
        XCTAssertEqual(doc.rows[0].decision.candidates?.map(\.trackId), ["a", "b"])
        XCTAssertNil(doc.rows[0].point)
        XCTAssertNil(doc.rows[0].experimentalDrac.calculation)
        XCTAssertTrue(doc.frameSourceNote([]).contains("unavailable"))
        XCTAssertTrue(doc.frameSourceNote(["source/v2/other"]).contains("differs"))
        XCTAssertTrue(doc.frameSourceNote(["source/v2/a", "source/v2/other"]).contains("differs"))
        XCTAssertTrue(doc.frameSourceNote(["source/v2/a"]).contains("still require verification"))
    }

    func testRefusesUnsupportedSchemaAndConflictingIdentity() {
        for bad in [
            fixture.replacingOccurrences(
                of: "following_evidence_v1", with: "following_evidence_v2"),
            fixture.replacingOccurrences(
                of: "\"binding_id\":\"binding\"", with: "\"binding_id\":\"\""),
            fixture.replacingOccurrences(of: "\"track_id\":\"b\"", with: "\"track_id\":\"a\""),
            fixture.replacingOccurrences(of: "\"track_id\":\"a\"", with: "\"track_id\":\"f\""),
            fixture.replacingOccurrences(of: "\"interval_nanos\":0", with: "\"interval_nanos\":-1"),
            fixture.replacingOccurrences(
                of: "\"calculation\":null",
                with: "\"calculation\":{\"status\":\"supported\",\"value_mps2\":1}"),
        ] { XCTAssertThrowsError(try FollowingEvidence.decode(Data(bad.utf8))) }
    }

    func testReviewSlicesKeepExactSamplesAndRequireTheirProvenance() throws {
        let sha = "sha256:" + String(repeating: "a", count: 64)
        let metadata = """
            "review_slice":{"canonical_evidence_sha256":"\(sha)",
            "sample_start_nanos":1788380529986405320,"sample_end_exclusive_nanos":1788380529986405340,
            "recording_header_sha256":"\(sha)","recording_build_git_sha":"producer","calibration_id":"calibration/v1/a"},
            """
        let json = fixture.replacingOccurrences(
            of: "\"schema\":\"following_evidence_v1\",",
            with: "\"schema\":\"following_review_slice_v1\"," + metadata)
        let doc = try FollowingEvidence.decode(Data(json.utf8))
        XCTAssertEqual(doc.rows[0].captureUnixNanos, 1_788_380_529_986_405_330)
        XCTAssertEqual(doc.reviewSlice?.canonicalEvidenceSha256, sha)
        XCTAssertNil(doc.rows[0].experimentalDrac.calculation)
        for bad in [
            json.replacingOccurrences(of: "1788380529986405340", with: "1788380529986405325"),
            json.replacingOccurrences(of: sha, with: "missing"),
            json.replacingOccurrences(
                of: "following_review_slice_v1", with: "following_evidence_v1"),
            fixture.replacingOccurrences(
                of: "following_evidence_v1", with: "following_review_slice_v1"),
        ] { XCTAssertThrowsError(try FollowingEvidence.decode(Data(bad.utf8))) }
    }
}
