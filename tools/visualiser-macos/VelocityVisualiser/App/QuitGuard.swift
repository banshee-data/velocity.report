// QuitGuard.swift
// Asks before quitting over unsaved annotation work.
//
// The annotation window holds its close button while anything is unsaved,
// but quitting closes every window at once and never asks a window. This
// asks the open annotation sessions instead.

import AppKit

/// The annotation sessions that are open, held weakly, so quitting can ask
/// whether any has unsaved work.
@MainActor final class UnsavedWorkRegistry {
    static let shared = UnsavedWorkRegistry()

    private struct Entry { weak var session: AnnotationSession? }
    private var entries: [Entry] = []

    func register(_ session: AnnotationSession) {
        entries.removeAll { $0.session == nil || $0.session === session }
        entries.append(Entry(session: session))
    }

    /// What unsaved work there is, in words, or nil when there is none.
    var unsavedSummary: String? {
        entries.removeAll { $0.session == nil }
        let blocked = entries.compactMap { $0.session }.compactMap { session -> String? in
            switch session.navigationGuard() {
            case nil: return nil
            case .unsavedPhysical:
                return "the physical-reference draft of \(session.pack.directory.lastPathComponent)"
            case .unsavedMembership:
                return "a frame's membership in \(session.pack.directory.lastPathComponent)"
            case .strokeInProgress, .propagating:
                return "an edit still in progress in \(session.pack.directory.lastPathComponent)"
            }
        }
        return blocked.isEmpty ? nil : blocked.joined(separator: "; ")
    }
}

final class QuitGuard: NSObject, NSApplicationDelegate {
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        MainActor.assumeIsolated {
            guard let summary = UnsavedWorkRegistry.shared.unsavedSummary else {
                return .terminateNow
            }
            let alert = NSAlert()
            alert.messageText = "Quit with unsaved annotation work?"
            alert.informativeText = "Unsaved: \(summary). Quitting discards it."
            alert.addButton(withTitle: "Keep Working")
            alert.addButton(withTitle: "Quit Anyway")
            alert.alertStyle = .warning
            return alert.runModal() == .alertSecondButtonReturn ? .terminateNow : .terminateCancel
        }
    }
}
