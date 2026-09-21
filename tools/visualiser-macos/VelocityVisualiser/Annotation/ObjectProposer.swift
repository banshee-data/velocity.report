// ObjectProposer.swift
// Proposes objects for an operator to grade, so that the unit of their work is
// an object and not one object in one frame.
//
// Measured on a real twenty-second pack: about 43 clusters a frame, 8,600
// object-frames, and six seconds each by hand. The proposer walks the frames
// once. Foreground that sits in the same place in most frames is put aside as
// fixed clutter, one proposal per patch. What is left is clustered on the
// column lattice, and each cluster nobody has claimed seeds a chain that is
// carried from frame to frame by the same footprint fit, under the same stop
// rules, as an operator's own propagation.
//
// It clusters the pack's points itself rather than reading the run's clusters
// and tracks. A recording keeps cluster boxes, not which returns were in them,
// and chaining by the tracker's identities would hand its fragmentation to the
// reference it is meant to be judged against: the run this was sized on starts
// 218 tracks in those twenty seconds.

import Foundation
import simd

/// An object the proposer found, as the frames and returns it would cover.
struct ObjectProposal: Identifiable, Equatable {
    enum Kind: Equatable {
        case moving
        /// In the same place in most frames.
        case fixed
    }

    let id: Int
    let kind: Kind
    /// A starting point for the operator's choice, from size and movement.
    var classGuess: String
    /// Position in the pack's frame order, to canonical indices in that frame.
    var frames: [Int: [Int]]
    /// Metres between where it was first and last seen.
    var travelled: Float
    /// Typical size: along its longer horizontal axis, and in height.
    var length: Float
    var height: Float

    var firstFrame: Int { frames.keys.min() ?? 0 }
    var lastFrame: Int { frames.keys.max() ?? 0 }
    var totalPoints: Int { frames.values.reduce(0) { $0 + $1.count } }
    var meanPoints: Int { frames.isEmpty ? 0 : totalPoints / frames.count }
}

/// What the proposer is shown of one frame.
struct ProposerFrame {
    var points: PackPoints
    /// Display classes: see `PointClass.displayClasses`.
    var classes: [UInt8]
    /// Returns a saved mask already has. Not proposed again.
    var labelled: Set<Int>

    func isFree(_ index: Int) -> Bool {
        FrameCompleteness.isLabelable(classes, index) && !labelled.contains(index)
    }
}

struct ObjectProposer {
    /// The column lattice's pitch: clusters are joined across it.
    static let pitch: Float = 0.5
    /// Occupied in this share of frames, a voxel is fixed clutter.
    static let persistentShare: Float = 0.8
    /// Fewer returns than this is not seeded: it is the speckle every frame has.
    static let minimumSeedPoints = 8
    /// A chain shorter than this was a flicker, not an object.
    static let minimumFrames = 5
    /// A chain may miss this many frames running, behind a pole or another
    /// vehicle, before it is ended.
    static let maximumMisses = 3
    /// A fixed patch needs this many returns a frame to be worth a proposal.
    static let minimumFixedPoints = 8

    /// How far from its prediction a chain is looked for.
    static let searchRadius: Float = 2
    /// Edge of the plan bins a frame's returns are indexed by.
    static let binSize: Float = 2

    static func bin(_ x: Float, _ y: Float) -> SIMD2<Int32> {
        SIMD2(Int32((x / binSize).rounded(.down)), Int32((y / binSize).rounded(.down)))
    }

    // MARK: Fixed clutter

    static func voxel(_ p: simd_float3) -> SIMD3<Int32> {
        SIMD3(
            Int32((p.x / pitch).rounded(.down)), Int32((p.y / pitch).rounded(.down)),
            Int32((p.z / pitch).rounded(.down)))
    }

    /// Counts, per voxel, the frames it held a free return in. Fed one frame
    /// at a time so that a long pack is never held in memory at once.
    struct Persistence {
        private(set) var frames = 0
        private var seen: [SIMD3<Int32>: Int] = [:]

