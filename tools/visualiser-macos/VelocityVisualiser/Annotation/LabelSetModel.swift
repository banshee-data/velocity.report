// LabelSetModel.swift
// A label set's state: the packet table, the records, the foreground domains,
// the windows, each domain's returns per window, and the masks.
//
// Mirrors internal/lidar/annotation/labelset/state.go, keys.go and codec.go,
// field for field and rule for rule; the format specification is that
// package's README. Labels cite capture returns by (packet key, block,
// channel), numbered through the label set's own packet table, so they apply
// to every run of the capture rather than to one run's pack. The Go fixtures
// in testdata/parity hold this file to the same bytes and digests.

import CryptoKit
import Foundation

// MARK: - Packet keys and layout

/// Packet keys are the first 64 bits of SHA-256 over a UDP payload, read
/// big-endian. They are plain UInt64 values here; these are their forms.
enum PacketKeys {
    /// The only derivation this build reads.
    static let scheme = "sha256-64:udp-payload"

    /// Sixteen lower-case hex digits, as JSON carries a key: a JSON number
    /// loses precision above 2^53.
    static func hex(_ key: UInt64) -> String {
        let s = String(key, radix: 16)
        return String(repeating: "0", count: 16 - s.count) + s
    }

    static func parse(_ text: String) throws -> UInt64 {
        guard text.count == 16, text.allSatisfy(\.isHexDigit), let v = UInt64(text, radix: 16)
        else { throw LabelSetCodingError("packet key \"\(text)\" is not 16 hex digits") }
        return v
    }

    /// The key of a UDP payload.
    static func key(of payload: Data) -> UInt64 {
        let sum = Array(SHA256.hash(data: payload))
        return sum[0..<8].reduce(UInt64(0)) { ($0 << 8) | UInt64($1) }
    }

    /// Little-endian uint64s, as packets.bin holds them.
    static func encode(_ keys: [UInt64]) -> Data {
        var data = Data(count: 8 * keys.count)
        data.withUnsafeMutableBytes { buffer in
            for (i, k) in keys.enumerated() {
                buffer.storeBytes(of: k.littleEndian, toByteOffset: 8 * i, as: UInt64.self)
            }
        }
        return data
    }

    static func decode(_ data: Data, name: String) throws -> [UInt64] {
        guard data.count % 8 == 0 else {
            throw LabelSetCodingError("\(name) is \(data.count) bytes, not whole uint64s")
        }
        return data.withUnsafeBytes { buffer in
            (0..<(data.count / 8)).map {
                UInt64(littleEndian: buffer.loadUnaligned(fromByteOffset: 8 * $0, as: UInt64.self))
            }
        }
    }
}

/// How a sensor's packet divides into returns. A return number is
/// `packet_index × blocks × channels + block × channels + (channel − 1)`, block
/// from 0 and channel from 1, as the Pandar40P parser numbers them. The pack
/// manifest's `return_layout` has the same shape.
struct ReturnLayout: Codable, Equatable, Hashable {
    var model: String
    var blocks: Int
    var channels: Int

    static let pandar40P = ReturnLayout(model: "hesai-pandar40p", blocks: 10, channels: 40)

    /// The returns one packet can carry.
    var perPacket: UInt64 { UInt64(blocks) * UInt64(channels) }

    func validate() throws {
        guard !model.isEmpty else { throw LabelSetCodingError("layout has no sensor model") }
        guard blocks >= 1, channels >= 1, blocks <= 1 << 10, channels <= 1 << 10 else {
            throw LabelSetCodingError("layout \(model): \(blocks) blocks of \(channels) channels")
        }
    }

    func returnNumber(packet: Int, block: Int, channel: Int) throws -> UInt64 {
        guard packet >= 0, block >= 0, block < blocks, channel >= 1, channel <= channels else {
            throw LabelSetCodingError(
                "return (packet \(packet), block \(block), channel \(channel)) is outside layout \(model)"
            )
        }
        return UInt64(packet) * perPacket + UInt64(block) * UInt64(channels) + UInt64(channel - 1)
    }

    /// Inverts `returnNumber`.
    func locate(_ r: UInt64) -> (packet: Int, block: Int, channel: Int) {
        let within = r % perPacket
        return (
            Int(r / perPacket), Int(within / UInt64(channels)), Int(within % UInt64(channels)) + 1
        )
    }

    var json: LabelSetJSON {
        .object(["model": .string(model), "blocks": .int(blocks), "channels": .int(channels)])
    }

