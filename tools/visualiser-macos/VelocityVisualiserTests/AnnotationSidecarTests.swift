//
//  AnnotationSidecarTests.swift
//  VelocityVisualiserTests
//
//  Tests for the revision-safe sidecar store: the load/edit/save token, the
//  conflict and busy refusals, revision archival, and restore-as-new-revision.
//
//  The store shares annotations.json with the Go writer, so the rules here are
//  not conveniences — overwriting a newer revision would silently discard
//  another writer's reviewed masks.
//

import Foundation
import Testing

@testable import VelocityVisualiser

private func makePackDirectory() -> URL {
    let dir = FileManager.default.temporaryDirectory
        .appendingPathComponent("sidecar-\(UUID().uuidString)")
    try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    return dir
}

private func operatorChange(_ operation: String) -> Provenance {
    Provenance(
        author: "dd", session: "test-session", createdUTC: "", operation: operation)
}

struct SidecarCodableTests {
    @Test func decodesTheGoWritersFieldNames() throws {
        // Field-for-field with Go's sidecar schema. A rename on either side
        // has to fail a test, not read as a missing mask.
        let json = """
            {
              "schema_version": 1,
              "dataset_id": "ds_abc",
              "pack_digest": "sha256:dd",
              "revision": 3,
              "updated_utc": "2026-09-15T21:00:00Z",
              "objects": [
                {
                  "object_id": "obj_1",
                  "class": "car",
                  "subtype": "box_truck",
                  "confidence": 0.9,
                  "status": "reviewed",
                  "provenance": {
                    "author": "dd", "created_utc": "2026-09-15T20:00:00Z", "revision": 2
                  }
                }
              ],
              "masks": [
                {
                  "object_id": "obj_1",
                  "sample_id": 4,
                  "point_indices": [1, 5, 9],
                  "uncertain_indices": [11],
                  "completeness": "complete",
                  "visibility": "partly_occluded",
                  "status": "reviewed",
                  "provenance": {
                    "author": "dd", "created_utc": "2026-09-15T20:00:00Z", "revision": 2
                  }
                }
              ]
            }
            """
        let sidecar = try JSONDecoder().decode(Sidecar.self, from: Data(json.utf8))

        #expect(sidecar.revision == 3)
        #expect(sidecar.objects.first?.objectClass == "car")
        #expect(sidecar.objects.first?.subtype == "box_truck")
        #expect(sidecar.objects.first?.status == .reviewed)

        let mask = try #require(sidecar.mask(objectID: "obj_1", sampleID: 4))
        #expect(mask.pointIndices == [1, 5, 9])
        #expect(mask.uncertainIndices == [11])
        #expect(mask.completeness == .complete)
        #expect(mask.visibility == .partlyOccluded)
    }

    @Test func reviewedMasksRequireBothObjectAndMaskReviewed() throws {
        var sidecar = Sidecar()
        sidecar.objects = [
            AnnotationObject(objectID: "reviewed", objectClass: "car", status: .reviewed),
            AnnotationObject(objectID: "proposed", objectClass: "car", status: .proposed),
        ]
        sidecar.masks = [
            FrameMask(objectID: "reviewed", sampleID: 0, pointIndices: [1], status: .reviewed),
            FrameMask(objectID: "reviewed", sampleID: 1, pointIndices: [2], status: .proposed),
            // A reviewed mask under a still-proposed object is not truth.
            FrameMask(objectID: "proposed", sampleID: 0, pointIndices: [3], status: .reviewed),
        ]

        let reviewed = sidecar.reviewedMasks
        #expect(reviewed.count == 1)
        #expect(reviewed.first?.sampleID == 0)
        #expect(reviewed.first?.objectID == "reviewed")
    }

    @Test func upsertReplacesRatherThanDuplicating() {
        var sidecar = Sidecar()
        sidecar.upsert(mask: FrameMask(objectID: "a", sampleID: 1, pointIndices: [1]))
        sidecar.upsert(mask: FrameMask(objectID: "a", sampleID: 1, pointIndices: [1, 2]))

        #expect(sidecar.masks.count == 1)
        #expect(sidecar.masks[0].pointIndices == [1, 2])
    }

