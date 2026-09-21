//
//  BrushSelectionTests.swift
//  VelocityVisualiserTests
//
//  The sphere and column brushes, and the local column grid they share.
//
//  Like the lasso, these decide which returns end up in a saved mask, so they
//  are tested against explicit geometry: a return that is near in the view and
//  far in depth, a column with its ground voxel switched off, a cell on the
//  negative side of the origin. None of this mounts a view.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

// MARK: - The grid

struct ColumnGridTests {
    private let grid = ColumnGrid(pitch: 0.5, groundZ: -2)

    @Test func aPositionFallsInTheColumnThatContainsIt() {
        #expect(grid.cell(x: 0.1, y: 0.1) == ColumnCell(i: 0, j: 0))
        #expect(grid.cell(x: 0.49, y: 0.51) == ColumnCell(i: 0, j: 1))
        #expect(grid.cell(x: 1.0, y: 2.25) == ColumnCell(i: 2, j: 4))
    }

    /// Truncating toward zero would make the columns either side of the
    /// origin both column 0: a metre-wide cell that no other cell matches.
    @Test func columnsWestAndSouthOfTheOriginAreNegativeNotASecondZero() {
        #expect(grid.cell(x: -0.1, y: -0.1) == ColumnCell(i: -1, j: -1))
        #expect(grid.cell(x: -0.5, y: 0) == ColumnCell(i: -1, j: 0))
        #expect(grid.cell(x: -0.51, y: 0) == ColumnCell(i: -2, j: 0))
    }

    @Test func aColumnsCentreIsInsideThatColumn() {
        for cell in [ColumnCell(i: 0, j: 0), ColumnCell(i: -3, j: 7), ColumnCell(i: 12, j: -1)] {
            let centre = grid.centre(of: cell)
            #expect(grid.cell(x: centre.x, y: centre.y) == cell)
        }
    }

    @Test func voxelsAreHalfMetreBandsMeasuredFromTheGround() {
        #expect(grid.voxel(z: -2.0) == 0, "on the ground")
        #expect(grid.voxel(z: -1.51) == 0)
        #expect(grid.voxel(z: -1.5) == 1, "0.5 m above the ground starts voxel 1")
        #expect(grid.voxel(z: -0.3) == 3, "1.7 m up: a head")
        #expect(grid.voxel(z: 1.99) == 7, "the top voxel ends at 4 m")
    }

    @Test func returnsAboveTheColumnBelongToNoVoxel() {
        #expect(grid.voxel(z: 2.0) == nil, "4 m above the ground is overhead structure")
        #expect(grid.voxel(z: 9) == nil)
    }

    /// A plane is an approximation. A wheel's lowest returns sit a little
    /// under it, and must not fall out of every column because of that.
    @Test func aReturnJustBelowTheGroundIsStillTheGroundVoxel() {
        #expect(grid.voxel(z: -2.2) == 0)
        #expect(grid.voxel(z: -2.25) == 0)
        #expect(grid.voxel(z: -2.3) == nil, "further down is a stray return, not a wheel")
    }

    @Test func theStackIsVoxelsOneToFour() {
        let on = (0..<ColumnGrid.voxelCount).filter { ColumnGrid.stackMask & (1 << UInt8($0)) != 0 }
        #expect(on == [1, 2, 3, 4])
        #expect(grid.heightRange(ofVoxel: 1).lowerBound == 0.5)
        #expect(grid.heightRange(ofVoxel: 4).upperBound == 2.5)
    }

    @Test func aBrushOfNoRadiusIsOneColumn() {
        #expect(grid.cells(around: simd_float2(0.2, 0.2), radius: 0) == [ColumnCell(i: 0, j: 0)])
    }