    init(model: String, blocks: Int, channels: Int) {
        self.model = model
        self.blocks = blocks
        self.channels = channels
    }

    init(json: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(json, context)
        model = try f.string("model")
        blocks = try f.int("blocks")
        channels = try f.int("channels")
        try f.finish()
    }
}

// MARK: - State

/// The only kind of domain: annotation is downstream of foreground
/// separation, so every mask is made on what some extraction kept.
let labelSetDomainForeground = "foreground"

/// The foreground extraction a mask was made on. A return outside a mask's
/// domain was never shown to the labeller and is unlabelled, never a
/// negative.
struct LabelSetDomain: Equatable {
    var domainID: String
    var kind: String = labelSetDomainForeground
    var runID: String?
    var runConfigID: String?
    var configHash: String?
    var paramsHash: String?
    var buildVersion: String?
    var buildGitSHA: String?
    var heightBand: HeightBand?
    var note: String?

    /// Empty optional strings read as absent, as Go's omitempty writes them.
    var normalised: LabelSetDomain {
        var d = self
        d.runID = d.runID.nonEmpty
        d.runConfigID = d.runConfigID.nonEmpty
        d.configHash = d.configHash.nonEmpty
        d.paramsHash = d.paramsHash.nonEmpty
        d.buildVersion = d.buildVersion.nonEmpty
        d.buildGitSHA = d.buildGitSHA.nonEmpty
        d.note = d.note.nonEmpty
        return d
    }
}

/// One frame as the labeller saw it: every return of the frame, as the
/// half-open range of return numbers [start, end).
struct LabelSetWindow: Equatable {
    var windowID: Int
    var start: UInt64
    var end: UInt64
    /// Capture times, for people reading the file; nothing joins on them.
    var captureStartNs: Int64 = 0
    var captureEndNs: Int64 = 0
    /// The version 1 sample this window was migrated from.
    var legacySampleID: Int?

    var length: UInt64 { end - start }
}

/// The returns a domain's extraction kept in one window, as offsets from the
/// window's start.
struct LabelSetDomainReturns: Equatable {
    var domainID: String
    var windowID: Int
    var returns: ReturnRanges
}

/// One object's membership within one window. Members and uncertain returns
/// are offsets from the window's start, inside the mask's domain.
struct LabelSetMask: Equatable {
    var objectID: String
    var windowID: Int
    var domainID: String
    var members: ReturnRanges = .empty
    var uncertain: ReturnRanges = .empty
    var completeness: MaskCompleteness = .unreviewed
    var visibility: Visibility = .unknown
    var status: ReviewStatus = .proposed
    var pose: AnnotationPose?
    var provenance = Provenance()
}

/// Object-level records.
struct LabelSetRecords: Equatable {
    var objects: [AnnotationObject] = []
    var correspondences: [TrackCorrespondence] = []
    var dismissedProposals: [String] = []
}

/// Everything a revision pins.
struct LabelSetState: Equatable {
    var packets: [UInt64] = []
    var records = LabelSetRecords()
    var domains: [LabelSetDomain] = []
    var windows: [LabelSetWindow] = []
    var domainReturns: [LabelSetDomainReturns] = []
    var masks: [LabelSetMask] = []

    /// Puts the state into its one permitted order and normalises what
    /// Go's encoding cannot distinguish. The packet table keeps its order:
    /// it is the return numbering.
    mutating func canonicalise() {
        records.objects = records.objects.map(\.labelSetNormalised).sorted {
            $0.objectID.utf8.lexicographicallyPrecedes($1.objectID.utf8)
        }
        records.correspondences = records.correspondences.map(\.labelSetNormalised).sorted { a, b in
            if a.objectID != b.objectID {
                return a.objectID.utf8.lexicographicallyPrecedes(b.objectID.utf8)
            }
            let ar = a.runID ?? ""
            let br = b.runID ?? ""
            if ar != br { return ar.utf8.lexicographicallyPrecedes(br.utf8) }
            if a.startNs != b.startNs { return a.startNs < b.startNs }
            return a.endNs < b.endNs
        }
        records.dismissedProposals.sort { $0.utf8.lexicographicallyPrecedes($1.utf8) }
        domains = domains.map(\.normalised).sorted {
            $0.domainID.utf8.lexicographicallyPrecedes($1.domainID.utf8)
        }
        windows.sort { $0.windowID < $1.windowID }
        domainReturns.sort {
            $0.domainID != $1.domainID
                ? $0.domainID.utf8.lexicographicallyPrecedes($1.domainID.utf8)
                : $0.windowID < $1.windowID
        }
        masks = masks.map { m in
            var m = m
            m.pose = m.pose?.labelSetNormalised
            m.provenance = m.provenance.labelSetNormalised
            return m
        }.sorted {
            $0.windowID != $1.windowID
                ? $0.windowID < $1.windowID
                : $0.objectID.utf8.lexicographicallyPrecedes($1.objectID.utf8)
        }
    }

