// RoadReviewPane.swift
// The road surface review pane: choose a prepared site on the Gerardus
// connection, load its road surface preview, switch the evidence layers, and
// inspect a patch. State is RoadReviewSession's.

import AppKit
import SwiftUI

/// The toolbar's switch for the pane.
@available(macOS 15.0, *) struct RoadToolbarButton: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        Toggle(isOn: $appState.showRoadPane) { Label("Road", systemImage: "road.lanes") }
            .toggleStyle(.button).help("Review Gerardus's road surface preview")
    }
}

@available(macOS 15.0, *) struct RoadReviewPane: View {
    @ObservedObject var session: RoadReviewSession
    @ObservedObject var loader: RoadSurfaceLoader
    @ObservedObject var drive: RoadDriveSession
    @EnvironmentObject var appState: AppState

    static let width: CGFloat = 340

    init(session: RoadReviewSession, drive: RoadDriveSession) {
        self.session = session
        loader = session.loader
        self.drive = drive
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header.padding([.horizontal, .top]).padding(.bottom, 10)
            Divider()
            ScrollView(.vertical) {
                VStack(alignment: .leading, spacing: 14) {
                    sitesSection
                    statusSection
                    Divider()
                    layersSection
                    Divider()
                    RoadDriveSection(session: session, drive: drive)
                    Divider()
                    if let inspector = session.inspector {
                        RoadInspectorView(inspector: inspector)
                        Divider()
                    } else {
                        Text("Click a road patch to inspect it.").font(.caption).foregroundStyle(
                            .secondary)
                    }
                    coverageSection
                    legendSection
                    Spacer()
                }.padding()
            }
        }.frame(maxHeight: .infinity).background(Color(nsColor: .controlBackgroundColor))
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            Text("Road surface").font(.headline)
            Text("prototype").font(.caption2).padding(.horizontal, 5).background(
                Capsule().fill(.orange.opacity(0.25)))
            Spacer()
            Button(action: { appState.showRoadPane = false }) { Image(systemName: "xmark") }
                .buttonStyle(.borderless).help("Close the road pane")
        }
    }

    private var sitesSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Site").font(.subheadline.weight(.semibold))
            if session.sites.isEmpty {
                Text(session.sitesNote ?? "No prepared sites.").font(.caption).foregroundStyle(
                    .secondary)
            }
            ForEach(session.sites) { choice in
                Button(action: { session.open(siteToken: choice.id) }) {
                    HStack {
                        Image(
                            systemName: session.openedSite == choice.id
                                ? "checkmark.circle.fill" : "circle")
                        Text(choice.label).lineLimit(1)
                        Spacer()
                        Text(choice.id).font(.caption.monospaced()).foregroundStyle(.secondary)
                    }
                }.buttonStyle(.plain)
            }
            HStack {
                Button("Refresh") { Task { await session.refreshSites() } }
                if session.openedSite != nil {
                    Button("Frame") { session.frameCorridor() }.help(
                        "Overhead view of the loaded patches")
                    Button("Close") { session.close() }
                }
            }.controlSize(.small)
        }
    }

    private var statusSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                if loader.state.isLoading { ProgressView().controlSize(.small) }
                Text(session.statusLine).font(.caption)
                if loader.state.isLoading {
                    Button("Cancel") { session.cancelLoad() }.controlSize(.small)
                }
            }
            if let blocker = session.blocker {
                Label(blocker, systemImage: "exclamationmark.triangle").font(.caption)
                    .foregroundStyle(.orange).fixedSize(horizontal: false, vertical: true)
            }
            if !loader.store.conflicts.isEmpty {
                Text(
                    "Conflicting geometry under one patch id: \(loader.store.conflicts.joined(separator: ", "))"
                ).font(.caption).foregroundStyle(.red)
            }
            if !loader.store.rejected.isEmpty {
                Text("\(loader.store.rejected.count) patch(es) refused as corrupt, not drawn.")
                    .font(.caption).foregroundStyle(.red)
            }
            ForEach(loader.siteNotes.sorted(by: { $0.key < $1.key }), id: \.key) { _, note in
                Text(note).font(.caption).foregroundStyle(.orange)
            }
        }
    }

    private var layersSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Layers").font(.subheadline.weight(.semibold))
            Toggle("Road surface", isOn: $session.options.surface)
            Picker("Shading", selection: $session.options.shading) {
                ForEach(RoadShading.allCases, id: \.self) { Text($0.label).tag($0) }
            }.pickerStyle(.segmented).disabled(!session.options.surface)
            Toggle("Wireframe", isOn: $session.options.wireframe)
            Toggle("Centreline", isOn: $session.options.centreline)
            Toggle("Boundary rings", isOn: $session.options.boundary)
            Toggle("City kerbs", isOn: $session.options.cityKerbs)
            Toggle("Class-2 3DEP points", isOn: $session.options.class2Points)
            Toggle("City centrelines", isOn: $session.options.cityCentrelines)
            if let note = session.contextNote {
                Text(note).font(.caption2).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
            }
        }.toggleStyle(.checkbox).font(.callout)
    }

    private var coverageSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Coverage").font(.subheadline.weight(.semibold))
            if session.coverage.isEmpty {
                Text("No corridor loaded.").font(.caption).foregroundStyle(.secondary)
            }
            ForEach(session.coverage, id: \.corridorID) { c in
                VStack(alignment: .leading, spacing: 2) {
                    Text(c.corridorID).font(.callout.weight(.medium))
                    Text(
                        "\(c.patchCount) patches · frame accuracy \(String(format: "%.1f", c.frameAccuracyM * 1000)) mm"
                    ).font(.caption)
                    Text(
                        "Inventory \(Self.metres(c.inventoryLengthM)): surfaced \(Self.metres(c.surfacedLengthM)) (\(Int((c.surfacedShare * 100).rounded())) %), of which inferred width \(Self.metres(c.inferredLengthM)); missing \(Self.metres(c.missingLengthM))."
                    ).font(.caption).fixedSize(horizontal: false, vertical: true)
                    Text("This is a corridor, not the whole park.").font(.caption).foregroundStyle(
                        .orange)
                    ForEach(Array(c.gaps.enumerated()), id: \.offset) { _, gap in
                        Text(
                            "Gap: \(gap.name.isEmpty ? gap.cnn : gap.name) — \(gap.reason), \(Self.metres(gap.lengthM)). \(gap.message)"
                        ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                            horizontal: false, vertical: true)
                    }
                    Text("build \(c.buildHash.prefix(12))").font(.caption2.monospaced())
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    private var legendSection: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text("Key").font(.subheadline.weight(.semibold))
            ForEach([RoadSupportClass.observed, .interpolated, .missing], id: \.rawValue) { s in
                legend(
                    RoadSurfaceStyle.colour(s),
                    s == .missing ? "missing: left open, edge in red" : s.label)
            }
            ForEach(
                [RoadBoundarySourceClass.kerbPair, .cityKerb, .inferredWidth, .authored],
                id: \.rawValue
            ) { b in
                legend(
                    RoadSurfaceStyle.colour(b), b.isInferred ? "inferred width (dashed)" : b.label)
            }
        }
    }

    private func legend(_ c: SIMD4<Float>, _ text: String) -> some View {
        HStack(spacing: 6) {
            Rectangle().fill(Color(red: Double(c.x), green: Double(c.y), blue: Double(c.z))).frame(
                width: 14, height: 8)
            Text(text).font(.caption2)
        }
    }

    static func metres(_ m: Float) -> String {
        m >= 1000 ? String(format: "%.2f km", m / 1000) : String(format: "%.0f m", m)
    }
}

