//
//  AnnotationObjectFlowTests.swift
//  VelocityVisualiserTests
//
//  Tests for following one object through a pack: what "ground" means, the
//  order an operator creates and saves an object in, the sphere as a paint
//  brush, carrying a selection to the next sample and nudging it, applying a
//  fixed object's mask to every sample, and the colours each state is drawn in.
//

import AppKit
import Foundation
import SwiftUI
import Testing
import simd

@testable import VelocityVisualiser

// MARK: - A pack made to order

/// Writes a digest-valid pack from point lists, one per sample. The shared
/// fixture is two tiny samples of fixed bytes; following an object needs a
/// cluster that moves by a known amount between samples.
enum SyntheticPack {
    typealias Point = (x: Float, y: Float, z: Float, classification: UInt8)

    /// A settled-background snapshot: the recording position it was taken at
    /// (sample `n` is at position `2n + 1`, so a snapshot can fall between any
    /// two), and its points.
    typealias Backdrop = (ordinal: Int, points: [simd_float3])

    static func write(
        _ samples: [[Point]], heightBand: String? = nil, backdrops: [Backdrop] = []
    ) throws -> URL {
        var bytes = Data()
        var entries: [String] = []
        for (id, points) in samples.enumerated() {
            let offset = bytes.count
            func append(_ values: [Float]) {
                for value in values {
                    withUnsafeBytes(of: value.bitPattern.littleEndian) { bytes.append(contentsOf: $0) }
                }
            }
            append(points.map(\.x))
            append(points.map(\.y))
            append(points.map(\.z))
            bytes.append(contentsOf: [UInt8](repeating: 100, count: points.count))
            bytes.append(contentsOf: points.map(\.classification))
            entries.append(
                """
                {"sample_id": \(id), "source_ordinal": \(2 * id + 1), "source_frame_id": \(100 + id), \
                "timestamp_ns": \(1_000_000_000 + id * 100_000_000), "sensor_id": "synthetic", \
                "point_count": \(points.count), "byte_offset": \(offset)}
                """)
        }
        let samplesJSON = "[" + entries.joined(separator: ",") + "]"
        let pointsSHA = AnnotationPack.digest(bytes)
        let samplesSHA = AnnotationPack.digest(Data(samplesJSON.utf8))
        let packDigest = AnnotationPack.digest(Data((pointsSHA + "\n" + samplesSHA).utf8))
        let band = heightBand.map { ", \"height_band\": \($0)" } ?? ""

        var backgroundBytes = Data()
        var backgroundEntries: [String] = []
        for (id, backdrop) in backdrops.enumerated() {
            let offset = backgroundBytes.count
            for axis in [backdrop.points.map(\.x), backdrop.points.map(\.y), backdrop.points.map(\.z)] {
                for value in axis {
                    withUnsafeBytes(of: value.bitPattern.littleEndian) {
                        backgroundBytes.append(contentsOf: $0)
                    }
                }
            }
            backgroundEntries.append(
                """
                {"background_id": \(id), "source_ordinal": \(backdrop.ordinal), "timestamp_ns": 999, \
                "sequence_number": 0, "settling_complete": true, \
                "point_count": \(backdrop.points.count), "byte_offset": \(offset)}
                """)
        }
        let backgroundsJSON = "[" + backgroundEntries.joined(separator: ",") + "]"
        let backgroundKeys =
            backdrops.isEmpty
            ? ""
            : """
            , "background_count": \(backdrops.count),
             "backgrounds_sha256": "\(AnnotationPack.digest(Data(backgroundsJSON.utf8)))",
             "background_points_sha256": "\(AnnotationPack.digest(backgroundBytes))"
            """
        let manifest = """
            {"schema_version": 1, "dataset_id": "ds_synthetic", "created_ns": 0,
             "source": {"vrlog_path": "synthetic.vrlog", "vrlog_header_sha256": "sha256:aa",
                        "vrlog_frames_sha256": "sha256:bb", "sensor_id": "synthetic"\(band)},
             "coordinate": {"units": "metres", "frame_id": "sensor", "reference_frame": "site",
                            "handedness": "right", "origin_note": "sensor origin",
                            "transform_version": ""},
             "coverage": "foreground_only", "sample_count": \(samples.count),
             "point_count": \(samples.map(\.count).reduce(0, +)),
             "points_sha256": "\(pointsSHA)", "samples_sha256": "\(samplesSHA)",
             "pack_digest": "\(packDigest)", "has_intensity": true, "has_classification": true\(backgroundKeys)}
            """
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(
            "synthetic-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        try Data(manifest.utf8).write(to: dir.appendingPathComponent("manifest.json"))
        try Data((samplesJSON + "\n").utf8).write(to: dir.appendingPathComponent("samples.json"))
        try bytes.write(to: dir.appendingPathComponent("points.bin"))
        if !backdrops.isEmpty {
            try Data((backgroundsJSON + "\n").utf8).write(
                to: dir.appendingPathComponent("backgrounds.json"))
            try backgroundBytes.write(to: dir.appendingPathComponent("background.bin"))
        }
        return dir
    }

    /// A 3 x 3 x 2 block of foreground returns 0.3 m apart, a metre above the
    /// sensor's horizontal, with its low corner at `origin`.
    static func car(at origin: simd_float2) -> [Point] {
        var points: [Point] = []
        for i in 0..<3 {
            for j in 0..<3 {
                for k in 0..<2 {
                    points.append(
                        (origin.x + Float(i) * 0.3, origin.y + Float(j) * 0.3, -1 + Float(k) * 0.3, 1))
                }
            }
        }
        return points
    }

    /// A wall that is in the same place in every sample, far from the car.
    static let wall: [Point] = (0..<6).map { (20 + Float($0) * 0.3, 20, -0.5, 1) }
}

@MainActor
private func openSession(_ samples: [[SyntheticPack.Point]], heightBand: String? = nil) throws -> (
    AnnotationSession, URL
) {
    let dir = try SyntheticPack.write(samples, heightBand: heightBand)
    let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
    session.operatorName = "dd"
    return (session, dir)
}

private let everything = SelectionPolygon(
    rectFrom: simd_float2(-100, -100), to: simd_float2(100, 100))
/// Round the car wherever it has got to in these tests, and never the wall.
private let roundTheCar = SelectionPolygon(rectFrom: simd_float2(-1, -1), to: simd_float2(12, 12))

// MARK: - Ground

struct HeightBandTests {
    @Test func theComparisonsAreStrictAsTheFiltersAre() {
        let band = HeightBand(floorM: -2.8, ceilingM: 1.5, removeGround: true)
        #expect(band.removes(z: -2.9))
        #expect(band.removes(z: 1.6))
        // Exactly on a bound was kept: Go's filter tests `Z < floor` and
        // `Z > ceiling`.
        #expect(!band.removes(z: -2.8))
        #expect(!band.removes(z: 1.5))
        #expect(!band.removes(z: 0))
    }

