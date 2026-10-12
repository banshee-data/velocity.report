// LabelSetMembership.swift
// What the annotation window reads and saves membership through.
//
// The window works in one pack's terms: masks by sample id and point index,
// the Sidecar shape it has always edited. Where those masks are kept depends
// on the pack:
//
//  - A version 2 pack is a view of a label set. Its membership is the label
//    set projected onto the pack's samples, and a save folds the edited
//    projection back as a delta: only the masks that changed, each through
//    the view map so members this view does not show survive. The label set
//    lives at <annotation dir>/labels/<name>/ and the pack points to it with
//    label-set-link.json. Until the first save there is no label set: the
//    first save creates one from the pack.
//  - A version 1 pack's labels are bound to one run's export. Its
//    annotations.json is shown as it stands, and nothing is saved: the
//    migration carries it into a label set.

import Foundation

/// One loaded or saved revision of the window's membership.
struct MembershipDocument {
    /// The pack's projection: masks by sample id and point index.
    var sidecar: Sidecar
    /// The token a physical reference or a feature pins: the label set's
    /// state digest, or for a version 1 pack the digest of annotations.json's
    /// bytes. Empty before anything is saved.
    let loadedDigest: String
    let revision: Int
}

/// Why membership could not be loaded or saved.
enum MembershipStoreError: Error, Equatable {
    /// Another writer holds the lock. Retry after it completes.
    case busy
    /// The label set advanced underneath this edit: reload and reconcile.
    /// The operator's unsaved selection is never discarded by this error.
    case conflict(loadedRevision: Int, currentRevision: Int)
    case malformed(String)
    case unwritable(String)
    /// The pack cannot be saved to: a version 1 pack, or a label set folder
    /// the app has not been given.
    case readOnly(String)
    /// The edit is one the label set cannot hold.
    case refused(String)
}

/// One revision in the label set's journal, for a history list.
struct MembershipRevision: Equatable, Identifiable {
    var revision: Int
    var author: String
    var operation: String
    var createdUTC: String
    var restoredFrom: Int?

    var id: Int { revision }
}

/// The storage behind an annotation session.
protocol MembershipStore: AnyObject {
    /// Why nothing can be saved, or nil when saving works.
    var readOnlyReason: String? { get }
    /// One line saying where membership is kept, for the window.
    var summary: String { get }
    /// Reads the current revision from disk.
    func load() throws -> MembershipDocument
    /// Saves `sidecar`, an edit of `base`, as the next revision. Writes
    /// nothing when nothing changed.
    func save(
        _ sidecar: Sidecar, over base: MembershipDocument, change: Provenance
    ) throws -> MembershipDocument
    /// Makes an earlier revision current again as a new revision.
    func restore(revision: Int, change: Provenance) throws -> MembershipDocument
    /// The saved revisions, oldest first.
    func revisions() -> [MembershipRevision]
}

extension MembershipStore { var isReadOnly: Bool { readOnlyReason != nil } }

/// Picks the store a pack's membership lives in.
enum MembershipStores {
    static func forPack(_ pack: AnnotationPack) -> any MembershipStore {
        pack.hasReturns ? LabelSetMembership(pack: pack) : LegacySidecarReader(pack: pack)
    }

    /// The command that moves a version 1 pack's labels into a label set.
    static func migrateCommand(packDirectory: URL) -> String {
        "velocity lidar labels migrate --pack \(packDirectory.path) --pcap <file>"
    }
}

// MARK: - Version 1: read only

/// Shows a version 1 pack's annotations.json as it stands. Saves are refused:
/// those labels cite this run's sample and point numbers, and a new revision
/// of them would be more work bound to one export. The migration moves them.
final class LegacySidecarReader: MembershipStore {
    let pack: AnnotationPack
    /// The sidecar's ceiling, as the Go store has it.
    static let maxBytes = 64 << 20

    init(pack: AnnotationPack) { self.pack = pack }

