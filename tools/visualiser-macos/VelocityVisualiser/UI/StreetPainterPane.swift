// StreetPainterPane.swift
// The alignment pane's paint mode: the street painter's tools, the selected
// mark, the layers drawn under the marks, and Save. Shown in place of the
// alignment sections while painting (StreetPainterSession).

import SwiftUI

/// The toolbar's switch into paint mode. Draws nothing unless Gerardus answered.
@available(macOS 15.0, *) struct StreetPaintToolbarButton: View {
    @ObservedObject var painter: StreetPainterSession
    @ObservedObject var session: AlignmentSession
    @EnvironmentObject var appState: AppState

    var body: some View {
        if session.isAvailable {
            Toggle(isOn: painting) { Label("Paint streets", systemImage: "paintbrush.pointed") }
                .toggleStyle(.button).disabled(!painter.isPainting && !painter.canPaint).help(
                    painter.paintBlocker
                        ?? "Mark the site's surfaces, lanes, spaces and driveways from above")
        }
    }

    private var painting: Binding<Bool> {
        Binding(get: { painter.isPainting }, set: { _ in appState.toggleStreetPainting() })
    }
}

@available(macOS 15.0, *) struct StreetPainterPane: View {
    @ObservedObject var painter: StreetPainterSession
    @ObservedObject var session: AlignmentSession
    @AppStorage(StreetPainterSession.authorKey) private var author = ""
    @State private var widthText = ""

    var body: some View {
        ScrollView(.vertical) {
            VStack(alignment: .leading, spacing: 14) {
                statusSection
                toolSection
                if painter.selected != nil {
                    Divider()
                    selectedSection
                }
                Divider()
                saveSection
                Divider()
                marksSection
                Divider()
                layersSection
                Spacer()
            }.padding()
        }
    }

    // MARK: Status

