//
//  LaneReviewTests.swift
//  VelocityVisualiserTests
//
//  City mode's Lanes to review list (lidar-lane-field-batch-plan Item 6):
//  counting a ListLaneProposals answer into high, medium, adopted,
//  rejected and outstanding; the sort; the reasons given instead of zeros;
//  the reads, one call at a time and cached for the session, dropped by a
//  review; and a row's switch-and-open through AppState's site switch,
//  against the fake Gerardus and the fake velocity.report replay calls.
//  Synthetic geometry only.
//

import Foundation
import GRPCCore
import Testing
import simd

@testable import VelocityVisualiser

private func proposal(
    _ id: String, grade: String, adopted: String = "", rejected: Bool = false
) -> Gerardus_Align_V1alpha1_LaneProposal {
    var p = Gerardus_Align_V1alpha1_LaneProposal()
    p.featureID = id
    p.rowID = "row-\(id)"
    p.line.kind = .travelLane
    p.line.xyz = [0, -2, 21, 40, -2, 21]
    p.line.tags = grade.isEmpty ? [:] : ["confidence": grade, "lane_type": "general"]
    p.adoptedBy = adopted
    if rejected {
        p.review.decision = .rejected
        p.review.featureID = id
    }
    return p
}

/// A ListLaneProposals answer with every case the list counts.
private func fixtureResponse() -> Gerardus_Align_V1alpha1_ListLaneProposalsResponse {
    var r = Gerardus_Align_V1alpha1_ListLaneProposalsResponse()
    r.modelID = "sha256:model-a"
    r.proposals = [
        proposal("gms:h1", grade: "high"), proposal("gms:h2", grade: "high"),
        proposal("gms:h3", grade: "high", adopted: "gms:lane1"),
        proposal("gms:h4", grade: "high", rejected: true), proposal("gms:m1", grade: "medium"),
        proposal("gms:m2", grade: "medium", adopted: "gms:lane2"),
        // A review reopened is not a rejection.
        {
            var p = proposal("gms:m3", grade: "medium")
            p.review.decision = .reopened
            return p
        }(),
        // A grade the list has no column for still counts as a proposal.
        proposal("gms:x1", grade: ""),
    ]
    return r
}

private func site(_ id: String, title: String, cell: String, placed: Bool = true) -> CitySite {
    var s = CitySite(id: id, title: title)
    s.cells.l16 = cell
    if placed { s.sensorToCity = matrix_identity_double4x4 }
    return s
}

private func counts(outstanding: Int) -> LaneReviewEntry {
    var c = LaneReviewCounts()
    c.total = outstanding
    c.outstanding = outstanding
    return .counted(c)
}

struct LaneReviewCountTests {
    @Test func countsAListLaneProposalsAnswer() {
        let c = LaneReviewCounts(fixtureResponse())
        #expect(c.total == 8)
        #expect(c.high == 4)
        #expect(c.medium == 3)
        #expect(c.adopted == 2)
        #expect(c.rejected == 1)
        // h1, h2, m1, m3 and x1: neither adopted nor rejected.
        #expect(c.outstanding == 5)
        #expect(c.summary == "8 proposals: 4 high, 3 medium; 2 adopted, 1 rejected, 5 outstanding")
        #expect(LaneReviewCounts().summary == "No proposals")
    }

    @Test func aProposalBothAdoptedAndRejectedIsNotOutstanding() {
        let c = LaneReviewCounts([
            LaneProposalView(proposal("gms:p", grade: "high", adopted: "gms:l", rejected: true))
        ])
        #expect(c.adopted == 1 && c.rejected == 1 && c.outstanding == 0)
    }