    @Test func aRunThatRemovedNothingRemovesNothing() {
        let band = HeightBand(floorM: -2.8, ceilingM: 1.5, removeGround: false)
        #expect(!band.removes(z: -5))
        #expect(!band.removes(z: 5))
    }

    @Test func whatTheBandRemovedIsGroundWhateverTheRecorderCalledIt() {
        // The recorder classes a return before L4 runs, so a foreground return
        // under the floor is recorded as foreground and never clustered.
        let points = PackPoints(
            x: [0, 0, 0], y: [0, 0, 0], z: [-3, 0, 2], intensity: [0, 0, 0],
            classification: [1, 1, 0])
        let classes = PointClass.displayClasses(
            points: points, hasClassification: true, band: .pipelineDefault)
        #expect(classes == [PointClass.ground, PointClass.foreground, PointClass.ground])
    }

    @Test func aPackWithoutClassesIsStillFilteredByHeight() {
        let points = PackPoints(
            x: [0, 0], y: [0, 0], z: [-3, 0], intensity: [0, 0], classification: [0, 0])
        let classes = PointClass.displayClasses(
            points: points, hasClassification: false, band: .pipelineDefault)
        // Its zero class bytes are padding, not a claim of background.
        #expect(classes == [PointClass.ground, PointClass.unclassified])
        #expect(PointVisibility(background: false, foreground: false, ground: false).shows(classes[1]))
    }
}

@MainActor
struct SessionHeightBandTests {
    @Test func thePacksOwnBandIsUsedAndNotFlaggedAsAssumed() throws {
        let (session, dir) = try openSession(
            [[(0, 0, -1.5, 1), (0, 0, 0, 1)]],
            heightBand: #"{"floor_m": -1.0, "ceiling_m": 0.5, "remove_ground": true}"#)
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(session.heightBand == HeightBand(floorM: -1.0, ceilingM: 0.5, removeGround: true))
        #expect(!session.heightBandIsAssumed)
        // -1.5 is above the default floor and below this run's.
        #expect(session.currentClasses == [PointClass.ground, PointClass.foreground])
        #expect(session.classCounts[PointClass.ground] == 1)
    }

    @Test func aPackThatRecordsNoBandGetsTheDefaultAndSaysSo() throws {
        let (session, dir) = try openSession([[(0, 0, -1.5, 1), (0, 0, -3, 1)]])
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(session.heightBand == .pipelineDefault)
        #expect(session.heightBandIsAssumed)
        #expect(session.currentClasses == [PointClass.foreground, PointClass.ground])
    }

    @Test func hidingGroundLeavesExactlyWhatTheClustererSaw() throws {
        let (session, dir) = try openSession([[(0, 0, -3, 1), (0, 0, 0, 1), (0, 0, 2, 1)]])
        defer { try? FileManager.default.removeItem(at: dir) }

        session.visibility.ground = false
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.canonicalSelection == [1])
    }
}

// MARK: - Creating and saving an object

@MainActor
struct ObjectOrderTests {
    @Test func namingWhatIsSelectedDoesNotThrowTheSelectionAway() throws {
        let (session, dir) = try openSession([SyntheticPack.car(at: .zero)])
        defer { try? FileManager.default.removeItem(at: dir) }

        // Select first, then say what it is: the natural order. Creating the
        // object used to reload its (empty) saved mask over the selection.
        #expect(session.select(polygon: everything, mode: .replace))
        _ = session.createObject(objectClass: "car")

        #expect(session.selectionCount == 18)
        #expect(session.savedSelection.isEmpty)
        #expect(session.save())
        #expect(session.savedSelection.count == 18)
        #expect(session.navigationGuard() == nil)
        #expect(
            FileManager.default.fileExists(
                atPath: dir.appendingPathComponent("annotations.json").path))
    }

