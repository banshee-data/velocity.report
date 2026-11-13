// RoadDriveSession.swift
// The road review's driving camera: a perspective view along a route chained
// from the loaded patches' profiles, at a speed and an eye height, that stops
// at missing height, unloaded geometry or an unresolved choice and says why.
//
// The camera is taken the way the camera pane takes it (`holdCamera`) under
// the `.drive` holder, so orbit, flight and the street painter's plan view
// are refused while driving, and Overhead gives the view back as it was.
// The geometry is RoadDrive's; this class keeps the clock, the route and the
// controls.

import AppKit
import Combine
import Foundation
import simd

/// A source of display-linked frames; MetalRenderer is the real one.
@MainActor protocol RoadFrameSource: AnyObject { func setContinuousDrawing(tick: (() -> Void)?) }

extension MetalRenderer: RoadFrameSource {}

@available(macOS 15.0, *) @MainActor final class RoadDriveSession: ObservableObject {
    static let tickSeconds = 1.0 / 60

    @Published private(set) var driver: RoadDriver?
    /// Why the view is not moving, in a line: the route's own stop, a held
    /// camera, a failed load.
    @Published private(set) var message: String?
    @Published private(set) var isDriving = false
    @Published private(set) var startPatch: String?
    @Published private(set) var reversed = false
    /// Junction patch id -> the arm taken there.
    @Published private(set) var choices: [String: String] = [:]
    @Published var eyeHeight: Float = 1.5 {
        didSet {
            driver?.setEyeHeight(eyeHeight)
            applyPose()
        }
    }
    @Published var speed: Float = 8 { didSet { driver?.setSpeed(speed) } }

    weak var sink: CameraControlSink?
    /// Where the clock comes from while playing: the renderer's display-linked
    /// frames when it offers them, else a timer.
    weak var frames: RoadFrameSource?
    var clock: () -> TimeInterval = { ProcessInfo.processInfo.systemUptime }

    private let review: RoadReviewSession
    private var subscriptions = Set<AnyCancellable>()
    private var timer: Timer?
    private var lastTick: TimeInterval?
    private var index = RoadHeightIndex(patches: [])
    private var indexedMeshes: [RoadMeshIdentity] = []
    private(set) var indexBuilds = 0
    /// Frame times recorded while driving, s.
    private(set) var frameIntervals: [Double] = []
    /// The working-set radii in force while driving.
    @Published var policy = RoadResidencyPolicy()
    private(set) var meter = TraverseMeter()
    /// A timed traverse in progress: ping-pongs the route until `until`.
    @Published private(set) var traverseUntil: TimeInterval?
    @Published private(set) var traverseLoops = 0
    @Published private(set) var lastTraverse: TraverseSummary?
    /// Called each frame with the renderer's draw calls, when there is one.
    var drawCalls: () -> Int = { 0 }

    init(review: RoadReviewSession) {
        self.review = review
        review.sceneWillChange.sink { [weak self] in self?.endDrive() }.store(in: &subscriptions)
        review.loader.$store.sink { [weak self] _ in
            DispatchQueue.main.async { self?.patchesChanged() }
        }.store(in: &subscriptions)
    }

    // MARK: - Starting

    /// Segment patches a drive can start on.
    var startOptions: [RoadPatch] {
        review.patches.filter { $0.kind == .segment && $0.profile.count >= 2 }
    }

    var distanceAlong: Float { driver?.s ?? 0 }
    var routeLength: Float { driver?.length ?? 0 }
    var stopReason: RoadStop? { driver?.stopped }

    /// Starts at `patch`, driving its profile forwards or backwards. Paused:
    /// press Play.
    func begin(on patchID: String, reversed: Bool = false) {
        startPatch = patchID
        self.reversed = reversed
        rebuild(keepingPlace: false)
        guard driver != nil else { return }
        driver?.setSpeed(speed)
        driver?.setEyeHeight(eyeHeight)
        isDriving = true
        applyPose()
        maintainWorkingSet()
    }

    /// Takes `armPatch` at `junction` and rebuilds the route from the start.
    func choose(arm armPatch: String, at junction: String) {
        choices[junction] = armPatch
        rebuild(keepingPlace: true)
        applyPose()
    }

