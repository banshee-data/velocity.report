// RefinementSummary.swift
// What a site refinement did, one row per scan, for the top of the
// refinement column: how far each refined pose moved, what accepting it
// records, and a state that colours the row and the scan's sensor arrow and
// correction line in the scene. Nothing here talks to a server.

import Foundation
import simd

/// What the refinement means for one scan, as the column and the scene
/// colour it. One palette for both (docs/ui/visualiser/gerardus-alignment.md).
enum RefinedScanState: String, CaseIterable, Sendable {
    /// Moved under 1 cm and under 0.05°: accepting changes nothing to see.
    case unchanged
    /// Moved, and its run passed every gate: accepting records REGISTERED.
    case registered
    /// Moved, and a gate failed (or its gates were not read): accepting
    /// records MANUALLY_PLACED.
    case manuallyPlaced
    /// Left out of the joint solve (no pair of its was used, or it was not
    /// refined at all), or stale: accepted again since the refinement ran.
    case leftOut

    var title: String {
        switch self {
        case .unchanged: return "unchanged"
        case .registered: return "moved, gates pass"
        case .manuallyPlaced: return "moved, a gate fails"
        case .leftOut: return "left out or stale"
        }
    }

    /// Okabe–Ito colours, chosen apart from every reference layer's (a test
    /// holds them at least 0.25 apart in RGB from each, and from each other).
    var colour: SIMD4<Float> {
        switch self {
        case .unchanged: return SIMD4(0.50, 0.50, 0.56, 1)  // slate grey #80808F
        case .registered: return SIMD4(0.00, 0.62, 0.45, 1)  // bluish green #009E73
        case .manuallyPlaced: return SIMD4(0.84, 0.37, 0.00, 1)  // vermillion #D55E00
        case .leftOut: return SIMD4(0.00, 0.45, 0.70, 1)  // blue #0072B2
        }
    }
}

/// One scan's line in the refinement summary.
struct RefinementSummaryRow: Identifiable, Equatable, Sendable {
    var scanID: String
    /// The scan's capture_ref from the site model; empty when unknown.
    var captureRef: String
    /// The capture shortened, then the scan's 6-character id.
    var label: String
    /// How far the refined pose moved, in centimetres and degrees; zero for
    /// a scan the refinement left out.
    var deltaCM: Double
    var deltaDeg: Double
    /// What Accept refinement records for it: REGISTERED or MANUALLY_PLACED;
    /// nil for a scan not in the refinement.
    var records: Gerardus_Align_V1alpha1_RegistrationState?
    var jointRank: UInt32?
    var state: RefinedScanState
    /// False for a scan in the site model the refinement did not refine.
    var refined: Bool
    /// Why the scan is left out or stale, when it is.
    var leftOutReason: String?

    var id: String { scanID }

    /// "REGISTERED", "MANUALLY_PLACED", or "not refined".
    var recordsText: String {
        switch records {
        case .registered: return "REGISTERED"
        case .manuallyPlaced: return "MANUALLY_PLACED"
        default: return "not refined"
        }
    }
}

enum RefinementSummary {
    /// Under both of these a refined pose reads as unchanged.
    static let unchangedM = 0.01
    static let unchangedDeg = 0.05

    /// The state for one scan. Left out wins over movement: a scan whose
    /// pose was solved from a stale start, or not in the joint solve, is
    /// flagged whatever it moved.
    static func state(
        deltaM: Double, deltaDeg: Double, passed: Bool, leftOut: Bool
    ) -> RefinedScanState {
        if leftOut { return .leftOut }
        if abs(deltaM) < unchangedM && abs(deltaDeg) < unchangedDeg { return .unchanged }
        return passed ? .registered : .manuallyPlaced
    }

    /// True when a refined scan's run passed every gate, so accepting it
    /// records REGISTERED. A run not read counts as not passed.
    static func passed(_ run: Gerardus_Align_V1alpha1_Registration?) -> Bool {
        guard let run, run.state == .candidate, !run.gates.isEmpty else { return false }
        return run.gates.allSatisfy(\.passed)
    }

    /// The last six characters of a scan id, as the pairs table shows them.
    static func shortID(_ scanID: String) -> String { String(scanID.suffix(6)) }

    /// A capture_ref short enough for a row: its start and end, with the
    /// middle elided past `maxLength` characters.
    static func shortCaptureRef(_ ref: String, maxLength: Int = 24) -> String {
        guard ref.count > maxLength, maxLength >= 5 else { return ref }
        let tail = (maxLength - 1) / 2
        let head = maxLength - 1 - tail
        return String(ref.prefix(head)) + "\u{2026}" + String(ref.suffix(tail))
    }

