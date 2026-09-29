// PhysicalReferenceModels.swift
// The physical-reference record, mirroring Go's internal/lidar/annotation
// physical.go field for field.
//
// A mask says which returns belong to an object; a physical reference says
// what is known about its body. The two are separate files with separate
// reviews. This client never writes physical-references.json itself: every
// edit goes through the local Go service (PhysicalReferenceAPIClient), which
// holds the evidence rules, the origin ledger and the revision protocol. These
// types exist to show a document and to send the edited objects back without
// losing a field, so every field Go writes for an object is here, with its
// explicit key.

import Foundation

/// How one component is known. See Go's EvidenceStatus.
enum PhysicalEvidence: String, Codable, CaseIterable, Equatable {
    case observed
    case inferred
    case priorOnly = "prior_only"
    case unknown

    var label: String {
        switch self {
        case .observed: return "Observed"
        case .inferred: return "Inferred"
        case .priorOnly: return "Prior only"
        case .unknown: return "Unknown"
        }
    }

    /// Independent evidence about the object: observed or inferred. A class
    /// prior is evidence about the class, and unknown is none.
    var scorable: Bool { self == .observed || self == .inferred }
}

/// Whether a record was authored without tracker output in view.
enum PhysicalOrigin: String, Codable, Equatable {
    case independent
    case trackerAssisted = "tracker_assisted"
}

/// The point a keyframe's position refers to.
enum PhysicalAnchorKind: String, Codable, CaseIterable, Equatable {
    case bodyCentre = "body_centre"
    case frontFace = "front_face"
    case rearFace = "rear_face"
    case leftFace = "left_face"
    case rightFace = "right_face"

    var label: String {
        switch self {
        case .bodyCentre: return "Body centre"
        case .frontFace: return "Front face"
        case .rearFace: return "Rear face"
        case .leftFace: return "Left face"
        case .rightFace: return "Right face"
        }
    }

    var isFace: Bool { self != .bodyCentre }
}

/// What is known of the body's orientation.
enum PhysicalAxisState: String, Codable, CaseIterable, Equatable {
    case resolved
    case frontRearAmbiguous = "front_rear_ambiguous"
    case unknown

    var label: String {
        switch self {
        case .resolved: return "Resolved (front known)"
        case .frontRearAmbiguous: return "Front/rear ambiguous"
        case .unknown: return "Unknown"
        }
    }
}

/// Whether a dimension's evidence covered it end to end.
enum PhysicalSpan: String, Codable, CaseIterable, Equatable {
    case full
    case partial
}

/// What a component rests on: frames of this pack, or an external reference.
struct PhysicalSupport: Codable, Equatable {
    var frames: [Int]?
    var external: String?

    enum CodingKeys: String, CodingKey {
        case frames
        case external
    }

    var frameList: [Int] { frames ?? [] }

    /// Adds or removes a frame, keeping the list sorted and unique, as Go
    /// requires it.
    mutating func toggle(frame: Int) {
        var set = Set(frameList)
        if set.contains(frame) { set.remove(frame) } else { set.insert(frame) }
        frames = set.isEmpty ? nil : set.sorted()
    }

    /// Nothing named: what an unknown component must carry.
    static let none = PhysicalSupport()
}

/// One body dimension. A partial span is a lower bound only.
struct PhysicalDimension: Codable, Equatable {
    var status: PhysicalEvidence = .unknown
    var span: PhysicalSpan?
    var lowerM: Double?
    var upperM: Double?
    var valueM: Double?
    var support: PhysicalSupport = .none

    enum CodingKeys: String, CodingKey {
        case status
        case span
        case lowerM = "lower_m"
        case upperM = "upper_m"
        case valueM = "value_m"
        case support
    }

    var bounded: Bool { lowerM != nil && upperM != nil }

    /// The stated value, or the interval's midpoint, and the half-width that
    /// covers the interval from it. Nil unless bounded. Mirrors Go's Best.
    var best: (value: Double, halfWidth: Double)? {
        guard let lo = lowerM, let hi = upperM else { return nil }
        let value = valueM ?? (lo + hi) / 2
        return (value, max(value - lo, hi - value))
    }
}

