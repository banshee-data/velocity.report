// GerardusAddress.swift
// Where the Gerardus alignment server listens, as the operator types it in
// Settings and as the app keeps it in its preferences.
//
// A host name or an IP literal, with an optional port:
//
//   127.0.0.1                     machine.tailnet.ts.net
//   100.64.0.1:50061              [fd7a:115c:a1e0::1]:50061
//
// Over Tailscale the server is reached through its MagicDNS name or its
// 100.x address; `gerardus serve` has to listen on more than loopback for
// that to work.

import Darwin
import Foundation

struct GerardusAddress: Equatable, Hashable, Sendable, CustomStringConvertible {
    static let defaultPort = 50061
    /// The server on this Mac. An unset preference means this.
    static let local = GerardusAddress(host: "127.0.0.1", port: defaultPort)
    /// UserDefaults key, next to `velocityReport.serverURL`.
    static let preferenceKey = "alignment.serviceAddress"

    /// A host name in lower case, or an IP literal; IPv6 without brackets.
    let host: String
    let port: Int

    /// "host:port", with an IPv6 literal in brackets.
    var description: String { host.contains(":") ? "[\(host)]:\(port)" : "\(host):\(port)" }

    enum ParseError: Error, LocalizedError, Equatable {
        case empty
        case notAnAddress(String)
        case invalidHost(String)
        case invalidPort(String)

        var errorDescription: String? {
            switch self {
            case .empty: return "Give a host name or IP address."
            case .notAnAddress(let text):
                return "\(text) is not a host and port: give a name such as "
                    + "machine.tailnet.ts.net or an address such as 100.64.0.1:50061."
            case .invalidHost(let host): return "\(host) is not a host name or IP address."
            case .invalidPort(let port): return "\(port) is not a port: use 1 to 65535."
            }
        }
    }

    /// Reads what an operator typed. Surrounding whitespace is ignored, a
    /// missing port is 50061, an IPv6 literal may come with or without
    /// brackets (brackets are needed to give it a port), and a host name is
    /// kept in lower case. URLs, paths and anything else are refused.
    static func parse(_ text: String) throws -> GerardusAddress {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { throw ParseError.empty }
        guard !trimmed.contains(where: \.isWhitespace), !trimmed.contains("/"),
            !trimmed.contains("@")
        else { throw ParseError.notAnAddress(trimmed) }

        var hostPart = trimmed
        var portPart: String?
        if trimmed.hasPrefix("[") {
            // [IPv6] or [IPv6]:port
            guard let close = trimmed.firstIndex(of: "]") else {
                throw ParseError.notAnAddress(trimmed)
            }
            hostPart = String(trimmed[trimmed.index(after: trimmed.startIndex)..<close])
            let rest = trimmed[trimmed.index(after: close)...]
            if !rest.isEmpty {
                guard rest.hasPrefix(":") else { throw ParseError.notAnAddress(trimmed) }
                portPart = String(rest.dropFirst())
            }
            guard isIPv6(hostPart) else { throw ParseError.invalidHost(hostPart) }
        } else if trimmed.filter({ $0 == ":" }).count > 1 {
            // A bare IPv6 literal, which cannot carry a port.
            guard isIPv6(trimmed) else { throw ParseError.invalidHost(trimmed) }
        } else if let colon = trimmed.firstIndex(of: ":") {
            hostPart = String(trimmed[..<colon])
            portPart = String(trimmed[trimmed.index(after: colon)...])
        }

        let port = try portPart.map(parsePort) ?? defaultPort
        if isIPv6(hostPart) { return GerardusAddress(host: hostPart.lowercased(), port: port) }
        guard let host = normalisedHost(hostPart) else { throw ParseError.invalidHost(hostPart) }
        return GerardusAddress(host: host, port: port)
    }

    private static func parsePort(_ text: String) throws -> Int {
        guard !text.isEmpty, text.allSatisfy(\.isASCIIDigit), let port = Int(text),
            (1...65_535).contains(port)
        else { throw ParseError.invalidPort(text.isEmpty ? "An empty port" : text) }
        return port
    }

    /// An IPv4 literal as it stands, or a host name (RFC 1123 labels) in
    /// lower case without a trailing dot. Dotted numbers that are not a
    /// valid IPv4 address are refused rather than looked up as a name.
    static func normalisedHost(_ text: String) -> String? {
        if isIPv4(text) { return text }
        var name = text.lowercased()
        if name.hasSuffix(".") { name.removeLast() }
        guard !name.isEmpty, name.count <= 253 else { return nil }
        let labels = name.split(separator: ".", omittingEmptySubsequences: false)
        let validLabels = labels.allSatisfy { label in
            !label.isEmpty && label.count <= 63 && label.first != "-" && label.last != "-"
                && label.allSatisfy { $0.isASCIIDigit || ("a"..."z").contains($0) || $0 == "-" }
        }
        guard validLabels else { return nil }
        if labels.allSatisfy({ $0.allSatisfy(\.isASCIIDigit) }) { return nil }
        return name
    }

    static func isIPv4(_ text: String) -> Bool {
        var address = in_addr()
        return text.withCString { inet_pton(AF_INET, $0, &address) } == 1
    }

    static func isIPv6(_ text: String) -> Bool {
        var address = in6_addr()
        return text.withCString { inet_pton(AF_INET6, $0, &address) } == 1
    }

    // MARK: - Preference

    /// The saved address, or the local server when nothing is saved. A saved
    /// value that no longer parses also gives the local server, so a bad
    /// edit made outside the app cannot leave it pointing nowhere.
    static func saved(in defaults: UserDefaults?) -> GerardusAddress {
        guard let text = defaults?.string(forKey: preferenceKey), let address = try? parse(text)
        else { return local }
        return address
    }

    /// Keeps this address as the preference. The local server is the
    /// default, so it is kept by removing the key rather than writing it.
    func save(to defaults: UserDefaults?) {
        if self == Self.local {
            defaults?.removeObject(forKey: Self.preferenceKey)
        } else {
            defaults?.set(description, forKey: Self.preferenceKey)
        }
    }
}

extension Character { fileprivate var isASCIIDigit: Bool { ("0"..."9").contains(self) } }
