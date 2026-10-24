// CameraPaths.swift
// The camera pane's geometry, as pure functions the tests can reach without
// a window: the orbit's ellipse and the pose at each phase of it, the three
// ways of measuring its radii, free flight's speed ramp, the ray through a
// click, and the settings and viewpoints the pane keeps per site.
//
// Frames. Everything here is in the frame the renderer draws in: the
// sensor's, z up. North is the site's north carried into that frame when a
// sensor is placed, else +y. Headings are compass degrees, clockwise from
// north seen from above, so heading 90 points east.
//
// The ellipse. Radius A lies along the heading, radius B across it, at
// heading + 90. At angle θ the camera is
//
//     centre + A cos θ · â + B sin θ · b̂   (+ its height)
//
// and θ = start + s · 2π · (laps + f(fraction of this lap)), where f is the
// speed curve and s is +1 clockwise, -1 anticlockwise. Phase 0 (θ = start)
// is where Play begins.

import CoreGraphics
import Foundation
import simd

// MARK: - Poses and settings

/// Where a camera is and what it looks at: what a saved viewpoint keeps.
struct CameraPose: Codable, Equatable, Sendable {
    var position: SIMD3<Float>
    var target: SIMD3<Float>
    var up: SIMD3<Float> = SIMD3(0, 0, 1)
    var fov: Float = 60

    init(
        position: SIMD3<Float>, target: SIMD3<Float>, up: SIMD3<Float> = SIMD3(0, 0, 1),
        fov: Float = 60
    ) {
        self.position = position
        self.target = target
        self.up = up
        self.fov = fov
    }

    init(_ camera: Camera) {
        self.init(position: camera.position, target: camera.target, up: camera.up, fov: camera.fov)
    }

    /// `base` moved to this pose, perspective, at base's aspect and planes.
    func applied(to base: Camera) -> Camera {
        var c = base
        c.position = position
        c.target = target
        c.up = up
        c.fov = fov
        c.projection = .perspective
        return c
    }

    /// The straight line from a to b, at s (which may overshoot either end).
    static func mix(_ a: CameraPose, _ b: CameraPose, _ s: Double) -> CameraPose {
        let f = Float(s)
        var up = a.up + (b.up - a.up) * f
        up = simd_length(up) > 1e-4 ? simd_normalize(up) : b.up
        return CameraPose(
            position: a.position + (b.position - a.position) * f,
            target: a.target + (b.target - a.target) * f, up: up, fov: a.fov + (b.fov - a.fov) * f)
    }
}

/// A named camera pose the operator saved.
struct SavedViewpoint: Codable, Equatable, Identifiable, Sendable {
    var id = UUID()
    var name: String
    var pose: CameraPose
}

struct OrbitSettings: Codable, Equatable, Sendable {
    enum Centre: String, Codable, CaseIterable, Identifiable, Sendable {
        case cameraTarget
        case selectedObject
        case siteOrigin

        var id: String { rawValue }
        var label: String {
            switch self {
            case .cameraTarget: return "Camera target"
            case .selectedObject: return "Selected object"
            case .siteOrigin: return "Site origin"
            }
        }
    }

    enum HeightMode: String, Codable, CaseIterable, Identifiable, Sendable {
        /// A fixed height above the centre.
        case height
        /// A fixed angle up from the look-at point, so the camera rises
        /// where the ellipse is wide.
        case elevation

        var id: String { rawValue }
    }

    static let minRadius = 0.5

    var centre: Centre = .cameraTarget
    /// Along the heading, metres.
    var radiusA = 40.0
    /// Across the heading, metres.
    var radiusB = 25.0
    /// Editing one radius scales the other with it.
    var lockRatio = false
    var headingDeg = 0.0
    /// Where on the ellipse phase 0 is, degrees from A's end towards B's.
    var startAngleDeg = 0.0
    var heightMode: HeightMode = .elevation
    var heightM = 20.0
    var elevationDeg = 30.0
    var lookAtHeightM = 0.0
    var lapSeconds = 30.0
    var clockwise = true
    /// Lap after lap, or once round and stop.
    var loop = true
    var showPreview = true
    /// What Measure from scene adds beyond the scene's extent.
    var marginPercent = 10.0

