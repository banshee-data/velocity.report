// LabelSetView.swift
// A version 2 pack as a view of a label set: which window each sample shows,
// where each point falls in it, and the edits that keep what the view hides.
//
// Mirrors internal/lidar/annotation/labelset/view.go. Returns are joined by
// packet key, never by coordinates: a point is the return (packet, block,
// channel) it was decoded from, renumbered through the label set's table.

import Foundation

extension AnnotationPack {
    /// The pack's return layout, which a label set of it must share.
    func labelSetLayout() throws -> ReturnLayout {
        guard hasReturns, let layout = manifest.returnLayout else {
            throw LabelSetError.invalid(
                "pack \(directory.lastPathComponent) carries no returns: it is version \(manifest.schemaVersion)"
            )
        }
        return layout
    }

    /// The foreground extraction this pack holds: the run it was cut from,
    /// with that run's configuration and build. Its id is the pack digest's
    /// first twelve hex digits.
    var labelSetDomain: LabelSetDomain {
        let src = manifest.source
        let digest = manifest.packDigest
        let hex = digest.hasPrefix("sha256:") ? digest.dropFirst(7) : Substring(digest)
        return LabelSetDomain(
            domainID: "fg-" + String(hex.prefix(12)), kind: labelSetDomainForeground,
            runID: goPathBase(src.vrlogPath), runConfigID: src.runConfigID,
            configHash: src.configHash, paramsHash: src.paramsHash, buildVersion: src.buildVersion,
            buildGitSHA: src.buildGitSHA, heightBand: src.heightBand,
            note: "foreground of pack \(digest) (\(manifest.coverage.rawValue))"
        ).normalised
    }
}

/// Go's filepath.Base, so a domain's run id is the same string Go derives:
/// "." for an empty path, trailing slashes dropped, "/" for slashes alone.
func goPathBase(_ path: String) -> String {
    if path.isEmpty { return "." }
    var p = Substring(path)
    while p.hasSuffix("/") { p = p.dropLast() }
    if let slash = p.lastIndex(of: "/") { p = p[p.index(after: slash)...] }
    return p.isEmpty ? "/" : String(p)
}

/// Relates one version 2 pack, a view of one run, to a label set of the same
/// capture.
struct LabelSetViewMap {
    let layout: ReturnLayout
    /// By sample id (dense in a version 2 pack): the window it shows, or -1.
    private(set) var sampleWindow: [Int]
    /// By sample id, by point: the point's offset in its window, or -1 for a
    /// return the label set does not number.
    private(set) var offsets: [[Int64]]
    private let windowSample: [Int: Int]

    /// Joins a pack to a label set state.
    init(layout: ReturnLayout, state: LabelSetState, pack: AnnotationPack) throws {
        let packLayout = try pack.labelSetLayout()
        guard packLayout == layout else {
            throw LabelSetError.invalid(
                "pack layout \(packLayout) differs from label set layout \(layout)")
        }
        self.layout = layout
        var index: [UInt64: Int] = [:]
        index.reserveCapacity(state.packets.count)
        for (i, k) in state.packets.enumerated() { index[k] = i }
        let packets = pack.packets
        func toSet(_ viewReturn: UInt64) -> UInt64? {
            let (vp, block, channel) = layout.locate(viewReturn)
            guard vp < packets.count, let li = index[packets[vp]] else { return nil }
            return try? layout.returnNumber(packet: li, block: block, channel: channel)
        }
        let byStart = state.windows.sorted { $0.start < $1.start }
        func windowOf(_ r: UInt64) -> LabelSetWindow? {
            var lo = 0
            var hi = byStart.count
            while lo < hi {
                let mid = (lo + hi) / 2
                if byStart[mid].end > r { hi = mid } else { lo = mid + 1 }
            }
            return lo < byStart.count && byStart[lo].start <= r ? byStart[lo] : nil
        }

        var sampleWindow = [Int](repeating: -1, count: pack.samples.count)
        var offsets = [[Int64]](repeating: [], count: pack.samples.count)
        for (i, sample) in pack.samples.enumerated() {
            let returns = try pack.returns(sampleID: i)
            var offs = [Int64](repeating: -1, count: returns.count)
            var window = -1
            for (j, vr) in returns.enumerated() {
                guard let r = toSet(vr), let w = windowOf(r) else { continue }
                if window >= 0, w.windowID != window {
                    throw LabelSetError.invalid(
                        "sample \(sample.sampleID) spans windows \(window) and \(w.windowID): this run framed the capture differently"
                    )
                }
                window = w.windowID
                offs[j] = Int64(r - w.start)
            }
            // A frame with no point the set numbers still shows the window
            // its frame range is, if one matches exactly: an empty frame is
            // where "fully occluded" is said. Go's map has no points to go
            // by here and leaves the sample unmapped.
            if window < 0, let start = sample.returnStart, let end = sample.returnEnd, end > start,
                let first = toSet(start), let last = toSet(end - 1), let w = windowOf(first),
                w.start == first, w.end == last + 1
            {
                window = w.windowID
            }
            sampleWindow[i] = window
            offsets[i] = offs
        }
        self.sampleWindow = sampleWindow
        self.offsets = offsets
        var windowSample: [Int: Int] = [:]
        for (i, w) in sampleWindow.enumerated() where w >= 0 && windowSample[w] == nil {
            windowSample[w] = i
        }
        self.windowSample = windowSample
    }

