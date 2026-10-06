// PhysicalComparisonReport.swift
// Reads the physical section of a per-frame evaluation report, so a tuning
// comparison can be inspected at the instant it was scored.
//
// The report is Go's (internal/lidar/perframeeval): a comparison of two
// estimate arms, each with the reference geometry it was scored against, the
// prediction, the match and the component errors at every expected instant.
// Nothing is rescored here. The geometry drawn is the geometry the report
// recorded, which is not necessarily what the reference says now: a report
// is bound to the revision it scored, and a later edit makes a new report,
// never a spliced one.

import Foundation

/// Go's annotation.PhysicalGeometry, as a report records it.
struct ReportGeometry: Decodable, Equatable {
    struct Planar: Decodable, Equatable {
        var x: Double
        var y: Double
        var bound: Double
        enum CodingKeys: String, CodingKey {
            case x = "x_m"
            case y = "y_m"
            case bound = "bound_m"
        }
        var planar: PhysicalPlanar { PhysicalPlanar(x: x, y: y, bound: bound) }
    }
    struct Angle: Decodable, Equatable {
        var rad: Double
        var boundRad: Double
        var axis: PhysicalAxisState
        var status: PhysicalEvidence
        enum CodingKeys: String, CodingKey {
            case rad
            case boundRad = "bound_rad"
            case axis
            case status
        }
    }
    struct Linear: Decodable, Equatable {
        var lower: Double
        var upper: Double
        var value: Double
        var halfWidth: Double
        var status: PhysicalEvidence
        enum CodingKeys: String, CodingKey {
            case lower = "lower_m"
            case upper = "upper_m"
            case value = "value_m"
            case halfWidth = "half_width_m"
            case status
        }
    }
    struct Box: Decodable, Equatable {
        var centreX: Double
        var centreY: Double
        var yawRad: Double
        var length: Double
        var width: Double
        enum CodingKeys: String, CodingKey {
            case centreX = "centre_x_m"
            case centreY = "centre_y_m"
            case yawRad = "yaw_rad"
            case length = "length_m"
            case width = "width_m"
        }
    }

    var objectID: String
    var keyframeID: String
    var sampleID: Int
    var timestampNs: Int64
    var reviewStatus: ReviewStatus
    var origin: PhysicalOrigin
    var truth: Bool
    var bodyTruth: Bool
    var anchor: PhysicalAnchorKind
    var anchorPoint: Planar?
    var centre: Planar?
    var centreUnavailable: String?
    var yaw: Angle?
    var yawUnavailable: String?
    var length: Linear?
    var width: Linear?
    var height: Linear?
    var front: Planar?
    var frontUnavailable: String?
    var rear: Planar?
    var rearUnavailable: String?
    var ends: [Planar]?
    var box: Box?
    var boxUnavailable: String?

    enum CodingKeys: String, CodingKey {
        case objectID = "object_id"
        case keyframeID = "keyframe_id"
        case sampleID = "sample_id"
        case timestampNs = "timestamp_ns"
        case reviewStatus = "review_status"
        case origin
        case truth
        case bodyTruth = "body_truth"
        case anchor
        case anchorPoint = "anchor_point"
        case centre
        case centreUnavailable = "centre_unavailable"
        case yaw
        case yawUnavailable = "yaw_unavailable"
        case length
        case width
        case height
        case front
        case frontUnavailable = "front_unavailable"
        case rear
        case rearUnavailable = "rear_unavailable"
        case ends
        case box
        case boxUnavailable = "box_unavailable"
    }

    /// The same geometry in the overlay's terms.
    var geometry: PhysicalGeometry {
        var g = PhysicalGeometry(anchorKind: anchor)
        g.anchorPoint = anchorPoint?.planar
        g.centre = centre?.planar
        g.centreUnavailable = centreUnavailable
        g.yaw = yaw.map {
            PhysicalAngle(rad: $0.rad, boundRad: $0.boundRad, axis: $0.axis, status: $0.status)
        }
        g.yawUnavailable = yawUnavailable
        func linear(_ l: Linear?) -> PhysicalLinear? {
            l.map {
                PhysicalLinear(
                    lower: $0.lower, upper: $0.upper, value: $0.value, halfWidth: $0.halfWidth,
                    status: $0.status)
            }
        }
        g.length = linear(length)
        g.width = linear(width)
        g.height = linear(height)
        g.front = front?.planar
        g.frontUnavailable = frontUnavailable
        g.rear = rear?.planar
        g.rearUnavailable = rearUnavailable
        g.ends = (ends ?? []).map(\.planar)
        g.box = box.map {
            PhysicalBox(
                centreX: $0.centreX, centreY: $0.centreY, yawRad: $0.yawRad, length: $0.length,
                width: $0.width)
        }
        g.boxUnavailable = boxUnavailable
        return g
    }
}

