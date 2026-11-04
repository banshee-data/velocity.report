//
//  LaneProposalTests.swift
//  VelocityVisualiserTests
//
//  The street painter's lane proposals and limits (lane painter plan,
//  Items 2 and 3), against the fake Gerardus: approve and approve all high
//  as undoable marks naming their proposal, reject and reopen with the row
//  id and a stable request id, split at a sign, the statutory prefill, the
//  limit checks, direction names, and the tags Save sends. Synthetic
//  geometry only.
//

import Foundation
import GRPCCore
import Testing
import simd

@testable import VelocityVisualiser

private func proposal(
    _ id: String, grade: String, from a: SIMD2<Double>, to b: SIMD2<Double>, adopted: String = "",
    rejected: Bool = false
) -> Gerardus_Align_V1alpha1_LaneProposal {
    var p = Gerardus_Align_V1alpha1_LaneProposal()
    p.featureID = id
    p.rowID = "row-\(id)"
    p.line.kind = .travelLane
    p.line.xyz = [Float(a.x), Float(a.y), 21, Float(b.x), Float(b.y), 21]
    p.line.tags = [
        "confidence": grade, "lane_type": "general", "width_m": "3.2", "street": "COLUMBUS AVE",
        "bound": "E", "bearing_deg": "90.0", "tracks": "120", "spread_m": "0.4",
    ]
    var profile = Gerardus_Align_V1alpha1_LateralProfile()
    profile.stationM = 5
    profile.centre.x = (a.x + b.x) / 2
    profile.centre.y = (a.y + b.y) / 2
    profile.bearingDeg = 90
    profile.offsetM = [-1, 0, 1]
    profile.tracks = [2, 9, 3]
    p.profiles = [profile]
    p.adoptedBy = adopted
    if rejected {
        p.review.decision = .rejected
        p.review.featureID = id
    }
    return p
}

private let laneSite = AlignmentSite(
    clipID: "clip-lanes", l16Token: "808580f47", l13Token: "808580f4",
    description: "S2 archive: Columbus at Broadway (10 min)", sensorID: "hesai-01",
    latitude: 37.7978, longitude: -122.4065)

@MainActor private func lanePainter(
    _ proposals: [Gerardus_Align_V1alpha1_LaneProposal]
) async -> (StreetPainterSession, FakeAlignmentService, FakeStreetSink) {
    let service = FakeAlignmentService()
    service.supportsLaneFields = true
    service.streetUseRulesVersion = 1
    service.availableLayers += [.cityStreets, .kerbs, .streetUse]
    service.laneProposals = proposals
    let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
    let painter = StreetPainterSession(alignment: session, laneFields: FakeLaneFieldSource())
    let sink = FakeStreetSink()
    painter.sink = sink
    painter.viewSize = CGSize(width: 800, height: 600)
    await session.probe()
    session.choose(laneSite)
    session.northAzimuthText = "35"
    await session.placeFromHint()
    await painter.pendingLoad?.value
    return (painter, service, sink)
}

private let high = proposal("gms:p1", grade: "high", from: SIMD2(0, -2), to: SIMD2(40, -2))
private let medium = proposal("gms:p2", grade: "medium", from: SIMD2(40, 2), to: SIMD2(0, 2))
private let adopted = proposal("gms:p3", grade: "high", from: SIMD2(0, -6), to: SIMD2(40, -6), adopted: "gms:lane")
private let rejectedHigh = proposal("gms:p4", grade: "high", from: SIMD2(0, -9), to: SIMD2(40, -9), rejected: true)

@MainActor struct LaneProposalReviewTests {
    @Test func proposalsAreReadWithTheMarks() async {
        let (painter, service, _) = await lanePainter([high, medium])
        #expect(service.calls.contains("ListLaneProposals"))
        #expect(painter.proposals.proposals.map(\.featureID) == ["gms:p1", "gms:p2"])
        #expect(painter.proposals.proposals[0].profiles.count == 1)
        #expect(painter.proposals.grouped.map(\.bound) == ["E"])
    }

    @Test func approveMakesAMarkNamingItsProposalWithTheStatutoryDefault() async {
        let (painter, _, _) = await lanePainter([high])
        painter.approveProposal("gms:p1")
        let mark = try! #require(painter.selected)
        #expect(mark.kind == .travelLane)
        #expect(mark.featureID.isEmpty)
        #expect(mark.points.count == 2)
        #expect(mark.tags["proposal"] == "gms:p1")
        #expect(mark.tags["confidence"] == "high")
        #expect(mark.tags["lane_type"] == "general")
        #expect(mark.tags["width_m"] == "3.2")
        #expect(mark.tags["speed_limit"] == "25")
        #expect(mark.tags["speed_limit_unit"] == "mph")
        #expect(mark.tags["speed_limit_source"] == "statutory")
        #expect(mark.tags["speed_limit_jurisdiction"] == "US-CA")
        #expect(LaneLimits.isDay(mark.tags["speed_limit_effective_from"] ?? ""))
        painter.undo()
        #expect(painter.edit.marks.isEmpty)
    }

