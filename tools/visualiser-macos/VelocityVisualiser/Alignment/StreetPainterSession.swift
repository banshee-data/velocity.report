// StreetPainterSession.swift
// State behind the street painter: the operator marks a site's street use
// (surfaces, travel lanes, parked-car spaces and driveways) from a top-down
// plan view over the reference the alignment pane placed, and saves the
// marks to Gerardus in one batch (Gerardus docs/plans/VECTOR-MAP.md §4.1).
//
// The painter replaces the web street editor (web/src/routes/lidar/streets),
// whose logic it ports: StreetGeometry for the snap preview, rows of spaces
// and a lane's direction; StreetUseEditor for the marks, the save diff, and
// Undo and Redo.
//
// The operator's path, one RPC per step:
//
//   Place from hint  (the alignment pane) streams the street layers with the
//                    reference; then ListStreetUse reads the site's marks
//   Paint            nothing on the wire: marks change locally, with a snap
//                    preview against kerbs, centrelines, box walls and the
//                    other marks within 0.5 m
//   Save             EditStreetUse with every change since the marks were
//                    read, all or nothing; Gerardus snaps, drapes and checks
//                    each, and the marks it answers with replace the local
//                    ones, problems beside the marks they name
//
// While painting, the camera is locked to the plan view (PlanView): it looks
// straight down the site frame, north up, and pans and zooms but does not
// turn. Leaving paint mode puts back the camera the operator had.

import Combine
import CoreGraphics
import Foundation
import GRPCCore
import simd

/// Where the painter's overlay and camera go. MetalRenderer is the real one.
protocol StreetPaintSink: AnyObject {
    func setStreetOverlay(_ vertices: StreetOverlayVertices?)
    /// Locks the camera to the plan view. The first call while unlocked
    /// keeps the camera it replaces, for `exitPlanView()` to put back.
    func setPlanCamera(_ camera: Camera)
    /// Unlocks the camera and puts back the one kept.
    func exitPlanView()
}

/// What the street painter snaps to and stands on, from the reference the
/// alignment session streamed.
struct StreetReference: Sendable {
    var kerbs: [StreetGeometry.SnapTarget] = []
    /// The City's centrelines; absent city data provides no road snap targets.
    var centrelines: [StreetGeometry.SnapTarget] = []
    /// Box walls' feet and footprint outlines.
    var walls: [StreetGeometry.SnapTarget] = []
    var ground = GroundSurface()
    /// The street layers' draped vertices, for a click where there is no
    /// ground mesh.
    private var heights: [(point: PlanPoint, z: Double)] = []

    init() {}

    init(geometry: ReferenceGeometry) {
        kerbs = geometry.kerbs.enumerated().map { Self.target($1, layer: .kerb, index: $0) }
        let streets = geometry.cityStreets
        centrelines = streets.enumerated().map { Self.target($1, layer: .centreline, index: $0) }
        walls = (geometry.boxes + geometry.footprints).enumerated().map {
            Self.target($1, layer: .wall, index: $0)
        }.filter { !$0.points.isEmpty }
        ground = GroundSurface(heightfields: geometry.groundMesh)
        for feature in geometry.kerbs + streets {
            for i in stride(from: 0, to: feature.xyz.count - 2, by: 3) {
                heights.append(
                    (
                        PlanPoint(Double(feature.xyz[i]), Double(feature.xyz[i + 1])),
                        Double(feature.xyz[i + 2])
                    ))
            }
        }
    }

    var targets: [StreetGeometry.SnapTarget] { kerbs + centrelines + walls }

    /// How far a street vertex may lie from a click and still lend it its
    /// height, when there is no ground mesh.
    static let heightReachM = 60.0

    /// The ground's height under p: the ground mesh, else the nearest street
    /// vertex's, else 0, which Gerardus drapes like any other height.
    func groundZ(at p: PlanPoint) -> Double {
        if let z = ground.height(at: p) { return z }
        var best: (d: Double, z: Double)?
        for h in heights {
            let d = simd_length_squared(h.point - p)
            if best == nil || d < best!.d { best = (d, h.z) }
        }
        if let best, best.d <= Self.heightReachM * Self.heightReachM { return best.z }
        return 0
    }

