// RoadSurfaceLoader.swift
// Loads the road surface preview (layer 17) for one corridor, through one or
// more neighbouring site bundles, into one scene frame.
//
// Rules the loader keeps:
//  - Ask only when GetServerInfo says road_surface_version is 1. An empty
//    `layers` list never returns layer 17, so the layer is always named.
//  - A new `open` cancels the previous load and bumps a generation; a late
//    chunk or error from the old load changes nothing.
//  - A failed or cancelled load keeps what arrived and says it is partial.
//  - The same patch_id through two site bundles is one patch.
//
// Frames. Each site bundle is in its own east-north-up frame. The first site
// is the scene frame; the others are shifted into it by the difference of
// their geodetic origins. Over the few hundred metres of a corridor that
// matches the server's frames to a few millimetres (the fixture test checks
// it against the manifest's control points), which is inside the layer's own
// stated frame accuracy for the prototype's purposes but is not a substitute
// for it: `RoadSurfaceLoader.frameShiftBoundM` records what this client adds.

import Combine
import Foundation
import GRPCCore
import simd

/// One site bundle a corridor is read through.
struct RoadSiteSource: Sendable, Equatable {
    var l16Token: String
    var originLatitudeDeg: Double
    var originLongitudeDeg: Double

    init(l16Token: String, originLatitudeDeg: Double, originLongitudeDeg: Double) {
        self.l16Token = l16Token
        self.originLatitudeDeg = originLatitudeDeg
        self.originLongitudeDeg = originLongitudeDeg
    }

    init(_ site: Gerardus_Align_V1alpha1_Site) {
        self.init(
            l16Token: site.l16Token, originLatitudeDeg: site.frame.origin.latitudeDeg,
            originLongitudeDeg: site.frame.origin.longitudeDeg)
    }

    /// Where `self`'s origin sits in `scene`'s east-north-up frame, in double
    /// precision, on the WGS84 ellipsoid at the mean latitude. Up is zero:
    /// both frames put z at NAVD88 height.
    func offset(in scene: RoadSiteSource) -> SIMD3<Double> {
        let a = 6_378_137.0
        let e2 = 0.006_694_379_990_14
        let lat = (originLatitudeDeg + scene.originLatitudeDeg) / 2 * .pi / 180
        let s = sin(lat)
        let w = (1 - e2 * s * s).squareRoot()
        let prime = a / w
        let meridian = a * (1 - e2) / (w * w * w)
        let dLon = (originLongitudeDeg - scene.originLongitudeDeg) * .pi / 180
        let dLat = (originLatitudeDeg - scene.originLatitudeDeg) * .pi / 180
        return SIMD3(dLon * prime * cos(lat), dLat * meridian, 0)
    }
}

/// How a load stands.
enum RoadLoadState: Equatable, Sendable {
    case idle
    case loading(messages: Int)
    case loaded
    /// Some of it arrived, then it failed or was cancelled.
    case partial(reason: String)
    case failed(String)
    /// The server cannot send it, with why.
    case unavailable(RoadSurfaceAvailability)
    case cancelled

    var isLoading: Bool { if case .loading = self { return true } else { return false } }
}

@available(macOS 15.0, *) @MainActor final class RoadSurfaceLoader: ObservableObject {
    /// What this client adds to the layer's own frame accuracy when it shifts a
    /// neighbouring bundle into the scene frame: the tangent-plane
    /// approximation over a corridor under 1 km, rounded up.
    nonisolated static let frameShiftBoundM: Float = 0.005

    @Published private(set) var store = RoadSurfaceStore()
    @Published private(set) var state: RoadLoadState = .idle
    @Published private(set) var corridorKey: String = ""
    /// The scene frame's site token: the first site of the open corridor.
    @Published private(set) var sceneToken: String = ""
    /// Sites that answered NOT_FOUND or otherwise gave nothing, with why.
    @Published private(set) var siteNotes: [String: String] = [:]

    private var completedStreams = Set<String>()
    private var streamLayers: [String: Gerardus_Align_V1alpha1_RoadSurfaceLayer] = [:]
    private var selectedCorridorID: String?
    private var windowGeneration = 0
    private var generation = 0
    private var task: Task<Void, Never>?
    private var windowTask: Task<Void, Never>?
    /// The last window fetch that failed, in a line.
    @Published private(set) var windowNote: String?

    /// Messages the caller can read in tests: how many chunks were applied.
    private(set) var appliedChunks = 0
    /// Chunks that arrived for a load that had been replaced.
    private(set) var discardedLate = 0