    @Test func mostOutstandingFirstThenReadingThenFailed() {
        let sites = [
            site("a", title: "Alpha", cell: "c1"), site("b", title: "Bravo", cell: "c2"),
            site("c", title: "Charlie", cell: "c3"), site("d", title: "Delta", cell: "c4"),
            site("e", title: "Echo", cell: "c5"), site("f", title: "Foxtrot", cell: "c6"),
            // Not listed: not placed, or no cell to ask about.
            site("g", title: "Golf", cell: "c7", placed: false),
            site("h", title: "Hotel", cell: ""),
        ]
        var index = LaneReviewIndex()
        index.entries = [
            "c1": counts(outstanding: 1), "c2": counts(outstanding: 4),
            "c3": .failed("unavailable: gone"), "c4": .reading, "c6": counts(outstanding: 4),
            "c7": counts(outstanding: 9),
        ]
        // Bravo and Foxtrot tie on 4 and go by title; Delta (reading) and
        // Echo (not asked) follow the counted; Charlie (failed) is last.
        #expect(index.rows(sites).map(\.id) == ["b", "f", "a", "d", "e", "c"])
        #expect(index.outstanding(for: sites[1]) == 4)
        #expect(index.outstanding(for: sites[3]) == nil)
        // Not yet counted, or failed: asked for again.
        #expect(index.cellsToRead(sites) == ["c3", "c5"])
    }

    @Test func twoSitesAtOneCellShareOneCall() {
        let sites = [site("a", title: "Alpha", cell: "c1"), site("b", title: "Bravo", cell: "c1")]
        #expect(LaneReviewIndex.cells(sites) == ["c1"])
        var index = LaneReviewIndex()
        index.entries["c1"] = counts(outstanding: 2)
        #expect(index.rows(sites).compactMap(\.counts?.outstanding) == [2, 2])
    }

    @Test func aServerThatCannotCountGivesItsReasonNotZeros() {
        let sites = [site("a", title: "Alpha", cell: "c1")]
        var lanes = Gerardus_Align_V1alpha1_GetServerInfoResponse()
        lanes.supportsLaneFields = true
        let none = Gerardus_Align_V1alpha1_GetServerInfoResponse()
        #expect(
            LaneReviewIndex.reason(
                availability: .unknown, serverInfo: nil, probeFailure: nil, sites: sites)?.contains(
                    "No Gerardus server has answered") == true)
        let unreachable = LaneReviewIndex.reason(
            availability: .unavailable, serverInfo: nil, probeFailure: "not connected", sites: sites
        )
        #expect(unreachable?.contains("cannot be reached (not connected)") == true)
        #expect(
            LaneReviewIndex.reason(
                availability: .available(serverVersion: "0.1.0"), serverInfo: none,
                probeFailure: nil, sites: sites)
                == "This Gerardus (0.1.0) serves no lane proposals: it reports "
                + "supports_lane_fields false.")
        #expect(
            LaneReviewIndex.reason(
                availability: .available(serverVersion: "0.1.0"), serverInfo: lanes,
                probeFailure: nil, sites: [site("g", title: "Golf", cell: "c", placed: false)])
                == "No placed site in the site index.")
        #expect(
            LaneReviewIndex.reason(
                availability: .available(serverVersion: "0.1.0"), serverInfo: lanes,
                probeFailure: nil, sites: sites) == nil)
    }
}

@MainActor struct LaneReviewSessionTests {
    private func session(
        _ configure: (FakeAlignmentService) -> Void = { _ in }
    ) async -> (LaneReviewSession, FakeAlignmentService) {
        let service = FakeAlignmentService()
        service.supportsLaneFields = true
        configure(service)
        let alignment = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await alignment.probe()
        return (LaneReviewSession(alignment: alignment), service)
    }

    private let sites = [
        site("a", title: "Alpha", cell: "c1"), site("b", title: "Bravo", cell: "c2"),
        site("c", title: "Charlie", cell: "c3"),
    ]

    @Test func readsEachCellOnceOneCallAtATimeAndKeepsTheCounts() async {
        let (lanes, service) = await session { service in
            service.laneListDelay = .milliseconds(5)
            service.laneProposalsByCell = [
                "c1": [proposal("p1", grade: "high")],
                "c2": [
                    proposal("p2", grade: "high"), proposal("p3", grade: "medium"),
                    proposal("p4", grade: "medium", rejected: true),
                ], "c3": [proposal("p5", grade: "high", adopted: "l")],
            ]
        }
        // Closed city mode reads nothing.
        lanes.refresh()
        #expect(lanes.pendingRead == nil)
        lanes.setActive(true, sites: sites)
        // Asked again while reading: taken up after, not a second call out.
        lanes.refresh()
        lanes.refresh()
        await lanes.pendingRead?.value
        #expect(service.laneListCells == ["c1", "c2", "c3"])
        #expect(service.maxLaneListsInFlight == 1)
        #expect(lanes.rows.map(\.id) == ["b", "a", "c"])
        #expect(lanes.rows.map { $0.counts?.outstanding } == [2, 1, 0])
        #expect(lanes.outstanding(siteID: "b") == 2)
        // None outstanding: the marker carries nothing.
        #expect(lanes.outstanding(siteID: "c") == nil)

        // Closed and opened again: the counts are the session's.
        lanes.setActive(false, sites: sites)
        lanes.setActive(true, sites: sites)
        await lanes.pendingRead?.value
        #expect(service.laneListCells.count == 3)
    }

