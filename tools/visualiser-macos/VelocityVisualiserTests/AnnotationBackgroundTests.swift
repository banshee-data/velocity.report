//
//  AnnotationBackgroundTests.swift
//  VelocityVisualiserTests
//
//  Tests for the settled background behind a frame, how much of a frame is
//  labelled and where, the frame strip, and the names objects go by.
//

import CoreGraphics
import Foundation
import Testing
import simd

@testable import VelocityVisualiser

@MainActor
private func openSession(
    _ samples: [[SyntheticPack.Point]], backdrops: [SyntheticPack.Backdrop] = []
) throws -> (AnnotationSession, URL) {
    let dir = try SyntheticPack.write(samples, backdrops: backdrops)
    let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
    session.operatorName = "dd"
    return (session, dir)
}

private let everything = SelectionPolygon(
    rectFrom: simd_float2(-100, -100), to: simd_float2(100, 100))

// MARK: - Reading the background

struct BackgroundPackTests {
    /// The bytes and digests Go's writer produces for this snapshot, pinned in
    /// `internal/lidar/annotation/swift_fixture_test.go`. The layout is written
    /// by hand on both sides, so it is held to the same bytes on both.
    private let goBytes = "AADAPwAAAMAAAEBAAACIQAAAAL8AAMBA"
    private let goPointsSHA =
        "sha256:9e890764a8e9cc124c1296e1c8fdff379ee970f840df9c6d3d2e886d0eb86239"
    private let goIndexSHA =
        "sha256:691fdcc25726fbf8872126b9252ae21c20d8fffb466cf1c23508a78298355529"
    private let goIndexJSON = """
        [
          {
            "background_id": 0,
            "source_ordinal": 0,
            "timestamp_ns": 999,
            "sequence_number": 0,
            "settling_complete": true,
            "point_count": 2,
            "byte_offset": 0
          }
        ]
        """

    @Test func readsTheBytesGoWrites() throws {
        let dir = try SyntheticPack.write([[(0, 0, 0, 1)]])
        defer { try? FileManager.default.removeItem(at: dir) }
        // Swap in Go's own background files and point the manifest at them.
        try Data(base64Encoded: goBytes)!.write(to: dir.appendingPathComponent("background.bin"))
        try Data((goIndexJSON + "\n").utf8).write(
            to: dir.appendingPathComponent("backgrounds.json"))
        let manifestURL = dir.appendingPathComponent("manifest.json")
        var manifest = try String(contentsOf: manifestURL, encoding: .utf8)
        manifest = manifest.replacingOccurrences(
            of: "\"has_classification\": true",
            with: """
                "has_classification": true, "background_count": 1, \
                "backgrounds_sha256": "\(goIndexSHA)", "background_points_sha256": "\(goPointsSHA)"
                """)
        try Data(manifest.utf8).write(to: manifestURL)

        let pack = try AnnotationPack.open(directory: dir)
        let background = try #require(pack.backgrounds.first)
        #expect(background.settlingComplete)
        let points = pack.backgroundPoints(background)
        #expect(points.x == [1.5, -2])
        #expect(points.y == [3, 4.25])
        #expect(points.z == [-0.5, 6])
    }

    @Test func aTamperedBackgroundIsRefused() throws {
        let dir = try SyntheticPack.write(
            [[(0, 0, 0, 1)]], backdrops: [(0, [simd_float3(1, 2, 3)])])
        defer { try? FileManager.default.removeItem(at: dir) }
        let url = dir.appendingPathComponent("background.bin")
        var bytes = try Data(contentsOf: url)
        bytes[0] ^= 0xFF
        try bytes.write(to: url)

        #expect(throws: AnnotationPackError.self) { try AnnotationPack.open(directory: dir) }
    }

    @Test func theSnapshotInForceIsTheLastRecordedBeforeTheFrame() throws {
        // Samples are at recording positions 1, 3, 5. Snapshots at 0 and 4.
        let dir = try SyntheticPack.write(
            [[(0, 0, 0, 1)], [(0, 0, 0, 1)], [(0, 0, 0, 1)]],
            backdrops: [(0, [simd_float3(1, 1, 1)]), (4, [simd_float3(2, 2, 2)])])
        defer { try? FileManager.default.removeItem(at: dir) }
        let pack = try AnnotationPack.open(directory: dir)
        let samples = pack.chronologicalSamples

        #expect(pack.backgroundInForce(at: samples[0])?.backgroundID == 0)
        #expect(pack.backgroundInForce(at: samples[1])?.backgroundID == 0)
        #expect(pack.backgroundInForce(at: samples[2])?.backgroundID == 1)
    }

    @Test func aPackWithoutOneOpensAsItAlwaysDid() throws {
        let dir = try SyntheticPack.write([[(0, 0, 0, 1)]])
        defer { try? FileManager.default.removeItem(at: dir) }
        let pack = try AnnotationPack.open(directory: dir)
        #expect(pack.backgrounds.isEmpty)
        #expect(pack.backgroundInForce(at: pack.samples[0]) == nil)
    }
}

