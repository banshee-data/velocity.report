// RoadSurface.swift
// The road surface preview of Gerardus (REFERENCE_LAYER_ROAD_SURFACE, 17):
// the Swift model of its patches and the store that gathers its chunks.
//
// The contract is a prototype (docs/plans/GGP-ROAD-PROTOTYPE.md in
// banshee-data/gerardus). Its support classes and boundary provenance are
// meant to last and are kept whole here: nothing in this file turns a
// MISSING vertex into a height, and nothing drops a patch's weakest edge
// source. The 0.5 m grid and the mesh payload are scaffolding.
//
// Heights. A MISSING vertex has z NaN and a NaN normal, and no triangle uses
// it. `RoadPatch.height(at:)` and every profile reader answer nil there; no
// code path carries a previous height across one.

import Foundation
import simd

// MARK: - Support and provenance

/// Where a vertex's height comes from (RoadSupport).
enum RoadSupportClass: Int, Sendable, CaseIterable {
    case unspecified = 0
    case observed = 1
    case interpolated = 2
    case missing = 3

    init(_ proto: Gerardus_Align_V1alpha1_RoadSupport) {
        switch proto {
        case .observed: self = .observed
        case .interpolated: self = .interpolated
        case .missing: self = .missing
        default: self = .unspecified
        }
    }

    var label: String {
        switch self {
        case .observed: return "observed"
        case .interpolated: return "interpolated"
        case .missing: return "missing"
        case .unspecified: return "unspecified"
        }
    }
}

/// Where a stretch of boundary comes from (RoadBoundarySource), strongest
/// first. `isInferred` marks the one a reviewer must not mistake for a
/// survey: a hand-authored width about the City centreline.
enum RoadBoundarySourceClass: Int, Sendable, CaseIterable, Comparable {
    case kerbPair = 1
    case cityKerb = 2
    case inferredWidth = 3
    case authored = 4
    case unspecified = 0

    init(_ proto: Gerardus_Align_V1alpha1_RoadBoundarySource) {
        switch proto {
        case .kerbPair: self = .kerbPair
        case .cityKerb: self = .cityKerb
        case .inferredWidth: self = .inferredWidth
        case .authored: self = .authored
        default: self = .unspecified
        }
    }

    static func < (a: Self, b: Self) -> Bool { a.rawValue < b.rawValue }

    var isInferred: Bool { self == .inferredWidth }

    var label: String {
        switch self {
        case .kerbPair: return "kerb pair"
        case .cityKerb: return "City kerb (one side)"
        case .inferredWidth: return "inferred width"
        case .authored: return "authored cut"
        case .unspecified: return "unspecified"
        }
    }
}

enum RoadPatchKindClass: Sendable { case segment, junction, unspecified }

// MARK: - Patch parts

struct RoadRing: Sendable, Equatable {
    /// Closed ring, closing vertex not repeated. z is NaN where the boundary
    /// has no height.
    var points: [SIMD3<Float>]
    /// One per edge: edge i runs from point i to point i + 1.
    var edgeSource: [RoadBoundarySourceClass]
    var hole: Bool
    var sourceFeatureID: String
}

struct RoadProfileStation: Sendable, Equatable {
    var s: Float
    /// Site frame; z is NaN where the surface is MISSING.
    var position: SIMD3<Float>
    /// dz/ds; NaN where unsupported.
    var grade: Float
    var support: RoadSupportClass

    /// A station with no height: the driving camera stops here.
    var isMissing: Bool { support == .missing || !position.z.isFinite }
}

struct RoadAssessment: Sendable, Equatable {
    var component: String
    var assessed: Bool
    var source: String
    var observed: String
}

struct RoadPatchStats: Sendable, Equatable {
    var areaM2: Float = 0
    var observedAreaM2: Float = 0
    var interpolatedAreaM2: Float = 0
    var missingAreaM2: Float = 0
    var groundPoints: Int = 0
    var heldOutPoints: Int = 0
    /// NaN with no held-out points.
    var residualMedianM: Float = .nan
    var residualRmsM: Float = .nan
    var residualP95AbsM: Float = .nan
    var riderLineObserved: Float = 0
    var riderLineInterpolated: Float = 0
    var riderLineMissing: Float = 0

