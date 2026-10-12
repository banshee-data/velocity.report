// AnnotationSidecar.swift
// Reference annotation records, and the version 1 sidecar shape the window
// edits in.
//
// The records mirror Go's internal/lidar/annotation schema field for field. A
// label set keeps objects, poses and provenance in exactly these shapes; only
// membership moved, from sample and point index to window and return. The
// window still edits one pack's view as a Sidecar: masks by sample id and
// point index. LabelSetMembership projects a label set into that shape and
// folds edits back as deltas.
//
// This client no longer writes annotations.json. A version 1 pack's sidecar
// is read for display only (LegacySidecarReader), and the migration carries
// it into a label set.

import CryptoKit
import Foundation

// MARK: - Records

/// Whether a record was produced by an algorithm, confirmed by a person, or
/// examined and rejected. A proposal may never be exported as reference truth.
enum ReviewStatus: String, Codable, Equatable {
    case proposed
    case reviewed
    case rejected
}

/// What could be seen of the object, which is not the same as what was
/// labelled: an object fully occluded in a frame has an empty mask for an
/// understandable reason, and that differs from nobody having looked yet.
enum Visibility: String, Codable, Equatable, CaseIterable {
    case present
    case partlyOccluded = "partly_occluded"
    case fullyOccluded = "fully_occluded"
    case outsideView = "outside_view"
    case unknown

    var label: String {
        switch self {
        case .present: return "Present"
        case .partlyOccluded: return "Partly occluded"
        case .fullyOccluded: return "Fully occluded"
        case .outsideView: return "Outside view"
        case .unknown: return "Unknown"
        }
    }
}

/// Whether a mask claims every visible return of the object in that sample.
/// Even a complete object mask does not establish background negatives.
enum MaskCompleteness: String, Codable, Equatable, CaseIterable {
    case complete
    case partial
    case unreviewed

    var label: String {
        switch self {
        case .complete: return "Complete"
        case .partial: return "Partial"
        case .unreviewed: return "Unreviewed"
        }
    }
}

/// Who or what produced a record. The store supplies revision numbers and the
/// timestamp; it never invents a human author.
struct Provenance: Codable, Equatable {
    var author: String = ""
    var session: String?
    var createdUTC: String = ""
    var operation: String?
    var parentRevision: Int?
    var revision: Int = 0
    var algorithm: String?
    var algorithmVersion: String?

    enum CodingKeys: String, CodingKey {
        case author
        case session
        case createdUTC = "created_utc"
        case operation
        case parentRevision = "parent_revision"
        case revision
        case algorithm
        case algorithmVersion = "algorithm_version"
    }
}

/// A human reference object, identified independently of any predicted track.
struct AnnotationObject: Codable, Equatable, Identifiable {
    var objectID: String
    var objectClass: String
    var subtype: String?
    var confidence: Double = 1.0
    var status: ReviewStatus = .proposed
    var provenance: Provenance = Provenance()
    var notes: String?

    var id: String { objectID }

    enum CodingKeys: String, CodingKey {
        case objectID = "object_id"
        case objectClass = "class"
        case subtype
        case confidence
        case status
        case provenance
        case notes
    }
}

/// An optional physical box. Separate from point membership on purpose: a box
/// includes inferred unseen extent, so it carries its own confidence and
/// cannot be derived from a partial mask's extremes.
struct AnnotationPose: Codable, Equatable {
    var centerX: Double
    var centerY: Double
    var centerZ: Double
    var length: Double
    var width: Double
    var height: Double
    var yawRad: Double
    var axisAmbiguous: Bool = false
    var confidence: Double = 0
    var status: ReviewStatus = .proposed
    var provenance: Provenance = Provenance()

    enum CodingKeys: String, CodingKey {
        case centerX = "center_x"
        case centerY = "center_y"
        case centerZ = "center_z"
        case length
        case width
        case height
        case yawRad = "yaw_rad"
        case axisAmbiguous = "axis_ambiguous"
        case confidence
        case status
        case provenance
    }
}

/// One object's membership in one sample. `pointIndices` are canonical pack
/// indices, ascending, valid only under the pack digest recorded alongside.
struct FrameMask: Codable, Equatable {
    var objectID: String
    var sampleID: Int
    var pointIndices: [Int] = []
    var uncertainIndices: [Int]?
    var completeness: MaskCompleteness = .unreviewed
    var visibility: Visibility = .unknown
    var status: ReviewStatus = .proposed
    var pose: AnnotationPose?
    var provenance: Provenance = Provenance()

    enum CodingKeys: String, CodingKey {
        case objectID = "object_id"
        case sampleID = "sample_id"
        case pointIndices = "point_indices"
        case uncertainIndices = "uncertain_indices"
        case completeness
        case visibility
        case status
        case pose
        case provenance
    }
}

