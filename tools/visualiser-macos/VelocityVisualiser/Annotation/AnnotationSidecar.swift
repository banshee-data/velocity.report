// AnnotationSidecar.swift
// Reference annotation records and their revision-safe local store.
//
// These types mirror Go's internal/lidar/annotation sidecar schema field for
// field, because both writers touch the same annotations.json. The Swift
// client follows the same protocol the Go store documents: a non-blocking
// kernel lock, a revision and byte-digest check against the loaded baseline,
// the prior snapshot archived before the current file advances, then an
// atomic replace.
//
// A save that cannot prove it is editing the snapshot it loaded is refused.
// Overwriting a newer revision would silently discard another writer's
// reviewed masks, which is worse than making the operator reconcile.

import CryptoKit
import Foundation

// MARK: - Records

/// Whether a record was produced by an algorithm, confirmed by a person, or
/// examined and rejected. A proposal may never be exported as reference truth.
enum ReviewStatus: String, Codable, Equatable {
    case proposed
    case reviewed
    case rejected
}

/// What could be seen of the object, which is not the same as what was
/// labelled: an object fully occluded in a frame has an empty mask for an
/// understandable reason, and that differs from nobody having looked yet.
enum Visibility: String, Codable, Equatable, CaseIterable {
    case present
    case partlyOccluded = "partly_occluded"
    case fullyOccluded = "fully_occluded"
    case outsideView = "outside_view"
    case unknown

    var label: String {
        switch self {
        case .present: return "Present"
        case .partlyOccluded: return "Partly occluded"
        case .fullyOccluded: return "Fully occluded"
        case .outsideView: return "Outside view"
        case .unknown: return "Unknown"
        }
    }
}

/// Whether a mask claims every visible return of the object in that sample.
/// Even a complete object mask does not establish background negatives.
enum MaskCompleteness: String, Codable, Equatable, CaseIterable {
    case complete
    case partial
    case unreviewed

    var label: String {
        switch self {
        case .complete: return "Complete"
        case .partial: return "Partial"
        case .unreviewed: return "Unreviewed"
        }
    }
}

/// Who or what produced a record. The store supplies revision numbers and the
/// timestamp; it never invents a human author.
struct Provenance: Codable, Equatable {
    var author: String = ""
    var session: String?
    var createdUTC: String = ""
    var operation: String?
    var parentRevision: Int?
    var revision: Int = 0
    var algorithm: String?
    var algorithmVersion: String?

    enum CodingKeys: String, CodingKey {
        case author
        case session
        case createdUTC = "created_utc"
        case operation
        case parentRevision = "parent_revision"
        case revision
        case algorithm
        case algorithmVersion = "algorithm_version"
    }
}

/// A human reference object, identified independently of any predicted track.
struct AnnotationObject: Codable, Equatable, Identifiable {
    var objectID: String
    var objectClass: String
    var subtype: String?
    var confidence: Double = 1.0
    var status: ReviewStatus = .proposed
    var provenance: Provenance = Provenance()
    var notes: String?

    var id: String { objectID }

    enum CodingKeys: String, CodingKey {
        case objectID = "object_id"
        case objectClass = "class"
        case subtype
        case confidence
        case status
        case provenance
        case notes
    }
}

/// An optional physical box. Separate from point membership on purpose: a box
/// includes inferred unseen extent, so it carries its own confidence and
/// cannot be derived from a partial mask's extremes.
struct AnnotationPose: Codable, Equatable {
    var centerX: Double
    var centerY: Double
    var centerZ: Double
    var length: Double
    var width: Double
    var height: Double
    var yawRad: Double
    var axisAmbiguous: Bool = false
    var confidence: Double = 0
    var status: ReviewStatus = .proposed
    var provenance: Provenance = Provenance()

    enum CodingKeys: String, CodingKey {
        case centerX = "center_x"
        case centerY = "center_y"
        case centerZ = "center_z"
        case length
        case width
        case height
        case yawRad = "yaw_rad"
        case axisAmbiguous = "axis_ambiguous"
        case confidence
        case status
        case provenance
    }
}

/// One object's membership in one sample. `pointIndices` are canonical pack
/// indices, ascending, valid only under the pack digest recorded alongside.
struct FrameMask: Codable, Equatable {
    var objectID: String
    var sampleID: Int
    var pointIndices: [Int] = []
    var uncertainIndices: [Int]?
    var completeness: MaskCompleteness = .unreviewed
    var visibility: Visibility = .unknown
    var status: ReviewStatus = .proposed
    var pose: AnnotationPose?
    var provenance: Provenance = Provenance()

