// RefinementSummaryTests.swift
// The refinement column's summary: its rows' order and labels, the state
// each scan is coloured by and its thresholds, what Accept refinement says
// it does, the scans the next Refine re-solves, and the scan picked in it.

import Foundation
import SwiftUI
import Testing
import simd

@testable import VelocityVisualiser

private func refined(
    _ id: String, deltaM: Double, deltaDeg: Double, rank: UInt32 = 6
) -> Gerardus_Align_V1alpha1_RefinedScan {
    var scan = Gerardus_Align_V1alpha1_RefinedScan()
    scan.scanID = id
    scan.registrationID = "gms:refine-\(id)"
    scan.deltaM = deltaM
    scan.deltaDeg = deltaDeg
    scan.jointConstraintRank = rank
    return scan
}

private func run(_ id: String, passed: Bool, gates: Int = 2) -> Gerardus_Align_V1alpha1_Registration
{
    var run = Gerardus_Align_V1alpha1_Registration()
    run.registrationID = "gms:refine-\(id)"
    run.scanID = id
    run.state = passed ? .candidate : .failed
    run.gates = (0..<gates).map { index in
        var gate = Gerardus_Align_V1alpha1_GateResult()
        gate.name = "gate_\(index)"
        // The last gate is the one a failing run fails.
        gate.passed = passed || index < gates - 1
        return gate
    }
    return run
}

private func pair(_ a: String, _ b: String, used: Bool) -> Gerardus_Align_V1alpha1_ScanPair {
    var pair = Gerardus_Align_V1alpha1_ScanPair()
    pair.scanA = a
    pair.scanB = b
    pair.used = used
    pair.reason = used ? "" : "too few matches"
    return pair
}

private func distance(_ a: SIMD4<Float>, _ b: SIMD4<Float>) -> Float {
    simd_length(SIMD3(a.x, a.y, a.z) - SIMD3(b.x, b.y, b.z))
}

