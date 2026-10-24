//
//  CameraPaneTests.swift
//  VelocityVisualiserTests
//
//  The camera pane without a window: every speed curve's ends, shape and
//  overshoot, the Bézier against known values and the loop seam; the
//  ellipse's pose at given phases, headings and directions; the three ways
//  of measuring its radii and the ray a pick unprojects; free flight's ramp
//  up and down; the settings and viewpoints round trip through
//  UserDefaults; and the session's playback, and its exclusion of paint
//  mode, both ways round.
//

import Foundation
import Metal
import MetalKit
import Testing
import simd

@testable import VelocityVisualiser

// MARK: - Helpers

private func near(_ a: SIMD3<Float>, _ b: SIMD3<Float>, _ tol: Float = 1e-3) -> Bool {
    simd_length(a - b) < tol
}

private func near(_ a: Double, _ b: Double, _ tol: Double = 1e-9) -> Bool { abs(a - b) < tol }

/// A sink that holds the camera the way MetalRenderer does.
final class FakeCameraSink: CameraControlSink {
    var camera = Camera()
    var referenceModelMatrix = matrix_identity_float4x4
    var holder: CameraHolder?
    var saved: Camera?
    var strip: [Float]?
    var ticks: [Float] = []
    var focused = 0

    @discardableResult func holdCamera(_ holder: CameraHolder, _ next: Camera) -> Bool {
        if let current = self.holder, current != holder { return false }
        if self.holder == nil {
            saved = camera
            self.holder = holder
        }
        camera = next
        return true
    }

    func releaseCamera(_ holder: CameraHolder, restore: Bool) {
        guard self.holder == holder else { return }
        if restore, let saved { camera = saved }
        saved = nil
        self.holder = nil
    }

    func setCameraPathPreview(strip: [Float]?, ticks: [Float]) {
        self.strip = strip
        self.ticks = ticks
    }

    func focusView() { focused += 1 }
}

/// A fixed orbit to check poses against: A 40 m along, B 20 m across,
/// 10 m above a centre at (5, 5, 1), looking 2 m above it.
private func fixedOrbit() -> CameraPaneSettings {
    var s = CameraPaneSettings()
    s.curve = .linear
    s.orbit.radiusA = 40
    s.orbit.radiusB = 20
    s.orbit.headingDeg = 0
    s.orbit.startAngleDeg = 0
    s.orbit.heightMode = .height
    s.orbit.heightM = 10
    s.orbit.lookAtHeightM = 2
    s.orbit.lapSeconds = 10
    return s
}

private let centre = SIMD3<Float>(5, 5, 1)

// MARK: - Speed curves

struct CameraEasingTests {
    @Test(arguments: CameraEasing.Curve.allCases) func everyCurveStartsAtZeroAndEndsAtOne(
        _ curve: CameraEasing.Curve
    ) {
        #expect(near(CameraEasing.value(curve, 0), 0, 1e-12))
        #expect(near(CameraEasing.value(curve, 1), 1, 1e-12))
        // Clamped outside [0, 1].
        #expect(near(CameraEasing.value(curve, -2), 0, 1e-12))
        #expect(near(CameraEasing.value(curve, 3), 1, 1e-12))
    }

    @Test(arguments: [
        CameraEasing.Curve.linear, .easeIn, .easeOut, .easeInOut, .exponentialIn, .exponentialOut,
        .exponentialInOut, .sineInOut, .bounceOut, .bezier,
    ]) func theCurvesThatDoNotOvershootRiseThroughout(_ curve: CameraEasing.Curve) {
        let values = CameraEasing.samples(curve, count: 400)
        for (a, b) in zip(values, values.dropFirst()) {
            if curve == .bounceOut {
                // Bounce falls back between its bounces, but never below 0
                // or above 1.
                #expect(b >= 0 && b <= 1 + 1e-12)
            } else {
                #expect(b >= a - 1e-12)
            }
        }
    }

    @Test func eachCurveRisesWhereItShould() {
        let v = { (c: CameraEasing.Curve, t: Double) in CameraEasing.value(c, t) }
        #expect(near(v(.linear, 0.3), 0.3))
        // In: slow at first. Out: fast at first. In-out: both, and halfway
        // at half time.
        for curve in [CameraEasing.Curve.easeIn, .exponentialIn] {
            #expect(v(curve, 0.25) < 0.25)
            #expect(v(curve, 0.75) < 0.75)
        }
        for curve in [CameraEasing.Curve.easeOut, .exponentialOut] {
            #expect(v(curve, 0.25) > 0.25)
            #expect(v(curve, 0.75) > 0.75)
        }
        for curve in [CameraEasing.Curve.easeInOut, .exponentialInOut, .sineInOut, .backInOut] {
            #expect(v(curve, 0.25) < 0.25)
            #expect(near(v(curve, 0.5), 0.5, 1e-12))
            #expect(v(curve, 0.75) > 0.75)
            // Symmetric about the middle.
            #expect(near(v(curve, 0.2) + v(curve, 0.8), 1, 1e-12))
        }
        #expect(near(v(.easeIn, 0.5), 0.125, 1e-12))
        #expect(near(v(.easeOut, 0.5), 0.875, 1e-12))
        #expect(near(v(.sineInOut, 0.25), (1 - cos(Double.pi / 4)) / 2, 1e-12))
        // Bounce's first arc lands on 1 at t = 1/2.75.
        #expect(near(v(.bounceOut, 1 / 2.75), 1, 1e-9))
    }

