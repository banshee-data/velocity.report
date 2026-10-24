// CameraPane.swift
// The camera pane: orbit the 3D view's camera round an ellipse, or fly it
// freely, with one speed curve shaping both; measure the ellipse's radii
// from the camera, the scene or two clicks; and keep viewpoints to fly to.
// State is CameraControlSession's.

import SwiftUI

/// The toolbar's switch for the pane.
@available(macOS 15.0, *) struct CameraToolbarButton: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        Toggle(isOn: $appState.showCameraPane) { Label("Camera", systemImage: "move.3d") }
            .toggleStyle(.button).help("Orbit or fly the 3D view's camera (⌥⌘C)")
    }
}

@available(macOS 15.0, *) struct CameraPane: View {
    @ObservedObject var session: CameraControlSession
    @ObservedObject var painter: StreetPainterSession
    @EnvironmentObject var appState: AppState
    @State private var viewpointName = ""

    static let width: CGFloat = 330

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: 8) {
                header
                Picker("Mode", selection: $session.settings.mode) {
                    Text("Orbit").tag(CameraPaneSettings.Mode.orbit)
                    Text("Fly").tag(CameraPaneSettings.Mode.fly)
                }.pickerStyle(.segmented).labelsHidden()
                if let blocker = session.blocker {
                    Label(blocker, systemImage: "lock").font(.caption).foregroundStyle(.orange)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }.padding([.horizontal, .top]).padding(.bottom, 10)
            Divider()
            ScrollView(.vertical) {
                VStack(alignment: .leading, spacing: 14) {
                    CurveSection(session: session)
                    Divider()
                    if session.settings.mode == .orbit {
                        OrbitSection(session: session)
                    } else {
                        FlySection(session: session)
                    }
                    Divider()
                    viewpointsSection
                    Divider()
                    persistenceSection
                    if let message = session.message {
                        Text(message).font(.caption).foregroundStyle(.secondary).fixedSize(
                            horizontal: false, vertical: true)
                    }
                    Spacer()
                }.padding()
            }
        }.frame(maxHeight: .infinity).background(Color(nsColor: .controlBackgroundColor)).onAppear {
            session.refreshPreview()
        }
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            Text("Camera").font(.headline)
            Spacer()
            Button(action: { appState.showCameraPane = false }) { Image(systemName: "xmark") }
                .buttonStyle(.borderless).help("Close the camera pane")
        }
    }

    // MARK: Viewpoints

    private var viewpointsSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Viewpoints").font(.subheadline.weight(.semibold))
            HStack {
                TextField("Name", text: $viewpointName).textFieldStyle(.roundedBorder)
                Button("Save view") {
                    session.saveViewpoint(named: viewpointName)
                    viewpointName = ""
                }.disabled(!session.canSaveViewpoint).help(
                    session.canSaveViewpoint
                        ? "Keep where the camera is now, for this site"
                        : "\(CameraPaneStore.maxViewpoints) are kept: delete one first")
            }
            if session.viewpoints.isEmpty {
                Text("None saved for this site.").font(.caption).foregroundStyle(.secondary)
            }
            ForEach(session.viewpoints) { viewpoint in
                HStack {
                    Text(viewpoint.name).font(.callout).lineLimit(1)
                    Spacer()
                    Button("Fly to") { session.fly(to: viewpoint) }.controlSize(.small).disabled(
                        session.blocker != nil
                    ).help(
                        "Move the camera here over \(session.settings.fly.flyToSeconds.formatted()) s along the speed curve"
                    )
                    Button(role: .destructive) {
                        session.deleteViewpoint(viewpoint.id)
                    } label: {
                        Image(systemName: "trash")
                    }.buttonStyle(.borderless).controlSize(.small).help("Delete this viewpoint")
                }
            }
            if let heading = session.flyingTo {
                Text("Flying to \(heading.name)…").font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    // MARK: Persistence

    private var persistenceSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(
                session.siteKey.map { "Settings are kept for site \($0)." }
                    ?? "No site is loaded: settings are kept as the default."
            ).font(.caption2).foregroundStyle(.secondary)
            if session.siteKey != nil {
                Button("Use these as the default") { session.saveAsDefault() }.controlSize(.small)
                    .help("Start sites without settings of their own from these")
            }
        }
    }
}

