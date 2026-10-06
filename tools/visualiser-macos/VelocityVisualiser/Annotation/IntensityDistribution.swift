// IntensityDistribution.swift
// Histogram and experimental peak summary of raw intensity codes for a
// selected point population (plan 5.8, "Histogram scope", "Up to four peaks",
// "Availability and interpretation").
//
// Pure and deterministic: no UI, no I/O, no state. The same input always
// yields the same histogram and the same peaks. Codes are the sensor's raw
// 0-255 intensity, not calibrated reflectance. Display range, gamma and
// brightness never reach this file: they cannot change a population, a bin
// edge or a peak.

import Foundation

// MARK: - Binning

/// Fixed bin edges over the 256 integer codes. Sixteen bins are 16 codes
/// wide; thirty-two bins are 8 codes wide. The edges tile 0...255 exactly, so
/// the last bin includes 255 and no code falls outside a bin.
enum IntensityBinning: Int, CaseIterable, Hashable {
    case sixteen = 16
    case thirtyTwo = 32

    var binCount: Int { rawValue }

    /// Codes per bin: 16 or 8.
    var binWidth: Int { 256 / rawValue }

    /// The bin holding `code`. Bin `b` holds codes `b*width ... b*width+width-1`.
    func bin(of code: UInt8) -> Int { Int(code) / binWidth }

    /// Inclusive raw-code range of bin `b`. `b` must be in `0..<binCount`.
    func range(ofBin b: Int) -> ClosedRange<Int> {
        precondition(b >= 0 && b < binCount, "bin \(b) outside 0..<\(binCount)")
        let lower = b * binWidth
        return lower...(lower + binWidth - 1)
    }
}

// MARK: - Errors

/// A malformed selection or source array. These are data errors: the
/// offending values are never truncated, wrapped or silently dropped.
enum IntensityDistributionError: Error, Equatable {
    /// `count` distinct selected indices lay outside `0..<pointCount`;
    /// `smallest` is the lowest such index, for the log.
    case indicesOutOfRange(count: Int, smallest: Int, pointCount: Int)
    /// Intensity was declared present but its array does not cover the points.
    case intensityLengthMismatch(intensityCount: Int, pointCount: Int)
}

// MARK: - Histogram

/// Counts of measured codes per bin. `fractions` share one denominator,
/// `measuredCount`, and are `nil` when nothing was measured: an empty
/// population has no fractions rather than fractions of zero.
struct IntensityHistogram: Equatable {
    let binning: IntensityBinning
    /// One count per bin; sums to `measuredCount`.
    let counts: [Int]
    /// Points whose code was counted.
    let measuredCount: Int
    /// Distinct selected points left out because the caller excluded them
    /// (uncertain mask members). Disclosed, never counted in a bin.
    let excludedCount: Int
    /// Measured codes in ascending point-index order (or caller order for
    /// `init(codes:binning:)`). Kept so peaks can use the raw values.
    let measuredCodes: [UInt8]

    /// `counts[b] / measuredCount`, or `nil` for an empty population.
    var fractions: [Double]? {
        guard measuredCount > 0 else { return nil }
        let total = Double(measuredCount)
        return counts.map { Double($0) / total }
    }

    var isEmpty: Bool { measuredCount == 0 }

    /// Histogram of an already-filtered measured population. Every code is
    /// counted once, including duplicates in `codes`: the caller owns
    /// de-duplication on this path.
    init(codes: [UInt8], binning: IntensityBinning, excludedCount: Int = 0) {
        var counts = [Int](repeating: 0, count: binning.binCount)
        for code in codes { counts[binning.bin(of: code)] += 1 }
        self.binning = binning
        self.counts = counts
        self.measuredCount = codes.count
        self.excludedCount = excludedCount
        self.measuredCodes = codes
    }

    /// Histogram of selected point indices into a pack sample's intensity
    /// array. Repeated indices count once. Selected indices in `excluded`
    /// are counted in `excludedCount` only. Any index outside the intensity
    /// array throws: a malformed index is a data error, not a point to drop.
    /// Availability is not decided here; see `IntensityPopulation`.
    init(
        intensity: [UInt8], indices: some Sequence<Int>, excluded: Set<Int>,
        binning: IntensityBinning
    ) throws {
        let distinct = try IntensityHistogram.distinctValidated(
            indices, pointCount: intensity.count)
        var codes: [UInt8] = []
        codes.reserveCapacity(distinct.count)
        var excludedCount = 0
        for i in distinct {
            if excluded.contains(i) { excludedCount += 1 } else { codes.append(intensity[i]) }
        }
        self.init(codes: codes, binning: binning, excludedCount: excludedCount)
    }

    /// Sorted distinct indices, or a throw naming every out-of-range index.
    static func distinctValidated(_ indices: some Sequence<Int>, pointCount: Int) throws -> [Int] {
        let distinct = Set(indices).sorted()
        let bad = distinct.filter { $0 < 0 || $0 >= pointCount }
        if let smallest = bad.first {
            throw IntensityDistributionError.indicesOutOfRange(
                count: bad.count, smallest: smallest, pointCount: pointCount)
        }
        return distinct
    }
}