    @Test func bounceElasticAndBackOvershootWithinBounds() {
        let bounce = CameraEasing.samples(.bounceOut, count: 2000)
        #expect(bounce.min()! >= 0)
        #expect(bounce.max()! <= 1 + 1e-12)
        let elastic = CameraEasing.samples(.elasticOut, count: 10_000)
        // It overshoots 1 by about 37% and never dips below its start.
        #expect(elastic.max()! > 1.3 && elastic.max()! < 1.4)
        #expect(elastic.min()! >= 0)
        let back = CameraEasing.samples(.backInOut, count: 10_000)
        #expect(back.min()! < -0.09 && back.min()! > -0.11)
        #expect(back.max()! > 1.09 && back.max()! < 1.11)
    }

    @Test func theBezierMatchesKnownValues() {
        // CSS's ease, worked to 1e-12 by bisection.
        let ease = CameraEasing.Bezier.ease
        #expect(near(CameraEasing.cubicBezier(0.25, ease), 0.408_510_591_355, 1e-6))
        #expect(near(CameraEasing.cubicBezier(0.5, ease), 0.802_403_387_585, 1e-6))
        #expect(near(CameraEasing.cubicBezier(0.75, ease), 0.960_458_978_349, 1e-6))
        // CSS's ease-in.
        let easeIn = CameraEasing.Bezier(0.42, 0, 1, 1)
        #expect(near(CameraEasing.cubicBezier(0.5, easeIn), 0.315_356_812_573, 1e-6))
        // CSS's ease-in-out is symmetric.
        let inOut = CameraEasing.Bezier(0.42, 0, 0.58, 1)
        #expect(near(CameraEasing.cubicBezier(0.5, inOut), 0.5, 1e-9))
        #expect(near(CameraEasing.cubicBezier(0.25, inOut), 0.129_161_931_047, 1e-6))
        // Control points on the diagonal draw the diagonal.
        let straight = CameraEasing.Bezier(0.3, 0.3, 0.7, 0.7)
        for t in stride(from: 0.0, through: 1, by: 0.1) {
            #expect(near(CameraEasing.cubicBezier(t, straight), t, 1e-9))
        }
        // Through value(): the curve's own Bézier.
        #expect(near(CameraEasing.value(.bezier, 0.5, bezier: inOut), 0.5, 1e-9))
    }

    @Test func onlyTheCurvesWhoseEndsMatchLoopSmoothly() {
        for curve in [
            CameraEasing.Curve.linear, .easeInOut, .exponentialInOut, .sineInOut, .backInOut,
        ] { #expect(CameraEasing.isSeamContinuous(curve), "\(curve)") }
        for curve in [CameraEasing.Curve.easeIn, .easeOut, .exponentialIn, .bounceOut, .elasticOut]
        { #expect(!CameraEasing.isSeamContinuous(curve), "\(curve)") }
        // A Bézier is judged by its own slopes: ease leaves at 0.4 and
        // arrives at 0; ease-in-out leaves and arrives at 0.
        #expect(!CameraEasing.isSeamContinuous(.bezier, bezier: .ease))
        #expect(
            CameraEasing.isSeamContinuous(.bezier, bezier: CameraEasing.Bezier(0.42, 0, 0.58, 1)))
    }

    @Test func theCurvesRoundTripByName() throws {
        for curve in CameraEasing.Curve.allCases {
            let data = try JSONEncoder().encode(curve)
            #expect(try JSONDecoder().decode(CameraEasing.Curve.self, from: data) == curve)
            #expect(!curve.label.isEmpty)
        }
    }
}

// MARK: - The ellipse

struct OrbitPathTests {
    @Test func axesFollowTheCompass() {
        let frame = OrbitFrame(centre: .zero)
        #expect(near(frame.axes(headingDeg: 0).a, SIMD3(0, 1, 0), 1e-6))
        #expect(near(frame.axes(headingDeg: 0).b, SIMD3(1, 0, 0), 1e-6))
        #expect(near(frame.axes(headingDeg: 90).a, SIMD3(1, 0, 0), 1e-6))
        #expect(near(frame.axes(headingDeg: 90).b, SIMD3(0, -1, 0), 1e-6))
        #expect(near(frame.heading(of: SIMD3(1, 1, 0)), 45, 1e-4))
        #expect(near(frame.heading(of: SIMD3(-1, 0, 0)), 270, 1e-4))
        // North tilted out of level is flattened onto the ground.
        let tilted = OrbitFrame(centre: .zero, north: SIMD3(0, 2, 1))
        #expect(near(tilted.north, SIMD3(0, 1, 0), 1e-6))
    }

    @Test func thePoseAtEachQuarterLapClockwise() {
        let s = fixedOrbit()
        let frame = OrbitFrame(centre: centre)
        let at = { (laps: Double) in OrbitPath.pose(laps: laps, settings: s, frame: frame) }
        // Phase 0 at A's end, due north; a quarter on, B's end, east.
        #expect(near(at(0).position, SIMD3(5, 45, 11)))
        #expect(near(at(0.25).position, SIMD3(25, 5, 11)))
        #expect(near(at(0.5).position, SIMD3(5, -35, 11)))
        #expect(near(at(0.75).position, SIMD3(-15, 5, 11)))
        #expect(near(at(1).position, at(0).position))
        #expect(near(at(2.25).position, at(0.25).position))
        for laps in [0.0, 0.1, 0.6] {
            #expect(near(at(laps).target, SIMD3(5, 5, 3)))
            #expect(at(laps).up == SIMD3(0, 0, 1))
        }
    }