    var readOnlyReason: String? {
        "Legacy pack (version 1): read only. Its labels cite this run's points, not the capture's "
            + "returns. Migrate it with "
            + "`\(MembershipStores.migrateCommand(packDirectory: pack.directory))`."
    }

    var summary: String {
        FileManager.default.fileExists(atPath: url.path)
            ? "annotations.json (version 1, read only)" : "No labels (version 1, read only)"
    }

    private var url: URL { pack.directory.appendingPathComponent("annotations.json") }

    func load() throws -> MembershipDocument {
        guard FileManager.default.fileExists(atPath: url.path) else {
            var empty = Sidecar()
            empty.packDigest = pack.manifest.packDigest
            empty.datasetID = pack.manifest.datasetID
            return MembershipDocument(sidecar: empty, loadedDigest: "", revision: 0)
        }
        let data: Data
        do { data = try Data(contentsOf: url) } catch {
            throw MembershipStoreError.malformed("read annotations.json: \(error)")
        }
        guard data.count <= LegacySidecarReader.maxBytes else {
            throw MembershipStoreError.malformed("annotations.json is \(data.count) bytes")
        }
        let sidecar: Sidecar
        do { sidecar = try JSONDecoder().decode(Sidecar.self, from: data) } catch {
            // A corrupt file opens with an error, never as an empty pack: an
            // empty pack would read as "nothing labelled yet".
            throw MembershipStoreError.malformed("decode annotations.json: \(error)")
        }
        if !sidecar.packDigest.isEmpty, sidecar.packDigest != pack.manifest.packDigest {
            throw MembershipStoreError.malformed(
                "annotations belong to pack \(sidecar.packDigest), not \(pack.manifest.packDigest)")
        }
        return MembershipDocument(
            sidecar: sidecar, loadedDigest: AnnotationPack.digest(data), revision: sidecar.revision)
    }

    func save(
        _ sidecar: Sidecar, over base: MembershipDocument, change: Provenance
    ) throws -> MembershipDocument {
        throw MembershipStoreError.readOnly(readOnlyReason ?? "read only")
    }

    func restore(revision: Int, change: Provenance) throws -> MembershipDocument {
        throw MembershipStoreError.readOnly(readOnlyReason ?? "read only")
    }

    func revisions() -> [MembershipRevision] { [] }
}

// MARK: - The link from a pack to its label set

/// label-set-link.json in a pack directory: which label set the pack is a
/// view of, and where it is, relative to the pack. Outside the pack digest.
struct LabelSetLink: Equatable {
    static let filename = "label-set-link.json"

    var labelSetID: String
    var path: String

    static func read(packDirectory: URL) throws -> LabelSetLink? {
        let url = packDirectory.appendingPathComponent(filename)
        guard FileManager.default.fileExists(atPath: url.path) else { return nil }
        do {
            var f = try LabelSetFields(
                LabelSetJSON.parse(try Data(contentsOf: url)), LabelSetLink.filename)
            let link = LabelSetLink(
                labelSetID: try f.string("label_set_id"), path: try f.string("path"))
            try f.finish()
            guard !link.labelSetID.isEmpty, !link.path.isEmpty else {
                throw LabelSetCodingError("\(filename) names no label set")
            }
            return link
        } catch { throw MembershipStoreError.malformed("\(filename): \(error)") }
    }

    func write(packDirectory: URL) throws {
        let json = LabelSetJSON.object(["label_set_id": .string(labelSetID), "path": .string(path)])
        do {
            try LabelSetStore.writeAtomic(packDirectory, LabelSetLink.filename, json.indentedData)
        } catch {
            throw MembershipStoreError.unwritable("write \(LabelSetLink.filename): \(error)")
        }
    }

    func directory(packDirectory: URL) -> URL {
        packDirectory.appendingPathComponent(path).standardizedFileURL
    }
}

/// A label set beside a pack that shares packets with it: one the pack could
/// be attached to.
struct LabelSetCandidate: Equatable, Identifiable {
    var directory: URL
    var name: String
    var labelSetID: String
    /// How many of the pack's packets the label set's table holds.
    var sharedPackets: Int
    var packPackets: Int

