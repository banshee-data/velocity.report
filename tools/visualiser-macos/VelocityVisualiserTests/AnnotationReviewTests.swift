//
//  AnnotationReviewTests.swift
//  VelocityVisualiserTests
//
//  Tests that a review reaches the masks. A mask is reference truth when the
//  mask and its object are both reviewed; this client used to review only the
//  object, so an hour of labelling produced nothing that counted.
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

private let everything = SelectionPolygon(
    rectFrom: simd_float2(-100, -100), to: simd_float2(100, 100))
private let roundTheWall = SelectionPolygon(rectFrom: simd_float2(19, 19), to: simd_float2(23, 21))

/// What Go's `Sidecar.ReviewedMasks` returns: masks reviewed on an object
/// that is reviewed.
private func referenceTruth(_ sidecar: Sidecar) -> [FrameMask] {
    let reviewedObjects = Set(sidecar.objects.filter { $0.status == .reviewed }.map(\.objectID))
    return sidecar.masks.filter { $0.status == .reviewed && reviewedObjects.contains($0.objectID) }
}

@MainActor
struct MaskReviewTests {
    @Test func reviewingAFrameMakesItsMaskCount() throws {
        let (session, dir) = try openSession([SyntheticPack.car(at: .zero)])
        defer { try? FileManager.default.removeItem(at: dir) }
        let car = session.createObject(objectClass: "car")
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.save())
        #expect(referenceTruth(session.sidecar).isEmpty)

        #expect(session.markFrameReviewed(secondViewConfirmed: true))

        #expect(referenceTruth(session.sidecar).count == 1)
        #expect(session.reviewedSampleCount(objectID: car.objectID) == 1)
        #expect(session.sidecar.mask(objectID: car.objectID, sampleID: 0)?.provenance.operation == "review_mask")

        // On disk, not only in memory: another session reads it back.
        let reopened = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        #expect(referenceTruth(reopened.sidecar).count == 1)
    }

    @Test func aReviewIsOfWhatIsSavedAndCheckedInTheSecondView() throws {
        let (session, dir) = try openSession([SyntheticPack.car(at: .zero)])
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: everything, mode: .replace))

        // Not saved yet.
        #expect(!session.markFrameReviewed(secondViewConfirmed: true))
        #expect(session.save())
        // Saved, but not checked from a second angle.
        #expect(!session.markFrameReviewed(secondViewConfirmed: false))
        // Saved, then changed on screen: the review would be of something else.
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(-1, -1), to: simd_float2(0.1, 0.1)),
                mode: .subtract))
        #expect(!session.markFrameReviewed(secondViewConfirmed: true))
        #expect(referenceTruth(session.sidecar).isEmpty)
    }

    @Test func changingAReviewedFramesPointsPutsItBackToProposed() throws {
        let (session, dir) = try openSession([SyntheticPack.car(at: .zero)])
        defer { try? FileManager.default.removeItem(at: dir) }
        let car = session.createObject(objectClass: "car")
        #expect(session.select(polygon: everything, mode: .replace))
        #expect(session.save())
        #expect(session.markFrameReviewed(secondViewConfirmed: true))

        // Saved again with the same points: the review still stands.
        #expect(session.save())
        #expect(session.sidecar.mask(objectID: car.objectID, sampleID: 0)?.status == .reviewed)

        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(-1, -1), to: simd_float2(0.1, 0.1)),
                mode: .subtract))
        #expect(session.save())
        #expect(session.sidecar.mask(objectID: car.objectID, sampleID: 0)?.status == .proposed)
        #expect(referenceTruth(session.sidecar).isEmpty)
    }

    @Test func reviewingEveryFrameTurnsWhatWasFilledInIntoAgreed() throws {
        let frame = SyntheticPack.car(at: .zero) + SyntheticPack.wall
        let (session, dir) = try openSession([frame, frame, frame])
        defer { try? FileManager.default.removeItem(at: dir) }
        let wall = session.createObject(objectClass: "building")
        #expect(session.select(polygon: roundTheWall, mode: .replace))
        #expect(session.applySelectionToAllSamples() == 3)
        #expect(session.frameProgress[1].inQuestion == 6)

        #expect(session.markAllFramesReviewed(secondViewConfirmed: false) == nil)
        #expect(session.markAllFramesReviewed(secondViewConfirmed: true) == 3)

        #expect(referenceTruth(session.sidecar).count == 3)
        #expect(session.frameProgress[1].inQuestion == 0)
        #expect(session.frameProgress[1].agreed == 6)
        #expect(session.completeness.whole.agreed == 6)
        // How the points got there is still on the record.
        let filled = try #require(session.sidecar.mask(objectID: wall.objectID, sampleID: 1))
        #expect(filled.provenance.algorithm == "footprint_carry")
        #expect(filled.provenance.operation == "review_object_masks")
        // One save for the lot, not one per frame.
        #expect(session.sidecar.revision == 2)
    }
}
