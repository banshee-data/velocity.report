//
//  PhysicalReferenceModelTests.swift
//  VelocityVisualiserTests
//
//  The physical-reference record against Go's: every field survives a round
//  trip, timestamps stay exact integers, unsupported documents open read-only,
//  the draft edits keep a record consistent with its evidence rules, and the
//  overlay's geometry equals what Go derives from the same keyframes.
//

import Foundation
import Testing

@testable import VelocityVisualiser

/// The repository root, from this file's own path.
private let repositoryRoot = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()

/// The fixture Go's TestPhysicalGeometrySwiftFixture writes and checks.
private func geometryFixture() throws -> Data {
    try Data(
        contentsOf: repositoryRoot.appendingPathComponent(
            "internal/lidar/annotation/testdata/physical_geometry_fixture.json"))
}

/// Go's PhysicalGeometry JSON, as the fixture spells it.
private struct GoGeometry: Decodable {
    struct Planar: Decodable {
        var x_m: Double
        var y_m: Double
        var bound_m: Double
    }
    struct Angle: Decodable {
        var rad: Double
        var bound_rad: Double
        var axis: String
        var status: String
    }
    struct Linear: Decodable {
        var lower_m: Double
        var upper_m: Double
        var value_m: Double
        var half_width_m: Double
        var status: String
    }
    struct Box: Decodable {
        var centre_x_m: Double
        var centre_y_m: Double
        var yaw_rad: Double
        var length_m: Double
        var width_m: Double
    }
    var anchor: String
    var anchor_point: Planar?
    var anchor_unavailable: String?
    var centre: Planar?
    var centre_unavailable: String?
    var yaw: Angle?
    var yaw_unavailable: String?
    var length: Linear?
    var length_unavailable: String?
    var width: Linear?
    var width_unavailable: String?
    var height: Linear?
    var height_unavailable: String?
    var front: Planar?
    var front_unavailable: String?
    var rear: Planar?
    var rear_unavailable: String?
    var ends: [Planar]?
    var box: Box?
    var box_unavailable: String?
}

private struct Fixture: Decodable {
    var objects: [PhysicalObject]
    var geometries: [String: GoGeometry]
}

private func close(_ a: Double?, _ b: Double?, _ what: String, _ id: String) {
    switch (a, b) {
    case (nil, nil): return
    case (let a?, let b?): #expect(abs(a - b) < 1e-9, "\(id) \(what): Swift \(a), Go \(b)")
    default:
        Issue.record("\(id) \(what): Swift \(String(describing: a)), Go \(String(describing: b))")
    }
}

private func samePlanar(_ s: PhysicalPlanar?, _ g: GoGeometry.Planar?, _ what: String, _ id: String)
{
    close(s?.x, g?.x_m, what + ".x", id)
    close(s?.y, g?.y_m, what + ".y", id)
    close(s?.bound, g?.bound_m, what + ".bound", id)
}

private func sameLinear(_ s: PhysicalLinear?, _ g: GoGeometry.Linear?, _ what: String, _ id: String)
{
    close(s?.lower, g?.lower_m, what + ".lower", id)
    close(s?.upper, g?.upper_m, what + ".upper", id)
    close(s?.value, g?.value_m, what + ".value", id)
    close(s?.halfWidth, g?.half_width_m, what + ".halfWidth", id)
    #expect(s?.status.rawValue == g?.status, "\(id) \(what).status")
}

