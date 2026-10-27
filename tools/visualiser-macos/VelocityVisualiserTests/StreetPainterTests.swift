//
//  StreetPainterTests.swift
//  VelocityVisualiserTests
//
//  The street painter without a server: the geometry ported from the web
//  street editor (snapping, a row of spaces, a lane's direction, inserting a
//  vertex), the ground a click lands on, the plan view's camera and its lock,
//  the save diff, and the session against the fake Gerardus — the author it
//  needs, the problems it maps, and the street layers it streams.
//

import Foundation
import GRPCCore
import Metal
import MetalKit
import Testing
import simd

@testable import VelocityVisualiser

// MARK: - Helpers

private func plan(_ x: Double, _ y: Double) -> PlanPoint { PlanPoint(x, y) }

private func target(
    _ id: String, _ points: [PlanPoint], closed: Bool = false,
    layer: StreetGeometry.SnapTarget.Layer = .kerb
) -> StreetGeometry.SnapTarget {
    StreetGeometry.SnapTarget(id: id, layer: layer, points: points, closed: closed)
}

private func close(_ a: PlanPoint, _ b: PlanPoint, _ tol: Double = 1e-9) -> Bool {
    simd_length(a - b) < tol
}

private func mark(
    _ key: String, id: String = "", kind: StreetUseKind, _ points: [PlanPoint], z: Double = 21,
    tags: [String: String] = [:]
) -> StreetMark {
    StreetMark(key: key, featureID: id, kind: kind, points: points.map { SIMD3($0, z) }, tags: tags)
}

/// A tilted pose, so the plan view is checked through a sensor that is not
/// level.
private func tiltedPose() -> SitePose {
    SitePose(
        translation: SIMD3(-9.4, -38.5, 23.0),
        rotation: simd_quatd(angle: 0.7, axis: simd_normalize(SIMD3(0.1, -0.2, 1.0))))
}

/// Records what the painter hands the renderer.
final class FakeStreetSink: StreetPaintSink {
    var overlays: [StreetOverlayVertices?] = []
    var cameras: [Camera] = []
    var exits = 0

    func setStreetOverlay(_ vertices: StreetOverlayVertices?) { overlays.append(vertices) }
    func setPlanCamera(_ camera: Camera) { cameras.append(camera) }
    func exitPlanView() { exits += 1 }
}

private let columbus = AlignmentSite(
    clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
    description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
    sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

/// A painter over a site placed against the fake, which lists the street
/// layers: a street running east along y = 0, its kerbs at y = ±5, all at
/// 21 m.
@MainActor private func placedPainter(
    configure: (FakeAlignmentService) -> Void = { _ in }
) async -> (StreetPainterSession, AlignmentSession, FakeAlignmentService, FakeStreetSink) {
    let service = FakeAlignmentService()
    service.availableLayers += [.boxes, .cityStreets, .cityIntersections, .kerbs, .streetUse]
    configure(service)
    let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
    let painter = StreetPainterSession(alignment: session)
    let sink = FakeStreetSink()
    painter.sink = sink
    painter.viewSize = CGSize(width: 800, height: 600)
    await session.probe()
    session.choose(columbus)
    session.northAzimuthText = "35"
    await session.placeFromHint()
    await painter.pendingLoad?.value
    return (painter, session, service, sink)
}

@MainActor private func click(_ painter: StreetPainterSession, _ site: PlanPoint, count: Int = 1) {
    let p = painter.plan!.viewPoint(ofSite: site, size: painter.viewSize)
    painter.mouseDown(at: p)
    painter.mouseUp(at: p, clickCount: count)
}

@MainActor private func drag(_ painter: StreetPainterSession, from a: PlanPoint, to b: PlanPoint) {
    let p = painter.plan!.viewPoint(ofSite: a, size: painter.viewSize)
    let q = painter.plan!.viewPoint(ofSite: b, size: painter.viewSize)
    painter.mouseDown(at: p)
    painter.mouseDragged(to: CGPoint(x: (p.x + q.x) / 2, y: (p.y + q.y) / 2))
    painter.mouseDragged(to: q)
    painter.mouseUp(at: q, clickCount: 1)
}

// MARK: - Snapping

struct StreetSnapTests {
    private let kerb = target("kerb", [plan(0, 5), plan(20, 5)])
    private let other = target("mark", [plan(10, 5.3), plan(10, 9)], layer: .mark)

    @Test func snapsToAnEdgeWithinHalfAMetre() {
        let s = StreetGeometry.snap(plan(4, 5.4), to: [kerb])
        #expect(s.on == .edge)
        #expect(close(s.point, plan(4, 5)))
        #expect(abs(s.distance - 0.4) < 1e-9)
        #expect(s.target?.id == "kerb")
    }

    @Test func aVertexInReachWinsOverANearerEdge() {
        // 0.3 m from the kerb's edge, 0.42 m from the other mark's end.
        let s = StreetGeometry.snap(plan(10.3, 5.0), to: [kerb, other])
        #expect(s.on == .vertex)
        #expect(close(s.point, plan(10, 5.3)))
    }

    @Test func nothingBeyondHalfAMetre() {
        let s = StreetGeometry.snap(plan(4, 5.6), to: [kerb])
        #expect(s.on == .none)
        #expect(!s.snapped)
        #expect(close(s.point, plan(4, 5.6)))
        #expect(StreetGeometry.snapMetres == 0.5)
    }

    @Test func aMarkDoesNotSnapToItself() {
        let s = StreetGeometry.snap(plan(10, 5.6), to: [other], excluding: "mark")
        #expect(s.on == .none)
    }

    @Test func aClosedRingSnapsToItsClosingEdge() {
        let ring = target("ring", [plan(0, 0), plan(4, 0), plan(4, 4), plan(0, 4)], closed: true)
        let s = StreetGeometry.snap(plan(-0.2, 2), to: [ring])
        #expect(s.on == .edge)
        #expect(close(s.point, plan(0, 2)))
        let open = target("line", ring.points)
        #expect(StreetGeometry.snap(plan(-0.2, 2), to: [open]).on == .none)
    }

    @Test func aBoxWallSnapsAtItsFoot() {
        // A vertical rectangle: its corners fall in pairs across the ground.
        var wall = Gerardus_Align_V1alpha1_LineFeature()
        wall.kind = .building
        wall.closed = true
        wall.xyz = [10, 10, 21, 20, 10, 21, 20, 10, 33, 10, 10, 33]
        let t = StreetReference.target(wall, layer: .wall, index: 0)
        #expect(t.points == [plan(10, 10), plan(20, 10)])
        #expect(!t.closed)
        let s = StreetGeometry.snap(plan(15, 10.3), to: [t])
        #expect(close(s.point, plan(15, 10)))
    }
}

// MARK: - A row of spaces

struct StreetRowTests {
    @Test func aRowOfThreeAlongTheKerb() throws {
        let row = try #require(
            StreetGeometry.rowOfSpaces(plan(0, 5), plan(15, 5), count: 3, depth: 2.4, side: -1))
        #expect(row.ring.count == 4)
        #expect(close(row.ring[0], plan(0, 5)))
        #expect(close(row.ring[1], plan(15, 5)))
        #expect(close(row.ring[2], plan(15, 2.6)))
        #expect(close(row.ring[3], plan(0, 2.6)))
        #expect(abs(row.spaceLength - 5) < 1e-9)
        #expect(row.dividers.count == 2)
        #expect(close(row.dividers[0].a, plan(5, 5)) && close(row.dividers[0].b, plan(5, 2.6)))
        #expect(close(row.dividers[1].a, plan(10, 5)) && close(row.dividers[1].b, plan(10, 2.6)))
    }

    @Test func theLeftSideIsTheOtherWay() throws {
        let row = try #require(
            StreetGeometry.rowOfSpaces(plan(0, 5), plan(15, 5), count: 1, depth: 2, side: 1))
        #expect(close(row.ring[2], plan(15, 7)))
        #expect(row.dividers.isEmpty)
    }

    @Test func noRowWithoutLengthCountOrDepth() {
        #expect(
            StreetGeometry.rowOfSpaces(plan(1, 1), plan(1, 1), count: 2, depth: 2, side: 1) == nil)
        #expect(
            StreetGeometry.rowOfSpaces(plan(0, 0), plan(5, 0), count: 0, depth: 2, side: 1) == nil)
        #expect(
            StreetGeometry.rowOfSpaces(plan(0, 0), plan(5, 0), count: 2, depth: 0, side: 1) == nil)
    }

    @Test func theStreetSideFacesTheNearestCentreline() {
        let centre = target("c", [plan(-40, 0), plan(40, 0)], layer: .centreline)
        // Along the north kerb eastwards, the street is on the right.
        #expect(StreetGeometry.streetSide(plan(0, 5), plan(15, 5), centrelines: [centre]) == -1)
        // Along the south kerb eastwards, on the left.
        #expect(StreetGeometry.streetSide(plan(0, -5), plan(15, -5), centrelines: [centre]) == 1)
        // With no centreline, left.
        #expect(StreetGeometry.streetSide(plan(0, 5), plan(15, 5), centrelines: []) == 1)
    }

    @Test func aRowDrawnAsOneOutlineHasItsDividers() {
        let ring = [plan(0, 5), plan(15, 5), plan(15, 2.6), plan(0, 2.6)]
        let dividers = StreetGeometry.rowDividers(ring, count: 3)
        #expect(dividers.count == 2)
        #expect(close(dividers[0].a, plan(5, 5)) && close(dividers[0].b, plan(5, 2.6)))
        #expect(StreetGeometry.rowDividers(ring, count: 1).isEmpty)
        #expect(StreetGeometry.rowDividers(Array(ring.prefix(3)), count: 3).isEmpty)
    }
}

