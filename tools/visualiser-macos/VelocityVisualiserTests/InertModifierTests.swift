//
//  InertModifierTests.swift
//  VelocityVisualiserTests
//
//  17 of 24 AttributeGraph cycles captured on 2026-08-28 were rooted at
//  `-[NSCell setEnabled:]` → `nextValidKeyView` → AppKit recomputing the
//  window's key-view loop → back into SwiftUI's view graph, inside the update
//  that changed the enabled state. Controls whose availability flips during an
//  update use `.inert` instead, which never touches AppKit's enabled state.
//

import SwiftUI
import XCTest

@testable import VelocityVisualiser

@available(macOS 15.0, *) @MainActor final class InertModifierTests: XCTestCase {

    /// Walks up from this file to find ContentView.swift, so the test does not
    /// depend on the working directory the runner happens to use.
    private static func contentViewPath(file: String = #filePath) -> String {
        var dir = URL(fileURLWithPath: file).deletingLastPathComponent()
        for _ in 0..<6 {
            let candidate = dir.appendingPathComponent(
                "VelocityVisualiser/UI/ContentView.swift")
            if FileManager.default.fileExists(atPath: candidate.path) { return candidate.path }
            dir = dir.deletingLastPathComponent()
        }
        return ""
    }

    /// The source control must not use `.disabled()`: it flips on every source
    /// change, which is exactly the pattern that produced the cycles.
    func testSourceControlDoesNotUseDisabled() throws {
        let source = try String(contentsOfFile: Self.contentViewPath(), encoding: .utf8)

        guard let toggle = source.range(of: "struct LiveToggleView") else {
            return XCTFail("LiveToggleView not found")
        }
        // Strip comment lines: this file explains in prose why it avoids
        // `.disabled()`, and a naive substring search matches the explanation.
        let body =
            source[toggle.lowerBound...]
            .prefix(2000)
            .split(separator: "\n", omittingEmptySubsequences: false)
            .filter { !$0.trimmingCharacters(in: .whitespaces).hasPrefix("//") }
            .joined(separator: "\n")

        XCTAssertFalse(
            body.contains(".disabled("),
            "LiveToggleView uses .disabled(); changing a cell's enabled state during an "
                + "update re-enters the view graph through AppKit's key-view loop")
        XCTAssertTrue(body.contains(".inert("), "LiveToggleView should use .inert instead")
    }

}
