// CityMode.swift
// The pieces of the visualiser's city mode that are not drawing
// (lidar-city-view-plan Item 7): the flight between sites, the site markers
// and street-name labels laid over the view, and the separation cues that
// keep each site's recording to itself.
//
// Each intersection is recorded on its own. Nothing here joins one site's
// road users to another's: colour is by class, never by track; a site's
// boxes are drawn only inside its coverage and fade at its edge; each site
// is labelled with its own date and clock.

import CoreGraphics
import Foundation
import simd

/// The line of copy city mode carries (plan, "Said in words").
enum SeparationCopy {
    static let line =
        "Each intersection is recorded on its own. Vehicles are not followed from one "
        + "intersection to the next."
}

/// A played site's coverage: its points and boxes are drawn inside it only,
/// and boxes fade as they reach its edge.
struct SiteCoverage: Equatable, Sendable {
    /// The scene's vector radius: the published scenes draw their measured
    /// geometry within it, and the city's buildings fade inside it.
    static let vectorRadiusM: Float = 90
    /// The default coverage, metres from the sensor.
    static let standard = SiteCoverage(radius: 120)
    /// A site that cannot be placed in the city draws nothing there.
    static let hidden = SiteCoverage(radius: 0.001)
    /// The ring's height in the sensor frame: the road, about 2.3 m below
    /// a sensor on its pole.
    static let groundZ: Float = -2.3

    var radius: Float
    /// The share of the radius at which boxes start to fade.
    var fadeStart: Float = 0.85

    /// A box's opacity at `distance` metres from the sensor: 1 well inside,
    /// falling to 0 at the edge, 0 beyond.
    func opacity(atDistance distance: Float) -> Float {
        guard radius > 0 else { return 1 }
        let start = radius * fadeStart
        if distance <= start { return 1 }
        if distance >= radius { return 0 }
        return 1 - (distance - start) / (radius - start)
    }

    /// The ring at the edge, as line segments in the sensor's frame on its
    /// ground (z), for the line pipeline: x, y, z, r, g, b, a per vertex.
    func ring(groundZ: Float, segments: Int = 96) -> [Float] {
        var out = [Float]()
        out.reserveCapacity(segments * 14)
        let colour: [Float] = [0.85, 0.85, 0.9, 0.55]
        for i in 0..<segments {
            for k in [i, i + 1] {
                let a = Float(k) / Float(segments) * 2 * .pi
                out += [radius * cos(a), radius * sin(a), groundZ] + colour
            }
        }
        return out
    }
}

/// Colour by class only: a vehicle, a cycle, someone walking, or other.
/// Never by track, so nothing suggests two boxes are one road user.
enum ClassColour {
    static func colour(for classLabel: String) -> SIMD3<Float> {
        switch classLabel.lowercased() {
        case "car", "vehicle", "truck", "bus", "van", "motorcycle": return SIMD3(0.30, 0.62, 1.0)
        case "cyclist", "bicycle", "bike", "cycle", "scooter": return SIMD3(0.35, 0.85, 0.45)
        case "pedestrian", "person", "walking": return SIMD3(1.0, 0.62, 0.20)
        default: return SIMD3(0.72, 0.72, 0.76)
        }
    }
}

/// A camera flight across the city: it rises, crosses and comes down.
struct CityFlight: Equatable, Sendable {
    var fromTarget: SIMD3<Double>
    var fromEyeOffset: SIMD3<Double>
    var toTarget: SIMD3<Double>
    var toEyeOffset: SIMD3<Double>
    var duration: Double

    init(
        fromTarget: SIMD3<Double>, fromEye: SIMD3<Double>, toTarget: SIMD3<Double>,
        toEyeOffset: SIMD3<Double>
    ) {
        self.fromTarget = fromTarget
        self.fromEyeOffset = fromEye - fromTarget
        self.toTarget = toTarget
        self.toEyeOffset = toEyeOffset
        let across = simd_length(SIMD2(toTarget.x - fromTarget.x, toTarget.y - fromTarget.y))
        duration = min(max(1.2 + across / 3_000, 1.2), 4)
    }

    /// The view from above a site that a flight comes down to: 160 m out
    /// from the sensor's south-west and 120 m up.
    static let arrivalOffset = SIMD3<Double>(-110, -110, 120)

    private static func ease(_ t: Double) -> Double { t * t * (3 - 2 * t) }

