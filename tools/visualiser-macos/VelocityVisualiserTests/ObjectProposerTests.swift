//
//  ObjectProposerTests.swift
//  VelocityVisualiserTests
//
//  Tests for proposing objects: what stays put is set aside as fixed clutter,
//  what moves is followed as one proposal an object, and an operator's grade
//  of a proposal is saved once.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

/// A kerb: ten returns in a row, the same in every frame.
private let kerb: [SyntheticPack.Point] = (0..<10).map { (30 + Float($0) * 0.3, -10, -2.5, 1) }

private func frame(_ points: [SyntheticPack.Point], labelled: Set<Int> = []) -> ProposerFrame {
    let pack = PackPoints(
        x: points.map(\.x), y: points.map(\.y), z: points.map(\.z),
        intensity: [UInt8](repeating: 0, count: points.count),
        classification: points.map(\.classification))
    return ProposerFrame(
        points: pack,
        classes: PointClass.displayClasses(
            points: pack, hasClassification: true, band: .pipelineDefault), labelled: labelled)
}

/// Runs the proposer over frames as the session does: once for what persists,
/// once to follow what does not.
private func propose(_ frames: [ProposerFrame]) -> (moving: [ObjectProposal], fixed: [ObjectProposal]) {
    var persistence = ObjectProposer.Persistence()
    for frame in frames { persistence.add(frame) }
    var proposer = ObjectProposer(persistent: persistence.persistent)
    var patches = ObjectProposer.FixedPatches(persistent: persistence.persistent)
    for (index, frame) in frames.enumerated() {
        proposer.add(frame, at: index)
        patches.add(frame, at: index)
    }
    let moving = proposer.finish()
    return (moving, patches.finish(firstID: moving.count, bandFloor: -2.8))
}

struct ObjectProposerTests {
    @Test func aCarDrivingPastAKerbIsOneMovingProposalAndOneFixed() {
        let frames = (0..<8).map { i in
            frame(SyntheticPack.car(at: simd_float2(Float(i) * 0.5, 0)) + kerb)
        }
        let (moving, fixed) = propose(frames)

        #expect(moving.count == 1)
        let car = moving[0]
        #expect(car.frames.count == 8)
        #expect(car.frames.values.allSatisfy { $0 == Array(0..<18) })
        #expect(abs(car.travelled - 3.5) < 0.1)

        #expect(fixed.count == 1)
        #expect(fixed[0].kind == .fixed)
        #expect(fixed[0].frames.count == 8)
        #expect(fixed[0].frames.values.allSatisfy { $0 == Array(18..<28) })
        // Low against the height band's floor: road the floor did not remove.
        #expect(fixed[0].classGuess == "ground")
        // Ids follow the list, moving first.
        #expect(fixed[0].id == 1)
    }

    @Test func aFastCarIsFollowedFromItsFirstFrame() {
        // A metre and a half a frame is fifteen metres a second. Nothing is
        // known of its speed at the first step, so it cannot be held to zero.
        let frames = (0..<8).map { i in frame(SyntheticPack.car(at: simd_float2(Float(i) * 1.5, 0))) }
        let (moving, _) = propose(frames)
        #expect(moving.count == 1)
        #expect(moving.first?.frames.count == 8)
        #expect(moving.first?.classGuess != "noise")
    }

    @Test func aCarBehindSomethingForTwoFramesIsStillOneCar() {
        let frames = (0..<10).map { i -> ProposerFrame in
            let hidden = i == 4 || i == 5
            return frame(hidden ? [] : SyntheticPack.car(at: simd_float2(Float(i) * 0.5, 0)))
        }
        let (moving, _) = propose(frames)
        #expect(moving.count == 1)
        #expect(moving.first?.frames.count == 8)
        #expect(moving.first?.frames[4] == nil)
        #expect(moving.first?.lastFrame == 9)
    }