// MARK: - A lane's direction

struct StreetLaneTests {
    @Test func aLaneTravelsFromItsFirstVertexToItsLast() {
        #expect(StreetGeometry.laneDirection([plan(0, 0), plan(10, 0)]) == 90)
        #expect(StreetGeometry.laneDirection([plan(0, 0), plan(0, 10)]) == 0)
        #expect(StreetGeometry.laneDirection([plan(0, 0), plan(5, 5), plan(0, -10)]) == 180)
        #expect(StreetGeometry.laneDirection([plan(1, 1)]) == nil)
        #expect(StreetGeometry.laneDirection([plan(1, 1), plan(1, 1)]) == nil)
    }

    @Test func reverseTurnsTheLaneRound() {
        let lane = mark("l", kind: .travelLane, [plan(0, 0), plan(5, 1), plan(10, 0)])
        let reversed = StreetUseEditing.reversed(lane)
        #expect(reversed.plan == [plan(10, 0), plan(5, 1), plan(0, 0)])
        #expect(StreetGeometry.laneDirection(reversed.plan) == 270)
        #expect(StreetUseEditing.reversed(reversed) == lane)
    }

    @Test func arrowsEveryEightMetresOnTheirSegment() {
        let arrows = StreetGeometry.laneArrows([plan(0, 0), plan(20, 0), plan(20, 10)], spacing: 8)
        #expect(arrows.count == 4)
        #expect(close(arrows[0].at, plan(4, 0)) && arrows[0].bearing == 90)
        #expect(close(arrows[2].at, plan(20, 0)) || arrows[2].at.x == 20)
        #expect(arrows[3].bearing == 0)
        // Shorter than one spacing: one arrow at the middle.
        let short = StreetGeometry.laneArrows([plan(0, 0), plan(0, 4)], spacing: 8)
        #expect(short.count == 1 && close(short[0].at, plan(0, 2)))
    }
}

// MARK: - Inserting a vertex

struct StreetInsertionTests {
    @Test func theNearestEdgeWithinReachTakesTheVertex() throws {
        let line = [plan(0, 0), plan(10, 0), plan(10, 10)]
        let at = try #require(
            StreetGeometry.insertion(at: plan(4, 0.2), into: line, closed: false, tolerance: 0.5))
        #expect(at.index == 1)
        #expect(close(at.point, plan(4, 0)))
        let second = try #require(
            StreetGeometry.insertion(at: plan(9.8, 6), into: line, closed: false, tolerance: 0.5))
        #expect(second.index == 2)
        #expect(
            StreetGeometry.insertion(at: plan(4, 2), into: line, closed: false, tolerance: 0.5)
                == nil)
    }

    @Test func aRingTakesAVertexOnItsClosingEdge() throws {
        let ring = [plan(0, 0), plan(4, 0), plan(4, 4), plan(0, 4)]
        let at = try #require(
            StreetGeometry.insertion(at: plan(0.1, 2), into: ring, closed: true, tolerance: 0.5))
        #expect(at.index == 4)
        let surface = mark("s", kind: .surface, ring)
        let more = StreetUseEditing.insertVertex(surface, at: at.index, SIMD3(at.point, 21))
        #expect(more.plan == ring + [plan(0, 2)])
    }

    @Test func aDrivewayStaysOneSpan() {
        let driveway = mark("d", kind: .driveway, [plan(0, 5), plan(4, 5)])
        #expect(StreetUseEditing.insertVertex(driveway, at: 1, SIMD3(2, 5, 21)) == driveway)
        // Nor do vertices go below what a kind needs.
        #expect(StreetUseEditing.removeVertex(driveway, 0) == driveway)
        let surface = mark("s", kind: .surface, [plan(0, 0), plan(4, 0), plan(4, 4)])
        #expect(StreetUseEditing.removeVertex(surface, 1) == surface)
        let four = mark("s", kind: .surface, [plan(0, 0), plan(4, 0), plan(4, 4), plan(0, 4)])
        #expect(
            StreetUseEditing.removeVertex(four, 1).plan == [plan(0, 0), plan(4, 4), plan(0, 4)])
    }
}