    var canonical: LabelSetState {
        var c = self
        c.canonicalise()
        return c
    }

    /// Masks a person confirmed on an object a person confirmed: the only
    /// masks an evaluation may treat as reference truth.
    var reviewedMasks: [LabelSetMask] {
        let reviewed = Set(records.objects.filter { $0.status == .reviewed }.map(\.objectID))
        return masks.filter { $0.status == .reviewed && reviewed.contains($0.objectID) }
    }

    func window(_ id: Int) -> LabelSetWindow? { windows.first { $0.windowID == id } }

    func domainReturns(domainID: String, windowID: Int) -> ReturnRanges? {
        domainReturns.first { $0.domainID == domainID && $0.windowID == windowID }?.returns
    }

    func maskIndex(objectID: String, windowID: Int) -> Int? {
        masks.firstIndex { $0.objectID == objectID && $0.windowID == windowID }
    }
}

// MARK: - Validation

extension LabelSetState {
    /// Checks the state against its own rules, as Go's State.Validate does.
    /// It runs before every write and after every read: an invalid label set
    /// that loads produces plausible numbers, which is worse than one that
    /// refuses.
    func validate(layout: ReturnLayout) throws {
        func fail(_ message: String) -> LabelSetCodingError { LabelSetCodingError(message) }
        try layout.validate()
        var keys = Set<UInt64>()
        keys.reserveCapacity(packets.count)
        for (i, k) in packets.enumerated() where !keys.insert(k).inserted {
            throw fail("packet table lists \(PacketKeys.hex(k)) twice (index \(i))")
        }
        let limit = UInt64(packets.count) * layout.perPacket

        var objects: [String: ReviewStatus] = [:]
        for o in records.objects {
            if o.objectID.isEmpty { throw fail("an object has no id") }
            if objects[o.objectID] != nil {
                throw fail("object \"\(o.objectID)\" is declared twice")
            }
            objects[o.objectID] = o.status
        }
        for (i, c) in records.correspondences.enumerated() where objects[c.objectID] == nil {
            throw fail("correspondence \(i) references unknown object \"\(c.objectID)\"")
        }
        var dismissed = Set<String>()
        for id in records.dismissedProposals where id.isEmpty || !dismissed.insert(id).inserted {
            throw fail("invalid or duplicate dismissed proposal \"\(id)\"")
        }

        var domainIDs = Set<String>()
        for d in domains {
            if d.domainID.isEmpty || !domainIDs.insert(d.domainID).inserted {
                throw fail("invalid or duplicate domain \"\(d.domainID)\"")
            }
            if d.kind != labelSetDomainForeground {
                throw fail(
                    "domain \"\(d.domainID)\" has kind \"\(d.kind)\": every domain is a foreground extraction"
                )
            }
        }

        var windowsByID: [Int: LabelSetWindow] = [:]
        for w in windows {
            if windowsByID[w.windowID] != nil || w.windowID < 0 {
                throw fail("invalid or duplicate window \(w.windowID)")
            }
            if w.start >= w.end || w.end > limit {
                throw fail(
                    "window \(w.windowID) spans [\(w.start),\(w.end)), outside the \(limit) returns the packet table numbers"
                )
            }
            windowsByID[w.windowID] = w
        }
        let byStart = windows.sorted { $0.start < $1.start }
        for i in byStart.indices.dropFirst() where byStart[i].start < byStart[i - 1].end {
            throw fail("windows \(byStart[i - 1].windowID) and \(byStart[i].windowID) overlap")
        }

        struct DomainWindow: Hashable {
            var domain: String
            var window: Int
        }
        var returnsByKey: [DomainWindow: ReturnRanges] = [:]
        for d in domainReturns {
            let key = DomainWindow(domain: d.domainID, window: d.windowID)
            if returnsByKey[key] != nil {
                throw fail("domain \"\(d.domainID)\" lists window \(d.windowID) twice")
            }
            if !domainIDs.contains(d.domainID) {
                throw fail("returns for unknown domain \"\(d.domainID)\"")
            }
            guard let w = windowsByID[d.windowID] else {
                throw fail("domain \"\(d.domainID)\" returns cite unknown window \(d.windowID)")
            }
            do { try d.returns.validate() } catch let error as LabelSetCodingError {
                throw fail("domain \"\(d.domainID)\" window \(d.windowID): \(error.description)")
            }
            if d.returns.max > w.length {
                throw fail(
                    "domain \"\(d.domainID)\" window \(d.windowID) returns run past the window")
            }
            returnsByKey[key] = d.returns
        }

        struct MaskKey: Hashable {
            var object: String
            var window: Int
        }
        var seen = Set<MaskKey>()
        var claimed: [Int: [(String, ReturnRanges)]] = [:]
        for (i, m) in masks.enumerated() {
            if !seen.insert(MaskKey(object: m.objectID, window: m.windowID)).inserted {
                throw fail("duplicate mask for object \"\(m.objectID)\" window \(m.windowID)")
            }
            if objects[m.objectID] == nil {
                throw fail("mask \(i) references unknown object \"\(m.objectID)\"")
            }
            if windowsByID[m.windowID] == nil {
                throw fail("mask \(i) (\(m.objectID)) references unknown window \(m.windowID)")
            }
            guard let domain = returnsByKey[DomainWindow(domain: m.domainID, window: m.windowID)]
            else {
                throw fail(
                    "mask \(i) (\(m.objectID) window \(m.windowID)) cites domain \"\(m.domainID)\", which has no returns in that window"
                )
            }
            for (name, set) in [("members", m.members), ("uncertain", m.uncertain)] {
                do { try set.validate() } catch let error as LabelSetCodingError {
                    throw fail("mask \(i) (\(m.objectID)) \(name): \(error.description)")
                }
                let outside = ReturnRanges.difference(set, domain)
                if let first = outside.ranges.first {
                    throw fail(
                        "mask \(i) (\(m.objectID) window \(m.windowID)) \(name) include return \(first.start), which domain \"\(m.domainID)\" did not keep"
                    )
                }
            }
            if let both = ReturnRanges.intersect(m.members, m.uncertain).ranges.first {
                throw fail(
                    "mask \(i) (\(m.objectID)): return \(both.start) is both a member and uncertain"
                )
            }
            // A return belongs to at most one object in a window. Two objects
            // claiming one is a conflict for a person to resolve.
            for (other, set) in claimed[m.windowID] ?? [] {
                if let both = ReturnRanges.intersect(set, m.members).ranges.first {
                    throw fail(
                        "window \(m.windowID) return \(both.start) is claimed by both \"\(other)\" and \"\(m.objectID)\""
                    )
                }
            }
            claimed[m.windowID, default: []].append((m.objectID, m.members))
        }
    }
}

