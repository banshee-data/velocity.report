import SwiftUI

/// Shared by the native inspector and review window; uses the existing exact
/// timestamp seek and source-bound 3D overlay, not a second analysis pipeline.
struct FollowingEpisodeChart: View {
    @EnvironmentObject var appState: AppState
    @ObservedObject var review: FollowingReviewSession
    @State private var showTrend = false

    private var sourceMatches: Bool {
        review.managedRunID != nil && review.managedRunID == appState.currentRunID
            && review.managedTransport?.baseURL == VelocityReportServer.currentHTTPURL
    }

    private var canSeek: Bool {
        review.managedRunID != nil && review.managedRunID == appState.currentRunID
            && review.managedTransport?.baseURL == VelocityReportServer.currentHTTPURL
            && appState.isConnected && appState.isPaused && appState.hasValidTimelineRange
            && appState.displayPlaybackMode == .replaySeekable && !appState.playbackControlsBusy
            && !review.loading && !review.loadingWindow && review.managedDraft == nil
            && review.pendingSave == nil && !review.savingRecord
    }

    var body: some View {
        if sourceMatches, let document = review.document, let slice = document.reviewSlice {
            let series = FollowingEpisodeSeries(rows: document.rows, follower: review.follower)
            VStack(alignment: .leading, spacing: 5) {
                Text("Follower gap · per capture frame").font(.caption.bold())
                if series.samples.isEmpty {
                    Text("No samples for this follower in the loaded window.").font(.caption)
                } else {
                    plot(series, origin: slice.sampleStartNanos, end: slice.sampleEndExclusiveNanos)
                    if let selected = series.samples.first(where: {
                        $0.timestamp == Int64(review.timestamp)
                    }) {
                        Text(
                            "\(String(format: "%.3f", series.seconds(selected.timestamp, from: slice.sampleStartNanos))) s · leader \(selected.leaderID.map { String($0.suffix(8)) } ?? "unresolved") · \(selected.gapM.map { String(format: "%.3f m", $0) } ?? "gap unavailable")"
                        ).font(.caption).monospacedDigit()
                        Text(selected.reason.replacingOccurrences(of: "_", with: " "))
                            .font(.caption2).foregroundStyle(.secondary)
                    }
                    Toggle("Display trend (±0.2 s mean)", isOn: $showTrend)
                        .toggleStyle(.checkbox).font(.caption)
                    Text("Raw estimates, not physical truth. Orange = unsupported; grey ticks = no gap. Dashed rules = leader switches. Lines stop at missing time.")
                        .font(.caption2).foregroundStyle(.secondary)
                    Text("Seconds from loaded window start. Click a sample to seek; pause playback first.")
                        .font(.caption2).foregroundStyle(.secondary)
                }
            }.onChange(of: appState.selectedTrackID) { _, id in
                guard sourceMatches else { return }
                review.followPlayback(at: appState.currentTimestamp, selectedTrackID: id)
                appState.refreshFollowingOverlay()
            }
        }
    }

    private func plot(_ series: FollowingEpisodeSeries, origin: Int64, end: Int64) -> some View {
        let duration = max(0.001, series.seconds(end, from: origin))
        let values = series.samples.compactMap(\.gapM)
        let low = min(0, values.min() ?? 0)
        let high = max(low + 1, values.max() ?? 1)
        return Canvas { context, size in
            let left: Double = 44
            let width = max(1, size.width - left - 8)
            let height = max(1, size.height - 24)
            func x(_ timestamp: Int64) -> Double {
                left + series.seconds(timestamp, from: origin) / duration * width
            }
            func y(_ value: Double) -> Double { 4 + (high - value) / (high - low) * (height - 8) }
            for value in [low, (low + high) / 2, high] {
                let py = y(value)
                var grid = Path()
                grid.move(to: CGPoint(x: left, y: py))
                grid.addLine(to: CGPoint(x: left + width, y: py))
                context.stroke(grid, with: .color(.secondary.opacity(0.2)))
                context.draw(Text(String(format: "%.1f m", value)).font(.caption2),
                    at: CGPoint(x: left - 4, y: py), anchor: .trailing)
            }
            for segment in series.segments {
                var raw = Path()
                for (index, sample) in segment.enumerated() {
                    let point = CGPoint(x: x(sample.timestamp), y: y(sample.gapM ?? 0))
                    if index == 0 { raw.move(to: point) } else { raw.addLine(to: point) }
                }
                context.stroke(raw, with: .color(.cyan.opacity(0.5)), lineWidth: 1)
                if showTrend {
                    let trend = series.smoothed(segment)
                    var line = Path()
                    for (index, sample) in segment.enumerated() {
                        let point = CGPoint(x: x(sample.timestamp), y: y(trend[index]))
                        if index == 0 { line.move(to: point) } else { line.addLine(to: point) }
                    }
                    context.stroke(line, with: .color(.purple), lineWidth: 2)
                }
            }
            for timestamp in series.leaderSwitches {
                var rule = Path()
                rule.move(to: CGPoint(x: x(timestamp), y: 0))
                rule.addLine(to: CGPoint(x: x(timestamp), y: height))
                context.stroke(rule, with: .color(.purple.opacity(0.5)),
                    style: StrokeStyle(lineWidth: 1, dash: [3, 3]))
            }
            for sample in series.samples {
                let px = x(sample.timestamp)
                if let value = sample.gapM {
                    context.fill(Path(ellipseIn: CGRect(x: px - 2, y: y(value) - 2, width: 4, height: 4)),
                        with: .color(sample.supported ? .cyan : .orange))
                } else {
                    context.fill(Path(CGRect(x: px, y: height - 5, width: 2, height: 5)), with: .color(.gray))
                }
            }
            if let timestamp = Int64(review.timestamp), timestamp >= origin, timestamp < end {
                var cursor = Path()
                cursor.move(to: CGPoint(x: x(timestamp), y: 0))
                cursor.addLine(to: CGPoint(x: x(timestamp), y: height))
                context.stroke(cursor, with: .color(.primary.opacity(0.7)))
            }
            context.draw(Text("0 s").font(.caption2), at: CGPoint(x: left, y: size.height - 8), anchor: .leading)
            context.draw(Text(String(format: "%.2f s", duration)).font(.caption2),
                at: CGPoint(x: left + width, y: size.height - 8), anchor: .trailing)
        }.frame(height: 160).overlay {
            GeometryReader { geometry in
                Color.clear.contentShape(Rectangle()).onTapGesture { location in
                    guard canSeek else { return }
                    let fraction = min(1, max(0, (location.x - 44) / max(1, geometry.size.width - 52)))
                    if let sample = series.nearest(toSeconds: fraction * duration, origin: origin) {
                        appState.selectTrack(review.follower, showInspector: false)
                        appState.seekToTimestamp(sample.timestamp)
                        review.followPlayback(at: sample.timestamp, selectedTrackID: review.follower)
                        appState.refreshFollowingOverlay()
                    }
                }
            }
        }.accessibilityLabel("Per-frame projected bumper gap in metres. Raw cyan points, unsupported orange points, optional purple display trend.")
    }
}
