// AnnotationViewState.swift
// What the annotation views show, and from where: which point classes are
// drawn, and the framing each orthographic view holds.
//
// The framing is state, not something derived from the sample on screen. A
// view that re-fits itself to every sample changes scale and centre each time
// the operator steps, so a car that was under the cursor is somewhere else at
// a different size one frame later. Holding the framing still is what lets an
// operator follow one object through consecutive samples.

import CoreGraphics
import Foundation
import simd

// MARK: - Point classes

/// The per-point classes the tool draws and filters by.
///
/// The first three are the recorder's. `ground` also takes in every return the
/// run's height band removed, whatever the recorder classed it: the recorder
/// classes a return before L4 runs, so a foreground return under the floor is
/// recorded as foreground and then never reaches the clusterer. For judging a
/// track against the points it could have used, that return is ground.
enum PointClass {
    static let background: UInt8 = 0
    static let foreground: UInt8 = 1
    static let ground: UInt8 = 2
    /// A pack recorded without classes. Always drawn: there is nothing to
    /// filter it by, and its zero class bytes are padding, not "background".
    static let unclassified: UInt8 = 255

    /// The class each point is drawn and filtered as.
    static func displayClasses(
        points: PackPoints, hasClassification: Bool, band: HeightBand?
    ) -> [UInt8] {
        (0..<points.count).map { index in
            if let band, band.removes(z: points.z[index]) { return ground }
            guard hasClassification, index < points.classification.count else {
                return unclassified
            }
            return points.classification[index]
        }
    }
}

/// Which point classes are drawn, and therefore which can be selected.
///
/// The two are the same set on purpose. A gesture that took in returns the
/// operator had hidden would put points in a mask that nobody looked at.
/// How far through a return is: what the progress bars count, per point.
enum PointLabelState: Equatable {
    /// In a reviewed mask, or one made by hand. Settled.
    case agreed
    /// Saved, but still the algorithm's word for it.
    case inQuestion
    /// Nothing claims it yet.
    case unlabelled
}

struct PointVisibility: Equatable {
    var background = true
    var foreground = true
    var ground = true

    /// The same three states the progress bars count. Switching the settled
    /// ones off is how the work left is looked at on its own, and because a
    /// hidden return cannot be selected, it is also what keeps a lasso over
    /// the remainder from taking back what is already agreed.
    var agreed = true
    var inQuestion = true
    var unlabelled = true

    var showsEverything: Bool {
        background && foreground && ground && agreed && inQuestion && unlabelled
    }

    /// True when the label states are not filtered, so a caller can skip
    /// working out which state each return is in.
    var showsEveryLabelState: Bool { agreed && inQuestion && unlabelled }

    func shows(_ state: PointLabelState) -> Bool {
        switch state {
        case .agreed: return agreed
        case .inQuestion: return inQuestion
        case .unlabelled: return unlabelled
        }
    }

    /// A class this client does not know is shown rather than hidden: a newer
    /// recorder adding a class must not make returns vanish from an older app.
    func shows(_ classification: UInt8) -> Bool {
        switch classification {
        case PointClass.background: return background
        case PointClass.foreground: return foreground
        case PointClass.ground: return ground
        default: return true
        }
    }
}

/// Which of a frame's returns are agreed and which are in question.
///
/// Passed about as one value because the two sets are always wanted together
/// and are rebuilt together. `revision` is what callers compare: the sets are
/// thousands of indices and comparing them to decide whether to redraw would
/// cost more than the redraw.
struct PointLabelSets: Equatable {
    var agreed: Set<Int> = []
    var inQuestion: Set<Int> = []
    var revision = 0

    static func == (lhs: PointLabelSets, rhs: PointLabelSets) -> Bool {
        lhs.revision == rhs.revision
    }

    func state(of index: Int) -> PointLabelState {
        if agreed.contains(index) { return .agreed }
        if inQuestion.contains(index) { return .inQuestion }
        return .unlabelled
    }
}

extension PointVisibility {
    /// True when the point at `index` is drawn. `classes` are display classes
    /// (see `PointClass.displayClasses`), and a nil visibility is "no filter".
    static func isVisible(
        _ index: Int, classes: [UInt8], under visibility: PointVisibility?,
        labels: PointLabelSets? = nil
    ) -> Bool {
        guard let visibility, index >= 0, index < classes.count else { return true }
        guard visibility.shows(classes[index]) else { return false }
        return showsLabelState(index, under: visibility, labels: labels)
    }