    var id: String { directory.path }
}

// MARK: - Version 2: label sets through a view

/// A version 2 pack's membership, kept in a label set and seen through the
/// pack's view map.
final class LabelSetMembership: MembershipStore {
    let pack: AnnotationPack

    /// The opened label set, or nil until the first save creates it.
    private(set) var store: LabelSetStore?
    /// The state a pack with no label set starts from: its packets, windows
    /// and foreground, and no labels.
    private var unborn: LabelSetState?
    private(set) var view: LabelSetViewMap?
    private(set) var layout: ReturnLayout?
    /// True while the label set does not yet hold this pack's foreground: a
    /// pack linked by hand, or a set whose domain was deleted. The next save
    /// adds it.
    private(set) var domainPending = false
    /// Set when the app may not write where the label set lives: the
    /// operator declined to grant the folder. The window opens read only and
    /// says why, rather than failing at the first save.
    var accessRefusal: String?
    /// The order objects were last shown in, so "car 2" stays car 2 across a
    /// save or a reload in one window.
    private var objectOrder: [String] = []

    init(pack: AnnotationPack) { self.pack = pack }

    var domainID: String { pack.labelSetDomain.domainID }

    /// The packets a replay is checked against: the label set's table, or
    /// before the first save makes one, the pack's own.
    var packetTable: [UInt64] { store?.baseState.packets ?? pack.packets }

    var labelsDirectory: URL { pack.annotationDirectory.appendingPathComponent("labels") }

    var readOnlyReason: String? { accessRefusal }

    var summary: String {
        guard let store else {
            return "No label set yet: the first save makes one in \(labelsDirectory.path)"
        }
        let name = store.manifest.name ?? store.directory.lastPathComponent
        return "Label set \(name), revision \(store.revision)"
            + (domainPending ? " (this run's foreground is added on the next save)" : "")
    }

    // MARK: Load

    func load() throws -> MembershipDocument {
        cachedRevisions = nil
        let link = try LabelSetLink.read(packDirectory: pack.directory)
        var state: LabelSetState
        if let link {
            let dir = link.directory(packDirectory: pack.directory)
            let opened: LabelSetStore
            do { opened = try LabelSetStore.open(dir) } catch let error as LabelSetError {
                throw MembershipStoreError.malformed("label set \(link.path): \(error)")
            }
            guard opened.labelSetID == link.labelSetID else {
                throw MembershipStoreError.malformed(
                    "\(LabelSetLink.filename) names label set \(link.labelSetID), but \(link.path) holds \(opened.labelSetID)"
                )
            }
            store = opened
            unborn = nil
            layout = opened.layout
            state = opened.baseState
        } else {
            let (packLayout, initial) = try mapped { try LabelSetState.fromPack(pack) }
            store = nil
            unborn = initial
            layout = packLayout
            state = initial
        }
        domainPending = !state.domains.contains { $0.domainID == domainID }
        if domainPending {
            // The view is of the state as it will be once this pack's
            // foreground is in it, which is what the next save writes.
            _ = try mapped { try state.addPackDomain(layout: layout!, pack: pack) }
        }
        view = try mapped { try LabelSetViewMap(layout: layout!, state: state, pack: pack) }
        return document(projecting: state)
    }

    /// The state edits are folded into: the loaded head, or the pack's own
    /// starting state.
    private var base: LabelSetState? { store?.baseState ?? unborn }

    private func document(projecting state: LabelSetState) -> MembershipDocument {
        MembershipDocument(
            sidecar: project(state), loadedDigest: store?.stateDigest ?? "",
            revision: store?.revision ?? 0)
    }

