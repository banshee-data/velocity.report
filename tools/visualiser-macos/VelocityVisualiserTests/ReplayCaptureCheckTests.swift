//
//  ReplayCaptureCheckTests.swift
//  VelocityVisualiserTests
//
//  Whether the main view is replaying the labels' capture, by packet key:
//  the four answers and their boundaries at nine in ten and at none, the
//  keys carried from the protobuf through the main view's state, and the
//  annotation window making the check when frame sync matches a frame.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

private let everything = SelectionPolygon(
    rectFrom: simd_float2(-100, -100), to: simd_float2(100, 100))

struct ReplayCaptureCheckTests {
    private let table = Set<UInt64>(1...100)

    /// `inTable` keys from the table, then `outside` keys from beyond it.
    private func frame(_ inTable: Int, _ outside: Int) -> [UInt64] {
        let kept: [UInt64] = (0..<inTable).map { UInt64($0) + 1 }
        let foreign: [UInt64] = (0..<outside).map { UInt64($0) + 1000 }
        return kept + foreign
    }

    @Test func everyPacketInTheTableIsTheSameCapture() {
        #expect(
            ReplayCaptureCheck.evaluate(frameKeys: frame(10, 0), table: table)
                == .sameCapture(found: 10, of: 10))
    }

    @Test func nineInTenIsTheSameCaptureAndJustUnderIsPartly() {
        #expect(
            ReplayCaptureCheck.evaluate(frameKeys: frame(9, 1), table: table)
                == .sameCapture(found: 9, of: 10))
        #expect(
            ReplayCaptureCheck.evaluate(frameKeys: frame(90, 10), table: table)
                == .sameCapture(found: 90, of: 100))
        #expect(
            ReplayCaptureCheck.evaluate(frameKeys: frame(89, 11), table: table)
                == .partly(found: 89, of: 100))
        #expect(
            ReplayCaptureCheck.evaluate(frameKeys: frame(8, 1), table: table)
                == .partly(found: 8, of: 9), "eight of nine is under nine in ten")
    }

    @Test func oneKeyInTheTableIsPartlyAndNoneIsADifferentCapture() {
        #expect(
            ReplayCaptureCheck.evaluate(frameKeys: frame(1, 179), table: table)
                == .partly(found: 1, of: 180))
        #expect(
            ReplayCaptureCheck.evaluate(frameKeys: frame(0, 180), table: table)
                == .differentCapture(of: 180))
        #expect(
            ReplayCaptureCheck.evaluate(frameKeys: [1, 2], table: []) == .differentCapture(of: 2),
            "an empty table holds no frame")
    }

    @Test func aFrameWithoutKeysCannotBeChecked() {
        #expect(ReplayCaptureCheck.evaluate(frameKeys: [], table: table) == .cannotCheck)
        #expect(ReplayCaptureCheck.evaluate(frameKeys: [], table: []) == .cannotCheck)
    }

    @Test func onlyAFrameThatIsNotAllThisCaptureIsAWarning() {
        #expect(!ReplayCaptureCheck.sameCapture(found: 9, of: 10).differs)
        #expect(ReplayCaptureCheck.partly(found: 1, of: 10).differs)
        #expect(ReplayCaptureCheck.differentCapture(of: 10).differs)
        #expect(!ReplayCaptureCheck.cannotCheck.differs)
    }

    @Test func eachAnswerSaysWhatItComparedWith() {
        #expect(
            ReplayCaptureCheck.sameCapture(found: 180, of: 181).label(.labelSet)
                == "Same capture: 180 of 181 packets of the main view's frame are in this label set."
        )
        #expect(
            ReplayCaptureCheck.partly(found: 120, of: 180).label(.pack)
                == "Partly the same capture: 120 of 180 packets of the main view's frame are in "
                + "this pack.")
        #expect(
            ReplayCaptureCheck.differentCapture(of: 180).label(.labelSet)
                == "Different capture: the main view is replaying packets this label set does "
                + "not cover.")
        #expect(
            ReplayCaptureCheck.cannotCheck.label(.labelSet)
                == "Cannot check the capture: the main view's frame has no packet keys. It is an "
                + "older recording, a live source, or an older server.")
    }
}

// MARK: - The keys from the stream to the main view's state

@MainActor struct MainViewPacketKeysTests {
    @Test func theDecodedPointCloudCarriesThePacketKeys() {
        let client = VisualiserClient(address: "localhost:50051")
        var proto = Velocity_Visualiser_V1_FrameBundle()
        proto.pointCloud.x = [1]
        proto.pointCloud.y = [2]
        proto.pointCloud.z = [3]
        proto.pointCloud.pointCount = 1
        proto.pointCloud.packetKeys = [0xC0FF_EE00_0000_0001, 0xFFFF_FFFF_FFFF_FFFF, 7]
        let decoded = client.decodeFrameBundle(proto)
        #expect(
            decoded.pointCloud?.packetKeys == [0xC0FF_EE00_0000_0001, 0xFFFF_FFFF_FFFF_FFFF, 7])