// MARK: - Head files and the state digest

enum LabelSetFiles {
    static let manifest = "labels.json"
    static let packets = "packets.bin"
    static let records = "objects.json"
    static let domains = "domains.json"
    static let windows = "windows.json"
    static let domainReturns = "domain-returns.ndjson"
    static let masks = "masks.ndjson"
    static let journal = "journal.ndjson"
    static let journalBase = "journal-base"
    static let lock = ".lock"

    /// The order in which the state digest reads the head files.
    static let state = [packets, records, domains, windows, domainReturns, masks]

    /// The bound on any one head file or journal line on read.
    static let maxBytes = 256 << 20
}

extension LabelSetState {
    /// The bytes of the head files, in canonical order whatever order the
    /// state is in.
    func encodedFiles() throws -> [String: Data] {
        let s = canonical
        var files: [String: Data] = [:]
        files[LabelSetFiles.packets] = PacketKeys.encode(s.packets)
        files[LabelSetFiles.records] = try s.records.json().canonicalLine
        files[LabelSetFiles.domains] =
            LabelSetJSON.array(try s.domains.map { try $0.json() }).canonicalLine
        files[LabelSetFiles.windows] = LabelSetJSON.array(s.windows.map(\.json)).canonicalLine
        var lines = Data()
        for d in s.domainReturns { lines.append(d.json.canonicalLine) }
        files[LabelSetFiles.domainReturns] = lines
        lines = Data()
        for m in s.masks { lines.append(try m.json().canonicalLine) }
        files[LabelSetFiles.masks] = lines
        return files
    }

