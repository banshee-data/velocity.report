// AnnotationFrameSync.swift
// Keeps the annotation window and the main view on the same frame, and builds
// the frame the annotation window's 3D view draws.
//
// A pack is cut from a recording, and the main view can replay that recording
// with everything a pack leaves out: tracks, boxes, trails, the settled
// background. Putting both on one frame is what lets an operator look at the
// tracker's account of a moment while editing the points of that same moment.
//
// The two are matched by capture timestamp. A pack does not name the run it
// was cut from, and does not need to: a replay whose time range does not take
// in the pack's samples is a different recording, and one that does is the
// same scene at the same instant whichever run produced it.

import Foundation
import simd

/// Where frame sync stands, in terms an operator can act on.
enum FrameSyncStatus: Equatable {
    case off
    /// The main view is live, or replaying something it cannot seek in.
    case mainNotSeekable
    /// The main view's replay does not cover this pack's samples.
    case differentRecording
    /// The main view is on a frame this pack does not hold: before its first
    /// sample, after its last, or in a gap the export skipped.
    case outsidePack
    /// Following would discard unsaved membership, so the window stays put.
    case heldByUnsavedChanges
    case inSync

    var label: String {
        switch self {
        case .off: return "Not following the main view"
        case .mainNotSeekable: return "The main view is not in a seekable replay"
        case .differentRecording: return "The main view is replaying a different recording"
        case .outsidePack: return "The main view is on a frame outside this pack"
        case .heldByUnsavedChanges: return "Holding this sample: it has unsaved changes"
        case .inSync: return "On the same frame as the main view"
        }
    }
}

/// What the annotation window needs to know about the main view's replay.
struct MainViewPlayback: Equatable {
    var timestampNs: Int64
    var logStartNs: Int64
    var logEndNs: Int64
    var seekable: Bool
}

enum AnnotationFrameSync {
    /// How far apart two timestamps may be and still be the same frame: a
    /// little over half the period of a 10 Hz sensor, so a frame always
    /// resolves to the sample it is, and never to a neighbour.
    static let toleranceNs: Int64 = 60_000_000

    /// The sample nearest a timestamp. `samples` must be in chronological
    /// order, as `AnnotationPack.chronologicalSamples` returns them.
    static func nearestSample(
        to timestampNs: Int64, in samples: [AnnotationSample]
    ) -> (index: Int, deltaNs: Int64)? {
        guard !samples.isEmpty else { return nil }
        // First sample at or after the timestamp; the nearest is that one or
        // the one before it.
        var lo = 0
        var hi = samples.count
        while lo < hi {
            let mid = (lo + hi) / 2
            if samples[mid].timestampNs < timestampNs { lo = mid + 1 } else { hi = mid }
        }
        var best: (index: Int, deltaNs: Int64)?
        for index in [lo - 1, lo] where index >= 0 && index < samples.count {
            let delta = abs(samples[index].timestampNs - timestampNs)
            if best == nil || delta < best!.deltaNs { best = (index, delta) }
        }
        return best
    }

    /// True when the replay's time range takes in any of the pack's samples.
    static func replayCovers(
        _ samples: [AnnotationSample], logStartNs: Int64, logEndNs: Int64
    ) -> Bool {
        guard let first = samples.first, let last = samples.last, logEndNs > logStartNs else {
            return false
        }
        return first.timestampNs <= logEndNs + toleranceNs
            && last.timestampNs >= logStartNs - toleranceNs
    }
}

// MARK: - 3D scene

/// Builds the frame the annotation window's 3D view draws.
///
/// The 3D view is the main view's renderer pointed at a pack sample, so that
/// moving through it is the same as moving through the main view. The renderer
/// colours by class, which is how the annotation state is shown: each state is
/// given a class of its own from `AnnotationPalette`. This is display only.
/// The arrays built here are filtered and re-ordered for drawing, and nothing
/// reads an index back out of them.
enum AnnotationScene {
    /// What is drawn over the recorder's classes, strongest claim last.
    struct Marks {
        /// Other objects' saved masks in this sample, with their classes.
        var others: [(objectClass: String, indices: [Int])] = []
        /// The active object's class, saved mask and membership under edit.
        var activeClass: String?
        var saved: Set<Int> = []
        var selected: Set<Int> = []
        /// What a stroke, the brush under the cursor or a carried footprint
        /// would take.
        var candidates: Set<Int> = []
    }

    /// The settled background to draw behind the frame.
    struct Backdrop {
        var snapshot: AnnotationBackground
        var points: BackgroundPoints
        /// Which of the points this snapshot added or moved.
        var changed: [Int]
        /// False when the renderer already holds this snapshot: uploading
        /// seventy thousand points again for every brush movement would be
        /// most of the cost of a redraw.
        var upload: Bool
    }