    @Test func aReviewDropsItsCellWhichIsReadAgain() async {
        let (lanes, service) = await session { service in
            service.laneProposals = [proposal("p1", grade: "high")]
        }
        lanes.setActive(true, sites: sites)
        await lanes.pendingRead?.value
        #expect(lanes.outstanding(siteID: "b") == 1)
        service.laneProposals = [proposal("p1", grade: "high", rejected: true)]
        lanes.invalidate(cell: "c2")
        await lanes.pendingRead?.value
        #expect(service.laneListCells == ["c1", "c2", "c3", "c2"])
        #expect(lanes.outstanding(siteID: "b") == nil)
        #expect(lanes.rows.first { $0.id == "b" }?.counts?.rejected == 1)
    }

    @Test func aFailedCellSaysSoAndIsAskedAgain() async {
        let (lanes, service) = await session { service in
            service.laneListFailures = ["c1": RPCError(code: .unavailable, message: "gone")]
        }
        lanes.setActive(true, sites: sites)
        await lanes.pendingRead?.value
        let row = lanes.rows.last
        #expect(row?.id == "a")
        #expect(row?.entry == .failed("unavailable: gone"))
        #expect(lanes.outstanding(siteID: "a") == nil)
        service.laneListFailures = [:]
        lanes.setActive(false, sites: sites)
        lanes.setActive(true, sites: sites)
        await lanes.pendingRead?.value
        #expect(service.laneListCells == ["c1", "c2", "c3", "c1"])
        #expect(lanes.rows.first { $0.id == "a" }?.counts != nil)
    }

    @Test func aServerWithoutLaneFieldsIsNotAskedAndGivesItsReason() async {
        let (lanes, service) = await session { service in service.supportsLaneFields = false }
        lanes.setActive(true, sites: sites)
        await lanes.pendingRead?.value
        #expect(service.laneListCells.isEmpty)
        #expect(lanes.reason?.contains("supports_lane_fields false") == true)
        #expect(lanes.outstanding(siteID: "a") == nil)
    }

    @Test func anUnreachableGerardusGivesItsReason() async {
        let (lanes, service) = await session { service in service.reachable = false }
        lanes.setActive(true, sites: sites)
        await lanes.pendingRead?.value
        #expect(service.laneListCells.isEmpty)
        #expect(lanes.reason?.contains("Gerardus cannot be reached") == true)
    }
}

@MainActor struct LaneReviewOpenTests {
    private func replay() -> FakeSiteReplayService {
        FakeSiteReplayService(
            clips: [
                AlignmentSite(
                    clipID: "clip-columbus", l16Token: "808580f47", l13Token: "808580f4",
                    description: "S2 archive: Columbus at Broadway", sensorID: "hesai-pandar40p",
                    latitude: 37.7978, longitude: -122.4065),
                AlignmentSite(
                    clipID: "clip-west", l16Token: "8085805e5", l13Token: "8085805c",
                    description: "S2 archive: Embarcadero at Broadway", sensorID: "hesai-pandar40p",
                    latitude: 37.7989, longitude: -122.3972),
            ],
            files: [
                "clip-columbus": ["s2/20260902T1320_80858-0f4_columbus-broadway.pcapng"],
                "clip-west": ["s2/20260902T1522_80858-05c_embarcadero-broadway.pcapng"],
            ])
    }

