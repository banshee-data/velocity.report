// CopyableID.swift
// Identifiers the operator can select and copy.
//
// Track and run IDs are UUIDs whose whole purpose is to be pasted somewhere
// else — into a `lidar-e1-analysis` argument, a vrlog path, an issue thread.
// Until now they could only be read off the screen and retyped, which for a
// 36-character UUID is a transcription error waiting to happen.
//
// Two affordances rather than one, because they fail in different places:
// drag-selection copies exactly what is on screen, which is wrong wherever the
// display is abbreviated, and a context-menu copy always yields the full
// value. The run browser's rows show only an 8-character prefix, so there the
// menu is the only correct route.

import AppKit
import SwiftUI

/// Replaces the general pasteboard's contents with `value`.
///
/// A free function rather than a method so the pasteboard behaviour can be
/// tested without mounting a view. `clearContents()` first because
/// `setString` alone leaves previously declared types on the board, and a
/// stale one can win when another app reads it.
@discardableResult func copyIDToPasteboard(
    _ value: String, pasteboard: NSPasteboard = .general
) -> Bool {
    pasteboard.clearContents()
    return pasteboard.setString(value, forType: .string)
}

/// The text a `CopyableID` shows, given a label and an optional abbreviation.
///
/// Extracted so the label/display composition is testable without mounting a
/// view: showing the wrong string here is silent, and the whole point of the
/// view is that what is copied and what is shown may legitimately differ.
func copyableIDDisplayText(value: String, display: String?, label: String?) -> String {
    let body = display ?? value
    guard let label, !label.isEmpty else { return body }
    return "\(label): \(body)"
}

/// A monospaced identifier, selectable, with a right-click copy.
///
/// `display` is what the operator reads and `value` is what a copy yields;
/// they differ wherever the display is abbreviated. `label` prefixes the
/// display only — it is never copied, since "Run: " in a shell argument is
/// precisely the sort of thing this view exists to prevent.
struct CopyableID: View {
    let value: String
    /// Abbreviated text to show instead of `value`; nil shows `value` itself.
    var display: String?
    /// Field name shown before the ID, without its colon. Not copied.
    var label: String?
    /// Drag-selection is off where the ID sits inside a click target: an
    /// enabled text selection swallows the click, and in the run browser that
    /// click is how a run gets loaded. The context menu still copies.
    var selectable: Bool = true

    private var shown: String {
        copyableIDDisplayText(value: value, display: display, label: label)
    }

    private var copyTitle: String { "Copy \(label ?? "ID")" }

    var body: some View {
        // Two branches rather than a ternary: the enabled and disabled
        // selectability types are distinct, so they cannot share one call.
        Group {
            if selectable {
                Text(shown).textSelection(.enabled)
            } else {
                Text(shown).textSelection(.disabled)
            }
        }.font(.system(.caption, design: .monospaced)).contextMenu {
            Button(copyTitle) { copyIDToPasteboard(value) }
        }.help("Right-click to copy \(value)")
    }
}
