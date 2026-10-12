// LabelSetStore.swift
// A label set on disk: the head files, labels.json, and the journal of
// deltas, written under one writer lock.
//
// Mirrors internal/lidar/annotation/labelset/store.go. A save, holding an
// exclusive non-blocking flock on .lock:
//
//  1. checks that labels.json's head is the revision and state digest the
//     editor loaded, and that the head files still digest to it; otherwise
//     the editor must reload (a conflict);
//  2. computes the delta, and writes nothing if it is empty;
//  3. appends the delta as one line to journal.ndjson and syncs it;
//  4. replaces each head file by writing a temporary file beside it, syncing
//     and renaming;
//  5. replaces labels.json last, which commits the revision.
//
// A save interrupted after step 3 leaves a journal one line ahead of the
// head; opening the set rolls the head forward from the journal. A final line
// without a newline is an interrupted append, never committed, and is dropped.

import Foundation

// MARK: - Errors

enum LabelSetError: Error, Equatable, CustomStringConvertible {
    /// The head moved since it was loaded: reload and reconcile rather than
    /// retrying blindly.
    case conflict(loadedRevision: Int, currentRevision: Int, detail: String?)
    /// Another writer holds the label set's lock.
    case busy
    /// A revision older than the local journal's base was asked for.
    case historyNotRetained(Int)
    /// The files or the edit break the format's rules.
    case invalid(String)
    /// The files could not be read or written.
    case io(String)

    var description: String {
        switch self {
        case .conflict(let loaded, let current, let detail):
            return "label set revision changed from \(loaded) to \(current); reload before saving"
                + (detail.map { " (\($0))" } ?? "")
        case .busy: return "label set writer busy"
        case .historyNotRetained(let revision):
            return "revision \(revision) predates the history this copy retains"
        case .invalid(let detail): return detail
        case .io(let detail): return detail
        }
    }
}

// MARK: - labels.json

/// The revision the head files hold.
struct LabelSetHead: Equatable {
    var revision = 0
    var state = ""
    var updatedUTC = ""

    var json: LabelSetJSON {
        .object([
            "revision": .int(revision), "state": .string(state), "updated_utc": .string(updatedUTC),
        ])
    }

    init(revision: Int = 0, state: String = "", updatedUTC: String = "") {
        self.revision = revision
        self.state = state
        self.updatedUTC = updatedUTC
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        revision = try f.int("revision")
        state = try f.string("state")
        updatedUTC = try f.string("updated_utc")
        try f.finish()
    }
}

/// A capture file verified to hold the label set's packets. This client
/// carries bindings through unchanged; Go's `bind` writes them.
struct LabelSetBinding: Equatable {
    var name = ""
    var sha256 = ""
    var bytes: Int64 = 0
    var udpPort = 0
    var packetsFound = 0
    var packetsMissing = 0
    var windowsComplete = 0
    var windowsPartial = 0
    var windowsMissing = 0
    var verifiedUTC = ""
    var verifiedBuild: String?

    var json: LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.string("name", name)
        b.string("sha256", sha256)
        b.int("bytes", bytes)
        b.int("udp_port", udpPort)
        b.int("packets_found", packetsFound)
        b.int("packets_missing", packetsMissing)
        b.int("windows_complete", windowsComplete)
        b.int("windows_partial", windowsPartial)
        b.int("windows_missing", windowsMissing)
        b.string("verified_utc", verifiedUTC)
        b.omitEmpty("verified_build", verifiedBuild)
        return b.json
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        name = try f.string("name")
        sha256 = try f.string("sha256")
        bytes = try f.int64("bytes")
        udpPort = try f.int("udp_port")
        packetsFound = try f.int("packets_found")
        packetsMissing = try f.int("packets_missing")
        windowsComplete = try f.int("windows_complete")
        windowsPartial = try f.int("windows_partial")
        windowsMissing = try f.int("windows_missing")
        verifiedUTC = try f.string("verified_utc")
        verifiedBuild = try f.optionalString("verified_build")
        try f.finish()
    }
}

