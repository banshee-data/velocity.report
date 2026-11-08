// VisualiserURL.swift
// The velocityreport:// URL scheme: what a link names, and the inbox the
// app delegate hands opened links to.
//
// The web Tracks page's "Open in visualiser" links to a run at a frame:
//
//     velocityreport://run/<run_id>?frame=<n>
//
// `frame` is the run's VRLOG frame index from 0: the index each streamed
// frame carries (playback_info.current_frame_index), which is the frame on
// screen. The HTTP playback status's current_frame is the reader's cursor,
// one past the frame on screen while paused, so it is not the number to
// link. Without `frame` the run opens at its start. The pack form, velocityreport://pack/<pack_id>, is reserved
// for the segment finder's "Open in macOS" and is refused for now.

import Foundation

/// What a velocityreport:// link asks the app to open.
enum VisualiserURL: Equatable {
    /// A run's recording, shown at a frame index (from 0), or at its start.
    case run(runID: String, frame: UInt64?)

    /// The scheme the app registers in its Info.plist.
    static let scheme = "velocityreport"

    /// Why a link was refused, in words an operator can act on.
    enum ParseError: Error, Equatable {
        case wrongScheme(String)
        case notSupportedYet(String)
        case unknownTarget(String)
        case missingRunID
        case badRunID(String)
        case badFrame(String)

        var message: String {
            switch self {
            case .wrongScheme(let scheme):
                return "The link uses the \(scheme): scheme, not \(VisualiserURL.scheme):."
            case .notSupportedYet(let target):
                return
                    "Opening a \(target) from a link is not supported yet. Links can open a run: velocityreport://run/<run_id>?frame=<n>."
            case .unknownTarget(let target):
                return
                    "The link names \"\(target)\", which the visualiser cannot open. Links can open a run: velocityreport://run/<run_id>?frame=<n>."
            case .missingRunID:
                return "The link names no run. Expected velocityreport://run/<run_id>?frame=<n>."
            case .badRunID(let id):
                return
                    "\"\(id)\" is not a run ID. Run IDs are letters, digits, '-', '_' and '.', at most \(VisualiserURL.maxRunIDLength) characters."
            case .badFrame(let frame):
                return
                    "\"\(frame)\" is not a frame. The frame is a whole number from 0, the frame index in the run's recording."
            }
        }
    }

    /// The longest run ID a link may carry. Server IDs are UUIDs (36) or a
    /// clip replay's "replay-<clip>-<8 hex>"; this leaves room for long clips.
    static let maxRunIDLength = 128

    /// Parse a link. Pure: it reads nothing but the URL.
    static func parse(_ url: URL) -> Result<VisualiserURL, ParseError> {
        guard let components = URLComponents(url: url, resolvingAgainstBaseURL: false) else {
            return .failure(.unknownTarget(url.absoluteString))
        }
        let scheme = (components.scheme ?? "").lowercased()
        guard scheme == Self.scheme else { return .failure(.wrongScheme(scheme)) }

        // velocityreport://run/<id>: "run" is the host and the ID the path.
        let target = (components.host ?? "").lowercased()
        let path = components.path.split(separator: "/", omittingEmptySubsequences: true)
        switch target {
        case "run": break
        case "pack": return .failure(.notSupportedYet("pack"))
        case "": return .failure(.unknownTarget(url.absoluteString))
        default: return .failure(.unknownTarget(target))
        }

        guard let first = path.first else { return .failure(.missingRunID) }
        let runID = String(first)
        guard path.count == 1, isRunID(runID) else {
            return .failure(.badRunID(path.joined(separator: "/")))
        }

        let frameItems = (components.queryItems ?? []).filter { $0.name == "frame" }
        guard let frameItem = frameItems.first else {
            return .success(.run(runID: runID, frame: nil))
        }
        let value = frameItem.value ?? ""
        // Digits only: UInt64("+3") parses, and a link should say what it means.
        guard frameItems.count == 1, !value.isEmpty, value.allSatisfy(\.isASCIIDigit),
            let frame = UInt64(value)
        else { return .failure(.badFrame(frameItems.compactMap(\.value).joined(separator: ","))) }
        return .success(.run(runID: runID, frame: frame))
    }

    /// Whether a string can be a run ID. The ID goes into the server's
    /// /api/lidar/runs/<id> path, so nothing that could leave that path.
    static func isRunID(_ id: String) -> Bool {
        guard !id.isEmpty, id.count <= maxRunIDLength, id != ".", id != ".." else { return false }
        return id.unicodeScalars.allSatisfy { scalar in
            scalar.isASCII
                && (CharacterSet.alphanumerics.contains(scalar)
                    || "-_.".unicodeScalars.contains(scalar))
        }
    }

    /// The link for a run at a frame, the inverse of `parse`.
    static func link(runID: String, frame: UInt64?) -> URL? {
        guard isRunID(runID) else { return nil }
        var components = URLComponents()
        components.scheme = scheme
        components.host = "run"
        components.path = "/\(runID)"
        if let frame { components.queryItems = [URLQueryItem(name: "frame", value: "\(frame)")] }
        return components.url
    }
}

extension Character { fileprivate var isASCIIDigit: Bool { isASCII && isNumber } }

/// Where opened links wait for the app state that acts on them. The app
/// delegate can be handed a link before any window exists, when the app is
/// launched by the link itself, so links are held until a handler is set.
@MainActor final class VisualiserURLInbox {
    static let shared = VisualiserURLInbox()

    private var pending: [URL] = []
    private var handler: ((URL) -> Void)?

    /// Set the handler and hand it every link that arrived before it.
    func setHandler(_ handler: @escaping (URL) -> Void) {
        self.handler = handler
        let waiting = pending
        pending = []
        waiting.forEach(handler)
    }

    func receive(_ urls: [URL]) {
        guard let handler else {
            pending.append(contentsOf: urls)
            return
        }
        urls.forEach(handler)
    }
}
