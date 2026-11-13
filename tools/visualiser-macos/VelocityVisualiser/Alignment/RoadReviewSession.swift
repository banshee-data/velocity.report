// RoadReviewSession.swift
// The road surface review: pick a prepared site on the Gerardus connection,
// load its road surface preview and its neighbours' into one frame, draw it
// with the evidence layers, select a patch and read what is known about it.
// No sensor and no recording are needed: the reference-only view uses the
// first site's frame as the scene frame and says so.
//
// Honesty kept here: the coverage summary reports the corridor's inventory
// (surfaced, inferred, missing lengths) beside the patches, the gaps list the
// selected roads with no patch, and a missing bundle, an old server or a
// failed load is said in words, never shown as an empty view.

import AppKit
import Combine
import Foundation
import simd

/// Where the road overlay goes and where the camera is moved for review.
/// MetalRenderer is the real one.
protocol RoadOverlaySink: AnyObject {
    func setRoadOverlay(_ vertices: RoadOverlayVertices?)
    /// Frames the corridor from above, at a tilt, in the scene frame.
    func frameRoad(centre: SIMD3<Float>, extent: Float)
}

/// A prepared site the operator can open a corridor through.
struct RoadSiteChoice: Identifiable, Equatable, Sendable {
    var source: RoadSiteSource
    var label: String
    var id: String { source.l16Token }

    init(_ site: Gerardus_Align_V1alpha1_Site) {
        source = RoadSiteSource(site)
        label = site.label.isEmpty ? site.l16Token : site.label
    }
}

/// A built corridor the server names, with the sites that hold it.
struct RoadCorridorChoice: Identifiable, Equatable, Sendable {
    var info: RoadCorridorInfo
    var description: String
    var patchCount: Int
    var l16Tokens: [String]
    var id: String { info.corridorID }

    init(_ s: Gerardus_Align_V1alpha1_RoadCorridorSummary) {
        info = RoadCorridorInfo(s.layer)
        description = s.description_p
        patchCount = Int(s.patchCount)
        l16Tokens = s.l16Tokens
    }
}

/// The inspector's reading of one patch.
struct RoadPatchInspector: Equatable {
    var patchID: String
    var cnn: String
    var name: String
    var kind: String
    var boundarySource: RoadBoundarySourceClass
    var edgeSources: [RoadBoundarySourceClass: Int]
    var observed: String
    var sourceVersion: String
    var areaM2: Float
    var observedAreaM2: Float
    var interpolatedAreaM2: Float
    var missingAreaM2: Float
    var riderLine: (observed: Float, interpolated: Float, missing: Float)
    var heldOutPoints: Int
    var groundPoints: Int
    var residualMedianM: Float
    var residualRmsM: Float
    var residualP95AbsM: Float
    var geometryHash: String
    var notAssessed: Int
    var assessed: Int
    var corridor: String

    static func == (a: Self, b: Self) -> Bool {
        a.patchID == b.patchID && a.geometryHash == b.geometryHash
    }

    init(_ patch: RoadPatch, corridor: String) {
        patchID = patch.patchID
        cnn = patch.cnn
        name = patch.name
        kind = patch.kind == .junction ? "junction" : "segment"
        boundarySource = patch.boundarySource
        edgeSources = patch.rings.flatMap(\.edgeSource).reduce(into: [:]) {
            $0[$1, default: 0] += 1
        }
        observed = patch.observed
        sourceVersion = patch.sourceVersion
        areaM2 = patch.stats.areaM2
        observedAreaM2 = patch.stats.observedAreaM2
        interpolatedAreaM2 = patch.stats.interpolatedAreaM2
        missingAreaM2 = patch.stats.missingAreaM2
        riderLine = (
            patch.stats.riderLineObserved, patch.stats.riderLineInterpolated,
            patch.stats.riderLineMissing
        )
        heldOutPoints = patch.stats.heldOutPoints
        groundPoints = patch.stats.groundPoints
        residualMedianM = patch.stats.residualMedianM
        residualRmsM = patch.stats.residualRmsM
        residualP95AbsM = patch.stats.residualP95AbsM
        geometryHash = patch.geometryHash
        notAssessed = patch.assessments.filter { !$0.assessed }.count
        assessed = patch.assessments.filter(\.assessed).count
        self.corridor = corridor
    }
}