// MARK: - Population

/// What the histogram describes. Three states that must never be confused:
/// nothing selected, a selection whose intensity is not available, and a
/// measured population (which may itself be empty if every selected point
/// was excluded).
enum IntensityPopulation: Equatable {
    /// No point is selected.
    case emptySelection
    /// Points are selected but the source carries no intensity. Summaries
    /// are disabled; no zero-valued histogram is produced.
    /// `selectedCount` is the distinct selected point count.
    case unavailable(selectedCount: Int)
    /// Intensity is present; the histogram of the non-excluded points.
    case measured(IntensityHistogram)

    /// Builds the population. `intensityAvailable` must come from explicit
    /// source/pack presence (e.g. `has_intensity`), never from the bytes:
    /// with presence known, code 0 is a measurement. Indices are validated
    /// against `pointCount` in every state, and when intensity is present
    /// its array must be exactly `pointCount` long.
    init(
        intensityAvailable: Bool, intensity: [UInt8], pointCount: Int, indices: some Sequence<Int>,
        excluded: Set<Int>, binning: IntensityBinning
    ) throws {
        let distinct = try IntensityHistogram.distinctValidated(indices, pointCount: pointCount)
        if distinct.isEmpty {
            self = .emptySelection
            return
        }
        guard intensityAvailable else {
            self = .unavailable(selectedCount: distinct.count)
            return
        }
        guard intensity.count == pointCount else {
            throw IntensityDistributionError.intensityLengthMismatch(
                intensityCount: intensity.count, pointCount: pointCount)
        }
        self = .measured(
            try IntensityHistogram(
                intensity: intensity, indices: distinct, excluded: excluded, binning: binning))
    }

    var histogram: IntensityHistogram? {
        if case .measured(let h) = self { return h }
        return nil
    }
}

// MARK: - Peaks (experimental)

/// One derived peak. Units are raw intensity codes.
struct IntensityPeak: Equatable {
    /// Mean of the raw codes assigned to this basin.
    let centre: Double
    /// Population standard deviation of those raw codes. Zero is valid.
    let width: Double
    /// Number of measured points assigned to this basin.
    let support: Int
    /// `support / measuredCount` over the whole measured population.
    let fraction: Double
    /// Inclusive bin-index range of the basin.
    let binRange: ClosedRange<Int>
}

/// Experimental peak extraction over a fixed-bin histogram. The result
/// depends on the binning; it is not a binning-independent model and does
/// not identify a surface or material.
///
/// Rule `intensity-peaks/v1`, applied to bin counts `c[0..<n]`:
///
/// 1. Empty population: zero peaks, zero unassigned.
/// 2. Maxima. Split the bins into maximal runs of equal count. A run
///    `s...e` with count `v > 0` is a maximum when `v` is strictly greater
///    than both flanking bins, a flank beyond either end counting as 0. A
///    single bin is a run of length one; a longer run is a plateau and
///    counts as ONE maximum whose representative is its leftmost bin `s`.
/// 3. Basins. Between consecutive maxima (run ending at `e_k`, next run
///    starting at `s_k+1`) the valley is the bin of minimum count in
///    `e_k+1 ... s_k+1 - 1`; among equal minima the leftmost is the valley.
///    The valley bin belongs to the LEFT basin. The first basin starts at
///    bin 0 and the last ends at bin `n-1`, so every bin lies in exactly one
///    basin and every plateau lies wholly inside its own basin.
/// 4. Support of a basin is the number of measured codes whose bin lies in
///    it. Basins with support below `minimumSupport` (clamped to at least 1)
///    are not eligible.
/// 5. Eligible basins are ranked by support descending; equal support is
///    broken by the lower representative bin. At most `maxPeaks` are
///    reported, in rank order. Fewer is normal; four is never forced.
/// 6. `unassigned` is the measured count minus the reported support: codes
///    in ineligible basins and in basins beyond `maxPeaks`.
/// 7. Centre and width are the mean and population standard deviation of
///    the raw codes in the basin, not of bin centres. Fractions use the
///    full measured count, so reported fractions need not sum to one.
enum IntensityPeaks {
    static let algorithmVersion = "intensity-peaks/v1"
    static let defaultMaxPeaks = 4

    /// Peaks using the histogram's own measured codes.
    static func extract(
        histogram: IntensityHistogram, minimumSupport: Int, maxPeaks: Int = defaultMaxPeaks
    ) -> (peaks: [IntensityPeak], unassigned: Int) {
        extract(
            histogram: histogram, codes: histogram.measuredCodes, minimumSupport: minimumSupport,
            maxPeaks: maxPeaks)
    }

