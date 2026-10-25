// VelocityReportServer.swift
// Where the velocity.report server is, as the operator types it in
// Settings ▸ Servers and as the app keeps it in its preferences.
//
// The server answers on two ports of one host: its HTTP API (runs, labels,
// replays, `/api/version`), 8080 by default, and the gRPC frame stream the
// main window draws, 50051 by default (`--lidar-grpc-listen`). The operator
// gives the HTTP URL and, when it is not 50051, the stream's port:
//
//   http://localhost:8080          machine.tailnet.ts.net
//   https://lidar.example.org      http://[fd7a:115c:a1e0::1]:8080
//
// A URL without a scheme is http. Without a port, http means 8080 and https
// 443. A path is kept as a prefix for every API call, for a server behind a
// reverse proxy; a user name, query or fragment is refused.

import Foundation

struct VelocityReportServer: Equatable, Hashable, Sendable {
    static let defaultHTTPPort = 8080
    static let defaultHTTPSPort = 443
    static let defaultStreamPort = 50051
    /// The server on this Mac: what an unset preference means.
    static let local = VelocityReportServer(
        scheme: "http", host: "localhost", httpPort: defaultHTTPPort, path: "",
        streamPort: defaultStreamPort)

    /// UserDefaults keys, next to `alignment.serviceAddress`.
    static let urlKey = "velocityReport.serverURL"
    static let streamPortKey = "velocityReport.streamPort"
    /// The HTTP API's address before Settings had a field for it. Read when
    /// the new key is unset, and moved to it at launch (`migrate(in:)`).
    static let legacyURLKey = "annotation.serviceURL"

    /// "http" or "https".
    let scheme: String
    /// A host name in lower case, or an IP literal; IPv6 without brackets.
    let host: String
    let httpPort: Int
    /// "" or a prefix such as "/velocity", without a trailing slash.
    let path: String
    let streamPort: Int

    /// The host as it goes in a URL or a host:port: IPv6 in brackets.
    private var bracketedHost: String { host.contains(":") ? "[\(host)]" : host }

    /// "http://host:8080", always with its port, so what is shown is what is used.
    var urlDescription: String { "\(scheme)://\(bracketedHost):\(httpPort)\(path)" }
    /// The base of every HTTP API call.
    var httpURL: URL { URL(string: urlDescription) ?? URL(string: "http://localhost:8080")! }
    /// "host:50051", what the frame stream dials.
    var streamAddress: String { "\(bracketedHost):\(streamPort)" }

    enum ParseError: Error, LocalizedError, Equatable {
        case empty
        case unsupportedScheme(String)
        case notAURL(String)
        case invalidHost(String)
        case invalidPort(String)
        case invalidStreamPort(String)

        var errorDescription: String? {
            switch self {
            case .empty: return "Give the server's URL, such as http://localhost:8080."
            case .unsupportedScheme(let scheme):
                return "\(scheme):// is not supported: use http:// or https://."
            case .notAURL(let text):
                return "\(text) is not a server URL: give one such as "
                    + "http://machine.tailnet.ts.net:8080, with no user name, query or fragment."
            case .invalidHost(let host): return "\(host) is not a host name or IP address."
            case .invalidPort(let port): return "\(port) is not a port: use 1 to 65535."
            case .invalidStreamPort(let port):
                return "The stream port \(port) is not a port: use 1 to 65535."
            }
        }
    }

    /// Reads what an operator typed: the HTTP URL, and the stream's port
    /// (empty for 50051). Surrounding whitespace is ignored and a host name is
    /// kept in lower case.
    static func parse(
        url text: String, streamPort portText: String = ""
    ) throws -> VelocityReportServer {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { throw ParseError.empty }
        guard !trimmed.contains(where: \.isWhitespace) else { throw ParseError.notAURL(trimmed) }

        var withScheme = trimmed
        if let range = trimmed.range(of: "://") {
            let scheme = trimmed[..<range.lowerBound].lowercased()
            guard scheme == "http" || scheme == "https" else {
                throw ParseError.unsupportedScheme(String(trimmed[..<range.lowerBound]))
            }
        } else {
            withScheme = "http://" + trimmed
        }

        guard let components = URLComponents(string: withScheme),
            let scheme = components.scheme?.lowercased(), let rawHost = components.host,
            components.user == nil, components.password == nil, components.query == nil,
            components.fragment == nil
        else { throw invalidPortOrURL(withScheme, original: trimmed) }

        var hostText = rawHost
        if hostText.hasPrefix("["), hostText.hasSuffix("]") {
            hostText = String(hostText.dropFirst().dropLast())
        }
        guard !hostText.isEmpty else { throw ParseError.notAURL(trimmed) }
        let host: String
        if GerardusAddress.isIPv6(hostText) {
            host = hostText.lowercased()
        } else if let name = GerardusAddress.normalisedHost(hostText) {
            host = name
        } else {
            throw ParseError.invalidHost(hostText)
        }

        let httpPort: Int
        if let port = components.port {
            guard (1...65_535).contains(port) else { throw ParseError.invalidPort(String(port)) }
            httpPort = port
        } else {
            httpPort = scheme == "https" ? defaultHTTPSPort : defaultHTTPPort
        }

        var path = components.percentEncodedPath
        while path.hasSuffix("/") { path.removeLast() }

        let streamPort = try parseStreamPort(portText)
        return VelocityReportServer(
            scheme: scheme, host: host, httpPort: httpPort, path: path, streamPort: streamPort)
    }

