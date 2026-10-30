// AlignmentSession.swift
// State behind the alignment pane: which site the open capture belongs to,
// the Gerardus reference drawn over it, the pose and the operator's nudges,
// and the decisions recorded in Gerardus's alignment log.
//
// The operator's path, one RPC per step (Gerardus docs/INTERFACE-VISUALISER.md):
//
//   Place from hint   PrepareSite → PoseFromHint → StreamReference (150 m),
//                     StreamReference for the street layers the server lists
//                     (CITY_STREETS, CITY_INTERSECTIONS, KERBS), then
//                     StreamReference once per W4–W7 layer the server lists
//                     (BUILDINGS, LANE_MARKINGS, STREET_FURNITURE, TREES),
//                     then StreamReference once per registered layer
//   Site picker       GetSite, to read how current the stored bundle is; builds nothing
//   Update reference  PrepareSite (refresh false: Gerardus rebuilds a bundle that lacks
//   data              layers it can now build, from its caches), then the reference again
//   Regenerate…       after a confirmation, PrepareSite (refresh true: every source
//                     downloaded and 3DEP cut again), then the reference again
//   Nudge             nothing on the wire: only the model matrix changes
//   Accumulate        during a pcap replay, velocity.report's
//                     POST /api/lidar/replay/full_frames on, then nothing
//                     while the frames are collected, full frames off; then
//                     UploadScan (SCAN_KIND_ACCUMULATED), which Register then uses
//   Register          UploadScan (once per background) → Register, against
//                     3DEP alone or, with the switches on, against the site
//                     model, the DataSF boxes, or both
//   Accept / Reject   ReviewRegistration → ListRegistrationRecords →
//                     GetSiteModel → StreamReference per registered layer,
//                     when the site model moved
//   Refine site       RefineSite → GetRegistration per scan, for its gates
//   Accept refinement ReviewRegistrations of every scan's j1, atomically →
//                     ListRegistrationRecords → GetSiteModel → StreamReference
//
// The registered layers (REGISTERED_SCANS, OBSERVED, GROUND_MESH) are built
// from the site's accepted scans (Gerardus docs/plans/SITE-MODEL-SPIKE.md).
// Each is streamed on a call of its own, so a refusal of one (the scans are
// served only to loopback and tailnet peers) leaves the others drawn.
//
// Until Gerardus builds ICP, Register comes back FAILED carrying the pose it
// was given, and Accept records the operator's pose as MANUALLY_PLACED.

import Combine
import Foundation
import GRPCCore
import SwiftProtobuf
import simd

private let logger = DevLogger(category: "Alignment")

/// Where the reference overlay goes. MetalRenderer is the real one.
protocol ReferenceOverlaySink: AnyObject {
    func setReferenceOverlay(_ vertices: ReferenceOverlayVertices?)
    func setReferenceModelMatrix(_ matrix: simd_float4x4)
    func setReferenceVisibility(_ visibility: ReferenceVisibility)
    func setResidualOverlay(_ vertices: [Float]?, palette: [SIMD4<Float>])
    /// Moves the camera to frame a point of the site frame, keeping the
    /// direction it looks in.
    func focusCamera(onSitePoint point: SIMD3<Double>)
}

/// What the pane shows about the settled background it would upload.
struct AlignmentBackgroundStatus: Equatable {
    var points: Int
    var sequence: UInt64
    /// False while the grid is still settling: such a snapshot is not uploaded.
    var settled: Bool
}

enum AlignmentScanError: Error, LocalizedError {
    case noScan

    var errorDescription: String? { "No settled background or accumulated scan to register." }
}

/// Frames collected so far, while an accumulation runs.
struct AlignmentAccumulationProgress: Equatable {
    var frames: Int
    var of: Int
    var points: Int
}

/// The accumulated scan Register uses in place of the background.
struct AlignmentAccumulatedScan: Equatable {
    var scanID: String
    var captureRef: String
    var frames: Int
    /// Background and ground points collected, before thinning.
    var pointsCollected: Int
    /// Points uploaded, one per 0.05 m voxel.
    var points: Int
}

/// EvaluatePose's verdict on one pose.
struct PoseEvaluation {
    var quality: Gerardus_Align_V1alpha1_FitQuality
    var summary: Gerardus_Align_V1alpha1_PoseSummary
    /// True when the pose was refined by local ICP (Snap) before scoring.
    var refined: Bool
    /// One per uploaded scan point, in upload order; NaN where no reference
    /// lies within reach.
    var residuals: [Float]
}

/// What a registration was asked to fit against besides 3DEP: the site's
/// accepted scans (against_site_model), its corrected box walls
/// (against_boxes), or both. FitQuality's model_* residuals are against these.
struct AlignmentModelReference: Equatable {
    var siteModel = false
    var boxes = false

    /// "the site model", "the DataSF boxes" or "the site model and boxes".
    var label: String {
        switch (siteModel, boxes) {
        case (true, true): return "the site model and boxes"
        case (false, true): return "the DataSF boxes"
        default: return "the site model"
        }
    }

    /// The residual line's leading words: "site model", "boxes" or
    /// "site model and boxes".
    var shortLabel: String {
        switch (siteModel, boxes) {
        case (true, true): return "site model and boxes"
        case (false, true): return "boxes"
        default: return "site model"
        }
    }
}

/// What Accept records for the pose drawn now.
enum AlignmentAcceptance: Equatable {
    /// The registration's hypothesis at `index` (0 is the best): recorded
    /// REGISTERED when the run passed every gate, else MANUALLY_PLACED.
    case hypothesis(index: Int, registered: Bool)
    /// The pose as it stands, nudged or snapped: MANUALLY_PLACED.
    case pose
}

/// Counts per reference layer, as drawn.
struct ReferenceSummary: Equatable {
    var aerialPoints: Int
    var osmFeatures: Int
    /// DataSF's footprints, and the outlines Gerardus found in 3DEP where
    /// DataSF lists no building; both come in the footprints layer.
    var footprints: Int = 0
    var aerialOutlines: Int = 0
    /// Whether the server sent the boxes layer at all, and its walls (party
    /// walls among them).
    var boxesServed: Bool = false
    var boxWalls: Int = 0
    var partyWalls: Int = 0
    var intersections: Int
    /// Distinct registered scans, and their points as drawn.
    var registeredScans: Int = 0
    var registeredPoints: Int = 0
    /// Points of the open capture's own scan in the registered layer, drawn
    /// or not: it is hidden unless the operator asks for it.
    var ownScanPoints: Int = 0
    var observedFeatures: Int = 0
    /// Wall planes, and how many of each role.
    var wallPlanes: Int = 0
    var wallPlaneRoles: [ReferenceStyle.WallPlaneRole: Int] = [:]
    var groundMeshCells: Int = 0
    /// The street layers, when the server listed them: the City's
    /// centrelines and intersections, and kerb and island lines.
    var cityStreets: Int = 0
    var cityIntersections: Int = 0
    var kerbs: Int = 0
    var streetLayersServed: Bool = false
    /// The W4–W7 layers the server lists, and what each sent: building
    /// parts, their roof planes and wall surfaces, lane markings, poles and
    /// signs, trees.
    var featureLayersServed: Set<Gerardus_Align_V1alpha1_ReferenceLayer> = []
    var buildingParts: Int = 0
    var roofPlanes: Int = 0
    var wallSurfaces: Int = 0
    var laneMarkings: Int = 0
    var poles: Int = 0
    var signs: Int = 0
    var trees: Int = 0
    /// The site model the registered layers were built from; empty when no
    /// registered layer named one.
    var modelID: String = ""
    /// "© OpenStreetMap contributors", shown whenever OSM is visible.
    var osmAttribution: String
    var aerialAttribution: String
    var footprintAttribution: String = ""

    /// The footprints toggle's label: the 3DEP outlines are named apart, so
    /// they are not read as DataSF's.
    var footprintLabel: String {
        aerialOutlines == 0
            ? "DataSF footprints (\(footprints))"
            : "Footprints (\(footprints) DataSF, \(aerialOutlines) from 3DEP)"
    }

    static let footprintHelp =
        "Red: DataSF's building footprints, 2010 roofprints. Violet: outlines Gerardus found in the 2023 3DEP survey where DataSF lists no building, inferred from tall, smooth returns that do not reach the ground; not a surveyed footprint."

    /// The wall planes' counts by role, for the pane: "3 walls · 1 popout".
    /// Roles with none are left out.
    var wallPlaneRoleText: String {
        ReferenceStyle.WallPlaneRole.allCases.compactMap { role in
            guard let n = wallPlaneRoles[role], n > 0 else { return nil }
            let name: String
            switch role {
            case .wall: name = n == 1 ? "wall" : "walls"
            case .popout: name = n == 1 ? "popout" : "popouts"
            case .recess: name = n == 1 ? "recess" : "recesses"
            case .other: name = "other"
            }
            return "\(n) \(name)"
        }.joined(separator: " · ")
    }
}

@available(macOS 15.0, *) @MainActor final class AlignmentSession: ObservableObject {

    enum Availability: Equatable {
        case unknown
        case unavailable
        case available(serverVersion: String)
    }

    /// Radius of reference requested around the hinted sensor position.
    static let referenceRadiusM: Float = 150
    /// Where the typed north_azimuth_deg is kept per site. The earlier
    /// "alignment.heading." values were typed when the field meant the
    /// heading itself, so they are not read.
    static let northAzimuthKeyPrefix = "alignment.northAzimuth."
    static let reviewerKey = "alignment.reviewer"
    /// The hypothesis RefineSite writes for each scan.
    static let refinedHypothesisID = "j1"
    /// The weightings the pane offers for a refinement (plan §10 q2); 0 is
    /// the server's default, 500.
    static let refineNEffChoices: [Double] = [0, 250, 1000]

    // MARK: - Published state

    @Published private(set) var availability: Availability = .unknown
    /// The Gerardus server this session asks; Settings changes it.
    @Published private(set) var serviceAddress: GerardusAddress
    /// Why the last probe found no server, while none has answered.
    @Published private(set) var probeFailure: String?
    /// True while a probe waits for its answer.
    @Published private(set) var isChecking = false
    var isAvailable: Bool {
        if case .available = availability { return true }
        return false
    }

    @Published private(set) var sites: [AlignmentSite] = []
    @Published private(set) var site: AlignmentSite?
    @Published private(set) var siteMatch: AlignmentSiteMatch = .none
    /// The site's north_azimuth_deg from site-index.json, as the operator
    /// typed it. The hint sends the heading derived from it
    /// (`hintHeadingDeg`); empty sends no heading, which Gerardus treats as
    /// unknown.
    @Published var northAzimuthText: String = ""

    @Published private(set) var preparedSite: Gerardus_Align_V1alpha1_Site?
    /// The stored bundle of the chosen site, read with GetSite before any
    /// placement; nil while none is stored or the server has not answered.
    @Published private(set) var bundle: Gerardus_Align_V1alpha1_Site?
    /// Asks the operator to confirm Regenerate; nothing is sent until they do.
    @Published var confirmingRegenerate = false
    /// What the last Update or Regenerate did or did not add.
    @Published private(set) var referenceDataNote: String?
    @Published private(set) var progress: Gerardus_Align_V1alpha1_Progress?
    @Published private(set) var reference: ReferenceSummary?
    @Published var showAerial = true { didSet { pushVisibility() } }
    @Published var showOSM = true { didSet { pushVisibility() } }
    @Published var showFootprints = true { didSet { pushVisibility() } }
    @Published var showBoxes = true { didSet { pushVisibility() } }
    @Published var showIntersections = true { didSet { pushVisibility() } }
    @Published var showRegisteredScans = true { didSet { pushVisibility() } }
    @Published var showObserved = true { didSet { pushVisibility() } }
    @Published var showWallPlanes = true { didSet { pushVisibility() } }
    @Published var showGroundMesh = true { didSet { pushVisibility() } }
    @Published var showKerbs = true { didSet { pushVisibility() } }
    @Published var showCityStreets = true { didSet { pushVisibility() } }
    @Published var showBuildings = true { didSet { pushVisibility() } }
    @Published var showLaneMarkings = true { didSet { pushVisibility() } }
    @Published var showStreetFurniture = true { didSet { pushVisibility() } }
    @Published var showTrees = true { didSet { pushVisibility() } }
    /// How roof planes are coloured: the colours are in the vertices, so a
    /// change rebuilds the overlay.
    @Published var roofColouring: RoofColouring = .slope {
        didSet { if roofColouring != oldValue { scheduleRestyle() } }
    }
    /// The toolbar's faces switch: off hides the DataSF box walls and the
    /// wall planes whatever their own toggles say; on leaves those to them.
    @Published var showFaces = true { didSet { pushVisibility() } }
    /// The open capture's scan is drawn live; its registered copy is drawn
    /// too only when the operator asks, as a check of the accepted pose.
    @Published var showOwnScan = false {
        didSet { if showOwnScan != oldValue { scheduleRestyle() } }
    }
    /// Why a registered layer is missing, when the server did not send it.
    @Published private(set) var registeredLayersNote: String?
    /// Why the street layers are missing, when the server listed them and
    /// then did not send them.
    @Published private(set) var streetLayersNote: String?
    /// Why a W4–W7 layer is missing, when the server listed it and then did
    /// not send it.
    @Published private(set) var featureLayersNote: String?