    @Test func switchingObjectsIsRefusedWithUnsavedChanges() throws {
        let (session, dir) = try openSession([SyntheticPack.car(at: .zero)])
        defer { try? FileManager.default.removeItem(at: dir) }
        let first = session.createObject(objectClass: "car")
        let second = session.createObject(objectClass: "pedestrian")
        #expect(session.select(polygon: everything, mode: .replace))

        #expect(session.activate(objectID: first.objectID) != nil)
        #expect(session.activeObjectID == second.objectID)
        #expect(session.selectionCount == 18)
    }

    @Test func whatIsAlreadyLabelledIsShownWhileLabellingSomethingElse() throws {
        let (session, dir) = try openSession([SyntheticPack.car(at: .zero) + SyntheticPack.wall])
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())

        _ = session.createObject(objectClass: "building")

        #expect(session.otherMasks.count == 1)
        #expect(session.otherMasks.first?.objectClass == "car")
        #expect(session.otherMasks.first?.indices.count == 18)
    }

    @Test func theNextStepIsAlwaysSaid() throws {
        let (session, dir) = try openSession([SyntheticPack.car(at: .zero)])
        defer { try? FileManager.default.removeItem(at: dir) }

        session.operatorName = ""
        #expect(session.nextStep?.contains("name") == true)
        session.operatorName = "dd"
        #expect(session.nextStep?.contains("New object") == true)
        _ = session.createObject(objectClass: "car")
        #expect(session.nextStep?.contains("Select") == true)
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.nextStep?.contains("Save") == true)
        #expect(session.save())
        #expect(session.nextStep == nil)
    }

    @Test func aRefusedSaveSaysSoWhereItCanBeSeen() throws {
        let (session, dir) = try openSession([SyntheticPack.car(at: .zero)])
        defer { try? FileManager.default.removeItem(at: dir) }
        session.operatorName = "  "
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: everything, mode: .replace))

        #expect(!session.save())
        #expect(session.lastError?.contains("Labelled by") == true)
    }

    @Test func theOperatorsNameIsRememberedBetweenPacks() throws {
        let suite = "annotation-tests-\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let dir = try SyntheticPack.write([SyntheticPack.car(at: .zero)])
        defer { try? FileManager.default.removeItem(at: dir) }

        let first = try AnnotationSession(
            pack: try AnnotationPack.open(directory: dir), defaults: defaults)
        #expect(first.operatorName.isEmpty)
        first.operatorName = "dd"

        let second = try AnnotationSession(
            pack: try AnnotationPack.open(directory: dir), defaults: defaults)
        #expect(second.operatorName == "dd")
    }
}