    /// A feature's vertices across the ground. A box wall is a vertical
    /// rectangle, so its corners fall in pairs: repeats are dropped, leaving
    /// the wall's foot.
    static func target(
        _ feature: Gerardus_Align_V1alpha1_LineFeature, layer: StreetGeometry.SnapTarget.Layer,
        index: Int
    ) -> StreetGeometry.SnapTarget {
        var points = [PlanPoint]()
        for i in stride(from: 0, to: feature.xyz.count - 2, by: 3) {
            let p = PlanPoint(Double(feature.xyz[i]), Double(feature.xyz[i + 1]))
            if let last = points.last, StreetGeometry.distance(last, p) < 1e-6 { continue }
            points.append(p)
        }
        var closed = feature.closed
        if points.count > 1, let first = points.first, let last = points.last,
            StreetGeometry.distance(first, last) < 1e-6
        {
            points.removeLast()
        }
        if points.count < 3 { closed = false }
        let id = feature.featureID.isEmpty ? "\(layer.rawValue)-\(index)" : feature.featureID
        return StreetGeometry.SnapTarget(id: id, layer: layer, points: points, closed: closed)
    }
}

@available(macOS 15.0, *) @MainActor final class StreetPainterSession: ObservableObject {
    /// The editor's name, kept by Settings and the painter's Save.
    static let authorKey = "streets.author"

    /// How far, in points of the view, a press may move and still be a click.
    static let clickSlopPoints: CGFloat = 4
    static let vertexHitPoints: Double = 8
    static let edgeHitPoints: Double = 6
    static let markHitPoints: Double = 8
    /// The scale the overlay's handles assume outside the plan view.
    static let defaultMetresPerPoint = 0.05

    // MARK: - Published state

    @Published private(set) var isPainting = false
    @Published var tool: StreetPaintTool = .select {
        didSet {
            guard tool != oldValue else { return }
            drawing = []
            cursor = nil
            refresh()
        }
    }
    /// What a new mark's tags start as.
    @Published var fields = StreetUseTags.Fields() { didSet { refresh() } }
    /// How deep a row of spaces is, and whether it goes away from the street.
    @Published var rowDepthM = 2.4 { didSet { refresh() } }
    @Published var rowFlip = false { didSet { refresh() } }
    @Published private(set) var edit = StreetEditState()
    @Published var selectedKey: String? { didSet { if selectedKey != oldValue { refresh() } } }
    /// The vertices placed so far for the tool in hand.
    @Published private(set) var drawing: [SIMD3<Double>] = []
    /// Where the next vertex would go, and whether it snapped there.
    @Published private(set) var cursor: (point: SIMD3<Double>, snapped: Bool)?
    @Published var note = ""
    /// The site whose marks are held, once ListStreetUse has answered.
    @Published private(set) var loadedToken: String?
    @Published private(set) var isLoading = false
    @Published private(set) var isSaving = false
    @Published private(set) var errorMessage: String?
    @Published private(set) var savedMessage: String?
    /// The last save's problems, by the feature they name, and those naming
    /// none.
    @Published private(set) var problemsByID: [String: [String]] = [:]
    @Published private(set) var generalProblems: [String] = []
    /// Features ListStreetUse sent that the painter cannot draw.
    @Published private(set) var skipped = 0
    /// Why the server has no street use, when it answered UNIMPLEMENTED.
    @Published private(set) var unsupported: String?
    /// Whether the marks are drawn while not painting.
    @Published var showStreetUse = true { didSet { refresh() } }
    /// Keep the complete intent after a lost response; retries must not create
    /// another mark or silently adopt newer row preconditions.
    private var pendingSave: Gerardus_Align_V1alpha1_EditStreetUseRequest?
    var hasPendingSave: Bool { pendingSave != nil }
    @Published private(set) var conflictingMarks: [StreetMark] = []
    /// Why the camera pane will not give up the camera now (an orbit is
    /// playing, or the camera is flying); nil when it will. The camera pane
    /// sets it: paint mode and the pane's orbit and flight exclude each
    /// other, as their holds on the renderer's camera do.
    @Published var cameraBlocker: String?

