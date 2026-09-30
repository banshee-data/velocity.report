// IntensityDisplay.swift
// Raw intensity inspection for the annotation window: the colour ramp, its
// legend, and which return is under the cursor.
//
// The channel is the sensor's intensity code, 0 to 255. It is not a
// calibrated reflectance, so everything here says "raw". The ramp is a
// display transform only: it never changes a point's bytes, a mask, a
// reference or a review, and it never filters what is inspected.
//
// The Canvas views and the Metal view colour from one 256-entry table built
// here, so the two cannot drift: the same code gets the same colour in both.

import Combine
import Foundation
import SwiftUI
import simd

/// A colour ramp over the normalised intensity range.
enum IntensityRamp: String, CaseIterable, Identifiable, Equatable {
    /// Violet through blue, cyan, green, yellow and orange to white. The low
    /// end starts bright enough to read on the black views.
    case starburst
    case heat
    case grey

    var id: String { rawValue }

    var label: String {
        switch self {
        case .starburst: return "Starburst"
        case .heat: return "Heat"
        case .grey: return "Grey"
        }
    }

    fileprivate var stops: [(t: Float, c: SIMD3<Float>)] {
        switch self {
        case .starburst:
            return [
                (0.00, SIMD3(0.45, 0.25, 0.90)), (0.20, SIMD3(0.20, 0.40, 1.00)),
                (0.40, SIMD3(0.00, 0.85, 1.00)), (0.60, SIMD3(0.30, 1.00, 0.35)),
                (0.80, SIMD3(1.00, 0.85, 0.10)), (0.92, SIMD3(1.00, 0.45, 0.10)),
                (1.00, SIMD3(1.00, 1.00, 1.00)),
            ]
        case .heat:
            return [
                (0.00, SIMD3(0.40, 0.08, 0.08)), (0.40, SIMD3(0.90, 0.15, 0.05)),
                (0.75, SIMD3(1.00, 0.80, 0.10)), (1.00, SIMD3(1.00, 1.00, 1.00)),
            ]
        case .grey: return [(0.00, SIMD3(0.28, 0.28, 0.28)), (1.00, SIMD3(1.00, 1.00, 1.00))]
        }
    }

    func colour(at t: Float) -> SIMD3<Float> {
        let stops = self.stops
        let t = min(max(t, 0), 1)
        for i in 1..<stops.count where t <= stops[i].t {
            let a = stops[i - 1]
            let b = stops[i]
            let f = b.t > a.t ? (t - a.t) / (b.t - a.t) : 0
            return a.c + (b.c - a.c) * f
        }
        return stops[stops.count - 1].c
    }
}

/// Whether a raw code fell outside the display range and was clamped.
enum IntensityClip: Equatable {
    case none
    case below
    case above
}

/// How the annotation views colour points by raw intensity.
struct IntensityDisplaySpec: Equatable {
    static let rawMax = 255
    static let gammaRange: ClosedRange<Double> = 0.25...4
    static let brightnessRange: ClosedRange<Double> = 0.5...2

    /// Off by default: the normal class and object palette, with no
    /// intensity-driven colour or brightness at all.
    var enabled = false
    private(set) var lower = 0
    private(set) var upper = IntensityDisplaySpec.rawMax
    var ramp: IntensityRamp = .starburst
    private(set) var gamma = 1.0
    private(set) var brightness = 1.0

    /// The colour shown for a point whose intensity was not measured. Distinct
    /// from every ramp's colour for zero, which is a measurement.
    static let missingColour = SIMD3<Float>(0.62, 0.30, 0.52)

    /// Sets the lower limit, keeping it in range and below the upper one.
    mutating func setLower(_ value: Int) {
        lower = min(max(value, 0), IntensityDisplaySpec.rawMax - 1)
        if upper <= lower { upper = lower + 1 }
    }

    mutating func setUpper(_ value: Int) {
        upper = min(max(value, 1), IntensityDisplaySpec.rawMax)
        if lower >= upper { lower = upper - 1 }
    }