/// The record's own review and provenance, separate from membership review.
struct PhysicalReview: Codable, Equatable {
    var status: ReviewStatus = .proposed
    var origin: PhysicalOrigin = .independent
    var method: String = "manual_box"
    var trackerSource: String?
    var uncertaintyAssumptions: String?
    var provenance: Provenance = Provenance()

    enum CodingKeys: String, CodingKey {
        case status
        case origin
        case method
        case trackerSource = "tracker_source"
        case uncertaintyAssumptions = "uncertainty_assumptions"
        case provenance
    }
}

/// An object's persistent body belief, apart from any keyframe's pose.
struct PhysicalBody: Codable, Equatable {
    static let axisConvention = "x_front_y_left_z_up"

    var bodyID: String
    var axisConvention: String = PhysicalBody.axisConvention
    var length = PhysicalDimension()
    var width = PhysicalDimension()
    var height = PhysicalDimension()
    var review = PhysicalReview()

    enum CodingKeys: String, CodingKey {
        case bodyID = "body_id"
        case axisConvention = "axis_convention"
        case length
        case width
        case height
        case review
    }

    /// Whether this body's geometry claims anything. Go demands uncertainty
    /// assumptions of a record that does.
    var declaresBounds: Bool {
        length.status != .unknown || width.status != .unknown || height.status != .unknown
    }
}

/// The explicit point a keyframe's position refers to.
struct PhysicalAnchor: Codable, Equatable {
    var kind: PhysicalAnchorKind = .bodyCentre
    var offsetM: Double?
    var offsetBoundM: Double?

    enum CodingKeys: String, CodingKey {
        case kind
        case offsetM = "offset_m"
        case offsetBoundM = "offset_bound_m"
    }
}

/// The anchor's position in the pack's frame. The bound is horizontal; Z is
/// optional and unbounded.
struct PhysicalPosition: Codable, Equatable {
    var status: PhysicalEvidence = .unknown
    var xM: Double?
    var yM: Double?
    var zM: Double?
    var boundM: Double?
    var support: PhysicalSupport = .none

    enum CodingKeys: String, CodingKey {
        case status
        case xM = "x_m"
        case yM = "y_m"
        case zM = "z_m"
        case boundM = "bound_m"
        case support
    }
}

/// The body's orientation, radians anticlockwise from the pack's x axis.
struct PhysicalYaw: Codable, Equatable {
    var status: PhysicalEvidence = .unknown
    var axis: PhysicalAxisState = .unknown
    var yawRad: Double?
    var boundRad: Double?
    var support: PhysicalSupport = .none

    enum CodingKeys: String, CodingKey {
        case status
        case axis
        case yawRad = "yaw_rad"
        case boundRad = "bound_rad"
        case support
    }
}

/// How one bumper is known. Its position is derived, never stored.
struct PhysicalEndpoint: Codable, Equatable {
    var status: PhysicalEvidence = .unknown
    var support: PhysicalSupport = .none

    enum CodingKeys: String, CodingKey {
        case status
        case support
    }
}

/// One observation several components share.
struct PhysicalSharedError: Codable, Equatable {
    var observation: String
    var components: [String]
    var note: String?

    enum CodingKeys: String, CodingKey {
        case observation
        case components
        case note
    }
}

/// One instant of an object's pose. It covers its own sample and nothing else.
struct PhysicalKeyframe: Codable, Equatable, Identifiable {
    var keyframeID: String
    var sampleID: Int
    var timestampNs: Int64
    var anchor = PhysicalAnchor()
    var position = PhysicalPosition()
    var yaw = PhysicalYaw()
    var front = PhysicalEndpoint()
    var rear = PhysicalEndpoint()
    var sharedErrors: [PhysicalSharedError]?
    var review = PhysicalReview()

    var id: String { keyframeID }

    enum CodingKeys: String, CodingKey {
        case keyframeID = "keyframe_id"
        case sampleID = "sample_id"
        case timestampNs = "timestamp_ns"
        case anchor
        case position
        case yaw
        case front
        case rear
        case sharedErrors = "shared_errors"
        case review
    }

    var declaresBounds: Bool { position.status != .unknown || yaw.status != .unknown }
}