    /// The label set seen through this pack: every mask in a window the pack
    /// shows, with the members it shows.
    func project(_ state: LabelSetState) -> Sidecar {
        var sidecar = Sidecar()
        sidecar.datasetID = pack.manifest.datasetID
        sidecar.packDigest = pack.manifest.packDigest
        sidecar.revision = store?.revision ?? 0
        sidecar.updatedUTC = store?.manifest.head.updatedUTC ?? ""
        sidecar.correspondences =
            state.records.correspondences.isEmpty ? nil : state.records.correspondences
        sidecar.dismissedProposals =
            state.records.dismissedProposals.isEmpty ? nil : state.records.dismissedProposals
        var indexes: [Int: LabelSetViewMap.SampleIndex] = [:]
        var firstSeen: [String: Int] = [:]
        let order = Dictionary(
            pack.chronologicalSamples.enumerated().map { ($1.sampleID, $0) },
            uniquingKeysWith: { a, _ in a })
        for m in state.masks {
            guard let view, let sampleID = view.sample(forWindow: m.windowID) else { continue }
            let index = indexes[sampleID] ?? view.sampleIndex(sampleID)
            indexes[sampleID] = index
            let uncertain = index.shown(m.uncertain).indices
            sidecar.masks.append(
                FrameMask(
                    objectID: m.objectID, sampleID: sampleID,
                    pointIndices: index.shown(m.members).indices,
                    uncertainIndices: uncertain.isEmpty ? nil : uncertain,
                    completeness: m.completeness, visibility: m.visibility, status: m.status,
                    pose: m.pose, provenance: m.provenance))
            let position = order[sampleID] ?? Int.max
            firstSeen[m.objectID] = min(firstSeen[m.objectID] ?? Int.max, position)
        }
        sidecar.masks.sort {
            $0.sampleID == $1.sampleID ? $0.objectID < $1.objectID : $0.sampleID < $1.sampleID
        }
        // The label set keeps objects by id. Names number objects of a class
        // in the order they are listed, so list them as they were last shown
        // and otherwise by where they first appear in this pack.
        let known = Dictionary(
            objectOrder.enumerated().map { ($1, $0) }, uniquingKeysWith: { a, _ in a })
        sidecar.objects = state.records.objects.sorted { a, b in
            let ka = known[a.objectID] ?? Int.max
            let kb = known[b.objectID] ?? Int.max
            if ka != kb { return ka < kb }
            let fa = firstSeen[a.objectID] ?? Int.max
            let fb = firstSeen[b.objectID] ?? Int.max
            if fa != fb { return fa < fb }
            return a.objectID < b.objectID
        }
        objectOrder = sidecar.objects.map(\.objectID)
        return sidecar
    }

    // MARK: Save