    // MARK: - Collaborators

    let alignment: AlignmentSession
    weak var sink: StreetPaintSink? { didSet { refresh() } }
    /// The plan view while painting, and kept for the next time at this site.
    private(set) var plan: PlanView?
    private var planToken: String?
    /// The Metal view's size, in points; AppState keeps it current.
    var viewSize = CGSize(width: 800, height: 600) {
        didSet { if viewSize != oldValue, isPainting { refresh() } }
    }
    private(set) var reference = StreetReference()
    private var subscriptions = Set<AnyCancellable>()
    private var keyCounter = 0
    private var loading: Task<Void, Never>?

    private enum Gesture {
        /// The left button is down and has not moved far: a click so far.
        case pending(start: CGPoint, last: CGPoint)
        case panning(last: CGPoint)
        /// Dragging a vertex of a mark.
        case vertex(key: String, index: Int, start: CGPoint)
        /// Dragging a vertex added on an edge, before `index`.
        case insert(key: String, index: Int, start: CGPoint)
        case rightPending(start: CGPoint, last: CGPoint)
        case rightPanning(last: CGPoint)
    }
    private var gesture: Gesture?
    /// Where the vertex being dragged is now.
    private var dragPoint: SIMD3<Double>?

    init(alignment: AlignmentSession) {
        self.alignment = alignment
        alignment.$reference.sink { [weak self] summary in
            self?.referenceChanged(placed: summary != nil)
        }.store(in: &subscriptions)
        alignment.$pose.sink { [weak self] pose in self?.poseChanged(pose) }.store(
            in: &subscriptions)
        alignment.$availability.sink { [weak self] availability in
            if case .available = availability { return }
            self?.setPainting(false)
        }.store(in: &subscriptions)
    }

    // MARK: - Paint mode

    /// Why paint mode cannot start now; nil when it can.
    var paintBlocker: String? {
        if let cameraBlocker { return cameraBlocker }
        if !alignment.isAvailable { return "No Gerardus server has answered." }
        if alignment.site == nil || alignment.pose == nil || alignment.reference == nil {
            return
                "Place the sensor from its hint first: the plan view is drawn over the placed reference."
        }
        if let unsupported { return unsupported }
        return nil
    }

    var canPaint: Bool { paintBlocker == nil }

    func togglePainting() { setPainting(!isPainting) }

    /// Enters paint mode, snapping the camera to the plan view over the
    /// sensor, or leaves it and puts back the camera the operator had.
    func setPainting(_ on: Bool) {
        if on {
            guard !isPainting, canPaint, let pose = alignment.pose, let site = alignment.site else {
                return
            }
            if plan == nil || planToken != site.l16Token {
                let centre = PlanPoint(pose.translation.x, pose.translation.y)
                plan = PlanView(centre: centre, groundZ: reference.groundZ(at: centre))
                planToken = site.l16Token
            }
            isPainting = true
            pushCamera()
            if loadedToken != site.l16Token, !isLoading { scheduleLoad() }
        } else {
            guard isPainting else { return }
            isPainting = false
            drawing = []
            cursor = nil
            gesture = nil
            dragPoint = nil
            sink?.exitPlanView()
        }
        refresh()
    }

    /// `pose` is the pose about to be drawn, when it is not yet the session's.
    private func pushCamera(pose next: SitePose? = nil) {
        guard isPainting, let plan, let pose = next ?? alignment.pose else { return }
        let aspect = viewSize.height > 0 ? Float(viewSize.width / viewSize.height) : 1
        sink?.setPlanCamera(plan.camera(pose: pose, aspectRatio: aspect))
    }

    private func poseChanged(_ pose: SitePose?) {
        guard isPainting else { return }
        if let pose { pushCamera(pose: pose) } else { setPainting(false) }
    }

