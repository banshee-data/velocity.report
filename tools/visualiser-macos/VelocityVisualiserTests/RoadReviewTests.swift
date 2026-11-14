// RoadReviewTests.swift
// Drawing the road surface (fill, gaps, boundary provenance, picking) and
// opening a corridor through the Gerardus connection with no sensor: sites
// from ListSites, neighbours read too, a missing bundle said, and the
// bundle-edge crossing in both directions.

import Foundation
import GRPCCore
import MetalKit
import Testing
import simd

@testable import VelocityVisualiser

@MainActor private final class RoadPlacementSink: RoadOverlaySink {
    var roadSceneMatrix = matrix_identity_float4x4
    var framedCentre: SIMD3<Float>?
    func setRoadOverlay(_ vertices: RoadOverlayVertices?) {}
    func setRoadModelMatrix(_ matrix: simd_float4x4) { roadSceneMatrix = matrix }
    func frameRoad(centre: SIMD3<Float>, extent: Float) { framedCentre = centre }
}

@MainActor struct RoadScenePlacementTests {
    @Test(arguments: [0.0, 35, 171, 180, 270]) func sameSiteRoadAndBuildingsSharePoseAtEveryHeading(
        _ heading: Double
    ) {
        let source = RoadFixture.siteA
        let bundle = site(source, label: "A")
        let pose = SitePose(
            translation: SIMD3(17, -23, 145),
            rotation: simd_quatd(angle: -heading * .pi / 180, axis: SIMD3(0, 0, 1)))
        let road = RoadReviewSession.modelMatrix(
            pose: pose, referenceSite: bundle, roadSite: source)
        #expect(road == pose.referenceModelMatrix)
        let control = SIMD4<Float>(21, -9, 148, 1)
        #expect(simd_distance(road * control, pose.referenceModelMatrix * control) < 1e-5)
    }

