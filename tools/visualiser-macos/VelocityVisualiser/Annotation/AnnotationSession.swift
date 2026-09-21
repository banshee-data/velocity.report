// AnnotationSession.swift
// Operator state for one annotation pack: which object is being labelled,
// which sample is shown, what is selected, and whether that is saved.
//
// The session owns the rules the workflow depends on and the renderer does
// not: an unfinished stroke is not retargeted by navigation, stepping to the
// next sample keeps the object identity but not the previous frame's point
// indices, and a dirty session warns before it can be lost.

import Combine
import Foundation
import simd

/// Why a navigation or source change was refused, so the UI can say which.
enum AnnotationGuard: Equatable {
    case strokeInProgress
    case unsavedMembership(sampleID: Int, objectID: String)
}

/// Observable annotation state. `@MainActor` because the selection it holds is
/// read directly by the renderer's draw pass and mutated from UI callbacks.
@MainActor final class AnnotationSession: ObservableObject {
    // MARK: Source

    let pack: AnnotationPack
    private let store: SidecarStore

    /// The current saved snapshot plus its concurrency token.
    private var document: SidecarDocument

    @Published private(set) var sidecar: Sidecar

    // MARK: Position in the pack

    @Published private(set) var sampleIndex: Int = 0

    var samples: [AnnotationSample] { orderedSamples }
    private let orderedSamples: [AnnotationSample]

    var currentSample: AnnotationSample? {
        guard sampleIndex >= 0, sampleIndex < orderedSamples.count else { return nil }
        return orderedSamples[sampleIndex]
    }

    /// The decoded points for the current sample, cached so a redraw or a
    /// candidate evaluation does not re-read and re-decode the block.
    @Published private(set) var currentPoints: PackPoints = PackPoints()

    // MARK: Selection

    @Published private(set) var history = MembershipHistory()
    /// The candidate set of the gesture being previewed, so the operator sees
    /// the count before committing.
    @Published private(set) var pendingCandidates: SelectionCandidates?
    /// True between the first drag point and commit or cancel.
    @Published private(set) var strokeInProgress = false

    @Published var viewStandard: OrthoViewBasis.Standard = .top
    /// The confirming view: selection must be inspected from a second angle
    /// before a mask is marked reviewed.
    @Published var secondViewStandard: OrthoViewBasis.Standard = .front
    @Published var slab: DepthSlab?
    @Published var selectionMode: SelectionMode = .replace

    /// Which selection tool a gesture in the editable view uses.
    @Published var tool: SelectionTool = .lasso
    /// The local column lattice. Its ground is estimated once when a pack is
    /// opened and then left alone as samples are stepped through: road users
    /// move on one plane, and a ground that moved between samples would make
    /// the same column mean a different slice of the same object.
    @Published var columnGrid = ColumnGrid()
    /// Voxels the column brush selects from, bit `k` for voxel `k`.
    @Published var enabledVoxels: UInt8 = ColumnGrid.stackMask
    /// The sphere brush's last radius, repeated by a plain click.
    @Published var sphereRadius: Float = SelectionSphere.defaultRadius
    /// Column brush radius in metres. Zero paints one column at a time.
    @Published var columnBrushRadius: Float = 0
    /// Draw the lattice in the top view even when the column brush is not the
    /// active tool.
    @Published var showColumnGrid = false

    /// The sphere and the columns of the stroke in progress. Published, not
    /// held by the view that is being dragged in, so that the second view can
    /// draw them too: a sphere's reach along the axis the operator cannot see
    /// is the thing the second view is for.
    @Published private(set) var pendingSphere: SelectionSphere?
    @Published private(set) var pendingCells: Set<ColumnCell> = []

    // MARK: Object under edit

    @Published var activeObjectID: String?
    @Published var maskVisibility: Visibility = .present
    @Published var maskCompleteness: MaskCompleteness = .partial

    /// Samples whose current selection differs from what is saved.
    @Published private(set) var dirtySamples: Set<Int> = []

    @Published private(set) var lastError: String?
    /// Set when a save was refused because another writer advanced the file.
    @Published private(set) var conflict: SidecarStoreError?

    /// Operator identity, collected rather than invented. A save without it
    /// is refused: anonymous legacy data stays readable, but this client does
    /// not add more of it.
    @Published var operatorName: String = ""
    let sessionID: String = UUID().uuidString