    // NaN != NaN would make equal stats unequal.
    static func == (a: Self, b: Self) -> Bool {
        func same(_ x: Float, _ y: Float) -> Bool { x == y || (x.isNaN && y.isNaN) }
        return same(a.areaM2, b.areaM2) && same(a.observedAreaM2, b.observedAreaM2)
            && same(a.interpolatedAreaM2, b.interpolatedAreaM2)
            && same(a.missingAreaM2, b.missingAreaM2) && a.groundPoints == b.groundPoints
            && a.heldOutPoints == b.heldOutPoints && same(a.residualMedianM, b.residualMedianM)
            && same(a.residualRmsM, b.residualRmsM) && same(a.residualP95AbsM, b.residualP95AbsM)
            && same(a.riderLineObserved, b.riderLineObserved)
            && same(a.riderLineInterpolated, b.riderLineInterpolated)
            && same(a.riderLineMissing, b.riderLineMissing)
    }
}

// MARK: - Patch

/// One patch of road surface in a site frame (or, after `translated`, in the
/// frame of the scene that holds it).
struct RoadPatch: Sendable, Identifiable, Equatable {
    var id: String { patchID }
    var patchID: String
    var kind: RoadPatchKindClass
    var featureID: String
    var cnn: String
    var name: String
    var fromNode: String
    var toNode: String
    var sourceVersion: String
    /// ISO date of the ground observation.
    var observed: String
    var vertices: [SIMD3<Float>]
    var normals: [SIMD3<Float>]
    var indices: [UInt32]
    var support: [RoadSupportClass]
    var rings: [RoadRing]
    var boundarySource: RoadBoundarySourceClass
    var profile: [RoadProfileStation]
    var assessments: [RoadAssessment]
    var stats: RoadPatchStats
    var geometryHash: String
    var supportHash: String
    /// The `l16_token` of the site frame the coordinates are in.
    var frameToken: String = ""

    var triangleCount: Int { indices.count / 3 }

    func count(of class_: RoadSupportClass) -> Int { support.filter { $0 == class_ }.count }

    /// Why the patch cannot be drawn as sent, or nil. A corrupt patch is kept
    /// out of the scene and reported, never drawn partly.
    func defect() -> String? {
        if vertices.count != normals.count || vertices.count != support.count {
            return "vertex, normal and support counts differ"
        }
        if indices.count % 3 != 0 { return "index count is not a multiple of 3" }
        let n = UInt32(vertices.count)
        for index in indices where index >= n { return "index \(index) is outside the patch" }
        for t in 0..<triangleCount {
            for k in 0..<3 {
                let v = Int(indices[3 * t + k])
                if support[v] == .missing || !vertices[v].z.isFinite {
                    return "triangle \(t) uses a missing vertex"
                }
            }
        }
        return nil
    }

    /// The same patch moved by `offset` (a site-frame to site-frame shift).
    /// NaN heights stay NaN.
    func translated(by offset: SIMD3<Float>, frameToken: String) -> RoadPatch {
        var out = self
        out.vertices = vertices.map { $0 + offset }
        out.rings = rings.map {
            var r = $0
            r.points = r.points.map { $0 + offset }
            return r
        }
        out.profile = profile.map {
            var s = $0
            s.position += offset
            return s
        }
        out.frameToken = frameToken
        return out
    }

    /// Horizontal bounds of the patch: its vertices, or its rings once the mesh
    /// has been evicted.
    var bounds: (min: SIMD2<Float>, max: SIMD2<Float>)? {
        var lo = SIMD2<Float>(repeating: .infinity)
        var hi = SIMD2<Float>(repeating: -.infinity)
        func take(_ v: SIMD3<Float>) {
            guard v.x.isFinite, v.y.isFinite else { return }
            lo = simd_min(lo, SIMD2(v.x, v.y))
            hi = simd_max(hi, SIMD2(v.x, v.y))
        }
        if vertices.isEmpty {
            for ring in rings { ring.points.forEach(take) }
        } else {
            vertices.forEach(take)
        }
        return lo.x <= hi.x ? (lo, hi) : nil
    }

    /// Whether the mesh is held. An evicted patch keeps its rings, profile,
    /// stats and hashes (its skeleton) so a route can pass it and an inspector
    /// can name it, but draws nothing.
    var isResident: Bool { !vertices.isEmpty }

    /// This patch without its mesh.
    func skeleton() -> RoadPatch {
        var out = self
        out.vertices = []
        out.normals = []
        out.indices = []
        out.support = []
        return out
    }