    /// The digest of the state's canonical encoding.
    func digest() throws -> String { LabelSetState.digest(of: try encodedFiles()) }

    /// SHA-256 over the head files in a fixed order, each framed by its name
    /// and length, so no rearrangement of bytes between files keeps the
    /// digest.
    static func digest(of files: [String: Data]) -> String {
        var hasher = SHA256()
        hasher.update(data: Data("velocity.report label set state v2\n".utf8))
        for name in LabelSetFiles.state {
            let bytes = files[name] ?? Data()
            hasher.update(data: Data("\(name)\n\(bytes.count)\n".utf8))
            hasher.update(data: bytes)
        }
        return "sha256:" + hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }

    /// Parses head files. It does not validate; callers do, against the
    /// manifest's layout.
    static func decode(files: [String: Data]) throws -> LabelSetState {
        var s = LabelSetState()
        s.packets = try PacketKeys.decode(
            files[LabelSetFiles.packets] ?? Data(), name: LabelSetFiles.packets)
        s.records = try LabelSetRecords(
            json: LabelSetJSON.parse(files[LabelSetFiles.records] ?? Data()), LabelSetFiles.records)
        s.domains = try LabelSetJSON.parseArray(
            files[LabelSetFiles.domains] ?? Data(), LabelSetFiles.domains
        ).map { try LabelSetDomain(json: $0, LabelSetFiles.domains) }
        s.windows = try LabelSetJSON.parseArray(
            files[LabelSetFiles.windows] ?? Data(), LabelSetFiles.windows
        ).map { try LabelSetWindow(json: $0, LabelSetFiles.windows) }
        s.domainReturns = try LabelSetJSON.eachLine(
            files[LabelSetFiles.domainReturns] ?? Data(), LabelSetFiles.domainReturns
        ) { try LabelSetDomainReturns(json: $0, $1) }
        s.masks = try LabelSetJSON.eachLine(
            files[LabelSetFiles.masks] ?? Data(), LabelSetFiles.masks
        ) { try LabelSetMask(json: $0, $1) }
        return s
    }
}

extension LabelSetJSON {
    /// A file holding one JSON array; `null` reads as empty, as Go reads it.
    static func parseArray(_ data: Data, _ context: String) throws -> [LabelSetJSON] {
        let value: LabelSetJSON
        do { value = try parse(data) } catch let error as LabelSetCodingError {
            throw LabelSetCodingError("\(context): \(error.description)")
        }
        switch value {
        case .null: return []
        case .array(let items): return items
        default: throw LabelSetCodingError("\(context): expected an array")
        }
    }

    /// Parses newline-delimited JSON: one value a line, no empty lines.
    static func eachLine<T>(
        _ data: Data, _ context: String, _ body: (LabelSetJSON, String) throws -> T
    ) throws -> [T] {
        var out: [T] = []
        var n = 0
        var start = data.startIndex
        while start < data.endIndex {
            let end = data[start...].firstIndex(of: 0x0A) ?? data.endIndex
            n += 1
            let line = data[start..<end]
            guard !line.isEmpty else { throw LabelSetCodingError("\(context): line \(n) is empty") }
            let place = "\(context) line \(n)"
            do { out.append(try body(try parse(Data(line)), place)) } catch let error
                as LabelSetCodingError
            {
                throw error.description.hasPrefix(place)
                    ? error : LabelSetCodingError("\(place): \(error.description)")
            }
            start = end < data.endIndex ? data.index(after: end) : end
        }
        return out
    }
}

// MARK: - Record encodings

extension Optional where Wrapped == String {
    /// Nil for an empty string: what an omitempty field round-trips to.
    var nonEmpty: String? {
        guard let self, !self.isEmpty else { return nil }
        return self
    }
}

extension Provenance {
    var labelSetNormalised: Provenance {
        var p = self
        p.session = p.session.nonEmpty
        p.operation = p.operation.nonEmpty
        p.algorithm = p.algorithm.nonEmpty
        p.algorithmVersion = p.algorithmVersion.nonEmpty
        if p.parentRevision == 0 { p.parentRevision = nil }
        return p
    }

