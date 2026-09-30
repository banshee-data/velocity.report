//
//  IntensityDistributionTests.swift
//  VelocityVisualiserTests
//
//  Raw intensity histogram and the experimental peak rule
//  (intensity-peaks/v1): fixed bin edges, one denominator, duplicate and
//  excluded indices, data errors, the three population states, and every
//  edge, plateau, valley and tie case of peak extraction.
//

import Foundation
import Testing

@testable import VelocityVisualiser

private func codes(_ runs: [(UInt8, Int)]) -> [UInt8] {
    runs.flatMap { Array(repeating: $0.0, count: $0.1) }
}

private func meanAndPopulationSD(_ values: [UInt8]) -> (Double, Double) {
    let xs = values.map(Double.init)
    let mean = xs.reduce(0, +) / Double(xs.count)
    let variance = xs.map { ($0 - mean) * ($0 - mean) }.reduce(0, +) / Double(xs.count)
    return (mean, variance.squareRoot())
}

private func close(_ a: Double, _ b: Double) -> Bool { abs(a - b) < 1e-9 }

struct IntensityBinningTests {
    @Test func sixteenBinEdges() {
        let b = IntensityBinning.sixteen
        #expect(b.binCount == 16 && b.binWidth == 16)
        let cases: [(UInt8, Int)] = [
            (0, 0), (7, 0), (8, 0), (15, 0), (16, 1), (31, 1), (32, 2), (239, 14), (240, 15),
            (247, 15), (248, 15), (255, 15),
        ]
        for (code, bin) in cases { #expect(b.bin(of: code) == bin, "code \(code)") }
        #expect(b.range(ofBin: 0) == 0...15)
        #expect(b.range(ofBin: 1) == 16...31)
        #expect(b.range(ofBin: 15) == 240...255)
    }

    @Test func thirtyTwoBinEdges() {
        let b = IntensityBinning.thirtyTwo
        #expect(b.binCount == 32 && b.binWidth == 8)
        let cases: [(UInt8, Int)] = [
            (0, 0), (7, 0), (8, 1), (15, 1), (16, 2), (247, 30), (248, 31), (255, 31),
        ]
        for (code, bin) in cases { #expect(b.bin(of: code) == bin, "code \(code)") }
        #expect(b.range(ofBin: 0) == 0...7)
        #expect(b.range(ofBin: 30) == 240...247)
        #expect(b.range(ofBin: 31) == 248...255)
    }

    @Test func everyCodeLiesInsideItsBinRangeAndBinsTile() {
        for b in IntensityBinning.allCases {
            for code in 0...255 {
                let bin = b.bin(of: UInt8(code))
                #expect(b.range(ofBin: bin).contains(code))
            }
            let covered = (0..<b.binCount).map { b.range(ofBin: $0).count }.reduce(0, +)
            #expect(covered == 256)
        }
    }
}

struct IntensityHistogramTests {
    @Test func countsSumToMeasuredAndFractionsShareIt() throws {
        let all = (0...255).map(UInt8.init)
        for b in IntensityBinning.allCases {
            let h = IntensityHistogram(codes: all, binning: b)
            #expect(h.counts.count == b.binCount)
            #expect(h.counts.reduce(0, +) == h.measuredCount && h.measuredCount == 256)
            #expect(h.counts.allSatisfy { $0 == b.binWidth })
            let f = try #require(h.fractions)
            for (i, fr) in f.enumerated() {
                #expect(close(fr, Double(h.counts[i]) / Double(h.measuredCount)))
            }
            #expect(close(f.reduce(0, +), 1))
        }
    }

    @Test func zeroIsMeasuredInTheFirstBinAnd255InTheLast() {
        let h = IntensityHistogram(codes: [0, 0, 255], binning: .sixteen)
        #expect(h.counts[0] == 2 && h.counts[15] == 1 && h.measuredCount == 3)
    }

    @Test func emptyPopulationHasNoFractions() {
        let h = IntensityHistogram(codes: [], binning: .thirtyTwo)
        #expect(h.isEmpty && h.fractions == nil && h.counts == Array(repeating: 0, count: 32))
    }

    @Test func duplicateIndicesCountOnce() throws {
        let intensity: [UInt8] = [10, 20, 30, 40]
        let h = try IntensityHistogram(
            intensity: intensity, indices: [1, 1, 2, 2, 2, 1], excluded: [], binning: .sixteen)
        #expect(h.measuredCount == 2 && h.measuredCodes == [20, 30])
        #expect(h.counts[1] == 2 && h.counts.reduce(0, +) == 2)
    }

    @Test func excludedIndicesAreCountedSeparately() throws {
        let intensity: [UInt8] = [10, 20, 30, 40, 50]
        let h = try IntensityHistogram(
            intensity: intensity, indices: [0, 1, 2, 3, 3], excluded: [1, 3, 4], binning: .sixteen)
        // 4 is excluded but not selected, so it is not disclosed as excluded.
        #expect(h.excludedCount == 2 && h.measuredCount == 2 && h.measuredCodes == [10, 30])
        let f = try #require(h.fractions)
        #expect(close(f[0], 0.5) && close(f[1], 0.5))
    }

    @Test func outOfRangeIndexIsADataError() {
        let intensity: [UInt8] = [1, 2, 3]
        #expect(
            throws: IntensityDistributionError.indicesOutOfRange(
                count: 2, smallest: -1, pointCount: 3)
        ) {
            _ = try IntensityHistogram(
                intensity: intensity, indices: [0, 3, -1, 3], excluded: [], binning: .sixteen)
        }
        // An excluded index is still validated.
        #expect(throws: IntensityDistributionError.self) {
            _ = try IntensityHistogram(
                intensity: intensity, indices: [7], excluded: [7], binning: .sixteen)
        }
    }
}

