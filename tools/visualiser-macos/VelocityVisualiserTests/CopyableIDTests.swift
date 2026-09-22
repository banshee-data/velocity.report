//
//  CopyableIDTests.swift
//  VelocityVisualiserTests
//
//  What matters about a copyable ID is that the copy yields the identifier
//  and nothing else. The run browser shows an 8-character prefix and the
//  track inspector prefixes its value with a field name, so in both places
//  the string on screen is not the string a tool will accept — which is the
//  bug this view exists to prevent, and the one these tests pin.
//

import AppKit
import Foundation
import Testing

@testable import VelocityVisualiser

struct CopyableIDTests {
    private static let runID = "8e422582-1012-48b1-85ef-3fdb85ffa41b"

    // MARK: Display composition

    @Test func labelPrefixesTheValue() {
        #expect(
            copyableIDDisplayText(value: Self.runID, display: nil, label: "Run")
                == "Run: \(Self.runID)")
    }

    @Test func anAbbreviationReplacesTheValueOnScreen() {
        // The run browser's cell is 80 points wide; a full UUID cannot fit.
        #expect(
            copyableIDDisplayText(value: Self.runID, display: "8e422582", label: "Run")
                == "Run: 8e422582")
    }

    @Test func withoutALabelOnlyTheIdentifierIsShown() {
        #expect(copyableIDDisplayText(value: Self.runID, display: nil, label: nil) == Self.runID)
        // An empty label must not leave a bare colon on screen.
        #expect(copyableIDDisplayText(value: Self.runID, display: nil, label: "") == Self.runID)
    }

    // MARK: Pasteboard

    /// A named pasteboard, so the test never disturbs the developer's own
    /// clipboard and can run beside other tests.
    private func scratchPasteboard(_ name: String) -> NSPasteboard {
        let board = NSPasteboard(name: NSPasteboard.Name("velocity.test.\(name)"))
        board.clearContents()
        return board
    }

    @Test func copyPutsTheValueOnThePasteboard() {
        let board = scratchPasteboard("copy")
        #expect(copyIDToPasteboard(Self.runID, pasteboard: board))
        #expect(board.string(forType: .string) == Self.runID)
    }

    @Test func copyReplacesRatherThanAppends() {
        // Without clearContents() the previous value can still be read back
        // by another app, which for an ID means pasting the wrong run.
        let board = scratchPasteboard("replace")
        copyIDToPasteboard("previous-run", pasteboard: board)
        copyIDToPasteboard(Self.runID, pasteboard: board)
        #expect(board.string(forType: .string) == Self.runID)
    }

    @Test func theCopiedValueIsNeverTheAbbreviatedOrLabelledText() {
        // The regression that matters: a copy must not carry "Run: " into a
        // shell argument, nor stop at the 8-character prefix.
        let board = scratchPasteboard("full")
        let shown = copyableIDDisplayText(value: Self.runID, display: "8e422582", label: "Run")
        copyIDToPasteboard(Self.runID, pasteboard: board)

        let copied = board.string(forType: .string)
        #expect(copied == Self.runID)
        #expect(copied != shown)
        #expect(!(copied ?? "").contains("Run"))
    }

    // MARK: Wiring

    /// The view is only useful where it is actually used, and a revert to a
    /// plain `Text` would leave every other test passing. Checking the source
    /// is the same approach `InertModifierTests` takes for the same reason.
    @Test func theTrackInspectorAndRunBrowserUseIt() throws {
        for (name, expected) in [
            ("ContentView.swift", ["CopyableID(value: trackID", "CopyableID(value: runID"]),
            // Matched loosely for the row's cell: the formatter decides where
            // that call wraps, and a test should not fail on a line break.
            (
                "RunBrowserView.swift",
                ["CopyableID(value: selectedRunID", "value: run.runId", "selectable: false"]
            ),
        ] {
            let path = Self.uiFilePath(named: name)
            #expect(!path.isEmpty, "could not locate \(name)")
            let source = try String(contentsOfFile: path, encoding: .utf8)
            for fragment in expected {
                #expect(source.contains(fragment), "\(name) no longer uses \(fragment)")
            }
            #expect(
                !source.contains("Text(\"ID: \\(trackID)\")"),
                "\(name) reverted to a plain, uncopyable Text")
        }
    }

    private static func uiFilePath(named name: String, file: String = #filePath) -> String {
        var dir = URL(fileURLWithPath: file).deletingLastPathComponent()
        for _ in 0..<6 {
            let candidate = dir.appendingPathComponent("VelocityVisualiser/UI/\(name)")
            if FileManager.default.fileExists(atPath: candidate.path) { return candidate.path }
            dir = dir.deletingLastPathComponent()
        }
        return ""
    }
}