    // MARK: Init

    init(pack: AnnotationPack, store: SidecarStore? = nil) throws {
        self.pack = pack
        let resolvedStore = store ?? SidecarStore(packDirectory: pack.directory)
        self.store = resolvedStore
        self.document = try resolvedStore.load(
            packDigest: pack.manifest.packDigest, datasetID: pack.manifest.datasetID)
        self.sidecar = document.sidecar
        self.orderedSamples = pack.chronologicalSamples
        if let first = orderedSamples.first {
            self.currentPoints = (try? pack.points(sampleID: first.sampleID)) ?? PackPoints()
        }
        resetSlabToSampleExtent()
        // Once, from the first sample, and not again as samples are stepped
        // through: see columnGrid.
        estimateGround()
    }

    // MARK: - Objects

    /// Creates a reference object. It starts proposed: a person confirms it
    /// explicitly, and nothing this client creates is reference truth on
    /// arrival.
    func createObject(objectClass: String, subtype: String? = nil) -> AnnotationObject {
        let object = AnnotationObject(
            objectID: "obj_" + UUID().uuidString.prefix(8).lowercased(), objectClass: objectClass,
            subtype: subtype, confidence: 1.0, status: .proposed,
            provenance: Provenance(
                author: operatorName, session: sessionID, createdUTC: SidecarStore.utcTimestamp(),
                operation: "create_object"))
        sidecar.objects.append(object)
        activeObjectID = object.objectID
        loadSelectionForCurrentSample()
        return object
    }

    var activeObject: AnnotationObject? {
        guard let activeObjectID else { return nil }
        return sidecar.objects.first { $0.objectID == activeObjectID }
    }

    /// Marks the active object reviewed. Requires the second-view check,
    /// because a membership that has only been seen from one angle has not
    /// actually been inspected for contamination.
    func markObjectReviewed(secondViewConfirmed: Bool) -> Bool {
        guard secondViewConfirmed, let id = activeObjectID,
            let idx = sidecar.objects.firstIndex(where: { $0.objectID == id })
        else {
            lastError = "Confirm the selection in the second view before marking it reviewed."
            return false
        }
        sidecar.objects[idx].status = .reviewed
        sidecar.objects[idx].provenance = Provenance(
            author: operatorName, session: sessionID, createdUTC: SidecarStore.utcTimestamp(),
            operation: "review_object")
        return true
    }

    // MARK: - Navigation

    /// Steps to another sample, keeping the object identity but never the
    /// previous frame's point indices: a point index means a return in one
    /// scan, and carrying it across would fabricate membership.
    @discardableResult func step(to index: Int) -> AnnotationGuard? {
        if let blocker = navigationGuard() { return blocker }
        guard index >= 0, index < orderedSamples.count else { return nil }
        sampleIndex = index
        currentPoints = (try? pack.points(sampleID: orderedSamples[index].sampleID)) ?? PackPoints()
        pendingCandidates = nil
        resetSlabToSampleExtent()
        loadSelectionForCurrentSample()
        return nil
    }

    @discardableResult func stepForward() -> AnnotationGuard? { step(to: sampleIndex + 1) }

    @discardableResult func stepBackward() -> AnnotationGuard? { step(to: sampleIndex - 1) }

    /// What would block leaving this sample, if anything. Exposed so a caller
    /// can ask before offering navigation rather than after refusing it.
    func navigationGuard() -> AnnotationGuard? {
        if strokeInProgress { return .strokeInProgress }
        if let sample = currentSample, let objectID = activeObjectID,
            dirtySamples.contains(sample.sampleID)
        {
            return .unsavedMembership(sampleID: sample.sampleID, objectID: objectID)
        }
        return nil
    }

    /// Abandons the in-progress stroke, the explicit way past the guard.
    func cancelStroke() {
        strokeInProgress = false
        pendingCandidates = nil
        pendingSphere = nil
        pendingCells = []
    }

    // MARK: - Selection

    /// Begins a stroke. Held so navigation cannot retarget it midway.
    func beginStroke() { strokeInProgress = true }

