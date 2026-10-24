// CameraControlSession.swift
// State behind the camera pane: orbiting the main view's camera round an
// ellipse, flying it freely, and flying it to a saved viewpoint.
//
// The pane takes the camera the way the street painter's plan view does
// (MetalRenderer.holdCamera): Play holds it for the orbit and Stop puts
// back the camera from before Play; flight holds it while engaged and
// leaves it where it flew to. Only one holder at a time, so the orbit and
// flight are refused while painting, and painting is refused while either
// runs (StreetPainterSession.cameraBlocker).
//
// The geometry is CameraPaths' and the curves CameraEasing's; this class
// keeps the clock, the keys held, the settings per site, and the
// viewpoints.

import AppKit
import Combine
import CoreGraphics
import Foundation
import simd

/// Where the camera pane moves the camera and draws its preview.
/// MetalRenderer is the real one.
protocol CameraControlSink: AnyObject {
    var camera: Camera { get }
    /// The site frame drawn in the sensor's: where the site's origin and
    /// north are, and the reference's points.
    var referenceModelMatrix: simd_float4x4 { get }
    @discardableResult func holdCamera(_ holder: CameraHolder, _ camera: Camera) -> Bool
    func releaseCamera(_ holder: CameraHolder, restore: Bool)
    func setCameraPathPreview(strip: [Float]?, ticks: [Float])
    /// Gives the 3D view the keys, so flight hears WASD at once.
    func focusView()
}

extension MetalRenderer: CameraControlSink {
    func focusView() {
        guard let view else { return }
        view.window?.makeFirstResponder(view)
    }
}

@available(macOS 15.0, *) @MainActor final class CameraControlSession: ObservableObject {
    enum Playback: Equatable {
        case stopped
        case playing
        case paused
        /// A single lap came to its end; the camera stays there until Play
        /// or Stop.
        case finished
    }

    enum PickStep: Equatable {
        case a
        case b
    }

    /// The radii a Measure set, for the pane to show beside an edit.
    struct Measured: Equatable {
        var a: Double
        var b: Double
        var source: String
    }

    /// How many scene points Measure from scene looks at, at most.
    static let measurePointLimit = 60_000
    static let tickSeconds = 1.0 / 60

    // MARK: Published state

    @Published var settings: CameraPaneSettings { didSet { settingsChanged(from: oldValue) } }
    @Published private(set) var playback: Playback = .stopped { didSet { activityChanged() } }
    /// Laps played since Play, whole and part.
    @Published private(set) var laps = 0.0
    @Published private(set) var isFlying = false { didSet { activityChanged() } }
    /// The viewpoint a Fly to is heading for.
    @Published private(set) var flyingTo: SavedViewpoint? { didSet { activityChanged() } }
    @Published private(set) var picking: PickStep?
    @Published private(set) var measured: Measured?
    @Published private(set) var viewpoints: [SavedViewpoint] = []
    /// The last thing a button could not do, or did, in a line.
    @Published private(set) var message: String?
    /// The L16 token the settings are kept under; nil for the global default.
    @Published private(set) var siteKey: String?
    @Published var isPaneOpen = false { didSet { if isPaneOpen != oldValue { paneToggled() } } }

    // MARK: Collaborators

    weak var sink: CameraControlSink? { didSet { refreshPreview() } }
    let alignment: AlignmentSession?
    let painter: StreetPainterSession?
    /// The selected track's centre, in the renderer's frame.
    var selectedObjectCentre: () -> SIMD3<Float>? = { nil }
    /// Called after the pane moves the camera, to reproject labels.
    var onCameraMoved: () -> Void = {}
    /// Seconds, monotonic; tests replace it.
    var clock: () -> TimeInterval = { ProcessInfo.processInfo.systemUptime }

    private let store: CameraPaneStore
    private var loadingSettings = false
    private var subscriptions = Set<AnyCancellable>()
    private var timer: Timer?
    private var lastTick: TimeInterval?
    /// The centre the orbit playing now goes round, fixed at Play.
    private var playCentre: SIMD3<Float>?
    private var heldKeys = Set<UInt16>()
    private var motion = FlyMotion()
    private var flyFrom: CameraPose?
    private var flyStart: TimeInterval = 0

