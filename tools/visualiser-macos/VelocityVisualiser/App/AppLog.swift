//
//  AppLog.swift
//  VelocityVisualiser
//
//  Diagnostic output for the app's own logging.
//
//  Two problems this solves, both found on 2026-08-28 when `make run-mac`
//  first redirected the app to a file. Redirected stdout is block-buffered, so
//  most of the app's output sat in a 4 KB buffer and the captured log ended
//  mid-word; and nothing carried a timestamp, so app events could not be lined
//  up against the server's logs.
//

import Foundation

/// Timestamped, line-buffered diagnostic output.
enum AppLog {

    /// Forces line buffering on stdout so output reaches a redirected log as it
    /// happens. A terminal is line-buffered already; a file or pipe is not, and
    /// the difference is invisible until the log is the only evidence there is.
    static func configure() { setvbuf(stdout, nil, _IOLBF, 0) }

    private static let formatter: DateFormatter = {
        let f = DateFormatter()
        // Matches the server's log timestamps so the two can be read together.
        f.dateFormat = "yyyy/MM/dd HH:mm:ss.SSS"
        return f
    }()

    /// Writes one timestamped line.
    static func write(_ message: String) { print("\(formatter.string(from: Date())) \(message)") }
}

/// Shorthand for `AppLog.write`, used in place of bare `print` for diagnostics.
func vlog(_ message: String) { AppLog.write(message) }
