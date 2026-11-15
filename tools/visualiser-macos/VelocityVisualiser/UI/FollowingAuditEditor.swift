import SwiftUI

/// Manual review deliberately leaves playback/source agreement unverified.
/// A matching timestamp, chosen site or tracker ID is not a physical reference.
struct FollowingAuditEditor: View {
    @Environment(\.dismiss) private var dismiss
    @State var record: FollowingAudit.Record
    @State private var endTime = ""
    @State private var error: String?
    let save: (FollowingAudit.Record) throws -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Following / experimental DRAC audit").font(.headline)
            Text("Follower \(record.followerTrackID) · starts \(record.startNanos) ns")
                .textSelection(.enabled)
            Form {
                TextField("Reviewer label", text: $record.reviewer)
                TextField("Period end (exclusive Unix ns)", text: $endTime)
                observation("Physical identity continuity", selection: $record.identityContinuity)
                observation("Leader / common-path evidence", selection: $record.leaderAndPath)
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
                "Playback at \(record.playback.captureNanos) ns. Source/version agreement is unverified. Describe the recording and evidence you checked; this audit does not automatically seek, join tracks or qualify DRAC."
            ).font(.caption).foregroundStyle(.orange)
            if let error { Text(error).foregroundStyle(.red) }
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Add review record") {
                    guard let end = Int64(endTime) else {
                        error = "Enter an exact Int64 Unix-nanosecond period end."
                        return
                    }
                    record.endExclusiveNanos = end
                    record.savedAt = Date()
                    do { try save(record) } catch { self.error = error.localizedDescription }
                }.keyboardShortcut(.defaultAction)
            }
        }.padding(20).frame(width: 620).onAppear { endTime = String(record.endExclusiveNanos) }
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
