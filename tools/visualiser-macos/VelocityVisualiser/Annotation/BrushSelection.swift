// BrushSelection.swift
// Sphere and column brushes, and the local column grid they share.
//
// The lasso selects "everything under this outline, within this depth range".
// That is the right rule for a large, isolated object and the wrong one for a
// pedestrian beside a parked car, where the unit wanted is smaller than an
// outline and larger than one return. These two brushes supply that unit.
//
// Both are new ways of proposing candidates and nothing more. They return the
// same SelectionCandidates the lasso does, which are applied by the same
// PointSelectionEngine.apply and recorded in the same MembershipHistory, so a
// mask is still a list of canonical point indices whichever tool chose them.
// A column is how points are picked, never how membership is stored: the grid
// can change its pitch, its ground or its frame and no saved mask moves.
//
// The grid is local. It is laid in the pack's own frame, in metres, with a
// lattice point at the pack's origin, and needs no position on the Earth. A
// capture is annotated long before it is registered to anything, so a tool
// that waited for registration would wait for ever.

import Foundation
import simd

// MARK: - Tools

/// The ways a selection gesture can be made.
enum SelectionTool: String, CaseIterable, Identifiable, Equatable {
    /// Freehand outline through the slab; command-drag for a rectangle.
    case lasso
    /// Every return within a radius of a picked return, in three dimensions.
    case sphere
    /// Every return in the painted columns, over the enabled voxels.
    case column

    var id: String { rawValue }

    var label: String {
        switch self {
        case .lasso: return "Lasso"
        case .sphere: return "Sphere"
        case .column: return "Column"
        }
    }

    /// One line on how the tool is used, shown under the picker.
    var hint: String {
        switch self {
        case .lasso: return "Drag to lasso · ⌘ rectangle · ⇧ add · ⌥ subtract"
        case .sphere: return "Click or drag to paint · [ ] size · ⇧scroll depth · ⌥ subtracts"
        case .column: return "Click or paint columns in Top · [ ] size · ⌥ subtracts"
        }
    }

    /// Whether the tool paints. A brush adds unless told to subtract: a dab
    /// that replaced the selection would erase the dab before it, which is
    /// never what painting means. The lasso keeps its replace-by-default.
    var isBrush: Bool { self != .lasso }
}

extension SelectionMode {
    /// Resolves the mode for a tool from the modifier keys held, so the rule
    /// lives beside the semantics rather than in the view layer.
    static func from(tool: SelectionTool, shiftHeld: Bool, optionHeld: Bool) -> SelectionMode {
        guard tool.isBrush else { return from(shiftHeld: shiftHeld, optionHeld: optionHeld) }
        return optionHeld ? .subtract : .add
    }
}

// MARK: - Column grid

/// One cell of the lattice: whole steps east and north of the pack's origin.
struct ColumnCell: Hashable, Equatable {
    var i: Int32
    var j: Int32
}

/// A square lattice of columns over a planar ground, each column a stack of
/// cubic voxels.
///
/// The pitch is 0.5 m because that is the size at which a standing pedestrian
/// fills two to four columns and a car about four dozen, measured rather than
/// chosen: finer and a pedestrian is a dozen cells, coarser and one can vanish
/// into a single cell. With 0.5 m voxels each box is a cube.
///
/// Ground is one height for the whole sample. Road users move on a surface
/// that is locally planar, and a single plane is the honest model until a
/// fitted ground surface is carried in the pack. It is a parameter the
/// operator can see and change, not a constant buried in the maths.
struct ColumnGrid: Equatable {
    /// Lattice pitch and voxel height, in metres.
    static let defaultPitch: Float = 0.5
    /// Voxels per column: ground to 4 m.
    static let voxelCount = 8
    /// The band road users occupy: voxels 1 to 4, 0.5 m to 2.5 m. Voxel 0 is
    /// left out because it holds kerbs and ground residue, the top three
    /// because they hold signs and branches; a bus is still caught by the
    /// lower four.
    static let stackMask: UInt8 = 0b0001_1110
    static let allVoxelsMask: UInt8 = 0b1111_1111
    /// How far below the ground plane a return may sit and still count as
    /// voxel 0. A plane is an approximation and range noise is real; without
    /// this, a wheel's lowest returns would fall out of every column.
    static let belowGroundTolerance: Float = 0.25

    var pitch: Float = ColumnGrid.defaultPitch
    /// World Z of the ground plane, in the pack's frame.
    var groundZ: Float = 0
    /// How far the columns are turned from the sensor's axes, so they line up
    /// with the kerbs rather than with the mounting. The scene's
    /// `grid_azimuth_deg`; see AnnotationSession's grid azimuth for where the
    /// value comes from and who owns it.
    var azimuthDeg: Float = 0

    /// In plan this is an ordinary lattice, and it has to round the same way
    /// as the ones the footprint and the proposer use.
    private var lattice: Lattice { Lattice(pitch: pitch, azimuthDeg: azimuthDeg) }

