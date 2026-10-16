// LabelSetJournal.swift
// One save as a delta from its parent: the journal line.
//
// Mirrors internal/lidar/annotation/labelset/journal.go. A save that touches
// one return of one mask records one return, so history is a journal of
// deltas rather than a snapshot per save: the version 1 store kept 42 GB of
// snapshots for a 26 MB head.

import Foundation

/// Describes one save. The store supplies revision numbers and the time; it
/// never invents an author.
struct LabelSetChange: Equatable {
    var author: String
    var session: String?
    var operation: String
    var createdUTC: String = ""

    var json: LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.string("author", author)
        b.omitEmpty("session", session)
        b.string("operation", operation)
        b.string("created_utc", createdUTC)
        return b.json
    }

    init(author: String, session: String? = nil, operation: String, createdUTC: String = "") {
        self.author = author
        self.session = session
        self.operation = operation
        self.createdUTC = createdUTC
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        author = try f.string("author")
        session = try f.optionalString("session")
        operation = try f.string("operation")
        createdUTC = try f.string("created_utc")
        try f.finish()
    }
}

/// A mask's attributes other than its membership. A delta carries them whole
/// whenever any of them changes, since they are small.
struct LabelSetMaskFields: Equatable {
    var domainID: String
    var completeness: MaskCompleteness
    var visibility: Visibility
    var status: ReviewStatus
    var pose: AnnotationPose?
    var provenance: Provenance

    init(_ m: LabelSetMask) {
        domainID = m.domainID
        completeness = m.completeness
        visibility = m.visibility
        status = m.status
        pose = m.pose
        provenance = m.provenance
    }

    func json() throws -> LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.string("domain_id", domainID)
        b.string("completeness", completeness.rawValue)
        b.string("visibility", visibility.rawValue)
        b.string("status", status.rawValue)
        if let pose { b.set("pose", try pose.labelSetJSON()) }
        b.set("provenance", provenance.labelSetJSON)
        return b.json
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        domainID = try f.string("domain_id")
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

/// Changes one mask. Membership moves by additions and removals.
struct LabelSetMaskDelta: Equatable {
    var objectID: String
    var windowID: Int
    var delete = false
    var fields: LabelSetMaskFields?
    var membersAdd = ReturnRanges.empty
    var membersRemove = ReturnRanges.empty
    var uncertainAdd = ReturnRanges.empty
    var uncertainRemove = ReturnRanges.empty

    func json() throws -> LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.string("object_id", objectID)
        b.int("window_id", windowID)
        b.omitFalse("delete", delete)
        if let fields { b.set("fields", try fields.json()) }
        if !membersAdd.isEmpty { b.set("members_add", membersAdd.json) }
        if !membersRemove.isEmpty { b.set("members_remove", membersRemove.json) }
        if !uncertainAdd.isEmpty { b.set("uncertain_add", uncertainAdd.json) }
        if !uncertainRemove.isEmpty { b.set("uncertain_remove", uncertainRemove.json) }
        return b.json
    }

    init(objectID: String, windowID: Int, delete: Bool = false) {
        self.objectID = objectID
        self.windowID = windowID
        self.delete = delete
    }

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        objectID = try f.string("object_id")
        windowID = try f.int("window_id")
        delete = try f.bool("delete")
        fields = try f.value("fields").map { try LabelSetMaskFields(json: $0, context) }
        membersAdd = try ReturnRanges.decode(f.value("members_add"), "\(context) members_add")
        membersRemove = try ReturnRanges.decode(
            f.value("members_remove"), "\(context) members_remove")
        uncertainAdd = try ReturnRanges.decode(f.value("uncertain_add"), "\(context) uncertain_add")
        uncertainRemove = try ReturnRanges.decode(
            f.value("uncertain_remove"), "\(context) uncertain_remove")
        try f.finish()
    }
}

