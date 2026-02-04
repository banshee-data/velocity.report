// VelocityVisualiserUITestsLaunchTests.swift
// Launch tests for VelocityVisualiser
//
// This file is a placeholder to maintain Xcode project structure.

import XCTest

final class VelocityVisualiserUITestsLaunchTests: XCTestCase {

    override class var runsForEachTargetApplicationUIConfiguration: Bool { true }

    override func setUpWithError() throws { continueAfterFailure = false }

    @MainActor func testLaunch() throws {
        let app = XCUIApplication()
        app.launch()

        // Take a screenshot on launch
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = "Launch Screen"
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
