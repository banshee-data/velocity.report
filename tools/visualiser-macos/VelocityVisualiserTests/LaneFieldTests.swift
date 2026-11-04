//
//  LaneFieldTests.swift
//  VelocityVisualiserTests
//
//  The street painter's traffic layer (lane painter plan, Item 1): the lane
//  field read from velocity.report's JSON and Gerardus's summed field, its
//  quads placed by the pose, the live accumulator's floor, the upload's
//  chunks and record, and what blocks Send lane field. Synthetic fields only.
//

import Foundation
import GRPCCore
import SwiftProtobuf
import Testing
import simd

@testable import VelocityVisualiser

/// velocity.report's JSON for a two-cell vehicle field: one cell heading
/// along the sensor's +Y (bin 0), one parked (no heading).
let twoCellFieldJSON = """
    {"header":{"l16_token":"","sensor_id":"hesai-01","record_id":"","capture_ref":"cap@2026-09-02T20:20:36Z",
    "window_start":"2026-09-02T20:20:36.5Z","window_end":"2026-09-02T20:30:36Z","window_part":"all","cell_m":0.25,
    "origin":[1.125,2.125],"nx":2,"ny":1,"k":5,"pipeline_build":"v0+abc","tuning_fingerprint":"f","track_count":9},
    "groups":[{"class_group":"vehicle","ix":[0,1],"iy":[0,0],"tracks":[6,5],"box_tracks":[9,7],
    "heading":[6,0,0,0,0,0,0,0, 0,0,0,0,0,0,0,0],"speed_sum_mps":[30,0.5]}]}
    """

private func decode(_ s: String = twoCellFieldJSON) throws -> VelocityReportLaneField {
    try JSONDecoder().decode(VelocityReportLaneField.self, from: Data(s.utf8))
}

/// A pose turning the sensor 90° anticlockwise (+Y points west) at (10, 20).
private let turned = SitePose(
    translation: SIMD3(10, 20, 0), rotation: simd_quatd(angle: .pi / 2, axis: SIMD3(0, 0, 1)))

struct LaneFieldModelTests {
    @Test func velocityReportsJSONDecodesIntoASensorFrameGrid() throws {
        let field = try decode()
        let grid = field.grid
        #expect(grid.frame == .sensor)
        #expect(grid.source == .run)
        #expect(grid.cellCount == 2)
        #expect(grid.trackCount == 9)
        #expect(grid.centre(1, 0) == SIMD2(1.375, 2.125))
        let cells = try #require(grid.groups[.vehicle])
        #expect(cells.dominantBin(0) == 0)
        #expect(cells.dominantBin(1) == nil)
    }

    @Test func aGroupWithArraysThatDisagreeIsLeftOut() throws {
        let bad = twoCellFieldJSON.replacingOccurrences(of: "\"tracks\":[6,5]", with: "\"tracks\":[6]")
        #expect(try decode(bad).grid.groups.isEmpty)
    }

    @Test func theUploadHeaderNamesTheRecordAndKeepsTheRun() throws {
        let h = try decode().uploadHeader(l16Token: "808580f47", recordID: "gms:rec")
        #expect(h.l16Token == "808580f47")
        #expect(h.recordID == "gms:rec")
        #expect(h.sensorID == "hesai-01")
        #expect(h.windowPart == "all")
        #expect(h.k == 5)
        #expect(h.cellM == 0.25)
        #expect(h.origin.x == 1.125 && h.origin.y == 2.125)
        #expect(h.trackCount == 9)
        #expect(h.windowStart.seconds > 0 && h.windowStart.nanos == 500_000_000)
    }

    @Test func cellsAreChunkedWithAGroupSpanningChunks() throws {
        let chunks = try decode().uploadChunks(cellsPerChunk: 1)
        #expect(chunks.count == 2)
        for c in chunks {
            #expect(c.classGroup == "vehicle")
            #expect(c.ix.count == 1 && c.heading.count == 8 && c.speedSumMps.count == 1)
        }
        #expect(chunks[1].ix == [1] && chunks[1].tracks == [5])
    }

    @Test func aGroupWithArraysThatDisagreeIsNotUploaded() throws {
        let bad = twoCellFieldJSON.replacingOccurrences(
            of: "\"heading\":[6,0,0,0,0,0,0,0, 0,0,0,0,0,0,0,0]", with: "\"heading\":[6,0,0,0,0,0,0,0]")
        #expect(bad != twoCellFieldJSON)
        #expect(try decode(bad).uploadChunks(cellsPerChunk: 1).isEmpty)
    }