/// Where a migrated label set came from.
struct LabelSetMigration: Equatable {
    var packDigest = ""
    var datasetID = ""
    var packDir: String?
    var sidecarRevision = 0
    var pcapSHA256 = ""
    var pcapName = ""
    var decoderBuild: String?
    var mappingDigest = ""
    var note: String?

    var json: LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.string("pack_digest", packDigest)
        b.string("dataset_id", datasetID)
        b.omitEmpty("pack_dir", packDir)
        b.int("sidecar_revision", sidecarRevision)
        b.string("pcap_sha256", pcapSHA256)
        b.string("pcap_name", pcapName)
        b.omitEmpty("decoder_build", decoderBuild)
        b.string("mapping_digest", mappingDigest)
        b.omitEmpty("note", note)
        return b.json
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        packDigest = try f.string("pack_digest")
        datasetID = try f.string("dataset_id")
        packDir = try f.optionalString("pack_dir")
        sidecarRevision = try f.int("sidecar_revision")
        pcapSHA256 = try f.string("pcap_sha256")
        pcapName = try f.string("pcap_name")
        decoderBuild = try f.optionalString("decoder_build")
        mappingDigest = try f.string("mapping_digest")
        note = try f.optionalString("note")
        try f.finish()
    }
}

/// labels.json: identity, layout, bindings and the head. Outside the state
/// digest, so binding a file never makes a revision.
struct LabelSetManifest: Equatable {
    static let schemaVersion = 2

    var schemaVersion = LabelSetManifest.schemaVersion
    var labelSetID = ""
    var name: String?
    var layout = ReturnLayout.pandar40P
    var packetKey = PacketKeys.scheme
    var appliesTo: [LabelSetBinding] = []
    var head = LabelSetHead()
    var migratedFrom: LabelSetMigration?
    var note: String?

    var json: LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.int("schema_version", schemaVersion)
        b.string("label_set_id", labelSetID)
        b.omitEmpty("name", name)
        b.set("layout", layout.json)
        b.string("packet_key", packetKey)
        b.omitEmptyArray("applies_to", appliesTo.map(\.json))
        b.set("head", head.json)
        if let migratedFrom { b.set("migrated_from", migratedFrom.json) }
        b.omitEmpty("note", note)
        return b.json
    }

    init(labelSetID: String = "", name: String? = nil, layout: ReturnLayout = .pandar40P) {
        self.labelSetID = labelSetID
        self.name = name
        self.layout = layout
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        schemaVersion = try f.int("schema_version")
        labelSetID = try f.string("label_set_id")
        name = try f.optionalString("name")
        layout = try ReturnLayout(json: f.value("layout") ?? .object([:]), "\(context) layout")
        packetKey = try f.string("packet_key")
        appliesTo = try f.array("applies_to").map { try LabelSetBinding(json: $0, context) }
        head =
            try f.value("head").map { try LabelSetHead(json: $0, "\(context) head") }
            ?? LabelSetHead()
        migratedFrom = try f.value("migrated_from").map { try LabelSetMigration(json: $0, context) }
        note = try f.optionalString("note")
        try f.finish()
    }

    /// Reads and checks labels.json: the version, key scheme and layout this
    /// build understands, or a refusal.
    static func read(_ data: Data) throws -> LabelSetManifest {
        let m: LabelSetManifest
        do {
            m = try LabelSetManifest(json: LabelSetJSON.parse(data), LabelSetFiles.manifest)
        } catch { throw LabelSetError.invalid("parse \(LabelSetFiles.manifest): \(error)") }
        guard m.schemaVersion == schemaVersion else {
            throw LabelSetError.invalid(
                "label set schema version \(m.schemaVersion), this build reads \(schemaVersion)")
        }
        guard m.packetKey == PacketKeys.scheme else {
            throw LabelSetError.invalid(
                "packet key \"\(m.packetKey)\", this build reads \"\(PacketKeys.scheme)\"")
        }
        do { try m.layout.validate() } catch { throw LabelSetError.invalid("\(error)") }
        return m
    }
}

// MARK: - Store

