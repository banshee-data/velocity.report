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
        let saved = try headFiles(dir)
        let parent = store.stateDigest
        addMember(store, 40)
        try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        let want = store.stateDigest
        // The old head back, the new journal line and the commit's marker
        // kept: a crash after the append and before labels.json.
        try putHead(dir, saved)
        try LabelSetJSON.object(["parent_state": .string(parent), "revision": .int(2)])
            .canonicalLine.write(to: dir.appendingPathComponent(LabelSetFiles.pending))

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
        try putHead(published, try headFiles(dir))
        let copy = try LabelSetStore.open(published)
        #expect(copy.revision == 2)
        #expect(throws: LabelSetError.self) {
            try LabelSetStore.loadRevision(published, revision: 1)
        }

        // A one-mask edit of the copy is what someone with the dataset would
        // commit: one masks.ndjson line and the head's three labels.json
        // lines; every other head file keeps its bytes.
        let before = try headFiles(published)
        addMember(copy, 41)
        try copy.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        let after = try headFiles(published)
        #expect(Set(after.keys) == Set(before.keys))
        for (name, was) in before {
            let now = after[name] ?? Data()
            let changed = zip(
                String(decoding: was, as: UTF8.self).split(
                    separator: "\n", omittingEmptySubsequences: false),
                String(decoding: now, as: UTF8.self).split(
                    separator: "\n", omittingEmptySubsequences: false)
            ).filter { $0 != $1 }.count
            switch name {
            case LabelSetFiles.manifest:
                #expect(changed == 3, "labels.json changed \(changed) lines")
            case "masks/000000-000099.ndjson":
                #expect(changed == 1, "\(name) changed \(changed) lines")
            default: #expect(now == was, "\(name) changed")
            }
        }
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

    /// A set mid-merge, as Go's merge driver leaves labels.json, is refused
    /// with what to run.
    @Test func aSetMidMergeIsRefused() throws {
        let (dir, _) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        let url = dir.appendingPathComponent(LabelSetFiles.manifest)
        var text = String(decoding: try Data(contentsOf: url), as: UTF8.self)
        text = text.replacingOccurrences(
            of: "{\n", with: "{\n  \"merge\": {\"base\": {\"revision\": 1, \"state\": \"s\", \"updated_utc\": \"\"}},\n",
            options: [], range: text.range(of: "{\n"))
        try Data(text.utf8).write(to: url)
        do {
            _ = try LabelSetManifest.read(Data(contentsOf: url))
            Issue.record("a set mid-merge was read")
        } catch { #expect("\(error)".contains("finish-merge")) }
    }

    /// A set that needs a newer reader is refused; one at this build's level
    /// opens, and a save keeps its min_reader, as Go's reader tests state.
    @Test func minReaderAboveThisBuildIsRefusedAndKeptOtherwise() throws {
        let (dir, _) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        let url = dir.appendingPathComponent(LabelSetFiles.manifest)
        let written = try Data(contentsOf: url)
        #expect(!String(decoding: written, as: UTF8.self).contains("min_reader"))
        var manifest = try LabelSetManifest.read(written)

        manifest.minReader = ReaderLevel.current + 1
        #expect(throws: LabelSetError.self) { try LabelSetManifest.read(manifest.json.indentedData) }

        manifest.minReader = ReaderLevel.current
        try manifest.json.indentedData.write(to: url)
        let reopened = try LabelSetStore.open(dir)
        addMember(reopened, 40)
        try reopened.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        let saved = String(decoding: try Data(contentsOf: url), as: UTF8.self)
        #expect(saved.contains("\"min_reader\": \(ReaderLevel.current)"))
    }
}

/// Which folder the app needs to write a pack's labels: the pack's capture
/// folder whenever the label set is inside it, which covers a pack in packs/
/// and a view in its set's views/, and the linked set's folder otherwise.
struct LabelSetWriteFolderTests {
    @Test func theFolderToGrantHoldsTheLabelSet() {
        let capture = URL(fileURLWithPath: "/data/annotation-packs/kirk0")
        #expect(
            LabelSetMembership.writeFolder(
                captureDirectory: capture,
                labelSet: capture.appendingPathComponent("kirk0-reference")) == capture)
        #expect(
            LabelSetMembership.writeFolder(
                captureDirectory: capture,
                labelSet: URL(fileURLWithPath: "/elsewhere/sets/kirk0-reference")
            ).path == "/elsewhere/sets")
    }
}

