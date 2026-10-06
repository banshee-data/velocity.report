// AnnotationCompleteness.swift
// How much of a frame has been labelled, and where the rest is.
//
// A frame holds thousands of foreground returns and an operator labels them an
// object at a time, so "am I done with this frame" has no answer on screen.
// This gives one: the foreground split into sixteen sectors round the sensor,
// each saying how much of it is agreed, how much is in question, and how much
// nobody has looked at.

import Foundation
import simd

/// How many labelable returns are in each state.
struct LabelTally: Equatable {
    var total = 0
    /// In a mask a person made, accepted or reviewed.
    var agreed = 0
    /// Proposed and not yet confirmed: propagated by an algorithm, marked
    /// uncertain, or changed in the editor and not saved.
    var inQuestion = 0

    var unlabelled: Int { max(total - agreed - inQuestion, 0) }
    var fractionAgreed: Double { total > 0 ? Double(agreed) / Double(total) : 0 }
    var fractionInQuestion: Double { total > 0 ? Double(inQuestion) / Double(total) : 0 }
    var isComplete: Bool { total > 0 && agreed == total }
}

struct FrameCompleteness: Equatable {
    /// Sixteen sectors of 22.5 degrees.
    static let sectorCount = 16

    var sectors = [LabelTally](repeating: LabelTally(), count: FrameCompleteness.sectorCount)
    var whole = LabelTally()

    /// The sector a position falls in. Sector 0 begins at the sensor's +X axis
    /// and sectors run anticlockwise, which is how the top view is drawn: the
    /// ring can be read against it without turning either.
    static func sector(x: Float, y: Float) -> Int {
        var angle = atan2(y, x)
        if angle < 0 { angle += 2 * .pi }
        let width = 2 * Float.pi / Float(sectorCount)
        return min(Int(angle / width), sectorCount - 1)
    }

    /// The range of bearings a sector covers, in radians anticlockwise from +X.
    static func bearings(ofSector sector: Int) -> ClosedRange<Float> {
        let width = 2 * Float.pi / Float(sectorCount)
        return (Float(sector) * width)...(Float(sector + 1) * width)
    }

    /// Tallies a frame. Only returns the tracker could have used count: what
    /// the height band removed, and recorded background, are not waiting to
    /// be labelled.
    static func tally(
        points: PackPoints, classes: [UInt8], agreed: Set<Int>, inQuestion: Set<Int>
    ) -> FrameCompleteness {
        var result = FrameCompleteness()
        for index in 0..<points.count where isLabelable(classes, index) {
            let sector = sector(x: points.x[index], y: points.y[index])
            result.sectors[sector].total += 1
            result.whole.total += 1
            // In question wins: a return one mask agrees on and another has
            // only proposed is not settled.
            if inQuestion.contains(index) {
                result.sectors[sector].inQuestion += 1
                result.whole.inQuestion += 1
            } else if agreed.contains(index) {
                result.sectors[sector].agreed += 1
                result.whole.agreed += 1
            }
        }
        return result
    }

    static func isLabelable(_ classes: [UInt8], _ index: Int) -> Bool {
        guard index < classes.count else { return true }
        return classes[index] == PointClass.foreground || classes[index] == PointClass.unclassified
    }
}

extension FrameMask {
    /// True when a person stands behind this mask: they made it, accepted a
    /// proposal into it, or reviewed it. A mask an algorithm propagated and
    /// nobody has reviewed is a proposal, however it was saved.
    var isAgreed: Bool { status == .reviewed || provenance.algorithm == nil }
}

/// One frame's place on the frame strip.
struct FrameProgress: Equatable {
    var labelable = 0
    var agreed = 0
    var inQuestion = 0
    /// True when a new background snapshot came into force at this frame.
    var backgroundUpdated = false

    var fractionAgreed: Double { labelable > 0 ? min(Double(agreed) / Double(labelable), 1) : 0 }
    var fractionInQuestion: Double {
        labelable > 0 ? min(Double(inQuestion) / Double(labelable), 1 - fractionAgreed) : 0
    }
}