/// Lengths and counts so a preview never implies complete park coverage.
struct RoadCoverageSummary: Equatable {
    var corridorID: String
    var patchCount: Int
    var inventoryLengthM: Float
    var surfacedLengthM: Float
    var inferredLengthM: Float
    var missingLengthM: Float
    var gaps: [RoadGapInfo]
    var frameAccuracyM: Float
    var buildHash: String

    /// Share of the inventory surfaced, 0–1; 0 with no inventory.
    var surfacedShare: Float { inventoryLengthM > 0 ? surfacedLengthM / inventoryLengthM : 0 }
}

@available(macOS 15.0, *) @MainActor final class RoadReviewSession: ObservableObject {
    /// How far from a corridor's patches a prepared site's origin is read too, m.
    /// A bundle covers its S2 cell and the eight around it, so a site whose
    /// origin is within a few hundred metres can hold the corridor's patches.
    nonisolated static let neighbourRadiusM: Double = 350
    nonisolated static let expansionRounds = 2

    @Published var options = RoadViewOptions() { didSet { if options != oldValue { restyle() } } }
    @Published var selectedPatchID: String? {
        didSet { if oldValue != selectedPatchID { restyle() } }
    }
    @Published private(set) var sites: [RoadSiteChoice] = []
    /// The corridors the server lists, when it can (`road_corridors_listed`).
    @Published private(set) var corridors: [RoadCorridorChoice] = []
    @Published private(set) var sitesNote: String?
    @Published private(set) var openedSite: String?
    @Published private(set) var contextNote: String?
    @Published private(set) var isPaneOpen = false
    /// The mesh checks over every resident patch, rebuilt when the corridor
    /// changes and not while a bounded drive is moving the working set.
    @Published private(set) var meshAudit: RoadMeshAudit?
    private var auditGeneration = 0
    private var auditedMeshes: [RoadMeshIdentity] = []
    private(set) var auditBuilds = 0

    let sceneWillChange = PassthroughSubject<Void, Never>()
    let loader = RoadSurfaceLoader()
    weak var sink: RoadOverlaySink?
    /// The reference overlay's sink (kerbs, class-2 points); the alignment
    /// session's. Used only while no site is placed there.
    weak var referenceSink: ReferenceOverlaySink?
    private let alignment: AlignmentSession
    private var subscriptions = Set<AnyCancellable>()
    private var expansion: Task<Void, Never>?
    private var contextGeneration = 0
    private var openedCorridorID: String?
    private(set) var overlay = RoadOverlayVertices()
    /// Every patch held, resident or skeleton.
    var patches: [RoadPatch] { loader.store.patches }
    /// The driving view's working set: the eye and the radii, or nil to draw
    /// and keep everything loaded.
    private(set) var bounded: (eye: SIMD2<Float>, policy: RoadResidencyPolicy)?
    private var restyleGeneration = 0
    private var lastVisibleIDs: Set<String> = []
    /// Patches in the last overlay built.
    private(set) var visibleCount = 0
    private var lastWindowCentre: SIMD2<Float>?
    /// Overlay builds finished, for the traverse report.
    private(set) var overlayBuilds = 0

    init(alignment: AlignmentSession) {
        self.alignment = alignment
        loader.$store.sink { [weak self] _ in DispatchQueue.main.async { self?.restyle() } }.store(
            in: &subscriptions)
        loader.objectWillChange.sink { [weak self] _ in
            DispatchQueue.main.async { self?.objectWillChange.send() }
        }.store(in: &subscriptions)
    }

    // MARK: - Availability

