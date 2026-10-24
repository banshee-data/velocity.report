// AlignmentPane.swift
// The alignment pane: place the sensor from its eye-read hint against the
// Gerardus reference, nudge it, and record the operator's decision. Shown
// only while a Gerardus server answers (AlignmentSession.isAvailable).

import SwiftProtobuf
import SwiftUI

/// The toolbar's way into the pane. Draws nothing unless Gerardus answered.
@available(macOS 15.0, *) struct AlignmentToolbarButton: View {
    @ObservedObject var session: AlignmentSession
    @EnvironmentObject var appState: AppState

    var body: some View {
        if session.isAvailable {
            Divider().frame(height: 20)
            Button(action: { appState.toggleAlignmentPane() }) {
                Label("Align", systemImage: "scope")
            }.help("Place this capture's sensor against the Gerardus reference")
        }
    }
}

/// "© OpenStreetMap contributors" over the scene whenever OSM is drawn.
@available(macOS 15.0, *) struct AlignmentAttributionView: View {
    @ObservedObject var session: AlignmentSession

    var body: some View {
        if let attribution = session.osmAttributionVisible {
            VStack {
                Spacer()
                HStack {
                    Spacer()
                    Text(attribution).font(.caption2).padding(.horizontal, 6).padding(.vertical, 2)
                        .background(.black.opacity(0.55)).foregroundStyle(.white.opacity(0.9))
                        .clipShape(RoundedRectangle(cornerRadius: 4))
                }
            }.padding(8)
        }
    }
}

