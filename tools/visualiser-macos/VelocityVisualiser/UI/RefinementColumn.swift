// RefinementColumn.swift
// Refine site in a column of its own, to the left of the alignment pane:
// the weighting and the Refine button, a summary with one row per scan
// (state colour, label, how far it moved, what accepting records, joint
// rank; its gates folded under it), the scan pairs folded away, and Accept
// refinement with what it does. It opens when a refinement runs or the
// operator opens it ("Refine site…" in the pane), and closing it keeps the
// refinement (AlignmentSession.refinementColumnOpen).

import SwiftUI

/// The alignment pane, with the refinement column to its left while open.
@available(macOS 15.0, *) struct AlignmentColumns: View {
    @ObservedObject var session: AlignmentSession

    var body: some View {
        HStack(spacing: 0) {
            if session.refinementColumnOpen {
                RefinementColumn(session: session).frame(width: RefinementColumn.width)
                Divider()
            }
            AlignmentPane(session: session).frame(width: AlignmentColumns.paneWidth)
        }
    }

    static let paneWidth: CGFloat = 340
}

@available(macOS 15.0, *) struct RefinementColumn: View {
    @ObservedObject var session: AlignmentSession

    /// Wide enough for a gate's name, value and threshold on one line.
    static let width: CGFloat = 460

