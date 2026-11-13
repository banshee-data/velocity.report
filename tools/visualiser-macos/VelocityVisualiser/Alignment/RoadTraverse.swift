// RoadTraverse.swift
// Measuring a traverse of the driving view: frame times, resident memory,
// draw calls and what the working set held, kept as plain values so the
// numbers can be summarised and written to a review note with the machine,
// build, settings and input hashes they were taken on.

import CryptoKit
import Darwin
import Foundation

enum ProcessMemory {
    /// The process's physical footprint, bytes (what Activity Monitor calls
    /// Memory); 0 if the kernel does not answer.
    static func footprintBytes() -> UInt64 {
        var info = task_vm_info_data_t()
        var count = mach_msg_type_number_t(
            MemoryLayout<task_vm_info_data_t>.size / MemoryLayout<integer_t>.size)
        let kr = withUnsafeMutablePointer(to: &info) {
            $0.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
                task_info(mach_task_self_, task_flavor_t(TASK_VM_INFO), $0, &count)
            }
        }
        return kr == KERN_SUCCESS ? info.phys_footprint : 0
    }
}

/// One sample of the traverse, taken once a frame.
struct TraverseSample: Sendable, Equatable {
    /// Seconds since the previous rendered frame.
    var interval: Double
    /// CPU time of the app's own tick (route, working set, camera) and of the
    /// renderer's encode, ms.
    var tickMs: Double
    var encodeMs: Double = 0
    /// GPU time of the frame, ms; 0 until its command buffer completes.
    var gpuMs: Double = 0
    /// Which pass over the route the frame belongs to.
    var loop: Int = 0
    var drawCalls: Int
    var visiblePatches: Int
    var residentPatches: Int
    var residentMeshBytes: Int
    var footprintBytes: UInt64
}

struct TraverseSummary: Sendable, Equatable {
    var frames: Int
    var seconds: Double
    var intervalP50Ms: Double
    var intervalP95Ms: Double
    var intervalP99Ms: Double
    var intervalMaxMs: Double
    /// Where the slowest frame fell: its index and the pass it was in.
    var worstFrameIndex: Int
    var worstFrameLoop: Int
    var tickP95Ms: Double
    var encodeP95Ms: Double
    var gpuP95Ms: Double
    var framesOver16_7: Int  // frames over 20 ms: a missed vsync at 60 Hz
    var drawCallsMin: Int
    var drawCallsMax: Int
    var residentPatchesMax: Int
    var residentMeshBytesMax: Int
    var footprintFirstMB: Double
    var footprintLastMB: Double
    var footprintMaxMB: Double
    /// Footprint growth over the last half of the run, MB: near 0 means it settled.
    var footprintSecondHalfGrowthMB: Double

    /// The provisional target: p95 frame interval at most 16.7 ms.
    var meetsTarget: Bool { intervalP95Ms <= 16.7 }
}

struct TraverseMeter: Sendable {
    private(set) var samples: [TraverseSample] = []
    static let cap = 200_000

    mutating func record(_ s: TraverseSample) { if samples.count < Self.cap { samples.append(s) } }

    mutating func reset() { samples = [] }

    /// Nearest-rank percentile of `xs`, 0 < p ≤ 1.
    static func percentile(_ xs: [Double], _ p: Double) -> Double {
        guard !xs.isEmpty else { return 0 }
        let sorted = xs.sorted()
        let rank = Int((p * Double(sorted.count)).rounded(.up)) - 1
        return sorted[min(max(rank, 0), sorted.count - 1)]
    }

    func summary() -> TraverseSummary? {
        guard !samples.isEmpty else { return nil }
        let ms = samples.map { $0.interval * 1000 }
        let tick = samples.map(\.tickMs)
        let mb = samples.map { Double($0.footprintBytes) / 1_048_576 }
        let half = mb.count / 2
        let grown = (mb.last ?? 0) - (mb.dropFirst(half).first ?? 0)
        return TraverseSummary(
            frames: samples.count, seconds: samples.map(\.interval).reduce(0, +),
            intervalP50Ms: Self.percentile(ms, 0.5), intervalP95Ms: Self.percentile(ms, 0.95),
            intervalP99Ms: Self.percentile(ms, 0.99), intervalMaxMs: ms.max() ?? 0,
            worstFrameIndex: ms.firstIndex(of: ms.max() ?? 0) ?? 0,
            worstFrameLoop: samples[ms.firstIndex(of: ms.max() ?? 0) ?? 0].loop,
            tickP95Ms: Self.percentile(tick, 0.95),
            encodeP95Ms: Self.percentile(samples.map(\.encodeMs), 0.95),
            gpuP95Ms: Self.percentile(samples.map(\.gpuMs), 0.95),
            framesOver16_7: ms.filter { $0 > 20 }.count,
            drawCallsMin: samples.map(\.drawCalls).min() ?? 0,
            drawCallsMax: samples.map(\.drawCalls).max() ?? 0,
            residentPatchesMax: samples.map(\.residentPatches).max() ?? 0,
            residentMeshBytesMax: samples.map(\.residentMeshBytes).max() ?? 0,
            footprintFirstMB: mb.first ?? 0, footprintLastMB: mb.last ?? 0,
            footprintMaxMB: mb.max() ?? 0, footprintSecondHalfGrowthMB: grown)
    }
}