    @Test func anticlockwiseGoesTheOtherWay() {
        var s = fixedOrbit()
        s.orbit.clockwise = false
        let frame = OrbitFrame(centre: centre)
        #expect(
            near(OrbitPath.pose(laps: 0.25, settings: s, frame: frame).position, SIMD3(-15, 5, 11)))
        #expect(
            near(OrbitPath.pose(laps: 0.75, settings: s, frame: frame).position, SIMD3(25, 5, 11)))
    }

    @Test func headingAndNorthTurnTheEllipse() {
        var s = fixedOrbit()
        s.orbit.headingDeg = 90
        let frame = OrbitFrame(centre: centre)
        #expect(near(OrbitPath.pose(laps: 0, settings: s, frame: frame).position, SIMD3(45, 5, 11)))
        #expect(
            near(OrbitPath.pose(laps: 0.25, settings: s, frame: frame).position, SIMD3(5, -15, 11)))
        // A sensor whose +x is the site's north: heading 0 lies along +x.
        s.orbit.headingDeg = 0
        let turned = OrbitFrame(centre: centre, north: SIMD3(1, 0, 0))
        #expect(
            near(OrbitPath.pose(laps: 0, settings: s, frame: turned).position, SIMD3(45, 5, 11)))
        // The start angle moves phase 0 round from A's end.
        s.orbit.startAngleDeg = 90
        #expect(near(OrbitPath.pose(laps: 0, settings: s, frame: frame).position, SIMD3(25, 5, 11)))
    }

    @Test func elevationRaisesTheCameraWhereTheEllipseIsWide() {
        var s = fixedOrbit()
        s.orbit.heightMode = .elevation
        s.orbit.elevationDeg = 45
        s.orbit.lookAtHeightM = 0
        let frame = OrbitFrame(centre: centre)
        #expect(near(OrbitPath.pose(laps: 0, settings: s, frame: frame).position, SIMD3(5, 45, 41)))
        #expect(
            near(OrbitPath.pose(laps: 0.25, settings: s, frame: frame).position, SIMD3(25, 5, 21)))
    }

    @Test func theCurveShapesProgressRoundTheLap() {
        var s = fixedOrbit()
        s.curve = .easeIn
        // Half the lap's time, an eighth of the way round.
        let theta = OrbitPath.angle(laps: 0.5, settings: s)
        #expect(near(theta, 2 * .pi * 0.125, 1e-12))
        // The seam: no jump in position from one lap into the next.
        let frame = OrbitFrame(centre: centre)
        let before = OrbitPath.pose(laps: 1 - 1e-6, settings: s, frame: frame).position
        let after = OrbitPath.pose(laps: 1 + 1e-6, settings: s, frame: frame).position
        #expect(near(before, after, 1e-2))
    }

    @Test func playbackAdvancesByLapTimeAndAOnceLapStops() {
        var s = fixedOrbit()
        var r = OrbitPath.advance(laps: 0, by: 2.5, settings: s)
        #expect(near(r.laps, 0.25) && !r.finished)
        r = OrbitPath.advance(laps: 0.9, by: 5, settings: s)
        #expect(near(r.laps, 1.4) && !r.finished)
        s.orbit.loop = false
        r = OrbitPath.advance(laps: 0.9, by: 5, settings: s)
        #expect(r.laps == 1 && r.finished)
    }

    @Test func thePreviewIsAClosedLoopThroughThePath() {
        let s = fixedOrbit()
        let frame = OrbitFrame(centre: centre)
        let v = OrbitPath.previewVertices(orbit: s.orbit, frame: frame, segments: 64)
        #expect(v.count == 65 * 7)
        let first = SIMD3(v[0], v[1], v[2])
        let last = SIMD3(v[64 * 7], v[64 * 7 + 1], v[64 * 7 + 2])
        #expect(near(first, last))
        #expect(near(first, SIMD3(5, 45, 11)))
        let marker = OrbitPath.startMarker(orbit: s.orbit, frame: frame)
        #expect(marker.count == 14)
        #expect(near(SIMD3(marker[7], marker[8], marker[9]), SIMD3(5, 45, 1)))
    }

    @Test func ratioLockDifferenceAndRatioEdits() {
        var o = OrbitSettings()
        o.radiusA = 40
        o.radiusB = 20
        o.setRadiusA(60)
        #expect(o.radiusA == 60 && o.radiusB == 20)
        o.lockRatio = true
        o.setRadiusA(30)
        #expect(near(o.radiusB, 10))
        o.setRadiusB(20)
        #expect(near(o.radiusA, 60))
        o.setDifference(15)
        #expect(near(o.radiusB, 45) && o.radiusA == 60)
        o.setRatio(4)
        #expect(near(o.radiusB, 15) && near(o.ratio, 4))
        o.setRatio(0)
        #expect(near(o.radiusB, 15))
        o.setDifference(1000)
        #expect(o.radiusB == OrbitSettings.minRadius)
    }
}

// MARK: - Measuring

