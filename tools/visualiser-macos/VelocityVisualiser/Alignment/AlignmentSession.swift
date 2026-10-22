// AlignmentSession.swift
// State behind the alignment pane: which site the open capture belongs to,
// the Gerardus reference drawn over it, the pose and the operator's nudges,
// and the decisions recorded in Gerardus's alignment log.
//
// The operator's path, one RPC per step (Gerardus docs/INTERFACE-VISUALISER.md):
//
//   Place from hint   PrepareSite → PoseFromHint → StreamReference (150 m)
//   Nudge             nothing on the wire: only the model matrix changes
//   Register          UploadScan (once per background) → Register
//   Accept / Reject   ReviewRegistration → ListRegistrationRecords
//
// Until Gerardus builds ICP, Register comes back FAILED carrying the pose it
// was given, and Accept records the operator's pose as MANUALLY_PLACED.

import Combine
import Foundation
import SwiftProtobuf
import simd

private let logger = DevLogger(category: "Alignment")

/// Where the reference overlay goes. MetalRenderer is the real one.
protocol ReferenceOverlaySink: AnyObject {
    func setReferenceOverlay(_ vertices: ReferenceOverlayVertices?)
    func setReferenceModelMatrix(_ matrix: simd_float4x4)
    func setReferenceVisibility(aerial: Bool, osm: Bool, intersections: Bool)
    func setResidualOverlay(_ vertices: [Float]?, palette: [SIMD4<Float>])
}

/// What the pane shows about the settled background it would upload.
struct AlignmentBackgroundStatus: Equatable {
    var points: Int
    var sequence: UInt64
    /// False while the grid is still settling: such a snapshot is not uploaded.
    var settled: Bool
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
    var intersections: Int
    /// "© OpenStreetMap contributors", shown whenever OSM is visible.
    var osmAttribution: String
    var aerialAttribution: String
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
    @Published private(set) var progress: Gerardus_Align_V1alpha1_Progress?
    @Published private(set) var reference: ReferenceSummary?
    @Published var showAerial = true { didSet { pushVisibility() } }
    @Published var showOSM = true { didSet { pushVisibility() } }
    @Published var showIntersections = true { didSet { pushVisibility() } }

    @Published private(set) var hintPose: SitePose?
    @Published private(set) var hintSummary: Gerardus_Align_V1alpha1_PoseSummary?
    /// The hint pose with the operator's nudges applied. Accept records it.
    @Published private(set) var pose: SitePose?
    @Published var translationStepM: Double = 0.5
    @Published var yawStepDeg: Double = 1

    @Published private(set) var background: AlignmentBackgroundStatus?
    @Published private(set) var scan: Gerardus_Align_V1alpha1_Scan?
    @Published private(set) var registration: Gerardus_Align_V1alpha1_Registration?
    /// Hypotheses as the run in progress reports them, then the finished
    /// registration's own list, ranked best first.
    @Published private(set) var hypotheses: [Gerardus_Align_V1alpha1_Hypothesis] = []
    @Published private(set) var isRegistering = false
    /// The hypothesis drawn now, for as long as the pose is exactly its pose.
    @Published private(set) var selectedHypothesisID: String?
    /// EvaluatePose's verdict on the pose drawn now, once it has rested.
    @Published private(set) var evaluation: PoseEvaluation?
    @Published private(set) var isEvaluating = false
    /// Colour the uploaded scan by residual whenever an evaluation has them.
    @Published var showResiduals = true { didSet { pushResiduals() } }
    @Published private(set) var records: [Gerardus_Align_V1alpha1_RegistrationRecord] = []
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
    /// How the pose drawn now was made, for what Accept records.
    private var poseSource: Gerardus_Align_V1alpha1_PoseSource = .hint
    private var registering: Task<Void, Never>?
    private var evaluating: Task<Void, Never>?
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