struct PhysicalReferenceGeometryParityTests {
    @Test func swiftDerivesWhatGoDerivesForEveryFixtureKeyframe() throws {
        let fixture = try JSONDecoder().decode(Fixture.self, from: geometryFixture())
        var checked = 0
        for object in fixture.objects {
            for keyframe in object.keyframes {
                let id = keyframe.keyframeID
                let go = try #require(fixture.geometries[id], "no Go geometry for \(id)")
                let swift = PhysicalGeometry.derive(
                    object: object, keyframe: keyframe, gate: .truth)
                #expect(swift.anchorKind.rawValue == go.anchor, "\(id) anchor")
                samePlanar(swift.anchorPoint, go.anchor_point, "anchor_point", id)
                #expect(
                    swift.anchorUnavailable == go.anchor_unavailable, "\(id) anchor_unavailable")
                samePlanar(swift.centre, go.centre, "centre", id)
                #expect(
                    swift.centreUnavailable == go.centre_unavailable, "\(id) centre_unavailable")
                close(swift.yaw?.rad, go.yaw?.rad, "yaw.rad", id)
                close(swift.yaw?.boundRad, go.yaw?.bound_rad, "yaw.bound", id)
                #expect(swift.yaw?.axis.rawValue == go.yaw?.axis, "\(id) yaw.axis")
                #expect(swift.yawUnavailable == go.yaw_unavailable, "\(id) yaw_unavailable")
                sameLinear(swift.length, go.length, "length", id)
                sameLinear(swift.width, go.width, "width", id)
                sameLinear(swift.height, go.height, "height", id)
                #expect(
                    swift.lengthUnavailable == go.length_unavailable, "\(id) length_unavailable")
                #expect(swift.widthUnavailable == go.width_unavailable, "\(id) width_unavailable")
                #expect(
                    swift.heightUnavailable == go.height_unavailable, "\(id) height_unavailable")
                samePlanar(swift.front, go.front, "front", id)
                samePlanar(swift.rear, go.rear, "rear", id)
                #expect(swift.frontUnavailable == go.front_unavailable, "\(id) front_unavailable")
                #expect(swift.rearUnavailable == go.rear_unavailable, "\(id) rear_unavailable")
                #expect(swift.ends.count == (go.ends?.count ?? 0), "\(id) ends")
                for (s, g) in zip(swift.ends, go.ends ?? []) { samePlanar(s, g, "end", id) }
                close(swift.box?.centreX, go.box?.centre_x_m, "box.x", id)
                close(swift.box?.centreY, go.box?.centre_y_m, "box.y", id)
                close(swift.box?.yawRad, go.box?.yaw_rad, "box.yaw", id)
                close(swift.box?.length, go.box?.length_m, "box.length", id)
                close(swift.box?.width, go.box?.width_m, "box.width", id)
                #expect(swift.boxUnavailable == go.box_unavailable, "\(id) box_unavailable")
                checked += 1
            }
        }
        // Every path the fixture was built to cover was compared.
        #expect(checked == fixture.geometries.count)
        #expect(checked >= 12)
    }

    @Test func previewDrawsADraftBodyThatTruthWouldNotScore() throws {
        let fixture = try JSONDecoder().decode(Fixture.self, from: geometryFixture())
        let object = try #require(fixture.objects.first { $0.objectID == "car-4" })
        let keyframe = try #require(object.keyframes.first)
        let truth = PhysicalGeometry.derive(object: object, keyframe: keyframe, gate: .truth)
        let preview = PhysicalGeometry.derive(object: object, keyframe: keyframe, gate: .preview)
        #expect(truth.box == nil)
        #expect(truth.lengthUnavailable == PhysicalUnavailable.bodyUnreviewed)
        #expect(preview.box != nil)
        #expect(preview.length != nil)
    }

    @Test func boxCornersAreTheFootprintTurnedByYaw() {
        let box = PhysicalBox(centreX: 1, centreY: 2, yawRad: .pi / 2, length: 4, width: 2)
        let corners = box.corners
        // Front-left of a car facing +Y is at (centre.x - 1, centre.y + 2).
        #expect(abs(corners[0].x - 0) < 1e-12 && abs(corners[0].y - 4) < 1e-12)
        #expect(abs(corners[2].x - 2) < 1e-12 && abs(corners[2].y - 0) < 1e-12)
    }
}