// MARK: - The background behind a frame

@MainActor
struct SessionBackgroundTests {
    private let wall = (0..<4).map { simd_float3(10, Float($0), 0) }

    private func street() throws -> (AnnotationSession, URL) {
        // The second snapshot keeps the wall, with the jitter a settled range
        // has from one snapshot to the next, and adds a parked car.
        let jittered = wall.map { $0 + simd_float3(0.03, -0.02, 0.01) }
        let parked = [simd_float3(3, 3, -1), simd_float3(3.3, 3, -1)]
        return try openSession(
            [[(0, 0, 0, 1)], [(0, 0, 0, 1)], [(0, 0, 0, 1)]],
            backdrops: [(0, wall), (4, jittered + parked)])
    }

    @Test func steppingForwardAcrossAnUpdateSaysSoAndShowsWhatChanged() throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(session.currentBackground?.backgroundID == 0)
        #expect(!session.backgroundUpdatedHere)
        #expect(session.backgroundChanged.isEmpty)

        session.stepForward()
        #expect(session.currentBackground?.backgroundID == 0)
        #expect(!session.backgroundUpdatedHere)

        session.stepForward()
        #expect(session.currentBackground?.backgroundID == 1)
        #expect(session.backgroundUpdatedHere)
        // The parked car is new. The wall only wandered by centimetres.
        #expect(session.backgroundChanged == [4, 5])
        #expect(session.backgroundPoints.count == 6)
    }

    @Test func steppingBackAcrossOneIsNotNews() throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }
        session.step(to: 2)
        session.stepBackward()

        #expect(session.currentBackground?.backgroundID == 0)
        #expect(!session.backgroundUpdatedHere)
        #expect(session.backgroundPoints.count == 4)
    }

    @Test func theBackgroundCountsAsBackgroundAndIsNeverSelected() throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(session.classCounts[PointClass.background] == 4)
        // The frame has one return of its own. A lasso over everything takes
        // that and nothing from the snapshot, which has no indices to take.
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.canonicalSelection == [0])
    }

    @Test func theFrameStripMarksWhereTheBackgroundChanges() throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }
        #expect(session.frameProgress.map(\.backgroundUpdated) == [false, false, true])
    }

    @Test func the3DViewIsSentEachSnapshotOnce() throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }
        let snapshot = try #require(session.currentBackground)
        func bundle(upload: Bool) -> FrameBundle {
            AnnotationScene.frame(
                points: session.currentPoints, classes: session.currentClasses, visibility: nil,
                marks: AnnotationScene.Marks(), sample: session.currentSample,
                backdrop: AnnotationScene.Backdrop(
                    snapshot: snapshot, points: session.backgroundPoints, changed: [1],
                    upload: upload))
        }

        let first = try #require(bundle(upload: true).background)
        #expect(first.x == session.backgroundPoints.x)
        // The renderer indexes confidence by point, so it has to be as long.
        #expect(first.confidence.count == first.pointCount)
        #expect(bundle(upload: false).background == nil)

        // What changed rides with the frame's points, in its own colour.
        let cloud = try #require(bundle(upload: false).pointCloud)
        #expect(cloud.pointCount == 2)
        #expect(
            cloud.classification.last
                == AnnotationPalette.shaderClass(AnnotationPalette.backgroundChangedIndex))
    }
}

// MARK: - How much of a frame is labelled

struct FrameCompletenessTests {
    @Test func sectorsRunAnticlockwiseFromPlusX() {
        #expect(FrameCompleteness.sector(x: 1, y: 0.01) == 0)
        #expect(FrameCompleteness.sector(x: 0.01, y: 1) == 3)
        #expect(FrameCompleteness.sector(x: -1, y: 0.01) == 7)
        #expect(FrameCompleteness.sector(x: -1, y: -0.01) == 8)
        #expect(FrameCompleteness.sector(x: 1, y: -0.01) == 15)
        // Sixteen of 22.5 degrees.
        let first = FrameCompleteness.bearings(ofSector: 0)
        #expect(abs(first.upperBound - .pi / 8) < 1e-6)
    }

    @Test func onlyWhatTheTrackerCouldUseIsCounted() {
        let points = PackPoints(
            x: [1, 1, 1, -1], y: [0.1, 0.1, 0.1, 0.1], z: [0, 0, 0, 0],
            intensity: [0, 0, 0, 0], classification: [1, 1, 1, 1])
        let classes: [UInt8] = [
            PointClass.foreground, PointClass.ground, PointClass.background, PointClass.foreground,
        ]
        let tally = FrameCompleteness.tally(
            points: points, classes: classes, agreed: [0, 1], inQuestion: [])

        #expect(tally.whole.total == 2)
        #expect(tally.whole.agreed == 1)
        #expect(tally.sectors[0] == LabelTally(total: 1, agreed: 1, inQuestion: 0))
        #expect(tally.sectors[7].unlabelled == 1)
        #expect(tally.sectors[0].isComplete)
        #expect(!tally.whole.isComplete)
    }