// MARK: - Filling a polygon

struct StreetFillTests {
    private func area(_ polygon: [PlanPoint], _ indices: [Int]) -> Double {
        stride(from: 0, to: indices.count, by: 3).reduce(0) { sum, t in
            sum
                + abs(
                    StreetGeometry.signedArea([
                        polygon[indices[t]], polygon[indices[t + 1]], polygon[indices[t + 2]],
                    ]))
        }
    }

    @Test func aConcaveOutlineIsFilledExactly() {
        // An L, clockwise: 3 by 3 less a 2 by 2 corner, 5 m².
        let l = [plan(0, 0), plan(0, 3), plan(1, 3), plan(1, 1), plan(3, 1), plan(3, 0)]
        let indices = StreetGeometry.triangulate(l)
        #expect(indices.count == (l.count - 2) * 3)
        #expect(abs(area(l, indices) - 5) < 1e-9)
        #expect(abs(abs(StreetGeometry.signedArea(l)) - 5) < 1e-9)
    }

    @Test func containmentFollowsTheOutline() {
        let l = [plan(0, 0), plan(0, 3), plan(1, 3), plan(1, 1), plan(3, 1), plan(3, 0)]
        #expect(StreetGeometry.contains(l, plan(0.5, 2)))
        #expect(!StreetGeometry.contains(l, plan(2, 2)))
    }
}

// MARK: - The ground under the cursor

struct GroundSurfaceTests {
    /// A tilted plane, z = 20 + 0.1·x + 0.05·y, on a 1 m grid from (−10, −10).
    private func plane() -> GroundSurface {
        let nx = 21
        let ny = 21
        var z = [Float]()
        for j in 0..<ny {
            for i in 0..<nx {
                let x = -10 + Double(i)
                let y = -10 + Double(j)
                z.append(Float(20 + 0.1 * x + 0.05 * y))
            }
        }
        return GroundSurface(fields: [
            GroundSurface.Field(origin: plan(-10, -10), cellM: 1, nx: nx, ny: ny, z: z)
        ])
    }

    @Test func heightIsBilinearBetweenCellCentres() throws {
        let ground = plane()
        let z = try #require(ground.height(at: plan(2.3, -4.6)))
        #expect(abs(z - (20 + 0.23 - 0.23)) < 1e-5)
        #expect(ground.height(at: plan(30, 0)) == nil)
    }

    @Test func aSlantedRayMeetsThePlaneWhereItShould() throws {
        let ground = plane()
        let origin = SIMD3<Double>(-5, -5, 40)
        let direction = simd_normalize(SIMD3<Double>(0.3, 0.2, -1))
        let hit = try #require(ground.intersect(origin: origin, direction: direction))
        // Solve 40 − t·dz = 20 + 0.1(−5 + t·dx) + 0.05(−5 + t·dy) for t.
        let t = (40 - 20 + 0.5 + 0.25) / (-direction.z + 0.1 * direction.x + 0.05 * direction.y)
        let expected = origin + direction * t
        #expect(simd_length(hit - expected) < 0.01)
        let z = try #require(ground.height(at: plan(hit.x, hit.y)))
        #expect(abs(hit.z - z) < 0.01)
    }

    @Test func straightDownLandsOnTheGroundUnderThePoint() throws {
        let ground = plane()
        let hit = try #require(
            ground.intersect(origin: SIMD3(3, 4, 300), direction: SIMD3(0, 0, -1)))
        #expect(abs(hit.x - 3) < 1e-9 && abs(hit.y - 4) < 1e-9)
        #expect(abs(hit.z - (20 + 0.3 + 0.2)) < 1e-5)
        #expect(ground.intersect(origin: SIMD3(3, 4, 300), direction: SIMD3(0, 0, 1)) == nil)
    }

    @Test func aCellWithGroundMissingTakesWhatItHas() throws {
        let ground = GroundSurface(fields: [
            GroundSurface.Field(
                origin: plan(0, 0), cellM: 1, nx: 2, ny: 2, z: [21, .nan, 23, .nan])
        ])
        #expect(try #require(ground.height(at: plan(0.5, 0.5))) == 22)
        let empty = GroundSurface(fields: [
            GroundSurface.Field(
                origin: plan(0, 0), cellM: 1, nx: 2, ny: 2, z: [.nan, .nan, .nan, .nan])
        ])
        #expect(empty.height(at: plan(0.5, 0.5)) == nil)
    }

    @Test func withoutAGroundMeshAClickTakesTheNearestStreetsHeight() {
        var g = ReferenceGeometry()
        var kerb = Gerardus_Align_V1alpha1_LineFeature()
        kerb.kind = .kerb
        kerb.xyz = [0, 5, 17.5, 20, 5, 18.5]
        g.kerbs = [kerb]
        let reference = StreetReference(geometry: g)
        #expect(reference.groundZ(at: plan(18, 6)) == 18.5)
        // Too far from any street: 0, which Gerardus drapes.
        #expect(reference.groundZ(at: plan(500, 500)) == 0)
    }
}

// MARK: - The plan view

struct PlanViewTests {
    private let size = CGSize(width: 800, height: 600)

    @Test func pixelsMapToTheSiteAndBack() {
        let view = PlanView(centre: plan(10, 20), groundZ: 21, halfHeight: 30)
        #expect(abs(view.metresPerPoint(viewHeight: 600) - 0.1) < 1e-12)
        #expect(close(view.sitePoint(atView: CGPoint(x: 400, y: 300), size: size), plan(10, 20)))
        // East to the right, north up (the view's origin is bottom left).
        #expect(close(view.sitePoint(atView: CGPoint(x: 500, y: 300), size: size), plan(20, 20)))
        #expect(close(view.sitePoint(atView: CGPoint(x: 400, y: 400), size: size), plan(10, 30)))
        let p = view.viewPoint(ofSite: plan(3.7, 41.2), size: size)
        #expect(close(view.sitePoint(atView: p, size: size), plan(3.7, 41.2), 1e-6))
    }

    @Test func theCameraDrawsTheSiteWhereThePixelsSay() {
        let pose = tiltedPose()
        let view = PlanView(centre: plan(-12, -30), groundZ: 21, halfHeight: 25)
        let camera = view.camera(pose: pose, aspectRatio: Float(size.width / size.height))
        #expect(camera.projection == .orthographic(halfHeight: 25))
        let mvp = camera.projectionMatrix * camera.viewMatrix * pose.referenceModelMatrix
        for site in [plan(-12, -30), plan(0, -20), plan(-30, -45)] {
            for z in [18.0, 21.0, 30.0] {
                let clip = mvp * SIMD4<Float>(Float(site.x), Float(site.y), Float(z), 1)
                let ndc = SIMD2<Double>(Double(clip.x / clip.w), Double(clip.y / clip.w))
                let pixel = CGPoint(
                    x: (ndc.x + 1) / 2 * size.width, y: (ndc.y + 1) / 2 * size.height)
                // Whatever the height, straight down: the same pixel.
                #expect(close(view.sitePoint(atView: pixel, size: size), site, 1e-3))
            }
        }
    }