    func save(
        _ sidecar: Sidecar, over document: MembershipDocument, change: Provenance
    ) throws -> MembershipDocument {
        if let accessRefusal { throw MembershipStoreError.readOnly(accessRefusal) }
        guard var working = base, let layout else {
            throw MembershipStoreError.malformed("the label set was not loaded")
        }
        // The view map depends only on the packet table and the windows. They
        // change here only when this pack's foreground is first added; the
        // map built at load is otherwise the one to use, and rebuilding it
        // would cost every point of the pack on every save.
        if domainPending {
            _ = try mapped { try working.addPackDomain(layout: layout, pack: pack) }
        }
        let view: LabelSetViewMap
        if let built = self.view, !domainPending {
            view = built
        } else {
            view = try mapped { try LabelSetViewMap(layout: layout, state: working, pack: pack) }
        }
        let next = try fold(
            sidecar, over: document.sidecar, into: working, view: view, layout: layout)
        let labelChange = LabelSetChange(
            author: change.author, session: change.session, operation: change.operation ?? "save")

        if let store {
            // Both states are canonical, so equal states are equal values: a
            // compare, not a diff, says nothing changed.
            guard next != store.baseState else { return document }
            store.state = next
            try mapped(refusing: "Not saved: ") { try store.save(change: labelChange) }
        } else {
            guard let unborn, next != unborn.canonical else { return document }
            let created = try create(from: unborn, layout: layout, change: labelChange)
            // The set exists from here, whether or not the edit below is
            // saved: a retry folds into it rather than making another.
            self.store = created
            self.unborn = nil
            created.state = next
            try mapped(refusing: "Not saved: ") { try created.save(change: labelChange) }
        }
        domainPending = false
        self.view = view
        noteCommitted(labelChange)
        // The edit as saved is this view's projection of the new state: what
        // a member the view hides was is kept, and what it shows is what the
        // operator left. Re-projecting the whole label set would only redo
        // that, at the cost of every mask.
        var saved = sidecar
        saved.objects = sidecar.objects.map(\.labelSetNormalised)
        saved.masks = sidecar.masks.map { mask in
            var m = mask
            m.provenance = m.provenance.labelSetNormalised
            m.pose = m.pose?.labelSetNormalised
            if m.uncertainIndices?.isEmpty == true { m.uncertainIndices = nil }
            return m
        }
        saved.dismissedProposals =
            next.records.dismissedProposals.isEmpty ? nil : next.records.dismissedProposals
        saved.revision = store?.revision ?? 0
        saved.updatedUTC = store?.manifest.head.updatedUTC ?? ""
        saved.packDigest = pack.manifest.packDigest
        saved.datasetID = pack.manifest.datasetID
        objectOrder = saved.objects.map(\.objectID)
        return MembershipDocument(
            sidecar: saved, loadedDigest: store?.stateDigest ?? "", revision: store?.revision ?? 0)
    }

    /// Writes a new label set holding the pack's starting state as revision
    /// 1, then the link that makes this pack a view of it.
    private func create(
        from initial: LabelSetState, layout: ReturnLayout, change: LabelSetChange
    ) throws -> LabelSetStore {
        // Another window may have made one for this pack since this one
        // opened: saving over it would orphan that set.
        if let link = try LabelSetLink.read(packDirectory: pack.directory) {
            let current =
                (try? LabelSetStore.open(link.directory(packDirectory: pack.directory)))?.revision
                ?? -1
            throw MembershipStoreError.conflict(loadedRevision: 0, currentRevision: current)
        }
        let id = UUID().uuidString.lowercased()
        let name = uniqueName(labelSetID: id)
        let directory = labelsDirectory.appendingPathComponent(name)
        var manifest = LabelSetManifest(labelSetID: id, name: name, layout: layout)
        manifest.note = "Started from pack \(pack.manifest.packDigest) (\(pack.manifest.datasetID))"
        var creation = change
        creation.operation = "create_from_pack"
        let created = try mapped {
            try LabelSetStore.create(
                at: directory, manifest: manifest, initial: initial, change: creation)
        }
        try LabelSetLink(labelSetID: id, path: "../labels/\(name)").write(
            packDirectory: pack.directory)
        return created
    }

    /// The pcap's stem and the start of the label set's id, made unique
    /// among the label sets already there.
    private func uniqueName(labelSetID: String) -> String {
        let source =
            pack.manifest.source.pcapBasename.nonEmpty ?? Optional(
                URL(fileURLWithPath: pack.manifest.source.vrlogPath).lastPathComponent
            ).nonEmpty ?? "label-set"
        var stem = source
        for suffix in [".pcapng", ".pcap", ".vrlog", ".gz"]
        where stem.lowercased().hasSuffix(suffix) { stem = String(stem.dropLast(suffix.count)) }
        let allowed = CharacterSet.alphanumerics.union(CharacterSet(charactersIn: "-_."))
        stem = String(
            stem.unicodeScalars.map { allowed.contains($0) && $0.isASCII ? Character($0) : "-" })
        if stem.isEmpty || stem.hasPrefix(".") { stem = "label-set" + stem }
        let base = "\(stem)-\(labelSetID.prefix(8))"
        var name = base
        var n = 2
        while FileManager.default.fileExists(
            atPath: labelsDirectory.appendingPathComponent(name).path)
        {
            name = "\(base)-\(n)"
            n += 1
        }
        return name
    }

