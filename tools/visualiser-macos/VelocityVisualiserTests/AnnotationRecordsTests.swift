//
//  AnnotationRecordsTests.swift
//  VelocityVisualiserTests
//
//  Tests for the annotation records: the field names proposal layers keep
//  objects and masks under, and the rules the window edits its in-memory view
//  by. Saving goes to label sets (LabelSetStoreTests, LabelSetSessionTests).
//

import Foundation
import Testing

@testable import VelocityVisualiser

struct AnnotationRecordsTests {
    @Test func objectsAndMasksDecodeTheGoWritersFieldNames() throws {
        // Field for field with Go's records, as proposal layers keep them. A
        // rename on either side has to fail a test, not read as a missing mask.
        let object = """
            {"object_id": "obj_1", "class": "car", "subtype": "box_truck", "confidence": 0.9,
             "status": "reviewed",
             "provenance": {"author": "dd", "created_utc": "2026-09-15T20:00:00Z", "revision": 2}}
            """
        let mask = """
            {"object_id": "obj_1", "sample_id": 4, "point_indices": [1, 5, 9],
             "uncertain_indices": [11], "completeness": "complete",
             "visibility": "partly_occluded", "status": "reviewed",
             "provenance": {"author": "dd", "created_utc": "2026-09-15T20:00:00Z", "revision": 2}}
            """
        let decodedObject = try JSONDecoder().decode(AnnotationObject.self, from: Data(object.utf8))
        #expect(decodedObject.objectClass == "car")
        #expect(decodedObject.subtype == "box_truck")
        #expect(decodedObject.status == .reviewed)
        #expect(decodedObject.provenance.revision == 2)

        let decodedMask = try JSONDecoder().decode(FrameMask.self, from: Data(mask.utf8))
        #expect(decodedMask.sampleID == 4)
        #expect(decodedMask.pointIndices == [1, 5, 9])
        #expect(decodedMask.uncertainIndices == [11])
        #expect(decodedMask.completeness == .complete)
        #expect(decodedMask.visibility == .partlyOccluded)
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

    @Test func maskOrderIsStableSoTheSameMasksCompareEqual() throws {
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
}
