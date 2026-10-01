// Compile with the production FacetGeometry.swift. This standalone harness
// measures its scalar fitting work, not a tracker, UI frame or sensor model.
// The minimal source types have the fields that the fitter actually reads;
// no protobuf runtime, point decoding or rendering is included in the timing.
import Foundation
import simd

enum FeatureGeometry { case unknown, edge, corner, patch, protrusion }

struct PackPoints {
    var x: [Float]
    var y: [Float]
    var z: [Float]

    func point(at index: Int) -> SIMD3<Float>? {
        guard index >= 0, index < x.count, index < y.count, index < z.count else { return nil }
        return SIMD3(x[index], y[index], z[index])
    }
}

@main enum FacetFitBenchmark {
    static func main() {
        let baseline = CommandLine.arguments.contains("--baseline")
        print(
            baseline
                ? "Clock/input baseline; no geometry fit"
                : "Production facet geometry fit; optimised build")
        #if arch(arm64)
            print("Architecture: arm64")
        #else
            print("Architecture: x86_64")
        #endif
        print(
            "Synthetic tilted planes; \(baseline ? 0 : 20) fit warm-ups and 200 timing samples per size"
        )
        var checksum = 0.0
        for count in [50, 200, 1000, 5000] {
            let side = Int(sqrt(Double(count)))
            var points = PackPoints(x: [], y: [], z: [])
            for index in 0..<count {
                let x = Float(index % side) / Float(side)
                let y = Float(index / side) / Float(side)
                points.x.append(x)
                points.y.append(y)
                points.z.append(1 + 0.1 * x + 0.2 * y)
            }
            let indices = (0..<count).map(UInt32.init)
            for _ in 0..<20 where !baseline {
                checksum +=
                    FacetGeometryFit.analyse(points: points, indices: indices, geometry: .patch)
                    .residualM
            }
            var samples: [Double] = []
            for _ in 0..<200 {
                let started = ContinuousClock.now
                if !baseline {
                    checksum +=
                        FacetGeometryFit.analyse(points: points, indices: indices, geometry: .patch)
                        .spanM
                } else {
                    checksum += Double(points.x[indices.count - 1])
                }
                let elapsed = started.duration(to: .now).components
                samples.append(Double(elapsed.seconds) * 1000 + Double(elapsed.attoseconds) / 1e15)
            }
            samples.sort()
            print(
                String(
                    format: "returns=%d p50_ms=%.6f p95_ms=%.6f max_ms=%.6f", count, samples[100],
                    samples[190], samples.last!))
        }
        print("checksum=\(checksum)")
    }
}