    @Test func panningDragsTheGroundAndZoomKeepsThePointerOnIt() {
        var view = PlanView(centre: plan(0, 0), groundZ: 0, halfHeight: 30)
        let under = view.sitePoint(atView: CGPoint(x: 600, y: 100), size: size)
        view.zoom(by: 0.5, about: CGPoint(x: 600, y: 100), size: size)
        #expect(view.halfHeight == 15)
        #expect(close(view.sitePoint(atView: CGPoint(x: 600, y: 100), size: size), under, 1e-9))
        let before = view.sitePoint(atView: CGPoint(x: 100, y: 100), size: size)
        view.pan(byViewDelta: 40, -20, size: size)
        #expect(close(view.sitePoint(atView: CGPoint(x: 140, y: 80), size: size), before, 1e-9))
        view.zoom(by: 1e6, about: .zero, size: size)
        #expect(view.halfHeight == PlanView.maxHalfHeight)
    }
}

/// The renderer holds the camera while painting, and gives the old one back.
@MainActor struct PlanCameraLockTests {
    private func renderer() -> MetalRenderer? {
        guard let device = MTLCreateSystemDefaultDevice() else { return nil }
        let view = MTKView()
        view.device = device
        return MetalRenderer(metalView: view)
    }

    @Test func thePlanViewLocksRotationAndRestoresTheCamera() {
        guard let renderer = renderer() else { return }
        renderer.camera.position = SIMD3(5, -40, 25)
        renderer.camera.target = SIMD3(1, 8, 0)
        renderer.camera.aspectRatio = 1.5
        let before = renderer.camera
        #expect(!renderer.isPlanLocked)

        let view = PlanView(centre: plan(-9, -38), groundZ: 21)
        renderer.setPlanCamera(view.camera(pose: tiltedPose(), aspectRatio: 1))
        #expect(renderer.isPlanLocked)
        #expect(
            renderer.camera.projection
                == .orthographic(halfHeight: Float(PlanView.defaultHalfHeight)))
        // The view's own aspect is kept.
        #expect(renderer.camera.aspectRatio == 1.5)
        let locked = renderer.camera

        // Orbiting, the renderer's own pan and zoom, and the arrow keys are
        // all refused: the painter pans and zooms the plan itself.
        renderer.handleMouseDrag(deltaX: 50, deltaY: 30, isRightButton: false, shiftHeld: false)
        renderer.handleMouseDrag(deltaX: 50, deltaY: 30, isRightButton: true, shiftHeld: false)
        renderer.handleZoom(delta: 3)
        #expect(!renderer.handleKeyDown(keyCode: 123, modifiers: []))
        #expect(!renderer.handleKeyDown(keyCode: 15, modifiers: []))
        renderer.focusCamera(onSitePoint: SIMD3(0, 0, 0))
        #expect(renderer.camera == locked)

        // A second plan camera moves the view but keeps the first camera.
        var panned = view
        panned.centre += SIMD2(5, 5)
        renderer.setPlanCamera(panned.camera(pose: tiltedPose(), aspectRatio: 1))
        #expect(renderer.camera != locked)

        renderer.exitPlanView()
        #expect(!renderer.isPlanLocked)
        #expect(renderer.camera == before)
        // Unlocked, orbiting turns the camera again.
        renderer.handleMouseDrag(deltaX: 50, deltaY: 0, isRightButton: false, shiftHeld: false)
        #expect(renderer.camera != before)
    }

    @Test func theOverlayHasTrianglesAndLines() {
        guard let renderer = renderer() else { return }
        var scene = StreetPaintScene()
        scene.marks = [
            mark(
                "s", kind: .surface, [plan(0, 0), plan(4, 0), plan(4, 4), plan(0, 4)],
                tags: ["surface": "sidewalk"])
        ]
        let vertices = StreetPaintStyle.vertices(scene)
        renderer.setStreetOverlay(vertices)
        // Two triangles fill the square; four segments outline it.
        #expect(renderer.streetFillVertexCount == 6)
        #expect(renderer.streetLineVertexCount == 8)
        renderer.setStreetOverlay(nil)
        #expect(renderer.streetFillVertexCount == 0 && renderer.streetLineVertexCount == 0)
    }
}

// MARK: - The save diff

struct StreetDiffTests {
    private let lane = mark(
        "gms:lane", id: "gms:lane", kind: .travelLane, [plan(0, 0), plan(10, 0)],
        tags: ["lane_type": "general", "width_m": "3.3"])
    private let space = mark(
        "gms:space", id: "gms:space", kind: .parkingSpace,
        [plan(0, 5), plan(5, 5), plan(5, 3), plan(0, 3)],
        tags: ["space_type": "metered", "count": "1"])

    @Test func unchangedSendsNothing() {
        #expect(StreetUseEditing.diff(base: [lane, space], marks: [lane, space]).isEmpty)
        // Heights are Gerardus's to drape; empty tags are not tags.
        var draped = lane
        draped.points = draped.points.map { SIMD3($0.x, $0.y, 25) }
        draped.tags["time_limit"] = "  "
        #expect(StreetUseEditing.diff(base: [lane], marks: [draped]).isEmpty)
    }

