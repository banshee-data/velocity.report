//
//  PointSelectionTests.swift
//  VelocityVisualiserTests
//
//  Tests for canonical-index point selection: the orthographic projection,
//  lasso containment, the depth slab, membership modes, index validation and
//  the undo stack.
//
//  These are the rules a saved mask depends on, so they are tested against
//  explicit geometry rather than against whatever the renderer happens to do.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

// MARK: - View basis

struct OrthoViewBasisTests {
    @Test func topViewProjectsGroundPlaneAndMeasuresHeightAsDepth() {
        let basis = OrthoViewBasis(.top)
        let p = simd_float3(3, 4, 5)

        // Looking down: the view plane is X-Y.
        #expect(basis.project(p) == simd_float2(3, 4))
        // Depth increases downward, so a higher point is nearer the viewer.
        #expect(basis.depth(p) == -5)
        #expect(basis.depth(simd_float3(0, 0, -2)) == 2)
    }

    @Test func frontViewProjectsXZAndMeasuresYAsDepth() {
        let basis = OrthoViewBasis(.front)
        let p = simd_float3(3, 4, 5)

        #expect(basis.project(p) == simd_float2(3, 5))
        #expect(basis.depth(p) == 4)
    }

    @Test func sideViewProjectsYZAndMeasuresXAsDepth() {
        let basis = OrthoViewBasis(.side)
        let p = simd_float3(3, 4, 5)

        #expect(basis.project(p) == simd_float2(4, 5))
        #expect(basis.depth(p) == -3)
    }

    @Test func originShiftsTheViewPlaneNotTheGeometry() {
        let basis = OrthoViewBasis(.top, origin: simd_float3(10, 20, 0))
        #expect(basis.project(simd_float3(13, 24, 0)) == simd_float2(3, 4))
    }

    @Test func basisVectorsAreOrthonormal() {
        for standard in OrthoViewBasis.Standard.allCases {
            let basis = OrthoViewBasis(standard)
            #expect(abs(simd_length(basis.right) - 1) < 1e-6)
            #expect(abs(simd_length(basis.up) - 1) < 1e-6)
            #expect(abs(simd_length(basis.forward) - 1) < 1e-6)
            #expect(abs(simd_dot(basis.right, basis.up)) < 1e-6)
            #expect(abs(simd_dot(basis.right, basis.forward)) < 1e-6)
            #expect(abs(simd_dot(basis.up, basis.forward)) < 1e-6)
        }
    }

    @Test func secondViewIsAlwaysADifferentAxis() {
        // The confirming view has to actually show a different angle, or the
        // second-view check verifies nothing.
        for standard in OrthoViewBasis.Standard.allCases {
            #expect(OrthoViewBasis.secondView(for: standard) != standard)
        }
    }
}

// MARK: - Polygon

struct SelectionPolygonTests {
    @Test func rectangleContainsInteriorAndExcludesOutside() {
        let rect = SelectionPolygon(rectFrom: simd_float2(0, 0), to: simd_float2(10, 5))

        #expect(rect.contains(simd_float2(5, 2.5)))
        #expect(!rect.contains(simd_float2(-1, 2.5)))
        #expect(!rect.contains(simd_float2(11, 2.5)))
        #expect(!rect.contains(simd_float2(5, 6)))
        #expect(!rect.contains(simd_float2(5, -1)))
    }

    @Test func rectangleNormalisesDraggedCorners() {
        // A drag from bottom-right to top-left is the same rectangle.
        let forward = SelectionPolygon(rectFrom: simd_float2(0, 0), to: simd_float2(10, 5))
        let backward = SelectionPolygon(rectFrom: simd_float2(10, 5), to: simd_float2(0, 0))
        #expect(forward.vertices == backward.vertices)
    }

    @Test func concaveLassoExcludesItsNotch() {
        // A C shape. A convex-hull test would wrongly include the notch, and
        // hand-drawn lassos are exactly where that bites.
        let c = SelectionPolygon(vertices: [
            simd_float2(0, 0), simd_float2(10, 0), simd_float2(10, 3),
            simd_float2(4, 3), simd_float2(4, 7), simd_float2(10, 7),
            simd_float2(10, 10), simd_float2(0, 10),
        ])

        #expect(c.contains(simd_float2(2, 5)))  // inside the spine
        #expect(c.contains(simd_float2(7, 1.5)))  // inside the lower arm
        #expect(!c.contains(simd_float2(7, 5)))  // inside the notch, so outside
    }