    @Test func aReturnBothAgreedAndInQuestionIsInQuestion() {
        let points = PackPoints(
            x: [1], y: [0.1], z: [0], intensity: [0], classification: [1])
        let tally = FrameCompleteness.tally(
            points: points, classes: [PointClass.foreground], agreed: [0], inQuestion: [0])
        #expect(tally.whole.inQuestion == 1)
        #expect(tally.whole.agreed == 0)
    }

    @Test func theRingIsReadTheWayTheTopViewIs() {
        let size = CGSize(width: 100, height: 100)
        // Right of centre is +X; above centre, on screen, is +Y.
        #expect(SectorRing.sector(at: CGPoint(x: 90, y: 49), in: size) == 0)
        #expect(SectorRing.sector(at: CGPoint(x: 51, y: 10), in: size) == 3)
        #expect(SectorRing.sector(at: CGPoint(x: 90, y: 51), in: size) == 15)
        #expect(SectorRing.sector(at: CGPoint(x: 50, y: 50), in: size) == nil)
    }
}

@MainActor
struct SessionProgressTests {
    /// A car to the east and a wall to the north-east, both foreground.
    private var frame: [SyntheticPack.Point] { SyntheticPack.car(at: simd_float2(5, 0.2)) + SyntheticPack.wall }
    private let roundTheCar = SelectionPolygon(
        rectFrom: simd_float2(4, 0), to: simd_float2(7, 2))

    @Test func unsavedIsInQuestionAndSavedByHandIsAgreed() throws {
        let (session, dir) = try openSession([frame, frame])
        defer { try? FileManager.default.removeItem(at: dir) }
        #expect(session.completeness.whole == LabelTally(total: 24, agreed: 0, inQuestion: 0))

        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.completeness.whole.inQuestion == 18)
        #expect(session.completeness.sectors[0].inQuestion == 18)

        #expect(session.save())
        #expect(session.completeness.whole == LabelTally(total: 24, agreed: 18, inQuestion: 0))
        #expect(session.frameProgress[0].agreed == 18)
        #expect(session.frameProgress[0].labelable == 24)
        #expect(session.frameProgress[1].agreed == 0)
    }

    @Test func whatAnAlgorithmPropagatedStaysInQuestion() throws {
        let (session, dir) = try openSession([frame, frame])
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "building")
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(19, 19), to: simd_float2(23, 21)),
                mode: .replace))
        #expect(session.applySelectionToAllSamples() == 2)

        // The frame the operator selected in is theirs. The other was filled
        // in for them, and nobody has looked at it.
        #expect(session.frameProgress[0].agreed == 6)
        #expect(session.frameProgress[1].inQuestion == 6)
        #expect(session.frameProgress[1].agreed == 0)
    }

    @Test func aSectorTakesTheViewsToWhatIsLeftInIt() throws {
        let (session, dir) = try openSession([frame])
        defer { try? FileManager.default.removeItem(at: dir) }
        // The car is in sector 0; the wall, at (20..21.5, 20), in sector 1.
        #expect(session.fitViews(toSector: 1))
        let top = session.viewport(for: .top, size: CGSize(width: 400, height: 200))
        #expect(abs(top.centre.y - 20) < 1e-4)
        // A sector with nothing in it leaves the views where they were.
        #expect(!session.fitViews(toSector: 9))
        #expect(session.viewport(for: .top, size: CGSize(width: 400, height: 200)) == top)
    }
}

// MARK: - Names

@MainActor
struct ObjectNameTests {
    @Test func objectsAreNumberedWithinTheirClass() throws {
        let (session, dir) = try openSession([[(0, 0, 0, 1)]])
        defer { try? FileManager.default.removeItem(at: dir) }
        let first = session.createObject(objectClass: "car")
        let walker = session.createObject(objectClass: "pedestrian")
        let second = session.createObject(objectClass: "car")

        #expect(session.displayName(objectID: first.objectID) == "car 1")
        #expect(session.displayName(objectID: walker.objectID) == "pedestrian 1")
        #expect(session.displayName(objectID: second.objectID) == "car 2")
        #expect(session.activeObjectName == "car 2")
        #expect(session.nextStep?.contains("car 2") == true)
    }

    @Test func groundIsAClassForForegroundThatIsNotARoadUser() {
        // Foreground the background model failed to settle on is labelled as
        // what it is, and it does not move.
        #expect(AnnotationPalette.isFixed("ground"))
        #expect(AnnotationPalette.annotationClass(named: "ground")?.mainViewLabel == "noise")
        #expect(AnnotationPalette.annotationClass(named: "van")?.mainViewLabel == "car")
    }
}
