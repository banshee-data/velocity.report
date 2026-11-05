// LaneStepsTests.swift
// The painter's steps from a recording to lanes, finding the run a replay
// plays, the lane centre between painted lines, and the markings and signs
// the painter reads from the reference.

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

private func line(
    _ kind: Gerardus_Align_V1alpha1_FeatureKind, _ xyz: [Float], closed: Bool = false,
    tags: [String: String] = [:]
) -> Gerardus_Align_V1alpha1_LineFeature {
    var f = Gerardus_Align_V1alpha1_LineFeature()
    f.kind = kind
    f.xyz = xyz
    f.closed = closed
    f.tags = tags
    return f
}

private func target(_ id: String, _ points: [PlanPoint], closed: Bool = false)
    -> StreetGeometry.SnapTarget
{
    StreetGeometry.SnapTarget(id: id, layer: .marking, points: points, closed: closed)
}

struct LaneStepsTests {
    private func state(_ steps: [LaneStep], _ kind: LaneStep.Kind) -> LaneStep.State? {
        steps.first { $0.kind == kind }?.state
    }

    @Test func nothingDoneMakesThePoseNextAndTheRestWait() {
        let steps = LaneSteps.steps(LaneStepsInput())
        #expect(steps.map(\.kind) == LaneStep.Kind.allCases)
        #expect(steps.first?.state == .next)
        #expect(steps.dropFirst().allSatisfy { $0.state == .waiting })
        #expect(steps[0].detail.contains("Register"))
    }

    @Test func onlyTheFirstStepNotDoneIsNext() {
        var input = LaneStepsInput(poseAccepted: true, runID: "abcdef0123456789")
        input.runFieldTracks = 21
        let steps = LaneSteps.steps(input)
        #expect(state(steps, .pose) == .done)
        #expect(state(steps, .run) == .done)
        #expect(steps.first { $0.kind == .run }?.detail == "Run abcdef01.")
        #expect(state(steps, .field) == .done)
        #expect(state(steps, .sent) == .next)
        #expect(steps.filter { $0.state == .next }.count == 1)
    }

    @Test func aReplayWithNoRunOpenNamesTheRunToOpen() {
        let steps = LaneSteps.steps(
            LaneStepsInput(poseAccepted: true, replayRunID: "0123456789abcdef"))
        let run = try! #require(steps.first { $0.kind == .run })
        #expect(run.state == .next)
        #expect(run.title == "Open this replay as a run")
        #expect(run.detail.contains("01234567"))
        #expect(steps.first { $0.kind == .field }?.detail == "Needs a run.")
    }

    @Test func noRunAndNoReplaySaysLiveIsNeverSent() {
        let run = LaneSteps.steps(LaneStepsInput(poseAccepted: true)).first { $0.kind == .run }
        #expect(run?.title == "Open a run")
        #expect(run?.detail.contains("Live is counted on screen and never sent") == true)
    }

    @Test func aRunWithNoFieldSaysSo() {
        let field = LaneSteps.steps(LaneStepsInput(poseAccepted: true, runID: "r1")).first {
            $0.kind == .field
        }
        #expect(field?.state == .next)
        #expect(field?.detail.contains("has none for this run") == true)
    }

    @Test func noProposalsAfterASendSaysHowMuchTrafficIsNeeded() {
        var input = LaneStepsInput(poseAccepted: true, runID: "r1", runFieldTracks: 21)
        input.siteCaptures = 1
        input.siteBusiestVehicleCell = 3
        let steps = LaneSteps.steps(input)
        #expect(state(steps, .sent) == .done)
        let proposals = try! #require(steps.first { $0.kind == .proposals })
        #expect(proposals.state == .next)
        #expect(proposals.detail.contains("busiest vehicle cell here has 3 tracks"))
        #expect(proposals.detail.contains("\(LaneSteps.mediumTracks) distinct vehicle tracks"))
        #expect(proposals.detail.contains("\(LaneSteps.highTracks) along 80%"))
    }