    private func state() async throws -> (AppState, FakeAlignmentService) {
        let gerardus = FakeAlignmentService()
        gerardus.supportsLaneFields = true
        gerardus.streetUseRulesVersion = 1
        gerardus.availableLayers += [.cityStreets, .kerbs, .streetUse]
        gerardus.laneProposalsByCell = [
            "808580f47": [proposal("gms:c1", grade: "high")],
            "8085805e5": [proposal("gms:e1", grade: "high"), proposal("gms:e2", grade: "medium")],
        ]
        let alignment = AlignmentSession(makeClient: { _ in gerardus }, lookup: nil, defaults: nil)
        await alignment.probe()
        let state = AppState(autoConnectOnStartup: false, alignment: alignment, serverDefaults: nil)
        state.siteReplayServiceOverride = replay()
        state.city.setIndex(try fixtureSiteIndex(), error: nil)
        return (state, gerardus)
    }

    @Test func cityModeReadsTheListAndARowOpensThePainterWithProposals() async throws {
        let (state, gerardus) = try await state()
        state.streets.proposals.visible = false
        state.city.setCityMode(true)
        await state.laneReview.pendingRead?.value
        #expect(gerardus.laneListCells == ["808580f47", "8085805e5"])
        // Embarcadero has two outstanding, Columbus one; SoMa is not placed.
        #expect(state.laneReview.rows.map(\.id) == ["embarcadero-broadway", "columbus-broadway"])
        #expect(state.laneReview.outstanding(siteID: "embarcadero-broadway") == 2)

        let row = try #require(state.laneReview.rows.first)
        await state.laneReview.open(row.site)
        #expect(state.laneReview.openError == nil)
        #expect(state.laneReview.opening == nil)
        #expect(state.siteContext.site?.id == "embarcadero-broadway")
        #expect(state.alignment.site?.clipID == "clip-west")
        #expect(state.alignment.pose != nil)
        #expect(!state.city.isCityMode)
        #expect(state.streets.isPainting)
        #expect(state.streets.proposals.visible)
        #expect(state.showAlignmentPane)
        await state.streets.pendingLoad?.value
        #expect(state.streets.proposals.proposals.map(\.featureID).contains("gms:e1"))

        // A review in the painter drops the site's count; city mode reads
        // it again, and only it.
        gerardus.laneProposals = gerardus.laneProposalsByCell["8085805e5"] ?? []
        gerardus.laneProposalsByCell["8085805e5"] = nil
        await state.streets.proposals.review("gms:e1", .rejected, author: "op")
        #expect(gerardus.laneProposalReviews.count == 1)
        #expect(state.laneReview.index.entries["8085805e5"] == nil)
        state.city.setCityMode(true)
        await state.laneReview.pendingRead?.value
        #expect(gerardus.laneListCells.filter { $0 == "8085805e5" }.count >= 2)
        #expect(state.laneReview.outstanding(siteID: "embarcadero-broadway") == 1)
        state.city.setCityMode(false)
    }

    @Test func aSiteThatCannotBePlacedStaysInTheCityAndSaysWhy() async throws {
        let (state, gerardus) = try await state()
        state.city.setCityMode(true)
        await state.laneReview.pendingRead?.value
        // The street painter needs street use; this Gerardus has none.
        gerardus.streetUse = nil
        let columbus = try #require(state.city.index?.site(id: "columbus-broadway"))
        await state.laneReview.open(columbus)
        #expect(state.siteContext.site?.id == "columbus-broadway")
        #expect(state.alignment.pose != nil)
        #expect(!state.streets.isPainting)
        #expect(state.city.isCityMode)
        let error = try #require(state.laneReview.openError)
        #expect(error.hasPrefix("Columbus at Broadway: "))
        #expect(error.contains("no street use"))
        state.city.setCityMode(false)
    }

    @Test func aSwitchTheServerRefusesIsReported() async throws {
        let (state, _) = try await state()
        let service = replay()
        service.refuse = true
        state.siteReplayServiceOverride = service
        state.city.setCityMode(true)
        let columbus = try #require(state.city.index?.site(id: "columbus-broadway"))
        await state.laneReview.open(columbus)
        #expect(state.laneReview.openError?.contains("Columbus at Broadway") == true)
        #expect(state.city.isCityMode)
        #expect(!state.streets.isPainting)
        state.city.setCityMode(false)
    }
}