    @Test func twoCarsAreTwoProposals() {
        let frames = (0..<8).map { i in
            frame(
                SyntheticPack.car(at: simd_float2(Float(i) * 0.5, 0))
                    + SyntheticPack.car(at: simd_float2(20 - Float(i) * 0.5, 6)))
        }
        let (moving, _) = propose(frames)
        #expect(moving.count == 2)
        #expect(Set(moving.map { $0.frames[0] ?? [] }) == [Array(0..<18), Array(18..<36)])
    }

    @Test func whatIsAlreadyLabelledIsNotProposedAgain() {
        let frames = (0..<8).map { i in
            frame(SyntheticPack.car(at: simd_float2(Float(i) * 0.5, 0)), labelled: Set(0..<18))
        }
        #expect(propose(frames).moving.isEmpty)
    }

    @Test func aFlickerIsNotAnObject() {
        // Three frames of something, then nothing.
        let frames = (0..<8).map { i in frame(i < 3 ? SyntheticPack.car(at: .zero) : []) }
        #expect(propose(frames).moving.isEmpty)
    }

    @Test func clustersJoinAcrossACornerButNotAcrossAGap() {
        // Two returns' worth of cells touching corner to corner, and a third
        // group two metres off.
        let blob = (0..<8).map { i -> SyntheticPack.Point in (0.1 + Float(i % 2) * 0.1, 0.1, -1, 1) }
        let corner = (0..<8).map { i -> SyntheticPack.Point in (0.6 + Float(i % 2) * 0.1, 0.6, -1, 1) }
        let apart = (0..<8).map { i -> SyntheticPack.Point in (3.1 + Float(i % 2) * 0.1, 0.1, -1, 1) }
        let clusters = ObjectProposer.clusters(
            in: frame(blob + corner + apart), excluding: [], claimed: [])
        #expect(clusters.map(\.count).sorted() == [8, 16])
        // Too few returns to seed anything.
        #expect(
            ObjectProposer.clusters(in: frame(Array(blob.prefix(5))), excluding: [], claimed: [])
                .isEmpty)
    }

    @Test func theSamePackProposesTheSameObjects() {
        let frames = (0..<8).map { i in
            frame(
                SyntheticPack.car(at: simd_float2(Float(i) * 0.5, 0))
                    + SyntheticPack.car(at: simd_float2(20 - Float(i) * 0.5, 6)) + kerb)
        }
        let first = propose(frames)
        let second = propose(frames)
        #expect(first.moving == second.moving)
        #expect(first.fixed == second.fixed)
    }

    @Test func tooFewFramesToSayWhatPersists() {
        let frames = (0..<3).map { _ in frame(kerb) }
        var persistence = ObjectProposer.Persistence()
        for frame in frames { persistence.add(frame) }
        #expect(persistence.persistent.isEmpty)
    }

    @Test func guessesAreFromSizeAndMovement() {
        #expect(ObjectProposer.guess(travelled: 0.5, length: 4, height: 1.5) == "noise")
        #expect(ObjectProposer.guess(travelled: 10, length: 0.7, height: 1.6) == "pedestrian")
        #expect(ObjectProposer.guess(travelled: 30, length: 4.4, height: 1.5) == "car")
        #expect(ObjectProposer.guess(travelled: 30, length: 11, height: 3) == "bus")
        #expect(ObjectProposer.guessFixed(length: 6, height: 2.5, top: 0.5, bandFloor: -2.8) == "building")
        #expect(ObjectProposer.guessFixed(length: 0.4, height: 2.2, top: 0, bandFloor: -2.8) == "sign")
    }
}

// MARK: - Grading a proposal

@MainActor
struct ProposalGradingTests {
    private func street(_ frames: Int = 8) throws -> (AnnotationSession, URL) {
        let samples = (0..<frames).map { i in
            SyntheticPack.car(at: simd_float2(Float(i) * 0.5, 0)) + kerb
        }
        let dir = try SyntheticPack.write(samples)
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        session.operatorName = "dd"
        return (session, dir)
    }

