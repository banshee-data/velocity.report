// PhysicalReferenceGeometry.swift
// What a keyframe establishes: the body centre, the bumpers and the box, each
// with its bound, or the reason it cannot be had.
//
// This mirrors Go's PhysicalObject.Geometry (physical_geometry.go) line for
// line, so the overlay an operator draws against and the geometry the scorer
// uses cannot disagree. A shared fixture holds the two to each other
// (PhysicalReferenceGeometryTests). Nothing is scored here.
//
// Go derives dimensions only from a reviewed, independent body, because that
// is what may be scored. An operator editing a draft needs to see the draft,
// so `gate: .preview` derives from the body as it stands and the overlay
// labels the result as a proposal. `.truth` is Go's rule exactly.

import Foundation

enum PhysicalGeometryGate {
    /// Go's rule: dimensions only from a reviewed, independent body.
    case truth
    /// Dimensions from the body as drafted, for drawing a proposal.
    case preview
    /// Editable sketch only: missing bounds stay unknown in the document.
    /// Never use this gate for comparison, review, or scoring.
    case authoring
}

struct PhysicalPlanar: Equatable {
    var x: Double
    var y: Double
    var bound: Double
}

struct PhysicalAngle: Equatable {
    var rad: Double
    var boundRad: Double
    var axis: PhysicalAxisState
    var status: PhysicalEvidence
}

struct PhysicalLinear: Equatable {
    var lower: Double
    var upper: Double
    var value: Double
    var halfWidth: Double
    var status: PhysicalEvidence
}

struct PhysicalBox: Equatable {
    var centreX: Double
    var centreY: Double
    var yawRad: Double
    var length: Double
    var width: Double

    /// The footprint's corners, anticlockwise from front-left.
    var corners: [(x: Double, y: Double)] {
        let c = cos(yawRad)
        let s = sin(yawRad)
        let hl = length / 2
        let hw = width / 2
        return [(hl, hw), (-hl, hw), (-hl, -hw), (hl, -hw)].map { (dx, dy) in
            (centreX + dx * c - dy * s, centreY + dx * s + dy * c)
        }
    }
}

/// Reasons a derived component is unavailable, as Go spells them.
enum PhysicalUnavailable {
    static let lowerBoundOnly = "lower_bound_only"
    static let noBody = "no_body"
    static let bodyUnreviewed = "body_unreviewed"
    static let bodyTrackerAssisted = "body_tracker_assisted"
    static let axisUnknown = "axis_unknown"
    static let axisAmbiguous = "axis_ambiguous"
    static let anchorOffsetUnknown = "anchor_offset_unknown"
    static let position = "position_unavailable"
    static let yaw = "yaw_unavailable"
    static let centre = "centre_unavailable"
    static let length = "length_unavailable"
    static let incompleteBox = "incomplete_box"

    /// Operator wording for a reason.
    static func describe(_ reason: String?) -> String {
        switch reason {
        case nil: return ""
        case lowerBoundOnly: return "only a lower bound"
        case noBody: return "no body"
        case bodyUnreviewed: return "body not reviewed"
        case bodyTrackerAssisted: return "body tracker-assisted"
        case axisUnknown: return "axis unknown"
        case axisAmbiguous: return "front/rear ambiguous"
        case anchorOffsetUnknown: return "face offset unknown"
        case position: return "no position"
        case yaw: return "no yaw"
        case centre: return "no centre"
        case length: return "no length"
        case incompleteBox: return "box incomplete"
        case "unknown": return "unknown"
        case "prior_only": return "prior only"
        default: return reason ?? ""
        }
    }
}

struct PhysicalGeometry: Equatable {
    var anchorKind: PhysicalAnchorKind
    /// Components drawn without a stated bound; zero drawing radius is not precision.
    var unboundedDraft: Bool = false
    var usesPriorSize: Bool = false
    var anchorPoint: PhysicalPlanar?
    var anchorUnavailable: String?
    var centre: PhysicalPlanar?
    var centreUnavailable: String?
    var yaw: PhysicalAngle?
    var yawUnavailable: String?
    var length: PhysicalLinear?
    var lengthUnavailable: String?
    var width: PhysicalLinear?
    var widthUnavailable: String?
    var height: PhysicalLinear?
    var heightUnavailable: String?
    var front: PhysicalPlanar?
    var frontUnavailable: String?
    var rear: PhysicalPlanar?
    var rearUnavailable: String?
    /// Both bumpers without saying which is which.
    var ends: [PhysicalPlanar] = []
    var box: PhysicalBox?
    var boxUnavailable: String?

