// RoadDriveTests.swift
// The driving view's core on Gerardus's fixture and on synthetic patches:
// routes chain through the junction without jumping between roads, the eye
// stays on the road, and the driver stops, with its reason, at missing
// height, unloaded geometry and an unresolved deck choice.

import Foundation
import MetalKit
import Testing
import simd

@testable import VelocityVisualiser

private let straightID = "gms-preview:road/fixture/segment/straight/0"
private let bendID = "gms-preview:road/fixture/segment/bend/0"
private let islandID = "gms-preview:road/fixture/segment/island/0"
private let unsupportedID = "gms-preview:road/fixture/segment/unsupported/0"
private let junctionID = "gms-preview:road/fixture/junction/9000001/0"

/// Point in polygon (ring in plan), even-odd.
private func inside(_ p: SIMD2<Float>, _ ring: RoadRing) -> Bool {
    var c = false
    let n = ring.points.count
    var j = n - 1
    for i in 0..<n {
        let a = ring.points[i]
        let b = ring.points[j]
        if (a.y > p.y) != (b.y > p.y), p.x < (b.x - a.x) * (p.y - a.y) / (b.y - a.y) + a.x {
            c.toggle()
        }
        j = i
    }
    return c
}

/// Whether a station is on the road: inside the outer ring of the patch it
/// belongs to, not in one of its holes.
private func onRoad(_ st: RouteStation, _ patches: [RoadPatch]) -> Bool {
    guard let patch = patches.first(where: { $0.patchID == st.patchID }) else { return false }
    let p = SIMD2(st.position.x, st.position.y)
    guard let outer = patch.rings.first(where: { !$0.hole }) else { return false }
    // The line may sit on a ring edge at a patch end: allow a centimetre.
    let near = RoadSurfaceStyle.distance(p, to: outer) < 0.05
    if !(inside(p, outer) || near) { return false }
    return !patch.rings.filter(\.hole).contains { inside(p, $0) }
}

struct RoadRouteTests {
    private func fixture() throws -> (patches: [RoadPatch], index: RoadHeightIndex) {
        let patches = try RoadFixture.store(site: "80858771f").patches
        return (patches, RoadHeightIndex(patches: patches))
    }