/// An opened label set. Edit `state`, then `save`.
final class LabelSetStore {
    let directory: URL
    private(set) var manifest: LabelSetManifest
    /// The editor's working state. Saving commits it.
    var state: LabelSetState

    /// The loaded head: its revision, digest and canonical state. The base of
    /// the next save's delta, and the token its conflict check compares.
    private(set) var revision: Int
    private(set) var stateDigest: String
    private(set) var baseState: LabelSetState

    private init(
        directory: URL, manifest: LabelSetManifest, state: LabelSetState, revision: Int,
        stateDigest: String
    ) {
        self.directory = directory
        self.manifest = manifest
        self.state = state
        self.revision = revision
        self.stateDigest = stateDigest
        self.baseState = state
    }

    var labelSetID: String { manifest.labelSetID }
    var layout: ReturnLayout { manifest.layout }

    // MARK: Create and open

    /// Writes a new label set as revision 1. `directory` must not exist or be
    /// empty.
    static func create(
        at directory: URL, manifest: LabelSetManifest, initial: LabelSetState,
        change: LabelSetChange
    ) throws -> LabelSetStore {
        let fm = FileManager.default
        if let entries = try? fm.contentsOfDirectory(atPath: directory.path), !entries.isEmpty {
            throw LabelSetError.invalid(
                "refusing to create a label set in non-empty \(directory.path)")
        }
        do { try fm.createDirectory(at: directory, withIntermediateDirectories: true) } catch {
            throw LabelSetError.io("create \(directory.path): \(error.localizedDescription)")
        }
        var m = manifest
        m.schemaVersion = LabelSetManifest.schemaVersion
        m.packetKey = PacketKeys.scheme
        if m.labelSetID.isEmpty { m.labelSetID = UUID().uuidString.lowercased() }
        let next = initial.canonical
        do { try next.validate(layout: m.layout) } catch {
            throw LabelSetError.invalid("invalid label set: \(error)")
        }
        let empty = LabelSetState().canonical
        let emptyDigest = LabelSetState.digest(of: try encode(empty))
        let store = LabelSetStore(
            directory: directory, manifest: m, state: empty, revision: 0, stateDigest: emptyDigest)
        store.state = next
        let lock = try LabelSetLock(directory: directory)
        defer { lock.release() }
        try store.commit(base: empty, change: change, restoredFrom: 0)
        return store
    }

    /// Reads and verifies a label set. A save interrupted after its journal
    /// line but before its head was replaced is rolled forward from the
    /// journal.
    static func open(_ directory: URL) throws -> LabelSetStore {
        do { return try load(directory) } catch LabelSetLoadOutcome.needsRecovery {
            let lock = try LabelSetLock(directory: directory)
            defer { lock.release() }
            do { try recoverHead(directory) } catch {
                throw LabelSetError.invalid("recover interrupted save: \(error)")
            }
            return try load(directory)
        }
    }

    private enum LabelSetLoadOutcome: Error { case needsRecovery }

    private static func load(_ directory: URL) throws -> LabelSetStore {
        let m = try LabelSetManifest.read(try readFile(directory, LabelSetFiles.manifest))
        let files = try readStateFiles(directory)
        let digest = LabelSetState.digest(of: files)
        let tip = try journalTip(directory)
        if digest != m.head.state
            || (tip.map { $0.revision != m.head.revision || $0.state != m.head.state } ?? false)
        {
            if let tip, tip.revision >= m.head.revision { throw LabelSetLoadOutcome.needsRecovery }
            throw LabelSetError.invalid(
                "head files digest \(digest), manifest says \(m.head.state) at revision \(m.head.revision)"
            )
        }
        let s: LabelSetState
        do { s = try LabelSetState.decode(files: files).canonical } catch {
            throw LabelSetError.invalid("\(error)")
        }
        do { try s.validate(layout: m.layout) } catch {
            throw LabelSetError.invalid("invalid label set: \(error)")
        }
        return LabelSetStore(
            directory: directory, manifest: m, state: s, revision: m.head.revision,
            stateDigest: m.head.state)
    }

    // MARK: Save and restore