    @Test func proposalsThenSaveThenSend() {
        var input = LaneStepsInput(poseAccepted: true, runID: "r1", runFieldTracks: 40)
        input.siteCaptures = 3
        input.proposalsHigh = 2
        input.proposalsMedium = 1
        var steps = LaneSteps.steps(input)
        #expect(steps.first { $0.kind == .proposals }?.detail.hasPrefix("2 high, 1 medium") == true)
        #expect(state(steps, .saved) == .next)
        input.unsavedChanges = true
        steps = LaneSteps.steps(input)
        #expect(steps.first { $0.kind == .saved }?.title == "Save")
        input.unsavedChanges = false
        input.savedLanes = 3
        input.snapshotUnsent = true
        steps = LaneSteps.steps(input)
        #expect(state(steps, .saved) == .done)
        #expect(state(steps, .snapshot) == .next)
        input.snapshotUnsent = false
        steps = LaneSteps.steps(input)
        #expect(steps.allSatisfy { $0.state == .done })
    }

    @Test func aServerWithoutLaneFieldsHasNoSendOrProposalSteps() {
        let steps = LaneSteps.steps(LaneStepsInput(serverSupportsFields: false))
        #expect(!steps.contains { $0.kind == .sent || $0.kind == .proposals })
        #expect(steps.map(\.kind) == [.pose, .run, .field, .saved, .snapshot])
    }

    @Test func theBusiestVehicleCellIsTheMostTracksInOne() {
        var grid = LaneFieldGrid(source: .site, frame: .site)
        var cells = LaneFieldCellList()
        cells.tracks = [5, 12, 7]
        grid.groups[.vehicle] = cells
        var people = LaneFieldCellList()
        people.tracks = [40]
        grid.groups[.pedestrian] = people
        #expect(LaneSteps.busiestVehicleCell(grid) == 12)
        #expect(LaneSteps.busiestVehicleCell(nil) == 0)
    }
}

struct ReplayRunTests {
    private let runs: [(runID: String, vrlogPath: String?)] = [
        ("a", "/Volumes/lidar/lidar/vrlog/kirk0-2026-10-01"),
        ("b", "/Users/david/code/sensor_data/lidar/vrlog/columbus-1/"),
        ("c", nil),
        ("d", "/x/dup"), ("e", "/y/dup"),
    ]

    @Test func matchesTheStoredPath() {
        #expect(
            LaneSteps.replayRun(vrlogPath: "/Volumes/lidar/lidar/vrlog/kirk0-2026-10-01", runs: runs)
                == "a")
        // A trailing slash on either side is the same directory.
        #expect(
            LaneSteps.replayRun(vrlogPath: "/Users/david/code/sensor_data/lidar/vrlog/columbus-1", runs: runs)
                == "b")
    }

    @Test func aCanonicalPathMatchesByItsDirectoryNameWhenOnlyOneRunHasIt() {
        // The server opens the resolved path; the run stores the symlinked one.
        #expect(
            LaneSteps.replayRun(vrlogPath: "/Volumes/lidar/offload/sensor_data/lidar/vrlog/columbus-1", runs: runs)
                == "b")
        #expect(LaneSteps.replayRun(vrlogPath: "/z/dup", runs: runs) == nil)
    }

    @Test func noPathOrNoMatchIsNil() {
        #expect(LaneSteps.replayRun(vrlogPath: nil, runs: runs) == nil)
        #expect(LaneSteps.replayRun(vrlogPath: " ", runs: runs) == nil)
        #expect(LaneSteps.replayRun(vrlogPath: "/nowhere/else", runs: runs) == nil)
    }
}

struct LaneCentreTests {
    // A centre line along y = 0 and a lane line along y = 3.5, east-west.
    private let centre = target("c", [PlanPoint(-20, 0), PlanPoint(20, 0)])
    private let laneLine = target("l", [PlanPoint(-20, 3.5), PlanPoint(20, 3.5)])