    /// The window a sample shows, if the label set has one.
    func window(forSample sampleID: Int) -> Int? {
        guard sampleID >= 0, sampleID < sampleWindow.count, sampleWindow[sampleID] >= 0 else {
            return nil
        }
        return sampleWindow[sampleID]
    }

    /// The sample that shows a window, if the pack has one.
    func sample(forWindow windowID: Int) -> Int? { windowSample[windowID] }

    /// The members of `set`, a window's offsets, that the sample showing the
    /// window displays, as point indices in point order; and how many it does
    /// not.
    func shown(sampleID: Int, _ set: ReturnRanges) -> (indices: [Int], hidden: UInt64) {
        guard sampleID >= 0, sampleID < offsets.count else { return ([], set.count) }
        var indices: [Int] = []
        if !set.isEmpty {
            for (j, off) in offsets[sampleID].enumerated()
            where off >= 0 && set.contains(UInt64(off)) { indices.append(j) }
        }
        return (indices, set.count - UInt64(indices.count))
    }

    /// Converts point indices of a sample to offsets in its window. An index
    /// whose return the label set does not number is refused.
    func offsets(sampleID: Int, indices: [Int]) throws -> ReturnRanges {
        guard sampleID >= 0, sampleID < offsets.count else {
            throw LabelSetError.invalid("sample \(sampleID) is outside the pack")
        }
        let offs = offsets[sampleID]
        var out: [UInt64] = []
        out.reserveCapacity(indices.count)
        for j in indices {
            guard j >= 0, j < offs.count else {
                throw LabelSetError.invalid("point \(j) is outside sample \(sampleID)")
            }
            guard offs[j] >= 0 else {
                throw LabelSetError.invalid(
                    "sample \(sampleID) point \(j) is a return this label set does not number")
            }
            out.append(UInt64(offs[j]))
        }
        return .of(out)
    }

    /// Every offset the sample displays.
    func shownSet(sampleID: Int) -> ReturnRanges {
        guard sampleID >= 0, sampleID < offsets.count else { return .empty }
        return .of(offsets[sampleID].lazy.filter { $0 >= 0 }.map { UInt64($0) })
    }

    /// Replaces what a set holds among the returns this sample shows, keeping
    /// the members it does not show. Editing through one run's view never
    /// drops a return another run's view labelled.
    func edit(sampleID: Int, _ set: ReturnRanges, shownIndices: [Int]) throws -> ReturnRanges {
        let shown = try offsets(sampleID: sampleID, indices: shownIndices)
        return .union(.difference(set, shownSet(sampleID: sampleID)), shown)
    }

    /// Projects many sets of one sample at once: a sorted offset index is
    /// built once and each set's ranges are looked up in it, so projecting a
    /// whole label set costs its members, not its masks times its points.
    struct SampleIndex {
        fileprivate let pairs: [(offset: UInt64, index: Int)]

        func shown(_ set: ReturnRanges) -> (indices: [Int], hidden: UInt64) {
            var indices: [Int] = []
            for r in set.ranges {
                var lo = 0
                var hi = pairs.count
                while lo < hi {
                    let mid = (lo + hi) / 2
                    if pairs[mid].offset < r.start { lo = mid + 1 } else { hi = mid }
                }
                while lo < pairs.count, pairs[lo].offset < r.end {
                    indices.append(pairs[lo].index)
                    lo += 1
                }
            }
            indices.sort()
            return (indices, set.count - UInt64(indices.count))
        }
    }