    /// Commits the edited state as the next revision. Throws a conflict if
    /// the head moved since this label set was loaded. A save that changes
    /// nothing writes nothing.
    func save(change: LabelSetChange) throws { try save(change: change, restoredFrom: 0) }

    private func save(change: LabelSetChange, restoredFrom: Int) throws {
        let next = state.canonical
        do { try next.validate(layout: manifest.layout) } catch {
            throw LabelSetError.invalid("invalid label set edit: \(error)")
        }
        let lock = try LabelSetLock(directory: directory)
        defer { lock.release() }
        let m = try LabelSetManifest.read(
            try LabelSetStore.readFile(directory, LabelSetFiles.manifest))
        guard m.head.revision == revision, m.head.state == stateDigest else {
            throw LabelSetError.conflict(
                loadedRevision: revision, currentRevision: m.head.revision, detail: nil)
        }
        let files = try LabelSetStore.readStateFiles(directory)
        guard LabelSetState.digest(of: files) == m.head.state else {
            throw LabelSetError.conflict(
                loadedRevision: revision, currentRevision: m.head.revision,
                detail: "head files were changed outside the store")
        }
        // Go decodes the base from the files at this point. The files digest
        // to the loaded state's digest, so they hold the state already in
        // memory, and decoding a large head again on every save would only
        // repeat what the digest has proved.
        manifest = m
        state = next
        try commit(base: baseState, change: change, restoredFrom: restoredFrom)
    }

    /// Makes an earlier revision current again as a new revision, rebuilt by
    /// replaying the journal. Earlier lines are not rewritten, and restoring
    /// never promotes a proposal: it recreates what that revision held.
    func restore(revision old: Int, change: LabelSetChange) throws {
        let rebuilt = try LabelSetStore.loadRevision(directory, revision: old)
        state = rebuilt
        try save(change: change, restoredFrom: old)
    }

    /// Appends the delta from `base` to `state` and replaces the head. The
    /// caller holds the lock.
    private func commit(base: LabelSetState, change: LabelSetChange, restoredFrom: Int) throws {
        let next = state.canonical
        var delta: LabelSetDelta
        do { delta = try LabelSetDelta.diff(from: base, to: next) } catch {
            throw LabelSetError.invalid("\(error)")
        }
        if delta.isEmpty && revision > 0 && restoredFrom == 0 {
            state = next
            return
        }
        let files = try LabelSetStore.encode(next)
        for (name, bytes) in files where bytes.count > LabelSetFiles.maxBytes {
            throw LabelSetError.invalid(
                "\(name) would be \(bytes.count) bytes, above the \(LabelSetFiles.maxBytes) limit")
        }
        let now = LabelSetStore.utcNow()
        var stamped = change
        stamped.createdUTC = now
        if restoredFrom > 0 {
            stamped.operation = "restore"
        } else if stamped.operation.isEmpty || stamped.operation == "restore" {
            stamped.operation = "save"
        }
        delta.revision = revision + 1
        delta.parentRevision = revision
        delta.parentState = stateDigest
        delta.state = LabelSetState.digest(of: files)
        delta.change = stamped
        delta.restoredFrom = restoredFrom

        try LabelSetStore.ensureJournal(directory, revision: revision, state: stateDigest)
        let line: Data
        do { line = try delta.json().canonicalLine } catch {
            throw LabelSetError.invalid("\(error)")
        }
        guard line.count <= LabelSetFiles.maxBytes else {
            throw LabelSetError.invalid(
                "journal line would be \(line.count) bytes, above the \(LabelSetFiles.maxBytes) limit"
            )
        }
        do { try LabelSetStore.appendJournal(directory, line) } catch {
            throw LabelSetError.io("append journal: \(error)")
        }
        for name in LabelSetFiles.state {
            do { try LabelSetStore.writeAtomic(directory, name, files[name] ?? Data()) } catch {
                throw LabelSetError.io("replace \(name) (reopen to recover): \(error)")
            }
        }
        var m = manifest
        m.head = LabelSetHead(revision: delta.revision, state: delta.state, updatedUTC: now)
        do {
            try LabelSetStore.writeAtomic(directory, LabelSetFiles.manifest, m.json.indentedData)
        } catch {
            throw LabelSetError.io(
                "replace \(LabelSetFiles.manifest) (reopen to recover): \(error)")
        }
        manifest = m
        revision = delta.revision
        stateDigest = delta.state
        baseState = next
        state = next
    }

