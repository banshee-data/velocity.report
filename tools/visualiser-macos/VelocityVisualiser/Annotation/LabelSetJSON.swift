// LabelSetJSON.swift
// The canonical JSON a label set's head files and journal lines are written
// in, and a strict reader for it.
//
// The rules are internal/lidar/annotation/labelset/README.md's, and the Go
// writer is the reference: object keys sorted by code point, no whitespace,
// numbers as ECMAScript's Number.prototype.toString writes them (which is what
// Go's encoding/json produces), and strings escaping only the quote, the
// backslash and control characters. One state has one encoding, so the state
// digest over it can be recomputed by either implementation. JSONEncoder is
// not used: it writes 1.0 as "1.0", escapes "/" and orders keys by its own
// rules, any one of which would give a different digest for the same labels.
//
// Reading is strict for the same reason Go's is: a field this build does not
// know would be dropped on the next save, which is losing someone's data
// without saying so. An unknown field is refused instead.

import Foundation

/// A parsed JSON value. Numbers keep their literal text, so an integer above
/// 2^53 is never rounded on the way through.
indirect enum LabelSetJSON: Equatable {
    case null
    case bool(Bool)
    case number(String)
    case string(String)
    case array([LabelSetJSON])
    case object([String: LabelSetJSON])
    /// Bytes already in canonical form, written as they are. Only writers
    /// make one, for the sets of ranges that are most of a label set's bytes;
    /// the parser never does.
    case canonicalBytes([UInt8])
}

/// Why a label-set file could not be read or written.
struct LabelSetCodingError: Error, CustomStringConvertible, Equatable {
    var description: String
    init(_ description: String) { self.description = description }
}

// MARK: - Canonical writer

extension LabelSetJSON {
    /// The canonical bytes of this value.
    var canonicalData: Data {
        var out = [UInt8]()
        out.reserveCapacity(256)
        LabelSetJSON.write(self, into: &out)
        return Data(out)
    }

    /// The canonical bytes followed by a newline: one head file or one line.
    var canonicalLine: Data {
        var out = [UInt8]()
        LabelSetJSON.write(self, into: &out)
        out.append(0x0A)
        return Data(out)
    }

    private static func write(_ value: LabelSetJSON, into out: inout [UInt8]) {
        switch value {
        case .null: out.append(contentsOf: "null".utf8)
        case .bool(let b): out.append(contentsOf: (b ? "true" : "false").utf8)
        case .number(let text): out.append(contentsOf: text.utf8)
        case .canonicalBytes(let bytes): out.append(contentsOf: bytes)
        case .string(let s): writeString(s, into: &out)
        case .array(let items):
            out.append(0x5B)
            for (i, item) in items.enumerated() {
                if i > 0 { out.append(0x2C) }
                write(item, into: &out)
            }
            out.append(0x5D)
        case .object(let fields):
            out.append(0x7B)
            // Go's sort.Strings orders by bytes, which for UTF-8 is code
            // point order. Swift's String comparison is by canonical
            // equivalence, so the bytes are compared explicitly.
            let keys = fields.keys.sorted { $0.utf8.lexicographicallyPrecedes($1.utf8) }
            for (i, key) in keys.enumerated() {
                if i > 0 { out.append(0x2C) }
                writeString(key, into: &out)
                out.append(0x3A)
                write(fields[key]!, into: &out)
            }
            out.append(0x7D)
        }
    }

    /// Escapes the quote, the backslash and control characters only;
    /// everything else, "/", "<", ">", "&" and non-ASCII included, is UTF-8.
    private static func writeString(_ s: String, into out: inout [UInt8]) {
        out.append(0x22)
        for scalar in s.unicodeScalars {
            switch scalar.value {
            case 0x22: out.append(contentsOf: [0x5C, 0x22])
            case 0x5C: out.append(contentsOf: [0x5C, 0x5C])
            case 0x08: out.append(contentsOf: [0x5C, 0x62])
            case 0x0C: out.append(contentsOf: [0x5C, 0x66])
            case 0x0A: out.append(contentsOf: [0x5C, 0x6E])
            case 0x0D: out.append(contentsOf: [0x5C, 0x72])
            case 0x09: out.append(contentsOf: [0x5C, 0x74])
            case 0..<0x20:
                out.append(contentsOf: Array(String(format: "\\u%04x", scalar.value).utf8))
            case 0..<0x80: out.append(UInt8(scalar.value))
            default: out.append(contentsOf: String(scalar).utf8)
            }
        }
        out.append(0x22)
    }

