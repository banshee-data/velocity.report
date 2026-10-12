//
//  AnnotationSidecarTests.swift
//  VelocityVisualiserTests
//
//  Tests for the version 1 sidecar records: the field names Go writes, and
//  the rules the window still edits by. The window no longer writes
//  annotations.json: a version 1 pack's sidecar is read for display only
//  (LegacyPackTests), and saving goes to label sets (LabelSetStoreTests,
//  LabelSetSessionTests).
//

import Foundation
import Testing

@testable import VelocityVisualiser

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