struct IntensityPopulationTests {
    @Test func emptySelectionDiffersFromUnavailableIntensity() throws {
        let empty = try IntensityPopulation(
            intensityAvailable: true, intensity: [0, 0], pointCount: 2, indices: [Int](),
            excluded: [], binning: .sixteen)
        #expect(empty == .emptySelection && empty.histogram == nil)

        // Zero-filled storage with the flag false is unavailable, not zeros.
        let absent = try IntensityPopulation(
            intensityAvailable: false, intensity: [0, 0], pointCount: 2, indices: [0, 1, 1],
            excluded: [], binning: .sixteen)
        #expect(absent == .unavailable(selectedCount: 2) && absent.histogram == nil)

        let absentNoArray = try IntensityPopulation(
            intensityAvailable: false, intensity: [], pointCount: 2, indices: [1], excluded: [],
            binning: .sixteen)
        #expect(absentNoArray == .unavailable(selectedCount: 1))
    }

    @Test func presentAllZeroIntensityIsMeasured() throws {
        let p = try IntensityPopulation(
            intensityAvailable: true, intensity: [0, 0, 0], pointCount: 3, indices: [0, 1, 2],
            excluded: [], binning: .thirtyTwo)
        let h = try #require(p.histogram)
        #expect(h.measuredCount == 3 && h.counts[0] == 3)
    }

    @Test func allSelectedExcludedIsAnEmptyMeasuredPopulation() throws {
        let p = try IntensityPopulation(
            intensityAvailable: true, intensity: [5, 6], pointCount: 2, indices: [0, 1],
            excluded: [0, 1], binning: .sixteen)
        let h = try #require(p.histogram)
        #expect(h.isEmpty && h.excludedCount == 2 && h.fractions == nil)
    }

    @Test func malformedSourcesThrowInEveryState() {
        #expect(
            throws: IntensityDistributionError.indicesOutOfRange(
                count: 1, smallest: 5, pointCount: 2)
        ) {
            _ = try IntensityPopulation(
                intensityAvailable: false, intensity: [], pointCount: 2, indices: [5], excluded: [],
                binning: .sixteen)
        }
        #expect(
            throws: IntensityDistributionError.intensityLengthMismatch(
                intensityCount: 1, pointCount: 2)
        ) {
            _ = try IntensityPopulation(
                intensityAvailable: true, intensity: [9], pointCount: 2, indices: [0], excluded: [],
                binning: .sixteen)
        }
    }
}

struct IntensityPeaksTests {
    private func run(
        _ values: [UInt8], _ binning: IntensityBinning = .sixteen, min: Int = 1
    ) -> (peaks: [IntensityPeak], unassigned: Int) {
        let h = IntensityHistogram(codes: values, binning: binning)
        return IntensityPeaks.extract(histogram: h, codes: values, minimumSupport: min)
    }

    @Test func versionIsExposed() {
        #expect(IntensityPeaks.algorithmVersion == "intensity-peaks/v1")
    }

    @Test func emptyPopulationHasNoPeaks() {
        let r = run([])
        #expect(r.peaks.isEmpty && r.unassigned == 0)
    }

