//
//  MainThreadWatchdog.swift
//  VelocityVisualiser
//
//  Measures main-thread responsiveness independently of the frame stream.
//
//  On 2026-08-28 the client stopped consuming the gRPC stream for 92 seconds
//  while the server sat blocked sending a 0.1 KB empty frame. Nothing in the
//  app's own logging could say whether the main thread was wedged at the time,
//  because every existing measurement ran on the stream's own path — the one
//  that had stopped.
//

import Foundation

/// Pings the main actor on a fixed interval and reports late replies.
///
/// The ping is scheduled off the main thread, so a stalled main thread delays
/// the reply rather than the schedule. The gap between the two is the stall.
final class MainThreadWatchdog: @unchecked Sendable {

    private let interval: TimeInterval
    private let threshold: Duration
    private var timer: DispatchSourceTimer?
    private let queue = DispatchQueue(label: "report.velocity.watchdog", qos: .utility)

    init(interval: TimeInterval = 1.0, threshold: Duration = .milliseconds(500)) {
        self.interval = interval
        self.threshold = threshold
    }

    func start() {
        let timer = DispatchSource.makeTimerSource(queue: queue)
        timer.schedule(deadline: .now() + interval, repeating: interval)
        timer.setEventHandler { [weak self] in
            guard let self else { return }
            let sent = ContinuousClock.now
            DispatchQueue.main.async {
                let waited = ContinuousClock.now - sent
                if waited > self.threshold {
                    vlog("⚠️ [Watchdog] main thread was unresponsive for \(waited.formattedMillis)")
                }
            }
        }
        timer.resume()
        self.timer = timer
        vlog("[Watchdog] started: pinging the main thread every \(interval)s")
    }

    func stop() {
        timer?.cancel()
        timer = nil
    }
}

extension ContinuousClock.Duration {
    /// Milliseconds to one decimal place, for timing logs.
    var formattedMillis: String {
        let (seconds, attoseconds) = components
        let millis = Double(seconds) * 1000 + Double(attoseconds) / 1_000_000_000_000_000
        return String(format: "%.1fms", millis)
    }
}