    init(
        alignment: AlignmentSession? = nil, painter: StreetPainterSession? = nil,
        defaults: UserDefaults?
    ) {
        self.alignment = alignment
        self.painter = painter
        store = CameraPaneStore(defaults: defaults)
        settings = store.settings(site: nil)
        viewpoints = store.viewpoints(site: nil)
        alignment?.$site.map { $0?.l16Token }.removeDuplicates().sink { [weak self] token in
            self?.siteChanged(token)
        }.store(in: &subscriptions)
        painter?.$isPainting.removeDuplicates().sink { [weak self] _ in
            // After the change lands, so the blocker reads it.
            DispatchQueue.main.async { self?.objectWillChange.send() }
        }.store(in: &subscriptions)
    }

    // MARK: - Mutual exclusion

    /// Why the orbit and flight cannot start now; nil when they can.
    var blocker: String? {
        if painter?.isPainting == true {
            return "Leave paint mode first: the plan view holds the camera."
        }
        if sink == nil { return "No 3D view yet." }
        return nil
    }

    /// True while the pane holds the camera or moves it.
    var isActive: Bool { playback != .stopped || isFlying || flyingTo != nil }

    private func activityChanged() {
        let reason: String?
        if playback != .stopped {
            reason = "Stop the camera pane's orbit first: it holds the camera."
        } else if isFlying || flyingTo != nil {
            reason = "Stop flying in the camera pane first: it holds the camera."
        } else {
            reason = nil
        }
        if painter?.cameraBlocker != reason { painter?.cameraBlocker = reason }
    }

    // MARK: - The frame

    /// North in the renderer's frame: the placed site's, else +y.
    var north: SIMD3<Float> {
        guard let pose = alignment?.pose else { return SIMD3(0, 1, 0) }
        let n = pose.rotationMatrix.transpose * SIMD3<Double>(0, 1, 0)
        return SIMD3(Float(n.x), Float(n.y), Float(n.z))
    }

    /// The site frame's origin in the renderer's, when a site is placed.
    var siteOrigin: SIMD3<Float>? {
        guard alignment?.pose != nil, let sink else { return nil }
        let p = sink.referenceModelMatrix * SIMD4<Float>(0, 0, 0, 1)
        return SIMD3(p.x, p.y, p.z)
    }

    /// The centre the settings name, now, and a note when it fell back to
    /// the camera's target.
    func resolveCentre() -> (centre: SIMD3<Float>, note: String?) {
        let target = sink?.camera.target ?? .zero
        switch settings.orbit.centre {
        case .cameraTarget: return (target, nil)
        case .selectedObject:
            if let centre = selectedObjectCentre() { return (centre, nil) }
            return (target, "No object is selected: going round the camera's target.")
        case .siteOrigin:
            if let origin = siteOrigin { return (origin, nil) }
            return (.zero, "No site is placed: going round the sensor.")
        }
    }

    /// The orbit's frame: the fixed centre while playing, else the
    /// settings' centre now.
    var frame: OrbitFrame { OrbitFrame(centre: playCentre ?? resolveCentre().centre, north: north) }

    // MARK: - Settings and persistence

    private func settingsChanged(from old: CameraPaneSettings) {
        guard settings != old else { return }
        if !loadingSettings { store.save(settings, site: siteKey) }
        if settings.mode != old.mode {
            if settings.mode == .fly { stop() } else { setFlying(false) }
            picking = nil
        }
        refreshPreview()
        if playback == .paused || playback == .finished { applyOrbitPose() }
    }

    private func siteChanged(_ token: String?) {
        guard token != siteKey else { return }
        stop()
        setFlying(false)
        cancelFlyTo()
        picking = nil
        siteKey = token
        loadingSettings = true
        settings = store.settings(site: token)
        loadingSettings = false
        viewpoints = store.viewpoints(site: token)
        measured = nil
        refreshPreview()
    }

    /// Whether the settings shown are this site's own, rather than the
    /// global default not yet changed here.
    var hasSiteSettings: Bool { store.hasSiteSettings(siteKey) }

    /// Makes these settings the default for sites with none of their own.
    func saveAsDefault() {
        store.saveDefault(settings)
        message = "Saved as the default for sites without settings of their own."
    }

    // MARK: - Editing the radii

    func setRadiusA(_ a: Double) { settings.orbit.setRadiusA(a) }
    func setRadiusB(_ b: Double) { settings.orbit.setRadiusB(b) }
    func setDifference(_ d: Double) { settings.orbit.setDifference(d) }
    func setRatio(_ r: Double) { settings.orbit.setRatio(r) }

    // MARK: - Measuring

    private func measuredInto(_ next: OrbitSettings, source: String) {
        settings.orbit = next
        measured = Measured(a: next.radiusA, b: next.radiusB, source: source)
    }