/// One sidecar object's physical reference.
struct PhysicalObject: Codable, Equatable, Identifiable {
    var objectID: String
    var body: PhysicalBody?
    var keyframes: [PhysicalKeyframe] = []

    var id: String { objectID }

    enum CodingKeys: String, CodingKey {
        case objectID = "object_id"
        case body
        case keyframes
    }

    func keyframe(sampleID: Int) -> PhysicalKeyframe? {
        keyframes.first { $0.sampleID == sampleID }
    }
}

/// A following record, read for display only. This client authors none, and
/// the service carries them over unchanged on every save.
struct PhysicalFollowingSummary: Codable, Equatable {
    var followingID: String
    var followerObjectID: String

    enum CodingKeys: String, CodingKey {
        case followingID = "following_id"
        case followerObjectID = "follower_object_id"
    }
}

/// The revisable document for one pack, as the service returns it.
struct PhysicalReferenceDocument: Codable, Equatable {
    static let schema = "velocity.report/physical-reference"
    static let supportedVersion = 1

    var schema: String
    var schemaVersion: Int
    var datasetID: String
    var packDigest: String
    var revision: Int
    var updatedUTC: String?
    var change: Provenance?
    var objects: [PhysicalObject]
    var following: [PhysicalFollowingSummary]?
    var recordOrigins: [String: PhysicalOrigin]?

    enum CodingKeys: String, CodingKey {
        case schema
        case schemaVersion = "schema_version"
        case datasetID = "dataset_id"
        case packDigest = "pack_digest"
        case revision
        case updatedUTC = "updated_utc"
        case change
        case objects
        case following
        case recordOrigins = "record_origins"
    }

    /// Whether this client can edit the document without losing anything it
    /// does not understand. Anything else opens read-only.
    var editable: Bool {
        schema == PhysicalReferenceDocument.schema
            && schemaVersion == PhysicalReferenceDocument.supportedVersion
    }

    func object(_ id: String) -> PhysicalObject? { objects.first { $0.objectID == id } }
}

/// A record that does not hold against the membership sidecar.
struct PhysicalLinkProblem: Codable, Equatable, Hashable {
    var record: String
    var problem: String
}

/// What the service returns for a pack: the document, its revision and
/// exact-byte token, and the membership it was checked against.
struct PhysicalPackState: Codable, Equatable {
    var pack: String
    var packDir: String
    var datasetID: String
    var packDigest: String
    var exists: Bool
    var revision: Int
    var digest: String
    var contentDigest: String
    var head: Bool
    var membershipDigest: String
    var membershipRevision: Int
    var stale: [PhysicalLinkProblem]
    var document: PhysicalReferenceDocument

    enum CodingKeys: String, CodingKey {
        case pack
        case packDir = "pack_dir"
        case datasetID = "dataset_id"
        case packDigest = "pack_digest"
        case exists
        case revision
        case digest
        case contentDigest = "content_digest"
        case head
        case membershipDigest = "membership_digest"
        case membershipRevision = "membership_revision"
        case stale
        case document
    }
}

/// A validation or save's diagnostics.
struct PhysicalEditResult: Codable, Equatable {
    var valid: Bool
    var invalid: String?
    var linkProblems: [PhysicalLinkProblem]
    var resetReviews: [String]
    var renamedBodies: [String: String]?
    var state: PhysicalPackState?

    enum CodingKeys: String, CodingKey {
        case valid
        case invalid
        case linkProblems = "link_problems"
        case resetReviews = "reset_reviews"
        case renamedBodies = "renamed_bodies"
        case state
    }
}

/// Which kind of saved record a review names.
enum PhysicalRecordKind: String, Codable {
    case body
    case keyframe
}

/// Unit conversion at the model boundary: the operator reads degrees, the
/// record stores radians.
enum PhysicalUnits {
    static func degrees(_ radians: Double) -> Double { radians * 180 / .pi }
    static func radians(_ degrees: Double) -> Double { degrees * .pi / 180 }

    /// An angle in (-180, 180], so a yaw typed as 350 reads back as -10.
    static func wrappedDegrees(_ degrees: Double) -> Double {
        var d = degrees.truncatingRemainder(dividingBy: 360)
        if d <= -180 { d += 360 }
        if d > 180 { d -= 360 }
        return d
    }
}
