//
//  LabelSetSessionTests.swift
//  VelocityVisualiserTests
//
//  The annotation window saving into label sets: the first save makes one
//  beside the pack and links it, every save after is one journal line, an
//  operation that writes many frames is still one, a save that changes
//  nothing writes nothing, and undo and restore append rather than rewrite.
//  Two runs' packs of one capture share a label set, and an edit through one
//  keeps what only the other showed.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

private let everything = SelectionPolygon(
    rectFrom: simd_float2(-100, -100), to: simd_float2(100, 100))
private let roundTheCar = SelectionPolygon(rectFrom: simd_float2(-1, -1), to: simd_float2(12, 12))
private let roundTheWall = SelectionPolygon(rectFrom: simd_float2(19, 19), to: simd_float2(22, 21))

@MainActor private func openLabelled(
    _ dir: URL, operator name: String = "dd"
) throws -> AnnotationSession {
    let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
    session.operatorName = name
    return session
}

private func labelSetDirectory(of pack: URL) throws -> URL {
    let link = try #require(try LabelSetLink.read(packDirectory: pack))
    return link.directory(packDirectory: pack)
}

@MainActor struct LabelSetSessionTests {
    private func street() -> [[SyntheticPack.Point]] {
        [simd_float2.zero, simd_float2(0.5, 0), simd_float2(1, 0)].map {
            SyntheticPack.car(at: $0) + SyntheticPack.wall
        }
    }

    @Test func theFirstSaveMakesALabelSetBesideThePackAndLinksIt() throws {
        let dir = try SyntheticPack.write(street())
        defer { removePack(dir) }
        let session = try openLabelled(dir)
        #expect(session.readOnlyReason == nil)
        #expect(session.savedRevision == 0)
        #expect(session.membershipDigest.isEmpty)
        #expect(session.canAttachLabelSet)
        #expect(try LabelSetLink.read(packDirectory: dir) == nil, "opening writes nothing")

        let car = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())