    /// Folds an edited projection into the label set state. Only masks that
    /// differ from the projection the edit started from are touched, each
    /// through the view map, so members another run's view holds survive.
    func fold(
        _ edited: Sidecar, over original: Sidecar, into base: LabelSetState, view: LabelSetViewMap,
        layout: ReturnLayout
    ) throws -> LabelSetState {
        var s = base
        let editedObjects = edited.objects.map(\.labelSetNormalised)
        let keptIDs = Set(editedObjects.map(\.objectID))
        let removed = Set(s.records.objects.map(\.objectID)).subtracting(keptIDs)
        s.records.objects = editedObjects
        s.records.correspondences = edited.correspondences ?? []
        var dismissed: [String] = []
        var seenDismissed = Set<String>()
        for id in edited.dismissedProposals ?? [] where seenDismissed.insert(id).inserted {
            dismissed.append(id)
        }
        s.records.dismissedProposals = dismissed

        func hidden(_ m: LabelSetMask) -> UInt64? {
            guard let sample = view.sample(forWindow: m.windowID) else { return nil }
            return view.shown(sampleID: sample, m.members).hidden
                + view.shown(sampleID: sample, m.uncertain).hidden
        }
        // An object's labels this view cannot see are not this view's to
        // delete: a merge or a removal made here would drop them unseen.
        for m in s.masks where removed.contains(m.objectID) {
            guard let unseen = hidden(m), unseen == 0 else {
                throw MembershipStoreError.refused(
                    "Not saved: \(m.objectID) has labels in frames or returns this pack does not "
                        + "show (window \(m.windowID)), made through another run's view. Remove or "
                        + "merge it from a pack that shows them.")
            }
        }
        s.masks.removeAll { removed.contains($0.objectID) }

        struct Key: Hashable {
            var object: String
            var sample: Int
        }
        let before = Dictionary(
            original.masks.map { (Key(object: $0.objectID, sample: $0.sampleID), $0) },
            uniquingKeysWith: { a, _ in a })
        let after = Dictionary(
            edited.masks.map { (Key(object: $0.objectID, sample: $0.sampleID), $0) },
            uniquingKeysWith: { a, _ in a })

        for (key, _) in before where after[key] == nil && !removed.contains(key.object) {
            guard let window = view.window(forSample: key.sample),
                let i = s.maskIndex(objectID: key.object, windowID: window)
            else { continue }
            guard hidden(s.masks[i]) == 0 else {
                throw MembershipStoreError.refused(
                    "Not saved: the mask of \(key.object) in sample \(key.sample) holds returns this "
                        + "pack does not show, made through another run's view.")
            }
            s.masks.remove(at: i)
        }

        for (key, mask) in after where before[key] != mask {
            guard let window = view.window(forSample: key.sample) else {
                throw MembershipStoreError.refused(
                    "Not saved: sample \(key.sample) is not a frame of this label set. This run "
                        + "framed the capture differently, so its points cannot be labelled here.")
            }
            let existingIndex = s.maskIndex(objectID: key.object, windowID: window)
            let existing = existingIndex.map { s.masks[$0] }
            let members: ReturnRanges
            let uncertain: ReturnRanges
            do {
                members = try view.edit(
                    sampleID: key.sample, existing?.members ?? .empty,
                    shownIndices: mask.pointIndices)
                uncertain = try view.edit(
                    sampleID: key.sample, existing?.uncertain ?? .empty,
                    shownIndices: mask.uncertainIndices ?? [])
            } catch { throw MembershipStoreError.refused("Not saved: \(error)") }
            let all = ReturnRanges.union(members, uncertain)
            func holds(_ domain: String) -> Bool {
                guard let returns = s.domainReturns(domainID: domain, windowID: window) else {
                    return false
                }
                return ReturnRanges.difference(all, returns).isEmpty
            }
            // A mask is made on one foreground. It keeps the one it was made
            // on while that holds every member, and otherwise moves to this
            // view's when nothing from the old one is left.
            let domain: String
            if let existing, holds(existing.domainID) {
                domain = existing.domainID
            } else if holds(domainID) {
                domain = domainID
            } else {
                throw MembershipStoreError.refused(
                    "Not saved: the mask of \(key.object) in sample \(key.sample) would mix returns "
                        + "kept by two runs' foregrounds. A mask is made on one foreground.")
            }
            let updated = LabelSetMask(
                objectID: key.object, windowID: window, domainID: domain, members: members,
                uncertain: uncertain, completeness: mask.completeness, visibility: mask.visibility,
                status: mask.status, pose: mask.pose, provenance: mask.provenance)
            if let existingIndex {
                s.masks[existingIndex] = updated
            } else {
                s.masks.append(updated)
            }
        }
        // Validated once, by the store, before anything is written.
        s.canonicalise()
        return s
    }

