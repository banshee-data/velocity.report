// PhysicalFieldErrors.swift
// Places the service's refusals beside the controls that set the fields they
// name, so a required value that is missing is reported where it is typed.
//
// The pilot stalled twice on fields that blocked a save while the reason
// appeared at the top and bottom of the column. The service now says where
// each refusal falls (PhysicalLocation); this maps that to the pane. The
// mapping reads only the structured location, never the message text, and a
// refusal it cannot place on screen stays in the save section in full.

import Foundation

/// A control in the physical-reference pane a refusal can sit beside.
enum PhysicalPaneField: Hashable, CaseIterable {
    /// The size record as a whole: its id or axis convention.
    case size
    case length
    case width
    case height
    /// The size's method and uncertainty assumptions.
    case sizeReview
    /// The pose record as a whole: its sample, capture time or id.
    case pose
    case front
    case rear
    case heading
    case anchor
    case position
    case sharedErrors
    /// The pose's method and uncertainty assumptions.
    case poseReview

    /// Inside the pose's "Adjust by hand" group, which starts closed; the
    /// pane holds it open while any of these is refused.
    var isUnderAdjust: Bool {
        switch self {
        case .heading, .anchor, .position, .sharedErrors, .poseReview: return true
        default: return false
        }
    }

    /// Where the operator finds it, for the pointer beside Save.
    var label: String {
        switch self {
        case .size: return "1 · Object size"
        case .length: return "Length"
        case .width: return "Width"
        case .height: return "Height"
        case .sizeReview: return "the size's method and assumptions"
        case .pose: return "2 · Pose at this frame"
        case .front: return "Front"
        case .rear: return "Rear"
        case .heading: return "the heading under Adjust by hand"
        case .anchor: return "the point fixed to the body under Adjust by hand"
        case .position: return "Position under Adjust by hand"
        case .sharedErrors: return "Shared errors under Adjust by hand"
        case .poseReview: return "the pose's method and assumptions under Adjust by hand"
        }
    }

    /// The pane control for a located refusal, or nil when the pane has no
    /// control for it: a following record, or the object as a whole.
    static func field(for location: PhysicalLocation) -> PhysicalPaneField? {
        let head = (location.field ?? "").split(separator: ".").first.map(String.init) ?? ""
        switch location.record {
        case "body":
            switch head {
            case "length": return .length
            case "width": return .width
            case "height": return .height
            case "review": return .sizeReview
            default: return .size
            }
        case "keyframe":
            switch head {
            case "front": return .front
            case "rear": return .rear
            case "yaw": return .heading
            case "anchor": return .anchor
            case "position": return .position
            case "shared_errors": return .sharedErrors
            case "review": return .poseReview
            default: return .pose
            }
        default: return nil
        }
    }
}

/// One refusal: the service's message, and the part of it to show at a field.
struct PhysicalRefusal: Hashable {
    /// The service's whole message, as it would be logged.
    var raw: String
    /// The refusal without its location, for display beside the field;
    /// the whole message when the service gave no location.
    var detail: String
    var location: PhysicalLocation?
}

/// A validation's refusals, split into those the pane can show beside a
/// control of the records on screen and those it cannot.
struct PhysicalFieldErrors: Equatable {
    private(set) var byField: [PhysicalPaneField: [PhysicalRefusal]] = [:]
    /// Refusals with no control on screen: a document-level refusal, one in
    /// another object or another frame's pose, a following record, or any
    /// refusal from a service that does not locate them. Shown in full at
    /// Save, as before.
    private(set) var elsewhere: [PhysicalRefusal] = []

    var isEmpty: Bool { byField.isEmpty && elsewhere.isEmpty }

    func at(_ field: PhysicalPaneField) -> [PhysicalRefusal] { byField[field] ?? [] }

    /// The fields that hold a refusal, in the order the pane shows them.
    var placedFields: [PhysicalPaneField] {
        PhysicalPaneField.allCases.filter { byField[$0] != nil }
    }

    /// Whether the Adjust group holds a refusal and must stay open.
    var needsAdjust: Bool { placedFields.contains { $0.isUnderAdjust } }

    /// Places a validation's refusals for the object and frame on screen.
    /// A pose's refusal is placed only at its own sample, since the pane
    /// shows one frame's pose.
    static func place(
        _ result: PhysicalEditResult?, objectID: String?, sampleID: Int?
    ) -> PhysicalFieldErrors {
        var out = PhysicalFieldErrors()
        guard let result, !result.valid else { return out }
        func add(_ refusal: PhysicalRefusal) {
            if let location = refusal.location, let field = PhysicalPaneField.field(for: location),
                location.objectID != nil, location.objectID == objectID,
                location.record != "keyframe"
                    || (location.sampleID != nil && location.sampleID == sampleID)
            {
                out.byField[field, default: []].append(refusal)
            } else {
                out.elsewhere.append(refusal)
            }
        }
        if let invalid = result.invalid {
            add(
                PhysicalRefusal(
                    raw: invalid, detail: result.invalidAt?.detail ?? invalid,
                    location: result.invalidAt))
        }
        for problem in result.linkProblems {
            let raw = "\(problem.record): \(problem.problem)"
            add(PhysicalRefusal(raw: raw, detail: problem.at?.detail ?? raw, location: problem.at))
        }
        return out
    }
}