    @Test func routeThroughTheJunctionFollowsTheMeshWithoutJumping() throws {
        let (patches, index) = try fixture()
        let route = RoadRouteBuilder.route(
            from: straightID, reversed: true, patches: patches, index: index)
        #expect(route.patchIDs == [straightID, junctionID, bendID])
        #expect(route.length > 90 && route.length < 115)
        #expect(route.stations.allSatisfy { !$0.isMissing })
        #expect(route.stop == .endOfRoute)
        // Smooth: no gap over 1.6 m, no height step over 0.35 m between stations.
        for (a, b) in zip(route.stations, route.stations.dropFirst()) {
            #expect(
                simd_length(SIMD2(b.position.x - a.position.x, b.position.y - a.position.y)) < 1.6)
            #expect(abs(b.position.z - a.position.z) < 0.35)
        }
        // On the road everywhere: inside the outer ring of its own patch.
        let off = route.stations.filter { !onRoad($0, patches) }
        #expect(
            off.isEmpty,
            "\(off.count) stations off the road, first \(String(describing: off.first))")
        // Strictly increasing distance.
        #expect(zip(route.stations, route.stations.dropFirst()).allSatisfy { $0.s < $1.s })
    }

    @Test func routeKeepsRightOfTheIslandOnTheIslandArm() throws {
        let (patches, index) = try fixture()
        let route = RoadRouteBuilder.route(from: islandID, patches: patches, index: index)
        #expect(route.stations.count > 30)
        #expect(route.stations.allSatisfy { onRoad($0, patches) })
    }

    @Test func missingGroundMarksStationsAndStopsTheRoute() throws {
        let (patches, index) = try fixture()
        let route = RoadRouteBuilder.route(from: straightID, patches: patches, index: index)
        #expect(route.patchIDs == [straightID, unsupportedID])
        let missing = route.stations.filter(\.isMissing)
        #expect(!missing.isEmpty)
        // A missing station has NaN height and no support class of OBSERVED.
        #expect(missing.allSatisfy { $0.position.z.isNaN && $0.support == .missing })
        // The first missing station is the one the driver must stop before.
        #expect(route.firstMissing(from: 0, within: 1000)?.s == missing.first?.s)
    }

    @Test func aMissingJunctionPatchIsUnloadedGeometryNotAJump() throws {
        let (all, _) = try fixture()
        let without = all.filter { $0.patchID != junctionID }
        let route = RoadRouteBuilder.route(
            from: straightID, reversed: true, patches: without,
            index: RoadHeightIndex(patches: without))
        #expect(route.patchIDs == [straightID])
        guard case .unloadedGeometry(let detail)? = route.stop else {
            Issue.record("stop \(String(describing: route.stop))")
            return
        }
        #expect(detail.contains("junction"))
    }

    @Test func choiceAtTheJunctionTakesTheNamedArm() throws {
        let (patches, index) = try fixture()
        // From the bend, backwards, into the junction: straightest is the straight
        // arm, but the island arm can be chosen.
        let straightest = RoadRouteBuilder.route(
            from: bendID, reversed: true, patches: patches, index: index)
        #expect(straightest.patchIDs.contains(straightID))
        let chosen = RoadRouteBuilder.route(
            from: bendID, reversed: true, patches: patches, index: index,
            choices: [junctionID: islandID])
        #expect(chosen.patchIDs == [bendID, junctionID, islandID])
        #expect(chosen.stations.allSatisfy { !$0.isMissing })
    }

    @Test func twoStackedDecksStopTheRouteForAChoice() {
        // Two roads continue straight on from x = 10, one at z 0 and one at z 5.
        func deck(
            _ id: String, x0: Float, x1: Float, z: Float, from: String, to: String
        ) -> RoadPatch {
            let n = Int(x1 - x0) + 1
            var vertices: [SIMD3<Float>] = []
            var indices: [UInt32] = []
            var profile: [RoadProfileStation] = []
            for i in 0..<n {
                let x = x0 + Float(i)
                vertices += [SIMD3(x, -3, z), SIMD3(x, 3, z)]
                profile.append(
                    RoadProfileStation(
                        s: Float(i), position: SIMD3(x, 0, z), grade: 0, support: .observed))
                if i > 0 {
                    let b = UInt32(2 * (i - 1))
                    indices += [b, b + 2, b + 1, b + 1, b + 2, b + 3]
                }
            }
            return RoadPatch(
                patchID: id, kind: .segment, fromNode: from, toNode: to, vertices: vertices,
                normals: vertices.map { _ in SIMD3(0, 0, 1) }, indices: indices,
                support: vertices.map { _ in .observed }, profile: profile)
        }
        let patches = [
            deck("a", x0: 0, x1: 10, z: 0, from: "1", to: "2"),
            deck("low", x0: 10, x1: 20, z: 0, from: "2", to: "3"),
            deck("high", x0: 10, x1: 20, z: 5, from: "2", to: "4"),
        ]
        let route = RoadRouteBuilder.route(
            from: "a", patches: patches, index: RoadHeightIndex(patches: patches))
        guard case .unresolvedChoice? = route.stop else {
            Issue.record("stop \(String(describing: route.stop))")
            return
        }
        #expect(route.patchIDs == ["a"])
    }

    @Test func heightIndexAnswersNilInGapsAndHoles() throws {
        let (patches, index) = try fixture()
        let island = try #require(patches.first { $0.patchID == islandID })
        let hole = try #require(island.rings.first { $0.hole })
        let c = hole.points.reduce(SIMD3<Float>.zero, +) / Float(hole.points.count)
        #expect(index.height(at: SIMD2(c.x, c.y)) == nil)
        #expect(index.height(at: SIMD2(1000, 1000)) == nil)
        // On the straight it matches the patch's own interpolation.
        let straight = try #require(patches.first { $0.patchID == straightID })
        let st = straight.profile[10].position
        let z = try #require(index.height(at: SIMD2(st.x, st.y)))
        #expect(abs(z - st.z) < 0.02)
    }
}

struct RoadDriverTests {
    private func routeThroughJunction() throws -> RoadRoute {
        let patches = try RoadFixture.store(site: "80858771f").patches
        return RoadRouteBuilder.route(
            from: straightID, reversed: true, patches: patches,
            index: RoadHeightIndex(patches: patches))
    }

