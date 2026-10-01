import Foundation
import simd

/// Diagnostic fit to selected returns, never a precision estimate or a rigid
/// registration. A plane leaves both tangent directions weak; a line leaves
/// motion along it weak. The visible centroid is not an identifiable body point.
struct FacetGeometryFit: Equatable {
    var count: Int
    var centre: SIMD3<Double>?
    var direction: SIMD3<Double>?
    var spanM: Double = 0
    var transverseSpreadM: Double = 0
    var residualM: Double = 0
    var supported = false
    var explanation: String
    var weakDirections: String

    static func analyse(points: PackPoints, indices: [UInt32], geometry: FeatureGeometry) -> Self {
        let values = Set(indices).sorted().compactMap { points.point(at: Int($0)) }.filter {
            $0.x.isFinite && $0.y.isFinite && $0.z.isFinite
        }.map { SIMD3<Double>(Double($0.x), Double($0.y), Double($0.z)) }
        var result = Self(count: values.count, explanation: "", weakDirections: "Unresolved")
        guard !values.isEmpty else {
            result.explanation = "No finite return support"
            return result
        }
        let centre = values.reduce(SIMD3<Double>.zero, +) / Double(values.count)
        result.centre = centre
        switch geometry {
        case .edge, .patch: break
        case .corner, .protrusion:
            result.explanation =
                "Local feature proposal: repeatable identity must be checked across frames"
            result.weakDirections = "No rigid relation established"
            return result
        default:
            result.explanation = "Choose a geometry type; points alone remain a valid proposal"
            return result
        }
        guard values.count >= 3 else {
            result.explanation =
                "At least three distinct returns are needed for an edge or surface fit"
            return result
        }
        var covariance = Array(repeating: Array(repeating: 0.0, count: 3), count: 3)
        for p in values {
            let d = p - centre
            for i in 0..<3 {
                for j in 0..<3 { covariance[i][j] += d[i] * d[j] / Double(values.count) }
            }
        }
        let eigen = symmetricEigen(covariance)
        let along = eigen[2].vector
        let across = eigen[1].vector
        let normal = eigen[0].vector
        func extent(_ direction: SIMD3<Double>) -> Double {
            let coordinates = values.map { simd_dot($0 - centre, direction) }
            return (coordinates.max() ?? 0) - (coordinates.min() ?? 0)
        }
        result.spanM = extent(along)
        result.transverseSpreadM = extent(across)
        // These floors are sampling-support checks, not sensor calibration or
        // uncertainty bounds. They must be audited on the experiment's tuning data.
        if geometry == .edge {
            result.direction = along
            result.residualM = sqrt(max(0, eigen[0].value + eigen[1].value))
            result.supported =
                result.spanM >= 0.1 && eigen[2].value > 0 && eigen[1].value / eigen[2].value <= 0.1
            result.explanation =
                result.supported
                ? "Line support found; inspect whether it is a physical edge or a scan boundary"
                : "Insufficient line span or support is too wide for an edge"
            result.weakDirections = "Along the edge; rotation about the edge"
        } else {
            result.direction = normal
            result.residualM = sqrt(max(0, eigen[0].value))
            result.supported =
                result.spanM >= 0.1 && result.transverseSpreadM >= 0.05 && eigen[1].value > 0
                && eigen[0].value / eigen[1].value <= 0.1
            result.explanation =
                result.supported
                ? "Surface support found; its changing visible centre is not a body anchor"
                : "Insufficient surface area, collinear support, or excessive thickness"
            result.weakDirections = "Both directions along the surface; rotation about its normal"
        }
        return result
    }

    /// Jacobi rotations of a real symmetric 3x3 covariance. Sorted ascending;
    /// signs are canonical only for display, never an assertion of front/rear.
    private static func symmetricEigen(
        _ input: [[Double]]
    ) -> [(value: Double, vector: SIMD3<Double>)] {
        var a = input
        var v = [[1.0, 0, 0], [0, 1.0, 0], [0, 0, 1.0]]
        for _ in 0..<32 {
            let pair = [(0, 1), (0, 2), (1, 2)].max { abs(a[$0.0][$0.1]) < abs(a[$1.0][$1.1]) }!
            let (p, q) = pair
            let scale = max(abs(a[0][0]) + abs(a[1][1]) + abs(a[2][2]), 1e-30)
            if abs(a[p][q]) <= scale * 1e-12 { break }
            let angle = 0.5 * atan2(2 * a[p][q], a[q][q] - a[p][p])
            let c = cos(angle)
            let s = sin(angle)
            let ap = a[p][p]
            let aq = a[q][q]
            let off = a[p][q]
            a[p][p] = c * c * ap - 2 * s * c * off + s * s * aq
            a[q][q] = s * s * ap + 2 * s * c * off + c * c * aq
            a[p][q] = 0
            a[q][p] = 0
            for i in 0..<3 where i != p && i != q {
                let ip = a[i][p]
                let iq = a[i][q]
                a[i][p] = c * ip - s * iq
                a[p][i] = a[i][p]
                a[i][q] = s * ip + c * iq
                a[q][i] = a[i][q]
            }
            for i in 0..<3 {
                let ip = v[i][p]
                let iq = v[i][q]
                v[i][p] = c * ip - s * iq
                v[i][q] = s * ip + c * iq
            }
        }
        return (0..<3).map { i in
            var vector = SIMD3(v[0][i], v[1][i], v[2][i])
            let major = (0..<3).max { abs(vector[$0]) < abs(vector[$1]) }!
            if vector[major] < 0 { vector = -vector }
            return (max(0, a[i][i]), vector)
        }.sorted { $0.0 < $1.0 }
    }
}