    /// The reference was placed, restyled or cleared. Its street layers and
    /// ground are what the painter snaps to and stands on; a newly placed
    /// site's marks are read.
    private func referenceChanged(placed: Bool) {
        guard placed else {
            setPainting(false)
            reference = StreetReference()
            if loadedToken != nil {
                loadedToken = nil
                edit = StreetEditState()
                selectedKey = nil
                problemsByID = [:]
                generalProblems = []
                skipped = 0
                unsupported = nil
                savedMessage = nil
                errorMessage = nil
            }
            refresh()
            return
        }
        // Published before the session's own `reference` changes, but after
        // the geometry it summarises is in place.
        reference = alignment.referenceGeometry.map(StreetReference.init) ?? StreetReference()
        if let token = alignment.site?.l16Token, token != loadedToken, !isLoading,
            unsupported == nil
        {
            scheduleLoad()
        }
        refresh()
    }

    // MARK: - Loading and saving

    /// The task reading the marks, for tests to await.
    var pendingLoad: Task<Void, Never>? { loading }

    /// Marked loading at once, so a second change before the task starts
    /// does not ask again.
    private func scheduleLoad() {
        isLoading = true
        loading = Task { [weak self] in await self?.load() }
    }

    /// Reads the site's marks (ListStreetUse), dropping any unsaved changes.
    ///
    /// Written without a `defer` or an early return inside `do`, as
    /// AlignmentSession.register() is: in that shape the test host aborted
    /// in swift_task_dealloc on the Xcode 26 toolchain.
    func load() async {
        guard let client = alignment.gerardusClient, let token = alignment.site?.l16Token else {
            isLoading = false
            return
        }
        isLoading = true
        errorMessage = nil
        var features: [Gerardus_Align_V1alpha1_StreetUseFeature]?
        var failure: Error?
        do { features = try await client.listStreetUse(l16Token: token) } catch { failure = error }
        isLoading = false
        // The site changed while this was asked.
        guard alignment.site?.l16Token == token else { return }
        if let features {
            pendingSave = nil
            conflictingMarks = []
            let read = StreetUseEditing.marks(from: features)
            edit = StreetEditState(base: read.marks)
            skipped = read.skipped
            loadedToken = token
            selectedKey = nil
            drawing = []
            problemsByID = [:]
            generalProblems = []
            unsupported = nil
        } else if let rpc = failure as? RPCError, rpc.code == .unimplemented {
            unsupported =
                "This Gerardus has no street use: it answered ListStreetUse UNIMPLEMENTED."
            loadedToken = nil
            setPainting(false)
        } else if let failure {
            errorMessage = "No street use read: " + GerardusClientError.describe(failure)
        }
        refresh()
    }

    /// Why Save is off now, one line each; empty when it is on.
    func saveBlockers(author: String) -> [String] {
        var reasons = [String]()
        if loadedToken == nil { reasons.append("No marks read for this site yet.") }
        if !edit.isDirty { reasons.append("No changes to save.") }
        if !drawing.isEmpty { reasons.append("Finish or cancel the mark being drawn first.") }
        if author.trimmingCharacters(in: .whitespaces).isEmpty {
            reasons.append(
                "No author: type your name in Author; Gerardus records who marked each feature.")
        }
        if isSaving {
            reasons.append("Saving.")
        } else if isLoading {
            reasons.append("Reading the marks.")
        }
        return reasons
    }