    /// Gives a label set without a journal (one published, or copied as its
    /// head alone) a base to grow from: a copy of the head in journal-base/
    /// and a base line naming it.
    private static func ensureJournal(_ directory: URL, revision: Int, state: String) throws {
        if FileManager.default.fileExists(
            atPath: directory.appendingPathComponent(LabelSetFiles.journal).path)
        {
            return
        }
        if revision == 0 { return }  // a new set's first line is its creation
        let files = try readStateFiles(directory)
        guard LabelSetState.digest(of: files) == state else {
            throw LabelSetError.invalid("head does not match revision \(revision)'s state")
        }
        let base = directory.appendingPathComponent(LabelSetFiles.journalBase)
        do {
            try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
        } catch { throw LabelSetError.io("create \(LabelSetFiles.journalBase): \(error)") }
        for name in LabelSetFiles.state { try writeAtomic(base, name, files[name] ?? Data()) }
        var d = LabelSetDelta()
        d.revision = revision
        d.parentRevision = revision
        d.parentState = state
        d.state = state
        d.base = true
        d.change = LabelSetChange(author: "store", operation: "journal_base", createdUTC: utcNow())
        try appendJournal(directory, try d.json().canonicalLine)
    }

    // MARK: History

    /// Rebuilds a revision from the journal and checks it against the digest
    /// recorded when it was saved.
    static func loadRevision(_ directory: URL, revision wanted: Int) throws -> LabelSetState {
        var out: LabelSetState?
        try replay(directory) { rev, state, delta in
            if rev == wanted {
                let got = try state.digest()
                guard got == delta.state else {
                    throw LabelSetError.invalid(
                        "revision \(rev) rebuilds to \(got), journal says \(delta.state)")
                }
                out = state
                return false
            }
            return rev < wanted
        }
        guard let out else { throw LabelSetError.historyNotRetained(wanted) }
        return out
    }

    /// Applies the journal in order, calling `body` after each revision with
    /// the state it produces. `body` returns false to stop.
    static func replay(
        _ directory: URL, _ body: (Int, LabelSetState, LabelSetDelta) throws -> Bool
    ) throws {
        let journalURL = directory.appendingPathComponent(LabelSetFiles.journal)
        guard FileManager.default.fileExists(atPath: journalURL.path) else {
            throw LabelSetError.historyNotRetained(0)
        }
        let data = try readFile(directory, LabelSetFiles.journal)
        var state = LabelSetState().canonical
        var prevRevision = 0
        var prevState = (try? state.digest()) ?? ""
        var n = 0
        var start = data.startIndex
        while start < data.endIndex {
            // An unterminated final line is an interrupted append, never
            // committed.
            guard let end = data[start...].firstIndex(of: 0x0A) else { return }
            n += 1
            let d: LabelSetDelta
            do {
                d = try LabelSetDelta(json: LabelSetJSON.parse(Data(data[start..<end])), "journal")
            } catch { throw LabelSetError.invalid("journal line \(n): \(error)") }
            start = data.index(after: end)
            if d.base {
                let base = try readStateDirectory(
                    directory.appendingPathComponent(LabelSetFiles.journalBase))
                guard (try? base.digest()) == d.state else {
                    throw LabelSetError.invalid("journal base does not digest to \(d.state)")
                }
                state = base
                prevRevision = d.revision
                prevState = d.state
                if try !body(d.revision, state, d) { return }
                continue
            }
            guard d.parentRevision == prevRevision, d.parentState == prevState,
                d.revision == prevRevision + 1
            else {
                throw LabelSetError.invalid(
                    "journal line \(n) (revision \(d.revision)) does not follow revision \(prevRevision)"
                )
            }
            do { state = try d.apply(to: state) } catch {
                throw LabelSetError.invalid("journal revision \(d.revision): \(error)")
            }
            prevRevision = d.revision
            prevState = d.state
            if try !body(d.revision, state, d) { return }
        }
    }