        mutating func add(_ frame: ProposerFrame) {
            frames += 1
            var here = Set<SIMD3<Int32>>()
            for index in 0..<frame.points.count where frame.isFree(index) {
                guard let p = frame.points.point(at: index) else { continue }
                here.insert(ObjectProposer.voxel(p))
            }
            for v in here { seen[v, default: 0] += 1 }
        }

        /// Voxels occupied in at least `persistentShare` of the frames. With
        /// too few frames to say what persists, nothing does.
        var persistent: Set<SIMD3<Int32>> {
            guard frames >= ObjectProposer.minimumFrames else { return [] }
            let needed = Int((Float(frames) * ObjectProposer.persistentShare).rounded(.up))
            return Set(seen.filter { $0.value >= needed }.keys)
        }
    }

    // MARK: Clusters

    /// Free returns outside fixed clutter and not yet claimed, joined across
    /// the lattice in plan. Eight-connected: a car seen corner-on is a
    /// diagonal of cells.
    static func clusters(
        in frame: ProposerFrame, excluding persistent: Set<SIMD3<Int32>>, claimed: Set<Int>
    ) -> [[Int]] {
        var cells: [SIMD2<Int32>: [Int]] = [:]
        for index in 0..<frame.points.count where frame.isFree(index) && !claimed.contains(index) {
            guard let p = frame.points.point(at: index), !persistent.contains(voxel(p)) else {
                continue
            }
            let v = voxel(p)
            cells[SIMD2(v.x, v.y), default: []].append(index)
        }
        var visited = Set<SIMD2<Int32>>()
        var result: [[Int]] = []
        // Sorted, so that the same pack proposes the same objects every time.
        for start in cells.keys.sorted(by: { ($0.x, $0.y) < ($1.x, $1.y) })
        where !visited.contains(start) {
            var stack = [start]
            visited.insert(start)
            var members: [Int] = []
            while let cell = stack.popLast() {
                members += cells[cell] ?? []
                for dx: Int32 in -1...1 {
                    for dy: Int32 in -1...1 {
                        let next = SIMD2(cell.x + dx, cell.y + dy)
                        if cells[next] != nil, !visited.contains(next) {
                            visited.insert(next)
                            stack.append(next)
                        }
                    }
                }
            }
            if members.count >= minimumSeedPoints { result.append(members.sorted()) }
        }
        return result
    }

    // MARK: Chains

    private struct Chain {
        var frames: [Int: [Int]] = [:]
        var footprint: SelectionFootprint
        var velocity = simd_float3.zero
        var speedKnown = false
        var previousCount: Int
        var previousRange: Float
        var misses = 0
        var first: simd_float3
        var last: simd_float3
        var lengths: [Float] = []
        var heights: [Float] = []

        var totalPoints: Int { frames.values.reduce(0) { $0 + $1.count } }
        var firstFrame: Int { frames.keys.min() ?? 0 }

        mutating func record(_ indices: [Int], of points: PackPoints, at frameIndex: Int) {
            let positions = indices.compactMap { points.point(at: $0) }
            guard !positions.isEmpty else { return }
            frames[frameIndex] = indices
            footprint = SelectionFootprint(points: positions)
            previousCount = indices.count
            previousRange = points.meanRange(of: Set(indices))
            var lo = positions[0]
            var hi = positions[0]
            var sum = simd_float3.zero
            for p in positions {
                lo = simd_min(lo, p)
                hi = simd_max(hi, p)
                sum += p
            }
            last = sum / Float(positions.count)
            lengths.append(max(hi.x - lo.x, hi.y - lo.y))
            heights.append(hi.z - lo.z)
        }
    }

    private var chains: [Chain] = []
    private var finished: [Chain] = []
    private let persistent: Set<SIMD3<Int32>>

    init(persistent: Set<SIMD3<Int32>>) { self.persistent = persistent }