    /// Peaks from `histogram` and the raw `codes` it was built from. `codes`
    /// must be exactly the histogram's measured population; a mismatch is a
    /// programming error and traps rather than yielding inconsistent support.
    static func extract(
        histogram: IntensityHistogram, codes: [UInt8], minimumSupport: Int,
        maxPeaks: Int = defaultMaxPeaks
    ) -> (peaks: [IntensityPeak], unassigned: Int) {
        let binning = histogram.binning
        let counts = histogram.counts
        precondition(
            IntensityHistogram(codes: codes, binning: binning).counts == counts
                && codes.count == histogram.measuredCount,
            "codes do not match the histogram's measured population")
        let measured = histogram.measuredCount
        guard measured > 0 else { return ([], 0) }

        // Per-code tallies make centre and width independent of code order.
        var perCode = [Int](repeating: 0, count: 256)
        for code in codes { perCode[Int(code)] += 1 }

        let maxima = maximumRuns(counts)
        guard !maxima.isEmpty else { return ([], measured) }  // unreachable when measured > 0

        // Basin boundaries.
        var basins: [(range: ClosedRange<Int>, representative: Int)] = []
        var start = 0
        for k in 0..<maxima.count {
            let end: Int
            if k + 1 < maxima.count {
                let gapLower = maxima[k].upperBound + 1
                let gapUpper = maxima[k + 1].lowerBound - 1
                var valley = gapLower
                for b in gapLower...gapUpper where counts[b] < counts[valley] { valley = b }
                end = valley
            } else {
                end = counts.count - 1
            }
            basins.append((start...end, maxima[k].lowerBound))
            start = end + 1
        }

        let minimum = max(1, minimumSupport)
        var candidates: [(support: Int, representative: Int, range: ClosedRange<Int>)] = []
        for basin in basins {
            let support = basin.range.reduce(0) { $0 + counts[$1] }
            if support >= minimum {
                candidates.append((support, basin.representative, basin.range))
            }
        }
        candidates.sort {
            $0.support != $1.support
                ? $0.support > $1.support : $0.representative < $1.representative
        }
        let reported = candidates.prefix(max(0, maxPeaks))

        var peaks: [IntensityPeak] = []
        var assigned = 0
        for c in reported {
            let codeLower = binning.range(ofBin: c.range.lowerBound).lowerBound
            let codeUpper = binning.range(ofBin: c.range.upperBound).upperBound
            var n = 0
            var sum = 0
            for code in codeLower...codeUpper {
                n += perCode[code]
                sum += perCode[code] * code
            }
            let mean = Double(sum) / Double(n)
            var squares = 0.0
            for code in codeLower...codeUpper where perCode[code] > 0 {
                let d = Double(code) - mean
                squares += Double(perCode[code]) * d * d
            }
            peaks.append(
                IntensityPeak(
                    centre: mean, width: (squares / Double(n)).squareRoot(), support: c.support,
                    fraction: Double(c.support) / Double(measured), binRange: c.range))
            assigned += c.support
        }
        return (peaks, measured - assigned)
    }

    /// Maximal equal-count runs that are strict maxima (step 2), left to right.
    static func maximumRuns(_ counts: [Int]) -> [ClosedRange<Int>] {
        var runs: [ClosedRange<Int>] = []
        var s = 0
        while s < counts.count {
            var e = s
            while e + 1 < counts.count && counts[e + 1] == counts[s] { e += 1 }
            let v = counts[s]
            let left = s > 0 ? counts[s - 1] : 0
            let right = e + 1 < counts.count ? counts[e + 1] : 0
            if v > 0 && v > left && v > right { runs.append(s...e) }
            s = e + 1
        }
        return runs
    }
}

// MARK: - Cache key

/// Identifies one histogram/peak computation so an asynchronous result can
/// be discarded when the pack, sample, selection, availability, binning or
/// rule has changed since it was requested. Indices are stored sorted and
/// de-duplicated so equal selections compare equal regardless of order.
struct IntensityDistributionKey: Hashable {
    let packDigest: String
    let sampleID: Int
    let selectedIndices: [Int]
    let excludedIndices: [Int]
    let intensityAvailable: Bool
    let binning: IntensityBinning
    let minimumSupport: Int
    let algorithmVersion: String

    init(
        packDigest: String, sampleID: Int, selectedIndices: some Sequence<Int>,
        excludedIndices: some Sequence<Int> = [Int](), intensityAvailable: Bool,
        binning: IntensityBinning, minimumSupport: Int,
        algorithmVersion: String = IntensityPeaks.algorithmVersion
    ) {
        let selected = Set(selectedIndices)
        self.packDigest = packDigest
        self.sampleID = sampleID
        self.selectedIndices = selected.sorted()
        // Only exclusions that touch the selection change the result.
        self.excludedIndices = Set(excludedIndices).intersection(selected).sorted()
        self.intensityAvailable = intensityAvailable
        self.binning = binning
        self.minimumSupport = minimumSupport
        self.algorithmVersion = algorithmVersion
    }
}