/// Names one domain's returns in one window.
struct LabelSetDomainReturnsKey: Equatable, Hashable {
    var domainID: String
    var windowID: Int
}

/// One journal line: the change from the parent state to `state`. Records
/// other than masks are replaced whole when they change.
struct LabelSetDelta: Equatable {
    var revision = 0
    var parentRevision = 0
    var parentState = ""
    var state = ""
    var change = LabelSetChange(author: "", operation: "")
    var restoredFrom = 0
    /// Marks a journal that starts from a head it did not record, such as one
    /// published without history. Its state is kept in journal-base/.
    var base = false

    var packetsAppend: [UInt64] = []
    var objectsPut: [AnnotationObject] = []
    var objectsDelete: [String] = []
    /// Nil when unchanged; an empty list when every correspondence went.
    var correspondences: [TrackCorrespondence]?
    var dismissedAdd: [String] = []
    var dismissedRemove: [String] = []
    var domainsPut: [LabelSetDomain] = []
    var domainsDelete: [String] = []
    var windowsPut: [LabelSetWindow] = []
    var windowsDelete: [Int] = []
    var domainReturnsPut: [LabelSetDomainReturns] = []
    var domainReturnsDelete: [LabelSetDomainReturnsKey] = []
    var masks: [LabelSetMaskDelta] = []

    /// Whether the delta changes nothing.
    var isEmpty: Bool {
        packetsAppend.isEmpty && objectsPut.isEmpty && objectsDelete.isEmpty
            && correspondences == nil && dismissedAdd.isEmpty && dismissedRemove.isEmpty
            && domainsPut.isEmpty && domainsDelete.isEmpty && windowsPut.isEmpty
            && windowsDelete.isEmpty && domainReturnsPut.isEmpty && domainReturnsDelete.isEmpty
            && masks.isEmpty
    }

    func json() throws -> LabelSetJSON {
        var b = LabelSetObjectBuilder()
        b.int("revision", revision)
        b.int("parent_revision", parentRevision)
        b.string("parent_state", parentState)
        b.string("state", state)
        b.set("change", change.json)
        b.omitZero("restored_from", restoredFrom)
        b.omitFalse("base", base)
        b.omitEmptyArray("packets_append", packetsAppend.map { .string(PacketKeys.hex($0)) })
        b.omitEmptyArray("objects_put", try objectsPut.map { try $0.labelSetJSON() })
        b.omitEmptyArray("objects_delete", objectsDelete.map(LabelSetJSON.string))
        if let correspondences {
            b.set("correspondences", .array(correspondences.map(\.labelSetJSON)))
        }
        b.omitEmptyArray("dismissed_add", dismissedAdd.map(LabelSetJSON.string))
        b.omitEmptyArray("dismissed_remove", dismissedRemove.map(LabelSetJSON.string))
        b.omitEmptyArray("domains_put", try domainsPut.map { try $0.json() })
        b.omitEmptyArray("domains_delete", domainsDelete.map(LabelSetJSON.string))
        b.omitEmptyArray("windows_put", windowsPut.map(\.json))
        b.omitEmptyArray("windows_delete", windowsDelete.map { .int($0) })
        b.omitEmptyArray("domain_returns_put", domainReturnsPut.map(\.json))
        b.omitEmptyArray(
            "domain_returns_delete",
            domainReturnsDelete.map {
                .object(["domain_id": .string($0.domainID), "window_id": .int($0.windowID)])
            })
        b.omitEmptyArray("masks", try masks.map { try $0.json() })
        return b.json
    }

    init() {}