    @Test func approveAllHighTakesOnlyUnadoptedUnrejectedHighOnesAsOneChange() async {
        let (painter, _, _) = await lanePainter([high, medium, adopted, rejectedHigh])
        #expect(painter.approvableHigh.map(\.featureID) == ["gms:p1"])
        painter.approveAllHigh()
        #expect(painter.edit.marks.compactMap { $0.tags["proposal"] } == ["gms:p1"])
        // Approved once, it is not offered again.
        #expect(painter.approvableHigh.isEmpty)
        painter.undo()
        #expect(painter.edit.marks.isEmpty)
    }

    @Test func aRejectedProposalCannotBeApproved() async {
        let (painter, _, _) = await lanePainter([rejectedHigh])
        painter.approveProposal("gms:p4")
        #expect(painter.edit.marks.isEmpty)
    }

    @Test func rejectAndReopenSendTheRowAndAuthorAndStick() async {
        let (painter, service, _) = await lanePainter([high])
        await painter.proposals.review("gms:p1", .rejected, author: "operator")
        let sent = try! #require(service.laneProposalReviews.last)
        #expect(sent.featureID == "gms:p1")
        #expect(sent.decision == .rejected)
        #expect(sent.author == "operator")
        #expect(sent.expectedRowID == "row-gms:p1")
        #expect(!sent.requestID.isEmpty)
        #expect(painter.proposals.proposals[0].rejected)
        // The answer's proposal has no profiles; the listed ones are kept.
        #expect(painter.proposals.proposals[0].profiles.count == 1)
        // Read again, the rejection is still there.
        await painter.proposals.load()
        #expect(painter.proposals.proposals[0].rejected)
        await painter.proposals.review("gms:p1", .reopened, author: "operator")
        #expect(!painter.proposals.proposals[0].rejected)
    }

    @Test func aReviewWithoutAnAuthorIsNotSent() async {
        let (painter, service, _) = await lanePainter([high])
        await painter.proposals.review("gms:p1", .rejected, author: "  ")
        #expect(service.laneProposalReviews.isEmpty)
        #expect(painter.proposals.errorMessage?.contains("author") == true)
    }

    @Test func aLostAnswerIsRetriedWithTheSameRequestID() async {
        let (painter, service, _) = await lanePainter([high])
        service.laneReviewUnavailable = 1
        await painter.proposals.review("gms:p1", .rejected, author: "operator")
        #expect(painter.proposals.pending != nil)
        await painter.proposals.review("gms:p1", .rejected, author: "operator")
        #expect(service.laneProposalReviews.count == 2)
        #expect(service.laneProposalReviews[0].requestID == service.laneProposalReviews[1].requestID)
        #expect(painter.proposals.pending == nil)
        #expect(painter.proposals.proposals[0].rejected)
    }

    @Test func aStaleRowReadsTheProposalsAgain() async {
        let (painter, service, _) = await lanePainter([high])
        service.laneProposals[0].rowID = "row-newer"
        await painter.proposals.review("gms:p1", .rejected, author: "operator")
        #expect(painter.proposals.errorMessage?.contains("aborted") == true)
        #expect(painter.proposals.pending == nil)
        #expect(painter.proposals.proposals[0].rowID == "row-newer")
    }

    @Test func aSecondRejectionIsRefusedAndDropped() async {
        let (painter, service, _) = await lanePainter([rejectedHigh])
        await painter.proposals.review("gms:p4", .rejected, author: "operator")
        #expect(service.laneProposalReviews.count == 1)
        #expect(painter.proposals.pending == nil)
        #expect(painter.proposals.errorMessage?.contains("failedPrecondition") == true)
    }

    @Test func proposalsAreDrawnWhilePaintingAndHiddenByTheirToggle() async {
        let (painter, _, sink) = await lanePainter([high, medium])
        painter.setPainting(true)
        let with = sink.overlays.last??.lines.count ?? 0
        painter.proposals.visible = false
        let without = sink.overlays.last??.lines.count ?? 0
        #expect(with > without)
        // Selecting one draws its profile across it.
        painter.proposals.visible = true
        let unselected = sink.overlays.last??.lines.count ?? 0
        painter.proposals.selectedID = "gms:p1"
        #expect((sink.overlays.last??.lines.count ?? 0) > unselected)
    }

