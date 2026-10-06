//
//  AnnotationPlaybackTests.swift
//  VelocityVisualiserTests
//
//  Tests for the annotation window beside a main view that is playing: keeping
//  the replay inside the pack's frames, pacing how often the window follows,
//  and the work that is done once rather than on every pass of the loop.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

@MainActor
private func openSession(
    _ samples: [[SyntheticPack.Point]], backdrops: [SyntheticPack.Backdrop] = []
) throws -> (AnnotationSession, URL) {
    let dir = try SyntheticPack.write(samples, backdrops: backdrops)
    let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
    session.operatorName = "dd"
    return (session, dir)
}

/// Synthetic samples are at 1.0 s, 1.1 s, 1.2 s ...
private func main(at timestampNs: Int64, playing: Bool = true, finished: Bool = false, seekable: Bool = true)
    -> MainViewPlayback
{
    MainViewPlayback(
        timestampNs: timestampNs, logStartNs: 0, logEndNs: 60_000_000_000, seekable: seekable,
        playing: playing, finished: finished)
}

@MainActor
struct PackLoopTests {
    private let frame: [SyntheticPack.Point] = [(0, 0, 0, 1)]

    @Test func aPlayingMainViewIsSentBackToTheFirstFrameOnceItPassesTheLast() throws {
        let (session, dir) = try openSession([frame, frame, frame])
        defer { try? FileManager.default.removeItem(at: dir) }
        let first: Int64 = 1_000_000_000

        // Inside the pack: left alone.
        #expect(session.loopTarget(for: main(at: 1_100_000_000)) == nil)
        #expect(session.loopTarget(for: main(at: 1_200_000_000)) == nil)
        // One frame past the last.
        #expect(session.loopTarget(for: main(at: 1_300_000_000)) == first)
        // Still short of the first, as when the replay has just begun.
        #expect(session.loopTarget(for: main(at: 500_000_000)) == first)
        // Run off the end of the recording.
        #expect(session.loopTarget(for: main(at: 1_200_000_000, playing: false, finished: true)) == first)
    }

    @Test func aPausedMainViewIsLeftWhereTheOperatorPutIt() throws {
        let (session, dir) = try openSession([frame, frame, frame])
        defer { try? FileManager.default.removeItem(at: dir) }
        #expect(session.loopTarget(for: main(at: 9_000_000_000, playing: false)) == nil)
    }

    @Test func itOnlyLoopsWhenAskedToAndWhenItCan() throws {
        let (session, dir) = try openSession([frame, frame, frame])
        defer { try? FileManager.default.removeItem(at: dir) }
        let past = main(at: 9_000_000_000)

        session.loopPackFrames = false
        #expect(session.loopTarget(for: past) == nil)
        session.loopPackFrames = true
        session.syncWithMainView = false
        #expect(session.loopTarget(for: past) == nil)
        session.syncWithMainView = true
        #expect(session.loopTarget(for: main(at: 9_000_000_000, seekable: false)) == nil)
        // A replay of some other recording is not this pack's to steer.
        let elsewhere = MainViewPlayback(
            timestampNs: 9_000_000_000_000, logStartNs: 8_000_000_000_000,
            logEndNs: 9_500_000_000_000, seekable: true, playing: true)
        #expect(session.loopTarget(for: elsewhere) == nil)
    }

    @Test func aPackOfOneFrameHasNothingToLoop() throws {
        let (session, dir) = try openSession([frame])
        defer { try? FileManager.default.removeItem(at: dir) }
        #expect(session.loopTarget(for: main(at: 9_000_000_000)) == nil)
    }
}

struct FollowThrottleTests {
    @Test func aPausedMainViewIsFollowedAtOnce() {
        var throttle = FollowThrottle()
        throttle.followed(at: 10)
        throttle.measured(cost: 0.2)
        #expect(throttle.wait(now: 10.01, playing: false) == 0)
    }

    @Test func playbackIsFollowedForAThirdOfTheMainThread() {
        var throttle = FollowThrottle()
        // Nothing measured yet: follow.
        #expect(throttle.wait(now: 10, playing: true) == 0)
        throttle.followed(at: 10)
        throttle.measured(cost: 0.05)
        // 50 ms of work earns 150 ms between follows.
        #expect(abs(throttle.wait(now: 10.05, playing: true) - 0.10) < 1e-9)
        #expect(throttle.wait(now: 10.15, playing: true) == 0)
        #expect(throttle.wait(now: 11, playing: true) == 0)
    }

