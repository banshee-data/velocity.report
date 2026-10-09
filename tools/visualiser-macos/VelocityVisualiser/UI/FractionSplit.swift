// FractionSplit.swift
// Two panes divided at a fraction of the space between them, with a divider
// to drag.
//
// SwiftUI's split views take their first layout from AppKit, which shares the
// space by the panes' minimum sizes in points and cannot be told a starting
// proportion. In the annotation window that gave the 3D view nearly the whole
// height and left the five selection views a strip along the bottom. A
// fraction holds its proportion as the window is resized, and is the same on
// a laptop as on a large display.

import AppKit
import SwiftUI

struct FractionSplit<First: View, Second: View>: View {
    enum Axis {
        /// The first pane above the second.
        case vertical
        /// The first pane left of the second.
        case horizontal
    }

    let axis: Axis
    /// The first pane's share of the space, not counting the divider.
    @Binding var fraction: Double
    /// Where a drag may take it, so that neither pane can be lost.
    var range: ClosedRange<Double> = 0.1...0.9
    /// What a double-click on the divider puts back.
    var reset: Double?
    @ViewBuilder var first: First
    @ViewBuilder var second: Second

    @State private var dragStart: Double?

    static var dividerThickness: CGFloat { 5 }

    /// The fraction a drag of `translation` points makes of one that began at
    /// `start`, over `length` points of space, held inside `range`.
    static func dragged(
        from start: Double, by translation: CGFloat, over length: CGFloat,
        range: ClosedRange<Double>
    ) -> Double {
        guard length > 0 else { return min(max(start, range.lowerBound), range.upperBound) }
        let next = start + Double(translation / length)
        return min(max(next, range.lowerBound), range.upperBound)
    }

    var body: some View {
        GeometryReader { geometry in
            let total = axis == .vertical ? geometry.size.height : geometry.size.width
            let space = max(total - Self.dividerThickness, 0)
            let clamped = min(max(fraction, range.lowerBound), range.upperBound)
            let firstLength = (space * clamped).rounded()
            let layout =
                axis == .vertical
                ? AnyLayout(VStackLayout(spacing: 0)) : AnyLayout(HStackLayout(spacing: 0))
            layout {
                first.frame(
                    width: axis == .horizontal ? firstLength : nil,
                    height: axis == .vertical ? firstLength : nil
                ).frame(
                    maxWidth: axis == .vertical ? .infinity : nil,
                    maxHeight: axis == .horizontal ? .infinity : nil
                ).clipped()
                divider(space: space)
                second.frame(maxWidth: .infinity, maxHeight: .infinity).clipped()
            }
        }
    }

    private func divider(space: CGFloat) -> some View {
        Rectangle().fill(Color(nsColor: .separatorColor)).frame(
            width: axis == .horizontal ? Self.dividerThickness : nil,
            height: axis == .vertical ? Self.dividerThickness : nil
        ).contentShape(Rectangle()).onHover { inside in
            let cursor: NSCursor = axis == .vertical ? .resizeUpDown : .resizeLeftRight
            if inside { cursor.push() } else { NSCursor.pop() }
        }.gesture(
            DragGesture(minimumDistance: 1, coordinateSpace: .global).onChanged { value in
                let start = dragStart ?? fraction
                dragStart = start
                fraction = Self.dragged(
                    from: start,
                    by: axis == .vertical ? value.translation.height : value.translation.width,
                    over: space, range: range)
            }.onEnded { _ in dragStart = nil }
        ).onTapGesture(count: 2) { if let reset { fraction = reset } }.help(
            "Drag to resize. Double-click to put it back.")
    }
}