    var difference: Double { radiusA - radiusB }
    var ratio: Double { radiusB > 0 ? radiusA / radiusB : 0 }

    /// Sets A; with the ratio locked, B follows.
    mutating func setRadiusA(_ a: Double) {
        let a = max(a, Self.minRadius)
        if lockRatio, radiusA > 0 { radiusB = max(radiusB * a / radiusA, Self.minRadius) }
        radiusA = a
    }

    /// Sets B; with the ratio locked, A follows.
    mutating func setRadiusB(_ b: Double) {
        let b = max(b, Self.minRadius)
        if lockRatio, radiusB > 0 { radiusA = max(radiusA * b / radiusB, Self.minRadius) }
        radiusB = b
    }

    /// Keeps A and moves B so that A − B is `d`.
    mutating func setDifference(_ d: Double) { radiusB = max(radiusA - d, Self.minRadius) }

    /// Keeps A and moves B so that A / B is `r`.
    mutating func setRatio(_ r: Double) {
        guard r > 0, r.isFinite else { return }
        radiusB = max(radiusA / r, Self.minRadius)
    }
}

struct FlySettings: Codable, Equatable, Sendable {
    /// Metres a second at full throttle.
    var maxSpeed = 10.0
    /// Seconds from standing to full speed after a key goes down, and from
    /// full speed to standing after it comes up.
    var rampSeconds = 0.8
    /// Radians of turn per point of mouse movement while looking.
    var lookSensitivity = 0.004
    /// How long Fly to takes.
    var flyToSeconds = 3.0
}

/// Everything the camera pane keeps for a site.
struct CameraPaneSettings: Codable, Equatable, Sendable {
    enum Mode: String, Codable, CaseIterable, Identifiable, Sendable {
        case orbit
        case fly
        var id: String { rawValue }
    }

    var mode: Mode = .orbit
    /// One curve for both modes.
    var curve: CameraEasing.Curve = .linear
    var bezier: CameraEasing.Bezier = .ease
    var orbit = OrbitSettings()
    var fly = FlySettings()

    func ease(_ t: Double) -> Double { CameraEasing.value(curve, t, bezier: bezier) }
}

// MARK: - The orbit

/// The orbit's frame: its centre and which way is north.
struct OrbitFrame: Equatable, Sendable {
    var centre: SIMD3<Float>
    /// Horizontal and unit; `init` makes it so.
    var north: SIMD3<Float>
    let up = SIMD3<Float>(0, 0, 1)

    init(centre: SIMD3<Float>, north: SIMD3<Float> = SIMD3(0, 1, 0)) {
        self.centre = centre
        var n = SIMD3<Float>(north.x, north.y, 0)
        if simd_length(n) < 1e-6 { n = SIMD3(0, 1, 0) }
        self.north = simd_normalize(n)
    }

    var east: SIMD3<Float> { simd_cross(north, up) }

    /// The unit horizontal direction at a compass heading.
    func direction(headingDeg: Double) -> SIMD3<Float> {
        let h = Float(headingDeg * .pi / 180)
        return north * cos(h) + east * sin(h)
    }

    /// The compass heading of a horizontal offset, in [0, 360).
    func heading(of offset: SIMD3<Float>) -> Double {
        let deg = Double(atan2(simd_dot(offset, east), simd_dot(offset, north))) * 180 / .pi
        return deg < 0 ? deg + 360 : deg
    }

    /// A's axis and B's.
    func axes(headingDeg: Double) -> (a: SIMD3<Float>, b: SIMD3<Float>) {
        (direction(headingDeg: headingDeg), direction(headingDeg: headingDeg + 90))
    }