    private func routeIntoMissing() throws -> RoadRoute {
        let patches = try RoadFixture.store(site: "80858771f").patches
        return RoadRouteBuilder.route(
            from: straightID, patches: patches, index: RoadHeightIndex(patches: patches))
    }

    @Test func movesAtSpeedPausesAndScrubs() throws {
        var driver = RoadDriver(route: try routeThroughJunction())
        #expect(driver.stopped == nil)
        driver.setSpeed(10)
        driver.play()
        for _ in 0..<60 { driver.advance(by: 1.0 / 60) }
        #expect(abs(driver.s - 10) < 0.2)
        driver.pause()
        let at = driver.s
        driver.advance(by: 1)
        #expect(driver.s == at)
        driver.scrub(to: 50)
        #expect(driver.s == 50)
        driver.scrub(to: -5)
        #expect(driver.s == 0)
        driver.scrub(to: 1e6)
        #expect(driver.s == driver.length)
        #expect(driver.stopped == .endOfRoute)
        driver.setSpeed(500)
        #expect(driver.speed == RoadDriver.speedRange.upperBound)
    }

    @Test func eyeIsAboveTheSurfaceByTheEyeHeight() throws {
        var driver = RoadDriver(route: try routeThroughJunction())
        driver.scrub(to: 20)
        let surface = try #require(driver.point(at: driver.s))
        let pose = try #require(driver.pose())
        #expect(abs(pose.position.z - (surface.z + 1.5)) < 1e-4)
        driver.setEyeHeight(2.2)
        let higher = try #require(driver.pose())
        #expect(abs(higher.position.z - (surface.z + 2.2)) < 1e-4)
        driver.setEyeHeight(10)
        #expect(driver.eyeHeight == RoadDriver.eyeRange.upperBound)
        // Looking forward along the road.
        #expect(simd_length(pose.target - pose.position) > 5)
    }

    @Test func throughTheBendAndJunctionThePoseNeverJumps() throws {
        var driver = RoadDriver(route: try routeThroughJunction())
        driver.setSpeed(12)
        driver.play()
        var last = try #require(driver.pose())
        var maxStep: Float = 0
        var steps = 0
        while driver.playing && steps < 5000 {
            driver.advance(by: 1.0 / 60)
            steps += 1
            guard let now = driver.pose() else { break }
            maxStep = max(maxStep, simd_length(now.position - last.position))
            last = now
        }
        // At 12 m/s and 60 Hz a step is 0.2 m; a jump would be far more.
        #expect(maxStep < 0.3, "largest step \(maxStep) m")
        #expect(driver.stopped == .endOfRoute)
        #expect(driver.s > driver.length - 0.5)
    }

    @Test func stopsBeforeMissingHeightAndSaysSo() throws {
        let route = try routeIntoMissing()
        let firstMissing = try #require(route.stations.first { $0.isMissing })
        var driver = RoadDriver(route: route)
        driver.setSpeed(20)
        driver.play()
        var steps = 0
        while driver.playing && steps < 5000 {
            driver.advance(by: 1.0 / 60)
            steps += 1
        }
        guard case .missingHeight(let at, let detail)? = driver.stopped else {
            Issue.record("stopped \(String(describing: driver.stopped))")
            return
        }
        #expect(abs(at - firstMissing.s) < 1e-3)
        #expect(detail.contains("missing ground"))
        #expect(driver.s < firstMissing.s)
        #expect(driver.stopped?.message.contains("no road height") == true)
        // It cannot be started again past the gap, and its pose is still on the road.
        driver.play()
        #expect(!driver.playing)
        let pose = try #require(driver.pose())
        #expect(pose.position.z.isFinite)
        // No height was carried over the gap: no station past the stop has one.
        let after = route.stations.filter { $0.s >= firstMissing.s }
        #expect(after.allSatisfy { $0.isMissing })
    }

    @Test func scrubbingIntoTheGapClampsBeforeIt() throws {
        let route = try routeIntoMissing()
        let firstMissing = try #require(route.stations.first { $0.isMissing })
        var driver = RoadDriver(route: route)
        driver.scrub(to: route.length)
        #expect(driver.s < firstMissing.s)
        guard case .missingHeight? = driver.stopped else {
            Issue.record("stopped \(String(describing: driver.stopped))")
            return
        }
        #expect(driver.pose() != nil)
    }