    /// Horizontal distance from `p` to the patch's bounds; 0 inside them.
    func distance(from p: SIMD2<Float>) -> Float {
        guard let b = bounds else { return .infinity }
        let dx = max(b.min.x - p.x, 0, p.x - b.max.x)
        let dy = max(b.min.y - p.y, 0, p.y - b.max.y)
        return (dx * dx + dy * dy).squareRoot()
    }

    /// Bytes the mesh holds, for the memory report.
    var meshBytes: Int {
        vertices.count * MemoryLayout<SIMD3<Float>>.stride * 2 + indices.count * 4 + support.count
    }

    /// The patch's height at a point, from the triangle that contains it; nil
    /// outside every triangle, which is where the surface is missing or open.
    func height(at point: SIMD2<Float>) -> Float? {
        for t in 0..<triangleCount {
            let a = vertices[Int(indices[3 * t])]
            let b = vertices[Int(indices[3 * t + 1])]
            let c = vertices[Int(indices[3 * t + 2])]
            if let z = Self.interpolate(point, a, b, c) { return z }
        }
        return nil
    }

    static func interpolate(
        _ p: SIMD2<Float>, _ a: SIMD3<Float>, _ b: SIMD3<Float>, _ c: SIMD3<Float>
    ) -> Float? {
        let d = (b.y - c.y) * (a.x - c.x) + (c.x - b.x) * (a.y - c.y)
        guard abs(d) > 1e-12 else { return nil }
        let l1 = ((b.y - c.y) * (p.x - c.x) + (c.x - b.x) * (p.y - c.y)) / d
        let l2 = ((c.y - a.y) * (p.x - c.x) + (a.x - c.x) * (p.y - c.y)) / d
        let l3 = 1 - l1 - l2
        let eps: Float = -1e-5
        guard l1 >= eps, l2 >= eps, l3 >= eps else { return nil }
        let z = l1 * a.z + l2 * b.z + l3 * c.z
        return z.isFinite ? z : nil
    }

    init(
        patchID: String, kind: RoadPatchKindClass = .segment, featureID: String = "",
        cnn: String = "", name: String = "", fromNode: String = "", toNode: String = "",
        sourceVersion: String = "", observed: String = "", vertices: [SIMD3<Float>] = [],
        normals: [SIMD3<Float>] = [], indices: [UInt32] = [], support: [RoadSupportClass] = [],
        rings: [RoadRing] = [], boundarySource: RoadBoundarySourceClass = .unspecified,
        profile: [RoadProfileStation] = [], assessments: [RoadAssessment] = [],
        stats: RoadPatchStats = RoadPatchStats(), geometryHash: String = "",
        supportHash: String = "", frameToken: String = ""
    ) {
        self.patchID = patchID
        self.kind = kind
        self.featureID = featureID
        self.cnn = cnn
        self.name = name
        self.fromNode = fromNode
        self.toNode = toNode
        self.sourceVersion = sourceVersion
        self.observed = observed
        self.vertices = vertices
        self.normals = normals
        self.indices = indices
        self.support = support
        self.rings = rings
        self.boundarySource = boundarySource
        self.profile = profile
        self.assessments = assessments
        self.stats = stats
        self.geometryHash = geometryHash
        self.supportHash = supportHash
        self.frameToken = frameToken
    }