struct OrbitMeasureTests {
    @Test(arguments: [
        SIMD3<Float>(30, -20, 15), SIMD3<Float>(5, 60, 1), SIMD3<Float>(-12, 7, -4),
        SIMD3<Float>(6, 5.5, 40),
    ]) func measureFromCameraPutsPhaseZeroOnTheCamera(_ position: SIMD3<Float>) {
        for heightMode in OrbitSettings.HeightMode.allCases {
            for heading in [0.0, 37.0, 200.0] {
                var s = fixedOrbit()
                s.orbit.heightMode = heightMode
                s.orbit.headingDeg = heading
                let frame = OrbitFrame(centre: centre, north: SIMD3(0.3, 1, 0))
                let measured = OrbitPath.measureFromCamera(
                    position: position, orbit: s.orbit, frame: frame)
                #expect(measured != nil)
                guard let measured else { continue }
                s.orbit = measured
                // The ratio is kept, and phase 0 is the camera exactly.
                #expect(near(measured.ratio, 2, 1e-9))
                let p = OrbitPath.pose(laps: 0, settings: s, frame: frame).position
                #expect(near(p, position, 1e-3), "\(heightMode) \(heading): \(p) vs \(position)")
            }
        }
    }

    @Test func measureFromCameraNeedsTheCameraOffTheCentre() {
        let frame = OrbitFrame(centre: centre)
        #expect(
            OrbitPath.measureFromCamera(
                position: centre + SIMD3(0, 0, 30), orbit: OrbitSettings(), frame: frame) == nil)
    }

    /// A 61 × 25 grid, ±30 m north and south and ±12 m east and west of the
    /// centre.
    private func grid() -> [SIMD3<Float>] {
        var points = [SIMD3<Float>]()
        for i in -30...30 {
            for j in -12...12 { points.append(centre + SIMD3(Float(j), Float(i), Float(i % 3))) }
        }
        return points
    }

    @Test func measureFromSceneFitsTheExtentWithTheMargin() throws {
        var orbit = OrbitSettings()
        orbit.marginPercent = 10
        let frame = OrbitFrame(centre: centre)
        let fit = try #require(
            OrbitPath.measureFromScene(points: grid(), orbit: orbit, frame: frame, percentile: 1))
        #expect(near(fit.a, 33, 1e-4))
        #expect(near(fit.b, 13.2, 1e-4))
        // Turned a quarter, the axes swap.
        orbit.headingDeg = 90
        let turned = try #require(
            OrbitPath.measureFromScene(points: grid(), orbit: orbit, frame: frame, percentile: 1))
        #expect(near(turned.a, 13.2, 1e-4))
        #expect(near(turned.b, 33, 1e-4))
        // No margin, no growth.
        orbit.headingDeg = 0
        orbit.marginPercent = 0
        let bare = try #require(
            OrbitPath.measureFromScene(points: grid(), orbit: orbit, frame: frame, percentile: 1))
        #expect(near(bare.a, 30, 1e-4) && near(bare.b, 12, 1e-4))
        #expect(OrbitPath.measureFromScene(points: [], orbit: orbit, frame: frame) == nil)
    }

    @Test func measureFromSceneIgnoresAFewStrays() throws {
        var orbit = OrbitSettings()
        orbit.marginPercent = 0
        let frame = OrbitFrame(centre: centre)
        let points = grid() + (0..<5).map { centre + SIMD3(900, 900 + Float($0), 0) }
        let fit = try #require(
            OrbitPath.measureFromScene(points: points, orbit: orbit, frame: frame, percentile: 0.99)
        )
        #expect(fit.a <= 30 + 1e-4 && fit.a > 28)
        #expect(fit.b <= 12 + 1e-4 && fit.b > 11)
    }

    @Test func scenePointsPreferTheScansElseBoxesAndKerbs() {
        let model = simd_float4x4(translation: SIMD3(100, 0, -5))
        var geometry = ReferenceGeometry()
        var box = Gerardus_Align_V1alpha1_LineFeature()
        box.xyz = [1, 2, 3, 4, 5, 6]
        var kerb = Gerardus_Align_V1alpha1_LineFeature()
        kerb.xyz = [7, 8, 9]
        geometry.boxes = [box]
        geometry.kerbs = [kerb]
        let lines = CameraControlSession.scenePoints(geometry, model: model, limit: 100)
        #expect(lines.source == "boxes and kerbs")
        #expect(lines.points == [SIMD3(101, 2, -2), SIMD3(104, 5, 1), SIMD3(107, 8, 4)])

        var scan = Gerardus_Align_V1alpha1_ScanPoints()
        scan.points.x = (0..<1000).map(Float.init)
        scan.points.y = Array(repeating: 1, count: 1000)
        scan.points.z = Array(repeating: 2, count: 1000)
        geometry.scans = [scan]
        let scans = CameraControlSession.scenePoints(geometry, model: model, limit: 100)
        #expect(scans.source == "registered scans")
        #expect(scans.points.count == 100)
        #expect(scans.points.first == SIMD3(100, 1, -3))
    }

    @Test func pickingSetsAsEndThenBs() throws {
        let frame = OrbitFrame(centre: .zero)
        let a = try #require(
            OrbitPath.pickA(SIMD3(10, 10, 0), orbit: OrbitSettings(), frame: frame))
        #expect(near(a.radiusA, 200.0.squareRoot(), 1e-5))
        #expect(near(a.headingDeg, 45, 1e-4))
        // B's axis is at 135°: south-east.
        let b = OrbitPath.pickB(SIMD3(-5, 5, 0), orbit: a, frame: frame)
        #expect(near(b.radiusB, 50.0.squareRoot(), 1e-5))
        #expect(b.radiusA == a.radiusA)
        #expect(OrbitPath.pickA(.zero, orbit: OrbitSettings(), frame: frame) == nil)
    }

    @Test func aClickUnprojectsOntoTheGroundItShows() throws {
        let size = CGSize(width: 800, height: 600)
        var perspective = Camera()
        perspective.aspectRatio = Float(size.width / size.height)
        var plan = Camera()
        plan.setOrthoView(.top, centre: SIMD3(3, 4, 0), halfHeight: 30)
        plan.aspectRatio = perspective.aspectRatio
        for camera in [perspective, plan] {
            for ground in [SIMD3<Float>(7, 4, 0), SIMD3<Float>(-12, 25, 0), SIMD3<Float>(0, 10, 0)]
            {
                let mvp: simd_float4x4 = camera.projectionMatrix * camera.viewMatrix
                let clip = mvp * SIMD4<Float>(ground.x, ground.y, ground.z, 1)
                let pixel = CGPoint(
                    x: CGFloat((clip.x / clip.w + 1) / 2) * size.width,
                    y: CGFloat((clip.y / clip.w + 1) / 2) * size.height)
                let hit = try #require(
                    CameraRay.ground(camera: camera, viewPoint: pixel, viewSize: size, planeZ: 0))
                #expect(near(hit, ground, 1e-2), "\(hit) vs \(ground)")
            }
        }
        // Looking level, the horizon's rays never meet a plane below.
        var level = Camera()
        level.position = SIMD3(0, 0, 10)
        level.target = SIMD3(0, 50, 10)
        level.aspectRatio = perspective.aspectRatio
        #expect(
            CameraRay.ground(
                camera: level, viewPoint: CGPoint(x: 400, y: 590), viewSize: size, planeZ: 0) == nil
        )
    }
}

