// AlignmentGeometry.swift
// Frames, poses, scan packing and reference geometry for the Gerardus
// alignment client. Nothing here talks to a server.
//
// Three frames, as the vendored align.proto defines them:
//
//   sensor  velocity.report's pipeline frame, in which the visualiser draws
//           everything. +Y is azimuth zero, +X is 90° clockwise from it, +Z
//           is up (l2frames.SphericalToCartesian: x = d·cos(el)·sin(az),
//           y = d·cos(el)·cos(az)).
//   site    east-north-up at the centre of the site's S2 L16 cell, metres.
//           Reference geometry arrives in it.
//   world   geodetic positions, which the server reports and this file
//           never computes.
//
// A pose maps sensor to site: p_site = R(q)·p_sensor + t, with q a Hamilton
// quaternion, w first. The visualiser draws the reference in the sensor frame
// with the inverse of the pose as its model matrix, so moving the pose costs a
// matrix and no network call.

import Foundation
import SwiftProtobuf
import simd

// MARK: - Pose

/// A sensor pose in a site frame, in doubles.
struct SitePose: Equatable, Sendable {
    /// The sensor origin in the site frame, metres.
    var translation: SIMD3<Double>
    /// The sensor axes in the site frame. simd's quaternions are Hamilton
    /// quaternions, the convention the proto names.
    var rotation: simd_quatd

    init(translation: SIMD3<Double>, rotation: simd_quatd) {
        self.translation = translation
        self.rotation = rotation.normalized
    }

    init(_ pose: Gerardus_Align_V1alpha1_SensorPose) {
        let q = pose.rotation
        // An unset rotation decodes as all zeros; treat it as the identity
        // rather than normalising a zero quaternion into NaNs.
        let isZero = q.w == 0 && q.x == 0 && q.y == 0 && q.z == 0
        self.init(
            translation: SIMD3(pose.translationM.x, pose.translationM.y, pose.translationM.z),
            rotation: isZero
                ? simd_quatd(ix: 0, iy: 0, iz: 0, r: 1)
                : simd_quatd(ix: q.x, iy: q.y, iz: q.z, r: q.w))
    }

    /// The pose on the wire. Covariance is left empty: a pose an operator
    /// placed by eye carries no estimate of its own uncertainty.
    func proto(
        siteFrameID: String, sensorFrameID: String, source: Gerardus_Align_V1alpha1_PoseSource
    ) -> Gerardus_Align_V1alpha1_SensorPose {
        var pose = Gerardus_Align_V1alpha1_SensorPose()
        pose.siteFrameID = siteFrameID
        pose.sensorFrameID = sensorFrameID
        pose.translationM.x = translation.x
        pose.translationM.y = translation.y
        pose.translationM.z = translation.z
        pose.rotation.w = rotation.real
        pose.rotation.x = rotation.imag.x
        pose.rotation.y = rotation.imag.y
        pose.rotation.z = rotation.imag.z
        pose.source = source
        return pose
    }

    /// R(q), written out from the Hamilton quaternion (w, x, y, z) so the
    /// convention is on the page rather than inside simd.
    var rotationMatrix: simd_double3x3 {
        let w = rotation.real
        let x = rotation.imag.x
        let y = rotation.imag.y
        let z = rotation.imag.z
        return simd_double3x3(rows: [
            SIMD3(1 - 2 * (y * y + z * z), 2 * (x * y - w * z), 2 * (x * z + w * y)),
            SIMD3(2 * (x * y + w * z), 1 - 2 * (x * x + z * z), 2 * (y * z - w * x)),
            SIMD3(2 * (x * z - w * y), 2 * (y * z + w * x), 1 - 2 * (x * x + y * y)),
        ])
    }

    /// Sensor to site: [R t; 0 1].
    var matrix: simd_double4x4 {
        let r = rotationMatrix
        return simd_double4x4(
            columns: (
                SIMD4(r.columns.0, 0), SIMD4(r.columns.1, 0), SIMD4(r.columns.2, 0),
                SIMD4(translation, 1)
            ))
    }

    /// Site to sensor: [Rᵀ −Rᵀt; 0 1]. Built from the transpose rather than by
    /// a general inverse, so a pose stays exactly rigid however often it is
    /// nudged.
    var inverseMatrix: simd_double4x4 {
        let rt = rotationMatrix.transpose
        let t = -(rt * translation)
        return simd_double4x4(
            columns: (
                SIMD4(rt.columns.0, 0), SIMD4(rt.columns.1, 0), SIMD4(rt.columns.2, 0), SIMD4(t, 1)
            ))
    }

