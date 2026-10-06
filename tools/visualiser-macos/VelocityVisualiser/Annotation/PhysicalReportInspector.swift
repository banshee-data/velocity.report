// PhysicalReportInspector.swift
// The report open in Compare mode: which arm is shown, whether the report
// belongs to this pack, and whether the reference revision it scored is still
// retained, byte for byte.
//
// Compare is read-only, but looking is not free. Once an estimate for an
// object has been on screen, an edit to that object's reference is tracker
// assisted, whatever mode it is made in afterwards; see
// PhysicalDraft.applyExposure. A held-out pack does not open a report at all.

import Combine
import CryptoKit
import Foundation

@MainActor final class PhysicalReportInspector: ObservableObject {
    @Published private(set) var report: PhysicalComparisonReport?
    @Published private(set) var reportName: String?
    @Published private(set) var reportDigest: String?
    @Published private(set) var error: String?
    /// Which arm is shown: 0 for A, 1 for B.
    @Published var arm = 0
    /// The retained revision the report scored, when it still reads with the
    /// same bytes. Nil while unchecked or when it does not.
    @Published private(set) var retained: PhysicalPackState?
    /// Why the scored revision's provenance cannot be shown, if it cannot.
    @Published private(set) var provenanceNote: String?

    /// Bumped by every load, so a late answer cannot fill in a newer report.
    private var generation = 0

    var armIdentity: ReportArmIdentity? {
        guard let report, report.arms.indices.contains(arm) else { return nil }
        return report.arms[arm].arm
    }

    func close() {
        generation &+= 1
        report = nil
        reportName = nil
        reportDigest = nil
        retained = nil
        provenanceNote = nil
        error = nil
    }

    /// Opens a report for this pack. A report for another pack, one with no
    /// physical section, or any report on a held-out pack is refused.
    func open(url: URL, packDigest: String, role: String?, physical: PhysicalReferenceSession) async
    {
        generation &+= 1
        let asked = generation
        report = nil
        reportDigest = nil
        retained = nil
        provenanceNote = nil
        error = nil
        guard role != "held_out" else {
            error =
                "This pack is held out. Estimates are not compared on it until the held-out "
                + "evaluation contract allows it."
            return
        }
        let decoded: Result<(PhysicalComparisonReport, String), Error> = await Task.detached {
            Result {
                let bytes = try Data(contentsOf: url)
                return (
                    try PhysicalComparisonReport.decode(bytes, packDigest: packDigest),
                    "sha256:" + SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined()
                )
            }
        }.value
        guard asked == generation else { return }
        switch decoded {
        case .failure(let failure):
            error = (failure as? LocalizedError)?.errorDescription ?? String(describing: failure)
            return
        case .success(let (loaded, digest)):
            if loaded.reference.splitRole == "held_out" {
                error = "The report scored a held-out split; it is not opened here."
                return
            }
            report = loaded
            reportDigest = digest
            reportName = url.lastPathComponent
            arm = 0
        }
        await checkProvenance(physical: physical, asked: asked)
    }

    /// Reads the revision the report scored and holds it to the report's
    /// digest. Never fills provenance in from the current head.
    private func checkProvenance(physical: PhysicalReferenceSession, asked: Int) async {
        guard let reference = report?.reference else { return }
        do {
            let state = try await physical.loadRetained(revision: reference.physicalRevision)
            guard asked == generation else { return }
            if state.digest == reference.physicalRevisionDigest {
                retained = state
            } else {
                provenanceNote =
                    "Revision \(reference.physicalRevision) now reads with different bytes than the "
                    + "report scored; its evidence and review details are not shown."
            }
        } catch {
            guard asked == generation else { return }
            provenanceNote =
                "Revision \(reference.physicalRevision) is not available (\(error.localizedDescription)); "
                + "the report's own comparison is shown, without evidence and review details."
        }
    }
}

extension AnnotationSession {
    /// Whether Compare may show estimates on this pack.
    var comparisonAllowed: Bool { packRole != "held_out" }

    /// Records that the estimates at this sample are on screen.
    func exposeComparedObjects() {
        guard workMode == .compare, let report = reportInspector.report,
            let identity = reportInspector.armIdentity, let sample = currentSample
        else { return }
        let seen = report.predictedObjects(arm: reportInspector.arm, sampleID: sample.sampleID)
        guard !seen.isEmpty else { return }
        startPhysicalIfNeeded()
        physical.expose(objectIDs: seen, source: identity.source)
    }
}