    @Test func saveSendsTheApprovedLaneWithItsLimitTags() async {
        let (painter, service, _) = await lanePainter([high])
        painter.approveProposal("gms:p1")
        painter.setSelectedLimit(["speed_limit": "30", "speed_limit_source": "sign", "speed_limit_effective_from": "2026-06-01"])
        await painter.save(author: "operator")
        let request = try! #require(service.streetUseEdits.last)
        #expect(request.edits.count == 1)
        guard case .upsert(let feature) = request.edits[0].change else {
            Issue.record("no upsert")
            return
        }
        #expect(feature.kind == .travelLane)
        #expect(feature.tags["proposal"] == "gms:p1")
        #expect(feature.tags["speed_limit"] == "30")
        #expect(feature.tags["speed_limit_unit"] == "mph")
        #expect(feature.tags["speed_limit_source"] == "sign")
        #expect(feature.tags["speed_limit_effective_from"] == "2026-06-01")
    }
}

@MainActor struct LaneSplitAndNameTests {
    @Test func splittingALaneMakesTwoSharingTheVertex() {
        var lane = StreetMark(
            key: "l", featureID: "gms:lane", kind: .travelLane,
            points: [SIMD3(0, 0, 21), SIMD3(20, 0, 21), SIMD3(40, 0, 21)],
            tags: [
                "lane_type": "bus", "width_m": "3.5", "speed_limit": "25", "speed_limit_unit": "mph",
                "speed_limit_source": "sign", "speed_limit_jurisdiction": "US-CA",
                "speed_limit_effective_from": "2026-01-01", "proposal": "gms:p1",
            ])
        lane.rowID = "row-1"
        let (up, down) = try! #require(StreetUseEditing.split(lane, at: PlanPoint(30, 0.4), newKey: "new-9"))
        #expect(up.featureID == "gms:lane" && up.key == "l")
        #expect(down.featureID.isEmpty && down.key == "new-9" && down.rowID.isEmpty)
        #expect(up.points.last == down.points.first)
        #expect(up.points.last == SIMD3(30, 0, 21))
        #expect(up.points.count == 3 && down.points.count == 2)
        // Direction, type and width kept; the downstream limit opened.
        #expect(down.tags["lane_type"] == "bus" && down.tags["width_m"] == "3.5")
        #expect(down.tags["speed_limit"] == nil && down.tags["speed_limit_source"] == nil)
        #expect(down.tags["speed_limit_effective_from"] == nil && down.tags["proposal"] == nil)
        #expect(down.tags["speed_limit_unit"] == "mph" && down.tags["speed_limit_jurisdiction"] == "US-CA")
        #expect(up.tags == lane.tags)
        // Not at an end, nor off the lane.
        #expect(StreetUseEditing.split(lane, at: PlanPoint(0.2, 0), newKey: "x") == nil)
        #expect(StreetUseEditing.split(lane, at: PlanPoint(20, 5), newKey: "x") == nil)
    }

    @Test func aDirectionNameGoesOnEveryLaneOfTheDirection() async {
        let second = proposal("gms:p5", grade: "high", from: SIMD2(0, -5), to: SIMD2(40, -5))
        let (painter, _, _) = await lanePainter([high, second, medium])
        painter.approveAllHigh()
        painter.approveProposal("gms:p2")
        painter.selectedKey = painter.edit.marks.first { $0.tags["proposal"] == "gms:p1" }?.key
        #expect(painter.selectedDirection.count == 2)
        painter.setDirectionName("Columbus Ave eastbound")
        let named = painter.edit.marks.filter { $0.tags["direction_name"] == "Columbus Ave eastbound" }
        #expect(Set(named.compactMap { $0.tags["proposal"] }) == ["gms:p1", "gms:p5"])
        #expect(LaneLimits.defaultDirectionName(street: "COLUMBUS AVE", bound: "E") == "Columbus Ave eastbound")
    }
}

