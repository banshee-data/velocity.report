//
//  LatticeTests.swift
//  VelocityVisualiserTests
//
//  The lattice exists so that four places round the same way. These pin the
//  rounding, not the arithmetic: a cell boundary and the origin are where four
//  private copies would have drifted apart.
//

import Testing
import simd

@testable import VelocityVisualiser

struct LatticeTests {

    /// Negative coordinates are the case a truncating conversion gets wrong:
    /// it would fold -0.2 and +0.2 into the same cell and give the origin a
    /// cell twice the width of every other.
    @Test func theCellWestOfTheOriginIsMinusOneAndNotASecondCopyOfZero() {
        let lattice = Lattice(pitch: 0.5)

        #expect(lattice.cell(x: 0.2, y: 0.2) == SIMD2<Int32>(0, 0))
        #expect(lattice.cell(x: -0.2, y: -0.2) == SIMD2<Int32>(-1, -1))
        #expect(lattice.cell(x: -0.6, y: -0.6) == SIMD2<Int32>(-2, -2))
    }

    /// A position exactly on a boundary belongs to the cell above it, so that
    /// two adjacent cells never both claim it.
    @Test func aPositionOnABoundaryBelongsToTheCellAboveIt() {
        let lattice = Lattice(pitch: 0.5)

        #expect(lattice.cell(x: 0.5, y: 1.0) == SIMD2<Int32>(1, 2))
        #expect(lattice.cell(x: 0.499_9, y: 0.999_9) == SIMD2<Int32>(0, 1))
        #expect(lattice.cell(x: -0.5, y: -1.0) == SIMD2<Int32>(-1, -2))
    }

    @Test func aVoxelIsTheCellWithHeightAgreeingOnAllThreeAxes() {
        let lattice = Lattice(pitch: 0.25)
        let p = simd_float3(1.1, -0.3, 0.75)

        let voxel = lattice.voxel(p)
        #expect(voxel == SIMD3<Int32>(4, -2, 3))
        // The plan projection of a voxel is the cell of the same position.
        #expect(SIMD2(voxel.x, voxel.y) == lattice.cell(x: p.x, y: p.y))
    }