// MARK: - Sphere brush

@MainActor
struct SpherePaintTests {
    /// Three returns in a line along x at one height, and one directly above
    /// the first, in the top view's line of sight.
    private let line: [SyntheticPack.Point] = [
        (0, 0, -1, 1), (1, 0, -1, 1), (2, 0, -1, 1), (0, 0, 1, 1),
    ]

    @Test func aStrokeTakesEverythingTheBrushPassedOver() throws {
        let (session, dir) = try openSession([line])
        defer { try? FileManager.default.removeItem(at: dir) }
        session.tool = .sphere
        session.sphereRadius = 0.3
        // The slab keeps the brush on the lower returns.
        session.setSlab(DepthSlab(minDepth: 0.5, maxDepth: 1.5))

        session.beginStroke()
        for x: Float in [0, 1, 2] {
            session.paint(
                sphere: session.brushSphere(atViewPoint: simd_float2(x, 0), pickDistance: 0.2))
        }
        // One position alone covers one return; the stroke covers all three.
        #expect(session.pendingCandidates?.indices == [0, 1, 2])
        session.selectionMode = .add
        #expect(session.commitSelection())
        #expect(session.canonicalSelection == [0, 1, 2])
    }

    @Test func theBrushTakesItsDepthFromTheReturnUnderTheCursor() throws {
        let (session, dir) = try openSession([line])
        defer { try? FileManager.default.removeItem(at: dir) }
        session.tool = .sphere
        session.sphereRadius = 0.3

        let overSecond = session.brushSphere(atViewPoint: simd_float2(1, 0), pickDistance: 0.2)
        #expect(abs(overSecond.centre.z + 1) < 1e-5)
        #expect(abs(overSecond.centre.x - 1) < 1e-5)

        // Over a gap it keeps the depth it had, rather than dropping to zero.
        let overNothing = session.brushSphere(atViewPoint: simd_float2(1.5, 5), pickDistance: 0.2)
        #expect(abs(overNothing.centre.z + 1) < 1e-5)
        #expect(abs(overNothing.centre.y - 5) < 1e-5)
    }

