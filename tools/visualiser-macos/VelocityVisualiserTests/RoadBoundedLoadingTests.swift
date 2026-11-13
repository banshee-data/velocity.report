// RoadBoundedLoadingTests.swift
// The driving view's working set: draw within 75 m, fetch to 150 m, drop
// beyond 200 m; eviction keeps skeletons so a route survives it; windows ask
// the server for the eye's neighbourhood; and the traverse meter's numbers.

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

/// The fixture's chunks as a server filtering by `centre` and `radius_m`
/// would send them: whole patches with any part within the radius.
private func windowed(
    _ request: Gerardus_Align_V1alpha1_StreamReferenceRequest
) throws -> [(Gerardus_Align_V1alpha1_RoadSurfaceChunk, Bool)] {
    let chunks = try RoadFixture.chunks(site: request.l16Token)
    guard request.radiusM > 0 else { return chunks }
    let centre = SIMD2<Float>(Float(request.centre.x), Float(request.centre.y))
    return chunks.map { chunk, last in
        var c = chunk
        c.patches = c.patches.filter { RoadPatch($0).distance(from: centre) <= request.radiusM }
        return (c, last)
    }
}

/// Waits (up to 15 s) for `condition`; the suite runs beside the whole app's
/// tests, so waits are on state, not on the clock.
@MainActor func eventually(_ condition: () -> Bool) async -> Bool {
    for _ in 0..<3000 {
        if condition() { return true }
        try? await Task.sleep(for: .milliseconds(5))
    }
    return condition()
}

/// Waits until the corridor has loaded and neighbour expansion has stopped
/// reloading it (the state stays `.loaded` with five patches for 400 ms).
@MainActor func settled(_ road: RoadReviewSession) async {
    var steady = 0
    for _ in 0..<3000 {
        if road.loader.state == .loaded && road.loader.store.patches.count == 5 { steady += 1 } else { steady = 0 }
        if steady >= 80 { return }
        try? await Task.sleep(for: .milliseconds(5))
    }
}

private let straightID = "gms-preview:road/fixture/segment/straight/0"
private let bendID = "gms-preview:road/fixture/segment/bend/0"
private let junctionID = "gms-preview:road/fixture/junction/9000001/0"

struct RoadWorkingSetTests {
    private func patches() throws -> [RoadPatch] { try RoadFixture.store(site: "80858771f").patches }

    @Test func policyRadiiMustNest() {
        #expect(RoadResidencyPolicy().isValid)
        #expect(RoadResidencyPolicy().visibleM == 75 && RoadResidencyPolicy().prefetchM == 150)
        #expect(RoadResidencyPolicy().evictM == 200)
        #expect(!RoadResidencyPolicy(visibleM: 100, prefetchM: 50, evictM: 200).isValid)
        #expect(!RoadResidencyPolicy(visibleM: 0, prefetchM: 50, evictM: 200).isValid)
    }

    @Test func visibleKeepsOnlyResidentPatchesWithinTheRadius() throws {
        let all = try patches()
        // At the junction everything in the fixture is within 75 m.
        let eye = SIMD2<Float>(-33, 44)
        let policy = RoadResidencyPolicy()
        #expect(RoadWorkingSet.visible(all, eye: eye, policy: policy).count == all.count)
        // Far west: only the patches whose bounds reach within the radius.
        let far = SIMD2<Float>(-300, 44)
        #expect(RoadWorkingSet.visible(all, eye: far, policy: policy).isEmpty)
        let narrow = RoadResidencyPolicy(visibleM: 20, prefetchM: 40, evictM: 60)
        let near = RoadWorkingSet.visible(all, eye: SIMD2(-95, 44), policy: narrow)
        #expect(near.map(\.patchID) == ["gms-preview:road/fixture/segment/unsupported/0"] || near.count <= 2)
        #expect(near.count < all.count)
        // A skeleton is never drawn.
        let skeletons = all.map { $0.skeleton() }
        #expect(RoadWorkingSet.visible(skeletons, eye: eye, policy: policy).isEmpty)
    }

    @Test func theWindowMovesAfterAQuarterOfThePrefetchRadius() {
        let p = RoadResidencyPolicy()
        #expect(RoadWorkingSet.needsWindow(eye: .zero, lastRequestedAt: nil, policy: p))
        #expect(!RoadWorkingSet.needsWindow(eye: SIMD2(30, 0), lastRequestedAt: .zero, policy: p))
        #expect(RoadWorkingSet.needsWindow(eye: SIMD2(38, 0), lastRequestedAt: .zero, policy: p))
    }
}