    @Test func degeneratePolygonSelectsNothing() {
        #expect(SelectionPolygon(vertices: []).isDegenerate)
        #expect(SelectionPolygon(vertices: [simd_float2(0, 0)]).isDegenerate)
        #expect(SelectionPolygon(vertices: [simd_float2(0, 0), simd_float2(1, 1)]).isDegenerate)
        #expect(!SelectionPolygon(vertices: [.zero, simd_float2(1, 0), simd_float2(0, 1)]).isDegenerate)
        // A two-point "lasso" encloses nothing, so it must not select the
        // whole frame by parity accident.
        #expect(!SelectionPolygon(vertices: [simd_float2(0, 0), simd_float2(1, 1)]).contains(.zero))
    }

    @Test func pointLevelWithAVertexIsNotCountedTwice() {
        // A triangle with a vertex exactly at y = 5. A naive inclusive span
        // test flips parity twice here and reports the interior as outside.
        let tri = SelectionPolygon(vertices: [
            simd_float2(0, 0), simd_float2(10, 5), simd_float2(0, 10),
        ])
        #expect(tri.contains(simd_float2(2, 5)))
    }

    @Test func boundsCoverEveryVertex() {
        let poly = SelectionPolygon(vertices: [
            simd_float2(-3, 2), simd_float2(4, -1), simd_float2(1, 8),
        ])
        let bounds = poly.bounds!
        #expect(bounds.min == simd_float2(-3, -1))
        #expect(bounds.max == simd_float2(4, 8))
        #expect(SelectionPolygon(vertices: []).bounds == nil)
    }
}

// MARK: - Slab

struct DepthSlabTests {
    @Test func containsIsInclusiveAtBothEdges() {
        let slab = DepthSlab(minDepth: -2, maxDepth: 3)
        #expect(slab.contains(-2))
        #expect(slab.contains(0))
        #expect(slab.contains(3))
        #expect(!slab.contains(-2.1))
        #expect(!slab.contains(3.1))
    }

    @Test func boundsAreNormalisedWhateverOrderTheyArrive() {
        let slab = DepthSlab(minDepth: 5, maxDepth: 1)
        #expect(slab.minDepth == 1)
        #expect(slab.maxDepth == 5)
    }
}

// MARK: - Engine

struct PointSelectionEngineTests {
    /// A ground-level cluster of three points, one point 8 m up directly above
    /// it (the lamp post case the slab exists for), and one far outlier.
    private func scene() -> PackPoints {
        PackPoints(
            x: [1.0, 1.5, 2.0, 1.5, 40.0],
            y: [1.0, 1.25, 1.5, 1.25, -30.0],
            z: [0.5, 0.75, 1.0, 8.0, 3.0],
            intensity: [10, 20, 30, 40, 50],
            classification: [1, 1, 1, 1, 2])
    }

    @Test func lassoSelectsCanonicalIndicesInsideIt() {
        let lasso = SelectionPolygon(rectFrom: simd_float2(0, 0), to: simd_float2(3, 3))
        let result = PointSelectionEngine.candidates(
            points: scene(), basis: OrthoViewBasis(.top), polygon: lasso, slab: nil)

        // Indices are canonical pack indices, ascending. The far outlier at
        // index 4 is outside the outline.
        #expect(result.indices == [0, 1, 2, 3])
        #expect(result.count == 4)
        #expect(result.excludedBySlab == 0)
    }

    @Test func slabExcludesThePointAboveTheCluster() {
        // Top-down depth is -Z, so a ground slab is a negative range.
        let lasso = SelectionPolygon(rectFrom: simd_float2(0, 0), to: simd_float2(3, 3))
        let groundSlab = DepthSlab(minDepth: -2, maxDepth: 0)
        let result = PointSelectionEngine.candidates(
            points: scene(), basis: OrthoViewBasis(.top), polygon: lasso, slab: groundSlab)

        // Index 3 sits 8 m up inside the same outline: the slab is what
        // separates the van from the lamp post above it.
        #expect(result.indices == [0, 1, 2])
        // Surfaced so an operator expecting four points can see why they got
        // three, rather than assuming the lasso missed.
        #expect(result.excludedBySlab == 1)
    }