    /// The model matrix that draws site-frame geometry in the sensor frame.
    var referenceModelMatrix: simd_float4x4 {
        let m = inverseMatrix
        return simd_float4x4(
            columns: (
                SIMD4<Float>(m.columns.0), SIMD4<Float>(m.columns.1), SIMD4<Float>(m.columns.2),
                SIMD4<Float>(m.columns.3)
            ))
    }

    /// Bearing of the sensor's +Y (azimuth zero), clockwise from the site's
    /// +Y, which is true north at the site origin. [0, 360).
    var headingDeg: Double {
        let ahead = rotationMatrix * SIMD3(0, 1, 0)
        return Self.wrap360(atan2(ahead.x, ahead.y) * 180 / .pi)
    }

    /// The pose after the operator moves the reference by `nudge` in the
    /// sensor frame. If N moves the reference, the new pose is P·N⁻¹, so the
    /// reference lands where the operator put it.
    func nudged(by nudge: PoseNudge) -> SitePose {
        let undoTurn = simd_quatd(angle: -nudge.yawDeg * .pi / 180, axis: SIMD3(0, 0, 1))
        let rotated = rotation * undoTurn
        let shift = SitePose(translation: .zero, rotation: rotated).rotationMatrix * nudge.offset
        return SitePose(translation: translation - shift, rotation: rotated)
    }

    static func wrap360(_ degrees: Double) -> Double {
        let wrapped = degrees.truncatingRemainder(dividingBy: 360)
        return wrapped < 0 ? wrapped + 360 : wrapped
    }

    /// The signed difference a − b, in (−180, 180].
    static func headingDelta(_ a: Double, _ b: Double) -> Double {
        let d = wrap360(a - b)
        return d > 180 ? d - 360 : d
    }
}

/// One step of an operator's adjustment, applied to the reference as it is
/// drawn in the sensor frame: turn it by `yawDeg` about the sensor's +Z
/// (anticlockwise seen from above), then shift it by `offset` metres along the
/// sensor axes.
struct PoseNudge: Equatable, Sendable {
    var offset: SIMD3<Double> = .zero
    var yawDeg: Double = 0

    /// N = T(offset)·Rz(yaw), the transform the reference undergoes.
    var matrix: simd_double4x4 {
        let a = yawDeg * .pi / 180
        let c = cos(a)
        let s = sin(a)
        return simd_double4x4(
            columns: (SIMD4(c, s, 0, 0), SIMD4(-s, c, 0, 0), SIMD4(0, 0, 1, 0), SIMD4(offset, 1)))
    }
}

/// How far a nudged pose sits from the pose it started from, in the terms the
/// pane shows: site axes (east, north, up) and heading.
struct PoseDelta: Equatable {
    var east: Double
    var north: Double
    var up: Double
    var headingDeg: Double

    init(from base: SitePose, to pose: SitePose) {
        let d = pose.translation - base.translation
        east = d.x
        north = d.y
        up = d.z
        headingDeg = SitePose.headingDelta(pose.headingDeg, base.headingDeg)
    }

    var isZero: Bool {
        abs(east) < 1e-9 && abs(north) < 1e-9 && abs(up) < 1e-9 && abs(headingDeg) < 1e-9
    }
}

// MARK: - Scan upload

/// Packs a settled background snapshot for UploadScan.
enum ScanPacking {
    /// The contract's largest block, about 1 MB on the wire.
    static let maxPointsPerBlock = 65_536

    /// Point blocks in the sensor frame, in snapshot order: x, y, z and the
    /// grid's confidence (TimesSeenCount), which the server weights by. A
    /// snapshot whose arrays disagree in length is cut to the shortest, so a
    /// block never carries arrays of different lengths.
    static func blocks(
        from snapshot: BackgroundSnapshot, maxPoints: Int = maxPointsPerBlock
    ) -> [Gerardus_Align_V1alpha1_PointBlock] {
        let count = min(snapshot.x.count, snapshot.y.count, snapshot.z.count)
        guard count > 0, maxPoints > 0 else { return [] }
        let withConfidence = snapshot.confidence.count >= count
        return stride(from: 0, to: count, by: maxPoints).map { start in
            let end = min(start + maxPoints, count)
            var block = Gerardus_Align_V1alpha1_PointBlock()
            block.x = Array(snapshot.x[start..<end])
            block.y = Array(snapshot.y[start..<end])
            block.z = Array(snapshot.z[start..<end])
            if withConfidence { block.confidence = Array(snapshot.confidence[start..<end]) }
            return block
        }
    }