    func horizontal(_ v: SIMD3<Float>) -> SIMD3<Float> { v - up * simd_dot(v, up) }
}

enum OrbitPath {
    /// The angle round the ellipse after `laps` laps of playback (whole
    /// laps plus a fraction), in radians from A's end towards B's.
    static func angle(laps: Double, settings: CameraPaneSettings) -> Double {
        let laps = max(laps, 0)
        let whole = floor(laps)
        // f(0) = 0 and f(1) = 1, so a lap's end and the next one's start
        // are the same angle: the seam has no jump in position.
        let fraction = laps - whole
        let sign = settings.orbit.clockwise ? 1.0 : -1.0
        return settings.orbit.startAngleDeg * .pi / 180 + sign * 2 * .pi
            * (whole + settings.ease(fraction))
    }

    /// The camera on the ellipse at angle θ, looking at the look-at point.
    static func pose(angle theta: Double, orbit: OrbitSettings, frame: OrbitFrame) -> CameraPose {
        let axes = frame.axes(headingDeg: orbit.headingDeg)
        let offset =
            axes.a * Float(orbit.radiusA * cos(theta)) + axes.b * Float(orbit.radiusB * sin(theta))
        let target = frame.centre + frame.up * Float(orbit.lookAtHeightM)
        let height: Float
        switch orbit.heightMode {
        case .height: height = Float(orbit.heightM)
        case .elevation:
            let e = min(max(orbit.elevationDeg, -89), 89) * .pi / 180
            height = Float(orbit.lookAtHeightM) + simd_length(offset) * Float(tan(e))
        }
        return CameraPose(
            position: frame.centre + offset + frame.up * height, target: target, up: frame.up)
    }

    static func pose(laps: Double, settings: CameraPaneSettings, frame: OrbitFrame) -> CameraPose {
        pose(angle: angle(laps: laps, settings: settings), orbit: settings.orbit, frame: frame)
    }

    /// Laps played after `elapsed` more seconds from `laps`, and whether a
    /// single lap has come to its end.
    static func advance(
        laps: Double, by elapsed: Double, settings: CameraPaneSettings
    ) -> (laps: Double, finished: Bool) {
        let lap = max(settings.orbit.lapSeconds, 0.1)
        let next = laps + max(elapsed, 0) / lap
        if !settings.orbit.loop, next >= 1 { return (1, true) }
        return (next, false)
    }

    /// The path preview: a closed line strip round the ellipse, at the
    /// camera's own heights, in the line pipeline's layout (x, y, z, r, g,
    /// b, a per vertex).
    static func previewVertices(
        orbit: OrbitSettings, frame: OrbitFrame, segments: Int = 128,
        colour: SIMD4<Float> = SIMD4(1, 0.78, 0.2, 0.85)
    ) -> [Float] {
        var vertices = [Float]()
        vertices.reserveCapacity((segments + 3) * 7 * 2)
        for i in 0...segments {
            let theta = Double(i) / Double(segments) * 2 * .pi
            let p = pose(angle: theta, orbit: orbit, frame: frame).position
            vertices += [p.x, p.y, p.z, colour.x, colour.y, colour.z, colour.w]
        }
        return vertices
    }

    /// Where phase 0 sits on the preview: a short tick from the start point
    /// down to the centre's height, as two line vertices.
    static func startMarker(orbit: OrbitSettings, frame: OrbitFrame) -> [Float] {
        let p = pose(angle: orbit.startAngleDeg * .pi / 180, orbit: orbit, frame: frame).position
        let foot = SIMD3(p.x, p.y, frame.centre.z)
        return [p.x, p.y, p.z, 1, 1, 1, 0.9, foot.x, foot.y, foot.z, 1, 1, 1, 0.5]
    }

    // MARK: Measuring