struct LaneLimitCheckTests {
    @Test func theServersRefusalsAreCheckedBeforeSave() {
        #expect(LaneLimits.refusals(["speed_limit": "25", "speed_limit_unit": "mph", "speed_limit_source": "sign"]).isEmpty)
        #expect(!LaneLimits.refusals(["speed_limit": "91", "speed_limit_unit": "mph", "speed_limit_source": "sign"]).isEmpty)
        #expect(LaneLimits.refusals(["speed_limit": "120", "speed_limit_unit": "km/h", "speed_limit_source": "sign"]).isEmpty)
        #expect(!LaneLimits.refusals(["speed_limit": "25.5", "speed_limit_unit": "mph", "speed_limit_source": "sign"]).isEmpty)
        #expect(!LaneLimits.refusals(["speed_limit": "25", "speed_limit_source": "sign"]).isEmpty)
        #expect(!LaneLimits.refusals(["speed_limit": "25", "speed_limit_unit": "mph"]).isEmpty)
        #expect(!LaneLimits.refusals(["speed_limit_unit": "mph"]).isEmpty)
        #expect(!LaneLimits.refusals(["speed_limit": "25", "speed_limit_unit": "kph", "speed_limit_source": "sign"]).isEmpty)
        #expect(!LaneLimits.refusals(["speed_limit_source": "guess"]).isEmpty)
        #expect(!LaneLimits.refusals(["speed_limit_jurisdiction": "California"]).isEmpty)
        #expect(LaneLimits.refusals(["speed_limit_jurisdiction": "US-CA"]).isEmpty)
        #expect(!LaneLimits.refusals(["speed_limit_effective_from": "2026-13-01"]).isEmpty)
        #expect(!LaneLimits.refusals(["speed_limit_effective_from": "2026-06-01", "speed_limit_effective_to": "2026-05-31"]).isEmpty)
    }

    @Test func aChangedLimitNeedsANewDateAndAMissingOneIsWarned() {
        let old = StreetMark(
            key: "a", featureID: "gms:a", kind: .travelLane, points: [SIMD3(0, 0, 0), SIMD3(10, 0, 0)],
            tags: ["speed_limit": "25", "speed_limit_unit": "mph", "speed_limit_source": "sign", "speed_limit_effective_from": "2026-01-01"])
        var changed = old
        changed.tags["speed_limit"] = "20"
        var w = LaneLimits.warnings(marks: [changed], base: [old])
        #expect(w.byKey["a"]?.contains { $0.contains("took effect") } == true)
        changed.tags["speed_limit_effective_from"] = "2026-09-01"
        w = LaneLimits.warnings(marks: [changed], base: [old])
        #expect(w.byKey["a"] == nil)
        var bare = old
        bare.key = "b"
        bare.featureID = ""
        bare.tags = [:]
        #expect(LaneLimits.warnings(marks: [bare], base: []).byKey["b"] == ["No limit."])
    }

    @Test func twoLanesOfOneDirectionWithDifferentLimitsAndNoSplitAreWarned() {
        func lane(_ key: String, y: Double, from x0: Double, to x1: Double, limit: String) -> StreetMark {
            StreetMark(
                key: key, featureID: "", kind: .travelLane, points: [SIMD3(x0, y, 0), SIMD3(x1, y, 0)],
                tags: ["speed_limit": limit, "speed_limit_unit": "mph", "speed_limit_source": "sign", "street": "KIRKHAM ST"])
        }
        let apart = LaneLimits.warnings(marks: [lane("a", y: 0, from: 0, to: 10, limit: "25"), lane("b", y: 3, from: 0, to: 10, limit: "30")], base: [])
        #expect(apart.byKey["a"]?.first?.contains("without a split") == true)
        // Split at a sign: the second begins where the first ends.
        let split = LaneLimits.warnings(marks: [lane("a", y: 0, from: 0, to: 10, limit: "25"), lane("b", y: 0, from: 10, to: 20, limit: "30")], base: [])
        #expect(split.byKey.isEmpty)
    }

    @Test func boundsAndTheStatutoryDefault() {
        #expect(LaneLimits.bound(0) == "N" && LaneLimits.bound(90) == "E" && LaneLimits.bound(311) == "NW")
        #expect(LaneLimits.statutoryDefault("US-CA")?.value == 25)
        #expect(LaneLimits.statutoryDefault("GB-LND") == nil)
        let p = LaneLimits.prefill(captureDate: "2026-09-02")
        #expect(p["speed_limit_source"] == "statutory" && p["speed_limit_effective_from"] == "2026-09-02")
    }
}

struct DottedLineTests {
    @Test func aZeroGapDrawsASolidLineAndFinishes() {
        let points: [SIMD3<Double>] = [SIMD3(0, 0, 0), SIMD3(10, 0, 0), SIMD3(10, 5, 0)]
        let colour = SIMD4<Float>(1, 1, 1, 1)
        var dotted = StreetPaintStyle.Builder()
        dotted.dotted(points, dash: 1, gap: 0, colour: colour)
        var solid = StreetPaintStyle.Builder()
        solid.polyline(points, closed: false, colour: colour)
        #expect(dotted.vertices.lines == solid.vertices.lines)
        var dashed = StreetPaintStyle.Builder()
        dashed.dotted(points, dash: 1, gap: 1, colour: colour)
        #expect(!dashed.vertices.lines.isEmpty && dashed.vertices.lines.count < 2 * solid.vertices.lines.count * 8)
    }
}