    @Test func shiftScrollMovesTheBrushAlongTheLineOfSight() throws {
        let (session, dir) = try openSession([line])
        defer { try? FileManager.default.removeItem(at: dir) }
        session.tool = .sphere
        session.sphereRadius = 0.3
        session.hover(atViewPoint: simd_float2(1, 0), pickDistance: 0.2)
        let before = try #require(session.hoverSphere)

        // The top view looks down, so away from the viewer is down.
        session.adjustBrushDepth(steps: 5)
        let after = try #require(session.hoverSphere)
        #expect(abs(after.centre.z - (before.centre.z - 0.5)) < 1e-5)
        #expect(abs(session.brushDepthOffset - 0.5) < 1e-6)
        #expect(session.hoverIndices.isEmpty)

        session.resetBrushDepth()
        #expect(session.brushDepthOffset == 0)
    }

    @Test func hoveringShowsWhatAClickWouldTakeAndOnlyForTheSphere() throws {
        let (session, dir) = try openSession([line])
        defer { try? FileManager.default.removeItem(at: dir) }
        session.sphereRadius = 0.3

        session.tool = .lasso
        session.hover(atViewPoint: simd_float2(1, 0), pickDistance: 0.2)
        #expect(session.hoverSphere == nil)

        session.tool = .sphere
        session.hover(atViewPoint: simd_float2(1, 0), pickDistance: 0.2)
        #expect(session.hoverIndices == [1])

        // Nothing is selected by looking.
        #expect(session.selectionCount == 0)
        session.hover(atViewPoint: nil, pickDistance: 0.2)
        #expect(session.hoverSphere == nil)
        #expect(session.hoverIndices.isEmpty)
    }
}

// MARK: - Footprint

struct SelectionFootprintTests {
    private func points(_ list: [simd_float3]) -> PackPoints {
        PackPoints(
            x: list.map(\.x), y: list.map(\.y), z: list.map(\.z),
            intensity: [UInt8](repeating: 0, count: list.count),
            classification: [UInt8](repeating: 1, count: list.count))
    }

    @Test func aFootprintCoversTheSpaceRoundItsPointsAndNoFurther() {
        let footprint = SelectionFootprint(points: [simd_float3(0.1, 0.1, 0.1)])
        // One voxel of growth each way: the next scan never lands exactly
        // where this one did.
        #expect(footprint.contains(simd_float3(0.3, 0.1, 0.1), offset: .zero))
        #expect(footprint.contains(simd_float3(-0.2, -0.2, -0.2), offset: .zero))
        #expect(!footprint.contains(simd_float3(0.6, 0.1, 0.1), offset: .zero))
        #expect(SelectionFootprint(points: []).isEmpty)
    }

    @Test func movedFootprintSelectsTheMovedPoints() {
        let footprint = SelectionFootprint(points: [simd_float3(0.1, 0.1, 0.1)])
        let scan = points([simd_float3(0.1, 0.1, 0.1), simd_float3(3.1, 0.1, 0.1)])
        #expect(footprint.indices(in: scan, offset: .zero) { _ in true } == [0])
        #expect(footprint.indices(in: scan, offset: simd_float3(3, 0, 0)) { _ in true } == [1])
        // What is hidden is not proposed.
        #expect(footprint.indices(in: scan, offset: .zero) { _ in false }.isEmpty)
    }

    @Test func theSearchFindsWhereTheObjectWent() {
        let car = SyntheticPack.car(at: .zero).map { simd_float3($0.x, $0.y, $0.z) }
        let footprint = SelectionFootprint(points: car)
        let moved = points(car.map { $0 + simd_float3(1.0, 0.5, 0) })

        let offset = footprint.bestOffset(in: moved, near: .zero) { _ in true }
        #expect(footprint.indices(in: moved, offset: offset) { _ in true }.count == car.count)
        #expect(abs(offset.x - 1.0) <= 0.25)
        #expect(abs(offset.y - 0.5) <= 0.25)
        // Road users move on the ground: the search never lifts a footprint.
        #expect(offset.z == 0)
    }