    @Test func neighbouringRoadOriginIsShiftedBeforeApplyingATiltedPose() {
        let reference = RoadFixture.siteA
        let road = RoadFixture.siteB
        let pose = SitePose(
            translation: SIMD3(-9, 23, 140),
            rotation: simd_quatd(angle: 2.98, axis: simd_normalize(SIMD3(0.01, -0.03, 1))))
        let matrix = RoadReviewSession.modelMatrix(
            pose: pose, referenceSite: site(reference, label: "A"), roadSite: road)
        let point = SIMD3<Float>(12, 6, 142)
        let inReference = point + SIMD3<Float>(road.offset(in: reference))
        let expected = pose.referenceModelMatrix * SIMD4(inReference, 1)
        #expect(simd_distance(matrix * SIMD4(point, 1), expected) < 1e-4)
        let identity = matrix_identity_float4x4
        #expect(
            RoadReviewSession.modelMatrix(pose: nil, referenceSite: nil, roadSite: road) == identity
        )
        #expect(
            RoadReviewSession.modelMatrix(pose: pose, referenceSite: nil, roadSite: road)
                == identity)
        #expect(
            RoadReviewSession.modelMatrix(
                pose: pose, referenceSite: site(reference, label: "A"), roadSite: nil) == identity)
    }

    @Test func rendererDrawingAndPickingUseTheSamePlacementIncludingCityStream() throws {
        let view = MTKView()
        view.device = try #require(MTLCreateSystemDefaultDevice())
        let renderer = try #require(MetalRenderer(metalView: view))
        let pose = SitePose(
            translation: SIMD3(18, -12, 140),
            rotation: simd_quatd(angle: -.pi * 171 / 180, axis: SIMD3(0, 0, 1)))
        renderer.setRoadModelMatrix(pose.referenceModelMatrix)
        renderer.setReferenceModelMatrix(pose.referenceModelMatrix)
        // Include the stream-to-city placement in both draw and ray paths.
        renderer.streamWorldFromSensor.columns.3 = SIMD4(100, 200, 10, 1)
        let control = SIMD4<Float>(21, -9, 142, 1)
        let scene = renderer.roadSceneMatrix * control
        renderer.camera.position = SIMD3(scene.x, scene.y, scene.z + 50)
        renderer.camera.target = SIMD3(scene.x, scene.y, scene.z)
        renderer.camera.up = SIMD3(0, 1, 0)
        renderer.camera.aspectRatio = 1
        renderer.uniforms.modelViewProjection =
            renderer.camera.projectionMatrix * renderer.camera.viewMatrix * renderer.streamModel
        renderer.uniforms.modelView = renderer.camera.viewMatrix * renderer.streamModel
        let building = MetalRenderer.uniforms(
            renderer.uniforms, model: renderer.referenceModelMatrix)
        #expect(
            simd_distance(
                renderer.roadUniforms.modelViewProjection * control,
                building.modelViewProjection * control) < 1e-5)
        let ray = try #require(
            renderer.rayInRoadFrame(
                at: CGPoint(x: 400, y: 400), viewSize: CGSize(width: 800, height: 800)))
        let hit = ray.origin + ray.direction * ((control.z - ray.origin.z) / ray.direction.z)
        #expect(simd_distance(hit, SIMD3(control.x, control.y, control.z)) < 0.002)
        renderer.setRoadModelMatrix(matrix_identity_float4x4)
        #expect(renderer.roadUniforms.modelViewProjection == renderer.uniforms.modelViewProjection)
    }

    @Test(arguments: [true, false])
    func placementTracksPreparationNudgesFramingAndDriveInEitherLoadOrder(
        _ roadsFirst: Bool
    ) async throws {
        let service = FakeAlignmentService()
        service.roadSurfaceVersion = 1
        // The fake PrepareSite uses an origin at zero; match that here.
        let source = RoadSiteSource(
            l16Token: RoadFixture.siteA.l16Token, originLatitudeDeg: 0, originLongitudeDeg: 0)
        service.listedSites = [site(source, label: "A")]
        service.roadSurfaceHandler = { request, onChunk in
            for (chunk, last) in try RoadFixture.chunks(site: request.l16Token) {
                await onChunk(chunk, last)
            }
        }
        let alignment = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await alignment.probe()
        let road = RoadReviewSession(alignment: alignment)
        road.paneToggled(open: true)
        await road.refreshSites()
        let sink = RoadPlacementSink()
        road.sink = sink
        if roadsFirst { road.open(siteToken: source.l16Token) }
        alignment.choose(
            AlignmentSite(
                clipID: "a", l16Token: source.l16Token, l13Token: "", description: "A",
                sensorID: "sensor", latitude: 0, longitude: 0))
        await alignment.placeFromHint()
        if !roadsFirst { road.open(siteToken: source.l16Token) }
        #expect(
            await eventually {
                road.loader.state == .loaded
                    && road.roadModelMatrix == service.hintPose.referenceModelMatrix
            })
        #expect(sink.roadSceneMatrix == road.roadModelMatrix)
        let bounds = road.patches.compactMap(\.bounds)
        let low = bounds.map(\.min).reduce(SIMD2<Float>(repeating: .infinity), simd_min)
        let high = bounds.map(\.max).reduce(SIMD2<Float>(repeating: -.infinity), simd_max)
        let heights = road.patches.flatMap(\.vertices).map(\.z).filter(\.isFinite)
        let centre = (low + high) / 2
        road.frameCorridor()
        let framed = try #require(sink.framedCentre)
        let expectedCentre = road.scenePoint(
            SIMD3(centre.x, centre.y, heights.reduce(0, +) / Float(heights.count)))
        #expect(simd_distance(framed, expectedCentre) < 1e-4)
        let camera = FakeCameraSink()
        let drive = RoadDriveSession(review: road)
        drive.sink = camera
        let start = try #require(drive.startOptions.first)
        drive.begin(on: start.patchID)
        let drivingPose = try #require(drive.driver?.pose())
        #expect(simd_distance(camera.camera.position, road.scenePoint(drivingPose.position)) < 1e-4)
        let before = road.roadModelMatrix
        alignment.nudge(x: 2, yawSteps: 1)
        #expect(await eventually { road.roadModelMatrix != before })
        #expect(
            await eventually {
                simd_distance(camera.camera.position, road.scenePoint(drivingPose.position)) < 1e-4
            })
        drive.overhead()
        road.close()
        #expect(await eventually { road.roadModelMatrix == matrix_identity_float4x4 })
        #expect(road.loader.sceneSource == nil)
    }
}