    /// A and B from the camera's offset from the centre, so that phase 0
    /// is where the camera is.
    func measureFromCamera() {
        guard let camera = sink?.camera else { return }
        let resolved = resolveCentre()
        let frame = OrbitFrame(centre: playCentre ?? resolved.centre, north: north)
        guard
            let next = OrbitPath.measureFromCamera(
                position: camera.position, orbit: settings.orbit, frame: frame)
        else {
            message = "The camera is over the centre: move it away to measure from it."
            return
        }
        measuredInto(next, source: "camera")
        message = resolved.note
    }

    /// A and B fitted to what is drawn: the registered scans, else the
    /// placed reference's boxes and kerbs.
    func measureFromScene() {
        guard let sink else { return }
        guard let geometry = alignment?.referenceGeometry, alignment?.pose != nil else {
            message = "Nothing to measure: place a site's reference first."
            return
        }
        let gathered = Self.scenePoints(
            geometry, model: sink.referenceModelMatrix, limit: Self.measurePointLimit)
        guard
            let fit = OrbitPath.measureFromScene(
                points: gathered.points, orbit: settings.orbit, frame: frame)
        else {
            message = "Nothing to measure: the reference has no scans, boxes or kerbs."
            return
        }
        var next = settings.orbit
        next.radiusA = fit.a
        next.radiusB = fit.b
        measuredInto(next, source: gathered.source)
        message = "Fitted to \(gathered.points.count.formatted()) points of the \(gathered.source)."
    }

    /// The reference's points, in the renderer's frame, at most `limit` of
    /// them: the registered scans when there are any, else the vertices of
    /// the boxes and the kerbs.
    nonisolated static func scenePoints(
        _ geometry: ReferenceGeometry, model: simd_float4x4, limit: Int
    ) -> (points: [SIMD3<Float>], source: String) {
        func placed(_ x: Float, _ y: Float, _ z: Float) -> SIMD3<Float> {
            let q = model * SIMD4<Float>(x, y, z, 1)
            return SIMD3(q.x, q.y, q.z)
        }
        let total = geometry.scans.reduce(0) { $0 + $1.points.x.count }
        if total > 0 {
            let stride = max(1, (total + limit - 1) / max(limit, 1))
            var out = [SIMD3<Float>]()
            out.reserveCapacity(min(total, limit))
            var n = 0
            for scan in geometry.scans {
                let block = scan.points
                let count = min(block.x.count, block.y.count, block.z.count)
                for i in 0..<count {
                    if n % stride == 0 { out.append(placed(block.x[i], block.y[i], block.z[i])) }
                    n += 1
                }
            }
            return (out, "registered scans")
        }
        var out = [SIMD3<Float>]()
        for feature in geometry.boxes + geometry.kerbs {
            let xyz = feature.xyz
            for i in Swift.stride(from: 0, to: xyz.count - 2, by: 3) {
                out.append(placed(xyz[i], xyz[i + 1], xyz[i + 2]))
            }
        }
        return (out, "boxes and kerbs")
    }

    /// The next two clicks in the 3D view set A's end, then B's.
    func startPicking() {
        guard settings.mode == .orbit else { return }
        picking = .a
        message = "Click in the 3D view where A should end."
    }

    func cancelPicking() {
        picking = nil
        message = nil
    }

    /// A click in the 3D view while picking: the ground under it, at the
    /// centre's height. True when the click was the pick's.
    @discardableResult func pick(at viewPoint: CGPoint, viewSize: CGSize) -> Bool {
        guard let step = picking, let camera = sink?.camera else { return false }
        let frame = self.frame
        guard
            let point = CameraRay.ground(
                camera: camera, viewPoint: viewPoint, viewSize: viewSize, planeZ: frame.centre.z)
        else {
            message = "That click does not meet the ground at the centre's height: try lower."
            return true
        }
        switch step {
        case .a:
            guard let next = OrbitPath.pickA(point, orbit: settings.orbit, frame: frame) else {
                message = "That is the centre: click where A should end."
                return true
            }
            settings.orbit = next
            picking = .b
            message = "Now click where B should end."
        case .b:
            let next = OrbitPath.pickB(point, orbit: settings.orbit, frame: frame)
            measuredInto(next, source: "picked points")
            picking = nil
            message = nil
        }
        return true
    }

    // MARK: - Orbit playback