struct RefinementSummaryTests {
    @Test func rowsAreSortedByMovementAndLabelledByCapture() {
        var result = Gerardus_Align_V1alpha1_RefineSiteResult()
        result.scans = [
            refined("sha256:aaaa11", deltaM: 0.002, deltaDeg: 0.01),
            refined("sha256:bbbb22", deltaM: 0.031, deltaDeg: 0.12, rank: 5),
            refined("sha256:cccc33", deltaM: 0.031, deltaDeg: 0.40),
        ]
        result.pairs = [
            pair("sha256:aaaa11", "sha256:bbbb22", used: true),
            pair("sha256:bbbb22", "sha256:cccc33", used: true),
        ]
        let rows = RefinementSummary.rows(
            refinement: result,
            runs: [
                "sha256:aaaa11": run("sha256:aaaa11", passed: true),
                "sha256:bbbb22": run("sha256:bbbb22", passed: false),
                "sha256:cccc33": run("sha256:cccc33", passed: true),
            ],
            captureRefs: [
                "sha256:aaaa11": "kirkham-0412", "sha256:bbbb22": "columbus-at-broadway-2026-04-12",
            ], siteModelScanIDs: ["sha256:aaaa11", "sha256:dddd44", "sha256:bbbb22"], stale: [])
        // Largest move first; equal moves by heading; then the model's scan
        // the refinement left out.
        #expect(
            rows.map(\.scanID) == [
                "sha256:cccc33", "sha256:bbbb22", "sha256:aaaa11", "sha256:dddd44",
            ])
        #expect(rows[0].label == "cccc33", "no capture_ref: the id alone")
        #expect(rows[1].label == "columbus-at-…-2026-04-12 · bbbb22")
        #expect(rows[2].label == "kirkham-0412 · aaaa11")
        #expect(rows[1].deltaCM == 3.1 && rows[1].deltaDeg == 0.12)
        #expect(rows[1].jointRank == 5)
        #expect(
            rows.map(\.recordsText) == [
                "REGISTERED", "MANUALLY_PLACED", "REGISTERED", "not refined",
            ])
        #expect(rows.map(\.state) == [.registered, .manuallyPlaced, .unchanged, .leftOut])
        #expect(!rows[3].refined && rows[3].leftOutReason == "not in this refinement")
        #expect(
            RefinementSummary.verdict(rows)
                == "3 scans · largest move 3.1 cm, 0.40° · 1 fails a gate · 1 left out or stale")
    }

    @Test func theVerdictSaysAllGatesPass() {
        var result = Gerardus_Align_V1alpha1_RefineSiteResult()
        result.scans = (1...7).map {
            refined("sha256:\($0)", deltaM: 0.005 * Double($0) / 7, deltaDeg: 0.05)
        }
        let runs = Dictionary(
            uniqueKeysWithValues: result.scans.map { ($0.scanID, run($0.scanID, passed: true)) })
        let rows = RefinementSummary.rows(
            refinement: result, runs: runs, captureRefs: [:], siteModelScanIDs: [], stale: [])
        #expect(
            RefinementSummary.verdict(rows)
                == "7 scans · largest move 0.5 cm, 0.05° · all gates pass")
    }

    @Test func theStateTurnsAtOneCentimetreAndAHundredthOfFiveDegrees() {
        let state = RefinementSummary.state
        #expect(state(0.0099, 0.0499, true, false) == .unchanged)
        #expect(state(0.0099, 0.0499, false, false) == .unchanged, "unchanged whatever its gates")
        #expect(state(0.01, 0, true, false) == .registered, "1 cm is moved")
        #expect(state(0, 0.05, true, false) == .registered, "0.05° is moved")
        #expect(state(0.01, 0, false, false) == .manuallyPlaced)
        #expect(state(0.5, 2, true, true) == .leftOut, "left out wins over any move")
        #expect(state(0, 0, true, true) == .leftOut)
    }

    @Test func aScanWithNoUsedPairOrAcceptedAgainIsLeftOut() {
        var result = Gerardus_Align_V1alpha1_RefineSiteResult()
        result.scans = [
            refined("sha256:a", deltaM: 0.05, deltaDeg: 0.1),
            refined("sha256:b", deltaM: 0.04, deltaDeg: 0.1),
            refined("sha256:c", deltaM: 0.03, deltaDeg: 0.1),
        ]
        result.pairs = [
            pair("sha256:a", "sha256:b", used: true), pair("sha256:b", "sha256:c", used: false),
        ]
        let runs = Dictionary(
            uniqueKeysWithValues: result.scans.map { ($0.scanID, run($0.scanID, passed: true)) })
        let rows = RefinementSummary.rows(
            refinement: result, runs: runs, captureRefs: [:], siteModelScanIDs: [],
            stale: ["sha256:a"])
        #expect(rows.map(\.state) == [.leftOut, .registered, .leftOut])
        #expect(rows[0].leftOutReason == "accepted again since this refinement ran")
        #expect(rows[2].leftOutReason == "none of its pairs was used")
        // What accepting records is still said for a scan left out.
        #expect(rows[2].records == .registered)

        // A refinement that reports no pairs leaves no scan out for that.
        result.pairs = []
        let unpaired = RefinementSummary.rows(
            refinement: result, runs: runs, captureRefs: [:], siteModelScanIDs: [], stale: [])
        #expect(unpaired.allSatisfy { $0.state == .registered })
    }

    @Test func aRunNotReadOrWithoutGatesDoesNotPass() {
        #expect(!RefinementSummary.passed(nil))
        #expect(!RefinementSummary.passed(run("a", passed: true, gates: 0)))
        #expect(RefinementSummary.passed(run("a", passed: true)))
        #expect(!RefinementSummary.passed(run("a", passed: false)))
    }

    @Test func acceptSaysItAcceptsAllOfThemTogether() {
        #expect(
            RefinementSummary.acceptText(count: 7)
                == "Accepts all 7 refined poses together. Each was solved against the others, so they are accepted as one; to leave a scan out, refine without it."
        )
        #expect(
            RefinementSummary.acceptText(count: 1).hasPrefix("Accepts the 1 refined pose together.")
        )
    }

    @Test func labelsShortenTheCaptureAndKeepSixCharactersOfTheID() {
        #expect(RefinementSummary.shortID("sha256:a5b32eee529ff479") == "9ff479")
        #expect(RefinementSummary.shortCaptureRef("short") == "short")
        let long = "6451411d-81c3-47f3-a5ff-5ce37b3ab768#f120-900"
        let short = RefinementSummary.shortCaptureRef(long)
        #expect(short.count == 24)
        #expect(short == "6451411d-81c…68#f120-900")
        #expect(
            RefinementSummary.label(scanID: "sha256:abcdef123456", captureRef: "  ") == "123456")
    }

    /// The states read apart from each other and from every reference
    /// layer's line colour, since they tint the sensors drawn among them.
    @Test func stateColoursDifferFromTheLayersAndEachOther() {
        let states = RefinedScanState.allCases.map(\.colour)
        for (i, a) in states.enumerated() {
            for b in states[(i + 1)...] { #expect(distance(a, b) >= 0.25) }
        }
        let kinds: [Gerardus_Align_V1alpha1_FeatureKind] = [
            .road, .path, .kerb, .rail, .building, .sensor, .correction, .facade, .pole, .other,
            .roofPlane, .buildingPart, .laneMarking, .sign, .tree, .wallSurface,
        ]
        var layers = kinds.map(ReferenceStyle.lineColour)
        layers += ReferenceStyle.WallPlaneRole.allCases.map(ReferenceStyle.wallPlaneColour)
        layers += [
            ReferenceStyle.boxWallColour, ReferenceStyle.intersectionColour,
            ReferenceStyle.groundMeshColour, ReferenceStyle.treeTrunkColour,
        ]
        for state in RefinedScanState.allCases {
            for layer in layers {
                #expect(distance(state.colour, layer) >= 0.25, "\(state) against \(layer)")
            }
        }
    }

    // MARK: The scene's emphasis

    private func sensor(_ scanID: String) -> Gerardus_Align_V1alpha1_LineFeature {
        var feature = Gerardus_Align_V1alpha1_LineFeature()
        feature.kind = .sensor
        feature.xyz = [0, 0, 21, 0, 0, 23, 0, 3, 23]
        feature.tags = ["scan_id": scanID, "capture_ref": "capture/\(scanID)"]
        return feature
    }

    private func correction(_ scanID: String) -> Gerardus_Align_V1alpha1_LineFeature {
        var feature = Gerardus_Align_V1alpha1_LineFeature()
        feature.kind = .correction
        feature.xyz = [-1, 0, 21.2, 0, 0, 21.2]
        feature.tags = ["scan_id": scanID]
        return feature
    }

    /// The colour of each vertex of debug-line vertices.
    private func colours(_ vertices: [Float]) -> [SIMD4<Float>] {
        stride(from: 0, to: vertices.count, by: 7).map {
            SIMD4(vertices[$0 + 3], vertices[$0 + 4], vertices[$0 + 5], vertices[$0 + 6])
        }
    }

    @Test func tintsColourTheirScansSensorAndCorrectionOnly() {
        let lines = [sensor("sha256:a"), correction("sha256:a"), sensor("sha256:b")]
        let plain = ReferenceStyle.observedVertices(lines, emphasis: ReferenceEmphasis())
        #expect(plain == ReferenceStyle.lineVertices(lines))
        let tint = RefinedScanState.manuallyPlaced.colour
        let tinted = ReferenceStyle.observedVertices(
            lines, emphasis: ReferenceEmphasis(tints: ["sha256:a": tint]))
        let drawn = colours(tinted)
        // a's sensor (2 segments here) and correction (1) in the tint, then
        // b's sensor as before: 4 + 2 + 4 vertices.
        #expect(drawn.count == 10)
        #expect(drawn[0..<6].allSatisfy { $0 == tint })
        #expect(drawn[6...].allSatisfy { $0 == ReferenceStyle.lineColour(.sensor) })
    }

    @Test func theSelectedScanIsDrawnBoldAndTheOthersDimmed() {
        let lines = [sensor("sha256:a"), sensor("sha256:b")]
        let bold = ReferenceStyle.observedVertices(
            lines, emphasis: ReferenceEmphasis(selectedScanID: "sha256:b"))
        // b's arrow five times over (as given and at four offsets), a's once.
        let boldVertices: Int = bold.count / 7
        #expect(boldVertices == 24)

        let order = ["sha256:a", "sha256:b"]
        let palette = ReferenceStyle.scanPalette(selectedScanID: "sha256:b", order: order)
        let a = ReferenceStyle.scanCode(index: 0)
        let b = ReferenceStyle.scanCode(index: 1)
        #expect(palette[b] == ReferenceStyle.palette[b])
        let full: SIMD4<Float> = ReferenceStyle.palette[a]
        let dimmed = SIMD4<Float>(full.x * 0.3, full.y * 0.3, full.z * 0.3, full.w)
        let off: Float = simd_length(palette[a] - dimmed)
        #expect(off < 1e-6)
        // The 3DEP ramps are untouched, and nothing changes without a
        // selection or for a scan that is not drawn.
        let ramps = 0..<ReferenceStyle.scanCodeBase
        #expect(Array(palette[ramps]) == Array(ReferenceStyle.palette[ramps]))
        #expect(
            ReferenceStyle.scanPalette(selectedScanID: nil, order: order) == ReferenceStyle.palette)
        #expect(
            ReferenceStyle.scanPalette(selectedScanID: "sha256:z", order: order)
                == ReferenceStyle.palette)
    }
}