    @Published private(set) var hintPose: SitePose?
    @Published private(set) var hintSummary: Gerardus_Align_V1alpha1_PoseSummary?
    /// The hint pose with the operator's nudges applied. Accept records it.
    @Published private(set) var pose: SitePose?
    @Published var translationStepM: Double = 0.5
    @Published var yawStepDeg: Double = 1
    /// Horizontal radius from the drawn pose, shared by Register and Snap.
    @Published var searchRadiusM: Double = 5 {
        didSet {
            if !searchRadiusM.isFinite {
                searchRadiusM = 5
            } else if searchRadiusM < 1 {
                searchRadiusM = 1
            } else if searchRadiusM > 30 {
                searchRadiusM = 30
            }
        }
    }

    var supportsBoundedPlacement: Bool { serverInfo?.supportsBoundedPlacement == true }

    @Published private(set) var background: AlignmentBackgroundStatus?
    @Published private(set) var scan: Gerardus_Align_V1alpha1_Scan?
    @Published private(set) var registration: Gerardus_Align_V1alpha1_Registration?
    /// Hypotheses as the run in progress reports them, then the finished
    /// registration's own list, ranked best first.
    @Published private(set) var hypotheses: [Gerardus_Align_V1alpha1_Hypothesis] = []
    @Published private(set) var isRegistering = false
    /// Register also fits against the site's accepted scans
    /// (RegisterRequest.against_site_model). Off by default, and sent only
    /// while `againstSiteModelUnavailable` is nil.
    @Published var againstSiteModel = false
    /// Register also fits against the site's DataSF box walls, corrected by
    /// the measured wall planes (RegisterRequest.against_boxes). Off by
    /// default, and sent only while `againstBoxesUnavailable` is nil. It can
    /// be on with `againstSiteModel`.
    @Published var againstBoxes = false
    /// What the registration shown now was asked to fit against besides
    /// 3DEP, for the caption on its model residuals.
    @Published private(set) var registeredAgainst = AlignmentModelReference()
    /// The scans the site model holds, by scan_id: GetSiteModel's list, or
    /// the registered layer's when GetSiteModel is not answered; nil when
    /// neither said.
    @Published private(set) var siteModelScanIDs: [String]?
    /// How many frames Accumulate collects.
    @Published var accumulateFrameCount = FrameAccumulation.defaultFrameCount
    /// While frames are being collected.
    @Published private(set) var accumulating: AlignmentAccumulationProgress?
    /// The uploaded accumulated scan, which Register and scoring use in
    /// place of the background until it is dropped.
    @Published private(set) var accumulated: AlignmentAccumulatedScan?
    /// Why the stream's frames cannot be accumulated, from the latest frame.
    @Published private(set) var streamRefusal: String?
    /// Why the last accumulation stopped short.
    @Published private(set) var accumulationRefusal: String?
    /// The hypothesis drawn now, for as long as the pose is exactly its pose.
    @Published private(set) var selectedHypothesisID: String?
    /// EvaluatePose's verdict on the pose drawn now, once it has rested.
    @Published private(set) var evaluation: PoseEvaluation?
    @Published private(set) var isEvaluating = false
    /// Colour the uploaded scan by residual whenever an evaluation has them.
    @Published var showResiduals = true { didSet { pushResiduals() } }
    @Published private(set) var records: [Gerardus_Align_V1alpha1_RegistrationRecord] = []
    /// The record the pose was last drawn from, until the pose is moved off
    /// it: the one clicked in the pane, the one Accept just wrote, or, on
    /// placement, the newest with a pose (this capture's own first).
    @Published private(set) var selectedRecordID: String?
    private var recordHeads: [String: String] = [:]
    private var pendingReview: Gerardus_Align_V1alpha1_ReviewRegistrationRequest?
    private var pendingRefinement: Gerardus_Align_V1alpha1_ReviewRegistrationsRequest?
    private var refinementHeads: [String: String] = [:]
    @Published private(set) var reviewNeedsReload = false
    @Published private(set) var conflictingRecords: [Gerardus_Align_V1alpha1_RegistrationRecord] =
        []
    @Published var reviewer: String = "" {
        didSet { defaults?.set(reviewer, forKey: Self.reviewerKey) }
    }
    @Published var note: String = ""
    @Published var rejectReason: String = ""

    /// What is running now, for the pane; nil when idle.
    @Published private(set) var busy: String?
    @Published private(set) var errorMessage: String?
    /// The record most recently written, until the pane is told otherwise.
    @Published private(set) var lastRecord: Gerardus_Align_V1alpha1_RegistrationRecord?
    /// RefineSite's answer, and the run it wrote for each scan, by scan id.
    @Published private(set) var refinement: Gerardus_Align_V1alpha1_RefineSiteResult?
    @Published private(set) var refinedRuns: [String: Gerardus_Align_V1alpha1_Registration] = [:]
    /// Scans whose refined pose has been recorded.
    @Published private(set) var refinedAccepted: Set<String> = []
    @Published private(set) var isRefining = false
    /// A scan of the refinement shown whose pose was accepted on its own
    /// since the refinement ran; Gerardus refuses its j1.
    @Published private(set) var refinementOverridden: String?
    /// The refinement column beside the pane: opened by Refine site or the
    /// operator, closed only by the operator (or a new site). Closing it
    /// keeps the refinement, which shows again when it is reopened.
    @Published private(set) var refinementColumnOpen = false
    /// Independent residuals per fit when weighting the graph; 0: the default.
    @Published var refineNEff: Double = 0
    /// Every scan of the refinement shown that was accepted on its own since
    /// it ran (`refinementOverridden` is the latest of them).
    @Published private(set) var staleRefinedScanIDs: Set<String> = []
    /// Scans of the site model the next Refine leaves out (RefineSite's
    /// scan_ids). Accepting part of a refinement stays impossible: a scan is
    /// left out by refining without it.
    @Published private(set) var refineExcluded: Set<String> = []
    /// The scan picked in the refinement summary: the camera framed it, and
    /// the scene draws it bold while the refinement is shown. Kept while the
    /// column is closed and reopened; cleared by a new refinement.
    @Published private(set) var selectedRefinedScanID: String?
    /// Each site-model scan's capture_ref, by scan_id, from GetSiteModel and
    /// the registered layers, for the summary's labels.
    @Published private(set) var scanCaptureRefs: [String: String] = [:]

    var delta: PoseDelta? {
        guard let hintPose, let pose else { return nil }
        return PoseDelta(from: hintPose, to: pose)
    }

    var osmAttributionVisible: String? {
        guard showOSM, let reference, reference.osmFeatures > 0, !reference.osmAttribution.isEmpty
        else { return nil }
        return reference.osmAttribution
    }

    // MARK: - Collaborators

    private let makeClient: (GerardusAddress) -> AlignmentServiceClient
    private var client: AlignmentServiceClient?
    /// The client that answered, for the street painter's ListStreetUse and
    /// EditStreetUse; nil while no server has.
    var gerardusClient: AlignmentServiceClient? { client }
    /// What the server that answered said about itself.
    @Published private(set) var serverInfo: Gerardus_Align_V1alpha1_GetServerInfoResponse?
    /// Bumped when the address changes, so a probe of the old server that
    /// answers late cannot claim the session for it.
    private var addressGeneration = 0
    private let lookup: AlignmentSiteLookupClient?
    private let defaults: UserDefaults?
    weak var sink: ReferenceOverlaySink?
    private var availableLayers: [Gerardus_Align_V1alpha1_ReferenceLayer] = []
    private var latestBackground: BackgroundSnapshot?
    /// The site and background sequence `scan` was uploaded from.
    private var uploadedFrom: (l16Token: String, sequence: UInt64)?
    /// The snapshot `scan` was uploaded from: residuals follow its order.
    private var uploadedSnapshot: BackgroundSnapshot?
    /// The accumulation in progress, and the run its capture_ref names.
    private var accumulator: FrameAccumulator?
    private var accumulationRun = ""
    private var accumulationTask: Task<Void, Never>?
    /// The accumulated scan as uploaded; nil while the background is used.
    private var accumulatedScan: Gerardus_Align_V1alpha1_Scan?
    /// Whether the frames come from a live sensor, as they last said.
    private var frameSource: SourceMode = .unspecified
    /// The replay's log span, from its frames' playback info: the first and
    /// last frames' capture times. Zero until a frame carries them.
    private var logStartNs: Int64 = 0
    private var logEndNs: Int64 = 0
    /// velocity.report's full-frames switch, which a pcap replay's
    /// accumulation needs; nil when velocity.report is skipped, as in tests.
    private let fullFrames: ReplayFullFramesSwitch?
    /// True from the request that switches full frames on until the one that
    /// switches them off is sent.
    private var fullFramesRequested = false
    /// The request switching full frames on, and the one switching them off;
    /// each waits for the one before, so the server sees them in order.
    private var fullFramesOn: Task<Void, Never>?
    private var fullFramesOff: Task<Void, Never>?
    /// Bumped by each accumulation, so a late answer to an earlier one's
    /// request cannot stop this one.
    private var accumulationGeneration = 0
    /// The VRLOG loaded now was recorded with full frames (frame_mode full).
    private var loadedRunHoldsFullFrames = false
    /// What the loaded run calls its capture: its clip id, else its run id.
    /// A background replayed from it is uploaded under this name, not the
    /// site picker's clip, which may be another position at the same site.
    private var loadedRunCaptureRef: String?
    /// How the pose drawn now was made, for what Accept records.
    private var poseSource: Gerardus_Align_V1alpha1_PoseSource = .hint
    private var registering: Task<Void, Never>?
    private var evaluating: Task<Void, Never>?
    /// The EvaluatePose call on the wire, if any. Cancelling a call here does
    /// not stop it on the server, so the next waits for it: overlapping calls
    /// each build their 3DEP reference there, and ten at once ran Gerardus on
    /// arrow out of memory.
    private var onTheWire: Task<Void, Never>?
    /// Bumped whenever the drawn pose changes, so a late evaluation of an
    /// earlier pose is dropped.
    private var poseGeneration = 0
    /// How long the pose must rest before it is scored: the end of a burst
    /// of nudges, standing in for the release of a drag.
    var evaluateDelay: Duration = .milliseconds(400)
    /// How that rest is waited out. Tests replace it to await the
    /// evaluation itself instead of a clock.
    var evaluateSleep: @MainActor (Duration) async throws -> Void = {
        try await Task.sleep(for: $0)
    }
    private var hintFrameIDs: (site: String, sensor: String) = ("", "")
    /// What was streamed, kept so the overlay can be rebuilt when a toggle or
    /// the site model changes; nil until placed.
    private var geometry: ReferenceGeometry?
    /// What was streamed, for the street painter's snapping and ground.
    var referenceGeometry: ReferenceGeometry? { geometry }
    /// The streaming centre: the hint pose's origin, in the site frame.
    private var referenceCentre: SIMD3<Double>?
    /// The site model the registered layers drawn now came from, as the
    /// stream or GetSiteModel named it; empty when neither did.
    private var siteModelID = ""
    /// The scan whose registered copy the overlay was last built without.
    private var styledOwnScanID: String?
    /// Each site-model scan's sensor position in the site frame, from
    /// GetSiteModel, for framing a scan the refinement did not move.
    private var siteModelPositions: [String: SIMD3<Double>] = [:]
    /// Bumped by each rebuild, so a slow one cannot replace a newer one.
    private var styleGeneration = 0
    private var styling: Task<Void, Never>?