    /// Replays the whole journal, checking every revision's digest and that
    /// the last one is the head. Returns how many revisions it checked.
    @discardableResult static func verify(_ directory: URL) throws -> Int {
        let opened = try open(directory)
        var count = 0
        var last = 0
        do {
            try replay(directory) { rev, state, delta in
                let got = try state.digest()
                guard got == delta.state else {
                    throw LabelSetError.invalid(
                        "revision \(rev) rebuilds to \(got), journal says \(delta.state)")
                }
                last = rev
                count += 1
                return true
            }
        } catch LabelSetError.historyNotRetained { return 0 }
        if last != 0, last != opened.revision {
            throw LabelSetError.invalid(
                "journal ends at revision \(last), head is \(opened.revision)")
        }
        return count
    }

    /// The journal's lines without their changes applied, oldest first: what
    /// a history list shows.
    ///
    /// Only each line's revision, change, restore and base fields are read:
    /// listing a journal of thousands of saves should not cost what
    /// replaying it does. Replay is what checks the lines.
    static func history(_ directory: URL) throws -> [LabelSetDelta] {
        let url = directory.appendingPathComponent(LabelSetFiles.journal)
        guard FileManager.default.fileExists(atPath: url.path) else { return [] }
        let data = try readFile(directory, LabelSetFiles.journal)
        var out: [LabelSetDelta] = []
        var start = data.startIndex
        var n = 0
        while start < data.endIndex, let end = data[start...].firstIndex(of: 0x0A) {
            n += 1
            guard
                let object = try? JSONSerialization.jsonObject(with: Data(data[start..<end]))
                    as? [String: Any], let revision = (object["revision"] as? NSNumber)?.intValue
            else { throw LabelSetError.invalid("journal line \(n) is not a journal line") }
            let change = object["change"] as? [String: Any] ?? [:]
            var d = LabelSetDelta()
            d.revision = revision
            d.parentRevision = (object["parent_revision"] as? NSNumber)?.intValue ?? 0
            d.state = object["state"] as? String ?? ""
            d.restoredFrom = (object["restored_from"] as? NSNumber)?.intValue ?? 0
            d.base = object["base"] as? Bool ?? false
            d.change = LabelSetChange(
                author: change["author"] as? String ?? "", session: change["session"] as? String,
                operation: change["operation"] as? String ?? "",
                createdUTC: change["created_utc"] as? String ?? "")
            out.append(d)
            start = data.index(after: end)
        }
        return out
    }

    /// Rebuilds the head from the journal after an interrupted save.
    private static func recoverHead(_ directory: URL) throws {
        var m = try LabelSetManifest.read(try readFile(directory, LabelSetFiles.manifest))
        guard let tip = try journalTip(directory) else {
            throw LabelSetError.invalid("no journal to recover from")
        }
        var rebuilt: LabelSetState?
        try replay(directory) { rev, state, _ in
            if rev == tip.revision { rebuilt = state }
            return true
        }
        if rebuilt == nil {
            guard tip.base, tip.revision == m.head.revision else {
                throw LabelSetError.invalid("journal does not rebuild revision \(tip.revision)")
            }
            rebuilt = try readStateDirectory(
                directory.appendingPathComponent(LabelSetFiles.journalBase))
        }
        let files = try encode(rebuilt!)
        guard LabelSetState.digest(of: files) == tip.state else {
            throw LabelSetError.invalid(
                "journal rebuilds revision \(tip.revision) to a different digest")
        }
        for name in LabelSetFiles.state { try writeAtomic(directory, name, files[name] ?? Data()) }
        m.head = LabelSetHead(revision: tip.revision, state: tip.state, updatedUTC: tip.createdUTC)
        try writeAtomic(directory, LabelSetFiles.manifest, m.json.indentedData)
    }

