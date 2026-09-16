//
//  AnnotationSessionTests.swift
//  VelocityVisualiserTests
//
//  Tests for the operator workflow rules: navigation guards, object identity
//  across samples, the second-view review gate, operator provenance, and what
//  happens to a dirty selection when a save is refused.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

@MainActor
private func makeSession() throws -> (AnnotationSession, URL) {
    let dir = try PackFixture.write()
    let pack = try AnnotationPack.open(directory: dir)
    let session = try AnnotationSession(pack: pack)
    session.operatorName = "dd"
    return (session, dir)
}

/// The three-point cluster in fixture sample 0, excluding the far outlier.
private let clusterLasso = SelectionPolygon(
    rectFrom: simd_float2(0, 0), to: simd_float2(3, 3))

@MainActor
struct AnnotationSessionTests {
    @Test func startsOnTheFirstSampleWithItsPointsDecoded() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(session.currentSample?.sampleID == 0)
        #expect(session.currentPoints.count == 4)
        #expect(session.selectionCount == 0)
    }

    @Test func newObjectStartsProposedNotReviewed() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        let object = session.createObject(objectClass: "car")

        // Nothing this client creates is reference truth on arrival.
        #expect(object.status == .proposed)
        #expect(object.provenance.author == "dd")
        #expect(session.activeObjectID == object.objectID)
    }

    @Test func lassoSelectsThenAddAndSubtractAdjustMembership() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")

        #expect(session.select(polygon: clusterLasso, mode: .replace))
        #expect(session.canonicalSelection == [0, 1, 2])

        // Subtract the middle point.
        let middle = SelectionPolygon(
            rectFrom: simd_float2(1.4, 1.1), to: simd_float2(1.6, 1.4))
        #expect(session.select(polygon: middle, mode: .subtract))
        #expect(session.canonicalSelection == [0, 2])

        #expect(session.select(polygon: middle, mode: .add))
        #expect(session.canonicalSelection == [0, 1, 2])
    }

    @Test func previewShowsTheCandidateCountBeforeItIsApplied() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")

        session.beginStroke()
        let candidates = session.previewSelection(polygon: clusterLasso)

        // The workflow requires the count before acceptance, so preview must
        // not have changed membership yet.
        #expect(candidates.count == 3)
        #expect(session.selectionCount == 0)

        #expect(session.commitSelection())
        #expect(session.selectionCount == 3)
    }

    @Test func slabDefaultsToTheSamplesFullDepthExtent() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        // A fresh sample must not start with an invisible selection volume.
        let slab = try #require(session.slab)
        // Top view depth is -Z; fixture z values span 0.5 to 3.0.
        #expect(slab.minDepth <= -3.0)
        #expect(slab.maxDepth >= -0.5)
    }

    @Test func navigationIsBlockedWhileAStrokeIsInProgress() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")

        session.beginStroke()
        session.previewSelection(polygon: clusterLasso)

        // Navigation must not retarget an unfinished stroke.
        #expect(session.stepForward() == .strokeInProgress)
        #expect(session.currentSample?.sampleID == 0)

        session.cancelStroke()
        #expect(session.stepForward() == nil)
        #expect(session.currentSample?.sampleID == 1)
    }

    @Test func navigationIsBlockedWhileMembershipIsUnsaved() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        let object = session.createObject(objectClass: "car")

        _ = session.select(polygon: clusterLasso)
        #expect(session.stepForward() == .unsavedMembership(sampleID: 0, objectID: object.objectID))
        #expect(session.currentSample?.sampleID == 0)

        #expect(session.save())
        #expect(session.stepForward() == nil)
        #expect(session.currentSample?.sampleID == 1)
    }

    @Test func steppingKeepsObjectIdentityButNotPointIndices() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        let object = session.createObject(objectClass: "car")

        _ = session.select(polygon: clusterLasso)
        #expect(session.save())
        #expect(session.stepForward() == nil)

        // A point index means a return in one scan. Carrying it across would
        // fabricate membership in the next frame.
        #expect(session.activeObjectID == object.objectID)
        #expect(session.selectionCount == 0)
        #expect(session.currentPoints.count == 2)
    }

    @Test func returningToASampleReloadsItsSavedMask() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")

        _ = session.select(polygon: clusterLasso)
        #expect(session.save())
        #expect(session.stepForward() == nil)
        #expect(session.stepBackward() == nil)

        #expect(session.canonicalSelection == [0, 1, 2])
        // Undo must not walk back into the other sample's selection.
        #expect(!session.history.canUndo)
    }

    @Test func saveRefusedWithoutAnOperatorName() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        session.operatorName = "   "
        _ = session.createObject(objectClass: "car")
        _ = session.select(polygon: clusterLasso)

        // The store never invents a human author.
        #expect(!session.save())
        #expect(session.lastError?.contains("operator name") == true)
    }

    @Test func saveRefusedWithoutAnActiveObject() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.select(polygon: clusterLasso)

        #expect(!session.save())
        #expect(session.lastError?.contains("reference object") == true)
    }

    @Test func savedMaskIsProposedNotReviewed() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        let object = session.createObject(objectClass: "car")
        _ = session.select(polygon: clusterLasso)
        #expect(session.save())

        // Saving membership is not reviewing it.
        let mask = try #require(session.sidecar.mask(objectID: object.objectID, sampleID: 0))
        #expect(mask.status == .proposed)
        #expect(mask.pointIndices == [0, 1, 2])
        #expect(session.sidecar.reviewedMasks.isEmpty)
    }

    @Test func reviewRequiresTheSecondViewConfirmation() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        _ = session.select(polygon: clusterLasso)

        // Membership seen from one angle has not been inspected for
        // contamination, so it cannot be marked reviewed.
        #expect(!session.markObjectReviewed(secondViewConfirmed: false))
        #expect(session.activeObject?.status == .proposed)
        #expect(session.lastError?.contains("second view") == true)

        #expect(session.markObjectReviewed(secondViewConfirmed: true))
        #expect(session.activeObject?.status == .reviewed)
    }

    @Test func savePersistsCanonicalSortedIndices() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        let object = session.createObject(objectClass: "car")

        // Select out of order: third point, then the first two.
        let third = SelectionPolygon(rectFrom: simd_float2(1.9, 1.4), to: simd_float2(2.1, 1.6))
        let firstTwo = SelectionPolygon(rectFrom: simd_float2(0.9, 0.9), to: simd_float2(1.6, 1.3))
        _ = session.select(polygon: third, mode: .replace)
        _ = session.select(polygon: firstTwo, mode: .add)
        #expect(session.save())

        let mask = try #require(session.sidecar.mask(objectID: object.objectID, sampleID: 0))
        #expect(mask.pointIndices == [0, 1, 2])
    }

    @Test func conflictKeepsTheDirtySelectionForReconciliation() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        _ = session.select(polygon: clusterLasso)

        // Another writer advances the file underneath this session.
        let other = SidecarStore(packDirectory: dir)
        var theirs = try other.load()
        theirs.sidecar.objects = [AnnotationObject(objectID: "theirs", objectClass: "bus")]
        _ = try other.save(
            theirs,
            change: Provenance(author: "other", session: "s", createdUTC: "", operation: "save_mask"))

        #expect(!session.save())
        // Refusing must not discard the operator's work.
        #expect(session.canonicalSelection == [0, 1, 2])
        guard case .conflict = try #require(session.conflict) else {
            Issue.record("expected a conflict")
            return
        }
        #expect(session.lastError?.contains("reconcile") == true)
    }

    @Test func reloadIsTheExplicitWayToDropUnsavedWork() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        _ = session.select(polygon: clusterLasso)
        #expect(session.selectionCount == 3)

        session.reload()

        // The object was never saved, so reloading drops it and its selection.
        #expect(session.selectionCount == 0)
        #expect(session.conflict == nil)
    }

    @Test func undoAndRedoTrackDirtinessAgainstWhatIsSaved() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")

        _ = session.select(polygon: clusterLasso)
        #expect(session.save())
        #expect(session.navigationGuard() == nil)

        // An edit away from the saved state is dirty; undoing back to it is
        // clean again, so the guard does not nag about work that matches disk.
        let middle = SelectionPolygon(rectFrom: simd_float2(1.4, 1.1), to: simd_float2(1.6, 1.4))
        _ = session.select(polygon: middle, mode: .subtract)
        #expect(session.navigationGuard() != nil)

        session.undo()
        #expect(session.canonicalSelection == [0, 1, 2])
        #expect(session.navigationGuard() == nil)
    }

    @Test func visibilityAndCompletenessRoundTripThroughASave() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        let object = session.createObject(objectClass: "car")
        _ = session.select(polygon: clusterLasso)
        session.maskVisibility = .partlyOccluded
        session.maskCompleteness = .complete
        #expect(session.save())

        let mask = try #require(session.sidecar.mask(objectID: object.objectID, sampleID: 0))
        #expect(mask.visibility == .partlyOccluded)
        #expect(mask.completeness == .complete)

        // And they come back when the sample is revisited.
        #expect(session.stepForward() == nil)
        #expect(session.stepBackward() == nil)
        #expect(session.maskVisibility == .partlyOccluded)
        #expect(session.maskCompleteness == .complete)
    }

    @Test func emptyMaskSavesAsEmptyRatherThanBeingSkipped() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        let object = session.createObject(objectClass: "car")

        // A fully occluded object has an empty mask for an understandable
        // reason, and that differs from nobody having looked yet.
        session.maskVisibility = .fullyOccluded
        #expect(session.save())

        let mask = try #require(session.sidecar.mask(objectID: object.objectID, sampleID: 0))
        #expect(mask.pointIndices.isEmpty)
        #expect(mask.visibility == .fullyOccluded)
    }

    @Test func secondViewDefaultsToADifferentAxisThanTheEditingView() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(session.viewStandard != session.secondViewStandard)
    }
}
