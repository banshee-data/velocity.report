import SwiftUI

/// Manual review deliberately leaves playback/source agreement unverified.
/// A matching timestamp, chosen site or tracker ID is not a physical reference.
struct FollowingAuditEditor: View {
    @Environment(\.dismiss) private var dismiss
    @State var record: FollowingAudit.Record
    @State private var endTime = ""
    @State private var error: String?
    var candidateTrackIDs: [String] = []
    var timeOriginNanos: Int64? = nil
    var initialEndText: String? = nil
    var onDraftChange: (FollowingAudit.Record) -> Void = { _ in }
    var onEndTextChange: (String) -> Void = { _ in }
    var onCancel: () -> Void = {}
    let save: (FollowingAudit.Record) throws -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Review follower period").font(.headline)
            Text("Follower \(record.followerTrackID.suffix(8)) · \(startLabel)").textSelection(
                .enabled)
            Form {
                TextField("Reviewer label", text: $record.reviewer)
                TextField(
                    timeOriginNanos == nil
                        ? "Period end (exclusive Unix ns)" : "Period end (seconds into recording)",
                    text: $endTime)
                observation("Physical identity continuity", selection: $record.identityContinuity)
                observation("Leader / common-path evidence", selection: $record.leaderAndPath)
                if !candidateTrackIDs.isEmpty {
                    Picker(
                        "Observed leader",
                        selection: Binding(
                            get: { record.proposedLeaderTrackID ?? "" },
                            set: { record.proposedLeaderTrackID = $0.isEmpty ? nil : $0 })
                    ) {
                        Text("Unknown / no alternative asserted").tag("")
                        ForEach(candidateTrackIDs, id: \.self) { id in
                            Text(String(id.suffix(8))).tag(id)
                        }
                    }
                }
                observation("Visible front / rear endpoints", selection: $record.visibleEndpoints)
                observation("Evidence for both projected speeds", selection: $record.speedEvidence)
                Picker("Review conclusion", selection: $record.conclusion) {
                    ForEach(FollowingAudit.Conclusion.allCases, id: \.self) { value in
                        Text(value.title).tag(value)
                    }
                }
            }
            Text(
                "Record ambiguity, adjacent/crossing traffic, lost returns, fragmentation, support changes and disagreement. Include negative cases; unknown is an observation, not a pass."
            ).font(.caption).foregroundStyle(.secondary)
            TextEditor(text: $record.notes).frame(minHeight: 120).border(.secondary.opacity(0.3))
                .accessibilityLabel("Audit observations and uncertainty")
            Text(
                "This review retains the displayed frame's context. Record what the scans establish; matching a recording does not verify physical identity, endpoints or speeds."
            ).font(.caption).foregroundStyle(.orange)
            if let error { Text(error).foregroundStyle(.red) }
            HStack {
                Spacer()
                Button("Cancel") {
                    onCancel()
                    dismiss()
                }.keyboardShortcut(.cancelAction)
                Button("Add review record") {
                    guard let end = decodedEnd else {
                        error = "Enter a valid period end within the recording."
                        return
                    }
                    record.endExclusiveNanos = end
                    record.savedAt = Date()
                    do { try save(record) } catch { self.error = error.localizedDescription }
                }.keyboardShortcut(.defaultAction)
            }
        }.onChange(of: record) { _, value in onDraftChange(value) }.onChange(of: endTime) {
            _, value in onEndTextChange(value)
        }.padding(20).frame(width: 620).onAppear {
            if let initialEndText {
                endTime = initialEndText
            } else if let origin = timeOriginNanos {
                endTime = String(format: "%.9f", Double(record.endExclusiveNanos - origin) / 1e9)
            } else {
                endTime = String(record.endExclusiveNanos)
            }
        }
    }

    private var startLabel: String {
        if let origin = timeOriginNanos {
            return String(format: "%.3f seconds", Double(record.startNanos - origin) / 1e9)
        }
        return "\(record.startNanos) ns"
    }
    private var decodedEnd: Int64? {
        guard let origin = timeOriginNanos else { return Int64(endTime) }
        guard let seconds = Double(endTime), seconds.isFinite, seconds >= 0,
            seconds * 1e9 < Double(Int64.max - origin)
        else { return nil }
        let end = origin.addingReportingOverflow(Int64((seconds * 1e9).rounded()))
        return end.overflow ? nil : end.partialValue
    }

    private func observation(
        _ title: String, selection: Binding<FollowingAudit.Observation>
    ) -> some View {
        Picker(title, selection: selection) {
            ForEach(FollowingAudit.Observation.allCases, id: \.self) { value in
                Text(value.title).tag(value)
            }
        }
    }
}