    @Test func aBrushTakesEveryColumnWhoseCentreItReaches() {
        // Centred on a column's centre, 0.5 m reaches the four edge
        // neighbours (0.5 m away) and not the diagonal ones (0.71 m away).
        let cells = grid.cells(around: simd_float2(0.25, 0.25), radius: 0.5)
        #expect(
            cells == [
                ColumnCell(i: 0, j: 0), ColumnCell(i: 1, j: 0), ColumnCell(i: -1, j: 0),
                ColumnCell(i: 0, j: 1), ColumnCell(i: 0, j: -1),
            ])
    }

    /// Whatever the radius, the column under the brush is painted: a brush
    /// that could miss the very cell it is over would read as a broken click.
    @Test func theColumnUnderTheBrushIsAlwaysIncluded() {
        let corner = simd_float2(0.01, 0.01)
        #expect(grid.cells(around: corner, radius: 0.1).contains(ColumnCell(i: 0, j: 0)))
    }

    @Test func groundIsEstimatedFromTheLowReturnsNotTheLowestOne() {
        // Forty returns from feet and wheels upward, and one stray far below,
        // as every recording has.
        var z = (0..<40).map { -2.0 + Float($0) * 0.05 }
        z.append(-11.85)
        let points = PackPoints(
            x: [Float](repeating: 0, count: z.count), y: [Float](repeating: 0, count: z.count), z: z)

        let ground = ColumnGrid.estimateGroundZ(points: points)

        #expect(ground != nil)
        #expect(abs((ground ?? 0) - (-1.95)) < 0.06, "near the feet, not at the stray return")
    }

    @Test func aSampleWithNoReturnsHasNoGroundEstimate() {
        #expect(ColumnGrid.estimateGroundZ(points: PackPoints()) == nil)
    }
}

// MARK: - Sphere

struct SphereSelectionTests {
    /// Index 0 is the centre. 1 is 0.3 m away. 2 sits under the same spot in
    /// the top view and 3 m below. 3 is 0.6 m away.
    private let points = PackPoints(
        x: [0, 0.3, 0.05, 0.6], y: [0, 0, 0, 0], z: [0, 0, -3, 0])
    private let top = OrthoViewBasis(.top)

    /// The reason the sphere exists. A lasso in the top view takes everything
    /// under its outline at every height; a sphere takes what is near.
    @Test func aSphereSelectsByTrueDistanceNotByWhatOverlapsInTheView() {
        let got = PointSelectionEngine.candidates(
            points: points, sphere: SelectionSphere(centre: .zero, radius: 0.5), basis: top,
            slab: nil)

        #expect(got.indices == [0, 1], "index 2 is 5 cm away in the view and 3 m away in space")
    }

    @Test func aLargerRadiusReachesFurther() {
        let got = PointSelectionEngine.candidates(
            points: points, sphere: SelectionSphere(centre: .zero, radius: 0.7), basis: top,
            slab: nil)
        #expect(got.indices == [0, 1, 3])
    }

    @Test func theSlabStillAppliesAndSaysWhatItExcluded() {
        // Top view: depth is -Z, so this slab keeps heights from -1 to 1.
        let slab = DepthSlab(minDepth: -1, maxDepth: 1)
        let got = PointSelectionEngine.candidates(
            points: points, sphere: SelectionSphere(centre: .zero, radius: 4), basis: top,
            slab: slab)

        #expect(got.indices == [0, 1, 3])
        #expect(got.excludedBySlab == 1)
    }

    @Test func aSphereWithNoRadiusSelectsNothing() {
        for radius in [Float(0), -1, .nan, .infinity] {
            let got = PointSelectionEngine.candidates(
                points: points, sphere: SelectionSphere(centre: .zero, radius: radius), basis: top,
                slab: nil)
            #expect(got.indices.isEmpty, "radius \(radius)")
        }
    }

    @Test func theCentreSnapsToTheNearestReturnInTheView() {
        let index = PointSelectionEngine.nearestPoint(
            points: points, basis: top, viewPoint: simd_float2(0.28, 0.02), slab: nil,
            maxViewDistance: 0.2)
        #expect(index == 1)
    }

    /// Two returns share a spot in the top view. The slab says which one the
    /// operator can mean, so the sphere is centred on it and not on the one
    /// three metres underneath.
    @Test func snappingHonoursTheSlab() {
        let upper = DepthSlab(minDepth: -1, maxDepth: 1)
        let lower = DepthSlab(minDepth: 2, maxDepth: 4)
        let click = simd_float2(0.04, 0)

        #expect(
            PointSelectionEngine.nearestPoint(
                points: points, basis: top, viewPoint: click, slab: upper, maxViewDistance: 0.2)
                == 0)
        #expect(
            PointSelectionEngine.nearestPoint(
                points: points, basis: top, viewPoint: click, slab: lower, maxViewDistance: 0.2)
                == 2)
    }

    @Test func aClickOnEmptySpaceSnapsToNothing() {
        let index = PointSelectionEngine.nearestPoint(
            points: points, basis: top, viewPoint: simd_float2(5, 5), slab: nil,
            maxViewDistance: 0.2)
        #expect(index == nil)
    }

    @Test func aDragSetsTheRadiusAndAClickRepeatsTheLastOne() {
        #expect(BrushStroke.sphereRadius(dragMetres: 0.8, remembered: 0.5) == 0.8)
        #expect(BrushStroke.sphereRadius(dragMetres: 0.0, remembered: 0.35) == 0.35)
        #expect(BrushStroke.sphereRadius(dragMetres: 0.01, remembered: 0.35) == 0.35)
    }

    @Test func aRadiusIsNeverSmallEnoughToSelectOnlyItsOwnCentre() {
        #expect(
            BrushStroke.sphereRadius(dragMetres: 0.031, remembered: 0.5)
                == SelectionSphere.minimumRadius)
        #expect(
            BrushStroke.sphereRadius(dragMetres: 0, remembered: 0) == SelectionSphere.minimumRadius)
    }
}