    @Test func itNeverFallsFurtherBehindThanHalfASecond() {
        var throttle = FollowThrottle()
        throttle.followed(at: 10)
        throttle.measured(cost: 3)
        #expect(abs(throttle.wait(now: 10, playing: true) - FollowThrottle.longestWait) < 1e-9)
    }
}

@MainActor
struct OncePerChangeTests {
    private let wall = (0..<4).map { simd_float3(10, Float($0), 0) }

    @Test func aSnapshotIsToldApartOnceHoweverOftenTheLoopComesRound() throws {
        let parked = [simd_float3(3, 3, -1)]
        let (session, dir) = try openSession(
            [[(0, 0, 0, 1)], [(0, 0, 0, 1)], [(0, 0, 0, 1)]],
            backdrops: [(0, wall), (4, wall + parked)])
        defer { try? FileManager.default.removeItem(at: dir) }

        for _ in 0..<3 {
            session.step(to: 1)
            #expect(session.backgroundChanged.isEmpty)
            #expect(session.backgroundPoints.count == 4)
            session.step(to: 2)
            // The same answer on every pass, and the signal each time the
            // update is stepped onto.
            #expect(session.backgroundChanged == [4])
            #expect(session.backgroundUpdatedHere)
            #expect(session.classCounts[PointClass.background] == 5)
        }
    }

    @Test func lookingRoundANewReturnFindsWhatGrowingEveryOldOneFound() {
        // The cheaper test has to draw the same line: within a voxel of
        // anything the last snapshot held is not a change, beyond it is.
        let before = BackgroundPoints(x: [0.1, 5], y: [0.1, 5], z: [0.1, 5])
        let after = BackgroundPoints(
            x: [0.1, 0.9, 1.1, 5.4, -0.4, 2.5], y: [0.1, 0.1, 0.1, 5, 0.1, 2.5],
            z: [0.1, 0.1, 0.1, 5, 0.1, 2.5])
        // At half a metre: 0.9 is one voxel over (kept), 1.1 is two (changed),
        // 5.4 is the same voxel as 5, -0.4 is one voxel the other way.
        #expect(AnnotationSession.changedIndices(in: after, against: before, pitch: 0.5) == [2, 5])
    }

    @Test func theObjectListIsCountedOncePerChange() throws {
        let frame = SyntheticPack.car(at: .zero)
        let (session, dir) = try openSession([frame, frame])
        defer { try? FileManager.default.removeItem(at: dir) }
        let car = session.createObject(objectClass: "car")
        #expect(session.summary(objectID: car.objectID).savedFrames == 0)

        let everything = SelectionPolygon(
            rectFrom: simd_float2(-100, -100), to: simd_float2(100, 100))
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.save())
        // The cache follows the sidecar: a save, a review and a new object
        // each show up the next time the list asks.
        #expect(session.summary(objectID: car.objectID).savedFrames == 1)
        #expect(session.markFrameReviewed(secondViewConfirmed: true))
        #expect(
            session.summary(objectID: car.objectID)
                == AnnotationSession.ObjectSummary(name: "car 1", savedFrames: 1, reviewedFrames: 1))
        let second = session.createObject(objectClass: "car")
        #expect(session.displayName(objectID: second.objectID) == "car 2")
    }

    @Test func theWholePackIsTalliedFromEveryFrame() throws {
        let frame = SyntheticPack.car(at: .zero)
        let (session, dir) = try openSession([frame, frame])
        defer { try? FileManager.default.removeItem(at: dir) }
        #expect(session.packTally == LabelTally(total: 36, agreed: 0, inQuestion: 0))

        _ = session.createObject(objectClass: "car")
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(-1, -1), to: simd_float2(0.4, 2)),
                mode: .replace))
        #expect(session.save())
        // Twelve of one frame's eighteen, by hand: a third of the pack.
        #expect(session.packTally == LabelTally(total: 36, agreed: 12, inQuestion: 0))
        #expect(abs(session.packTally.fractionAgreed - 1.0 / 3) < 1e-9)
    }
}
