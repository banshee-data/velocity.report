// BrushHover.swift
// Where the sphere brush would mark if the button went down now.
//
// This is its own observable object because it changes on every mouse move and
// only two views read it: the overlay that draws the brush and the 3D view
// that marks the returns under it. Held on the session as a plain `let`, a
// hover cannot republish the session, so the eight other views observing the
// session stop redrawing while the cursor moves.

import Combine
import Foundation
import simd

@MainActor final class BrushHover: ObservableObject {
    /// Shown in every view, because the one the cursor is in cannot show its
    /// depth.
    @Published private(set) var sphere: SelectionSphere?
    /// The returns inside `sphere`.
    @Published private(set) var indices: [Int] = []
    /// Moves the brush along the view's depth axis, away from the return it
    /// took its depth from. In the top view that is up and down.
    @Published private(set) var depthOffset: Float = 0

    /// Called after any change, so that the 3D view's redraw token moves with
    /// it. The renderer compares a revision rather than observing, so without
    /// this the marks under the brush would lag the brush.
    var didChange: (() -> Void)?

    /// No change means no publish: a cursor moving inside one return's
    /// neighbourhood reports the same sphere many times over.
    func show(_ sphere: SelectionSphere?, indices: [Int]) {
        guard sphere != self.sphere || indices != self.indices else { return }
        self.sphere = sphere
        self.indices = indices
        didChange?()
    }

    func clear() {
        guard sphere != nil || !indices.isEmpty else { return }
        sphere = nil
        indices = []
        didChange?()
    }

    func setDepthOffset(_ offset: Float) {
        guard offset != depthOffset else { return }
        depthOffset = offset
        didChange?()
    }
}