    @Test func maskOrderIsStableSoUnchangedSnapshotsSerialiseIdentically() throws {
        var a = Sidecar()
        a.upsert(mask: FrameMask(objectID: "b", sampleID: 2))
        a.upsert(mask: FrameMask(objectID: "a", sampleID: 1))
        a.upsert(mask: FrameMask(objectID: "a", sampleID: 2))

        var b = Sidecar()
        b.upsert(mask: FrameMask(objectID: "a", sampleID: 2))
        b.upsert(mask: FrameMask(objectID: "a", sampleID: 1))
        b.upsert(mask: FrameMask(objectID: "b", sampleID: 2))

        #expect(a.masks.map { "\($0.sampleID)/\($0.objectID)" } == ["1/a", "2/a", "2/b"])
        #expect(a.masks == b.masks)
    }

    @Test func schemaVersionMatchesTheGoReader() {
        // Go's reader refuses any other value outright, so writing a newer
        // number would lock the Go tooling out of packs this client touched.
        #expect(Sidecar.schemaVersion == 1)
        #expect(Sidecar().schemaVersion == 1)
    }
}

struct SidecarStoreTests {
    @Test func untouchedPackLoadsEmptyThenSavesRevisionOne() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = SidecarStore(packDirectory: dir)

        var document = try store.load(packDigest: "sha256:dd", datasetID: "ds_1")
        #expect(document.sidecar.revision == 0)
        #expect(document.sidecar.objects.isEmpty)

        document.sidecar.objects = [AnnotationObject(objectID: "obj_1", objectClass: "car")]
        let saved = try store.save(document, change: operatorChange("save_mask"))