    mutating func setGamma(_ value: Double) {
        gamma = min(
            max(value, IntensityDisplaySpec.gammaRange.lowerBound),
            IntensityDisplaySpec.gammaRange.upperBound)
    }

    mutating func setBrightness(_ value: Double) {
        brightness = min(
            max(value, IntensityDisplaySpec.brightnessRange.lowerBound),
            IntensityDisplaySpec.brightnessRange.upperBound)
    }

    /// Full range, the default ramp, no contrast or brightness change. Whether
    /// the colouring is on is left as it was.
    mutating func reset() {
        let on = enabled
        self = IntensityDisplaySpec()
        enabled = on
    }

    var isDefaultRange: Bool { lower == 0 && upper == IntensityDisplaySpec.rawMax }

    /// Where a raw code falls on the ramp, after range and gamma, and whether
    /// it was clamped to get there.
    func position(of raw: UInt8) -> (t: Float, clip: IntensityClip) {
        let r = Int(raw)
        if r < lower { return (0, .below) }
        if r > upper { return (1, .above) }
        let linear = Double(r - lower) / Double(upper - lower)
        return (Float(pow(linear, gamma)), .none)
    }

    func colour(of raw: UInt8) -> SIMD3<Float> {
        let c = ramp.colour(at: position(of: raw).t) * Float(brightness)
        return simd_clamp(c, SIMD3(repeating: 0), SIMD3(repeating: 1))
    }

    /// One colour per raw code, for both renderers. When intensity was not
    /// measured every entry is the missing colour, so no measured-looking
    /// colour is ever drawn from zero-filled bytes.
    func table(available: Bool) -> [SIMD4<Float>] {
        (0...IntensityDisplaySpec.rawMax).map { raw in
            let c = available ? colour(of: UInt8(raw)) : IntensityDisplaySpec.missingColour
            return SIMD4(c, 1)
        }
    }

    /// Legend ticks: where along the bar (0 to 1) each label sits, and the raw
    /// code there. With gamma other than one the codes are not evenly spaced,
    /// which is the point of showing them.
    func legendTicks(count: Int = 5) -> [(position: Double, raw: Double)] {
        guard count >= 2 else { return [] }
        return (0..<count).map { i in
            let position = Double(i) / Double(count - 1)
            let linear = pow(position, 1 / gamma)
            return (position, Double(lower) + linear * Double(upper - lower))
        }
    }

    /// How many of these codes the range clamps, below and above.
    func clipCounts(_ codes: some Sequence<UInt8>) -> (below: Int, above: Int) {
        var below = 0
        var above = 0
        for raw in codes {
            if Int(raw) < lower { below += 1 } else if Int(raw) > upper { above += 1 }
        }
        return (below, above)
    }
}

/// Whether this pack's intensity is a measurement.
///
/// The pack says so once, for the whole pack. The exporter sets that flag if
/// any sample carried intensity and zero-fills the rest, so a true flag does
/// not prove every sample was measured; that is said beside the readout rather
/// than inferred from whether a byte is zero.
enum IntensityAvailability: Equatable {
    /// This sample's source frame carried intensity.
    case measured
    /// The pack carries intensity, but this sample's source frame did not:
    /// its bytes are zero-fill.
    case absentThisSample
    /// The pack declares intensity as a whole and records nothing per sample.
    case packFlag
    /// The pack carries no intensity at all.
    case absent

    /// From the pack-wide flag and, when the pack records it, the sample's own.
    init(hasIntensity: Bool, sample: Bool? = nil) {
        switch (hasIntensity, sample) {
        case (false, _): self = .absent
        case (true, true?): self = .measured
        case (true, false?): self = .absentThisSample
        case (true, nil): self = .packFlag
        }
    }

    var available: Bool { self == .measured || self == .packFlag }