    @Test func constantCodeHasZeroWidth() {
        let r = run(Array(repeating: 77, count: 12))
        #expect(r.peaks.count == 1 && r.unassigned == 0)
        let p = r.peaks[0]
        #expect(p.centre == 77 && p.width == 0 && p.support == 12 && p.fraction == 1)
        #expect(p.binRange == 0...15)  // the only basin spans every bin
    }

    @Test func oneModeUsesRawCodesNotBinCentres() {
        let values = codes([(100, 3), (104, 5), (111, 2), (118, 1)])
        let r = run(values)
        #expect(r.peaks.count == 1 && r.unassigned == 0)
        let (mean, sd) = meanAndPopulationSD(values)
        #expect(close(r.peaks[0].centre, mean) && close(r.peaks[0].width, sd))
        #expect(r.peaks[0].support == values.count)
    }

    @Test func twoModesSplitAtTheValleyAndMatchIndependentStatistics() {
        // 16 bins: bin 2 (count 6), bin 3 (1), bin 4 (1) valley tie, bin 5 (4).
        let left = codes([(33, 2), (40, 4), (50, 1)])
        let right = codes([(64, 1), (82, 3), (90, 1)])
        let values = left + right
        let r = run(values)
        #expect(r.peaks.count == 2 && r.unassigned == 0)
        // Tie between valley bins 3 and 4: leftmost (3) is the valley and
        // belongs to the left basin.
        #expect(r.peaks[0].binRange == 0...3 && r.peaks[1].binRange == 4...15)
        let (lm, ls) = meanAndPopulationSD(left)
        let (rm, rs) = meanAndPopulationSD(right)
        #expect(close(r.peaks[0].centre, lm) && close(r.peaks[0].width, ls))
        #expect(close(r.peaks[1].centre, rm) && close(r.peaks[1].width, rs))
        #expect(r.peaks[0].support == 7 && r.peaks[1].support == 5)
        #expect(close(r.peaks[0].fraction, 7.0 / 12) && close(r.peaks[1].fraction, 5.0 / 12))
    }

    @Test func fourModesRankedBySupport() {
        let values = codes([(10, 3), (70, 9), (130, 5), (200, 7)])
        let r = run(values)
        #expect(r.peaks.map(\.support) == [9, 7, 5, 3] && r.unassigned == 0)
        #expect(r.peaks.map(\.centre) == [70, 200, 130, 10])
        #expect(close(r.peaks.map(\.fraction).reduce(0, +), 1))
    }

    @Test func moreThanFourModesReportsFourAndLeavesTheRestUnassigned() {
        let values = codes([(5, 2), (40, 8), (80, 6), (120, 7), (160, 3), (230, 5)])
        let r = run(values)
        #expect(r.peaks.map(\.support) == [8, 7, 6, 5])
        #expect(r.unassigned == 2 + 3)
        let fractionSum = r.peaks.map(\.fraction).reduce(0, +)
        #expect(close(fractionSum, 26.0 / 31) && fractionSum < 1)
    }

    @Test func fewerPeaksAreNeverPadded() {
        let r = run(codes([(10, 4), (200, 4)]))
        #expect(r.peaks.count == 2)
    }