@available(macOS 15.0, *) struct AlignmentPane: View {
    @ObservedObject var session: AlignmentSession
    @ObservedObject var painter: StreetPainterSession
    @EnvironmentObject var appState: AppState

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Pinned above the scroll, so the one Reviewer field both Accepts
            // need cannot be scrolled out of sight while either is shown.
            VStack(alignment: .leading, spacing: 8) {
                header
                paintSwitch
                if session.showsReviewerField, !painter.isPainting { reviewerField }
            }.padding([.horizontal, .top]).padding(.bottom, 10)
            Divider()
            if painter.isPainting {
                StreetPainterPane(painter: painter, session: session)
            } else {
                scrollingSections
            }
        }.frame(maxHeight: .infinity).background(Color(nsColor: .controlBackgroundColor)).task(
            id: appState.currentRunID
        ) { await session.refreshSites(runID: appState.currentRunID) }
    }

    private var scrollingSections: some View {
        ScrollView(.vertical) {
            VStack(alignment: .leading, spacing: 14) {
                siteSection
                statusSection
                if session.reference != nil { layersSection }
                if session.pose != nil {
                    Divider()
                    poseSection
                    nudgeSection
                    Divider()
                    registrationSection
                }
                if session.registration != nil { reviewSection }
                if session.siteModelScanIDs != nil {
                    Divider()
                    siteModelSection
                }
                if session.site != nil {
                    Divider()
                    recordsSection
                }
                Spacer()
            }.padding()
        }
    }

    // MARK: Header

    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            Text("Alignment").font(.headline)
            if case .available(let version) = session.availability {
                Text("Gerardus \(version) at \(session.serviceAddress.description)").font(.caption)
                    .foregroundStyle(.secondary).help("Change the server in Settings (⌘,)")
            }
            Spacer()
            Button(action: { appState.showAlignmentPane = false }) { Image(systemName: "xmark") }
                .buttonStyle(.borderless).help("Close the alignment pane")
        }
    }

    /// Paint mode's switch: the plan view and the street painter's tools in
    /// place of the alignment sections, or back.
    private var paintSwitch: some View {
        HStack {
            Toggle(isOn: Binding(get: { painter.isPainting }, set: painter.setPainting)) {
                Label("Paint streets", systemImage: "paintbrush.pointed")
            }.toggleStyle(.button).disabled(!painter.isPainting && !painter.canPaint).help(
                painter.paintBlocker
                    ?? "Look straight down on the site and mark its surfaces, lanes, spaces and driveways"
            )
            if !painter.isPainting, let blocker = painter.paintBlocker,
                session.reference == nil || painter.unsupported != nil
            {
                Text(blocker).font(.caption2).foregroundStyle(.secondary).lineLimit(2)
            } else if !painter.isPainting, !painter.edit.marks.isEmpty {
                Text("\(painter.edit.marks.count) marks").font(.caption2).foregroundStyle(
                    .secondary)
            }
        }
    }

    /// The reviewer every decision is recorded under, Accept, Reject and
    /// Accept refinement alike.
    private var reviewerField: some View {
        let blank = session.reviewer.trimmingCharacters(in: .whitespaces).isEmpty
        return HStack(alignment: .firstTextBaseline) {
            Text("Reviewer").font(.callout).foregroundStyle(blank ? Color.orange : .secondary)
            TextField("Your name", text: $session.reviewer).textFieldStyle(.roundedBorder)
        }.help("Gerardus records every decision under this name, and refuses one without it")
    }

    // MARK: Site

    private var siteSelection: Binding<String> {
        Binding(
            get: { session.site?.id ?? "" },
            set: { id in
                if let chosen = session.sites.first(where: { $0.id == id }) {
                    session.choose(chosen)
                }
            })
    }

    private var siteSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Picker("Site", selection: siteSelection) {
                    if session.site == nil { Text("Choose a site").tag("") }
                    ForEach(session.sites) { site in
                        Text("\(site.name) · \(site.l16Display)").tag(site.id)
                    }
                }
                Button(action: { Task { await session.refreshSites(runID: appState.currentRunID) } }
                ) { Image(systemName: "arrow.clockwise") }.buttonStyle(.borderless).help(
                    "Look up the open capture's site again")
            }
            Text(session.siteMatch.explanation).font(.caption).foregroundStyle(.secondary)
            if let site = session.site {
                Text(
                    "Eye-read \(site.latitude, specifier: "%.5f"), \(site.longitude, specifier: "%.5f") · sensor \(site.sensorID)"
                ).font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
                HStack {
                    Text("north_azimuth_deg")
                    TextField("°", text: $session.northAzimuthText).frame(width: 70)
                        .multilineTextAlignment(.trailing)
                    if let heading = session.hintHeadingDeg {
                        Text("hint heading \(heading, specifier: "%.0f")°").foregroundStyle(
                            .secondary)
                    }
                }.font(.callout)
                Text(
                    "As in site-index.json; leave empty if unknown. The hint sends 180° − this: the bearing of the sensor's +Y."
                ).font(.caption2).foregroundStyle(.secondary)
                Button("Place from hint") { Task { await session.placeFromHint() } }.disabled(
                    session.busy != nil
                ).help("Prepare the site, place the sensor from its hint, and draw the reference")
            }
        }
    }

    @ViewBuilder private var statusSection: some View {
        if let busy = session.busy {
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    ProgressView().controlSize(.small)
                    Text(busy).font(.callout)
                }
                if let progress = session.progress {
                    if progress.fraction >= 0 {
                        ProgressView(value: Double(min(progress.fraction, 1)))
                    }
                    Text("\(progress.stage): \(progress.message)").font(.caption).foregroundStyle(
                        .secondary)
                }
            }
        }
        if let error = session.errorMessage {
            Text(error).font(.caption).foregroundStyle(.red).textSelection(.enabled)
        }
    }

    private var layersSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            if let reference = session.reference {
                Toggle(
                    "3DEP points (\(reference.aerialPoints.formatted()))", isOn: $session.showAerial
                )
                Toggle("OSM ways (\(reference.osmFeatures))", isOn: $session.showOSM)
                Toggle("DataSF footprints (\(reference.footprints))", isOn: $session.showFootprints)
                if reference.boxesServed {
                    Toggle("DataSF boxes (\(reference.boxWalls) walls)", isOn: $session.showBoxes)
                        .help(
                            "Footprints extruded from the 3DEP ground to the roof measured in 3DEP, moved by the site's footprint shift. Party walls (\(reference.partyWalls)) are fainter."
                        )
                }
                Toggle(
                    "Intersections (\(reference.intersections))", isOn: $session.showIntersections)
                if reference.streetLayersServed {
                    Toggle(
                        "City streets (\(reference.cityStreets), \(reference.cityIntersections) intersections)",
                        isOn: $session.showCityStreets
                    ).help("DataSF street centrelines, and a cross where three or more meet")
                    Toggle("Kerbs and islands (\(reference.kerbs))", isOn: $session.showKerbs).help(
                        "DataSF kerbs and islands, and kerbs observed in registered scans")
                }
                if let note = session.streetLayersNote {
                    Text(note).font(.caption2).foregroundStyle(.secondary).fixedSize(
                        horizontal: false, vertical: true)
                }
                if session.showAerial, !reference.aerialAttribution.isEmpty {
                    Text(reference.aerialAttribution).font(.caption2).foregroundStyle(.secondary)
                }
                if (session.showFootprints && reference.footprints > 0)
                    || (session.showBoxes && reference.boxWalls > 0),
                    !reference.footprintAttribution.isEmpty
                {
                    Text(reference.footprintAttribution).font(.caption2).foregroundStyle(.secondary)
                }
                Divider()
                Text("Site model").font(.subheadline.weight(.semibold))
                Toggle(
                    "Registered scans (\(reference.registeredScans), \(reference.registeredPoints.formatted()) points)",
                    isOn: $session.showRegisteredScans)
                if reference.ownScanPoints > 0 {
                    Toggle(
                        "Include this capture's own scan (\(reference.ownScanPoints.formatted()) points)",
                        isOn: $session.showOwnScan
                    ).padding(.leading, 16).help(
                        "Draw this capture's registered copy over its live points, as a check of the accepted pose"
                    )
                }
                Toggle(
                    "Observed features (\(reference.observedFeatures))", isOn: $session.showObserved
                ).help("Sensors white, corrections amber, facade traces cyan, poles blue")
                Toggle("Wall planes (\(reference.wallPlanes))", isOn: $session.showWallPlanes).help(
                    "Measured against the box walls: walls cyan-teal, popouts orange, recesses violet"
                )
                if reference.wallPlanes > 0 {
                    Text(reference.wallPlaneRoleText).font(.caption2).foregroundStyle(.secondary)
                        .padding(.leading, 16)
                }
                Toggle(
                    "Ground mesh (\(reference.groundMeshCells.formatted()) cells)",
                    isOn: $session.showGroundMesh)
                if !reference.modelID.isEmpty {
                    Text("Model \(AlignmentFormat.shortID(reference.modelID))").font(.caption2)
                        .foregroundStyle(.secondary).textSelection(.enabled).help(reference.modelID)
                }
                if let note = session.registeredLayersNote {
                    Text(note).font(.caption2).foregroundStyle(.secondary).fixedSize(
                        horizontal: false, vertical: true)
                }
            }
        }.toggleStyle(.checkbox).font(.callout)
    }

    // MARK: Pose

    private var poseSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Pose from hint").font(.subheadline.weight(.semibold))
            if let summary = session.hintSummary {
                let sigma =
                    summary.sigmaHeadingDeg > 0
                    ? String(format: " ± %.0f°", summary.sigmaHeadingDeg) : ""
                AlignmentRow("Heading", String(format: "%.1f°", summary.headingDeg) + sigma)
                AlignmentRow(
                    "Pitch, roll", String(format: "%.2f°, %.2f°", summary.pitchDeg, summary.rollDeg)
                )
                AlignmentRow("Above ground", String(format: "%.2f m", summary.heightAboveGroundM))
                AlignmentRow(
                    "Position",
                    String(
                        format: "%.6f, %.6f", summary.position.latitudeDeg,
                        summary.position.longitudeDeg)
                        + (summary.sigmaHorizontalM > 0
                            ? String(format: " ± %.0f m", summary.sigmaHorizontalM) : ""))
                AlignmentRow(
                    "NAVD88 height",
                    summary.position.hasOrthometricHeightM
                        ? String(format: "%.2f m", summary.position.orthometricHeightM) : "—")
                AlignmentRow(
                    "Ellipsoidal",
                    summary.position.hasEllipsoidalHeightM
                        ? String(format: "%.2f m", summary.position.ellipsoidalHeightM)
                        : "not given (no GEOID18 yet)")
            }
            if let delta = session.delta, let pose = session.pose {
                Text("Nudged").font(.subheadline.weight(.semibold)).padding(.top, 4)
                AlignmentRow(
                    "East, north, up",
                    String(format: "%+.2f, %+.2f, %+.2f m", delta.east, delta.north, delta.up))
                AlignmentRow(
                    "Heading", String(format: "%.1f° (%+.1f°)", pose.headingDeg, delta.headingDeg))
            }
        }
    }

    private var nudgeSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Move the reference").font(.subheadline.weight(.semibold))
            Picker("Step", selection: $session.translationStepM) {
                Text("0.1 m").tag(0.1)
                Text("0.5 m").tag(0.5)
                Text("2 m").tag(2.0)
            }.pickerStyle(.segmented)
            Picker("Turn", selection: $session.yawStepDeg) {
                Text("0.2°").tag(0.2)
                Text("1°").tag(1.0)
                Text("5°").tag(5.0)
            }.pickerStyle(.segmented)
            HStack(alignment: .top, spacing: 16) {
                Grid(horizontalSpacing: 4, verticalSpacing: 4) {
                    GridRow {
                        nudgeButton("arrow.counterclockwise", "Turn anticlockwise") {
                            session.nudge(yawSteps: 1)
                        }
                        nudgeButton("arrow.up", "Along the sensor's +Y") { session.nudge(y: 1) }
                        nudgeButton("arrow.clockwise", "Turn clockwise") {
                            session.nudge(yawSteps: -1)
                        }
                    }
                    GridRow {
                        nudgeButton("arrow.left", "Along the sensor's −X") { session.nudge(x: -1) }
                        nudgeButton("arrow.uturn.backward", "Back to the hint pose") {
                            session.resetNudges()
                        }
                        nudgeButton("arrow.right", "Along the sensor's +X") { session.nudge(x: 1) }
                    }
                    GridRow {
                        Color.clear.gridCellUnsizedAxes([.horizontal, .vertical])
                        nudgeButton("arrow.down", "Along the sensor's −Y") { session.nudge(y: -1) }
                        Color.clear.gridCellUnsizedAxes([.horizontal, .vertical])
                    }
                }
                VStack(spacing: 4) {
                    nudgeButton("chevron.up", "Up") { session.nudge(z: 1) }
                    nudgeButton("chevron.down", "Down") { session.nudge(z: -1) }
                }
            }
            Text("Moves only the drawing: nothing is sent until you register or accept.").font(
                .caption2
            ).foregroundStyle(.secondary)
        }
    }

    private func nudgeButton(
        _ symbol: String, _ help: String, action: @escaping () -> Void
    ) -> some View {
        Button(action: action) { Image(systemName: symbol).frame(width: 22, height: 18) }.help(help)
            .disabled(session.busy != nil)
    }

    // MARK: Registration and review

    private var registrationSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Registration").font(.subheadline.weight(.semibold))
            if let background = session.background {
                Text(
                    "Background: \(background.points.formatted()) points, snapshot \(background.sequence)"
                        + (background.settled ? "" : ", still settling")
                ).font(.caption).foregroundStyle(background.settled ? .secondary : Color.orange)
            } else {
                Text("No background snapshot yet: play the capture until it settles.").font(
                    .caption
                ).foregroundStyle(.secondary)
            }
            accumulationView
            HStack {
                Button("Register") { session.startRegistration() }.disabled(!session.canRegister)
                    .help(
                        session.accumulated == nil
                            ? "Upload the settled background and search 30 m around the pose drawn now, at every heading, for where it fits the reference (10 to 25 s)"
                            : "Search 30 m around the pose drawn now, at every heading, for where the accumulated frames fit the reference (10 to 25 s)"
                    )
                VStack(alignment: .leading, spacing: 2) {
                    Toggle("Against site model", isOn: againstSiteModel).toggleStyle(.switch)
                        .controlSize(.mini).font(.caption).disabled(
                            session.againstSiteModelUnavailable != nil || session.isRegistering
                        ).help(
                            "Fit against 3DEP and the site's accepted scans. The gates are still measured against 3DEP alone."
                        )
                    Toggle("Against DataSF boxes", isOn: againstBoxes).toggleStyle(.switch)
                        .controlSize(.mini).font(.caption).disabled(
                            session.againstBoxesUnavailable != nil || session.isRegistering
                        ).help(
                            "Fit against 3DEP and the site's DataSF box walls, corrected by the measured wall planes. The gates are still measured against 3DEP alone."
                        )
                }
                if session.isRegistering {
                    Button("Cancel") { session.cancelRegistration() }.help(
                        "Stop the run; the server logs it as far as it got")
                }
            }
            ForEach(switchReasons, id: \.self) { reason in
                Text(reason).font(.caption2).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
            }
            if let registration = session.registration {
                AlignmentRow("Result", AlignmentFormat.state(registration.state))
                if !registration.failureReason.isEmpty {
                    Text(registration.failureReason).font(.caption).foregroundStyle(.secondary)
                        .textSelection(.enabled)
                }
                if !registration.gates.isEmpty { gatesView(registration.gates) }
                if let fit = session.modelFit,
                    let line = AlignmentFormat.modelFit(fit, against: session.registeredAgainst)
                {
                    Text(AlignmentFormat.modelFitCaption(session.registeredAgainst) + line).font(
                        .caption2.monospacedDigit()
                    ).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                }
                Text(registration.registrationID).font(.caption2.monospaced()).foregroundStyle(
                    .secondary
                ).textSelection(.enabled)
            }
            if !session.hypotheses.isEmpty { hypothesesView }
            fitView
        }
    }

    /// Shows the switch on only while it is sent.
    private var againstSiteModel: Binding<Bool> {
        Binding(get: { session.registersAgainstSiteModel }, set: { session.againstSiteModel = $0 })
    }

    private var againstBoxes: Binding<Bool> {
        Binding(get: { session.registersAgainstBoxes }, set: { session.againstBoxes = $0 })
    }

    /// Why either switch is disabled, site model first.
    private var switchReasons: [String] {
        [session.againstSiteModelUnavailable, session.againstBoxesUnavailable].compactMap { $0 }
    }

    @ViewBuilder private var accumulationView: some View {
        VStack(alignment: .leading, spacing: 4) {
            if let accumulated = session.accumulated {
                HStack(alignment: .firstTextBaseline) {
                    Text(
                        "Registering accumulated frames: \(accumulated.points.formatted()) points (\(accumulated.pointsCollected.formatted()) before thinning) from \(accumulated.frames) frames"
                    ).font(.caption).fixedSize(horizontal: false, vertical: true)
                    Spacer()
                    Button("Use background") { session.useBackground() }.controlSize(.small)
                        .disabled(session.busy != nil).help(
                            "Register the settled background again instead")
                }
                Text(accumulated.captureRef).font(.caption2.monospaced()).foregroundStyle(
                    .secondary
                ).textSelection(.enabled)
            }
            HStack {
                if let progress = session.accumulating {
                    ProgressView(value: Double(progress.frames), total: Double(max(progress.of, 1)))
                        .frame(width: 80)
                    Text(
                        "\(progress.frames)/\(progress.of) frames, \(progress.points.formatted()) points"
                    ).font(.caption.monospacedDigit())
                    Button("Cancel") { session.cancelAccumulating() }.controlSize(.small)
                } else {
                    Button("Accumulate frames") {
                        session.startAccumulating(run: appState.currentRunID)
                    }.disabled(!session.canAccumulate).help(
                        "Collect the background and ground points of the next frames a pcap replay plays (velocity.report streams full frames while they are collected) or a full-frames run's VRLOG plays, thin them to 0.05 m and upload them as an accumulated scan, which Register then uses. Foreground points are never collected."
                    )
                    Picker("Frames", selection: $session.accumulateFrameCount) {
                        Text("25").tag(25)
                        Text("50").tag(50)
                        Text("100").tag(100)
                        Text("200").tag(200)
                    }.labelsHidden().frame(width: 64).help("Frames to collect (50 is about 5 s)")
                }
            }
            if let refusal = session.accumulationRefusal ?? session.streamRefusal {
                Text(refusal).font(.caption2).foregroundStyle(.orange).fixedSize(
                    horizontal: false, vertical: true)
            }
        }
    }

    private func gatesView(_ gates: [Gerardus_Align_V1alpha1_GateResult]) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text("Gates, on the best hypothesis").font(.caption.weight(.semibold))
            ForEach(Array(gates.enumerated()), id: \.offset) { _, gate in
                AlignmentGateRow(gate: gate)
            }
        }
    }

    private var hypothesesView: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(session.registration == nil ? "Hypotheses so far" : "Hypotheses, best first").font(
                .caption.weight(.semibold))
            ForEach(Array(session.hypotheses.enumerated()), id: \.element.hypothesisID) {
                index, hypothesis in
                let selected = session.selectedHypothesisID == hypothesis.hypothesisID
                Button {
                    session.showHypothesis(hypothesis)
                } label: {
                    VStack(alignment: .leading, spacing: 1) {
                        HStack {
                            Text("\(index + 1)").font(.caption.weight(.semibold))
                            Text(
                                String(
                                    format: "%.1f° · moved %.1f m", hypothesis.summary.headingDeg,
                                    movedFromHint(hypothesis))
                            ).font(.caption.monospacedDigit())
                            Spacer()
                            if selected { Image(systemName: "eye").font(.caption) }
                        }
                        Text(AlignmentFormat.quality(hypothesis.quality)).font(
                            .caption2.monospacedDigit()
                        ).foregroundStyle(.secondary)
                        if let model = AlignmentFormat.modelFit(
                            hypothesis.quality, against: session.registeredAgainst)
                        {
                            Text(model).font(.caption2.monospacedDigit()).foregroundStyle(
                                .secondary)
                        }
                        Text(
                            "seeds \(hypothesis.seedsConverged)/\(hypothesis.seedsTotal)"
                                + AlignmentFormat.weak(hypothesis.quality)
                        ).font(.caption2).foregroundStyle(.secondary)
                    }.padding(4).frame(maxWidth: .infinity, alignment: .leading).background(
                        selected ? Color.accentColor.opacity(0.15) : Color.secondary.opacity(0.06)
                    ).clipShape(RoundedRectangle(cornerRadius: 4))
                }.buttonStyle(.plain).disabled(session.registration == nil).help(
                    "Draw this hypothesis; Accept then records it")
            }
        }
    }

    private func movedFromHint(_ hypothesis: Gerardus_Align_V1alpha1_Hypothesis) -> Double {
        guard let hint = session.hintPose else { return 0 }
        let delta = PoseDelta(from: hint, to: SitePose(hypothesis.pose))
        return (delta.east * delta.east + delta.north * delta.north).squareRoot()
    }

    @ViewBuilder private var fitView: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text("Fit of the pose drawn now").font(.caption.weight(.semibold))
                if session.isEvaluating { ProgressView().controlSize(.mini) }
                Spacer()
                Button("Snap") { Task { await session.snap() } }.disabled(
                    !session.canRegister || session.isEvaluating
                ).help("Refine the pose drawn now with local ICP from it")
            }
            if let evaluation = session.evaluation {
                Text(
                    (evaluation.refined ? "Snapped. " : "")
                        + AlignmentFormat.quality(evaluation.quality)
                ).font(.caption2.monospacedDigit())
                Text(
                    String(
                        format: "%.1f° · rank %d", evaluation.summary.headingDeg,
                        evaluation.quality.constraintRank)
                        + AlignmentFormat.weak(evaluation.quality)
                ).font(.caption2).foregroundStyle(.secondary)
                if let model = AlignmentFormat.modelFit(evaluation.quality) {
                    Text(model).font(.caption2.monospacedDigit()).foregroundStyle(.secondary)
                }
                Toggle("Colour the scan by residual", isOn: $session.showResiduals).toggleStyle(
                    .checkbox
                ).font(.caption)
                if session.showResiduals {
                    Text(
                        "Green on the 3DEP surface, yellow to red up to 0.5 m off, grey with none in reach."
                    ).font(.caption2).foregroundStyle(.secondary)
                }
            } else if session.canRegister {
                Text("Scored by Gerardus once the pose rests.").font(.caption2).foregroundStyle(
                    .secondary)
            }
        }
    }

    private var acceptTitle: String {
        switch session.acceptance {
        case .hypothesis(let index, let registered):
            return "Accept hypothesis \(index + 1) ("
                + (registered ? "registered" : "manually placed") + ")"
        case .pose: return "Accept this pose (manually placed)"
        }
    }

    private var reviewSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Decision").font(.subheadline.weight(.semibold))
            TextField("Note (optional)", text: $session.note)
            Button(acceptTitle) { Task { await session.accept() } }.buttonStyle(.borderedProminent)
                .disabled(!session.canReview).help(
                    "Record the pose drawn now: a hypothesis as drawn, or your own pose once you move it"
                )
            HStack {
                TextField("Why reject?", text: $session.rejectReason)
                Button("Reject") { Task { await session.reject() } }.disabled(!session.canReview)
            }
            AlignmentBlockers(reasons: session.reviewBlockers)
            if let record = session.lastRecord {
                Text("Recorded \(AlignmentFormat.state(record.state)): \(record.recordID)").font(
                    .caption
                ).foregroundStyle(.green).textSelection(.enabled)
            }
        }
    }

    // MARK: Site model

    /// The refinement itself is in its own column (RefinementColumn), opened
    /// from here; the pane keeps only the count and the way in.
    private var siteModelSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("Site model").font(.subheadline.weight(.semibold))
                Spacer()
                Button("Refine site…") { session.openRefinementColumn() }.help(
                    "Open the refinement column: re-solve every accepted pose at the site together")
            }
            Text(siteModelCaption).font(.caption).foregroundStyle(.secondary)
        }
    }

    private var siteModelCaption: String {
        let scans = "\(session.siteModelScanIDs?.count ?? 0) accepted scans"
        if session.isRefining { return scans + " · refining" }
        if let refinement = session.refinement, !session.refinementColumnOpen {
            return scans + " · a refinement of \(refinement.scans.count) scans is in its column"
        }
        return scans
    }

    private var recordsSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("Records").font(.subheadline.weight(.semibold))
                Spacer()
                Button(action: { Task { await session.loadRecords() } }) {
                    Image(systemName: "arrow.clockwise")
                }.buttonStyle(.borderless).help("Read this site's records again")
            }
            if session.records.isEmpty {
                Text("None for this site.").font(.caption).foregroundStyle(.secondary)
            }
            ForEach(session.records, id: \.recordID) { record in
                VStack(alignment: .leading, spacing: 2) {
                    HStack {
                        Text(AlignmentFormat.state(record.state)).font(.caption.weight(.semibold))
                        Spacer()
                        Text(record.reviewedAt.date.formatted(date: .abbreviated, time: .shortened))
                            .font(.caption2).foregroundStyle(.secondary)
                    }
                    if record.hasSummary {
                        Text(
                            String(
                                format: "%.1f° · %.6f, %.6f", record.summary.headingDeg,
                                record.summary.position.latitudeDeg,
                                record.summary.position.longitudeDeg)
                        ).font(.caption2.monospaced())
                    }
                    Text(
                        [record.reviewer, record.note].filter { !$0.isEmpty }.joined(
                            separator: " · ")
                    ).font(.caption2).foregroundStyle(.secondary)
                }.padding(6).background(Color.secondary.opacity(0.08)).clipShape(
                    RoundedRectangle(cornerRadius: 4))
            }
        }
    }
}