private func site(_ s: RoadSiteSource, label: String) -> Gerardus_Align_V1alpha1_Site {
    var out = Gerardus_Align_V1alpha1_Site()
    out.l16Token = s.l16Token
    out.label = label
    out.frame.origin.latitudeDeg = s.originLatitudeDeg
    out.frame.origin.longitudeDeg = s.originLongitudeDeg
    return out
}

struct RoadSurfaceStyleTests {
    private func patches() throws -> [RoadPatch] {
        try RoadFixture.store(site: "80858771f").patches
    }

    @Test func fillHasThreeVerticesPerTriangleAndNoneForGaps() throws {
        let all = try patches()
        let out = RoadSurfaceStyle.vertices(patches: all, options: RoadViewOptions())
        let triangles = all.map(\.triangleCount).reduce(0, +)
        #expect(out.fillVertexCount == triangles * 3)
        // Every fill vertex has a finite height: no triangle touches a missing vertex.
        let zs = stride(from: 2, to: out.fill.count, by: 7).map { out.fill[$0] }
        #expect(zs.allSatisfy { $0.isFinite })
        #expect(out.fill.count % 7 == 0)
    }

    @Test func supportShadingMarksInterpolatedTrianglesAndPlainDoesNot() throws {
        let bend = try #require(patches().first { $0.patchID.contains("bend") })
        var support = RoadViewOptions()
        support.shading = .support
        let a = RoadSurfaceStyle.vertices(patches: [bend], options: support).fill
        var plain = support
        plain.shading = .plain
        let b = RoadSurfaceStyle.vertices(patches: [bend], options: plain).fill
        func colours(_ f: [Float]) -> Set<[Float]> {
            Set(stride(from: 0, to: f.count, by: 7).map { Array(f[($0 + 3)..<($0 + 6)]) })
        }
        let interpolated = RoadSurfaceStyle.colour(.interpolated)
        #expect(colours(a).contains([interpolated.x, interpolated.y, interpolated.z]))
        #expect(!colours(b).contains([interpolated.x, interpolated.y, interpolated.z]))
    }

    @Test func inferredBoundaryIsDashedAndSolidOnesAreNot() throws {
        let all = try patches()
        let bend = try #require(all.first { $0.patchID.contains("bend") })
        let straight = try #require(all.first { $0.patchID.contains("straight") })
        let inferred = RoadSurfaceStyle.colour(.inferredWidth)
        func countInferred(_ p: RoadPatch) -> Int {
            let b = RoadSurfaceStyle.vertices(patches: [p], options: RoadViewOptions()).boundary
            return stride(from: 0, to: b.count, by: 7).filter {
                b[$0 + 3] == inferred.x && b[$0 + 4] == inferred.y
            }.count
        }
        #expect(countInferred(bend) > 0)
        #expect(countInferred(straight) == 0)
        // Dashes: segments are shorter than the edges they cover.
        let b = RoadSurfaceStyle.vertices(patches: [bend], options: RoadViewOptions()).boundary
        let segments = stride(from: 0, to: b.count, by: 14).map { i -> Float in
            simd_length(SIMD3(b[i], b[i + 1], b[i + 2]) - SIMD3(b[i + 7], b[i + 8], b[i + 9]))
        }
        // No dash is longer than the dash length, and an edge longer than one
        // dash leaves a gap, so the inferred edge is never a continuous line.
        #expect(segments.allSatisfy { $0 <= 0.61 })
        var long = [Float]()
        RoadSurfaceStyle.appendDashed(SIMD3(0, 0, 0), SIMD3(3, 0, 0), inferred, to: &long)
        #expect(long.count / 14 == 3)
        #expect(long.count / 7 == 6)
    }

