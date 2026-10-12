// LabelSetRanges.swift
// Sets of return numbers, written [[start, length], …].
//
// Mirrors internal/lidar/annotation/labelset/ranges.go. The canonical form is
// sorted, every range non-empty, and each range starting after the previous
// one's end plus one, so one set has exactly one encoding and the state digest
// over it is unique.

import Foundation

/// A run of consecutive return numbers.
struct ReturnRange: Equatable, Hashable {
    var start: UInt64
    var length: UInt64

    /// One past the last member. Callers hold validated ranges, which cannot
    /// overflow; decoding refuses one that would.
    var end: UInt64 { start + length }
}

/// A set of return numbers, or of offsets within a window.
struct ReturnRanges: Equatable, Hashable {
    private(set) var ranges: [ReturnRange]

    init(_ ranges: [ReturnRange] = []) { self.ranges = ranges }

    static let empty = ReturnRanges()

    /// The canonical set holding the given members, in any order and with
    /// repeats.
    static func of<S: Sequence>(_ members: S) -> ReturnRanges where S.Element == UInt64 {
        var sorted = Array(members)
        guard !sorted.isEmpty else { return ReturnRanges() }
        sorted.sort()
        var out = [ReturnRange(start: sorted[0], length: 1)]
        for m in sorted.dropFirst() {
            let last = out[out.count - 1]
            if m < last.end { continue }
            if m == last.end {
                out[out.count - 1].length += 1
            } else {
                out.append(ReturnRange(start: m, length: 1))
            }
        }
        return ReturnRanges(out)
    }

    var isEmpty: Bool { ranges.isEmpty }

    /// Refuses a set that is not in canonical form.
    func validate() throws {
        for (i, r) in ranges.enumerated() {
            if r.length == 0 { throw LabelSetCodingError("range \(i) is empty") }
            if r.start.addingReportingOverflow(r.length).overflow {
                throw LabelSetCodingError("range \(i) overflows")
            }
            if i > 0, r.start <= ranges[i - 1].end {
                throw LabelSetCodingError(
                    "range \(i) starts at \(r.start), not after the previous range's end \(ranges[i - 1].end)"
                )
            }
        }
    }

    /// The number of members.
    var count: UInt64 { ranges.reduce(0) { $0 + $1.length } }

    /// Whether `m` is a member.
    func contains(_ m: UInt64) -> Bool {
        var lo = 0
        var hi = ranges.count
        while lo < hi {
            let mid = (lo + hi) / 2
            if ranges[mid].end > m { hi = mid } else { lo = mid + 1 }
        }
        return lo < ranges.count && ranges[lo].start <= m
    }

    /// Every member, in order.
    var members: [UInt64] {
        var out: [UInt64] = []
        out.reserveCapacity(Int(clamping: count))
        for r in ranges {
            var m = r.start
            while m < r.end {
                out.append(m)
                m += 1
            }
        }
        return out
    }

    /// One past the largest member, or 0 for an empty set.
    var max: UInt64 { ranges.last?.end ?? 0 }

    // The shortcuts below give what the sweep would, for the cases a save's
    // validation meets thousands of times: an empty set, sets that do not
    // meet, and a mask inside its domain's one range.

    /// The members of either set.
    static func union(_ a: ReturnRanges, _ b: ReturnRanges) -> ReturnRanges {
        if a.isEmpty { return b }
        if b.isEmpty { return a }
        return merge(a, b) { $0 || $1 }
    }

    /// The members of both sets.
    static func intersect(_ a: ReturnRanges, _ b: ReturnRanges) -> ReturnRanges {
        if a.isEmpty || b.isEmpty || a.max <= b.ranges[0].start || b.max <= a.ranges[0].start {
            return ReturnRanges()
        }
        return merge(a, b) { $0 && $1 }
    }

    /// The members of `a` not in `b`.
    static func difference(_ a: ReturnRanges, _ b: ReturnRanges) -> ReturnRanges {
        if a.isEmpty { return ReturnRanges() }
        if b.isEmpty || a.max <= b.ranges[0].start || b.max <= a.ranges[0].start { return a }
        if let covering = b.ranges.first(where: { $0.end >= a.ranges[0].start }),
            covering.start <= a.ranges[0].start, covering.end >= a.max
        {
            return ReturnRanges()
        }
        return merge(a, b) { $0 && !$1 }
    }