    private func rebuild(keepingPlace: Bool) {
        guard let startPatch else { return }
        let patches = review.patches
        indexedMeshes = patches.map(RoadMeshIdentity.init)
        index = RoadHeightIndex(patches: patches)
        indexBuilds += 1
        var route = RoadRouteBuilder.route(
            from: startPatch, reversed: reversed, patches: patches, index: index, choices: choices)
        // A route that ends while the corridor is still loading, or partly
        // failed, ends for lack of geometry, not because the road ends.
        if route.stop == .endOfRoute {
            switch review.loader.state {
            case .loading: route.stop = .unloadedGeometry(detail: "the corridor is still loading")
            case .partial(let why):
                route.stop = .unloadedGeometry(detail: "the load was partial: \(why)")
            default: break
            }
        }
        if var d = driver, keepingPlace {
            d.replace(route: route)
            driver = d
        } else {
            driver = RoadDriver(route: route)
        }
        updateMessage()
    }

    /// More of the corridor arrived (or some was evicted): a route that stopped
    /// for lack of geometry goes on from where it stopped. It is not rebuilt
    /// from its start, so the stretch already driven keeps its heights.
    private func patchesChanged() {
        guard isDriving, var d = driver else { return }
        let patches = review.patches
        let key = patches.map(RoadMeshIdentity.init)
        if key != indexedMeshes {
            indexedMeshes = key
            index = RoadHeightIndex(patches: patches)
            indexBuilds += 1
        }
        // Completion and partial-load status still update the route even when
        // the final message carried no new mesh.
        var route = RoadRouteBuilder.extended(
            d.route, patches: patches, index: index, choices: choices)
        if route.stop == .endOfRoute {
            switch review.loader.state {
            case .loading: route.stop = .unloadedGeometry(detail: "the corridor is still loading")
            case .partial(let why):
                route.stop = .unloadedGeometry(detail: "the load was partial: \(why)")
            default: break
            }
        }
        d.replace(route: route)
        driver = d
        updateMessage()
        applyPose()
    }

    /// Keeps the working set around the eye.
    private func maintainWorkingSet() {
        guard isDriving, let pose = driver?.pose() else { return }
        let eye = SIMD2(pose.position.x, pose.position.y)
        if review.bounded == nil {
            review.enterBounded(eye: eye, policy: policy)
        } else {
            review.updateEye(eye)
        }
    }

    // MARK: - Controls

    func play() {
        guard var d = driver else { return }
        d.play()
        driver = d
        if d.playing { startTimer() }
        updateMessage()
    }

    func pause() {
        guard var d = driver else { return }
        d.pause()
        driver = d
        stopTimer()
        updateMessage()
    }

    func togglePlay() { driver?.playing == true ? pause() : play() }

    func scrub(to s: Float) {
        guard var d = driver else { return }
        d.scrub(to: s)
        driver = d
        updateMessage()
        applyPose()
    }

    /// Back to the overhead view: the driving camera is let go and the corridor
    /// is framed from above, to inspect a defect.
    func overhead() {
        stopTraverse()
        pause()
        review.leaveBounded()
        sink?.releaseCamera(.drive, restore: false)
        isDriving = false
        message = nil
        review.frameCorridor()
    }

    /// A closed or replaced corridor cannot keep its route or camera clock.
    private func endDrive() {
        stopTraverse()
        pause()
        sink?.releaseCamera(.drive, restore: false)
        isDriving = false
        driver = nil
        startPatch = nil
        choices = [:]
        index = RoadHeightIndex(patches: [])
        indexedMeshes = []
        message = nil
    }

    // MARK: - Clock

    private func startTimer() {
        stopTimer()
        lastTick = clock()
        if let frames {
            frames.setContinuousDrawing { [weak self] in MainActor.assumeIsolated { self?.tick() } }
            usingFrames = true
            return
        }
        timer = Timer.scheduledTimer(withTimeInterval: Self.tickSeconds, repeats: true) {
            [weak self] _ in MainActor.assumeIsolated { self?.tick() }
        }
    }

    private var usingFrames = false

    private func stopTimer() {
        if usingFrames {
            frames?.setContinuousDrawing(tick: nil)
            usingFrames = false
        }
        timer?.invalidate()
        timer = nil
        lastTick = nil
    }