// MARK: - Column

struct ColumnSelectionTests {
    /// One column, (0, 0), holding a return in each of voxels 0, 1, 3 and one
    /// above the column; and a return at the same heights next door.
    private let points = PackPoints(
        x: [0.2, 0.2, 0.2, 0.2, 0.7], y: [0.2, 0.2, 0.2, 0.2, 0.2],
        z: [0.1, 0.6, 1.7, 5.0, 0.6])
    private let grid = ColumnGrid(pitch: 0.5, groundZ: 0)
    private let top = OrthoViewBasis(.top)
    private let home: Set<ColumnCell> = [ColumnCell(i: 0, j: 0)]

    @Test func aColumnSelectsItsReturnsInTheEnabledVoxelsOnly() {
        let got = PointSelectionEngine.candidates(
            points: points, cells: home, grid: grid, enabledVoxels: ColumnGrid.stackMask,
            basis: top, slab: nil)

        #expect(got.indices == [1, 2], "voxels 1 and 3; the ground voxel is off in the stack")
    }

    /// "This cell, but not the ground" has to read as that, and not as the
    /// brush having missed two returns.
    @Test func whatTheVoxelsExcludedIsCountedAndNamed() {
        let got = PointSelectionEngine.candidates(
            points: points, cells: home, grid: grid, enabledVoxels: ColumnGrid.stackMask,
            basis: top, slab: nil)

        #expect(got.excludedByVoxels == 2, "the ground return and the one above 4 m")
        #expect(got.excludedBySlab == 0)
    }

    @Test func switchingTheGroundVoxelOnTakesTheGroundReturnToo() {
        let got = PointSelectionEngine.candidates(
            points: points, cells: home, grid: grid, enabledVoxels: ColumnGrid.allVoxelsMask,
            basis: top, slab: nil)
        #expect(got.indices == [0, 1, 2], "everything but the return above the column")
    }

    @Test func aNeighbouringColumnIsNotSelected() {
        let got = PointSelectionEngine.candidates(
            points: points, cells: home, grid: grid, enabledVoxels: ColumnGrid.allVoxelsMask,
            basis: top, slab: nil)
        #expect(!got.indices.contains(4))
    }

    @Test func movingTheGroundMovesWhichVoxelAReturnIsIn() {
        // The return at 0.6 m is voxel 1 over ground at 0, and voxel 0 over
        // ground at 0.5: the same return, no longer in the stack.
        let raised = ColumnGrid(pitch: 0.5, groundZ: 0.5)
        let got = PointSelectionEngine.candidates(
            points: points, cells: home, grid: raised, enabledVoxels: ColumnGrid.stackMask,
            basis: top, slab: nil)
        #expect(got.indices == [2])
    }

    @Test func noColumnsSelectNothing() {
        let got = PointSelectionEngine.candidates(
            points: points, cells: [], grid: grid, enabledVoxels: ColumnGrid.allVoxelsMask,
            basis: top, slab: nil)
        #expect(got.indices.isEmpty)
    }

    /// A fast drag reports positions metres apart. The stroke between them
    /// must not have holes in it.
    @Test func aFastStrokePaintsEveryColumnItCrosses() {
        let cells = BrushStroke.cells(
            from: simd_float2(0.25, 0.25), to: simd_float2(3.25, 0.25), grid: grid, radius: 0)

        #expect(cells == Set((0...6).map { ColumnCell(i: Int32($0), j: 0) }))
    }

    @Test func aStrokeThatDoesNotMoveIsOneDab() {
        let here = simd_float2(0.25, 0.25)
        #expect(
            BrushStroke.cells(from: here, to: here, grid: grid, radius: 0)
                == [ColumnCell(i: 0, j: 0)])
    }
}

// MARK: - Modes