    /// Takes the next frame, in order: carries every live chain into it, then
    /// seeds a new chain from each cluster nothing claimed.
    mutating func add(_ frame: ProposerFrame, at frameIndex: Int) {
        // What a chain may take, worked out once for the frame, and binned in
        // plan so that each chain reads the returns round it and not the
        // whole frame.
        var available = [Bool](repeating: false, count: frame.points.count)
        var bins: [SIMD2<Int32>: [Int]] = [:]
        for index in 0..<frame.points.count where frame.isFree(index) {
            guard let p = frame.points.point(at: index),
                !persistent.contains(ObjectProposer.voxel(p))
            else { continue }
            available[index] = true
            bins[ObjectProposer.bin(p.x, p.y), default: []].append(index)
        }
        func near(_ lower: simd_float3, _ upper: simd_float3) -> [Int] {
            let lo = ObjectProposer.bin(lower.x, lower.y)
            let hi = ObjectProposer.bin(upper.x, upper.y)
            guard lo.x <= hi.x, lo.y <= hi.y else { return [] }
            var found: [Int] = []
            for bx in lo.x...hi.x { for by in lo.y...hi.y { found += bins[SIMD2(bx, by)] ?? [] } }
            return found
        }

        var claimed = Set<Int>()
        // Longest first: where two chains want the same returns, the one with
        // the longer history of being an object has the better claim.
        chains.sort { $0.frames.count > $1.frames.count }
        var live: [Chain] = []
        for var chain in chains {
            let include: (Int) -> Bool = { available[$0] && !claimed.contains($0) }
            let prediction = chain.velocity * Float(chain.misses + 1)
            let reach = simd_float3(repeating: ObjectProposer.searchRadius)
            let candidates = near(
                chain.footprint.lower + prediction - reach,
                chain.footprint.upper + prediction + reach)
            let fit = chain.footprint.fit(
                in: frame.points, near: prediction, searchRadius: ObjectProposer.searchRadius,
                among: candidates, where: include)
            let indices = chain.footprint.indices(
                in: frame.points, offset: fit.offset, among: candidates, where: include)
            let range = frame.points.meanRange(of: Set(indices))
            let expected = PropagationJudge.expected(
                previousCount: chain.previousCount, previousRange: chain.previousRange, range: range
            )
            let verdict = PropagationJudge.verdict(
                fit: fit, prediction: prediction, expected: expected, isFixed: false,
                speedKnown: chain.speedKnown)

            switch verdict {
            case nil:
                let step = fit.offset / Float(chain.misses + 1)
                chain.velocity = chain.speedKnown ? chain.velocity * 0.5 + step * 0.5 : step
                chain.speedKnown = true
                chain.misses = 0
                chain.record(indices, of: frame.points, at: frameIndex)
                claimed.formUnion(indices)
                live.append(chain)
            case .lost where chain.misses < ObjectProposer.maximumMisses:
                // Behind something. Kept where it was last seen, and looked
                // for further along next frame.
                chain.misses += 1
                live.append(chain)
            default: finished.append(chain)
            }
        }
        chains = live

        for members in ObjectProposer.clusters(in: frame, excluding: persistent, claimed: claimed) {
            let points = members.compactMap { frame.points.point(at: $0) }
            guard let first = points.first else { continue }
            var chain = Chain(
                footprint: SelectionFootprint(points: points), previousCount: members.count,
                previousRange: frame.points.meanRange(of: Set(members)), first: first, last: first)
            chain.record(members, of: frame.points, at: frameIndex)
            chain.first = chain.last
            chains.append(chain)
        }
    }

    /// The chains that lasted, largest first.
    mutating func finish() -> [ObjectProposal] {
        finished += chains
        chains = []
        let kept = finished.filter { $0.frames.count >= ObjectProposer.minimumFrames }.sorted {
            (a: Chain, b: Chain) in
            a.totalPoints != b.totalPoints
                ? a.totalPoints > b.totalPoints : a.firstFrame < b.firstFrame
        }
        return kept.enumerated().map { id, chain in
            let travelled = simd_distance(
                simd_float2(chain.first.x, chain.first.y), simd_float2(chain.last.x, chain.last.y))
            let length = ObjectProposer.median(chain.lengths)
            let height = ObjectProposer.median(chain.heights)
            return ObjectProposal(
                id: id, kind: .moving,
                classGuess: ObjectProposer.guess(
                    travelled: travelled, length: length, height: height), frames: chain.frames,
                travelled: travelled, length: length, height: height)
        }
    }

