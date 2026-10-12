//
//  LabelSetStoreTests.swift
//  VelocityVisualiserTests
//
//  The label-set store's save protocol, mirroring Go's store_test.go: one
//  journal line per save and none for a save that changes nothing, the
//  conflict and busy refusals, restore as a new revision, and recovery from a
//  save interrupted between the journal and the head.
//

import Foundation
import Testing

@testable import VelocityVisualiser

struct LabelSetStoreTests {
    @Test func createSaveReopenAndReplay() throws {
        let (dir, store) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        #expect(store.revision == 1)
        for offset: UInt64 in [40, 41, 45] {
            addMember(store, offset)
            try store.save(
                change: LabelSetChange(author: "dd", session: "s1", operation: "save_mask"))
        }
        #expect(store.revision == 4)

        // A save that changes nothing writes nothing.
        let journalBefore = try Data(contentsOf: dir.appendingPathComponent(LabelSetFiles.journal))
        try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        #expect(store.revision == 4)
        #expect(
            try Data(contentsOf: dir.appendingPathComponent(LabelSetFiles.journal)) == journalBefore
        )

        let back = try LabelSetStore.open(dir)
        #expect(back.revision == 4)
        #expect(back.stateDigest == store.stateDigest)
        #expect(try LabelSetStore.verify(dir) == 4)
        let r2 = try LabelSetStore.loadRevision(dir, revision: 2)
        let mask = try #require(r2.masks.first { $0.objectID == "obj_a" && $0.windowID == 0 })
        #expect(mask.members.contains(40) && !mask.members.contains(41))

        let lines = journalLines(dir)
        #expect(lines.count == 4)
        // A typical save is small: one return of one mask.
        #expect(lines[3].utf8.count < 600, "\(lines[3])")
        #expect(lines[3].contains(#""members_add":[[45,1]]"#))
    }

    @Test func aStaleEditIsAConflictNotAnOverwrite() throws {
        let (dir, a) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        let b = try LabelSetStore.open(dir)
        addMember(a, 40)
        try a.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        addMember(b, 41)
        #expect(throws: LabelSetError.conflict(loadedRevision: 1, currentRevision: 2, detail: nil))
        { try b.save(change: LabelSetChange(author: "dd", operation: "save_mask")) }
        // A's revision is intact.
        #expect(try LabelSetStore.open(dir).stateDigest == a.stateDigest)

        // Editing the head outside the store is caught too.
        try Data(
            Array(#"{"correspondences":[],"dismissed_proposals":[],"objects":[]}"#.utf8) + [0x0A]
        ).write(to: dir.appendingPathComponent(LabelSetFiles.records))
        addMember(a, 42)
        #expect(throws: LabelSetError.self) {
            try a.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        }
    }

    @Test func anInvalidEditIsRefusedBeforeAnythingIsWritten() throws {
        let (dir, store) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        store.state.masks[0].members = ReturnRanges([ReturnRange(start: 700, length: 1)])
        #expect(throws: LabelSetError.self) {
            try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        }
        #expect(journalLines(dir).count == 1)
    }

    @Test func aHeldLockIsBusyRatherThanAWait() throws {
        let (dir, store) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        let held = try LabelSetLock(directory: dir)
        defer { held.release() }
        addMember(store, 40)
        #expect(throws: LabelSetError.busy) {
            try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        }
        held.release()
        try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        #expect(store.revision == 2)
        // The lock file outlives its holders, so writers share one inode.
        #expect(FileManager.default.fileExists(atPath: dir.appendingPathComponent(".lock").path))
    }

    @Test func restoreAppendsARevisionThatRecreatesTheOldOne() throws {
        let (dir, store) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        addMember(store, 40)
        try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        let want = try LabelSetStore.loadRevision(dir, revision: 1).digest()

        try store.restore(revision: 1, change: LabelSetChange(author: "dd", operation: "undo"))
        #expect(store.revision == 3)
        #expect(store.stateDigest == want)
        let tip = try #require(LabelSetStore.history(dir).last)
        #expect(tip.restoredFrom == 1)
        #expect(tip.change.operation == "restore")
        // Nothing earlier was rewritten, and a proposal stayed a proposal.
        #expect(try LabelSetStore.verify(dir) == 3)
        #expect(store.state.masks.first { $0.objectID == "obj_b" }?.status == .proposed)
    }

    @Test func openingRollsAnInterruptedSaveForward() throws {
        let (dir, store) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        var saved: [String: Data] = [:]
        for name in LabelSetFiles.state + [LabelSetFiles.manifest] {
            saved[name] = try Data(contentsOf: dir.appendingPathComponent(name))
        }
        addMember(store, 40)
        try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        let want = store.stateDigest
        // The old head back, the new journal line kept: a crash after the
        // append and before the head was replaced.
        for (name, bytes) in saved { try bytes.write(to: dir.appendingPathComponent(name)) }

        let back = try LabelSetStore.open(dir)
        #expect(back.revision == 2)
        #expect(back.stateDigest == want)
        #expect(try LabelSetStore.verify(dir) == 2)
    }

    @Test func aTornFinalLineIsDroppedNotRead() throws {
        let (dir, store) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        let handle = try FileHandle(forWritingTo: dir.appendingPathComponent(LabelSetFiles.journal))
        try handle.seekToEnd()
        try handle.write(contentsOf: Data(#"{"revision":2,"parent"#.utf8))
        try handle.close()

        _ = try LabelSetStore.open(dir)
        addMember(store, 40)
        try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        #expect(try LabelSetStore.verify(dir) == 2)
        #expect(journalLines(dir).count == 2)
    }

    @Test func aCopyWithoutHistoryStartsAJournalFromItsHead() throws {
        let (dir, store) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        addMember(store, 40)
        try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        // A published copy: the head and labels.json, no journal.
        let published = dir.deletingLastPathComponent().appendingPathComponent("published")
        try FileManager.default.createDirectory(at: published, withIntermediateDirectories: true)
        for name in LabelSetFiles.state + [LabelSetFiles.manifest] {
            try FileManager.default.copyItem(
                at: dir.appendingPathComponent(name), to: published.appendingPathComponent(name))
        }
        let copy = try LabelSetStore.open(published)
        #expect(copy.revision == 2)
        #expect(throws: LabelSetError.self) {
            try LabelSetStore.loadRevision(published, revision: 1)
        }

        addMember(copy, 41)
        try copy.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        #expect(try LabelSetStore.verify(published) == 2)
        #expect(
            try LabelSetStore.loadRevision(published, revision: 2).digest() == store.stateDigest)
        #expect(throws: LabelSetError.self) {
            try LabelSetStore.loadRevision(published, revision: 1)
        }
    }

    @Test func creatingIntoAFolderWithFilesIsRefused() throws {
        let dir = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        try Data("x".utf8).write(to: dir.appendingPathComponent("x"))
        #expect(throws: LabelSetError.self) {
            _ = try LabelSetStore.create(
                at: dir, manifest: LabelSetManifest(layout: .pandar40P), initial: goTestState(),
                change: LabelSetChange(author: "dd", operation: "create"))
        }
    }

    @Test func labelsJSONIsReadStrictly() throws {
        let (dir, _) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        let manifest = try LabelSetManifest.read(
            Data(contentsOf: dir.appendingPathComponent(LabelSetFiles.manifest)))
        #expect(manifest.name == "kirk0-reference")
        #expect(manifest.layout == .pandar40P)
        #expect(manifest.head.revision == 1)
        var bumped = manifest
        bumped.schemaVersion = 3
        #expect(throws: LabelSetError.self) { try LabelSetManifest.read(bumped.json.indentedData) }
    }
}