    /// Sends every change since the marks were read as one EditStreetUse,
    /// all or nothing, then draws the marks Gerardus answers with: it snaps
    /// and drapes every vertex, so they are not quite the ones sent.
    func save(author: String) async {
        guard saveBlockers(author: author).isEmpty, let client = alignment.gerardusClient,
            let token = loadedToken
        else { return }
        let edits = edit.pendingEdits
        var request = Gerardus_Align_V1alpha1_EditStreetUseRequest()
        request.l16Token = token
        request.author = author.trimmingCharacters(in: .whitespaces)
        request.note = note.trimmingCharacters(in: .whitespacesAndNewlines)
        request.edits = edits
        let retrying = pendingSave != nil
        if let pendingSave {
            request.requestID = pendingSave.requestID
            guard request == pendingSave else {
                errorMessage =
                    "An earlier save has an unknown outcome. Keep its draft unchanged to retry, or reload the marks before making a new save."
                return
            }
        } else {
            request.requestID = UUID().uuidString
        }
        pendingSave = request
        isSaving = true
        errorMessage = nil
        savedMessage = nil
        var response: Gerardus_Align_V1alpha1_EditStreetUseResponse?
        do { response = try await client.editStreetUse(request) } catch {
            errorMessage = "Draft kept: " + GerardusClientError.describe(error)
            if let rpc = error as? RPCError, rpc.code == .aborted {
                // Read current state separately: never overwrite the draft or
                // automatically retry it against the newly read row IDs.
                if let latest = try? await client.listStreetUse(l16Token: token) {
                    conflictingMarks = StreetUseEditing.marks(from: latest).marks
                }
                errorMessage =
                    "Conflict: your draft is kept. Reload marks to discard it and start from the current version."
            } else if !retrying && GerardusClientError.isDefiniteRejection(error) {
                // Correct rejected geometry without discarding other drafts.
                pendingSave = nil
            }
        }
        isSaving = false
        if let response, loadedToken == token { saved(response, sent: edits.count) }
    }

    private func saved(_ response: Gerardus_Align_V1alpha1_EditStreetUseResponse, sent: Int) {
        pendingSave = nil
        conflictingMarks = []
        let read = StreetUseEditing.marks(from: response.features)
        edit = StreetEditState(base: read.marks)
        skipped = read.skipped
        let mapped = StreetUseEditing.mapProblems(
            response.problems, ids: read.marks.map(\.featureID))
        problemsByID = mapped.byID
        generalProblems = mapped.general
        selectedKey = nil
        note = ""
        let n = read.marks.count
        let flagged = response.problems.count
        savedMessage =
            "Saved \(sent) change\(sent == 1 ? "" : "s"); Gerardus holds \(n) mark\(n == 1 ? "" : "s") for this site"
            + (flagged > 0
                ? ", with \(flagged) problem\(flagged == 1 ? "" : "s") to look at." : ".")
        refresh()
    }

    /// The last save's problems for a mark.
    func problems(for mark: StreetMark) -> [String] {
        mark.featureID.isEmpty ? [] : problemsByID[mark.featureID] ?? []
    }

    // MARK: - Editing

    var selected: StreetMark? { edit.mark(selectedKey) }

    private func newKey() -> String {
        keyCounter += 1
        return "new-\(keyCounter)"
    }

    /// Makes the mark the tool's vertices describe, or keeps drawing when
    /// there are too few yet.
    func finishDrawing() {
        guard let kind = tool.kind else { return }
        let points = drawing
        var made: [SIMD3<Double>]?
        switch tool {
        case .row:
            if points.count == 2, let row = rowPreview(points[0], points[1]) {
                made = row.ring.map { SIMD3($0, reference.groundZ(at: $0)) }
            }
        default:
            if points.count >= kind.minVertices, points.count <= kind.maxVertices { made = points }
        }
        guard let made else {
            // Not enough vertices yet: keep drawing.
            if points.count >= kind.maxVertices || tool.finishesAtTwo { drawing = [] }
            refresh()
            return
        }
        drawing = []
        let mark = StreetMark(
            key: newKey(), featureID: "", kind: kind, points: made,
            tags: StreetUseTags.tags(for: kind, fields))
        edit.add(mark)
        selectedKey = mark.key
        savedMessage = nil
        refresh()
    }

    func cancelDrawing() {
        drawing = []
        refresh()
    }

    func undo() {
        if !drawing.isEmpty {
            drawing.removeLast()
        } else {
            edit.undo()
            if edit.mark(selectedKey) == nil { selectedKey = nil }
        }
        refresh()
    }

    func redo() {
        edit.redo()
        if edit.mark(selectedKey) == nil { selectedKey = nil }
        refresh()
    }

    var canUndo: Bool { !drawing.isEmpty || edit.canUndo }
    var canRedo: Bool { drawing.isEmpty && edit.canRedo }