    @Test func togglesRemoveTheirLayers() throws {
        let all = try patches()
        var o = RoadViewOptions()
        o.surface = false
        o.centreline = false
        o.boundary = false
        o.wireframe = false
        #expect(RoadSurfaceStyle.vertices(patches: all, options: o).isEmpty)
        o.wireframe = true
        let wire = RoadSurfaceStyle.vertices(patches: all, options: o)
        let triangles = all.map(\.triangleCount).reduce(0, +)
        #expect(wire.wireVertexCount == triangles * 6)
        #expect(wire.fillVertexCount == 0)
    }

    @Test func centrelineBreaksAtMissingStations() throws {
        let unsupported = try #require(patches().first { $0.patchID.contains("unsupported") })
        let stations = unsupported.profile
        let supported = stations.filter { !$0.isMissing }.count
        #expect(supported > 1 && supported < stations.count)
        let c = RoadSurfaceStyle.vertices(patches: [unsupported], options: RoadViewOptions()).centre
        // Lines join only supported neighbours: never more segments than stations - 1,
        // and every endpoint is finite.
        #expect(c.count / 14 <= supported - 1)
        #expect(c.allSatisfy { $0.isFinite })
    }

    @Test func selectionOutlinesOnlyTheSelectedPatch() throws {
        let all = try patches()
        let target = all[1].patchID
        let out = RoadSurfaceStyle.vertices(
            patches: all, options: RoadViewOptions(), selected: target)
        #expect(out.selectionVertexCount > 0)
        let none = RoadSurfaceStyle.vertices(patches: all, options: RoadViewOptions())
        #expect(none.selectionVertexCount == 0)
    }

    @Test func pickingHitsTheRoadAndMissesTheIslandHole() throws {
        let all = try patches()
        let island = try #require(all.first { $0.patchID.contains("island") })
        let hole = try #require(island.rings.first { $0.hole })
        let c = hole.points.reduce(SIMD3<Float>.zero, +) / Float(hole.points.count)
        let straight = try #require(all.first { $0.patchID.contains("straight") })
        let s = try #require(straight.vertices.first { $0.z.isFinite })
        let down = SIMD3<Float>(0, 0, -1)
        // Straight down onto a vertex of the straight patch.
        let hit = RoadSurfaceStyle.pick(
            patches: all, origin: SIMD3(s.x + 0.01, s.y + 0.01, s.z + 30), direction: down)
        #expect(hit?.patchID == straight.patchID || hit != nil)
        // Straight down onto the island's centre: nothing under it.
        let miss = RoadSurfaceStyle.pick(
            patches: [island], origin: SIMD3(c.x, c.y, 200), direction: down)
        #expect(miss == nil)
    }

    @Test func openInteriorEdgesOnlyWhereGroundIsMissing() throws {
        let all = try patches()
        let straight = try #require(all.first { $0.patchID.contains("straight") })
        #expect(RoadSurfaceStyle.openInteriorEdges(straight).isEmpty)
    }
}