    @Test func aPointBetweenTwoLinesALaneApartGoesMidway() throws {
        let c = try #require(
            StreetGeometry.laneCentre(PlanPoint(4, 1.2), between: [centre, laneLine]))
        #expect(abs(c.point.x - 4) < 1e-9 && abs(c.point.y - 1.75) < 1e-9)
        #expect(abs(c.widthM - 3.5) < 1e-9)
        #expect(abs(c.distance - 0.55) < 1e-9)
    }

    @Test func linesTooFarApartOrTooCloseBoundNoLane() {
        let wide = target("w", [PlanPoint(-20, 6), PlanPoint(20, 6)])
        #expect(StreetGeometry.laneCentre(PlanPoint(0, 3), between: [centre, wide]) == nil)
        let near = target("n", [PlanPoint(-20, 1.5), PlanPoint(20, 1.5)])
        #expect(StreetGeometry.laneCentre(PlanPoint(0, 0.7), between: [centre, near]) == nil)
    }

    @Test func linesFarFromParallelBoundNoLane() {
        // 30° off the centre line, crossing y = 3 at x = 0.
        let angle = 30.0 * .pi / 180
        let skew = target(
            "s",
            [PlanPoint(-10 * cos(angle), 3 - 10 * sin(angle)), PlanPoint(10 * cos(angle), 3 + 10 * sin(angle))])
        #expect(StreetGeometry.laneCentre(PlanPoint(0, 1.5), between: [centre, skew]) == nil)
    }

    @Test func aPointOutsideBothLinesIsNotBetweenThem() {
        #expect(StreetGeometry.laneCentre(PlanPoint(0, 4), between: [centre, laneLine]) == nil)
    }

    @Test func threeLinesGiveTheLaneThePointIsIn() throws {
        let outer = target("o", [PlanPoint(-20, 7), PlanPoint(20, 7)])
        let c = try #require(
            StreetGeometry.laneCentre(PlanPoint(0, 5), between: [centre, laneLine, outer]))
        #expect(abs(c.point.y - 5.25) < 1e-9)
        #expect(abs(c.widthM - 3.5) < 1e-9)
    }

    @Test func closedMarkingsDoNotBoundALane() {
        let box = target(
            "x", [PlanPoint(-2, 3.5), PlanPoint(2, 3.5), PlanPoint(2, 6), PlanPoint(-2, 6)],
            closed: true)
        #expect(StreetGeometry.laneCentre(PlanPoint(0, 1.5), between: [centre, box]) == nil)
    }
}

struct StreetReferenceMarkingTests {
    private func reference(markings: [Gerardus_Align_V1alpha1_LineFeature], furniture: [Gerardus_Align_V1alpha1_LineFeature] = [])
        -> StreetReference
    {
        var geometry = ReferenceGeometry()
        var m = Gerardus_Align_V1alpha1_StreamReferenceResponse()
        m.layer = .laneMarkings
        m.lines.features = markings
        m.layerComplete = true
        geometry.append(m)
        var f = Gerardus_Align_V1alpha1_StreamReferenceResponse()
        f.layer = .streetFurniture
        f.lines.features = furniture
        f.layerComplete = true
        geometry.append(f)
        return StreetReference(geometry: geometry)
    }

    @Test func publicRowsPlacedOnACentrelineAreNotPaint() {
        let r = reference(markings: [
            line(.laneMarking, [0, 0, 21, 10, 0, 21], tags: ["marking": "lane_line"]),
            line(
                .laneMarking, [0, 0, 21, 10, 0, 21],
                tags: ["marking": "bike_lane", "position": "centreline"]),
        ])
        #expect(r.markings.count == 1)
        #expect(r.markings.allSatisfy { $0.layer == .marking })
        // The painter's other targets are unchanged.
        #expect(r.targets.isEmpty)
    }

    @Test func onlyLaneCentreAndEdgeLinesBoundALane() {
        let r = reference(markings: [
            line(.laneMarking, [0, 0, 21, 10, 0, 21], tags: ["marking": "lane_line"]),
            line(.laneMarking, [0, 1, 21, 10, 1, 21], tags: ["marking": "centre_line"]),
            line(.laneMarking, [0, 2, 21, 10, 2, 21], tags: ["marking": "edge_line"]),
            line(.laneMarking, [0, 3, 21, 0, 6, 21], tags: ["marking": "stop_line"]),
            line(
                .laneMarking, [0, 0, 21, 2, 0, 21, 2, 2, 21, 0, 2, 21], closed: true,
                tags: ["marking": "crosswalk"]),
        ])
        #expect(r.markings.count == 5)
        #expect(r.laneLines.count == 3)
    }

    @Test func signsStandAtTheirFacesCentreAndPolesAreLeftOut() throws {
        let r = reference(
            markings: [],
            furniture: [
                line(.pole, [3, 3, 21, 3, 3, 27]),
                line(.sign, [3.2, 3, 23.5, 3.4, 3.2, 24.3]),
            ])
        #expect(r.signs.count == 1)
        let s = try #require(r.signs.first)
        #expect(abs(s.x - 3.3) < 1e-6 && abs(s.y - 3.1) < 1e-6)
        #expect(r.sign(near: PlanPoint(3, 7)) != nil)
        #expect(r.sign(near: PlanPoint(3, 9)) == nil)
    }
}