    static func derive(
        object: PhysicalObject, keyframe k: PhysicalKeyframe, gate: PhysicalGeometryGate = .truth
    ) -> PhysicalGeometry {
        var g = PhysicalGeometry(anchorKind: k.anchor.kind)
        if k.position.status.scorable, let x = k.position.xM, let y = k.position.yM,
            let bound = k.position.boundM, x.isFinite, y.isFinite, bound.isFinite, bound >= 0
        {
            g.anchorPoint = PhysicalPlanar(x: x, y: y, bound: bound)
        } else {
            g.anchorUnavailable = k.position.status.rawValue
        }
        if k.yaw.axis == .unknown {
            g.yawUnavailable = PhysicalUnavailable.axisUnknown
        } else if !k.yaw.status.scorable {
            g.yawUnavailable = k.yaw.status.rawValue
        } else if let rad = k.yaw.yawRad, let bound = k.yaw.boundRad, rad.isFinite, bound.isFinite,
            bound >= 0, gate != .authoring || bound <= .pi
        {
            g.yaw = PhysicalAngle(rad: rad, boundRad: bound, axis: k.yaw.axis, status: k.yaw.status)
        } else {
            g.yawUnavailable = PhysicalUnavailable.yaw
        }
        if gate == .authoring {
            if g.anchorPoint == nil, k.position.status != .unknown, let x = k.position.xM,
                let y = k.position.yM, x.isFinite, y.isFinite, k.position.boundM == nil
            {
                g.anchorPoint = PhysicalPlanar(x: x, y: y, bound: 0)
                g.anchorUnavailable = nil
                g.unboundedDraft = true
            }
            if g.yaw == nil, k.yaw.axis != .unknown, k.yaw.status != .unknown,
                let rad = k.yaw.yawRad, rad.isFinite, k.yaw.boundRad == nil
            {
                g.yaw = PhysicalAngle(rad: rad, boundRad: 0, axis: k.yaw.axis, status: k.yaw.status)
                g.yawUnavailable = nil
                g.unboundedDraft = true
            }
        }
        g.applyBody(object.body, gate: gate)
        g.applyCentre(k.anchor)
        (g.front, g.frontUnavailable) = g.endpoint(k.front, sign: 1, face: .frontFace)
        (g.rear, g.rearUnavailable) = g.endpoint(k.rear, sign: -1, face: .rearFace)
        if g.centre != nil, g.yaw != nil, g.length != nil {
            g.ends = [g.alongAxis(1), g.alongAxis(-1)]
        }
        if let c = g.centre, let yaw = g.yaw, let length = g.length, let width = g.width {
            g.box = PhysicalBox(
                centreX: c.x, centreY: c.y, yawRad: yaw.rad, length: length.value,
                width: width.value)
        } else {
            g.boxUnavailable = PhysicalUnavailable.incompleteBox
        }
        return g
    }

    private mutating func applyBody(_ body: PhysicalBody?, gate: PhysicalGeometryGate) {
        var reason: String?
        switch (body, gate) {
        case (nil, _): reason = PhysicalUnavailable.noBody
        case (let b?, .truth) where b.review.origin != .independent:
            reason = PhysicalUnavailable.bodyTrackerAssisted
        case (let b?, .truth) where b.review.status != .reviewed:
            reason = PhysicalUnavailable.bodyUnreviewed
        default: break
        }
        if let reason {
            lengthUnavailable = reason
            widthUnavailable = reason
            heightUnavailable = reason
            return
        }
        guard let body else { return }
        if gate == .authoring {
            let dimensions = [body.length, body.width, body.height]
            unboundedDraft =
                unboundedDraft
                || dimensions.contains {
                    $0.status != .unknown && $0.span != .partial && $0.valueM != nil && !$0.bounded
                }
            usesPriorSize = dimensions.contains { $0.status == .priorOnly }
        }
        (length, lengthUnavailable) = Self.linear(body.length, gate: gate)
        (width, widthUnavailable) = Self.linear(body.width, gate: gate)
        (height, heightUnavailable) = Self.linear(body.height, gate: gate)
    }

