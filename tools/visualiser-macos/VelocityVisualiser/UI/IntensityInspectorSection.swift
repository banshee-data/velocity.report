// IntensityInspectorSection.swift
// Raw intensity in the annotation window: an explicit colour toggle, the ramp
// and its legend, and an exact readout of the return under the cursor.
//
// Independent of the Points and Physical reference modes, and of everything
// saved: changing the ramp or reading a value writes nothing and marks
// nothing dirty.

import SwiftUI

struct IntensityInspectorSection: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject private var inspection: IntensityInspectionState

    init(session: AnnotationSession) {
        self.session = session
        self._inspection = ObservedObject(wrappedValue: session.inspection)
    }

    private var spec: IntensityDisplaySpec { session.intensityDisplay }
    private var available: Bool { session.intensityAvailability.available }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Intensity (raw 0–255)").font(.headline)
            Toggle("Reflectivity colour", isOn: $session.intensityDisplay.enabled).font(.caption)
                .help(
                    "Colour returns by the sensor's raw intensity code. Off restores the normal palette."
                )
            if spec.enabled { rampControls }
            Toggle("Inspect returns", isOn: $session.inspectIntensity).font(.caption).help(
                "Read out the return under the cursor. M pins it; N steps to the next one under the same place."
            )
            if session.inspectIntensity { readout }
            Text(session.intensityAvailability.caveat).font(.caption2).foregroundStyle(
                available ? Color.secondary : Color.orange
            ).fixedSize(horizontal: false, vertical: true)
            Text(
                "Sensor \(session.pack.manifest.source.sensorID) · stored as the recorded byte · "
                    + "no calibration recorded. Intensity changes with range and incidence; it is "
                    + "not a material reflectance, and codes from different sensors are not comparable."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    // MARK: Ramp

    private var rampControls: some View {
        VStack(alignment: .leading, spacing: 4) {
            Picker("Ramp", selection: $session.intensityDisplay.ramp) {
                ForEach(IntensityRamp.allCases) { Text($0.label).tag($0) }
            }.font(.caption)
            HStack(spacing: 6) {
                rangeField("min", value: spec.lower) { session.intensityDisplay.setLower($0) }
                rangeField("max", value: spec.upper) { session.intensityDisplay.setUpper($0) }
            }
            slider("Contrast (γ)", value: spec.gamma, range: IntensityDisplaySpec.gammaRange) {
                session.intensityDisplay.setGamma($0)
            }
            slider(
                "Brightness", value: spec.brightness, range: IntensityDisplaySpec.brightnessRange
            ) { session.intensityDisplay.setBrightness($0) }
            legend
            clipLine
            Button("Reset to full range") { session.intensityDisplay.reset() }.controlSize(.small)
                .disabled(
                    spec
                        == {
                            var d = IntensityDisplaySpec()
                            d.enabled = true
                            return d
                        }())
        }
    }

    private func rangeField(_ label: String, value: Int, set: @escaping (Int) -> Void) -> some View
    {
        HStack(spacing: 2) {
            Text(label).font(.caption2).foregroundStyle(.secondary)
            TextField("", value: Binding(get: { value }, set: set), format: .number).textFieldStyle(
                .roundedBorder
            ).font(.caption.monospacedDigit()).frame(width: 44)
            Stepper(
                "", value: Binding(get: { value }, set: set), in: 0...IntensityDisplaySpec.rawMax
            ).labelsHidden()
        }
    }

    private func slider(
        _ label: String, value: Double, range: ClosedRange<Double>, set: @escaping (Double) -> Void
    ) -> some View {
        HStack {
            Text(label).font(.caption2).frame(width: 74, alignment: .leading)
            Slider(value: Binding(get: { value }, set: set), in: range)
            Text(String(format: "%.2f", value)).font(.caption2.monospacedDigit()).frame(width: 30)
        }
    }

    /// The active ramp between the chosen limits, with the raw codes where
    /// the ticks fall: evenly spaced along the bar, and so unevenly in codes
    /// when the contrast is not one. Beyond the limits, colours clamp.
    private var legend: some View {
        let spec = spec
        let available = available
        return VStack(alignment: .leading, spacing: 1) {
            Canvas { context, size in
                let steps = 64
                for i in 0..<steps {
                    let position = (Double(i) + 0.5) / Double(steps)
                    let raw =
                        Double(spec.lower) + pow(position, 1 / spec.gamma)
                        * Double(spec.upper - spec.lower)
                    let c =
                        available
                        ? spec.colour(of: UInt8(min(max(raw.rounded(), 0), 255)))
                        : IntensityDisplaySpec.missingColour
                    let x = CGFloat(i) / CGFloat(steps) * size.width
                    context.fill(
                        Path(
                            CGRect(
                                x: x, y: 0, width: size.width / CGFloat(steps) + 0.5,
                                height: size.height)), with: .color(c.swiftUIColour))
                }
            }.frame(height: 10).background(Color.black)
            HStack {
                ForEach(Array(spec.legendTicks().enumerated()), id: \.offset) { index, tick in
                    if index > 0 { Spacer(minLength: 0) }
                    Text(String(format: "%.0f", tick.raw)).font(.system(size: 8).monospacedDigit())
                }
            }
            if !available {
                Text("Unavailable: drawn in the missing-intensity colour").font(.caption2)
                    .foregroundStyle(.orange)
            } else if !spec.isDefaultRange {
                Text(
                    "Codes below \(spec.lower) or above \(spec.upper) are clamped to the end colours."
                ).font(.caption2).foregroundStyle(.secondary)
            }
        }
    }

    private var clipLine: some View {
        let counts = spec.clipCounts(session.currentPoints.intensity)
        return Group {
            if available, counts.below + counts.above > 0 {
                Text("This frame: \(counts.below) clamped low, \(counts.above) clamped high").font(
                    .caption2
                ).foregroundStyle(.secondary)
            }
        }
    }

    // MARK: Readout

    private var readout: some View {
        VStack(alignment: .leading, spacing: 3) {
            if let pinned = inspection.pinned {
                readoutLine(pinned, title: "Pinned")
                Button("Unpin") { inspection.unpin() }.controlSize(.mini)
            }
            if let current = inspection.current {
                readoutLine(current, title: "Under cursor")
                if inspection.hovered.count > 1 {
                    Text(
                        "\(inspection.hovered.count) returns here; showing \(inspection.hoveredChoice + 1). "
                            + "N shows the next."
                    ).font(.caption2).foregroundStyle(.secondary)
                }
            } else if inspection.pinned == nil {
                Text("Move the cursor over a return.").font(.caption2).foregroundStyle(.secondary)
            }
        }
    }

    private func readoutLine(_ r: IntensityReadout, title: String) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(title).font(.caption2.bold())
            Text(r.raw.map { "intensity \($0) (raw)" } ?? "intensity unavailable").font(
                .caption.monospacedDigit())
            Text("point \(r.pointIndex) · sample \(r.sampleID) · frame \(r.sourceOrdinal)").font(
                .caption2.monospacedDigit()
            ).foregroundStyle(.secondary)
            Text(
                String(format: "x %.2f  y %.2f  z %.2f m", r.position.x, r.position.y, r.position.z)
            ).font(.caption2.monospacedDigit()).foregroundStyle(.secondary)
        }.textSelection(.enabled)
    }
}