    /// What opening needs of the last complete journal line. A final line
    /// without a newline was never committed and is ignored.
    struct JournalTip {
        var revision: Int
        var state: String
        var base: Bool
        var createdUTC: String
    }

    /// Reads only the tip's revision, state, base flag and time. The line
    /// can be a whole migration's worth of masks, and opening needs four
    /// fields of it; replay, which applies it, reads it strictly.
    private static func journalTip(_ directory: URL) throws -> JournalTip? {
        let url = directory.appendingPathComponent(LabelSetFiles.journal)
        guard FileManager.default.fileExists(atPath: url.path) else { return nil }
        let data = try readFile(directory, LabelSetFiles.journal)
        guard let lastNewline = data.lastIndex(of: 0x0A) else { return nil }
        let before = data[data.startIndex..<lastNewline]
        let start = before.lastIndex(of: 0x0A).map { data.index(after: $0) } ?? data.startIndex
        let line = data[start..<lastNewline]
        guard !line.isEmpty else { return nil }
        guard let object = try? JSONSerialization.jsonObject(with: Data(line)) as? [String: Any],
            let revision = (object["revision"] as? NSNumber)?.intValue,
            let state = object["state"] as? String
        else { throw LabelSetError.invalid("journal tip is not a journal line") }
        let change = object["change"] as? [String: Any]
        return JournalTip(
            revision: revision, state: state, base: (object["base"] as? Bool) ?? false,
            createdUTC: (change?["created_utc"] as? String) ?? "")
    }

    // MARK: Files

    static func encode(_ state: LabelSetState) throws -> [String: Data] {
        do { return try state.encodedFiles() } catch { throw LabelSetError.invalid("\(error)") }
    }

    private static func readStateFiles(_ directory: URL) throws -> [String: Data] {
        var files: [String: Data] = [:]
        for name in LabelSetFiles.state { files[name] = try readFile(directory, name) }
        return files
    }

    private static func readStateDirectory(_ directory: URL) throws -> LabelSetState {
        do {
            return try LabelSetState.decode(files: try readStateFiles(directory)).canonical
        } catch let error as LabelSetError { throw error } catch {
            throw LabelSetError.invalid("\(error)")
        }
    }

    /// Reads a regular file of at most the size bound. A symlink or a
    /// directory in a label set is refused rather than followed.
    static func readFile(_ directory: URL, _ name: String) throws -> Data {
        let url = directory.appendingPathComponent(name)
        var info = stat()
        guard lstat(url.path, &info) == 0 else {
            throw LabelSetError.io("read \(name): \(String(cString: strerror(errno)))")
        }
        guard (info.st_mode & S_IFMT) == S_IFREG else {
            throw LabelSetError.invalid("\(name) is not a regular file")
        }
        guard info.st_size <= LabelSetFiles.maxBytes else {
            throw LabelSetError.invalid("\(name) exceeds \(LabelSetFiles.maxBytes) bytes")
        }
        do { return try Data(contentsOf: url) } catch {
            throw LabelSetError.io("read \(name): \(error.localizedDescription)")
        }
    }

    /// Writes beside the destination, syncs, renames over it and syncs the
    /// directory.
    static func writeAtomic(_ directory: URL, _ name: String, _ bytes: Data) throws {
        let destination = directory.appendingPathComponent(name)
        let temp = directory.appendingPathComponent("\(name).\(UUID().uuidString.lowercased()).tmp")
        let fd = Darwin.open(temp.path, O_CREAT | O_EXCL | O_WRONLY, 0o644)
        guard fd >= 0 else {
            throw LabelSetError.io(
                "create \(temp.lastPathComponent): \(String(cString: strerror(errno)))")
        }
        var failure: String?
        bytes.withUnsafeBytes { buffer in
            var offset = 0
            while offset < buffer.count {
                let n = Darwin.write(fd, buffer.baseAddress! + offset, buffer.count - offset)
                if n < 0 {
                    if errno == EINTR { continue }
                    failure = String(cString: strerror(errno))
                    return
                }
                offset += n
            }
        }
        if failure == nil, fsync(fd) != 0 { failure = String(cString: strerror(errno)) }
        if Darwin.close(fd) != 0, failure == nil { failure = String(cString: strerror(errno)) }
        if failure == nil, rename(temp.path, destination.path) != 0 {
            failure = String(cString: strerror(errno))
        }
        if let failure {
            unlink(temp.path)
            throw LabelSetError.io("write \(name): \(failure)")
        }
        syncDirectory(destination.deletingLastPathComponent())
    }