    enum CodingKeys: String, CodingKey {
        case objectID = "object_id"
        case sampleID = "sample_id"
        case pointIndices = "point_indices"
        case uncertainIndices = "uncertain_indices"
        case completeness
        case visibility
        case status
        case pose
        case provenance
    }
}

/// An optional link from a human object to predicted tracks. One reference
/// object may map to several tracks, and one merged track to several objects.
struct TrackCorrespondence: Codable, Equatable {
    var objectID: String
    var runID: String?
    var trackIDs: [String] = []
    var startNs: Int64 = 0
    var endNs: Int64 = 0
    var note: String?

    enum CodingKeys: String, CodingKey {
        case objectID = "object_id"
        case runID = "run_id"
        case trackIDs = "track_ids"
        case startNs = "start_ns"
        case endNs = "end_ns"
        case note
    }
}

/// The whole annotation snapshot for one pack.
struct Sidecar: Codable, Equatable {
    /// Must match Go's SidecarSchemaVersion: that reader refuses any other
    /// value outright, so writing a newer number here would lock the Go
    /// tooling out of packs this client has touched.
    static let schemaVersion = 1

    var schemaVersion: Int = Sidecar.schemaVersion
    var datasetID: String = ""
    var packDigest: String = ""
    var revision: Int = 0
    var updatedUTC: String = ""
    var change: Provenance?
    var restoredFrom: Int?
    var objects: [AnnotationObject] = []
    var masks: [FrameMask] = []
    var correspondences: [TrackCorrespondence]?

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case datasetID = "dataset_id"
        case packDigest = "pack_digest"
        case revision
        case updatedUTC = "updated_utc"
        case change
        case restoredFrom = "restored_from"
        case objects
        case masks
        case correspondences
    }

    /// The mask for one object in one sample, if any.
    func mask(objectID: String, sampleID: Int) -> FrameMask? {
        masks.first { $0.objectID == objectID && $0.sampleID == sampleID }
    }

    /// Masks whose object and mask are both reviewed. A mask reviewed under a
    /// still-proposed object is not reference truth, so both gates apply.
    var reviewedMasks: [FrameMask] {
        let reviewedObjects = Set(objects.filter { $0.status == .reviewed }.map(\.objectID))
        return masks.filter { $0.status == .reviewed && reviewedObjects.contains($0.objectID) }
    }

    /// Replaces or inserts a mask, keeping masks ordered by (sample, object)
    /// so an unchanged snapshot serialises to unchanged bytes.
    mutating func upsert(mask: FrameMask) {
        if let idx = masks.firstIndex(where: {
            $0.objectID == mask.objectID && $0.sampleID == mask.sampleID
        }) {
            masks[idx] = mask
        } else {
            masks.append(mask)
        }
        masks.sort {
            $0.sampleID == $1.sampleID ? $0.objectID < $1.objectID : $0.sampleID < $1.sampleID
        }
    }
}

// MARK: - Store

enum SidecarStoreError: Error, Equatable {
    /// Another writer owns the transaction. Retry after it completes.
    case busy
    /// The base changed underneath this edit: reload and reconcile. The
    /// caller's dirty data is never discarded or rewritten by this error.
    case conflict(loadedRevision: Int, currentRevision: Int)
    case malformed(String)
    case unwritable(String)
    case tooLarge(bytes: Int)
    case packMismatch(expected: String, actual: String)
}

/// A loaded snapshot plus the private token needed to save an edit of it.
///
/// The token is the digest of the exact bytes read. Editing the public
/// revision number cannot bypass it, which is what stops a hand-edited
/// revision from overwriting a newer file.
struct SidecarDocument {
    var sidecar: Sidecar
    fileprivate let loadedDigest: String
    fileprivate let loadedRevision: Int
    /// True when no annotations.json existed: an untouched pack starts empty
    /// and the first save must create rather than replace.
    fileprivate let isNew: Bool
}

/// The revision-safe local sidecar store.
final class SidecarStore {
    /// Matches the Go store's per-snapshot ceiling.
    static let maxSnapshotBytes = 64 << 20

    private let packDirectory: URL

    init(packDirectory: URL) { self.packDirectory = packDirectory }

    private var currentURL: URL { packDirectory.appendingPathComponent("annotations.json") }
    private var lockURL: URL { packDirectory.appendingPathComponent(".annotations.lock") }
    private var revisionsDirectory: URL {
        packDirectory.appendingPathComponent("annotation-revisions")
    }

    private static func revisionFilename(_ revision: Int) -> String {
        String(format: "%010d.json", revision)
    }

    private static let encoder: JSONEncoder = {
        let e = JSONEncoder()
        // Match the Go writer's two-space indentation and stable key order so
        // a snapshot round-tripped through either writer keeps its shape.
        e.outputFormatting = [.prettyPrinted, .sortedKeys]
        return e
    }()