    @Test func withNothingToFindTheFootprintStaysWhereItWasPredicted() {
        let footprint = SelectionFootprint(points: [simd_float3(0, 0, 0)])
        let prediction = simd_float3(1, 2, 0)
        let empty = points([simd_float3(50, 50, 0)])
        #expect(footprint.bestOffset(in: empty, near: prediction) { _ in true } == prediction)
    }
}

// MARK: - Carrying a selection

@MainActor
struct CarriedSelectionTests {
    private func drive(_ positions: [simd_float2]) -> [[SyntheticPack.Point]] {
        positions.map { SyntheticPack.car(at: $0) + SyntheticPack.wall }
    }

    @Test func theSelectionIsLaidOverTheNextSampleWhereTheCarWent() throws {
        let (session, dir) = try openSession(drive([.zero, simd_float2(1.0, 0.5)]))
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())

        #expect(session.stepForward() == nil)

        // A proposal, not a membership: nothing is selected until accepted.
        let carried = try #require(session.carried)
        #expect(carried.direction == 1)
        #expect(session.carriedIndices == Array(0..<18))
        #expect(session.selectionCount == 0)
        #expect(session.navigationGuard() == nil)

        #expect(session.acceptCarried())
        #expect(session.canonicalSelection == Array(0..<18))
        #expect(session.carried == nil)
        // Accepted is not saved.
        #expect(session.savedSelection.isEmpty)
        #expect(session.navigationGuard() != nil)
    }

    @Test func aNudgeMovesTheProposalInTheViewsOwnDirections() throws {
        // Three metres a sample: further than the search reaches.
        let (session, dir) = try openSession(drive([.zero, simd_float2(3, 0)]))
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())
        session.stepForward()
        #expect(session.carriedIndices.count < 18)

        // Right in the top view is +x.
        let start = try #require(session.carried).offset
        session.nudgeCarried(right: 3 - start.x, up: -start.y)
        #expect(session.carriedIndices == Array(0..<18))

        // Up in the front view is +z, which lifts the footprint off the car.
        session.viewStandard = .front
        session.nudgeCarried(right: 0, up: 2)
        #expect(session.carriedIndices.isEmpty)
        #expect(abs(try #require(session.carried).offset.z - 2) < 1e-5)
    }

    @Test func theNextCarryStartsFromHowFarTheLastOneMoved() throws {
        let (session, dir) = try openSession(
            drive([.zero, simd_float2(3, 0), simd_float2(6, 0)]))
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())

        // First step: out of the search's reach, so the operator nudges.
        session.stepForward()
        let start = try #require(session.carried).offset
        session.nudgeCarried(right: 3 - start.x, up: -start.y)
        #expect(session.acceptCarried())
        #expect(session.save())

        // Second step: predicted from the first, and found without help.
        session.stepForward()
        #expect(session.carriedIndices == Array(0..<18))
    }

    @Test func aBuildingIsCarriedWhereItIsAndNotSearchedFor() throws {
        // In the second sample half the wall is occluded, and a metre along
        // there is something wall-shaped with every return present. A search
        // would prefer it: six returns against three. A building has not
        // moved, so it is not searched for.
        let halfWall = Array(SyntheticPack.wall.prefix(3))
        let decoy: [SyntheticPack.Point] = SyntheticPack.wall.map { ($0.x, $0.y + 1, $0.z, 1) }
        let (session, dir) = try openSession([SyntheticPack.wall, halfWall + decoy])
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "building")
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.save())

        session.stepForward()
        #expect(try #require(session.carried).offset == .zero)
        #expect(session.carriedIndices == [0, 1, 2])
    }

    @Test func aCarInTheSameSceneIsSearchedFor() throws {
        // The same two samples, labelled as a car: the search does go to where
        // the most returns are. This is what the test above is the contrast to.
        let halfWall = Array(SyntheticPack.wall.prefix(3))
        let decoy: [SyntheticPack.Point] = SyntheticPack.wall.map { ($0.x, $0.y + 1, $0.z, 1) }
        let (session, dir) = try openSession([SyntheticPack.wall, halfWall + decoy])
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.save())

        session.stepForward()
        #expect(try #require(session.carried).offset != .zero)
        #expect(session.carriedIndices.count == 6)
    }

    @Test func nothingIsCarriedOntoASampleAlreadyLabelledOrAcrossAJump() throws {
        let (session, dir) = try openSession(
            drive([.zero, simd_float2(1, 0), simd_float2(2, 0)]))
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())

        // Two samples on: the footprint is somewhere the car no longer is.
        session.step(to: 2)
        #expect(session.carried == nil)

        // Back to a sample that has its mask: nothing to propose.
        session.step(to: 1)
        session.dismissCarried()
        session.step(to: 0)
        #expect(session.carried == nil)
        #expect(session.selectionCount == 18)
    }

    @Test func selectingByHandReplacesTheProposal() throws {
        let (session, dir) = try openSession(drive([.zero, simd_float2(1, 0)]))
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())
        session.stepForward()
        #expect(session.carried != nil)

        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.carried == nil)
        #expect(session.carriedIndices.isEmpty)
    }
}