// MARK: - The speed curve

@available(macOS 15.0, *) private struct CurveSection: View {
    @ObservedObject var session: CameraControlSession

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .center) {
                Picker("Speed curve", selection: $session.settings.curve) {
                    ForEach(CameraEasing.Curve.allCases) { curve in Text(curve.label).tag(curve) }
                }
                CurvePreview(curve: session.settings.curve, bezier: session.settings.bezier).frame(
                    width: 56, height: 40)
            }
            if session.settings.curve == .bezier {
                HStack(spacing: 4) {
                    NumberField(label: "x1", value: $session.settings.bezier.x1, width: 44)
                    NumberField(label: "y1", value: $session.settings.bezier.y1, width: 44)
                    NumberField(label: "x2", value: $session.settings.bezier.x2, width: 44)
                    NumberField(label: "y2", value: $session.settings.bezier.y2, width: 44)
                }.help("Control points of a cubic Bézier from (0, 0) to (1, 1), as CSS writes them")
            }
            Text(
                session.settings.mode == .orbit
                    ? "Shapes progress round each lap." : "Shapes how speed ramps up and down."
            ).font(.caption2).foregroundStyle(.secondary)
            if let warning = session.seamWarning {
                Label(warning, systemImage: "exclamationmark.triangle").font(.caption2)
                    .foregroundStyle(.orange).fixedSize(horizontal: false, vertical: true)
            }
        }
    }
}

/// The curve, f(t) against t, scaled to show any overshoot.
struct CurvePreview: View {
    let curve: CameraEasing.Curve
    let bezier: CameraEasing.Bezier

    var body: some View {
        Canvas { context, size in
            let values = CameraEasing.samples(curve, bezier: bezier, count: 48)
            let low = min(values.min() ?? 0, 0)
            let high = max(values.max() ?? 1, 1)
            let span = max(high - low, 1e-6)
            func point(_ i: Int) -> CGPoint {
                CGPoint(
                    x: size.width * CGFloat(i) / CGFloat(values.count - 1),
                    y: size.height * CGFloat(1 - (values[i] - low) / span))
            }
            var frame = Path()
            frame.addRect(CGRect(origin: .zero, size: size))
            context.stroke(frame, with: .color(.secondary.opacity(0.3)), lineWidth: 0.5)
            var line = Path()
            line.move(to: point(0))
            for i in 1..<values.count { line.addLine(to: point(i)) }
            context.stroke(line, with: .color(.accentColor), lineWidth: 1.5)
        }.accessibilityLabel("\(curve.label) curve")
    }
}

// MARK: - Orbit

@available(macOS 15.0, *) private struct OrbitSection: View {
    @ObservedObject var session: CameraControlSession