    static func header(
        l16Token: String, sensorID: String, captureRef: String, snapshot: BackgroundSnapshot
    ) -> Gerardus_Align_V1alpha1_ScanHeader {
        var header = Gerardus_Align_V1alpha1_ScanHeader()
        header.l16Token = l16Token
        header.sensorID = sensorID
        header.sensorModel = "hesai-pandar40p"
        header.kind = .background
        header.captureRef = captureRef
        header.backgroundSequence = snapshot.sequenceNumber
        if snapshot.timestampNanos > 0 {
            header.capturedAt.seconds = snapshot.timestampNanos / 1_000_000_000
            header.capturedAt.nanos = Int32(snapshot.timestampNanos % 1_000_000_000)
        }
        return header
    }
}

/// Gathers the background- and ground-classed points of replayed frames into
/// one scan for UploadScan (SCAN_KIND_ACCUMULATED). The settled background
/// keeps one point per grid cell; a few seconds of frames keep every return,
/// thinned to `voxelM`. Foreground points (things that moved) are never kept.
/// Only full frames will do: a decimated frame would thin the scan unevenly,
/// and a foreground-only frame has no background to keep.
struct FrameAccumulator {
    enum Refusal: Error, Equatable, LocalizedError {
        case decimated(ratio: Float)
        case foregroundOnly
        case unclassified

        var errorDescription: String? {
            switch self {
            case .decimated(let ratio):
                return String(
                    format:
                        "Frames are decimated (ratio %.2f): accumulate from full frames, with decimation off",
                    ratio)
            case .foregroundOnly:
                return "Frames carry only foreground points: accumulate from full frames"
            case .unclassified:
                return "Frames carry no point classes, so background cannot be told from foreground"
            }
        }
    }

    /// One point is kept per voxel of this size, metres.
    static let voxelM: Float = 0.05
    /// PointCloudFrame classes: background 0 and ground 2; foreground 1 is dropped.
    static let keptClasses: Set<UInt8> = [0, 2]

    let targetFrames: Int
    private(set) var frameCount = 0
    private(set) var firstFrameID: UInt64?
    private(set) var lastFrameID: UInt64?
    private(set) var x: [Float] = []
    private(set) var y: [Float] = []
    private(set) var z: [Float] = []
    private var seen = Set<SIMD3<Int32>>()

    init(targetFrames: Int = 50) { self.targetFrames = max(1, targetFrames) }

    var isComplete: Bool { frameCount >= targetFrames }
    var pointCount: Int { x.count }

    /// Why a frame cannot be accumulated, or nothing when it can.
    static func check(_ cloud: PointCloudFrame, frameType: FrameType) throws {
        if frameType == .foreground || cloud.decimationMode == .foregroundOnly {
            throw Refusal.foregroundOnly
        }
        // Spelled out, so `.none` cannot be taken for Optional.none.
        let decimated =
            cloud.decimationMode != DecimationMode.none
            || !(cloud.decimationRatio == 0 || cloud.decimationRatio == 1)
        if decimated {
            throw Refusal.decimated(ratio: cloud.decimationRatio)
        }
        let n = min(cloud.x.count, cloud.y.count, cloud.z.count)
        if n > 0, cloud.classification.count < n { throw Refusal.unclassified }
    }

    /// Adds one frame's background and ground points, or throws why the frame
    /// cannot be used; a refused frame adds nothing.
    mutating func add(_ cloud: PointCloudFrame, frameType: FrameType = .full) throws {
        try Self.check(cloud, frameType: frameType)
        let n = min(cloud.x.count, cloud.y.count, cloud.z.count, cloud.classification.count)
        for i in 0..<n where Self.keptClasses.contains(cloud.classification[i]) {
            let p = SIMD3<Float>(cloud.x[i], cloud.y[i], cloud.z[i])
            // Far beyond any return, and keeps the voxel index inside Int32.
            guard p.x.isFinite, p.y.isFinite, p.z.isFinite, simd_length(p) < 10_000 else { continue }
            let cell = (p / Self.voxelM).rounded(.down)
            if seen.insert(SIMD3<Int32>(Int32(cell.x), Int32(cell.y), Int32(cell.z))).inserted {
                x.append(p.x)
                y.append(p.y)
                z.append(p.z)
            }
        }
        frameCount += 1
        if firstFrameID == nil { firstFrameID = cloud.frameID }
        lastFrameID = cloud.frameID
    }