    func sampleIndex(_ sampleID: Int) -> SampleIndex {
        guard sampleID >= 0, sampleID < offsets.count else { return SampleIndex(pairs: []) }
        var pairs: [(offset: UInt64, index: Int)] = []
        pairs.reserveCapacity(offsets[sampleID].count)
        for (j, off) in offsets[sampleID].enumerated() where off >= 0 {
            pairs.append((UInt64(off), j))
        }
        pairs.sort { $0.offset < $1.offset }
        return SampleIndex(pairs: pairs)
    }
}

// MARK: - Starting from a pack

extension LabelSetState {
    /// Starts a label set from a version 2 pack: its packet table, one window
    /// per sample spanning the sample's frame, and the pack's foreground as
    /// the first domain.
    static func fromPack(_ pack: AnnotationPack) throws -> (ReturnLayout, LabelSetState) {
        let layout = try pack.labelSetLayout()
        var s = LabelSetState(packets: pack.packets)
        _ = try s.addPackDomain(layout: layout, pack: pack)
        return (layout, s)
    }

    /// Adds a pack's foreground to a label set of the same capture: packets
    /// it lacks are appended to the table, samples the label set has no
    /// window for become windows, and the pack's returns become a domain. A
    /// sample whose frame overlaps an existing window without matching it is
    /// reported (by sample id) and skipped: its run framed the capture
    /// differently. The state is left unchanged when this throws.
    @discardableResult mutating func addPackDomain(
        layout: ReturnLayout, pack: AnnotationPack
    ) throws -> [Int] {
        let packLayout = try pack.labelSetLayout()
        guard packLayout == layout else {
            throw LabelSetError.invalid(
                "pack layout \(packLayout) differs from label set layout \(layout)")
        }
        var s = self
        var index: [UInt64: Int] = [:]
        index.reserveCapacity(s.packets.count + pack.packets.count)
        for (i, k) in s.packets.enumerated() { index[k] = i }
        for k in pack.packets where index[k] == nil {
            index[k] = s.packets.count
            s.packets.append(k)
        }
        let packets = pack.packets
        func toSet(_ vr: UInt64) -> UInt64 {
            let (vp, block, channel) = layout.locate(vr)
            return
                (try? layout.returnNumber(
                    packet: index[packets[vp]]!, block: block, channel: channel)) ?? 0
        }
        let domain = pack.labelSetDomain
        if s.domains.contains(where: { $0.domainID == domain.domainID }) {
            throw LabelSetError.invalid(
                "label set already holds pack \(pack.manifest.packDigest)'s domain")
        }
        s.domains.append(domain)
        var nextID = (s.windows.map(\.windowID).max() ?? -1) + 1
        let per = layout.perPacket
        var skipped: [Int] = []
        for (i, sample) in pack.samples.enumerated() {
            guard let returnStart = sample.returnStart, let returnEnd = sample.returnEnd else {
                skipped.append(i)
                continue
            }
            // A frame is a window only if its packets are consecutive in the
            // label set's table, as they are in the pack's.
            let firstPacket = Int(returnStart / per)
            let base = index[packets[firstPacket]]!
            var consecutive = true
            for vp in firstPacket...Int((returnEnd - 1) / per)
            where index[packets[vp]]! != base + (vp - firstPacket) {
                consecutive = false
                break
            }
            let start = toSet(returnStart)
            let last = toSet(returnEnd - 1)
            if !consecutive || last < start {
                skipped.append(i)
                continue
            }
            var window: LabelSetWindow?
            var clash = false
            for w in s.windows {
                if w.start == start && w.end == last + 1 {
                    window = w
                    break
                }
                if w.start < last + 1 && start < w.end { clash = true }
            }
            if window == nil && clash {
                skipped.append(i)
                continue
            }
            if window == nil {
                let frame = LabelSetWindow(
                    windowID: nextID, start: start, end: last + 1,
                    captureStartNs: sample.timestampNs)
                nextID += 1
                s.windows.append(frame)
                window = frame
            }
            let w = window!
            var offs: [UInt64] = []
            offs.reserveCapacity(sample.pointCount)
            for (j, vr) in try pack.returns(sampleID: i).enumerated() {
                let r = toSet(vr)
                guard r >= w.start, r < w.end else {
                    throw LabelSetError.invalid("sample \(i) point \(j) falls outside its frame")
                }
                offs.append(r - w.start)
            }
            s.domainReturns.append(
                LabelSetDomainReturns(
                    domainID: domain.domainID, windowID: w.windowID, returns: .of(offs)))
        }
        s.canonicalise()
        self = s
        return skipped
    }
}
