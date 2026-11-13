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

    private var generation = 0
    private var task: Task<Void, Never>?

    /// Messages the caller can read in tests: how many chunks were applied.
    private(set) var appliedChunks = 0
    /// Chunks that arrived for a load that had been replaced.
    private(set) var discardedLate = 0

    /// Opens a corridor: the road layer of every site in `sites`, in order,
    /// merged into the first site's frame. Cancels any load in flight.
    func open(
        corridor: String, sites: [RoadSiteSource], client: AlignmentServiceClient,
        serverRoadSurfaceVersion: UInt32
    ) {
        cancel()
        generation += 1
        let mine = generation
        store = RoadSurfaceStore()
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
        task?.cancel()
        task = nil
        if state.isLoading {
            generation += 1
            state = store.isEmpty ? .cancelled : .partial(reason: "cancelled")
        }
    }

    /// Forgets everything, as when the site or corridor is left.
    func reset() {
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
                    await self?.apply(
                        chunk, complete: last, from: site, offset: offset, generation: mine)
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

    /// Applies one chunk unless its load has been replaced.
    private func apply(
        _ chunk: Gerardus_Align_V1alpha1_RoadSurfaceChunk, complete: Bool,
        from site: RoadSiteSource, offset: SIMD3<Float>, generation mine: Int
    ) {
        guard mine == generation else {
            discardedLate += 1
            return
        }
        if chunk.hasLayer { store.appendLayer(chunk.layer) }
        // Move each patch into the scene frame before it joins the store, so
        // dedupe by patch_id compares like with like.
        for proto in chunk.patches {
            let patch = RoadPatch(proto, frameToken: site.l16Token)
            store.add(
                offset == .zero ? patch : patch.translated(by: offset, frameToken: sceneToken))
        }
        appliedChunks += 1
        if case .loading = state { state = .loading(messages: appliedChunks) }
    }
}