// MARK: - Fixed objects

@MainActor
struct ApplyToEverySampleTests {
    private let roundTheWall = SelectionPolygon(
        rectFrom: simd_float2(19, 19), to: simd_float2(23, 21))

    private func street() -> [[SyntheticPack.Point]] {
        [simd_float2.zero, simd_float2(1, 0), simd_float2(2, 0)].map {
            SyntheticPack.car(at: $0) + SyntheticPack.wall
        }
    }

    @Test func aBuildingIsSavedIntoEverySampleAndSaysHowItGotThere() throws {
        let (session, dir) = try openSession(street())
        defer { try? FileManager.default.removeItem(at: dir) }
        let building = session.createObject(objectClass: "building")
        #expect(session.select(polygon: roundTheWall, mode: .replace))

        #expect(session.applySelectionToAllSamples() == 3)

        #expect(session.savedSampleCount(objectID: building.objectID) == 3)
        #expect(session.navigationGuard() == nil)
        for sampleID in 0..<3 {
            let mask = try #require(
                session.sidecar.mask(objectID: building.objectID, sampleID: sampleID))
            #expect(mask.pointIndices == Array(18..<24))
            #expect(mask.status == .proposed)
            // The sample the operator selected in is theirs; the rest were
            // propagated, and a later reader has to be able to tell.
            #expect(mask.provenance.algorithm == (sampleID == 0 ? nil : "footprint_carry"))
        }
    }

    @Test func aSampleAlreadyMaskedIsLeftAsItWas() throws {
        let (session, dir) = try openSession(street())
        defer { try? FileManager.default.removeItem(at: dir) }
        let building = session.createObject(objectClass: "building")
        // Half the wall, saved by hand in sample 1.
        session.step(to: 1)
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(19, 19), to: simd_float2(20.4, 21)),
                mode: .replace))
        #expect(session.save())
        session.step(to: 0)
        session.dismissCarried()
        #expect(session.select(polygon: roundTheWall, mode: .replace))

        #expect(session.applySelectionToAllSamples() == 2)
        #expect(
            session.sidecar.mask(objectID: building.objectID, sampleID: 1)?.pointIndices
                == [18, 19])
    }

    @Test func aCarCannotBeAppliedToEverySample() throws {
        let (session, dir) = try openSession(street())
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))

        #expect(session.applySelectionToAllSamples() == nil)
        #expect(session.lastError?.contains("does not move") == true)
        #expect(session.sidecar.masks.isEmpty)
    }
}

// MARK: - Colours and keys