@available(macOS 15.0, *) struct RoadInspectorView: View {
    let inspector: RoadPatchInspector

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text("Patch").font(.subheadline.weight(.semibold))
            row("id", inspector.patchID)
            row("CNN", inspector.cnn.isEmpty ? "none" : "\(inspector.cnn) \(inspector.name)")
            row("kind", inspector.kind)
            row("corridor", inspector.corridor)
            row(
                "boundary",
                inspector.boundarySource.label
                    + (inspector.boundarySource.isInferred ? " — hand-authored, not surveyed" : ""))
            row(
                "edges",
                inspector.edgeSources.sorted { $0.key < $1.key }.map {
                    "\($0.value) \($0.key.label)"
                }.joined(separator: ", "))
            row("ground observed", inspector.observed.isEmpty ? "unknown" : inspector.observed)
            row("sources", inspector.sourceVersion)
            row(
                "area",
                String(
                    format: "%.0f m²: %.0f observed, %.0f interpolated, %.0f missing",
                    inspector.areaM2, inspector.observedAreaM2, inspector.interpolatedAreaM2,
                    inspector.missingAreaM2))
            row(
                "rider line",
                String(
                    format: "%.0f %% observed, %.0f %% interpolated, %.0f %% missing",
                    inspector.riderLine.observed * 100, inspector.riderLine.interpolated * 100,
                    inspector.riderLine.missing * 100))
            row(
                "held out",
                inspector.heldOutPoints == 0
                    ? "none (no residual)"
                    : String(
                        format: "%d of %d points: median %+.3f, RMS %.3f, p95 %.3f m",
                        inspector.heldOutPoints, inspector.groundPoints + inspector.heldOutPoints,
                        inspector.residualMedianM, inspector.residualRmsM, inspector.residualP95AbsM
                    ))
            row(
                "safety",
                inspector.assessed == 0
                    ? "C1–C18 not assessed"
                    : "\(inspector.assessed) assessed, \(inspector.notAssessed) not")
            row("geometry", String(inspector.geometryHash.prefix(16)))
        }
    }

    private func row(_ k: String, _ v: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            Text(k).font(.caption).foregroundStyle(.secondary).frame(width: 78, alignment: .leading)
            Text(v).font(.caption).textSelection(.enabled).fixedSize(
                horizontal: false, vertical: true)
        }
    }
}