    /// Point blocks in the sensor frame, in the order points were first seen.
    func blocks(maxPoints: Int = ScanPacking.maxPointsPerBlock) -> [Gerardus_Align_V1alpha1_PointBlock]
    {
        guard !x.isEmpty, maxPoints > 0 else { return [] }
        return stride(from: 0, to: x.count, by: maxPoints).map { start in
            let end = min(start + maxPoints, x.count)
            var block = Gerardus_Align_V1alpha1_PointBlock()
            block.x = Array(x[start..<end])
            block.y = Array(y[start..<end])
            block.z = Array(z[start..<end])
            return block
        }
    }

    /// The capture reference names the replay and the frames: "<replay>#frames=<first>-<last>".
    func captureRef(replayID: String) -> String {
        "\(replayID)#frames=\(firstFrameID ?? 0)-\(lastFrameID ?? 0)"
    }

    func header(l16Token: String, sensorID: String, replayID: String)
        -> Gerardus_Align_V1alpha1_ScanHeader
    {
        var header = Gerardus_Align_V1alpha1_ScanHeader()
        header.l16Token = l16Token
        header.sensorID = sensorID
        header.sensorModel = "hesai-pandar40p"
        header.kind = .accumulated
        header.captureRef = captureRef(replayID: replayID)
        return header
    }
}

// MARK: - Reference geometry

/// One accepted scan of the site model, as StreamReference sends it: points
/// in the site frame, placed by the record that accepted the scan's pose.
struct RegisteredScan: Equatable, Sendable {
    var scanID: String
    var recordID: String
    var kind: Gerardus_Align_V1alpha1_ScanKind
    var captureRef: String
    var x: [Float] = []
    var y: [Float] = []
    var z: [Float] = []

    var pointCount: Int { x.count }
}

/// Reference geometry gathered from StreamReference, in the site frame.
struct ReferenceGeometry: Sendable {
    /// Aerial points as parallel arrays, as they arrived.
    var aerialX: [Float] = []
    var aerialY: [Float] = []
    var aerialZ: [Float] = []
    var aerialClass: [UInt32] = []
    var aerialIntensity: [UInt32] = []
    /// OSM ways.
    var lines: [Gerardus_Align_V1alpha1_LineFeature] = []
    /// DataSF building footprints, kept apart from OSM so each has a toggle.
    var footprints: [Gerardus_Align_V1alpha1_LineFeature] = []
    var intersections: [Gerardus_Align_V1alpha1_Intersection] = []
    /// The site model's layers: accepted scans, the lines derived from them
    /// (sensors, corrections, facade traces and other proposed features) and
    /// the ground mesh.
    var scans: [RegisteredScan] = []
    var observed: [Gerardus_Align_V1alpha1_LineFeature] = []
    var groundMesh: Gerardus_Align_V1alpha1_Heightfield?
    /// The site model those layers were built from; empty when none came.
    var modelID: String = ""
    /// Layers whose last chunk arrived.
    var completeLayers: Set<Gerardus_Align_V1alpha1_ReferenceLayer> = []

    var aerialCount: Int { aerialX.count }
    var scanPointCount: Int { scans.reduce(0) { $0 + $1.pointCount } }

    /// The layers built from the site model, which change whenever a record
    /// does; the rest come from the bundle and stay put.
    static let modelLayers: [Gerardus_Align_V1alpha1_ReferenceLayer] = [
        .registeredScans, .observed, .groundMesh,
    ]

    /// Takes the site model's layers from `other`, keeping the bundle's.
    mutating func replaceModelLayers(with other: ReferenceGeometry) {
        scans = other.scans
        observed = other.observed
        groundMesh = other.groundMesh
        modelID = other.modelID
        for layer in Self.modelLayers {
            if other.completeLayers.contains(layer) {
                completeLayers.insert(layer)
            } else {
                completeLayers.remove(layer)
            }
        }
    }