    /// Reads the current snapshot and its concurrency token. An untouched
    /// pack returns an empty document rather than an error.
    func load(packDigest: String? = nil, datasetID: String? = nil) throws -> SidecarDocument {
        guard FileManager.default.fileExists(atPath: currentURL.path) else {
            // A pack with retained history but no current file is a damaged
            // head, not a fresh start: refusing here stops a new save from
            // quietly replacing lost reviewed masks.
            if let entries = try? FileManager.default.contentsOfDirectory(
                atPath: revisionsDirectory.path), !entries.isEmpty
            {
                throw SidecarStoreError.malformed(
                    "annotations.json is missing but \(entries.count) retained revision(s) exist; recover explicitly"
                )
            }
            var fresh = Sidecar()
            fresh.packDigest = packDigest ?? ""
            fresh.datasetID = datasetID ?? ""
            return SidecarDocument(sidecar: fresh, loadedDigest: "", loadedRevision: 0, isNew: true)
        }

        let data: Data
        do { data = try Data(contentsOf: currentURL) } catch {
            throw SidecarStoreError.malformed("read annotations.json: \(error)")
        }
        guard data.count <= SidecarStore.maxSnapshotBytes else {
            throw SidecarStoreError.tooLarge(bytes: data.count)
        }
        let sidecar: Sidecar
        do { sidecar = try JSONDecoder().decode(Sidecar.self, from: data) } catch {
            // A corrupt current file opens with an error, never as an empty
            // dataset: an empty dataset would look like "nothing labelled yet".
            throw SidecarStoreError.malformed("decode annotations.json: \(error)")
        }
        if let packDigest, !sidecar.packDigest.isEmpty, sidecar.packDigest != packDigest {
            throw SidecarStoreError.packMismatch(expected: packDigest, actual: sidecar.packDigest)
        }

        return SidecarDocument(
            sidecar: sidecar, loadedDigest: AnnotationPack.digest(data),
            loadedRevision: sidecar.revision, isNew: false)
    }

    /// Saves an edit of the loaded document, advancing its revision.
    ///
    /// The caller does not set the revision: the store does, after proving the
    /// file still holds the bytes the caller loaded.
    @discardableResult func save(
        _ document: SidecarDocument, change: Provenance
    ) throws -> SidecarDocument {
        let lock = try FileLock(url: lockURL)
        defer { lock.release() }
        guard lock.acquired else { throw SidecarStoreError.busy }

        // Re-read under the lock. Between load and save another writer may
        // have committed, and the digest is what detects it.
        let onDisk =
            FileManager.default.fileExists(atPath: currentURL.path)
            ? try? Data(contentsOf: currentURL) : nil

        if document.isNew {
            // A fresh document cannot overwrite an existing one.
            if onDisk != nil {
                let currentRevision =
                    (try? JSONDecoder().decode(Sidecar.self, from: onDisk!))?.revision ?? -1
                throw SidecarStoreError.conflict(
                    loadedRevision: 0, currentRevision: currentRevision)
            }
        } else {
            guard let onDisk else {
                throw SidecarStoreError.malformed(
                    "annotations.json disappeared while editing revision \(document.loadedRevision)"
                )
            }
            let digest = AnnotationPack.digest(onDisk)
            guard digest == document.loadedDigest else {
                let currentRevision =
                    (try? JSONDecoder().decode(Sidecar.self, from: onDisk))?.revision ?? -1
                throw SidecarStoreError.conflict(
                    loadedRevision: document.loadedRevision, currentRevision: currentRevision)
            }
        }

        var next = document.sidecar
        next.schemaVersion = Sidecar.schemaVersion
        next.revision = document.loadedRevision + 1
        next.updatedUTC = SidecarStore.utcTimestamp()
        var stamped = change
        stamped.parentRevision = document.loadedRevision
        stamped.revision = next.revision
        if stamped.createdUTC.isEmpty { stamped.createdUTC = next.updatedUTC }
        next.change = stamped
        // An ordinary save clears any restore marker: this snapshot is an
        // edit in its own right, not a recovered one.
        next.restoredFrom = nil

        let encoded: Data
        do { encoded = try SidecarStore.encoder.encode(next) } catch {
            throw SidecarStoreError.unwritable("encode snapshot: \(error)")
        }
        guard encoded.count <= SidecarStore.maxSnapshotBytes else {
            throw SidecarStoreError.tooLarge(bytes: encoded.count)
        }

        // Archive the exact prior bytes before the current file advances. A
        // failed archive write leaves the current revision unchanged.
        if let onDisk, !document.isNew { try archive(onDisk, revision: document.loadedRevision) }
        try atomicReplace(currentURL, with: encoded)

        return SidecarDocument(
            sidecar: next, loadedDigest: AnnotationPack.digest(encoded),
            loadedRevision: next.revision, isNew: false)
    }

