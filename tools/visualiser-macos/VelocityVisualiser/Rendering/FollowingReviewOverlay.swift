import Foundation
import Metal
import simd

struct FollowingReviewOverlay {
    var followerID: String?
    var leaderID: String?
    var candidateIDs: Set<String> = []
    var vertices: [Float] = []

    static func make(
        frame: FrameBundle, runID: String?, metadata: FollowingReviewMetadata?,
        managedRunID: String?, geometry: FollowingReviewGeometry?, row: FollowingEvidence.Row?,
        path: FollowingReviewGeometry.Path?
    ) -> Self {
        guard let row, let metadata, runID == managedRunID, runID == metadata.runId,
            frame.timestampNanos == row.captureUnixNanos, let tracks = frame.tracks,
            tracks.tracks.contains(where: { $0.trackID == row.followerTrackId })
        else { return Self() }
        // Track IDs alone are insufficient across source/version changes.
        let sources = Set(
            tracks.trails.flatMap { $0.points }.filter {
                $0.captureTimestampNanos == row.captureUnixNanos
            }.map(\.sourceID).filter { !$0.isEmpty })
        guard sources == [metadata.sourceId] else { return Self() }
        let visible = Set(tracks.tracks.map(\.trackID))
        let leader = row.decision.leaderTrackId.flatMap { visible.contains($0) ? $0 : nil }
        var result = Self(
            followerID: row.followerTrackId, leaderID: leader,
            candidateIDs: Set((row.decision.candidates ?? []).map(\.trackId)).intersection(visible))
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
        return result
    }
}

extension AppState {
    func refreshFollowingOverlay() {
        guard let frame = currentFrame,
            followingReview.managedTransport?.baseURL == VelocityReportServer.currentHTTPURL
        else {
            renderer?.setFollowingReviewOverlay(FollowingReviewOverlay())
            return
        }
        let review = followingReview
        renderer?.setFollowingReviewOverlay(
            .make(
                frame: frame, runID: currentRunID, metadata: review.managedMetadata,
                managedRunID: review.managedRunID, geometry: review.managedGeometry,
                row: review.selectedRow, path: review.selectedPath))
    }
}