    @Test func gerardussSummedFieldIsASiteFrameGrid() {
        var r = Gerardus_Align_V1alpha1_GetLaneFieldResponse()
        r.origin.x = -3.875
        r.origin.y = 4.125
        r.cellM = 0.25
        r.nx = 1
        r.ny = 1
        r.cells.classGroup = "vehicle"
        r.cells.ix = [0]
        r.cells.iy = [0]
        r.cells.tracks = [12]
        r.cells.boxTracks = [20]
        r.cells.heading = [0, 0, 12, 0, 0, 0, 0, 0]
        r.cells.speedSumMps = [60]
        r.captures = 3
        r.trackCount = 40
        r.fieldIds = ["sha256:b", "sha256:a"]
        let grid = LaneFieldGrid.site([.vehicle: r])
        #expect(grid.frame == .site)
        #expect(grid.captures == 3 && grid.trackCount == 40)
        #expect(grid.fieldIDs == ["sha256:a", "sha256:b"])
        #expect(grid.centre(0, 0) == SIMD2(-3.875, 4.125))
    }
}

struct LaneFieldStyleTests {
    @Test func aSensorFrameCellIsPlacedByThePose() throws {
        let grid = try decode().grid
        var display = LaneFieldDisplay()
        display.show = .centres
        let fills = LaneFieldStyle.fills(grid, display: display, pose: turned, groundZ: { _ in 20 })
        // Two cells, two triangles each, seven floats a vertex.
        #expect(fills.count == 2 * 6 * 7)
        // The first cell's centre (1.125, 2.125) turned 90° anticlockwise is
        // (−2.125, 1.125), then moved to (7.875, 21.125).
        var cx = 0.0
        var cy = 0.0
        for v in 0..<6 {
            cx += Double(fills[7 * v])
            cy += Double(fills[7 * v + 1])
        }
        #expect(abs(cx / 6 - 7.875) < 0.1)
        #expect(abs(cy / 6 - 21.125) < 0.1)
        #expect(abs(Double(fills[2]) - (20 + LaneFieldStyle.lift)) < 1e-4)
    }

    @Test func noPoseNoSensorFrameField() throws {
        #expect(LaneFieldStyle.fills(try decode().grid, display: .init(), pose: nil, groundZ: { _ in 0 }).isEmpty)
    }

    @Test func theFloorAndTheClassesHideCells() throws {
        let grid = try decode().grid
        var display = LaneFieldDisplay()
        display.show = .centres
        display.floor = 6
        #expect(LaneFieldStyle.fills(grid, display: display, pose: turned, groundZ: { _ in 0 }).count == 6 * 7)
        display.floor = 5
        display.classes = [.pedestrian]
        #expect(LaneFieldStyle.fills(grid, display: display, pose: turned, groundZ: { _ in 0 }).isEmpty)
    }

    @Test func headingBinsTurnWithThePose() {
        // +Y bears 0° at the identity: bin 0 is the 0–45° sector.
        #expect(LaneFieldStyle.octant(bin: 0, turnDeg: 0) == 0)
        // The turned pose points +Y west (270°): bin 0's centre, 22.5°, turns
        // to 292.5°, the WNW sector.
        let turn = LaneFieldStyle.sensorYBearing(turned)
        #expect(abs(turn - 270) < 1e-6)
        #expect(LaneFieldStyle.octant(bin: 0, turnDeg: turn) == 6)
        #expect(LaneFieldStyle.compass[6] == "WNW")
    }

    @Test func theLegendNamesTheDirectionsHeld() throws {
        let legend = LaneFieldStyle.legend(try decode().grid, pose: turned)
        #expect(legend.count == 1)
        #expect(legend.first?.bound == "WNW")
    }
}

struct LaneFieldLiveTests {
    private func car(_ id: String, x: Float, y: Float, misses: Int = 0) -> Track {
        var t = Track()
        t.trackID = id
        t.classLabel = "car"
        t.x = x
        t.y = y
        t.vy = 5
        t.speedMps = 5
        t.misses = misses
        return t
    }

    @Test func liveCellsUnderFiveTracksAreNotShown() {
        var live = LaneFieldLiveAccumulator()
        for i in 0..<4 { live.add([car("t\(i)", x: 0.1, y: 0.1)]) }
        #expect(live.grid.cellCount == 0)
        live.add([car("t4", x: 0.1, y: 0.1)])
        let grid = live.grid
        #expect(grid.cellCount == 1)
        #expect(grid.source == .live)
        #expect(grid.groups[.vehicle]?.tracks == [5])
        #expect(grid.groups[.vehicle]?.dominantBin(0) == 0)
    }

