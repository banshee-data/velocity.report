//
//  MaskPropagationTests.swift
//  VelocityVisualiserTests
//
//  Tests for carrying a mask through the frames unattended: that it writes the
//  frames a person would have accepted, stops on the ones they would not, and
//  saves once.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

@MainActor
private func openSession(_ samples: [[SyntheticPack.Point]]) throws -> (AnnotationSession, URL) {
    let dir = try SyntheticPack.write(samples)
    let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
    session.operatorName = "dd"
    return (session, dir)
}

private let roundTheCar = SelectionPolygon(rectFrom: simd_float2(-1, -1), to: simd_float2(14, 14))
private let roundTheWall = SelectionPolygon(rectFrom: simd_float2(19, 19), to: simd_float2(23, 21))

/// A car at each position, with the wall always there.
private func drive(_ positions: [simd_float2?]) -> [[SyntheticPack.Point]] {
    positions.map { ($0.map(SyntheticPack.car(at:)) ?? []) + SyntheticPack.wall }
}

// MARK: - The rules

struct PropagationJudgeTests {
    private func fit(_ count: Int, runnerUp: Int = 0, at offset: simd_float3 = .zero) -> FootprintFit {
        FootprintFit(offset: offset, count: count, runnerUp: runnerUp)
    }

    @Test func aGoodFitIsAccepted() {
        #expect(
            PropagationJudge.verdict(
                fit: fit(95, runnerUp: 20), prediction: .zero, expected: 100, isFixed: false) == nil)
    }

    @Test func halfTheExpectedReturnsIsLost() {
        #expect(
            PropagationJudge.verdict(fit: fit(49), prediction: .zero, expected: 100, isFixed: false)
                == .lost(found: 49, expected: 100))
        // However few were expected, a handful of returns is not an object.
        #expect(
            PropagationJudge.verdict(fit: fit(4), prediction: .zero, expected: 6, isFixed: false)
                == .lost(found: 4, expected: 6))
    }

    @Test func twiceTheExpectedReturnsHasRunIntoSomething() {
        #expect(
            PropagationJudge.verdict(fit: fit(211), prediction: .zero, expected: 100, isFixed: false)
                == .grew(found: 211, expected: 100))
    }

    @Test func aRivalNearlyAsGoodMakesItAmbiguous() {
        #expect(
            PropagationJudge.verdict(
                fit: fit(100, runnerUp: 80), prediction: .zero, expected: 100, isFixed: false)
                == .ambiguous)
    }

    @Test func aFitFurtherThanTheObjectCouldMoveIsAJump() {
        // Predicted a metre a frame: allowed 0.75 + 0.5 = 1.25 m off that.
        let prediction = simd_float3(1, 0, 0)
        #expect(
            PropagationJudge.verdict(
                fit: fit(100, at: simd_float3(2.2, 0, 0)), prediction: prediction, expected: 100,
                isFixed: false) == nil)
        #expect(
            PropagationJudge.verdict(
                fit: fit(100, at: simd_float3(2.5, 0, 0)), prediction: prediction, expected: 100,
                isFixed: false) == .jumped(metres: 1.5))
    }

    @Test func aBuildingHasNoRivalAndCannotJump() {
        #expect(
            PropagationJudge.verdict(
                fit: fit(100, runnerUp: 100), prediction: .zero, expected: 100, isFixed: true) == nil)
    }

    @Test func aRecedingObjectIsExpectedToLoseReturns() {
        // Twice as far: a quarter of the returns, floored at half.
        #expect(PropagationJudge.expected(previousCount: 100, previousRange: 10, range: 14) == 51)
        #expect(PropagationJudge.expected(previousCount: 100, previousRange: 10, range: 40) == 50)
        #expect(PropagationJudge.expected(previousCount: 100, previousRange: 10, range: 5) == 200)
        #expect(PropagationJudge.expected(previousCount: 100, previousRange: 0, range: 5) == 100)
    }

    private func scan(_ points: [simd_float3]) -> PackPoints {
        PackPoints(
            x: points.map(\.x), y: points.map(\.y), z: points.map(\.z),
            intensity: [UInt8](repeating: 0, count: points.count),
            classification: [UInt8](repeating: 1, count: points.count))
    }

    @Test func aLongObjectIsNotItsOwnRival() {
        // A bus side, six metres of returns. Moved a metre along itself it
        // still covers most of them, and that is one place, not two: counting
        // it as a rival refused to carry a real car at all.
        let bus = (0..<60).flatMap { i in
            (0..<3).map { k in simd_float3(Float(i) * 0.1, 0, -1 + Float(k) * 0.4) }
        }
        let fit = SelectionFootprint(points: bus).fit(in: scan(bus), near: .zero) { _ in true }
        #expect(fit.count == bus.count)
        #expect(fit.runnerUp == 0)
    }

    /// A 9 x 1 strip of scores along x, best at the middle, as the search
    /// lays them out: x outer, y inner.
    private func strip(_ counts: [Int]) -> [(delta: simd_float3, count: Int)] {
        let side = counts.count
        var scores: [(delta: simd_float3, count: Int)] = []
        for ix in 0..<side {
            for iy in 0..<side {
                // Only the middle row carries the profile; the rest is empty.
                scores.append((simd_float3(Float(ix), Float(iy), 0), iy == side / 2 ? counts[ix] : 0))
            }
        }
        return scores
    }

    @Test func aBumpOnTheShoulderOfTheBestFitIsNotARival() {
        // Real score surfaces are not smooth. A local maximum further along
        // the same object, with no real dip before it, is the same object.
        let bumpy = [10, 20, 60, 80, 100, 80, 70, 75, 40]
        #expect(
            SelectionFootprint.rival(to: .zero, in: strip(bumpy), side: 9, step: 0.5) == 0)
        // The same second peak with a real valley before it is somewhere else.
        let twoPlaces = [10, 20, 60, 80, 100, 80, 30, 75, 40]
        #expect(
            SelectionFootprint.rival(to: .zero, in: strip(twoPlaces), side: 9, step: 0.5) == 75)
    }

    @Test func aSecondObjectTheSameShapeIsARival() {
        let car = SyntheticPack.car(at: .zero).map { simd_float3($0.x, $0.y, $0.z) }
        let twin = car.map { $0 + simd_float3(0, 1.75, 0) }
        let fit = SelectionFootprint(points: car).fit(in: scan(car + twin), near: .zero) { _ in
            true
        }
        #expect(fit.count == 18)
        #expect(fit.runnerUp == 18)
    }

    @Test func theRunnerUpIsSomewhereElseNotTheSamePlaceAVoxelOver() {
        let car = SyntheticPack.car(at: .zero).map { simd_float3($0.x, $0.y, $0.z) }
        let footprint = SelectionFootprint(points: car)
        let scan = PackPoints(
            x: car.map(\.x), y: car.map(\.y), z: car.map(\.z),
            intensity: [UInt8](repeating: 0, count: car.count),
            classification: [UInt8](repeating: 1, count: car.count))
        let alone = footprint.fit(in: scan, near: .zero) { _ in true }
        #expect(alone.count == 18)
        // A quarter of a metre to one side still covers most of the car, and
        // that is the same fit, not a rival to it.
        #expect(alone.runnerUp < 9)
    }
}