    private static func linear(
        _ d: PhysicalDimension, gate: PhysicalGeometryGate
    ) -> (PhysicalLinear?, String?) {
        guard d.status.scorable || (gate == .authoring && d.status == .priorOnly) else {
            return (nil, d.status.rawValue)
        }
        if gate == .authoring, d.span != .partial, !d.bounded, let value = d.valueM, value.isFinite,
            value >= 0
        {
            return (
                PhysicalLinear(
                    lower: value, upper: value, value: value, halfWidth: 0, status: d.status), nil
            )
        }
        guard d.span != .partial, let lo = d.lowerM, let hi = d.upperM, let best = d.best else {
            return (nil, PhysicalUnavailable.lowerBoundOnly)
        }
        return (
            PhysicalLinear(
                lower: lo, upper: hi, value: best.value, halfWidth: best.halfWidth, status: d.status
            ), nil
        )
    }

    private mutating func applyCentre(_ a: PhysicalAnchor) {
        guard let anchor = anchorPoint else {
            centreUnavailable = PhysicalUnavailable.position
            return
        }
        if a.kind == .bodyCentre {
            centre = anchor
            return
        }
        guard let offset = a.offsetM, let offsetBound = a.offsetBoundM else {
            centreUnavailable = PhysicalUnavailable.anchorOffsetUnknown
            return
        }
        guard let yaw else {
            centreUnavailable = PhysicalUnavailable.yaw
            return
        }
        guard yaw.axis == .resolved, offset.isFinite, offsetBound.isFinite, offset >= 0,
            offsetBound >= 0
        else {
            centreUnavailable = PhysicalUnavailable.anchorOffsetUnknown
            return
        }
        let n = Self.inwardNormal(a.kind, yaw: yaw.rad)
        centre = PhysicalPlanar(
            x: anchor.x + offset * n.x, y: anchor.y + offset * n.y,
            bound: anchor.bound + offsetBound + offset * Self.swing(yaw.boundRad))
        if let c = centre, !c.x.isFinite || !c.y.isFinite || !c.bound.isFinite {
            centre = nil
            centreUnavailable = PhysicalUnavailable.centre
        }
    }

    private func endpoint(
        _ e: PhysicalEndpoint, sign: Double, face: PhysicalAnchorKind
    ) -> (PhysicalPlanar?, String?) {
        if let yaw, yaw.axis == .frontRearAmbiguous {
            return (nil, PhysicalUnavailable.axisAmbiguous)
        }
        if yawUnavailable == PhysicalUnavailable.axisUnknown {
            return (nil, PhysicalUnavailable.axisUnknown)
        }
        if !e.status.scorable { return (nil, e.status.rawValue) }
        if yaw == nil { return (nil, PhysicalUnavailable.yaw) }
        if anchorKind == face, let anchorPoint { return (anchorPoint, nil) }
        if centre == nil { return (nil, PhysicalUnavailable.centre) }
        if length == nil { return (nil, PhysicalUnavailable.length) }
        return (alongAxis(sign), nil)
    }

    private func alongAxis(_ sign: Double) -> PhysicalPlanar {
        guard let centre, let yaw, let length else { return PhysicalPlanar(x: 0, y: 0, bound: 0) }
        let half = length.value / 2
        return PhysicalPlanar(
            x: centre.x + sign * half * cos(yaw.rad), y: centre.y + sign * half * sin(yaw.rad),
            bound: centre.bound + length.halfWidth / 2 + half * Self.swing(yaw.boundRad))
    }

    /// The displacement per metre of lever arm a yaw bound allows: the chord.
    static func swing(_ boundRad: Double) -> Double { 2 * sin(min(boundRad, .pi) / 2) }

    /// The unit vector from a face's centre towards the body centre. Mirrors
    /// Go's inwardNormal.
    static func inwardNormal(_ kind: PhysicalAnchorKind, yaw: Double) -> (x: Double, y: Double) {
        let c = cos(yaw)
        let s = sin(yaw)
        switch kind {
        case .frontFace: return (-c, -s)
        case .rearFace: return (c, s)
        case .leftFace: return (s, -c)
        case .rightFace: return (-s, c)
        case .bodyCentre: return (0, 0)
        }
    }
}
