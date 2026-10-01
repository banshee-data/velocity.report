import Foundation
import Testing
import simd

@testable import VelocityVisualiser

struct FacetGeometryTests {
    private func points(_ values: [SIMD3<Float>]) -> PackPoints {
        PackPoints(
            x: values.map(\.x), y: values.map(\.y), z: values.map(\.z),
            intensity: Array(repeating: 0, count: values.count),
            classification: Array(repeating: 1, count: values.count))
    }

    @Test func aRotatedLineHasOnlyLineSupportAndDoesNotClaimAFace() {
        let direction = simd_normalize(SIMD3<Float>(1, 2, 3))
        let p = points([-0.5, 0, 0.5].map { SIMD3<Float>(4, 7, 2) + Float($0) * direction })
        let line = FacetGeometryFit.analyse(points: p, indices: [2, 0, 1, 1], geometry: .edge)
        #expect(line.count == 3 && line.supported)
        #expect(abs(line.spanM - 1) < 1e-5 && line.residualM < 1e-6)
        #expect(
            abs(
                simd_dot(
                    line.direction!,
                    SIMD3<Double>(Double(direction.x), Double(direction.y), Double(direction.z))))
                > 0.99999)
        #expect(line.weakDirections.contains("Along the edge"))
        let surface = FacetGeometryFit.analyse(points: p, indices: [0, 1, 2], geometry: .patch)
        #expect(!surface.supported, "a scan strand is not a supported surface")
    }

    @Test func aRotatedSurfaceHasANormalButItsCentreIsNotAStableBodyPoint() {
        let u = simd_normalize(SIMD3<Float>(1, 1, 0))
        let v = simd_normalize(SIMD3<Float>(-1, 1, 2))
        var values: [SIMD3<Float>] = []
        let origin = SIMD3<Float>(4, 5, 2)
        for x: Float in [-0.5, 0.5] {
            for y: Float in [-0.2, 0.2] {
                let along = x * u
                let across = y * v
                values.append(origin + along + across)
            }
        }
        let p = points(values)
        let fit = FacetGeometryFit.analyse(points: p, indices: [0, 1, 2, 3], geometry: .patch)
        #expect(fit.supported && fit.count == 4 && fit.residualM < 1e-6)
        let n = simd_normalize(simd_cross(u, v))
        #expect(
            abs(simd_dot(fit.direction!, SIMD3<Double>(Double(n.x), Double(n.y), Double(n.z))))
                > 0.99999)
        #expect(fit.weakDirections.contains("Both directions along"))
        #expect(fit.explanation.contains("not a body anchor"))
        #expect(
            !FacetGeometryFit.analyse(points: p, indices: [0, 1, 2, 3], geometry: .edge).supported)
    }

    @Test func sparseNonFiniteCoincidentAndVolumetricSupportRemainProposals() {
        let p = points([SIMD3(.nan, 0, 0), .zero, .zero, .zero])
        #expect(FacetGeometryFit.analyse(points: p, indices: [0, 99], geometry: .edge).count == 0)
        #expect(!FacetGeometryFit.analyse(points: p, indices: [1, 2], geometry: .edge).supported)
        #expect(
            !FacetGeometryFit.analyse(points: p, indices: [1, 2, 3], geometry: .patch).supported)
        var corners: [SIMD3<Float>] = []
        for x: Float in [-1, 1] {
            for y: Float in [-1, 1] { for z: Float in [-1, 1] { corners.append(SIMD3(x, y, z)) } }
        }
        let cube = points(corners)
        #expect(
            !FacetGeometryFit.analyse(points: cube, indices: Array(0..<8), geometry: .patch)
                .supported)
        #expect(
            !FacetGeometryFit.analyse(points: cube, indices: [0], geometry: .protrusion).supported)
        #expect(!FacetGeometryFit.analyse(points: cube, indices: [0], geometry: .unknown).supported)
    }
}
