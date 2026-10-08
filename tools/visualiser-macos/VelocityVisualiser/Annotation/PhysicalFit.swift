// PhysicalFit.swift
// A physical reference fitted to an object's reviewed returns
// (docs/plans/lidar-physical-fit-to-points-plan.md): the service's answer, how
// it enters the draft, and the rules that replace the evidence pickers.
//
// A fit is an ordinary proposal. The draft takes it as one edit, one undo
// step, and it is saved and reviewed like anything typed by hand. The operator
// no longer chooses observed, inferred or prior for each field: the fit states
// how each component is known, and a value set by hand takes the status its
// action implies.

import Foundation

/// One named contribution to a fitted bound, in metres.
struct PhysicalFitBoundTerm: Codable, Equatable {
    var name: String
    var m: Double
}

/// What one frame shows of one face: whether the returns reach a real face,
/// why, and its bound with the terms it adds up.
struct PhysicalFitFace: Codable, Equatable {
    var seen: Bool
    var reason: String
    var atM: Double
    var boundM: Double
    var terms: [PhysicalFitBoundTerm]?

    enum CodingKeys: String, CodingKey {
        case seen
        case reason
        case atM = "at_m"
        case boundM = "bound_m"
        case terms
    }
}

/// One frame's diagnostics from the fit.
struct PhysicalFitFrame: Codable, Equatable {
    var sampleID: Int
    var returns: Int
    var rangeM: Double
    var aspectDeg: Double
    var yawRad: Double
    var yawBoundRad: Double
    var yawSource: String
    var axis: String
    var front: PhysicalFitFace
    var rear: PhysicalFitFace
    var left: PhysicalFitFace
    var right: PhysicalFitFace
    var lengthSpanM: Double
    var widthSpanM: Double
    var heightSpanM: Double
    var widthHow: String?
    var detached: [String]?
    var usedFor: [String]?
    var skipped: String?

    enum CodingKeys: String, CodingKey {
        case sampleID = "sample_id"
        case returns
        case rangeM = "range_m"
        case aspectDeg = "aspect_deg"
        case yawRad = "yaw_rad"
        case yawBoundRad = "yaw_bound_rad"
        case yawSource = "yaw_source"
        case axis
        case front
        case rear
        case left
        case right
        case lengthSpanM = "length_span_m"
        case widthSpanM = "width_span_m"
        case heightSpanM = "height_span_m"
        case widthHow = "width_how"
        case detached
        case usedFor = "used_for"
        case skipped
    }
}

/// The import file the fit writes; only its objects enter the draft.
struct PhysicalFitImport: Codable, Equatable { var objects: [PhysicalObject] }

/// The service's answer to a fit: proposals for one object, the membership
/// they were fitted from, and what every frame showed.
struct PhysicalFitResult: Codable, Equatable {
    var objectID: String
    var membershipRevision: Int
    var membershipDigest: String
    var fitted: PhysicalFitImport
    var frames: [PhysicalFitFrame]
    var notes: [String]?

    enum CodingKeys: String, CodingKey {
        case objectID = "object_id"
        case membershipRevision = "membership_revision"
        case membershipDigest = "membership_digest"
        case fitted = "import"
        case frames
        case notes
    }

    var object: PhysicalObject? { fitted.objects.first { $0.objectID == objectID } }
    func frame(sampleID: Int) -> PhysicalFitFrame? { frames.first { $0.sampleID == sampleID } }
}

/// What a fit replaces in the draft.
enum PhysicalFitScope: Equatable {
    /// The size and every fitted pose.
    case object
    /// The pose at one frame, and the size only if the draft has none.
    case pose(sampleID: Int)
}

/// How a dimension is stated, without naming its evidence: end to end, a
/// lower bound, or not at all.
enum PhysicalDimensionChoice: String, CaseIterable, Equatable {
    case full
    case atLeast
    case unknown

    var label: String {
        switch self {
        case .full: return "Full"
        case .atLeast: return "At least"
        case .unknown: return "Unknown"
        }
    }
}

extension PhysicalDimension {
    var choice: PhysicalDimensionChoice {
        if status == .unknown { return .unknown }
        return span == .partial ? .atLeast : .full
    }
}

extension PhysicalDraft {
    /// The assumptions a record set by hand states, so a bound typed in the
    /// window never waits on a free-text field to be saved.
    static let manualAssumptions =
        "Set by hand in the annotation window: values read off the reviewed returns; each bound "
        + "is the tolerance stated beside it, a conservative limit and not a confidence."