struct RoadEvictionTests {
    @Test func evictionKeepsTheSkeletonAndFreesTheMesh() throws {
        var store = try RoadFixture.store(site: "80858771f")
        let before = store.residentMeshBytes
        #expect(before > 0 && store.residentPatches.count == 5)
        let n = store.evictMeshes(beyond: 60, from: SIMD2(-95, 44))
        #expect(n > 0 && n < 5)
        #expect(store.residentMeshBytes < before)
        #expect(store.patches.count == 5)
        let gone = try #require(store.patches.first { !$0.isResident })
        #expect(gone.vertices.isEmpty && gone.indices.isEmpty)
        // Rings, profile, stats and hashes remain.
        #expect(!gone.rings.isEmpty)
        #expect(!gone.geometryHash.isEmpty)
        #expect(gone.bounds != nil)
        #expect(gone.defect() == nil)
        // Nothing within the radius is dropped.
        let straight = try #require(store.patch(id: "gms-preview:road/fixture/segment/unsupported/0"))
        #expect(straight.isResident)
    }

    @Test func aSkeletonBecomesResidentAgainWhenItsMeshArrives() throws {
        var store = try RoadFixture.store(site: "80858771f")
        let full = store.patches
        store.evictMeshes(beyond: 0, from: SIMD2(-1000, 0))
        #expect(store.residentPatches.isEmpty)
        for p in full { store.add(p) }
        #expect(store.residentPatches.count == 5)
        #expect(store.duplicates == 0)
        #expect(store.conflicts.isEmpty)
    }

    @Test func aRouteKeepsItsHeightsAfterItsMeshIsEvicted() throws {
        let all = try RoadFixture.store(site: "80858771f").patches
        let route = RoadRouteBuilder.route(
            from: straightID, reversed: true, patches: all, index: RoadHeightIndex(patches: all))
        // Rebuild with the straight evicted: its heights come from the profile.
        let evicted = all.map { $0.patchID == straightID ? $0.skeleton() : $0 }
        let again = RoadRouteBuilder.route(
            from: straightID, reversed: true, patches: evicted,
            index: RoadHeightIndex(patches: evicted))
        #expect(again.stations.count == route.stations.count)
        #expect(again.stations.allSatisfy { !$0.isMissing })
        for (a, b) in zip(route.stations, again.stations) where a.patchID == straightID {
            #expect(abs(a.position.z - b.position.z) < 0.03)
        }
    }

    @Test func aRouteStoppedForLackOfGeometryContinuesWhenItArrives() throws {
        let all = try RoadFixture.store(site: "80858771f").patches
        let without = all.filter { $0.patchID != junctionID && $0.patchID != bendID }
        let first = RoadRouteBuilder.route(
            from: straightID, reversed: true, patches: without,
            index: RoadHeightIndex(patches: without))
        guard case .unloadedGeometry? = first.stop else {
            Issue.record("stop \(String(describing: first.stop))")
            return
        }
        let before = first.stations
        let more = RoadRouteBuilder.extended(
            first, patches: all, index: RoadHeightIndex(patches: all))
        #expect(more.patchIDs.contains(junctionID) && more.patchIDs.contains(bendID))
        #expect(more.stop == .endOfRoute)
        // What was driven is untouched, and distance carries on without a jump.
        #expect(Array(more.stations.prefix(before.count)) == before)
        for (a, b) in zip(more.stations, more.stations.dropFirst()) {
            #expect(b.s > a.s)
            #expect(simd_length(SIMD2(b.position.x - a.position.x, b.position.y - a.position.y)) < 1.6)
            #expect(abs(b.position.z - a.position.z) < 0.35)
        }
        // A route that stopped for another reason is returned as it is.
        let done = RoadRouteBuilder.extended(more, patches: all, index: RoadHeightIndex(patches: all))
        #expect(done.stations.count == more.stations.count)
    }
}

@MainActor struct RoadBoundedSessionTests {
    private func setup(
        policy: RoadResidencyPolicy
    ) async throws -> (RoadDriveSession, RoadReviewSession, FakeAlignmentService, FakeCameraSink) {
        let service = FakeAlignmentService()
        service.roadSurfaceVersion = 1
        service.listedSites = [
            Gerardus_Align_V1alpha1_Site.fixture(RoadFixture.siteA),
            Gerardus_Align_V1alpha1_Site.fixture(RoadFixture.siteB),
        ]
        service.roadSurfaceHandler = { request, onChunk in
            for (chunk, last) in try windowed(request) { await onChunk(chunk, last) }
        }
        let alignment = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await alignment.probe()
        let road = RoadReviewSession(alignment: alignment)
        road.paneToggled(open: true)
        await road.refreshSites()
        road.open(siteToken: "80858771f")
        await settled(road)
        let drive = RoadDriveSession(review: road)
        drive.policy = policy
        let sink = FakeCameraSink()
        drive.sink = sink
        return (drive, road, service, sink)
    }

