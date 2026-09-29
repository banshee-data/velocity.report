// PhysicalReferencePane.swift
// The editing column in physical-reference mode: the object's persistent body,
// this keyframe's pose, the evidence behind each, and save and review as two
// separate steps.
//
// Metres and degrees on screen; radians only at the model boundary. Nothing
// is prefilled as observed or inferred: a new body is all unknown, and a
// placed position has no bound until the operator states one. Every control
// is a plain field, picker or button, so the whole record can be authored from
// the keyboard without dragging anything.

import SwiftUI

struct PhysicalReferencePane: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var physical: PhysicalReferenceSession

    init(session: AnnotationSession) {
        self.session = session
        self.physical = session.physical
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            statusSection
            if physical.availability == .ready || isReadOnly {
                if let object = session.activeObject {
                    Divider()
                    bodySection(object.objectID)
                    Divider()
                    keyframeSection(object.objectID)
                } else {
                    Text("Choose an object in the list to author its physical reference.").font(
                        .caption
                    ).foregroundStyle(.secondary)
                }
                Divider()
                saveSection
            }
        }
    }

    private var isReadOnly: Bool {
        if case .readOnly = physical.availability { return true }
        return false
    }

    // MARK: Status

    private var statusSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Physical reference").font(.headline)
            Label(
                "Blind authoring: the main-view link is paused. Keep the main window's tracker "
                    + "boxes out of sight while authoring an independent reference.",
                systemImage: "eye.slash"
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            switch physical.availability {
            case .notLoaded, .loading: ProgressView().controlSize(.small)
            case .unavailable(let reason):
                Text(reason).font(.caption).foregroundStyle(.red).fixedSize(
                    horizontal: false, vertical: true)
            case .readOnly(let reason):
                Text(reason).font(.caption).foregroundStyle(.orange).fixedSize(
                    horizontal: false, vertical: true)
            case .ready:
                if let state = physical.state {
                    Text(
                        state.exists
                            ? "Saved revision \(state.revision) · membership revision \(state.membershipRevision)"
                            : "No references saved for this pack yet"
                    ).font(.caption2).foregroundStyle(.secondary)
                }
            }
            if !physical.staleProblems.isEmpty {
                Text("Records that no longer hold against membership").font(.caption.bold())
                    .foregroundStyle(.orange)
                ForEach(physical.staleProblems, id: \.self) { problem in
                    Text("\(problem.record): \(problem.problem)").font(.caption2).foregroundStyle(
                        .orange
                    ).fixedSize(horizontal: false, vertical: true)
                }
            }
            if let error = physical.lastError {
                Text(error).font(.caption).foregroundStyle(.red).fixedSize(
                    horizontal: false, vertical: true
                ).textSelection(.enabled)
            }
            if let note = physical.lastNote {
                Text(note).font(.caption2).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
            }
            HStack {
                if physical.needsReload || physical.lastErrorCode == "conflict" {
                    Button("Reload, keep draft") { Task { await physical.load(keepDraft: true) } }
                        .help(
                            "Read the stored document and keep your draft to reconcile against it")
                }
                Button(physical.isDirty ? "Reload, discard draft" : "Reload") {
                    Task { await physical.load() }
                }
            }.controlSize(.small).disabled(physical.busy)
        }
    }

    // MARK: Body

    private func bodySection(_ objectID: String) -> some View {
        let body = physical.object(objectID)?.body
        let saved = physical.savedBody(objectID: objectID)
        return VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("Body · whole object").font(.subheadline.bold())
                Spacer()
                if let saved { reviewBadge(saved.review.status) }
            }
            Text(
                "One body for the whole episode. A changed dimension is a new body, and every "
                    + "keyframe's review is reset when it is saved."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            if body != nil {
                dimensionEditor("Length", objectID: objectID, \.length)
                dimensionEditor("Width", objectID: objectID, \.width)
                dimensionEditor("Height", objectID: objectID, \.height)
                recordReviewFields(
                    method: bodyBinding(objectID, \.review.method, ""),
                    assumptions: bodyBinding(objectID, \.review.uncertaintyAssumptions, nil),
                    needsAssumptions: body?.declaresBounds ?? false)
                HStack {
                    Button("Review body") {
                        guard let saved else { return }
                        Task {
                            await physical.review(
                                kind: .body, objectID: objectID, recordID: saved.bodyID)
                        }
                    }.disabled(
                        saved == nil || saved?.review.status == .reviewed || physical.isDirty
                            || !physical.canEdit
                    ).help(
                        "Confirms the saved body. Save first; an unsaved edit cannot be reviewed.")
                    Spacer()
                    Button("Remove body", role: .destructive) {
                        physical.edit { PhysicalDraft.removeBody(objectID: objectID, from: &$0) }
                    }.disabled(!physical.canEdit)
                }.controlSize(.small)
            } else {
                Button("Add body (all unknown)") {
                    let author = session.operatorName
                    let id = session.sessionID
                    physical.edit {
                        PhysicalDraft.addBody(
                            objectID: objectID, author: author, session: id, to: &$0)
                    }
                }.controlSize(.small).disabled(!physical.canEdit)
            }
        }
    }

    private func dimensionEditor(
        _ name: String, objectID: String, _ path: WritableKeyPath<PhysicalBody, PhysicalDimension>
    ) -> some View {
        let d = physical.object(objectID)?.body?[keyPath: path] ?? PhysicalDimension()
        func update(_ change: @escaping (inout PhysicalDimension) -> Void) {
            physical.edit { objects in
                PhysicalDraft.updateBody(objectID: objectID, in: &objects) {
                    change(&$0[keyPath: path])
                }
            }
        }
        func number(_ field: WritableKeyPath<PhysicalDimension, Double?>) -> Binding<Double?> {
            Binding(
                get: { d[keyPath: field] }, set: { value in update { $0[keyPath: field] = value } })
        }
        return VStack(alignment: .leading, spacing: 3) {
            HStack {
                Text(name).font(.caption.bold()).frame(width: 48, alignment: .leading)
                evidencePicker(
                    Binding(
                        get: { d.status },
                        set: { s in update { PhysicalDraft.setStatus(s, of: &$0) } }))
                if d.status == .observed {
                    Picker(
                        "",
                        selection: Binding(
                            get: { d.span ?? .full },
                            set: { span in update { PhysicalDraft.setSpan(span, of: &$0) } })
                    ) {
                        Text("Full").tag(PhysicalSpan.full)
                        Text("Partial").tag(PhysicalSpan.partial)
                    }.labelsHidden().frame(width: 76).help(
                        "Partial: the evidence covers only part of the body, so it is a lower bound."
                    )
                }
            }
            if d.status != .unknown {
                HStack(spacing: 4) {
                    metres("min", number(\.lowerM))
                    if d.span != .partial {
                        metres("max", number(\.upperM))
                        metres("value", number(\.valueM)).help("Optional, inside [min, max]")
                    }
                }
                supportEditor(
                    Binding(get: { d.support }, set: { s in update { $0.support = s } }),
                    status: d.status)
            }
        }
    }

    // MARK: Keyframe

    private func keyframeSection(_ objectID: String) -> some View {
        let sample = session.currentSample
        let k = session.physicalKeyframe
        let saved = sample.flatMap {
            physical.savedKeyframe(objectID: objectID, sampleID: $0.sampleID)
        }
        return VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("Keyframe · this frame only").font(.subheadline.bold())
                Spacer()
                if let saved { reviewBadge(saved.review.status) }
            }
            if let sample {
                Text(
                    "Frame \(sample.sourceOrdinal) · sample \(sample.sampleID) · \(sample.timestampNs) ns"
                ).font(.caption2.monospacedDigit()).foregroundStyle(.secondary).textSelection(
                    .enabled)
            }
            if let k, let sample {
                keyframeEditor(objectID: objectID, k: k, sample: sample)
                HStack {
                    Button("Review keyframe") {
                        guard let saved else { return }
                        Task {
                            await physical.review(
                                kind: .keyframe, objectID: objectID, recordID: saved.keyframeID)
                        }
                    }.disabled(
                        saved == nil || saved?.review.status == .reviewed || physical.isDirty
                            || !physical.canEdit
                    ).help("Confirms the saved keyframe. It does not review the mask or the body.")
                    Spacer()
                    Button("Remove keyframe", role: .destructive) {
                        physical.edit {
                            PhysicalDraft.removeKeyframe(
                                objectID: objectID, sampleID: sample.sampleID, from: &$0)
                        }
                    }.disabled(!physical.canEdit)
                }.controlSize(.small)
            } else if let sample {
                Text("Click in the Top view to place this frame's position, or add it empty.").font(
                    .caption2
                ).foregroundStyle(.secondary)
                Button("Add keyframe at this frame") {
                    let author = session.operatorName
                    let id = session.sessionID
                    physical.edit {
                        PhysicalDraft.addKeyframe(
                            objectID: objectID, sample: sample, author: author, session: id, to: &$0
                        )
                    }
                }.controlSize(.small).disabled(!physical.canEdit)
            }
        }
    }

    private func keyframeEditor(
        objectID: String, k: PhysicalKeyframe, sample: AnnotationSample
    ) -> some View {
        func update(_ change: @escaping (inout PhysicalKeyframe) -> Void) {
            physical.edit { objects in
                PhysicalDraft.updateKeyframe(
                    objectID: objectID, sampleID: sample.sampleID, in: &objects, change)
            }
        }
        func number(_ field: WritableKeyPath<PhysicalKeyframe, Double?>) -> Binding<Double?> {
            Binding(
                get: { k[keyPath: field] }, set: { value in update { $0[keyPath: field] = value } })
        }
        let resolved = k.yaw.axis == .resolved
        return VStack(alignment: .leading, spacing: 5) {
            // Orientation first: which faces and ends can be named depends on it.
            Text("Axis").font(.caption.bold())
            Picker(
                "",
                selection: Binding(
                    get: { k.yaw.axis }, set: { a in update { PhysicalDraft.setAxis(a, of: &$0) } })
            ) { ForEach(PhysicalAxisState.allCases, id: \.self) { Text($0.label).tag($0) } }
            .labelsHidden()
            if k.yaw.axis != .unknown {
                HStack(spacing: 4) {
                    evidencePicker(
                        Binding(
                            get: { k.yaw.status },
                            set: { s in update { $0.yaw.status = s == .unknown ? .observed : s } }),
                        allowUnknown: false)
                    degrees(
                        "yaw",
                        Binding(
                            get: { k.yaw.yawRad.map(PhysicalUnits.degrees) },
                            set: { d in
                                update {
                                    $0.yaw.yawRad = d.map {
                                        PhysicalUnits.radians(PhysicalUnits.wrappedDegrees($0))
                                    }
                                }
                            })
                    ).help("Direction of the body's front, anticlockwise from the pack's +X")
                    degrees(
                        "±",
                        Binding(
                            get: { k.yaw.boundRad.map(PhysicalUnits.degrees) },
                            set: { d in update { $0.yaw.boundRad = d.map(PhysicalUnits.radians) } })
                    )
                }
                supportEditor(
                    Binding(get: { k.yaw.support }, set: { s in update { $0.yaw.support = s } }),
                    status: k.yaw.status, ownSample: sample.sampleID)
            }

            Text("Anchor").font(.caption.bold()).padding(.top, 4)
            Picker(
                "",
                selection: Binding(
                    get: { k.anchor.kind },
                    set: { a in update { PhysicalDraft.setAnchor(a, of: &$0) } })
            ) {
                ForEach(PhysicalAnchorKind.allCases, id: \.self) { kind in
                    Text(kind.label).tag(kind)
                }
            }.labelsHidden().help(
                resolved
                    ? "The point the position names. A face without an offset locates the face only."
                    : "A face can be named only when the axis is resolved.")
            if k.anchor.kind.isFace && !resolved {
                Text("A face needs a resolved axis.").font(.caption2).foregroundStyle(.orange)
            }
            if k.anchor.kind.isFace {
                HStack(spacing: 4) {
                    metres("to centre", number(\.anchor.offsetM))
                    metres("±", number(\.anchor.offsetBoundM))
                }.help(
                    "Optional. Without both, the anchor locates the face and not the body centre.")
            }

            Text("Position").font(.caption.bold()).padding(.top, 4)
            evidencePicker(
                Binding(
                    get: { k.position.status },
                    set: { s in update { PhysicalDraft.setStatus(s, of: &$0.position) } }))
            if k.position.status != .unknown {
                HStack(spacing: 4) {
                    metres("x", number(\.position.xM))
                    metres("y", number(\.position.yM))
                    metres("±", number(\.position.boundM)).help(
                        "Horizontal radius that bounds the anchor")
                }
                HStack(spacing: 4) {
                    Toggle(
                        "Height",
                        isOn: Binding(
                            get: { k.position.zM != nil },
                            set: { on in
                                update { $0.position.zM = on ? ($0.position.zM ?? 0) : nil }
                            })
                    ).font(.caption).help(
                        "Optional and unbounded: version 1 records no vertical uncertainty.")
                    if k.position.zM != nil { metres("z", number(\.position.zM)) }
                }
                supportEditor(
                    Binding(
                        get: { k.position.support },
                        set: { s in update { $0.position.support = s } }),
                    status: k.position.status, ownSample: sample.sampleID)
            }

            Text("Ends").font(.caption.bold()).padding(.top, 4)
            if resolved {
                endpointEditor(
                    "Front", Binding(get: { k.front }, set: { e in update { $0.front = e } }),
                    ownSample: sample.sampleID)
                endpointEditor(
                    "Rear", Binding(get: { k.rear }, set: { e in update { $0.rear = e } }),
                    ownSample: sample.sampleID)
                Text(
                    "End positions follow from the anchor, yaw and body length; only their evidence is set here."
                ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
            } else {
                Text("No front or rear can be named until the axis is resolved.").font(.caption2)
                    .foregroundStyle(.secondary)
            }

            derivedSummary(objectID: objectID, k: k)

            recordReviewFields(
                method: Binding(
                    get: { k.review.method }, set: { m in update { $0.review.method = m } }),
                assumptions: Binding(
                    get: { k.review.uncertaintyAssumptions },
                    set: { a in update { $0.review.uncertaintyAssumptions = a } }),
                needsAssumptions: k.declaresBounds)
        }
    }

    /// What the draft establishes, in the terms the scorer will use.
    private func derivedSummary(objectID: String, k: PhysicalKeyframe) -> some View {
        let g = physical.object(objectID).map {
            PhysicalGeometry.derive(object: $0, keyframe: k, gate: .preview)
        }
        return VStack(alignment: .leading, spacing: 1) {
            Text("Derived (draft)").font(.caption.bold()).padding(.top, 4)
            if let g {
                line(
                    "centre",
                    g.centre.map { String(format: "%.2f, %.2f ± %.2f m", $0.x, $0.y, $0.bound) },
                    g.centreUnavailable)
                line(
                    "front",
                    g.front.map { String(format: "%.2f, %.2f ± %.2f m", $0.x, $0.y, $0.bound) },
                    g.frontUnavailable)
                line(
                    "rear",
                    g.rear.map { String(format: "%.2f, %.2f ± %.2f m", $0.x, $0.y, $0.bound) },
                    g.rearUnavailable)
                line(
                    "box", g.box.map { String(format: "%.2f × %.2f m", $0.length, $0.width) },
                    g.boxUnavailable)
            }
        }
    }

    private func line(_ name: String, _ value: String?, _ reason: String?) -> some View {
        Text("\(name): \(value ?? "— \(PhysicalUnavailable.describe(reason))")").font(
            .caption2.monospacedDigit()
        ).foregroundStyle(value == nil ? .secondary : .primary)
    }

    // MARK: Save

    private var saveSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let v = physical.validation {
                if v.valid {
                    Label("The service accepts this draft", systemImage: "checkmark.circle").font(
                        .caption
                    ).foregroundStyle(.green)
                } else {
                    if let invalid = v.invalid {
                        Text(invalid).font(.caption).foregroundStyle(.red).fixedSize(
                            horizontal: false, vertical: true)
                    }
                    ForEach(v.linkProblems, id: \.self) { p in
                        Text("\(p.record): \(p.problem)").font(.caption2).foregroundStyle(.red)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                if !v.resetReviews.isEmpty {
                    Text("Saving returns to proposed: \(v.resetReviews.joined(separator: ", "))")
                        .font(.caption2).foregroundStyle(.orange).fixedSize(
                            horizontal: false, vertical: true)
                }
            } else if physical.isDirty {
                Text("Checking the draft…").font(.caption2).foregroundStyle(.secondary)
            }
            HStack {
                Button("Undo") { physical.undo() }.disabled(!physical.canUndo || !physical.canEdit)
                Button("Redo") { physical.redo() }.disabled(!physical.canRedo || !physical.canEdit)
                Button("Discard", role: .destructive) { physical.discard() }.disabled(
                    !physical.isDirty)
            }.controlSize(.small)
            Button("Save proposal") { Task { await physical.save() } }.keyboardShortcut(
                "s", modifiers: [.command, .shift]
            ).disabled(!physical.isDirty || !physical.canEdit || physical.needsReload).help(
                "Saves the draft as a proposal. Saving never reviews: review each saved record separately."
            )
            Text(
                "Edit → Save proposal → inspect in both views → Review. Reviewing a mask does not review a pose."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    // MARK: Pieces

    private func reviewBadge(_ status: ReviewStatus) -> some View {
        Text(status == .reviewed ? "reviewed" : status == .rejected ? "rejected" : "proposed").font(
            .caption2
        ).padding(.horizontal, 5).padding(.vertical, 1).background(
            (status == .reviewed ? Color.green : Color.secondary).opacity(0.25), in: Capsule())
    }

    private func evidencePicker(
        _ binding: Binding<PhysicalEvidence>, allowUnknown: Bool = true
    ) -> some View {
        Picker("", selection: binding) {
            ForEach(PhysicalEvidence.allCases.filter { allowUnknown || $0 != .unknown }, id: \.self)
            { Text($0.label).tag($0) }
        }.labelsHidden().frame(width: 104).help(
            "Observed: seen in cited frames. Inferred: follows from named frames or an external "
                + "reference. Prior only: a named class prior, never scored. Unknown: no value.")
    }

    private func metres(_ label: String, _ value: Binding<Double?>) -> some View {
        HStack(spacing: 2) {
            Text(label).font(.caption2).foregroundStyle(.secondary)
            TextField("m", value: value, format: .number.precision(.fractionLength(0...3)))
                .textFieldStyle(.roundedBorder).font(.caption.monospacedDigit()).frame(width: 52)
        }
    }

    private func degrees(_ label: String, _ value: Binding<Double?>) -> some View {
        HStack(spacing: 2) {
            Text(label).font(.caption2).foregroundStyle(.secondary)
            TextField("°", value: value, format: .number.precision(.fractionLength(0...2)))
                .textFieldStyle(.roundedBorder).font(.caption.monospacedDigit()).frame(width: 52)
            Text("°").font(.caption2).foregroundStyle(.secondary)
        }
    }

    /// Frames and an external reference, as the status needs them: an
    /// observation cites frames, an inference frames or a reference, a prior
    /// names itself, and unknown names nothing.
    private func supportEditor(
        _ support: Binding<PhysicalSupport>, status: PhysicalEvidence, ownSample: Int? = nil
    ) -> some View {
        let current = session.currentSample?.sampleID
        let frames = support.wrappedValue.frameList
        return VStack(alignment: .leading, spacing: 2) {
            if status == .observed || status == .inferred {
                HStack(spacing: 4) {
                    Text(
                        frames.isEmpty
                            ? "cites no frame"
                            : "cites \(frames.map(String.init).joined(separator: ", "))"
                    ).font(.caption2.monospacedDigit()).foregroundStyle(
                        frames.isEmpty ? .orange : .secondary)
                    if let current {
                        Button(frames.contains(current) ? "Uncite this frame" : "Cite this frame") {
                            var s = support.wrappedValue
                            s.toggle(frame: current)
                            support.wrappedValue = s
                        }.controlSize(.mini)
                    }
                }
                if status == .observed, let ownSample, !frames.contains(ownSample) {
                    Text("An observation at a keyframe must cite its own frame (\(ownSample)).")
                        .font(.caption2).foregroundStyle(.orange)
                }
            }
            if status == .inferred || status == .priorOnly {
                TextField(
                    status == .priorOnly ? "Name the prior" : "External reference (optional)",
                    text: Binding(
                        get: { support.wrappedValue.external ?? "" },
                        set: { text in
                            var s = support.wrappedValue
                            s.external = text.isEmpty ? nil : text
                            support.wrappedValue = s
                        })
                ).textFieldStyle(.roundedBorder).font(.caption2)
            }
        }
    }

    private func endpointEditor(
        _ name: String, _ endpoint: Binding<PhysicalEndpoint>, ownSample: Int
    ) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Text(name).font(.caption).frame(width: 40, alignment: .leading)
                evidencePicker(
                    Binding(
                        get: { endpoint.wrappedValue.status },
                        set: { s in
                            var e = endpoint.wrappedValue
                            PhysicalDraft.setStatus(s, of: &e)
                            endpoint.wrappedValue = e
                        }))
            }
            if endpoint.wrappedValue.status != .unknown {
                supportEditor(
                    Binding(
                        get: { endpoint.wrappedValue.support },
                        set: { s in
                            var e = endpoint.wrappedValue
                            e.support = s
                            endpoint.wrappedValue = e
                        }), status: endpoint.wrappedValue.status, ownSample: ownSample)
            }
        }
    }

    private func recordReviewFields(
        method: Binding<String>, assumptions: Binding<String?>, needsAssumptions: Bool
    ) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 4) {
                Text("method").font(.caption2).foregroundStyle(.secondary)
                TextField("manual_box", text: method).textFieldStyle(.roundedBorder).font(.caption2)
            }
            TextField(
                "Uncertainty assumptions" + (needsAssumptions ? " (required)" : ""),
                text: Binding(
                    get: { assumptions.wrappedValue ?? "" },
                    set: { assumptions.wrappedValue = $0.isEmpty ? nil : $0 }), axis: .vertical
            ).textFieldStyle(.roundedBorder).font(.caption2).lineLimit(1...3).help(
                "What the bounds assume: how they were read off, and what they would not survive. "
                    + "Bounds are conservative limits, not confidence scores.")
        }
    }

    private func bodyBinding<T>(
        _ objectID: String, _ path: WritableKeyPath<PhysicalBody, T>, _ fallback: T
    ) -> Binding<T> {
        Binding(
            get: { physical.object(objectID)?.body?[keyPath: path] ?? fallback },
            set: { value in
                physical.edit { objects in
                    PhysicalDraft.updateBody(objectID: objectID, in: &objects) {
                        $0[keyPath: path] = value
                    }
                }
            })
    }
}