    func deleteSelected() {
        guard let key = selectedKey else { return }
        edit.remove(key)
        selectedKey = nil
        refresh()
    }

    /// Turns the selected lane round.
    func reverseSelected() {
        guard let key = selectedKey, selected?.kind == .travelLane else { return }
        edit.update(key, StreetUseEditing.reversed)
        refresh()
    }

    /// Sets a tag on the selected mark, as one undoable change.
    func setSelectedTag(_ key: String, _ value: String) {
        guard let selectedKey, selected?.tags[key] != value else { return }
        edit.update(selectedKey) { StreetUseEditing.settingTag($0, key, value) }
        refresh()
    }

    /// Drops every unsaved change.
    func revert() {
        edit.revert()
        drawing = []
        if edit.mark(selectedKey) == nil { selectedKey = nil }
        refresh()
    }

    /// Selects a mark and, while painting, centres the view on it.
    func focus(_ mark: StreetMark) {
        selectedKey = mark.key
        guard isPainting, !mark.points.isEmpty else { return }
        let points = mark.plan
        plan?.centre = points.reduce(PlanPoint.zero, +) / Double(points.count)
        pushCamera()
        refresh()
    }

    // MARK: - Pointing

    private var metresPerPoint: Double {
        guard isPainting, let plan else { return Self.defaultMetresPerPoint }
        return plan.metresPerPoint(viewHeight: viewSize.height)
    }

    /// Everything a vertex snaps to: the street layers and the marks, but
    /// not the mark being edited.
    private func snapTargets(excluding key: String?) -> [StreetGeometry.SnapTarget] {
        reference.targets
            + edit.marks.filter { $0.key != key }.map {
                StreetGeometry.SnapTarget(
                    id: $0.key, layer: .mark, points: $0.plan, closed: $0.kind.closed)
            }
    }

    /// The ground under a point of the view, snapped as Gerardus will snap it.
    func point(
        atView p: CGPoint, excluding key: String? = nil
    ) -> (point: SIMD3<Double>, snapped: Bool)? {
        guard let plan else { return nil }
        let site = plan.sitePoint(atView: p, size: viewSize)
        let snap = StreetGeometry.snap(site, to: snapTargets(excluding: key))
        return (SIMD3(snap.point, reference.groundZ(at: snap.point)), snap.snapped)
    }

    /// The mark under a point of the view: a lane, space or driveway within
    /// reach, topmost first, else the smallest surface holding the point.
    func mark(atView p: CGPoint) -> StreetMark? {
        guard let plan else { return nil }
        let site = plan.sitePoint(atView: p, size: viewSize)
        let reach = Self.markHitPoints * metresPerPoint
        for mark in edit.marks.reversed() where mark.kind != .surface {
            var within = reach
            if mark.kind == .driveway { within = max(reach, StreetPaintStyle.drivewayWidthM / 2) }
            if mark.kind == .travelLane, let width = Double(mark.tags[StreetUseTags.widthM] ?? "") {
                within = max(reach, width / 2)
            }
            let d = StreetGeometry.distance(
                from: site, toPolyline: mark.plan, closed: mark.kind.closed)
            if d <= within
                || (mark.kind == .parkingSpace && StreetGeometry.contains(mark.plan, site))
            {
                return mark
            }
        }
        return edit.marks.filter {
            $0.kind == .surface
                && (StreetGeometry.contains($0.plan, site)
                    || StreetGeometry.distance(from: site, toPolyline: $0.plan, closed: true)
                        <= reach)
        }.min { abs(StreetGeometry.signedArea($0.plan)) < abs(StreetGeometry.signedArea($1.plan)) }
    }

    private func vertex(of mark: StreetMark, atView p: CGPoint) -> Int? {
        guard let plan else { return nil }
        let site = plan.sitePoint(atView: p, size: viewSize)
        let reach = Self.vertexHitPoints * metresPerPoint
        let near = mark.plan.enumerated().map { ($0, StreetGeometry.distance($1, site)) }.filter {
            $0.1 <= reach
        }
        return near.min { $0.1 < $1.1 }?.0
    }

