// RefinementColumn.swift
// Refine site in a column of its own, to the left of the alignment pane:
// the weighting and the Refine button, each refined scan with its gates one
// row apiece, the scan pairs, and Accept refinement. It opens when a
// refinement runs or the operator opens it ("Refine site…" in the pane), and
// closing it keeps the refinement (AlignmentSession.refinementColumnOpen).

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

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header.padding([.horizontal, .top]).padding(.bottom, 10)
            Divider()
            ScrollView(.vertical) { content }
        }.frame(maxHeight: .infinity).background(Color(nsColor: .controlBackgroundColor))
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
                scansView(refinement)
                if !refinement.pairs.isEmpty { pairsView(refinement) }
                Divider()
                acceptView
            } else if !session.isRefining {
                Text(
                    "Re-solves every accepted pose at the site together: each scan's fit to 3DEP and the fits between overlapping scans. Each scan's refined pose is then gated as its own run, and Accept refinement records them all."
                ).font(.caption).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
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
                    "Re-solve every accepted pose at the site together. Needs two accepted scans")
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

    private func summary(_ refinement: Gerardus_Align_V1alpha1_RefineSiteResult) -> some View {
        let used = refinement.pairs.filter(\.used).count
        return VStack(alignment: .leading, spacing: 2) {
            AlignmentRow("Scans", "\(refinement.scans.count)")
            AlignmentRow("Pairs used", "\(used) of \(refinement.pairs.count)")
            AlignmentRow("Iterations", "\(refinement.iterations)")
            AlignmentRow("N_eff", "\(Int(refinement.nEff))")
            if !refinement.modelID.isEmpty {
                AlignmentRow("Model", AlignmentFormat.shortID(refinement.modelID))
            }
        }
    }

    // MARK: Scans

    private func scansView(_ refinement: Gerardus_Align_V1alpha1_RefineSiteResult) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Refined scans").font(.subheadline.weight(.semibold))
            ForEach(refinement.scans, id: \.scanID) { refined in scanCard(refined) }
        }
    }

    private func scanCard(_ refined: Gerardus_Align_V1alpha1_RefinedScan) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .firstTextBaseline) {
                Text(String(refined.scanID.suffix(16))).font(.caption.monospaced()).textSelection(
                    .enabled
                ).help(refined.scanID)
                Spacer()
                scanState(refined)
            }
            Text(
                String(
                    format: "moved %.3f m, %.2f° · joint rank %d", refined.deltaM, refined.deltaDeg,
                    refined.jointConstraintRank)
            ).font(.caption.monospacedDigit())
            if let run = session.refinedRuns[refined.scanID] {
                if run.gates.isEmpty {
                    Text("Its run reports no gates.").font(.caption2).foregroundStyle(.secondary)
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
            Text(AlignmentFormat.quality(refined.afterQuality)).font(.caption2.monospacedDigit())
                .foregroundStyle(.secondary)
            if !refined.jointWeakDirections.isEmpty {
                Text("weak: " + refined.jointWeakDirections.joined(separator: ", ")).font(.caption2)
                    .foregroundStyle(.orange)
            }
        }.padding(6).frame(maxWidth: .infinity, alignment: .leading).background(
            Color.secondary.opacity(0.06)
        ).clipShape(RoundedRectangle(cornerRadius: 4))
    }

    @ViewBuilder private func scanState(_ refined: Gerardus_Align_V1alpha1_RefinedScan) -> some View
    {
        if session.refinedAccepted.contains(refined.scanID) {
            Label("recorded", systemImage: "checkmark").font(.caption2).foregroundStyle(.green)
        } else {
            Text(
                session.refinedPassed(refined) ? "would be registered" : "would be manually placed"
            ).font(.caption2).foregroundStyle(.secondary)
        }
    }

    // MARK: Pairs

    private func pairsView(_ refinement: Gerardus_Align_V1alpha1_RefineSiteResult) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Pairs").font(.subheadline.weight(.semibold))
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
                        Text("\(pair.scanA.suffix(6)) ← \(pair.scanB.suffix(6))").font(
                            .caption.monospaced())
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
            }
        }
    }

    // MARK: Accept

    private var acceptView: some View {
        VStack(alignment: .leading, spacing: 6) {
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