    /// The column containing a world position.
    func cell(x: Float, y: Float) -> ColumnCell {
        let c = lattice.cell(x: x, y: y)
        return ColumnCell(i: c.x, j: c.y)
    }

    /// World position of a column's centre.
    func centre(of cell: ColumnCell) -> simd_float2 {
        lattice.centre(ofCell: SIMD2(cell.i, cell.j))
    }

    /// The voxel a height falls in, or nil above the column or too far below
    /// the ground to be a return from anything standing on it.
    func voxel(z: Float) -> Int? {
        let h = z - groundZ
        if h < 0 { return h >= -ColumnGrid.belowGroundTolerance ? 0 : nil }
        let k = Int((h / pitch).rounded(.down))
        return k < ColumnGrid.voxelCount ? k : nil
    }

    /// Height range of a voxel above the ground, in metres.
    func heightRange(ofVoxel k: Int) -> ClosedRange<Float> {
        Float(k) * pitch...Float(k + 1) * pitch
    }

    /// The columns a brush of `radius` covers around a position: the column
    /// under the brush, always, and every column whose centre is within the
    /// radius. A radius of zero is one column.
    func cells(around centre: simd_float2, radius: Float) -> Set<ColumnCell> {
        let home = cell(x: centre.x, y: centre.y)
        var out: Set<ColumnCell> = [home]
        guard radius > 0, pitch > 0 else { return out }
        let reach = Int32((radius / pitch).rounded(.up))
        for di in -reach...reach {
            for dj in -reach...reach {
                let candidate = ColumnCell(i: home.i + di, j: home.j + dj)
                if simd_distance(self.centre(of: candidate), centre) <= radius {
                    out.insert(candidate)
                }
            }
        }
        return out
    }

    /// A ground height for a sample: the height below which a twentieth of its
    /// returns lie. The lowest returns from road users are wheels and feet, so
    /// a low percentile sits at the ground; the minimum would instead follow
    /// the stray returns metres below it that every recording has.
    static func estimateGroundZ(points: PackPoints) -> Float? {
        var heights: [Float] = []
        heights.reserveCapacity(points.count)
        for index in 0..<points.count {
            guard let p = points.point(at: index), p.z.isFinite else { continue }
            heights.append(p.z)
        }
        guard !heights.isEmpty else { return nil }
        heights.sort()
        return heights[min(heights.count - 1, heights.count / 20)]
    }
}

// MARK: - Sphere

/// A sphere in the pack's frame.
struct SelectionSphere: Equatable {
    var centre: simd_float3
    var radius: Float

    static let defaultRadius: Float = 0.5
    /// Largest radius the slider and the bracket keys reach. A drag may go
    /// further; this only bounds the controls.
    static let maximumRadius: Float = 3
    /// What one press of a bracket key changes the radius by.
    static let radiusStep: Float = 0.1
    /// Smallest radius a drag can set. Below this a sphere selects its own
    /// centre and nothing else, which is a click that looks like a no-op.
    static let minimumRadius: Float = 0.05
}

// MARK: - Strokes

/// Turns a brush gesture into a shape. Pure, so the rules are tested without
/// mounting a view.
enum BrushStroke {
    /// The brush positions between two cursor positions, `to` included and
    /// `from` not, no further apart than half the brush's radius. A fast drag
    /// reports the cursor every few dozen pixels, and marking only where it
    /// was reported would leave a dotted line through the object.
    static func path(from: simd_float2, to: simd_float2, radius: Float) -> [simd_float2] {
        let spacing = max(radius / 2, 0.02)
        let steps = max(1, Int((simd_distance(from, to) / spacing).rounded(.up)))
        return (1...steps).map { from + (to - from) * (Float($0) / Float(steps)) }
    }

    /// Largest column brush radius, and what one bracket press changes it by.
    static let maximumColumnRadius: Float = 2
    static let columnRadiusStep: Float = 0.25

    /// The next size up or down on a ladder of whole steps.
    ///
    /// A radius left by a drag is rarely on the ladder. Pressing a bracket
    /// moves to the next rung in that direction rather than adding a step to
    /// an odd number: 0.37 m goes to 0.4 m or 0.3 m, not 0.47 m or 0.27 m, so
    /// two presses in opposite directions land on a round value and stay there.
    static func stepped(
        _ value: Float, steps: Int, step: Float, minimum: Float, maximum: Float
    ) -> Float {
        guard steps != 0, step > 0 else { return min(max(value, minimum), maximum) }
        let rungs = value / step
        // A hundredth of a rung of slack, so 0.30000001 counts as being on
        // the 0.3 rung and not just above it.
        let slack: Float = 0.01
        let base = steps > 0 ? (rungs + slack).rounded(.down) : (rungs - slack).rounded(.up)
        let next = (base + Float(steps)) * step
        return min(max(next, minimum), maximum)
    }