    /// `lookup` nil skips velocity.report entirely, as tests do; `defaults`
    /// nil keeps typed headings and the server address out of the app's
    /// preferences. `fullFrames` nil takes velocity.report's switch at the
    /// lookup's server, and none without a lookup.
    init(
        makeClient: @escaping (GerardusAddress) -> AlignmentServiceClient = {
            GerardusClient(address: $0)
        }, lookup: AlignmentSiteLookupClient? = AlignmentSiteLookupClient(),
        defaults: UserDefaults? = .standard, fullFrames: ReplayFullFramesSwitch? = nil
    ) {
        self.makeClient = makeClient
        self.lookup = lookup
        self.fullFrames =
            fullFrames ?? lookup.map { ReplayFullFramesClient(baseURL: $0.configuredBaseURL) }
        self.defaults = defaults
        self.serviceAddress = GerardusAddress.saved(in: defaults)
        self.reviewer = Self.savedReviewer(in: defaults)
    }

    /// The reviewer last typed, else the annotation operator's name, else
    /// the account's. A name saved blank (the field cleared once) is passed
    /// over: kept, it left every decision disabled from then on.
    static func savedReviewer(in defaults: UserDefaults?) -> String {
        let named = [
            defaults?.string(forKey: reviewerKey),
            defaults?.string(forKey: "annotation.operatorName"),
        ].compactMap { $0?.trimmingCharacters(in: .whitespaces) }.first { !$0.isEmpty }
        return named ?? NSFullUserName()
    }

    // MARK: - Availability

    /// Asks the alignment service what it is. When nothing answers, the
    /// session stays unavailable and every alignment control stays hidden.
    func probe() async {
        let generation = addressGeneration
        let candidate = client ?? makeClient(serviceAddress)
        if !isChecking { isChecking = true }
        var info: Gerardus_Align_V1alpha1_GetServerInfoResponse?
        var failure: String?
        do { info = try await candidate.connect() } catch {
            failure = GerardusClientError.describe(error)
        }
        if generation != addressGeneration {
            // The address changed while this was asked: the answer is stale.
            if info != nil { candidate.disconnect() }
        } else if let info {
            client = candidate
            availableLayers = info.availableLayers
            serverInfo = info
            availability = .available(serverVersion: info.serverVersion)
            probeFailure = nil
            isChecking = false
            if bundle == nil { Task { await refreshBundle() } }
        } else {
            client = nil
            serverInfo = nil
            // Set only on a change, so a repeated miss redraws nothing.
            if availability != .unavailable { availability = .unavailable }
            if probeFailure != failure { probeFailure = failure }
            isChecking = false
        }
    }

    /// Settings' Test connection: GetServerInfo on a channel of its own,
    /// timed, at `address` (the one in use when nil). The session's own
    /// connection is left alone, except that its server, found while the
    /// session had none, is taken up at once.
    func testConnection(
        _ address: GerardusAddress? = nil, now: ServerConnectionTest.Clock = { .now }
    ) async -> ConnectionTestResult {
        let target = address ?? serviceAddress
        let client = makeClient(target)
        let result = await ServerConnectionTest.gerardus(connect: client.connect, now: now)
        client.disconnect()
        if result.isOK && target == serviceAddress && !isAvailable && !isChecking { await probe() }
        return result
    }

    /// Points the session at another Gerardus server, and keeps the address
    /// as the app's preference. Whatever was placed against the old server
    /// is dropped, the pane closes until the new one answers, and the new
    /// one is asked at once. Called again with the same address, it simply
    /// asks again.
    func useServiceAddress(_ address: GerardusAddress) {
        let connected = probing != nil
        stopProbing()
        addressGeneration += 1
        client?.disconnect()
        client = nil
        serverInfo = nil
        serviceAddress = address
        address.save(to: defaults)
        clearPlacement()
        availability = .unknown
        probeFailure = nil
        isChecking = false
        // While connected the loop keeps asking every reprobeInterval;
        // otherwise one answer is enough to show whether the address works.
        if connected { startProbing() } else { Task { await probe() } }
    }

    /// How long to wait between probes while no server answers.
    var reprobeInterval: Duration = .seconds(30)
    /// How the loop waits out `reprobeInterval`. Tests replace it so they
    /// can await the loop itself instead of a clock.
    var reprobeSleep: @MainActor (Duration) async throws -> Void = { try await Task.sleep(for: $0) }
    /// The loop `startProbing()` runs; it ends once a server answers.
    private(set) var probing: Task<Void, Never>?

    /// Probes now and, while nothing answers, again every `reprobeInterval`
    /// until a server does or `stopProbing()` is called, so a Gerardus
    /// started after the visualiser connected still brings up the pane.
    func startProbing() {
        probing?.cancel()
        probing = Task { [weak self] in
            while !Task.isCancelled {
                guard await self?.probeOnce() == false, let interval = self?.reprobeInterval,
                    let nap = self?.reprobeSleep
                else { break }
                try? await nap(interval)
            }
        }
    }

    func stopProbing() {
        probing?.cancel()
        probing = nil
    }

    /// One probe; true once a server has answered.
    private func probeOnce() async -> Bool {
        await probe()
        return isAvailable
    }

    // MARK: - Site

    /// Lists velocity.report's located cases and picks the open capture's.
    /// A site the operator chose by hand is kept unless the capture names
    /// another one.
    func refreshSites(runID: String?) async {
        guard isAvailable, let lookup else { return }
        do { sites = try await lookup.locatedSites() } catch {
            errorMessage = "velocity.report has no site list: \(error.localizedDescription)"
            return
        }
        let evidence = await lookup.openCaptureEvidence(runID: runID)
        var found = AlignmentSiteResolver.resolve(evidence, sites: sites, filesByCase: [:])
        if found == nil, !evidence.sourcePaths.isEmpty {
            var files = [String: [String]]()
            for site in sites {
                files[site.clipID] = (try? await lookup.captureFiles(clipID: site.clipID)) ?? []
            }
            found = AlignmentSiteResolver.resolve(evidence, sites: sites, filesByCase: files)
        }
        if let found {
            if found.site != site { select(found.site) }
            siteMatch = found.match
        } else if site == nil {
            siteMatch = .none
        }
    }

    /// The operator picks the site from the list.
    func choose(_ chosen: AlignmentSite) {
        guard chosen != site else { return }
        select(chosen)
        siteMatch = .chosen
    }

    private func select(_ chosen: AlignmentSite) {
        site = chosen
        northAzimuthText =
            defaults?.string(forKey: Self.northAzimuthKeyPrefix + chosen.l16Token) ?? ""
        clearPlacement()
        bundle = nil
        confirmingRegenerate = false
        referenceDataNote = nil
        Task { await refreshBundle() }
    }

    /// The typed north_azimuth_deg, or nil when the field is empty or
    /// unreadable.
    var northAzimuthDeg: Double? {
        let text = northAzimuthText.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty, let value = Double(text), value.isFinite else { return nil }
        return SitePose.wrap360(value)
    }

    /// The heading the hint sends: the bearing of the sensor's +Y.
    var hintHeadingDeg: Double? { northAzimuthDeg.map(Self.hintHeading(northAzimuthDeg:)) }

    /// north_azimuth_deg is measured with the web scene viewer's "Align from
    /// above" tool, against that viewer's camera rather than the sensor's
    /// axes, so it is not the bearing of +Y. Gerardus's registration, which
    /// searched every heading and ignored the hint, found the sensor's +Y at
    /// 143.5°, 49.4° and 129.9° at sites whose north_azimuth_deg is 35, 131
    /// and 51: 180° − N each time, within 1.5°.
    nonisolated static func hintHeading(northAzimuthDeg: Double) -> Double {
        SitePose.wrap360(180 - northAzimuthDeg)
    }

    // MARK: - Placement

    /// Prepares the site, places the sensor from its eye-read hint, and
    /// streams the reference around that position.
    func placeFromHint() async {
        guard let client, let site, busy == nil else { return }
        if let northAzimuth = northAzimuthDeg {
            defaults?.set(String(northAzimuth), forKey: Self.northAzimuthKeyPrefix + site.l16Token)
        }
        let heading = hintHeadingDeg
        errorMessage = nil
        defer {
            busy = nil
            progress = nil
        }
        do {
            busy = "Preparing \(site.name)"
            let prepared = try await client.prepareSite(l16Token: site.l16Token, label: site.name) {
                [weak self] progress in await self?.show(progress)
            }
            preparedSite = prepared
            bundle = prepared

            busy = "Placing from the hint"
            var request = Gerardus_Align_V1alpha1_PoseFromHintRequest()
            request.l16Token = site.l16Token
            request.sensorID = sensorID
            request.hint = hint(heading: heading)
            let placed = try await client.poseFromHint(request)
            let start = SitePose(placed.pose)
            hintFrameIDs = (placed.pose.siteFrameID, placed.pose.sensorFrameID)
            hintPose = start
            hintSummary = placed.summary
            pose = start
            poseSource = .hint
            poseGeneration += 1
            // An accumulated scan is kept: it belongs to the site, not the pose.
            scan = accumulatedScan
            registration = nil
            hypotheses = []
            selectedHypothesisID = nil
            selectedRecordID = nil
            evaluation = nil
            sink?.setResidualOverlay(nil, palette: ResidualStyle.palette)

            let (geometry, registered) = try await streamReferenceLayers(
                client: client, l16Token: site.l16Token, centre: start.translation)
            self.geometry = geometry
            referenceCentre = start.translation
            siteModelID = geometry.modelID
            await restyleReference()
            sink?.setReferenceModelMatrix(start.referenceModelMatrix)
            logger.info(
                "Reference for \(site.l16Token): \(geometry.aerialCount) points, \(geometry.lines.count) lines, \(geometry.footprints.count) footprints, \(geometry.boxes.count) box walls, \(geometry.intersections.count) intersections, \(geometry.scanOrder.count) registered scans (\(geometry.scanPointCount) points), \(geometry.observed.count) observed features, \(geometry.wallPlanes.count) wall planes, \(geometry.groundMeshCells) ground cells, model \(geometry.modelID)"
            )

            busy = "Reading the site model"
            await readSiteModelScans(client: client, l16Token: site.l16Token, streamed: registered)

            busy = "Reading records"
            await loadRecords()
            // The last decision is the best knowledge of where the sensor
            // stands: draw it, so Register starts from it rather than the
            // eye-read hint. Reset nudges goes back to the hint.
            if let record = Self.defaultRecord(
                records, captureRef: loadedRunCaptureRef ?? site.clipID, scanID: scan?.scanID)
            {
                showRecord(record)
            }
        } catch { errorMessage = GerardusClientError.describe(error) }
    }

    private func show(_ progress: Gerardus_Align_V1alpha1_Progress) { self.progress = progress }

    /// The reference around `centre`, layer group by layer group, and the
    /// registered layers on their own so the caller can read the site model.
    private func streamReferenceLayers(
        client: AlignmentServiceClient, l16Token: String, centre: SIMD3<Double>
    ) async throws -> (geometry: ReferenceGeometry, registered: ReferenceGeometry) {
        busy = "Streaming the reference"
        var geometry = try await client.streamReference(
            referenceRequest(l16Token: l16Token, centre: centre))
        busy = "Streaming the streets"
        geometry.merge(await streamStreetLayers(client: client, l16Token: l16Token, centre: centre))
        busy = "Streaming buildings, markings, furniture and trees"
        geometry.merge(
            await streamFeatureLayers(client: client, l16Token: l16Token, centre: centre))
        busy = "Streaming the site model"
        let registered = await streamRegisteredLayers(
            client: client, l16Token: l16Token, centre: centre)
        geometry.replaceRegistered(with: registered)
        return (geometry, registered)
    }

    // MARK: - Reference data

    /// Reads how current the chosen site's stored bundle is. Builds nothing.
    func refreshBundle() async {
        guard let client, let site else { return }
        let found: Gerardus_Align_V1alpha1_Site?
        do { found = try await client.getSite(l16Token: site.l16Token) } catch {
            logger.info(
                "No bundle read for \(site.l16Token): \(GerardusClientError.describe(error))")
            return
        }
        guard self.site == site, busy == nil else { return }
        bundle = found
    }

    /// Layers this server builds into a bundle and the stored one lacks.
    var missingReferenceLayers: [Gerardus_Align_V1alpha1_ReferenceLayer] {
        guard let bundle else { return [] }
        return ReferenceDataStatus.missingLayers(bundle: bundle, available: availableLayers)
    }

    /// The bundle is older than what the server can build.
    var referenceDataIsOutOfDate: Bool { !missingReferenceLayers.isEmpty }

    /// Update reference data: Gerardus rebuilds a bundle that lacks layers
    /// it can build now, from the downloads it already holds. Nothing is
    /// downloaded again and the 3DEP cut is kept.
    func updateReferenceData() async { await prepareAgain(refresh: false) }

