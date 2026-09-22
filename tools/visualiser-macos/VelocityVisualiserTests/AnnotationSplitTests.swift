//
//  AnnotationSplitTests.swift
//  VelocityVisualiserTests
//
//  Tests for what an operator does once proposals exist: listing them in a
//  useful order, and splitting an object that turned out to be two.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

// MARK: - Listing proposals

struct ProposalListingTests {
    private func proposal(
        _ id: Int, _ guess: String, frames: [Int: Int], moved: Float, fixed: Bool = false
    ) -> ObjectProposal {
        ObjectProposal(
            id: id, kind: fixed ? .fixed : .moving, classGuess: guess,
            frames: frames.mapValues { Array(0..<$0) }, travelled: moved, length: 1, height: 1)
    }

    private var proposals: [ObjectProposal] {
        [
            proposal(0, "car", frames: [0: 900, 1: 900], moved: 40),
            proposal(1, "pedestrian", frames: Dictionary(uniqueKeysWithValues: (5..<45).map { ($0, 60) }), moved: 12),
            proposal(2, "car", frames: [2: 100, 3: 300, 4: 90, 5: 310], moved: 3),
            proposal(3, "noise", frames: [0: 9, 1: 9, 2: 9], moved: 0.2),
            proposal(4, "ground", frames: [0: 50, 1: 50], moved: 0, fixed: true),
        ]
    }

    @Test func eachOrderPutsWhatItNamesFirst() {
        #expect(ProposalSort.mostFrames.sorted(proposals).first?.id == 1)
        #expect(ProposalSort.mostPoints.sorted(proposals).first?.id == 1)
        #expect(ProposalSort.furthestMoved.sorted(proposals).first?.id == 0)
        #expect(ProposalSort.earliest.sorted(proposals).map(\.id).prefix(3) == [0, 3, 4])
        // The chain whose count lurches between 100 and 300 is the least
        // steady, and goes last.
        #expect(ProposalSort.steadiest.sorted(proposals).last?.id == 2)
    }

    @Test func aTieNeverReshufflesTheList() {
        let tied = [proposal(7, "car", frames: [0: 5], moved: 1), proposal(3, "car", frames: [1: 5], moved: 1)]
        #expect(ProposalSort.mostFrames.sorted(tied).map(\.id) == [3, 7])
        #expect(ProposalSort.mostFrames.sorted(tied.reversed()).map(\.id) == [3, 7])
    }

    @Test func unsteadinessIsTheTypicalJumpInReturnCount() {
        #expect(proposals[0].unsteadiness == 0)
        // 100 -> 300 -> 90 -> 310: each step changes by about two thirds.
        #expect(proposals[2].unsteadiness > 0.6)
    }

    @Test func theFilterListsByProposedTypeAndHidesTheSpeckle() {
        var filter = ProposalFilter()
        // Small and short is hidden; fixed clutter is always listed.
        #expect(proposals.filter(filter.admits).map(\.id) == [0, 1, 2, 4])
        filter.includeSmall = true
        #expect(proposals.filter(filter.admits).count == 5)
        filter.type = "car"
        #expect(proposals.filter(filter.admits).map(\.id) == [0, 2])
        filter.type = "fixed"
        #expect(proposals.filter(filter.admits).map(\.id) == [4])

        let types = ProposalFilter.types(in: proposals)
        #expect(types.first?.name == "car")
        #expect(types.first?.count == 2)
        #expect(types.contains { $0.name == "fixed" && $0.count == 1 })
    }
}

// MARK: - Splitting

@MainActor
struct ObjectSplitTests {
    /// Two people walking side by side, a metre and a half apart, a quarter of
    /// a metre a frame. Each is the synthetic block of 18 returns: indices
    /// 0-17 and 18-35 in every frame.
    private func walkers(_ frames: Int) throws -> (AnnotationSession, URL) {
        let samples = (0..<frames).map { i in
            SyntheticPack.car(at: simd_float2(Float(i) * 0.25, 0))
                + SyntheticPack.car(at: simd_float2(Float(i) * 0.25, 1.5))
        }
        let dir = try SyntheticPack.write(samples)
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        session.operatorName = "dd"
        return (session, dir)
    }

    private let everything = SelectionPolygon(
        rectFrom: simd_float2(-100, -100), to: simd_float2(100, 100))

    @Test func whatWasOnePedestrianBecomesTwoInEveryFrame() async throws {
        let (session, dir) = try walkers(5)
        defer { try? FileManager.default.removeItem(at: dir) }
        // Labelled as one, in every frame.
        let first = session.createObject(objectClass: "pedestrian")
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.save())
        #expect(try #require(await session.propagate()).framesWritten == 4)
        #expect(session.markAllFramesReviewed(secondViewConfirmed: true) == 5)

        // In the middle frame the operator takes the second person out.
        session.step(to: 2)
        session.dismissCarried()
        let secondPerson = SelectionPolygon(
            rectFrom: simd_float2(-1, 1.2), to: simd_float2(5, 2.5))
        #expect(session.select(polygon: secondPerson, mode: .subtract))
        #expect(session.removedFromSaved == Set(18..<36))
        let revision = session.sidecar.revision

        let result = try #require(session.splitRemovedIntoNewObject(objectClass: "pedestrian"))