    // MARK: Restore and history

    func restore(revision: Int, change: Provenance) throws -> MembershipDocument {
        if let accessRefusal { throw MembershipStoreError.readOnly(accessRefusal) }
        guard let store else {
            throw MembershipStoreError.refused(
                "Nothing has been saved yet, so there is nothing to restore.")
        }
        try mapped {
            try store.restore(
                revision: revision,
                change: LabelSetChange(
                    author: change.author, session: change.session, operation: "restore"))
        }
        noteCommitted(
            LabelSetChange(author: change.author, session: change.session, operation: "restore"),
            restoredFrom: revision)
        let state = store.baseState
        domainPending = !state.domains.contains { $0.domainID == domainID }
        var viewed = state
        if domainPending, let layout {
            _ = try mapped { try viewed.addPackDomain(layout: layout, pack: pack) }
        }
        view = try mapped {
            try LabelSetViewMap(layout: layout ?? store.layout, state: viewed, pack: pack)
        }
        return document(projecting: state)
    }

    /// Read from the journal once, then kept: the window lists them on every
    /// redraw. A save in this window appends to the list; a load, which may
    /// follow another window's saves, reads it again.
    func revisions() -> [MembershipRevision] {
        if let cachedRevisions { return cachedRevisions }
        guard let store, let lines = try? LabelSetStore.history(store.directory) else { return [] }
        let read = lines.filter { !$0.base }.map {
            MembershipRevision(
                revision: $0.revision, author: $0.change.author, operation: $0.change.operation,
                createdUTC: $0.change.createdUTC,
                restoredFrom: $0.restoredFrom > 0 ? $0.restoredFrom : nil)
        }
        cachedRevisions = read
        return read
    }

    private var cachedRevisions: [MembershipRevision]?

    /// Brings the cached list up to the head after this window committed
    /// `operation` (and, for a new set, its creation before it).
    private func noteCommitted(_ change: LabelSetChange, restoredFrom: Int? = nil) {
        guard let store, var list = cachedRevisions else { return }
        guard (list.last?.revision ?? 0) == store.revision - 1 else {
            cachedRevisions = nil
            return
        }
        list.append(
            MembershipRevision(
                revision: store.revision, author: change.author,
                operation: restoredFrom == nil ? change.operation : "restore",
                createdUTC: store.manifest.head.updatedUTC, restoredFrom: restoredFrom))
        cachedRevisions = list
    }

    // MARK: Attaching another run's pack

