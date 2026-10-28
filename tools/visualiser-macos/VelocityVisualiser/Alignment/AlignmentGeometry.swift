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

    /// `capturedAtNanos` is when the points were captured (`capturedAtNanos(...)`),
    /// or nil to leave `captured_at` unset: never the time of the replay.
    static func header(
        l16Token: String, sensorID: String, captureRef: String, snapshot: BackgroundSnapshot,
        capturedAtNanos: Int64?
    ) -> Gerardus_Align_V1alpha1_ScanHeader {
        var header = Gerardus_Align_V1alpha1_ScanHeader()
        header.l16Token = l16Token
        header.sensorID = sensorID
        header.sensorModel = "hesai-pandar40p"
        header.kind = .background
        header.captureRef = captureRef
        header.backgroundSequence = snapshot.sequenceNumber
        if let at = capturedAtNanos, at > 0 {
            header.capturedAt.seconds = at / 1_000_000_000
            header.capturedAt.nanos = Int32(at % 1_000_000_000)
        }
        return header
    }

    /// When a scan's points were captured, for `captured_at`: Gerardus's
    /// permanence filter counts the days a point was seen on, so a replay
    /// must stamp the capture's day, not its own. `observedNanos` is the
    /// time on the points (a snapshot's, or the first accumulated frame's).
    ///
    /// - Live: the points were captured as they were observed.
    /// - A pcap or VRLOG replay: the observed time when it lies in the log's
    ///   span (a frame stamped with its capture time), else the log's first
    ///   frame (a background snapshot is stamped when the server took it).
    /// - Otherwise, or with no log span: nil, and the field stays unset.
    static func capturedAtNanos(
        source: SourceMode, logStartNs: Int64, logEndNs: Int64, observedNanos: Int64
    ) -> Int64? {
        switch source {
        case .live: return observedNanos > 0 ? observedNanos : nil
        case .pcap, .pcapAnalysis, .vrlog:
            guard logStartNs > 0 else { return nil }
            if logEndNs >= logStartNs, observedNanos >= logStartNs, observedNanos <= logEndNs {
                return observedNanos
            }
            return logStartNs
        case .unspecified: return nil
        }
    }

    /// The header for accumulated frames: SCAN_KIND_ACCUMULATED, named by the
    /// replay and its frame range, captured when the first frame was. No
    /// background sequence: the points did not come from the grid. The
    /// frames' moving objects go up as its transient boxes.
    static func accumulatedHeader(
        l16Token: String, sensorID: String, captureRef: String, snapshot: BackgroundSnapshot,
        capturedAtNanos: Int64?, transient: [TransientBox] = []
    ) -> Gerardus_Align_V1alpha1_ScanHeader {
        var accumulated = header(
            l16Token: l16Token, sensorID: sensorID, captureRef: captureRef, snapshot: snapshot,
            capturedAtNanos: capturedAtNanos)
        accumulated.kind = .accumulated
        accumulated.backgroundSequence = 0
        accumulated.transient = transient.map(box3)
        return accumulated
    }

    /// A transient box as Gerardus's Box3, in the scan's sensor frame.
    static func box3(_ box: TransientBox) -> Gerardus_Align_V1alpha1_Box3 {
        var b = Gerardus_Align_V1alpha1_Box3()
        b.centre.x = Double(box.centre.x)
        b.centre.y = Double(box.centre.y)
        b.centre.z = Double(box.centre.z)
        b.sizeM.x = Double(box.size.x)
        b.sizeM.y = Double(box.size.y)
        b.sizeM.z = Double(box.size.z)
        b.headingRad = Double(box.headingRad)
        b.label = box.label
        return b
    }
}

// MARK: - Reference geometry

/// Reference geometry gathered from StreamReference, in the site frame.
///
/// The registered layers (REGISTERED_SCANS, OBSERVED, GROUND_MESH) are built
/// from the site model, the site's accepted scans, and carry the model's id.
/// They move whenever a record is accepted, so the session re-streams them on
/// their own and swaps them in with `replaceRegistered(with:)`. The boxes
/// (BOXES) are reference data built with the bundle, so they stay.
struct ReferenceGeometry: Sendable {
    /// The layers built from the site model rather than the bundle.
    static let registeredLayers: Set<Gerardus_Align_V1alpha1_ReferenceLayer> = [
        .registeredScans, .observed, .groundMesh,
    ]

