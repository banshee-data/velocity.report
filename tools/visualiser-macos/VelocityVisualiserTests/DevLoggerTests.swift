//
//  DevLoggerTests.swift
//  VelocityVisualiserTests
//
//  Tests for the DevLogger lightweight logging wrapper.
//

import Foundation
import Testing

@testable import VelocityVisualiser

// MARK: - DevLogger Tests

struct DevLoggerTests {

    // MARK: - Initialisation

    @Test func initCreatesLoggerWithCategory() {
        let logger = DevLogger(category: "TestCategory")
        // Should not crash; logger is ready to use.
        _ = logger  // Silence unused warning
    }

    @Test func initWithEmptyCategory() {
        let logger = DevLogger(category: "")
        // Empty category is valid — should not crash.
        logger.debug("message with empty category")
    }

    @Test func initWithUnicodeCategory() {
        let logger = DevLogger(category: "Données")
        logger.debug("unicode category works")
    }

    // MARK: - Logging Methods

    @Test func debugDoesNotCrash() {
        let logger = DevLogger(category: "DebugTest")
        // debug() takes an @autoclosure — verify it evaluates without error.
        logger.debug("test debug message")
    }

    @Test func debugAutoclosureIsLazy() {
        let logger = DevLogger(category: "LazyTest")
        var evaluated = false
        logger.debug(
            {
                evaluated = true
                return "lazy message"
            }())
        // In DEBUG builds, the closure IS evaluated (print is called).
        // In Release builds it would be compiled out. Either way, no crash.
        #if DEBUG
            #expect(evaluated == true)
        #endif
    }

    @Test func infoDoesNotCrash() {
        let logger = DevLogger(category: "InfoTest")
        logger.info("test info message")
    }

    @Test func warningDoesNotCrash() {
        let logger = DevLogger(category: "WarningTest")
        logger.warning("test warning message")
    }

    @Test func errorDoesNotCrash() {
        let logger = DevLogger(category: "ErrorTest")
        logger.error("test error message")
    }

    @Test func allLevelsCanBeCalledSequentially() {
        let logger = DevLogger(category: "AllLevels")
        logger.debug("d")
        logger.info("i")
        logger.warning("w")
        logger.error("e")
    }

    // MARK: - Message Formatting

    @Test func debugWithInterpolatedString() {
        let logger = DevLogger(category: "Interpolation")
        let value = 42
        logger.debug("count = \(value)")
    }

    @Test func infoWithEmptyMessage() {
        let logger = DevLogger(category: "Empty")
        logger.info("")
    }

    @Test func warningWithLongMessage() {
        let logger = DevLogger(category: "Long")
        let longMessage = String(repeating: "x", count: 1000)
        logger.warning(longMessage)
    }

    @Test func errorWithSpecialCharacters() {
        let logger = DevLogger(category: "Special")
        logger.error("line1\nline2\ttab \"quoted\" 🚀")
    }

    // MARK: - Multiple Instances

    @Test func multipleLoggersWithDifferentCategories() {
        let logger1 = DevLogger(category: "Alpha")
        let logger2 = DevLogger(category: "Beta")
        logger1.info("from alpha")
        logger2.info("from beta")
    }

    @Test func loggerIsValueType() {
        let logger1 = DevLogger(category: "Original")
        var logger2 = logger1  // Copy (struct)
        _ = logger2
        logger1.debug("original still works after copy")
        logger2 = DevLogger(category: "Replaced")
        logger2.debug("replaced works too")
    }
}