struct AnnotationPaletteTests {
    @Test func carsPedestriansAndBuildingsAreToldApart() {
        let names = ["car", "pedestrian", "building"]
        let indices = Set(names.map(AnnotationPalette.paletteIndex(forClass:)))
        #expect(indices.count == 3)
        // And none of them is a colour that means an edit state.
        #expect(
            indices.isDisjoint(with: [
                AnnotationPalette.unsavedIndex, AnnotationPalette.candidateIndex,
                AnnotationPalette.removedIndex,
            ]))
        #expect(AnnotationPalette.isFixed("building"))
        #expect(!AnnotationPalette.isFixed("car"))
        #expect(
            AnnotationPalette.paletteIndex(forClass: "hovercraft")
                == AnnotationPalette.unknownClassIndex)
    }

    @Test func everyClassHasAColourOfItsOwn() {
        let indices = AnnotationPalette.classes.map(\.paletteIndex)
        #expect(Set(indices).count == indices.count)
        #expect(indices.allSatisfy { $0 < AnnotationPalette.colours.count })
    }

    @Test func theShadersTableIsThisTable() throws {
        var dir = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
        var shader: String?
        for _ in 0..<6 {
            let candidate = dir.appendingPathComponent(
                "VelocityVisualiser/Rendering/Shaders/PointCloud.metal")
            if let text = try? String(contentsOf: candidate, encoding: .utf8) {
                shader = text
                break
            }
            dir = dir.deletingLastPathComponent()
        }
        let source = try #require(shader, "could not locate PointCloud.metal")

        #expect(
            source.contains("#define ANNOTATION_PALETTE_COUNT \(AnnotationPalette.colours.count)"))
        for (index, colour) in AnnotationPalette.colours.enumerated() {
            let line = String(
                format: "float3(%.2f, %.2f, %.2f), // %d ", colour.x, colour.y, colour.z, index)
            #expect(source.contains(line), "the shader's palette entry \(index) is not \(line)")
        }
        #expect(AnnotationPalette.shaderClass(0) == 16)
    }
}

struct ViewportKeyTests {
    private func key(_ code: UInt16, shift: Bool = false) throws -> NSEvent {
        try #require(
            NSEvent.keyEvent(
                with: .keyDown, location: .zero, modifierFlags: shift ? [.shift] : [],
                timestamp: 0, windowNumber: 0, context: nil, characters: "",
                charactersIgnoringModifiers: "", isARepeat: false, keyCode: code))
    }

    @Test func arrowsAreDirectionsInTheViewAndShiftIsCoarse() throws {
        #expect(
            ViewportInputView.viewportKey(for: try key(123)) == .nudge(right: -1, up: 0, coarse: false))
        #expect(
            ViewportInputView.viewportKey(for: try key(124)) == .nudge(right: 1, up: 0, coarse: false))
        #expect(
            ViewportInputView.viewportKey(for: try key(126, shift: true))
                == .nudge(right: 0, up: 1, coarse: true))
        #expect(
            ViewportInputView.viewportKey(for: try key(125)) == .nudge(right: 0, up: -1, coarse: false))
        #expect(ViewportInputView.viewportKey(for: try key(36)) == .accept)
        #expect(ViewportInputView.viewportKey(for: try key(53)) == .cancel)
        #expect(ViewportInputView.viewportKey(for: try key(0)) == nil)
    }

    @Test func undoAndRedoAreBoundToTheSelectionHistory() throws {
        var dir = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
        var app: String?
        for _ in 0..<6 {
            let candidate = dir.appendingPathComponent(
                "VelocityVisualiser/App/VelocityVisualiserApp.swift")
            if let text = try? String(contentsOf: candidate, encoding: .utf8) {
                app = text
                break
            }
            dir = dir.deletingLastPathComponent()
        }
        let source = try #require(app)
        #expect(source.contains("CommandGroup(replacing: .undoRedo)"))
        #expect(source.contains("annotationSession.undo()"))
        #expect(source.contains("annotationSession.redo()"))
        // A text field being edited keeps its own undo.
        #expect(source.contains("AppCommands.textHasFocus"))
    }
}