    @Test func drivingDrawsOnlyWithinTheVisibleRadiusAndEvictsBeyondTheKeepRadius() async throws {
        let narrow = RoadResidencyPolicy(visibleM: 25, prefetchM: 40, evictM: 40)
        let (drive, road, _, _) = try await setup(policy: narrow)
        let all = road.patches.count
        drive.begin(on: "gms-preview:road/fixture/segment/unsupported/0")
        _ = await eventually { !road.overlay.isEmpty && road.visibleCount < all }
        #expect(road.bounded != nil)
        #expect(road.visibleCount < all)
        #expect(road.visibleCount >= 1)
        // Far patches were dropped to skeletons; all remain known.
        let resident = road.loader.store.residentPatches.count
        #expect(resident < all)
        #expect(road.patches.count == all)
        // Overhead gives back the whole corridor.
        drive.overhead()
        #expect(road.bounded == nil)
        for _ in 0..<4000 where road.loader.store.residentPatches.count < all {
            try await Task.sleep(for: .milliseconds(5))
        }
        #expect(road.loader.store.residentPatches.count == all)
    }

    @Test func windowsAskTheServerForTheEyesNeighbourhood() async throws {
        let narrow = RoadResidencyPolicy(visibleM: 25, prefetchM: 50, evictM: 70)
        let (drive, road, service, _) = try await setup(policy: narrow)
        let seen = LockedRequests()
        service.roadSurfaceHandler = { request, onChunk in
            seen.add(request)
            for (chunk, last) in try windowed(request) { await onChunk(chunk, last) }
        }
        drive.begin(on: "gms-preview:road/fixture/segment/straight/0", reversed: true)
        _ = await eventually { seen.all.contains { $0.radiusM > 0 } }
        let requests = seen.all.filter { $0.radiusM > 0 }
        #expect(!requests.isEmpty)
        #expect(requests.allSatisfy { $0.radiusM == 50 })
        #expect(requests.allSatisfy { $0.layers == [.roadSurface] })
        // The centre is in each site's own frame: B's is A's shifted south by
        // the sites' offset, so the two differ.
        let a = requests.first { $0.l16Token == "80858771f" }
        let b = requests.first { $0.l16Token == "808587721" }
        if let a, let b { #expect(abs(a.centre.y - b.centre.y) > 100) }
        _ = road
    }

    @Test func aLateWindowFromAReplacedCorridorChangesNothing() async throws {
        let (_, road, service, _) = try await setup(policy: RoadResidencyPolicy())
        let gate = AsyncGate()
        service.roadSurfaceHandler = { request, onChunk in
            await gate.wait()
            for (chunk, last) in try RoadFixture.chunks(site: request.l16Token) {
                await onChunk(chunk, last)
            }
        }
        let sites = road.sites.map(\.source)
        road.loader.extend(
            around: .zero, radius: 50, sites: sites, client: service, serverRoadSurfaceVersion: 1)
        try await Task.sleep(for: .milliseconds(80))
        road.loader.reset()
        await gate.open()
        _ = await eventually { road.loader.discardedLate >= 1 }
        #expect(road.loader.store.isEmpty)
        #expect(road.loader.discardedLate >= 1)
    }
}

final class LockedRequests: @unchecked Sendable {
    private let lock = NSLock()
    private var items: [Gerardus_Align_V1alpha1_StreamReferenceRequest] = []
    func add(_ r: Gerardus_Align_V1alpha1_StreamReferenceRequest) {
        lock.lock()
        items.append(r)
        lock.unlock()
    }
    var all: [Gerardus_Align_V1alpha1_StreamReferenceRequest] {
        lock.lock()
        defer { lock.unlock() }
        return items
    }
    var count: Int { all.count }
}

extension Gerardus_Align_V1alpha1_Site {
    static func fixture(_ s: RoadSiteSource) -> Gerardus_Align_V1alpha1_Site {
        var out = Gerardus_Align_V1alpha1_Site()
        out.l16Token = s.l16Token
        out.label = s.l16Token
        out.frame.origin.latitudeDeg = s.originLatitudeDeg
        out.frame.origin.longitudeDeg = s.originLongitudeDeg
        return out
    }
}

struct TraverseMeterTests {
    @Test func percentilesAreNearestRank() {
        let xs = (1...100).map(Double.init)
        #expect(TraverseMeter.percentile(xs, 0.5) == 50)
        #expect(TraverseMeter.percentile(xs, 0.95) == 95)
        #expect(TraverseMeter.percentile(xs, 0.99) == 99)
        #expect(TraverseMeter.percentile([], 0.95) == 0)
        #expect(TraverseMeter.percentile([7], 0.95) == 7)
    }