// MARK: - Free flight

struct FlyDynamicsTests {
    private func fly(_ curve: CameraEasing.Curve = .linear) -> CameraPaneSettings {
        var s = CameraPaneSettings()
        s.curve = curve
        s.fly.maxSpeed = 10
        s.fly.rampSeconds = 1
        return s
    }

    /// Integrates `seconds` of flight in 10 ms steps, returning the motion
    /// and the distance flown.
    private func integrate(
        _ motion: FlyMotion, input: SIMD3<Float>, seconds: Double, settings: CameraPaneSettings
    ) -> (FlyMotion, Double) {
        var m = motion
        var distance = 0.0
        for _ in 0..<Int((seconds / 0.01).rounded()) {
            m = FlyDynamics.step(m, input: input, dt: 0.01, fly: settings.fly)
            distance += FlyDynamics.speed(m, settings: settings) * 0.01
        }
        return (m, distance)
    }

    @Test func speedRampsUpWhileHeldAndDownAfterRelease() {
        let s = fly()
        let forward = SIMD3<Float>(0, 1, 0)
        var (m, d) = integrate(FlyMotion(), input: forward, seconds: 0.5, settings: s)
        #expect(near(FlyDynamics.speed(m, settings: s), 5, 1e-6))
        (m, d) = integrate(FlyMotion(), input: forward, seconds: 1, settings: s)
        #expect(near(m.throttle, 1, 1e-9))
        #expect(near(FlyDynamics.speed(m, settings: s), 10, 1e-6))
        // Half of top speed times the ramp, to within a step.
        #expect(abs(d - 5) < 0.06)
        // Held longer, it stays at top speed.
        let (cruise, cruised) = integrate(m, input: forward, seconds: 2, settings: s)
        #expect(cruise.throttle == 1 && abs(cruised - 20) < 1e-6)
        // Released, it slows the same way, still heading forward.
        let (halfway, _) = integrate(m, input: .zero, seconds: 0.5, settings: s)
        #expect(near(FlyDynamics.speed(halfway, settings: s), 5, 1e-6))
        #expect(halfway.direction == forward)
        let (stopped, coasted) = integrate(m, input: .zero, seconds: 1.5, settings: s)
        #expect(stopped.throttle == 0 && !stopped.isMoving)
        #expect(abs(coasted - 5) < 0.06)
    }

    @Test func theCurveShapesTheRamp() {
        let s = fly(.easeIn)
        let (m, _) = integrate(FlyMotion(), input: SIMD3(1, 0, 0), seconds: 0.5, settings: s)
        #expect(near(FlyDynamics.speed(m, settings: s), 1.25, 1e-6))
        // Back dips below zero at first; flight does not reverse.
        let back = fly(.backInOut)
        let (early, _) = integrate(FlyMotion(), input: SIMD3(1, 0, 0), seconds: 0.1, settings: back)
        #expect(FlyDynamics.speed(early, settings: back) == 0)
    }

    @Test func theKeysAddUpToADirection() {
        #expect(FlyDynamics.input(held: [13, 1]) == .zero)
        #expect(FlyDynamics.input(held: [13, 2]) == SIMD3(1, 1, 0))
        #expect(FlyDynamics.input(held: [126, 14]) == SIMD3(0, 1, 1))
        #expect(FlyDynamics.input(held: [12]) == SIMD3(0, 0, -1))
        #expect(FlyDynamics.input(held: [99]) == .zero)
        let m = FlyDynamics.step(FlyMotion(), input: SIMD3(1, 1, 0), dt: 0.1, fly: FlySettings())
        #expect(near(Double(simd_length(m.direction)), 1, 1e-6))
    }

