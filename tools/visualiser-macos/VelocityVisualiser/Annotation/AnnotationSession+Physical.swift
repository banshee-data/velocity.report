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
        case .features:
            // Acceptance is explicit in the feature pane, never a generic save key.
            return false
        // Read-only: nothing to save, and X only steps.
        case .compare: guard advance else { return false }
        }
        guard advance else { return true }
        guard sampleIndex < samples.count - 1 else { return false }
        return stepForward() == nil
    }
}

// MARK: - Drag handles

/// A draggable part of the active keyframe in the Top view, and the
/// constraint the drag keeps.
enum PhysicalHandle: Equatable {
    /// Moves the anchor; the body's size and the yaw are held.
    case move
    /// Turns the yaw about the centre (or the anchor); position and size are held.
    case turn
    /// Revises the body's persistent length along the axis; the anchor is
    /// held. A body change resets every keyframe's review when saved.
    case length
}

enum PhysicalHandles {
    /// How far the axis arrow reaches on screen, and so where its handle is.
    static let axisReach: CGFloat = 34
    /// How close a press must be to take a handle, in points.
    static let pickRadius: CGFloat = 7

    /// Where the handles are on screen, for drawing and for hit testing.
    static func positions(
        _ g: PhysicalGeometry, basis: OrthoViewBasis, viewport: OrthoViewport
    ) -> (move: CGPoint?, turn: CGPoint?, length: CGPoint?) {
        func screen(_ x: Double, _ y: Double) -> CGPoint {
            viewport.screenPoint(from: basis.project(simd_float3(Float(x), Float(y), 0)))
        }
        let move = g.anchorPoint.map { screen($0.x, $0.y) }
        guard let yaw = g.yaw, let origin = g.centre ?? g.anchorPoint else {
            return (move, nil, nil)
        }
        let o = screen(origin.x, origin.y)
        let t = screen(origin.x + cos(yaw.rad), origin.y + sin(yaw.rad))
        let n = max(hypot(t.x - o.x, t.y - o.y), 0.001)
        let turn = CGPoint(
            x: o.x + (t.x - o.x) / n * axisReach, y: o.y + (t.y - o.y) / n * axisReach)
        let length = lengthHandlePoint(g).map { screen($0.x, $0.y) }
        return (move, turn, length)
    }

    /// Where the length handle sits: the end of the body farthest from the
    /// anchor along the axis, when a full-span length is known. The ends are
    /// the derived ones (centre, yaw and length), not the evidenced bumpers:
    /// a handle revises the belief, and needs no observation of the end it
    /// pulls. A face anchor on a side, or an axis without a front, has no
    /// such end.
    static func lengthHandlePoint(_ g: PhysicalGeometry) -> PhysicalPlanar? {
        guard let yaw = g.yaw, yaw.axis == .resolved, g.length != nil, g.ends.count == 2 else {
            return nil
        }
        switch g.anchorKind {
        case .bodyCentre, .rearFace: return g.ends[0]
        case .frontFace: return g.ends[1]
        case .leftFace, .rightFace: return nil
        }
    }

    /// The length a drag to `pointer` asks for: the distance from the held
    /// anchor to the pointer's projection on the axis, doubled for a centre
    /// anchor, whose two ends move together.
    static func draggedLength(
        anchor: PhysicalPlanar, anchorKind: PhysicalAnchorKind, yawRad: Double, pointer: simd_float2
    ) -> Double {
        let along =
            (Double(pointer.x) - anchor.x) * cos(yawRad) + (Double(pointer.y) - anchor.y)
            * sin(yawRad)
        let reach = abs(along)
        return max(anchorKind == .bodyCentre ? reach * 2 : reach, 0.1)
    }
}

extension AnnotationSession {
    /// The active keyframe's geometry as the overlay draws it.
    var physicalPreview: PhysicalGeometry? {
        guard let objectID = activeObjectID, let k = physicalKeyframe,
            let object = physical.object(objectID)
        else { return nil }
        return PhysicalGeometry.derive(object: object, keyframe: k, gate: .preview)
    }

    /// The handle under a press in the Top view, if any. The turn handle wins
    /// a tie: it sits on the end of the arrow drawn from the move handle.
    func physicalHandle(at screen: CGPoint, viewport: OrthoViewport) -> PhysicalHandle? {
        guard workMode == .physical, physical.canEdit, let g = physicalPreview else { return nil }
        let at = PhysicalHandles.positions(g, basis: basis(.top), viewport: viewport)
        func near(_ p: CGPoint?) -> Bool {
            p.map { hypot($0.x - screen.x, $0.y - screen.y) <= PhysicalHandles.pickRadius } ?? false
        }
        if near(at.length) { return .length }
        if near(at.turn) { return .turn }
        if near(at.move) { return .move }
        return nil
    }

    func beginPhysicalDrag() { physical.beginGesture() }

    /// Follows a drag of `handle` from where it began to a view-plane point
    /// in the Top view. A move follows the pointer's travel, so taking the
    /// handle off-centre does not make the anchor jump.
    func updatePhysicalDrag(
        _ handle: PhysicalHandle, from startPoint: simd_float2, to viewPoint: simd_float2
    ) {
        guard let objectID = activeObjectID, let sample = currentSample, let g = physicalPreview,
            let start = physical.gestureStartKeyframe(objectID: objectID, sampleID: sample.sampleID)
        else { return }
        let world = worldPoint(in: .top, viewPoint: viewPoint)
        let from = worldPoint(in: .top, viewPoint: startPoint)
        let origin = g.centre ?? g.anchorPoint
        if handle == .length {
            guard let anchor = g.anchorPoint, let yaw = g.yaw,
                let startBody = physical.gestureStartBody(objectID: objectID),
                startBody.length.bounded
            else { return }
            let length = PhysicalHandles.draggedLength(
                anchor: anchor, anchorKind: g.anchorKind, yawRad: yaw.rad,
                pointer: simd_float2(world.x, world.y))
            physical.updateGesture { objects in
                PhysicalDraft.updateBody(objectID: objectID, in: &objects) {
                    PhysicalDraft.setLength(length, of: &$0.length, from: startBody.length)
                }
            }
            return
        }
        physical.updateGesture { objects in
            PhysicalDraft.updateKeyframe(
                objectID: objectID, sampleID: sample.sampleID, in: &objects
            ) { k in
                switch handle {
                case .move:
                    guard let x = start.position.xM, let y = start.position.yM else { return }
                    k.position.xM = x + Double(world.x - from.x)
                    k.position.yM = y + Double(world.y - from.y)
                case .turn:
                    guard let origin, k.yaw.axis != .unknown else { return }
                    let rad = atan2(Double(world.y) - origin.y, Double(world.x) - origin.x)
                    k.yaw.yawRad = PhysicalUnits.radians(
                        PhysicalUnits.wrappedDegrees(PhysicalUnits.degrees(rad)))
                case .length: break
                }
            }
        }
    }

    func endPhysicalDrag() { physical.endGesture() }
}