        #expect(result.frames == 5)
        #expect(session.sidecar.revision == revision + 1)
        #expect(session.displayName(objectID: result.objectID) == "pedestrian 2")
        for sampleID in 0..<5 {
            let kept = try #require(session.sidecar.mask(objectID: first.objectID, sampleID: sampleID))
            let moved = try #require(
                session.sidecar.mask(objectID: result.objectID, sampleID: sampleID))
            #expect(kept.pointIndices == Array(0..<18))
            #expect(moved.pointIndices == Array(18..<36))
            // The points under a review have changed, so the review has gone.
            #expect(kept.status == .proposed)
            // The frame the operator divided is theirs; the rest were carried.
            #expect(moved.provenance.algorithm == (sampleID == 2 ? nil : "footprint_split"))
        }
        // Nothing is left unsaved, and the first person is still being edited.
        #expect(session.navigationGuard() == nil)
        #expect(session.activeObjectID == first.objectID)
        #expect(session.savedSelection == Set(0..<18))
    }

    @Test func theSplitStopsWhereTheSecondObjectWasNeverInTheLabel() async throws {
        let (session, dir) = try walkers(4)
        defer { try? FileManager.default.removeItem(at: dir) }
        let first = session.createObject(objectClass: "pedestrian")
        // Frame 0 was labelled as the first person only; frames 1-3 as both.
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(-1, -1), to: simd_float2(5, 1)),
                mode: .replace))
        #expect(session.save())
        for index in 1...3 {
            session.step(to: index)
            session.dismissCarried()
            #expect(session.select(polygon: everything, mode: .replace))
            #expect(session.save())
        }
        session.step(to: 2)
        session.dismissCarried()
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(-1, 1.2), to: simd_float2(5, 2.5)),
                mode: .subtract))

        let result = try #require(session.splitRemovedIntoNewObject(objectClass: "pedestrian"))

        #expect(result.frames == 3)
        #expect(session.sidecar.mask(objectID: result.objectID, sampleID: 0) == nil)
        #expect(session.sidecar.mask(objectID: first.objectID, sampleID: 0)?.pointIndices == Array(0..<18))
    }

    @Test func aFewStrayReturnsAreNotTheSecondObject() async throws {
        // Frame 0 holds the first person and four stray returns where the
        // second would be. They are nearer the second's centre, so dividing by
        // nearness alone would hand them over as "the second person".
        let strays: [SyntheticPack.Point] = (0..<4).map { (0.3 + Float($0) * 0.05, 1.8, -1, 1) }
        let both = { (i: Int) in
            SyntheticPack.car(at: simd_float2(Float(i) * 0.25, 0))
                + SyntheticPack.car(at: simd_float2(Float(i) * 0.25, 1.5))
        }
        let dir = try SyntheticPack.write([SyntheticPack.car(at: .zero) + strays, both(1), both(2)])
        defer { try? FileManager.default.removeItem(at: dir) }
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        session.operatorName = "dd"
        let first = session.createObject(objectClass: "pedestrian")
        for index in 0...2 {
            session.step(to: index)
            session.dismissCarried()
            #expect(session.select(polygon: everything, mode: .replace))
            #expect(session.save())
        }
        session.step(to: 1)
        session.dismissCarried()
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(-1, 1.2), to: simd_float2(5, 2.5)),
                mode: .subtract))

        let result = try #require(session.splitRemovedIntoNewObject(objectClass: "pedestrian"))

        #expect(result.frames == 2)
        #expect(session.sidecar.mask(objectID: result.objectID, sampleID: 0) == nil)
        #expect(session.sidecar.mask(objectID: first.objectID, sampleID: 0)?.pointIndices.count == 22)
    }

    @Test func thereIsNothingToSplitUntilPointsAreTakenOut() throws {
        let (session, dir) = try walkers(2)
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "pedestrian")
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.save())

        #expect(session.splitRemovedIntoNewObject(objectClass: "pedestrian") == nil)
        #expect(session.lastError?.contains("Take the second object's points out") == true)
        #expect(session.sidecar.objects.count == 1)
    }
}

// MARK: - Which view is edited in

@MainActor
struct EditingViewTests {
    @Test func changingTheEditingViewMovesTheCheckViewAndDropsTheSlab() throws {
        let dir = try SyntheticPack.write([SyntheticPack.car(at: .zero), SyntheticPack.car(at: .zero)])
        defer { try? FileManager.default.removeItem(at: dir) }
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        session.setSlab(DepthSlab(minDepth: 0.8, maxDepth: 1.2))
        session.beginStroke()

        session.makeEditingView(.side)

        #expect(session.viewStandard == .side)
        #expect(session.secondViewStandard != .side)
        #expect(!session.slabIsPinned)
        // A stroke begun under the old view's rule does not survive the change.
        #expect(session.navigationGuard() == nil)
    }

    @Test func theSecondViewCheckIsAboutTheFrameAndObjectItWasMadeOn() throws {
        let dir = try SyntheticPack.write([SyntheticPack.car(at: .zero), SyntheticPack.car(at: .zero)])
        defer { try? FileManager.default.removeItem(at: dir) }
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        let car = session.createObject(objectClass: "car")
        session.secondViewChecked = true
        session.stepForward()
        #expect(!session.secondViewChecked)

        session.secondViewChecked = true
        _ = session.createObject(objectClass: "van")
        _ = session.activate(objectID: car.objectID)
        #expect(!session.secondViewChecked)
    }
}