    @Test func anEmptyRouteStopsWithItsReason() {
        let driver = RoadDriver(
            route: RoadRoute(
                stations: [], stop: .unloadedGeometry(detail: "no profile"), patchIDs: []))
        #expect(driver.stopped == .unloadedGeometry(detail: "no profile"))
        #expect(driver.pose() == nil)
        var d = driver
        d.play()
        #expect(!d.playing)
    }

    @Test func gradeReadsFromTheSurface() throws {
        var driver = RoadDriver(route: try routeThroughJunction())
        driver.scrub(to: 10)
        let g = try #require(driver.grade())
        // The fixture's ground falls about 2 % along the straight.
        #expect(abs(g) < 0.1)
    }
}

@MainActor struct RoadDriveSessionTests {
    private func setup() async throws -> (RoadDriveSession, RoadReviewSession, FakeCameraSink) {
        let service = FakeAlignmentService()
        service.roadSurfaceVersion = 1
        service.listedSites = [.fixture(RoadFixture.siteA), .fixture(RoadFixture.siteB)]
        service.roadSurfaceHandler = { request, onChunk in
            for (chunk, last) in try RoadFixture.chunks(site: request.l16Token) {
                await onChunk(chunk, last)
            }
        }
        let alignment = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await alignment.probe()
        let road = RoadReviewSession(alignment: alignment)
        await road.refreshSites()
        // Load straight into the loader: the pane's site list is not under test.
        road.loader.open(
            corridor: "fixture", sites: [RoadFixture.siteA], client: service,
            serverRoadSurfaceVersion: 1)
        for _ in 0..<2000 where road.loader.state.isLoading {
            try await Task.sleep(for: .milliseconds(5))
        }
        let drive = RoadDriveSession(review: road)
        let sink = FakeCameraSink()
        drive.sink = sink
        return (drive, road, sink)
    }

    @Test func drivesThroughTheJunctionHoldingTheCameraAtEyeHeight() async throws {
        let (drive, _, sink) = try await setup()
        drive.begin(on: straightID, reversed: true)
        #expect(drive.isDriving)
        #expect(sink.holder == .drive)
        let start = sink.camera.position
        drive.speed = 14
        drive.play()
        #expect(drive.driver?.playing == true)
        var last = sink.camera.position
        var maxStep: Float = 0
        for _ in 0..<2000 where drive.driver?.playing == true {
            drive.advance(by: 1.0 / 60)
            maxStep = max(maxStep, simd_length(sink.camera.position - last))
            last = sink.camera.position
        }
        #expect(drive.stopReason == .endOfRoute)
        #expect(simd_length(last - start) > 60)
        #expect(maxStep < 0.35, "largest camera step \(maxStep) m")
        #expect(sink.camera.projection == .perspective)
        #expect(drive.frameIntervals.count > 100)
    }

    @Test func stopsAtMissingHeightAndShowsTheReason() async throws {
        let (drive, _, sink) = try await setup()
        drive.begin(on: straightID)
        drive.speed = 20
        drive.play()
        for _ in 0..<2000 where drive.driver?.playing == true { drive.advance(by: 1.0 / 60) }
        guard case .missingHeight? = drive.stopReason else {
            Issue.record("stop \(String(describing: drive.stopReason))")
            return
        }
        #expect(drive.message?.contains("no road height") == true)
        #expect(sink.camera.position.z.isFinite)
        // Play does not resume past the gap.
        drive.play()
        #expect(drive.driver?.playing == false)
        // The camera stayed on the supported road, short of the gap.
        #expect(drive.distanceAlong < 45)
    }

    @Test func scrubMovesTheCameraAndPauseHolds() async throws {
        let (drive, _, sink) = try await setup()
        drive.begin(on: straightID, reversed: true)
        drive.scrub(to: 30)
        let at = sink.camera.position
        drive.advance(by: 1)
        #expect(sink.camera.position == at)
        drive.scrub(to: 60)
        #expect(sink.camera.position != at)
        drive.eyeHeight = 2.5
        let surface = try #require(drive.driver?.point(at: 60))
        #expect(abs(sink.camera.position.z - (surface.z + 2.5)) < 1e-3)
    }

