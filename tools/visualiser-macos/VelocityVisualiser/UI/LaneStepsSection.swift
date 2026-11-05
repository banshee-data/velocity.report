// LaneStepsSection.swift
// The street painter's steps from a recording to lanes (LaneSteps): one row
// each, done, next or waiting, with what to do next spelt out and the action
// that does it where the painter holds one.

import SwiftUI

@available(macOS 15.0, *) struct LaneStepsSection: View {
    @ObservedObject var painter: StreetPainterSession
    @ObservedObject var traffic: StreetTrafficLayer
    @ObservedObject var proposals: StreetProposals
    @ObservedObject var session: AlignmentSession
    @EnvironmentObject var appState: AppState
    @State private var replayRunID: String?
    @State private var opening = false

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("From traffic to lanes").font(.subheadline.weight(.semibold))
            ForEach(steps) { step in
                VStack(alignment: .leading, spacing: 2) {
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Image(systemName: symbol(step.state)).foregroundStyle(colour(step.state))
                            .font(.caption)
                        Text(step.title).font(
                            .caption.weight(step.state == .next ? .semibold : .regular)
                        ).foregroundStyle(step.state == .waiting ? .secondary : .primary)
                    }.help(step.detail)
                    if step.state != .waiting {
                        Text(step.detail).font(.caption2).foregroundStyle(.secondary).fixedSize(
                            horizontal: false, vertical: true
                        ).padding(.leading, 18)
                    }
                    if step.state == .next { action(step.kind).padding(.leading, 18) }
                }
            }
        }.task(id: replayKey) { replayRunID = await appState.runOfCurrentReplay() }
    }

    private var steps: [LaneStep] {
        let live = proposals.proposals.filter { !$0.rejected }
        return LaneSteps.steps(
            LaneStepsInput(
                serverSupportsFields: traffic.serverSupportsFields,
                poseAccepted: session.acceptedRecord != nil, runID: appState.currentRunID,
                replayRunID: replayRunID,
                runFieldTracks: traffic.runID == appState.currentRunID
                    ? traffic.runField.map { $0.grid.trackCount } : nil,
                siteCaptures: traffic.siteField?.captures ?? 0,
                siteBusiestVehicleCell: LaneSteps.busiestVehicleCell(traffic.siteField),
                proposalsHigh: live.filter(\.isHigh).count,
                proposalsMedium: live.filter { !$0.isHigh }.count,
                savedLanes: painter.edit.base.filter { $0.kind == .travelLane }.count,
                unsavedChanges: painter.edit.isDirty, snapshotUnsent: painter.snapshotUnsent))
    }

    /// Asked again when the run or the server's source changes.
    private var replayKey: String {
        "\(appState.currentRunID ?? "")|\(appState.sourceMode.rawValue)"
    }

    @ViewBuilder private func action(_ kind: LaneStep.Kind) -> some View {
        switch kind {
        case .run:
            if let replayRunID {
                Button("Open run \(String(replayRunID.prefix(8)))") {
                    opening = true
                    Task {
                        await appState.openRun(replayRunID)
                        opening = false
                    }
                }.controlSize(.small).disabled(opening)
            } else {
                Button("Runs…") { appState.showRunBrowser = true }.controlSize(.small)
            }
        case .field:
            if appState.currentRunID != nil {
                Button("Read again") { Task { await traffic.load(runID: appState.currentRunID) } }
                    .controlSize(.small).disabled(traffic.isLoading)
            }
        case .proposals:
            Button("Read proposals again") { Task { await proposals.load() } }.controlSize(.small)
                .disabled(proposals.isLoading)
        case .pose, .sent, .saved, .snapshot: EmptyView()
        }
    }

    private func symbol(_ state: LaneStep.State) -> String {
        switch state {
        case .done: return "checkmark.circle.fill"
        case .next: return "arrow.right.circle.fill"
        case .waiting: return "circle"
        }
    }

    private func colour(_ state: LaneStep.State) -> Color {
        switch state {
        case .done: return .green
        case .next: return .accentColor
        case .waiting: return .secondary
        }
    }
}