    @Test func plateauOfEqualAdjacentMaximaIsOnePeak() {
        // Bins 4 and 5 each hold 5 codes; flanks are lower.
        let values = codes([(50, 1), (64, 5), (80, 5), (100, 1)])
        let r = run(values)
        #expect(r.peaks.count == 1 && r.peaks[0].support == 12 && r.unassigned == 0)
        #expect(
            IntensityPeaks.maximumRuns(IntensityHistogram(codes: values, binning: .sixteen).counts)
                == [4...5])
    }

    @Test func plateauRepresentativeIsItsLeftmostBinForTies() {
        // Plateau at bins 1-2 (support 8) versus single max at bin 10 (support 8):
        // equal support, the plateau's leftmost bin 1 < 10 ranks first.
        let values = codes([(16, 4), (32, 4), (160, 8)])
        let r = run(values)
        #expect(r.peaks.map(\.support) == [8, 8])
        #expect(r.peaks[0].centre == 24 && r.peaks[1].centre == 160)
    }

    @Test func equalSupportTieGoesToTheLowerBin() {
        let values = codes([(200, 5), (20, 5)])
        let r = run(values, min: 1)
        #expect(r.peaks.map(\.centre) == [20, 200])
        let one = IntensityPeaks.extract(
            histogram: IntensityHistogram(codes: values, binning: .sixteen), minimumSupport: 1,
            maxPeaks: 1)
        #expect(one.peaks.map(\.centre) == [20] && one.unassigned == 5)
    }

    @Test func sparseNoiseBelowMinimumSupportIsUnassigned() {
        let values = codes([(100, 20), (10, 1), (180, 2), (240, 1)])
        let r = run(values, min: 3)
        #expect(r.peaks.count == 1 && r.peaks[0].support == 20 && r.peaks[0].centre == 100)
        #expect(r.unassigned == 4)
        #expect(close(r.peaks[0].fraction, 20.0 / 24))
    }

    @Test func modesInTheFirstAndLastBins() {
        for b in IntensityBinning.allCases {
            let values = codes([(0, 6), (1, 1), (255, 4), (254, 2)])
            let r = run(values, b)
            #expect(r.peaks.count == 2 && r.unassigned == 0)
            #expect(r.peaks[0].binRange.lowerBound == 0)
            #expect(r.peaks[1].binRange.upperBound == b.binCount - 1)
            let (m0, s0) = meanAndPopulationSD(codes([(0, 6), (1, 1)]))
            let (m1, s1) = meanAndPopulationSD(codes([(255, 4), (254, 2)]))
            #expect(close(r.peaks[0].centre, m0) && close(r.peaks[0].width, s0))
            #expect(close(r.peaks[1].centre, m1) && close(r.peaks[1].width, s1))
        }
    }

    @Test func maxPeaksZeroReportsNothing() {
        let values = codes([(10, 3)])
        let r = IntensityPeaks.extract(
            histogram: IntensityHistogram(codes: values, binning: .sixteen), minimumSupport: 1,
            maxPeaks: 0)
        #expect(r.peaks.isEmpty && r.unassigned == 3)
    }

    @Test func deterministicAndBinningDependent() {
        let values = codes([(3, 2), (21, 5), (29, 4), (90, 3), (97, 6), (180, 2), (250, 7)])
        let shuffled = values.reversed().map { $0 }
        for b in IntensityBinning.allCases {
            let a = run(values, b, min: 2)
            let again = run(values, b, min: 2)
            let reordered = run(shuffled, b, min: 2)
            #expect(a.peaks == again.peaks && a.unassigned == again.unassigned)
            #expect(a.peaks == reordered.peaks && a.unassigned == reordered.unassigned)
            #expect(a.peaks.reduce(0) { $0 + $1.support } + a.unassigned == values.count)
            #expect(!a.peaks.isEmpty && a.peaks.count <= 4)
        }
        // 16 bins: 3 (bin 0) falls into the basin of 21 and 29 (bin 1), support
        // 11. 32 bins: 3 (bin 0) is its own maximum, 21 and 29 form support 9.
        let sixteen = run(values, .sixteen, min: 1)
        let thirtyTwo = run(values, .thirtyTwo, min: 1)
        #expect(sixteen.peaks.first?.support == 11)
        #expect(!thirtyTwo.peaks.contains { $0.support == 11 })
        #expect(thirtyTwo.peaks.contains { $0.support == 9 && $0.binRange == 2...4 })
    }
}

struct IntensityDistributionKeyTests {
    @Test func orderAndDuplicatesDoNotChangeTheKey() {
        let a = IntensityDistributionKey(
            packDigest: "p", sampleID: 3, selectedIndices: [3, 1, 2, 1], excludedIndices: [9, 2],
            intensityAvailable: true, binning: .sixteen, minimumSupport: 3)
        let b = IntensityDistributionKey(
            packDigest: "p", sampleID: 3, selectedIndices: [1, 2, 3], excludedIndices: [2],
            intensityAvailable: true, binning: .sixteen, minimumSupport: 3)
        #expect(a == b && a.hashValue == b.hashValue)
        #expect(a.algorithmVersion == IntensityPeaks.algorithmVersion)
    }

    @Test func everyInputChangesTheKey() {
        func key(
            pack: String = "p", sample: Int = 0, sel: [Int] = [1], exc: [Int] = [],
            avail: Bool = true, bins: IntensityBinning = .sixteen, min: Int = 3,
            version: String = IntensityPeaks.algorithmVersion
        ) -> IntensityDistributionKey {
            IntensityDistributionKey(
                packDigest: pack, sampleID: sample, selectedIndices: sel, excludedIndices: exc,
                intensityAvailable: avail, binning: bins, minimumSupport: min,
                algorithmVersion: version)
        }
        let base = key()
        let variants = [
            key(pack: "q"), key(sample: 1), key(sel: [2]), key(exc: [1]), key(avail: false),
            key(bins: .thirtyTwo), key(min: 4), key(version: "intensity-peaks/v2"),
        ]
        for v in variants { #expect(v != base) }
    }
}
