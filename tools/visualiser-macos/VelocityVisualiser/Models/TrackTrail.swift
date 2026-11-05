// Historical geometry and support, independent of Metal and the app host.
import Foundation

struct TrackPoint {
    var x: Float = 0
    var y: Float = 0
    var timestampNanos: Int64 = 0
    var z: Float = 0
    var support: String = ""
    var estimateStage: String = ""
    var referenceAnchor: String = ""
    var revisionID: String = ""
    var sourceID: String = ""
    var frameTimestampNanos: Int64 = 0

    var captureTimestampNanos: Int64 {
        frameTimestampNanos != 0 ? frameTimestampNanos : timestampNanos
    }
}

struct TrackTrail {
    var trackID: String = ""
    var points: [TrackPoint] = []
}

enum TrailStyle: Equatable {
    case observed
    case predicted
    case unknown
    case provisional
    case final

    var colour: SIMD3<Float> {
        switch self {
        case .observed: return SIMD3(0.2, 0.8, 0.3)
        case .predicted: return SIMD3(0.96, 0.62, 0.04)
        case .unknown: return SIMD3(0.58, 0.64, 0.72)
        case .provisional: return SIMD3(0.22, 0.74, 0.97)
        case .final: return SIMD3(0.75, 0.52, 0.99)
        }
    }

    var dashed: Bool { self == .predicted || self == .unknown }
}

struct TrailEdge: Equatable {
    var start: SIMD3<Float>
    var end: SIMD3<Float>
    var style: TrailStyle
    var alpha: Float
}

enum TrailGeometry {
    static let maxPoints = 2000
    static let maxEdges = 100_000
    static let maxGapNanos: Int64 = 500_000_000

    /// Fresh reconstruction makes seeks and normal playback identical. No
    /// extrapolation, cross-identity joins or unrecorded support is invented.
    static func edges(
        trails: [TrackTrail], visibleIDs: Set<String>, timestampNanos: Int64
    ) -> [TrailEdge] {
        var result: [TrailEdge] = []
        let counts = Dictionary(trails.map { ($0.trackID, 1) }, uniquingKeysWith: +)
        for trail in trails
        where !trail.trackID.isEmpty && visibleIDs.contains(trail.trackID) && counts[trail.trackID] == 1 {

            var local: [TrailEdge] = []
            var previous: TrackPoint?
            let points = trail.points.suffix(maxPoints)
            let firstTime = points.first?.captureTimestampNanos ?? 0
            let lastTime = points.last?.captureTimestampNanos ?? firstTime
            let (duration, overflow) = lastTime.subtractingReportingOverflow(firstTime)
            let span = overflow ? 1 : max(duration, 1)
            for point in points {
                guard finite(point), timestampNanos == 0 || point.captureTimestampNanos <= timestampNanos
                else {
                    previous = nil
                    continue
                }
                defer { previous = point }
                guard let prior = previous else { continue }
                let (gap, overflows) = point.captureTimestampNanos.subtractingReportingOverflow(
                    prior.captureTimestampNanos)
                if !sameEstimate(prior, point) || (!overflows && gap <= 0) {
                    local.removeAll(keepingCapacity: true)
                    continue
                }
                guard !overflows, gap > 0, gap <= maxGapNanos else { continue }
                let (elapsed, elapsedOverflows) = point.captureTimestampNanos.subtractingReportingOverflow(
                    firstTime)
                let progress = elapsedOverflows ? 1 : min(1, max(0, Float(elapsed) / Float(span)))
                append(
                    start: position(prior), end: position(point), style: style(prior, point),
                    alpha: 0.1 + 0.7 * progress, to: &local, limit: maxEdges - result.count)
            }
            result.append(contentsOf: local)
            if result.count >= maxEdges { break }
        }
        return result
    }

    static func style(_ a: TrackPoint, _ b: TrackPoint) -> TrailStyle {
        let predicted = ["coasted", "occluded_inferred", "missed_unknown", "out_of_fov"]
        if predicted.contains(a.support) || predicted.contains(b.support) { return .predicted }
        guard a.support == "observed", b.support == "observed" else { return .unknown }
        if b.estimateStage == "final" { return .final }
        if b.estimateStage == "provisional" { return .provisional }
        return .observed
    }

    private static func finite(_ point: TrackPoint) -> Bool {
        point.x.isFinite && point.y.isFinite && point.z.isFinite
    }

    private static func position(_ point: TrackPoint) -> SIMD3<Float> {
        // Old recordings had XY only and drew at 0.1 m. Keep that explicitly
        // unknown legacy placement, rather than claim an observed road height.
        let height: Float =
            point.referenceAnchor.isEmpty || point.referenceAnchor == "filter_state_xy"
            ? 0.1 : point.z + 0.05
        return SIMD3(point.x, point.y, height)
    }

    private static func sameEstimate(_ a: TrackPoint, _ b: TrackPoint) -> Bool {
        a.sourceID == b.sourceID && a.estimateStage == b.estimateStage
            && a.revisionID == b.revisionID && a.referenceAnchor == b.referenceAnchor
    }

    private static func append(
        start: SIMD3<Float>, end: SIMD3<Float>, style: TrailStyle, alpha: Float,
        to edges: inout [TrailEdge], limit: Int
    ) {
        guard edges.count < limit else { return }
        let delta = end - start
        let distance = (delta.x * delta.x + delta.y * delta.y + delta.z * delta.z).squareRoot()
        guard distance.isFinite, distance > 0 else { return }
        if !style.dashed {
            edges.append(TrailEdge(start: start, end: end, style: style, alpha: alpha))
            return
        }
        // Disconnected world-distance dash geometry works with the ordinary
        // line pipeline and cannot bridge a missing sample or estimate change.
        var offset: Float = 0
        while offset < distance && edges.count < limit {
            let stop = min(offset + 0.35, distance)
            edges.append(
                TrailEdge(
                    start: start + delta * (offset / distance),
                    end: start + delta * (stop / distance), style: style, alpha: alpha))
            offset += 0.55
        }
    }
}