    var availability: RoadSurfaceAvailability? { alignment.roadSurfaceAvailability }

    /// Why nothing can be shown, in words; nil when something can.
    var blocker: String? {
        if alignment.availability == .unavailable { return "No Gerardus server answers." }
        if let availability, !availability.isAvailable { return availability.message }
        if case .unavailable(let why) = loader.state { return why.message }
        if case .failed(let why) = loader.state { return "Load failed: \(why)" }
        return nil
    }

    var statusLine: String {
        switch loader.state {
        case .idle: return openedSite == nil ? "Choose a site." : "Not loaded."
        case .loading(let n): return "Loading… \(n) message\(n == 1 ? "" : "s")"
        case .loaded: return "Loaded \(patches.count) patches."
        case .partial(let why): return "Partial: \(patches.count) patches, then \(why)."
        case .failed(let why): return "Failed: \(why)"
        case .unavailable(let why): return why.message
        case .cancelled: return "Cancelled."
        }
    }

    // MARK: - Sites

    func paneToggled(open: Bool) {
        isPaneOpen = open
        if open { Task { await refreshSites() } } else { restyle() }
    }

    func refreshSites() async {
        guard let client = alignment.gerardusClient else {
            sites = []
            sitesNote = "Not connected to a Gerardus server."
            return
        }
        do {
            let listed = try await client.listSites()
            sites = listed.map(RoadSiteChoice.init).sorted { $0.label < $1.label }
            sitesNote = sites.isEmpty ? "The server lists no prepared sites." : nil
            if alignment.serverInfo?.roadCorridorsListed == true {
                corridors = try await client.listRoadCorridors().map(RoadCorridorChoice.init)
                if corridors.isEmpty {
                    sitesNote =
                        "The server lists no built road corridor; sites below are probed one by one."
                }
            } else {
                corridors = []
            }
        } catch {
            sites = []
            sitesNote = "Could not list sites: \(GerardusClientError.describe(error))"
        }
    }

    // MARK: - Opening

    /// Opens a named corridor through exactly the sites the server says hold
    /// it: no probing, no neighbour search.
    func open(corridor id: String) {
        guard let choice = corridors.first(where: { $0.id == id }) else {
            sitesNote = "Corridor \(id) is not one this server lists."
            return
        }
        guard let client = alignment.gerardusClient,
            let version = alignment.serverInfo?.roadSurfaceVersion
        else {
            sitesNote = "Not connected to a Gerardus server."
            return
        }
        let held = choice.l16Tokens.compactMap { token in sites.first { $0.id == token }?.source }
        guard !held.isEmpty else {
            sitesNote = "None of corridor \(id)'s sites is a prepared bundle on this server."
            return
        }
        sceneWillChange.send()
        expansion?.cancel()
        contextGeneration += 1
        bounded = nil
        lastWindowCentre = nil
        openedCorridorID = id
        openedSite = held[0].l16Token
        selectedPatchID = nil
        loader.open(
            corridor: id, sites: held, client: client, serverRoadSurfaceVersion: version,
            selectedCorridorID: id)
        let contextMine = contextGeneration
        Task { await loadContext(centreSite: held[0], client: client, generation: contextMine) }
    }

    /// Opens the road surface through `token`'s bundle and its prepared
    /// neighbours. Cancels a load in flight; its late chunks are discarded.
    func open(siteToken token: String) {
        guard let client = alignment.gerardusClient else {
            sitesNote = "Not connected to a Gerardus server."
            return
        }
        guard let version = alignment.serverInfo?.roadSurfaceVersion else {
            sitesNote = "The server has not answered yet."
            return
        }
        guard let first = sites.first(where: { $0.id == token }) else {
            sitesNote = "Site \(token) is not a prepared bundle on this server."
            return
        }
        sceneWillChange.send()
        expansion?.cancel()
        contextGeneration += 1
        bounded = nil
        lastWindowCentre = nil
        openedCorridorID = nil
        openedSite = token
        selectedPatchID = nil
        loader.open(
            corridor: "site \(token)", sites: [first.source], client: client,
            serverRoadSurfaceVersion: version)
        expansion = Task { [weak self] in
            await self?.expand(from: first, client: client, version: version)
        }
        let contextMine = contextGeneration
        Task {
            await loadContext(centreSite: first.source, client: client, generation: contextMine)
        }
    }