    /// An indented rendering with sorted keys, for labels.json: a file people
    /// read, outside the state digest, so it is not held to one encoding.
    var indentedData: Data {
        var out = ""
        LabelSetJSON.indent(self, depth: 0, into: &out)
        out += "\n"
        return Data(out.utf8)
    }

    private static func indent(_ value: LabelSetJSON, depth: Int, into out: inout String) {
        let pad = String(repeating: "  ", count: depth + 1)
        let close = String(repeating: "  ", count: depth)
        switch value {
        case .array(let items) where !items.isEmpty:
            out += "[\n"
            for (i, item) in items.enumerated() {
                out += pad
                indent(item, depth: depth + 1, into: &out)
                out += i < items.count - 1 ? ",\n" : "\n"
            }
            out += close + "]"
        case .object(let fields) where !fields.isEmpty:
            out += "{\n"
            let keys = fields.keys.sorted { $0.utf8.lexicographicallyPrecedes($1.utf8) }
            for (i, key) in keys.enumerated() {
                out += pad + String(decoding: LabelSetJSON.string(key).canonicalData, as: UTF8.self)
                out += ": "
                indent(fields[key]!, depth: depth + 1, into: &out)
                out += i < keys.count - 1 ? ",\n" : "\n"
            }
            out += close + "}"
        default: out += String(decoding: value.canonicalData, as: UTF8.self)
        }
    }
}

// MARK: - Numbers

extension LabelSetJSON {
    static func int<T: BinaryInteger>(_ value: T) -> LabelSetJSON { .number(String(value)) }

    /// A double as Go's encoding/json writes it, which is ECMAScript's
    /// Number.prototype.toString: the shortest digits that round-trip,
    /// positional between 1e-6 and 1e21 and exponential outside, with no
    /// ".0" on a whole number. Not-a-number and infinities are refused, as
    /// Go refuses them.
    static func double(_ value: Double) throws -> LabelSetJSON { .number(try formatDouble(value)) }

    static func formatDouble(_ value: Double) throws -> String {
        guard value.isFinite else {
            throw LabelSetCodingError("\(value) cannot be written as JSON")
        }
        // Go writes negative zero as "-0"; ECMAScript would write "0". The
        // Go writer is the reference.
        if value == 0 { return value.sign == .minus ? "-0" : "0" }

        // Swift's description is the shortest round-trip form, in one of
        // two shapes: "123.45" or "1.2345e-07". Take its digits and decimal
        // exponent and lay them out again by the ECMAScript rules.
        let text = "\(abs(value))"
        var mantissa = Substring(text)
        var exponent = 0
        if let e = text.firstIndex(where: { $0 == "e" || $0 == "E" }) {
            mantissa = text[..<e]
            guard let parsed = Int(text[text.index(after: e)...]) else {
                throw LabelSetCodingError("cannot format \(value)")
            }
            exponent = parsed
        }
        let parts = mantissa.split(separator: ".", omittingEmptySubsequences: false)
        let whole = parts[0]
        let fraction = parts.count > 1 ? parts[1] : ""
        var digits = Array(whole) + Array(fraction)
        // The value is 0.d1d2…dk × 10^n, written as n = point position.
        var n = whole.count + exponent
        while digits.first == "0" {
            digits.removeFirst()
            n -= 1
        }
        while digits.last == "0" { digits.removeLast() }
        guard !digits.isEmpty else { return "0" }
        let k = digits.count
        let ds = String(digits)
        var out: String
        if k <= n && n <= 21 {
            out = ds + String(repeating: "0", count: n - k)
        } else if 0 < n && n <= 21 {
            out = String(digits[0..<n]) + "." + String(digits[n...])
        } else if -6 < n && n <= 0 {
            out = "0." + String(repeating: "0", count: -n) + ds
        } else {
            let e = n - 1
            let sign = e < 0 ? "-" : "+"
            out =
                (k == 1 ? ds : String(digits[0]) + "." + String(digits[1...])) + "e" + sign
                + String(abs(e))
        }
        if value < 0 { out = "-" + out }
        return out
    }
}

