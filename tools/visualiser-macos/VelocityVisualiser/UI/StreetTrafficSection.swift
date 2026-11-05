// StreetTrafficSection.swift
// The street painter's Traffic section (lane painter plan, Item 1): which
// lane field is drawn under the marks, how, and Send lane field.

import SwiftUI

@available(macOS 15.0, *) struct StreetTrafficSection: View {
    @ObservedObject var painter: StreetPainterSession
    @ObservedObject var traffic: StreetTrafficLayer
    @ObservedObject var session: AlignmentSession
    @EnvironmentObject var appState: AppState

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Toggle("Traffic", isOn: $traffic.visible).toggleStyle(.switch).controlSize(.small)
                    .font(.subheadline.weight(.semibold))
                if traffic.isLoading { ProgressView().controlSize(.small) }
                Spacer()
                Button(action: { Task { await traffic.load(runID: appState.currentRunID) } }) {
                    Image(systemName: "arrow.clockwise")
                }.buttonStyle(.borderless).disabled(traffic.isLoading).help(
                    "Read this recording's field from velocity.report and the site's from Gerardus")
            }
            if traffic.visible {
                Picker("Source", selection: $traffic.choice) {
                    ForEach(choices) { Text($0.rawValue).tag($0) }
                }.pickerStyle(.segmented).labelsHidden()
                Text(provenance).font(.caption2).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
                Picker("Show", selection: $traffic.display.show) {
                    ForEach(LaneFieldDisplay.Show.allCases) { Text($0.rawValue).tag($0) }
                }.pickerStyle(.segmented).help(
                    "Centres: distinct tracks whose centre was in the cell. Extents: distinct tracks whose box covered it."
                )
                HStack {
                    ForEach(LaneFieldClassGroup.allCases) { group in
                        Toggle(label(group), isOn: classBinding(group)).toggleStyle(.checkbox)
                            .controlSize(.small)
                    }
                }
                HStack {
                    Text("Floor").font(.caption)
                    Stepper(
                        "\(traffic.display.floor) tracks", value: $traffic.display.floor,
                        in: 5...200
                    ).font(.caption).help(
                        "The field holds no cell under five distinct tracks; a higher floor hides thinner cells."
                    )
                }
                HStack {
                    Text("Opacity").font(.caption)
                    Slider(value: $traffic.display.opacity, in: 0.1...1).controlSize(.small)
                }
                if !legend.isEmpty {
                    Text(
                        "Vehicles by direction: "
                            + legend.map { "\($0.bound) \($0.cells)" }.joined(separator: " · ")
                    ).font(.caption2).foregroundStyle(.secondary)
                }
                if traffic.serverSupportsFields { sendRow }
            }
            if let message = traffic.message {
                Text(message).font(.caption).foregroundStyle(.secondary)
            }
            if let error = traffic.errorMessage {
                Text(error).font(.caption).foregroundStyle(.red).textSelection(.enabled)
            }
        }.task(id: loadKey) {
            // Read the open run's field and the site's whenever the run or
            // the site changes, not only on ↻: Send needs the run's.
            await traffic.load(runID: appState.currentRunID)
        }
    }

    /// The run and the site the fields are read for.
    private var loadKey: String { "\(appState.currentRunID ?? "")|\(session.site?.l16Token ?? "")" }

    /// The site's field and Send are hidden on a server that has none.
    private var choices: [StreetTrafficLayer.Choice] {
        traffic.serverSupportsFields ? StreetTrafficLayer.Choice.allCases : [.run, .live]
    }

    private var sendRow: some View {
        let blocker = traffic.sendBlocker(pendingSave: painter.hasPendingSave)
        return VStack(alignment: .leading, spacing: 2) {
            HStack {
                Button("Send lane field") {
                    Task {
                        await traffic.send(pendingSave: painter.hasPendingSave)
                        // Gerardus derives proposals when they are read: read
                        // them now, so a new field's lanes show at once.
                        await painter.proposals.load()
                    }
                }.disabled(blocker != nil).help(
                    blocker
                        ?? "Upload this run's three fields (the whole run and its halves) to Gerardus against the accepted pose"
                )
                if traffic.isSending { ProgressView().controlSize(.small) }
            }
            // Why it is off, in the pane: a tooltip on a disabled button is
            // easy to miss.
            if let blocker {
                Text(blocker).font(.caption2).foregroundStyle(.orange).fixedSize(
                    horizontal: false, vertical: true)
            }
        }
    }

    private var provenance: String {
        guard let grid = traffic.shown else {
            switch traffic.choice {
            case .site: return "Gerardus holds no field for this site yet."
            case .run:
                return appState.currentRunID == nil
                    ? "Open a recorded run to draw its field."
                    : "velocity.report has no field for this run."
            case .live:
                return "Live, this replay only: counted from the stream as it plays, never saved."
            }
        }
        switch grid.source {
        case .site:
            return
                "The site: \(grid.captures) capture\(grid.captures == 1 ? "" : "s") summed, \(grid.trackCount) tracks, \(grid.cellCount) cells, \(grid.fieldIDs.count) field\(grid.fieldIDs.count == 1 ? "" : "s")."
        case .run:
            return
                "This recording: run \(String((traffic.runID ?? "").prefix(8))), \(grid.trackCount) tracks, \(grid.cellCount) cells, placed by the pose drawn now."
        case .live:
            return
                "Live, this replay only: \(grid.trackCount) tracks, \(grid.cellCount) cells so far. Never saved."
        }
    }

    private var legend: [(bound: String, cells: Int)] {
        guard let grid = traffic.shown else { return [] }
        return Array(LaneFieldStyle.legend(grid, pose: session.pose).prefix(4))
    }

    private func label(_ group: LaneFieldClassGroup) -> String {
        guard let grid = traffic.shown, let cells = grid.groups[group] else { return group.label }
        return "\(group.label) \(cells.count)"
    }

    private func classBinding(_ group: LaneFieldClassGroup) -> Binding<Bool> {
        Binding(
            get: { traffic.display.classes.contains(group) },
            set: { on in
                if on {
                    traffic.display.classes.insert(group)
                } else {
                    traffic.display.classes.remove(group)
                }
            })
    }
}