    /// Opens a corridor: the road layer of every site in `sites`, in order,
    /// merged into the first site's frame. Cancels any load in flight.
    func open(
        corridor: String, sites: [RoadSiteSource], client: AlignmentServiceClient,
        serverRoadSurfaceVersion: UInt32, selectedCorridorID: String? = nil
    ) {
        cancel()
        generation += 1
        let mine = generation
        store = RoadSurfaceStore()
        completedStreams = []
        streamLayers = [:]
        self.selectedCorridorID = selectedCorridorID
        windowNote = nil
        siteNotes = [:]
        appliedChunks = 0
        corridorKey = corridor
        sceneToken = sites.first?.l16Token ?? ""
        let availability = RoadSurfaceAvailability.forServer(version: serverRoadSurfaceVersion)
        guard availability.isAvailable else {
            state = .unavailable(availability)
            return
        }
        guard let scene = sites.first else {
            state = .failed("No site bundle names this corridor")
            return
        }
        state = .loading(messages: 0)
        task = Task { [weak self] in
            await self?.run(generation: mine, scene: scene, sites: sites, client: client)
        }
    }

    /// Stops the load; what arrived stays, marked partial.
    func cancel() {
        generation += 1
        windowGeneration += 1
        windowTask?.cancel()
        task?.cancel()
        task = nil
        if state.isLoading { state = store.isEmpty ? .cancelled : .partial(reason: "cancelled") }
    }

    /// Forgets everything, as when the site or corridor is left.
    func reset() {
        windowGeneration += 1
        windowNote = nil
        windowTask?.cancel()
        task?.cancel()
        task = nil
        generation += 1
        store = RoadSurfaceStore()
        state = .idle
        corridorKey = ""
        sceneToken = ""
        siteNotes = [:]
    }

    private func run(
        generation mine: Int, scene: RoadSiteSource, sites: [RoadSiteSource],
        client: AlignmentServiceClient
    ) async {
        var failure: String?
        var refusal: RoadSurfaceAvailability?
        for site in sites {
            if Task.isCancelled || mine != generation { return }
            let shift = site.offset(in: scene)
            let offset = SIMD3<Float>(Float(shift.x), Float(shift.y), Float(shift.z))
            var request = Gerardus_Align_V1alpha1_StreamReferenceRequest()
            request.l16Token = site.l16Token
            request.layers = [.roadSurface]
            do {
                try await client.streamRoadSurface(request) { [weak self] chunk, last in
                    // The conversion from the wire is the heavy part: it runs
                    // here, off the main actor, and only the finished patches
                    // are handed over.
                    let prepared = RoadSurfaceLoader.prepare(
                        chunk, from: site, offset: offset, sceneToken: scene.l16Token)
                    await self?.apply(
                        prepared, complete: last, stream: site.l16Token, generation: mine)
                }
                guard mine == generation else { return }
                if !completedStreams.contains(site.l16Token) {
                    failure = "\(site.l16Token): stream ended before layer_complete"
                    siteNotes[site.l16Token] = failure
                }
            } catch is CancellationError {
                if mine == generation { finish(partial: "cancelled", generation: mine) }
                return
            } catch let error as RPCError {
                if mine != generation { return }
                if error.code == .cancelled {
                    finish(partial: "cancelled", generation: mine)
                    return
                }
                if error.code == .notFound {
                    siteNotes[site.l16Token] = "No bundle prepared for site \(site.l16Token)"
                    continue
                }
                if error.code == .invalidArgument,
                    let r = RoadSurfaceAvailability.forRefusal(
                        code: "\(error.code)", message: error.message)
                {
                    refusal = r
                    break
                }
                failure = "\(site.l16Token): \(GerardusClientError.describe(error))"
                siteNotes[site.l16Token] = failure
            } catch {
                if mine != generation { return }
                failure = "\(site.l16Token): \(GerardusClientError.describe(error))"
                siteNotes[site.l16Token] = failure
            }
        }
        guard mine == generation else { return }
        if let refusal {
            state = .unavailable(refusal)
        } else if let failure {
            state = store.isEmpty ? .failed(failure) : .partial(reason: failure)
        } else if store.isEmpty {
            // Complete layers with no patch, or every site missing.
            if siteNotes.count == sites.count {
                state = .failed(siteNotes.values.sorted().joined(separator: "; "))
            } else {
                state = .unavailable(.emptyLayer)
            }
        } else if !siteNotes.isEmpty {
            state = .partial(reason: siteNotes.values.sorted().joined(separator: "; "))
        } else {
            store.markComplete()
            state = .loaded
        }
    }

    private func finish(partial reason: String, generation mine: Int) {
        guard mine == generation else { return }
        state = store.isEmpty ? .cancelled : .partial(reason: reason)
    }