// MARK: - Parser

extension LabelSetJSON {
    /// Parses one JSON value that must fill `data` (bar trailing whitespace).
    static func parse(_ data: Data) throws -> LabelSetJSON {
        var parser = Parser(bytes: [UInt8](data))
        parser.skipWhitespace()
        let value = try parser.value(depth: 0)
        parser.skipWhitespace()
        guard parser.index == parser.bytes.count else {
            throw LabelSetCodingError("trailing data after JSON value")
        }
        return value
    }

    private struct Parser {
        let bytes: [UInt8]
        var index = 0

        static let maxDepth = 64

        mutating func skipWhitespace() {
            while index < bytes.count,
                bytes[index] == 0x20 || bytes[index] == 0x0A || bytes[index] == 0x0D
                    || bytes[index] == 0x09
            { index += 1 }
        }

        func fail(_ what: String) -> LabelSetCodingError {
            LabelSetCodingError("invalid JSON at byte \(index): \(what)")
        }

        mutating func value(depth: Int) throws -> LabelSetJSON {
            guard depth < Parser.maxDepth else { throw fail("nested too deeply") }
            guard index < bytes.count else { throw fail("unexpected end") }
            switch bytes[index] {
            case 0x7B: return try object(depth: depth)
            case 0x5B: return try array(depth: depth)
            case 0x22: return .string(try string())
            case 0x74:
                try literal("true")
                return .bool(true)
            case 0x66:
                try literal("false")
                return .bool(false)
            case 0x6E:
                try literal("null")
                return .null
            default: return .number(try number())
            }
        }

        mutating func literal(_ word: String) throws {
            let w = Array(word.utf8)
            guard index + w.count <= bytes.count, Array(bytes[index..<index + w.count]) == w else {
                throw fail("expected \(word)")
            }
            index += w.count
        }

        mutating func object(depth: Int) throws -> LabelSetJSON {
            index += 1
            var fields: [String: LabelSetJSON] = [:]
            skipWhitespace()
            if index < bytes.count, bytes[index] == 0x7D {
                index += 1
                return .object(fields)
            }
            while true {
                skipWhitespace()
                guard index < bytes.count, bytes[index] == 0x22 else { throw fail("expected key") }
                let key = try string()
                skipWhitespace()
                guard index < bytes.count, bytes[index] == 0x3A else { throw fail("expected :") }
                index += 1
                skipWhitespace()
                fields[key] = try value(depth: depth + 1)
                skipWhitespace()
                guard index < bytes.count else { throw fail("unterminated object") }
                if bytes[index] == 0x2C {
                    index += 1
                    continue
                }
                guard bytes[index] == 0x7D else { throw fail("expected , or }") }
                index += 1
                return .object(fields)
            }
        }

        mutating func array(depth: Int) throws -> LabelSetJSON {
            index += 1
            var items: [LabelSetJSON] = []
            skipWhitespace()
            if index < bytes.count, bytes[index] == 0x5D {
                index += 1
                return .array(items)
            }
            while true {
                skipWhitespace()
                items.append(try value(depth: depth + 1))
                skipWhitespace()
                guard index < bytes.count else { throw fail("unterminated array") }
                if bytes[index] == 0x2C {
                    index += 1
                    continue
                }
                guard bytes[index] == 0x5D else { throw fail("expected , or ]") }
                index += 1
                return .array(items)
            }
        }

        mutating func number() throws -> String {
            let start = index
            if index < bytes.count, bytes[index] == 0x2D { index += 1 }
            func digits() -> Int {
                let from = index
                while index < bytes.count, bytes[index] >= 0x30, bytes[index] <= 0x39 { index += 1 }
                return index - from
            }
            guard index < bytes.count else { throw fail("expected a number") }
            if bytes[index] == 0x30 {
                index += 1
            } else if digits() == 0 {
                throw fail("expected a value")
            }
            if index < bytes.count, bytes[index] == 0x2E {
                index += 1
                guard digits() > 0 else { throw fail("expected fraction digits") }
            }
            if index < bytes.count, bytes[index] == 0x65 || bytes[index] == 0x45 {
                index += 1
                if index < bytes.count, bytes[index] == 0x2B || bytes[index] == 0x2D { index += 1 }
                guard digits() > 0 else { throw fail("expected exponent digits") }
            }
            return String(decoding: bytes[start..<index], as: UTF8.self)
        }