    /// Radii, start angle and height that put phase 0 exactly where the
    /// camera is. The camera's horizontal offset from the centre is
    /// projected onto A's and B's axes, (pa, pb); the ellipse through that
    /// point keeping the current A / B ratio k has A = √(pa² + k² pb²),
    /// B = A / k, and the camera at θ = atan2(pb / B, pa / A). The height
    /// (or elevation) is the camera's too. Nil when the camera stands on
    /// the centre.
    static func measureFromCamera(
        position: SIMD3<Float>, orbit: OrbitSettings, frame: OrbitFrame
    ) -> OrbitSettings? {
        let offset = frame.horizontal(position - frame.centre)
        guard simd_length(offset) > 1e-3 else { return nil }
        let axes = frame.axes(headingDeg: orbit.headingDeg)
        let pa = Double(simd_dot(offset, axes.a))
        let pb = Double(simd_dot(offset, axes.b))
        let k = orbit.radiusB > 0 ? orbit.radiusA / orbit.radiusB : 1
        var out = orbit
        out.radiusA = (pa * pa + k * k * pb * pb).squareRoot()
        out.radiusB = out.radiusA / k
        out.startAngleDeg = atan2(pb / out.radiusB, pa / out.radiusA) * 180 / .pi
        let rise = Double(simd_dot(position - frame.centre, frame.up))
        switch orbit.heightMode {
        case .height: out.heightM = rise
        case .elevation:
            out.elevationDeg =
                atan2(rise - orbit.lookAtHeightM, Double(simd_length(offset))) * 180 / .pi
        }
        return out
    }

    /// Radii fitted to the extent of `points` round the centre: along each
    /// axis, the `percentile` of the points' distances from the centre
    /// (1 takes the farthest; less ignores strays), plus the margin. Nil
    /// with no points.
    static func measureFromScene(
        points: [SIMD3<Float>], orbit: OrbitSettings, frame: OrbitFrame, percentile: Double = 0.99
    ) -> (a: Double, b: Double)? {
        guard !points.isEmpty else { return nil }
        let axes = frame.axes(headingDeg: orbit.headingDeg)
        var along = [Float]()
        var across = [Float]()
        along.reserveCapacity(points.count)
        across.reserveCapacity(points.count)
        for p in points {
            let d = p - frame.centre
            along.append(abs(simd_dot(d, axes.a)))
            across.append(abs(simd_dot(d, axes.b)))
        }
        func rank(_ values: [Float]) -> Double {
            let sorted = values.sorted()
            let q = min(max(percentile, 0), 1)
            let i = Int((Double(sorted.count - 1) * q).rounded())
            return Double(sorted[i])
        }
        let grow = 1 + max(orbit.marginPercent, 0) / 100
        return (
            max(rank(along) * grow, OrbitSettings.minRadius),
            max(rank(across) * grow, OrbitSettings.minRadius)
        )
    }

    /// The first picked point is A's end: A's length and the heading.
    static func pickA(
        _ point: SIMD3<Float>, orbit: OrbitSettings, frame: OrbitFrame
    ) -> OrbitSettings? {
        let d = frame.horizontal(point - frame.centre)
        guard simd_length(d) > 1e-3 else { return nil }
        var out = orbit
        out.radiusA = max(Double(simd_length(d)), OrbitSettings.minRadius)
        out.headingDeg = frame.heading(of: d)
        return out
    }

    /// The second is B's end: its distance across A's axis.
    static func pickB(
        _ point: SIMD3<Float>, orbit: OrbitSettings, frame: OrbitFrame
    ) -> OrbitSettings {
        let b = frame.axes(headingDeg: orbit.headingDeg).b
        var out = orbit
        out.radiusB = max(Double(abs(simd_dot(point - frame.centre, b))), OrbitSettings.minRadius)
        return out
    }
}

// MARK: - Free flight

/// Free flight's state between ticks: how far along its ramp the speed is,
/// and which way the camera is moving, in the camera's own terms (x right,
/// y forward, z up).
struct FlyMotion: Equatable, Sendable {
    var throttle = 0.0
    var direction = SIMD3<Float>.zero