    /// "kirkham-0412#f1-200 · 29f479", or the id alone without a capture.
    static func label(scanID: String, captureRef: String) -> String {
        let ref = captureRef.trimmingCharacters(in: .whitespaces)
        let id = shortID(scanID)
        return ref.isEmpty ? id : "\(shortCaptureRef(ref)) · \(id)"
    }

    /// One row per refined scan, sorted by movement, largest first, then a
    /// row for each scan of the site model the refinement did not refine.
    static func rows(
        refinement: Gerardus_Align_V1alpha1_RefineSiteResult,
        runs: [String: Gerardus_Align_V1alpha1_Registration], captureRefs: [String: String],
        siteModelScanIDs: [String], stale: Set<String>
    ) -> [RefinementSummaryRow] {
        // Without pairs there is nothing to say a scan was left out of.
        let paired = Set(refinement.pairs.filter(\.used).flatMap { [$0.scanA, $0.scanB] })
        let judgePairs = !refinement.pairs.isEmpty
        var rows = refinement.scans.map { refined -> RefinementSummaryRow in
            let run = runs[refined.scanID]
            let passed = passed(run)
            var reason: String?
            if stale.contains(refined.scanID) {
                reason = "accepted again since this refinement ran"
            } else if judgePairs && !paired.contains(refined.scanID) {
                reason = "none of its pairs was used"
            }
            let ref = captureRefs[refined.scanID] ?? ""
            return RefinementSummaryRow(
                scanID: refined.scanID, captureRef: ref,
                label: label(scanID: refined.scanID, captureRef: ref),
                deltaCM: refined.deltaM * 100, deltaDeg: refined.deltaDeg,
                records: passed ? .registered : .manuallyPlaced,
                jointRank: refined.jointConstraintRank,
                state: state(
                    deltaM: refined.deltaM, deltaDeg: refined.deltaDeg, passed: passed,
                    leftOut: reason != nil), refined: true, leftOutReason: reason)
        }
        rows.sort { a, b in
            if a.deltaCM != b.deltaCM { return a.deltaCM > b.deltaCM }
            if a.deltaDeg != b.deltaDeg { return a.deltaDeg > b.deltaDeg }
            return a.scanID < b.scanID
        }
        let refinedIDs = Set(rows.map(\.scanID))
        for scanID in siteModelScanIDs where !refinedIDs.contains(scanID) {
            let ref = captureRefs[scanID] ?? ""
            rows.append(
                RefinementSummaryRow(
                    scanID: scanID, captureRef: ref, label: label(scanID: scanID, captureRef: ref),
                    deltaCM: 0, deltaDeg: 0, records: nil, jointRank: nil, state: .leftOut,
                    refined: false, leftOutReason: "not in this refinement"))
        }
        return rows
    }

    /// "7 scans · largest move 0.5 cm, 0.05° · all gates pass".
    static func verdict(_ rows: [RefinementSummaryRow]) -> String {
        let refined = rows.filter(\.refined)
        var parts = ["\(refined.count) \(refined.count == 1 ? "scan" : "scans")"]
        let maxCM = refined.map(\.deltaCM).max() ?? 0
        let maxDeg = refined.map(\.deltaDeg).max() ?? 0
        parts.append(String(format: "largest move %.1f cm, %.2f°", maxCM, maxDeg))
        let failing = refined.filter { $0.records != .registered }.count
        parts.append(
            failing == 0 ? "all gates pass" : "\(failing) \(failing == 1 ? "fails" : "fail") a gate"
        )
        let leftOut = rows.filter { $0.state == .leftOut }.count
        if leftOut > 0 { parts.append("\(leftOut) left out or stale") }
        return parts.joined(separator: " · ")
    }

    /// What Accept refinement does, said beside the button.
    static func acceptText(count: Int) -> String {
        let poses = count == 1 ? "the 1 refined pose" : "all \(count) refined poses"
        return
            "Accepts \(poses) together. Each was solved against the others, so they are accepted as one; to leave a scan out, refine without it."
    }

    /// Where the camera goes to frame a scan: centred on `sitePoint` (its
    /// sensor position in the site frame, drawn through `model`), `distance`
    /// back from it along the camera's current view direction.
    static func focus(
        sitePoint: SIMD3<Double>, model: simd_float4x4, cameraPosition: simd_float3,
        cameraTarget: simd_float3, distance: Float = focusDistanceM
    ) -> (position: simd_float3, target: simd_float3) {
        let p = model * SIMD4<Float>(Float(sitePoint.x), Float(sitePoint.y), Float(sitePoint.z), 1)
        let target = simd_float3(p.x, p.y, p.z)
        var back = cameraPosition - cameraTarget
        // A camera sitting on its target has no direction: look down and in.
        if simd_length(back) < 1e-4 { back = simd_float3(0, -1, 1) }
        return (target + simd_normalize(back) * distance, target)
    }

    /// Far enough back to see the sensor's arrow and the scan's points near it.
    static let focusDistanceM: Float = 25
}