    @Test func movingFollowsTheViewAndLookingTurnsIt() {
        let pose = CameraPose(position: SIMD3(0, 0, 10), target: SIMD3(0, 10, 0))
        var motion = FlyMotion(throttle: 1, direction: SIMD3(0, 1, 0))
        let ahead = FlyDynamics.move(pose, motion: motion, speed: 2, dt: 1)
        let view = simd_normalize(pose.target - pose.position)
        #expect(near(ahead.position, pose.position + view * 2))
        #expect(near(ahead.target - ahead.position, pose.target - pose.position))
        motion.direction = SIMD3(0, 0, 1)
        #expect(
            near(FlyDynamics.move(pose, motion: motion, speed: 3, dt: 1).position, SIMD3(0, 0, 13)))
        motion.direction = SIMD3(1, 0, 0)
        #expect(
            near(FlyDynamics.move(pose, motion: motion, speed: 1, dt: 1).position, SIMD3(1, 0, 10)))

        let level = CameraPose(position: .zero, target: SIMD3(0, 10, 0))
        // A quarter turn left from north looks west.
        let left = FlyDynamics.look(level, yaw: .pi / 2, pitch: 0)
        #expect(near(simd_normalize(left.target - left.position), SIMD3(-1, 0, 0), 1e-5))
        #expect(near(Double(simd_length(left.target - left.position)), 10, 1e-4))
        // Pitch stops short of straight up.
        let up = FlyDynamics.look(level, yaw: 0, pitch: 3)
        let dir = simd_normalize(up.target - up.position)
        #expect(near(Double(asin(dir.z)) * 180 / .pi, 85, 1e-3))
    }
}

// MARK: - Persistence

struct CameraPanePersistenceTests {
    private func defaults() -> UserDefaults {
        let name = "CameraPaneTests.\(UUID().uuidString)"
        let d = UserDefaults(suiteName: name)!
        d.removePersistentDomain(forName: name)
        return d
    }

    private func edited() -> CameraPaneSettings {
        var s = CameraPaneSettings()
        s.mode = .fly
        s.curve = .bezier
        s.bezier = CameraEasing.Bezier(0.1, 0.7, 0.4, 1.2)
        s.orbit.centre = .siteOrigin
        s.orbit.radiusA = 72.5
        s.orbit.radiusB = 31
        s.orbit.lockRatio = true
        s.orbit.headingDeg = 123
        s.orbit.heightMode = .height
        s.orbit.clockwise = false
        s.orbit.loop = false
        s.orbit.showPreview = false
        s.fly.maxSpeed = 22
        s.fly.rampSeconds = 1.7
        return s
    }

    @Test func settingsRoundTripPerSiteWithAGlobalDefault() {
        let store = CameraPaneStore(defaults: defaults())
        #expect(store.settings(site: nil) == CameraPaneSettings())
        let mine = edited()
        store.save(mine, site: "808580f47")
        #expect(store.settings(site: "808580f47") == mine)
        #expect(store.hasSiteSettings("808580f47"))
        // Another site, and no site, have the built-in default until the
        // global one is set; then they start from it.
        #expect(store.settings(site: "80858a1") == CameraPaneSettings())
        var global = CameraPaneSettings()
        global.orbit.lapSeconds = 90
        store.saveDefault(global)
        #expect(store.settings(site: "80858a1") == global)
        #expect(store.settings(site: nil) == global)
        #expect(store.settings(site: "808580f47") == mine)
        #expect(!store.hasSiteSettings("80858a1"))
        // Without UserDefaults nothing is kept.
        let none = CameraPaneStore(defaults: nil)
        none.save(mine, site: nil)
        #expect(none.settings(site: nil) == CameraPaneSettings())
    }

    @Test func viewpointsRoundTripAndStopAtEight() {
        let store = CameraPaneStore(defaults: defaults())
        let views = (0..<10).map {
            SavedViewpoint(
                name: "View \($0)",
                pose: CameraPose(position: SIMD3(Float($0), 2, 3), target: SIMD3(0, 0, 1), fov: 50))
        }
        store.save(viewpoints: views, site: "808580f47")
        let read = store.viewpoints(site: "808580f47")
        #expect(read == Array(views.prefix(8)))
        #expect(store.viewpoints(site: nil).isEmpty)
    }

    @MainActor @Test func theSessionKeepsWhatItIsGivenAcrossLaunches() {
        let d = defaults()
        let sink = FakeCameraSink()
        let first = CameraControlSession(defaults: d)
        first.sink = sink
        first.settings.orbit.radiusA = 55
        first.settings.curve = .sineInOut
        first.saveViewpoint(named: "Corner")
        let second = CameraControlSession(defaults: d)
        #expect(second.settings.orbit.radiusA == 55)
        #expect(second.settings.curve == .sineInOut)
        #expect(second.viewpoints.map(\.name) == ["Corner"])
    }
}

// MARK: - The session

@MainActor struct CameraControlSessionTests {
    private func session(
        _ settings: CameraPaneSettings = fixedOrbit()
    ) -> (CameraControlSession, FakeCameraSink) {
        let s = CameraControlSession(defaults: nil)
        let sink = FakeCameraSink()
        sink.camera.position = SIMD3(5, -30, 20)
        sink.camera.target = centre
        s.sink = sink
        s.clock = { 0 }
        s.settings = settings
        s.isPaneOpen = true
        return (s, sink)
    }