    /// Aerial points as parallel arrays, as they arrived.
    var aerialX: [Float] = []
    var aerialY: [Float] = []
    var aerialZ: [Float] = []
    var aerialClass: [UInt32] = []
    var aerialIntensity: [UInt32] = []
    /// OSM ways.
    var lines: [Gerardus_Align_V1alpha1_LineFeature] = []
    /// DataSF building footprints, draped.
    var footprints: [Gerardus_Align_V1alpha1_LineFeature] = []
    /// DataSF footprints as boxes (site model spike §6.6): one closed
    /// vertical rectangle per wall, from the 3DEP ground to a roof height
    /// measured in 3DEP.
    var boxes: [Gerardus_Align_V1alpha1_LineFeature] = []
    var intersections: [Gerardus_Align_V1alpha1_Intersection] = []
    /// The street layers the street painter draws over and snaps to: DataSF
    /// street centrelines (CITY_STREETS), the nodes where three or more meet
    /// (CITY_INTERSECTIONS), and kerb and island lines (KERBS), from DataSF
    /// and from the registered scans.
    var cityStreets: [Gerardus_Align_V1alpha1_LineFeature] = []
    var cityIntersections: [Gerardus_Align_V1alpha1_Intersection] = []
    var kerbs: [Gerardus_Align_V1alpha1_LineFeature] = []
    /// The W4–W7 layers, when the server lists them: building parts and
    /// their roof planes (BUILDINGS), painted markings (LANE_MARKINGS),
    /// poles and signs (STREET_FURNITURE), and trees (TREES).
    var buildingParts: [Gerardus_Align_V1alpha1_LineFeature] = []
    var roofPlanes: [Gerardus_Align_V1alpha1_LineFeature] = []
    var laneMarkings: [Gerardus_Align_V1alpha1_LineFeature] = []
    var streetFurniture: [Gerardus_Align_V1alpha1_LineFeature] = []
    var trees: [Gerardus_Align_V1alpha1_LineFeature] = []
    /// Registered scans, block by block, in the site frame.
    var scans: [Gerardus_Align_V1alpha1_ScanPoints] = []
    /// The observed layer: sensors, corrections and the proposed features
    /// (facade traces, poles) derived from the registered scans. Wall planes
    /// arrive in the same layer but are kept in `wallPlanes`.
    var observed: [Gerardus_Align_V1alpha1_LineFeature] = []
    /// The observed layer's wall planes: closed vertical rectangles the
    /// registered scans measure against a box wall, each with a role.
    var wallPlanes: [Gerardus_Align_V1alpha1_LineFeature] = []
    /// The ground surface, normally one heightfield.
    var groundMesh: [Gerardus_Align_V1alpha1_Heightfield] = []
    /// The site model the registered layers were built from; empty until a
    /// registered layer names one.
    var modelID: String = ""
    /// Layers whose last chunk arrived.
    var completeLayers: Set<Gerardus_Align_V1alpha1_ReferenceLayer> = []

    var aerialCount: Int { aerialX.count }

    /// Box walls shared with a neighbour, which nobody sees from the street.
    var partyWallCount: Int { boxes.filter(ReferenceStyle.isPartyWall).count }

    /// Wall planes by role.
    var wallPlaneRoles: [ReferenceStyle.WallPlaneRole: Int] {
        wallPlanes.reduce(into: [:]) { $0[ReferenceStyle.wallPlaneRole($1), default: 0] += 1 }
    }

    /// Registered scan points, whatever is drawn.
    var scanPointCount: Int { scans.reduce(0) { $0 + Self.count($1.points) } }

    /// Each registered scan's id once, in the order the scans arrived: the
    /// order that gives each scan its colour.
    var scanOrder: [String] {
        var seen = Set<String>()
        return scans.map(\.scanID).filter { seen.insert($0).inserted }
    }

    /// Ground mesh cells that hold ground.
    var groundMeshCells: Int {
        groundMesh.reduce(0) { total, field in total + field.z.filter(\.isFinite).count }
    }