    /// True when this return's label state is shown, ignoring its class.
    ///
    /// The layers that draw masks ask this rather than `isVisible`, because a
    /// mask is drawn through a hidden class on purpose — switching the
    /// background off must not hide what a mask claims about it — while
    /// switching a label state off is a request to hide exactly those masks.
    /// The two filters are not the same kind of thing and the mask override
    /// only ever applied to the first.
    static func showsLabelState(
        _ index: Int, under visibility: PointVisibility?, labels: PointLabelSets?
    ) -> Bool {
        guard let visibility, !visibility.showsEveryLabelState, let labels else { return true }
        return visibility.shows(labels.state(of: index))
    }
}

// MARK: - View framing

/// The framing of one orthographic view: where it looks and at what scale.
struct OrthoViewState: Equatable {
    /// View-plane coordinates at the centre of the view, in metres.
    var centre: simd_float2
    /// Half-height of the visible world volume, in metres.
    var halfHeight: Float

    /// Two returns half a metre apart still separate at this scale.
    static let minimumHalfHeight: Float = 0.25
    /// Beyond the sensor's range in every direction.
    static let maximumHalfHeight: Float = 500

    /// Moves the view so the content follows a drag of `delta` screen points.
    mutating func pan(byPoints delta: CGSize, metresPerPoint: Float) {
        guard metresPerPoint.isFinite, metresPerPoint > 0 else { return }
        centre.x -= Float(delta.width) * metresPerPoint
        // Screen y grows downward; the view plane's up axis grows upward.
        centre.y += Float(delta.height) * metresPerPoint
    }

    /// Scales the view by `factor` (below one zooms in), keeping the world
    /// point `anchor` where it is on screen: zooming on a car brings the car
    /// closer rather than sliding it out of view.
    mutating func zoom(by factor: Float, about anchor: simd_float2) {
        guard factor.isFinite, factor > 0, halfHeight > 0 else { return }
        let next = min(
            max(halfHeight * factor, OrthoViewState.minimumHalfHeight),
            OrthoViewState.maximumHalfHeight)
        // The applied factor, not the requested one: at a limit the scale
        // stops changing, so the centre must stop moving too.
        let applied = next / halfHeight
        centre = anchor + (centre - anchor) * applied
        halfHeight = next
    }
}

/// What the views are asked to frame.
enum AnnotationFitTarget: Equatable {
    /// Every drawn return of the current sample.
    case sample
    /// The current sample's foreground returns: where the road users are.
    case foreground
    /// The points in the membership under edit.
    case selection
}

/// A region of the scene, for pointing the 3D view at what the orthographic
/// views have just been asked to frame.
struct AnnotationSceneFocus: Equatable {
    var centre: simd_float3
    var radius: Float
    /// Distinguishes a repeated request for the same region, so asking to fit
    /// twice moves the camera back twice.
    var revision: Int
}

/// The view-plane extent of the chosen points, with the outermost `trim`
/// fraction on each side of each axis left out.
///
/// A revolution of a 200 m sensor always has a few returns at the edge of its
/// range. Framing on the true minimum and maximum gives those few returns most
/// of the view and leaves the street a few dozen pixels across.
func annotationExtent(
    of points: PackPoints, basis: OrthoViewBasis, trim: Float, where include: (Int) -> Bool
) -> AnnotationExtent? {
    var xs: [Float] = []
    var ys: [Float] = []
    xs.reserveCapacity(points.count)
    ys.reserveCapacity(points.count)
    for index in 0..<points.count where include(index) {
        guard let p = points.point(at: index), p.x.isFinite, p.y.isFinite, p.z.isFinite,
            basis.shows(p)
        else { continue }
        let v = basis.project(p)
        xs.append(v.x)
        ys.append(v.y)
    }
    guard !xs.isEmpty else { return nil }
    xs.sort()
    ys.sort()
    let clamped = min(max(trim, 0), 0.49)
    let lo = Int((Float(xs.count - 1) * clamped).rounded(.down))
    let hi = xs.count - 1 - lo
    return AnnotationExtent(
        centre: simd_float2((xs[lo] + xs[hi]) / 2, (ys[lo] + ys[hi]) / 2),
        halfHeight: max((ys[hi] - ys[lo]) / 2, 0), halfWidth: max((xs[hi] - xs[lo]) / 2, 0))
}