    @Test func playPauseAndStopPutTheCameraBack() {
        let (s, sink) = session()
        let before = sink.camera
        s.play()
        #expect(s.playback == .playing)
        #expect(sink.holder == .orbit)
        #expect(near(sink.camera.position, SIMD3(5, 45, 11)))
        s.advance(by: 2.5, now: 0)
        #expect(near(sink.camera.position, SIMD3(25, 5, 11)))
        // The operator's hand pauses it.
        s.manualInput()
        #expect(s.playback == .paused)
        s.advance(by: 2.5, now: 0)
        #expect(near(sink.camera.position, SIMD3(25, 5, 11)))
        // Play resumes from there, round the centre fixed at Play.
        sink.camera.target = SIMD3(100, 100, 0)
        s.play()
        s.advance(by: 2.5, now: 0)
        #expect(near(sink.camera.position, SIMD3(5, -35, 11)))
        s.stop()
        #expect(s.playback == .stopped && sink.holder == nil)
        #expect(sink.camera == before)
    }

    @Test func aSingleLapEndsWhereItStartedAndStopStillPutsBack() {
        var settings = fixedOrbit()
        settings.orbit.loop = false
        let (s, sink) = session(settings)
        let before = sink.camera
        s.play()
        s.advance(by: 12, now: 0)
        #expect(s.playback == .finished)
        #expect(near(sink.camera.position, SIMD3(5, 45, 11)))
        s.stop()
        #expect(sink.camera == before)
    }

    @Test func thePreviewIsDrawnOnlyWhileTheOrbitPaneShowsIt() {
        let (s, sink) = session()
        #expect(sink.strip?.count == 129 * 7)
        #expect(sink.ticks.count == 14)
        s.settings.orbit.showPreview = false
        #expect(sink.strip == nil)
        s.settings.orbit.showPreview = true
        s.settings.mode = .fly
        #expect(sink.strip == nil)
        s.settings.mode = .orbit
        #expect(sink.strip != nil)
        s.isPaneOpen = false
        #expect(sink.strip == nil)
    }

    @Test func measuringShowsWhatWasMeasured() {
        let (s, sink) = session()
        sink.camera.position = SIMD3(5, 65, 20)
        s.measureFromCamera()
        #expect(near(s.settings.orbit.radiusA, 60, 1e-4))
        #expect(near(s.measured?.a ?? 0, 60, 1e-4))
        #expect(s.measured?.source == "camera")
        s.setRadiusA(70)
        #expect(s.settings.orbit.radiusA == 70)
        #expect(near(s.measured?.a ?? 0, 60, 1e-4))
    }

    @Test func twoClicksSetTheRadii() {
        let (s, sink) = session()
        let size = CGSize(width: 800, height: 600)
        sink.camera.aspectRatio = Float(size.width / size.height)
        func pixel(_ p: SIMD3<Float>) -> CGPoint {
            let mvp: simd_float4x4 = sink.camera.projectionMatrix * sink.camera.viewMatrix
            let clip = mvp * SIMD4<Float>(p.x, p.y, p.z, 1)
            return CGPoint(
                x: CGFloat((clip.x / clip.w + 1) / 2) * size.width,
                y: CGFloat((clip.y / clip.w + 1) / 2) * size.height)
        }
        s.startPicking()
        #expect(s.picking == .a)
        #expect(s.pick(at: pixel(centre + SIMD3(0, 30, 0)), viewSize: size))
        #expect(s.picking == .b)
        #expect(s.pick(at: pixel(centre + SIMD3(-12, 3, 0)), viewSize: size))
        #expect(s.picking == nil)
        #expect(near(s.settings.orbit.radiusA, 30, 0.05))
        #expect(near(s.settings.orbit.headingDeg, 0, 0.1))
        #expect(near(s.settings.orbit.radiusB, 12, 0.05))
        #expect(s.measured?.source == "picked points")
        // Not picking, a click is not the pane's.
        #expect(!s.pick(at: .zero, viewSize: size))
    }

    @Test func flightHoldsTheCameraAndLeavesItWhereItFlew() {
        var settings = fixedOrbit()
        settings.mode = .fly
        settings.fly.maxSpeed = 10
        settings.fly.rampSeconds = 0.5
        let (s, sink) = session(settings)
        #expect(!s.flyKeyDown(13))
        s.setFlying(true)
        #expect(s.isFlying && sink.holder == .fly && sink.focused == 1)
        let start = sink.camera.position
        #expect(s.flyKeyDown(13))
        for _ in 0..<50 { s.advance(by: 0.01, now: 0) }
        #expect(near(s.flySpeed, 10, 1e-6))
        #expect(s.flyKeyUp(13))
        for _ in 0..<50 { s.advance(by: 0.01, now: 0) }
        #expect(s.flySpeed == 0)
        let flown = simd_length(sink.camera.position - start)
        #expect(abs(flown - 5) < 0.2)
        // Keys outside flight's are not its.
        #expect(!s.flyKeyDown(15))
        s.setFlying(false)
        #expect(sink.holder == nil)
        #expect(simd_length(sink.camera.position - start) > 4)
    }

    @Test func flyToFollowsTheCurveToTheViewpoint() {
        var settings = fixedOrbit()
        settings.mode = .fly
        settings.curve = .easeIn
        settings.fly.flyToSeconds = 2
        let (s, sink) = session(settings)
        let from = sink.camera.position
        let goal = SavedViewpoint(
            name: "Goal", pose: CameraPose(position: SIMD3(50, 50, 30), target: SIMD3(0, 0, 0)))
        s.fly(to: goal)
        #expect(s.flyingTo == goal && sink.holder == .fly)
        s.advance(by: 0, now: 1)
        // Half the time, an eighth of the way.
        #expect(near(sink.camera.position, from + (goal.pose.position - from) * 0.125, 1e-3))
        s.advance(by: 0, now: 2)
        #expect(s.flyingTo == nil && sink.holder == nil)
        #expect(near(sink.camera.position, goal.pose.position))
        // The operator's hand stops one under way.
        sink.camera.position = from
        s.fly(to: goal)
        s.manualInput()
        #expect(s.flyingTo == nil && sink.holder == nil)
    }