    /// Regenerate…: opens the confirmation. Nothing is sent yet.
    func requestRegenerate() {
        guard client != nil, site != nil, bundle != nil, busy == nil else { return }
        confirmingRegenerate = true
    }

    func cancelRegenerate() { confirmingRegenerate = false }

    /// Sends `refresh = true`, and only once the operator has confirmed.
    func confirmRegenerate() async {
        guard confirmingRegenerate else { return }
        confirmingRegenerate = false
        await prepareAgain(refresh: true)
    }

    private func prepareAgain(refresh: Bool) async {
        guard let client, let site, busy == nil else { return }
        errorMessage = nil
        referenceDataNote = nil
        let before = Set(missingReferenceLayers)
        defer {
            busy = nil
            progress = nil
        }
        do {
            busy =
                refresh ? "Regenerating \(site.name)" : "Updating reference data for \(site.name)"
            let prepared = try await client.prepareSite(
                l16Token: site.l16Token, label: site.name, refresh: refresh
            ) { [weak self] progress in await self?.show(progress) }
            guard self.site == site else { return }
            bundle = prepared
            let after = Set(missingReferenceLayers)
            referenceDataNote = ReferenceDataStatus.outcome(
                refresh: refresh, before: before, after: after)
            if preparedSite != nil { preparedSite = prepared }
            // Drawn already: draw it again from the new bundle at the same place.
            if let centre = referenceCentre, geometry != nil {
                let (fresh, registered) = try await streamReferenceLayers(
                    client: client, l16Token: site.l16Token, centre: centre)
                guard self.site == site, referenceCentre == centre else { return }
                geometry = fresh
                siteModelID = fresh.modelID
                await restyleReference()
                busy = "Reading the site model"
                await readSiteModelScans(
                    client: client, l16Token: site.l16Token, streamed: registered)
            }
        } catch { errorMessage = GerardusClientError.describe(error) }
    }

    /// The bundle's layers, on one call.
    static let bundleLayers: [Gerardus_Align_V1alpha1_ReferenceLayer] = [
        .aerialPoints, .osm, .footprints, .intersections,
    ]
    /// Reference layers asked for on the bundle's call only when the server
    /// lists them: a server from before them would refuse a layer it does
    /// not know, and with it the bundle.
    static let listedOnlyLayers: [Gerardus_Align_V1alpha1_ReferenceLayer] = [.boxes]
    /// The street layers the street painter draws over and snaps to, on one
    /// call of their own and only when the server lists them: a server from
    /// before them would refuse the call.
    static let streetLayers: [Gerardus_Align_V1alpha1_ReferenceLayer] = [
        .cityStreets, .cityIntersections, .kerbs,
    ]
    /// The W4–W7 layers (Gerardus's buildings, lane markings, street
    /// furniture and trees), each on a call of its own and only when the
    /// server lists it: they are built in parallel, so one not working yet
    /// must not take the others with it.
    static let featureLayers: [Gerardus_Align_V1alpha1_ReferenceLayer] = [
        .buildings, .laneMarkings, .streetFurniture, .trees,
    ]
    /// The site model's layers, each on a call of its own.
    static let registeredLayers: [Gerardus_Align_V1alpha1_ReferenceLayer] = [
        .registeredScans, .observed, .groundMesh,
    ]

    /// The wanted layers the server lists; all of them when it lists none.
    private func served(
        _ wanted: [Gerardus_Align_V1alpha1_ReferenceLayer]
    ) -> [Gerardus_Align_V1alpha1_ReferenceLayer] {
        wanted.filter { availableLayers.isEmpty || availableLayers.contains($0) }
    }

    func referenceRequest(
        l16Token: String, centre: SIMD3<Double>,
        layers: [Gerardus_Align_V1alpha1_ReferenceLayer]? = nil
    ) -> Gerardus_Align_V1alpha1_StreamReferenceRequest {
        var request = Gerardus_Align_V1alpha1_StreamReferenceRequest()
        request.l16Token = l16Token
        request.layers =
            layers
            ?? (served(Self.bundleLayers) + Self.listedOnlyLayers.filter(availableLayers.contains))
        request.centre.x = centre.x
        request.centre.y = centre.y
        request.centre.z = centre.z
        request.radiusM = Self.referenceRadiusM
        // 0 asks for the server's default spacing.
        request.voxelSizeM = 0
        return request
    }

    /// The street layers the server lists, on one request; nil when it
    /// lists none of them.
    func streetReferenceRequest(
        l16Token: String, centre: SIMD3<Double>
    ) -> Gerardus_Align_V1alpha1_StreamReferenceRequest? {
        let listed = Self.streetLayers.filter(availableLayers.contains)
        guard !listed.isEmpty else { return nil }
        return referenceRequest(l16Token: l16Token, centre: centre, layers: listed)
    }

    /// Streams the street layers. A failure leaves them empty and says so
    /// in `streetLayersNote`; the bundle drawn already is untouched.
    private func streamStreetLayers(
        client: AlignmentServiceClient, l16Token: String, centre: SIMD3<Double>
    ) async -> ReferenceGeometry {
        streetLayersNote = nil
        guard let request = streetReferenceRequest(l16Token: l16Token, centre: centre) else {
            return ReferenceGeometry()
        }
        do { return try await client.streamReference(request) } catch {
            streetLayersNote = "Streets and kerbs not drawn: \(GerardusClientError.describe(error))"
            logger.info("No street layers for \(l16Token): \(GerardusClientError.describe(error))")
            return ReferenceGeometry()
        }
    }

    /// The W4–W7 layers the server lists, one request each; none when it
    /// lists none of them, or lists no layers at all.
    func featureReferenceRequests(
        l16Token: String, centre: SIMD3<Double>
    ) -> [Gerardus_Align_V1alpha1_StreamReferenceRequest] {
        Self.featureLayers.filter(availableLayers.contains).map {
            referenceRequest(l16Token: l16Token, centre: centre, layers: [$0])
        }
    }

    /// Streams each W4–W7 layer the server lists on its own call. A layer
    /// that fails is left empty and named in `featureLayersNote`.
    private func streamFeatureLayers(
        client: AlignmentServiceClient, l16Token: String, centre: SIMD3<Double>
    ) async -> ReferenceGeometry {
        var features = ReferenceGeometry()
        var notes = [String]()
        for request in featureReferenceRequests(l16Token: l16Token, centre: centre) {
            let layer = request.layers.first ?? .unspecified
            do { features.merge(try await client.streamReference(request)) } catch {
                notes.append(Self.note(layer: layer, error: error))
                logger.info(
                    "No \(Self.layerName(layer)) for \(l16Token): \(GerardusClientError.describe(error))"
                )
            }
        }
        featureLayersNote = notes.isEmpty ? nil : notes.joined(separator: "\n")
        return features
    }

    /// The registered layers the server lists, one request each.
    func registeredReferenceRequests(
        l16Token: String, centre: SIMD3<Double>
    ) -> [Gerardus_Align_V1alpha1_StreamReferenceRequest] {
        served(Self.registeredLayers).map {
            referenceRequest(l16Token: l16Token, centre: centre, layers: [$0])
        }
    }

    /// Streams each registered layer on its own call. A layer the server
    /// refuses or fails is left empty and named in `registeredLayersNote`;
    /// the others, and the bundle's layers, are untouched. A layer served
    /// empty (no accepted scans yet) is simply empty.
    private func streamRegisteredLayers(
        client: AlignmentServiceClient, l16Token: String, centre: SIMD3<Double>
    ) async -> ReferenceGeometry {
        var registered = ReferenceGeometry()
        var notes = [String]()
        for request in registeredReferenceRequests(l16Token: l16Token, centre: centre) {
            let layer = request.layers.first ?? .unspecified
            do { registered.merge(try await client.streamReference(request)) } catch {
                notes.append(Self.note(layer: layer, error: error))
                logger.info(
                    "No \(Self.layerName(layer)) for \(l16Token): \(GerardusClientError.describe(error))"
                )
            }
        }
        registeredLayersNote = notes.isEmpty ? nil : notes.joined(separator: "\n")
        return registered
    }

    nonisolated static func layerName(_ layer: Gerardus_Align_V1alpha1_ReferenceLayer) -> String {
        switch layer {
        case .registeredScans: return "Registered scans"
        case .observed: return "Observed features"
        case .groundMesh: return "Ground mesh"
        case .buildings: return "Buildings"
        case .laneMarkings: return "Lane markings"
        case .streetFurniture: return "Street furniture"
        case .trees: return "Trees"
        default: return "Layer \(layer.rawValue)"
        }
    }

    nonisolated static func note(
        layer: Gerardus_Align_V1alpha1_ReferenceLayer, error: Error
    ) -> String {
        if case GerardusClientError.permissionDenied = error, layer == .registeredScans {
            return
                "Registered scans not served to this machine: Gerardus sends them only under `serve --serve-scans`, to loopback and tailnet peers."
        }
        return "\(layerName(layer)) not drawn: \(GerardusClientError.describe(error))"
    }

    private func hint(heading: Double?) -> Gerardus_Align_V1alpha1_PoseHint {
        var hint = Gerardus_Align_V1alpha1_PoseHint()
        if let site {
            hint.position.latitudeDeg = site.latitude
            hint.position.longitudeDeg = site.longitude
        }
        // The bearing of the sensor's +Y, already derived from
        // north_azimuth_deg (hintHeading(northAzimuthDeg:)).
        if let heading { hint.headingDeg = heading }
        return hint
    }

    private var sensorID: String {
        guard let id = site?.sensorID, !id.isEmpty else { return "lidar" }
        return id
    }

    private func attribution(
        _ site: Gerardus_Align_V1alpha1_Site, layer: Gerardus_Align_V1alpha1_ReferenceLayer
    ) -> String {
        site.sources.filter { $0.layer == layer }.map(\.attribution).filter { !$0.isEmpty }.joined(
            separator: "; ")
    }

    // MARK: - Nudging

    /// Moves the reference by one step in the sensor frame. Local only: the
    /// pose and the model matrix change, and nothing is sent.
    func nudge(_ step: PoseNudge) {
        guard let current = pose else { return }
        setPose(current.nudged(by: step), source: .manual)
    }

    func nudge(x: Double = 0, y: Double = 0, z: Double = 0, yawSteps: Double = 0) {
        nudge(PoseNudge(offset: SIMD3(x, y, z) * translationStepM, yawDeg: yawSteps * yawStepDeg))
    }

    func resetNudges() {
        guard let hintPose else { return }
        setPose(hintPose, source: .hint)
    }

    /// Draws one of the registration's hypotheses. Accept then records that
    /// hypothesis, until the pose is moved off it.
    func showHypothesis(_ hypothesis: Gerardus_Align_V1alpha1_Hypothesis) {
        setPose(SitePose(hypothesis.pose), source: .icp, hypothesis: hypothesis.hypothesisID)
    }

    /// Snaps the drawn pose to a record's: the sensor where that decision put
    /// it. Accept then records it again as MANUALLY_PLACED, the operator
    /// having chosen it. A rejected record carries no pose and is left alone.
    func showRecord(_ record: Gerardus_Align_V1alpha1_RegistrationRecord) {
        guard record.hasPose else { return }
        setPose(SitePose(record.pose), source: .manual, record: record.recordID)
    }

    /// The record drawn by default once a site's records are read on
    /// placement: the newest with a pose for this capture, else the newest
    /// with a pose at the site. Nil when none has one.
    nonisolated static func defaultRecord(
        _ records: [Gerardus_Align_V1alpha1_RegistrationRecord], captureRef: String?,
        scanID: String?
    ) -> Gerardus_Align_V1alpha1_RegistrationRecord? {
        let posed = records.filter(\.hasPose)
        let own = posed.first { record in
            (captureRef.map { !$0.isEmpty && record.captureRef == $0 } ?? false)
                || (scanID.map { !$0.isEmpty && record.scanID == $0 } ?? false)
        }
        return own ?? posed.first
    }

    /// Draws a new pose. Any residual colouring belongs to the old one, so it
    /// goes; the new pose is scored once it has rested for `evaluateDelay`.
    /// Moving the pose sends nothing until then.
    private func setPose(
        _ newPose: SitePose, source: Gerardus_Align_V1alpha1_PoseSource, hypothesis: String? = nil,
        record: String? = nil, evaluate: Bool = true
    ) {
        pose = newPose
        poseSource = source
        selectedHypothesisID = hypothesis
        selectedRecordID = record
        poseGeneration += 1
        evaluation = nil
        sink?.setReferenceModelMatrix(newPose.referenceModelMatrix)
        sink?.setResidualOverlay(nil, palette: ResidualStyle.palette)
        if evaluate { scheduleEvaluation() }
    }