    var isMoving: Bool { throttle > 0 && direction != .zero }
}

enum FlyDynamics {
    /// The keys free flight listens to, by key code, and the direction each
    /// pushes in camera terms. Arrows move like WASD.
    static let keys: [UInt16: SIMD3<Float>] = [
        13: SIMD3(0, 1, 0), 126: SIMD3(0, 1, 0),  // W, ↑: forward
        1: SIMD3(0, -1, 0), 125: SIMD3(0, -1, 0),  // S, ↓: back
        0: SIMD3(-1, 0, 0), 123: SIMD3(-1, 0, 0),  // A, ←: left
        2: SIMD3(1, 0, 0), 124: SIMD3(1, 0, 0),  // D, →: right
        12: SIMD3(0, 0, -1),  // Q: down
        14: SIMD3(0, 0, 1),  // E: up
    ]

    /// The direction the held keys add up to; zero when they cancel.
    static func input(held: Set<UInt16>) -> SIMD3<Float> {
        held.reduce(SIMD3<Float>.zero) { $0 + (keys[$1] ?? .zero) }
    }

    /// One tick: while a key is held the throttle climbs its ramp, and the
    /// motion takes the keys' direction; once none is, it falls back down
    /// the ramp along the last direction.
    static func step(
        _ motion: FlyMotion, input: SIMD3<Float>, dt: Double, fly: FlySettings
    ) -> FlyMotion {
        var next = motion
        let rate = dt / max(fly.rampSeconds, 0.01)
        if simd_length(input) > 1e-6 {
            next.direction = simd_normalize(input)
            next.throttle = min(motion.throttle + rate, 1)
        } else {
            next.throttle = max(motion.throttle - rate, 0)
        }
        return next
    }

    /// Metres a second: the curve of the throttle, times the top speed.
    /// The curves that dip below zero do not fly backwards.
    static func speed(_ motion: FlyMotion, settings: CameraPaneSettings) -> Double {
        max(settings.ease(motion.throttle), 0) * settings.fly.maxSpeed
    }

    /// The camera moved for `dt` seconds at the motion's speed: forward
    /// along the view, sideways level, up and down in the world.
    static func move(_ pose: CameraPose, motion: FlyMotion, speed: Double, dt: Double) -> CameraPose
    {
        guard motion.direction != .zero, speed > 0 else { return pose }
        let up = SIMD3<Float>(0, 0, 1)
        var forward = pose.target - pose.position
        forward = simd_length(forward) > 1e-6 ? simd_normalize(forward) : SIMD3(0, 1, 0)
        var right = simd_cross(forward, up)
        right = simd_length(right) > 1e-6 ? simd_normalize(right) : SIMD3(1, 0, 0)
        let d = motion.direction
        let step = (right * d.x + forward * d.y + up * d.z) * Float(speed * dt)
        var out = pose
        out.position += step
        out.target += step
        return out
    }

    /// Turns the view about the eye: yaw round the world's up, pitch held
    /// short of straight up or down. Radians; positive yaw turns left.
    static func look(_ pose: CameraPose, yaw: Double, pitch: Double) -> CameraPose {
        let offset = pose.target - pose.position
        let distance = max(simd_length(offset), 1)
        let horizontal = (offset.x * offset.x + offset.y * offset.y).squareRoot()
        var azimuth = Double(atan2(offset.y, offset.x))
        var elevation = Double(atan2(offset.z, horizontal))
        azimuth += yaw
        let limit = 85 * Double.pi / 180
        elevation = min(max(elevation + pitch, -limit), limit)
        let dir = SIMD3<Float>(
            Float(cos(elevation) * cos(azimuth)), Float(cos(elevation) * sin(azimuth)),
            Float(sin(elevation)))
        var out = pose
        out.target = pose.position + dir * distance
        out.up = SIMD3(0, 0, 1)
        return out
    }
}