    var caveat: String {
        switch self {
        case .measured:
            return "This frame's source carried intensity; the stored bytes are its measurements."
        case .absentThisSample:
            return "This frame's source carried no intensity. Its stored bytes are zero-fill, not "
                + "measurements, so nothing is coloured or read from them here; other frames may differ."
        case .packFlag:
            return "The pack declares intensity for the pack as a whole; presence is not recorded "
                + "per frame. A frame the source did not measure would read as zeros."
        case .absent:
            return "This pack carries no intensity. Its stored bytes are zero-fill, not "
                + "measurements, so nothing is coloured or read from them."
        }
    }
}

/// One return and its exact stored values.
struct IntensityReadout: Equatable {
    var sampleID: Int
    var sourceOrdinal: Int
    var pointIndex: Int
    /// The stored byte, or nil when the pack has no intensity.
    var raw: UInt8?
    var position: SIMD3<Float>
    var displayClass: UInt8
    /// The view it was picked in, and its depth there: nearer the viewer is
    /// smaller.
    var depth: Float
}

/// The returns under the cursor in one view, nearest first.
enum IntensityPicker {
    /// Every return drawn within `radius` points of `screen`, ordered by
    /// screen distance and then by depth, so the first is the one on top.
    /// Picks only what the view draws: the same projection, facing rule and
    /// class filter.
    static func candidates(
        points: PackPoints, classes: [UInt8], basis: OrthoViewBasis, viewport: OrthoViewport,
        at screen: CGPoint, radius: CGFloat, isVisible: (Int) -> Bool, limit: Int = 8
    ) -> [(index: Int, distance: CGFloat, depth: Float)] {
        var found: [(index: Int, distance: CGFloat, depth: Float)] = []
        for index in 0..<points.count where isVisible(index) {
            guard let p = points.point(at: index), basis.shows(p) else { continue }
            let s = viewport.screenPoint(from: basis.project(p))
            let d = hypot(s.x - screen.x, s.y - screen.y)
            guard d <= radius else { continue }
            found.append((index, d, basis.depth(p)))
        }
        found.sort {
            if abs($0.distance - $1.distance) > 0.5 { return $0.distance < $1.distance }
            if $0.depth != $1.depth { return $0.depth < $1.depth }
            return $0.index < $1.index
        }
        return Array(found.prefix(limit))
    }
}

/// What the intensity inspector shows: the returns under the cursor, and the
/// one the operator pinned.
///
/// Its own object, like BrushHover, so a mouse move republishes the inspector
/// and not the session's views.
@MainActor final class IntensityInspectionState: ObservableObject {
    @Published private(set) var hovered: [IntensityReadout] = []
    /// Which of the hovered returns is shown first; cycled with a key.
    @Published private(set) var hoveredChoice = 0
    /// A return the operator chose, kept until the frame changes.
    @Published private(set) var pinned: IntensityReadout?
    /// Returns under the cursor when the pin was made, so the next key can
    /// step to the one behind it.
    private var pinnedCandidates: [IntensityReadout] = []

    var current: IntensityReadout? {
        hovered.indices.contains(hoveredChoice) ? hovered[hoveredChoice] : hovered.first
    }

    func setHovered(_ readouts: [IntensityReadout]) {
        guard readouts != hovered else { return }
        hovered = readouts
        hoveredChoice = 0
    }

    /// Pins the return currently shown under the cursor.
    func pin() {
        guard let current else { return }
        pinned = current
        pinnedCandidates = hovered
    }

    /// Steps the pin, or the hover, to the next return under the same place.
    func next() {
        if let pinned, pinnedCandidates.count > 1, let i = pinnedCandidates.firstIndex(of: pinned) {
            self.pinned = pinnedCandidates[(i + 1) % pinnedCandidates.count]
            return
        }
        guard hovered.count > 1 else { return }
        hoveredChoice = (hoveredChoice + 1) % hovered.count
    }

    func unpin() {
        pinned = nil
        pinnedCandidates = []
    }

    /// A point index names a return in one scan; a new frame clears it.
    func clear() {
        hovered = []
        hoveredChoice = 0
        unpin()
    }
}

extension SIMD3 where Scalar == Float {
    var swiftUIColour: Color { Color(red: Double(x), green: Double(y), blue: Double(z)) }
}
