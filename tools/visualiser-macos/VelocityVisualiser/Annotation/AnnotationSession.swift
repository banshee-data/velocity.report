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
    @Published private(set) var currentPoints: PackPoints = PackPoints() {
        didSet { sceneRevision &+= 1 }
    }

    // MARK: Selection

    @Published private(set) var history = MembershipHistory() { didSet { sceneRevision &+= 1 } }
    /// The candidate set of the gesture being previewed, so the operator sees
    /// the count before committing.
    @Published private(set) var pendingCandidates: SelectionCandidates? {
        didSet { sceneRevision &+= 1 }
    }
    /// True between the first drag point and commit or cancel.
    @Published private(set) var strokeInProgress = false

    @Published var viewStandard: OrthoViewBasis.Standard = .top
    /// The confirming view: selection must be inspected from a second angle
    /// before a mask is marked reviewed.
    @Published var secondViewStandard: OrthoViewBasis.Standard = .front
    @Published var slab: DepthSlab?
    /// True once the operator has set the slab themselves. A slab set by hand
    /// is kept across samples, for the same reason the framing is: the ground
    /// an operator excluded in one sample is the ground in the next.
    @Published private(set) var slabIsPinned = false
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

    // MARK: Display

    /// Which point classes are drawn. Hidden classes are not selectable: see
    /// `PointVisibility`.
    @Published var visibility = PointVisibility() {
        didSet {
            sceneRevision &+= 1
            // A stroke previewed under the old filter names points the new
            // one may hide.
            if visibility != oldValue { cancelStroke() }
        }
    }

    /// What each orthographic view frames until the operator moves it. Set
    /// when a pack is opened and when a fit is asked for, never by stepping:
    /// the scale an operator is working at must survive the next sample.
    @Published private(set) var referenceExtents: [OrthoViewBasis.Standard: AnnotationExtent] = [:]
    /// Where the operator has panned and zoomed each view to. Takes precedence
    /// over the reference extent, and is likewise untouched by stepping.
    @Published private(set) var viewStates: [OrthoViewBasis.Standard: OrthoViewState] = [:]
    /// The region the 3D view was last asked to look at.
    @Published private(set) var sceneFocus: AnnotationSceneFocus?

    /// Bumped whenever what the 3D view draws has changed, so that view can
    /// tell a change of content from one of the many publishes that are not.
    private(set) var sceneRevision = 0

    // MARK: Frame sync

    /// Keep this window and the main view on the same frame, in both
    /// directions: stepping here seeks the main view, and moving the main view
    /// steps here.
    @Published var syncWithMainView = true { didSet { if !syncWithMainView { syncStatus = .off } } }
    @Published private(set) var syncStatus: FrameSyncStatus = .off
    /// Counts the steps the operator made in this window, as distinct from
    /// steps made to follow the main view.
    @Published private(set) var operatorNavigationRevision = 0

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
        fitViews(to: .sample)
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
        step(to: index, followingMainView: false)
    }

    private func step(to index: Int, followingMainView: Bool) -> AnnotationGuard? {
        if let blocker = navigationGuard() { return blocker }
        guard index >= 0, index < orderedSamples.count else { return nil }
        sampleIndex = index
        // Only a step the operator made here asks the main view to come along.
        // One made to follow the main view must not: during playback the main
        // view has already moved on, and seeking it back to this sample would
        // have the two windows drag each other backwards.
        if !followingMainView { operatorNavigationRevision &+= 1 }
        currentPoints = (try? pack.points(sampleID: orderedSamples[index].sampleID)) ?? PackPoints()
        pendingCandidates = nil
        if !slabIsPinned { resetSlabToSampleExtent() }
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
        let candidates = visibleOnly(
            PointSelectionEngine.candidates(
                points: currentPoints, basis: basis, polygon: polygon, slab: slab))
        pendingCandidates = candidates
        return candidates
    }

    /// Evaluates a sphere without applying it. The view it was made in decides
    /// what the slab means, as it does for the lasso.
    @discardableResult func previewSelection(sphere: SelectionSphere) -> SelectionCandidates {
        let candidates = visibleOnly(
            PointSelectionEngine.candidates(
                points: currentPoints, sphere: sphere, basis: OrthoViewBasis(viewStandard),
                slab: slab))
        pendingSphere = sphere
        pendingCandidates = candidates
        return candidates
    }

    /// Evaluates a set of painted columns without applying it.
    @discardableResult func previewSelection(cells: Set<ColumnCell>) -> SelectionCandidates {
        let candidates = visibleOnly(
            PointSelectionEngine.candidates(
                points: currentPoints, cells: cells, grid: columnGrid, enabledVoxels: enabledVoxels,
                basis: OrthoViewBasis(viewStandard), slab: slab))
        pendingCells = cells
        pendingCandidates = candidates
        return candidates
    }

    /// The return nearest a position in the editable view, within the slab:
    /// where a sphere is centred. `maxViewDistance` is in metres.
    func nearestPointIndex(toViewPoint viewPoint: simd_float2, maxViewDistance: Float) -> Int? {
        let filter = effectiveVisibility
        return PointSelectionEngine.nearestPoint(
            points: currentPoints, basis: OrthoViewBasis(viewStandard), viewPoint: viewPoint,
            slab: slab, maxViewDistance: maxViewDistance,
            where: { self.currentPoints.isVisible($0, under: filter) })
    }

    /// The class filter in force, or nil when nothing is filtered: every class
    /// is on, or the pack carries no classes to filter by.
    var effectiveVisibility: PointVisibility? {
        guard pack.manifest.hasClassification, !visibility.showsEverything else { return nil }
        return visibility
    }

    /// Drops candidates the class filter hides, and counts them, so a gesture
    /// that took fewer points than it covered says why.
    private func visibleOnly(_ candidates: SelectionCandidates) -> SelectionCandidates {
        guard let filter = effectiveVisibility else { return candidates }
        var result = candidates
        result.indices = candidates.indices.filter { currentPoints.isVisible($0, under: filter) }
        result.excludedByVisibility = candidates.indices.count - result.indices.count
        return result
    }

    /// Makes the active brush larger or smaller by whole steps: the bracket
    /// keys. The lasso has no size, so it ignores them.
    ///
    /// `eventTimestamp` is the key press that asked. The brackets are bound
    /// both in this window and in the app's Playback menu, and which of the
    /// two AppKit offers the key to first is not something to depend on, so a
    /// press that arrives by both routes is applied once.
    func adjustBrushSize(steps: Int, eventTimestamp: TimeInterval? = nil) {
        if let eventTimestamp {
            guard eventTimestamp != lastBrushKeyTimestamp else { return }
            lastBrushKeyTimestamp = eventTimestamp
        }
        switch tool {
        case .lasso: return
        case .sphere: sphereRadius = BrushStroke.steppedSphereRadius(sphereRadius, steps: steps)
        case .column:
            columnBrushRadius = BrushStroke.steppedColumnRadius(columnBrushRadius, steps: steps)
        }
    }

    private var lastBrushKeyTimestamp: TimeInterval?

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

    /// Sets the slab by hand, which also pins it across samples.
    func setSlab(_ newSlab: DepthSlab) {
        slab = newSlab
        slabIsPinned = true
    }

    /// Back to the sample's full depth, and back to following each sample.
    func unpinSlab() {
        slabIsPinned = false
        resetSlabToSampleExtent()
    }

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

    // MARK: - Framing

    /// Fraction of returns left out on each side when framing a whole sample.
    static let sampleFitTrim: Float = 0.01

    /// The viewport a view of `size` shows: where the operator left it, or
    /// else the reference extent framed to fit.
    func viewport(for standard: OrthoViewBasis.Standard, size: CGSize) -> OrthoViewport {
        if let state = viewStates[standard] {
            return OrthoViewport(halfHeight: state.halfHeight, size: size, centre: state.centre)
        }
        guard let extent = referenceExtents[standard] else {
            return OrthoViewport(halfHeight: 10, size: size, centre: .zero)
        }
        return OrthoViewport(
            halfHeight: annotationFramingHalfHeight(extent: extent, size: size), size: size,
            centre: extent.centre)
    }

    /// Pans a view by a drag of `delta` screen points.
    func pan(_ standard: OrthoViewBasis.Standard, size: CGSize, byPoints delta: CGSize) {
        guard size.height > 0 else { return }
        var state = viewState(standard, size: size)
        state.pan(byPoints: delta, metresPerPoint: state.halfHeight * 2 / Float(size.height))
        viewStates[standard] = state
    }

    /// Zooms a view by `factor` about a screen point, which stays where it is.
    func zoom(
        _ standard: OrthoViewBasis.Standard, size: CGSize, by factor: Float,
        aboutScreenPoint anchor: CGPoint
    ) {
        let world = viewport(for: standard, size: size).worldPoint(from: anchor)
        var state = viewState(standard, size: size)
        state.zoom(by: factor, about: world)
        viewStates[standard] = state
    }

    private func viewState(_ standard: OrthoViewBasis.Standard, size: CGSize) -> OrthoViewState {
        let current = viewport(for: standard, size: size)
        return OrthoViewState(centre: current.centre, halfHeight: current.halfHeight)
    }

    /// Frames every view, and the 3D view, on the chosen points. Returns false
    /// and leaves the views alone when there are none to frame.
    @discardableResult func fitViews(to target: AnnotationFitTarget) -> Bool {
        let filter = effectiveVisibility
        let selection = history.current
        let include: (Int) -> Bool
        let trim: Float
        switch target {
        case .sample:
            include = { self.currentPoints.isVisible($0, under: filter) }
            trim = AnnotationSession.sampleFitTrim
        case .foreground:
            include = { index in
                index < self.currentPoints.classification.count
                    && self.currentPoints.classification[index] == PointClass.foreground
            }
            trim = 0
        case .selection:
            include = { selection.contains($0) }
            trim = 0
        }

        var extents: [OrthoViewBasis.Standard: AnnotationExtent] = [:]
        for standard in OrthoViewBasis.Standard.allCases {
            guard
                let extent = annotationExtent(
                    of: currentPoints, basis: OrthoViewBasis(standard), trim: trim, where: include)
            else { return false }
            extents[standard] = extent
        }
        referenceExtents = extents
        viewStates = [:]

        // Top gives X and Y, front gives Z: together, the box in the world.
        if let top = extents[.top], let front = extents[.front] {
            sceneFocus = AnnotationSceneFocus(
                centre: simd_float3(top.centre.x, top.centre.y, front.centre.y),
                radius: max(top.halfWidth, top.halfHeight, front.halfHeight),
                revision: (sceneFocus?.revision ?? 0) + 1)
        }
        return true
    }

    // MARK: - Frame sync

    /// Follows the main view to the frame it is showing. Returns true when
    /// this window stepped.
    @discardableResult func follow(_ main: MainViewPlayback) -> Bool {
        guard syncWithMainView else { return false }
        guard let target = syncTarget(for: main) else { return false }
        guard target != sampleIndex else {
            syncStatus = .inSync
            return false
        }
        guard step(to: target, followingMainView: true) == nil else {
            syncStatus = .heldByUnsavedChanges
            return false
        }
        syncStatus = .inSync
        return true
    }

    /// The timestamp the main view should seek to so that it shows this
    /// window's sample, or nil when it is already there or cannot follow.
    func seekTarget(for main: MainViewPlayback) -> Int64? {
        guard syncWithMainView, let sample = currentSample else { return nil }
        guard main.seekable else {
            syncStatus = .mainNotSeekable
            return nil
        }
        guard
            AnnotationFrameSync.replayCovers(
                orderedSamples, logStartNs: main.logStartNs, logEndNs: main.logEndNs)
        else {
            syncStatus = .differentRecording
            return nil
        }
        syncStatus = .inSync
        guard abs(main.timestampNs - sample.timestampNs) > AnnotationFrameSync.toleranceNs else {
            return nil
        }
        return sample.timestampNs
    }

    /// The sample the main view's frame corresponds to, recording why not when
    /// there is none.
    private func syncTarget(for main: MainViewPlayback) -> Int? {
        guard main.seekable else {
            syncStatus = .mainNotSeekable
            return nil
        }
        guard
            AnnotationFrameSync.replayCovers(
                orderedSamples, logStartNs: main.logStartNs, logEndNs: main.logEndNs)
        else {
            syncStatus = .differentRecording
            return nil
        }
        guard
            let nearest = AnnotationFrameSync.nearestSample(
                to: main.timestampNs, in: orderedSamples),
            nearest.deltaNs <= AnnotationFrameSync.toleranceNs
        else {
            syncStatus = .outsidePack
            return nil
        }
        return nearest.index
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
