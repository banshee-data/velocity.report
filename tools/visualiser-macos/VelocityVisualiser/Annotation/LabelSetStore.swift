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
    /// The lowest reader level that reads this set (`ReaderLevel`); 0 when
    /// absent, which is level 1. Kept as read, so a save never lowers it.
    var minReader = 0
    var labelSetID = ""
    var name: String?
    var layout = ReturnLayout.pandar40P
    var packetKey = PacketKeys.scheme
    var appliesTo: [LabelSetBinding] = []
    var head = LabelSetHead()
    /// How the head is laid out on disk. Nil only in a set written before
    /// the storage layout, which `read` refuses.
    var storage: LabelSetStorage? = LabelSetStorage()
    var migratedFrom: LabelSetMigration?
    var note: String?

    var json: LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.int("schema_version", schemaVersion)
        b.omitZero("min_reader", minReader)
        b.string("label_set_id", labelSetID)
        b.omitEmpty("name", name)
        b.set("layout", layout.json)
        b.string("packet_key", packetKey)
        b.omitEmptyArray("applies_to", appliesTo.map(\.json))
        b.set("head", head.json)
        if let storage { b.set("storage", storage.json) }
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
        minReader = try f.int("min_reader")
        labelSetID = try f.string("label_set_id")
        name = try f.optionalString("name")
        layout = try ReturnLayout(json: f.value("layout") ?? .object([:]), "\(context) layout")
        packetKey = try f.string("packet_key")
        appliesTo = try f.array("applies_to").map { try LabelSetBinding(json: $0, context) }
        head =
            try f.value("head").map { try LabelSetHead(json: $0, "\(context) head") }
            ?? LabelSetHead()
        storage = try f.value("storage").map { try LabelSetStorage(json: $0, "\(context) storage") }
        migratedFrom = try f.value("migrated_from").map { try LabelSetMigration(json: $0, context) }
        note = try f.optionalString("note")
        try f.finish()
    }

    /// The storage layout `read` has checked is there.
    var layoutOnDisk: LabelSetStorage { storage ?? LabelSetStorage() }

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
        if let refusal = ReaderLevel.refusal(LabelSetFiles.manifest, minReader: m.minReader) {
            throw LabelSetError.invalid(refusal)
        }
        guard m.packetKey == PacketKeys.scheme else {
            throw LabelSetError.invalid(
                "packet key \"\(m.packetKey)\", this build reads \"\(PacketKeys.scheme)\"")
        }
        do { try m.layout.validate() } catch { throw LabelSetError.invalid("\(error)") }
        guard let storage = m.storage else {
            throw LabelSetError.invalid(
                "label set is in the single-file layout: convert it with 'velocity lidar labels relayout'"
            )
        }
        do { try storage.validate() } catch { throw LabelSetError.invalid("\(error)") }
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
        if m.storage == nil { m.storage = LabelSetStorage() }
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
        try store.commit(base: empty, change: change, restoredFrom: 0, head: nil)
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
        let tip = try journalTip(directory)
        // Only a journal one line ahead of labels.json, with the commit's
        // marker still in place, is an interrupted save. Head files that
        // disagree with labels.json at the same revision were changed outside
        // the store, by a merge or by hand, and are never overwritten.
        let interrupted = try journalState(directory, m, tip)
        let have: [String: Data]
        let s: LabelSetState
        do {
            have = try readStoredFiles(directory, m.layoutOnDisk)
            s = try LabelSetState.decodeStored(files: have).canonical
        } catch {
            if interrupted { throw LabelSetLoadOutcome.needsRecovery }
            if let error = error as? LabelSetError { throw error }
            throw LabelSetError.invalid("\(error)")
        }
        if interrupted { throw LabelSetLoadOutcome.needsRecovery }
        let digest: String
        do { digest = try s.digest() } catch { throw LabelSetError.invalid("\(error)") }
        if digest != m.head.state
            || (tip.map { $0.revision != m.head.revision || $0.state != m.head.state } ?? false)
        {
            throw LabelSetError.invalid(
                "head files digest \(digest), manifest says \(m.head.state) at revision \(m.head.revision) "
                    + "(after a merge, 'velocity lidar labels rehead' makes the merged files the head)"
            )
        }
        guard (try? s.storedFiles(m.layoutOnDisk)) == have else {
            throw LabelSetError.invalid(
                "the head files are not in the label set's layout (rewrite them with 'velocity lidar labels rehead')"
            )
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
        let have = try LabelSetStore.readStoredFiles(directory, m.layoutOnDisk)
        // Go decodes the base from the files at this point. Files that are
        // byte for byte the loaded state's layout hold that state, so the
        // state already in memory is the base without decoding a large head
        // again on every save.
        guard (try? baseState.storedFiles(m.layoutOnDisk)) == have else {
            throw LabelSetError.conflict(
                loadedRevision: revision, currentRevision: m.head.revision,
                detail: "head files were changed outside the store")
        }
        manifest = m
        state = next
        try commit(base: baseState, change: change, restoredFrom: restoredFrom, head: have)
    }

    /// Makes an earlier revision current again as a new revision, rebuilt by
    /// replaying the journal. Earlier lines are not rewritten, and restoring
    /// never promotes a proposal: it recreates what that revision held.
    func restore(revision old: Int, change: LabelSetChange) throws {
        let rebuilt = try LabelSetStore.loadRevision(directory, revision: old)
        state = rebuilt
        try save(change: change, restoredFrom: old)
    }

    /// Appends the delta from `base` to `state` and replaces the head; `head`
    /// is the head as read, so files it already holds are not rewritten. The
    /// caller holds the lock.
    private func commit(
        base: LabelSetState, change: LabelSetChange, restoredFrom: Int, head: [String: Data]?
    ) throws {
        let next = state.canonical
        var delta: LabelSetDelta
        do { delta = try LabelSetDelta.diff(from: base, to: next) } catch {
            throw LabelSetError.invalid("\(error)")
        }
        if delta.isEmpty && revision > 0 && restoredFrom == 0 {
            state = next
            return
        }
        let files: [String: Data]
        let nextDigest: String
        do {
            files = try next.storedFiles(manifest.layoutOnDisk)
            nextDigest = try next.digest()
        } catch { throw LabelSetError.invalid("\(error)") }
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
        delta.state = nextDigest
        delta.change = stamped
        delta.restoredFrom = restoredFrom

        try LabelSetStore.writePending(
            directory, revision: delta.revision, parentState: stateDigest)
        try LabelSetStore.ensureJournal(directory, revision: revision, head: head)
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
        try LabelSetStore.writeHead(directory, files, have: head)
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
        try LabelSetStore.clearPending(directory)
    }

    /// Gives a label set without a journal (one published, or copied as its
    /// head alone) a base to grow from: a copy of the head in journal-base/
    /// and a base line naming it.
    /// `head` is the head as read, which `save` has proved is the loaded
    /// state's layout.
    private static func ensureJournal(_ directory: URL, revision: Int, head: [String: Data]?) throws
    {
        if FileManager.default.fileExists(
            atPath: directory.appendingPathComponent(LabelSetFiles.journal).path)
        {
            return
        }
        if revision == 0 { return }  // a new set's first line is its creation
        guard let head else { throw LabelSetError.invalid("no head to base the journal on") }
        let state: String
        do { state = try LabelSetState.decodeStored(files: head).canonical.digest() } catch {
            throw LabelSetError.invalid("\(error)")
        }
        let base = directory.appendingPathComponent(LabelSetFiles.journalBase)
        do {
            try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
        } catch { throw LabelSetError.io("create \(LabelSetFiles.journalBase): \(error)") }
        try writeHead(base, head, have: nil)
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
        let storage = try LabelSetManifest.read(try readFile(directory, LabelSetFiles.manifest))
            .layoutOnDisk
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
                let base = try readStoredState(
                    directory.appendingPathComponent(LabelSetFiles.journalBase), storage)
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
            rebuilt = try readStoredState(
                directory.appendingPathComponent(LabelSetFiles.journalBase), m.layoutOnDisk)
        }
        let files: [String: Data]
        do {
            guard try rebuilt!.digest() == tip.state else {
                throw LabelSetError.invalid(
                    "journal rebuilds revision \(tip.revision) to a different digest")
            }
            files = try rebuilt!.storedFiles(m.layoutOnDisk)
        } catch let error as LabelSetError { throw error } catch {
            throw LabelSetError.invalid("\(error)")
        }
        try writeHead(directory, files, have: nil)
        m.head = LabelSetHead(revision: tip.revision, state: tip.state, updatedUTC: tip.createdUTC)
        try writeAtomic(directory, LabelSetFiles.manifest, m.json.indentedData)
        try clearPending(directory)
    }

    /// What opening needs of the last complete journal line. A final line
    /// without a newline was never committed and is ignored.
    struct JournalTip {
        var revision: Int
        var state: String
        var base: Bool
        var createdUTC: String
        var parentRevision = 0
        var parentState = ""
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
            createdUTC: (change?["created_utc"] as? String) ?? "",
            parentRevision: (object["parent_revision"] as? NSNumber)?.intValue ?? 0,
            parentState: (object["parent_state"] as? String) ?? "")
    }

    /// Whether the local journal is an interrupted save (true), at the head
    /// (false), or another head's, which is refused: in a Git working tree
    /// most often another branch's, since the journal is local and outlives
    /// a checkout. Go's `journalState`.
    private static func journalState(
        _ directory: URL, _ m: LabelSetManifest, _ tip: JournalTip?
    ) throws -> Bool {
        guard let tip else { return false }
        if let pending = try readPending(directory), tip.revision == pending.revision,
            tip.revision == m.head.revision + 1, tip.parentRevision == m.head.revision,
            tip.parentState == m.head.state
        {
            return true
        }
        if tip.revision != m.head.revision || tip.state != m.head.state {
            throw LabelSetError.invalid(
                "the local journal does not continue this head: it ends at revision \(tip.revision) and "
                    + "labels.json names revision \(m.head.revision). It records edits to another head, in a Git "
                    + "working tree most likely another branch's: switch back to that branch, or move "
                    + "\(LabelSetFiles.journal) and \(LabelSetFiles.journalBase) aside to edit this one"
            )
        }
        return false
    }

    /// The commit marker: the revision being made and its parent's state.
    private static func readPending(
        _ directory: URL
    ) throws -> (revision: Int, parentState: String)? {
        let url = directory.appendingPathComponent(LabelSetFiles.pending)
        guard FileManager.default.fileExists(atPath: url.path) else { return nil }
        do {
            var f = try LabelSetFields(
                LabelSetJSON.parse(try readFile(directory, LabelSetFiles.pending)),
                LabelSetFiles.pending)
            let pending = (try f.int("revision"), try f.string("parent_state"))
            try f.finish()
            return pending
        } catch let error as LabelSetError { throw error } catch {
            throw LabelSetError.invalid("parse \(LabelSetFiles.pending): \(error)")
        }
    }

    private static func writePending(_ directory: URL, revision: Int, parentState: String) throws {
        let line = LabelSetJSON.object([
            "parent_state": .string(parentState), "revision": .int(revision),
        ]).canonicalLine
        try writeAtomic(directory, LabelSetFiles.pending, line)
    }

    private static func clearPending(_ directory: URL) throws {
        let url = directory.appendingPathComponent(LabelSetFiles.pending)
        guard FileManager.default.fileExists(atPath: url.path) else { return }
        do { try FileManager.default.removeItem(at: url) } catch {
            throw LabelSetError.io("remove \(LabelSetFiles.pending): \(error.localizedDescription)")
        }
    }

    // MARK: Files

    static func encode(_ state: LabelSetState) throws -> [String: Data] {
        do { return try state.encodedFiles() } catch { throw LabelSetError.invalid("\(error)") }
    }

    /// Reads the head kept under `directory`, keyed as `storedFiles` keys
    /// it. Hidden files and interrupted writes in the chunk directories are
    /// passed over; any other stray file, a chunk of another span, and the
    /// old single files are refused.
    static func readStoredFiles(
        _ directory: URL, _ storage: LabelSetStorage
    ) throws -> [String: Data] {
        for old in [LabelSetFiles.masks, LabelSetFiles.domainReturns]
        where FileManager.default.fileExists(atPath: directory.appendingPathComponent(old).path) {
            throw LabelSetError.invalid(
                "\(directory.lastPathComponent) holds \(old), a file of the single-file layout, beside the chunked head"
            )
        }
        var files: [String: Data] = [:]
        for name in LabelSetFiles.whole { files[name] = try readFile(directory, name) }
        for chunkDirectory in LabelSetFiles.chunkDirectories {
            for name in try chunkFiles(directory.appendingPathComponent(chunkDirectory)) {
                guard let span = LabelSetStorage.span(of: name),
                    span.first % storage.windowChunk == 0,
                    span.last == span.first + storage.windowChunk - 1
                else {
                    throw LabelSetError.invalid(
                        "\(chunkDirectory)/\(name) does not span \(storage.windowChunk) windows from a multiple of \(storage.windowChunk)"
                    )
                }
                let key = chunkDirectory + "/" + name
                files[key] = try readFile(directory, key)
            }
        }
        return files
    }

    /// A chunk directory's file names, sorted; a missing directory is empty.
    private static func chunkFiles(_ directory: URL) throws -> [String] {
        guard FileManager.default.fileExists(atPath: directory.path) else { return [] }
        let names: [String]
        do { names = try FileManager.default.contentsOfDirectory(atPath: directory.path) } catch {
            throw LabelSetError.io(
                "list \(directory.lastPathComponent): \(error.localizedDescription)")
        }
        var out: [String] = []
        for name in names.sorted() where !name.hasPrefix(".") && !name.hasSuffix(".tmp") {
            guard LabelSetStorage.span(of: name) != nil else {
                throw LabelSetError.invalid(
                    "\(directory.lastPathComponent)/\(name) is not a chunk file")
            }
            out.append(name)
        }
        return out
    }

    private static func readStoredState(
        _ directory: URL, _ storage: LabelSetStorage
    ) throws -> LabelSetState {
        do {
            return try LabelSetState.decodeStored(files: try readStoredFiles(directory, storage))
                .canonical
        } catch let error as LabelSetError { throw error } catch {
            throw LabelSetError.invalid("\(error)")
        }
    }

    /// Replaces the head under `directory` with `files`: a file whose bytes
    /// equal `have`'s is left alone, and a chunk file no longer in `files` is
    /// removed. Go's `writeHead`.
    static func writeHead(_ directory: URL, _ files: [String: Data], have: [String: Data]?) throws {
        for chunkDirectory in LabelSetFiles.chunkDirectories {
            do {
                try FileManager.default.createDirectory(
                    at: directory.appendingPathComponent(chunkDirectory),
                    withIntermediateDirectories: true)
            } catch { throw LabelSetError.io("create \(chunkDirectory): \(error)") }
        }
        for name in files.keys.sorted() where have?[name] != files[name] {
            do { try writeAtomic(directory, name, files[name]!) } catch {
                throw LabelSetError.io("replace \(name) (reopen to recover): \(error)")
            }
        }
        for chunkDirectory in LabelSetFiles.chunkDirectories {
            for name in try chunkFiles(directory.appendingPathComponent(chunkDirectory))
            where files[chunkDirectory + "/" + name] == nil {
                do {
                    try FileManager.default.removeItem(
                        at: directory.appendingPathComponent(chunkDirectory + "/" + name))
                } catch {
                    throw LabelSetError.io(
                        "remove \(chunkDirectory)/\(name) (reopen to recover): \(error)")
                }
            }
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