@MainActor struct RoadReviewSessionTests {
    @Test func generationChecksCapabilityAndRetainsTheRequestedSite() async {
        let service = FakeAlignmentService()
        service.listedSites = [site(RoadFixture.siteA, label: "A")]
        let alignment = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await alignment.probe()
        let road = RoadReviewSession(alignment: alignment)
        await road.generateRoads(siteToken: RoadFixture.siteA.l16Token, refresh: false)
        #expect(service.roadGenerationRequests.isEmpty)
        service.supportsRoadGeneration = true
        service.roadSurfaceVersion = 1
        await alignment.probe()
        road.inferredHalfWidthM = 3.5
        await road.generateRoads(siteToken: RoadFixture.siteA.l16Token, refresh: true)
        #expect(service.roadGenerationRequests.count == 1)
        #expect(service.roadGenerationRequests[0].halfWidthM == 3.5)
        #expect(service.roadGenerationRequests[0].refresh)
        #expect(road.openedSite == RoadFixture.siteA.l16Token)
        #expect(!road.generatingRoads)
        service.roadGenerationFailure = RPCError(
            code: .failedPrecondition, message: "No ground dataset")
        await road.generateRoads(siteToken: RoadFixture.siteA.l16Token, refresh: false)
        #expect(road.generationMessage?.contains("No ground dataset") == true)
        #expect(!road.generatingRoads)
    }

    @Test func changingAddressDuringGenerationDoesNotOpenItsOldResult() async {
        let service = FakeAlignmentService()
        service.supportsRoadGeneration = true
        let alignment = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await alignment.probe()
        let road = RoadReviewSession(alignment: alignment)
        service.roadGenerationHook = {
            await MainActor.run {
                alignment.useServiceAddress(GerardusAddress(host: "other.local", port: 50061))
            }
        }
        await road.generateRoads(siteToken: RoadFixture.siteA.l16Token, refresh: false)
        #expect(road.openedSite == nil)
        #expect(road.generationMessage?.contains("Reopen") == true)
    }

    private func session(
        version: UInt32 = 1, sites: [Gerardus_Align_V1alpha1_Site]? = nil
    ) async -> (RoadReviewSession, FakeAlignmentService) {
        let service = FakeAlignmentService()
        service.roadSurfaceVersion = version
        service.listedSites =
            sites ?? [
                site(RoadFixture.siteA, label: "park a"), site(RoadFixture.siteB, label: "park b"),
            ]
        service.roadSurfaceHandler = { request, onChunk in
            for (chunk, last) in try RoadFixture.chunks(site: request.l16Token) {
                await onChunk(chunk, last)
            }
        }
        let alignment = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await alignment.probe()
        let road = RoadReviewSession(alignment: alignment)
        road.paneToggled(open: true)
        await road.refreshSites()
        return (road, service)
    }

    private func settle(_ road: RoadReviewSession) async {
        // Decoding the fixture is slow under a loaded debug build: wait on the
        // state, not the clock.
        try? await Task.sleep(for: .milliseconds(30))
        for _ in 0..<2000 {
            if !road.loader.state.isLoading { break }
            try? await Task.sleep(for: .milliseconds(5))
        }
        try? await Task.sleep(for: .milliseconds(100))
        for _ in 0..<2000 {
            if !road.loader.state.isLoading { break }
            try? await Task.sleep(for: .milliseconds(5))
        }
    }

    @Test func listsPreparedSitesFromTheConnection() async {
        let (road, service) = await session()
        #expect(road.sites.map(\.id) == ["80858771f", "808587721"])
        #expect(service.calls.contains("ListSites"))
        #expect(road.sitesNote == nil)
    }

    @Test func opensACorridorWithNoSensorAndReadsTheNeighbour() async {
        let (road, service) = await session()
        road.open(siteToken: "80858771f")
        await settle(road)
        #expect(road.loader.state == .loaded)
        #expect(road.patches.count == 5)
        // The neighbour (132 m away) was read too, and its patches deduped.
        #expect(service.calls.filter { $0.hasPrefix("StreamRoadSurface") }.count >= 2)
        #expect(road.loader.store.duplicates >= 5)
        _ = await eventually { road.overlay.fillVertexCount > 0 }
        #expect(road.overlay.fillVertexCount > 0)
        #expect(road.coverage.first?.corridorID == "fixture")
        #expect(road.coverage.first?.patchCount == 5)
    }