    func play() {
        guard settings.mode == .orbit else { return }
        if let blocker {
            message = blocker
            return
        }
        cancelFlyTo()
        setFlying(false)
        switch playback {
        case .playing: return
        case .paused: break
        case .finished: laps = 0
        case .stopped:
            let resolved = resolveCentre()
            playCentre = resolved.centre
            laps = 0
            message = resolved.note
        }
        guard applyOrbitPose() else {
            playCentre = nil
            message = "The camera is held elsewhere."
            return
        }
        playback = .playing
        startTimer()
    }

    func pause() {
        guard playback == .playing else { return }
        playback = .paused
    }

    /// Ends the orbit and puts back the camera from before Play.
    func stop() {
        guard playback != .stopped else { return }
        playback = .stopped
        laps = 0
        playCentre = nil
        sink?.releaseCamera(.orbit, restore: true)
        onCameraMoved()
        refreshPreview()
    }

    func togglePlayPause() { if playback == .playing { pause() } else { play() } }

    @discardableResult private func applyOrbitPose() -> Bool {
        guard let sink, playCentre != nil else { return false }
        let pose = OrbitPath.pose(laps: laps, settings: settings, frame: frame)
        guard sink.holdCamera(.orbit, pose.applied(to: sink.camera)) else { return false }
        onCameraMoved()
        return true
    }

    /// The operator orbited, panned, zoomed or used a camera key: an orbit
    /// playing pauses where it is, and a Fly to stops.
    func manualInput() {
        if playback == .playing { pause() }
        cancelFlyTo()
    }

    /// Whether a speed curve that jumps at the seam is set to loop.
    var seamWarning: String? {
        guard settings.mode == .orbit, settings.orbit.loop,
            !CameraEasing.isSeamContinuous(settings.curve, bezier: settings.bezier)
        else { return nil }
        return
            "\(settings.curve.label) does not end at the speed it starts at, so the speed jumps each time a lap starts again. Linear and the in-out curves loop smoothly."
    }

    // MARK: - Free flight

    func setFlying(_ on: Bool) {
        if on {
            guard !isFlying, settings.mode == .fly else { return }
            if let blocker {
                message = blocker
                return
            }
            stop()
            cancelFlyTo()
            guard let sink, sink.holdCamera(.fly, sink.camera) else {
                message = "The camera is held elsewhere."
                return
            }
            isFlying = true
            message = nil
            sink.focusView()
        } else {
            guard isFlying else { return }
            isFlying = false
            heldKeys = []
            motion = FlyMotion()
            if flyingTo == nil { sink?.releaseCamera(.fly, restore: false) }
        }
    }

    /// A key went down in the 3D view. True when flight took it.
    func flyKeyDown(_ keyCode: UInt16) -> Bool {
        guard isFlying, FlyDynamics.keys[keyCode] != nil else { return false }
        cancelFlyTo()
        heldKeys.insert(keyCode)
        startTimer()
        return true
    }

    /// A key came up. True when flight had it.
    func flyKeyUp(_ keyCode: UInt16) -> Bool {
        guard isFlying, FlyDynamics.keys[keyCode] != nil else { return false }
        heldKeys.remove(keyCode)
        return true
    }

    /// The 3D view lost the keys: nothing is held any more.
    func releaseKeys() { heldKeys = [] }

    /// Mouse movement while looking, in points (y down, as NSEvent's
    /// deltas are).
    func look(dx: CGFloat, dy: CGFloat) {
        guard isFlying, let sink else { return }
        let s = settings.fly.lookSensitivity
        let pose = FlyDynamics.look(
            CameraPose(sink.camera), yaw: -Double(dx) * s, pitch: -Double(dy) * s)
        sink.holdCamera(.fly, pose.applied(to: sink.camera))
        onCameraMoved()
    }

    // MARK: - Viewpoints and Fly to

    var canSaveViewpoint: Bool { viewpoints.count < CameraPaneStore.maxViewpoints }

    func saveViewpoint(named name: String) {
        guard let camera = sink?.camera else { return }
        guard canSaveViewpoint else {
            message = "\(CameraPaneStore.maxViewpoints) viewpoints are saved: delete one first."
            return
        }
        let trimmed = name.trimmingCharacters(in: .whitespaces)
        let label = trimmed.isEmpty ? "View \(viewpoints.count + 1)" : trimmed
        viewpoints.append(SavedViewpoint(name: label, pose: CameraPose(camera)))
        store.save(viewpoints: viewpoints, site: siteKey)
    }

    func deleteViewpoint(_ id: SavedViewpoint.ID) {
        viewpoints.removeAll { $0.id == id }
        store.save(viewpoints: viewpoints, site: siteKey)
    }