    @Test func aLoopingCurveThatJumpsIsWarnedOf() {
        var settings = fixedOrbit()
        settings.curve = .bounceOut
        let (s, _) = session(settings)
        #expect(s.seamWarning != nil)
        s.settings.orbit.loop = false
        #expect(s.seamWarning == nil)
        s.settings.orbit.loop = true
        s.settings.curve = .easeInOut
        #expect(s.seamWarning == nil)
    }

    @Test func closingThePaneStopsTheOrbit() {
        let (s, sink) = session()
        let before = sink.camera
        s.play()
        s.advance(by: 1, now: 0)
        s.isPaneOpen = false
        #expect(s.playback == .stopped && sink.camera == before)
    }
}

// MARK: - Mutual exclusion with paint mode

private let columbus = AlignmentSite(
    clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
    description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
    sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

@MainActor struct CameraPaintExclusionTests {
    private func renderer() -> MetalRenderer? {
        guard let device = MTLCreateSystemDefaultDevice() else { return nil }
        let view = MTKView()
        view.device = device
        return MetalRenderer(metalView: view)
    }

    @Test func theRendererHasOneHolderAtATime() {
        guard let renderer = renderer() else { return }
        renderer.camera.position = SIMD3(5, -40, 25)
        let before = renderer.camera
        var orbiting = before
        orbiting.position = SIMD3(40, 0, 10)
        #expect(renderer.holdCamera(.orbit, orbiting))
        #expect(renderer.cameraHolder == .orbit && !renderer.isPlanLocked)
        // The plan view is refused while the orbit holds the camera.
        let plan = PlanView(centre: PlanPoint(0, 0), groundZ: 0)
        renderer.setPlanCamera(
            plan.camera(
                pose: SitePose(translation: .zero, rotation: simd_quatd(ix: 0, iy: 0, iz: 0, r: 1)),
                aspectRatio: 1))
        #expect(renderer.camera == orbiting && !renderer.isPlanLocked)
        // Hand input still moves an orbit's camera (the pane pauses first).
        renderer.handleZoom(delta: 1)
        #expect(renderer.camera != orbiting)
        renderer.releaseCamera(.orbit, restore: true)
        #expect(renderer.camera == before && renderer.cameraHolder == nil)

        renderer.setPlanCamera(
            plan.camera(
                pose: SitePose(translation: .zero, rotation: simd_quatd(ix: 0, iy: 0, iz: 0, r: 1)),
                aspectRatio: 1))
        #expect(renderer.isPlanLocked)
        let locked = renderer.camera
        #expect(!renderer.holdCamera(.orbit, orbiting))
        #expect(!renderer.holdCamera(.fly, orbiting))
        #expect(renderer.camera == locked)
        // Releasing for a holder that does not hold it does nothing.
        renderer.releaseCamera(.orbit, restore: true)
        #expect(renderer.isPlanLocked)
        renderer.exitPlanView()
        #expect(renderer.camera == before)
        // Flight lets go where it flew to.
        #expect(renderer.holdCamera(.fly, orbiting))
        renderer.releaseCamera(.fly, restore: false)
        #expect(renderer.camera.position == orbiting.position && renderer.cameraHolder == nil)
    }

    @Test func paintModeAndTheCameraPaneExcludeEachOther() async {
        let service = FakeAlignmentService()
        service.availableLayers += [.boxes, .cityStreets, .cityIntersections, .kerbs, .streetUse]
        let alignment = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        let painter = StreetPainterSession(alignment: alignment)
        let paintSink = FakeStreetSink()
        painter.sink = paintSink
        await alignment.probe()
        alignment.choose(columbus)
        alignment.northAzimuthText = "35"
        await alignment.placeFromHint()
        await painter.pendingLoad?.value
        #expect(painter.canPaint)

        let camera = CameraControlSession(alignment: alignment, painter: painter, defaults: nil)
        let sink = FakeCameraSink()
        camera.sink = sink
        camera.clock = { 0 }
        camera.isPaneOpen = true
        // Settings are this site's.
        #expect(camera.siteKey == "808580f47")

        // Orbiting: paint mode is refused, with the reason.
        camera.play()
        #expect(camera.playback == .playing)
        #expect(painter.paintBlocker != nil && !painter.canPaint)
        painter.setPainting(true)
        #expect(!painter.isPainting)
        camera.stop()
        #expect(painter.canPaint)

        // Flying: likewise.
        camera.settings.mode = .fly
        camera.setFlying(true)
        #expect(camera.isFlying && !painter.canPaint)
        camera.setFlying(false)
        #expect(painter.canPaint)

        // Painting: Play, flight and Fly to are refused.
        painter.setPainting(true)
        #expect(painter.isPainting)
        #expect(camera.blocker != nil)
        camera.setFlying(true)
        #expect(!camera.isFlying)
        camera.fly(to: SavedViewpoint(name: "x", pose: CameraPose(sink.camera)))
        #expect(camera.flyingTo == nil)
        camera.settings.mode = .orbit
        camera.play()
        #expect(camera.playback == .stopped && sink.holder == nil)
        painter.setPainting(false)
        camera.play()
        #expect(camera.playback == .playing)
        camera.stop()
    }
}