    @Test func duplicateChunksDoNotRebuildTheDrivingHeightIndex() async throws {
        let (drive, road, _) = try await setup()
        drive.begin(on: straightID, reversed: true)
        let built = drive.indexBuilds
        let service = FakeAlignmentService()
        service.roadSurfaceHandler = { request, onChunk in
            for (chunk, last) in try RoadFixture.chunks(site: request.l16Token) {
                await onChunk(chunk, last)
            }
        }
        road.loader.extend(
            around: .zero, radius: 150, sites: [RoadFixture.siteA], client: service,
            serverRoadSurfaceVersion: 1)
        #expect(await eventually { road.loader.store.duplicates >= 5 })
        try await Task.sleep(for: .milliseconds(50))
        #expect(drive.indexBuilds == built)
    }

    @Test func aHeldCameraIsReportedNotStolen() async throws {
        let (drive, _, sink) = try await setup()
        sink.holdCamera(.orbit, sink.camera)
        drive.begin(on: straightID, reversed: true)
        #expect(sink.holder == .orbit)
        #expect(drive.message?.contains("held elsewhere") == true)
        #expect(drive.driver?.playing != true)
    }

    @Test func roadPickingUsesTheReviewFrameWithAnAlignedCaptureOpen() throws {
        let view = MTKView(frame: CGRect(x: 0, y: 0, width: 200, height: 200))
        let renderer = try #require(MetalRenderer(metalView: view))
        let point = CGPoint(x: 100, y: 100)
        let expected = try #require(
            CameraRay.ray(camera: renderer.camera, viewPoint: point, viewSize: view.bounds.size))
        var alignment = matrix_identity_float4x4
        alignment.columns.3 = SIMD4(100, 200, 30, 1)
        renderer.setReferenceModelMatrix(alignment)
        let actual = try #require(renderer.rayInSiteFrame(at: point, viewSize: view.bounds.size))
        #expect(simd_length(actual.origin - expected.origin) < 1e-5)
        #expect(simd_length(actual.direction - expected.direction) < 1e-5)
    }

    @Test func overheadGivesTheCameraBack() async throws {
        let (drive, _, sink) = try await setup()
        drive.begin(on: straightID, reversed: true)
        #expect(sink.holder == .drive)
        drive.overhead()
        #expect(sink.holder == nil)
        #expect(!drive.isDriving)
        #expect(drive.driver?.playing != true)
    }

    @Test func closingOrReplacingTheCorridorStopsDrivingAndReleasesTheCamera() async throws {
        for close in [true, false] {
            let (drive, road, sink) = try await setup()
            drive.begin(on: straightID, reversed: true)
            drive.play()
            #expect(sink.holder == .drive)
            if close { road.close() } else { road.open(siteToken: "808587721") }
            #expect(!drive.isDriving)
            #expect(drive.driver == nil)
            #expect(sink.holder == nil)
            #expect(drive.traverseUntil == nil)
        }
    }

    @Test func choosingAnArmRebuildsTheRoute() async throws {
        let (drive, _, _) = try await setup()
        drive.begin(on: bendID, reversed: true)
        #expect(drive.driver?.route.patchIDs.contains(straightID) == true)
        drive.choose(arm: islandID, at: junctionID)
        #expect(drive.driver?.route.patchIDs == [bendID, junctionID, islandID])
        #expect(drive.driver?.stopped == nil)
    }

    @Test func aRouteThatEndsMidLoadStopsAsUnloadedGeometry() async throws {
        let (drive, road, _) = try await setup()
        // A partial load: the loader says so, and the end of the route is
        // then lack of geometry, not the end of the road.
        let gate = AsyncGate()
        let service = FakeAlignmentService()
        service.roadSurfaceVersion = 1
        service.roadSurfaceHandler = { request, onChunk in
            let chunks = try RoadFixture.chunks(site: request.l16Token)
            await onChunk(chunks[0].0, false)
            await gate.wait()
        }
        road.loader.open(
            corridor: "fixture", sites: [RoadFixture.siteA], client: service,
            serverRoadSurfaceVersion: 1)
        for _ in 0..<2000 where road.patches.count < 2 {
            try await Task.sleep(for: .milliseconds(5))
        }
        drive.begin(on: straightID)
        drive.scrub(to: 1e6)
        guard case .unloadedGeometry? = drive.stopReason else {
            Issue.record("stop \(String(describing: drive.stopReason))")
            await gate.open()
            return
        }
        #expect(drive.message?.contains("not loaded") == true)
        road.loader.cancel()
        await gate.open()
    }
}