    private var orbit: OrbitSettings { session.settings.orbit }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            playControls
            centreRow
            radiiRows
            measureRows
            Divider()
            shapeRows
            Divider()
            timingRows
            Toggle("Show the path in the 3D view", isOn: $session.settings.orbit.showPreview)
                .toggleStyle(.checkbox)
        }
    }

    private var playControls: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Button(action: session.play) { Label("Play", systemImage: "play.fill") }.disabled(
                    session.playback == .playing || session.blocker != nil)
                Button(action: session.pause) { Label("Pause", systemImage: "pause.fill") }
                    .disabled(session.playback != .playing)
                Button(action: session.stop) { Label("Stop", systemImage: "stop.fill") }.disabled(
                    session.playback == .stopped
                ).help("End the orbit and put the camera back where it was before Play")
            }.controlSize(.small)
            Text(playbackLine).font(.caption).foregroundStyle(.secondary)
        }
    }

    private var playbackLine: String {
        let lap = Int(session.laps) + 1
        let percent = Int((session.laps - floor(session.laps)) * 100)
        switch session.playback {
        case .stopped: return "Stopped. Orbiting, the view pauses at any orbit, pan or zoom."
        case .playing: return orbit.loop ? "Lap \(lap), \(percent)%" : "\(percent)% of one lap"
        case .paused: return "Paused at lap \(lap), \(percent)%: Play resumes, Stop puts back"
        case .finished: return "Lap done. Play goes round again; Stop puts the camera back."
        }
    }

    private var centreRow: some View {
        VStack(alignment: .leading, spacing: 2) {
            Picker("Centre", selection: $session.settings.orbit.centre) {
                ForEach(OrbitSettings.Centre.allCases) { Text($0.label).tag($0) }
            }.disabled(session.playback != .stopped)
            let resolved = session.resolveCentre()
            Text(
                String(
                    format: "At (%.1f, %.1f, %.1f) m", resolved.centre.x, resolved.centre.y,
                    resolved.centre.z) + (resolved.note.map { " · " + $0 } ?? "")
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    private var radiiRows: some View {
        VStack(alignment: .leading, spacing: 6) {
            RadiusRow(
                label: "A (along)", value: orbit.radiusA, measured: session.measured?.a,
                set: session.setRadiusA)
            RadiusRow(
                label: "B (across)", value: orbit.radiusB, measured: session.measured?.b,
                set: session.setRadiusB)
            Toggle("Lock the A/B ratio", isOn: $session.settings.orbit.lockRatio).toggleStyle(
                .checkbox
            ).help("Editing one radius scales the other with it")
            HStack {
                Text("A − B").frame(width: 70, alignment: .leading)
                NumberField(
                    value: Binding(get: { orbit.difference }, set: session.setDifference),
                    width: 70, unit: "m")
                Text("A/B").frame(width: 32, alignment: .trailing)
                NumberField(
                    value: Binding(get: { orbit.ratio }, set: session.setRatio), width: 60,
                    digits: 3)
            }.help("Editing either keeps A and moves B")
        }
    }

    private var measureRows: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Measure").font(.callout.weight(.medium))
            HStack {
                Button("From camera") { session.measureFromCamera() }.help(
                    "A and B from the camera's offset from the centre, so the orbit starts where the camera is"
                )
                Button("From scene") { session.measureFromScene() }.help(
                    "Fit A and B to the registered scans, or the boxes and kerbs, around the centre"
                )
                if session.picking == nil {
                    Button("Pick two points") { session.startPicking() }.help(
                        "Click in the 3D view where A ends, then where B ends")
                } else {
                    Button("Cancel pick") { session.cancelPicking() }
                }
            }.controlSize(.small)
            HStack {
                Text("Margin").font(.caption)
                NumberField(value: $session.settings.orbit.marginPercent, width: 50, unit: "%")
                Text("beyond the scene").font(.caption).foregroundStyle(.secondary)
            }
            if let measured = session.measured {
                Text(
                    String(
                        format: "Measured from the %@: A %.1f m, B %.1f m", measured.source,
                        measured.a, measured.b)
                ).font(.caption2).foregroundStyle(.secondary)
            }
        }
    }

    private var shapeRows: some View {
        VStack(alignment: .leading, spacing: 6) {
            SliderRow(
                label: "Heading", value: $session.settings.orbit.headingDeg, range: 0...360,
                unit: "°", help: "A's direction, clockwise from north")
            SliderRow(
                label: "Start", value: $session.settings.orbit.startAngleDeg, range: -180...180,
                unit: "°", help: "Where Play begins, from A's end towards B's")
            Picker("Height", selection: $session.settings.orbit.heightMode) {
                Text("Height").tag(OrbitSettings.HeightMode.height)
                Text("Elevation").tag(OrbitSettings.HeightMode.elevation)
            }.pickerStyle(.segmented)
            if orbit.heightMode == .height {
                SliderRow(
                    label: "Above", value: $session.settings.orbit.heightM, range: -10...200,
                    unit: "m", help: "The camera's height above the centre")
            } else {
                SliderRow(
                    label: "Angle", value: $session.settings.orbit.elevationDeg, range: -30...89,
                    unit: "°", help: "The camera's angle up from the look-at point")
            }
            SliderRow(
                label: "Look at", value: $session.settings.orbit.lookAtHeightM, range: -10...50,
                unit: "m", help: "The height above the centre the camera looks at")
        }
    }

    private var timingRows: some View {
        VStack(alignment: .leading, spacing: 6) {
            SliderRow(
                label: "Lap", value: $session.settings.orbit.lapSeconds, range: 2...300, unit: "s",
                help: "Seconds for one lap")
            Picker("Direction", selection: $session.settings.orbit.clockwise) {
                Text("Clockwise").tag(true)
                Text("Anticlockwise").tag(false)
            }.pickerStyle(.segmented)
            Picker("Repeat", selection: $session.settings.orbit.loop) {
                Text("Loop").tag(true)
                Text("Once").tag(false)
            }.pickerStyle(.segmented)
        }
    }
}

