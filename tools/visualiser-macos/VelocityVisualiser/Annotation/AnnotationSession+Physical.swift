// AnnotationSession+Physical.swift
// Where the orthographic views meet the physical-reference draft and the
// intensity readout: a click becomes an anchor position in the pack's frame,
// and the cursor becomes the returns under it.
//
// Both go through the same basis and viewport the views draw with, so a
// placement lands where it was clicked and a readout names the return drawn
// there, whatever the grid rotation, pan or zoom.

import Foundation
import simd

extension AnnotationSession {
    /// The world point a view-plane position names, at zero depth. Every
    /// basis here has its origin at the pack's origin.
    func worldPoint(in standard: OrthoViewBasis.Standard, viewPoint: simd_float2) -> simd_float3 {
        let b = basis(standard)
        return b.origin + b.right * viewPoint.x + b.up * viewPoint.y
    }

    /// Places the active object's keyframe anchor at a click.
    ///
    /// In the top view it sets the horizontal position, creating this
    /// sample's keyframe if there is none. In an elevation it sets only the
    /// optional height, and only once a horizontal position exists: a height
    /// with nothing to hang it on is not a position.
    func placePhysicalAnchor(in standard: OrthoViewBasis.Standard, at viewPoint: simd_float2) {
        guard workMode == .physical, physical.canEdit else { return }
        guard let objectID = activeObjectID, let sample = currentSample else {
            physical.refuse("Choose an object before placing a physical reference.")
            return
        }
        let world = worldPoint(in: standard, viewPoint: viewPoint)
        let author = operatorName
        let session = sessionID
        if standard == .top {
            physical.edit { objects in
                PhysicalDraft.addKeyframe(
                    objectID: objectID, sample: sample, author: author, session: session,
                    to: &objects)
                PhysicalDraft.place(
                    objectID: objectID, sampleID: sample.sampleID, x: Double(world.x),
                    y: Double(world.y), in: &objects)
            }
            return
        }
        guard let k = physical.object(objectID)?.keyframe(sampleID: sample.sampleID),
            k.position.xM != nil
        else {
            physical.refuse(
                "Place the position in the Top view first; an elevation sets only its height.")
            return
        }
        physical.edit { objects in
            PhysicalDraft.updateKeyframe(
                objectID: objectID, sampleID: sample.sampleID, in: &objects
            ) { $0.position.zM = Double(world.z) }
        }
    }

    /// The keyframe the physical pane edits: the active object's at this
    /// sample, from the draft.
    var physicalKeyframe: PhysicalKeyframe? {
        guard let objectID = activeObjectID, let sample = currentSample else { return nil }
        return physical.object(objectID)?.keyframe(sampleID: sample.sampleID)
    }

    /// The object's saved physical progress, separate from its membership:
    /// the body's review and how many keyframes are saved and reviewed. Nil
    /// until the physical references have been read.
    func physicalProgress(objectID: String) -> String? {
        guard physical.state != nil else { return nil }
        guard let saved = physical.savedObject(objectID) else { return "body none · no keyframes" }
        let body = saved.body.map { $0.review.status.rawValue } ?? "none"
        let reviewed = saved.keyframes.filter { $0.review.status == .reviewed }.count
        return "body \(body) · \(saved.keyframes.count) keyframes, \(reviewed) reviewed"
    }

    /// Reads out the returns under the cursor in one view.
    func inspectReturns(
        in standard: OrthoViewBasis.Standard, viewport: OrthoViewport, at location: CGPoint?
    ) {
        guard let location, let sample = currentSample else {
            inspection.setHovered([])
            return
        }
        let visibility = effectiveVisibility
        let labels = effectiveLabelSets
        let classes = currentClasses
        let b = basis(standard)
        let available = intensityAvailability.available
        let found = IntensityPicker.candidates(
            points: currentPoints, classes: classes, basis: b, viewport: viewport, at: location,
            radius: 6,
            isVisible: {
                PointVisibility.isVisible($0, classes: classes, under: visibility, labels: labels)
            })
        inspection.setHovered(
            found.compactMap { hit in
                guard let p = currentPoints.point(at: hit.index) else { return nil }
                return IntensityReadout(
                    sampleID: sample.sampleID, sourceOrdinal: sample.sourceOrdinal,
                    pointIndex: hit.index,
                    raw: available && hit.index < currentPoints.intensity.count
                        ? currentPoints.intensity[hit.index] : nil, position: p,
                    displayClass: hit.index < classes.count
                        ? classes[hit.index] : PointClass.unclassified, depth: hit.depth)
            })
    }
}

// MARK: - Save keys

extension AnnotationSession {
    /// What S and X do, in whichever mode is active: save the current work,
    /// and with `advance`, step to the next frame once it is saved.
    ///
    /// Returns false when something was refused: the save, or the step. In
    /// physical mode a draft with nothing unsaved is not a failed save, so X
    /// still moves on.
    @discardableResult func saveCurrent(advance: Bool) async -> Bool {
        switch workMode {
        case .points: guard save() else { return false }
        case .physical: if physical.isDirty { guard await physical.save() else { return false } }
        }
        guard advance else { return true }
        guard sampleIndex < samples.count - 1 else { return false }
        return stepForward() == nil
    }
}