struct RoadMeshAuditTests {
    @Test func theFixtureIsWellFormedAndItsSeamsAreWithinFiveMillimetres() throws {
        let store = try RoadFixture.store(site: "80858771f")
        let audit = RoadMeshAudit.audit(store.patches)
        #expect(audit.patches.count == 5)
        #expect(audit.invertedTotal == 0)
        #expect(audit.degenerateTotal == 0)
        #expect(audit.overlapTotal == 0, "\(audit.patches.filter { !$0.isClean })")
        #expect(audit.seamPairs > 0)
        #expect(audit.seamStepMaxM <= 0.005, "worst seam step \(audit.seamStepMaxM) m")
        #expect(audit.isClean)
    }

    private func quad(
        _ id: String, x0: Float, x1: Float, z: Float = 0, flip: Bool = false
    ) -> RoadPatch {
        let v = [SIMD3<Float>(x0, 0, z), SIMD3(x1, 0, z), SIMD3(x1, 4, z), SIMD3(x0, 4, z)]
        let idx: [UInt32] = flip ? [0, 2, 1, 0, 3, 2] : [0, 1, 2, 0, 2, 3]
        return RoadPatch(
            patchID: id, vertices: v, normals: v.map { _ in SIMD3(0, 0, 1) }, indices: idx,
            support: v.map { _ in .observed })
    }

    @Test func aFlippedTriangleIsCountedInverted() {
        let audit = RoadMeshAudit.audit([quad("a", x0: 0, x1: 4, flip: true)])
        #expect(audit.invertedTotal == 2)
        #expect(!audit.isClean)
    }

    @Test func overlappingPatchesAreCounted() {
        let audit = RoadMeshAudit.audit([quad("a", x0: 0, x1: 4), quad("b", x0: 1, x1: 5)])
        #expect(audit.patches.reduce(0) { $0 + $1.overlapsAcross } > 0)
        #expect(!audit.isClean)
    }

    @Test func crossingTrianglesOverlapEvenWhenNeitherCentroidIsInsideTheOther() {
        let a = [SIMD3<Float>(0, 0, 0), SIMD3(10, 0, 0), SIMD3(0, 1, 0)]
        let b = [SIMD3<Float>(8, -4, 0), SIMD3(9, 6, 0), SIMD3(8, 6, 0)]
        func patch(_ id: String, _ v: [SIMD3<Float>], _ indices: [UInt32]) -> RoadPatch {
            RoadPatch(
                patchID: id, vertices: v, normals: v.map { _ in SIMD3(0, 0, 1) }, indices: indices,
                support: v.map { _ in .observed })
        }
        let across = RoadMeshAudit.audit([patch("a", a, [0, 1, 2]), patch("b", b, [0, 1, 2])])
        #expect(across.overlapTotal == 2)
        let within = RoadMeshAudit.audit([patch("both", a + b, [0, 1, 2, 3, 4, 5])])
        #expect(within.patches.first?.overlapsWithin == 2)
    }

    @Test func aHeightStepAtASharedSeamIsReported() {
        // Two quads that touch along x = 4, the second 2 cm higher.
        let audit = RoadMeshAudit.audit([quad("a", x0: 0, x1: 4), quad("b", x0: 4, x1: 8, z: 0.02)])
        #expect(audit.seamPairs == 2)
        #expect(abs(audit.seamStepMaxM - 0.02) < 1e-4)
        #expect(audit.seamViolations.count == 2)
        #expect(audit.overlapTotal == 0)
        // Flush, they are clean.
        #expect(RoadMeshAudit.audit([quad("a", x0: 0, x1: 4), quad("b", x0: 4, x1: 8)]).isClean)
    }

    @Test func evictedPatchesAreLeftOut() throws {
        var store = try RoadFixture.store(site: "80858771f")
        store.evictMeshes(beyond: 0, from: SIMD2(-1000, 0))
        #expect(RoadMeshAudit.audit(store.patches).patches.isEmpty)
    }
}