    @Test func aNewMarkIsAnUpsertWithoutAnID() throws {
        let surface = mark(
            "new-1", kind: .surface, [plan(0, 0), plan(4, 0), plan(4, 4)],
            tags: ["surface": "verge"])
        let edits = StreetUseEditing.diff(base: [lane], marks: [lane, surface])
        #expect(edits.count == 1)
        guard case .upsert(let feature) = try #require(edits.first?.change) else {
            Issue.record("not an upsert")
            return
        }
        #expect(feature.featureID.isEmpty)
        #expect(feature.kind == .surface)
        #expect(feature.closed)
        #expect(feature.xyz == [0, 0, 21, 4, 0, 21, 4, 4, 21])
        #expect(feature.tags == ["surface": "verge"])
        #expect(feature.author.isEmpty && feature.rowID.isEmpty)
    }

    @Test func aMovedMarkIsAnUpsertOfItsID() throws {
        let moved = StreetUseEditing.moveVertex(lane, 1, to: SIMD3(12, 0, 21))
        let edits = StreetUseEditing.diff(base: [lane], marks: [moved])
        #expect(edits.count == 1)
        #expect(edits[0].hasExpectedRowID && edits[0].expectedRowID == lane.rowID)
        guard case .upsert(let feature) = try #require(edits.first?.change) else {
            Issue.record("not an upsert")
            return
        }
        #expect(feature.featureID == "gms:lane")
        #expect(feature.kind == .travelLane)
        #expect(!feature.closed)
        #expect(feature.xyz == [0, 0, 21, 12, 0, 21])
    }

    @Test func aRetaggedMarkIsAnUpsert() {
        let retagged = StreetUseEditing.settingTag(lane, "lane_type", "bus")
        #expect(StreetUseEditing.diff(base: [lane], marks: [retagged]).count == 1)
    }

    @Test func aRemovedMarkIsARemoveAndRemovesComeFirst() {
        let surface = mark("new-1", kind: .surface, [plan(0, 0), plan(4, 0), plan(4, 4)])
        let edits = StreetUseEditing.diff(base: [lane, space], marks: [surface, lane])
        #expect(edits.count == 2)
        #expect(edits.first?.removeFeatureID == "gms:space")
        if case .upsert(let feature) = edits.last?.change {
            #expect(feature.kind == .surface)
        } else {
            Issue.record("the last edit is not the new surface's upsert")
        }
    }

    @Test func movedAndMovedBackSendsNothing() {
        var state = StreetEditState(base: [lane])
        state.update(lane.key) { StreetUseEditing.moveVertex($0, 1, to: SIMD3(12, 0, 21)) }
        #expect(state.pendingEdits.count == 1)
        state.update(lane.key) { StreetUseEditing.moveVertex($0, 1, to: SIMD3(10, 0, 19)) }
        #expect(state.pendingEdits.isEmpty)
        #expect(!state.isDirty)
    }

    @Test func undoAndRedoStepThroughTheChanges() {
        var state = StreetEditState(base: [lane])
        state.remove(lane.key)
        state.add(space)
        #expect(state.marks == [space])
        state.undo()
        #expect(state.marks.isEmpty)
        state.undo()
        #expect(state.marks == [lane])
        #expect(!state.canUndo && state.canRedo)
        state.redo()
        #expect(state.marks.isEmpty)
        // A new change drops what was undone.
        state.add(lane)
        #expect(!state.canRedo)
        // A change that changes nothing is not kept.
        let depth = state.canUndo
        state.update("no such mark") { $0 }
        state.update(lane.key) { $0 }
        #expect(state.canUndo == depth)
    }

    @Test func featuresReadBackAsMarks() {
        var ring = Gerardus_Align_V1alpha1_StreetUseFeature()
        ring.featureID = "gms:a"
        ring.kind = .surface
        ring.closed = true
        // Sent closed: the first vertex repeated at the end.
        ring.xyz = [0, 0, 21, 4, 0, 21, 4, 4, 21, 0, 0, 21]
        ring.tags = ["surface": "median"]
        ring.author = "Ada"
        ring.rowID = "row-1"
        var road = Gerardus_Align_V1alpha1_StreetUseFeature()
        road.kind = .road
        road.xyz = [0, 0, 0, 1, 1, 0]
        var short = Gerardus_Align_V1alpha1_StreetUseFeature()
        short.featureID = "gms:b"
        short.kind = .parkingSpace
        short.xyz = [0, 0, 0, 1, 1, 0]
        let read = StreetUseEditing.marks(from: [ring, road, short])
        #expect(read.skipped == 2)
        #expect(read.marks.count == 1)
        #expect(read.marks[0].key == "gms:a")
        #expect(read.marks[0].plan == [plan(0, 0), plan(4, 0), plan(4, 4)])
        #expect(read.marks[0].author == "Ada" && read.marks[0].rowID == "row-1")
        #expect(read.marks[0].summary == "median")
    }

    @Test func newTagsComeFromTheToolsFields() {
        var fields = StreetUseTags.Fields()
        fields.laneType = "bike"
        fields.widthM = 1.8
        fields.count = 0
        fields.timeLimit = " 2hr "
        #expect(
            StreetUseTags.tags(for: .travelLane, fields) == ["lane_type": "bike", "width_m": "1.8"])
        #expect(
            StreetUseTags.tags(for: .parkingSpace, fields) == [
                "space_type": "metered", "count": "1", "time_limit": "2hr",
            ])
        #expect(StreetUseTags.tags(for: .surface, fields) == ["surface": "carriageway"])
        #expect(StreetUseTags.tags(for: .driveway, fields).isEmpty)
        #expect(StreetUseTags.format(3.3) == "3.3" && StreetUseTags.format(3) == "3")
    }
}

// MARK: - Problems

struct StreetProblemTests {
    @Test func problemsAreMappedToTheFeaturesTheyName() {
        let mapped = StreetUseEditing.mapProblems(
            [
                "gms:01HZX: lane crosses a kerb", "gms:01HZ: space in a lane", "gms:01HZX:   ",
                "driveway off any kerb",
            ], ids: ["gms:01HZ", "gms:01HZX", ""])
        // The longer id is matched first, though the shorter is its prefix.
        #expect(mapped.byID["gms:01HZX"] == ["lane crosses a kerb", "gms:01HZX:"])
        #expect(mapped.byID["gms:01HZ"] == ["space in a lane"])
        #expect(mapped.general == ["driveway off any kerb"])
    }

    @Test func aFlaggedMarkIsDrawnWithAHalo() {
        var scene = StreetPaintScene()
        scene.marks = [mark("gms:a", id: "gms:a", kind: .driveway, [plan(0, 5), plan(4, 5)])]
        let plain = StreetPaintStyle.vertices(scene)
        scene.flagged = ["gms:a"]
        let flagged = StreetPaintStyle.vertices(scene)
        // The halo is one more band: two more triangles, in red.
        #expect(flagged.fillVertexCount == plain.fillVertexCount + 6)
        let red = Array(flagged.fills[3..<7])
        #expect(
            red == [
                StreetPaintStyle.problem.x, StreetPaintStyle.problem.y, StreetPaintStyle.problem.z,
                StreetPaintStyle.problem.w,
            ])
    }
}

// MARK: - Styling

struct StreetStyleTests {
    @Test func eachKindDrawsItsOwnWay() {
        var scene = StreetPaintScene()
        scene.marks = [
            mark(
                "lane", kind: .travelLane, [plan(0, 0), plan(16, 0)],
                tags: ["lane_type": "bus", "width_m": "3"])
        ]
        let lane = StreetPaintStyle.vertices(scene)
        // The width band (two triangles), the centreline and two chevrons.
        #expect(lane.fillVertexCount == 6)
        #expect(lane.lineVertexCount == 2 + 2 * 4)

        scene.marks = [
            mark(
                "row", kind: .parkingSpace, [plan(0, 5), plan(15, 5), plan(15, 2.6), plan(0, 2.6)],
                tags: ["count": "3"])
        ]
        let row = StreetPaintStyle.vertices(scene)
        // The outline and two dividers.
        #expect(row.lineVertexCount == 2 * 4 + 2 * 2)

        scene.marks = [mark("d", kind: .driveway, [plan(0, 5), plan(4, 5)])]
        #expect(StreetPaintStyle.vertices(scene).fillVertexCount == 6)
    }