/// A radius: a slider, a field, and the measured value beside it once the
/// field has been edited away from it.
private struct RadiusRow: View {
    let label: String
    let value: Double
    let measured: Double?
    let set: (Double) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Text(label).frame(width: 70, alignment: .leading)
                Slider(value: Binding(get: { min(max(value, 0.5), 300) }, set: set), in: 0.5...300)
                NumberField(value: Binding(get: { value }, set: set), width: 60, unit: "m")
            }
            if let measured, abs(measured - value) > 0.005 {
                Text(
                    String(
                        format: "measured %.2f m · edited by %+.2f m", measured, value - measured)
                ).font(.caption2).foregroundStyle(.secondary).padding(.leading, 74)
            }
        }
    }
}

// MARK: - Fly

@available(macOS 15.0, *) private struct FlySection: View {
    @ObservedObject var session: CameraControlSession

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Toggle(isOn: Binding(get: { session.isFlying }, set: { session.setFlying($0) })) {
                Label(session.isFlying ? "Flying" : "Fly", systemImage: "airplane")
            }.toggleStyle(.button).disabled(!session.isFlying && session.blocker != nil)
            Text(
                "W/S or ↑/↓ forward and back, A/D or ←/→ sideways, Q/E down and up. Hold ⌥ and move the mouse to look. Click the 3D view first if the keys do nothing."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            if session.isFlying {
                Text(String(format: "%.1f m/s", session.flySpeed)).font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
            }
            SliderRow(
                label: "Top speed", value: $session.settings.fly.maxSpeed, range: 0.5...100,
                unit: "m/s", help: "Speed with the keys held to the end of the ramp")
            SliderRow(
                label: "Ramp", value: $session.settings.fly.rampSeconds, range: 0.05...5, unit: "s",
                help:
                    "Seconds from standing to top speed after a key goes down, and back after it comes up"
            )
            SliderRow(
                label: "Fly to", value: $session.settings.fly.flyToSeconds, range: 0...20,
                unit: "s", help: "How long Fly to takes to reach a viewpoint")
        }
    }
}

// MARK: - Fields

/// A slider with its value as an editable field beside it.
private struct SliderRow: View {
    let label: String
    @Binding var value: Double
    let range: ClosedRange<Double>
    let unit: String
    let help: String

    var body: some View {
        HStack {
            Text(label).frame(width: 70, alignment: .leading)
            Slider(
                value: Binding(
                    get: { min(max(value, range.lowerBound), range.upperBound) },
                    set: { value = $0 }), in: range)
            NumberField(value: $value, width: 60, unit: unit)
        }.help(help)
    }
}

/// A number typed in, committed on Return or when the field loses focus.
private struct NumberField: View {
    var label: String = ""
    @Binding var value: Double
    var width: CGFloat
    var unit: String = ""
    var digits = 2

    var body: some View {
        HStack(spacing: 2) {
            if !label.isEmpty { Text(label).font(.caption).foregroundStyle(.secondary) }
            TextField(label, value: $value, format: .number.precision(.fractionLength(0...digits)))
                .frame(width: width).multilineTextAlignment(.trailing).textFieldStyle(
                    .roundedBorder)
            if !unit.isEmpty { Text(unit).font(.caption).foregroundStyle(.secondary) }
        }
    }
}
