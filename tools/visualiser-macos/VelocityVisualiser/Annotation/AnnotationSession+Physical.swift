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
    /// Revises width about the centre; position, yaw and length are held.
    case width
}

enum PhysicalHandles {
    /// How far the axis arrow reaches on screen, and so where its handle is.
    static let axisReach: CGFloat = 34
    /// How close a press must be to take a handle, in points.
    static let pickRadius: CGFloat = 7

    /// Where the handles are on screen, for drawing and for hit testing.
    static func positions(
        _ g: PhysicalGeometry, basis: OrthoViewBasis, viewport: OrthoViewport
    ) -> (move: CGPoint?, turn: CGPoint?, length: CGPoint?, width: CGPoint?) {
        func screen(_ x: Double, _ y: Double) -> CGPoint {
            viewport.screenPoint(from: basis.project(simd_float3(Float(x), Float(y), 0)))
        }
        let move = g.anchorPoint.map { screen($0.x, $0.y) }
        guard let origin = g.centre ?? g.anchorPoint else { return (move, nil, nil, nil) }
        let o = screen(origin.x, origin.y)
        // An unset axis gets a tool handle, not a claimed heading. Dragging it
        // explicitly authors the axis; no angle or bound is prefilled.
        let rad = g.yaw?.rad ?? 0
        let t = screen(origin.x + cos(rad), origin.y + sin(rad))
        let n = max(hypot(t.x - o.x, t.y - o.y), 0.001)
        let turn = CGPoint(
            x: o.x + (t.x - o.x) / n * axisReach, y: o.y + (t.y - o.y) / n * axisReach)
        let length = lengthHandlePoint(g).map { screen($0.x, $0.y) }
        let width = widthHandlePoint(g).map { screen($0.x, $0.y) }
        return (move, turn, length, width)
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

    /// Width is centred on the known body centre. A side-face anchor needs
    /// an explicitly coupled offset change, so it has no width handle here.
    static func widthHandlePoint(_ g: PhysicalGeometry) -> PhysicalPlanar? {
        guard let centre = g.centre, let yaw = g.yaw, let width = g.width,
            g.anchorKind != .leftFace, g.anchorKind != .rightFace
        else { return nil }
        return PhysicalPlanar(
            x: centre.x - sin(yaw.rad) * width.value / 2,
            y: centre.y + cos(yaw.rad) * width.value / 2, bound: centre.bound)
    }

    static func draggedWidth(centre: PhysicalPlanar, yawRad: Double, pointer: simd_float2) -> Double
    {
        let across =
            -(Double(pointer.x) - centre.x) * sin(yawRad) + (Double(pointer.y) - centre.y)
            * cos(yawRad)
        return max(abs(across) * 2, 0.1)
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
        return PhysicalGeometry.derive(object: object, keyframe: k, gate: .authoring)
    }

    /// The handle under a press in the Top view, if any: the nearest one
    /// within reach. Zoomed out, the width and length handles crowd the
    /// anchor, and taking them in a fixed order let a resize capture a press
    /// aimed at the move handle beside it.
    ///
    /// An exact tie goes to the turn handle, which sits on the end of the
    /// arrow drawn from the move handle, and then to move: a body resize
    /// resets every keyframe's review when saved, so it is never the guess.
    func physicalHandle(at screen: CGPoint, viewport: OrthoViewport) -> PhysicalHandle? {
        guard workMode == .physical, physical.canEdit, let g = physicalPreview else { return nil }
        let at = PhysicalHandles.positions(g, basis: basis(.top), viewport: viewport)
        let ranked: [(PhysicalHandle, CGPoint?)] = [
            (.turn, at.turn), (.move, at.move), (.length, at.length), (.width, at.width),
        ]
        var nearest: (handle: PhysicalHandle, distance: Double)?
        for case (let handle, let p?) in ranked {
            let distance = Double(hypot(p.x - screen.x, p.y - screen.y))
            guard distance <= Double(PhysicalHandles.pickRadius),
                distance < (nearest?.distance ?? .infinity)
            else { continue }
            nearest = (handle, distance)
        }
        return nearest?.handle
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
        // A value-only size is drawn as a sketch with its handles, so the
        // handles revise it as well; its missing bounds stay missing.
        if handle == .width {
            guard let centre = g.centre, let yaw = g.yaw,
                let startBody = physical.gestureStartBody(objectID: objectID),
                PhysicalDraft.isDraggable(startBody.width),
                PhysicalHandles.widthHandlePoint(g) != nil
            else { return }
            let width = PhysicalHandles.draggedWidth(
                centre: centre, yawRad: yaw.rad, pointer: simd_float2(world.x, world.y))
            physical.updateGesture { objects in
                PhysicalDraft.updateBody(objectID: objectID, in: &objects) {
                    PhysicalDraft.dragDimension(width, of: &$0.width, from: startBody.width)
                }
            }
            return
        }
        if handle == .length {
            guard let anchor = g.anchorPoint, let yaw = g.yaw,
                let startBody = physical.gestureStartBody(objectID: objectID),
                PhysicalDraft.isDraggable(startBody.length)
            else { return }
            let length = PhysicalHandles.draggedLength(
                anchor: anchor, anchorKind: g.anchorKind, yawRad: yaw.rad,
                pointer: simd_float2(world.x, world.y))
            physical.updateGesture { objects in
                PhysicalDraft.updateBody(objectID: objectID, in: &objects) {
                    PhysicalDraft.dragDimension(length, of: &$0.length, from: startBody.length)
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
                    // About where the body's centre was when the drag began,
                    // which every update starts from. Turning about the live
                    // centre chased a point that the turn itself moved, and a
                    // fitted pose anchored on its front face swung about that
                    // face instead of its middle.
                    guard let pivot = PhysicalDraft.turnPivot(of: k) else { return }
                    if k.yaw.axis == .unknown { PhysicalDraft.setAxis(.frontRearAmbiguous, of: &k) }
                    PhysicalDraft.turn(
                        &k, toRad: atan2(Double(world.y) - pivot.y, Double(world.x) - pivot.x))
                case .length, .width: break
                }
            }
        }
    }

    func endPhysicalDrag() { physical.endGesture() }
}

extension PhysicalDraft {
    /// What a turn pivots about: the body centre when the keyframe places it,
    /// otherwise the anchor itself. A face anchor places the centre when the
    /// axis is resolved and its offset is known; the offset's bound is not
    /// needed to know where the middle is.
    static func turnPivot(of k: PhysicalKeyframe) -> (x: Double, y: Double)? {
        guard let x = k.position.xM, let y = k.position.yM, x.isFinite, y.isFinite else {
            return nil
        }
        guard k.anchor.kind.isFace, k.yaw.axis == .resolved, let yaw = k.yaw.yawRad,
            let offset = k.anchor.offsetM, offset.isFinite, offset > 0
        else { return (x, y) }
        let n = PhysicalGeometry.inwardNormal(k.anchor.kind, yaw: yaw)
        return (x + offset * n.x, y + offset * n.y)
    }