struct BrushModeTests {
    /// A dab that replaced the selection would erase the dab before it.
    @Test func aBrushAddsUnlessToldToSubtract() {
        for tool in [SelectionTool.sphere, .column] {
            #expect(SelectionMode.from(tool: tool, shiftHeld: false, optionHeld: false) == .add)
            #expect(SelectionMode.from(tool: tool, shiftHeld: true, optionHeld: false) == .add)
            #expect(SelectionMode.from(tool: tool, shiftHeld: false, optionHeld: true) == .subtract)
        }
    }

    @Test func theLassoKeepsItsReplaceByDefault() {
        #expect(SelectionMode.from(tool: .lasso, shiftHeld: false, optionHeld: false) == .replace)
        #expect(SelectionMode.from(tool: .lasso, shiftHeld: true, optionHeld: false) == .add)
        #expect(SelectionMode.from(tool: .lasso, shiftHeld: false, optionHeld: true) == .subtract)
    }
}

// MARK: - Brush size

struct BrushSizeTests {
    @Test func aBracketMovesTheSphereByOneStep() {
        #expect(abs(BrushStroke.steppedSphereRadius(0.5, steps: 1) - 0.6) < 1e-5)
        #expect(abs(BrushStroke.steppedSphereRadius(0.5, steps: -1) - 0.4) < 1e-5)
    }

    /// A drag leaves a radius that is on no rung. A bracket goes to the next
    /// rung that way, not an odd number plus a step.
    @Test func anOddRadiusGoesToTheNextRungNotAnOddRadiusPlusAStep() {
        #expect(abs(BrushStroke.steppedSphereRadius(0.37, steps: 1) - 0.4) < 1e-5)
        #expect(abs(BrushStroke.steppedSphereRadius(0.37, steps: -1) - 0.3) < 1e-5)
    }

    /// Float arithmetic leaves 0.3 as 0.30000001. That is on the rung, so one
    /// step down is 0.2 and not 0.3 again.
    @Test func aRadiusThatIsOnARungInAllButTheLastBitStepsAsIfItWereOnIt() {
        let almost = Float(0.1) + Float(0.2)
        #expect(abs(BrushStroke.steppedSphereRadius(almost, steps: -1) - 0.2) < 1e-5)
        #expect(abs(BrushStroke.steppedSphereRadius(almost, steps: 1) - 0.4) < 1e-5)
    }

    @Test func sizesStopAtTheirLimits() {
        #expect(
            BrushStroke.steppedSphereRadius(0.1, steps: -1) == SelectionSphere.minimumRadius,
            "never down to a sphere that selects only its centre")
        #expect(
            BrushStroke.steppedSphereRadius(SelectionSphere.maximumRadius, steps: 1)
                == SelectionSphere.maximumRadius)
        #expect(BrushStroke.steppedColumnRadius(0, steps: -1) == 0, "one column is the smallest brush")
        #expect(
            BrushStroke.steppedColumnRadius(BrushStroke.maximumColumnRadius, steps: 1)
                == BrushStroke.maximumColumnRadius)
    }

    @Test func theColumnBrushStepsByAQuarterMetreFromOneColumn() {
        #expect(BrushStroke.steppedColumnRadius(0, steps: 1) == 0.25)
        #expect(BrushStroke.steppedColumnRadius(0.25, steps: 1) == 0.5)
        #expect(BrushStroke.steppedColumnRadius(0.5, steps: -1) == 0.25)
    }

    @Test func pressingUpThenDownReturnsToWhereItStarted() {
        var radius: Float = 0.5
        for _ in 0..<7 { radius = BrushStroke.steppedSphereRadius(radius, steps: 1) }
        for _ in 0..<7 { radius = BrushStroke.steppedSphereRadius(radius, steps: -1) }
        #expect(abs(radius - 0.5) < 1e-4)
    }
}

// MARK: - Through the session

/// Fixture sample 0: a cluster at (1, 1, 0.5), (1.5, 1.25, 0.75), (2, 1.5, 1.0)
/// and an outlier at (40, -30, 3).
@MainActor
struct BrushSessionTests {
    private func makeSession() throws -> AnnotationSession {
        let pack = try AnnotationPack.open(directory: try PackFixture.write())
        let session = try AnnotationSession(pack: pack)
        session.operatorName = "dd"
        return session
    }

    @Test func theGroundIsEstimatedWhenAPackIsOpened() throws {
        let session = try makeSession()
        #expect(session.columnGrid.groundZ == 0.5, "the lowest of the fixture's four returns")
        #expect(session.enabledVoxels == ColumnGrid.stackMask)
    }