    /// `lookup` nil skips velocity.report entirely, as tests do; `defaults`
    /// nil keeps typed headings and the server address out of the app's
    /// preferences.
    init(
        makeClient: @escaping (GerardusAddress) -> AlignmentServiceClient = {
            GerardusClient(address: $0)
        }, lookup: AlignmentSiteLookupClient? = AlignmentSiteLookupClient(),
        defaults: UserDefaults? = .standard
    ) {
        self.makeClient = makeClient
        self.lookup = lookup
        self.defaults = defaults
        self.serviceAddress = GerardusAddress.saved(in: defaults)
        self.reviewer =
            defaults?.string(forKey: Self.reviewerKey) ?? defaults?.string(
                forKey: "annotation.operatorName") ?? NSFullUserName()
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
            availability = .available(serverVersion: info.serverVersion)
            probeFailure = nil
            isChecking = false
        } else {
            client = nil
            // Set only on a change, so a repeated miss redraws nothing.
            if availability != .unavailable { availability = .unavailable }
            if probeFailure != failure { probeFailure = failure }
            isChecking = false
        }
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
                files[site.replayCaseID] =
                    (try? await lookup.captureFiles(replayCaseID: site.replayCaseID)) ?? []
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
            scan = nil
            registration = nil
            hypotheses = []
            selectedHypothesisID = nil
            evaluation = nil
            sink?.setResidualOverlay(nil, palette: ResidualStyle.palette)

            busy = "Streaming the reference"
            let geometry = try await client.streamReference(
                referenceRequest(l16Token: site.l16Token, centre: start.translation))
            let vertices = await Task.detached { ReferenceStyle.vertices(for: geometry) }.value
            reference = ReferenceSummary(
                aerialPoints: geometry.aerialCount, osmFeatures: geometry.lines.count,
                intersections: geometry.intersections.count,
                osmAttribution: attribution(prepared, layer: .osm),
                aerialAttribution: attribution(prepared, layer: .aerialPoints))
            sink?.setReferenceOverlay(vertices)
            sink?.setReferenceModelMatrix(start.referenceModelMatrix)
            pushVisibility()
            logger.info(
                "Reference for \(site.l16Token): \(geometry.aerialCount) points, \(geometry.lines.count) lines, \(geometry.intersections.count) intersections"
            )