        mutating func hex4() throws -> UInt32 {
            guard index + 4 <= bytes.count,
                let v = UInt32(String(decoding: bytes[index..<index + 4], as: UTF8.self), radix: 16)
            else { throw fail("bad \\u escape") }
            index += 4
            return v
        }

        mutating func string() throws -> String {
            index += 1
            var scalars = String.UnicodeScalarView()
            var runStart = index
            func flushRun() throws {
                if runStart < index {
                    guard let run = String(validating: bytes[runStart..<index], as: UTF8.self)
                    else { throw fail("invalid UTF-8 in string") }
                    scalars.append(contentsOf: run.unicodeScalars)
                }
            }
            while true {
                guard index < bytes.count else { throw fail("unterminated string") }
                let b = bytes[index]
                if b == 0x22 {
                    try flushRun()
                    index += 1
                    return String(scalars)
                }
                if b < 0x20 { throw fail("control character in string") }
                if b != 0x5C {
                    index += 1
                    continue
                }
                try flushRun()
                index += 1
                guard index < bytes.count else { throw fail("unterminated escape") }
                let e = bytes[index]
                index += 1
                switch e {
                case 0x22: scalars.append("\"")
                case 0x5C: scalars.append("\\")
                case 0x2F: scalars.append("/")
                case 0x62: scalars.append("\u{08}")
                case 0x66: scalars.append("\u{0C}")
                case 0x6E: scalars.append("\n")
                case 0x72: scalars.append("\r")
                case 0x74: scalars.append("\t")
                case 0x75:
                    var code = try hex4()
                    if (0xD800..<0xDC00).contains(code) {
                        // A surrogate pair, or Go's replacement for half of one.
                        if index + 6 <= bytes.count, bytes[index] == 0x5C, bytes[index + 1] == 0x75
                        {
                            let save = index
                            index += 2
                            let low = try hex4()
                            if (0xDC00..<0xE000).contains(low) {
                                code = 0x10000 + ((code - 0xD800) << 10) + (low - 0xDC00)
                            } else {
                                index = save
                                code = 0xFFFD
                            }
                        } else {
                            code = 0xFFFD
                        }
                    } else if (0xDC00..<0xE000).contains(code) {
                        code = 0xFFFD
                    }
                    scalars.append(Unicode.Scalar(code) ?? "\u{FFFD}")
                default: throw fail("bad escape")
                }
                runStart = index
            }
        }
    }
}

// MARK: - Strict field access

/// Reads one JSON object's fields by name and refuses any it was not asked
/// for. Absent and null fields read as the zero value, as Go's decoder leaves
/// them.
struct LabelSetFields {
    private let fields: [String: LabelSetJSON]
    private var read: Set<String> = []
    let context: String

    init(_ value: LabelSetJSON, _ context: String) throws {
        guard case .object(let fields) = value else {
            throw LabelSetCodingError("\(context): expected an object")
        }
        self.fields = fields
        self.context = context
    }

    private mutating func take(_ key: String) -> LabelSetJSON? {
        read.insert(key)
        guard let value = fields[key], value != .null else { return nil }
        return value
    }

    /// Refuses a field nothing asked for: it would be lost on the next save.
    func finish() throws {
        let unknown = Set(fields.keys).subtracting(read)
        if let first = unknown.sorted().first {
            throw LabelSetCodingError("\(context): unknown field \"\(first)\"")
        }
    }

    mutating func has(_ key: String) -> Bool {
        read.insert(key)
        if let value = fields[key], value != .null { return true }
        return false
    }

    mutating func string(_ key: String) throws -> String {
        guard let value = take(key) else { return "" }
        guard case .string(let s) = value else {
            throw LabelSetCodingError("\(context).\(key): expected a string")
        }
        return s
    }

    /// An optional string, where empty and absent are the same thing, as
    /// they are to an omitempty field in Go.
    mutating func optionalString(_ key: String) throws -> String? {
        let s = try string(key)
        return s.isEmpty ? nil : s
    }