    /// Appends one line and syncs it. An interrupted final line is dropped
    /// first, so it cannot be read as the start of this one.
    static func appendJournal(_ directory: URL, _ line: Data) throws {
        let url = directory.appendingPathComponent(LabelSetFiles.journal)
        // Only the last byte says whether the journal ends cleanly, so only
        // it is read; the whole journal is read only to repair a torn line.
        if FileManager.default.fileExists(atPath: url.path), lastByte(of: url) != 0x0A,
            let existing = try? readFile(directory, LabelSetFiles.journal), !existing.isEmpty
        {
            let keep = existing.lastIndex(of: 0x0A).map { $0 - existing.startIndex + 1 } ?? 0
            guard truncate(url.path, off_t(keep)) == 0 else {
                throw LabelSetError.io("truncate journal: \(String(cString: strerror(errno)))")
            }
        }
        let fd = Darwin.open(url.path, O_CREAT | O_WRONLY | O_APPEND, 0o644)
        guard fd >= 0 else {
            throw LabelSetError.io("open journal: \(String(cString: strerror(errno)))")
        }
        var failure: String?
        line.withUnsafeBytes { buffer in
            var offset = 0
            while offset < buffer.count {
                let n = Darwin.write(fd, buffer.baseAddress! + offset, buffer.count - offset)
                if n < 0 {
                    if errno == EINTR { continue }
                    failure = String(cString: strerror(errno))
                    return
                }
                offset += n
            }
        }
        if failure == nil, fsync(fd) != 0 { failure = String(cString: strerror(errno)) }
        Darwin.close(fd)
        if let failure { throw LabelSetError.io("append journal: \(failure)") }
        syncDirectory(directory)
    }

    /// The file's last byte, or nil for an empty or unreadable file.
    private static func lastByte(of url: URL) -> UInt8? {
        guard let handle = try? FileHandle(forReadingFrom: url) else { return nil }
        defer { try? handle.close() }
        guard let size = try? handle.seekToEnd(), size > 0 else { return nil }
        try? handle.seek(toOffset: size - 1)
        return (try? handle.read(upToCount: 1))?.first
    }

    private static func syncDirectory(_ directory: URL) {
        let fd = Darwin.open(directory.path, O_RDONLY)
        if fd >= 0 {
            _ = fsync(fd)
            Darwin.close(fd)
        }
    }

    static func utcNow() -> String {
        let formatter = ISO8601DateFormatter()
        formatter.timeZone = TimeZone(identifier: "UTC")
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.string(from: Date())
    }
}

/// A non-blocking kernel lock on a label set's .lock file.
///
/// The kernel releases it when the descriptor closes or the process exits, so
/// a crashed writer does not leave the set locked. The file is never deleted:
/// removing it while another writer held it would let two writers each hold a
/// lock on a different inode.
final class LabelSetLock {
    private let fd: Int32
    private var held = true

    /// Takes the lock, or throws `.busy` at once rather than waiting.
    init(directory: URL) throws {
        let path = directory.appendingPathComponent(LabelSetFiles.lock).path
        fd = Darwin.open(path, O_CREAT | O_RDWR, 0o600)
        guard fd >= 0 else {
            throw LabelSetError.io("open label set lock: \(String(cString: strerror(errno)))")
        }
        guard flock(fd, LOCK_EX | LOCK_NB) == 0 else {
            Darwin.close(fd)
            throw LabelSetError.busy
        }
    }

    func release() {
        guard held else { return }
        held = false
        _ = flock(fd, LOCK_UN)
        Darwin.close(fd)
    }

    deinit { release() }
}