/// One estimate version's claim about one track at one instant.
struct ReportPrediction: Decodable, Equatable {
    struct Extent: Decodable, Equatable {
        var metres: Double
        var sigmaMetres: Double
        var provenance: String
        var evidence: Bool
        enum CodingKeys: String, CodingKey {
            case metres
            case sigmaMetres = "sigma_metres"
            case provenance
            case evidence
        }
    }
    struct Heading: Decodable, Equatable {
        var rad: Double
        var sigmaRad: Double
        var resolved: Bool
        enum CodingKeys: String, CodingKey {
            case rad
            case sigmaRad = "sigma_rad"
            case resolved
        }
    }

    var trackKey: String
    var timestampNs: Int64
    /// What the predicted position is a position of: a body centre, or a
    /// visible-return centre such as a medoid. Shown as such, never relabelled.
    var reference: String
    var x: Double
    var y: Double
    var positionSigma: Double
    var physical: Bool
    var centreX: Double?
    var centreY: Double?
    var heading: Heading?
    var length: Extent?
    var width: Extent?
    var height: Extent?

    enum CodingKeys: String, CodingKey {
        case trackKey = "track_key"
        case timestampNs = "timestamp_ns"
        case reference
        case x = "x_m"
        case y = "y_m"
        case positionSigma = "position_sigma_m"
        case physical
        case centreX = "centre_x_m"
        case centreY = "centre_y_m"
        case heading
        case length
        case width
        case height
    }

    /// The predicted footprint when the estimate states one: a physical body
    /// with a heading, a length and a width.
    var box: PhysicalBox? {
        guard physical, let heading, let length, let width else { return nil }
        return PhysicalBox(
            centreX: centreX ?? x, centreY: centreY ?? y, yawRad: heading.rad,
            length: length.metres, width: width.metres)
    }
}

struct ReportMatch: Decodable, Equatable {
    var trackKey: String
    var distance: Double
    var offsetNs: Int64
    var candidates: Int
    enum CodingKeys: String, CodingKey {
        case trackKey = "track_key"
        case distance = "distance_m"
        case offsetNs = "offset_ns"
        case candidates
    }
}

struct ReportOutcome: Decodable, Equatable {
    var category: String
    var reason: String?
}

/// The scored differences at one instant. Only the few a person reads first
/// are typed; the rest stay in the report.
struct ReportComparison: Decodable, Equatable {
    struct Centre: Decodable, Equatable {
        struct Step: Decodable, Equatable {
            var fromSampleID: Int
            var referenceMove: Double
            var predictionMove: Double
            var error: Double
            var sameTrack: Bool
            enum CodingKeys: String, CodingKey {
                case fromSampleID = "from_sample_id"
                case referenceMove = "reference_move_m"
                case predictionMove = "prediction_move_m"
                case error = "error_m"
                case sameTrack = "same_track"
            }
        }
        var error: Double
        var referenceBound: Double
        var predictionSigma: Double
        var predictionReference: String
        var step: Step?
        enum CodingKeys: String, CodingKey {
            case error = "error_m"
            case referenceBound = "reference_bound_m"
            case predictionSigma = "prediction_sigma_m"
            case predictionReference = "prediction_reference"
            case step
        }
    }
    struct Yaw: Decodable, Equatable {
        var errorRad: Double
        var axisOnly: Bool
        var referenceBoundRad: Double
        enum CodingKeys: String, CodingKey {
            case errorRad = "error_rad"
            case axisOnly = "axis_only"
            case referenceBoundRad = "reference_bound_rad"
        }
    }
    struct Dimension: Decodable, Equatable {
        var predicted: Double
        var referenceValue: Double
        var error: Double
        var outsideBound: Double
        enum CodingKeys: String, CodingKey {
            case predicted = "predicted_m"
            case referenceValue = "reference_value_m"
            case error = "error_m"
            case outsideBound = "outside_bound_m"
        }
    }
    struct Endpoint: Decodable, Equatable {
        var distance: Double
        var referenceBound: Double
        enum CodingKeys: String, CodingKey {
            case distance = "distance_m"
            case referenceBound = "reference_bound_m"
        }
    }
    struct Box: Decodable, Equatable { var iou: Double }

