// AnnotationTree.swift
// The annotation tree, as Go's annotation/layout.go lays it out. One layout,
// locally under the server's --lidar-annotation-dir and in the dataset under
// labels/:
//
//   <root>/<capture>/<set>/              a label set
//   <root>/<capture>/<set>/views/<view>/ a pack linked to it (link ../..)
//   <root>/<capture>/packs/<run>/        a pack not in a set yet
//   <root>/clip-<job>-<n>/pack/          a Segments clip job's pack
//
// A pack's labels go in its capture folder: the first save from a pack in
// packs/ makes the set beside packs/ and links the pack where it is.

import Foundation

enum AnnotationTree {
    static let packsDirectory = "packs"
    static let viewsDirectory = "views"
    /// The link of a view nested in its label set.
    static let viewLinkPath = "../.."

    private static let reserved: Set<String> = ["splits", "labels", packsDirectory, viewsDirectory]

    /// The capture folder name for a capture file, as Go's CaptureNameOf:
    /// its base name without the extension, with anything but ASCII letters,
    /// digits, dots, dashes and underscores replaced by a dash.
    static func captureName(of captureFile: String?) -> String {
        let path = (captureFile ?? "").replacingOccurrences(of: "\\", with: "/")
        var base =
            path.split(separator: "/", omittingEmptySubsequences: true).last.map(String.init) ?? ""
        for ext in [".gz", ".pcapng", ".pcap"] where base.hasSuffix(ext) {
            base = String(base.dropLast(ext.count))
        }
        let safe = String(
            base.unicodeScalars.map { s -> Character in
                let c = Character(s)
                return s.isASCII && (c.isLetter || c.isNumber || c == "." || c == "-" || c == "_")
                    ? c : "-"
            })
        let name = safe.trimmingCharacters(in: CharacterSet(charactersIn: "."))
        return name.isEmpty || reserved.contains(name) ? "capture" : name
    }

    /// Whether a directory is the pack in a clip job's folder.
    static func isClipPack(_ directory: URL) -> Bool {
        let dir = directory.standardizedFileURL
        return dir.lastPathComponent == "pack"
            && dir.deletingLastPathComponent().lastPathComponent.hasPrefix("clip-")
    }

    /// The capture folder a pack's labels belong in, as Go's CaptureDirOf. A
    /// pack outside the layout gets one beside it, named for its capture.
    static func captureDirectory(of packDirectory: URL, pcapBasename: String?) -> URL {
        let dir = packDirectory.standardizedFileURL
        let parent = dir.deletingLastPathComponent()
        switch parent.lastPathComponent {
        case viewsDirectory: return parent.deletingLastPathComponent().deletingLastPathComponent()
        case packsDirectory: return parent.deletingLastPathComponent()
        default:
            let base = isClipPack(dir) ? parent.deletingLastPathComponent() : parent
            return base.appendingPathComponent(captureName(of: pcapBasename), isDirectory: true)
        }
    }

    /// The handle the server knows a pack by: its path relative to the
    /// server's root, which is read from where the pack sits in the layout.
    /// A pack outside the layout gets its folder name, which the server
    /// refuses with the shapes it accepts.
    static func handle(for packDirectory: URL) -> String {
        let parts = packDirectory.standardizedFileURL.pathComponents.filter { $0 != "/" }
        guard let last = parts.last else { return "" }
        let parent = parts.count >= 2 ? parts[parts.count - 2] : ""
        let count: Int
        if parent == viewsDirectory {
            count = 4
        } else if parent == packsDirectory {
            count = 3
        } else if last == "pack" && parent.hasPrefix("clip-") {
            count = 2
        } else {
            count = 1
        }
        return parts.suffix(min(count, parts.count)).joined(separator: "/")
    }

    /// The relative path from one directory to another, with forward slashes,
    /// as Go's filepath.Rel gives it for a link.
    static func relativePath(from: URL, to: URL) -> String {
        let a = from.standardizedFileURL.pathComponents
        let b = to.standardizedFileURL.pathComponents
        var common = 0
        while common < a.count, common < b.count, a[common] == b[common] { common += 1 }
        let up = Array(repeating: "..", count: a.count - common)
        let down = Array(b[common...])
        let path = (up + down).joined(separator: "/")
        return path.isEmpty ? "." : path
    }
}