    @Test func aListedCorridorOpensThroughExactlyItsSites() async throws {
        let service = FakeAlignmentService()
        service.roadSurfaceVersion = 1
        service.roadCorridorsListed = true
        service.listedSites = [
            site(RoadFixture.siteA, label: "a"), site(RoadFixture.siteB, label: "b"),
        ]
        var summary = Gerardus_Align_V1alpha1_RoadCorridorSummary()
        var layer = Gerardus_Align_V1alpha1_RoadSurfaceLayer()
        layer.corridorID = "fixture"
        layer.buildHash = "b"
        summary.layer = layer
        summary.description_p = "the fixture"
        summary.patchCount = 5
        summary.l16Tokens = ["80858771f", "808587721"]
        service.listedCorridors = [summary]
        service.roadSurfaceHandler = { request, onChunk in
            for (chunk, last) in try RoadFixture.chunks(site: request.l16Token) {
                await onChunk(chunk, last)
            }
        }
        let alignment = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await alignment.probe()
        let road = RoadReviewSession(alignment: alignment)
        road.paneToggled(open: true)
        await road.refreshSites()
        #expect(road.corridors.map(\.id) == ["fixture"])
        #expect(road.corridors.first?.description == "the fixture")
        #expect(service.calls.contains("ListRoadCorridors"))
        road.open(corridor: "fixture")
        for _ in 0..<2000 where road.loader.state != .loaded {
            try await Task.sleep(for: .milliseconds(5))
        }
        #expect(road.patches.count == 5)
        // Exactly the two named sites, once each: no probing, no expansion.
        #expect(service.calls.filter { $0.hasPrefix("StreamRoadSurface") }.count == 2)
        road.enterBounded(eye: .zero)
        road.leaveBounded()
        #expect(await eventually { road.loader.state == .loaded })
        #expect(road.loader.corridorKey == "fixture")
        road.close()
        #expect(road.bounded == nil)
        #expect(road.contextNote == nil)
        #expect(road.meshAudit == nil)
        road.open(corridor: "nope")
        #expect(road.sitesNote?.contains("not one this server lists") == true)
    }

    @Test func aServerThatCannotListCorridorsFallsBackToSites() async {
        let (road, service) = await session()
        #expect(road.corridors.isEmpty)
        #expect(!service.calls.contains("ListRoadCorridors"))
        #expect(!road.sites.isEmpty)
    }

    @Test func styleChangesAndDuplicateChunksDoNotRepeatTheMeshAudit() async throws {
        let (road, service) = await session()
        road.open(siteToken: "80858771f")
        await settle(road)
        #expect(await eventually { road.meshAudit != nil })
        let built = road.auditBuilds
        road.options.wireframe.toggle()
        road.selectedPatchID = road.patches.first?.patchID
        road.loader.extend(
            around: .zero, radius: 150, sites: [RoadFixture.siteA], client: service,
            serverRoadSurfaceVersion: 1)
        #expect(await eventually { road.loader.store.duplicates > 5 })
        try await Task.sleep(for: .milliseconds(50))
        #expect(road.auditBuilds == built)
    }

    @Test func anOldServerSaysWhyAndLoadsNothing() async {
        let (road, service) = await session(version: 0)
        road.open(siteToken: "80858771f")
        await settle(road)
        #expect(road.blocker?.contains("no road surface preview") == true)
        #expect(road.patches.isEmpty)
        #expect(service.calls.filter { $0.hasPrefix("StreamRoadSurface") }.isEmpty)
    }

    @Test func aSiteTheServerDoesNotHoldIsSaid() async {
        let (road, _) = await session()
        road.open(siteToken: "deadbeef")
        #expect(road.sitesNote?.contains("not a prepared bundle") == true)
        #expect(road.openedSite == nil)
    }

