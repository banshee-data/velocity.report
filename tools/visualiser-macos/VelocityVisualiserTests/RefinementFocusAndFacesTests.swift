// RefinementFocusAndFacesTests.swift
// Framing the scan picked in the refinement summary, and the toolbar's
// faces switch over the DataSF boxes and the wall planes.

import Foundation
import SwiftUI
import Testing
import simd

@testable import VelocityVisualiser

struct RefinementFocusTests {
    @Test func theCameraCentresOnTheSensorKeepingItsDirection() {
        // The reference drawn through the sensor's inverse pose: the site
        // point at the sensor's own origin lands on the scene's origin.
        let pose = SitePose(
            translation: SIMD3(-9.4, -38.5, 23),
            rotation: simd_quatd(angle: 0.6, axis: SIMD3(0, 0, 1)))
        let start = simd_float3(0, -30, 20)
        let looking = simd_float3(0, 10, 0)
        let own = RefinementSummary.focus(
            sitePoint: pose.translation, model: pose.referenceModelMatrix, cameraPosition: start,
            cameraTarget: looking)
        let ownOff: Float = simd_length(own.target)
        #expect(ownOff < 1e-4)
        // Another scan's sensor 5 m east of this one, in the sensor frame.
        let east = pose.translation + SIMD3(5, 0, 0)
        let other = RefinementSummary.focus(
            sitePoint: east, model: pose.referenceModelMatrix, cameraPosition: start,
            cameraTarget: looking)
        let expected: SIMD4<Double> = pose.inverseMatrix * SIMD4(east, 1)
        let want = simd_float3(Float(expected.x), Float(expected.y), Float(expected.z))
        let otherOff: Float = simd_length(other.target - want)
        #expect(otherOff < 1e-4)
        // The same direction, at the focus distance.
        let before = simd_normalize(start - looking)
        let after = other.position - other.target
        let distanceOff: Float = abs(simd_length(after) - RefinementSummary.focusDistanceM)
        #expect(distanceOff < 1e-3)
        let alignment: Float = simd_dot(simd_normalize(after), before)
        #expect(alignment > 0.9999)
    }

    @Test func aCameraOnItsTargetStillGetsADirection() {
        let placed = RefinementSummary.focus(
            sitePoint: SIMD3(1, 2, 3), model: matrix_identity_float4x4, cameraPosition: .zero,
            cameraTarget: .zero)
        #expect(placed.target == simd_float3(1, 2, 3))
        let back: Float = simd_length(placed.position - placed.target)
        #expect(abs(back - RefinementSummary.focusDistanceM) < 1e-3)
    }
}

@MainActor struct RefinementFocusSessionTests {
    private let site = AlignmentSite(
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
        description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

    @Test func pickingAScanFramesItsRefinedSensorOrItsAcceptedOne() async {
        let service = FakeAlignmentService()
        service.siteModelID = "sha256:model-a"
        service.siteModelScans = ["sha256:0000aa", "sha256:0000bb", "sha256:0000cc"]
        service.siteModelPositions = ["sha256:0000cc": SIMD3(4, 5, 21)]
        for (id, at) in [
            ("sha256:0000aa", SIMD3(-9.0, -38.0, 23.0)), ("sha256:0000bb", SIMD3(1.0, 2.0, 22.0)),
        ] {
            var scan = Gerardus_Align_V1alpha1_RefinedScan()
            scan.scanID = id
            scan.registrationID = "gms:refine-\(id)"
            scan.deltaM = 0.03
            scan.after.translationM.x = at.x
            scan.after.translationM.y = at.y
            scan.after.translationM.z = at.z
            service.refineResult.scans.append(scan)
        }
        let sink = FakeOverlaySink()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.sink = sink
        await session.probe()
        session.choose(site)
        session.northAzimuthText = "35"
        await session.placeFromHint()
        await session.refine()

        session.selectRefinedScan("sha256:0000bb")
        #expect(sink.focuses == [SIMD3(1, 2, 22)], "the refined pose's sensor")
        // A second click clears without moving the camera.
        session.selectRefinedScan("sha256:0000bb")
        #expect(sink.focuses.count == 1)
        // A scan the refinement left out is framed at its accepted pose.
        session.selectRefinedScan("sha256:0000cc")
        #expect(sink.focuses.last == SIMD3(4, 5, 21))
        // Reopening the column keeps the selection and does not move the camera.
        session.closeRefinementColumn()
        session.openRefinementColumn()
        #expect(session.selectedRefinedScanID == "sha256:0000cc")
        #expect(sink.focuses.count == 2)
    }
}

@MainActor struct FacesToggleTests {
    private let site = AlignmentSite(
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
        description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

    private func placed() async -> (AlignmentSession, FakeOverlaySink) {
        let service = FakeAlignmentService()
        service.availableLayers += [.boxes]
        let sink = FakeOverlaySink()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.sink = sink
        await session.probe()
        session.choose(site)
        session.northAzimuthText = "35"
        await session.placeFromHint()
        return (session, sink)
    }

    @Test func facesOffHidesTheBoxesAndTheWallPlanesAndNothingElse() async {
        let (session, sink) = await placed()
        #expect(sink.visibility?.boxes == true && sink.visibility?.wallPlanes == true)
        session.showFaces = false
        #expect(sink.visibility?.boxes == false && sink.visibility?.wallPlanes == false)
        #expect(sink.visibility?.footprints == true && sink.visibility?.observed == true)
        // The pane's own toggles stay as they were, and count again once
        // faces are back on.
        #expect(session.showBoxes && session.showWallPlanes)
        session.showWallPlanes = false
        session.showFaces = true
        #expect(sink.visibility?.boxes == true && sink.visibility?.wallPlanes == false)
    }

    @Test func theToolbarsLetterReachesTheSession() async {
        let (session, sink) = await placed()
        let state = AppState(autoConnectOnStartup: false, alignment: session)
        #expect(state.showFaces && session.showFaces)
        #expect(handleKeyPress(.toggleFaces, appState: state) == .handled)
        #expect(!state.showFaces && !session.showFaces)
        #expect(sink.visibility?.boxes == false && sink.visibility?.wallPlanes == false)
        state.showFaces = true
        #expect(session.showFaces && sink.visibility?.boxes == true)
    }
}