    var visibility: ReferenceVisibility {
        ReferenceVisibility(
            aerial: showAerial, osm: showOSM, footprints: showFootprints,
            boxes: showBoxes && showFaces, intersections: showIntersections,
            registeredScans: showRegisteredScans, observed: showObserved,
            wallPlanes: showWallPlanes && showFaces, groundMesh: showGroundMesh, kerbs: showKerbs,
            cityStreets: showCityStreets, buildings: showBuildings, laneMarkings: showLaneMarkings,
            streetFurniture: showStreetFurniture, trees: showTrees)
    }

    private func pushVisibility() { sink?.setReferenceVisibility(visibility) }

    // MARK: - Reference overlay

    /// Scans left out of the registered layer: the open capture's own, by
    /// scan_id, once it is known from the upload, unless the operator asks
    /// for it.
    private var hiddenScanIDs: Set<String> {
        guard !showOwnScan, let id = scan?.scanID, !id.isEmpty else { return [] }
        return [id]
    }

    /// Rebuilds the overlay from the geometry kept, off the main actor, and
    /// hands it to the renderer with the counts the pane shows.
    func restyleReference() async {
        guard let geometry else { return }
        styleGeneration += 1
        let generation = styleGeneration
        let centre = referenceCentre
        let hidden = hiddenScanIDs
        let emphasis = refinementEmphasis
        let roofs = roofColouring
        let vertices = await Task.detached {
            ReferenceStyle.vertices(
                for: geometry, centre: centre, hiddenScanIDs: hidden, emphasis: emphasis,
                roofColouring: roofs)
        }.value
        guard generation == styleGeneration else { return }
        styledOwnScanID = scan?.scanID
        reference = summary(geometry, vertices: vertices)
        sink?.setReferenceOverlay(vertices)
        pushVisibility()
    }

    /// Rebuilds the overlay in a task, for callers that cannot wait.
    private func scheduleRestyle() {
        guard geometry != nil else { return }
        styling = Task { [weak self] in await self?.restyleReference() }
    }

    /// The task rebuilding the overlay, for tests to await.
    var pendingStyling: Task<Void, Never>? { styling }

    /// The open capture's scan changed: rebuild if its registered copy is
    /// drawn, or should now be hidden.
    private func ownScanChanged() async {
        guard geometry != nil, scan?.scanID != styledOwnScanID else { return }
        await restyleReference()
    }

    private func summary(
        _ geometry: ReferenceGeometry, vertices: ReferenceOverlayVertices
    ) -> ReferenceSummary {
        let own = scan?.scanID
        let ownPoints = geometry.scans.filter { own != nil && $0.scanID == own }.reduce(0) {
            $0 + ReferenceGeometry.count($1.points)
        }
        var out = ReferenceSummary(
            aerialPoints: geometry.aerialCount, osmFeatures: geometry.lines.count,
            intersections: geometry.intersections.count,
            osmAttribution: preparedSite.map { attribution($0, layer: .osm) } ?? "",
            aerialAttribution: preparedSite.map { attribution($0, layer: .aerialPoints) } ?? "")
        out.aerialOutlines = geometry.footprints.filter(ReferenceStyle.isAerialOutline).count
        out.footprints = geometry.footprints.count - out.aerialOutlines
        out.footprintAttribution = preparedSite.map { attribution($0, layer: .footprints) } ?? ""
        out.boxesServed = !geometry.boxes.isEmpty || geometry.completeLayers.contains(.boxes)
        out.boxWalls = geometry.boxes.count
        out.partyWalls = geometry.partyWallCount
        out.registeredScans = geometry.scanOrder.count
        out.registeredPoints = vertices.scanCount
        out.ownScanPoints = ownPoints
        out.observedFeatures = geometry.observed.count
        out.wallPlanes = geometry.wallPlanes.count
        out.wallPlaneRoles = geometry.wallPlaneRoles
        out.groundMeshCells = geometry.groundMeshCells
        out.cityStreets = geometry.cityStreets.count
        out.cityIntersections = geometry.cityIntersections.count
        out.kerbs = geometry.kerbs.count
        out.streetLayersServed = Self.streetLayers.contains(where: availableLayers.contains)
        out.featureLayersServed = Set(Self.featureLayers.filter(availableLayers.contains))
        out.buildingParts = geometry.buildingParts.count
        out.roofPlanes = geometry.roofPlanes.count
        out.wallSurfaces = geometry.wallSurfaces.count
        out.laneMarkings = geometry.laneMarkings.count
        out.poles = geometry.streetFurniture.filter { $0.kind == .pole }.count
        out.signs = geometry.streetFurniture.filter { $0.kind == .sign }.count
        out.trees = geometry.trees.count
        out.modelID = geometry.modelID
        return out
    }

    /// After a decision, asks whether the site model moved, and re-streams
    /// the registered layers if it did. With no answer to compare (a server
    /// that answers GetSiteModel UNIMPLEMENTED, or an error) they are
    /// re-streamed anyway: an accept is what moves the model.
    private func refreshSiteModel(client: AlignmentServiceClient) async {
        guard let site, let centre = referenceCentre, geometry != nil,
            !served(Self.registeredLayers).isEmpty
        else { return }
        var current: Gerardus_Align_V1alpha1_GetSiteModelResponse?
        do { current = try await client.getSiteModel(l16Token: site.l16Token) } catch {
            logger.info(
                "No site model for \(site.l16Token): \(GerardusClientError.describe(error))")
        }
        if let current {
            siteModelScanIDs = current.scans.map(\.scanID)
            noteSiteModel(answer: current, streamed: geometry)
        }
        if let current, !current.modelID.isEmpty, current.modelID == siteModelID { return }
        // Put back as found: left set, it blocked every control after an
        // Accept refinement, which had already cleared its own.
        let caller = busy
        busy = "Re-streaming the site model"
        defer { busy = caller }
        let registered = await streamRegisteredLayers(
            client: client, l16Token: site.l16Token, centre: centre)
        // The site was changed or cleared while this was asked.
        guard self.site == site, referenceCentre == centre, var geometry else { return }
        geometry.replaceRegistered(with: registered)
        self.geometry = geometry
        siteModelID = registered.modelID.isEmpty ? (current?.modelID ?? "") : registered.modelID
        if current == nil {
            siteModelScanIDs = Self.modelScanIDs(answer: nil, streamed: registered)
        }
        noteSiteModel(answer: current, streamed: geometry)
        await restyleReference()
    }

    /// Asks which scans the site model holds, for the switch that registers
    /// against it. Only asked of a server that lists a registered layer.
    private func readSiteModelScans(
        client: AlignmentServiceClient, l16Token: String, streamed: ReferenceGeometry
    ) async {
        guard !served(Self.registeredLayers).isEmpty else {
            siteModelScanIDs = nil
            return
        }
        var answer: Gerardus_Align_V1alpha1_GetSiteModelResponse?
        do { answer = try await client.getSiteModel(l16Token: l16Token) } catch {
            logger.info("No site model for \(l16Token): \(GerardusClientError.describe(error))")
        }
        siteModelScanIDs = Self.modelScanIDs(answer: answer, streamed: streamed)
        noteSiteModel(answer: answer, streamed: streamed)
    }

    /// Keeps each scan's capture_ref and sensor position, for the
    /// refinement summary's labels and for framing a scan.
    private func noteSiteModel(
        answer: Gerardus_Align_V1alpha1_GetSiteModelResponse?, streamed: ReferenceGeometry?
    ) {
        var refs = Self.captureRefs(answer: answer, streamed: streamed)
        for (id, ref) in scanCaptureRefs where refs[id] == nil { refs[id] = ref }
        scanCaptureRefs = refs
        for scan in answer?.scans ?? [] where scan.hasPose {
            siteModelPositions[scan.scanID] = SitePose(scan.pose).translation
        }
    }

    /// capture_ref by scan_id: GetSiteModel's where it names one, else the
    /// registered layer's scan blocks', else the sensor features' tags.
    nonisolated static func captureRefs(
        answer: Gerardus_Align_V1alpha1_GetSiteModelResponse?, streamed: ReferenceGeometry?
    ) -> [String: String] {
        var refs = [String: String]()
        for feature in streamed?.observed ?? [] where feature.kind == .sensor {
            if let id = feature.tags["scan_id"], let ref = feature.tags["capture_ref"], !ref.isEmpty
            {
                refs[id] = ref
            }
        }
        for block in streamed?.scans ?? [] where !block.captureRef.isEmpty {
            refs[block.scanID] = block.captureRef
        }
        for scan in answer?.scans ?? [] where !scan.captureRef.isEmpty {
            refs[scan.scanID] = scan.captureRef
        }
        return refs
    }

    /// GetSiteModel's scans when it answered; else the registered layer's,
    /// when that layer streamed to its end; else unknown.
    nonisolated static func modelScanIDs(
        answer: Gerardus_Align_V1alpha1_GetSiteModelResponse?, streamed: ReferenceGeometry
    ) -> [String]? {
        if let answer { return answer.scans.map(\.scanID) }
        if streamed.completeLayers.contains(.registeredScans) { return streamed.scanOrder }
        return nil
    }

    /// Why Register cannot fit against the site model now, or nil when it
    /// can: the server must list the registered-scans layer, and the model
    /// must hold an accepted scan other than the one being registered.
    var againstSiteModelUnavailable: String? {
        guard !served([.registeredScans]).isEmpty else {
            return "This Gerardus lists no registered-scans layer, so it has no site model."
        }
        guard let ids = siteModelScanIDs else {
            return "Gerardus has not said which scans the site model holds."
        }
        if ids.isEmpty { return "The site model holds no accepted scan yet." }
        let own = scan?.scanID
        if ids.allSatisfy({ $0 == own }) {
            return "The site model holds no accepted scan besides this one."
        }
        return nil
    }

    /// What Register sends as against_site_model.
    var registersAgainstSiteModel: Bool { againstSiteModel && againstSiteModelUnavailable == nil }

    /// Why Register cannot fit against the DataSF boxes now, or nil when it
    /// can: the server must list the boxes layer, and the layer it streamed
    /// must hold walls. Gerardus answers FAILED_PRECONDITION to a site
    /// without them.
    var againstBoxesUnavailable: String? {
        guard availableLayers.contains(.boxes) else {
            return "This Gerardus lists no boxes layer, so it has no DataSF box walls."
        }
        guard let reference else { return "The reference has not been streamed yet." }
        if reference.boxWalls == 0 { return "The boxes layer holds no walls at this site." }
        return nil
    }

    /// What Register sends as against_boxes.
    var registersAgainstBoxes: Bool { againstBoxes && againstBoxesUnavailable == nil }

    // MARK: - Scan

    /// Keeps the newest background snapshot. Called for every background
    /// frame; cheap, because the arrays are shared rather than copied.
    func noteBackground(_ snapshot: BackgroundSnapshot) {
        latestBackground = snapshot
        let status = AlignmentBackgroundStatus(
            points: snapshot.pointCount, sequence: snapshot.sequenceNumber,
            settled: snapshot.gridMetadata?.settlingComplete ?? true)
        if status != background { background = status }
    }

    /// A different capture is open: its background and any run against the
    /// old one no longer apply. The reference stays until a site is placed.
    func captureChanged() {
        registering?.cancel()
        evaluating?.cancel()
        dropCallOnTheWire()
        latestBackground = nil
        background = nil
        dropAccumulation()
        streamRefusal = nil
        frameSource = .unspecified
        logStartNs = 0
        logEndNs = 0
        loadedRunHoldsFullFrames = false
        loadedRunCaptureRef = nil
        scan = nil
        uploadedSnapshot = nil
        registration = nil
        hypotheses = []
        selectedHypothesisID = nil
        evaluation = nil
        sink?.setResidualOverlay(nil, palette: ResidualStyle.palette)
        // The old capture's registered copy is no longer this capture's own.
        if geometry != nil, styledOwnScanID != nil { scheduleRestyle() }
    }

    /// A scan to register: the accumulated one, or a settled background.
    private var hasScanToRegister: Bool {
        accumulatedScan != nil || (background?.settled == true && (background?.points ?? 0) > 0)
    }

    var canRegister: Bool { busy == nil && pose != nil && site != nil && hasScanToRegister }

