// CityLocationAccess.swift
// How the city view reads the folders it is pointed at from inside the app
// sandbox. A folder or file the user chose in an open panel is kept as a
// security-scoped bookmark beside its plain path, resolved at launch and
// held open while it is the location in use, so every read beneath it (the
// tileset, each tile in the cache's loader, the index, each scene's parts)
// is allowed. A typed path works only where the sandbox already allows it;
// elsewhere the app says to choose it instead of showing Cocoa's error.

import Foundation

/// The folders the sandbox lets the app read because the user chose them.
/// Every root registered here has had `startAccessingSecurityScopedResource`
/// called on it, and keeps it until it is replaced.
final class SecurityScopedLocations: @unchecked Sendable {
    static let shared = SecurityScopedLocations()

    private let lock = NSLock()
    private var roots: [String: URL] = [:]

    /// Holds `url` open under `key`, closing whatever the key held before.
    func hold(_ url: URL?, for key: String) {
        let started = url.map { ($0, $0.startAccessingSecurityScopedResource()) }
        lock.lock()
        let previous = roots[key]
        if let (url, ok) = started, ok { roots[key] = url } else { roots[key] = nil }
        lock.unlock()
        previous?.stopAccessingSecurityScopedResource()
    }

    /// The chosen root that holds `url`, if any.
    func root(containing url: URL) -> URL? {
        guard url.isFileURL else { return nil }
        let path = url.standardizedFileURL.path
        lock.lock()
        defer { lock.unlock() }
        return roots.values.first { root in
            let r = root.standardizedFileURL.path
            return path == r || path.hasPrefix(r.hasSuffix("/") ? r : r + "/")
        }
    }

    /// Runs `body` with the root holding `url` started for it, so a read
    /// made while the location is being replaced still has its grant.
    func reading<T>(_ url: URL, _ body: () throws -> T) rethrows -> T {
        guard let root = root(containing: url) else { return try body() }
        let ok = root.startAccessingSecurityScopedResource()
        defer { if ok { root.stopAccessingSecurityScopedResource() } }
        return try body()
    }
}

enum CityLocationAccess {
    /// The defaults key a location's bookmark is kept under.
    static func bookmarkKey(_ key: String) -> String { key + ".bookmark" }

    /// The user's own home folder. In the sandbox `NSHomeDirectory()` and
    /// `~` are the app's container; a path typed with `~` means the user's.
    static var realHome: String {
        if let entry = getpwuid(getuid()), let dir = entry.pointee.pw_dir {
            return String(cString: dir)
        }
        return NSHomeDirectory()
    }

    /// `~` and `~/…` against the real home; other paths as typed.
    static func expandTilde(_ path: String, home: String = realHome) -> String {
        guard path == "~" || path.hasPrefix("~/") else { return path }
        return home + path.dropFirst()
    }

    /// A bookmark for a chosen folder or file, read-only.
    static func bookmark(for url: URL) throws -> Data {
        try url.bookmarkData(
            options: [.withSecurityScope, .securityScopeAllowOnlyReadAccess],
            includingResourceValuesForKeys: nil, relativeTo: nil)
    }

    /// The location a bookmark names, and its data again when it went stale.
    static func resolve(_ data: Data) -> (url: URL, refreshed: Data?)? {
        var stale = false
        guard
            let url = try? URL(
                resolvingBookmarkData: data, options: [.withSecurityScope], relativeTo: nil,
                bookmarkDataIsStale: &stale)
        else { return nil }
        return (url, stale ? try? bookmark(for: url) : nil)
    }

    /// Whether an error is the sandbox (or the file's permissions) refusing
    /// a read, rather than the file not being there.
    static func isPermissionDenied(_ error: Error) -> Bool {
        let ns = error as NSError
        if ns.domain == NSCocoaErrorDomain
            && [NSFileReadNoPermissionError, NSFileWriteNoPermissionError].contains(ns.code)
        {
            return true
        }
        if ns.domain == NSPOSIXErrorDomain && [Int(EPERM), Int(EACCES)].contains(ns.code) {
            return true
        }
        if let underlying = ns.userInfo[NSUnderlyingErrorKey] as? Error {
            return isPermissionDenied(underlying)
        }
        return false
    }