    private static func moved(_ a: CGPoint, _ b: CGPoint) -> Bool {
        hypot(a.x - b.x, a.y - b.y) > clickSlopPoints
    }

    func mouseDown(at p: CGPoint) {
        guard isPainting, let plan else { return }
        if tool == .select, let mark = selected {
            if let index = vertex(of: mark, atView: p) {
                gesture = .vertex(key: mark.key, index: index, start: p)
                dragPoint = mark.points[index]
                refresh()
                return
            }
            if mark.points.count < mark.kind.maxVertices,
                let at = StreetGeometry.insertion(
                    at: plan.sitePoint(atView: p, size: viewSize), into: mark.plan,
                    closed: mark.kind.closed, tolerance: Self.edgeHitPoints * metresPerPoint)
            {
                gesture = .insert(key: mark.key, index: at.index, start: p)
                dragPoint = SIMD3(at.point, reference.groundZ(at: at.point))
                refresh()
                return
            }
        }
        gesture = .pending(start: p, last: p)
    }

    func mouseDragged(to p: CGPoint) {
        guard isPainting else { return }
        switch gesture {
        case .pending(let start, let last):
            if Self.moved(start, p) {
                gesture = .panning(last: last)
                pan(from: last, to: p)
            } else {
                gesture = .pending(start: start, last: last)
            }
        case .panning(let last): pan(from: last, to: p)
        case .vertex(let key, _, _), .insert(let key, _, _):
            dragPoint = point(atView: p, excluding: key)?.point
            refresh()
        default: break
        }
    }

    func mouseUp(at p: CGPoint, clickCount: Int) {
        guard isPainting else { return }
        let ended = gesture
        gesture = nil
        switch ended {
        case .vertex(let key, let index, let start):
            if Self.moved(start, p), let to = point(atView: p, excluding: key)?.point {
                edit.update(key) { StreetUseEditing.moveVertex($0, index, to: to) }
            }
        case .insert(let key, let index, let start):
            let to = Self.moved(start, p) ? point(atView: p, excluding: key)?.point : dragPoint
            if let to { edit.update(key) { StreetUseEditing.insertVertex($0, at: index, to) } }
        case .pending: click(at: p, clickCount: clickCount)
        default: break
        }
        dragPoint = nil
        refresh()
    }

    private func click(at p: CGPoint, clickCount: Int) {
        if tool == .select {
            selectedKey = mark(atView: p)?.key
            return
        }
        // The second click of a double-click finishes a polygon or a lane;
        // the first already placed its vertex.
        if clickCount >= 2 {
            if !tool.finishesAtTwo { finishDrawing() }
            return
        }
        guard let at = point(atView: p)?.point else { return }
        if let last = drawing.last,
            StreetGeometry.distance(PlanPoint(last.x, last.y), PlanPoint(at.x, at.y)) < 0.01
        {
            return
        }
        drawing.append(at)
        if tool.finishesAtTwo, drawing.count == 2 { finishDrawing() }
        if let kind = tool.kind, drawing.count >= kind.maxVertices, !tool.finishesAtTwo {
            finishDrawing()
        }
    }

    func rightMouseDown(at p: CGPoint) {
        guard isPainting else { return }
        gesture = .rightPending(start: p, last: p)
    }

    func rightMouseDragged(to p: CGPoint) {
        guard isPainting else { return }
        switch gesture {
        case .rightPending(let start, let last):
            if Self.moved(start, p) {
                gesture = .rightPanning(last: last)
                pan(from: last, to: p)
            }
        case .rightPanning(let last): pan(from: last, to: p)
        default: break
        }
    }

    /// A right-click removes the vertex under it from the selected mark, or
    /// the last vertex placed while drawing.
    func rightMouseUp(at p: CGPoint) {
        guard isPainting else { return }
        let ended = gesture
        gesture = nil
        guard case .rightPending = ended else { return }
        if tool != .select {
            if !drawing.isEmpty { drawing.removeLast() }
        } else if let mark = selected, let index = vertex(of: mark, atView: p) {
            edit.update(mark.key) { StreetUseEditing.removeVertex($0, index) }
        }
        refresh()
    }