    /// The camera's target and eye at `t` in [0, 1].
    func at(_ t: Double) -> (target: SIMD3<Double>, eye: SIMD3<Double>) {
        let u = Self.ease(min(max(t, 0), 1))
        let target = fromTarget + (toTarget - fromTarget) * u
        var offset = fromEyeOffset + (toEyeOffset - fromEyeOffset) * u
        // Rise in the middle, by half the distance across, at most 4 km.
        let across = simd_length(SIMD2(toTarget.x - fromTarget.x, toTarget.y - fromTarget.y))
        offset.z += min(across * 0.5, 4_000) * sin(.pi * u)
        return (target, target + offset)
    }
}

/// A marker over the view: one per site in the index.
struct CityMarker: Identifiable, Equatable, Sendable {
    var id: String
    var title: String
    /// "2026-09-02", and for the site playing, its clock.
    var date: String
    var clock: String?
    var placed: Bool
    var focused: Bool
    var screen: CGPoint
}

/// A street name over the view, turned along its street.
struct ScreenStreetLabel: Identifiable, Equatable, Sendable {
    var id: String
    var name: String
    var screen: CGPoint
    /// Degrees, clockwise on screen, kept within ±90 so text reads upright.
    var angleDeg: Double
    var arterial: Bool
}

/// What city mode lays over the view each frame.
struct CityOverlay: Equatable, Sendable {
    var markers: [CityMarker] = []
    var streets: [ScreenStreetLabel] = []
}

enum CityOverlayLayout {
    /// Where a point of the local frame falls on a view of `size` points
    /// (origin top left), or nil behind the eye or off the view.
    static func project(
        _ local: SIMD3<Float>, viewProjection: simd_float4x4, size: CGSize, margin: CGFloat = 40
    ) -> CGPoint? {
        let clip = viewProjection * SIMD4(local, 1)
        guard clip.w > 1e-4 else { return nil }
        let x = CGFloat((clip.x / clip.w + 1) / 2) * size.width
        let y = CGFloat((1 - clip.y / clip.w) / 2) * size.height
        guard x > -margin, y > -margin, x < size.width + margin, y < size.height + margin else {
            return nil
        }
        return CGPoint(x: x, y: y)
    }

    /// A label's angle on screen from its anchor and a point a little along
    /// its street, kept upright.
    static func angle(from a: CGPoint, to b: CGPoint) -> Double {
        var degrees = atan2(Double(b.y - a.y), Double(b.x - a.x)) * 180 / .pi
        if degrees > 90 { degrees -= 180 }
        if degrees < -90 { degrees += 180 }
        return degrees
    }

    /// Thins street labels so none overlaps another: arterials first, then
    /// in the order given; each name at most once within `repeatGap` points.
    static func thin(
        _ labels: [ScreenStreetLabel], charWidth: CGFloat = 6.5, height: CGFloat = 14,
        repeatGap: CGFloat = 220, limit: Int = 60
    ) -> [ScreenStreetLabel] {
        let ordered = labels.enumerated().sorted { a, b in
            a.element.arterial != b.element.arterial ? a.element.arterial : a.offset < b.offset
        }.map(\.element)
        var kept = [ScreenStreetLabel]()
        var boxes = [CGRect]()
        for label in ordered {
            guard kept.count < limit else { break }
            // The label's footprint as an axis-aligned box around its turned
            // extent: generous, but cheap and never overlapping.
            let w = CGFloat(label.name.count) * charWidth
            let a = label.angleDeg * .pi / 180
            let bw = abs(w * CGFloat(cos(a))) + abs(height * CGFloat(sin(a)))
            let bh = abs(w * CGFloat(sin(a))) + abs(height * CGFloat(cos(a)))
            let box = CGRect(
                x: label.screen.x - bw / 2, y: label.screen.y - bh / 2, width: bw, height: bh)
            if boxes.contains(where: { $0.intersects(box) }) { continue }
            if kept.contains(where: {
                $0.name == label.name
                    && hypot($0.screen.x - label.screen.x, $0.screen.y - label.screen.y) < repeatGap
            }) {
                continue
            }
            kept.append(label)
            boxes.append(box)
        }
        return kept
    }

    /// A site's clock, San Francisco time, from a playback timestamp.
    static func clock(timestampNanos: Int64) -> String? {
        guard timestampNanos > 0 else { return nil }
        let date = Date(timeIntervalSince1970: Double(timestampNanos) / 1e9)
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone(identifier: "America/Los_Angeles")
        formatter.dateFormat = "HH:mm:ss"
        return formatter.string(from: date)
    }
}