    mutating func append(_ message: Gerardus_Align_V1alpha1_StreamReferenceResponse) {
        switch message.payload {
        case .points(let block):
            let n = Self.count(block)
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
            // Four layers send line features; each keeps its own toggle, and
            // the observed layer's wall planes have one of their own.
            switch message.layer {
            case .observed:
                for feature in set.features {
                    if feature.kind == .wallPlane {
                        wallPlanes.append(feature)
                    } else {
                        observed.append(feature)
                    }
                }
            case .footprints: footprints.append(contentsOf: set.features)
            case .boxes: boxes.append(contentsOf: set.features)
            case .cityStreets: cityStreets.append(contentsOf: set.features)
            case .kerbs: kerbs.append(contentsOf: set.features)
            case .buildings:
                for feature in set.features {
                    if feature.kind == .roofPlane {
                        roofPlanes.append(feature)
                    } else {
                        buildingParts.append(feature)
                    }
                }
            case .laneMarkings: laneMarkings.append(contentsOf: set.features)
            case .streetFurniture: streetFurniture.append(contentsOf: set.features)
            case .trees: trees.append(contentsOf: set.features)
            default: lines.append(contentsOf: set.features)
            }
        case .intersections(let set):
            if message.layer == .cityIntersections {
                cityIntersections.append(contentsOf: set.intersections)
            } else {
                intersections.append(contentsOf: set.intersections)
            }
        case .scanPoints(let block): scans.append(block)
        case .groundMesh(let field): groundMesh.append(field)
        case nil: break
        }
        // BUILDINGS names a model of its own: only the registered layers'
        // is the site model's.
        if !message.modelID.isEmpty, Self.registeredLayers.contains(message.layer) {
            modelID = message.modelID
        }
        if message.layerComplete { completeLayers.insert(message.layer) }
    }

    /// Adds another stream's geometry to this one.
    mutating func merge(_ other: ReferenceGeometry) {
        aerialX.append(contentsOf: other.aerialX)
        aerialY.append(contentsOf: other.aerialY)
        aerialZ.append(contentsOf: other.aerialZ)
        aerialClass.append(contentsOf: other.aerialClass)
        aerialIntensity.append(contentsOf: other.aerialIntensity)
        lines.append(contentsOf: other.lines)
        footprints.append(contentsOf: other.footprints)
        boxes.append(contentsOf: other.boxes)
        intersections.append(contentsOf: other.intersections)
        cityStreets.append(contentsOf: other.cityStreets)
        cityIntersections.append(contentsOf: other.cityIntersections)
        kerbs.append(contentsOf: other.kerbs)
        buildingParts.append(contentsOf: other.buildingParts)
        roofPlanes.append(contentsOf: other.roofPlanes)
        laneMarkings.append(contentsOf: other.laneMarkings)
        streetFurniture.append(contentsOf: other.streetFurniture)
        trees.append(contentsOf: other.trees)
        scans.append(contentsOf: other.scans)
        observed.append(contentsOf: other.observed)
        wallPlanes.append(contentsOf: other.wallPlanes)
        groundMesh.append(contentsOf: other.groundMesh)
        if !other.modelID.isEmpty { modelID = other.modelID }
        completeLayers.formUnion(other.completeLayers)
    }

    /// Swaps in `other`'s registered layers and model id, keeping the bundle's
    /// layers (aerial, OSM, footprints, boxes, intersections) as they are.
    mutating func replaceRegistered(with other: ReferenceGeometry) {
        scans = other.scans
        observed = other.observed
        wallPlanes = other.wallPlanes
        groundMesh = other.groundMesh
        modelID = other.modelID
        completeLayers.subtract(Self.registeredLayers)
        completeLayers.formUnion(other.completeLayers.intersection(Self.registeredLayers))
    }

    static func count(_ block: Gerardus_Align_V1alpha1_PointBlock) -> Int {
        min(block.x.count, block.y.count, block.z.count)
    }
}

/// Which reference layers are drawn.
struct ReferenceVisibility: Equatable, Sendable {
    var aerial = true
    var osm = true
    var footprints = true
    var boxes = true
    var intersections = true
    var registeredScans = true
    var observed = true
    var wallPlanes = true
    var groundMesh = true
    var kerbs = true
    /// The City's centrelines and their intersections.
    var cityStreets = true
    /// The W4–W7 layers: building parts with their roofs, lane markings,
    /// poles and signs, and trees.
    var buildings = true
    var laneMarkings = true
    var streetFurniture = true
    var trees = true
}

/// How a refinement shown in its column marks the scene: each scan's sensor
/// arrow and correction line in its state's colour (RefinedScanState), and
/// the selected scan bold, its sensor arrow drawn several times over, with
/// the other registered scans' points dimmed.
struct ReferenceEmphasis: Equatable, Sendable {
    /// Colour by scan_id for SENSOR and CORRECTION features.
    var tints: [String: SIMD4<Float>] = [:]
    var selectedScanID: String?
}