    func mouseMoved(to p: CGPoint) {
        guard isPainting, tool != .select else { return }
        cursor = point(atView: p)
        refresh()
    }

    func mouseExited() {
        guard cursor != nil else { return }
        cursor = nil
        refresh()
    }

    /// A scroll of `delta` (positive zooms in) about a point of the view.
    func zoom(delta: CGFloat, at p: CGPoint) {
        guard isPainting, var plan else { return }
        plan.zoom(by: exp(-0.1 * Double(delta)), about: p, size: viewSize)
        self.plan = plan
        pushCamera()
        refresh()
    }

    private func pan(from a: CGPoint, to b: CGPoint) {
        guard var plan else { return }
        switch gesture {
        case .rightPending, .rightPanning: gesture = .rightPanning(last: b)
        default: gesture = .panning(last: b)
        }
        plan.pan(byViewDelta: b.x - a.x, b.y - a.y, size: viewSize)
        self.plan = plan
        pushCamera()
    }

    enum Key {
        case finish
        case cancel
        case delete
    }

    /// Return finishes the mark being drawn; Escape cancels it, or clears the
    /// selection; Delete takes back the last vertex placed, or deletes the
    /// selected mark. True when the key was used.
    func key(_ key: Key) -> Bool {
        guard isPainting else { return false }
        switch key {
        case .finish:
            guard !drawing.isEmpty else { return false }
            finishDrawing()
        case .cancel:
            if !drawing.isEmpty {
                cancelDrawing()
            } else if selectedKey != nil {
                selectedKey = nil
            } else {
                return false
            }
        case .delete:
            if !drawing.isEmpty {
                drawing.removeLast()
                refresh()
            } else if selectedKey != nil {
                deleteSelected()
            } else {
                return false
            }
        }
        return true
    }

    // MARK: - Drawing

    private func rowPreview(_ a: SIMD3<Double>, _ b: SIMD3<Double>) -> StreetGeometry.SpaceRow? {
        let pa = PlanPoint(a.x, a.y)
        let pb = PlanPoint(b.x, b.y)
        let side = StreetGeometry.streetSide(pa, pb, centrelines: reference.centrelines)
        return StreetGeometry.rowOfSpaces(
            pa, pb, count: max(1, fields.count), depth: rowDepthM, side: rowFlip ? -side : side)
    }

    /// What the overlay shows now.
    var scene: StreetPaintScene {
        var scene = StreetPaintScene()
        scene.marks = edit.marks
        if case .vertex(let key, let index, _) = gesture, let dragPoint,
            let i = scene.marks.firstIndex(where: { $0.key == key })
        {
            scene.marks[i] = StreetUseEditing.moveVertex(scene.marks[i], index, to: dragPoint)
            scene.dragKey = key
            scene.dragIndex = index
        }
        if case .insert(let key, let index, _) = gesture, let dragPoint,
            let i = scene.marks.firstIndex(where: { $0.key == key })
        {
            scene.marks[i] = StreetUseEditing.insertVertex(scene.marks[i], at: index, dragPoint)
            scene.dragKey = key
            scene.dragIndex = index
        }
        scene.selectedKey = tool == .select ? selectedKey : nil
        scene.flagged = Set(problemsByID.keys)
        scene.tool = tool
        scene.drawing = drawing
        scene.cursor = cursor?.point
        scene.cursorSnapped = cursor?.snapped ?? false
        if tool == .row, drawing.count == 1, let a = drawing.first, let b = cursor?.point {
            scene.rowPreview = rowPreview(a, b)
            scene.rowPreviewZ = (a.z + b.z) / 2
        }
        scene.painting = isPainting
        scene.metresPerPoint = metresPerPoint
        return scene
    }

    /// Rebuilds the overlay and hands it to the renderer.
    func refresh() {
        guard let sink else { return }
        guard isPainting || (showStreetUse && !edit.marks.isEmpty) else {
            sink.setStreetOverlay(nil)
            return
        }
        sink.setStreetOverlay(StreetPaintStyle.vertices(scene))
    }
}

extension MetalRenderer: StreetPaintSink {}