    mutating func bool(_ key: String) throws -> Bool {
        guard let value = take(key) else { return false }
        guard case .bool(let b) = value else {
            throw LabelSetCodingError("\(context).\(key): expected a boolean")
        }
        return b
    }

    private mutating func numberText(_ key: String) throws -> String? {
        guard let value = take(key) else { return nil }
        guard case .number(let text) = value else {
            throw LabelSetCodingError("\(context).\(key): expected a number")
        }
        return text
    }

    mutating func int(_ key: String) throws -> Int {
        guard let text = try numberText(key) else { return 0 }
        guard let v = Int(text) else {
            throw LabelSetCodingError("\(context).\(key): \(text) is not an integer")
        }
        return v
    }

    mutating func int64(_ key: String) throws -> Int64 {
        guard let text = try numberText(key) else { return 0 }
        guard let v = Int64(text) else {
            throw LabelSetCodingError("\(context).\(key): \(text) is not an integer")
        }
        return v
    }

    mutating func uint64(_ key: String) throws -> UInt64 {
        guard let text = try numberText(key) else { return 0 }
        guard let v = UInt64(text) else {
            throw LabelSetCodingError("\(context).\(key): \(text) is not an unsigned integer")
        }
        return v
    }

    mutating func double(_ key: String) throws -> Double {
        guard let text = try numberText(key) else { return 0 }
        guard let v = Double(text), v.isFinite else {
            throw LabelSetCodingError("\(context).\(key): \(text) is not a number")
        }
        return v
    }

    mutating func value(_ key: String) -> LabelSetJSON? { take(key) }

    mutating func array(_ key: String) throws -> [LabelSetJSON] {
        guard let value = take(key) else { return [] }
        guard case .array(let items) = value else {
            throw LabelSetCodingError("\(context).\(key): expected an array")
        }
        return items
    }

    /// Distinguishes an absent array from an empty one, for the one field
    /// (a delta's correspondences) where the difference is meaningful.
    mutating func optionalArray(_ key: String) throws -> [LabelSetJSON]? {
        guard let value = take(key) else { return nil }
        guard case .array(let items) = value else {
            throw LabelSetCodingError("\(context).\(key): expected an array")
        }
        return items
    }

    mutating func strings(_ key: String) throws -> [String] {
        try array(key).map { item in
            guard case .string(let s) = item else {
                throw LabelSetCodingError("\(context).\(key): expected strings")
            }
            return s
        }
    }

    mutating func ints(_ key: String) throws -> [Int] {
        try array(key).map { item in
            guard case .number(let text) = item, let v = Int(text) else {
                throw LabelSetCodingError("\(context).\(key): expected integers")
            }
            return v
        }
    }

    /// A raw-value enum such as a review status, refused when unknown.
    mutating func enumValue<T: RawRepresentable>(_ key: String, _: T.Type) throws -> T
    where T.RawValue == String {
        let raw = try string(key)
        guard let v = T(rawValue: raw) else {
            throw LabelSetCodingError("\(context).\(key): unknown value \"\(raw)\"")
        }
        return v
    }
}

/// Builds an object, leaving out what Go's omitempty would.
struct LabelSetObjectBuilder {
    private(set) var fields: [String: LabelSetJSON] = [:]

    var json: LabelSetJSON { .object(fields) }

    mutating func set(_ key: String, _ value: LabelSetJSON) { fields[key] = value }

    mutating func string(_ key: String, _ value: String) { fields[key] = .string(value) }

    mutating func omitEmpty(_ key: String, _ value: String?) {
        if let value, !value.isEmpty { fields[key] = .string(value) }
    }

    mutating func int<T: BinaryInteger>(_ key: String, _ value: T) { fields[key] = .int(value) }

    mutating func omitZero<T: BinaryInteger>(_ key: String, _ value: T?) {
        if let value, value != 0 { fields[key] = .int(value) }
    }

    mutating func double(_ key: String, _ value: Double) throws { fields[key] = try .double(value) }

    mutating func bool(_ key: String, _ value: Bool) { fields[key] = .bool(value) }

    mutating func omitFalse(_ key: String, _ value: Bool) { if value { fields[key] = .bool(true) } }

    mutating func omitEmptyArray(_ key: String, _ items: [LabelSetJSON]) {
        if !items.isEmpty { fields[key] = .array(items) }
    }
}