/// Vertex data the renderer draws for the reference, already coloured.
struct ReferenceOverlayVertices: Sendable {
    /// Point pipeline layout: [x, y, z, colour code / 255, 0] per point. The
    /// code indexes `palette`, which the point shader reads in its table mode.
    var aerial: [Float] = []
    /// Registered scans in the point layout, one palette code per scan.
    var scans: [Float] = []
    /// Debug-line layout: [x, y, z, r, g, b, a] per vertex, two per segment.
    var osmLines: [Float] = []
    var footprintLines: [Float] = []
    /// Box walls, party walls fainter.
    var boxLines: [Float] = []
    var intersectionLines: [Float] = []
    var observedLines: [Float] = []
    /// Wall planes, coloured by role.
    var wallPlaneLines: [Float] = []
    /// The ground surface as a wireframe, through the same line layout.
    var groundMeshLines: [Float] = []
    /// Kerb and island lines, and the City's centrelines with a cross at
    /// each of its intersections.
    var kerbLines: [Float] = []
    var cityStreetLines: [Float] = []
    /// Building parts as wireframe prisms, by kind of edge, with their roof
    /// planes' outlines; the walls' and the roof planes' fills as triangles
    /// in the same layout. Opacity and thickness are applied when drawn.
    var eaveLines: [Float] = []
    var cornerLines: [Float] = []
    var roofEdgeLines: [Float] = []
    var wallFills: [Float] = []
    var roofFills: [Float] = []
    /// Every edge of every building, the kinds together.
    var buildingLines: [Float] { eaveLines + cornerLines + roofEdgeLines }
    var laneMarkingLines: [Float] = []
    var streetFurnitureLines: [Float] = []
    var treeLines: [Float] = []
    var palette: [SIMD4<Float>] = ReferenceStyle.palette

    var aerialCount: Int { aerial.count / 5 }
    var scanCount: Int { scans.count / 5 }
    var osmVertexCount: Int { osmLines.count / 7 }
    var footprintVertexCount: Int { footprintLines.count / 7 }
    var boxVertexCount: Int { boxLines.count / 7 }
    var intersectionVertexCount: Int { intersectionLines.count / 7 }
    var observedVertexCount: Int { observedLines.count / 7 }
    var wallPlaneVertexCount: Int { wallPlaneLines.count / 7 }
    var groundMeshVertexCount: Int { groundMeshLines.count / 7 }
    var kerbVertexCount: Int { kerbLines.count / 7 }
    var cityStreetVertexCount: Int { cityStreetLines.count / 7 }
    var eaveVertexCount: Int { eaveLines.count / 7 }
    var cornerVertexCount: Int { cornerLines.count / 7 }
    var roofEdgeVertexCount: Int { roofEdgeLines.count / 7 }
    var buildingVertexCount: Int { eaveVertexCount + cornerVertexCount + roofEdgeVertexCount }
    var wallFillVertexCount: Int { wallFills.count / 7 }
    var roofFillVertexCount: Int { roofFills.count / 7 }
    var laneMarkingVertexCount: Int { laneMarkingLines.count / 7 }
    var streetFurnitureVertexCount: Int { streetFurnitureLines.count / 7 }
    var treeVertexCount: Int { treeLines.count / 7 }
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

    /// The first code above the ASPRS groups' runs (7 × 32 = 224). Registered
    /// scans take the codes from here to 255, one per scan.
    static let scanCodeBase = Group.allCases.count * shadesPerGroup
    static let scanCodeCount = 256 - scanCodeBase

    /// 256 colours: a ramp per group, then one colour per registered scan.
    static let palette: [SIMD4<Float>] = {
        var table = [SIMD4<Float>](repeating: SIMD4(1, 1, 1, 1), count: 256)
        for group in Group.allCases {
            for shade in 0..<shadesPerGroup {
                // Never darker than 35 %, so an unlit point is still seen.
                let level = 0.35 + 0.65 * Float(shade) / Float(shadesPerGroup - 1)
                table[group.rawValue * shadesPerGroup + shade] = SIMD4(group.colour * level, 1)
            }
        }
        for index in 0..<scanCodeCount { table[scanCodeBase + index] = scanColour(index) }
        return table
    }()

    /// The palette code for the registered scan that arrived `index`th.
    /// Past 32 scans the colours repeat.
    static func scanCode(index: Int) -> Int { scanCodeBase + index % scanCodeCount }

    /// A distinct hue per scan: golden-ratio steps round the colour wheel,
    /// skipping the greens the live foreground is drawn in.
    static func scanColour(_ index: Int) -> SIMD4<Float> {
        let step = (Double(index) * 0.618_033_988_75).truncatingRemainder(dividingBy: 1)
        // Hues 0.45 to 1.20 of a turn: cyan, blue, violet, red, orange, yellow.
        let hue = (0.45 + step * 0.75).truncatingRemainder(dividingBy: 1)
        return SIMD4(hsv(hue: hue, saturation: 0.6, value: 1), 1)
    }