    init(_ p: Gerardus_Align_V1alpha1_RoadSurfacePatch, frameToken: String = "") {
        func triples(_ f: [Float]) -> [SIMD3<Float>] {
            stride(from: 0, to: f.count - 2, by: 3).map { SIMD3(f[$0], f[$0 + 1], f[$0 + 2]) }
        }
        self.init(
            patchID: p.patchID,
            kind: {
                switch p.kind {
                case .segment: return .segment
                case .junction: return .junction
                default: return .unspecified
                }
            }(), featureID: p.featureID, cnn: p.cnn, name: p.name, fromNode: p.fromNode,
            toNode: p.toNode, sourceVersion: p.sourceVersion, observed: p.observed,
            vertices: triples(p.vertices), normals: triples(p.normals), indices: p.indices,
            support: p.support.map(RoadSupportClass.init),
            rings: p.rings.map {
                RoadRing(
                    points: triples($0.xyz),
                    edgeSource: $0.edgeSource.map(RoadBoundarySourceClass.init), hole: $0.hole,
                    sourceFeatureID: $0.sourceFeatureID)
            }, boundarySource: RoadBoundarySourceClass(p.boundarySource),
            profile: p.profile.map {
                RoadProfileStation(
                    s: $0.sM,
                    position: SIMD3(
                        Float($0.position.x), Float($0.position.y), Float($0.position.z)),
                    grade: $0.grade, support: RoadSupportClass($0.support))
            },
            assessments: p.assessments.map {
                RoadAssessment(
                    component: $0.component, assessed: $0.state == .assessed, source: $0.source,
                    observed: $0.observed)
            },
            stats: {
                let s = p.stats
                return RoadPatchStats(
                    areaM2: s.areaM2, observedAreaM2: s.observedAreaM2,
                    interpolatedAreaM2: s.interpolatedAreaM2, missingAreaM2: s.missingAreaM2,
                    groundPoints: Int(s.groundPoints), heldOutPoints: Int(s.heldOutPoints),
                    residualMedianM: s.residualMedianM, residualRmsM: s.residualRmsM,
                    residualP95AbsM: s.residualP95AbsM, riderLineObserved: s.riderLineObserved,
                    riderLineInterpolated: s.riderLineInterpolated,
                    riderLineMissing: s.riderLineMissing)
            }(), geometryHash: p.geometryHash, supportHash: p.supportHash, frameToken: frameToken)
    }
}

// MARK: - Layer header

struct RoadGapInfo: Sendable, Equatable {
    var featureID: String
    var cnn: String
    var name: String
    var reason: String
    var message: String
    var lengthM: Float
}

/// What a corridor's patches were built from (RoadSurfaceLayer).
struct RoadCorridorInfo: Sendable, Equatable {
    var version: Int
    var corridorID: String
    var manifestHash: String
    var buildHash: String
    var sourceCRS: String
    var sourceVertical: String
    var frameAccuracyM: Float
    var gridM: Float
    var fitRadiusM: Float
    var minPoints: Int
    var maxInterpolationM: Float
    var gaps: [RoadGapInfo]
    var inventoryLengthM: Float
    var surfacedLengthM: Float
    var inferredLengthM: Float
    var missingLengthM: Float

    init(_ l: Gerardus_Align_V1alpha1_RoadSurfaceLayer) {
        version = Int(l.version)
        corridorID = l.corridorID
        manifestHash = l.manifestHash
        buildHash = l.buildHash
        sourceCRS = l.sourceCrs
        sourceVertical = l.sourceVertical
        frameAccuracyM = l.frameAccuracyM
        gridM = l.gridM
        fitRadiusM = l.fitRadiusM
        minPoints = Int(l.minPoints)
        maxInterpolationM = l.maxInterpolationM
        gaps = l.gaps.map {
            RoadGapInfo(
                featureID: $0.featureID, cnn: $0.cnn, name: $0.name, reason: $0.reason,
                message: $0.message, lengthM: $0.lengthM)
        }
        inventoryLengthM = l.inventoryLengthM
        surfacedLengthM = l.surfacedLengthM
        inferredLengthM = l.inferredLengthM
        missingLengthM = l.missingLengthM
    }
}

// MARK: - Why the layer may be absent

/// Why a corridor cannot be shown, so the operator is told rather than shown
/// an empty view.
enum RoadSurfaceAvailability: Equatable, Sendable {
    /// The server speaks road_surface_version 1.
    case available
    /// GetServerInfo said 0 or older than the contract.
    case serverHasNoRoadSurface(version: UInt32)
    /// StreamReference answered INVALID_ARGUMENT for layer 17: a server older
    /// than the enum value.
    case serverRejectsLayer(String)
    /// The layer completed and held no patch for this site.
    case emptyLayer

    var isAvailable: Bool { self == .available }

    var message: String {
        switch self {
        case .available: return ""
        case .serverHasNoRoadSurface(let version):
            return version == 0
                ? "This Gerardus server has no road surface preview (road_surface_version 0)."
                : "This Gerardus server speaks road surface version \(version); this app reads 1."
        case .serverRejectsLayer(let detail):
            return "This Gerardus server is older than the road surface layer (\(detail))."
        case .emptyLayer: return "The server holds no road surface preview for this site."
        }
    }

    /// The contract version this app reads.
    static let supportedVersion: UInt32 = 1

    static func forServer(version: UInt32) -> RoadSurfaceAvailability {
        version == supportedVersion ? .available : .serverHasNoRoadSurface(version: version)
    }

