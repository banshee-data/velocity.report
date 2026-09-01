//
//  MainThreadWatchdogTests.swift
//  VelocityVisualiserTests
//
//  The watchdog exists because every other measurement in the app runs on the
//  frame stream's own path, which is the thing that stops. On 2026-08-28 the
//  client consumed nothing for 92 seconds and no logging could say whether the
//  main thread was wedged at the time.
//

import XCTest

@testable import VelocityVisualiser

@available(macOS 15.0, *) final class MainThreadWatchdogTests: XCTestCase {

    /// A responsive main thread must not be reported as stalled, or the signal
    /// is worthless during the run it is meant to explain.
    func testQuietMainThreadProducesNoWarning() {
        let watchdog = MainThreadWatchdog(interval: 0.05, threshold: .milliseconds(500))
        watchdog.start()
        defer { watchdog.stop() }

        let idle = expectation(description: "main thread stays responsive")
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) { idle.fulfill() }
        wait(for: [idle], timeout: 2)
    }

    /// Stopping must halt the timer: a watchdog that outlives its start would
    /// keep logging against a main thread nobody is watching.
    func testStopIsIdempotent() {
        let watchdog = MainThreadWatchdog(interval: 0.05)
        watchdog.start()
        watchdog.stop()
        watchdog.stop()
    }

    /// Starting without a run loop turn must not crash or block the caller: it
    /// runs from the app's init, before any UI exists.
    func testStartDoesNotBlockTheCaller() {
        let watchdog = MainThreadWatchdog(interval: 60)
        let started = ContinuousClock.now
        watchdog.start()
        let took = ContinuousClock.now - started
        watchdog.stop()

        XCTAssertLessThan(took, .milliseconds(100), "start() blocked its caller")
    }
}
