// MaskPropagation.swift
// Carrying an object's mask through the frames without a person in the loop,
// and knowing when to stop and ask.
//
// Measured on a real pack, an operator accepting one carried proposal at a
// time spends about six seconds a frame, and nearly all of it agreeing with
// what the footprint had already found. This does the agreeing, and stops on
// the frames where a person would not have agreed either: the object is gone,
// it has doubled in size, something else fits as well, or the fit is somewhere
// the object could not have got to.

import Foundation
import simd

/// Why a propagation ended.
enum PropagationStop: Equatable {
    case endOfPack
    case frameLimit
    case cancelled
    /// The object already has a mask in the next frame.
    case alreadyLabelled
    /// Too few returns where the object should be.
    case lost(found: Int, expected: Int)
    /// Far more returns than the object had: it has run into something.
    case grew(found: Int, expected: Int)
    /// Another place fits nearly as well as the best one.
    case ambiguous
    /// The best fit is further from the prediction than the object could move.
    case jumped(metres: Float)
    /// The fit takes in returns another object already has.
    case overlaps(objectName: String)

    /// True when a person is needed at the frame it stopped on.
    var needsOperator: Bool {
        switch self {
        case .endOfPack, .frameLimit, .cancelled, .alreadyLabelled: return false
        case .lost, .grew, .ambiguous, .jumped, .overlaps: return true
        }
    }

    var explanation: String {
        switch self {
        case .endOfPack: return "reached the end of the pack"
        case .frameLimit: return "reached the frame limit"
        case .cancelled: return "stopped"
        case .alreadyLabelled: return "the next frame is already labelled"
        case .lost(let found, let expected):
            return "lost it: \(found) points where about \(expected) were expected"
        case .grew(let found, let expected):
            return "it grew: \(found) points where about \(expected) were expected, so it has "
                + "probably run into something"
        case .ambiguous: return "another position fits nearly as well"
        case .jumped(let metres):
            return String(format: "the best fit is %.1f m from where it should be", metres)
        case .overlaps(let name): return "the fit takes in points that belong to \(name)"
        }
    }
}

/// What a propagation did.
struct PropagationOutcome: Equatable {
    var objectName: String
    var framesWritten: Int
    var stop: PropagationStop

    var summary: String {
        "Carried \(objectName) through \(framesWritten) frame\(framesWritten == 1 ? "" : "s"): "
            + stop.explanation + "."
    }
}

/// Judges one frame's fit. Kept apart from the session so that the thresholds
/// can be tested as rules rather than through a pack.
enum PropagationJudge {
    /// Below this share of the expected returns, the object is lost.
    static let lostFraction: Float = 0.5
    /// Above this multiple of them, it has merged with something.
    static let grewFactor: Float = 2.0
    /// A rival fit this close to the best makes the best ambiguous.
    static let rivalFraction: Float = 0.8
    /// Too few returns to be worth calling an object at all.
    static let minimumPoints = 5
    /// How far a fit may lie from the prediction: a fixed allowance for the
    /// search's own coarseness, plus half the predicted movement, because a
    /// faster object changes speed by more between frames.
    static func allowedDeviation(predicted: Float) -> Float { 0.75 + 0.5 * predicted }
    /// Share of a fit that may already belong to another object.
    static let overlapFraction: Float = 0.2

    /// The returns to expect, given how many the object had and how its range
    /// has changed. A spinning sensor's returns from a surface fall off with
    /// the square of the range, so a car driving away should lose them, and
    /// calling that "lost" would stop every propagation of a receding object.
    static func expected(previousCount: Int, previousRange: Float, range: Float) -> Int {
        guard previousRange > 0, range > 0 else { return previousCount }
        let scale = min(max((previousRange / range) * (previousRange / range), 0.5), 2.0)
        return max(Int((Float(previousCount) * scale).rounded()), 1)
    }

    /// Nil when the fit should be accepted.
    ///
    /// `speedKnown` is false on the first step from a frame nobody has carried
    /// from: the prediction is then "it has not moved", which is not a
    /// prediction, and holding a car doing ten metres a second to it would
    /// refuse every vehicle at its first frame. The search radius is the only
    /// limit until a first fit says how fast the object is going.
    static func verdict(
        fit: FootprintFit, prediction: simd_float3, expected: Int, isFixed: Bool,
        speedKnown: Bool = true
    ) -> PropagationStop? {
        if fit.count < max(minimumPoints, Int(Float(expected) * lostFraction)) {
            return .lost(found: max(fit.count, 0), expected: expected)
        }
        if Float(fit.count) > Float(expected) * grewFactor + 10 {
            return .grew(found: fit.count, expected: expected)
        }
        // Something that does not move is not searched for, so it has no
        // rival and cannot jump.
        guard !isFixed else { return nil }
        if Float(fit.runnerUp) >= Float(fit.count) * rivalFraction { return .ambiguous }
        let deviation = simd_distance(fit.offset, prediction)
        if speedKnown, deviation > allowedDeviation(predicted: simd_length(prediction)) {
            return .jumped(metres: deviation)
        }
        return nil
    }
}
