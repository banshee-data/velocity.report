//
//  LaneSnapshotTests.swift
//  VelocityVisualiserTests
//
//  Send lanes to velocity.report (lane painter plan, Item 4): after a save,
//  the features as Gerardus returned them and the accepted record, sent as
//  the lane snapshot; the standing notice until sent; a refusal shown and not
//  retried; an unknown outcome retried with the same request id.
//

import Foundation
import GRPCCore
import Testing

@testable import VelocityVisualiser

final class FakeLaneSnapshotSink: LaneSnapshotSink, @unchecked Sendable {
    var bodies: [Data] = []
    var answers: [(Int, String)] = []
    var transportFailures = 0
    func putLanes(l16Token: String, body: Data) async throws -> (status: Int, body: Data) {
        bodies.append(body)
        if transportFailures > 0 {
            transportFailures -= 1
            throw URLError(.networkConnectionLost)
        }
        let (status, text) = answers.isEmpty
            ? (201, #"{"snapshot_id":"lsn_0123456789ab","repeated":false,"lanes":1,"directions":[{"id":"columbus-ave:e","name":"Columbus Ave eastbound","lanes":["gms:x"]}]}"#)
            : answers.removeFirst()
        return (status, Data(text.utf8))
    }
    var requests: [LaneSnapshotRequest] { bodies.compactMap { try? JSONDecoder().decode(LaneSnapshotRequest.self, from: $0) } }
}

private let snapSite = AlignmentSite(
    clipID: "clip-snap", l16Token: "808580f47", l13Token: "808580f4",
    description: "S2 archive: Columbus at Broadway (10 min)", sensorID: "hesai-01",
    latitude: 37.7978, longitude: -122.4065)

@MainActor private func snapshotPainter(accepted: Bool = true) async -> (StreetPainterSession, FakeAlignmentService, FakeLaneSnapshotSink) {
    let service = FakeAlignmentService()
    service.supportsLaneFields = true
    service.availableLayers += [.cityStreets, .kerbs, .streetUse]
    var p = Gerardus_Align_V1alpha1_LaneProposal()
    p.featureID = "gms:p1"
    p.rowID = "row-p1"
    p.line.xyz = [0, -2, 21, 40, -2, 21]
    p.line.tags = ["confidence": "high", "lane_type": "general", "width_m": "3.2"]
    service.laneProposals = [p]
    if accepted {
        var record = Gerardus_Align_V1alpha1_RegistrationRecord()
        record.recordID = "gms:accepted"
        record.scanID = "sha256:scan"
        record.captureRef = snapSite.clipID
        record.sensorID = "hesai-01"
        record.state = .registered
        record.pose.translationM.x = -14.1
        record.pose.rotation.w = 1
        service.records = [record]
    }
    let sink = FakeLaneSnapshotSink()
    let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
    let painter = StreetPainterSession(alignment: session, laneFields: FakeLaneFieldSource(), snapshots: sink)
    painter.viewSize = CGSize(width: 800, height: 600)
    await session.probe()
    session.choose(snapSite)
    session.northAzimuthText = "35"
    await session.placeFromHint()
    await painter.pendingLoad?.value
    return (painter, service, sink)
}

@MainActor struct LaneSnapshotSendTests {
    @Test func aSaveLeavesANoticeUntilTheSnapshotIsSent() async throws {
        let (painter, _, sink) = await snapshotPainter()
        #expect(painter.snapshotBlocker(author: "op")?.contains("Save") == true)
        painter.approveProposal("gms:p1")
        await painter.save(author: "op")
        #expect(painter.snapshotUnsent)
        #expect(painter.snapshotBlocker(author: "op") == nil)
        await painter.sendSnapshot(author: "op")
        #expect(!painter.snapshotUnsent)
        #expect(painter.snapshotMessage?.contains("Columbus Ave eastbound") == true)
        let sent = try #require(sink.requests.first)
        #expect(sent.writer == "painter")
        #expect(sent.recordID == "gms:accepted" && sent.sensorID == "hesai-01")
        #expect(sent.pose.translation[0] == -14.1 && sent.pose.rotation == [1, 0, 0, 0])
        #expect(sent.features.count == 1)
        let f = try #require(sent.features.first)
        #expect(f.kind == "travel_lane" && !f.featureID.isEmpty && !f.rowID.isEmpty)
        #expect(f.tags["proposal"] == "gms:p1" && f.tags["speed_limit"] == "25")
        #expect(sent.author == "op" && !sent.requestID.isEmpty && sent.gerardusVersion == "0.1.0")
    }

    @Test func withoutAnAcceptedPoseNothingIsSent() async {
        let (painter, _, sink) = await snapshotPainter(accepted: false)
        painter.approveProposal("gms:p1")
        await painter.save(author: "op")
        #expect(painter.snapshotBlocker(author: "op")?.contains("Accept a pose") == true)
        await painter.sendSnapshot(author: "op")
        #expect(sink.bodies.isEmpty)
        #expect(painter.snapshotUnsent)
    }

    @Test func aRefusalIsShownAndNotRetried() async {
        let (painter, _, sink) = await snapshotPainter()
        sink.answers = [(404, #"{"error":"no site at this token yet"}"#)]
        painter.approveProposal("gms:p1")
        await painter.save(author: "op")
        await painter.sendSnapshot(author: "op")
        #expect(painter.snapshotError?.contains("no site at this token yet") == true)
        #expect(painter.pendingSnapshot == nil)
        #expect(painter.snapshotUnsent)
        #expect(sink.bodies.count == 1)
    }

    @Test func anUnknownOutcomeIsRetriedWithTheSameRequest() async {
        let (painter, _, sink) = await snapshotPainter()
        sink.transportFailures = 1
        painter.approveProposal("gms:p1")
        await painter.save(author: "op")
        await painter.sendSnapshot(author: "op")
        #expect(painter.pendingSnapshot != nil)
        #expect(painter.snapshotUnsent)
        await painter.sendSnapshot(author: "op")
        #expect(sink.bodies.count == 2)
        #expect(sink.bodies[0] == sink.bodies[1])
        #expect(!painter.snapshotUnsent)
    }
}