    mutating func append(_ message: Gerardus_Align_V1alpha1_StreamReferenceResponse) {
        switch message.payload {
        case .points(let block):
            let n = min(block.x.count, block.y.count, block.z.count)
            aerialX.append(contentsOf: block.x.prefix(n))
            aerialY.append(contentsOf: block.y.prefix(n))
            aerialZ.append(contentsOf: block.z.prefix(n))
            // Optional arrays are empty or full length; pad an empty one so
            // every point keeps an entry.
            aerialClass.append(
                contentsOf: block.classification.count >= n
                    ? Array(block.classification.prefix(n)) : Array(repeating: 1, count: n))
            aerialIntensity.append(
                contentsOf: block.intensity.count >= n
                    ? Array(block.intensity.prefix(n)) : Array(repeating: 0, count: n))
        case .lines(let set):
            switch message.layer {
            case .footprints: footprints.append(contentsOf: set.features)
            case .observed: observed.append(contentsOf: set.features)
            default: lines.append(contentsOf: set.features)
            }
        case .intersections(let set): intersections.append(contentsOf: set.intersections)
        case .scanPoints(let part):
            let block = part.points
            let n = min(block.x.count, block.y.count, block.z.count)
            // A scan's blocks arrive in order; later ones extend the first.
            let index: Int
            if let found = scans.firstIndex(where: { $0.scanID == part.scanID }) {
                index = found
            } else {
                scans.append(
                    RegisteredScan(
                        scanID: part.scanID, recordID: part.recordID, kind: part.kind,
                        captureRef: part.captureRef))
                index = scans.count - 1
            }
            scans[index].x.append(contentsOf: block.x.prefix(n))
            scans[index].y.append(contentsOf: block.y.prefix(n))
            scans[index].z.append(contentsOf: block.z.prefix(n))
        case .groundMesh(let field): groundMesh = field
        case nil: break
        }
        if !message.modelID.isEmpty { modelID = message.modelID }
        if message.layerComplete { completeLayers.insert(message.layer) }
    }
}

/// Vertex data the renderer draws for the reference, already coloured.
struct ReferenceOverlayVertices: Sendable {
    /// Point pipeline layout: [x, y, z, colour code / 255, 0] per point. The
    /// code indexes `palette`, which the point shader reads in its table mode.
    var aerial: [Float] = []
    /// Debug-line layout: [x, y, z, r, g, b, a] per vertex, two per segment.
    var osmLines: [Float] = []
    var intersectionLines: [Float] = []
    var footprintLines: [Float] = []
    /// Sensors, corrections and proposed features from the site model.
    var observedLines: [Float] = []
    /// The ground mesh as a wireframe: lines only, no triangles.
    var groundMeshLines: [Float] = []
    /// Registered scans in the point layout, each scan its own palette code.
    var scans: [Float] = []
    var palette: [SIMD4<Float>] = ReferenceStyle.palette

    var aerialCount: Int { aerial.count / 5 }
    var osmVertexCount: Int { osmLines.count / 7 }
    var intersectionVertexCount: Int { intersectionLines.count / 7 }
    var footprintVertexCount: Int { footprintLines.count / 7 }
    var observedVertexCount: Int { observedLines.count / 7 }
    var groundMeshVertexCount: Int { groundMeshLines.count / 7 }
    var scanCount: Int { scans.count / 5 }
}

/// Colours for the reference, chosen here rather than by the server: the
/// proto carries ASPRS classes and feature kinds, never colours.
enum ReferenceStyle {
    /// Each ASPRS class group owns a run of palette codes, dark to light, so
    /// one byte carries both the class and the point's intensity.
    static let shadesPerGroup = 32

    /// The groups, in palette order, with the colour each ramps up to. None
    /// is the sensor background's grey-blue or the foreground's green.
    enum Group: Int, CaseIterable {
        case ground = 0  // ASPRS 2
        case unclassified  // 0, 1: facades, trees and roofs at sites without classes
        case building  // 6
        case vegetation  // 3, 4, 5
        case water  // 9
        case bridge  // 17
        case other

        init(asprsClass: UInt32) {
            switch asprsClass {
            case 2: self = .ground
            case 0, 1: self = .unclassified
            case 6: self = .building
            case 3, 4, 5: self = .vegetation
            case 9: self = .water
            case 17: self = .bridge
            default: self = .other
            }
        }

        var colour: SIMD3<Float> {
            switch self {
            case .ground: return SIMD3(1.00, 0.72, 0.30)
            case .unclassified: return SIMD3(0.90, 0.50, 1.00)
            case .building: return SIMD3(1.00, 0.42, 0.32)
            case .vegetation: return SIMD3(0.80, 0.85, 0.30)
            case .water: return SIMD3(0.30, 0.55, 1.00)
            case .bridge: return SIMD3(0.30, 0.90, 0.85)
            case .other: return SIMD3(0.85, 0.80, 0.80)
            }
        }
    }

