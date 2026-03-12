// DevLogger.swift
// Lightweight wrapper around os.Logger that prints to stdout in DEBUG builds.
//
// In Debug configuration (`make dev-mac`), all log levels print to the
// terminal so you can see them alongside the app output.  In Release
// (`make build-mac`), only `.error` and above are printed; `.debug`
// messages are compiled out entirely.
//
// Usage — drop-in replacement for os.Logger:
//
//     private let logger = DevLogger(category: "AppState")
//     logger.debug("togglePlayPause() called")

import os

struct DevLogger {
    private let osLogger: Logger
    private let tag: String

    init(category: String) {
        self.osLogger = Logger(subsystem: "report.velocity.visualiser", category: category)
        self.tag = category
    }

    /// Debug level — printed to stdout only in DEBUG builds.
    /// Completely compiled out in Release.
    func debug(_ message: @autoclosure () -> String) {
        #if DEBUG
            let msg = message()
            print("[\(tag)] \(msg)")
        #endif
    }

    /// Info level — always goes to unified log; printed to stdout in DEBUG.
    func info(_ message: String) {
        #if DEBUG
            print("[\(tag)] \(message)")
        #endif
        osLogger.info("\(message, privacy: .public)")
    }

    /// Warning level — always goes to unified log; printed to stdout in DEBUG.
    func warning(_ message: String) {
        #if DEBUG
            print("[\(tag)] ⚠️ \(message)")
        #endif
        osLogger.warning("\(message, privacy: .public)")
    }

    /// Error level — always printed to stdout and unified log.
    func error(_ message: String) {
        print("[\(tag)] ❌ \(message)")
        osLogger.error("\(message, privacy: .public)")
    }
}

struct TraceInterval {
    fileprivate let name: StaticString
    fileprivate let state: OSSignpostIntervalState

    func end(_ message: String? = nil) {
        if let message, !message.isEmpty {
            PerformanceTrace.signposter.endInterval(name, state, "\(message, privacy: .public)")
        } else {
            PerformanceTrace.signposter.endInterval(name, state)
        }
    }
}

enum PerformanceTrace {
    static let signposter = OSSignposter(
        subsystem: "report.velocity.visualiser", category: "Performance")

    static func begin(_ name: StaticString, _ message: String? = nil) -> TraceInterval {
        let state: OSSignpostIntervalState
        if let message, !message.isEmpty {
            state = signposter.beginInterval(name, "\(message, privacy: .public)")
        } else {
            state = signposter.beginInterval(name)
        }
        return TraceInterval(name: name, state: state)
    }

    static func event(_ name: StaticString, _ message: String) {
        signposter.emitEvent(name, "\(message, privacy: .public)")
    }
}