    /// Uploads the settled background unless this one is already up, or
    /// takes the accumulated scan, then registers it from the current pose.
    ///
    /// Written without a `defer` or an early return inside `do`: in that shape
    /// the test host aborted in swift_task_dealloc ("freed pointer was not the
    /// last allocation") on the Xcode 26 toolchain.
    func register(drawBest: Bool = true) async {
        guard let client, let site, let pose, canRegister else { return }
        // Taken here, as the first version took it, rather than inside
        // `registrationScan`: without it in this frame the test host aborted
        // in swift_task_dealloc on the Xcode 26 toolchain.
        let snapshot = latestBackground
        evaluating?.cancel()
        errorMessage = nil
        registration = nil
        hypotheses = []
        isRegistering = true
        var result: Gerardus_Align_V1alpha1_Registration?
        do {
            busy = "Uploading the background"
            let uploaded = try await registrationScan(
                client: client, site: site, snapshot: snapshot)
            busy = "Registering"
            var request = Gerardus_Align_V1alpha1_RegisterRequest()
            request.scanID = uploaded.scanID
            // Search around the pose drawn now, at every heading. An
            // operator acceptance only needs the persisted manual run.
            request.initialPose = posePayload(pose)
            if supportsBoundedPlacement {
                request.options.searchRadiusM = Float(searchRadiusM)
                request.manualOnly = !drawBest
            }
            request.hint = hint(heading: hintHeadingDeg)
            // Decided once the scan's id is known, so a model holding only
            // this scan is not asked for.
            request.againstSiteModel = registersAgainstSiteModel
            request.againstBoxes = registersAgainstBoxes
            registeredAgainst = AlignmentModelReference(
                siteModel: request.againstSiteModel, boxes: request.againstBoxes)
            result = try await client.register(
                request, onProgress: { [weak self] progress in await self?.show(progress) },
                onHypothesis: { [weak self] hypothesis in await self?.received(hypothesis) })
        } catch {
            errorMessage =
                Task.isCancelled ? "Registration cancelled." : GerardusClientError.describe(error)
        }
        busy = nil
        progress = nil
        isRegistering = false
        if let result {
            registration = result
            hypotheses = result.hypotheses
            // The best hypothesis is drawn at once; a FAILED run with none
            // leaves the operator's pose, to place by hand.
            if drawBest, let best = result.hypotheses.first { showHypothesis(best) }
        }
    }

    /// Registers in a task the pane can cancel.
    func startRegistration() {
        registering?.cancel()
        registering = Task { [weak self] in await self?.register() }
    }

    /// Stops a run in progress: cancelling the call stops it on the server.
    func cancelRegistration() { registering?.cancel() }

    /// The task running registration, for tests to await.
    var pendingRegistration: Task<Void, Never>? { registering }

    private func received(_ hypothesis: Gerardus_Align_V1alpha1_Hypothesis) {
        hypotheses.append(hypothesis)
    }

    /// True when the registration passed every gate, so accepting one of its
    /// hypotheses records REGISTERED.
    var registrationPassed: Bool {
        guard let registration, registration.state == .candidate, !registration.gates.isEmpty else {
            return false
        }
        return registration.gates.allSatisfy(\.passed)
    }

    /// The best hypothesis's fit against what the run added to 3DEP
    /// (`registeredAgainst`), when it measured one (model_points > 0).
    /// Reported beside the gates; it is not one of them.
    var modelFit: Gerardus_Align_V1alpha1_FitQuality? {
        guard let quality = registration?.hypotheses.first?.quality, quality.modelPoints > 0 else {
            return nil
        }
        return quality
    }

    var acceptance: AlignmentAcceptance {
        if let id = selectedHypothesisID, registration != nil,
            let index = hypotheses.firstIndex(where: { $0.hypothesisID == id })
        {
            return .hypothesis(index: index, registered: registrationPassed)
        }
        return .pose
    }

    // MARK: - Evaluation

    /// Scores the pose once it has rested, unless it moves again first.
    private func scheduleEvaluation() {
        evaluating?.cancel()
        guard canRegister else { return }
        let generation = poseGeneration
        evaluating = Task { [weak self] in
            guard let nap = self?.evaluateSleep, let delay = self?.evaluateDelay else { return }
            try? await nap(delay)
            if !Task.isCancelled { await self?.evaluate(refine: false, generation: generation) }
        }
    }

    /// Refines the pose drawn now with local ICP from it (EvaluatePose with
    /// refine), then draws and scores the result.
    func snap() async {
        evaluating?.cancel()
        await evaluate(refine: true, generation: poseGeneration)
    }

    /// The task scoring the pose, for tests to await.
    var pendingEvaluation: Task<Void, Never>? { evaluating }

    private func evaluate(refine: Bool, generation: Int) async {
        while let previous = onTheWire {
            await previous.value
            // A completed task need not suspend this waiter. Clear its slot
            // here too, or we can spin before its owner resumes to clear it.
            if onTheWire == previous { onTheWire = nil }
        }
        // Cancelled while it waited (a snap supersedes a scheduled score), or
        // the pose moved, whose own evaluation follows.
        guard !Task.isCancelled, generation == poseGeneration else { return }
        // Not cancelled with the task that asked: once sent, it runs to the
        // end on the server either way, and a late answer is dropped below.
        let call = Task { await self.send(refine: refine, generation: generation) }
        onTheWire = call
        await call.value
        if onTheWire == call { onTheWire = nil }
    }

    /// Starting over (another capture or site): the call on the wire is
    /// cancelled and not waited for, so one that hangs holds up nothing.
    private func dropCallOnTheWire() {
        onTheWire?.cancel()
        onTheWire = nil
    }

    private func send(refine: Bool, generation: Int) async {
        guard let client, let site, let pose, canRegister else { return }
        let snapshot = latestBackground
        isEvaluating = true
        var response: Gerardus_Align_V1alpha1_EvaluatePoseResponse?
        var failure: String?
        do {
            let uploaded = try await registrationScan(
                client: client, site: site, snapshot: snapshot)
            var request = Gerardus_Align_V1alpha1_EvaluatePoseRequest()
            request.scanID = uploaded.scanID
            request.pose = posePayload(pose)
            request.refine = refine
            if refine && supportsBoundedPlacement { request.searchRadiusM = Float(searchRadiusM) }
            request.includePointResiduals = true
            response = try await client.evaluatePose(request)
        } catch { failure = GerardusClientError.describe(error) }
        isEvaluating = false
        if generation != poseGeneration {
            // The pose moved while this was asked; its own evaluation follows.
        } else if let response {
            if refine { setPose(SitePose(response.pose), source: .icp, evaluate: false) }
            evaluation = PoseEvaluation(
                quality: response.quality, summary: response.summary, refined: refine,
                residuals: response.pointResidualsM)
            pushResiduals()
        } else if let failure {
            errorMessage = failure
        }
    }

    private func pushResiduals() {
        let vertices =
            showResiduals
            ? evaluation.flatMap { evaluation in
                uploadedSnapshot.flatMap {
                    ResidualStyle.vertices(snapshot: $0, residuals: evaluation.residuals)
                }
            } : nil
        sink?.setResidualOverlay(vertices, palette: ResidualStyle.palette)
    }

    /// The scan Register and scoring use: the accumulated one when there is
    /// one, else the latest background, uploaded the first time.
    private func registrationScan(
        client: AlignmentServiceClient, site: AlignmentSite, snapshot: BackgroundSnapshot?
    ) async throws -> Gerardus_Align_V1alpha1_Scan {
        let chosen: Gerardus_Align_V1alpha1_Scan
        if let accumulatedScan {
            chosen = accumulatedScan
        } else if let snapshot {
            chosen = try await uploadedScan(client: client, site: site, snapshot: snapshot)
        } else {
            throw AlignmentScanError.noScan
        }
        return chosen
    }

    /// When the points a scan holds were captured: the capture's time, never
    /// the replay's (ScanPacking.capturedAtNanos).
    private func capturedAtNanos(observed: Int64) -> Int64? {
        ScanPacking.capturedAtNanos(
            source: frameSource, logStartNs: logStartNs, logEndNs: logEndNs, observedNanos: observed
        )
    }

    /// The scan for this background, uploading it the first time.
    private func uploadedScan(
        client: AlignmentServiceClient, site: AlignmentSite, snapshot: BackgroundSnapshot
    ) async throws -> Gerardus_Align_V1alpha1_Scan {
        if let scan, uploadedFrom?.l16Token == site.l16Token,
            uploadedFrom?.sequence == snapshot.sequenceNumber
        {
            return scan
        }
        let header = ScanPacking.header(
            l16Token: site.l16Token, sensorID: sensorID,
            captureRef: loadedRunCaptureRef ?? site.clipID, snapshot: snapshot,
            capturedAtNanos: capturedAtNanos(observed: snapshot.timestampNanos))
        let uploaded = try await client.uploadScan(
            header: header, blocks: ScanPacking.blocks(from: snapshot))
        scan = uploaded.scan
        uploadedFrom = (site.l16Token, snapshot.sequenceNumber)
        uploadedSnapshot = snapshot
        await ownScanChanged()
        return uploaded.scan
    }

    // MARK: - Accumulated frames

    static let liveRefusal =
        "The frames come from a live sensor: open a replay to accumulate frames from it."
    static let vrlogRefusal =
        "This run's VRLOG holds foreground points only; replay its pcap with Full frames, or load a full-frames run."
    static let noFullFramesRefusal =
        "Accumulating from a pcap replay needs velocity.report to stream full frames, and this session has no velocity.report to ask."

    var canAccumulate: Bool {
        busy == nil && client != nil && site != nil && accumulating == nil
            && accumulationTask == nil && streamRefusal == nil
    }

    /// A pcap replay is split-streamed, foreground points only, until
    /// velocity.report is asked for full frames, which Accumulate does.
    private var replaysPCAP: Bool { frameSource == .pcap || frameSource == .pcapAnalysis }

    /// Why the frames from this source cannot be accumulated, whatever they hold.
    private var sourceRefusal: String? {
        switch frameSource {
        case .live: return Self.liveRefusal
        // A full-frames run's VRLOG carries background in every frame, so it
        // accumulates as recorded, with nothing to switch.
        case .vrlog: return loadedRunHoldsFullFrames ? nil : Self.vrlogRefusal
        case .pcap, .pcapAnalysis, .unspecified: return nil
        }
    }

    /// Why this frame's points cannot be accumulated. A pcap replay's split
    /// frames are not a reason: Accumulate switches full frames on and waits.
    private func refusal(for cloud: PointCloudFrame) -> String? {
        if replaysPCAP, cloud.decimationMode == .foregroundOnly { return nil }
        return FrameAccumulation.refusal(cloud)
    }

    /// Called for every frame the visualiser receives. Cheap unless an
    /// accumulation is running: it notes only whether the stream's frames
    /// could be accumulated, and publishes that only when it changes.
    func noteFrame(_ frame: FrameBundle) {
        if let mode = frame.playbackInfo?.sourceMode, mode != .unspecified { frameSource = mode }
        if let playback = frame.playbackInfo, playback.logStartNs > 0 {
            logStartNs = playback.logStartNs
            logEndNs = playback.logEndNs
        }
        if let refusal = sourceRefusal {
            if refusal != streamRefusal { streamRefusal = refusal }
            if accumulator != nil { stopAccumulating(refusal: refusal) }
            return
        }
        guard let cloud = frame.pointCloud else { return }
        let refusal = refusal(for: cloud)
        if refusal != streamRefusal { streamRefusal = refusal }
        guard var collecting = accumulator else { return }
        if let refusal {
            stopAccumulating(refusal: refusal)
            return
        }
        if cloud.decimationMode == .foregroundOnly {
            // Frames sent before the switch took effect: wait for full ones.
            // Without a request in flight none will come.
            if !fullFramesRequested { stopAccumulating(refusal: FrameAccumulation.refusal(cloud)) }
            return
        }
        // The replay's position in its log where it has one (VRLOG), else
        // the frame's own id.
        var number = frame.frameID
        if let playback = frame.playbackInfo, playback.totalFrames > 0 {
            number = playback.currentFrameIndex
        }
        // Released while the frame is added, so the growing arrays are not
        // copied on write.
        accumulator = nil
        switch collecting.add(cloud, tracks: frame.tracks?.tracks ?? [], frameNumber: number) {
        case .refused(let reason): stopAccumulating(refusal: reason)
        case .skipped: accumulator = collecting
        case .taken:
            accumulator = collecting
            accumulating = AlignmentAccumulationProgress(
                frames: collecting.framesTaken, of: collecting.frameCount,
                points: collecting.pointsKept)
            if collecting.isComplete { finishAccumulating() }
        }
    }

