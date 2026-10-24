// CameraEasing.swift
// The speed curves the camera pane shapes motion with: an orbit's progress
// round each lap, a flight's speed as it ramps up and down, and a fly-to's
// progress towards a saved viewpoint.
//
// Pure functions of t in [0, 1], with f(0) = 0 and f(1) = 1. The formulas
// are the usual Penner set (easings.net), except the exponential curves,
// which are normalised, (2^(10t) - 1) / 1023, so that they meet 0 and 1
// exactly rather than to within 2^-10. Elastic and back overshoot: their
// values leave [0, 1] between the ends. Nothing here imports AppKit, Metal
// or SwiftUI.

import Foundation

enum CameraEasing {
    /// Every curve the pane offers, in the order the dropdown lists them.
    enum Curve: String, CaseIterable, Codable, Identifiable, Sendable {
        case linear
        case easeIn
        case easeOut
        case easeInOut
        case exponentialIn
        case exponentialOut
        case exponentialInOut
        case sineInOut
        case bounceOut
        case elasticOut
        case backInOut
        case bezier

        var id: String { rawValue }

        var label: String {
            switch self {
            case .linear: return "Linear"
            case .easeIn: return "Ease in (cubic)"
            case .easeOut: return "Ease out (cubic)"
            case .easeInOut: return "Ease in-out (cubic)"
            case .exponentialIn: return "Exponential in"
            case .exponentialOut: return "Exponential out"
            case .exponentialInOut: return "Exponential in-out"
            case .sineInOut: return "Sine in-out"
            case .bounceOut: return "Bounce out"
            case .elasticOut: return "Elastic out"
            case .backInOut: return "Back in-out"
            case .bezier: return "Custom cubic Bézier"
            }
        }
    }

    /// A CSS-style cubic Bézier from (0, 0) to (1, 1) through two control
    /// points. x1 and x2 are held to [0, 1] so that x rises with the curve
    /// parameter and every t has one value.
    struct Bezier: Codable, Equatable, Sendable {
        var x1: Double
        var y1: Double
        var x2: Double
        var y2: Double

        init(_ x1: Double, _ y1: Double, _ x2: Double, _ y2: Double) {
            self.x1 = x1
            self.y1 = y1
            self.x2 = x2
            self.y2 = y2
        }

        /// CSS's `ease`.
        static let ease = Bezier(0.25, 0.1, 0.25, 1)
    }

    /// The curve's value at t, which is clamped to [0, 1].
    static func value(_ curve: Curve, _ t: Double, bezier: Bezier = .ease) -> Double {
        let t = min(max(t, 0), 1)
        switch curve {
        case .linear: return linear(t)
        case .easeIn: return easeIn(t)
        case .easeOut: return easeOut(t)
        case .easeInOut: return easeInOut(t)
        case .exponentialIn: return exponentialIn(t)
        case .exponentialOut: return exponentialOut(t)
        case .exponentialInOut: return exponentialInOut(t)
        case .sineInOut: return sineInOut(t)
        case .bounceOut: return bounceOut(t)
        case .elasticOut: return elasticOut(t)
        case .backInOut: return backInOut(t)
        case .bezier: return cubicBezier(t, bezier)
        }
    }

    // MARK: - The curves

    static func linear(_ t: Double) -> Double { t }

    static func easeIn(_ t: Double) -> Double { t * t * t }

    static func easeOut(_ t: Double) -> Double {
        let u = 1 - t
        return 1 - u * u * u
    }

    static func easeInOut(_ t: Double) -> Double {
        if t < 0.5 { return 4 * t * t * t }
        let u = -2 * t + 2
        return 1 - u * u * u / 2
    }

    static func exponentialIn(_ t: Double) -> Double { (pow(2, 10 * t) - 1) / 1023 }

    static func exponentialOut(_ t: Double) -> Double { 1 - exponentialIn(1 - t) }

    static func exponentialInOut(_ t: Double) -> Double {
        t < 0.5 ? exponentialIn(2 * t) / 2 : 1 - exponentialIn(2 - 2 * t) / 2
    }