    init(json value: LabelSetJSON, _ context: String) throws {
        var f = try LabelSetFields(value, context)
        revision = try f.int("revision")
        parentRevision = try f.int("parent_revision")
        parentState = try f.string("parent_state")
        state = try f.string("state")
        change =
            try f.value("change").map { try LabelSetChange(json: $0, context) }
            ?? LabelSetChange(author: "", operation: "")
        restoredFrom = try f.int("restored_from")
        base = try f.bool("base")
        packetsAppend = try f.strings("packets_append").map { try PacketKeys.parse($0) }
        objectsPut = try f.array("objects_put").map {
            try AnnotationObject(labelSetJSON: $0, context)
        }
        objectsDelete = try f.strings("objects_delete")
        correspondences = try f.optionalArray("correspondences")?.map {
            try TrackCorrespondence(labelSetJSON: $0, context)
        }
        dismissedAdd = try f.strings("dismissed_add")
        dismissedRemove = try f.strings("dismissed_remove")
        domainsPut = try f.array("domains_put").map { try LabelSetDomain(json: $0, context) }
        domainsDelete = try f.strings("domains_delete")
        windowsPut = try f.array("windows_put").map { try LabelSetWindow(json: $0, context) }
        windowsDelete = try f.ints("windows_delete")
        domainReturnsPut = try f.array("domain_returns_put").map {
            try LabelSetDomainReturns(json: $0, context)
        }
        domainReturnsDelete = try f.array("domain_returns_delete").map {
            var k = try LabelSetFields($0, "\(context) domain_returns_delete")
            let key = LabelSetDomainReturnsKey(
                domainID: try k.string("domain_id"), windowID: try k.int("window_id"))
            try k.finish()
            return key
        }
        masks = try f.array("masks").map { try LabelSetMaskDelta(json: $0, context) }
        try f.finish()
    }
}

/// A delta that does not fit the state it is applied to.
struct LabelSetApplyError: Error, CustomStringConvertible { var description: String }

