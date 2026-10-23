// FrameAccumulation.swift
// Accumulated frames: a second kind of scan for Gerardus, gathered from the
// replay's own frames rather than from the background grid (Gerardus
// docs/plans/SITE-MODEL-SPIKE.md §6.3).
//
// N frames (default 50, about 5 s) are collected while the replay plays. Only
// points classed background (0) or ground (2) are kept; foreground (1) is
// never collected (ADR-0011: an accumulated scan is a settled background, with
// no foreground and no per-point time). Only full-rate frames count: a frame
// whose points were decimated refuses the whole accumulation. The points stay
// in the sensor frame, are thinned to one per 0.05 m voxel here, and go up
// with UploadScan as SCAN_KIND_ACCUMULATED, named by the replay and the frames
// they came from.

import Foundation

enum FrameAccumulation {
    /// About 5 s of a 10 Hz sensor.
    static let defaultFrameCount = 50
    /// The thinning voxel, in metres, sensor frame.
    static let voxelM: Float = 0.05
    /// Point classes kept: background and ground. Foreground (1) never is.
    static let backgroundClass: UInt8 = 0
    static let groundClass: UInt8 = 2

    /// Why this frame's points cannot be accumulated, or nil when they can:
    /// full-rate frames (decimation ratio 0 or 1, nothing stripped) whose
    /// points each carry a class.
    /// Called for every frame, so it formats nothing for a frame it takes.
    static func refusal(_ cloud: PointCloudFrame) -> String? {
        var ratio: String { String(format: "%.2f", cloud.decimationRatio) }
        switch cloud.decimationMode {
        case .foregroundOnly:
            return
                "The stream carries foreground points only (velocity.report's split streaming), so its frames hold no background or ground to accumulate."
        case .voxel:
            return
                "The stream is voxel-decimated (ratio \(ratio)); accumulating needs full-rate frames."
        case .none, .uniform:
            guard cloud.decimationRatio == 0 || cloud.decimationRatio == 1 else {
                return
                    "The stream is decimated (ratio \(ratio)); accumulating needs full-rate frames."
            }
        }
        let points = min(cloud.x.count, cloud.y.count, cloud.z.count)
        if cloud.classification.count < points {
            return "The stream's points carry no classes, so foreground could not be left out."
        }
        return nil
    }

    /// One point per `voxelM` voxel, at the centroid of the points in it, in
    /// voxel order so the same points always pack the same way. Returned as a
    /// snapshot so it packs and colours like a background: no confidence,
    /// since an accumulated scan weights every point the same.
    static func thinned(
        _ accumulator: FrameAccumulator, voxelM: Float = voxelM
    ) -> BackgroundSnapshot {
        struct Key: Hashable, Comparable {
            var x: Int32
            var y: Int32
            var z: Int32
            static func < (a: Key, b: Key) -> Bool { (a.x, a.y, a.z) < (b.x, b.y, b.z) }
        }
        let inverse = 1 / Double(voxelM)
        var index = [Key: Int]()
        var sums = [SIMD3<Double>]()
        var counts = [Int]()
        let (xs, ys, zs) = (accumulator.x, accumulator.y, accumulator.z)
        for i in 0..<xs.count {
            let p = SIMD3(Double(xs[i]), Double(ys[i]), Double(zs[i]))
            let cell = (p * inverse).rounded(.down)
            let key = Key(x: Int32(cell.x), y: Int32(cell.y), z: Int32(cell.z))
            if let at = index[key] {
                sums[at] += p
                counts[at] += 1
            } else {
                index[key] = sums.count
                sums.append(p)
                counts.append(1)
            }
        }
        var snapshot = BackgroundSnapshot()
        snapshot.timestampNanos = accumulator.firstTimestampNanos
        snapshot.gridMetadata = GridMetadata(settlingComplete: true)
        let ordered = index.sorted { $0.key < $1.key }
        snapshot.x.reserveCapacity(ordered.count)
        snapshot.y.reserveCapacity(ordered.count)
        snapshot.z.reserveCapacity(ordered.count)
        for (_, at) in ordered {
            let centroid = sums[at] / Double(counts[at])
            snapshot.x.append(Float(centroid.x))
            snapshot.y.append(Float(centroid.y))
            snapshot.z.append(Float(centroid.z))
        }
        return snapshot
    }
}

/// Background and ground points gathered from successive replay frames.
struct FrameAccumulator: Sendable {
    enum Outcome: Equatable {
        /// The frame's background and ground points were kept.
        case taken
        /// The frame was seen already (a repeat of the last one).
        case skipped
        /// The accumulation cannot go on, for this reason.
        case refused(String)
    }

    /// Frames to collect.
    let frameCount: Int
    private(set) var framesTaken = 0
    private(set) var firstFrame: UInt64?
    private(set) var lastFrame: UInt64?
    private(set) var firstTimestampNanos: Int64 = 0
    /// Kept points, sensor frame, before thinning.
    private(set) var x: [Float] = []
    private(set) var y: [Float] = []
    private(set) var z: [Float] = []

    init(frameCount: Int = FrameAccumulation.defaultFrameCount) {
        self.frameCount = max(1, frameCount)
    }

    var pointsKept: Int { x.count }
    var isComplete: Bool { framesTaken >= frameCount }

    /// Takes one frame's background and ground points. `frameNumber` places
    /// the frame in the replay; it must not go backwards.
    mutating func add(_ cloud: PointCloudFrame, frameNumber: UInt64) -> Outcome {
        if let refusal = FrameAccumulation.refusal(cloud) { return .refused(refusal) }
        if let last = lastFrame {
            if frameNumber == last { return .skipped }
            if frameNumber < last {
                return .refused(
                    "The replay went back from frame \(last) to \(frameNumber); accumulate again once it plays on."
                )
            }
        }
        let count = min(cloud.x.count, cloud.y.count, cloud.z.count, cloud.classification.count)
        for i in 0..<count {
            let c = cloud.classification[i]
            guard c == FrameAccumulation.backgroundClass || c == FrameAccumulation.groundClass
            else { continue }
            let (px, py, pz) = (cloud.x[i], cloud.y[i], cloud.z[i])
            guard px.isFinite, py.isFinite, pz.isFinite else { continue }
            x.append(px)
            y.append(py)
            z.append(pz)
        }
        if firstFrame == nil {
            firstFrame = frameNumber
            firstTimestampNanos = cloud.timestampNanos
        }
        lastFrame = frameNumber
        framesTaken += 1
        return .taken
    }

    /// "<run>#f<first>-<last>": the replay and the frames the points came from.
    func captureRef(run: String) -> String { "\(run)#f\(firstFrame ?? 0)-\(lastFrame ?? 0)" }
}