    static func sineInOut(_ t: Double) -> Double { -(cos(Double.pi * t) - 1) / 2 }

    static func bounceOut(_ t: Double) -> Double {
        let n1 = 7.5625
        let d1 = 2.75
        if t < 1 / d1 { return n1 * t * t }
        if t < 2 / d1 {
            let u = t - 1.5 / d1
            return n1 * u * u + 0.75
        }
        if t < 2.5 / d1 {
            let u = t - 2.25 / d1
            return n1 * u * u + 0.9375
        }
        let u = t - 2.625 / d1
        return n1 * u * u + 0.984375
    }

    static func elasticOut(_ t: Double) -> Double {
        if t <= 0 { return 0 }
        if t >= 1 { return 1 }
        let c4 = 2 * Double.pi / 3
        return pow(2, -10 * t) * sin((10 * t - 0.75) * c4) + 1
    }

    static func backInOut(_ t: Double) -> Double {
        let c2 = 1.70158 * 1.525
        if t < 0.5 {
            let u = 2 * t
            return u * u * ((c2 + 1) * u - c2) / 2
        }
        let u = 2 * t - 2
        return (u * u * ((c2 + 1) * u + c2) + 2) / 2
    }

    /// y at the point of the Bézier whose x is t: Newton's method from t,
    /// falling back to bisection where the slope is too flat to trust.
    static func cubicBezier(_ t: Double, _ b: Bezier) -> Double {
        let x1 = min(max(b.x1, 0), 1)
        let x2 = min(max(b.x2, 0), 1)
        func coordinate(_ s: Double, _ p1: Double, _ p2: Double) -> Double {
            let u = 1 - s
            return 3 * u * u * s * p1 + 3 * u * s * s * p2 + s * s * s
        }
        func slope(_ s: Double, _ p1: Double, _ p2: Double) -> Double {
            let u = 1 - s
            return 3 * u * u * p1 + 6 * u * s * (p2 - p1) + 3 * s * s * (1 - p2)
        }
        if t <= 0 { return 0 }
        if t >= 1 { return 1 }
        var s = t
        for _ in 0..<8 {
            let error = coordinate(s, x1, x2) - t
            if abs(error) < 1e-12 { return coordinate(s, b.y1, b.y2) }
            let d = slope(s, x1, x2)
            if abs(d) < 1e-6 { break }
            s -= error / d
            if s < 0 || s > 1 { break }
        }
        var lo = 0.0
        var hi = 1.0
        s = t
        for _ in 0..<64 {
            if coordinate(s, x1, x2) < t { lo = s } else { hi = s }
            s = (lo + hi) / 2
        }
        return coordinate(s, b.y1, b.y2)
    }

    // MARK: - The loop seam

    /// The curve's slope at each end, by one-sided differences.
    static func endSlopes(_ curve: Curve, bezier: Bezier = .ease) -> (start: Double, end: Double) {
        let h = 1e-5
        let start = (value(curve, h, bezier: bezier) - value(curve, 0, bezier: bezier)) / h
        let end = (value(curve, 1, bezier: bezier) - value(curve, 1 - h, bezier: bezier)) / h
        return (start, end)
    }

    /// True when a loop's speed carries on unbroken from one lap into the
    /// next: the slope leaving t = 0 matches the slope arriving at t = 1.
    /// Linear and the in-out curves pass; ease in, ease out, bounce and
    /// elastic jump.
    static func isSeamContinuous(_ curve: Curve, bezier: Bezier = .ease) -> Bool {
        let s = endSlopes(curve, bezier: bezier)
        return abs(s.start - s.end) <= 0.02 * max(1, abs(s.start), abs(s.end))
    }

    /// Samples of the curve for a preview, `count` + 1 of them from t = 0.
    static func samples(_ curve: Curve, bezier: Bezier = .ease, count: Int = 64) -> [Double] {
        (0...max(count, 1)).map { value(curve, Double($0) / Double(max(count, 1)), bezier: bezier) }
    }
}