    /// Places a fit in the draft and returns how many poses it placed. A fitted
    /// pose replaces any pose at its frame; other poses stay. Fitting the object
    /// replaces its size; fitting one pose keeps the draft's size when there is
    /// one, since the operator may have adjusted it.
    @discardableResult static func applyFit(
        _ fit: PhysicalFitResult, scope: PhysicalFitScope, to objects: inout [PhysicalObject]
    ) -> Int {
        guard let fitted = fit.object else { return 0 }
        let i = index(of: fit.objectID, in: &objects)
        var poses = fitted.keyframes
        switch scope {
        case .object: objects[i].body = fitted.body
        case .pose(let sampleID):
            poses = poses.filter { $0.sampleID == sampleID }
            if objects[i].body == nil { objects[i].body = fitted.body }
        }
        for pose in poses {
            objects[i].keyframes.removeAll { $0.sampleID == pose.sampleID }
            objects[i].keyframes.append(pose)
        }
        if objects[i].body == nil && objects[i].keyframes.isEmpty { objects.remove(at: i) }
        return poses.count
    }

    /// Sets how a dimension is stated. A full span set by hand is inferred from
    /// the frame it was read at, so it is not held to a heading it may not have;
    /// a lower bound can only be an observation, of that frame. A dimension
    /// that already has evidence keeps it.
    static func setChoice(
        _ choice: PhysicalDimensionChoice, of d: inout PhysicalDimension, citing sample: Int?
    ) {
        let cite = sample.map { PhysicalSupport(frames: [$0]) } ?? .none
        switch choice {
        case .unknown: d = PhysicalDimension()
        case .full:
            if d.status == .unknown || d.status == .priorOnly {
                d.status = .inferred
                d.support = cite
            }
            d.span = .full
        case .atLeast:
            if d.status != .observed || d.support.frameList.isEmpty {
                d.status = .observed
                d.support = cite
            }
            setSpan(.partial, of: &d)
        }
    }

    /// The one judgement left per end: whether the returns at this frame reach
    /// its real end. Seen is an observation of this frame. Not seen, the end
    /// follows from the body's length when that is known end to end, and is
    /// unknown otherwise.
    static func setEnd(
        seen: Bool, _ end: WritableKeyPath<PhysicalKeyframe, PhysicalEndpoint>,
        of k: inout PhysicalKeyframe, body: PhysicalBody?
    ) {
        guard k.yaw.axis == .resolved else {
            k[keyPath: end] = PhysicalEndpoint()
            return
        }
        if seen {
            k[keyPath: end] = PhysicalEndpoint(
                status: .observed, support: PhysicalSupport(frames: [k.sampleID]))
        } else if let length = body?.length, length.span == .full, length.status.scorable {
            let frames = Set(length.support.frameList + [k.sampleID]).sorted()
            k[keyPath: end] = PhysicalEndpoint(
                status: .inferred, support: PhysicalSupport(frames: frames))
        } else {
            k[keyPath: end] = PhysicalEndpoint()
        }
    }

    /// Sets one coordinate of a keyframe's position. A position typed at a
    /// frame is an observation of that frame, as a click is.
    static func setPosition(
        _ field: WritableKeyPath<PhysicalPosition, Double?>, _ value: Double?,
        of k: inout PhysicalKeyframe
    ) {
        k.position[keyPath: field] = value
        if k.position.status == .unknown, value != nil {
            k.position.status = .observed
            k.position.support = PhysicalSupport(frames: [k.sampleID])
        }
    }

    /// States the manual assumptions on every hand-made record that declares
    /// bounds and states none. Fitted, imported and seeded records state their
    /// own and are left alone.
    static func fillManualAssumptions(_ objects: inout [PhysicalObject]) {
        func fill(_ review: inout PhysicalReview, declares: Bool) {
            guard declares, review.method == "manual_box",
                (review.uncertaintyAssumptions ?? "").trimmingCharacters(
                    in: .whitespacesAndNewlines
                ).isEmpty
            else { return }
            review.uncertaintyAssumptions = manualAssumptions
        }
        for i in objects.indices {
            if let declares = objects[i].body?.declaresBounds {
                fill(&objects[i].body!.review, declares: declares)
            }
            for k in objects[i].keyframes.indices {
                fill(
                    &objects[i].keyframes[k].review,
                    declares: objects[i].keyframes[k].declaresBounds)
            }
        }
    }
}