    @Test func summaryReportsTheTargetFootprintAndGrowth() {
        var meter = TraverseMeter()
        #expect(meter.summary() == nil)
        for i in 0..<200 {
            meter.record(
                TraverseSample(
                    interval: i == 199 ? 0.030 : 0.0167, tickMs: 0.2, encodeMs: 0.8, gpuMs: 1.1,
                    drawCalls: 6, visiblePatches: 3, residentPatches: 5,
                    residentMeshBytes: 1_000_000, footprintBytes: UInt64(300 + min(i, 100)) << 20))
        }
        let s = try! #require(meter.summary())
        #expect(s.frames == 200)
        #expect(s.framesOver16_7 == 200 || s.framesOver16_7 >= 1)
        #expect(s.intervalMaxMs == 30)
        #expect(s.drawCallsMax == 6 && s.drawCallsMin == 6)
        #expect(s.footprintMaxMB == 400)
        // Grew for the first 100 frames then flat: the second half does not grow.
        #expect(s.footprintSecondHalfGrowthMB == 0)
        let p = TraverseProvenance(
            mac: "Mac", os: "15", buildSHA: "abc", buildVersion: "1", settings: "s",
            inputHashes: ["build:x": "h"])
        let note = TraverseReport.markdown(s, p, date: "2026-11-13", label: "test")
        #expect(note.contains("git abc") && note.contains("`h`") && note.contains("p95"))
    }

    @Test func provenanceNamesTheMachineBuildAndInputHashes() throws {
        let store = try RoadFixture.store(site: "80858771f")
        let p = TraverseProvenance.current(
            settings: "x", corridor: store.corridors, patches: store.patches)
        #expect(!p.mac.isEmpty && !p.os.isEmpty)
        #expect(p.inputHashes["patch_geometry_hashes_sha256"]?.count == 64)
        #expect(p.inputHashes["build:fixture"]?.count == 64)
        // The same inputs give the same hash.
        let q = TraverseProvenance.current(
            settings: "y", corridor: store.corridors, patches: store.patches.reversed())
        #expect(p.inputHashes == q.inputHashes)
        #expect(ProcessMemory.footprintBytes() > 0)
    }
}

/// A ten-minute traverse of the fixture at 60 Hz of virtual time, with the
/// working set tight enough that patches are fetched and evicted on every
/// pass. Measures the app's own per-frame CPU (route, working set, camera,
/// overlay hand-off) and resident memory, not the GPU: the GPU side is read
/// from the renderer in the running app (see the review note).
@MainActor struct RoadTraverseBenchmarkTests {
    @Test func tenMinuteTraverseSettlesAndStaysCheap() async throws {
        let service = FakeAlignmentService()
        service.roadSurfaceVersion = 1
        service.listedSites = [.fixture(RoadFixture.siteA), .fixture(RoadFixture.siteB)]
        service.roadSurfaceHandler = { request, onChunk in
            for (chunk, last) in try windowed(request) { await onChunk(chunk, last) }
        }
        let alignment = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await alignment.probe()
        let road = RoadReviewSession(alignment: alignment)
        road.paneToggled(open: true)
        await road.refreshSites()
        road.open(siteToken: "80858771f")
        await settled(road)
        let drive = RoadDriveSession(review: road)
        drive.policy = RoadResidencyPolicy(visibleM: 25, prefetchM: 40, evictM: 40)
        drive.sink = FakeCameraSink()
        var now: TimeInterval = 1000
        drive.clock = { now }
        drive.begin(on: "gms-preview:road/fixture/segment/straight/0", reversed: true)
        drive.speed = 10
        drive.startTraverse(seconds: 600)
        let clock = ContinuousClock()
        var frames = 0
        while drive.traverseUntil != nil && frames < 40_000 {
            now += 1.0 / 60
            let began = clock.now
            drive.tick()
            let spent = clock.now - began
            let ms = Double(spent.components.attoseconds) / 1e15 + Double(spent.components.seconds) * 1000
            drive.noteFrame(interval: 1.0 / 60, encodeMs: ms, gpuMs: 0)
            frames += 1
            if frames % 20 == 0 { try await Task.sleep(for: .microseconds(300)) }
        }
        let summary = try #require(drive.lastTraverse)
        print("TRAVERSE-BENCH", summary)
        #expect(summary.frames >= 35_000)
        #expect(drive.traverseLoops >= 1)
        // The app's own work per frame stays far inside the 16.7 ms budget.
        #expect(summary.encodeP95Ms < 4, "tick p95 \(summary.encodeP95Ms) ms")
        // Memory settles with the local content: no growth over the second half.
        #expect(summary.footprintSecondHalfGrowthMB < 30, "grew \(summary.footprintSecondHalfGrowthMB) MB")
        // Never more than the corridor's five patches resident, and fewer when evicting.
        #expect(summary.residentPatchesMax <= 5)
        drive.overhead()
    }
}