/// The layout's rules, as Go's layout tests state them.
struct AnnotationTreeTests {
    @Test func captureNamesFollowGo() {
        #expect(AnnotationTree.captureName(of: "kirk0.pcapng") == "kirk0")
        #expect(
            AnnotationTree.captureName(
                of: "/Volumes/lidar/pcaps/20260902T1320_80858-0f4_columbus-broadway.pcapng")
                == "20260902T1320_80858-0f4_columbus-broadway")
        #expect(AnnotationTree.captureName(of: "capture.pcap.gz") == "capture")
        #expect(AnnotationTree.captureName(of: "two words.pcap") == "two-words")
        #expect(AnnotationTree.captureName(of: nil) == "capture")
        #expect(AnnotationTree.captureName(of: "splits.pcapng") == "capture")
        #expect(AnnotationTree.captureName(of: "C:\\caps\\x.pcapng") == "x")
    }

    @Test func aPacksLabelsGoInItsCaptureFolder() {
        func capture(_ path: String) -> String {
            AnnotationTree.captureDirectory(
                of: URL(fileURLWithPath: path), pcapBasename: "kirk0.pcapng"
            ).path
        }
        #expect(capture("/r/kirk0/packs/run-a") == "/r/kirk0")
        #expect(capture("/r/clip-j-1/pack") == "/r/kirk0")
        #expect(capture("/r/kirk0/kirk0-reference/views/v1") == "/r/kirk0")
        #expect(capture("/elsewhere/run-a") == "/elsewhere/kirk0")
    }

    @Test func linksAreRelative() {
        let root = URL(fileURLWithPath: "/r/kirk0")
        #expect(
            AnnotationTree.relativePath(
                from: root.appendingPathComponent("packs/run-a"),
                to: root.appendingPathComponent("kirk0-reference")) == "../../kirk0-reference")
        #expect(
            AnnotationTree.relativePath(
                from: root.appendingPathComponent("kirk0-reference/views/v1"),
                to: root.appendingPathComponent("kirk0-reference")) == AnnotationTree.viewLinkPath)
    }
}

/// The storage layout's rules, as Go's storage tests state them.
struct LabelSetStorageTests {
    /// A journal ahead of the head with no commit marker is another head's,
    /// what a Git working tree holds after editing on one branch and checking
    /// out another: opening refuses it and rewrites nothing.
    @Test func aForeignJournalIsNeverApplied() throws {
        let (dir, store) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        let saved = try headFiles(dir)
        addMember(store, 40)
        try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        try putHead(dir, saved)
        #expect(throws: LabelSetError.self) { try LabelSetStore.open(dir) }
        #expect(try headFiles(dir) == saved)
    }

    /// A save rewrites only the chunk its edit changed, and removes a chunk
    /// whose masks are all gone.
    @Test func aSaveRewritesOnlyTheChangedChunk() throws {
        let dir = labelSetTemporaryDirectory().appendingPathComponent("kirk0-reference")
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        var manifest = LabelSetManifest(name: "kirk0-reference", layout: .pandar40P)
        manifest.storage = LabelSetStorage(windowChunk: 1)
        let store = try LabelSetStore.create(
            at: dir, manifest: manifest, initial: goTestState(),
            change: LabelSetChange(author: "dd", operation: "create"))
        let untouched = dir.appendingPathComponent("masks/000001-000001.ndjson")
        let before =
            try FileManager.default.attributesOfItem(atPath: untouched.path)[.modificationDate]
            as? Date
        Thread.sleep(forTimeInterval: 0.02)
        addMember(store, 40)
        try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        let after =
            try FileManager.default.attributesOfItem(atPath: untouched.path)[.modificationDate]
            as? Date
        #expect(before == after)
        store.state.masks.removeAll { $0.windowID == 1 }
        try store.save(change: LabelSetChange(author: "dd", operation: "save_mask"))
        #expect(!FileManager.default.fileExists(atPath: untouched.path))
        #expect(try LabelSetStore.verify(dir) == 3)
    }

    /// The old single file beside a chunked head, and a reformatted file,
    /// are refused; Finder's .DS_Store in a chunk directory is not.
    @Test func theHeadIsHeldToItsLayout() throws {
        let (dir, _) = try newLabelSet()
        defer { try? FileManager.default.removeItem(at: dir.deletingLastPathComponent()) }
        try Data("finder".utf8).write(to: dir.appendingPathComponent("masks/.DS_Store"))
        _ = try LabelSetStore.open(dir)
        let windows = dir.appendingPathComponent(LabelSetFiles.windows)
        let laid = try Data(contentsOf: windows)
        try LabelSetJSON.parse(laid).canonicalLine.write(to: windows)
        #expect(throws: LabelSetError.self) { try LabelSetStore.open(dir) }
        try laid.write(to: windows)
        try Data().write(to: dir.appendingPathComponent(LabelSetFiles.masks))
        #expect(throws: LabelSetError.self) { try LabelSetStore.open(dir) }
    }
}