    /// Reads the sites around what loaded, up to `expansionRounds` times.
    private func expand(
        from first: RoadSiteChoice, client: AlignmentServiceClient, version: UInt32
    ) async {
        var requested = [first.source]
        for _ in 0..<Self.expansionRounds {
            // Wait for the load in flight.
            while loader.state.isLoading {
                if Task.isCancelled { return }
                try? await Task.sleep(for: .milliseconds(20))
            }
            guard case .loaded = loader.state, !Task.isCancelled else { return }
            let next = Self.neighbours(
                of: loader.store.patches, scene: first.source, sites: sites.map(\.source),
                excluding: requested)
            guard !next.isEmpty else { return }
            requested += next
            loader.open(
                corridor: loader.corridorKey, sites: requested, client: client,
                serverRoadSurfaceVersion: version)
        }
    }

    /// Prepared sites whose origin is within `neighbourRadiusM` of the patches'
    /// extent, in the scene frame, that are not already read.
    nonisolated static func neighbours(
        of patches: [RoadPatch], scene: RoadSiteSource, sites: [RoadSiteSource],
        excluding: [RoadSiteSource]
    ) -> [RoadSiteSource] {
        var lo = SIMD2<Double>(repeating: .infinity)
        var hi = SIMD2<Double>(repeating: -.infinity)
        for patch in patches {
            guard let b = patch.bounds else { continue }
            lo = simd_min(lo, SIMD2(Double(b.min.x), Double(b.min.y)))
            hi = simd_max(hi, SIMD2(Double(b.max.x), Double(b.max.y)))
        }
        guard lo.x <= hi.x else { return [] }
        let taken = Set(excluding.map(\.l16Token))
        return sites.filter { site in
            guard !taken.contains(site.l16Token) else { return false }
            let o = site.offset(in: scene)
            let dx = max(lo.x - o.x, 0, o.x - hi.x)
            let dy = max(lo.y - o.y, 0, o.y - hi.y)
            return (dx * dx + dy * dy).squareRoot() <= neighbourRadiusM
        }
    }

    func cancelLoad() {
        expansion?.cancel()
        loader.cancel()
    }

    func close() {
        sceneWillChange.send()
        contextGeneration += 1
        restyleGeneration += 1
        auditGeneration += 1
        auditedMeshes = []
        contextNote = nil
        meshAudit = nil
        bounded = nil
        lastWindowCentre = nil
        expansion?.cancel()
        loader.reset()
        openedCorridorID = nil
        openedSite = nil
        selectedPatchID = nil
        sink?.setRoadOverlay(nil)
    }

    // MARK: - Context layers

    /// City kerbs and class-2 points beside the road, through the existing
    /// reference overlay, only while no site is placed there.
    private func loadContext(
        centreSite: RoadSiteSource, client: AlignmentServiceClient, generation mine: Int
    ) async {
        guard mine == contextGeneration else { return }
        guard alignment.referenceGeometry == nil, let referenceSink else { return }
        var request = Gerardus_Align_V1alpha1_StreamReferenceRequest()
        request.l16Token = centreSite.l16Token
        request.layers = [.kerbs, .cityStreets, .aerialPoints]
        request.aerialClasses = [2]
        request.radiusM = 250
        do {
            let geometry = try await client.streamReference(request)
            guard mine == contextGeneration, alignment.referenceGeometry == nil else { return }
            referenceSink.setReferenceOverlay(ReferenceStyle.vertices(for: geometry))
            contextNote =
                "Context: \(geometry.kerbs.count) City kerb lines, \(geometry.cityStreets.count) centrelines, \(geometry.aerialCount) class-2 points (first site's bundle only)."
            applyContextVisibility()
        } catch {
            guard mine == contextGeneration else { return }
            contextNote = "Context layers unavailable: \(GerardusClientError.describe(error))"
        }
    }