    static func frame(
        points: PackPoints, classes: [UInt8], visibility: PointVisibility?, marks: Marks,
        sample: AnnotationSample?, backdrop: Backdrop? = nil
    ) -> FrameBundle {
        // Resolved per point up front, so the loop below is one lookup.
        var marked: [Int: UInt8] = [:]
        for other in marks.others {
            let shade = AnnotationPalette.shaderClass(
                AnnotationPalette.paletteIndex(forClass: other.objectClass))
            for index in other.indices { marked[index] = shade }
        }
        let activeShade = AnnotationPalette.shaderClass(
            AnnotationPalette.paletteIndex(forClass: marks.activeClass ?? ""))
        for index in marks.saved.subtracting(marks.selected) {
            marked[index] = AnnotationPalette.shaderClass(AnnotationPalette.removedIndex)
        }
        for index in marks.selected {
            marked[index] =
                marks.saved.contains(index)
                ? activeShade : AnnotationPalette.shaderClass(AnnotationPalette.unsavedIndex)
        }
        for index in marks.candidates where !marks.selected.contains(index) {
            marked[index] = AnnotationPalette.shaderClass(AnnotationPalette.candidateIndex)
        }

        var cloud = PointCloudFrame()
        cloud.frameID = sample?.sourceFrameID ?? 0
        cloud.timestampNanos = sample?.timestampNs ?? 0
        cloud.sensorID = sample?.sensorID ?? ""
        cloud.x.reserveCapacity(points.count)
        cloud.y.reserveCapacity(points.count)
        cloud.z.reserveCapacity(points.count)
        cloud.intensity.reserveCapacity(points.count)
        cloud.classification.reserveCapacity(points.count)

        for index in 0..<points.count {
            let mark = marked[index]
            // A marked return is drawn even when its class is hidden: it is in
            // a mask, and hiding it would hide what the mask claims.
            guard
                mark != nil || PointVisibility.isVisible(index, classes: classes, under: visibility)
            else { continue }
            guard points.x[index].isFinite, points.y[index].isFinite, points.z[index].isFinite
            else { continue }
            cloud.x.append(points.x[index])
            cloud.y.append(points.y[index])
            cloud.z.append(points.z[index])
            cloud.intensity.append(index < points.intensity.count ? points.intensity[index] : 0)
            let own = index < classes.count ? classes[index] : PointClass.unclassified
            // The renderer has no colour for "unclassified"; its foreground
            // green is the closest to "a return, not yet said to be anything".
            cloud.classification.append(
                mark ?? (own == PointClass.unclassified ? PointClass.foreground : own))
        }
        // What the latest snapshot changed is drawn over the background in a
        // colour of its own. It goes in with the frame's points because the
        // renderer draws every background return in one grey.
        if let backdrop, visibility?.background ?? true {
            let shade = AnnotationPalette.shaderClass(AnnotationPalette.backgroundChangedIndex)
            for index in backdrop.changed where index < backdrop.points.count {
                cloud.x.append(backdrop.points.x[index])
                cloud.y.append(backdrop.points.y[index])
                cloud.z.append(backdrop.points.z[index])
                cloud.intensity.append(255)
                cloud.classification.append(shade)
            }
        }
        cloud.pointCount = cloud.x.count

        var bundle = FrameBundle()
        bundle.frameID = cloud.frameID
        bundle.timestampNanos = cloud.timestampNanos
        bundle.sensorID = cloud.sensorID
        bundle.frameType = .full
        bundle.pointCloud = cloud
        if let backdrop, backdrop.upload {
            var snapshot = BackgroundSnapshot()
            // The recorder's own sequence number only moves on a grid reset,
            // so the pack's id is what tells one snapshot from the next.
            snapshot.sequenceNumber = UInt64(backdrop.snapshot.backgroundID)
            snapshot.timestampNanos = backdrop.snapshot.timestampNs
            snapshot.x = backdrop.points.x
            snapshot.y = backdrop.points.y
            snapshot.z = backdrop.points.z
            // The renderer shades by how often a cell was seen. A settled
            // snapshot is all seen often.
            snapshot.confidence = [UInt32](repeating: 10, count: backdrop.points.count)
            bundle.background = snapshot
            bundle.backgroundSeq = snapshot.sequenceNumber
        }
        return bundle
    }
}

extension Camera {
    /// Points the camera at a region from the main view's default bearing, far
    /// enough back to take all of it in.
    mutating func lookAt(_ focus: AnnotationSceneFocus) {
        let radius = max(focus.radius, 1)
        let halfFov = fov * .pi / 360
        // Clamped to what the zoom control itself allows, so the first scroll
        // after a fit does not jump.
        let distance = min(max(radius / tan(halfFov) * 1.2, 2), 500)
        let bearing = simd_normalize(simd_float3(0, -1, 0.7))
        target = focus.centre
        position = focus.centre + bearing * distance
        up = simd_float3(0, 0, 1)
        projection = .perspective
    }
}