/// What a measurement was taken on and from, so it can be repeated or doubted.
struct TraverseProvenance: Sendable, Equatable {
    var mac: String
    var os: String
    var buildSHA: String
    var buildVersion: String
    var settings: String
    var inputHashes: [String: String]

    static func current(
        settings: String, corridor: [RoadCorridorInfo], patches: [RoadPatch]
    ) -> TraverseProvenance {
        var size = 0
        sysctlbyname("hw.model", nil, &size, nil, 0)
        var model = [CChar](repeating: 0, count: max(size, 1))
        sysctlbyname("hw.model", &model, &size, nil, 0)
        var brand = [CChar](repeating: 0, count: 128)
        var bsize = brand.count
        sysctlbyname("machdep.cpu.brand_string", &brand, &bsize, nil, 0)
        let info = Bundle.main.infoDictionary
        let hashes = patches.map(\.geometryHash).sorted().joined(separator: ",")
        var inputs: [String: String] = [
            "patch_geometry_hashes_sha256": SHA256.hash(data: Data(hashes.utf8)).hex
        ]
        for c in corridor {
            inputs["manifest:\(c.corridorID)"] = c.manifestHash
            inputs["build:\(c.corridorID)"] = c.buildHash
        }
        return TraverseProvenance(
            mac: "\(String(cString: model)) · \(String(cString: brand))",
            os: ProcessInfo.processInfo.operatingSystemVersionString,
            buildSHA: (info?["VRGitSHA"] as? String) ?? "unrecorded",
            buildVersion: (info?["CFBundleShortVersionString"] as? String) ?? "unknown",
            settings: settings, inputHashes: inputs)
    }
}

extension SHA256.Digest { var hex: String { map { String(format: "%02x", $0) }.joined() } }

enum TraverseReport {
    /// A short Markdown note of one measurement.
    static func markdown(
        _ s: TraverseSummary, _ p: TraverseProvenance, date: String, label: String
    ) -> String {
        func f(_ v: Double, _ d: Int = 2) -> String { String(format: "%.\(d)f", v) }
        var out = "# Driving traverse: \(label)\n\n"
        out += "- date: \(date)\n- Mac: \(p.mac), macOS \(p.os)\n"
        out += "- build: \(p.buildVersion), git \(p.buildSHA)\n- settings: \(p.settings)\n"
        for (k, v) in p.inputHashes.sorted(by: { $0.key < $1.key }) { out += "- \(k): `\(v)`\n" }
        out += "\n| Measure | Value |\n| --- | --- |\n"
        out += "| frames | \(s.frames) over \(f(s.seconds, 1)) s |\n"
        out +=
            "| frame interval p50 / p95 / p99 / max | \(f(s.intervalP50Ms)) / \(f(s.intervalP95Ms)) / \(f(s.intervalP99Ms)) / \(f(s.intervalMaxMs)) ms |\n"
        out += "| frames over 20 ms (a missed vsync) | \(s.framesOver16_7) |\n"
        out += "| slowest frame | #\(s.worstFrameIndex), pass \(s.worstFrameLoop + 1) |\n"
        out +=
            "| app tick p95 / render encode p95 / GPU p95 | \(f(s.tickP95Ms, 3)) / \(f(s.encodeP95Ms, 3)) / \(f(s.gpuP95Ms, 3)) ms |\n"
        out += "| draw calls | \(s.drawCallsMin)–\(s.drawCallsMax) |\n"
        out +=
            "| resident patches (max) / mesh | \(s.residentPatchesMax) / \(f(Double(s.residentMeshBytesMax) / 1_048_576)) MB |\n"
        out +=
            "| footprint first / last / max | \(f(s.footprintFirstMB, 1)) / \(f(s.footprintLastMB, 1)) / \(f(s.footprintMaxMB, 1)) MB |\n"
        out += "| footprint growth, second half | \(f(s.footprintSecondHalfGrowthMB, 1)) MB |\n"
        out += "| p95 ≤ 16.7 ms target | \(s.meetsTarget ? "met" : "not met") |\n"
        return out
    }
}