/// Why the button above is disabled, one line per reason, in plain sight
/// rather than only in a tooltip. Draws nothing while it is enabled.
struct AlignmentBlockers: View {
    let reasons: [String]

    var body: some View {
        if !reasons.isEmpty {
            VStack(alignment: .leading, spacing: 2) {
                ForEach(reasons, id: \.self) { reason in
                    Label(reason, systemImage: "exclamationmark.circle").font(.caption)
                        .foregroundStyle(.orange).fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }
}

/// One gate on one line: its mark, its name, and its value against its
/// threshold. The registration's gates and each refined scan's use it.
struct AlignmentGateRow: View {
    let gate: Gerardus_Align_V1alpha1_GateResult

    var body: some View {
        HStack {
            Image(systemName: gate.passed ? "checkmark.circle" : "xmark.circle").foregroundStyle(
                gate.passed ? .green : .orange)
            Text(gate.name).font(.caption.monospaced()).lineLimit(1)
            Spacer()
            Text(
                "\(AlignmentFormat.gateValue(gate.value)) \(gate.passed ? "within" : "against") \(AlignmentFormat.gateValue(gate.threshold))"
            ).font(.caption.monospacedDigit()).foregroundStyle(.secondary).lineLimit(1)
        }.help(gate.passed ? "Passed" : "Failed")
    }
}

/// A label and a value on one line.
struct AlignmentRow: View {
    let label: String
    let value: String

    init(_ label: String, _ value: String) {
        self.label = label
        self.value = value
    }

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(label).foregroundStyle(.secondary)
            Spacer()
            Text(value).monospacedDigit().textSelection(.enabled)
        }.font(.caption)
    }
}

enum AlignmentFormat {
    /// "ground p95 0.18 m · facade p75 0.16 m · slope 0.0001 · inliers 82 %".
    static func quality(_ q: Gerardus_Align_V1alpha1_FitQuality) -> String {
        var parts = [
            String(format: "ground p95 %.2f m", q.groundP95M),
            String(format: "facade p75 %.2f m", q.facadeP75M),
        ]
        if q.facadeRangeSlope.isFinite {
            parts.append(String(format: "slope %.4f", q.facadeRangeSlope))
        }
        parts.append(String(format: "inliers %.0f %%", q.inlierFraction * 100))
        return parts.joined(separator: " · ")
    }