    @Test func surfacesAreColouredBySurfaceAndTranslucent() {
        let surfaces = StreetUseTags.surfaces.map(StreetPaintStyle.surfaceColour)
        #expect(Set(surfaces.map { [$0.x, $0.y, $0.z] }).count == surfaces.count)
        #expect(StreetPaintStyle.fillAlpha < 0.5)
        #expect(ReferenceStyle.lineColour(.surface) != ReferenceStyle.otherColour)
    }

    @Test func handlesAndDraftsOnlyWhilePainting() {
        var scene = StreetPaintScene()
        scene.marks = [mark("d", kind: .driveway, [plan(0, 5), plan(4, 5)])]
        scene.selectedKey = "d"
        scene.tool = .select
        let idle = StreetPaintStyle.vertices(scene)
        scene.painting = true
        let painting = StreetPaintStyle.vertices(scene)
        // A selection band and two handles.
        #expect(painting.fillVertexCount == idle.fillVertexCount + 6 + 2 * 6)
    }
}

// MARK: - The session

@MainActor struct StreetPainterSessionTests {
    @Test func placingASiteStreamsTheStreetLayersAndReadsItsMarks() async {
        var saved = Gerardus_Align_V1alpha1_StreetUseFeature()
        saved.featureID = "gms:01LANE"
        saved.kind = .travelLane
        saved.xyz = [-20, 2.5, 21, 20, 2.5, 21]
        saved.tags = ["lane_type": "general"]
        let (painter, session, service, _) = await placedPainter { $0.streetUse = [saved] }
        let streets = service.references.first { $0.layers.contains(.kerbs) }
        #expect(streets?.layers == [.cityStreets, .cityIntersections, .kerbs])
        #expect(session.reference?.kerbs == 2)
        #expect(session.reference?.cityStreets == 1)
        #expect(session.reference?.cityIntersections == 1)
        #expect(session.reference?.streetLayersServed == true)
        // Kerbs and the City's streets are drawn in layers of their own.
        let vertices = ReferenceStyle.vertices(for: session.referenceGeometry!)
        #expect(vertices.kerbVertexCount == 4)
        #expect(vertices.cityStreetVertexCount == 2 + 4)
        #expect(session.referenceGeometry?.lines.count == 1)  // OSM's road alone
        #expect(service.calls.filter { $0 == "ListStreetUse" }.count == 1)
        #expect(painter.loadedToken == columbus.l16Token)
        #expect(painter.edit.marks.map(\.featureID) == ["gms:01LANE"])
        #expect(painter.reference.kerbs.count == 2)
    }

    @Test func aServerWithoutStreetLayersIsNotAskedForThem() async {
        let service = FakeAlignmentService()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await session.probe()
        session.choose(columbus)
        await session.placeFromHint()
        #expect(!service.references.contains { $0.layers.contains(.kerbs) })
        #expect(session.reference?.streetLayersServed == false)
    }

    @Test func paintModeLocksThePlanViewAndLeavingPutsTheCameraBack() async throws {
        let (painter, session, _, sink) = await placedPainter()
        #expect(painter.canPaint)
        painter.setPainting(true)
        #expect(painter.isPainting)
        let camera = try #require(sink.cameras.last)
        #expect(camera.projection == .orthographic(halfHeight: Float(PlanView.defaultHalfHeight)))
        // Centred over the sensor.
        let pose = session.pose!
        #expect(close(painter.plan!.centre, plan(pose.translation.x, pose.translation.y)))
        // Scrolling zooms the plan view, and pushes the camera again.
        painter.zoom(delta: 5, at: CGPoint(x: 400, y: 300))
        #expect(painter.plan!.halfHeight < PlanView.defaultHalfHeight)
        #expect(sink.cameras.count == 2)
        painter.setPainting(false)
        #expect(sink.exits == 1)
        // Clearing the site leaves paint mode too.
        painter.setPainting(true)
        session.choose(
            AlignmentSite(
                clipID: "x", l16Token: "808580f49", l13Token: "808580f4", description: "",
                sensorID: "lidar", latitude: 37.8, longitude: -122.4))
        #expect(!painter.isPainting)
        #expect(sink.exits == 2)
        #expect(painter.loadedToken == nil)
        #expect(painter.paintBlocker != nil)
    }

    @Test func drawingASurfaceSnapsToTheKerbAndStandsOnTheGround() async throws {
        let (painter, _, _, _) = await placedPainter()
        painter.setPainting(true)
        painter.tool = .surface
        painter.fields.surface = "sidewalk"
        click(painter, plan(0, 5.3))  // 0.3 m off the north kerb
        click(painter, plan(10, 5.1))
        click(painter, plan(10, 8))
        click(painter, plan(10, 8), count: 2)  // a double-click finishes
        let surface = try #require(painter.edit.marks.last)
        #expect(surface.kind == .surface)
        #expect(surface.points.count == 3)
        #expect(abs(surface.points[0].y - 5) < 1e-6 && abs(surface.points[1].y - 5) < 1e-6)
        // No ground mesh there: the kerb's height.
        #expect(surface.points.allSatisfy { $0.z == 21 })
        #expect(surface.tags == ["surface": "sidewalk"])
        #expect(painter.selectedKey == surface.key)
        #expect(painter.drawing.isEmpty)
    }

    @Test func aRowOfSpacesRunsOnTheStreetSide() async throws {
        let (painter, _, _, _) = await placedPainter()
        painter.setPainting(true)
        painter.tool = .row
        painter.fields.count = 3
        click(painter, plan(0, 5))
        click(painter, plan(15, 5))
        let row = try #require(painter.edit.marks.last)
        #expect(row.kind == .parkingSpace)
        #expect(row.plan.count == 4)
        // The street is south of the north kerb.
        #expect(abs(row.plan[2].y - 2.6) < 1e-6 && abs(row.plan[3].y - 2.6) < 1e-6)
        #expect(row.tags["count"] == "3" && row.tags["space_type"] == "metered")
    }

    @Test func aDrivewayIsTwoClicksAndALaneCanBeReversed() async throws {
        let (painter, _, _, _) = await placedPainter()
        painter.setPainting(true)
        painter.tool = .driveway
        click(painter, plan(3, -5.2))
        click(painter, plan(7, -4.9))
        let driveway = try #require(painter.edit.marks.last)
        #expect(driveway.kind == .driveway)
        #expect(driveway.plan.count == 2 && driveway.plan.allSatisfy { abs($0.y + 5) < 1e-6 })

        painter.tool = .lane
        click(painter, plan(-20, 2.5))
        click(painter, plan(20, 2.5))
        #expect(painter.key(.finish))
        let lane = try #require(painter.edit.marks.last)
        #expect(StreetGeometry.laneDirection(lane.plan) == 90)
        painter.reverseSelected()
        #expect(StreetGeometry.laneDirection(painter.selected!.plan) == 270)
    }

