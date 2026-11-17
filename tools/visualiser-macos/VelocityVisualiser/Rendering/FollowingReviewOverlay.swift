import Foundation
import Metal
import simd

struct FollowingReviewOverlay {
    var followerID: String?
    var leaderID: String?
    var candidateIDs: Set<String> = []
    var vertices: [Float] = []
    var notice = "3D overlay unavailable: no matching frame and review sample."

    static func make(
        frame: FrameBundle, runID: String?, metadata: FollowingReviewMetadata?,
        managedRunID: String?, geometry: FollowingReviewGeometry?, row: FollowingEvidence.Row?,
        path: FollowingReviewGeometry.Path?
    ) -> Self {
        guard let row, let metadata, runID == managedRunID, runID == metadata.runId,
            frame.timestampNanos == row.captureUnixNanos, let tracks = frame.tracks,
            tracks.tracks.contains(where: { $0.trackID == row.followerTrackId })
        else { return Self() }
        if let mode = frame.playbackInfo?.sourceMode, mode != .vrlog && mode != .unspecified {
            return Self(notice: "3D overlay unavailable: playback is not a recording replay.")
        }
        // Track IDs alone are insufficient across source/version changes.
        let sources = Set(
            tracks.trails.flatMap { $0.points }.filter {
                $0.captureTimestampNanos == row.captureUnixNanos
            }.map(\.sourceID).filter { !$0.isEmpty })
        let playback = frame.playbackInfo
        let hasRecordingIdentity =
            playback?.recordingPath.isEmpty == false
            || playback?.recordingHeaderSHA256.isEmpty == false
        if hasRecordingIdentity {
            guard let playback, playback.sourceMode == .vrlog,
                let expectedPath = metadata.recordingPath, !expectedPath.isEmpty,
                playback.recordingPath == expectedPath,
                playback.recordingHeaderSHA256 == metadata.recordingHeaderSha256,
                playback.recordingHeaderSHA256.hasPrefix("sha256:"),
                playback.recordingHeaderSHA256.utf8.count == 71,
                playback.recordingHeaderSHA256.dropFirst(7).allSatisfy({
                    "0123456789abcdef".contains($0)
                })
            else {
                return Self(
                    notice: "3D overlay unavailable: recording identity does not match the review.")
            }
            // A recording receipt may establish a source omitted by old trail
            // formats. It must never override contradictory recorded sources.
            guard sources.isEmpty || sources == [metadata.sourceId] else {
                return Self(
                    notice: "3D overlay unavailable: frame sources conflict with the review.")
            }
        } else {
            guard sources == [metadata.sourceId] else {
                return Self(
                    notice: "3D overlay unavailable: frame source identity is missing or different."
                )
            }
        }
        let visible = Set(tracks.tracks.map(\.trackID))
        let leader = row.decision.leaderTrackId.flatMap { visible.contains($0) ? $0 : nil }
        var result = Self(
            followerID: row.followerTrackId, leaderID: leader,
            candidateIDs: Set((row.decision.candidates ?? []).map(\.trackId)).intersection(visible),
            notice:
                "3D pair matches; a supported gap and matching coordinate geometry are required.")
        guard let geometry, geometry.recordingCoordinates, geometry.sourceId == metadata.sourceId,
            geometry.calibrationId == metadata.calibrationId,
            geometry.canonicalEvidenceSha256 == metadata.evidenceSha256, let path, leader != nil,
            let z = tracks.tracks.first(where: { $0.trackID == row.followerTrackId })?.z, z.isFinite
        else { return result }
        let scene = FollowingRelationshipScene(row: row, path: path)
        // This is a projected gap at the follower's display height, not a
        // reconstructed physical bumper surface or a surveyed road plane.
        for pair in zip(scene.gap, scene.gap.dropFirst()) {
            for point in [pair.0, pair.1] {
                let x = Float(point.x)
                let y = Float(point.y)
                guard x.isFinite, y.isFinite else { return Self() }
                result.vertices += [x, y, z, 0.15, 0.85, 0.95, 1]
            }
        }
        if !result.vertices.isEmpty {
            result.notice =
                hasRecordingIdentity
                ? "3D gap: recording and analysis match."
                : "3D gap: frame source and analysis match."
        }
        return result
    }
}

extension AppState {
    var currentFollowingReviewOverlay: FollowingReviewOverlay {
        guard let frame = currentFrame,
            followingReview.managedTransport?.baseURL == VelocityReportServer.currentHTTPURL
        else { return FollowingReviewOverlay() }
        let review = followingReview
        return .make(
            frame: frame, runID: currentRunID, metadata: review.managedMetadata,
            managedRunID: review.managedRunID, geometry: review.managedGeometry,
            row: review.selectedRow, path: review.selectedPath)
    }

    func refreshFollowingOverlay() {
        renderer?.setFollowingReviewOverlay(currentFollowingReviewOverlay)
    }
}