extension LabelSetDelta {
    /// The delta from `base` to `next`. Both must be canonical. The packet
    /// table may only grow at its end: return numbers are positions in it, so
    /// any other change would renumber every return.
    static func diff(from base: LabelSetState, to next: LabelSetState) throws -> LabelSetDelta {
        var d = LabelSetDelta()
        guard next.packets.count >= base.packets.count else {
            throw LabelSetApplyError(
                description:
                    "packet table shrank from \(base.packets.count) to \(next.packets.count): the table is append-only"
            )
        }
        for (i, k) in base.packets.enumerated() where next.packets[i] != k {
            throw LabelSetApplyError(description: "packet \(i) changed: the table is append-only")
        }
        d.packetsAppend = Array(next.packets[base.packets.count...])

        let baseObjects = Dictionary(
            base.records.objects.map { ($0.objectID, $0) }, uniquingKeysWith: { a, _ in a })
        let nextObjectIDs = Set(next.records.objects.map(\.objectID))
        for o in next.records.objects where baseObjects[o.objectID] != o { d.objectsPut.append(o) }
        for o in base.records.objects where !nextObjectIDs.contains(o.objectID) {
            d.objectsDelete.append(o.objectID)
        }
        if base.records.correspondences != next.records.correspondences {
            d.correspondences = next.records.correspondences
        }
        let baseDismissed = Set(base.records.dismissedProposals)
        let nextDismissed = Set(next.records.dismissedProposals)
        d.dismissedAdd = next.records.dismissedProposals.filter { !baseDismissed.contains($0) }
        d.dismissedRemove = base.records.dismissedProposals.filter { !nextDismissed.contains($0) }

        let baseDomains = Dictionary(
            base.domains.map { ($0.domainID, $0) }, uniquingKeysWith: { a, _ in a })
        let nextDomainIDs = Set(next.domains.map(\.domainID))
        for x in next.domains where baseDomains[x.domainID] != x { d.domainsPut.append(x) }
        for x in base.domains where !nextDomainIDs.contains(x.domainID) {
            d.domainsDelete.append(x.domainID)
        }

        let baseWindows = Dictionary(
            base.windows.map { ($0.windowID, $0) }, uniquingKeysWith: { a, _ in a })
        let nextWindowIDs = Set(next.windows.map(\.windowID))
        for w in next.windows where baseWindows[w.windowID] != w { d.windowsPut.append(w) }
        for w in base.windows where !nextWindowIDs.contains(w.windowID) {
            d.windowsDelete.append(w.windowID)
        }

        func key(_ x: LabelSetDomainReturns) -> LabelSetDomainReturnsKey {
            LabelSetDomainReturnsKey(domainID: x.domainID, windowID: x.windowID)
        }
        let baseReturns = Dictionary(
            base.domainReturns.map { (key($0), $0.returns) }, uniquingKeysWith: { a, _ in a })
        let nextReturnKeys = Set(next.domainReturns.map(key))
        for x in next.domainReturns where baseReturns[key(x)] != x.returns {
            d.domainReturnsPut.append(x)
        }
        for x in base.domainReturns where !nextReturnKeys.contains(key(x)) {
            d.domainReturnsDelete.append(key(x))
        }

        struct MaskKey: Hashable {
            var object: String
            var window: Int
        }
        let baseMasks = Dictionary(
            base.masks.map { (MaskKey(object: $0.objectID, window: $0.windowID), $0) },
            uniquingKeysWith: { a, _ in a })
        var nextMaskKeys = Set<MaskKey>()
        for m in next.masks {
            let k = MaskKey(object: m.objectID, window: m.windowID)
            nextMaskKeys.insert(k)
            let prev = baseMasks[k]
            if let prev, prev == m { continue }
            var md = LabelSetMaskDelta(objectID: m.objectID, windowID: m.windowID)
            if prev == nil || LabelSetMaskFields(prev!) != LabelSetMaskFields(m) {
                md.fields = LabelSetMaskFields(m)
            }
            let prevMembers = prev?.members ?? .empty
            let prevUncertain = prev?.uncertain ?? .empty
            md.membersAdd = .difference(m.members, prevMembers)
            md.membersRemove = .difference(prevMembers, m.members)
            md.uncertainAdd = .difference(m.uncertain, prevUncertain)
            md.uncertainRemove = .difference(prevUncertain, m.uncertain)
            if md.fields != nil || !md.membersAdd.isEmpty || !md.membersRemove.isEmpty
                || !md.uncertainAdd.isEmpty || !md.uncertainRemove.isEmpty
            {
                d.masks.append(md)
            }
        }
        for m in base.masks
        where !nextMaskKeys.contains(MaskKey(object: m.objectID, window: m.windowID)) {
            d.masks.append(
                LabelSetMaskDelta(objectID: m.objectID, windowID: m.windowID, delete: true))
        }
        return d
    }