    @Test func selectMovesInsertsAndDeletesVerticesWithUndoAndRedo() async throws {
        var saved = Gerardus_Align_V1alpha1_StreetUseFeature()
        saved.featureID = "gms:01LANE"
        saved.kind = .travelLane
        saved.xyz = [-20, 2.5, 21, 20, 2.5, 21]
        saved.tags = ["lane_type": "general"]
        let (painter, _, _, _) = await placedPainter { $0.streetUse = [saved] }
        painter.setPainting(true)
        painter.tool = .select
        click(painter, plan(0, 2.6))
        #expect(painter.selectedKey == "gms:01LANE")

        // Drag the east end north.
        drag(painter, from: plan(20, 2.5), to: plan(20, 3.5))
        #expect(painter.selected!.plan == [plan(-20, 2.5), plan(20, 3.5)])
        // Drag the middle of the edge: a new vertex.
        drag(painter, from: plan(0, 3.0), to: plan(0, 1.5))
        #expect(painter.selected!.plan.count == 3)
        #expect(close(painter.selected!.plan[1], plan(0, 1.5), 1e-6))
        // Right-click a vertex: gone again.
        let p = painter.plan!.viewPoint(ofSite: plan(0, 1.5), size: painter.viewSize)
        painter.rightMouseDown(at: p)
        painter.rightMouseUp(at: p)
        #expect(painter.selected!.plan.count == 2)
        #expect(painter.edit.pendingEdits.count == 1)

        painter.undo()
        #expect(painter.selected!.plan.count == 3)
        painter.undo()
        painter.undo()
        #expect(painter.selected!.plan == [plan(-20, 2.5), plan(20, 2.5)])
        #expect(painter.edit.pendingEdits.isEmpty)
        painter.redo()
        #expect(painter.selected!.plan == [plan(-20, 2.5), plan(20, 3.5)])

        // Delete removes the selected mark; Save would send a remove.
        #expect(painter.key(.delete))
        #expect(painter.edit.marks.isEmpty)
        #expect(painter.edit.pendingEdits.map(\.removeFeatureID) == ["gms:01LANE"])
    }

    @Test func dragsOnEmptyGroundPanAndDoNotDraw() async {
        let (painter, _, _, sink) = await placedPainter()
        painter.setPainting(true)
        painter.tool = .surface
        let before = painter.plan!.centre
        painter.mouseDown(at: CGPoint(x: 400, y: 300))
        painter.mouseDragged(to: CGPoint(x: 420, y: 300))
        painter.mouseDragged(to: CGPoint(x: 440, y: 290))
        painter.mouseUp(at: CGPoint(x: 440, y: 290), clickCount: 1)
        #expect(painter.drawing.isEmpty)
        // Dragged 40 points east and 10 down at 0.1 m a point.
        #expect(close(painter.plan!.centre, before + plan(-4, 1), 1e-9))
        #expect(sink.cameras.count >= 2)
    }

    @Test func saveNeedsAnAuthorAndSendsTheDiffAsOneEdit() async throws {
        let (painter, _, service, _) = await placedPainter()
        painter.setPainting(true)
        painter.tool = .driveway
        click(painter, plan(3.04, -5.2))
        click(painter, plan(7.01, -4.9))
        #expect(painter.edit.pendingEdits.count == 1)

        // No author: nothing is sent.
        #expect(painter.saveBlockers(author: "  ").contains { $0.hasPrefix("No author") })
        await painter.save(author: "  ")
        #expect(!service.calls.contains("EditStreetUse"))

        painter.note = "west side"
        await painter.save(author: " Ada ")
        let request = try #require(service.streetUseEdits.last)
        #expect(request.l16Token == columbus.l16Token)
        #expect(request.author == "Ada")
        #expect(request.note == "west side")
        #expect(!request.requestID.isEmpty)
        #expect(request.edits.count == 1)
        // What Gerardus answered replaces the marks: its id, its snap.
        let saved = try #require(painter.edit.marks.first)
        #expect(saved.featureID == "gms:01NEW1")
        #expect(abs(saved.plan[0].x - 3.0) < 1e-6)
        #expect(saved.author == "Ada")
        #expect(painter.edit.pendingEdits.isEmpty)
        #expect(painter.note.isEmpty)
        #expect(painter.savedMessage?.hasPrefix("Saved 1 change;") == true)
        // Nothing left to send.
        #expect(painter.saveBlockers(author: "Ada") == ["No changes to save."])
    }

    @Test func problemsComeBackBesideTheirMarks() async throws {
        let (painter, _, service, sink) = await placedPainter {
            $0.streetUseProblems = { features in
                features.map { "\($0.featureID): driveway off any kerb" } + [
                    "site: no kerbs observed"
                ]
            }
        }
        painter.setPainting(true)
        painter.tool = .driveway
        click(painter, plan(3, 20))
        click(painter, plan(7, 20))
        let plain = sink.overlays.last??.fillVertexCount ?? 0
        await painter.save(author: "Ada")
        #expect(service.streetUseEdits.count == 1)
        let saved = try #require(painter.edit.marks.first)
        #expect(painter.problems(for: saved) == ["driveway off any kerb"])
        #expect(painter.generalProblems == ["site: no kerbs observed"])
        #expect(painter.savedMessage?.contains("2 problems") == true)
        // Highlighted in the view: a halo band more than the mark alone.
        let flagged = sink.overlays.last??.fillVertexCount ?? 0
        #expect(flagged == plain + 6)
    }

    @Test func aRefusedSaveKeepsTheChanges() async {
        let (painter, _, service, _) = await placedPainter {
            $0.refuseStreetUse = "lane 3 crosses itself"
        }
        painter.setPainting(true)
        painter.tool = .driveway
        click(painter, plan(3, -5))
        click(painter, plan(7, -5))
        await painter.save(author: "Ada")
        #expect(painter.errorMessage?.contains("lane 3 crosses itself") == true)
        #expect(painter.edit.pendingEdits.count == 1)
        let rejectedID = service.streetUseEdits[0].requestID
        service.refuseStreetUse = nil
        painter.note = "corrected intent"
        await painter.save(author: "Ada")
        #expect(!painter.edit.isDirty)
        #expect(service.streetUseEdits.last?.requestID != rejectedID)
    }

