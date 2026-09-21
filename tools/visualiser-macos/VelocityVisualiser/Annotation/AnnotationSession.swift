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

private let sessionLogger = DevLogger(category: "AnnotationSession")

/// Why a navigation or source change was refused, so the UI can say which.
enum AnnotationGuard: Equatable {
    case strokeInProgress
    /// A propagation is writing frames; nothing else may move the session.
    case propagating
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

    @Published private(set) var sidecar: Sidecar {
        // A review changes no selection and no point, only what a mask is
        // worth, and the tallies have to notice that too.
        didSet { labelRevision &+= 1 }
    }

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
        didSet {
            currentClasses = PointClass.displayClasses(
                points: currentPoints, hasClassification: pack.manifest.hasClassification,
                band: heightBand)
            sceneRevision &+= 1
            labelRevision &+= 1
        }
    }

    /// The class each of the current points is drawn and filtered as: the
    /// recorder's, with what the run's height band removed counted as ground.
    private(set) var currentClasses: [UInt8] = []

    /// The height band "ground" is judged by: the pack's own when it records
    /// one, and otherwise the pipeline's default, flagged as assumed.
    let heightBand: HeightBand
    let heightBandIsAssumed: Bool

    /// How many of the current sample's points are in each display class, so
    /// a toggle can say that it has nothing to hide.
    var classCounts: [UInt8: Int] {
        var counts: [UInt8: Int] = currentClasses.reduce(into: [:]) { counts, value in
            counts[value, default: 0] += 1
        }
        // The settled background behind the frame is background too, and it is
        // what the Background toggle shows and hides.
        counts[PointClass.background, default: 0] += backgroundPoints.count
        return counts
    }

    // MARK: Selection

    @Published private(set) var history = MembershipHistory() {
        didSet {
            sceneRevision &+= 1
            labelRevision &+= 1
        }
    }
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

    // MARK: Settled background

    /// The background snapshot in force at this frame, or nil when the pack
    /// carries none.
    @Published private(set) var currentBackground: AnnotationBackground? {
        didSet { if currentBackground != oldValue { sceneRevision &+= 1 } }
    }
    /// Its points. Context only: they have no index a mask could cite, and no
    /// tool selects from them.
    private(set) var backgroundPoints = BackgroundPoints()
    /// Which of them the snapshot added or moved, against the one before it.
    private(set) var backgroundChanged: [Int] = []
    /// True on the first frame a new snapshot is in force: the signal that the
    /// settled scene behind the foreground has just changed.
    @Published private(set) var backgroundUpdatedHere = false

    // MARK: Progress

    /// Bumped when what is labelled in this frame changes, so the tally below
    /// is made once per change and not once per redraw.
    private var labelRevision = 0
    private var talliedRevision = -1
    private var tallied = FrameCompleteness()
    /// Labelable returns per frame, counted once when the pack is opened.
    private var labelableCounts: [Int] = []
    /// Every frame's progress, for the frame strip.
    @Published private(set) var frameProgress: [FrameProgress] = []

    // MARK: Brush

    /// Where the sphere brush would mark if the button went down now. Shown in
    /// every view, because the one the cursor is in cannot show its depth.
    @Published private(set) var hoverSphere: SelectionSphere? {
        didSet { if hoverSphere != oldValue { sceneRevision &+= 1 } }
    }
    /// The returns inside `hoverSphere`.
    @Published private(set) var hoverIndices: [Int] = []
    /// Moves the brush along the view's depth axis, away from the return it
    /// took its depth from. In the top view that is up and down.
    @Published private(set) var brushDepthOffset: Float = 0
    /// The depth the brush last found a return at, used where there is none
    /// under the cursor so that a stroke does not jump between depths.
    private var lastBrushDepth: Float?

    // MARK: Carried selection

    /// The previous sample's selection laid over this one, as a proposal.
    @Published private(set) var carried: CarriedSelection? { didSet { sceneRevision &+= 1 } }
    /// The returns inside the carried footprint where it now lies.
    @Published private(set) var carriedIndices: [Int] = []
    /// How far each object's footprint had to be moved on its last accepted
    /// carry. The next carry starts from there: a car doing ten metres a
    /// second last frame is doing about that this frame.
    private var carryVelocity: [String: simd_float3] = [:]

    // MARK: Propagation

    /// Set while a propagation runs: the frame it has reached.
    @Published private(set) var propagationProgress: Int?
    /// What the last propagation did, until the next edit.
    @Published private(set) var lastPropagation: PropagationOutcome?
    private var propagationCancelled = false

    // MARK: Saved masks

    /// The active object's saved mask in this sample. Drawn in the object's
    /// class colour; what differs from it is drawn as unsaved.
    @Published private(set) var savedSelection: Set<Int> = [] {
        didSet {
            sceneRevision &+= 1
            labelRevision &+= 1
        }
    }
    /// Every other object's saved mask in this sample, so an operator can see
    /// what is already labelled before labelling it again.
    @Published private(set) var otherMasks: [(name: String, objectClass: String, indices: [Int])] =
        []
    {
        didSet {
            sceneRevision &+= 1
            labelRevision &+= 1
        }
    }

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
    @Published var operatorName: String = "" {
        didSet { defaults?.set(operatorName, forKey: AnnotationSession.operatorKey) }
    }
    /// Where the operator's name is remembered. Nil under test, so that one
    /// test's operator is not another's.
    private let defaults: UserDefaults?
    let sessionID: String = UUID().uuidString

    private static let operatorKey = "annotation.operatorName"

    // MARK: Init

    init(
        pack: AnnotationPack, store: SidecarStore? = nil,
        defaults: UserDefaults? = AppState.isRunningUnderXCTest ? nil : .standard
    ) throws {
        self.pack = pack
        self.defaults = defaults
        let resolvedStore = store ?? SidecarStore(packDirectory: pack.directory)
        self.store = resolvedStore
        self.document = try resolvedStore.load(
            packDigest: pack.manifest.packDigest, datasetID: pack.manifest.datasetID)
        self.sidecar = document.sidecar
        self.orderedSamples = pack.chronologicalSamples
        self.heightBand = pack.manifest.source.heightBand ?? .pipelineDefault
        self.heightBandIsAssumed = pack.manifest.source.heightBand == nil
        // The name last saved under, so that it is typed once per machine and
        // not once per pack. It is still the operator's own, and still editable.
        self.operatorName = defaults?.string(forKey: AnnotationSession.operatorKey) ?? ""
        if let first = orderedSamples.first {
            let points = (try? pack.points(sampleID: first.sampleID)) ?? PackPoints()
            // Property observers do not run in an initialiser.
            self.currentPoints = points
            self.currentClasses = PointClass.displayClasses(
                points: points, hasClassification: pack.manifest.hasClassification, band: heightBand
            )
        }
        resetSlabToSampleExtent()
        // Once, from the first sample, and not again as samples are stepped
        // through: see columnGrid.
        estimateGround()
        fitViews(to: .sample)
        refreshBackground(previousSampleIndex: nil)
        labelableCounts = orderedSamples.map { sample in
            guard let points = try? pack.points(sampleID: sample.sampleID) else { return 0 }
            let classes = PointClass.displayClasses(
                points: points, hasClassification: pack.manifest.hasClassification, band: heightBand
            )
            return (0..<points.count).filter { FrameCompleteness.isLabelable(classes, $0) }.count
        }
        refreshFrameProgress()
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
        // The selection on screen stays: it becomes this object's unsaved
        // membership. Selecting a car and then saying what it is is the
        // natural order, and reloading here used to wipe the selection the
        // moment the object it was for was created.
        savedSelection = []
        carried = nil
        carriedIndices = []
        refreshOtherMasks()
        markDirty()
        return object
    }

    /// Makes another object the one under edit, loading its saved mask.
    /// Refused while the current one has unsaved changes.
    @discardableResult func activate(objectID: String) -> AnnotationGuard? {
        if let blocker = navigationGuard() { return blocker }
        activeObjectID = objectID
        carried = nil
        carriedIndices = []
        loadSelectionForCurrentSample()
        return nil
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

    // MARK: - Review

    /// What a mask's status is once saved over `previous`. A reviewed mask
    /// stays reviewed only while its points are the ones that were reviewed:
    /// the review was of those points, and changing them and keeping the
    /// status would pass unreviewed membership off as checked.
    static func statusAfterSaving(_ mask: FrameMask, over previous: FrameMask?) -> ReviewStatus {
        guard let previous, previous.status == .reviewed,
            Set(previous.pointIndices) == Set(mask.pointIndices)
        else { return .proposed }
        return .reviewed
    }

    /// Marks this frame's saved mask reviewed, and the object with it.
    ///
    /// Marking only the object is what this used to do, and it made nothing
    /// count: a mask is reference truth when the mask and its object are both
    /// reviewed, and no control in this client ever reviewed a mask.
    @discardableResult func markFrameReviewed(secondViewConfirmed: Bool) -> Bool {
        lastError = nil
        guard secondViewConfirmed else {
            return fail("Check the points in the second view before marking them reviewed.")
                ?? false
        }
        guard let sample = currentSample, let objectID = activeObjectID,
            sidecar.mask(objectID: objectID, sampleID: sample.sampleID) != nil
        else { return fail("Save this frame's points before reviewing them.") ?? false }
        guard navigationGuard() == nil else {
            return fail("Save the changes first: a review is of what is saved.") ?? false
        }
        return review(objectID: objectID, sampleIDs: [sample.sampleID], operation: "review_mask")
            != nil
    }

    /// Marks every saved mask of the active object reviewed: the operator's
    /// statement that they have been through all of its frames. Returns how
    /// many masks that was.
    @discardableResult func markAllFramesReviewed(secondViewConfirmed: Bool) -> Int? {
        lastError = nil
        guard secondViewConfirmed else {
            return fail("Check the points in the second view before marking them reviewed.")
        }
        guard let objectID = activeObjectID else {
            return fail("Choose the object whose frames you have been through.")
        }
        guard navigationGuard() == nil else {
            return fail("Save the changes first: a review is of what is saved.")
        }
        let sampleIDs = sidecar.masks.filter { $0.objectID == objectID }.map(\.sampleID)
        guard !sampleIDs.isEmpty else { return fail("This object has no saved frames to review.") }
        return review(
            objectID: objectID, sampleIDs: Set(sampleIDs), operation: "review_object_masks")
    }

    private func review(objectID: String, sampleIDs: Set<Int>, operation: String) -> Int? {
        guard !operatorName.trimmingCharacters(in: .whitespaces).isEmpty else {
            return fail(
                "Enter your name under Labelled by before reviewing: a review has an author.")
        }
        var change = Provenance(
            author: operatorName, session: sessionID, createdUTC: SidecarStore.utcTimestamp(),
            operation: operation)
        var edited = document
        edited.sidecar = sidecar
        var reviewed = 0
        for index in edited.sidecar.masks.indices
        where edited.sidecar.masks[index].objectID == objectID
            && sampleIDs.contains(edited.sidecar.masks[index].sampleID)
        {
            // How the points got there is kept: a mask an algorithm filled in
            // and a person then checked is still a mask an algorithm filled in.
            change.algorithm = edited.sidecar.masks[index].provenance.algorithm
            change.algorithmVersion = edited.sidecar.masks[index].provenance.algorithmVersion
            edited.sidecar.masks[index].status = .reviewed
            edited.sidecar.masks[index].provenance = change
            reviewed += 1
        }
        change.algorithm = nil
        change.algorithmVersion = nil
        if let index = edited.sidecar.objects.firstIndex(where: { $0.objectID == objectID }) {
            edited.sidecar.objects[index].status = .reviewed
            edited.sidecar.objects[index].provenance = change
        }
        do {
            document = try store.save(edited, change: change)
            sidecar = document.sidecar
            refreshFrameProgress()
            return reviewed
        } catch let error as SidecarStoreError {
            conflict = error
            return fail(AnnotationSession.describe(error))
        } catch { return fail("\(error)") }
    }

    /// How many of the object's saved frames are reviewed.
    func reviewedSampleCount(objectID: String) -> Int {
        sidecar.masks.filter { $0.objectID == objectID && $0.status == .reviewed }.count
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
        // Taken before the points change: the space this sample's membership
        // occupies, to be laid over the sample being stepped to.
        let leaving = footprintOfCurrentSelection()
        let direction = index - sampleIndex
        let fromSampleID = currentSample?.sampleID
        let previousIndex = sampleIndex
        sampleIndex = index
        // Only a step the operator made here asks the main view to come along.
        // One made to follow the main view must not: during playback the main
        // view has already moved on, and seeking it back to this sample would
        // have the two windows drag each other backwards.
        if !followingMainView { operatorNavigationRevision &+= 1 }
        currentPoints = (try? pack.points(sampleID: orderedSamples[index].sampleID)) ?? PackPoints()
        pendingCandidates = nil
        hoverSphere = nil
        hoverIndices = []
        if !slabIsPinned { resetSlabToSampleExtent() }
        loadSelectionForCurrentSample()
        carry(leaving, from: fromSampleID, direction: direction)
        refreshBackground(previousSampleIndex: previousIndex)
        return nil
    }

    @discardableResult func stepForward() -> AnnotationGuard? { step(to: sampleIndex + 1) }

    @discardableResult func stepBackward() -> AnnotationGuard? { step(to: sampleIndex - 1) }

    /// What would block leaving this sample, if anything. Exposed so a caller
    /// can ask before offering navigation rather than after refusing it.
    func navigationGuard() -> AnnotationGuard? {
        if propagationProgress != nil { return .propagating }
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
        let candidates = sphereCandidates(sphere)
        pendingSphere = sphere
        pendingCandidates = candidates
        return candidates
    }

    /// Adds one more position of the sphere brush to the stroke in progress:
    /// the candidates are everything any position of the brush has covered.
    @discardableResult func paint(sphere: SelectionSphere) -> SelectionCandidates {
        let here = sphereCandidates(sphere)
        var total = pendingCandidates ?? SelectionCandidates(indices: [], excludedBySlab: 0)
        total.indices = Set(total.indices).union(here.indices).sorted()
        // Not summed: the same return is excluded again at every position the
        // brush overlaps it from, and a count that grows with the length of
        // the drag says nothing. The latest position's counts are what the
        // operator is looking at.
        total.excludedBySlab = here.excludedBySlab
        total.excludedByVisibility = here.excludedByVisibility
        pendingSphere = sphere
        pendingCandidates = total
        return total
    }

    private func sphereCandidates(_ sphere: SelectionSphere) -> SelectionCandidates {
        visibleOnly(
            PointSelectionEngine.candidates(
                points: currentPoints, sphere: sphere, basis: OrthoViewBasis(viewStandard),
                slab: slab))
    }

    /// The sphere the brush marks with at a position in the editing view.
    ///
    /// A cursor gives two coordinates and a sphere needs three. The third is
    /// the depth of the nearest drawn return under the cursor, so the brush
    /// rides the surface being painted; where there is none, the depth it last
    /// had, so that crossing a gap does not drop it to the ground. The
    /// operator moves it off that depth with `adjustBrushDepth`.
    func brushSphere(atViewPoint viewPoint: simd_float2, pickDistance: Float) -> SelectionSphere {
        let basis = OrthoViewBasis(viewStandard)
        let reach = max(pickDistance, sphereRadius)
        if let index = nearestPointIndex(toViewPoint: viewPoint, maxViewDistance: reach),
            let p = currentPoints.point(at: index)
        {
            lastBrushDepth = basis.depth(p)
        }
        let depth =
            (lastBrushDepth ?? slab.map { ($0.minDepth + $0.maxDepth) / 2 } ?? 0) + brushDepthOffset
        let centre =
            basis.origin + basis.right * viewPoint.x + basis.up * viewPoint.y + basis.forward
            * depth
        return SelectionSphere(centre: centre, radius: sphereRadius)
    }

    /// Shows where the sphere brush would mark at this position, or clears it.
    func hover(atViewPoint viewPoint: simd_float2?, pickDistance: Float) {
        guard tool == .sphere, !strokeInProgress, let viewPoint else {
            if hoverSphere != nil {
                hoverSphere = nil
                hoverIndices = []
            }
            return
        }
        let sphere = brushSphere(atViewPoint: viewPoint, pickDistance: pickDistance)
        hoverIndices = sphereCandidates(sphere).indices
        hoverSphere = sphere
    }

    /// Moves the brush along the depth axis by whole steps of a tenth of a
    /// metre. Positive is away from the viewer: downward, in the top view.
    func adjustBrushDepth(steps: Int) {
        brushDepthOffset = ((brushDepthOffset + Float(steps) * 0.1) * 10).rounded() / 10
        if let sphere = hoverSphere {
            let basis = OrthoViewBasis(viewStandard)
            let moved = SelectionSphere(
                centre: sphere.centre + basis.forward * Float(steps) * 0.1, radius: sphere.radius)
            hoverIndices = sphereCandidates(moved).indices
            hoverSphere = moved
        }
    }

    func resetBrushDepth() { brushDepthOffset = 0 }

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
        PointSelectionEngine.nearestPoint(
            points: currentPoints, basis: OrthoViewBasis(viewStandard), viewPoint: viewPoint,
            slab: slab, maxViewDistance: maxViewDistance, where: isVisible)
    }

    /// The class filter in force, or nil when every class is on.
    var effectiveVisibility: PointVisibility? { visibility.showsEverything ? nil : visibility }

    /// True when the current sample's point at `index` is drawn, and so can
    /// be selected.
    func isVisible(_ index: Int) -> Bool {
        PointVisibility.isVisible(index, classes: currentClasses, under: effectiveVisibility)
    }

    /// Drops candidates the class filter hides, and counts them, so a gesture
    /// that took fewer points than it covered says why.
    private func visibleOnly(_ candidates: SelectionCandidates) -> SelectionCandidates {
        guard effectiveVisibility != nil else { return candidates }
        var result = candidates
        result.indices = candidates.indices.filter(isVisible)
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
        // A selection made by hand replaces the proposal it was offered.
        if changed {
            carried = nil
            carriedIndices = []
            markDirty()
        }
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
    @discardableResult func fitViews(toIndices indices: Set<Int>) -> Bool {
        fitViews(trim: 0) { indices.contains($0) }
    }

    @discardableResult func fitViews(to target: AnnotationFitTarget) -> Bool {
        let selection = history.current
        let include: (Int) -> Bool
        let trim: Float
        switch target {
        case .sample:
            include = isVisible
            trim = AnnotationSession.sampleFitTrim
        case .foreground:
            // Display classes: what the height band removed is not foreground
            // for this purpose, or the fit would take in the road.
            include = { index in
                index < self.currentClasses.count
                    && self.currentClasses[index] == PointClass.foreground
            }
            trim = 0
        case .selection:
            include = { selection.contains($0) }
            trim = 0
        }
        return fitViews(trim: trim, where: include)
    }

    private func fitViews(trim: Float, where include: (Int) -> Bool) -> Bool {
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

    // MARK: - Carrying a selection between samples

    /// How far one press of an arrow key moves a carried footprint, in metres.
    static let nudgeStep: Float = 0.1
    static let coarseNudgeStep: Float = 0.5

    private func footprintOfCurrentSelection() -> SelectionFootprint? {
        guard activeObjectID != nil, !history.current.isEmpty else { return nil }
        let footprint = SelectionFootprint(
            points: history.current.compactMap { currentPoints.point(at: $0) })
        return footprint.isEmpty ? nil : footprint
    }

    /// Lays the sample just left's selection over the one just arrived at.
    ///
    /// Only onto a neighbouring sample, and only where the object has no mask
    /// yet: a footprint ten samples old is somewhere the object no longer is,
    /// and a sample already labelled has nothing to propose.
    private func carry(_ footprint: SelectionFootprint?, from sampleID: Int?, direction: Int) {
        carried = nil
        carriedIndices = []
        guard let footprint, let sampleID, let object = activeObject, abs(direction) == 1,
            savedSelection.isEmpty, history.current.isEmpty
        else { return }

        // A building is where it was. A car is about as far on as it moved
        // last time, and the search takes it from there.
        var offset = simd_float3.zero
        if !AnnotationPalette.isFixed(object.objectClass) {
            let prediction = (carryVelocity[object.objectID] ?? .zero) * Float(direction)
            offset = footprint.bestOffset(in: currentPoints, near: prediction, where: isVisible)
        }
        carried = CarriedSelection(
            footprint: footprint, offset: offset, fromSampleID: sampleID, direction: direction)
        refreshCarriedIndices()
    }

    private func refreshCarriedIndices() {
        guard let carried else {
            carriedIndices = []
            return
        }
        carriedIndices = carried.footprint.indices(
            in: currentPoints, offset: carried.offset, where: isVisible)
    }

    /// Moves the carried footprint by an offset in the editing view's plane:
    /// `right` and `up` as the operator sees them, in metres.
    func nudgeCarried(right: Float, up: Float) {
        guard var moved = carried else { return }
        let basis = OrthoViewBasis(viewStandard)
        moved.offset += basis.right * right + basis.up * up
        carried = moved
        refreshCarriedIndices()
    }

    /// Searches again for where the carried footprint fits best, from where
    /// it now lies: the way to recover after a nudge in the wrong direction.
    func refitCarried() {
        guard var moved = carried else { return }
        moved.offset = moved.footprint.bestOffset(
            in: currentPoints, near: moved.offset, where: isVisible)
        carried = moved
        refreshCarriedIndices()
    }

    /// Makes the carried proposal this sample's membership. It is unsaved
    /// until saved, like any other selection.
    @discardableResult func acceptCarried() -> Bool {
        guard let accepted = carried, let object = activeObject else { return false }
        let changed = history.commit(Set(carriedIndices))
        // Per sample stepped, in the direction of time, so that carrying
        // backwards later predicts the opposite way.
        carryVelocity[object.objectID] = accepted.offset * Float(accepted.direction)
        carried = nil
        carriedIndices = []
        if changed { markDirty() }
        return changed
    }

    func dismissCarried() {
        carried = nil
        carriedIndices = []
    }

    /// Saves the active object's mask into every sample, from the space its
    /// selection occupies in this one. For things that do not move.
    ///
    /// Each mask is recorded as made by this propagation and not by hand, and
    /// stays proposed: the operator has looked at one sample of it. Samples
    /// where the object already has a mask are left alone. Returns how many
    /// masks were written, or nil when nothing could be saved.
    @discardableResult func applySelectionToAllSamples() -> Int? {
        lastError = nil
        guard !operatorName.trimmingCharacters(in: .whitespaces).isEmpty else {
            return fail("Enter your name under Labelled by before saving: a label has an author.")
        }
        guard let object = activeObject, let here = currentSample else {
            return fail("Select an object before applying its mask to other samples.")
        }
        guard AnnotationPalette.isFixed(object.objectClass) else {
            return fail("Only an object that does not move can be applied to every sample.")
        }
        guard let footprint = footprintOfCurrentSelection() else {
            return fail("Select the object's points in this sample first.")
        }

        var edited = document
        edited.sidecar = sidecar
        edited.sidecar.packDigest = pack.manifest.packDigest
        edited.sidecar.datasetID = pack.manifest.datasetID
        var change = Provenance(
            author: operatorName, session: sessionID, createdUTC: SidecarStore.utcTimestamp(),
            operation: "apply_fixed_mask")
        change.algorithm = "footprint_carry"
        change.algorithmVersion = "1"

        var written = 0
        for sample in orderedSamples {
            var mask: FrameMask
            if sample.sampleID == here.sampleID {
                // This one is the operator's own selection, not a propagation.
                mask =
                    sidecar.mask(objectID: object.objectID, sampleID: sample.sampleID)
                    ?? FrameMask(objectID: object.objectID, sampleID: sample.sampleID)
                mask.pointIndices = canonicalSelection
                mask.provenance = Provenance(
                    author: operatorName, session: sessionID,
                    createdUTC: SidecarStore.utcTimestamp(), operation: "save_mask")
            } else {
                guard sidecar.mask(objectID: object.objectID, sampleID: sample.sampleID) == nil,
                    let points = try? pack.points(sampleID: sample.sampleID)
                else { continue }
                let classes = PointClass.displayClasses(
                    points: points, hasClassification: pack.manifest.hasClassification,
                    band: heightBand)
                let filter = effectiveVisibility
                let indices = footprint.indices(in: points, offset: .zero) {
                    PointVisibility.isVisible($0, classes: classes, under: filter)
                }
                guard !indices.isEmpty else { continue }
                mask = FrameMask(objectID: object.objectID, sampleID: sample.sampleID)
                mask.pointIndices = indices
                mask.provenance = change
            }
            mask.completeness = maskCompleteness
            mask.visibility = maskVisibility
            mask.status = AnnotationSession.statusAfterSaving(
                mask, over: sidecar.mask(objectID: object.objectID, sampleID: sample.sampleID))
            edited.sidecar.upsert(mask: mask)
            written += 1
        }

        do {
            document = try store.save(edited, change: change)
            sidecar = document.sidecar
            dirtySamples.remove(here.sampleID)
            savedSelection = history.current
            refreshFrameProgress()
            return written
        } catch let error as SidecarStoreError {
            conflict = error
            return fail(AnnotationSession.describe(error))
        } catch { return fail("\(error)") }
    }

    /// Records a failure where the operator can see it and where the log can:
    /// a save that fails silently is indistinguishable from one that worked.
    private func fail<T>(_ message: String) -> T? {
        lastError = message
        sessionLogger.error("\(message)")
        return nil
    }

    /// How many samples the object has a saved mask in.
    func savedSampleCount(objectID: String) -> Int {
        sidecar.masks.filter { $0.objectID == objectID && !$0.pointIndices.isEmpty }.count
    }

    /// What the operator has to do next before a mask can be saved, or nil
    /// when nothing stands in the way.
    var nextStep: String? {
        if operatorName.trimmingCharacters(in: .whitespaces).isEmpty {
            return "Enter your name under Labelled by. Every label is saved with its author."
        }
        if activeObjectID == nil {
            return selectionCount == 0
                ? "Select an object's points, then choose its class and press New object."
                : "Choose a class and press New object to say what these points are."
        }
        if carried != nil {
            return "The last sample's selection is laid over this one. Nudge it, then accept."
        }
        if selectionCount == 0 {
            return "Select the points of \(activeObjectName ?? "this object") in the editing view."
        }
        if navigationGuard() != nil { return "Unsaved changes. Save to move on." }
        return nil
    }

    // MARK: - Propagation

    func cancelPropagation() { propagationCancelled = true }

    /// Carries the active object's mask through the following frames (or the
    /// preceding ones, with `direction` -1) for as long as the fit is one a
    /// person would have accepted, and saves them all at once.
    ///
    /// The frames it writes are recorded as made by this propagation and stay
    /// in question until reviewed. Where it stops for a reason that needs a
    /// person, it leaves the session on that frame with the failed fit laid
    /// over it as a proposal to nudge.
    @discardableResult func propagate(
        direction: Int = 1, maxFrames: Int? = nil
    ) async -> PropagationOutcome? {
        lastError = nil
        lastPropagation = nil
        guard direction == 1 || direction == -1 else { return nil }
        guard !operatorName.trimmingCharacters(in: .whitespaces).isEmpty else {
            return fail("Enter your name under Labelled by before saving: a label has an author.")
        }
        guard let object = activeObject, let start = currentSample else {
            return fail("Choose the object to carry through the frames.")
        }
        // Carried from what is saved. Unsaved changes are saved first: the
        // operator has just made them on the frame they are carrying from.
        if navigationGuard() != nil, !save() { return nil }
        guard !savedSelection.isEmpty else {
            return fail("Label \(displayName(objectID: object.objectID)) in this frame first.")
        }

        let isFixed = AnnotationPalette.isFixed(object.objectClass)
        let filter = effectiveVisibility
        var footprint = SelectionFootprint(
            points: savedSelection.compactMap { currentPoints.point(at: $0) })
        var previousCount = savedSelection.count
        var previousRange = AnnotationSession.meanRange(of: savedSelection, in: currentPoints)
        // Per frame, in the direction being carried.
        var velocity = (carryVelocity[object.objectID] ?? .zero) * Float(direction)
        var pending: [FrameMask] = []
        var stop = PropagationStop.endOfPack
        var stopFit: (footprint: SelectionFootprint, offset: simd_float3, from: Int)?
        var lastSampleID = start.sampleID
        var index = sampleIndex + direction

        propagationCancelled = false
        propagationProgress = sampleIndex
        defer { propagationProgress = nil }

        var change = Provenance(
            author: operatorName, session: sessionID, createdUTC: SidecarStore.utcTimestamp(),
            operation: "propagate_mask")
        change.algorithm = "footprint_carry"
        change.algorithmVersion = "1"

        while index >= 0, index < orderedSamples.count {
            if propagationCancelled {
                stop = .cancelled
                break
            }
            if let maxFrames, pending.count >= maxFrames {
                stop = .frameLimit
                break
            }
            let sample = orderedSamples[index]
            if sidecar.mask(objectID: object.objectID, sampleID: sample.sampleID) != nil {
                stop = .alreadyLabelled
                break
            }
            guard let points = try? pack.points(sampleID: sample.sampleID) else { break }
            let classes = PointClass.displayClasses(
                points: points, hasClassification: pack.manifest.hasClassification, band: heightBand
            )
            let include: (Int) -> Bool = {
                PointVisibility.isVisible($0, classes: classes, under: filter)
            }

            var fit: FootprintFit
            if isFixed {
                fit = FootprintFit(offset: .zero)
                fit.count = footprint.indices(in: points, offset: .zero, where: include).count
            } else {
                fit = footprint.fit(in: points, near: velocity, where: include)
            }
            let indices = footprint.indices(in: points, offset: fit.offset, where: include)
            let range = AnnotationSession.meanRange(of: Set(indices), in: points)
            let expected = PropagationJudge.expected(
                previousCount: previousCount, previousRange: previousRange, range: range)

            var verdict = PropagationJudge.verdict(
                fit: fit, prediction: velocity, expected: expected, isFixed: isFixed)
            if verdict == nil,
                let rival = objectSharing(indices, inSample: sample.sampleID, not: object.objectID)
            {
                verdict = .overlaps(objectName: rival)
            }
            if let verdict {
                stop = verdict
                stopFit = (footprint, fit.offset, lastSampleID)
                break
            }

            var mask = FrameMask(objectID: object.objectID, sampleID: sample.sampleID)
            mask.pointIndices = indices
            mask.completeness = maskCompleteness
            mask.visibility = maskVisibility
            mask.status = .proposed
            mask.provenance = change
            pending.append(mask)

            // The next frame is fitted from where and what the object now is.
            footprint = SelectionFootprint(points: indices.compactMap { points.point(at: $0) })
            velocity = isFixed ? .zero : velocity * 0.5 + fit.offset * 0.5
            previousCount = indices.count
            previousRange = range
            lastSampleID = sample.sampleID
            propagationProgress = index
            index += direction
            // Lets the window draw the frame counter and take a Stop.
            await Task.yield()
        }

        if !pending.isEmpty {
            var edited = document
            edited.sidecar = sidecar
            edited.sidecar.packDigest = pack.manifest.packDigest
            edited.sidecar.datasetID = pack.manifest.datasetID
            for mask in pending { edited.sidecar.upsert(mask: mask) }
            do {
                document = try store.save(edited, change: change)
                sidecar = document.sidecar
                refreshFrameProgress()
            } catch let error as SidecarStoreError {
                conflict = error
                return fail(AnnotationSession.describe(error))
            } catch { return fail("\(error)") }
            carryVelocity[object.objectID] = velocity * Float(direction)
        }

        // Land where the work is: on the frame that needs a person, with the
        // fit that was refused laid over it, or else on the last frame written.
        propagationProgress = nil
        let landing = stop.needsOperator ? index : index - direction
        if landing != sampleIndex, landing >= 0, landing < orderedSamples.count {
            _ = step(to: landing, followingMainView: false)
        }
        if stop.needsOperator, let stopFit {
            carried = CarriedSelection(
                footprint: stopFit.footprint, offset: stopFit.offset, fromSampleID: stopFit.from,
                direction: direction)
            refreshCarriedIndices()
        }
        let outcome = PropagationOutcome(
            objectName: displayName(objectID: object.objectID), framesWritten: pending.count,
            stop: stop)
        lastPropagation = outcome
        sessionLogger.info("\(outcome.summary)")
        return outcome
    }

    /// The name of another object that already has a fifth or more of these
    /// returns in this frame, if there is one.
    private func objectSharing(
        _ indices: [Int], inSample sampleID: Int, not objectID: String
    ) -> String? {
        guard !indices.isEmpty else { return nil }
        let mine = Set(indices)
        for mask in sidecar.masks where mask.sampleID == sampleID && mask.objectID != objectID {
            let shared = mask.pointIndices.filter(mine.contains).count
            if Float(shared) >= Float(indices.count) * PropagationJudge.overlapFraction {
                return displayName(objectID: mask.objectID)
            }
        }
        return nil
    }

    /// Mean horizontal distance from the sensor.
    static func meanRange(of indices: Set<Int>, in points: PackPoints) -> Float {
        var sum: Float = 0
        var count: Float = 0
        for index in indices where index >= 0 && index < points.count {
            sum += (points.x[index] * points.x[index] + points.y[index] * points.y[index])
                .squareRoot()
            count += 1
        }
        return count > 0 ? sum / count : 0
    }

    // MARK: - Names

    /// What an object is called on screen: its class and its number among
    /// objects of that class, in the order they were made. "car 2" says which
    /// of two cars is meant; "obj_5f3a9c1e" does not.
    func displayName(objectID: String) -> String {
        guard let object = sidecar.objects.first(where: { $0.objectID == objectID }) else {
            return objectID
        }
        let ordinal =
            sidecar.objects.prefix { $0.objectID != objectID }.filter {
                $0.objectClass == object.objectClass
            }.count + 1
        return "\(object.objectClass) \(ordinal)"
    }

    var activeObjectName: String? { activeObjectID.map(displayName(objectID:)) }

    // MARK: - Settled background

    /// Voxel edge used to tell a moved background return from a repeated one.
    ///
    /// Half a metre, the column lattice's pitch. Measured on a real pair of
    /// snapshots fifteen seconds apart (67,015 returns): a quarter of a metre
    /// flags 6,507 of them, most of it range jitter on foliage and far walls;
    /// half a metre flags 2,295; a metre flags 271 and starts to miss a car
    /// that stopped. The thing to show is a surface that moved by about a
    /// vehicle, which is what the background settling on stopped traffic is.
    static let backgroundChangePitch: Float = 0.5

    private func refreshBackground(previousSampleIndex: Int?) {
        guard let sample = currentSample, let inForce = pack.backgroundInForce(at: sample) else {
            currentBackground = nil
            backgroundPoints = BackgroundPoints()
            backgroundChanged = []
            backgroundUpdatedHere = false
            return
        }
        // Whatever snapshot was in force at the frame just left. Stepping
        // forward across an update is the moment to say so; stepping back
        // across one is not news.
        let previous = previousSampleIndex.flatMap { index -> AnnotationBackground? in
            guard index >= 0, index < orderedSamples.count else { return nil }
            return pack.backgroundInForce(at: orderedSamples[index])
        }
        backgroundUpdatedHere = previous.map { $0.backgroundID < inForce.backgroundID } ?? false

        guard inForce != currentBackground else { return }
        let points = pack.backgroundPoints(inForce)
        backgroundPoints = points
        backgroundChanged = []
        if inForce.backgroundID > 0 {
            let before = pack.backgroundPoints(pack.backgrounds[inForce.backgroundID - 1])
            backgroundChanged = AnnotationSession.changedIndices(in: points, against: before)
        }
        currentBackground = inForce
    }

    /// Indices of the returns in `points` that lie where `before` had nothing
    /// within a voxel. The settled range of a cell wanders by centimetres from
    /// one snapshot to the next, and that is not a change; a return half a
    /// metre from anything the last snapshot held is.
    static func changedIndices(
        in points: BackgroundPoints, against before: BackgroundPoints,
        pitch: Float = AnnotationSession.backgroundChangePitch
    ) -> [Int] {
        func voxel(_ x: Float, _ y: Float, _ z: Float) -> SIMD3<Int32> {
            SIMD3(
                Int32((x / pitch).rounded(.down)), Int32((y / pitch).rounded(.down)),
                Int32((z / pitch).rounded(.down)))
        }
        var occupied = Set<SIMD3<Int32>>()
        occupied.reserveCapacity(before.count * 4)
        for i in 0..<before.count {
            let v = voxel(before.x[i], before.y[i], before.z[i])
            for dx: Int32 in -1...1 {
                for dy: Int32 in -1...1 {
                    for dz: Int32 in -1...1 { occupied.insert(SIMD3(v.x + dx, v.y + dy, v.z + dz)) }
                }
            }
        }
        return (0..<points.count).filter {
            !occupied.contains(voxel(points.x[$0], points.y[$0], points.z[$0]))
        }
    }

    // MARK: - Progress

    /// The current frame's labelling, by sector.
    var completeness: FrameCompleteness {
        if talliedRevision != labelRevision {
            let (agreed, inQuestion) = labelStates()
            tallied = FrameCompleteness.tally(
                points: currentPoints, classes: currentClasses, agreed: agreed,
                inQuestion: inQuestion)
            talliedRevision = labelRevision
        }
        return tallied
    }

    /// Which of the current frame's returns are agreed and which in question.
    private func labelStates() -> (agreed: Set<Int>, inQuestion: Set<Int>) {
        guard let sample = currentSample else { return ([], []) }
        var agreed = Set<Int>()
        var inQuestion = Set<Int>()
        for mask in sidecar.masks where mask.sampleID == sample.sampleID {
            if mask.objectID == activeObjectID { continue }
            if mask.isAgreed {
                agreed.formUnion(mask.pointIndices)
            } else {
                inQuestion.formUnion(mask.pointIndices)
            }
            inQuestion.formUnion(mask.uncertainIndices ?? [])
        }
        // The object under edit is judged as it stands on screen. What matches
        // its saved mask is as agreed as that mask is; what has been added or
        // taken out since is in question until saved.
        let current = history.current
        let activeMask = activeObjectID.flatMap {
            sidecar.mask(objectID: $0, sampleID: sample.sampleID)
        }
        let kept = current.intersection(savedSelection)
        if activeMask?.isAgreed ?? true {
            agreed.formUnion(kept)
        } else {
            inQuestion.formUnion(kept)
        }
        inQuestion.formUnion(current.symmetricDifference(savedSelection))
        inQuestion.formUnion(activeMask?.uncertainIndices ?? [])
        return (agreed, inQuestion)
    }

    /// Rebuilds the frame strip from what is saved. Counts labelled returns
    /// against labelable ones without re-reading every frame's classes, so a
    /// mask that takes in a few returns the height band removed can overstate
    /// a frame by those few: the strip says where the work is, and the sector
    /// ring, which is exact, says whether a frame is done.
    private func refreshFrameProgress() {
        var agreed = [Int: Set<Int>]()
        var inQuestion = [Int: Set<Int>]()
        for mask in sidecar.masks {
            if mask.isAgreed {
                agreed[mask.sampleID, default: []].formUnion(mask.pointIndices)
            } else {
                inQuestion[mask.sampleID, default: []].formUnion(mask.pointIndices)
            }
            inQuestion[mask.sampleID, default: []].formUnion(mask.uncertainIndices ?? [])
        }
        var lastBackground: Int?
        frameProgress = orderedSamples.enumerated().map { index, sample in
            let questioned = inQuestion[sample.sampleID] ?? []
            let settled = (agreed[sample.sampleID] ?? []).subtracting(questioned)
            let background = pack.backgroundInForce(at: sample)?.backgroundID
            defer { lastBackground = background }
            return FrameProgress(
                labelable: index < labelableCounts.count ? labelableCounts[index] : 0,
                agreed: settled.count, inQuestion: questioned.count,
                backgroundUpdated: index > 0 && background != lastBackground)
        }
    }

    /// Frames the returns nobody has labelled in one sector, or the whole
    /// sector when there are none left: where to look next.
    @discardableResult func fitViews(toSector sector: Int) -> Bool {
        let (agreed, inQuestion) = labelStates()
        let inSector: (Int) -> Bool = { index in
            FrameCompleteness.isLabelable(self.currentClasses, index)
                && FrameCompleteness.sector(
                    x: self.currentPoints.x[index], y: self.currentPoints.y[index]) == sector
        }
        let unlabelled = (0..<currentPoints.count).filter {
            inSector($0) && !agreed.contains($0) && !inQuestion.contains($0)
        }
        let target = unlabelled.isEmpty ? (0..<currentPoints.count).filter(inSector) : unlabelled
        return fitViews(toIndices: Set(target))
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
            return fail("Enter your name under Labelled by before saving: a label has an author.")
                ?? false
        }
        guard let sample = currentSample, let objectID = activeObjectID else {
            return fail("Create or choose an object before saving: a mask belongs to one.") ?? false
        }

        let indices = canonicalSelection
        switch PointSelectionEngine.validate(indices: indices, pointCount: currentPoints.count) {
        case .failure(let error): return fail(error.message) ?? false
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
            mask.status = AnnotationSession.statusAfterSaving(
                mask, over: sidecar.mask(objectID: objectID, sampleID: sample.sampleID))
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
                savedSelection = Set(validated)
                refreshFrameProgress()
                return true
            } catch let error as SidecarStoreError {
                // Neither busy nor conflict discards the operator's dirty
                // membership: it stays in the history for reconciliation.
                conflict = error
                return fail(AnnotationSession.describe(error)) ?? false
            } catch { return fail("\(error)") ?? false }
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
            // An object that was never saved went with the reload.
            if let id = activeObjectID, !sidecar.objects.contains(where: { $0.objectID == id }) {
                activeObjectID = nil
            }
            carried = nil
            carriedIndices = []
            loadSelectionForCurrentSample()
            refreshFrameProgress()
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
        refreshOtherMasks()
        guard let sample = currentSample, let objectID = activeObjectID,
            let mask = sidecar.mask(objectID: objectID, sampleID: sample.sampleID)
        else {
            history.reset(to: [])
            savedSelection = []
            return
        }
        history.reset(to: Set(mask.pointIndices))
        savedSelection = Set(mask.pointIndices)
        maskVisibility = mask.visibility
        maskCompleteness = mask.completeness
    }

    private func refreshOtherMasks() {
        guard let sample = currentSample else {
            otherMasks = []
            return
        }
        let classes = Dictionary(
            sidecar.objects.map { ($0.objectID, $0.objectClass) }, uniquingKeysWith: { a, _ in a })
        otherMasks = sidecar.masks.filter {
            $0.sampleID == sample.sampleID && $0.objectID != activeObjectID
        }.map {
            (
                name: displayName(objectID: $0.objectID), objectClass: classes[$0.objectID] ?? "",
                indices: $0.pointIndices
            )
        }
    }
}