    private func applyContextVisibility() {
        guard alignment.referenceGeometry == nil else { return }
        var v = ReferenceVisibility()
        v.aerial = options.class2Points
        v.kerbs = options.cityKerbs
        v.cityStreets = options.cityCentrelines
        v.footprints = false
        v.boxes = false
        v.registeredScans = false
        v.observed = false
        v.wallPlanes = false
        v.groundMesh = false
        v.buildings = false
        v.laneMarkings = false
        v.streetFurniture = false
        v.trees = false
        referenceSink?.setReferenceVisibility(v)
    }

    // MARK: - Drawing and selection

    /// Rebuilds the overlay on a worker and swaps it in on the main actor
    /// between frames. A build that finishes after a newer one was asked for is
    /// dropped.
    private func restyle() {
        guard isPaneOpen || openedSite != nil else { return }
        restyleGeneration += 1
        let mine = restyleGeneration
        let shown: [RoadPatch]
        if let bounded {
            shown = RoadWorkingSet.visible(patches, eye: bounded.eye, policy: bounded.policy)
            lastVisibleIDs = Set(shown.map(\.patchID))
        } else {
            shown = loader.store.residentPatches
        }
        visibleCount = shown.count
        let options = options
        let selected = selectedPatchID
        let all = loader.store.residentPatches
        let auditKey = all.map(RoadMeshIdentity.init)
        if bounded == nil, auditKey != auditedMeshes {
            auditedMeshes = auditKey
            auditGeneration += 1
            auditBuilds += 1
            let auditMine = auditGeneration
            Task.detached(priority: .utility) { [weak self] in
                let result = all.isEmpty ? nil : RoadMeshAudit.audit(all)
                await MainActor.run {
                    guard let self, auditMine == self.auditGeneration else { return }
                    if self.meshAudit != result { self.meshAudit = result }
                }
            }
        }
        Task.detached(priority: .userInitiated) { [weak self] in
            let built = RoadSurfaceStyle.vertices(
                patches: shown, options: options, selected: selected)
            await MainActor.run {
                guard let self, mine == self.restyleGeneration else { return }
                self.overlay = built
                self.overlayBuilds += 1
                self.sink?.setRoadOverlay(built)
                self.applyContextVisibility()
            }
        }
    }

    // MARK: - Bounded loading

    /// Starts keeping to a working set around the eye: draws within the visible
    /// radius, and (through `updateEye`) fetches and evicts around it.
    func enterBounded(eye: SIMD2<Float>, policy: RoadResidencyPolicy = RoadResidencyPolicy()) {
        guard policy.isValid else { return }
        bounded = (eye, policy)
        lastWindowCentre = nil
        updateEye(eye)
    }

    /// Back to everything loaded: the overview wants the whole corridor, so it
    /// is read again in full.
    func leaveBounded() {
        guard bounded != nil else { return }
        bounded = nil
        lastWindowCentre = nil
        if let id = openedCorridorID {
            open(corridor: id)
        } else if let token = openedSite {
            open(siteToken: token)
        } else {
            restyle()
        }
    }