    /// Evaluates a gesture without applying it, so the candidate count can be
    /// shown before acceptance.
    @discardableResult func previewSelection(polygon: SelectionPolygon) -> SelectionCandidates {
        let basis = OrthoViewBasis(viewStandard)
        let candidates = PointSelectionEngine.candidates(
            points: currentPoints, basis: basis, polygon: polygon, slab: slab)
        pendingCandidates = candidates
        return candidates
    }

    /// Evaluates a sphere without applying it. The view it was made in decides
    /// what the slab means, as it does for the lasso.
    @discardableResult func previewSelection(sphere: SelectionSphere) -> SelectionCandidates {
        let candidates = PointSelectionEngine.candidates(
            points: currentPoints, sphere: sphere, basis: OrthoViewBasis(viewStandard), slab: slab)
        pendingSphere = sphere
        pendingCandidates = candidates
        return candidates
    }

    /// Evaluates a set of painted columns without applying it.
    @discardableResult func previewSelection(cells: Set<ColumnCell>) -> SelectionCandidates {
        let candidates = PointSelectionEngine.candidates(
            points: currentPoints, cells: cells, grid: columnGrid, enabledVoxels: enabledVoxels,
            basis: OrthoViewBasis(viewStandard), slab: slab)
        pendingCells = cells
        pendingCandidates = candidates
        return candidates
    }

    /// The return nearest a position in the editable view, within the slab:
    /// where a sphere is centred. `maxViewDistance` is in metres.
    func nearestPointIndex(toViewPoint viewPoint: simd_float2, maxViewDistance: Float) -> Int? {
        PointSelectionEngine.nearestPoint(
            points: currentPoints, basis: OrthoViewBasis(viewStandard), viewPoint: viewPoint,
            slab: slab, maxViewDistance: maxViewDistance)
    }

    /// Makes the active brush larger or smaller by whole steps: the bracket
    /// keys. The lasso has no size, so it ignores them.
    func adjustBrushSize(steps: Int) {
        switch tool {
        case .lasso: return
        case .sphere: sphereRadius = BrushStroke.steppedSphereRadius(sphereRadius, steps: steps)
        case .column:
            columnBrushRadius = BrushStroke.steppedColumnRadius(columnBrushRadius, steps: steps)
        }
    }

    /// Sets the grid's ground from the current sample.
    func estimateGround() {
        if let z = ColumnGrid.estimateGroundZ(points: currentPoints) { columnGrid.groundZ = z }
    }

    /// Applies the previewed gesture to the membership under the current mode.
    @discardableResult func commitSelection() -> Bool {
        guard let candidates = pendingCandidates else { return false }
        let next = PointSelectionEngine.apply(
            mode: selectionMode, candidates: candidates.indices, to: history.current)
        let changed = history.commit(next)
        strokeInProgress = false
        pendingCandidates = nil
        pendingSphere = nil
        pendingCells = []
        if changed { markDirty() }
        return changed
    }

    /// Runs a gesture end to end. Convenience for the keyboard and test paths
    /// that do not need the intermediate preview.
    @discardableResult func select(polygon: SelectionPolygon, mode: SelectionMode? = nil) -> Bool {
        if let mode { selectionMode = mode }
        beginStroke()
        previewSelection(polygon: polygon)
        return commitSelection()
    }

    func undo() { if history.undo() { markDirty() } }

    func redo() { if history.redo() { markDirty() } }

    func clearSelection() { if history.commit([]) { markDirty() } }

    var selectionCount: Int { history.current.count }

    /// Membership as the canonical stored form.
    var canonicalSelection: [Int] { PointSelectionEngine.canonicalIndices(history.current) }

    // MARK: - Slab

    /// Resets the slab to the current sample's full depth extent, so a fresh
    /// sample never starts with an invisible selection volume.
    func resetSlabToSampleExtent() {
        guard currentPoints.count > 0 else {
            slab = nil
            return
        }
        let basis = OrthoViewBasis(viewStandard)
        var lo = Float.greatestFiniteMagnitude
        var hi = -Float.greatestFiniteMagnitude
        for i in 0..<currentPoints.count {
            guard let p = currentPoints.point(at: i), p.x.isFinite, p.y.isFinite, p.z.isFinite
            else { continue }
            let d = basis.depth(p)
            lo = min(lo, d)
            hi = max(hi, d)
        }
        slab = lo <= hi ? DepthSlab(minDepth: lo, maxDepth: hi) : nil
    }