@available(macOS 15.0, *) struct RoadDriveSection: View {
    @ObservedObject var session: RoadReviewSession
    @ObservedObject var drive: RoadDriveSession
    @State private var start: String = ""
    @State private var backwards = false

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Drive").font(.subheadline.weight(.semibold))
            let options = drive.startOptions
            if options.isEmpty {
                Text("Load a corridor with road profiles to drive it.").font(.caption)
                    .foregroundStyle(.secondary)
            } else {
                Picker("Start", selection: $start) {
                    ForEach(options) { Text(Self.short($0.patchID)).tag($0.patchID) }
                }.labelsHidden()
                Toggle("Drive from the far end", isOn: $backwards).toggleStyle(.checkbox)
                HStack {
                    Button(drive.isDriving ? "Restart" : "Start driving") {
                        drive.begin(
                            on: start.isEmpty ? options[0].patchID : start, reversed: backwards)
                    }
                    if drive.isDriving {
                        Button(drive.driver?.playing == true ? "Pause" : "Play") {
                            drive.togglePlay()
                        }.disabled(drive.stopReason != nil)
                        Button("Overhead") { drive.overhead() }.help(
                            "Leave the driving camera and look from above, to inspect a defect")
                    }
                }.controlSize(.small)
            }
            if drive.isDriving {
                HStack {
                    Text("Position").font(.caption)
                    Slider(
                        value: Binding(
                            get: { Double(drive.distanceAlong) },
                            set: { drive.scrub(to: Float($0)) }),
                        in: 0...Double(max(drive.routeLength, 1)))
                    Text(String(format: "%.0f / %.0f m", drive.distanceAlong, drive.routeLength))
                        .font(.caption.monospacedDigit())
                }
                HStack {
                    Text("Speed").font(.caption)
                    Slider(
                        value: Binding(
                            get: { Double(drive.speed) }, set: { drive.speed = Float($0) }),
                        in: 0...30)
                    Text(String(format: "%.0f m/s", drive.speed)).font(.caption.monospacedDigit())
                }
                HStack {
                    Text("Eye height").font(.caption)
                    Slider(
                        value: Binding(
                            get: { Double(drive.eyeHeight) }, set: { drive.eyeHeight = Float($0) }),
                        in: 0.5...3)
                    Text(String(format: "%.1f m", drive.eyeHeight)).font(.caption.monospacedDigit())
                }
                if let d = drive.driver {
                    Text(
                        "Under the eye: \(d.supportHere().label)\(d.grade().map { String(format: ", grade %+.1f %%", $0 * 100) } ?? "")"
                    ).font(.caption).foregroundStyle(.secondary)
                }
                junctionChoices
                traverseControls
            }
            if let message = drive.message {
                Label(message, systemImage: "hand.raised").font(.caption).foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    /// The working-set radii and the timed traverse that measures them.
    @ViewBuilder private var traverseControls: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(
                String(
                    format: "Working set: draw %.0f m, fetch %.0f m, drop beyond %.0f m",
                    drive.policy.visibleM, drive.policy.prefetchM, drive.policy.evictM)
            ).font(.caption2).foregroundStyle(.secondary)
            HStack {
                if drive.traverseUntil == nil {
                    Button("Traverse 10 min") { drive.startTraverse(seconds: 600) }
                    Button("1 min") { drive.startTraverse(seconds: 60) }
                } else {
                    Button("Stop traverse") { drive.stopTraverse() }
                    Text("loop \(drive.traverseLoops + 1)").font(.caption2)
                }
            }.controlSize(.small)
            if let s = drive.lastTraverse {
                let note = TraverseReport.markdown(
                    s, drive.provenance(), date: ISO8601DateFormatter().string(from: Date()),
                    label: "road view")
                Text(
                    String(
                        format: "p95 %.1f ms over %d frames (%@)", s.intervalP95Ms, s.frames,
                        s.meetsTarget ? "target met" : "target not met")
                ).font(.caption)
                Button("Copy note") {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(note, forType: .string)
                }.controlSize(.small)
            }
        }
    }

    /// At each junction the route passes, the arms that could be taken.
    @ViewBuilder private var junctionChoices: some View {
        let junctions = session.patches.filter { $0.kind == .junction }
        if !junctions.isEmpty {
            VStack(alignment: .leading, spacing: 2) {
                Text("Junction route").font(.caption.weight(.medium))
                ForEach(junctions) { j in
                    HStack {
                        Text("node \(Self.node(j))").font(.caption2)
                        ForEach(
                            session.patches.filter {
                                $0.kind == .segment
                                    && ($0.fromNode == Self.node(j) || $0.toNode == Self.node(j))
                            }
                        ) { arm in
                            Button(Self.short(arm.patchID)) {
                                drive.choose(arm: arm.patchID, at: j.patchID)
                            }.controlSize(.mini)
                        }
                    }
                }
            }
        }
    }

    static func node(_ junction: RoadPatch) -> String {
        junction.featureID.split(separator: "/").last.map(String.init) ?? junction.cnn
    }

    /// "segment/straight/0" from the full id.
    static func short(_ id: String) -> String {
        id.split(separator: "/").suffix(3).joined(separator: "/")
    }
}