    /// Starts collecting `accumulateFrameCount` frames from the replay. During
    /// a pcap replay it first asks velocity.report for full frames, which it
    /// switches off again however the collection ends.
    /// `run` names the replay in the scan's capture_ref; the site's clip id
    /// stands in when the replay has no run id.
    func startAccumulating(run: String?) {
        guard canAccumulate, let site else { return }
        accumulationRefusal = nil
        errorMessage = nil
        if replaysPCAP {
            guard let fullFrames else {
                accumulationRefusal = Self.noFullFramesRefusal
                return
            }
            requestFullFrames(from: fullFrames)
        }
        let named = run?.trimmingCharacters(in: .whitespaces) ?? ""
        accumulationRun = named.isEmpty ? site.clipID : named
        accumulator = FrameAccumulator(frameCount: accumulateFrameCount)
        accumulating = AlignmentAccumulationProgress(
            frames: 0, of: accumulator?.frameCount ?? 0, points: 0)
    }

    /// Stops collecting, or stops the upload that follows; nothing is kept.
    func cancelAccumulating() {
        stopAccumulating(refusal: nil)
        accumulationTask?.cancel()
    }

    /// A run's VRLOG was loaded for replay: whether it was recorded with full
    /// frames (its frame_mode), which is what lets Accumulate use it, and the
    /// name its background is uploaded under. Called after captureChanged,
    /// which forgets the previous run's.
    func noteLoadedRun(fullFrames: Bool, captureRef: String? = nil) {
        loadedRunHoldsFullFrames = fullFrames
        let named = captureRef?.trimmingCharacters(in: .whitespaces) ?? ""
        loadedRunCaptureRef = named.isEmpty ? nil : named
        if frameSource == .vrlog {
            let refusal = sourceRefusal
            if refusal != streamRefusal { streamRefusal = refusal }
        }
    }

    /// The pane was closed: a collection in progress stops (and full frames
    /// with it); an upload already under way is let finish.
    func paneClosed() { if accumulator != nil { stopAccumulating(refusal: nil) } }

    /// The task thinning and uploading the frames, for tests to await.
    var pendingAccumulation: Task<Void, Never>? { accumulationTask }

    /// The requests to velocity.report's full-frames switch, for tests to await.
    var pendingFullFrames: Task<Void, Never>? { fullFramesOff ?? fullFramesOn }

    private func stopAccumulating(refusal: String?) {
        accumulator = nil
        accumulating = nil
        if let refusal { accumulationRefusal = refusal }
        releaseFullFrames()
    }

    /// Asks for full frames, after any earlier request to switch them off.
    private func requestFullFrames(from fullFrames: ReplayFullFramesSwitch) {
        accumulationGeneration += 1
        let generation = accumulationGeneration
        let previous = fullFramesOff
        fullFramesRequested = true
        fullFramesOn = Task { [weak self] in
            await previous?.value
            do { try await fullFrames.setFullFrames(true) } catch {
                self?.fullFramesFailed(error, generation: generation)
            }
        }
    }

    /// The switch would not turn on: the accumulation cannot go on.
    private func fullFramesFailed(_ error: Error, generation: Int) {
        guard generation == accumulationGeneration, accumulator != nil else { return }
        if let refused = error as? ReplayFullFramesError {
            // A refusal, or a server without the switch: nothing was turned
            // on, so there is nothing to turn off.
            if case .failed = refused {} else { fullFramesRequested = false }
            stopAccumulating(refusal: refused.errorDescription)
        } else {
            stopAccumulating(
                refusal: ReplayFullFramesError.failed(error.localizedDescription).errorDescription)
        }
    }

    /// Switches full frames off again, once the request that turned them on
    /// has been answered. A failure is only logged: the server ends full
    /// frames with the replay in any case.
    private func releaseFullFrames() {
        guard fullFramesRequested, let fullFrames else { return }
        fullFramesRequested = false
        let enabling = fullFramesOn
        fullFramesOn = nil
        fullFramesOff = Task {
            await enabling?.value
            do { try await fullFrames.setFullFrames(false) } catch {
                logger.info(
                    "Full frames were not switched off: \(error.localizedDescription); the replay's end switches them off"
                )
            }
        }
    }

    private func finishAccumulating() {
        guard let collected = accumulator, let client, let site else {
            stopAccumulating(refusal: nil)
            return
        }
        stopAccumulating(refusal: nil)
        guard collected.pointsKept > 0 else {
            accumulationRefusal =
                "\(collected.framesTaken) frames held no background or ground points to accumulate."
            return
        }
        let captureRef = collected.captureRef(run: accumulationRun)
        accumulationTask = Task { [weak self] in
            await self?.uploadAccumulated(
                collected, captureRef: captureRef, client: client, site: site)
        }
    }

    /// Thins the frames off the main actor and uploads them. Written, like
    /// `register()`, without a `defer` or an early return inside `do`.
    private func uploadAccumulated(
        _ collected: FrameAccumulator, captureRef: String, client: AlignmentServiceClient,
        site: AlignmentSite
    ) async {
        busy = "Thinning \(collected.pointsKept.formatted()) points"
        let snapshot = await Task.detached { FrameAccumulation.thinned(collected) }.value
        var uploaded: Gerardus_Align_V1alpha1_Scan?
        if !Task.isCancelled, self.site == site {
            busy = "Uploading the accumulated frames"
            do {
                let header = ScanPacking.accumulatedHeader(
                    l16Token: site.l16Token, sensorID: sensorID, captureRef: captureRef,
                    snapshot: snapshot,
                    capturedAtNanos: capturedAtNanos(observed: snapshot.timestampNanos),
                    transient: collected.transient)
                uploaded = try await client.uploadScan(
                    header: header, blocks: ScanPacking.blocks(from: snapshot)
                ).scan
            } catch {
                errorMessage =
                    Task.isCancelled
                    ? "Accumulation cancelled." : GerardusClientError.describe(error)
            }
        }
        busy = nil
        // A cancelled task was already let go, and may have a successor.
        if !Task.isCancelled { accumulationTask = nil }
        // Cancelled, or the site changed while this ran: the scan is not
        // this capture's.
        if let uploaded, !Task.isCancelled, self.site == site {
            useAccumulated(
                uploaded, snapshot: snapshot,
                status: AlignmentAccumulatedScan(
                    scanID: uploaded.scanID, captureRef: captureRef, frames: collected.framesTaken,
                    pointsCollected: collected.pointsKept, points: snapshot.pointCount))
        }
    }

    /// Makes the uploaded accumulated scan the one Register and scoring use.
    /// The pose drawn now is scored again against it.
    private func useAccumulated(
        _ uploaded: Gerardus_Align_V1alpha1_Scan, snapshot: BackgroundSnapshot,
        status: AlignmentAccumulatedScan
    ) {
        accumulatedScan = uploaded
        accumulated = status
        scan = uploaded
        uploadedSnapshot = snapshot
        uploadedFrom = nil
        evaluation = nil
        sink?.setResidualOverlay(nil, palette: ResidualStyle.palette)
        scheduleRestyle()
        scheduleEvaluation()
    }

    /// Goes back to registering the background.
    func useBackground() {
        guard accumulatedScan != nil else { return }
        dropAccumulation()
        scan = nil
        uploadedSnapshot = nil
        uploadedFrom = nil
        evaluation = nil
        sink?.setResidualOverlay(nil, palette: ResidualStyle.palette)
        scheduleRestyle()
        scheduleEvaluation()
    }

    private func dropAccumulation() {
        cancelAccumulating()
        accumulationTask = nil
        accumulatedScan = nil
        accumulated = nil
        accumulationRefusal = nil
    }

    private func posePayload(_ pose: SitePose) -> Gerardus_Align_V1alpha1_SensorPose {
        let source: Gerardus_Align_V1alpha1_PoseSource = (delta?.isZero ?? true) ? .hint : .manual
        return pose.proto(
            siteFrameID: hintFrameIDs.site, sensorFrameID: hintFrameIDs.sensor, source: source)
    }

    // MARK: - Review

    /// Why Accept and Reject are off now, one line each for the pane; empty
    /// when they are on. Gerardus refuses any decision without a reviewer,
    /// so a blank one blocks Reject too.
    var reviewBlockers: [String] {
        var reasons = [String]()
        if registration == nil { reasons.append(Self.noRegistrationReason) }
        if pose == nil { reasons.append(Self.noPoseReason) }
        if let busy { reasons.append(Self.busyReason(busy)) }
        if reviewerName.isEmpty { reasons.append(Self.noReviewerReason) }
        if reviewNeedsReload {
            reasons.append(
                "Conflict: read the current records before deciding again. Your pose and note are kept."
            )
        }
        return reasons
    }

    var canReview: Bool { reviewBlockers.isEmpty }

    /// Accept can create the required registration without replacing the
    /// operator's pose. Reject still requires an existing registration.
    var acceptBlockers: [String] {
        if registration != nil { return reviewBlockers }
        var reasons = reviewBlockers.filter { $0 != Self.noRegistrationReason }
        if !hasScanToRegister {
            reasons.append("No settled background or accumulated scan to accept.")
        }
        return reasons
    }

    var canAccept: Bool { acceptBlockers.isEmpty }

    /// Whether the pane shows its Reviewer field: whenever Accept or Accept
    /// refinement is shown. There is one field for both, so a blank name
    /// is never out of sight while it blocks either.
    var showsReviewerField: Bool { pose != nil || (refinementColumnOpen && refinement != nil) }

    /// The reviewer as sent: Gerardus trims it and refuses it blank.
    private var reviewerName: String { reviewer.trimmingCharacters(in: .whitespaces) }

    static let noRegistrationReason = "No registration to decide on: press Register first."
    static let noPoseReason = "No pose drawn: place the sensor from its hint first."
    static let noReviewerReason =
        "No reviewer name: type yours in Reviewer at the top of the alignment pane; Gerardus records who decided."

    /// "Busy re-streaming the site model: wait for it to finish."
    static func busyReason(_ busy: String) -> String {
        "Busy " + busy.prefix(1).lowercased() + busy.dropFirst() + ": wait for it to finish."
    }

    /// Records the pose drawn now as accepted: the hypothesis it is
    /// (REGISTERED when the run passed its gates, else MANUALLY_PLACED), or
    /// the operator's own pose (MANUALLY_PLACED).
    func accept() async {
        guard let pose, canAccept else { return }
        let source = poseSource
        // ReviewRegistration needs a persisted run, even for an operator's
        // pose. Make it without drawing any of its hypotheses. A subsequent
        // retry reuses this run and the unchanged manual decision.
        if registration == nil { await register(drawBest: false) }
        var decision = Gerardus_Align_V1alpha1_ReviewRegistrationRequest()
        if case .hypothesis = acceptance, let id = selectedHypothesisID {
            decision.acceptHypothesisID = id
        } else {
            decision.acceptPose = pose.proto(
                siteFrameID: hintFrameIDs.site, sensorFrameID: hintFrameIDs.sensor,
                source: source == .icp ? .icp : .manual)
        }
        await review(decision)
        // TODO(Gerardus MVP step 2): send `lastRecord` to velocity.report with
        // PUT /api/lidar/sites/{l16}/pose (record as protojson) once that
        // endpoint exists, so the accepted pose reaches velocity.report's own
        // site record. Until then the record lives only in Gerardus's log,
        // and ListRegistrationRecords is how either side reconciles.
    }

    func reject() async {
        let reason = rejectReason.trimmingCharacters(in: .whitespaces)
        guard !reason.isEmpty else {
            errorMessage = "Say why the registration is rejected."
            return
        }
        var decision = Gerardus_Align_V1alpha1_ReviewRegistrationRequest()
        decision.rejectReason = reason
        await review(decision)
    }

    private func review(_ base: Gerardus_Align_V1alpha1_ReviewRegistrationRequest) async {
        guard let client, let registration, canReview else { return }
        var request = base
        request.registrationID = registration.registrationID
        request.reviewer = reviewerName
        request.note = note
        let retrying = pendingReview != nil
        if let pendingReview {
            request.expectedRecordID = pendingReview.expectedRecordID
            request.requestID = pendingReview.requestID
            guard request == pendingReview else {
                errorMessage =
                    "The previous decision has an unknown outcome. Retry the unchanged decision or read current records before making a new one."
                return
            }
        } else {
            request.expectedRecordID = recordHeads[registration.scanID] ?? ""
            request.requestID = UUID().uuidString
        }
        pendingReview = request
        errorMessage = nil
        busy = "Recording the decision"
        defer { busy = nil }
        var recorded = false
        do {
            let record = try await client.reviewRegistration(request)
            lastRecord = record
            recordHeads[record.scanID] = record.recordID
            pendingReview = nil
            noteDecided(record)
            rejectReason = ""
            recorded = true
            await loadRecords()
            if record.hasPose { selectedRecordID = record.recordID }
        } catch {
            errorMessage = GerardusClientError.describe(error)
            if let rpc = error as? RPCError, rpc.code == .aborted {
                reviewNeedsReload = true
                conflictingRecords =
                    (try? await client.listRegistrationRecords(l16Token: site?.l16Token ?? ""))
                    ?? []
            } else if !retrying && GerardusClientError.isDefiniteRejection(error) {
                pendingReview = nil
            }
        }
        if recorded { await refreshSiteModel(client: client) }
    }

