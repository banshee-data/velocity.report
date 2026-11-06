// Runs the actual pure trail geometry without a Metal device or an app host.
import Foundation

@main
struct TrailGeometrySmoke {
    static func check(_ condition: Bool, _ message: String) {
        if !condition { fatalError(message) }
    }

    static func point(_ x: Float, _ time: Int64) -> TrackPoint {
        TrackPoint(
            x: x, y: 2, timestampNanos: time, z: -2, support: "observed",
            estimateStage: "online", referenceAnchor: "published_box_center")
    }

    static func edges(_ points: [TrackPoint], time: Int64 = 0) -> [TrailEdge] {
        TrailGeometry.edges(
            trails: [TrackTrail(trackID: "a", points: points)], visibleIDs: ["a"],
            timestampNanos: time)
    }

    static func main() {
        let a = point(0, 1_700_000_000_000_000_001)
        let b = point(1, a.timestampNanos + 100_000_000)
        let solid = edges([a, b])
        check(solid.count == 1, "observed edge missing")
        check(solid[0].style == .observed, "observed style lost")
        check(abs(solid[0].start.z + 1.95) < 0.0001, "historical box base lost")
        check(edges([a, b]) == solid, "seek reconstruction differs")
        check(edges([a, b], time: a.timestampNanos).isEmpty, "future sample drawn")
        var timedA = a
        var timedB = b
        timedA.frameTimestampNanos = a.timestampNanos - 1
        timedB.frameTimestampNanos = b.timestampNanos - 1
        check(edges([timedA, timedB], time: timedB.frameTimestampNanos).count == 1,
              "acquisition time incorrectly suppressed frame-time geometry")
        var late = b
        late.timestampNanos = a.timestampNanos + 500_000_001
        check(edges([a, late]).isEmpty, "long gap joined")
        late.timestampNanos = a.timestampNanos
        check(edges([a, late]).isEmpty, "duplicate capture time joined")
        late.timestampNanos = a.timestampNanos - 1
        check(edges([a, late]).isEmpty, "backwards capture time joined")
        let changes: [(inout TrackPoint) -> Void] = [
            { $0.revisionID = "r2" }, { $0.sourceID = "source-2" },
            { $0.estimateStage = "final" }, { $0.referenceAnchor = "filter_state" },
        ]
        for change in changes {
            var c = point(2, b.timestampNanos + 100_000_000)
            var d = point(3, c.timestampNanos + 100_000_000)
            change(&c)
            change(&d)
            let revised = edges([a, b, c, d])
            check(revised.count == 1 && revised[0].start.x == 2, "mixed estimate revisions")
        }
        var predicted = b
        predicted.support = "coasted"
        let dashed = edges([a, predicted])
        check(dashed.count == 2 && dashed[0].style == .predicted, "coast not dashed")
        check(dashed[0].end.x < dashed[1].start.x, "dash has no gap")
        predicted.estimateStage = "final"
        check(TrailGeometry.style(a, predicted) == .predicted, "final hides predicted support")
        var legacyA = a
        var legacyB = b
        legacyA.support = ""
        legacyB.support = ""
        legacyA.referenceAnchor = ""
        legacyB.referenceAnchor = ""
        let unknown = edges([legacyA, legacyB])
        check(unknown[0].style == .unknown && unknown[0].start.z == 0.1, "legacy truth invented")
        var invalid = b
        invalid.x = .nan
        check(edges([a, invalid, point(2, b.timestampNanos + 1)]).isEmpty, "invalid sample bridged")
        let trail = TrackTrail(trackID: "a", points: [a, b])
        check(
            TrailGeometry.edges(trails: [trail], visibleIDs: [], timestampNanos: 0).isEmpty,
            "expired or hidden identity retained")
        check(
            TrailGeometry.edges(trails: [trail, trail], visibleIDs: ["a"], timestampNanos: 0).isEmpty,
            "duplicate trail identity accepted")
        var earliest = a
        var latest = b
        earliest.timestampNanos = .min
        latest.timestampNanos = .max
        check(edges([earliest, latest]).isEmpty, "timestamp overflow joined")
        let many = (0..<(TrailGeometry.maxPoints + 10)).map {
            point(Float($0), Int64($0))
        }
        check(edges(many).count == TrailGeometry.maxPoints - 1, "history limit ignored")
        let crowded = (0..<60).map { TrackTrail(trackID: String($0), points: many) }
        let capped = TrailGeometry.edges(
            trails: crowded, visibleIDs: Set(crowded.map { $0.trackID }), timestampNanos: 0)
        check(capped.count == TrailGeometry.maxEdges, "global geometry limit ignored")
        for stage in ["provisional", "final"] {
            var p = a
            var q = b
            p.estimateStage = stage
            q.estimateStage = stage
            check(edges([p, q])[0].style == (stage == "final" ? .final : .provisional), "stage style lost")
        }
        print("Native trail geometry contracts passed")
    }
}