    func renameViewpoint(_ id: SavedViewpoint.ID, to name: String) {
        guard let i = viewpoints.firstIndex(where: { $0.id == id }) else { return }
        viewpoints[i].name = name
        store.save(viewpoints: viewpoints, site: siteKey)
    }

    /// Animates the camera to `viewpoint` over the fly-to time, along the
    /// speed curve; at once when that time is zero.
    func fly(to viewpoint: SavedViewpoint) {
        if let blocker {
            message = blocker
            return
        }
        stop()
        guard let sink else { return }
        guard sink.holdCamera(.fly, sink.camera) else {
            message = "The camera is held elsewhere."
            return
        }
        flyFrom = CameraPose(sink.camera)
        flyStart = clock()
        flyingTo = viewpoint
        if settings.fly.flyToSeconds <= 0 { finishFlyTo() } else { startTimer() }
    }

    private func cancelFlyTo() {
        guard flyingTo != nil else { return }
        flyingTo = nil
        flyFrom = nil
        if !isFlying { sink?.releaseCamera(.fly, restore: false) }
    }

    private func finishFlyTo() {
        guard let viewpoint = flyingTo, let sink else { return }
        sink.holdCamera(.fly, viewpoint.pose.applied(to: sink.camera))
        onCameraMoved()
        cancelFlyTo()
    }

    // MARK: - The clock

    private func startTimer() {
        guard timer == nil else { return }
        lastTick = clock()
        let timer = Timer(timeInterval: Self.tickSeconds, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.tick() }
        }
        RunLoop.main.add(timer, forMode: .common)
        self.timer = timer
    }

    private func stopTimer() {
        timer?.invalidate()
        timer = nil
        lastTick = nil
    }

    /// One step of whatever is moving the camera; the timer calls it.
    func tick() {
        let now = clock()
        let dt = min(max(now - (lastTick ?? now), 0), 0.1)
        lastTick = now
        advance(by: dt, now: now)
        if playback != .playing && flyingTo == nil
            && !(isFlying && (motion.isMoving || !heldKeys.isEmpty))
        {
            stopTimer()
        }
    }

    /// Moves everything on by `dt` seconds, at time `now`.
    func advance(by dt: Double, now: TimeInterval) {
        if playback == .playing {
            let next = OrbitPath.advance(laps: laps, by: dt, settings: settings)
            laps = next.laps
            applyOrbitPose()
            if next.finished { playback = .finished }
        }
        if let viewpoint = flyingTo, let from = flyFrom, let sink {
            let t = settings.fly.flyToSeconds > 0 ? (now - flyStart) / settings.fly.flyToSeconds : 1
            if t >= 1 {
                finishFlyTo()
            } else {
                let pose = CameraPose.mix(from, viewpoint.pose, settings.ease(t))
                sink.holdCamera(.fly, pose.applied(to: sink.camera))
                onCameraMoved()
            }
        }
        if isFlying, let sink {
            motion = FlyDynamics.step(
                motion, input: FlyDynamics.input(held: heldKeys), dt: dt, fly: settings.fly)
            let speed = FlyDynamics.speed(motion, settings: settings)
            if speed > 0 {
                let pose = FlyDynamics.move(
                    CameraPose(sink.camera), motion: motion, speed: speed, dt: dt)
                sink.holdCamera(.fly, pose.applied(to: sink.camera))
                onCameraMoved()
            }
        }
    }

    /// Free flight's speed now, metres a second.
    var flySpeed: Double { FlyDynamics.speed(motion, settings: settings) }

    // MARK: - The pane and the preview

    private func paneToggled() {
        if !isPaneOpen {
            stop()
            setFlying(false)
            cancelFlyTo()
            picking = nil
        }
        refreshPreview()
    }

    /// The camera moved by the operator's hand; a preview round the
    /// camera's target follows it.
    func cameraMoved() {
        if playCentre == nil, settings.orbit.centre == .cameraTarget { refreshPreview() }
    }

    func refreshPreview() {
        guard let sink else { return }
        guard isPaneOpen, settings.mode == .orbit, settings.orbit.showPreview else {
            sink.setCameraPathPreview(strip: nil, ticks: [])
            return
        }
        let frame = self.frame
        sink.setCameraPathPreview(
            strip: OrbitPath.previewVertices(orbit: settings.orbit, frame: frame),
            ticks: OrbitPath.startMarker(orbit: settings.orbit, frame: frame))
    }
}