    enum AccessError: Error, LocalizedError, Equatable {
        case notChosen(String)

        var errorDescription: String? {
            switch self {
            case .notChosen(let path):
                return "The app may not read \(path). Choose the folder with Choose… so the app "
                    + "may read it."
            }
        }
    }

    /// Checks that a local location may be read: a typed path outside the
    /// sandbox, never chosen, fails here with what to do about it. A path
    /// that does not exist passes, so the reader says what is missing.
    static func checkReadable(_ url: URL, fileManager: FileManager = .default) throws {
        guard url.isFileURL else { return }
        try SecurityScopedLocations.shared.reading(url) {
            var isDirectory: ObjCBool = false
            // In the sandbox an unreadable path can look absent; listing its
            // parent tells "not allowed" from "not there".
            let probe =
                fileManager.fileExists(atPath: url.path, isDirectory: &isDirectory)
                ? url : url.deletingLastPathComponent()
            do {
                if probe == url && !isDirectory.boolValue {
                    _ = try FileHandle(forReadingFrom: url).close()
                } else {
                    _ = try fileManager.contentsOfDirectory(atPath: probe.path)
                }
            } catch  where isPermissionDenied(error) { throw AccessError.notChosen(url.path) } catch
            {}
        }
    }

    /// Error text for a failed read: the sandbox's refusal as what to do.
    static func describe(_ error: Error, at url: URL) -> String {
        if isPermissionDenied(error) { return AccessError.notChosen(url.path).localizedDescription }
        return error.localizedDescription
    }
}

/// One location setting, a path or URL typed or chosen, with its bookmark.
struct CityLocationSetting {
    let key: String
    let defaults: UserDefaults?

    /// What the setting says: the chosen folder's path or the text typed.
    var text: String { defaults?.string(forKey: key) ?? "" }

    /// Saves typed text. Text that is not the chosen folder's path drops the
    /// bookmark, so the location read is always the one shown.
    func setTyped(_ text: String) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let chosenPath = resolvedBookmark()?.path
        if trimmed.isEmpty {
            defaults?.removeObject(forKey: key)
        } else {
            defaults?.set(trimmed, forKey: key)
        }
        if chosenPath != trimmed {
            defaults?.removeObject(forKey: CityLocationAccess.bookmarkKey(key))
        }
    }

    /// Saves a folder or file chosen in the open panel, with its bookmark.
    func setChosen(_ url: URL) throws {
        let data = try CityLocationAccess.bookmark(for: url)
        defaults?.set(url.path, forKey: key)
        defaults?.set(data, forKey: CityLocationAccess.bookmarkKey(key))
    }

    /// The chosen location, refreshing a stale bookmark.
    func resolvedBookmark() -> URL? {
        guard let data = defaults?.data(forKey: CityLocationAccess.bookmarkKey(key)),
            let (url, refreshed) = CityLocationAccess.resolve(data)
        else { return nil }
        if let refreshed { defaults?.set(refreshed, forKey: CityLocationAccess.bookmarkKey(key)) }
        return url
    }

    /// The URL to read: the chosen folder, held open for reading, or the
    /// typed text as a URL or path.
    func open(typed: String) -> URL? {
        let chosen = resolvedBookmark()
        SecurityScopedLocations.shared.hold(chosen, for: key)
        if let chosen { return chosen }
        return CityLocationSetting.url(for: typed)
    }

    /// A typed path or URL as a location: `~` is the user's real home, a
    /// bare path a file URL.
    static func url(for text: String) -> URL? {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        if let url = URL(string: trimmed), let scheme = url.scheme?.lowercased(),
            ["http", "https", "file"].contains(scheme)
        {
            return url
        }
        return URL(fileURLWithPath: CityLocationAccess.expandTilde(trimmed))
    }
}