    var centre: Centre?
    var yaw: Yaw?
    var length: Dimension?
    var width: Dimension?
    var height: Dimension?
    var front: Endpoint?
    var rear: Endpoint?
    var box: Box?
}

struct ReportInstant: Decodable, Equatable {
    var episodeID: String
    var objectID: String
    var sampleID: Int
    var timestampNs: Int64
    var referenceRevision: Int
    var estimate: String
    var reference: ReportGeometry?
    var prediction: ReportPrediction?
    var match: ReportMatch?
    var comparison: ReportComparison
    var outcomes: [String: ReportOutcome]

    enum CodingKeys: String, CodingKey {
        case episodeID = "episode_id"
        case objectID = "object_id"
        case sampleID = "sample_id"
        case timestampNs = "timestamp_ns"
        case referenceRevision = "reference_revision"
        case estimate
        case reference
        case prediction
        case match
        case comparison
        case outcomes
    }
}

struct ReportReferenceIdentity: Decodable, Equatable {
    var packDigest: String
    var datasetID: String
    var sidecarRevision: Int
    var physicalRevision: Int
    var physicalRevisionDigest: String
    var physicalContentDigest: String
    var split: String
    var splitRole: String
    var expectedInstants: Int

    enum CodingKeys: String, CodingKey {
        case packDigest = "pack_digest"
        case datasetID = "dataset_id"
        case sidecarRevision = "sidecar_revision"
        case physicalRevision = "physical_revision"
        case physicalRevisionDigest = "physical_revision_digest"
        case physicalContentDigest = "physical_content_digest"
        case split
        case splitRole = "split_role"
        case expectedInstants = "expected_instants"
    }
}

struct ReportArmIdentity: Decodable, Equatable {
    var label: String
    var kind: String
    var database: String
    var estimatorID: String?
    var runID: String?
    var stage: String
    var sourceID: String? = nil
    var observationModelID: String? = nil
    var paramHash: String? = nil

    enum CodingKeys: String, CodingKey {
        case label
        case kind
        case database
        case estimatorID = "estimator_id"
        case runID = "run_id"
        case stage
        case sourceID = "source_id"
        case observationModelID = "observation_model_id"
        case paramHash = "param_hash"
    }

    /// The identity an assisted record names as its tracker source.
    var source: String {
        [
            label, estimatorID, observationModelID, paramHash, stage, sourceID,
            runID.map { "run " + $0 }, database,
        ].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
    }
}

struct ReportComponentAccounting: Decodable, Equatable {
    var expected: Int
    var scored: Int
    /// Why the rest were not scored: category, then reason, then count.
    var unscored: [String: [String: Int]]
}

/// One follower at one instant: the reference gap, the estimate's, and the
/// scored difference or why there is none.
struct ReportFollowingInstant: Decodable, Equatable {
    struct ReferenceGap: Decodable, Equatable {
        var status: PhysicalEvidence
        var lower: Double?
        var upper: Double?
        var value: Double?
        enum CodingKeys: String, CodingKey {
            case status
            case lower = "lower_m"
            case upper = "upper_m"
            case value = "value_m"
        }
    }
    struct PredictedGap: Decodable, Equatable {
        var value: Double
        var sigma: Double
        var followerTrack: String
        var leaderTrack: String
        enum CodingKeys: String, CodingKey {
            case value = "value_m"
            case sigma = "sigma_m"
            case followerTrack = "follower_track"
            case leaderTrack = "leader_track"
        }
    }

    var episodeID: String
    var followingID: String
    var followerObjectID: String
    var leaderObjectID: String?
    var decision: String?
    var sampleID: Int
    var timestampNs: Int64
    var referenceGap: ReferenceGap?
    var predictedGap: PredictedGap?
    var error: Double?
    var outsideBound: Double?
    var outcome: ReportOutcome

    enum CodingKeys: String, CodingKey {
        case episodeID = "episode_id"
        case followingID = "following_id"
        case followerObjectID = "follower_object_id"
        case leaderObjectID = "leader_object_id"
        case decision
        case sampleID = "sample_id"
        case timestampNs = "timestamp_ns"
        case referenceGap = "reference_gap"
        case predictedGap = "predicted_gap"
        case error = "error_m"
        case outsideBound = "outside_bound_m"
        case outcome
    }
}