    // MARK: - Refinement

    /// RefineSite needs two accepted scans; with fewer the button stays off
    /// rather than asking for a FAILED_PRECONDITION.
    var canRefine: Bool { refineBlocker == nil }

    /// Why Refine site is off now, for the pane; nil when it is on.
    var refineBlocker: String? {
        if site == nil || preparedSite == nil { return "Place the sensor from its hint first." }
        let held = siteModelScanIDs?.count ?? 0
        if held < 2 { return "Refining needs two accepted scans; the site model holds \(held)." }
        let chosen = refineScanIDs.count
        if chosen < 2 {
            return
                "Refining needs two scans; \(chosen) of \(held) \(chosen == 1 ? "is" : "are") ticked."
        }
        if let busy { return Self.busyReason(busy) }
        return nil
    }

    /// The site-model scans the next Refine re-solves: all but those
    /// unticked, in the model's order.
    var refineScanIDs: [String] { (siteModelScanIDs ?? []).filter { !refineExcluded.contains($0) } }

    /// Whether the next Refine re-solves this scan.
    func isIncludedInRefine(_ scanID: String) -> Bool { !refineExcluded.contains(scanID) }

    /// Ticks or unticks a scan for the next Refine. The refinement shown is
    /// untouched: it is accepted whole or not at all.
    func setIncludedInRefine(_ scanID: String, _ included: Bool) {
        if included { refineExcluded.remove(scanID) } else { refineExcluded.insert(scanID) }
    }

    /// Re-solves every accepted pose at the site together (RefineSite), then
    /// reads the run written for each scan, which carries its gates.
    func refine() async {
        guard let client, let site, canRefine else { return }
        refinementColumnOpen = true
        errorMessage = nil
        refinement = nil
        refinedRuns = [:]
        refinedAccepted = []
        pendingRefinement = nil
        refinementHeads = recordHeads
        refinementOverridden = nil
        staleRefinedScanIDs = []
        selectedRefinedScanID = nil
        isRefining = true
        scheduleRestyle()
        busy = "Refining the site"
        var request = Gerardus_Align_V1alpha1_RefineSiteRequest()
        request.l16Token = site.l16Token
        // Empty asks for every accepted scan; a list only when some are left out.
        let chosen = refineScanIDs
        if chosen.count < siteModelScanIDs?.count ?? 0 { request.scanIds = chosen }
        request.nEff = refineNEff
        var result: Gerardus_Align_V1alpha1_RefineSiteResult?
        do {
            result = try await client.refineSite(request) { [weak self] progress in
                await self?.show(progress)
            }
        } catch { errorMessage = GerardusClientError.describe(error) }
        var runs: [String: Gerardus_Align_V1alpha1_Registration] = [:]
        for refined in result?.scans ?? [] {
            // Without the run the pane shows no gate badges; nothing else is lost.
            if let run = try? await client.getRegistration(id: refined.registrationID) {
                runs[refined.scanID] = run
            }
        }
        busy = nil
        progress = nil
        isRefining = false
        refinement = result
        refinedRuns = runs
        if let heads = runs.values.first(where: { !$0.refinementHeads.isEmpty })?.refinementHeads {
            // These are the revisions the solve actually read, carried by
            // its persisted runs. Never substitute heads read after the solve.
            refinementHeads = heads
        }
        scheduleRestyle()
    }

    /// Shows the refinement column, with the refinement if there is one.
    func openRefinementColumn() {
        guard !refinementColumnOpen else { return }
        refinementColumnOpen = true
        scheduleRestyle()
    }

    /// Hides the column; the refinement, its runs and the scan selected in
    /// it are kept, and the scene drops their colours until it reopens.
    func closeRefinementColumn() {
        guard refinementColumnOpen else { return }
        refinementColumnOpen = false
        scheduleRestyle()
    }

    /// True when the refined pose for this scan passed every gate, so
    /// accepting it records REGISTERED.
    func refinedPassed(_ refined: Gerardus_Align_V1alpha1_RefinedScan) -> Bool {
        RefinementSummary.passed(refinedRuns[refined.scanID])
    }

    /// True while the column shows a refinement: the scene colours it then.
    var refinementShown: Bool { refinementColumnOpen && refinement != nil }

    /// The summary's rows: each refined scan, largest move first, then the
    /// site model's scans the refinement left out.
    var refinementRows: [RefinementSummaryRow] {
        guard let refinement else { return [] }
        return RefinementSummary.rows(
            refinement: refinement, runs: refinedRuns, captureRefs: scanCaptureRefs,
            siteModelScanIDs: siteModelScanIDs ?? [], stale: staleRefinedScanIDs)
    }

    /// What the scene draws for the refinement shown; nothing otherwise.
    var refinementEmphasis: ReferenceEmphasis {
        guard refinementShown else { return ReferenceEmphasis() }
        var tints = [String: SIMD4<Float>]()
        for row in refinementRows { tints[row.scanID] = row.state.colour }
        return ReferenceEmphasis(tints: tints, selectedScanID: selectedRefinedScanID)
    }

    /// Where a scan's sensor sits in the site frame: its refined pose when
    /// the refinement moved it, else its accepted one.
    func sensorPosition(of scanID: String) -> SIMD3<Double>? {
        if let refined = refinement?.scans.first(where: { $0.scanID == scanID }), refined.hasAfter {
            return SitePose(refined.after).translation
        }
        return siteModelPositions[scanID]
    }

    /// Picks a scan in the summary: frames it in the scene and draws it
    /// bold. Picking the selected scan again clears the selection.
    func selectRefinedScan(_ scanID: String) {
        if selectedRefinedScanID == scanID {
            clearRefinedScanSelection()
            return
        }
        selectedRefinedScanID = scanID
        if let position = sensorPosition(of: scanID) { sink?.focusCamera(onSitePoint: position) }
        scheduleRestyle()
    }

    /// Clears the summary's selection (a second click, or Escape). True when
    /// there was one.
    @discardableResult func clearRefinedScanSelection() -> Bool {
        guard selectedRefinedScanID != nil else { return false }
        selectedRefinedScanID = nil
        scheduleRestyle()
        return true
    }

    /// Why Accept refinement is off now, one line each for the pane; empty
    /// when it is on.
    var acceptRefinementBlockers: [String] {
        var reasons = [String]()
        if let refinement {
            if refinement.scans.isEmpty {
                reasons.append("The refinement moved no scan: there is nothing to record.")
            } else if !refinedAccepted.isEmpty {
                reasons.append(
                    "Recorded \(refinedAccepted.count) of \(refinement.scans.count) refined poses: refine again to record another refinement."
                )
            } else if let scanID = refinementOverridden {
                reasons.append(
                    "Scan \(String(scanID.suffix(10))) was accepted again since this refinement ran, so Gerardus would refuse its refined pose: refine again."
                )
            }
        } else if !isRefining {
            reasons.append("No refinement yet: press Refine site.")
        }
        if let busy { reasons.append(Self.busyReason(busy)) }
        if reviewerName.isEmpty { reasons.append(Self.noReviewerReason) }
        return reasons
    }

    var canAcceptRefinement: Bool { acceptRefinementBlockers.isEmpty }

    /// A decision recorded on its own, as Accept or Reject records one: if it
    /// is for one of the refinement's scans, that scan's refined pose was
    /// solved from a pose no longer the accepted one, and Gerardus refuses
    /// it (FAILED_PRECONDITION). Said before Accept refinement records the
    /// scans ahead of it and stops part way.
    private func noteDecided(_ record: Gerardus_Align_V1alpha1_RegistrationRecord) {
        guard record.state != .rejected, !record.scanID.isEmpty,
            refinement?.scans.contains(where: { $0.scanID == record.scanID }) == true
        else { return }
        refinementOverridden = record.scanID
        staleRefinedScanIDs.insert(record.scanID)
        scheduleRestyle()
    }

    /// Accepts every j1 in one atomic RPC. A refusal leaves the entire
    /// refinement unaccepted; a lost response retries the identical batch.
    ///
    /// Busy until the records and the moved site model are read back, so
    /// nothing else starts in between.
    func acceptRefinement() async {
        guard let client, let refinement, canAcceptRefinement else { return }
        errorMessage = nil
        busy = "Recording the refined poses"
        let scans = refinement.scans
        var batch = Gerardus_Align_V1alpha1_ReviewRegistrationsRequest()
        batch.reviews = scans.map { refined in
            var request = Gerardus_Align_V1alpha1_ReviewRegistrationRequest()
            request.registrationID = refined.registrationID
            request.acceptHypothesisID = Self.refinedHypothesisID
            request.reviewer = reviewerName
            request.note = note
            request.expectedRecordID = refinementHeads[refined.scanID] ?? ""
            return request
        }
        let retrying = pendingRefinement != nil
        if let pendingRefinement {
            batch.requestID = pendingRefinement.requestID
            guard batch == pendingRefinement else {
                errorMessage =
                    "Retry the unchanged refinement decision or refine again before changing it."
                busy = nil
                return
            }
        } else {
            batch.requestID = UUID().uuidString
        }
        pendingRefinement = batch
        do {
            let accepted = try await client.reviewRegistrations(batch)
            lastRecord = accepted.last
            refinedAccepted = Set(scans.map(\.scanID))
            pendingRefinement = nil
        } catch {
            if !retrying && GerardusClientError.isDefiniteRejection(error) {
                pendingRefinement = nil
            }
            errorMessage =
                "\(GerardusClientError.describe(error)) (no partial acceptance; retry the unchanged batch after a connection failure, or refine again after a conflict)"
        }
        if !refinedAccepted.isEmpty {
            await loadRecords()
            await refreshSiteModel(client: client)
        }
        busy = nil
    }

    /// The site's records, newest first.
    func loadRecords() async {
        guard let client, let site else { return }
        do {
            let listed = try await client.listRegistrationRecords(l16Token: site.l16Token)
            // Append order chooses heads even if the wall clock moved back.
            // Timestamp sorting below is for display only.
            recordHeads = Dictionary(
                listed.map { ($0.scanID, $0.recordID) }, uniquingKeysWith: { first, _ in first })
            records = listed.sorted { a, b in
                (a.reviewedAt.seconds, a.reviewedAt.nanos) > (
                    b.reviewedAt.seconds, b.reviewedAt.nanos
                )
            }
            if let id = selectedRecordID, !records.contains(where: { $0.recordID == id }) {
                selectedRecordID = nil
            }
            pendingReview = nil
            reviewNeedsReload = false
            conflictingRecords = []
        } catch { errorMessage = GerardusClientError.describe(error) }
    }

    private func clearPlacement() {
        recordHeads = [:]
        pendingReview = nil
        pendingRefinement = nil
        refinementHeads = [:]
        reviewNeedsReload = false
        conflictingRecords = []
        preparedSite = nil
        reference = nil
        geometry = nil
        referenceCentre = nil
        siteModelID = ""
        styledOwnScanID = nil
        styleGeneration += 1
        styling?.cancel()
        registeredLayersNote = nil
        streetLayersNote = nil
        featureLayersNote = nil
        siteModelScanIDs = nil
        dropAccumulation()
        hintPose = nil
        hintSummary = nil
        pose = nil
        scan = nil
        uploadedSnapshot = nil
        registration = nil
        hypotheses = []
        selectedHypothesisID = nil
        evaluation = nil
        registering?.cancel()
        evaluating?.cancel()
        dropCallOnTheWire()
        records = []
        selectedRecordID = nil
        lastRecord = nil
        refinement = nil
        refinedRuns = [:]
        refinedAccepted = []
        refinementOverridden = nil
        staleRefinedScanIDs = []
        refineExcluded = []
        selectedRefinedScanID = nil
        scanCaptureRefs = [:]
        siteModelPositions = [:]
        refinementColumnOpen = false
        errorMessage = nil
        sink?.setReferenceOverlay(nil)
        sink?.setResidualOverlay(nil, palette: ResidualStyle.palette)
    }
}

extension MetalRenderer: ReferenceOverlaySink {}