    /// Rows whose gates are shown; all closed at first.
    @State private var expanded: Set<String> = []
    @State private var showPairs = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header.padding([.horizontal, .top]).padding(.bottom, 10)
            Divider()
            ScrollView(.vertical) { content }
        }.frame(maxHeight: .infinity).background(Color(nsColor: .controlBackgroundColor))
            .onKeyPress(.escape) { session.clearRefinedScanSelection() ? .handled : .ignored }
    }

    /// Everything below the header, as it scrolls.
    var content: some View {
        VStack(alignment: .leading, spacing: 14) {
            controls
            if session.isRefining { refiningView }
            if let error = session.errorMessage {
                Text(error).font(.caption).foregroundStyle(.red).textSelection(.enabled).fixedSize(
                    horizontal: false, vertical: true)
            }
            if let refinement = session.refinement {
                summary(refinement)
                if !refinement.pairs.isEmpty { pairsView(refinement) }
                Divider()
                acceptView(refinement)
            } else if !session.isRefining {
                Text(
                    "Re-solves the accepted poses at the site together: each scan's fit to 3DEP and the fits between overlapping scans. Each scan's refined pose is then gated as its own run, and Accept refinement records them all."
                ).font(.caption).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
                scansToRefine
            }
            Spacer()
        }.padding()
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            Text("Refine site").font(.headline)
            Text("\(session.siteModelScanIDs?.count ?? 0) accepted scans").font(.caption)
                .foregroundStyle(.secondary)
            Spacer()
            Button(action: { session.closeRefinementColumn() }) { Image(systemName: "xmark") }
                .buttonStyle(.borderless).help(
                    "Close the refinement column; the refinement is kept until the next one")
        }
    }

    private var controls: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Picker("Weighting", selection: $session.refineNEff) {
                    ForEach(AlignmentSession.refineNEffChoices, id: \.self) { n in
                        Text(n == 0 ? "N_eff 500 (default)" : "N_eff \(Int(n))").tag(n)
                    }
                }.fixedSize().help(
                    "Independent residuals each fit counts as when the graph weighs 3DEP against the scan pairs"
                )
                Spacer()
                Button(session.refinement == nil ? "Refine site" : "Refine again") {
                    Task { await session.refine() }
                }.disabled(!session.canRefine).help(
                    "Re-solve the ticked scans' accepted poses together. Needs two")
            }
            if !session.isRefining, let reason = session.refineBlocker {
                AlignmentBlockers(reasons: [reason])
            }
        }
    }

    private var refiningView: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 6) {
                ProgressView().controlSize(.small)
                Text(session.busy ?? "Refining the site").font(.callout)
            }
            if let progress = session.progress {
                if progress.fraction >= 0 { ProgressView(value: Double(min(progress.fraction, 1))) }
                Text("\(progress.stage): \(progress.message)").font(.caption).foregroundStyle(
                    .secondary)
            }
        }
    }

    /// Before any refinement: the site model's scans, each with the box
    /// that keeps it in the next Refine.
    @ViewBuilder private var scansToRefine: some View {
        if let ids = session.siteModelScanIDs, ids.count >= 2 {
            VStack(alignment: .leading, spacing: 3) {
                Text("Scans to refine").font(.subheadline.weight(.semibold))
                ForEach(ids, id: \.self) { id in
                    HStack(spacing: 6) {
                        includeBox(id)
                        Text(
                            RefinementSummary.label(
                                scanID: id, captureRef: session.scanCaptureRefs[id] ?? "")
                        ).font(.caption.monospaced()).lineLimit(1).truncationMode(.middle).help(id)
                    }
                }
            }
        }
    }

    /// The box that keeps a scan in the next Refine.
    private func includeBox(_ scanID: String) -> some View {
        Toggle(
            "",
            isOn: Binding(
                get: { session.isIncludedInRefine(scanID) },
                set: { session.setIncludedInRefine(scanID, $0) })
        ).toggleStyle(.checkbox).labelsHidden().help(
            "Include this scan in the next Refine. The refinement shown is accepted whole; untick a scan and refine again to leave it out"
        )
    }

    // MARK: Summary

    private func summary(_ refinement: Gerardus_Align_V1alpha1_RefineSiteResult) -> some View {
        let rows = session.refinementRows
        return VStack(alignment: .leading, spacing: 6) {
            Text(RefinementSummary.verdict(rows)).font(.callout.weight(.semibold)).fixedSize(
                horizontal: false, vertical: true)
            Text(solveLine(refinement)).font(.caption2.monospacedDigit()).foregroundStyle(
                .secondary
            ).textSelection(.enabled)
            VStack(alignment: .leading, spacing: 2) {
                summaryHeader
                ForEach(rows) { row in summaryRow(row) }
            }
            legend
            Text(
                "Click a scan to draw it bold in the scene; click it again or press Escape to clear. The box keeps it in the next Refine."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    /// "12 iterations · N_eff 500 · model sha256:1a2b3c4d5e6f".
    private func solveLine(_ refinement: Gerardus_Align_V1alpha1_RefineSiteResult) -> String {
        var parts = ["\(refinement.iterations) iterations", "N_eff \(Int(refinement.nEff))"]
        if !refinement.modelID.isEmpty {
            parts.append("model \(AlignmentFormat.shortID(refinement.modelID))")
        }
        return parts.joined(separator: " · ")
    }

    private enum Column {
        static let box: CGFloat = 16
        static let swatch: CGFloat = 10
        static let deltaCM: CGFloat = 50
        static let deltaDeg: CGFloat = 44
        static let records: CGFloat = 96
        static let rank: CGFloat = 24
        static let chevron: CGFloat = 12
    }

    private var summaryHeader: some View {
        HStack(spacing: 5) {
            Color.clear.frame(width: Column.box + Column.swatch + 5, height: 1)
            Text("scan").frame(maxWidth: .infinity, alignment: .leading)
            Text("Δ cm").frame(width: Column.deltaCM, alignment: .trailing)
            Text("Δ °").frame(width: Column.deltaDeg, alignment: .trailing)
            Text("records").frame(width: Column.records, alignment: .leading)
            Text("rank").frame(width: Column.rank, alignment: .trailing)
            Color.clear.frame(width: Column.chevron, height: 1)
        }.font(.caption2).foregroundStyle(.secondary).padding(.horizontal, 4)
    }

    private func summaryRow(_ row: RefinementSummaryRow) -> some View {
        let selected = session.selectedRefinedScanID == row.scanID
        let open = expanded.contains(row.scanID)
        return VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 5) {
                includeBox(row.scanID).frame(width: Column.box)
                Circle().fill(row.state.swatch).frame(width: Column.swatch, height: Column.swatch)
                    .help(row.state.title)
                HStack(spacing: 5) {
                    Text(row.label).font(.caption.monospaced()).fontWeight(
                        selected ? .bold : .regular
                    ).lineLimit(1).truncationMode(.middle).frame(
                        maxWidth: .infinity, alignment: .leading)
                    Text(row.refined ? String(format: "%.1f", row.deltaCM) : "—").frame(
                        width: Column.deltaCM, alignment: .trailing)
                    Text(row.refined ? String(format: "%.2f", row.deltaDeg) : "—").frame(
                        width: Column.deltaDeg, alignment: .trailing)
                    Text(row.recordsText).font(.caption2).lineLimit(1).frame(
                        width: Column.records, alignment: .leading)
                    Text(row.jointRank.map { "\($0)" } ?? "—").frame(
                        width: Column.rank, alignment: .trailing)
                }.font(.caption.monospacedDigit()).contentShape(Rectangle()).onTapGesture {
                    session.selectRefinedScan(row.scanID)
                }.help(
                    "\(row.captureRef.isEmpty ? "" : row.captureRef + " · ")\(row.scanID)\n\(row.state.title)\(row.leftOutReason.map { ": " + $0 } ?? ""). Click to select it."
                )
                Button(action: { toggleExpanded(row.scanID) }) {
                    Image(systemName: open ? "chevron.down" : "chevron.right").font(.caption2)
                }.buttonStyle(.borderless).frame(width: Column.chevron).help(
                    open ? "Hide its gates" : "Show its gates")
            }
            if open { scanDetail(row) }
        }.padding(.horizontal, 4).padding(.vertical, 3).background(
            selected ? Color.accentColor.opacity(0.12) : Color.clear
        ).overlay(
            RoundedRectangle(cornerRadius: 4).stroke(
                selected ? Color.accentColor : Color.clear, lineWidth: 2)
        ).clipShape(RoundedRectangle(cornerRadius: 4))
    }

    private func toggleExpanded(_ scanID: String) {
        if expanded.contains(scanID) { expanded.remove(scanID) } else { expanded.insert(scanID) }
    }

    /// A row's gate detail, under it while open.
    @ViewBuilder private func scanDetail(_ row: RefinementSummaryRow) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(row.scanID).font(.caption2.monospaced()).foregroundStyle(.secondary).textSelection(
                .enabled)
            if let reason = row.leftOutReason {
                Text("\(row.state.title): \(reason)").font(.caption2).foregroundStyle(
                    row.state.swatch)
            }
            if let refined = session.refinement?.scans.first(where: { $0.scanID == row.scanID }) {
                if session.refinedAccepted.contains(row.scanID) {
                    Label("recorded", systemImage: "checkmark").font(.caption2).foregroundStyle(
                        .green)
                }
                Text(
                    String(
                        format: "moved %.3f m, %.3f° · joint rank %d", refined.deltaM,
                        refined.deltaDeg, refined.jointConstraintRank)
                ).font(.caption2.monospacedDigit())
                if let run = session.refinedRuns[row.scanID] {
                    if run.gates.isEmpty {
                        Text("Its run reports no gates.").font(.caption2).foregroundStyle(
                            .secondary)
                    } else {
                        VStack(alignment: .leading, spacing: 2) {
                            ForEach(Array(run.gates.enumerated()), id: \.offset) { _, gate in
                                AlignmentGateRow(gate: gate)
                            }
                        }
                    }
                } else {
                    Text("Gates not read: Gerardus did not return this scan's run.").font(.caption2)
                        .foregroundStyle(.secondary)
                }
                Text(AlignmentFormat.quality(refined.afterQuality)).font(
                    .caption2.monospacedDigit()
                ).foregroundStyle(.secondary)
                if !refined.jointWeakDirections.isEmpty {
                    Text("weak: " + refined.jointWeakDirections.joined(separator: ", ")).font(
                        .caption2
                    ).foregroundStyle(.orange)
                }
            }
        }.padding(.leading, Column.box + Column.swatch + 10).padding(.bottom, 2)
    }

    /// The four states and their colours, as the scene draws them too.
    private var legend: some View {
        HStack(spacing: 10) {
            ForEach(RefinedScanState.allCases, id: \.self) { state in
                HStack(spacing: 3) {
                    Circle().fill(state.swatch).frame(width: 8, height: 8)
                    Text(state.title).font(.caption2).foregroundStyle(.secondary)
                }
            }
        }.help(
            "Each scan's sensor arrow and correction line are drawn in these colours while the refinement is shown"
        )
    }

    // MARK: Pairs

    private func pairsView(_ refinement: Gerardus_Align_V1alpha1_RefineSiteResult) -> some View {
        let used = refinement.pairs.filter(\.used).count
        return DisclosureGroup(isExpanded: $showPairs) {
            Grid(alignment: .leading, horizontalSpacing: 10, verticalSpacing: 3) {
                GridRow {
                    Text("")
                    Text("scans")
                    Text("p75 before → after")
                    Text("matched").gridColumnAlignment(.trailing)
                }.font(.caption2).foregroundStyle(.secondary)
                ForEach(Array(refinement.pairs.enumerated()), id: \.offset) { _, pair in
                    GridRow {
                        Image(systemName: pair.used ? "checkmark.circle" : "minus.circle")
                            .foregroundStyle(pair.used ? Color.green : .secondary)
                        Text(
                            "\(RefinementSummary.shortID(pair.scanA)) ← \(RefinementSummary.shortID(pair.scanB))"
                        ).font(.caption.monospaced())
                        Text(String(format: "%.3f → %.3f m", pair.p75BeforeM, pair.p75AfterM)).font(
                            .caption.monospacedDigit())
                        Text("\(pair.matched)").font(.caption.monospacedDigit())
                    }
                    if !pair.used {
                        GridRow {
                            Color.clear.gridCellUnsizedAxes([.horizontal, .vertical])
                            Text("not used: \(pair.reason)").font(.caption2).foregroundStyle(
                                .secondary
                            ).gridCellColumns(3).fixedSize(horizontal: false, vertical: true)
                        }
                    }
                }
            }.padding(.top, 4)
        } label: {
            Text("Pairs (used \(used) of \(refinement.pairs.count))").font(
                .subheadline.weight(.semibold))
        }
    }

    // MARK: Accept

    private func acceptView(_ refinement: Gerardus_Align_V1alpha1_RefineSiteResult) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(RefinementSummary.acceptText(count: refinement.scans.count)).font(.caption)
                .fixedSize(horizontal: false, vertical: true)
            Text("Recorded under the Reviewer and Note in the alignment pane.").font(.caption2)
                .foregroundStyle(.secondary)
            Button("Accept refinement") { Task { await session.acceptRefinement() } }.buttonStyle(
                .borderedProminent
            ).disabled(!session.canAcceptRefinement).help(
                "Record every scan's refined pose together: registered where its run passed every gate, else manually placed"
            )
            AlignmentBlockers(reasons: session.acceptRefinementBlockers)
        }
    }
}

extension RefinedScanState {
    /// The state's colour for SwiftUI, the same as the scene draws.
    var swatch: Color {
        let c = colour
        return Color(red: Double(c.x), green: Double(c.y), blue: Double(c.z))
    }
}