    /// "site model p50 0.04 m · p75 0.07 m · 12345 points", when the fit was
    /// measured against what the request added to 3DEP (model_points > 0);
    /// nil otherwise. `against` names it: "boxes p50 …" for the DataSF boxes
    /// alone, "site model and boxes p50 …" for both. Reported as the server
    /// gives it: it is not a gate.
    static func modelFit(
        _ q: Gerardus_Align_V1alpha1_FitQuality,
        against: AlignmentModelReference = AlignmentModelReference(siteModel: true)
    ) -> String? {
        guard q.modelPoints > 0 else { return nil }
        return against.shortLabel
            + String(
                format: " p50 %.2f m · p75 %.2f m · %u points", q.modelP50M, q.modelP75M,
                q.modelPoints)
    }

    /// The caption before the registration's model residuals: "Against the
    /// DataSF boxes, reported, not gated: ".
    static func modelFitCaption(_ against: AlignmentModelReference) -> String {
        "Against \(against.label), reported, not gated: "
    }

    /// " · weak: translation along 23 deg", or nothing at full rank.
    static func weak(_ q: Gerardus_Align_V1alpha1_FitQuality) -> String {
        q.weakDirections.isEmpty ? "" : " · weak: " + q.weakDirections.joined(separator: ", ")
    }

    /// "sha256:1a2b3c4d5e6f" from a full content address: enough to tell
    /// two models apart at a glance.
    static func shortID(_ id: String) -> String {
        guard let colon = id.firstIndex(of: ":") else { return String(id.prefix(12)) }
        return String(id[...colon]) + id[id.index(after: colon)...].prefix(12)
    }

    static func gateValue(_ value: Double) -> String {
        guard value.isFinite else { return "—" }
        if value == value.rounded() && abs(value) < 1000 { return String(format: "%.0f", value) }
        return abs(value) < 0.01 ? String(format: "%.4f", value) : String(format: "%.2f", value)
    }

    static func state(_ state: Gerardus_Align_V1alpha1_RegistrationState) -> String {
        switch state {
        case .running: return "running"
        case .candidate: return "candidate"
        case .failed: return "failed"
        case .registered: return "registered"
        case .manuallyPlaced: return "manually placed"
        case .rejected: return "rejected"
        case .unspecified, .UNRECOGNIZED: return "unknown"
        }
    }
}