struct PhysicalReferenceModelTests {
    @Test func everyFieldGoWritesSurvivesARoundTrip() throws {
        let data = try geometryFixture()
        let raw = try #require(
            (try JSONSerialization.jsonObject(with: data) as? [String: Any])?["objects"] as? [Any])
        let objects = try JSONDecoder().decode(
            [PhysicalObject].self, from: JSONSerialization.data(withJSONObject: raw))
        let reencoded = try JSONSerialization.jsonObject(with: JSONEncoder().encode(objects))
        #expect((reencoded as? NSArray) == (raw as NSArray))
    }

    @Test func timestampsStayExactIntegers() throws {
        let json = """
            {"keyframe_id":"k","sample_id":3,"timestamp_ns":1758000000123456789,
             "anchor":{"kind":"body_centre"},"position":{"status":"unknown","support":{}},
             "yaw":{"status":"unknown","axis":"unknown","support":{}},
             "front":{"status":"unknown","support":{}},"rear":{"status":"unknown","support":{}},
             "review":{"status":"proposed","origin":"independent","method":"manual_box",
                       "provenance":{"author":"op","created_utc":"","revision":0}}}
            """
        let k = try JSONDecoder().decode(PhysicalKeyframe.self, from: Data(json.utf8))
        #expect(k.timestampNs == 1_758_000_000_123_456_789)
        let back = try JSONSerialization.jsonObject(with: JSONEncoder().encode(k)) as? [String: Any]
        #expect((back?["timestamp_ns"] as? NSNumber)?.int64Value == 1_758_000_000_123_456_789)
    }

    @Test func onlyTheSupportedSchemaIsEditable() {
        var doc = PhysicalReferenceDocument(
            schema: PhysicalReferenceDocument.schema, schemaVersion: 1, datasetID: "d",
            packDigest: "p", revision: 1, objects: [])
        #expect(doc.editable)
        doc.schemaVersion = 2
        #expect(!doc.editable)
        doc.schemaVersion = 1
        doc.schema = "something/else"
        #expect(!doc.editable)
    }

    @Test func degreesAndRadiansConvertAndWrap() {
        #expect(abs(PhysicalUnits.radians(180) - .pi) < 1e-12)
        #expect(abs(PhysicalUnits.degrees(.pi / 2) - 90) < 1e-12)
        #expect(PhysicalUnits.wrappedDegrees(350) == -10)
        #expect(PhysicalUnits.wrappedDegrees(-190) == 170)
        #expect(PhysicalUnits.wrappedDegrees(180) == 180)
        #expect(PhysicalUnits.wrappedDegrees(-180) == 180)
        #expect(PhysicalUnits.wrappedDegrees(725) == 5)
    }

    @Test func packHandleIsTheFolderNameOrItsPackSubfolder() {
        #expect(
            PhysicalReferenceAPIClient.handle(for: URL(fileURLWithPath: "/a/packs/run-1"))
                == "run-1")
        #expect(
            PhysicalReferenceAPIClient.handle(for: URL(fileURLWithPath: "/a/packs/run-1/pack"))
                == "run-1/pack")
        #expect(
            PhysicalReferenceAPIClient.handle(for: URL(fileURLWithPath: "/a/packs/run-1/"))
                == "run-1")
    }
}

struct PhysicalDraftTests {
    private func sample(_ id: Int) -> AnnotationSample {
        AnnotationSample(
            sampleID: id, sourceOrdinal: id * 2 + 1, sourceFrameID: UInt64(100 + id),
            timestampNs: 1_000_000_000 + Int64(id) * 100_000_000, sensorID: "s", pointCount: 0,
            byteOffset: 0)
    }