// MARK: - Through a pack

@MainActor
struct PropagationTests {
    private func labelTheCar(_ session: AnnotationSession) -> AnnotationObject {
        let car = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())
        return car
    }

    @Test func aCarIsCarriedToTheEndOfThePackInOneSave() async throws {
        let positions = (0..<6).map { simd_float2(Float($0) * 0.5, 0) }
        let (session, dir) = try openSession(drive(positions))
        defer { try? FileManager.default.removeItem(at: dir) }
        let car = labelTheCar(session)
        let revision = session.sidecar.revision

        let outcome = try #require(await session.propagate())

        #expect(outcome.framesWritten == 5)
        #expect(outcome.stop == .endOfPack)
        #expect(session.sidecar.revision == revision + 1)
        for sampleID in 1...5 {
            let mask = try #require(session.sidecar.mask(objectID: car.objectID, sampleID: sampleID))
            #expect(mask.pointIndices == Array(0..<18))
            #expect(mask.provenance.algorithm == "footprint_carry")
            #expect(mask.status == .proposed)
        }
        // Left on the last frame it wrote, with nothing pending.
        #expect(session.sampleIndex == 5)
        #expect(session.carried == nil)
        #expect(session.navigationGuard() == nil)
        // Nobody has looked at these yet.
        #expect(session.frameProgress[3].inQuestion == 18)
        #expect(session.frameProgress[3].agreed == 0)
    }

    @Test func itStopsWhereTheCarIsGoneAndHandsThatFrameOver() async throws {
        let (session, dir) = try openSession(
            drive([.zero, simd_float2(0.5, 0), simd_float2(1, 0), nil, nil]))
        defer { try? FileManager.default.removeItem(at: dir) }
        let car = labelTheCar(session)

        let outcome = try #require(await session.propagate())

        #expect(outcome.framesWritten == 2)
        #expect(outcome.stop == .lost(found: 0, expected: 18))
        #expect(outcome.stop.needsOperator)
        #expect(session.sidecar.mask(objectID: car.objectID, sampleID: 3) == nil)
        // On the frame that needs a person, with the refused fit to nudge.
        #expect(session.sampleIndex == 3)
        #expect(session.carried != nil)
        #expect(session.selectionCount == 0)
    }

    @Test func itDoesNotFollowADifferentCarThatIsFurtherThanThisOneCouldMove() async throws {
        // The car stops existing and an identical one is 1.75 m to the side:
        // inside the search, a perfect fit, and not the same car.
        let (session, dir) = try openSession(
            drive([.zero, simd_float2(0.25, 0), simd_float2(0.5, 1.75)]))
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = labelTheCar(session)

        let outcome = try #require(await session.propagate())

        #expect(outcome.framesWritten == 1)
        guard case .jumped = outcome.stop else {
            Issue.record("stopped for \(outcome.stop), not because the fit had jumped")
            return
        }
        #expect(session.sampleIndex == 2)
    }

    @Test func itStopsShortOfAnotherObjectsPoints() async throws {
        let (session, dir) = try openSession(drive([.zero, simd_float2(0.25, 0), simd_float2(0.5, 0)]))
        defer { try? FileManager.default.removeItem(at: dir) }
        // Someone has already called the car in the last frame a van.
        session.step(to: 2)
        _ = session.createObject(objectClass: "van")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())
        session.step(to: 0)
        session.dismissCarried()
        _ = labelTheCar(session)

        let outcome = try #require(await session.propagate())
        #expect(outcome.framesWritten == 1)
        #expect(outcome.stop == .overlaps(objectName: "van 1"))
    }

    @Test func itStopsAtAFrameTheObjectAlreadyHas() async throws {
        let positions = (0..<4).map { simd_float2(Float($0) * 0.5, 0) }
        let (session, dir) = try openSession(drive(positions))
        defer { try? FileManager.default.removeItem(at: dir) }
        let car = labelTheCar(session)
        session.step(to: 2)
        session.dismissCarried()
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())
        let byHand = session.sidecar.mask(objectID: car.objectID, sampleID: 2)
        session.step(to: 0)

        let outcome = try #require(await session.propagate())
        #expect(outcome.framesWritten == 1)
        #expect(outcome.stop == .alreadyLabelled)
        // What was made by hand is not overwritten.
        #expect(session.sidecar.mask(objectID: car.objectID, sampleID: 2) == byHand)
    }

    @Test func itCarriesBackwardsToo() async throws {
        let positions = (0..<4).map { simd_float2(Float($0) * 0.5, 0) }
        let (session, dir) = try openSession(drive(positions))
        defer { try? FileManager.default.removeItem(at: dir) }
        session.step(to: 3)
        let car = labelTheCar(session)

        let outcome = try #require(await session.propagate(direction: -1))
        #expect(outcome.framesWritten == 3)
        #expect(session.sampleIndex == 0)
        #expect(session.savedSampleCount(objectID: car.objectID) == 4)
    }

    @Test func aFrameLimitStopsItEarly() async throws {
        let positions = (0..<6).map { simd_float2(Float($0) * 0.5, 0) }
        let (session, dir) = try openSession(drive(positions))
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = labelTheCar(session)

        let outcome = try #require(await session.propagate(maxFrames: 2))
        #expect(outcome.framesWritten == 2)
        #expect(outcome.stop == .frameLimit)
        #expect(session.sampleIndex == 2)
    }

    @Test func aFastCarIsFollowedOnceItsSpeedIsKnown() async throws {
        // Three metres a frame is beyond the search from a standing start.
        let positions = (0..<5).map { simd_float2(Float($0) * 3, 0) }
        let (session, dir) = try openSession(drive(positions))
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = labelTheCar(session)

        let first = try #require(await session.propagate())
        #expect(first.framesWritten == 0)
        #expect(first.stop.needsOperator)

        // The operator moves the refused fit onto the car and accepts it,
        // which is also how its speed becomes known.
        let offset = try #require(session.carried).offset
        session.nudgeCarried(right: 3 - offset.x, up: -offset.y)
        #expect(session.acceptCarried())
        #expect(session.save())

        let second = try #require(await session.propagate())
        #expect(second.framesWritten == 3)
        #expect(second.stop == .endOfPack)
    }

    @Test func unsavedChangesAreSavedBeforeTheyAreCarried() async throws {
        let (session, dir) = try openSession(drive([.zero, simd_float2(0.5, 0)]))
        defer { try? FileManager.default.removeItem(at: dir) }
        let car = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))

        let outcome = try #require(await session.propagate())
        #expect(outcome.framesWritten == 1)
        #expect(session.sidecar.mask(objectID: car.objectID, sampleID: 0)?.provenance.algorithm == nil)
    }

    @Test func itNeedsAnObjectAndSomethingToCarry() async throws {
        let (session, dir) = try openSession(drive([.zero, simd_float2(0.5, 0)]))
        defer { try? FileManager.default.removeItem(at: dir) }
        #expect(await session.propagate() == nil)
        _ = session.createObject(objectClass: "car")
        #expect(await session.propagate() == nil)
        #expect(session.lastError?.contains("Label car 1") == true)
    }
}