    /// `base` with this delta applied, canonicalised. Refuses a delta that
    /// does not fit: removing what is absent, adding what is present, or
    /// creating a mask without its fields.
    func apply(to base: LabelSetState) throws -> LabelSetState {
        var s = base
        s.packets.append(contentsOf: packetsAppend)

        let existing = Set(s.records.objects.map(\.objectID))
        for id in objectsDelete where !existing.contains(id) {
            throw LabelSetApplyError(description: "delta deletes unknown object \"\(id)\"")
        }
        let deletedObjects = Set(objectsDelete)
        s.records.objects.removeAll { deletedObjects.contains($0.objectID) }
        for o in objectsPut {
            if let i = s.records.objects.firstIndex(where: { $0.objectID == o.objectID }) {
                s.records.objects[i] = o
            } else {
                s.records.objects.append(o)
            }
        }
        if let correspondences { s.records.correspondences = correspondences }
        for id in dismissedRemove where !s.records.dismissedProposals.contains(id) {
            throw LabelSetApplyError(
                description: "delta un-dismisses \"\(id)\", which was not dismissed")
        }
        let undismissed = Set(dismissedRemove)
        s.records.dismissedProposals.removeAll { undismissed.contains($0) }
        for id in dismissedAdd {
            if s.records.dismissedProposals.contains(id) {
                throw LabelSetApplyError(description: "delta dismisses \"\(id)\" twice")
            }
            s.records.dismissedProposals.append(id)
        }

        let deletedDomains = Set(domainsDelete)
        s.domains.removeAll { deletedDomains.contains($0.domainID) }
        for x in domainsPut {
            s.domains.removeAll { $0.domainID == x.domainID }
            s.domains.append(x)
        }
        let deletedWindows = Set(windowsDelete)
        s.windows.removeAll { deletedWindows.contains($0.windowID) }
        for w in windowsPut {
            s.windows.removeAll { $0.windowID == w.windowID }
            s.windows.append(w)
        }
        let deletedReturns = Set(domainReturnsDelete)
        s.domainReturns.removeAll {
            deletedReturns.contains(
                LabelSetDomainReturnsKey(domainID: $0.domainID, windowID: $0.windowID))
        }
        for x in domainReturnsPut {
            s.domainReturns.removeAll { $0.domainID == x.domainID && $0.windowID == x.windowID }
            s.domainReturns.append(x)
        }

        struct MaskKey: Hashable {
            var object: String
            var window: Int
        }
        var index: [MaskKey: Int] = [:]
        for (i, m) in s.masks.enumerated() {
            index[MaskKey(object: m.objectID, window: m.windowID)] = i
        }
        var deleted = Set<MaskKey>()
        for md in masks {
            let k = MaskKey(object: md.objectID, window: md.windowID)
            if md.delete {
                guard index[k] != nil, !deleted.contains(k) else {
                    throw LabelSetApplyError(
                        description:
                            "delta deletes absent mask \(md.objectID) window \(md.windowID)")
                }
                deleted.insert(k)
                continue
            }
            let i: Int
            if let found = index[k] {
                i = found
            } else {
                guard md.fields != nil else {
                    throw LabelSetApplyError(
                        description:
                            "delta creates mask \(md.objectID) window \(md.windowID) without its fields"
                    )
                }
                s.masks.append(
                    LabelSetMask(objectID: md.objectID, windowID: md.windowID, domainID: ""))
                i = s.masks.count - 1
                index[k] = i
            }
            if let f = md.fields {
                s.masks[i].domainID = f.domainID
                s.masks[i].completeness = f.completeness
                s.masks[i].visibility = f.visibility
                s.masks[i].status = f.status
                s.masks[i].pose = f.pose
                s.masks[i].provenance = f.provenance
            }
            do {
                s.masks[i].members = try LabelSetDelta.applySet(
                    s.masks[i].members, add: md.membersAdd, remove: md.membersRemove)
            } catch let error as LabelSetApplyError {
                throw LabelSetApplyError(
                    description:
                        "mask \(md.objectID) window \(md.windowID) members: \(error.description)")
            }
            do {
                s.masks[i].uncertain = try LabelSetDelta.applySet(
                    s.masks[i].uncertain, add: md.uncertainAdd, remove: md.uncertainRemove)
            } catch let error as LabelSetApplyError {
                throw LabelSetApplyError(
                    description:
                        "mask \(md.objectID) window \(md.windowID) uncertain: \(error.description)")
            }
        }
        if !deleted.isEmpty {
            s.masks.removeAll {
                deleted.contains(MaskKey(object: $0.objectID, window: $0.windowID))
            }
        }
        s.canonicalise()
        return s
    }

    private static func applySet(
        _ set: ReturnRanges, add: ReturnRanges, remove: ReturnRanges
    ) throws -> ReturnRanges {
        if let absent = ReturnRanges.difference(remove, set).ranges.first {
            throw LabelSetApplyError(
                description: "removes return \(absent.start), which is not present")
        }
        if let present = ReturnRanges.intersect(add, set).ranges.first {
            throw LabelSetApplyError(
                description: "adds return \(present.start), which is already present")
        }
        return .union(.difference(set, remove), add)
    }
}
