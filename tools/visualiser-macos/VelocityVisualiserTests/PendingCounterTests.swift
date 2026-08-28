//
//  PendingCounterTests.swift
//  VelocityVisualiserTests
//
//  The read loop no longer waits for the main actor on every frame. Awaiting it
//  is what let a stalled main thread stall the transport: the loop could not
//  advance, the client stopped reading, and the server's send blocked behind it
//  for tens of seconds (37.3s on 2026-08-28, on an 18.5KB frame). This counter
//  is what keeps the replacement bounded.
//

import XCTest

@testable import VelocityVisualiser

@available(macOS 15.0, *) final class PendingCounterTests: XCTestCase {

    func testIncrementReturnsTheNewDepth() {
        let counter = PendingCounter()

        XCTAssertEqual(counter.increment(), 1)
        XCTAssertEqual(counter.increment(), 2)
        XCTAssertEqual(counter.depth, 2)
    }

    func testDecrementReducesTheDepth() {
        let counter = PendingCounter()
        counter.increment()
        counter.increment()

        counter.decrement()

        XCTAssertEqual(counter.depth, 1)
    }

    /// A stray decrement must not drive the count negative: that would let the
    /// queue grow past its bound before the check trips again.
    func testDecrementCannotGoNegative() {
        let counter = PendingCounter()

        counter.decrement()
        counter.decrement()

        XCTAssertEqual(counter.depth, 0)
        XCTAssertEqual(counter.increment(), 1)
    }

    /// The counter is read from the stream loop and written from the main
    /// actor, so it has to hold up under concurrent use.
    func testConcurrentIncrementsAreNotLost() {
        let counter = PendingCounter()
        let iterations = 1_000

        DispatchQueue.concurrentPerform(iterations: iterations) { _ in
            counter.increment()
        }

        XCTAssertEqual(counter.depth, iterations)

        DispatchQueue.concurrentPerform(iterations: iterations) { _ in
            counter.decrement()
        }

        XCTAssertEqual(counter.depth, 0)
    }

    /// The bound is deliberately small: the queue absorbs a hiccup, it does not
    /// buffer a backlog the renderer would only discard later.
    func testTheBoundIsSmall() {
        XCTAssertLessThanOrEqual(
            VisualiserClient.maxPendingMainActorFrames, 5,
            "a large bound reintroduces the backlog this replaced")
        XCTAssertGreaterThan(VisualiserClient.maxPendingMainActorFrames, 0)
    }
}