    // MARK: Fixed proposals

    /// Gathers, frame by frame, the free returns inside each connected patch
    /// of fixed clutter: one proposal per patch, covering every frame.
    struct FixedPatches {
        private var patchOf: [SIMD3<Int32>: Int] = [:]
        private var members: [[Int: [Int]]] = []
        private var lower: [simd_float3] = []
        private var upper: [simd_float3] = []

        /// Patches are persistent voxels joined through faces, edges and
        /// corners.
        init(persistent: Set<SIMD3<Int32>>) {
            var count = 0
            for start in persistent.sorted(by: { ($0.x, $0.y, $0.z) < ($1.x, $1.y, $1.z) })
            where patchOf[start] == nil {
                var stack = [start]
                patchOf[start] = count
                while let v = stack.popLast() {
                    for dx: Int32 in -1...1 {
                        for dy: Int32 in -1...1 {
                            for dz: Int32 in -1...1 {
                                let next = SIMD3(v.x + dx, v.y + dy, v.z + dz)
                                if persistent.contains(next), patchOf[next] == nil {
                                    patchOf[next] = count
                                    stack.append(next)
                                }
                            }
                        }
                    }
                }
                count += 1
            }
            members = [[Int: [Int]]](repeating: [:], count: count)
            lower = [simd_float3](repeating: simd_float3(repeating: .infinity), count: count)
            upper = [simd_float3](repeating: simd_float3(repeating: -.infinity), count: count)
        }

        mutating func add(_ frame: ProposerFrame, at frameIndex: Int) {
            for index in 0..<frame.points.count where frame.isFree(index) {
                guard let p = frame.points.point(at: index),
                    let patch = patchOf[ObjectProposer.voxel(p)]
                else { continue }
                members[patch][frameIndex, default: []].append(index)
                lower[patch] = simd_min(lower[patch], p)
                upper[patch] = simd_max(upper[patch], p)
            }
        }

        func finish(firstID: Int, bandFloor: Float) -> [ObjectProposal] {
            var found: [ObjectProposal] = []
            for patch in members.indices {
                let total = members[patch].values.reduce(0) { $0 + $1.count }
                guard !members[patch].isEmpty,
                    total / members[patch].count >= ObjectProposer.minimumFixedPoints
                else { continue }
                let size = upper[patch] - lower[patch]
                let length = max(size.x, size.y)
                found.append(
                    ObjectProposal(
                        id: 0, kind: .fixed,
                        classGuess: ObjectProposer.guessFixed(
                            length: length, height: size.z, top: upper[patch].z,
                            bandFloor: bandFloor), frames: members[patch], travelled: 0,
                        length: length, height: size.z))
            }
            // Largest first, then numbered, so the ids follow the list.
            found.sort { $0.totalPoints > $1.totalPoints }
            return found.enumerated().map { offset, proposal in
                ObjectProposal(
                    id: firstID + offset, kind: proposal.kind, classGuess: proposal.classGuess,
                    frames: proposal.frames, travelled: proposal.travelled, length: proposal.length,
                    height: proposal.height)
            }
        }
    }

    // MARK: Guesses

    /// A first guess at what a moving chain is. It is a guess: a partial view
    /// of a bus is the length of a car, and the operator's choice is what is
    /// saved.
    static func guess(travelled: Float, length: Float, height: Float) -> String {
        if travelled < 2 { return "noise" }
        if length < 1.2 { return "pedestrian" }
        if length < 2.5 { return "cyclist" }
        if length < 6.5 { return "car" }
        return "bus"
    }

    static func guessFixed(length: Float, height: Float, top: Float, bandFloor: Float) -> String {
        // Hugging the bottom of the height band: road the floor was set a
        // little too low to remove.
        if top - bandFloor < 0.6 { return "ground" }
        if length >= 3, height >= 1.5 { return "building" }
        if length < 1.2, height >= 1.5 { return "sign" }
        return "noise"
    }

    static func median(_ values: [Float]) -> Float {
        guard !values.isEmpty else { return 0 }
        return values.sorted()[values.count / 2]
    }
}