    /// INVALID_ARGUMENT "unknown layer 17" reads as an old server.
    static func forRefusal(code: String, message: String) -> RoadSurfaceAvailability? {
        message.contains("unknown layer") ? .serverRejectsLayer("\(code): \(message)") : nil
    }
}

// MARK: - Store

/// Chunks gathered into corridors and patches, with duplicates removed.
///
/// A patch is whole inside one message. The same `patch_id` through two site
/// bundles is one patch: the first is kept, and a different `geometry_hash`
/// under the same id is recorded as a conflict, never silently preferred.
struct RoadSurfaceStore: Sendable {
    private(set) var corridors: [RoadCorridorInfo] = []
    private(set) var patches: [RoadPatch] = []
    /// patch_id -> corridor id.
    private(set) var corridorOf: [String: String] = [:]
    /// Ids that arrived twice with different geometry.
    private(set) var conflicts: [String] = []
    /// Patches the store refused, with why.
    private(set) var rejected: [(patchID: String, reason: String)] = []
    private(set) var duplicates = 0
    private(set) var complete = false
    private var index: [String: Int] = [:]
    private var currentCorridor = ""

    var isEmpty: Bool { patches.isEmpty }

    /// Adds one message's chunk. `frameToken` names the site frame it is in.
    mutating func append(_ chunk: Gerardus_Align_V1alpha1_RoadSurfaceChunk, frameToken: String = "")
    {
        if chunk.hasLayer {
            let info = RoadCorridorInfo(chunk.layer)
            if !corridors.contains(where: { $0.corridorID == info.corridorID }) {
                corridors.append(info)
            }
            currentCorridor = info.corridorID
        }
        for proto in chunk.patches { add(RoadPatch(proto, frameToken: frameToken)) }
    }

    mutating func add(_ patch: RoadPatch) {
        if let defect = patch.defect() {
            rejected.append((patch.patchID, defect))
            return
        }
        if let i = index[patch.patchID] {
            // A skeleton whose mesh arrives again is made resident again.
            if !patches[i].isResident, patch.isResident,
                patches[i].geometryHash == patch.geometryHash
            {
                patches[i] = patch
                return
            }
            duplicates += 1
            if patches[i].geometryHash != patch.geometryHash, !conflicts.contains(patch.patchID) {
                conflicts.append(patch.patchID)
            }
            return
        }
        index[patch.patchID] = patches.count
        patches.append(patch)
        corridorOf[patch.patchID] = currentCorridor
    }

    mutating func markComplete() { complete = true }

    /// Drops the mesh of every resident patch farther than `radius` from `eye`,
    /// keeping its skeleton. Returns how many were evicted.
    @discardableResult mutating func evictMeshes(
        beyond radius: Float, from eye: SIMD2<Float>
    ) -> Int {
        var n = 0
        for i in patches.indices
        where patches[i].isResident && patches[i].distance(from: eye) > radius {
            patches[i] = patches[i].skeleton()
            n += 1
        }
        return n
    }

    var residentPatches: [RoadPatch] { patches.filter(\.isResident) }
    var residentMeshBytes: Int { patches.reduce(0) { $0 + $1.meshBytes } }

    /// Records a corridor header and makes it the one later patches belong to.
    mutating func appendLayer(_ layer: Gerardus_Align_V1alpha1_RoadSurfaceLayer) {
        let info = RoadCorridorInfo(layer)
        if !corridors.contains(where: { $0.corridorID == info.corridorID }) {
            corridors.append(info)
        }
        currentCorridor = info.corridorID
    }

    mutating func noteRejected(_ items: [(patchID: String, reason: String)]) {
        rejected.append(contentsOf: items)
    }

    func patch(id: String) -> RoadPatch? { index[id].map { patches[$0] } }

    func corridor(of patch: RoadPatch) -> RoadCorridorInfo? {
        corridorOf[patch.patchID].flatMap { id in corridors.first { $0.corridorID == id } }
    }

    /// Patches that are in the store but whose corridor header never arrived.
    var orphans: [RoadPatch] { patches.filter { corridorOf[$0.patchID, default: ""].isEmpty } }

    /// Total area by support class across the patches, m².
    var areaBySupport: (observed: Float, interpolated: Float, missing: Float) {
        patches.reduce(into: (Float(0), Float(0), Float(0))) {
            $0.0 += $1.stats.observedAreaM2
            $0.1 += $1.stats.interpolatedAreaM2
            $0.2 += $1.stats.missingAreaM2
        }
    }
}