    /// Sets the heading, keeping the body's centre where it is. A face anchor
    /// is a point on the body, so it moves round the centre with the turn; a
    /// centre anchor stays put. The anchor's bound and status are unchanged:
    /// a turn says where the face is, as a drag of the anchor would.
    static func turn(_ k: inout PhysicalKeyframe, toRad rad: Double) {
        let pivot = turnPivot(of: k)
        let wrapped = PhysicalUnits.radians(
            PhysicalUnits.wrappedDegrees(PhysicalUnits.degrees(rad)))
        k.yaw.yawRad = wrapped
        guard let pivot, k.anchor.kind.isFace, k.yaw.axis == .resolved,
            let offset = k.anchor.offsetM, offset.isFinite, offset > 0
        else { return }
        let n = PhysicalGeometry.inwardNormal(k.anchor.kind, yaw: wrapped)
        k.position.xM = pivot.x - offset * n.x
        k.position.yM = pivot.y - offset * n.y
    }
}

// MARK: - The 3D view

extension AnnotationSession {
    /// The physical references at this frame as boxes in the 3D view, styled
    /// as the orthographic views style them. Only a keyframe that establishes
    /// a whole footprint is drawn, as in the views.
    ///
    /// A reference has a footprint and, at most, a height; it has no floor.
    /// The box stands on the lowest of the object's returns in this frame,
    /// or on the column grid's ground when it has none here, and is as tall
    /// as the body's height, or its returns when that is not known.
    var physicalSceneBoxes: [MetalRenderer.OverlayBox] {
        guard workMode == .physical, let sample = currentSample else { return [] }
        var boxes: [MetalRenderer.OverlayBox] = []
        for object in physical.draft {
            guard let k = object.keyframe(sampleID: sample.sampleID),
                let box = PhysicalGeometry.derive(object: object, keyframe: k, gate: .authoring).box
            else { continue }
            let style = PhysicalReferenceOverlay.style(
                draft: k,
                saved: physical.savedKeyframe(objectID: object.objectID, sampleID: sample.sampleID),
                body: object.body, savedBody: physical.savedBody(objectID: object.objectID))
            let indices =
                object.objectID == activeObjectID
                ? history.current
                : Set(
                    sidecar.mask(objectID: object.objectID, sampleID: sample.sampleID)?.pointIndices
                        ?? [])
            let span = AnnotationSession.heightSpan(of: indices, in: currentPoints)
            let base = span?.lowerBound ?? columnGrid.groundZ
            let stated: Float? = object.body.flatMap { body in
                switch body.height.choice {
                case .full: return body.height.best.map { Float($0.value) }
                case .atLeast: return body.height.lowerM.map(Float.init)
                case .unknown: return nil
                }
            }
            let top = max(base + (stated ?? 0), span?.upperBound ?? base + (stated ?? 1.5))
            let active = object.objectID == activeObjectID
            boxes.append(
                MetalRenderer.OverlayBox(
                    base: simd_float3(Float(box.centreX), Float(box.centreY), base),
                    yawRad: Float(box.yawRad), length: Float(box.length), width: Float(box.width),
                    height: max(top - base, 0.1), colour: simd_float4(style.rgb, active ? 1 : 0.6)))
        }
        return boxes
    }

    /// The lowest and highest of these returns, or nil when there are none.
    static func heightSpan(of indices: Set<Int>, in points: PackPoints) -> ClosedRange<Float>? {
        var lo = Float.greatestFiniteMagnitude
        var hi = -Float.greatestFiniteMagnitude
        for index in indices {
            guard let p = points.point(at: index), p.z.isFinite else { continue }
            lo = min(lo, p.z)
            hi = max(hi, p.z)
        }
        return lo <= hi ? lo...hi : nil
    }
}