            busy = "Reading records"
            await loadRecords()
        } catch { errorMessage = GerardusClientError.describe(error) }
    }

    private func show(_ progress: Gerardus_Align_V1alpha1_Progress) { self.progress = progress }

    func referenceRequest(
        l16Token: String, centre: SIMD3<Double>
    ) -> Gerardus_Align_V1alpha1_StreamReferenceRequest {
        var request = Gerardus_Align_V1alpha1_StreamReferenceRequest()
        request.l16Token = l16Token
        let wanted: [Gerardus_Align_V1alpha1_ReferenceLayer] = [
            .aerialPoints, .osm, .intersections,
        ]
        request.layers = wanted.filter { availableLayers.isEmpty || availableLayers.contains($0) }
        request.centre.x = centre.x
        request.centre.y = centre.y
        request.centre.z = centre.z
        request.radiusM = Self.referenceRadiusM
        // 0 asks for the server's default spacing.
        request.voxelSizeM = 0
        return request
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

    /// Draws a new pose. Any residual colouring belongs to the old one, so it
    /// goes; the new pose is scored once it has rested for `evaluateDelay`.
    /// Moving the pose sends nothing until then.
    private func setPose(
        _ newPose: SitePose, source: Gerardus_Align_V1alpha1_PoseSource, hypothesis: String? = nil,
        evaluate: Bool = true
    ) {
        pose = newPose
        poseSource = source
        selectedHypothesisID = hypothesis
        poseGeneration += 1
        evaluation = nil
        sink?.setReferenceModelMatrix(newPose.referenceModelMatrix)
        sink?.setResidualOverlay(nil, palette: ResidualStyle.palette)
        if evaluate { scheduleEvaluation() }
    }

    private func pushVisibility() {
        sink?.setReferenceVisibility(
            aerial: showAerial, osm: showOSM, intersections: showIntersections)
    }

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
        latestBackground = nil
        background = nil
        scan = nil
        uploadedSnapshot = nil
        registration = nil
        hypotheses = []
        selectedHypothesisID = nil
        evaluation = nil
        sink?.setResidualOverlay(nil, palette: ResidualStyle.palette)
    }

    var canRegister: Bool {
        busy == nil && pose != nil && site != nil && background?.settled == true
            && (background?.points ?? 0) > 0
    }

    /// Uploads the settled background unless this one is already up, then
    /// registers it from the current pose.
    ///
    /// Written without a `defer` or an early return inside `do`: in that shape
    /// the test host aborted in swift_task_dealloc ("freed pointer was not the
    /// last allocation") on the Xcode 26 toolchain.
    func register() async {
        guard let client, let site, let pose, let snapshot = latestBackground, canRegister else {
            return
        }
        evaluating?.cancel()
        errorMessage = nil
        registration = nil
        hypotheses = []
        isRegistering = true
        var result: Gerardus_Align_V1alpha1_Registration?
        do {
            busy = "Uploading the background"
            let uploaded = try await uploadedScan(client: client, site: site, snapshot: snapshot)
            busy = "Registering"
            var request = Gerardus_Align_V1alpha1_RegisterRequest()
            request.scanID = uploaded.scanID
            // The server searches 30 m around this position at every
            // heading; it does not use the start's heading.
            request.initialPose = posePayload(pose)
            request.hint = hint(heading: hintHeadingDeg)
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
            if let best = result.hypotheses.first { showHypothesis(best) }
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
        guard let client, let site, let pose, let snapshot = latestBackground, canRegister else {
            return
        }
        isEvaluating = true
        var response: Gerardus_Align_V1alpha1_EvaluatePoseResponse?
        var failure: String?
        do {
            let uploaded = try await uploadedScan(client: client, site: site, snapshot: snapshot)
            var request = Gerardus_Align_V1alpha1_EvaluatePoseRequest()
            request.scanID = uploaded.scanID
            request.pose = posePayload(pose)
            request.refine = refine
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
            l16Token: site.l16Token, sensorID: sensorID, captureRef: site.replayCaseID,
            snapshot: snapshot)
        let uploaded = try await client.uploadScan(
            header: header, blocks: ScanPacking.blocks(from: snapshot))
        scan = uploaded.scan
        uploadedFrom = (site.l16Token, snapshot.sequenceNumber)
        uploadedSnapshot = snapshot
        return uploaded.scan
    }

    private func posePayload(_ pose: SitePose) -> Gerardus_Align_V1alpha1_SensorPose {
        let source: Gerardus_Align_V1alpha1_PoseSource = (delta?.isZero ?? true) ? .hint : .manual
        return pose.proto(
            siteFrameID: hintFrameIDs.site, sensorFrameID: hintFrameIDs.sensor, source: source)
    }

    // MARK: - Review

    var canReview: Bool { busy == nil && registration != nil && pose != nil }

    /// Records the pose drawn now as accepted: the hypothesis it is
    /// (REGISTERED when the run passed its gates, else MANUALLY_PLACED), or
    /// the operator's own pose (MANUALLY_PLACED).
    func accept() async {
        guard let pose else { return }
        var decision = Gerardus_Align_V1alpha1_ReviewRegistrationRequest()
        if case .hypothesis = acceptance, let id = selectedHypothesisID {
            decision.acceptHypothesisID = id
        } else {
            decision.acceptPose = pose.proto(
                siteFrameID: hintFrameIDs.site, sensorFrameID: hintFrameIDs.sensor,
                source: poseSource == .icp ? .icp : .manual)
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
        request.reviewer = reviewer.trimmingCharacters(in: .whitespaces)
        request.note = note
        errorMessage = nil
        busy = "Recording the decision"
        defer { busy = nil }
        do {
            lastRecord = try await client.reviewRegistration(request)
            rejectReason = ""
            await loadRecords()
        } catch { errorMessage = GerardusClientError.describe(error) }
    }

    /// The site's records, newest first.
    func loadRecords() async {
        guard let client, let site else { return }
        do {
            let listed = try await client.listRegistrationRecords(l16Token: site.l16Token)
            records = listed.sorted { a, b in
                (a.reviewedAt.seconds, a.reviewedAt.nanos) > (
                    b.reviewedAt.seconds, b.reviewedAt.nanos
                )
            }
        } catch { errorMessage = GerardusClientError.describe(error) }
    }

    private func clearPlacement() {
        preparedSite = nil
        reference = nil
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
        records = []
        lastRecord = nil
        errorMessage = nil
        sink?.setReferenceOverlay(nil)
        sink?.setResidualOverlay(nil, palette: ResidualStyle.palette)
    }
}

extension MetalRenderer: ReferenceOverlaySink {}