    /// Reads a retained revision without making it current.
    func loadRevision(_ revision: Int) throws -> Sidecar {
        let url = revisionsDirectory.appendingPathComponent(SidecarStore.revisionFilename(revision))
        guard let data = try? Data(contentsOf: url) else {
            throw SidecarStoreError.malformed("revision \(revision) is not retained")
        }
        do { return try JSONDecoder().decode(Sidecar.self, from: data) } catch {
            throw SidecarStoreError.malformed("decode revision \(revision): \(error)")
        }
    }

    /// Restores a retained snapshot as a new revision, keeping its membership
    /// and review status. Restoring revision 1 over revision 2 produces
    /// revision 3; it never rewrites an earlier file, and it never converts a
    /// proposal into a reviewed label.
    @discardableResult func restore(revision: Int, change: Provenance) throws -> SidecarDocument {
        let restored = try loadRevision(revision)
        let current = try load()
        var document = current
        document.sidecar.objects = restored.objects
        document.sidecar.masks = restored.masks
        document.sidecar.correspondences = restored.correspondences
        var stamped = change
        if stamped.operation == nil { stamped.operation = "restore" }
        var saved = try save(document, change: stamped)
        // Mark the provenance of the restore itself, so a later reader can
        // see this snapshot's membership came from an earlier revision.
        saved.sidecar.restoredFrom = revision
        return saved
    }

    /// Revisions retained on disk, ascending.
    func retainedRevisions() -> [Int] {
        guard
            let entries = try? FileManager.default.contentsOfDirectory(
                atPath: revisionsDirectory.path)
        else { return [] }
        return entries.compactMap { name in
            guard name.hasSuffix(".json") else { return nil }
            return Int(name.dropLast(5))
        }.sorted()
    }

    // MARK: - Private file mechanics

    private func archive(_ bytes: Data, revision: Int) throws {
        do {
            try FileManager.default.createDirectory(
                at: revisionsDirectory, withIntermediateDirectories: true)
        } catch { throw SidecarStoreError.unwritable("create annotation-revisions: \(error)") }
        let url = revisionsDirectory.appendingPathComponent(SidecarStore.revisionFilename(revision))
        // Never overwrite a retained snapshot: history is the recovery path.
        guard !FileManager.default.fileExists(atPath: url.path) else { return }
        try atomicReplace(url, with: bytes)
    }

    /// Unique temporary file, sync, atomic rename, then directory sync.
    private func atomicReplace(_ destination: URL, with bytes: Data) throws {
        let directory = destination.deletingLastPathComponent()
        let temp = directory.appendingPathComponent(
            ".\(destination.lastPathComponent).\(UUID().uuidString).tmp")
        do {
            try bytes.write(to: temp, options: .atomic)
            // Flush the file's own bytes before the rename makes it visible.
            let handle = try FileHandle(forWritingTo: temp)
            try handle.synchronize()
            try handle.close()
            _ = try FileManager.default.replaceItemAt(destination, withItemAt: temp)
        } catch {
            try? FileManager.default.removeItem(at: temp)
            throw SidecarStoreError.unwritable("replace \(destination.lastPathComponent): \(error)")
        }
        // Sync the directory so the rename itself is durable.
        let dirFD = open(directory.path, O_RDONLY)
        if dirFD >= 0 {
            _ = fsync(dirFD)
            close(dirFD)
        }
    }

    static func utcTimestamp() -> String {
        let formatter = ISO8601DateFormatter()
        formatter.timeZone = TimeZone(identifier: "UTC")
        return formatter.string(from: Date())
    }
}

/// A non-blocking kernel file lock over a persistent lock inode.
///
/// The kernel releases the lock when the descriptor closes or the process
/// exits, so a crashed writer does not leave the pack permanently locked. The
/// lock file itself is never deleted: removing it while another writer holds
/// it would let two writers each hold a lock on a different inode.
final class FileLock {
    private let fd: Int32
    let acquired: Bool

    init(url: URL) throws {
        fd = open(url.path, O_CREAT | O_RDWR, 0o644)
        guard fd >= 0 else {
            throw SidecarStoreError.unwritable("open \(url.lastPathComponent): errno \(errno)")
        }
        acquired = flock(fd, LOCK_EX | LOCK_NB) == 0
    }

    func release() {
        if acquired { _ = flock(fd, LOCK_UN) }
        close(fd)
    }
}
