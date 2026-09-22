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