struct ReportArm: Decodable, Equatable {
    var reference: ReportReferenceIdentity
    var arm: ReportArmIdentity
    var instants: [ReportInstant]
    var following: [ReportFollowingInstant]
    var accounting: [String: ReportComponentAccounting]
    var caveats: [String]

    enum CodingKeys: String, CodingKey {
        case reference
        case arm
        case instants
        case following
        case accounting
        case caveats
    }

    private enum AccountingKeys: String, CodingKey { case components }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        reference = try c.decode(ReportReferenceIdentity.self, forKey: .reference)
        arm = try c.decode(ReportArmIdentity.self, forKey: .arm)
        instants = try c.decode([ReportInstant].self, forKey: .instants)
        following = try c.decodeIfPresent([ReportFollowingInstant].self, forKey: .following) ?? []
        let a = try c.nestedContainer(keyedBy: AccountingKeys.self, forKey: .accounting)
        accounting = try a.decode([String: ReportComponentAccounting].self, forKey: .components)
        caveats = try c.decodeIfPresent([String].self, forKey: .caveats) ?? []
    }
}

enum PhysicalReportError: Error, LocalizedError, Equatable {
    case noPhysicalSection
    case unreadable(String)
    case wrongPack(report: String, open: String)
    case armsDisagree

    var errorDescription: String? {
        switch self {
        case .noPhysicalSection:
            return "This report has no physical section: it was run without -physical-reference."
        case .unreadable(let detail): return "Could not read the report: \(detail)"
        case .wrongPack(let report, let open):
            return "The report scored pack \(report); this window has \(open) open."
        case .armsDisagree: return "The report's two arms name different references."
        }
    }
}

/// A report's physical section, checked against the open pack and indexed
/// for stepping through it.
struct PhysicalComparisonReport: Equatable {
    var arms: [ReportArm]
    /// Instants by arm, then by sample.
    private var index: [[Int: [ReportInstant]]]
    private var followingIndex: [[Int: [ReportFollowingInstant]]]

    var reference: ReportReferenceIdentity { arms[0].reference }

    static func decode(_ data: Data, packDigest: String) throws -> PhysicalComparisonReport {
        struct Envelope: Decodable {
            struct Physical: Decodable {
                var armA: ReportArm
                var armB: ReportArm
                enum CodingKeys: String, CodingKey {
                    case armA = "arm_a"
                    case armB = "arm_b"
                }
            }
            var physical: Physical?
        }
        let envelope: Envelope
        do { envelope = try JSONDecoder().decode(Envelope.self, from: data) } catch {
            throw PhysicalReportError.unreadable(String(describing: error))
        }
        guard let physical = envelope.physical else { throw PhysicalReportError.noPhysicalSection }
        let arms = [physical.armA, physical.armB]
        guard arms[0].reference == arms[1].reference else { throw PhysicalReportError.armsDisagree }
        guard arms[0].reference.packDigest == packDigest else {
            throw PhysicalReportError.wrongPack(
                report: arms[0].reference.packDigest, open: packDigest)
        }
        return PhysicalComparisonReport(
            arms: arms, index: arms.map { Dictionary(grouping: $0.instants, by: \.sampleID) },
            followingIndex: arms.map { Dictionary(grouping: $0.following, by: \.sampleID) })
    }

    /// Every following instant the report scored at this sample for this arm.
    func following(arm: Int, sampleID: Int) -> [ReportFollowingInstant] {
        guard followingIndex.indices.contains(arm) else { return [] }
        return followingIndex[arm][sampleID] ?? []
    }

    /// Every instant the report scored at this sample for this arm. Two
    /// samples may share a timestamp; the report's sample ID tells them apart.
    func instants(arm: Int, sampleID: Int) -> [ReportInstant] {
        guard index.indices.contains(arm) else { return [] }
        return index[arm][sampleID] ?? []
    }

    /// Objects the report has a prediction for at this sample: what showing
    /// it exposes the operator to.
    func predictedObjects(arm: Int, sampleID: Int) -> Set<String> {
        Set(instants(arm: arm, sampleID: sampleID).filter { $0.prediction != nil }.map(\.objectID))
    }

    /// Instant counts by outcome category for one arm, for the summary line.
    func outcomeCounts(arm: Int, component: String) -> [String: Int] {
        guard arms.indices.contains(arm) else { return [:] }
        var counts: [String: Int] = [:]
        for instant in arms[arm].instants {
            if let outcome = instant.outcomes[component] {
                counts[outcome.category, default: 0] += 1
            }
        }
        return counts
    }
}