    /// Sweeps the boundaries of both sets and keeps the stretches where
    /// `keep` holds. Both inputs must be canonical; the result is.
    ///
    /// Go collects every boundary and sorts them. Each set's boundaries are
    /// already in order, so here the two lists are merged as they are walked:
    /// the same cuts in the same order, without the sort, which matters when
    /// a save validates thousands of masks.
    private static func merge(
        _ a: ReturnRanges, _ b: ReturnRanges, keep: (Bool, Bool) -> Bool
    ) -> ReturnRanges {
        if a.ranges.isEmpty && b.ranges.isEmpty { return ReturnRanges() }
        var cuts: [UInt64] = []
        cuts.reserveCapacity(2 * (a.ranges.count + b.ranges.count))
        var i = 0
        var j = 0
        let na = 2 * a.ranges.count
        let nb = 2 * b.ranges.count
        func cut(_ rs: [ReturnRange], _ k: Int) -> UInt64 {
            k % 2 == 0 ? rs[k / 2].start : rs[k / 2].end
        }
        while i < na || j < nb {
            if j >= nb || (i < na && cut(a.ranges, i) <= cut(b.ranges, j)) {
                cuts.append(cut(a.ranges, i))
                i += 1
            } else {
                cuts.append(cut(b.ranges, j))
                j += 1
            }
        }
        var out: [ReturnRange] = []
        var ia = 0
        var ib = 0
        for i in 0..<(cuts.count - 1) {
            let lo = cuts[i]
            let hi = cuts[i + 1]
            if lo == hi { continue }
            while ia < a.ranges.count, a.ranges[ia].end <= lo { ia += 1 }
            while ib < b.ranges.count, b.ranges[ib].end <= lo { ib += 1 }
            let inA = ia < a.ranges.count && a.ranges[ia].start <= lo
            let inB = ib < b.ranges.count && b.ranges[ib].start <= lo
            if !keep(inA, inB) { continue }
            if let last = out.last, last.end == lo {
                out[out.count - 1].length += hi - lo
            } else {
                out.append(ReturnRange(start: lo, length: hi - lo))
            }
        }
        return ReturnRanges(out)
    }

    // MARK: Encoding

    /// `[[start,length],…]`, rendered straight to canonical bytes: a label
    /// set's masks are mostly these, and building a value per number first
    /// would cost more than the rest of the encoding.
    var json: LabelSetJSON {
        var out: [UInt8] = [0x5B]
        out.reserveCapacity(2 + ranges.count * 12)
        for (i, r) in ranges.enumerated() {
            if i > 0 { out.append(0x2C) }
            out.append(0x5B)
            out.append(contentsOf: String(r.start).utf8)
            out.append(0x2C)
            out.append(contentsOf: String(r.length).utf8)
            out.append(0x5D)
        }
        out.append(0x5D)
        return .canonicalBytes(out)
    }

    /// Reads `[[start, length], …]`, refusing a set that is not canonical.
    /// Go's reader accepts one and lets validation or the digest refuse it
    /// later; refusing here is the same outcome, earlier, and keeps a
    /// malformed range from ever reaching arithmetic that could overflow.
    static func decode(_ value: LabelSetJSON?, _ context: String) throws -> ReturnRanges {
        guard let value, value != .null else { return ReturnRanges() }
        guard case .array(let items) = value else {
            throw LabelSetCodingError("\(context): expected [[start, length], …]")
        }
        var out: [ReturnRange] = []
        out.reserveCapacity(items.count)
        for item in items {
            guard case .array(let pair) = item, pair.count == 2, case .number(let s) = pair[0],
                case .number(let l) = pair[1], let start = UInt64(s), let length = UInt64(l)
            else { throw LabelSetCodingError("\(context): expected [start, length]") }
            out.append(ReturnRange(start: start, length: length))
        }
        let set = ReturnRanges(out)
        do { try set.validate() } catch let error as LabelSetCodingError {
            throw LabelSetCodingError("\(context): \(error.description)")
        }
        return set
    }
}