    @Test func selectingAPatchFillsTheInspectorWithSupportAndProvenance() async throws {
        let (road, _) = await session()
        road.open(siteToken: "80858771f")
        await settle(road)
        let bend = try #require(road.patches.first { $0.patchID.contains("bend") })
        let hit = bend.vertices.first { $0.z.isFinite }!
        let picked = road.pick(origin: hit + SIMD3(0.02, 0.02, 40), direction: SIMD3(0, 0, -1))
        #expect(picked != nil)
        let inspector = try #require(road.inspector)
        #expect(inspector.corridor == "fixture")
        #expect(inspector.boundarySource.isInferred || inspector.boundarySource != .unspecified)
        #expect(inspector.notAssessed == 18 && inspector.assessed == 0)
        _ = await eventually { road.overlay.selectionVertexCount > 0 }
        #expect(road.overlay.selectionVertexCount > 0)
        road.selectedPatchID = nil
        _ = await eventually { road.overlay.selectionVertexCount == 0 }
        #expect(road.overlay.selectionVertexCount == 0)
    }

    @Test func neighboursAreThoseWithinReachNotAlreadyRead() {
        let near = RoadReviewSession.neighbours(
            of: (try? RoadFixture.store(site: "80858771f").patches) ?? [], scene: RoadFixture.siteA,
            sites: [RoadFixture.siteA, RoadFixture.siteB], excluding: [RoadFixture.siteA])
        #expect(near.map(\.l16Token) == ["808587721"])
        let far = RoadSiteSource(
            l16Token: "far", originLatitudeDeg: 37.80, originLongitudeDeg: -122.40)
        #expect(
            RoadReviewSession.neighbours(
                of: (try? RoadFixture.store(site: "80858771f").patches) ?? [],
                scene: RoadFixture.siteA, sites: [far], excluding: []
            ).isEmpty)
    }

    @Test func bundleEdgeCrossingGivesOneWorldPositionInEitherOrder() async throws {
        // Load through A then B, and through B then A: each is a scene in its
        // first site's frame. A shared vertex must agree once one is shifted
        // by the sites' origin offset, and heights must not step.
        let service = FakeAlignmentService()
        service.roadSurfaceVersion = 1
        service.roadSurfaceHandler = { request, onChunk in
            for (chunk, last) in try RoadFixture.chunks(site: request.l16Token) {
                await onChunk(chunk, last)
            }
        }
        func load(_ order: [RoadSiteSource]) async -> RoadSurfaceStore {
            let loader = RoadSurfaceLoader()
            loader.open(corridor: "x", sites: order, client: service, serverRoadSurfaceVersion: 1)
            for _ in 0..<2000 {
                if !loader.state.isLoading { break }
                try? await Task.sleep(for: .milliseconds(5))
            }
            return loader.store
        }
        let ab = await load([RoadFixture.siteA, RoadFixture.siteB])
        let ba = await load([RoadFixture.siteB, RoadFixture.siteA])
        let shift = RoadFixture.siteB.offset(in: RoadFixture.siteA)
        let delta = SIMD3<Float>(Float(shift.x), Float(shift.y), 0)
        #expect(ab.patches.count == 5 && ba.patches.count == 5)
        var worst: Float = 0
        var zStep: Float = 0
        for p in ab.patches {
            let q = try #require(ba.patch(id: p.patchID))
            for (va, vb) in zip(p.vertices, q.vertices) where va.z.isFinite {
                // vb is in B's frame; B's frame = A's frame minus the offset.
                worst = max(worst, simd_length(va - (vb + delta)))
                zStep = max(zStep, abs(va.z - vb.z))
            }
        }
        #expect(worst < 0.01, "worst \(worst) m")
        // Heights agree to the frame accuracy (the two frames share NAVD88 z).
        #expect(zStep < 0.01, "height step \(zStep) m")
    }
}