        // A server or recording without return identity sends none.
        proto.pointCloud.packetKeys = []
        #expect(client.decodeFrameBundle(proto).pointCloud?.packetKeys == [])
    }

    private func pointFrame(at timestampNs: Int64, keys: [UInt64]) -> FrameBundle {
        var frame = FrameBundle()
        frame.timestampNanos = timestampNs
        frame.frameType = .foreground
        frame.pointCloud = PointCloudFrame(
            timestampNanos: timestampNs, x: [1], y: [2], z: [3], intensity: [0],
            classification: [0], pointCount: 1, packetKeys: keys)
        return frame
    }

    @Test func theMainViewsKeysAreThoseOfTheFrameAtItsTimestamp() {
        let state = AppState()
        state.onFrameReceived(pointFrame(at: 1_000, keys: [1, 2, 3]))
        #expect(state.currentTimestamp == 1_000)
        #expect(state.currentPacketKeys == [1, 2, 3])

        // A background frame has no points and the time of the frame before
        // it: the keys stay that frame's.
        var background = FrameBundle()
        background.timestampNanos = 1_000
        background.frameType = .background
        background.background = BackgroundSnapshot()
        state.onFrameReceived(background)
        #expect(state.currentPacketKeys == [1, 2, 3])

        // A point frame without keys has none, rather than the last frame's.
        state.onFrameReceived(pointFrame(at: 2_000, keys: []))
        #expect(state.currentPacketKeys.isEmpty)

        state.onFrameReceived(pointFrame(at: 3_000, keys: [9]))
        state.clearAll()
        #expect(state.currentPacketKeys.isEmpty, "a cleared view has no frame")
    }
}

// MARK: - The annotation window's check

@MainActor struct AnnotationReplayCheckTests {
    private let frame: [SyntheticPack.Point] = [(0, 0, 0, 1)]

    /// Synthetic samples are at 1.0 s, 1.1 s, 1.2 s; each is one packet.
    private func main(at timestampNs: Int64, keys: [UInt64]) -> MainViewPlayback {
        MainViewPlayback(
            timestampNs: timestampNs, logStartNs: 0, logEndNs: 60_000_000_000, seekable: true,
            packetKeys: keys)
    }

    @Test func aMatchedFrameIsCheckedAgainstThePackThenItsLabelSet() throws {
        let dir = try SyntheticPack.write([frame, frame, frame])
        defer { removePack(dir) }
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        session.operatorName = "dd"
        #expect(session.replayCheck == nil, "nothing is matched yet")
        let packets = session.pack.packets
        #expect(packets.count == 3)

        // Before a label set exists, the pack's own packets are the table.
        session.follow(main(at: 1_100_000_000, keys: [packets[1]]))
        #expect(session.replayCheck == .sameCapture(found: 1, of: 1))
        #expect(session.replayCheckSubject == .pack)

        session.follow(main(at: 1_100_000_000, keys: [packets[1], 0xBAD]))
        #expect(session.replayCheck == .partly(found: 1, of: 2))
        session.follow(main(at: 1_100_000_000, keys: [0xBAD, 0xBEEF]))
        #expect(session.replayCheck == .differentCapture(of: 2))
        #expect(session.replayCheck?.differs == true)
        session.follow(main(at: 1_100_000_000, keys: []))
        #expect(session.replayCheck == .cannotCheck)

        // Once the first save makes a label set, the same frame is checked
        // against it, without waiting for the main view to move.
        session.follow(main(at: 1_000_000_000, keys: [packets[0]]))
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.save())
        #expect(session.replayCheckSubject == .labelSet)
        #expect(session.replayCheck == .sameCapture(found: 1, of: 1))

        // A frame the pack does not hold is not checked.
        session.follow(main(at: 9_000_000_000, keys: [packets[0]]))
        #expect(session.syncStatus == .outsidePack)
        #expect(session.replayCheck == nil)

        session.follow(main(at: 1_200_000_000, keys: [packets[2]]))
        #expect(session.replayCheck != nil)
        session.syncWithMainView = false
        #expect(session.replayCheck == nil, "not following, so nothing is matched")
    }

    @Test func aFrameHeldByUnsavedChangesIsStillChecked() throws {
        let dir = try SyntheticPack.write([frame, frame])
        defer { removePack(dir) }
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        session.operatorName = "dd"
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.follow(main(at: 1_100_000_000, keys: [0xBAD])) == false)
        #expect(session.syncStatus == .heldByUnsavedChanges)
        #expect(session.replayCheck == .differentCapture(of: 1))
    }
}
