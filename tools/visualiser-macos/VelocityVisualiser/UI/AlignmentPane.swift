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
    @EnvironmentObject var appState: AppState

    var body: some View {
        ScrollView(.vertical) {
            VStack(alignment: .leading, spacing: 14) {
                header
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
                if session.site != nil {
                    Divider()
                    recordsSection
                }
                Spacer()
            }.padding()
        }.frame(maxHeight: .infinity).background(Color(nsColor: .controlBackgroundColor)).task(
            id: appState.currentRunID
        ) { await session.refreshSites(runID: appState.currentRunID) }
    }

    // MARK: Header

    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            Text("Alignment").font(.headline)
            if case .available(let version) = session.availability {
                Text("Gerardus \(version)").font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            Button(action: { appState.showAlignmentPane = false }) { Image(systemName: "xmark") }
                .buttonStyle(.borderless).help("Close the alignment pane")
        }
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
                Toggle(
                    "Intersections (\(reference.intersections))", isOn: $session.showIntersections)
                if session.showAerial, !reference.aerialAttribution.isEmpty {
                    Text(reference.aerialAttribution).font(.caption2).foregroundStyle(.secondary)
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
            Button("Register") { Task { await session.register() } }.disabled(!session.canRegister)
                .help("Upload the settled background and register it from the pose drawn now")
            if let registration = session.registration {
                AlignmentRow("Result", AlignmentFormat.state(registration.state))
                if !registration.failureReason.isEmpty {
                    Text(registration.failureReason).font(.caption).foregroundStyle(.secondary)
                        .textSelection(.enabled)
                }
                Text(registration.registrationID).font(.caption2.monospaced()).foregroundStyle(
                    .secondary
                ).textSelection(.enabled)
            }
        }
    }

    private var reviewSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Decision").font(.subheadline.weight(.semibold))
            TextField("Reviewer", text: $session.reviewer)
            TextField("Note (optional)", text: $session.note)
            Button("Accept this pose") { Task { await session.accept() } }.buttonStyle(
                .borderedProminent
            ).disabled(!session.canReview || session.reviewer.isEmpty).help(
                "Record the pose drawn now as manually placed")
            HStack {
                TextField("Why reject?", text: $session.rejectReason)
                Button("Reject") { Task { await session.reject() } }.disabled(!session.canReview)
            }
            if let record = session.lastRecord {
                Text("Recorded \(AlignmentFormat.state(record.state)): \(record.recordID)").font(
                    .caption
                ).foregroundStyle(.green).textSelection(.enabled)
            }
        }
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

/// A label and a value on one line.
private struct AlignmentRow: View {
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
