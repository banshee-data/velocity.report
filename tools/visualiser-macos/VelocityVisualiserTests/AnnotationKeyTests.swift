//
//  AnnotationKeyTests.swift
//  VelocityVisualiserTests
//
//  What an arrow does depends on what is on screen. These pin that order, and
//  the two column adjustments the keys reach.
//

import AppKit
import Testing

@testable import VelocityVisualiser

@MainActor
private func makeSession() throws -> (AnnotationSession, URL) {
    let dir = try PackFixture.write()
    let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
    session.operatorName = "dd"
    return (session, dir)
}

struct ViewportKeyMappingTests {
    private func key(_ code: UInt16, shift: Bool = false) -> ViewportKey? {
        let event = NSEvent.keyEvent(
            with: .keyDown, location: .zero, modifierFlags: shift ? [.shift] : [], timestamp: 0,
            windowNumber: 0, context: nil, characters: "", charactersIgnoringModifiers: "",
            isARepeat: false, keyCode: code)
        return event.flatMap { ViewportInputView.viewportKey(for: $0) }
    }

    @Test func theFourArrowsMapToTheirDirections() {
        #expect(key(123) == .nudge(right: -1, up: 0, coarse: false))
        #expect(key(124) == .nudge(right: 1, up: 0, coarse: false))
        #expect(key(125) == .nudge(right: 0, up: -1, coarse: false))
        #expect(key(126) == .nudge(right: 0, up: 1, coarse: false))
        #expect(key(126, shift: true) == .nudge(right: 0, up: 1, coarse: true))
    }

    /// The digit key codes are not in numeric order, which is the whole reason
    /// this is a lookup and not arithmetic.
    @Test func digitsZeroToSevenMapToTheirVoxels() {
        let codes: [UInt16] = [29, 18, 19, 20, 21, 23, 22, 26]
        for (expected, code) in codes.enumerated() {
            #expect(key(code) == .voxel(expected), "key code \(code)")
        }
        // 8 and 9 have no voxel, and the stack is eight deep.
        #expect(key(28) == nil)
        #expect(key(25) == nil)
    }

    @Test func returnAcceptsAndEscapeCancels() {
        #expect(key(36) == .accept)
        #expect(key(76) == .accept)
        #expect(key(53) == .cancel)
    }
}

struct ViewportKeyMeaningTests {
    /// While a proposal is on screen it claims all four arrows: moving it is
    /// the task in hand, and the frame must not step out from under it.
    @Test func aCarriedProposalClaimsEveryArrow() {
        for (right, up) in [(-1, 0), (1, 0), (0, -1), (0, 1)] {
            let key = ViewportKey.nudge(right: right, up: up, coarse: false)
            #expect(
                key.meaning(carrying: true)
                    == .nudgeCarried(right: right, up: up, coarse: false))
        }
        #expect(ViewportKey.accept.meaning(carrying: true) == .acceptCarried)
        #expect(ViewportKey.cancel.meaning(carrying: true) == .dismissCarried)
    }

    @Test func withNothingCarriedLeftAndRightStepTheFrame() {
        #expect(
            ViewportKey.nudge(right: 1, up: 0, coarse: false).meaning(carrying: false)
                == .stepFrame(forward: true))
        #expect(
            ViewportKey.nudge(right: -1, up: 0, coarse: false).meaning(carrying: false)
                == .stepFrame(forward: false))
    }

    @Test func withNothingCarriedUpAndDownMoveTheGround() {
        #expect(
            ViewportKey.nudge(right: 0, up: 1, coarse: false).meaning(carrying: false)
                == .moveGround(steps: 1, coarse: false))
        #expect(
            ViewportKey.nudge(right: 0, up: -1, coarse: true).meaning(carrying: false)
                == .moveGround(steps: -1, coarse: true))
    }

    /// Return and escape belong to the carried proposal alone; with none on
    /// screen they have to reach whatever else wants them.
    @Test func acceptAndCancelPassWhenNothingIsCarried() {
        #expect(ViewportKey.accept.meaning(carrying: false) == .pass)
        #expect(ViewportKey.cancel.meaning(carrying: false) == .pass)
    }

    @Test func aVoxelToggleWorksEitherWay() {
        #expect(ViewportKey.voxel(1).meaning(carrying: false) == .toggleVoxel(1))
        #expect(ViewportKey.voxel(1).meaning(carrying: true) == .toggleVoxel(1))
    }
}