/// An optional link from a human object to predicted tracks. One reference
/// object may map to several tracks, and one merged track to several objects.
struct TrackCorrespondence: Codable, Equatable {
    var objectID: String
    var runID: String?
    /// Nil and empty are different to Go, which writes null for one and []
    /// for the other, and the label set's digest is over those bytes.
    var trackIDs: [String]?
    var startNs: Int64 = 0
    var endNs: Int64 = 0
    var note: String?

    enum CodingKeys: String, CodingKey {
        case objectID = "object_id"
        case runID = "run_id"
        case trackIDs = "track_ids"
        case startNs = "start_ns"
        case endNs = "end_ns"
        case note
    }
}

/// The whole annotation snapshot for one pack.
struct Sidecar: Codable, Equatable {
    /// Must match Go's SidecarSchemaVersion: that reader refuses any other
    /// value outright, so writing a newer number here would lock the Go
    /// tooling out of packs this client has touched.
    static let schemaVersion = 1

    var schemaVersion: Int = Sidecar.schemaVersion
    var datasetID: String = ""
    var packDigest: String = ""
    var revision: Int = 0
    var updatedUTC: String = ""
    var change: Provenance?
    var restoredFrom: Int?
    var objects: [AnnotationObject] = []
    var masks: [FrameMask] = []
    var correspondences: [TrackCorrespondence]?
    var dismissedProposals: [String]?

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case datasetID = "dataset_id"
        case packDigest = "pack_digest"
        case revision
        case updatedUTC = "updated_utc"
        case change
        case restoredFrom = "restored_from"
        case objects
        case masks
        case correspondences
        case dismissedProposals = "dismissed_proposals"
    }

    /// The mask for one object in one sample, if any.
    func mask(objectID: String, sampleID: Int) -> FrameMask? {
        masks.first { $0.objectID == objectID && $0.sampleID == sampleID }
    }

    /// Masks whose object and mask are both reviewed. A mask reviewed under a
    /// still-proposed object is not reference truth, so both gates apply.
    var reviewedMasks: [FrameMask] {
        let reviewedObjects = Set(objects.filter { $0.status == .reviewed }.map(\.objectID))
        return masks.filter { $0.status == .reviewed && reviewedObjects.contains($0.objectID) }
    }

    /// Replaces or inserts a mask, keeping masks ordered by (sample, object)
    /// so an unchanged snapshot serialises to unchanged bytes.
    mutating func upsert(mask: FrameMask) {
        if let idx = masks.firstIndex(where: {
            $0.objectID == mask.objectID && $0.sampleID == mask.sampleID
        }) {
            masks[idx] = mask
        } else {
            masks.append(mask)
        }
        masks.sort {
            $0.sampleID == $1.sampleID ? $0.objectID < $1.objectID : $0.sampleID < $1.sampleID
        }
    }
}

// MARK: - Time

/// The UTC timestamp every record this client writes carries.
enum AnnotationClock {
    static func utcTimestamp() -> String {
        let formatter = ISO8601DateFormatter()
        formatter.timeZone = TimeZone(identifier: "UTC")
        return formatter.string(from: Date())
    }
}

// MARK: - Points claimed twice

/// A return two objects claim in one sample. Go's sidecar validation refuses
/// any such file outright, so every Go reader of the pack, the physical
/// reference service included, would refuse it too.
struct PointClaimConflict: Hashable {
    var sampleID: Int
    var point: Int
    /// The two objects, in the order Go reports them: the earlier mask first.
    var first: String
    var second: String

    /// The same conflict whichever mask came first.
    var key: String { "\(sampleID)/\(point)/" + [first, second].sorted().joined(separator: "|") }
}

extension Sidecar {
    /// Every return claimed by two objects in the same sample, by the rule
    /// Go's Sidecar.Validate applies: any two masks of different objects, of
    /// any status.
    func pointClaimConflicts() -> [PointClaimConflict] {
        var claimed: [Int: [Int: String]] = [:]
        var out: [PointClaimConflict] = []
        for mask in masks {
            for index in mask.pointIndices {
                if let other = claimed[mask.sampleID]?[index], other != mask.objectID {
                    out.append(
                        PointClaimConflict(
                            sampleID: mask.sampleID, point: index, first: other,
                            second: mask.objectID))
                } else {
                    claimed[mask.sampleID, default: [:]][index] = mask.objectID
                }
            }
        }
        return out
    }
}

/// A save that would make two objects claim one return.
struct PointClaimError: Error, CustomStringConvertible {
    var conflicts: [PointClaimConflict]

    var description: String {
        let first = conflicts[0]
        let more = conflicts.count > 1 ? " (and \(conflicts.count - 1) more)" : ""
        return "Not saved: sample \(first.sampleID) point \(first.point) would belong to both "
            + "\(first.first) and \(first.second)\(more). A return belongs to one object; take it "
            + "out of one of them first. Go's tools refuse labels where it does not."
    }
}