    /// Registered scans take the codes after the ASPRS groups' runs, one
    /// colour per scan, repeating after the last.
    static let scanCodeBase = Group.allCases.count * shadesPerGroup
    static let scanColours: [SIMD3<Float>] = [
        SIMD3(0.20, 0.95, 0.95), SIMD3(0.95, 0.35, 0.75), SIMD3(0.55, 1.00, 0.35),
        SIMD3(1.00, 0.60, 0.15), SIMD3(0.55, 0.60, 1.00), SIMD3(1.00, 1.00, 0.45),
        SIMD3(0.40, 0.85, 0.60), SIMD3(0.95, 0.55, 0.55),
    ]

    static func scanCode(index: Int) -> Int { scanCodeBase + index % scanColours.count }

    /// 256 colours: a ramp per group, then the scan colours, the rest unused
    /// and white.
    static let palette: [SIMD4<Float>] = {
        var table = [SIMD4<Float>](repeating: SIMD4(1, 1, 1, 1), count: 256)
        for group in Group.allCases {
            for shade in 0..<shadesPerGroup {
                // Never darker than 35 %, so an unlit point is still seen.
                let level = 0.35 + 0.65 * Float(shade) / Float(shadesPerGroup - 1)
                table[group.rawValue * shadesPerGroup + shade] = SIMD4(group.colour * level, 1)
            }
        }
        for (i, colour) in scanColours.enumerated() { table[scanCodeBase + i] = SIMD4(colour, 1) }
        return table
    }()

    /// The palette code for a point: its group's run, offset by brightness.
    static func code(asprsClass: UInt32, brightness: Float) -> Int {
        let shade = Int((min(max(brightness, 0), 1) * Float(shadesPerGroup - 1)).rounded())
        return Group(asprsClass: asprsClass).rawValue * shadesPerGroup + shade
    }

    /// LAS intensity is 16-bit with no fixed scale between surveys, so the
    /// brightest 5 % of a stream all read as full brightness. Zero when the
    /// survey recorded no intensity.
    static func intensityScale(_ intensity: [UInt32]) -> Float {
        guard !intensity.isEmpty else { return 0 }
        let step = max(1, intensity.count / 20_000)
        var sample = [UInt32]()
        sample.reserveCapacity(intensity.count / step + 1)
        for i in stride(from: 0, to: intensity.count, by: step) { sample.append(intensity[i]) }
        sample.sort()
        return Float(sample[min(sample.count - 1, sample.count * 95 / 100)])
    }

    static func lineColour(_ kind: Gerardus_Align_V1alpha1_FeatureKind) -> SIMD4<Float> {
        switch kind {
        case .road: return SIMD4(1.00, 0.92, 0.35, 0.95)
        case .path: return SIMD4(0.45, 0.85, 1.00, 0.85)
        case .kerb: return SIMD4(1.00, 0.55, 0.20, 0.95)
        case .rail: return SIMD4(0.95, 0.40, 0.90, 0.95)
        case .building: return SIMD4(1.00, 0.40, 0.35, 0.85)
        case .sensor: return SIMD4(1.00, 1.00, 1.00, 0.95)
        case .correction: return SIMD4(1.00, 0.75, 0.10, 0.95)
        case .facade: return SIMD4(0.10, 0.90, 1.00, 0.95)
        case .pole: return SIMD4(0.70, 1.00, 0.40, 0.95)
        case .other, .unspecified, .UNRECOGNIZED: return SIMD4(0.80, 0.80, 0.80, 0.70)
        }
    }

    static let groundMeshColour = SIMD4<Float>(0.55, 0.70, 0.60, 0.40)
    /// Within this many metres of the sensor the ground mesh is drawn at its
    /// own cell size; beyond it, every `groundMeshFarSpacingM` only.
    static let groundMeshNearM: Float = 40
    static let groundMeshFarSpacingM: Float = 2

    static let intersectionColour = SIMD4<Float>(1.00, 1.00, 1.00, 0.95)
    static let approachColour = SIMD4<Float>(0.35, 1.00, 0.95, 0.95)

    /// Draped lines lie on the 3DEP ground and would fight its points for
    /// depth, so they are drawn this far above it. Display only.
    static let lineLift: Float = 0.10
    /// Half the width of an intersection's cross, and an approach tick's length.
    static let crossHalfSize: Float = 2
    static let approachLength: Float = 8

    // MARK: Vertex building