    var labelSetJSON: LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.string("author", author)
        b.omitEmpty("session", session)
        b.string("created_utc", createdUTC)
        b.omitEmpty("operation", operation)
        b.omitZero("parent_revision", parentRevision)
        b.int("revision", revision)
        b.omitEmpty("algorithm", algorithm)
        b.omitEmpty("algorithm_version", algorithmVersion)
        return b.json
    }

    init(labelSetJSON value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        self.init(
            author: try f.string("author"), session: try f.optionalString("session"),
            createdUTC: try f.string("created_utc"), operation: try f.optionalString("operation"),
            parentRevision: try f.int("parent_revision"), revision: try f.int("revision"),
            algorithm: try f.optionalString("algorithm"),
            algorithmVersion: try f.optionalString("algorithm_version"))
        if parentRevision == 0 { parentRevision = nil }
        try f.finish()
    }
}

extension AnnotationObject {
    var labelSetNormalised: AnnotationObject {
        var o = self
        o.subtype = o.subtype.nonEmpty
        o.notes = o.notes.nonEmpty
        o.provenance = o.provenance.labelSetNormalised
        return o
    }

    func labelSetJSON() throws -> LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.string("object_id", objectID)
        b.string("class", objectClass)
        b.omitEmpty("subtype", subtype)
        try b.double("confidence", confidence)
        b.string("status", status.rawValue)
        b.set("provenance", provenance.labelSetJSON)
        b.omitEmpty("notes", notes)
        return b.json
    }

    init(labelSetJSON value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        self.init(
            objectID: try f.string("object_id"), objectClass: try f.string("class"),
            subtype: try f.optionalString("subtype"), confidence: try f.double("confidence"),
            status: try f.enumValue("status", ReviewStatus.self),
            provenance: try f.value("provenance").map { try Provenance(labelSetJSON: $0, context) }
                ?? Provenance(), notes: try f.optionalString("notes"))
        try f.finish()
    }
}

extension AnnotationPose {
    var labelSetNormalised: AnnotationPose {
        var p = self
        p.provenance = p.provenance.labelSetNormalised
        return p
    }

    func labelSetJSON() throws -> LabelSetJSON {
        var b = LabelSetObjectBuilder()
        try b.double("center_x", centerX)
        try b.double("center_y", centerY)
        try b.double("center_z", centerZ)
        try b.double("length", length)
        try b.double("width", width)
        try b.double("height", height)
        try b.double("yaw_rad", yawRad)
        b.bool("axis_ambiguous", axisAmbiguous)
        try b.double("confidence", confidence)
        b.string("status", status.rawValue)
        b.set("provenance", provenance.labelSetJSON)
        return b.json
    }

    init(labelSetJSON value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        self.init(
            centerX: try f.double("center_x"), centerY: try f.double("center_y"),
            centerZ: try f.double("center_z"), length: try f.double("length"),
            width: try f.double("width"), height: try f.double("height"),
            yawRad: try f.double("yaw_rad"), axisAmbiguous: try f.bool("axis_ambiguous"),
            confidence: try f.double("confidence"),
            status: try f.enumValue("status", ReviewStatus.self),
            provenance: try f.value("provenance").map { try Provenance(labelSetJSON: $0, context) }
                ?? Provenance())
        try f.finish()
    }
}

extension TrackCorrespondence {
    var labelSetNormalised: TrackCorrespondence {
        var c = self
        c.runID = c.runID.nonEmpty
        c.note = c.note.nonEmpty
        return c
    }

    var labelSetJSON: LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.string("object_id", objectID)
        b.omitEmpty("run_id", runID)
        // Not omitempty in Go: a nil list is written as null, an empty one
        // as [].
        b.set("track_ids", trackIDs.map { .array($0.map(LabelSetJSON.string)) } ?? .null)
        b.int("start_ns", startNs)
        b.int("end_ns", endNs)
        b.omitEmpty("note", note)
        return b.json
    }

    init(labelSetJSON value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        self.init(
            objectID: try f.string("object_id"), runID: try f.optionalString("run_id"),
            trackIDs: try f.has("track_ids") ? f.strings("track_ids") : nil,
            startNs: try f.int64("start_ns"), endNs: try f.int64("end_ns"),
            note: try f.optionalString("note"))
        try f.finish()
    }
}

extension HeightBand {
    func labelSetJSON() throws -> LabelSetJSON {
        var b = LabelSetObjectBuilder()
        try b.double("floor_m", floorM)
        try b.double("ceiling_m", ceilingM)
        b.bool("remove_ground", removeGround)
        return b.json
    }