    // MARK: - Persistence

    /// Writes the current membership into the snapshot and saves it.
    ///
    /// Validation runs before anything is written: an out-of-range or
    /// duplicated index is a wrong label, not something to clamp.
    @discardableResult func save() -> Bool {
        lastError = nil
        conflict = nil
        guard !operatorName.trimmingCharacters(in: .whitespaces).isEmpty else {
            lastError = "Enter an operator name before saving: provenance is not invented."
            return false
        }
        guard let sample = currentSample, let objectID = activeObjectID else {
            lastError = "Select a reference object and a sample before saving."
            return false
        }

        let indices = canonicalSelection
        switch PointSelectionEngine.validate(indices: indices, pointCount: currentPoints.count) {
        case .failure(let error):
            lastError = error.message
            return false
        case .success(let validated):
            let change = Provenance(
                author: operatorName, session: sessionID, createdUTC: SidecarStore.utcTimestamp(),
                operation: "save_mask")
            var mask =
                sidecar.mask(objectID: objectID, sampleID: sample.sampleID)
                ?? FrameMask(objectID: objectID, sampleID: sample.sampleID)
            mask.pointIndices = validated
            mask.completeness = maskCompleteness
            mask.visibility = maskVisibility
            // Saving membership does not review it. A mask becomes reviewed
            // through an explicit second-view confirmation.
            mask.status = mask.status == .reviewed ? .reviewed : .proposed
            mask.provenance = change

            var edited = document
            edited.sidecar = sidecar
            edited.sidecar.packDigest = pack.manifest.packDigest
            edited.sidecar.datasetID = pack.manifest.datasetID
            edited.sidecar.upsert(mask: mask)

            do {
                document = try store.save(edited, change: change)
                sidecar = document.sidecar
                dirtySamples.remove(sample.sampleID)
                return true
            } catch let error as SidecarStoreError {
                // Neither busy nor conflict discards the operator's dirty
                // membership: it stays in the history for reconciliation.
                conflict = error
                lastError = AnnotationSession.describe(error)
                return false
            } catch {
                lastError = "\(error)"
                return false
            }
        }
    }

    /// Re-reads the saved snapshot, discarding unsaved membership for the
    /// current sample. The explicit way to resolve a conflict after the
    /// operator has decided the other writer wins.
    func reload() {
        do {
            document = try store.load(
                packDigest: pack.manifest.packDigest, datasetID: pack.manifest.datasetID)
            sidecar = document.sidecar
            dirtySamples.removeAll()
            conflict = nil
            lastError = nil
            loadSelectionForCurrentSample()
        } catch { lastError = "\(error)" }
    }

    static func describe(_ error: SidecarStoreError) -> String {
        switch error {
        case .busy: return "Another writer holds the annotation lock. Retry when it finishes."
        case .conflict(let loaded, let current):
            return
                "This pack advanced to revision \(current) while you edited revision \(loaded). Your selection is kept; reconcile before saving."
        case .malformed(let detail): return "Annotation file problem: \(detail)"
        case .unwritable(let detail): return "Could not write: \(detail)"
        case .tooLarge(let bytes): return "Snapshot is \(bytes) bytes, above the 64 MiB limit."
        case .packMismatch(let expected, let actual):
            return "Annotations belong to pack \(actual), not \(expected)."
        }
    }

    // MARK: - Private

    private func markDirty() {
        guard let sample = currentSample else { return }
        let saved = Set(
            activeObjectID.flatMap { sidecar.mask(objectID: $0, sampleID: sample.sampleID) }?
                .pointIndices ?? [])
        if saved == history.current {
            dirtySamples.remove(sample.sampleID)
        } else {
            dirtySamples.insert(sample.sampleID)
        }
    }

    /// Loads the saved mask for the active object into the history, clearing
    /// stroke history so undo cannot walk into another sample's selection.
    private func loadSelectionForCurrentSample() {
        guard let sample = currentSample, let objectID = activeObjectID,
            let mask = sidecar.mask(objectID: objectID, sampleID: sample.sampleID)
        else {
            history.reset(to: [])
            return
        }
        history.reset(to: Set(mask.pointIndices))
        maskVisibility = mask.visibility
        maskCompleteness = mask.completeness
    }
}