    @Test func selectionIsThroughSlabNotHiddenSurfacePicking() {
        // Two points at the same X-Y, different heights, both inside the slab.
        // Through-slab selection takes both: the far one is not hidden by the
        // near one. This is the documented rule, so it is pinned.
        let stacked = PackPoints(
            x: [2, 2], y: [2, 2], z: [0.5, 1.5],
            intensity: [0, 0], classification: [0, 0])
        let lasso = SelectionPolygon(rectFrom: simd_float2(1, 1), to: simd_float2(3, 3))
        let result = PointSelectionEngine.candidates(
            points: stacked, basis: OrthoViewBasis(.top), polygon: lasso,
            slab: DepthSlab(minDepth: -5, maxDepth: 0))

        #expect(result.indices == [0, 1])
    }

    @Test func nonFiniteCoordinatesAreSkipped() {
        // A NaN cannot be inside anything, and letting one into a mask would
        // poison the saved membership.
        let broken = PackPoints(
            x: [1, Float.nan, 2, Float.infinity], y: [1, 1, 1, 1], z: [0, 0, 0, 0],
            intensity: [0, 0, 0, 0], classification: [0, 0, 0, 0])
        let lasso = SelectionPolygon(rectFrom: simd_float2(-10, -10), to: simd_float2(10, 10))
        let result = PointSelectionEngine.candidates(
            points: broken, basis: OrthoViewBasis(.top), polygon: lasso, slab: nil)

        #expect(result.indices == [0, 2])
    }

    @Test func degenerateGestureSelectsNothing() {
        let result = PointSelectionEngine.candidates(
            points: scene(), basis: OrthoViewBasis(.top),
            polygon: SelectionPolygon(vertices: [simd_float2(0, 0)]), slab: nil)
        #expect(result.indices.isEmpty)
    }

    @Test func differentViewSelectsDifferentPoints() {
        // The same numeric outline means a different world volume in a
        // different view, which is why the basis is part of the rule.
        let outline = SelectionPolygon(rectFrom: simd_float2(0, 0), to: simd_float2(3, 3))
        let top = PointSelectionEngine.candidates(
            points: scene(), basis: OrthoViewBasis(.top), polygon: outline, slab: nil)
        let front = PointSelectionEngine.candidates(
            points: scene(), basis: OrthoViewBasis(.front), polygon: outline, slab: nil)

        // Front view is X-Z: the 8 m point falls outside a 3 m tall outline.
        #expect(top.indices == [0, 1, 2, 3])
        #expect(front.indices == [0, 1, 2])
    }

