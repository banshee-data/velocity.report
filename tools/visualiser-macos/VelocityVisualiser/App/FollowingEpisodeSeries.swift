import Foundation

/// Display-only diagnostics over one loaded evidence window. Values remain the
/// exported projected gap arithmetic, never a replacement measurement or DRAC input.
struct FollowingEpisodeSeries {
    struct Sample: Identifiable {
        let timestamp: Int64
        let intervalNanos: Int64
        let leaderID: String?
        let gapM: Double?
        let supported: Bool
        let reason: String
        var id: Int64 { timestamp }
    }
    let samples: [Sample]
    let segments: [[Sample]]

    init(rows: [FollowingEvidence.Row], follower: String) {
        samples = rows.filter { $0.followerTrackId == follower }.sorted {
            $0.captureUnixNanos < $1.captureUnixNanos
        }.map { row in
            let gap = row.point?.gap?.valueM
            let reasons = (row.point?.reasons ?? []) + [row.point?.gap?.reason].compactMap { $0 }
            // Provisional estimates and headway's speed floor do not by themselves
            // invalidate gap arithmetic. All other suppressions break the trend.
            let allowed = Set(["estimate_not_final", "below_speed_floor", ""])
            let supported = row.bucket != "record_gap" && gap?.isFinite == true && (gap ?? 0) > 0
                && row.decision.leaderTrackId?.isEmpty == false
                && row.point?.follower?.leading.support == "observed"
                && row.point?.leader?.trailing.support == "observed"
                && reasons.allSatisfy { allowed.contains($0) }
            return Sample(
                timestamp: row.captureUnixNanos, intervalNanos: row.intervalNanos,
                leaderID: row.decision.leaderTrackId,
                gapM: gap?.isFinite == true ? gap : nil, supported: supported,
                reason: ([row.bucket] + reasons).joined(separator: " · "))
        }
        var result: [[Sample]] = []
        var previous: Sample?
        for sample in samples {
            defer { previous = sample }
            guard sample.supported else { continue }
            let end = previous?.timestamp.addingReportingOverflow(previous?.intervalNanos ?? 0)
            if let previous, previous.supported, previous.intervalNanos > 0,
                end?.overflow == false, end?.partialValue == sample.timestamp,
                previous.leaderID == sample.leaderID, !result.isEmpty
            {
                result[result.count - 1].append(sample)
            } else {
                result.append([sample])
            }
        }
        segments = result
    }

    /// Timestamp-windowed neighbourhood selection (not a fixed frame count).
    /// A centred moving mean is explicitly display-only and never crosses a break.
    func smoothed(_ segment: [Sample], radiusSeconds: Double = 0.2) -> [Double] {
        segment.map { sample in
            let values = segment.filter {
                abs(seconds($0.timestamp, from: sample.timestamp)) <= radiusSeconds
            }.compactMap(\.gapM)
            return values.reduce(0, +) / Double(max(1, values.count))
        }
    }

    func nearest(toSeconds target: Double, origin: Int64) -> Sample? {
        guard target.isFinite else { return nil }
        return samples.min {
            abs(seconds($0.timestamp, from: origin) - target)
                < abs(seconds($1.timestamp, from: origin) - target)
        }
    }

    func seconds(_ timestamp: Int64, from origin: Int64) -> Double {
        let difference = timestamp.subtractingReportingOverflow(origin)
        return difference.overflow
            ? (Double(timestamp) - Double(origin)) / 1e9 : Double(difference.partialValue) / 1e9
    }
}
