// StreetLanesSection.swift
// The street painter's Proposals section (lane painter plan, Item 2) and the
// selected travel lane's direction and limit (Item 3).

import SwiftUI

@available(macOS 15.0, *) struct StreetProposalsSection: View {
    @ObservedObject var painter: StreetPainterSession
    @ObservedObject var proposals: StreetProposals
    @AppStorage(StreetPainterSession.authorKey) private var author = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Toggle("Proposals", isOn: $proposals.visible).toggleStyle(.switch).controlSize(
                    .small
                ).font(.subheadline.weight(.semibold))
                if proposals.isLoading || proposals.isReviewing {
                    ProgressView().controlSize(.small)
                }
                Spacer()
                Button(action: { Task { await proposals.load() } }) {
                    Image(systemName: "arrow.clockwise")
                }.buttonStyle(.borderless).disabled(proposals.isLoading).help(
                    "Read Gerardus's proposed travel lanes again")
            }
            if proposals.proposals.isEmpty {
                Text("Gerardus proposes no travel lanes here yet: send lane fields first.").font(
                    .caption2
                ).foregroundStyle(.secondary)
            } else if proposals.visible {
                Button("Approve all high (\(painter.approvableHigh.count))") {
                    painter.approveAllHigh()
                }.disabled(painter.approvableHigh.isEmpty).help(
                    "Add a travel lane along every high proposal not yet adopted or rejected, as one change Undo takes back"
                )
                ForEach(proposals.grouped, id: \.bound) { group in
                    Text(group.bound.isEmpty ? "No bound" : "\(group.bound)-bound").font(
                        .caption.weight(.semibold))
                    ForEach(group.proposals) { proposal in row(proposal) }
                }
            }
            if proposals.pending != nil {
                HStack {
                    Text("A review's outcome is unknown.").font(.caption2).foregroundStyle(.orange)
                    Button("Read again") { Task { await proposals.discardPending() } }.controlSize(
                        .small)
                }
            }
            if let error = proposals.errorMessage {
                Text(error).font(.caption).foregroundStyle(.red).textSelection(.enabled)
            }
        }
    }

    private func row(_ proposal: LaneProposalView) -> some View {
        let selected = proposals.selectedID == proposal.featureID
        return VStack(alignment: .leading, spacing: 3) {
            Button(action: { proposals.selectedID = selected ? nil : proposal.featureID }) {
                HStack {
                    Image(systemName: proposal.isHigh ? "line.diagonal" : "ellipsis")
                    Text(proposal.summary).font(.caption).strikethrough(proposal.rejected)
                        .foregroundStyle(proposal.isAdopted ? .secondary : .primary)
                    Spacer()
                    if proposal.isAdopted {
                        Text("adopted").font(.caption2).foregroundStyle(.secondary)
                    }
                }
            }.buttonStyle(.plain)
            if selected {
                HStack {
                    Button("Approve") { painter.approveProposal(proposal.featureID) }.disabled(
                        proposal.rejected
                    ).controlSize(.small)
                    if proposal.rejected {
                        Button("Reopen") {
                            Task {
                                await proposals.review(
                                    proposal.featureID, .reopened, author: author)
                            }
                        }.controlSize(.small)
                    } else {
                        Button("Reject") {
                            Task {
                                await proposals.review(
                                    proposal.featureID, .rejected, author: author)
                            }
                        }.controlSize(.small).disabled(proposals.isReviewing)
                    }
                }
                Text(
                    "\(proposal.profiles.count) lateral profiles drawn across it; width \(proposal.widthM.map { StreetUseTags.format($0) } ?? "?") m."
                ).font(.caption2).foregroundStyle(.secondary)
            }
        }
    }
}

/// The selected travel lane's direction and limit.
@available(macOS 15.0, *) struct LaneLimitSection: View {
    @ObservedObject var painter: StreetPainterSession
    let lane: StreetMark
    @State private var value = ""
    @State private var directionName = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            let bound = LaneLimits.bound(of: lane)
            Text("Direction").font(.caption.weight(.semibold))
            HStack {
                Text(bound.isEmpty ? "No bound" : "\(bound)-bound").font(.caption)
                if let street = lane.tags["street"] {
                    Text(LaneLimits.titleCase(street)).font(.caption).foregroundStyle(.secondary)
                }
            }
            HStack {
                TextField("Direction name", text: $directionName).textFieldStyle(.roundedBorder)
                    .font(.caption).onSubmit { painter.setDirectionName(directionName) }
                Button(
                    "Name \(painter.selectedDirection.count) lane\(painter.selectedDirection.count == 1 ? "" : "s")"
                ) { painter.setDirectionName(directionName) }.controlSize(.small)
            }
            Text("Limit").font(.caption.weight(.semibold))
            HStack {
                TextField("25", text: $value).textFieldStyle(.roundedBorder).frame(width: 50)
                    .onSubmit { painter.setSelectedLimit([LaneLimits.speedLimit: value]) }
                Picker("", selection: tag(LaneLimits.unit, default: "mph")) {
                    ForEach(LaneLimits.units, id: \.self) { Text($0).tag($0) }
                }.labelsHidden().frame(width: 80)
                Picker("", selection: tag(LaneLimits.source, default: "")) {
                    Text("Source").tag("")
                    ForEach(LaneLimits.sources, id: \.self) { Text($0).tag($0) }
                }.labelsHidden()
            }
            field("Jurisdiction", LaneLimits.jurisdiction, prompt: "US-CA")
            field("Effective from", LaneLimits.effectiveFrom, prompt: "YYYY-MM-DD")
            field("Effective to", LaneLimits.effectiveTo, prompt: "optional")
            field("Note", LaneLimits.note, prompt: "school zone 15 mph when children are present")
            if lane.tags[LaneLimits.source] == "statutory" {
                Text(
                    "A statutory default, not read from a sign: change the source to sign once it is read on site."
                ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
            }
            ForEach(painter.limitWarnings.byKey[lane.key] ?? [], id: \.self) {
                Text($0).font(.caption2).foregroundStyle(.orange)
            }
        }.onAppear(perform: sync).onChange(of: lane) { _, _ in sync() }
    }

    private func sync() {
        value = lane.tags[LaneLimits.speedLimit] ?? ""
        let bound = LaneLimits.bound(of: lane)
        directionName =
            lane.tags[LaneLimits.directionName]
            ?? LaneLimits.defaultDirectionName(street: lane.tags["street"] ?? "", bound: bound)
    }

    private func tag(_ key: String, default fallback: String) -> Binding<String> {
        Binding(get: { lane.tags[key] ?? fallback }, set: { painter.setSelectedLimit([key: $0]) })
    }

    private func field(_ label: String, _ key: String, prompt: String) -> some View {
        HStack {
            Text(label).font(.caption).frame(width: 90, alignment: .leading)
            TextField(prompt, text: tag(key, default: "")).textFieldStyle(.roundedBorder).font(
                .caption)
        }
    }
}