    private static func hsv(hue: Double, saturation s: Double, value v: Double) -> SIMD3<Float> {
        let h = hue * 6
        let sector = Int(h) % 6
        let f = h - floor(h)
        let p = v * (1 - s)
        let q = v * (1 - s * f)
        let t = v * (1 - s * (1 - f))
        let rgb: (Double, Double, Double)
        switch sector {
        case 0: rgb = (v, t, p)
        case 1: rgb = (q, v, p)
        case 2: rgb = (p, v, t)
        case 3: rgb = (p, q, v)
        case 4: rgb = (t, p, v)
        default: rgb = (v, p, q)
        }
        return SIMD3(Float(rgb.0), Float(rgb.1), Float(rgb.2))
    }

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
        // The observed layer, from the registered scans and their records.
        case .sensor: return SIMD4(1.00, 1.00, 1.00, 1.00)
        case .correction: return SIMD4(1.00, 0.70, 0.00, 1.00)
        case .facade: return SIMD4(0.00, 0.90, 1.00, 0.95)
        case .pole: return SIMD4(0.35, 0.45, 1.00, 0.95)
        case .wallPlane, .wallSurface: return wallPlaneColour(.wall)
        // A traffic island's outline reads as the kerb it is.
        case .island: return SIMD4(1.00, 0.55, 0.20, 0.95)
        // Operator-marked street use (Gerardus VECTOR-MAP §4.1).
        case .driveway: return SIMD4(0.75, 1.00, 0.45, 0.95)
        case .parkingSpace: return SIMD4(0.55, 0.75, 1.00, 0.90)
        case .travelLane: return SIMD4(1.00, 1.00, 1.00, 0.60)
        case .surface: return SIMD4(0.60, 0.60, 0.65, 0.80)
        // The W4–W7 layers. A roof plane is drawn in its slope's or its
        // part's shape's colour (FeatureLayerStyle); this is its colour when
        // it carries neither.
        case .roofPlane: return SIMD4(0.85, 0.50, 0.40, 0.95)
        case .buildingPart: return SIMD4(0.95, 0.78, 0.55, 1.00)
        case .laneMarking: return SIMD4(1.00, 0.98, 0.80, 1.00)
        case .sign: return SIMD4(0.95, 0.20, 0.50, 1.00)
        case .tree: return SIMD4(0.55, 0.95, 0.30, 0.90)
        case .other, .unspecified, .UNRECOGNIZED: return otherColour
        }
    }

    static let otherColour = SIMD4<Float>(0.80, 0.80, 0.80, 0.70)

    // MARK: Boxes and wall planes (site model spike §6.6)

    /// A box wall: a darker, greyer red than the flat footprints' outline,
    /// so the two read apart when both are drawn.
    static let boxWallColour = SIMD4<Float>(0.72, 0.36, 0.33, 0.70)
    /// A party wall, shared with a neighbour and seen from no street: the
    /// same red, fainter.
    static let partyWallColour = SIMD4<Float>(0.72, 0.36, 0.33, 0.25)

    /// Tagged `party=true` by Gerardus.
    static func isPartyWall(_ wall: Gerardus_Align_V1alpha1_LineFeature) -> Bool {
        wall.tags["party"] == "true"
    }

    static func boxWallColour(_ wall: Gerardus_Align_V1alpha1_LineFeature) -> SIMD4<Float> {
        isPartyWall(wall) ? partyWallColour : boxWallColour
    }

    /// What a wall plane is, from its `role` tag: the wall itself, or a part
    /// standing out from it or set back. A role the client does not know is
    /// `other`.
    enum WallPlaneRole: String, CaseIterable, Sendable {
        case wall
        case popout
        case recess
        case other
    }

    static func wallPlaneRole(_ plane: Gerardus_Align_V1alpha1_LineFeature) -> WallPlaneRole {
        WallPlaneRole(rawValue: plane.tags["role"] ?? "") ?? .other
    }

    static func wallPlaneColour(_ role: WallPlaneRole) -> SIMD4<Float> {
        switch role {
        case .wall: return SIMD4(0.10, 0.80, 0.70, 0.95)  // cyan-teal
        case .popout: return SIMD4(1.00, 0.60, 0.10, 0.95)  // orange
        case .recess: return SIMD4(0.65, 0.40, 1.00, 0.95)  // violet
        case .other: return otherColour
        }
    }

    static let intersectionColour = SIMD4<Float>(1.00, 1.00, 1.00, 0.95)
    /// The City's centrelines: a pale slate, so they read apart from OSM's
    /// yellow roads when both are drawn.
    static let cityStreetColour = SIMD4<Float>(0.62, 0.70, 0.82, 0.90)
    static let approachColour = SIMD4<Float>(0.35, 1.00, 0.95, 0.95)
    /// Faint, so the wireframe reads as a surface under everything else.
    static let groundMeshColour = SIMD4<Float>(0.55, 0.80, 0.60, 0.35)

    /// Draped lines lie on the 3DEP ground and would fight its points for
    /// depth, so they are drawn this far above it. Display only.
    static let lineLift: Float = 0.10
    /// Half the width of an intersection's cross, and an approach tick's length.
    static let crossHalfSize: Float = 2
    static let approachLength: Float = 8

    /// The lift a kind is drawn with. Sensors, corrections and poles are
    /// placed in the air by the server (a sensor's stalk ends at its origin,
    /// a correction sits 0.2 m up), so they are drawn where they are.
    static func lift(_ kind: Gerardus_Align_V1alpha1_FeatureKind) -> Float {
        switch kind {
        case .sensor, .correction, .pole, .sign, .tree, .roofPlane: return 0
        default: return lineLift
        }
    }

    /// The ground mesh is clipped to the streaming radius, and drawn at its
    /// own cell size only near the sensor (site model spike §6.5).
    static let groundMeshRadiusM: Double = 150
    static let groundMeshDenseRadiusM: Double = 40
    static let groundMeshCoarseSpacingM: Double = 2

    // MARK: Vertex building

    /// Everything the renderer draws for `geometry`. `centre` is the sensor's
    /// position in the site frame, which the ground mesh is clipped around;
    /// scans in `hiddenScanIDs` are left out.
    static func vertices(
        for geometry: ReferenceGeometry, centre: SIMD3<Double>? = nil,
        hiddenScanIDs: Set<String> = [], emphasis: ReferenceEmphasis = ReferenceEmphasis(),
        roofColouring: RoofColouring = .slope
    ) -> ReferenceOverlayVertices {
        var out = ReferenceOverlayVertices()
        out.aerial = aerialVertices(geometry)
        out.scans = scanVertices(geometry, hidden: hiddenScanIDs)
        out.palette = scanPalette(
            selectedScanID: emphasis.selectedScanID, order: geometry.scanOrder)
        out.osmLines = lineVertices(geometry.lines)
        out.footprintLines = lineVertices(geometry.footprints)
        out.boxLines = boxVertices(geometry.boxes)
        out.intersectionLines = intersectionVertices(geometry.intersections)
        out.observedLines = observedVertices(geometry.observed, emphasis: emphasis)
        out.wallPlaneLines = wallPlaneVertices(geometry.wallPlanes)
        out.groundMeshLines = groundMeshVertices(
            geometry.groundMesh, centre: centre.map { SIMD2($0.x, $0.y) })
        out.kerbLines = lineVertices(geometry.kerbs)
        out.cityStreetLines = cityStreetVertices(
            geometry.cityStreets, intersections: geometry.cityIntersections)
        let roofs = roofPlaneVertices(
            geometry.roofPlanes, parts: geometry.buildingParts, colouring: roofColouring)
        let edges = buildingPartEdges(geometry.buildingParts)
        out.eaveLines = edges.eaves
        out.cornerLines = edges.corners
        out.roofEdgeLines = roofs.lines
        out.wallFills = wallFillVertices(geometry.buildingParts)
        out.roofFills = roofs.fills
        out.laneMarkingLines = laneMarkingVertices(geometry.laneMarkings)
        out.streetFurnitureLines = streetFurnitureVertices(geometry.streetFurniture)
        out.treeLines = treeVertices(geometry.trees)
        return out
    }

    /// The City's centrelines in their own colour, whatever kind they carry,
    /// and a cross at each of its intersections, without approach ticks.
    static func cityStreetVertices(
        _ streets: [Gerardus_Align_V1alpha1_LineFeature],
        intersections: [Gerardus_Align_V1alpha1_Intersection]
    ) -> [Float] {
        var out = [Float]()
        for street in streets { appendPolyline(&out, street, colour: cityStreetColour) }
        for node in intersections {
            let p = SIMD3<Float>(
                Float(node.position.x), Float(node.position.y), Float(node.position.z) + lineLift)
            let h = crossHalfSize
            appendSegment(&out, p - SIMD3(h, 0, 0), p + SIMD3(h, 0, 0), intersectionColour)
            appendSegment(&out, p - SIMD3(0, h, 0), p + SIMD3(0, h, 0), intersectionColour)
        }
        return out
    }

    /// How much of its colour a registered scan keeps while another is
    /// selected.
    static let dimmedScanLevel: Float = 0.3

    /// The palette, with every registered scan's colour but the selected
    /// one's dimmed. Unchanged without a selection, or for a scan not drawn.
    static func scanPalette(selectedScanID: String?, order: [String]) -> [SIMD4<Float>] {
        guard let selectedScanID, let index = order.firstIndex(of: selectedScanID) else {
            return palette
        }
        let keep = scanCode(index: index)
        var table = palette
        for code in scanCodeBase..<(scanCodeBase + scanCodeCount) where code != keep {
            let c = table[code]
            table[code] = SIMD4(SIMD3(c.x, c.y, c.z) * dimmedScanLevel, c.w)
        }
        return table
    }

    /// The scan a SENSOR or CORRECTION feature belongs to, from its tags.
    static func scanID(of feature: Gerardus_Align_V1alpha1_LineFeature) -> String? {
        guard feature.kind == .sensor || feature.kind == .correction else { return nil }
        let id = feature.tags["scan_id"] ?? ""
        return id.isEmpty ? nil : id
    }

    /// How far each extra copy of a selected scan's sensor arrow is set off,
    /// in metres: lines are a pixel wide, so the copies draw it bold.
    static let boldOffsetM: Float = 0.05

    /// The observed layer, with the emphasis applied: a tinted scan's sensor
    /// and correction in its tint, and the selected scan's sensor arrow and
    /// correction again at four small offsets.
    static func observedVertices(
        _ lines: [Gerardus_Align_V1alpha1_LineFeature], emphasis: ReferenceEmphasis
    ) -> [Float] {
        guard !emphasis.tints.isEmpty || emphasis.selectedScanID != nil else {
            return lineVertices(lines)
        }
        var out = [Float]()
        for line in lines {
            guard let id = scanID(of: line) else {
                out.append(contentsOf: lineVertices([line]))
                continue
            }
            let colour = emphasis.tints[id] ?? lineColour(line.kind)
            appendPolyline(&out, line, colour: colour)
            guard id == emphasis.selectedScanID else { continue }
            let d = boldOffsetM
            for offset in [SIMD3(d, 0, 0), SIMD3(-d, 0, 0), SIMD3(0, d, 0), SIMD3(0, -d, 0)] {
                appendPolyline(&out, line, colour: colour, offset: offset)
            }
        }
        return out
    }

    /// Registered scan points in the point layout, each in its scan's colour.
    /// A scan's colour follows its place in `scanOrder`, so hiding one scan
    /// does not recolour the rest.
    static func scanVertices(_ geometry: ReferenceGeometry, hidden: Set<String>) -> [Float] {
        var codes = [String: Float]()
        for (index, id) in geometry.scanOrder.enumerated() {
            codes[id] = Float(scanCode(index: index)) / 255
        }
        var out = [Float]()
        out.reserveCapacity(geometry.scanPointCount * 5)
        for block in geometry.scans where !hidden.contains(block.scanID) {
            let points = block.points
            let code = codes[block.scanID] ?? Float(scanCodeBase) / 255
            for i in 0..<ReferenceGeometry.count(points) {
                // Classification 0 keeps the shader on its table path.
                out.append(contentsOf: [points.x[i], points.y[i], points.z[i], code, 0])
            }
        }
        return out
    }

    /// A heightfield as a wireframe: an edge joins neighbouring cell centres
    /// that both hold ground. Row-major from the south-west cell centre:
    /// z[j·nx + i] lies at origin + (i·cell_m, j·cell_m); NaN is no ground.
    ///
    /// With a `centre`, cells beyond `radiusM` of it are left out, and beyond
    /// `denseRadiusM` only the lines every `coarseSpacingM` are drawn, with
    /// edges that span the cells between (which must all hold ground too).
    static func groundMeshVertices(
        _ fields: [Gerardus_Align_V1alpha1_Heightfield], centre: SIMD2<Double>?,
        radiusM: Double = groundMeshRadiusM, denseRadiusM: Double = groundMeshDenseRadiusM,
        coarseSpacingM: Double = groundMeshCoarseSpacingM
    ) -> [Float] {
        var out = [Float]()
        for field in fields {
            let nx = Int(field.nx)
            let ny = Int(field.ny)
            let cell = Double(field.cellM)
            guard nx > 0, ny > 0, cell > 0, field.z.count >= nx * ny else { continue }
            let step = max(1, Int((coarseSpacingM / cell).rounded()))
            let origin = SIMD2(field.origin.x, field.origin.y)
            func site(_ i: Double, _ j: Double) -> SIMD2<Double> { origin + SIMD2(i, j) * cell }
            func distance(_ i: Double, _ j: Double) -> Double {
                centre.map { simd_length(site(i, j) - $0) } ?? 0
            }
            // Which cell centres are drawn at all: ground, within the radius.
            var inside = [Bool](repeating: false, count: nx * ny)
            for j in 0..<ny {
                for i in 0..<nx {
                    let z = field.z[j * nx + i]
                    inside[j * nx + i] = z.isFinite && distance(Double(i), Double(j)) <= radiusM
                }
            }
            func vertex(_ i: Int, _ j: Int) -> SIMD3<Float> {
                let p = site(Double(i), Double(j))
                return SIMD3(Float(p.x), Float(p.y), field.z[j * nx + i] + lineLift)
            }
            // An edge of `span` cells from (i, j) along (di, dj), if every
            // centre on it is drawn and its midpoint falls in the right band.
            func edge(_ i: Int, _ j: Int, _ di: Int, _ dj: Int, span: Int, dense: Bool) {
                let ei = i + di * span
                let ej = j + dj * span
                guard ei < nx, ej < ny else { return }
                let mid = distance(Double(i + ei) / 2, Double(j + ej) / 2)
                // Without a centre every distance is 0: all of it is dense.
                guard dense == (mid <= denseRadiusM) else { return }
                for k in 0...span where !inside[(j + dj * k) * nx + i + di * k] { return }
                appendSegment(&out, vertex(i, j), vertex(ei, ej), groundMeshColour)
            }
            for j in 0..<ny {
                for i in 0..<nx {
                    guard inside[j * nx + i] else { continue }
                    edge(i, j, 1, 0, span: 1, dense: true)
                    edge(i, j, 0, 1, span: 1, dense: true)
                    if centre != nil, i % step == 0, j % step == 0 {
                        edge(i, j, 1, 0, span: step, dense: false)
                        edge(i, j, 0, 1, span: step, dense: false)
                    }
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

    /// Each polyline as a list of segments, closing a closed one. A facade
    /// trace with a height also gets its top edge and an upright at each end,
    /// so it stands as high as the scans saw the wall.
    static func lineVertices(_ lines: [Gerardus_Align_V1alpha1_LineFeature]) -> [Float] {
        var out = [Float]()
        for line in lines {
            let count = line.xyz.count / 3
            guard count >= 2 else { continue }
            let colour = lineColour(line.kind)
            let raise = lift(line.kind)
            func vertex(_ i: Int, up: Float = 0) -> SIMD3<Float> {
                SIMD3(line.xyz[i * 3], line.xyz[i * 3 + 1], line.xyz[i * 3 + 2] + raise + up)
            }
            appendPolyline(&out, line, colour: colour)
            if line.kind == .facade, line.heightM > 0 {
                let h = line.heightM
                for i in 0..<(count - 1) {
                    appendSegment(&out, vertex(i, up: h), vertex(i + 1, up: h), colour)
                }
                appendSegment(&out, vertex(0), vertex(0, up: h), colour)
                appendSegment(&out, vertex(count - 1), vertex(count - 1, up: h), colour)
            }
        }
        return out
    }

    /// Box walls as given: each a closed vertical rectangle, closed back to
    /// its first vertex. Its `height_m` is the roof, already in the
    /// rectangle, so nothing is added on top.
    static func boxVertices(_ walls: [Gerardus_Align_V1alpha1_LineFeature]) -> [Float] {
        var out = [Float]()
        for wall in walls { appendPolyline(&out, wall, colour: boxWallColour(wall)) }
        return out
    }

    /// Wall planes as given, closed, in their role's colour. Unlike a facade
    /// trace a plane is already the whole rectangle: no top edge or uprights.
    static func wallPlaneVertices(_ planes: [Gerardus_Align_V1alpha1_LineFeature]) -> [Float] {
        var out = [Float]()
        for plane in planes {
            appendPolyline(&out, plane, colour: wallPlaneColour(wallPlaneRole(plane)))
        }
        return out
    }

    /// A polyline's segments at its kind's lift, closing a closed one,
    /// moved by `offset`.
    private static func appendPolyline(
        _ out: inout [Float], _ line: Gerardus_Align_V1alpha1_LineFeature, colour: SIMD4<Float>,
        offset: SIMD3<Float> = .zero
    ) {
        let count = line.xyz.count / 3
        guard count >= 2 else { return }
        let raise = lift(line.kind)
        func vertex(_ i: Int) -> SIMD3<Float> {
            SIMD3(line.xyz[i * 3], line.xyz[i * 3 + 1], line.xyz[i * 3 + 2] + raise) + offset
        }
        for i in 0..<(count - 1) { appendSegment(&out, vertex(i), vertex(i + 1), colour) }
        if line.closed { appendSegment(&out, vertex(count - 1), vertex(0), colour) }
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