    @Test func aNewBodyAndKeyframeClaimNothing() {
        var objects: [PhysicalObject] = []
        PhysicalDraft.addBody(objectID: "obj_1", author: "op", session: "s", to: &objects)
        PhysicalDraft.addKeyframe(
            objectID: "obj_1", sample: sample(2), author: "op", session: "s", to: &objects)
        let object = objects[0]
        let body = try! #require(object.body)
        #expect(
            body.length.status == .unknown && body.width.status == .unknown
                && body.height.status == .unknown)
        #expect(body.review.status == .proposed && body.review.origin == .independent)
        #expect(body.review.provenance.author == "op")
        #expect(body.bodyID.hasPrefix("body_"))
        let k = object.keyframes[0]
        #expect(k.sampleID == 2 && k.timestampNs == 1_200_000_000)
        #expect(k.position.status == .unknown && k.yaw.axis == .unknown)
        #expect(!k.declaresBounds && !body.declaresBounds)
        // Adding again at the same sample is not a second keyframe.
        PhysicalDraft.addKeyframe(
            objectID: "obj_1", sample: sample(2), author: "op", session: "s", to: &objects)
        #expect(objects[0].keyframes.count == 1)
    }

    @Test func placingIsAnObservationAtThisSampleWithNoBound() {
        var objects: [PhysicalObject] = []
        PhysicalDraft.addKeyframe(
            objectID: "o", sample: sample(4), author: "op", session: "s", to: &objects)
        PhysicalDraft.place(objectID: "o", sampleID: 4, x: 3.5, y: -1, in: &objects)
        let p = objects[0].keyframes[0].position
        #expect(p.status == .observed && p.xM == 3.5 && p.yM == -1)
        #expect(p.support.frameList == [4])
        #expect(p.boundM == nil, "the evidence slack is not a precision to prefill")
        // A second placement moves it and keeps the stated evidence.
        PhysicalDraft.updateKeyframe(objectID: "o", sampleID: 4, in: &objects) {
            $0.position.status = .inferred
        }
        PhysicalDraft.place(objectID: "o", sampleID: 4, x: 1, y: 1, in: &objects)
        #expect(objects[0].keyframes[0].position.status == .inferred)
    }

    @Test func dimensionStatusAndSpanKeepTheirValuesHonest() {
        var d = PhysicalDimension(
            status: .observed, span: .full, lowerM: 4, upperM: 5, valueM: 4.5,
            support: .init(frames: [1]))
        PhysicalDraft.setSpan(.partial, of: &d)
        #expect(d.lowerM == 4 && d.upperM == nil && d.valueM == nil)
        PhysicalDraft.setStatus(.inferred, of: &d)
        #expect(d.span == .full, "only an observation can be partial")
        PhysicalDraft.setStatus(.unknown, of: &d)
        #expect(d == PhysicalDimension())
        #expect(
            PhysicalDimension(status: .observed, span: .full, lowerM: 4, upperM: 5).best!.value
                == 4.5)
        #expect(PhysicalDimension(status: .observed, span: .partial, lowerM: 4).best == nil)
    }

    @Test func anUnresolvedAxisNamesNoEndAndNoFace() {
        var k = PhysicalKeyframe(keyframeID: "k", sampleID: 1, timestampNs: 1)
        PhysicalDraft.setAxis(.resolved, of: &k)
        #expect(k.yaw.status == .observed && k.yaw.support.frameList == [1])
        k.anchor = PhysicalAnchor(kind: .rearFace, offsetM: 2, offsetBoundM: 0.1)
        k.front = PhysicalEndpoint(status: .observed, support: .init(frames: [1]))
        PhysicalDraft.setAxis(.frontRearAmbiguous, of: &k)
        #expect(k.front == PhysicalEndpoint() && k.rear == PhysicalEndpoint())
        #expect(k.anchor.kind == .bodyCentre)
        PhysicalDraft.setAxis(.unknown, of: &k)
        #expect(k.yaw == PhysicalYaw(status: .unknown, axis: .unknown))
        PhysicalDraft.setAnchor(.bodyCentre, of: &k)
        #expect(k.anchor.offsetM == nil)
    }

    @Test func supportFramesStaySortedAndUnique() {
        var s = PhysicalSupport()
        s.toggle(frame: 5)
        s.toggle(frame: 2)
        s.toggle(frame: 9)
        #expect(s.frameList == [2, 5, 9])
        s.toggle(frame: 5)
        #expect(s.frameList == [2, 9])
        s.toggle(frame: 2)
        s.toggle(frame: 9)
        #expect(s.frames == nil)
    }

    @Test func removingTheLastRecordRemovesTheObjectAndCanonicalOrderHolds() {
        var objects: [PhysicalObject] = []
        PhysicalDraft.addKeyframe(
            objectID: "b", sample: sample(3), author: "op", session: "s", to: &objects)
        PhysicalDraft.addKeyframe(
            objectID: "b", sample: sample(1), author: "op", session: "s", to: &objects)
        PhysicalDraft.addBody(objectID: "a", author: "op", session: "s", to: &objects)
        PhysicalDraft.canonicalise(&objects)
        #expect(objects.map(\.objectID) == ["a", "b"])
        #expect(objects[1].keyframes.map(\.sampleID) == [1, 3])
        PhysicalDraft.removeBody(objectID: "a", from: &objects)
        #expect(objects.map(\.objectID) == ["b"])
        PhysicalDraft.removeKeyframe(objectID: "b", sampleID: 1, from: &objects)
        PhysicalDraft.removeKeyframe(objectID: "b", sampleID: 3, from: &objects)
        #expect(objects.isEmpty)
    }
}