    private var statusSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text("Street use").font(.subheadline.weight(.semibold))
                if painter.isLoading { ProgressView().controlSize(.small) }
                Spacer()
                Button(action: { Task { await painter.load() } }) {
                    Image(systemName: "arrow.clockwise")
                }.buttonStyle(.borderless).disabled(painter.isLoading || painter.edit.isDirty).help(
                    painter.edit.isDirty
                        ? "Save or revert the changes first" : "Read this site's marks again")
            }
            Text(statusLine).font(.caption).foregroundStyle(.secondary)
            Text(
                "Top-down over the placed reference, north up. Drag to pan, scroll to zoom; the view does not turn while painting."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            if painter.skipped > 0 {
                Text(
                    "\(painter.skipped) feature\(painter.skipped == 1 ? "" : "s") could not be read and \(painter.skipped == 1 ? "is" : "are") not drawn."
                ).font(.caption2).foregroundStyle(.orange)
            }
            if let error = painter.errorMessage {
                if painter.hasPendingSave {
                    Button("Discard draft and reload") { Task { await painter.load() } }.disabled(
                        painter.isLoading || painter.isSaving)
                }
                Text(error).font(.caption).foregroundStyle(.red).textSelection(.enabled)
            }
        }
    }

    private var statusLine: String {
        let n = painter.edit.marks.count
        let pending = painter.edit.pendingEdits.count
        var line = "\(n) mark\(n == 1 ? "" : "s")"
        if pending > 0 { line += " · \(pending) unsaved change\(pending == 1 ? "" : "s")" }
        return line
    }

    // MARK: Tools

    private var toolSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Picker("Tool", selection: $painter.tool) {
                ForEach(StreetPaintTool.allCases) { tool in
                    Image(systemName: tool.symbol).help(tool.label).tag(tool)
                }
            }.pickerStyle(.segmented).labelsHidden()
            Text(painter.tool.label).font(.callout.weight(.medium))
            Text(painter.tool.hint).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            toolFields
            if !painter.drawing.isEmpty {
                HStack {
                    Text(
                        "\(painter.drawing.count) vertex\(painter.drawing.count == 1 ? "" : "es") placed"
                    ).font(.caption)
                    Spacer()
                    if !painter.tool.finishesAtTwo {
                        Button("Finish") { painter.finishDrawing() }.controlSize(.small)
                    }
                    Button("Cancel") { painter.cancelDrawing() }.controlSize(.small)
                }
            }
            HStack {
                Button(action: painter.undo) { Label("Undo", systemImage: "arrow.uturn.backward") }
                    .disabled(!painter.canUndo)
                Button(action: painter.redo) { Label("Redo", systemImage: "arrow.uturn.forward") }
                    .disabled(!painter.canRedo)
                Spacer()
                Button("Revert") { painter.revert() }.disabled(!painter.edit.isDirty).help(
                    "Drop every change since the marks were read")
            }.controlSize(.small)
        }
    }

    @ViewBuilder private var toolFields: some View {
        switch painter.tool {
        case .select: EmptyView()
        case .surface:
            Picker("Surface", selection: $painter.fields.surface) {
                ForEach(StreetUseTags.surfaces, id: \.self) { Text($0).tag($0) }
            }
        case .lane:
            Picker("Lane type", selection: $painter.fields.laneType) {
                ForEach(StreetUseTags.laneTypes, id: \.self) { Text($0).tag($0) }
            }
            HStack {
                Text("Width")
                TextField(
                    "m", value: $painter.fields.widthM,
                    format: .number.precision(.fractionLength(0...2))
                ).frame(width: 60).multilineTextAlignment(.trailing)
                Text("m").foregroundStyle(.secondary)
            }
        case .space, .row:
            Picker("Space type", selection: $painter.fields.spaceType) {
                ForEach(StreetUseTags.spaceTypes, id: \.self) { Text($0).tag($0) }
            }
            Stepper("Spaces: \(painter.fields.count)", value: $painter.fields.count, in: 1...40)
            TextField("Time limit (optional), such as 2hr", text: $painter.fields.timeLimit)
            if painter.tool == .row {
                Stepper(
                    "Depth: \(painter.rowDepthM, specifier: "%.1f") m", value: $painter.rowDepthM,
                    in: 1.5...8, step: 0.1)
                Toggle("Away from the street", isOn: $painter.rowFlip).toggleStyle(.checkbox).help(
                    "The row is drawn on the side of the kerb towards the nearest centreline; tick to draw it on the other side"
                )
            }
        case .driveway: EmptyView()
        }
    }

    // MARK: The selected mark

    @ViewBuilder private var selectedSection: some View {
        if let mark = painter.selected {
            VStack(alignment: .leading, spacing: 6) {
                HStack {
                    Text(mark.kind.label).font(.subheadline.weight(.semibold))
                    Spacer()
                    if mark.kind == .travelLane {
                        Button("Reverse") { painter.reverseSelected() }.controlSize(.small).help(
                            "Turn the lane round: its direction of travel is the order of its vertices"
                        )
                    }
                    Button(role: .destructive, action: painter.deleteSelected) {
                        Image(systemName: "trash")
                    }.controlSize(.small).help("Delete this mark (Delete)")
                }
                Text(provenance(mark)).font(.caption2).foregroundStyle(.secondary).textSelection(
                    .enabled)
                selectedTags(mark)
                if mark.kind == .travelLane, let bearing = StreetGeometry.laneDirection(mark.plan) {
                    Text(String(format: "Travels towards %.0f°", bearing)).font(.caption)
                        .foregroundStyle(.secondary)
                }
                Text("\(mark.points.count) vertices").font(.caption2).foregroundStyle(.secondary)
                ProblemLines(problems: painter.problems(for: mark))
            }
        }
    }

    private func provenance(_ mark: StreetMark) -> String {
        guard !mark.featureID.isEmpty else { return "New: not saved yet" }
        var parts = [mark.featureID]
        if !mark.author.isEmpty { parts.append(mark.author) }
        if let at = mark.editedAt {
            parts.append(at.formatted(date: .abbreviated, time: .shortened))
        }
        return parts.joined(separator: " · ")
    }

    private func tag(_ key: String) -> Binding<String> {
        Binding(
            get: { painter.selected?.tags[key] ?? "" }, set: { painter.setSelectedTag(key, $0) })
    }

    @ViewBuilder private func selectedTags(_ mark: StreetMark) -> some View {
        switch mark.kind {
        case .surface:
            Picker("Surface", selection: tag(StreetUseTags.surface)) {
                ForEach(
                    options(StreetUseTags.surfaces, mark.tags[StreetUseTags.surface]), id: \.self
                ) { Text($0).tag($0) }
            }
        case .travelLane:
            Picker("Lane type", selection: tag(StreetUseTags.laneType)) {
                ForEach(
                    options(StreetUseTags.laneTypes, mark.tags[StreetUseTags.laneType]), id: \.self
                ) { Text($0).tag($0) }
            }
            HStack {
                Text("Width")
                TextField("m", text: $widthText).frame(width: 60).multilineTextAlignment(.trailing)
                    .onSubmit { painter.setSelectedTag(StreetUseTags.widthM, widthText) }
                Text("m").foregroundStyle(.secondary)
            }.onAppear { widthText = mark.tags[StreetUseTags.widthM] ?? "" }.onChange(of: mark.key)
            { _, _ in widthText = painter.selected?.tags[StreetUseTags.widthM] ?? "" }
        case .parkingSpace:
            Picker("Space type", selection: tag(StreetUseTags.spaceType)) {
                ForEach(
                    options(StreetUseTags.spaceTypes, mark.tags[StreetUseTags.spaceType]),
                    id: \.self
                ) { Text($0).tag($0) }
            }
            Stepper(
                "Spaces: \(Int(mark.tags[StreetUseTags.count] ?? "") ?? 1)",
                onIncrement: { stepCount(mark, by: 1) }, onDecrement: { stepCount(mark, by: -1) })
            TextField("Time limit", text: tag(StreetUseTags.timeLimit))
        case .driveway: EmptyView()
        }
    }

    /// The values offered, with the mark's own first if Gerardus sent one
    /// the painter does not list.
    private func options(_ known: [String], _ current: String?) -> [String] {
        guard let current, !current.isEmpty, !known.contains(current) else { return [""] + known }
        return [current] + known
    }

    private func stepCount(_ mark: StreetMark, by step: Int) {
        let n = max(1, (Int(mark.tags[StreetUseTags.count] ?? "") ?? 1) + step)
        painter.setSelectedTag(StreetUseTags.count, String(n))
    }

    // MARK: Save

    private var saveSection: some View {
        let blockers = painter.saveBlockers(author: author)
        let blankAuthor = author.trimmingCharacters(in: .whitespaces).isEmpty
        return VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline) {
                Text("Author").font(.callout).foregroundStyle(
                    blankAuthor ? Color.orange : .secondary)
                TextField("Your name", text: $author).textFieldStyle(.roundedBorder)
            }.help("Gerardus records every mark under this name, and refuses an edit without it")
            TextField("Note (optional)", text: $painter.note)
            HStack {
                Button(saveTitle) { Task { await painter.save(author: author) } }.buttonStyle(
                    .borderedProminent
                ).disabled(!blockers.isEmpty).help(
                    "Send every change as one EditStreetUse: Gerardus applies all of it or none, then snaps and drapes each vertex"
                )
                if painter.isSaving { ProgressView().controlSize(.small) }
            }
            AlignmentBlockers(reasons: blockers.filter { $0 != "No changes to save." })
            if let saved = painter.savedMessage {
                Text(saved).font(.caption).foregroundStyle(.green).fixedSize(
                    horizontal: false, vertical: true)
            }
            ProblemLines(problems: painter.generalProblems)
        }
    }

    private var saveTitle: String {
        let n = painter.edit.pendingEdits.count
        return n == 0 ? "Save" : "Save \(n) change\(n == 1 ? "" : "s")"
    }

    // MARK: Marks

    private var marksSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Marks").font(.subheadline.weight(.semibold))
            if painter.edit.marks.isEmpty {
                Text("None at this site yet.").font(.caption).foregroundStyle(.secondary)
            }
            ForEach(painter.edit.marks) { mark in
                let problems = painter.problems(for: mark)
                Button {
                    painter.focus(mark)
                } label: {
                    VStack(alignment: .leading, spacing: 1) {
                        HStack(spacing: 6) {
                            Circle().fill(swatch(mark)).frame(width: 8, height: 8)
                            Text(mark.kind.label).font(.caption.weight(.medium))
                            Text(mark.summary).font(.caption).foregroundStyle(.secondary)
                            Spacer()
                            if mark.featureID.isEmpty {
                                Text("new").font(.caption2).foregroundStyle(.secondary)
                            }
                            if !problems.isEmpty {
                                Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(
                                    .red
                                ).font(.caption)
                            }
                        }
                        ProblemLines(problems: problems)
                    }.padding(4).frame(maxWidth: .infinity, alignment: .leading).background(
                        painter.selectedKey == mark.key
                            ? Color.accentColor.opacity(0.15) : Color.secondary.opacity(0.06)
                    ).clipShape(RoundedRectangle(cornerRadius: 4))
                }.buttonStyle(.plain).help("Select this mark and centre the view on it")
            }
        }
    }

    private func swatch(_ mark: StreetMark) -> Color {
        let c = StreetPaintStyle.colour(mark)
        return Color(red: Double(c.x), green: Double(c.y), blue: Double(c.z))
    }

    // MARK: Layers

    private var layersSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Drawn under the marks").font(.subheadline.weight(.semibold))
            if let reference = session.reference {
                if reference.streetLayersServed {
                    Toggle("Kerbs and islands (\(reference.kerbs))", isOn: $session.showKerbs)
                    Toggle(
                        "City streets (\(reference.cityStreets), \(reference.cityIntersections) intersections)",
                        isOn: $session.showCityStreets)
                } else {
                    Text(
                        "This Gerardus lists no street layers: marks snap to OSM roads and the boxes."
                    ).font(.caption2).foregroundStyle(.secondary)
                }
                if reference.boxesServed {
                    Toggle("DataSF boxes (\(reference.boxWalls) walls)", isOn: $session.showBoxes)
                }
                Toggle("DataSF footprints (\(reference.footprints))", isOn: $session.showFootprints)
                Toggle("OSM ways (\(reference.osmFeatures))", isOn: $session.showOSM)
                Toggle(
                    "Ground mesh (\(reference.groundMeshCells.formatted()) cells)",
                    isOn: $session.showGroundMesh)
                Toggle(
                    "Registered scans (\(reference.registeredPoints.formatted()) points)",
                    isOn: $session.showRegisteredScans)
                Toggle("3DEP points", isOn: $session.showAerial)
                if let note = session.streetLayersNote {
                    Text(note).font(.caption2).foregroundStyle(.secondary)
                }
                if reference.groundMeshCells == 0 {
                    Text(
                        "No ground mesh: new vertices take the nearest kerb's or street's height, and Gerardus drapes them on save."
                    ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                        horizontal: false, vertical: true)
                }
            }
            Toggle("Show the marks outside paint mode", isOn: $painter.showStreetUse)
        }.toggleStyle(.checkbox).font(.callout)
    }
}

/// A mark's problems, one red line each.
struct ProblemLines: View {
    let problems: [String]

    var body: some View {
        ForEach(problems, id: \.self) { problem in
            Label(problem, systemImage: "exclamationmark.triangle").font(.caption2).foregroundStyle(
                .red
            ).fixedSize(horizontal: false, vertical: true)
        }
    }
}