    /// A centre has to land back in its own cell, or a brush would paint the
    /// column next door.
    @Test func aCellsCentreFallsBackInsideThatCell() {
        for pitch in [Float(0.25), 0.5, 2] {
            let lattice = Lattice(pitch: pitch)
            for cell in [SIMD2<Int32>(0, 0), SIMD2(3, -7), SIMD2(-1, -1), SIMD2(12, 5)] {
                let centre = lattice.centre(ofCell: cell)
                #expect(
                    lattice.cell(x: centre.x, y: centre.y) == cell,
                    "pitch \(pitch), cell \(cell) centred at \(centre)")
            }
        }
    }

    /// The three pitches in use divide into each other, so a fine cell lies
    /// wholly inside one coarse cell rather than straddling two. Checked
    /// through the centres, because integer division truncates towards zero
    /// and would claim otherwise west or south of the origin.
    @Test func theLatticesInUseNestRatherThanStraddle() {
        let fine = Lattice(pitch: SelectionFootprint.pitch)
        let coarse = Lattice(pitch: ObjectProposer.pitch)
        let plan = Lattice(pitch: ObjectProposer.binSize)

        for p in [simd_float2(3.3, -4.6), simd_float2(-0.1, 0.1), simd_float2(-7.75, -0.25)] {
            let fineCentre = fine.centre(ofCell: fine.cell(x: p.x, y: p.y))
            #expect(
                coarse.cell(x: fineCentre.x, y: fineCentre.y) == coarse.cell(x: p.x, y: p.y),
                "fine cell straddles a coarse boundary at \(p)")

            let coarseCentre = coarse.centre(ofCell: coarse.cell(x: p.x, y: p.y))
            #expect(
                plan.cell(x: coarseCentre.x, y: coarseCentre.y) == plan.cell(x: p.x, y: p.y),
                "coarse cell straddles a plan bin at \(p)")
        }
    }

    // MARK: - Turned lattices

    /// A turned lattice is still a lattice: a cell's centre must land back in
    /// that cell, or a brush would paint the column next door.
    @Test func aTurnedCellsCentreStillFallsInsideThatCell() {
        for azimuth in [Float(0), 3, 17, 45, 90, 183, 359] {
            let lattice = Lattice(pitch: 0.5, azimuthDeg: azimuth)
            for cell in [SIMD2<Int32>(0, 0), SIMD2(3, -7), SIMD2(-1, -1), SIMD2(12, 5)] {
                let centre = lattice.centre(ofCell: cell)
                #expect(
                    lattice.cell(x: centre.x, y: centre.y) == cell,
                    "azimuth \(azimuth), cell \(cell)")
            }
        }
    }

    /// Turning the lattice must not change how big a cell is, only which way
    /// it faces: two adjacent centres stay one pitch apart.
    @Test func turningTheLatticeDoesNotChangeTheCellSize() {
        let lattice = Lattice(pitch: 0.5, azimuthDeg: 37)
        let a = lattice.centre(ofCell: SIMD2(4, 4))
        let alongX = lattice.centre(ofCell: SIMD2(5, 4))
        let alongY = lattice.centre(ofCell: SIMD2(4, 5))
        #expect(abs(simd_distance(a, alongX) - 0.5) < 1e-5)
        #expect(abs(simd_distance(a, alongY) - 0.5) < 1e-5)
    }

    /// A quarter turn maps the lattice onto itself, which is the one rotation
    /// whose answer can be written down by hand.
    @Test func aQuarterTurnTakesTheEastCellToTheNorthOne() {
        let turned = Lattice(pitch: 1, azimuthDeg: 90)
        // A point one metre along +X is, in a lattice turned a quarter turn,
        // one cell along the lattice's -Y.
        #expect(turned.cell(x: 1.5, y: 0.5) == SIMD2<Int32>(0, -2))
        // And the untouched lattice puts it where it plainly is.
        #expect(Lattice(pitch: 1).cell(x: 1.5, y: 0.5) == SIMD2<Int32>(1, 0))
    }

    @Test func anUnturnedLatticeIsExactlyWhatItWasBefore() {
        let plain = Lattice(pitch: 0.5)
        let zero = Lattice(pitch: 0.5, azimuthDeg: 0)
        for p in [simd_float3(3.3, -4.6, 1.2), simd_float3(-0.1, 0.1, 0), .zero] {
            #expect(plain.voxel(p) == zero.voxel(p))
        }
        #expect(plain == zero)
    }

    /// The column grid passes its azimuth through, so the brush paints the
    /// turned lattice rather than a second unturned one.
    @Test func theColumnGridPaintsTheTurnedLattice() {
        let grid = ColumnGrid(pitch: 0.5, groundZ: 0, azimuthDeg: 30)
        let lattice = Lattice(pitch: 0.5, azimuthDeg: 30)
        for (x, y) in [(0.2, 0.2), (-3.4, 7.1), (12.0, -5.0)] {
            let cell = grid.cell(x: Float(x), y: Float(y))
            #expect(SIMD2(cell.i, cell.j) == lattice.cell(x: Float(x), y: Float(y)))
        }
        // And a turned grid genuinely differs from an unturned one, or the
        // azimuth would be silently doing nothing.
        let plain = ColumnGrid(pitch: 0.5, groundZ: 0, azimuthDeg: 0)
        #expect(grid.cell(x: 4.2, y: 1.1) != plain.cell(x: 4.2, y: 1.1))
    }

    /// The column grid quantises in plan through the same lattice, so a brush
    /// and a footprint agree about which side of a boundary a return is on.
    @Test func theColumnGridAgreesWithALatticeOfItsOwnPitch() {
        let grid = ColumnGrid(pitch: 0.5, groundZ: 0)
        let lattice = Lattice(pitch: 0.5)

        for (x, y) in [(0.2, 0.2), (-0.2, -0.2), (0.5, -0.5), (7.75, -3.25)] {
            let cell = grid.cell(x: Float(x), y: Float(y))
            let expected = lattice.cell(x: Float(x), y: Float(y))
            #expect(SIMD2(cell.i, cell.j) == expected, "at \(x), \(y)")
            #expect(grid.centre(of: cell) == lattice.centre(ofCell: expected))
        }
    }
}
