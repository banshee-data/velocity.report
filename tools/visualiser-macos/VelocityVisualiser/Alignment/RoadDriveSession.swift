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
    var clock: () -> TimeInterval = { ProcessInfo.processInfo.systemUptime }

    private let review: RoadReviewSession
    private var subscriptions = Set<AnyCancellable>()
    private var timer: Timer?
    private var lastTick: TimeInterval?
    private var index = RoadHeightIndex(patches: [])
    /// Frame times recorded while driving, s.
    private(set) var frameIntervals: [Double] = []

    init(review: RoadReviewSession) {
        self.review = review
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
        index = RoadHeightIndex(patches: review.patches)
        rebuild(keepingPlace: false)
        guard driver != nil else { return }
        driver?.setSpeed(speed)
        driver?.setEyeHeight(eyeHeight)
        isDriving = true
        applyPose()
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
        index = RoadHeightIndex(patches: patches)
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

    private func patchesChanged() {
        guard isDriving, startPatch != nil else { return }
        rebuild(keepingPlace: true)
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
        pause()
        sink?.releaseCamera(.drive, restore: false)
        isDriving = false
        message = nil
        review.frameCorridor()
    }

    // MARK: - Clock

    private func startTimer() {
        stopTimer()
        lastTick = clock()
        timer = Timer.scheduledTimer(withTimeInterval: Self.tickSeconds, repeats: true) {
            [weak self] _ in MainActor.assumeIsolated { self?.tick() }
        }
    }

    private func stopTimer() {
        timer?.invalidate()
        timer = nil
        lastTick = nil
    }

    func tick() {
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
        if d.stopped != nil { stopTimer() }
        updateMessage()
        applyPose()
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