    /// Moves the eye: redraws only when the visible set changed, asks for the
    /// window around it when it has moved far enough, and drops meshes beyond
    /// the eviction radius.
    func updateEye(_ eye: SIMD2<Float>) {
        guard var current = bounded else { return }
        current.eye = eye
        bounded = current
        let policy = current.policy
        // Evict first, only when there is something to drop.
        if loader.store.patches.contains(where: {
            $0.isResident && $0.distance(from: eye) > policy.evictM
        }) {
            loader.evict(beyond: policy.evictM, from: eye)
        }
        if RoadWorkingSet.needsWindow(eye: eye, lastRequestedAt: lastWindowCentre, policy: policy),
            let client = alignment.gerardusClient,
            let version = alignment.serverInfo?.roadSurfaceVersion,
            let scene = sites.first(where: { $0.id == loader.sceneToken })?.source
        {
            lastWindowCentre = eye
            let reach = Double(policy.prefetchM) + Self.neighbourRadiusM
            let near = sites.map(\.source).filter { src in
                let o = src.offset(in: scene)
                return (pow(o.x - Double(eye.x), 2) + pow(o.y - Double(eye.y), 2)).squareRoot()
                    <= reach
            }
            loader.extend(
                around: eye, radius: policy.prefetchM, sites: [scene] + near.filter { $0 != scene },
                client: client, serverRoadSurfaceVersion: version)
        }
        let ids = RoadWorkingSet.visibleIDs(patches, eye: eye, policy: policy)
        if ids != lastVisibleIDs { restyle() }
    }

    /// The patch under a ray, selected and returned.
    @discardableResult func pick(origin: SIMD3<Float>, direction: SIMD3<Float>) -> RoadPatch? {
        guard
            let hit = RoadSurfaceStyle.pick(patches: patches, origin: origin, direction: direction)
        else {
            selectedPatchID = nil
            return nil
        }
        selectedPatchID = hit.patchID
        return loader.store.patch(id: hit.patchID)
    }

    var selectedPatch: RoadPatch? { selectedPatchID.flatMap { loader.store.patch(id: $0) } }

    var inspector: RoadPatchInspector? {
        guard let patch = selectedPatch else { return nil }
        return RoadPatchInspector(
            patch, corridor: loader.store.corridor(of: patch)?.corridorID ?? "")
    }

    var coverage: [RoadCoverageSummary] {
        loader.store.corridors.map { c in
            RoadCoverageSummary(
                corridorID: c.corridorID,
                patchCount: loader.store.patches.filter {
                    loader.store.corridorOf[$0.patchID] == c.corridorID
                }.count, inventoryLengthM: c.inventoryLengthM, surfacedLengthM: c.surfacedLengthM,
                inferredLengthM: c.inferredLengthM, missingLengthM: c.missingLengthM, gaps: c.gaps,
                frameAccuracyM: c.frameAccuracyM, buildHash: c.buildHash)
        }
    }

    /// Frames the loaded corridor from above.
    func frameCorridor() {
        var lo = SIMD2<Float>(repeating: .infinity)
        var hi = SIMD2<Float>(repeating: -.infinity)
        var z: Float = 0
        var n: Float = 0
        for p in patches {
            guard let b = p.bounds else { continue }
            lo = simd_min(lo, b.min)
            hi = simd_max(hi, b.max)
            for v in p.vertices where v.z.isFinite {
                z += v.z
                n += 1
            }
        }
        guard lo.x <= hi.x, n > 0 else { return }
        let c = (lo + hi) / 2
        sink?.frameRoad(centre: SIMD3(c.x, c.y, z / n), extent: simd_length(hi - lo))
    }
}

extension MetalRenderer: RoadOverlaySink {
    func frameRoad(centre: SIMD3<Float>, extent: Float) {
        // The driving camera and the plan view hold the camera; leave them be.
        guard cameraHolder == nil else { return }
        let reach = max(extent, 30)
        camera.target = centre
        camera.position = centre + SIMD3(0, -0.35 * reach, 0.95 * reach)
        camera.up = SIMD3(0, 0, 1)
        camera.farPlane = max(camera.farPlane, reach * 4)
        view?.needsDisplay = true
    }

    /// The road camera and overlay share the review's own site frame,
    /// independently of the capture's reference alignment matrix.
    func rayInSiteFrame(
        at point: CGPoint, viewSize: CGSize
    ) -> (origin: SIMD3<Float>, direction: SIMD3<Float>)? {
        CameraRay.ray(camera: camera, viewPoint: point, viewSize: viewSize)
    }
}