    @Test func aSphereStrokeIsPreviewedThenCommittedLikeALasso() throws {
        let session = try makeSession()
        session.beginStroke()
        let preview = session.previewSelection(
            sphere: SelectionSphere(centre: simd_float3(1, 1, 0.5), radius: 0.7))

        #expect(preview.indices == [0, 1])
        #expect(session.pendingSphere != nil, "both views draw the sphere from here")
        #expect(session.selectionCount == 0, "a preview changes nothing")

        session.selectionMode = .add
        #expect(session.commitSelection())
        #expect(session.canonicalSelection == [0, 1])
        #expect(session.pendingSphere == nil)
        #expect(session.pendingCandidates == nil)
    }

    @Test func aBrushSubtractsFromWhatTheLassoSelected() throws {
        let session = try makeSession()
        session.select(
            polygon: SelectionPolygon(rectFrom: simd_float2(0, 0), to: simd_float2(3, 3)))
        #expect(session.canonicalSelection == [0, 1, 2])

        session.beginStroke()
        session.previewSelection(
            sphere: SelectionSphere(centre: simd_float3(2, 1.5, 1.0), radius: 0.2))
        session.selectionMode = .subtract
        session.commitSelection()

        #expect(session.canonicalSelection == [0, 1], "deselecting is the same tool with option held")
    }

    @Test func aColumnStrokeUsesTheSessionsGridAndVoxels() throws {
        let session = try makeSession()
        session.columnGrid.groundZ = 0
        let cell = session.columnGrid.cell(x: 1, y: 1)

        session.beginStroke()
        let stack = session.previewSelection(cells: [cell])
        #expect(stack.indices == [0], "0.5 m above the ground is voxel 1, in the stack")

        session.columnGrid.groundZ = 0.5
        let raised = session.previewSelection(cells: [cell])
        #expect(raised.indices.isEmpty, "the same return is now in the ground voxel")
        #expect(raised.excludedByVoxels == 1)
        session.cancelStroke()
    }

    @Test func theBracketKeysSizeWhicheverBrushIsActive() throws {
        let session = try makeSession()

        session.tool = .sphere
        session.adjustBrushSize(steps: 1)
        #expect(abs(session.sphereRadius - 0.6) < 1e-5)
        #expect(session.columnBrushRadius == 0, "the other brush is left alone")

        session.tool = .column
        session.adjustBrushSize(steps: 2)
        #expect(session.columnBrushRadius == 0.5)
        #expect(abs(session.sphereRadius - 0.6) < 1e-5)
    }

    @Test func theLassoHasNoSizeForTheBracketsToChange() throws {
        let session = try makeSession()
        session.tool = .lasso
        session.adjustBrushSize(steps: 3)

        #expect(session.sphereRadius == SelectionSphere.defaultRadius)
        #expect(session.columnBrushRadius == 0)
    }

    @Test func cancellingAStrokeForgetsItsShapes() throws {
        let session = try makeSession()
        session.beginStroke()
        session.previewSelection(cells: [ColumnCell(i: 2, j: 2)])
        #expect(!session.pendingCells.isEmpty)

        session.cancelStroke()

        #expect(session.pendingCells.isEmpty)
        #expect(session.pendingSphere == nil)
        #expect(session.navigationGuard() == nil, "no stroke is left open to block stepping")
    }

    @Test func brushStrokesAreUndoneLikeAnyOther() throws {
        let session = try makeSession()
        session.beginStroke()
        session.previewSelection(
            sphere: SelectionSphere(centre: simd_float3(1, 1, 0.5), radius: 0.7))
        session.selectionMode = .add
        session.commitSelection()
        #expect(session.selectionCount == 2)

        session.undo()
        #expect(session.selectionCount == 0)
        session.redo()
        #expect(session.canonicalSelection == [0, 1])
    }

    /// A brush chooses points. It must not change what a saved mask is.
    @Test func aMaskMadeWithABrushIsStoredAsPointIndices() throws {
        let session = try makeSession()
        _ = session.createObject(objectClass: "car")
        session.beginStroke()
        session.previewSelection(
            sphere: SelectionSphere(centre: simd_float3(1, 1, 0.5), radius: 0.7))
        session.selectionMode = .add
        session.commitSelection()

        #expect(session.save())

        let mask = try #require(session.sidecar.masks.first)
        #expect(mask.pointIndices == [0, 1])
    }
}