    static func steppedSphereRadius(_ radius: Float, steps: Int) -> Float {
        stepped(
            radius, steps: steps, step: SelectionSphere.radiusStep,
            minimum: SelectionSphere.minimumRadius, maximum: SelectionSphere.maximumRadius)
    }

    static func steppedColumnRadius(_ radius: Float, steps: Int) -> Float {
        stepped(
            radius, steps: steps, step: columnRadiusStep, minimum: 0, maximum: maximumColumnRadius)
    }

    /// The columns painted by moving the brush from one position to another.
    ///
    /// A fast drag reports positions metres apart. Sampling only those would
    /// leave gaps in the stroke, so the segment is walked at half the pitch,
    /// which cannot step over a column.
    static func cells(
        from start: simd_float2, to end: simd_float2, grid: ColumnGrid, radius: Float
    ) -> Set<ColumnCell> {
        var out = grid.cells(around: end, radius: radius)
        let length = simd_distance(start, end)
        guard length > 0, grid.pitch > 0, length.isFinite else { return out }
        let steps = Int((length / (grid.pitch * 0.5)).rounded(.up))
        for step in 0..<steps {
            let t = Float(step) / Float(steps)
            out.formUnion(grid.cells(around: start + (end - start) * t, radius: radius))
        }
        return out
    }
}

// MARK: - Candidate evaluation

extension PointSelectionEngine {
    /// The return nearest a position in a view, among those inside the slab.
    ///
    /// A sphere needs a centre in three dimensions and a click gives two. The
    /// third is taken from a real return under the cursor rather than guessed,
    /// so the sphere is always centred on something the sensor saw. Returns
    /// nil when nothing lies within `maxViewDistance` metres of the click.
    static func nearestPoint(
        points: PackPoints, basis: OrthoViewBasis, viewPoint: simd_float2, slab: DepthSlab?,
        maxViewDistance: Float, where include: (Int) -> Bool = { _ in true }
    ) -> Int? {
        var best: Int?
        var bestDistance = maxViewDistance
        for index in 0..<points.count where include(index) {
            guard let p = points.point(at: index), p.x.isFinite, p.y.isFinite, p.z.isFinite else {
                continue
            }
            guard basis.shows(p) else { continue }
            if let slab, !slab.contains(basis.depth(p)) { continue }
            let d = simd_distance(basis.project(p), viewPoint)
            if d <= bestDistance {
                bestDistance = d
                best = index
            }
        }
        return best
    }

    /// Points within a sphere, by true distance in three dimensions.
    ///
    /// The slab still applies, and what it excluded is still counted: an
    /// operator who set a slab to keep the ground out means it for every tool.
    static func candidates(
        points: PackPoints, sphere: SelectionSphere, basis: OrthoViewBasis, slab: DepthSlab?
    ) -> SelectionCandidates {
        guard sphere.radius > 0, sphere.radius.isFinite else {
            return SelectionCandidates(indices: [], excludedBySlab: 0)
        }
        let limit = sphere.radius * sphere.radius
        var indices: [Int] = []
        var excludedBySlab = 0
        for index in 0..<points.count {
            guard let p = points.point(at: index), p.x.isFinite, p.y.isFinite, p.z.isFinite else {
                continue
            }
            guard simd_distance_squared(p, sphere.centre) <= limit else { continue }
            if let slab, !slab.contains(basis.depth(p)) {
                excludedBySlab += 1
                continue
            }
            indices.append(index)
        }
        return SelectionCandidates(indices: indices, excludedBySlab: excludedBySlab)
    }

    /// Points in a set of columns, over the enabled voxels.
    ///
    /// Returns inside a painted column but in a voxel that is switched off, or
    /// outside the column's height altogether, are counted in
    /// `excludedByVoxels`. "This cell, but not the ground" then reads as what
    /// it is, and not as the brush having missed.
    static func candidates(
        points: PackPoints, cells: Set<ColumnCell>, grid: ColumnGrid, enabledVoxels: UInt8,
        basis: OrthoViewBasis, slab: DepthSlab?
    ) -> SelectionCandidates {
        guard !cells.isEmpty else { return SelectionCandidates(indices: [], excludedBySlab: 0) }
        var indices: [Int] = []
        var excludedBySlab = 0
        var excludedByVoxels = 0
        for index in 0..<points.count {
            guard let p = points.point(at: index), p.x.isFinite, p.y.isFinite, p.z.isFinite else {
                continue
            }
            guard cells.contains(grid.cell(x: p.x, y: p.y)) else { continue }
            guard let k = grid.voxel(z: p.z), enabledVoxels & (1 << UInt8(k)) != 0 else {
                excludedByVoxels += 1
                continue
            }
            if let slab, !slab.contains(basis.depth(p)) {
                excludedBySlab += 1
                continue
            }
            indices.append(index)
        }
        return SelectionCandidates(
            indices: indices, excludedBySlab: excludedBySlab, excludedByVoxels: excludedByVoxels)
    }
}