// MARK: - The summary in the session

@MainActor struct RefinementSelectionTests {
    private let site = AlignmentSite(
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
        description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

    /// Placed at a site whose model holds a, b and c, with a refinement of a
    /// and b ready: a moved 4 cm and passed every gate, b barely moved.
    private func placed() async -> (AlignmentSession, FakeAlignmentService, FakeOverlaySink) {
        let service = FakeAlignmentService()
        service.siteModelID = "sha256:model-a"
        service.siteModelScans = ["sha256:0000aa", "sha256:0000bb", "sha256:0000cc"]
        service.siteModelCaptureRefs = [
            "sha256:0000aa": "kirkham-0412", "sha256:0000bb": "columbus-0413",
        ]
        service.refineResult.scans = [
            refined("sha256:0000bb", deltaM: 0.002, deltaDeg: 0.01),
            refined("sha256:0000aa", deltaM: 0.04, deltaDeg: 0.2),
        ]
        service.refineResult.pairs = [pair("sha256:0000aa", "sha256:0000bb", used: true)]
        for id in ["sha256:0000aa", "sha256:0000bb"] {
            service.runs["gms:refine-\(id)"] = run(id, passed: true)
        }
        let sink = FakeOverlaySink()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.sink = sink
        session.reviewer = "operator"
        await session.probe()
        session.choose(site)
        session.northAzimuthText = "35"
        await session.placeFromHint()
        return (session, service, sink)
    }

    @Test func theSummaryUsesTheSiteModelsCaptures() async {
        let (session, _, _) = await placed()
        #expect(session.scanCaptureRefs["sha256:0000aa"] == "kirkham-0412")
        await session.refine()
        let rows = session.refinementRows
        #expect(rows.map(\.label) == ["kirkham-0412 · 0000aa", "columbus-0413 · 0000bb", "0000cc"])
        #expect(rows.map(\.state) == [.registered, .unchanged, .leftOut])
    }

    @Test func untickedScansAreLeftOutOfTheNextRefine() async {
        let (session, service, _) = await placed()
        await session.refine()
        #expect(service.lastRefine?.scanIds == [], "every scan: the list is left empty")
        session.setIncludedInRefine("sha256:0000cc", false)
        #expect(!session.isIncludedInRefine("sha256:0000cc"))
        #expect(session.canAcceptRefinement, "the refinement shown is still accepted whole")
        await session.refine()
        #expect(service.lastRefine?.scanIds == ["sha256:0000aa", "sha256:0000bb"])
        session.setIncludedInRefine("sha256:0000bb", false)
        #expect(session.refineBlocker == "Refining needs two scans; 1 of 3 is ticked.")
        #expect(!session.canRefine)
        session.setIncludedInRefine("sha256:0000bb", true)
        #expect(session.canRefine)
    }

    @Test func pickingAScanSelectsItAndPickingItAgainClearsIt() async {
        let (session, _, _) = await placed()
        await session.refine()
        #expect(session.refinementEmphasis.selectedScanID == nil)
        #expect(
            session.refinementEmphasis.tints["sha256:0000aa"] == RefinedScanState.registered.colour)
        #expect(
            session.refinementEmphasis.tints["sha256:0000bb"] == RefinedScanState.unchanged.colour)
        #expect(
            session.refinementEmphasis.tints["sha256:0000cc"] == RefinedScanState.leftOut.colour)
        session.selectRefinedScan("sha256:0000aa")
        #expect(session.selectedRefinedScanID == "sha256:0000aa")
        #expect(session.refinementEmphasis.selectedScanID == "sha256:0000aa")
        await session.pendingStyling?.value
        session.selectRefinedScan("sha256:0000bb")
        #expect(session.selectedRefinedScanID == "sha256:0000bb")
        session.selectRefinedScan("sha256:0000bb")
        #expect(session.selectedRefinedScanID == nil, "a second click clears it")
        session.selectRefinedScan("sha256:0000aa")
        #expect(session.clearRefinedScanSelection(), "Escape clears it")
        #expect(session.selectedRefinedScanID == nil)
        #expect(!session.clearRefinedScanSelection(), "and says when there was nothing to clear")
    }

    @Test func escapeClearsTheSelectionAndIsOtherwisePassedOn() async {
        let (session, _, _) = await placed()
        let state = AppState(autoConnectOnStartup: false, alignment: session)
        #expect(handleKeyPress(.escape, appState: state) == .ignored)
        await session.refine()
        session.selectRefinedScan("sha256:0000aa")
        #expect(handleKeyPress(.escape, appState: state) == .handled)
        #expect(session.selectedRefinedScanID == nil)
    }

    @Test func theSelectionOutlivesClosingTheColumnButNotANewRefinement() async {
        let (session, _, _) = await placed()
        await session.refine()
        session.selectRefinedScan("sha256:0000aa")
        session.closeRefinementColumn()
        #expect(session.selectedRefinedScanID == "sha256:0000aa")
        #expect(session.refinementEmphasis == ReferenceEmphasis(), "nothing is drawn while closed")
        session.openRefinementColumn()
        #expect(session.refinementEmphasis.selectedScanID == "sha256:0000aa")
        await session.refine()
        #expect(session.selectedRefinedScanID == nil)
    }

    @Test func aScanAcceptedOnItsOwnTurnsStale() async {
        let (session, service, _) = await placed()
        // The open capture's scan (the fake uploads it as sha256:scan) is
        // one the refinement moved, paired with a.
        service.refineResult.scans.append(refined("sha256:scan", deltaM: 0.03, deltaDeg: 0.1))
        service.refineResult.pairs.append(pair("sha256:0000aa", "sha256:scan", used: true))
        service.runs["gms:refine-sha256:scan"] = run("sha256:scan", passed: true)
        await session.refine()
        #expect(session.refinementRows.first { $0.scanID == "sha256:scan" }?.state == .registered)
        session.noteBackground(snapshotOf(count: 4))
        await session.register()
        await session.accept()
        #expect(session.staleRefinedScanIDs == ["sha256:scan"])
        let own = session.refinementRows.first { $0.scanID == "sha256:scan" }
        #expect(own?.state == .leftOut)
        #expect(own?.leftOutReason == "accepted again since this refinement ran")
    }
}

/// A settled background of `count` points, for an upload.
private func snapshotOf(count: Int) -> BackgroundSnapshot {
    var snapshot = BackgroundSnapshot()
    snapshot.sequenceNumber = 7
    snapshot.x = (0..<count).map { Float($0) }
    snapshot.y = (0..<count).map { Float($0) * 2 }
    snapshot.z = (0..<count).map { _ in 0 }
    snapshot.confidence = (0..<count).map { _ in 1 }
    snapshot.gridMetadata = GridMetadata(
        rings: 40, azimuthBins: 1800, ringElevations: [], settlingComplete: true)
    return snapshot
}