    /// `hidingScans` leaves those scans out of the scan points; `sensor`, the
    /// sensor's position in the site frame, decides where the ground mesh
    /// thins out.
    static func vertices(
        for geometry: ReferenceGeometry, hidingScans: Set<String> = [], sensor: SIMD2<Float>? = nil
    ) -> ReferenceOverlayVertices {
        var out = ReferenceOverlayVertices()
        out.aerial = aerialVertices(geometry)
        out.osmLines = lineVertices(geometry.lines)
        out.intersectionLines = intersectionVertices(geometry.intersections)
        out.footprintLines = lineVertices(geometry.footprints)
        out.observedLines = lineVertices(geometry.observed)
        out.scans = scanVertices(geometry.scans, hiding: hidingScans)
        if let field = geometry.groundMesh {
            out.groundMeshLines = groundMeshVertices(field, sensor: sensor)
        }
        return out
    }

    /// Each scan's points in the point layout, coloured by the scan's place
    /// in the list, so a scan keeps its colour when another is hidden.
    static func scanVertices(_ scans: [RegisteredScan], hiding: Set<String> = []) -> [Float] {
        var out = [Float]()
        out.reserveCapacity(scans.reduce(0) { $0 + $1.pointCount } * 5)
        for (index, scan) in scans.enumerated() where !hiding.contains(scan.scanID) {
            let code = Float(scanCode(index: index)) / 255
            let n = min(scan.x.count, scan.y.count, scan.z.count)
            for i in 0..<n { out.append(contentsOf: [scan.x[i], scan.y[i], scan.z[i], code, 0]) }
        }
        return out
    }

    /// The heightfield as a wireframe: a segment between each pair of
    /// neighbouring nodes that both have ground. Beyond `groundMeshNearM` of
    /// the sensor only every `groundMeshFarSpacingM` row and column is drawn,
    /// so the far field does not become a solid sheet of lines.
    static func groundMeshVertices(
        _ field: Gerardus_Align_V1alpha1_Heightfield, sensor: SIMD2<Float>? = nil
    ) -> [Float] {
        let nx = Int(field.nx)
        let ny = Int(field.ny)
        guard nx > 0, ny > 0, field.z.count == nx * ny, field.cellM > 0 else { return [] }
        let cell = field.cellM
        let step = max(1, Int((groundMeshFarSpacingM / cell).rounded()))
        let ox = Float(field.origin.x)
        let oy = Float(field.origin.y)
        let lift = lineLift / 2
        func node(_ i: Int, _ j: Int) -> SIMD3<Float>? {
            let z = field.z[j * nx + i]
            guard z.isFinite else { return nil }
            return SIMD3(ox + Float(i) * cell, oy + Float(j) * cell, z + lift)
        }
        func near(_ p: SIMD3<Float>) -> Bool {
            guard let sensor else { return true }
            return simd_length(SIMD2(p.x, p.y) - sensor) <= groundMeshNearM
        }
        var out = [Float]()
        for j in 0..<ny {
            for i in 0..<nx {
                guard let a = node(i, j) else { continue }
                // Along x, on this row.
                if i + 1 < nx, let b = node(i + 1, j), near(a) || near(b) || j % step == 0 {
                    appendSegment(&out, a, b, groundMeshColour)
                }
                // Along y, on this column.
                if j + 1 < ny, let b = node(i, j + 1), near(a) || near(b) || i % step == 0 {
                    appendSegment(&out, a, b, groundMeshColour)
                }
            }
        }
        return out
    }

    static func aerialVertices(_ geometry: ReferenceGeometry) -> [Float] {
        let n = geometry.aerialCount
        let scale = intensityScale(geometry.aerialIntensity)
        var out = [Float](repeating: 0, count: n * 5)
        for i in 0..<n {
            let brightness: Float =
                scale > 0 ? min(1, Float(geometry.aerialIntensity[i]) / scale) : 0.6
            let code = code(asprsClass: geometry.aerialClass[i], brightness: brightness)
            out[i * 5] = geometry.aerialX[i]
            out[i * 5 + 1] = geometry.aerialY[i]
            out[i * 5 + 2] = geometry.aerialZ[i]
            out[i * 5 + 3] = Float(code) / 255
            // Classification 0 keeps the shader on its table path.
            out[i * 5 + 4] = 0
        }
        return out
    }