    @Test func aTrackCountsOnceAndAMissNotAtAll() {
        var live = LaneFieldLiveAccumulator()
        for _ in 0..<20 { live.add([car("same", x: 0.1, y: 0.1)]) }
        for i in 0..<10 { live.add([car("coast\(i)", x: 0.1, y: 0.1, misses: 1)]) }
        #expect(live.grid.cellCount == 0)
    }
}

struct AcceptedRecordTests {
    private func record(_ id: String, scan: String, _ state: Gerardus_Align_V1alpha1_RegistrationState)
        -> Gerardus_Align_V1alpha1_RegistrationRecord
    {
        var r = Gerardus_Align_V1alpha1_RegistrationRecord()
        r.recordID = id
        r.scanID = scan
        r.captureRef = "cap"
        r.state = state
        if state != .rejected { r.pose = Gerardus_Align_V1alpha1_SensorPose() }
        return r
    }

    @Test func theHeadOfTheCapturesScanPlacesTheField() {
        let records = [record("r2", scan: "s", .registered), record("r1", scan: "s", .manuallyPlaced)]
        let got = AlignmentSession.acceptedRecord(records, heads: ["s": "r2"], captureRef: "cap", scanID: nil)
        #expect(got?.recordID == "r2")
    }

    @Test func aRejectionAtTheHeadLeavesNone() {
        let records = [record("r2", scan: "s", .rejected), record("r1", scan: "s", .registered)]
        #expect(AlignmentSession.acceptedRecord(records, heads: ["s": "r2"], captureRef: "cap", scanID: nil) == nil)
    }

    @Test func anotherCapturesRecordDoesNotPlaceIt() {
        let records = [record("r1", scan: "s", .registered)]
        #expect(AlignmentSession.acceptedRecord(records, heads: ["s": "r1"], captureRef: "other", scanID: "x") == nil)
    }
}

// MARK: - The traffic layer against fake servers

/// velocity.report's route, faked: the three parts of one run.
final class FakeLaneFieldSource: LaneFieldSource, @unchecked Sendable {
    var fields: [String: VelocityReportLaneField] = [:]
    var asked: [String] = []
    func laneField(runID: String, part: String) async throws -> VelocityReportLaneField? {
        asked.append(part)
        return runID == "run-1" ? fields[part] : nil
    }
}

private let trafficSite = AlignmentSite(
    clipID: "clip-columbus", l16Token: "808580f47", l13Token: "808580f4",
    description: "S2 archive: Columbus at Broadway (10 min)", sensorID: "hesai-01",
    latitude: 37.7978, longitude: -122.4065)

@MainActor private func trafficPainter(
    accepted: Gerardus_Align_V1alpha1_RegistrationState? = .manuallyPlaced,
    configure: (FakeAlignmentService) -> Void = { _ in }
) async throws -> (StreetPainterSession, FakeAlignmentService, FakeLaneFieldSource, FakeStreetSink) {
    let service = FakeAlignmentService()
    service.supportsLaneFields = true
    service.streetUseRulesVersion = 1
    service.availableLayers += [.cityStreets, .kerbs, .streetUse]
    if let accepted {
        var record = Gerardus_Align_V1alpha1_RegistrationRecord()
        record.recordID = "gms:accepted"
        record.scanID = "sha256:scan-columbus"
        record.captureRef = trafficSite.clipID
        record.sensorID = "hesai-01"
        record.state = accepted
        if accepted != .rejected { record.pose = Gerardus_Align_V1alpha1_SensorPose() }
        service.records = [record]
    }
    configure(service)
    let source = FakeLaneFieldSource()
    for part in StreetTrafficLayer.parts {
        var json = twoCellFieldJSON.replacingOccurrences(
            of: "\"window_part\":\"all\"", with: "\"window_part\":\"\(part)\"")
        if part == "second_half" { json = json.replacingOccurrences(of: "[6,5]", with: "[5,6]") }
        source.fields[part] = try JSONDecoder().decode(VelocityReportLaneField.self, from: Data(json.utf8))
    }
    let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
    let painter = StreetPainterSession(alignment: session, laneFields: source)
    let sink = FakeStreetSink()
    painter.sink = sink
    painter.viewSize = CGSize(width: 800, height: 600)
    await session.probe()
    session.choose(trafficSite)
    session.northAzimuthText = "35"
    await session.placeFromHint()
    await painter.pendingLoad?.value
    return (painter, service, source, sink)
}

