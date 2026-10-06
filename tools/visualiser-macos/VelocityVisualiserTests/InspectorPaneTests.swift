//
//  InspectorPaneTests.swift
//  VelocityVisualiserTests
//
//  The side panel's visibility is showSidePanel alone. It used to also key off
//  whether a track was selected, which made the state unclosable: the Inspector
//  button toggled the flag off and the selection held the panel open anyway.
//

import XCTest

@testable import VelocityVisualiser

@available(macOS 15.0, *) @MainActor final class InspectorPaneTests: XCTestCase {

    /// Selecting a track opens the panel — by setting the flag, so the button
    /// still governs it afterwards.
    func testSelectingATrackOpensThePanel() {
        let state = AppState()
        XCTAssertFalse(state.showSidePanel)

        state.selectTrack("trk_1")

        XCTAssertTrue(state.showSidePanel, "selecting a track should open the inspector")
        XCTAssertEqual(state.selectedTrackID, "trk_1")
    }

    /// The reported bug: with a track selected, the Inspector button could not
    /// close the panel.
    func testThePanelClosesWhileATrackIsSelected() {
        let state = AppState()
        state.selectTrack("trk_1")
        XCTAssertTrue(state.showSidePanel)

        // What the Inspector button does.
        state.showSidePanel.toggle()

        XCTAssertFalse(
            state.showSidePanel,
            "the inspector stayed open with a track selected; the button cannot close it")
        XCTAssertEqual(
            state.selectedTrackID, "trk_1",
            "closing the inspector must not deselect the track")
    }

    /// And it reopens, so the toggle is a toggle.
    func testThePanelReopensAfterBeingClosed() {
        let state = AppState()
        state.selectTrack("trk_1")
        state.showSidePanel.toggle()
        state.showSidePanel.toggle()

        XCTAssertTrue(state.showSidePanel)
    }

    /// Deselecting does not force the panel shut: an operator who opened the
    /// inspector keeps it.
    func testDeselectingLeavesThePanelAsTheOperatorLeftIt() {
        let state = AppState()
        state.selectTrack("trk_1")

        state.selectTrack(nil)

        XCTAssertTrue(
            state.showSidePanel,
            "deselecting closed the inspector the operator had open")
        XCTAssertNil(state.selectedTrackID)
    }
}