    @Test func aLostResponseRetriesTheSameStreetIntentAndID() async {
        let (painter, _, service, _) = await placedPainter()
        painter.setPainting(true)
        painter.tool = .driveway
        click(painter, plan(3, -5))
        click(painter, plan(7, -5))
        service.streetUseFailure = RPCError(code: .unavailable, message: "lost response")
        await painter.save(author: "Ada")
        #expect(painter.edit.isDirty)
        service.streetUseFailure = nil
        await painter.save(author: "Ada")
        #expect(service.streetUseEdits.count == 2)
        #expect(service.streetUseEdits[0] == service.streetUseEdits[1])
        #expect(!painter.edit.isDirty)
    }

    @Test func aConflictReadsCurrentMarksWithoutReplacingTheDraftOrResubmitting() async {
        let (painter, _, service, _) = await placedPainter()
        painter.setPainting(true)
        painter.tool = .driveway
        click(painter, plan(3, -5))
        click(painter, plan(7, -5))
        let draft = painter.edit.marks
        service.streetUseFailure = RPCError(code: .aborted, message: "newer row")
        await painter.save(author: "Ada")
        #expect(painter.edit.marks == draft)
        #expect(service.streetUseEdits.count == 1)
        #expect(painter.errorMessage?.contains("draft is kept") == true)
        #expect(service.calls.last == "ListStreetUse")
    }

    @Test func aLaterRefusalCannotResolveAnEarlierUnknownOutcome() async {
        let (painter, _, service, _) = await placedPainter()
        painter.setPainting(true)
        painter.tool = .driveway
        click(painter, plan(3, -5))
        click(painter, plan(7, -5))
        service.streetUseFailure = RPCError(code: .unavailable, message: "lost response")
        await painter.save(author: "Ada")
        service.streetUseFailure = RPCError(
            code: .failedPrecondition, message: "reference temporarily missing")
        await painter.save(author: "Ada")
        #expect(painter.hasPendingSave)
        #expect(service.streetUseEdits[0] == service.streetUseEdits[1])
        painter.note = "a different intent"
        service.streetUseFailure = nil
        await painter.save(author: "Ada")
        #expect(service.streetUseEdits.count == 2)
        #expect(painter.edit.isDirty)
    }

    @Test func aSavedMarkIsMovedAndRemovedAgainstItsCurrentRow() async throws {
        let (painter, _, service, _) = await placedPainter()
        painter.setPainting(true)
        painter.tool = .driveway
        click(painter, plan(3, -5))
        click(painter, plan(7, -5))
        await painter.save(author: "Ada")
        let created = try #require(painter.edit.marks.first)
        #expect(service.streetUseEdits[0].edits[0].hasExpectedRowID == false)

        painter.tool = .select
        click(painter, plan(5, -5))
        drag(painter, from: plan(7, -5), to: plan(7, -6))
        await painter.save(author: "Ada")
        #expect(painter.errorMessage == nil)
        #expect(service.streetUseEdits[1].edits.map(\.expectedRowID) == [created.rowID])
        let moved = try #require(painter.edit.marks.first)
        #expect(moved.rowID != created.rowID)

        click(painter, plan(5, -5.5))
        painter.deleteSelected()
        await painter.save(author: "Ada")
        #expect(painter.errorMessage == nil)
        #expect(service.streetUseEdits[2].edits.map(\.expectedRowID) == [moved.rowID])
        #expect(service.streetUse?.isEmpty == true)
    }

    @Test func anotherAuthorsSaveSinceTheReadIsAConflict() async {
        var saved = Gerardus_Align_V1alpha1_StreetUseFeature()
        saved.featureID = "gms:01LANE"
        saved.rowID = "row-a"
        saved.kind = .travelLane
        saved.xyz = [-20, 2.5, 21, 20, 2.5, 21]
        saved.tags = ["lane_type": "general"]
        let (painter, _, service, _) = await placedPainter { $0.streetUse = [saved] }
        painter.setPainting(true)
        painter.tool = .select
        click(painter, plan(0, 2.6))
        drag(painter, from: plan(20, 2.5), to: plan(20, 3.5))
        service.streetUse?[0].rowID = "row-b"  // someone else saved since the read
        await painter.save(author: "Ada")
        #expect(service.streetUseEdits.last?.edits.map(\.expectedRowID) == ["row-a"])
        #expect(painter.errorMessage?.hasPrefix("Conflict") == true)
        #expect(painter.edit.isDirty)
        #expect(painter.conflictingMarks.map(\.rowID) == ["row-b"])
        #expect(service.streetUse?[0].xyz == saved.xyz)  // nothing applied
    }

    @Test func aSaveWhoseAnswerIsLostIsAppliedOnce() async {
        let (painter, _, service, _) = await placedPainter()
        painter.setPainting(true)
        painter.tool = .driveway
        click(painter, plan(3, -5))
        click(painter, plan(7, -5))
        service.lostResponse = RPCError(code: .unavailable, message: "lost after commit")
        await painter.save(author: "Ada")
        #expect(painter.edit.isDirty)
        #expect(service.streetUse?.count == 1)
        // The unchanged retry gets Gerardus's saved answer; no second mark.
        await painter.save(author: "Ada")
        #expect(!painter.edit.isDirty)
        #expect(service.streetUse?.count == 1)
        #expect(painter.edit.marks.map(\.featureID) == ["gms:01NEW1"])
    }

    @Test func aServerWithoutStreetUseCannotBePainted() async {
        let (painter, _, _, _) = await placedPainter { $0.streetUse = nil }
        #expect(painter.unsupported != nil)
        #expect(!painter.canPaint)
        painter.setPainting(true)
        #expect(!painter.isPainting)
    }

    @Test func keysFinishCancelAndTakeBack() async {
        let (painter, _, _, _) = await placedPainter()
        painter.setPainting(true)
        painter.tool = .space
        click(painter, plan(0, 10))
        click(painter, plan(5, 10))
        #expect(painter.key(.delete))
        #expect(painter.drawing.count == 1)
        painter.undo()
        #expect(painter.drawing.isEmpty)
        click(painter, plan(0, 10))
        click(painter, plan(5, 10))
        // Two corners are not a space yet: Return keeps drawing.
        #expect(painter.key(.finish))
        #expect(painter.drawing.count == 2)
        #expect(painter.key(.cancel))
        #expect(painter.drawing.isEmpty)
        #expect(!painter.key(.cancel))
        // Nothing is taken while not painting.
        painter.setPainting(false)
        #expect(!painter.key(.delete))
    }

    @Test func theMetalViewMapsPaintKeys() {
        #expect(InteractiveMetalView.paintKey(36) == .finish)
        #expect(InteractiveMetalView.paintKey(76) == .finish)
        #expect(InteractiveMetalView.paintKey(53) == .cancel)
        #expect(InteractiveMetalView.paintKey(51) == .delete)
        #expect(InteractiveMetalView.paintKey(117) == .delete)
        #expect(InteractiveMetalView.paintKey(0) == nil)
    }
}