    @Test func modeCombinesWithExistingMembership() {
        let current: Set<Int> = [1, 2, 3]

        #expect(PointSelectionEngine.apply(mode: .replace, candidates: [5, 6], to: current) == [5, 6])
        #expect(
            PointSelectionEngine.apply(mode: .add, candidates: [3, 4], to: current) == [1, 2, 3, 4])
        #expect(
            PointSelectionEngine.apply(mode: .subtract, candidates: [2, 9], to: current) == [1, 3])
    }

    @Test func modifierKeysMapToModes() {
        #expect(SelectionMode.from(shiftHeld: false, optionHeld: false) == .replace)
        #expect(SelectionMode.from(shiftHeld: true, optionHeld: false) == .add)
        #expect(SelectionMode.from(shiftHeld: false, optionHeld: true) == .subtract)
        // Subtract wins when both are held, so an accidental shift cannot
        // turn an erase into an add.
        #expect(SelectionMode.from(shiftHeld: true, optionHeld: true) == .subtract)
    }

    @Test func canonicalEncodingIsSortedAndStable() {
        // An identical membership must encode identically, so two saves of the
        // same selection produce the same bytes.
        let a: Set<Int> = [7, 1, 3]
        let b: Set<Int> = [3, 7, 1]
        #expect(PointSelectionEngine.canonicalIndices(a) == [1, 3, 7])
        #expect(PointSelectionEngine.canonicalIndices(a) == PointSelectionEngine.canonicalIndices(b))
        #expect(PointSelectionEngine.canonicalIndices([]) == [])
    }

    @Test func validationRejectsOutOfRangeAndDuplicateIndices() {
        #expect(PointSelectionEngine.validate(indices: [0, 2, 1], pointCount: 3) == .success([0, 1, 2]))

        // Clamping a bad index would produce a confidently wrong label.
        #expect(
            PointSelectionEngine.validate(indices: [0, 3], pointCount: 3)
                == .failure(.outOfRange(index: 3, pointCount: 3)))
        #expect(
            PointSelectionEngine.validate(indices: [-1], pointCount: 3)
                == .failure(.outOfRange(index: -1, pointCount: 3)))
        #expect(
            PointSelectionEngine.validate(indices: [1, 1], pointCount: 3)
                == .failure(.duplicate(index: 1)))
        #expect(PointSelectionEngine.validate(indices: [], pointCount: 0) == .success([]))
    }

    @Test func largeFrameSelectionStaysWithinTheWorkflowsResponseBudget() {
        // The acceptance scorecard asks for p95 selection response under
        // 200 ms for a 100,000-point frame. This is a coarse floor on the
        // hot path, not a benchmark: it catches an accidental O(n·edges)
        // regression that skips the bounds rejection.
        var x = [Float](repeating: 0, count: 100_000)
        var y = [Float](repeating: 0, count: 100_000)
        var z = [Float](repeating: 0, count: 100_000)
        for i in 0..<100_000 {
            x[i] = Float(i % 400) * 0.25 - 50
            y[i] = Float(i / 400) * 0.2 - 25
            z[i] = Float(i % 7) * 0.3
        }
        let points = PackPoints(
            x: x, y: y, z: z,
            intensity: [UInt8](repeating: 0, count: 100_000),
            classification: [UInt8](repeating: 0, count: 100_000))
        let lasso = SelectionPolygon(vertices: [
            simd_float2(-5, -5), simd_float2(5, -5), simd_float2(5, 5),
            simd_float2(0, 2), simd_float2(-5, 5),
        ])

        let start = Date()
        let result = PointSelectionEngine.candidates(
            points: points, basis: OrthoViewBasis(.top), polygon: lasso,
            slab: DepthSlab(minDepth: -3, maxDepth: 0))
        let elapsed = Date().timeIntervalSince(start)

        #expect(!result.indices.isEmpty)
        #expect(elapsed < 0.2)
    }
}

// MARK: - Undo

struct MembershipHistoryTests {
    @Test func undoAndRedoWalkEditsInOrder() {
        // The mutating calls are made before #expect: the macro captures its
        // argument in an immutable closure, so `#expect(history.undo())`
        // would not compile.
        var history = MembershipHistory(initial: [])
        #expect(!history.canUndo)
        #expect(!history.canRedo)

        history.commit([1, 2])
        history.commit([1, 2, 3])
        #expect(history.current == [1, 2, 3])

        var undid = history.undo()
        #expect(undid)
        #expect(history.current == [1, 2])
        undid = history.undo()
        #expect(undid)
        #expect(history.current == [])
        undid = history.undo()
        #expect(!undid)

        var redid = history.redo()
        #expect(redid)
        #expect(history.current == [1, 2])
        redid = history.redo()
        #expect(redid)
        #expect(history.current == [1, 2, 3])
        redid = history.redo()
        #expect(!redid)
    }

    @Test func noOpEditIsNotRecorded() {
        // Undo must never appear to do nothing.
        var history = MembershipHistory(initial: [1, 2])
        let recorded = history.commit([1, 2])
        #expect(!recorded)
        #expect(!history.canUndo)
    }

    @Test func newEditClearsTheRedoBranch() {
        var history = MembershipHistory(initial: [])
        history.commit([1])
        history.commit([1, 2])
        history.undo()
        #expect(history.canRedo)

        history.commit([9])
        #expect(!history.canRedo)
        #expect(history.current == [9])
    }

    @Test func historyIsBounded() {
        var history = MembershipHistory(initial: [], limit: 3)
        for i in 1...10 { history.commit(Set(1...i)) }

        var undos = 0
        while history.undo() { undos += 1 }
        #expect(undos == 3)
    }

    @Test func resetClearsHistorySoUndoCannotCrossSamples() {
        // Undo walking backwards into another frame's selection would write
        // one sample's membership into a different sample's mask.
        var history = MembershipHistory(initial: [])
        history.commit([1, 2])
        #expect(history.canUndo)

        history.reset(to: [7])
        #expect(history.current == [7])
        #expect(!history.canUndo)
        #expect(!history.canRedo)
    }
}