        let labels = try labelSetDirectory(of: dir)
        #expect(labels.deletingLastPathComponent().lastPathComponent == "labels")
        #expect(
            labels.deletingLastPathComponent().deletingLastPathComponent().standardizedFileURL.path
                == dir.deletingLastPathComponent().standardizedFileURL.path)
        #expect(labels.lastPathComponent.hasPrefix("synthetic-capture-"))
        let store = try LabelSetStore.open(labels)
        #expect(try LabelSetLink.read(packDirectory: dir)?.labelSetID == store.labelSetID)
        // Revision 1 is the pack's own starting state; revision 2 the save.
        let history = try LabelSetStore.history(labels)
        #expect(history.map(\.change.operation) == ["create_from_pack", "save_mask"])
        #expect(history.allSatisfy { $0.change.author == "dd" })
        #expect(session.savedRevision == 2)
        #expect(session.membershipDigest == store.stateDigest)
        #expect(!session.canAttachLabelSet)

        // The mask is in the label set by window and return, in this pack's
        // foreground domain.
        let mask = try #require(store.state.masks.first)
        #expect(mask.objectID == car.objectID)
        #expect(mask.members.count == 18)
        #expect(mask.domainID == store.state.domains[0].domainID)
        #expect(store.state.domains.count == 1 && store.state.windows.count == 3)
        // Nothing is written in the pack but the link, and no snapshot at all.
        let packFiles = try FileManager.default.contentsOfDirectory(atPath: dir.path)
        #expect(!packFiles.contains("annotations.json"))
        #expect(!packFiles.contains("annotation-revisions"))
        #expect(try LabelSetStore.verify(labels) == 2)
    }

    @Test func eachSaveIsOneJournalLineAndABatchIsOne() async throws {
        let dir = try SyntheticPack.write(street())
        defer { removePack(dir) }
        let session = try openLabelled(dir)
        let car = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())
        let labels = try labelSetDirectory(of: dir)
        #expect(journalLines(labels).count == 2)

        // A save that changes nothing writes nothing.
        #expect(session.save())
        #expect(journalLines(labels).count == 2)

        // A propagation writes every frame it carries the car into, in one
        // revision.
        let outcome = try #require(await session.propagate(direction: 1))
        #expect(outcome.framesWritten == 2)
        #expect(journalLines(labels).count == 3)
        #expect(session.savedSampleCount(objectID: car.objectID) == 3)

        // So does applying a fixed object's mask to every sample.
        session.step(to: 0)
        session.dismissCarried()
        let building = session.createObject(objectClass: "building")
        #expect(session.select(polygon: roundTheWall, mode: .replace))
        #expect(session.applySelectionToAllSamples() == 3)
        #expect(journalLines(labels).count == 4)
        #expect(session.savedSampleCount(objectID: building.objectID) == 3)

        // And merging, which deletes an object and moves its frames.
        #expect(session.mergeObjects([building.objectID], into: car.objectID) != nil)
        #expect(journalLines(labels).count == 5)
        #expect(try LabelSetStore.verify(labels) == 5)
    }

    @Test func reopeningShowsWhatWasSaved() throws {
        let dir = try SyntheticPack.write(street())
        defer { removePack(dir) }
        let first = try openLabelled(dir)
        let car = first.createObject(objectClass: "car")
        #expect(first.select(polygon: roundTheCar, mode: .replace))
        first.maskVisibility = .partlyOccluded
        first.maskCompleteness = .complete
        #expect(first.save())
        first.step(to: 1)
        let van = first.createObject(objectClass: "van")
        #expect(first.select(polygon: roundTheWall, mode: .replace))
        #expect(first.save())

        let second = try openLabelled(dir)
        #expect(second.sidecar.masks == first.sidecar.masks)
        #expect(Set(second.sidecar.objects.map(\.objectID)) == [car.objectID, van.objectID])
        #expect(
            second.sidecar.objects.map(\.objectID) == [car.objectID, van.objectID],
            "objects are listed by where they first appear, so names keep their numbers")
        let mask = try #require(second.sidecar.mask(objectID: car.objectID, sampleID: 0))
        #expect(mask.visibility == .partlyOccluded && mask.completeness == .complete)
        #expect(second.savedRevision == first.savedRevision)
        #expect(second.membershipDigest == first.membershipDigest)
    }

    @Test func anotherWritersSaveIsAConflictAndKeepsTheSelection() throws {
        let dir = try SyntheticPack.write(street())
        defer { removePack(dir) }
        let mine = try openLabelled(dir)
        let theirs = try openLabelled(dir, operator: "other")
        _ = mine.createObject(objectClass: "car")
        #expect(mine.select(polygon: roundTheCar, mode: .replace))

        // The other window saves first, which makes the label set.
        _ = theirs.createObject(objectClass: "bus")
        #expect(theirs.select(polygon: roundTheWall, mode: .replace))
        #expect(theirs.save())

        #expect(!mine.save())
        guard case .conflict = try #require(mine.conflict) else {
            Issue.record("expected a conflict, got \(String(describing: mine.conflict))")
            return
        }
        #expect(mine.selectionCount == 18, "refusing must not discard the operator's work")
        #expect(mine.lastError?.contains("reconcile") == true)

        // After a reload the next save goes through, on top of theirs.
        mine.reload()
        #expect(mine.conflict == nil)
        _ = mine.createObject(objectClass: "car")
        #expect(mine.select(polygon: roundTheCar, mode: .replace))
        #expect(mine.save())
        let store = try LabelSetStore.open(try labelSetDirectory(of: dir))
        #expect(store.state.records.objects.count == 2)
    }

    @Test func aHeldLockIsReportedAsBusy() throws {
        let dir = try SyntheticPack.write(street())
        defer { removePack(dir) }
        let session = try openLabelled(dir)
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())
        let lock = try LabelSetLock(directory: try labelSetDirectory(of: dir))
        defer { lock.release() }
        #expect(session.select(polygon: roundTheWall, mode: .add))
        #expect(!session.save())
        #expect(session.conflict == .busy)
        #expect(session.navigationGuard() != nil, "the unsaved selection is kept")
    }

    @Test func undoAndRestoreAppendRatherThanRewrite() throws {
        let dir = try SyntheticPack.write(street())
        defer { removePack(dir) }
        let session = try openLabelled(dir)
        let car = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())  // revision 2
        // Listed once from the journal; later saves here extend the list.
        #expect(session.revisionHistory().map(\.operation) == ["create_from_pack", "save_mask"])
        #expect(session.select(polygon: roundTheWall, mode: .add))
        #expect(session.save())  // revision 3
        #expect(session.sidecar.mask(objectID: car.objectID, sampleID: 0)?.pointIndices.count == 24)

        #expect(session.undoLastSave())
        #expect(session.savedRevision == 4)
        #expect(session.sidecar.mask(objectID: car.objectID, sampleID: 0)?.pointIndices.count == 18)
        #expect(session.canonicalSelection.count == 18, "the frame shows the restored mask")
        let labels = try labelSetDirectory(of: dir)
        let tip = try #require(LabelSetStore.history(labels).last)
        #expect(tip.restoredFrom == 2 && tip.change.operation == "restore")
        #expect(tip.change.author == "dd")

        // Restoring the first revision empties the label set again, as a
        // fifth revision.
        #expect(session.restoreRevision(1))
        #expect(session.savedRevision == 5)
        #expect(session.sidecar.masks.isEmpty)
        #expect(session.revisionHistory().map(\.revision) == [1, 2, 3, 4, 5])
        let fromJournal = try LabelSetStore.history(labels).map {
            MembershipRevision(
                revision: $0.revision, author: $0.change.author, operation: $0.change.operation,
                createdUTC: $0.change.createdUTC,
                restoredFrom: $0.restoredFrom > 0 ? $0.restoredFrom : nil)
        }
        #expect(session.revisionHistory() == fromJournal)
        #expect(try LabelSetStore.verify(labels) == 5)
    }

    @Test func restoringIsRefusedOverUnsavedWork() throws {
        let dir = try SyntheticPack.write(street())
        defer { removePack(dir) }
        let session = try openLabelled(dir)
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: roundTheCar, mode: .replace))
        #expect(session.save())
        #expect(session.select(polygon: roundTheWall, mode: .add))
        #expect(!session.restoreRevision(1))
        #expect(session.lastError?.contains("Save or discard") == true)
        #expect(session.savedRevision == 2)
    }
}