    func tick() {
        let began = clock()
        defer { traverseTick(started: began) }
        let now = clock()
        let dt = lastTick.map { now - $0 } ?? Self.tickSeconds
        lastTick = now
        advance(by: dt)
    }

    /// One step of the clock; tests call this directly.
    func advance(by dt: Double) {
        guard var d = driver else { return }
        // A long stall is not driven through: cap the step.
        d.advance(by: Float(min(dt, 0.1)))
        driver = d
        frameIntervals.append(dt)
        lastDt = dt
        if d.stopped != nil && traverseUntil == nil { stopTimer() }
        updateMessage()
        applyPose()
        maintainWorkingSet()
    }

    // MARK: - Traverse

    private var lastDt = 0.0

    /// Drives the route back and forth for `seconds` of wall time, measuring
    /// each frame; see `TraverseMeter`. The summary lands in `lastTraverse`.
    func startTraverse(seconds: TimeInterval) {
        guard isDriving, driver != nil else { return }
        meter.reset()
        firstFrameSeen = false
        traverseLoops = 0
        // A measured run must not be throttled as a background app: say the
        // work is user-initiated and latency critical, and bring the app forward.
        activity = ProcessInfo.processInfo.beginActivity(
            options: [.userInitiated, .latencyCritical], reason: "Road view traverse measurement")
        NSApp.activate(ignoringOtherApps: true)
        traverseUntil = clock() + seconds
        driver?.setSpeed(speed)
        play()
    }

    func stopTraverse() {
        guard traverseUntil != nil else { return }
        traverseUntil = nil
        if let activity { ProcessInfo.processInfo.endActivity(activity) }
        activity = nil
        lastTraverse = meter.summary()
    }

    private var lastTickMs = 0.0
    private var firstFrameSeen = false
    private var activity: NSObjectProtocol?

    /// The clock side of a traverse: the end of the run, and the turn at either
    /// end of the route (back to its start, so the working set is fetched and
    /// evicted again as in a repeated drive).
    private func traverseTick(started: TimeInterval) {
        guard let until = traverseUntil else { return }
        let now = clock()
        lastTickMs = (now - started) * 1000
        if now >= until {
            stopTraverse()
            pause()
            return
        }
        if let d = driver, d.stopped != nil {
            traverseLoops += 1
            scrub(to: 0)
            play()
        }
    }

    /// One rendered frame, from the renderer: its interval, encode time and
    /// (later) GPU time. Recorded only during a traverse.
    func noteFrame(interval: Double, encodeMs: Double, gpuMs: Double) {
        guard traverseUntil != nil else { return }
        // The first interval runs from the last draw before continuous drawing
        // began, so it is the idle gap, not a frame.
        if meter.samples.isEmpty, !firstFrameSeen {
            firstFrameSeen = true
            return
        }
        meter.record(
            TraverseSample(
                interval: interval, tickMs: lastTickMs, encodeMs: encodeMs, gpuMs: gpuMs,
                loop: traverseLoops, drawCalls: drawCalls(), visiblePatches: review.visibleCount,
                residentPatches: review.loader.store.residentPatches.count,
                residentMeshBytes: review.loader.store.residentMeshBytes,
                footprintBytes: ProcessMemory.footprintBytes()))
    }

    /// The provenance of a measurement taken now.
    func provenance() -> TraverseProvenance {
        TraverseProvenance.current(
            settings:
                "eye \(eyeHeight) m, speed \(speed) m/s, radii \(policy.visibleM)/\(policy.prefetchM)/\(policy.evictM) m",
            corridor: review.loader.store.corridors, patches: review.patches)
    }

    private func updateMessage() {
        guard let d = driver else { return }
        if let stop = d.stopped {
            message = stop.message
        } else if let failure = review.blocker, review.loader.state != .loaded {
            message = failure
        } else {
            message = nil
        }
    }

    private func applyPose() {
        guard isDriving, let sink, let pose = driver?.pose() else { return }
        var camera = sink.camera
        camera.position = pose.position
        camera.target = pose.target
        camera.up = SIMD3(0, 0, 1)
        camera.projection = .perspective
        camera.fov = 70
        camera.nearPlane = 0.1
        camera.farPlane = max(camera.farPlane, 400)
        if !sink.holdCamera(.drive, camera) {
            pause()
            message = "The camera is held elsewhere (orbit, flight or plan view): stop it first."
        }
    }
}