    /// The stream's port: empty means 50051.
    static func parseStreamPort(_ text: String) throws -> Int {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return defaultStreamPort }
        guard trimmed.allSatisfy({ ("0"..."9").contains($0) }), let port = Int(trimmed),
            (1...65_535).contains(port)
        else { throw ParseError.invalidStreamPort(trimmed) }
        return port
    }

    /// URLComponents refuses a URL whose port is not a number; say so rather
    /// than calling the whole thing not a URL.
    private static func invalidPortOrURL(_ text: String, original: String) -> ParseError {
        guard let afterScheme = text.range(of: "://").map({ text[$0.upperBound...] }) else {
            return .notAURL(original)
        }
        let authority = afterScheme.prefix { $0 != "/" }
        guard !authority.contains("@"), !authority.contains("?"), !authority.contains("#"),
            let colon = authority.lastIndex(of: ":"), !authority.hasSuffix("]")
        else { return .notAURL(original) }
        let port = authority[authority.index(after: colon)...]
        return port.isEmpty ? .notAURL(original) : .invalidPort(String(port))
    }

    // MARK: - Preference

    /// The saved server, or this Mac's when nothing is saved. A URL that no
    /// longer parses gives this Mac's too, so an edit made outside the app
    /// cannot leave it pointing nowhere. Before the first migration the old
    /// `annotation.serviceURL` is read in place of the new key.
    static func saved(in defaults: UserDefaults?) -> VelocityReportServer {
        guard let defaults else { return local }
        let url = defaults.string(forKey: urlKey) ?? defaults.string(forKey: legacyURLKey)
        let port = (defaults.object(forKey: streamPortKey) as? Int).map(String.init) ?? ""
        if let url, let server = try? parse(url: url, streamPort: port) { return server }
        return (try? parse(url: local.urlDescription, streamPort: port)) ?? local
    }

    /// Keeps this server as the preference. This Mac's is the default, so it
    /// is kept by removing the keys rather than writing them. The old key is
    /// removed either way: the new one now says where the server is.
    func save(to defaults: UserDefaults?) {
        guard let defaults else { return }
        defaults.removeObject(forKey: Self.legacyURLKey)
        let url = Self(
            scheme: scheme, host: host, httpPort: httpPort, path: path,
            streamPort: Self.defaultStreamPort)
        if url == Self.local {
            defaults.removeObject(forKey: Self.urlKey)
        } else {
            defaults.set(urlDescription, forKey: Self.urlKey)
        }
        if streamPort == Self.defaultStreamPort {
            defaults.removeObject(forKey: Self.streamPortKey)
        } else {
            defaults.set(streamPort, forKey: Self.streamPortKey)
        }
    }

    enum Migration: Equatable {
        /// The new key is set, or neither is.
        case nothingToMigrate
        /// The old URL was moved to the new key (or, when it was this Mac's,
        /// simply removed).
        case migrated(VelocityReportServer)
        /// The old URL does not parse; it is left where it was.
        case unreadable(String)
    }

    /// Moves `annotation.serviceURL` to `velocityReport.serverURL`, once, at
    /// launch. A value the new parser refuses stays put, so nothing an
    /// operator typed is thrown away.
    @discardableResult static func migrate(in defaults: UserDefaults?) -> Migration {
        guard let defaults, defaults.object(forKey: urlKey) == nil,
            let legacy = defaults.string(forKey: legacyURLKey)
        else { return .nothingToMigrate }
        let port = (defaults.object(forKey: streamPortKey) as? Int).map(String.init) ?? ""
        guard let server = try? parse(url: legacy, streamPort: port) else {
            return .unreadable(legacy)
        }
        server.save(to: defaults)
        return .migrated(server)
    }

    /// The saved server's HTTP API, read at each call so a change in
    /// Settings reaches every client without a relaunch.
    static var currentHTTPURL: URL { saved(in: .standard).httpURL }
}