// MARK: - Pointing into the scene

enum CameraRay {
    /// The ray through a point of the view (origin bottom left, as an
    /// unflipped NSView's), from the near plane to the far one.
    static func ray(
        camera: Camera, viewPoint p: CGPoint, viewSize size: CGSize
    ) -> (origin: SIMD3<Float>, direction: SIMD3<Float>)? {
        guard size.width > 0, size.height > 0 else { return nil }
        let x = Float(2 * p.x / size.width - 1)
        let y = Float(2 * p.y / size.height - 1)
        let inverse = (camera.projectionMatrix * camera.viewMatrix).inverse
        func unproject(_ z: Float) -> SIMD3<Float>? {
            let q = inverse * SIMD4<Float>(x, y, z, 1)
            guard abs(q.w) > 1e-9 else { return nil }
            return SIMD3(q.x, q.y, q.z) / q.w
        }
        guard let near = unproject(0), let far = unproject(1) else { return nil }
        let d = far - near
        guard simd_length(d) > 1e-9 else { return nil }
        return (near, simd_normalize(d))
    }

    /// Where the ray through a point of the view meets the level plane at
    /// height z; nil when it runs parallel or meets it behind the eye.
    static func ground(
        camera: Camera, viewPoint: CGPoint, viewSize: CGSize, planeZ: Float
    ) -> SIMD3<Float>? {
        guard let ray = ray(camera: camera, viewPoint: viewPoint, viewSize: viewSize),
            abs(ray.direction.z) > 1e-6
        else { return nil }
        let t = (planeZ - ray.origin.z) / ray.direction.z
        guard t >= 0 else { return nil }
        return ray.origin + ray.direction * t
    }
}

// MARK: - Persistence

/// The pane's settings and viewpoints in UserDefaults, as JSON: a site's
/// under its L16 token once one is loaded, falling back to the global
/// default, which is also where they go with no site.
struct CameraPaneStore {
    static let settingsKey = "cameraPane.settings"
    static let viewpointsKey = "cameraPane.viewpoints"
    static let maxViewpoints = 8

    let defaults: UserDefaults?

    static func key(_ base: String, site: String?) -> String {
        guard let site, !site.isEmpty else { return base }
        return base + "." + site
    }

    private func read<T: Decodable>(_ type: T.Type, _ key: String) -> T? {
        guard let data = defaults?.data(forKey: key) else { return nil }
        return try? JSONDecoder().decode(type, from: data)
    }

    private func write<T: Encodable>(_ value: T, _ key: String) {
        guard let defaults, let data = try? JSONEncoder().encode(value) else { return }
        defaults.set(data, forKey: key)
    }

    /// The site's settings, else the global default, else the built-in one.
    func settings(site: String?) -> CameraPaneSettings {
        read(CameraPaneSettings.self, Self.key(Self.settingsKey, site: site)) ?? read(
            CameraPaneSettings.self, Self.settingsKey) ?? CameraPaneSettings()
    }

    /// Whether this site has settings of its own.
    func hasSiteSettings(_ site: String?) -> Bool {
        guard let site, !site.isEmpty else { return false }
        return defaults?.data(forKey: Self.key(Self.settingsKey, site: site)) != nil
    }

    func save(_ settings: CameraPaneSettings, site: String?) {
        write(settings, Self.key(Self.settingsKey, site: site))
    }

    func saveDefault(_ settings: CameraPaneSettings) { write(settings, Self.settingsKey) }

    func viewpoints(site: String?) -> [SavedViewpoint] {
        Array(
            (read([SavedViewpoint].self, Self.key(Self.viewpointsKey, site: site)) ?? []).prefix(
                Self.maxViewpoints))
    }

    func save(viewpoints: [SavedViewpoint], site: String?) {
        write(
            Array(viewpoints.prefix(Self.maxViewpoints)), Self.key(Self.viewpointsKey, site: site))
    }
}