    /// Label sets in this pack's labels folder that share packets with it,
    /// most shared first: those this pack could be attached to.
    func candidates() -> [LabelSetCandidate] {
        guard let names = try? FileManager.default.contentsOfDirectory(atPath: labelsDirectory.path)
        else { return [] }
        let packKeys = Set(pack.packets)
        var out: [LabelSetCandidate] = []
        for name in names.sorted() {
            let dir = labelsDirectory.appendingPathComponent(name)
            guard let manifestData = try? LabelSetStore.readFile(dir, LabelSetFiles.manifest),
                let manifest = try? LabelSetManifest.read(manifestData),
                let packetData = try? LabelSetStore.readFile(dir, LabelSetFiles.packets),
                let keys = try? PacketKeys.decode(packetData, name: LabelSetFiles.packets)
            else { continue }
            let shared = keys.reduce(0) { $0 + (packKeys.contains($1) ? 1 : 0) }
            guard shared > 0 else { continue }
            out.append(
                LabelSetCandidate(
                    directory: dir, name: manifest.name ?? name, labelSetID: manifest.labelSetID,
                    sharedPackets: shared, packPackets: pack.packets.count))
        }
        return out.sorted { $0.sharedPackets > $1.sharedPackets }
    }

    /// Makes this pack a view of an existing label set of the same capture:
    /// its foreground becomes a new domain, as one revision, and the link is
    /// written. Returns the ids of samples that could not be joined because
    /// this run framed them differently.
    @discardableResult func attach(to directory: URL, change: Provenance) throws -> [Int] {
        if let link = try LabelSetLink.read(packDirectory: pack.directory) {
            throw MembershipStoreError.refused(
                "This pack is already a view of label set \(link.path).")
        }
        let target: LabelSetStore
        do { target = try LabelSetStore.open(directory) } catch {
            throw MembershipStoreError.malformed(
                "label set \(directory.lastPathComponent): \(error)")
        }
        let packLayout = try mapped { try pack.labelSetLayout() }
        guard packLayout == target.layout else {
            throw MembershipStoreError.refused(
                "This pack's sensor layout \(packLayout.model) is not the label set's \(target.layout.model)."
            )
        }
        let packKeys = Set(pack.packets)
        guard target.baseState.packets.contains(where: packKeys.contains) else {
            throw MembershipStoreError.refused(
                "\(directory.lastPathComponent) shares no packets with this pack: it is of another capture."
            )
        }
        var working = target.baseState
        var skipped: [Int] = []
        if !working.domains.contains(where: { $0.domainID == domainID }) {
            skipped = try mapped { try working.addPackDomain(layout: packLayout, pack: pack) }
            // Refuse a pack the label set cannot be seen through before
            // anything is written.
            _ = try mapped { try LabelSetViewMap(layout: packLayout, state: working, pack: pack) }
            target.state = working
            try mapped {
                try target.save(
                    change: LabelSetChange(
                        author: change.author, session: change.session, operation: "add_domain"))
            }
        }
        let relative = Self.relativePath(from: pack.directory, to: target.directory)
        try LabelSetLink(labelSetID: target.labelSetID, path: relative).write(
            packDirectory: pack.directory)
        return skipped
    }

    /// `to` relative to `from`, both directories, as a link records it.
    static func relativePath(from: URL, to: URL) -> String {
        let a = from.standardizedFileURL.resolvingSymlinksInPath().pathComponents
        let b = to.standardizedFileURL.resolvingSymlinksInPath().pathComponents
        var common = 0
        while common < a.count, common < b.count, a[common] == b[common] { common += 1 }
        let up = Array(repeating: "..", count: a.count - common)
        return (up + b[common...]).joined(separator: "/")
    }

    /// Runs a label-set operation, turning its errors into the window's.
    /// `refusing` prefixes a refusal, so a save that broke a rule says it
    /// was not saved.
    private func mapped<T>(refusing prefix: String = "", _ body: () throws -> T) throws -> T {
        do { return try body() } catch let error as LabelSetError {
            switch error {
            case .busy: throw MembershipStoreError.busy
            case .conflict(let loaded, let current, _):
                throw MembershipStoreError.conflict(
                    loadedRevision: loaded, currentRevision: current)
            case .historyNotRetained(let revision):
                throw MembershipStoreError.refused(
                    "Revision \(revision) is older than the history this copy keeps.")
            case .invalid(let detail): throw MembershipStoreError.refused(prefix + detail)
            case .io(let detail): throw MembershipStoreError.unwritable(detail)
            }
        }
    }
}