    init(labelSetJSON value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        self.init(
            floorM: try f.double("floor_m"), ceilingM: try f.double("ceiling_m"),
            removeGround: try f.bool("remove_ground"))
        try f.finish()
    }
}

extension LabelSetRecords {
    func json() throws -> LabelSetJSON {
        .object([
            "objects": .array(try objects.map { try $0.labelSetJSON() }),
            "correspondences": .array(correspondences.map(\.labelSetJSON)),
            "dismissed_proposals": .array(dismissedProposals.map(LabelSetJSON.string)),
        ])
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        objects = try f.array("objects").map { try AnnotationObject(labelSetJSON: $0, context) }
        correspondences = try f.array("correspondences").map {
            try TrackCorrespondence(labelSetJSON: $0, context)
        }
        dismissedProposals = try f.strings("dismissed_proposals")
        try f.finish()
    }
}

extension LabelSetDomain {
    func json() throws -> LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.string("domain_id", domainID)
        b.string("kind", kind)
        b.omitEmpty("run_id", runID)
        b.omitEmpty("run_config_id", runConfigID)
        b.omitEmpty("config_hash", configHash)
        b.omitEmpty("params_hash", paramsHash)
        b.omitEmpty("build_version", buildVersion)
        b.omitEmpty("build_git_sha", buildGitSHA)
        if let heightBand { b.set("height_band", try heightBand.labelSetJSON()) }
        b.omitEmpty("note", note)
        return b.json
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        domainID = try f.string("domain_id")
        kind = try f.string("kind")
        runID = try f.optionalString("run_id")
        runConfigID = try f.optionalString("run_config_id")
        configHash = try f.optionalString("config_hash")
        paramsHash = try f.optionalString("params_hash")
        buildVersion = try f.optionalString("build_version")
        buildGitSHA = try f.optionalString("build_git_sha")
        heightBand = try f.value("height_band").map { try HeightBand(labelSetJSON: $0, context) }
        note = try f.optionalString("note")
        try f.finish()
    }

}

extension LabelSetWindow {
    var json: LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.int("window_id", windowID)
        b.int("start", start)
        b.int("end", end)
        b.omitZero("capture_start_ns", captureStartNs)
        b.omitZero("capture_end_ns", captureEndNs)
        // A pointer in Go: omitted only when absent, so sample 0 is written.
        if let legacySampleID { b.int("legacy_sample_id", legacySampleID) }
        return b.json
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        windowID = try f.int("window_id")
        start = try f.uint64("start")
        end = try f.uint64("end")
        captureStartNs = try f.int64("capture_start_ns")
        captureEndNs = try f.int64("capture_end_ns")
        legacySampleID = try f.has("legacy_sample_id") ? f.int("legacy_sample_id") : nil
        try f.finish()
    }

}

extension LabelSetDomainReturns {
    var json: LabelSetJSON {
        .object([
            "domain_id": .string(domainID), "window_id": .int(windowID), "returns": returns.json,
        ])
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        domainID = try f.string("domain_id")
        windowID = try f.int("window_id")
        returns = try ReturnRanges.decode(f.value("returns"), "\(context) returns")
        try f.finish()
    }

}

extension LabelSetMask {
    func json() throws -> LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.string("object_id", objectID)
        b.int("window_id", windowID)
        b.string("domain_id", domainID)
        b.set("members", members.json)
        if !uncertain.isEmpty { b.set("uncertain", uncertain.json) }
        b.string("completeness", completeness.rawValue)
        b.string("visibility", visibility.rawValue)
        b.string("status", status.rawValue)
        if let pose { b.set("pose", try pose.labelSetJSON()) }
        b.set("provenance", provenance.labelSetJSON)
        return b.json
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        objectID = try f.string("object_id")
        windowID = try f.int("window_id")
        domainID = try f.string("domain_id")
        members = try ReturnRanges.decode(f.value("members"), "\(context) members")
        uncertain = try ReturnRanges.decode(f.value("uncertain"), "\(context) uncertain")
        completeness = try f.enumValue("completeness", MaskCompleteness.self)
        visibility = try f.enumValue("visibility", Visibility.self)
        status = try f.enumValue("status", ReviewStatus.self)
        pose = try f.value("pose").map { try AnnotationPose(labelSetJSON: $0, context) }
        provenance =
            try f.value("provenance").map { try Provenance(labelSetJSON: $0, context) }
            ?? Provenance()
        try f.finish()
    }

}