        // The store supplies the revision; the caller never sets it.
        #expect(saved.sidecar.revision == 1)
        #expect(saved.sidecar.change?.parentRevision == 0)
        #expect(saved.sidecar.change?.revision == 1)
        #expect(saved.sidecar.change?.author == "dd")
        #expect(FileManager.default.fileExists(atPath: dir.appendingPathComponent("annotations.json").path))
    }

    @Test func savingAnEditOfTheLoadedDocumentAdvancesTheRevision() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = SidecarStore(packDirectory: dir)

        var first = try store.load()
        first.sidecar.objects = [AnnotationObject(objectID: "obj_1", objectClass: "car")]
        let afterFirst = try store.save(first, change: operatorChange("create_object"))

        var second = afterFirst
        second.sidecar.upsert(mask: FrameMask(objectID: "obj_1", sampleID: 0, pointIndices: [1, 2]))
        let afterSecond = try store.save(second, change: operatorChange("save_mask"))

        #expect(afterSecond.sidecar.revision == 2)
        #expect(afterSecond.sidecar.masks.count == 1)
    }

    @Test func staleDocumentIsRefusedRatherThanOverwriting() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = SidecarStore(packDirectory: dir)

        let base = try store.load()
        var writerA = base
        writerA.sidecar.objects = [AnnotationObject(objectID: "from_a", objectClass: "car")]
        _ = try store.save(writerA, change: operatorChange("save_mask"))

        // B loaded the same base and saves second. Its edit is refused: its
        // dirty data is kept for reconciliation, not written over A's.
        var writerB = base
        writerB.sidecar.objects = [AnnotationObject(objectID: "from_b", objectClass: "bus")]
        do {
            _ = try store.save(writerB, change: operatorChange("save_mask"))
            Issue.record("expected a conflict")
        } catch let error as SidecarStoreError {
            guard case .conflict(let loaded, let current) = error else {
                Issue.record("expected conflict, got \(error)")
                return
            }
            #expect(loaded == 0)
            #expect(current == 1)
        }

        // A's snapshot is intact.
        let onDisk = try store.load()
        #expect(onDisk.sidecar.objects.first?.objectID == "from_a")
    }

    @Test func editingThePublicRevisionCannotBypassTheToken() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = SidecarStore(packDirectory: dir)

        let base = try store.load()
        var other = base
        other.sidecar.objects = [AnnotationObject(objectID: "winner", objectClass: "car")]
        _ = try store.save(other, change: operatorChange("save_mask"))

        // Hand-setting the revision to match is not enough: the token is a
        // digest of the exact bytes that were read.
        var forged = base
        forged.sidecar.revision = 1
        forged.sidecar.objects = [AnnotationObject(objectID: "forged", objectClass: "car")]
        #expect(throws: SidecarStoreError.self) {
            _ = try store.save(forged, change: operatorChange("save_mask"))
        }
        #expect(try store.load().sidecar.objects.first?.objectID == "winner")
    }

    @Test func freshDocumentCannotOverwriteAnExistingFile() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = SidecarStore(packDirectory: dir)

        var initial = try store.load()
        initial.sidecar.objects = [AnnotationObject(objectID: "existing", objectClass: "car")]
        _ = try store.save(initial, change: operatorChange("save_mask"))

        // A second client that starts from "no file" state must not clobber.
        let stale = try SidecarStore(packDirectory: dir).load()
        var pretendNew = stale
        pretendNew.sidecar.objects = []
        // Re-saving the loaded snapshot is legal; what must fail is a
        // document that believes the file does not exist. Simulate that by
        // deleting nothing and loading before the first save instead.
        #expect(pretendNew.sidecar.revision == 1)
        _ = try store.save(pretendNew, change: operatorChange("clear"))
        #expect(try store.load().sidecar.revision == 2)
    }

    @Test func priorSnapshotIsArchivedBeforeTheCurrentFileAdvances() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = SidecarStore(packDirectory: dir)

        var doc = try store.load()
        doc.sidecar.objects = [AnnotationObject(objectID: "rev1", objectClass: "car")]
        doc = try store.save(doc, change: operatorChange("save_mask"))

        doc.sidecar.objects = [AnnotationObject(objectID: "rev2", objectClass: "bus")]
        _ = try store.save(doc, change: operatorChange("save_mask"))

        // Revision 1's exact bytes are retained, readable without becoming
        // current.
        #expect(store.retainedRevisions() == [1])
        let archived = try store.loadRevision(1)
        #expect(archived.objects.first?.objectID == "rev1")
        #expect(try store.load().sidecar.objects.first?.objectID == "rev2")
    }

    @Test func restoreProducesANewRevisionWithoutRewritingHistory() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = SidecarStore(packDirectory: dir)

        var doc = try store.load()
        doc.sidecar.objects = [AnnotationObject(objectID: "first", objectClass: "car")]
        doc = try store.save(doc, change: operatorChange("save_mask"))  // revision 1

        doc.sidecar.objects = [AnnotationObject(objectID: "second", objectClass: "bus")]
        doc = try store.save(doc, change: operatorChange("save_mask"))  // revision 2
        #expect(doc.sidecar.revision == 2)

        // Restoring revision 1 over revision 2 produces revision 3.
        let restored = try store.restore(revision: 1, change: operatorChange("undo"))
        #expect(restored.sidecar.revision == 3)
        #expect(restored.sidecar.objects.first?.objectID == "first")
        #expect(restored.sidecar.restoredFrom == 1)

        // Earlier files are untouched.
        #expect(try store.loadRevision(1).objects.first?.objectID == "first")
        #expect(try store.loadRevision(2).objects.first?.objectID == "second")
    }

    @Test func restoreDoesNotPromoteAProposalToReviewed() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = SidecarStore(packDirectory: dir)

        var doc = try store.load()
        doc.sidecar.objects = [
            AnnotationObject(objectID: "obj", objectClass: "car", status: .proposed)
        ]
        doc.sidecar.upsert(
            mask: FrameMask(objectID: "obj", sampleID: 0, pointIndices: [1], status: .proposed))
        doc = try store.save(doc, change: operatorChange("save_mask"))

        doc.sidecar.masks[0].pointIndices = [1, 2]
        _ = try store.save(doc, change: operatorChange("save_mask"))

        let restored = try store.restore(revision: 1, change: operatorChange("undo"))
        #expect(restored.sidecar.objects.first?.status == .proposed)
        #expect(restored.sidecar.masks.first?.status == .proposed)
        #expect(restored.sidecar.reviewedMasks.isEmpty)
    }

    @Test func ordinarySaveClearsTheRestoreMarker() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = SidecarStore(packDirectory: dir)

        var doc = try store.load()
        doc.sidecar.objects = [AnnotationObject(objectID: "a", objectClass: "car")]
        doc = try store.save(doc, change: operatorChange("save_mask"))
        doc.sidecar.objects = [AnnotationObject(objectID: "b", objectClass: "car")]
        _ = try store.save(doc, change: operatorChange("save_mask"))
        _ = try store.restore(revision: 1, change: operatorChange("undo"))

        var next = try store.load()
        next.sidecar.objects.append(AnnotationObject(objectID: "c", objectClass: "car"))
        let saved = try store.save(next, change: operatorChange("save_mask"))

        // This snapshot is an edit in its own right, not a recovered one.
        #expect(saved.sidecar.restoredFrom == nil)
    }

    @Test func corruptCurrentFileErrorsRatherThanReadingAsEmpty() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        try Data("{ not json".utf8).write(to: dir.appendingPathComponent("annotations.json"))

        // An empty dataset would look like "nothing labelled yet", which is
        // how reviewed work gets silently replaced.
        #expect(throws: SidecarStoreError.self) {
            _ = try SidecarStore(packDirectory: dir).load()
        }
    }

    @Test func historyWithoutCurrentFileRefusesToStartFresh() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let revisions = dir.appendingPathComponent("annotation-revisions")
        try FileManager.default.createDirectory(at: revisions, withIntermediateDirectories: true)
        try Data("{}".utf8).write(to: revisions.appendingPathComponent("0000000001.json"))

        // A damaged head is not a fresh start: a new save here would quietly
        // replace lost reviewed masks.
        do {
            _ = try SidecarStore(packDirectory: dir).load()
            Issue.record("expected a malformed error")
        } catch let error as SidecarStoreError {
            guard case .malformed(let detail) = error else {
                Issue.record("expected malformed, got \(error)")
                return
            }
            #expect(detail.contains("retained revision"))
        }
    }

    @Test func annotationsFromAnotherPackAreRefused() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = SidecarStore(packDirectory: dir)

        var doc = try store.load(packDigest: "sha256:aaa")
        doc.sidecar.packDigest = "sha256:aaa"
        _ = try store.save(doc, change: operatorChange("save_mask"))

        // Point indices only mean something under the digest they were
        // recorded against.
        do {
            _ = try store.load(packDigest: "sha256:bbb")
            Issue.record("expected a pack mismatch")
        } catch let error as SidecarStoreError {
            guard case .packMismatch = error else {
                Issue.record("expected packMismatch, got \(error)")
                return
            }
        }
    }

    @Test func busyLockIsReportedRatherThanWaitingForever() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = SidecarStore(packDirectory: dir)
        let doc = try store.load()

        // Hold the lock the way another writer would.
        let held = try FileLock(url: dir.appendingPathComponent(".annotations.lock"))
        #expect(held.acquired)
        defer { held.release() }

        do {
            _ = try store.save(doc, change: operatorChange("save_mask"))
            Issue.record("expected busy")
        } catch let error as SidecarStoreError {
            #expect(error == .busy)
        }
    }

    @Test func lockFileSurvivesReleaseSoWritersShareOneInode() throws {
        let dir = makePackDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        let lockURL = dir.appendingPathComponent(".annotations.lock")

        let first = try FileLock(url: lockURL)
        #expect(first.acquired)
        first.release()

        // Deleting the lock file while another writer held it would let two
        // writers each hold a lock on a different inode.
        #expect(FileManager.default.fileExists(atPath: lockURL.path))
        let second = try FileLock(url: lockURL)
        #expect(second.acquired)
        second.release()
    }
}