// MARK: - Two runs of one capture

@MainActor struct SharedLabelSetTests {
    /// Run A and run B of one capture, beside each other in one annotation
    /// folder. B's table has an extra packet first, and B kept one return A
    /// did not (offset 8 of window 0) and lost one A kept (offset 3).
    private func twoRuns() throws -> (folder: URL, a: URL, b: URL) {
        let folder = labelSetTemporaryDirectory()
        func points(_ returns: [UInt64]) -> [SyntheticPack.Point] {
            returns.map { r in (Float(r % 400) * 0.1, 1, 0, 1) }
        }
        let a = try SyntheticPack.write(
            [points([0, 1, 2, 3, 4, 5]), points([400])],
            directory: folder.appendingPathComponent("a"), packets: [10, 11],
            frames: [(0, 1, [0, 1, 2, 3, 4, 5]), (1, 1, [400])], run: "run-a")
        let b = try SyntheticPack.write(
            [points([400, 401, 402, 404, 405, 408]), points([800])],
            directory: folder.appendingPathComponent("b"), packets: [9, 10, 11],
            frames: [(1, 1, [400, 401, 402, 404, 405, 408]), (2, 1, [800])], run: "run-b")
        return (folder, a, b)
    }

    @Test func aSecondRunAttachesAndAnEditThroughItKeepsWhatItHides() throws {
        let (folder, a, b) = try twoRuns()
        defer { try? FileManager.default.removeItem(at: folder) }

        // Run A labels offsets 0…5 of window 0.
        let viewA = try openLabelled(a)
        let car = viewA.createObject(objectClass: "car")
        #expect(viewA.select(polygon: everything, mode: .replace))
        #expect(viewA.save())
        let labels = try labelSetDirectory(of: a)

        // Run B finds A's label set by its packets and attaches to it.
        let viewB = try openLabelled(b)
        #expect(viewB.canAttachLabelSet)
        let candidate = try #require(viewB.labelSetCandidates().first)
        #expect(candidate.directory.standardizedFileURL == labels.standardizedFileURL)
        #expect(candidate.sharedPackets == 2 && candidate.packPackets == 3)
        #expect(viewB.attachLabelSet(at: candidate.directory))
        #expect(viewB.lastError == nil)
        #expect(!viewB.canAttachLabelSet)
        #expect(
            try LabelSetLink.read(packDirectory: b)?.labelSetID
                == LabelSetStore.open(labels).labelSetID)
        let afterAttach = try LabelSetStore.open(labels)
        #expect(afterAttach.state.domains.count == 2)
        #expect(afterAttach.state.packets == [10, 11, 9], "the table only grows at its end")
        #expect(try LabelSetStore.history(labels).last?.change.operation == "add_domain")