    @Test func acceptingAProposalIsAnObjectWithEveryFrameInOneSave() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }
        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })
        let revision = session.sidecar.revision

        #expect(session.acceptProposal(car.id, objectClass: "car") == 8)

        #expect(session.sidecar.revision == revision + 1)
        #expect(session.sidecar.objects.map(\.objectClass) == ["car"])
        #expect(session.activeObjectName == "car 1")
        #expect(session.savedSampleCount(objectID: session.activeObjectID ?? "") == 8)
        let mask = try #require(session.sidecar.masks.first)
        #expect(mask.provenance.algorithm == "cluster_chain")
        #expect(mask.provenance.operation == "accept_proposal")
        #expect(mask.status == .proposed)
        // Graded, so no longer listed; and in question until reviewed.
        #expect(!session.proposals.contains { $0.id == car.id })
        #expect(session.frameProgress[3].inQuestion == 18)
        // The frame on screen shows it as the saved mask of the new object.
        #expect(session.savedSelection == Set(0..<18))
    }

    /// Asking again part-way through grading must not throw the list away.
    /// The operator may have looked at a dozen of them and be keeping their
    /// place by id.
    @Test func proposingAgainKeepsTheListAndItsIds() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }

        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })
        // Grade one, so that a run which threw the list away would renumber
        // what is left. Without this a wipe and a rebuild are indistinguishable
        // from keeping the list: the proposer is deterministic.
        #expect(session.acceptProposal(car.id, objectClass: "car") == 8)
        let remaining = session.proposals
        #expect(!remaining.isEmpty)
        session.selectProposal(remaining[0].id)

        await session.proposeObjects()

        // Every ungraded proposal is still listed, with its id and its frames.
        for proposal in remaining {
            let still = try #require(
                session.proposals.first { $0.id == proposal.id },
                "proposal \(proposal.id) was dropped or renumbered by proposing again")
            #expect(still.frames == proposal.frames)
        }
        // And the operator's place in the list is where they left it.
        #expect(session.selectedProposalID == remaining[0].id)
    }

    /// Everything is already covered the second time round, so there is
    /// nothing left to find and no duplicate of what is listed.
    @Test func proposingAgainFindsNothingTwice() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }

        await session.proposeObjects()
        let count = session.proposals.count
        await session.proposeObjects()

        #expect(session.proposals.count == count)
        #expect(Set(session.proposals.map(\.id)).count == session.proposals.count, "ids collided")
    }

    /// A dismissal is a judgement about a suggestion. Handing it straight back
    /// on the next run would make dismissing pointless.
    @Test func aDismissedProposalDoesNotComeBack() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }

        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })
        let frames = car.frames
        session.dismissProposal(car.id)
        #expect(!session.proposals.contains { $0.id == car.id })

        await session.proposeObjects()

        #expect(!session.proposals.contains { $0.id == car.id })
        // Nor as a new proposal over the same returns.
        #expect(
            !session.proposals.contains { $0.frames == frames },
            "the dismissed proposal came back under a new id")
    }

    /// Accepting one and asking again must not re-propose what is now saved.
    @Test func whatHasBeenAcceptedIsNotProposedAgain() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }

        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })
        let frames = car.frames
        #expect(session.acceptProposal(car.id, objectClass: "car") == 8)

        await session.proposeObjects()

        #expect(!session.proposals.contains { $0.frames == frames })
    }

    /// Picking a proposal out of the list is asking to be shown it, and
    /// grading one means walking it from where it appears.
    @Test func selectingAProposalGoesToItsFirstFrame() async throws {
        let (session, dir) = try street(12)
        defer { try? FileManager.default.removeItem(at: dir) }
        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })

        // Somewhere in the middle of it, which is where stepping leaves you.
        #expect(session.step(to: 6) == nil)
        #expect(session.sampleIndex == 6)

        session.selectProposal(car.id)

        #expect(session.sampleIndex == car.firstFrame)
    }

    /// Same reason as a proposal: choosing an object out of the list is asking
    /// to be shown it, and working through it means starting where it appears.
    @Test func activatingAnObjectGoesToItsFirstLabelledFrame() async throws {
        let (session, dir) = try street(12)
        defer { try? FileManager.default.removeItem(at: dir) }
        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })
        // Only part of it, so the object starts somewhere other than frame 0.
        #expect(session.acceptProposal(car.id, objectClass: "car", frames: 4...9) == 6)
        let object = try #require(session.sidecar.objects.first)
        let first = try #require(session.firstLabelledFrame(objectID: object.objectID))
        #expect(first == 4)

        #expect(session.step(to: 11) == nil)
        #expect(session.activate(objectID: object.objectID) == nil)

        #expect(session.sampleIndex == first)
        // And its mask for that frame is loaded, not the frame it came from.
        #expect(!session.savedSelection.isEmpty)
    }

    /// The repair for a chain that came back as two objects because it went
    /// behind a bus, and the undo for a split that should not have happened.
    @Test func mergingAnObjectTakesItsFramesAndRemovesIt() async throws {
        let (session, dir) = try street(12)
        defer { try? FileManager.default.removeItem(at: dir) }
        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })

        // One chain accepted as two objects, the way a broken chain arrives.
        #expect(session.acceptProposal(car.id, objectClass: "car", frames: 0...5) == 6)
        let first = try #require(session.activeObjectID)
        let rest = try #require(session.proposals.first { $0.kind == .moving })
        #expect(session.acceptProposal(rest.id, objectClass: "car") != nil)
        let second = try #require(session.activeObjectID)
        #expect(first != second)
        let framesOfEach =
            session.savedSampleCount(objectID: first) + session.savedSampleCount(objectID: second)

        #expect(session.mergeObject(second, into: first) == framesOfEach)

        #expect(session.sidecar.objects.map(\.objectID) == [first])
        #expect(session.sidecar.masks.allSatisfy { $0.objectID == first })
        #expect(session.savedSampleCount(objectID: first) == framesOfEach)
        // The object under edit followed the merge rather than vanishing.
        #expect(session.activeObjectID == first)
    }

    /// Two masks over one thing are two accounts of the same returns, so the
    /// union is what the thing actually covered.
    @Test func mergingUnionsAFrameBothObjectsHave() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }
        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })
        #expect(session.acceptProposal(car.id, objectClass: "car") == 8)
        let target = try #require(session.activeObjectID)
        let patch = try #require(session.proposals.first { $0.kind == .fixed })
        #expect(session.acceptProposal(patch.id, objectClass: "ground") == 8)
        let other = try #require(session.activeObjectID)

        let sampleID = session.samples[0].sampleID
        let before = Set(
            (session.sidecar.mask(objectID: target, sampleID: sampleID)?.pointIndices ?? [])
                + (session.sidecar.mask(objectID: other, sampleID: sampleID)?.pointIndices ?? []))
        #expect(before.count > 0)

        #expect(session.mergeObject(other, into: target) != nil)

        let merged = try #require(session.sidecar.mask(objectID: target, sampleID: sampleID))
        #expect(Set(merged.pointIndices) == before)
        #expect(merged.pointIndices == merged.pointIndices.sorted(), "indices left unsorted")
        // What was checked was two objects; nobody has looked at the one.
        #expect(merged.status == .proposed)
    }

    @Test func mergingRefusesTheCasesThatWouldLoseWork() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }
        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })
        #expect(session.acceptProposal(car.id, objectClass: "car") == 8)
        let object = try #require(session.activeObjectID)

        #expect(session.mergeObject(object, into: object) == nil)
        #expect(session.mergeObject("obj_nosuch", into: object) == nil)
        #expect(session.mergeObject(object, into: "obj_nosuch") == nil)
        // Nothing was written by any of them.
        #expect(session.savedSampleCount(objectID: object) == 8)
    }

    @Test func fixedClutterIsOneProposalCoveringEveryFrame() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }
        await session.proposeObjects()
        let patch = try #require(session.proposals.first { $0.kind == .fixed })

        #expect(session.acceptProposal(patch.id, objectClass: "ground") == 8)
        #expect(session.sidecar.masks.allSatisfy { $0.provenance.algorithm == "persistent_voxels" })
        #expect(session.sidecar.masks.allSatisfy { $0.pointIndices == Array(18..<28) })
    }

    @Test func acceptingPartOfAProposalLeavesTheRestProposed() async throws {
        let (session, dir) = try street(12)
        defer { try? FileManager.default.removeItem(at: dir) }
        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })

        #expect(session.acceptProposal(car.id, objectClass: "car", frames: 0...4) == 5)

        let rest = try #require(session.proposals.first { $0.kind == .moving })
        #expect(rest.firstFrame == 5)
        #expect(rest.frames.count == 7)
    }

    @Test func aProposalCanBeAddedToAnObjectThatExists() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }
        // The operator labelled the first frame by hand.
        let car = session.createObject(objectClass: "car")
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(-1, -1), to: simd_float2(2, 2)),
                mode: .replace))
        #expect(session.save())
        let byHand = session.sidecar.mask(objectID: car.objectID, sampleID: 0)

        await session.proposeObjects()
        let chain = try #require(session.proposals.first { $0.kind == .moving })
        // What was labelled is not proposed again, so the chain starts after it.
        #expect(chain.firstFrame == 1)

        #expect(session.acceptProposal(chain.id, objectClass: "car", into: car.objectID) == 7)
        #expect(session.sidecar.objects.count == 1)
        #expect(session.savedSampleCount(objectID: car.objectID) == 8)
        #expect(session.sidecar.mask(objectID: car.objectID, sampleID: 0) == byHand)
    }

    @Test func aFrameTheObjectAlreadyHasIsLeftAsItWasMade() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }
        // Proposed first, so the chain covers frame 0, and then half the car
        // is labelled there by hand.
        await session.proposeObjects()
        let chain = try #require(session.proposals.first { $0.kind == .moving })
        #expect(chain.firstFrame == 0)
        let car = session.createObject(objectClass: "car")
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(-1, -1), to: simd_float2(0.4, 2)),
                mode: .replace))
        #expect(session.save())
        let byHand = try #require(session.sidecar.mask(objectID: car.objectID, sampleID: 0))
        #expect(byHand.pointIndices.count < 18)

        #expect(session.acceptProposal(chain.id, objectClass: "car", into: car.objectID) == 7)
        #expect(session.sidecar.mask(objectID: car.objectID, sampleID: 0) == byHand)
    }

    @Test func lookingAtAProposalGoesToItAndShowsItInEachFrame() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }
        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })

        session.selectProposal(car.id)
        #expect(session.proposalIndices == Array(0..<18))
        session.stepForward()
        #expect(session.proposalIndices == Array(0..<18))
        // Everything waiting in this frame: the car and the kerb.
        #expect(session.proposedIndices.count == 28)

        session.dismissProposal(car.id)
        #expect(session.selectedProposalID == nil)
        #expect(session.proposalIndices.isEmpty)
        #expect(session.sidecar.masks.isEmpty)
    }

    @Test func gradingWaitsForUnsavedChanges() async throws {
        let (session, dir) = try street()
        defer { try? FileManager.default.removeItem(at: dir) }
        await session.proposeObjects()
        let car = try #require(session.proposals.first { $0.kind == .moving })
        _ = session.createObject(objectClass: "van")
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(-1, -1), to: simd_float2(2, 2)),
                mode: .replace))

        #expect(session.acceptProposal(car.id, objectClass: "car") == nil)
        #expect(session.proposals.contains { $0.id == car.id })
    }
}