@MainActor struct StreetTrafficLayerTests {
    @Test func sendUploadsTheThreePartsAgainstTheAcceptedRecord() async throws {
        let (painter, service, _, _) = try await trafficPainter()
        let traffic = painter.traffic
        await traffic.load(runID: "run-1")
        #expect(traffic.sendBlocker(pendingSave: false) == nil)
        await traffic.send(pendingSave: false)
        #expect(service.laneFieldUploads.count == 3)
        #expect(service.laneFieldUploads.map(\.0.windowPart) == ["all", "first_half", "second_half"])
        for (header, cells) in service.laneFieldUploads {
            #expect(header.recordID == "gms:accepted")
            #expect(header.l16Token == "808580f47")
            #expect(header.sensorID == "hesai-01")
            #expect(cells.reduce(0) { $0 + $1.ix.count } == 2)
        }
        #expect(traffic.message?.contains("3 fields stored") == true)
        // Sent again, the same content is already there.
        await traffic.send(pendingSave: false)
        #expect(traffic.message?.contains("3 already there") == true)
        // The site's field is read again after each send.
        #expect(service.calls.filter { $0 == "GetLaneField" }.count >= 6)
    }

    @Test func sendIsBlockedWithoutAnAcceptedPose() async throws {
        let (painter, service, _, _) = try await trafficPainter(accepted: .rejected)
        await painter.traffic.load(runID: "run-1")
        #expect(painter.traffic.sendBlocker(pendingSave: false)?.contains("Accept a pose") == true)
        await painter.traffic.send(pendingSave: false)
        #expect(service.laneFieldUploads.isEmpty)
    }

    @Test func sendIsBlockedByAPendingSaveAndWithoutARun() async throws {
        let (painter, _, _, _) = try await trafficPainter()
        #expect(painter.traffic.sendBlocker(pendingSave: false)?.contains("recorded run") == true)
        await painter.traffic.load(runID: "run-1")
        #expect(painter.traffic.sendBlocker(pendingSave: true)?.contains("Save or discard") == true)
    }

    @Test func aServerWithoutLaneFieldsHidesTheSiteFieldAndSend() async throws {
        let (painter, service, _, _) = try await trafficPainter { $0.supportsLaneFields = false }
        await painter.traffic.load(runID: "run-1")
        #expect(!painter.traffic.serverSupportsFields)
        #expect(painter.traffic.sendBlocker(pendingSave: false)?.contains("does not take") == true)
        #expect(!service.calls.contains("GetLaneField"))
        // With no site field the run's is drawn.
        #expect(painter.traffic.choice == .run)
    }

    @Test func theFieldIsDrawnUnderTheMarksOnlyWhilePainting() async throws {
        let (painter, _, _, sink) = try await trafficPainter { $0.supportsLaneFields = false }
        await painter.traffic.load(runID: "run-1")
        #expect(painter.trafficFills.isEmpty)
        painter.setPainting(true)
        #expect(painter.isPainting)
        #expect(painter.trafficFills.count == 2 * 6 * 7)
        let overlay = try #require(sink.overlays.last ?? nil)
        #expect(Array(overlay.fills.prefix(painter.trafficFills.count)) == painter.trafficFills)
        painter.traffic.visible = false
        #expect(painter.trafficFills.isEmpty)
    }

    @Test func theSiteFieldIsChosenWhenGerardusHasOne() async throws {
        var r = Gerardus_Align_V1alpha1_GetLaneFieldResponse()
        r.cellM = 0.25
        r.cells.classGroup = "vehicle"
        r.cells.ix = [0]
        r.cells.iy = [0]
        r.cells.tracks = [7]
        r.cells.boxTracks = [9]
        r.cells.heading = [7, 0, 0, 0, 0, 0, 0, 0]
        r.cells.speedSumMps = [40]
        r.captures = 2
        let (painter, _, _, _) = try await trafficPainter { $0.siteLaneFields = ["vehicle": r] }
        await painter.traffic.load(runID: "run-1")
        #expect(painter.traffic.choice == .site)
        #expect(painter.traffic.shown?.captures == 2)
        #expect(painter.traffic.shown?.frame == .site)
    }
}