        // B shows the five of A's members it kept: offsets 0, 1, 2, 4 and 5.
        let shown = try #require(viewB.sidecar.mask(objectID: car.objectID, sampleID: 0))
        #expect(shown.pointIndices == [0, 1, 2, 3, 4])

        // B drops its point 0 (offset 0) and leaves out its point 5 (offset
        // 8, which A never kept). Offset 3, which B never showed, survives.
        #expect(viewB.activate(objectID: car.objectID) == nil)
        #expect(
            viewB.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(0.05, 0), to: simd_float2(0.55, 2)),
                mode: .replace))
        #expect(viewB.canonicalSelection == [1, 2, 3, 4])
        #expect(viewB.save())

        let after = try LabelSetStore.open(labels)
        let mask = try #require(after.state.masks.first { $0.objectID == car.objectID })
        #expect(mask.members == .of([1, 2, 3, 4, 5]))
        #expect(mask.domainID == afterAttach.state.domains.first { $0.runID == "run-a" }?.domainID)

        // Run A, reopened, sees its labels less the one B removed.
        let reopened = try openLabelled(a)
        #expect(
            reopened.sidecar.mask(objectID: car.objectID, sampleID: 0)?.pointIndices == [
                1, 2, 3, 4, 5,
            ])
        #expect(try LabelSetStore.verify(labels) == 4)
    }

    @Test func deletingAnObjectWithLabelsThisRunCannotSeeIsRefused() throws {
        let (folder, a, b) = try twoRuns()
        defer { try? FileManager.default.removeItem(at: folder) }
        let viewA = try openLabelled(a)
        let car = viewA.createObject(objectClass: "car")
        #expect(viewA.select(polygon: everything, mode: .replace))
        #expect(viewA.save())
        #expect(viewA.step(to: 1) == nil)
        viewA.dismissCarried()
        let other = viewA.createObject(objectClass: "car")
        #expect(viewA.select(polygon: everything, mode: .replace))
        #expect(viewA.save())

        let viewB = try openLabelled(b)
        #expect(viewB.attachLabelSet(at: try labelSetDirectory(of: a)))
        // Merging car into other from run B would drop offset 3 of window 0,
        // which run B does not show.
        #expect(viewB.mergeObjects([car.objectID], into: other.objectID) == nil)
        #expect(viewB.lastError?.contains("does not show") == true)
        let store = try LabelSetStore.open(try labelSetDirectory(of: a))
        #expect(store.state.masks.contains { $0.objectID == car.objectID })
    }

    @Test func aPackOfAnotherCaptureCannotAttach() throws {
        let folder = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let a = try SyntheticPack.write(
            [[(0, 0, 0, 1)]], directory: folder.appendingPathComponent("a"), packets: [1],
            frames: [(0, 1, [0])])
        let other = try SyntheticPack.write(
            [[(0, 0, 0, 1)]], directory: folder.appendingPathComponent("other"), packets: [2],
            frames: [(0, 1, [0])])
        let viewA = try openLabelled(a)
        _ = viewA.createObject(objectClass: "car")
        #expect(viewA.select(polygon: everything, mode: .replace))
        #expect(viewA.save())

        let viewOther = try openLabelled(other)
        #expect(viewOther.labelSetCandidates().isEmpty)
        #expect(!viewOther.attachLabelSet(at: try labelSetDirectory(of: a)))
        #expect(viewOther.lastError?.contains("shares no packets") == true)
        #expect(try LabelSetLink.read(packDirectory: other) == nil)
    }
}