    /// A chunk converted and moved into the scene frame, ready to store.
    struct Prepared: Sendable {
        var layer: Gerardus_Align_V1alpha1_RoadSurfaceLayer?
        var patches: [RoadPatch]
    }

    nonisolated static func prepare(
        _ chunk: Gerardus_Align_V1alpha1_RoadSurfaceChunk, from site: RoadSiteSource,
        offset: SIMD3<Float>, sceneToken: String
    ) -> Prepared {
        let patches = chunk.patches.map { proto -> RoadPatch in
            let patch = RoadPatch(proto, frameToken: site.l16Token)
            return offset == .zero ? patch : patch.translated(by: offset, frameToken: sceneToken)
        }
        return Prepared(layer: chunk.hasLayer ? chunk.layer : nil, patches: patches)
    }

    /// Applies one prepared chunk unless its load has been replaced.
    private func apply(
        _ chunk: Prepared, complete: Bool, stream: String, generation mine: Int, window: Int? = nil
    ) {
        guard mine == generation, window == nil || window == windowGeneration else {
            discardedLate += 1
            return
        }
        if complete { completedStreams.insert(stream) }
        if let layer = chunk.layer { streamLayers[stream] = layer }
        if let selectedCorridorID, streamLayers[stream]?.corridorID != selectedCorridorID { return }
        if let layer = streamLayers[stream] { store.appendLayer(layer) }
        for patch in chunk.patches { store.add(patch) }
        appliedChunks += 1
        if case .loading = state { state = .loading(messages: appliedChunks) }
    }

    // MARK: - Windows

    /// Adds the patches within `radius` of `centre` (scene frame) through each
    /// of `sites`, to what is already held, without resetting it: the fetch
    /// window following the eye. A window that fails leaves what is held, and
    /// says so in `windowNote`. Late answers for a replaced corridor are
    /// discarded like any other.
    func extend(
        around centre: SIMD2<Float>, radius: Float, sites: [RoadSiteSource],
        client: AlignmentServiceClient, serverRoadSurfaceVersion: UInt32
    ) {
        guard serverRoadSurfaceVersion == RoadSurfaceAvailability.supportedVersion,
            let scene = sites.first
        else { return }
        let mine = generation
        windowGeneration += 1
        streamLayers = streamLayers.filter { !$0.key.hasPrefix("window/") }
        completedStreams = completedStreams.filter { !$0.hasPrefix("window/") }
        let window = windowGeneration
        windowTask?.cancel()
        windowTask = Task { [weak self] in
            for site in sites {
                if Task.isCancelled { return }
                let shift = site.offset(in: scene)
                let offset = SIMD3<Float>(Float(shift.x), Float(shift.y), 0)
                var request = Gerardus_Align_V1alpha1_StreamReferenceRequest()
                request.l16Token = site.l16Token
                request.layers = [.roadSurface]
                // The site's own frame: the scene point minus the site's offset.
                request.centre.x = Double(centre.x) - shift.x
                request.centre.y = Double(centre.y) - shift.y
                request.radiusM = radius
                do {
                    let stream = "window/\(window)/\(site.l16Token)"
                    try await client.streamRoadSurface(request) { chunk, last in
                        let prepared = RoadSurfaceLoader.prepare(
                            chunk, from: site, offset: offset, sceneToken: scene.l16Token)
                        await self?.apply(
                            prepared, complete: last, stream: stream, generation: mine,
                            window: window)
                    }
                    await self?.windowFinished(
                        site: site.l16Token, error: nil, generation: mine, window: window)
                } catch {
                    await self?.windowFinished(
                        site: site.l16Token, error: error, generation: mine, window: window)
                }
            }
        }
    }

    private func windowFinished(site: String, error: Error?, generation mine: Int, window: Int) {
        guard mine == generation, window == windowGeneration else { return }
        if let error {
            if let rpc = error as? RPCError, rpc.code == .notFound || rpc.code == .cancelled {
                return
            }
            windowNote = "Window at \(site) failed: \(GerardusClientError.describe(error))"
        } else if !completedStreams.contains("window/\(window)/\(site)") {
            windowNote = "Window at \(site) failed: stream ended before layer_complete"
        } else if windowNote?.hasPrefix("Window at \(site)") == true {
            windowNote = nil
        }
    }

    /// Drops the meshes beyond `radius` of `eye`; see RoadSurfaceStore.
    @discardableResult func evict(beyond radius: Float, from eye: SIMD2<Float>) -> Int {
        let n = store.evictMeshes(beyond: radius, from: eye)
        return n
    }
}