    /// Each polyline as a list of segments, closing a closed one.
    static func lineVertices(_ lines: [Gerardus_Align_V1alpha1_LineFeature]) -> [Float] {
        var out = [Float]()
        for line in lines {
            let count = line.xyz.count / 3
            guard count >= 2 else { continue }
            let colour = lineColour(line.kind)
            func vertex(_ i: Int) -> SIMD3<Float> {
                SIMD3(line.xyz[i * 3], line.xyz[i * 3 + 1], line.xyz[i * 3 + 2] + lineLift)
            }
            for i in 0..<(count - 1) { appendSegment(&out, vertex(i), vertex(i + 1), colour) }
            if line.closed { appendSegment(&out, vertex(count - 1), vertex(0), colour) }
        }
        return out
    }

    /// A cross at each intersection, and a tick down each approach: an
    /// approach's bearing is the direction a vehicle travels on entering, so
    /// the road it arrives along lies on the opposite bearing.
    static func intersectionVertices(
        _ intersections: [Gerardus_Align_V1alpha1_Intersection]
    ) -> [Float] {
        var out = [Float]()
        for node in intersections {
            let p = SIMD3<Float>(
                Float(node.position.x), Float(node.position.y), Float(node.position.z) + lineLift)
            let h = crossHalfSize
            appendSegment(&out, p - SIMD3(h, 0, 0), p + SIMD3(h, 0, 0), intersectionColour)
            appendSegment(&out, p - SIMD3(0, h, 0), p + SIMD3(0, h, 0), intersectionColour)
            for approach in node.approaches {
                let from = Double(approach.bearingDeg + 180) * .pi / 180
                let direction = SIMD3<Float>(Float(sin(from)), Float(cos(from)), 0)
                appendSegment(&out, p, p + direction * approachLength, approachColour)
            }
        }
        return out
    }

    private static func appendSegment(
        _ out: inout [Float], _ a: SIMD3<Float>, _ b: SIMD3<Float>, _ colour: SIMD4<Float>
    ) {
        out.append(contentsOf: [a.x, a.y, a.z, colour.x, colour.y, colour.z, colour.w])
        out.append(contentsOf: [b.x, b.y, b.z, colour.x, colour.y, colour.z, colour.w])
    }
}

// MARK: - Residual colouring

/// Colours the uploaded scan by how far each point lies from the reference,
/// from EvaluatePose's per-point residuals: green on it, through yellow, to
/// red at half a metre or more, and grey where no reference lies within the
/// server's correspondence distance. The magnitude is what an operator
/// reads; the sign depends on which way the reference surface faces.
enum ResidualStyle {
    /// Residuals at or beyond this many metres take the last colour.
    static let fullScaleM: Float = 0.5

    /// Code 0 is "no reference"; codes 1 to 255 ramp from 0 to full scale.
    static let palette: [SIMD4<Float>] = {
        var table = [SIMD4<Float>](repeating: SIMD4(0.45, 0.45, 0.50, 1), count: 256)
        let green = SIMD3<Float>(0.20, 0.90, 0.35)
        let yellow = SIMD3<Float>(1.00, 0.85, 0.15)
        let red = SIMD3<Float>(1.00, 0.20, 0.15)
        for code in 1...255 {
            let t = Float(code - 1) / 254
            let colour =
                t < 0.3 ? blend(green, yellow, t / 0.3) : blend(yellow, red, (t - 0.3) / 0.7)
            table[code] = SIMD4(colour, 1)
        }
        return table
    }()

    static func code(residualM: Float) -> Int {
        guard residualM.isFinite else { return 0 }
        let t = min(abs(residualM) / fullScaleM, 1)
        return 1 + Int((t * 254).rounded())
    }

    /// Point-pipeline vertices for the uploaded snapshot's points in upload
    /// order, each coloured by its residual. Nil when the residuals do not
    /// match the points one for one, which means they are for another scan.
    static func vertices(snapshot: BackgroundSnapshot, residuals: [Float]) -> [Float]? {
        let count = min(snapshot.x.count, snapshot.y.count, snapshot.z.count)
        guard count > 0, residuals.count == count else { return nil }
        var out = [Float](repeating: 0, count: count * 5)
        for i in 0..<count {
            out[i * 5] = snapshot.x[i]
            out[i * 5 + 1] = snapshot.y[i]
            out[i * 5 + 2] = snapshot.z[i]
            out[i * 5 + 3] = Float(code(residualM: residuals[i])) / 255
        }
        return out
    }

    private static func blend(_ a: SIMD3<Float>, _ b: SIMD3<Float>, _ t: Float) -> SIMD3<Float> {
        a + (b - a) * t
    }
}