@MainActor
struct ColumnAdjustmentTests {
    @Test func theGroundMovesInWholeStepsAndDoesNotDrift() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        session.columnGrid.groundZ = 0
        session.adjustGroundZ(steps: 1)
        #expect(session.columnGrid.groundZ == 0.05)
        session.adjustGroundZ(steps: -1)
        #expect(session.columnGrid.groundZ == 0)

        // Twenty presses have to land exactly on a metre, or the readout and
        // the grid drift apart from rounding alone.
        for _ in 0..<20 { session.adjustGroundZ(steps: 1) }
        #expect(session.columnGrid.groundZ == 1)

        session.adjustGroundZ(steps: 1, coarse: true)
        #expect(session.columnGrid.groundZ == 1.25)
    }

    /// Excluding the road while keeping the bottom of a car standing on it is
    /// one press: voxel 0 off, the rest untouched.
    @Test func togglingOneVoxelLeavesTheRestOfTheStackAlone() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        session.enabledVoxels = ColumnGrid.allVoxelsMask
        session.toggleVoxel(0)
        #expect(session.enabledVoxels == ColumnGrid.allVoxelsMask & ~1)
        session.toggleVoxel(0)
        #expect(session.enabledVoxels == ColumnGrid.allVoxelsMask)

        session.enabledVoxels = ColumnGrid.stackMask
        session.toggleVoxel(1)
        #expect(session.enabledVoxels == ColumnGrid.stackMask & ~0b10)
    }

    @Test func aVoxelOutsideTheStackIsIgnored() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        session.enabledVoxels = ColumnGrid.stackMask
        session.toggleVoxel(-1)
        session.toggleVoxel(ColumnGrid.voxelCount)
        #expect(session.enabledVoxels == ColumnGrid.stackMask)
    }
}

/// The grid angle is the scene's `grid_azimuth_deg`. The value belongs to
/// map-marks.json; what the tool keeps is a working copy and a way to hand the
/// measured one over.
@MainActor
struct GridAzimuthTests {
    @Test func aHandTypedAngleMeansWhatThePersonMeant() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        // The same wrapping the scene's own angle editor applies, so an angle
        // read off one and typed into the other survives the trip.
        session.gridAzimuthDeg = 450
        #expect(session.gridAzimuthDeg == 90)
        session.gridAzimuthDeg = -90
        #expect(session.gridAzimuthDeg == 270)
        session.gridAzimuthDeg = 360
        #expect(session.gridAzimuthDeg == 0)
        session.gridAzimuthDeg = .nan
        #expect(session.gridAzimuthDeg == 0)
    }

    /// The angle is only useful if it reaches the lattice the brush paints.
    @Test func settingTheAngleTurnsTheColumnGrid() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        session.gridAzimuthDeg = 30
        #expect(session.columnGrid.azimuthDeg == 30)
        #expect(
            session.columnGrid.cell(x: 4.2, y: 1.1)
                != ColumnGrid(pitch: session.columnGrid.pitch, groundZ: 0, azimuthDeg: 0)
                .cell(x: 4.2, y: 1.1))
    }

    /// The line has to be pasteable into map-marks.json as it stands.
    @Test func theCopiedLineIsTheOneMapMarksWants() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        session.gridAzimuthDeg = 3
        let line = session.mapMarksLine(siteID: "columbus-broadway")
        #expect(line == #"{"id": "columbus-broadway", "grid_azimuth_deg": 3}"#)

        let parsed = try JSONSerialization.jsonObject(with: Data(line.utf8)) as? [String: Any]
        #expect(parsed?["id"] as? String == "columbus-broadway")
        #expect(parsed?["grid_azimuth_deg"] as? Double == 3)
    }

    /// A pack does not know its junction, so the id is the operator's to give.
    /// An empty one leaves an obvious blank rather than a plausible wrong id.
    @Test func anUnknownSiteLeavesAPlaceholderRatherThanAGuess() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(session.mapMarksLine(siteID: "   ").contains("SITE-ID"))
    }
}
