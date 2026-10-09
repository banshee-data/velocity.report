// PhysicalReferencePane.swift
// The editing column in physical-reference mode: the object's persistent body,
// this keyframe's pose, the evidence behind each, and save and review as two
// separate steps.
//
// Fitting comes first: the size and poses are measured from the reviewed
// returns, with every bound's terms shown, and the operator's judgement is
// whether each end the returns reach is real. Nothing asks which evidence
// status a field has: the fit states it, and a value set by hand takes the
// status its action implies. The manual controls stay, under Adjust, for what
// a fit cannot do. Metres and degrees on screen; radians only at the model
// boundary.

import AppKit
import SwiftUI

struct PhysicalReferencePane: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var physical: PhysicalReferenceSession
    @ObservedObject private var inspector: PhysicalReportInspector
    @State private var assistanceSource = ""

    init(session: AnnotationSession) {
        self.session = session
        self.physical = session.physical
        self.inspector = session.reportInspector
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            statusSection
            if physical.availability == .ready || isReadOnly {
                if let object = session.activeObject {
                    PhysicalFitSection(
                        session: session, physical: physical, objectID: object.objectID)
                    Divider()
                    if session.comparisonAllowed { trackerSeedSection }
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
                Divider()
                historySection
            }
        }
    }

    private var isReadOnly: Bool {
        if case .readOnly = physical.availability { return true }
        return false
    }

    private func humanMessage(_ raw: String) -> String {
        // Include retained records when a draft replaced their IDs. These
        // labels affect presentation only; repairs still receive raw records.
        let saved = physical.state?.document.objects ?? []
        return PhysicalAuthoringMessage.describe(
            raw, objects: physical.draft + saved, name: { session.displayName(objectID: $0) })
    }

    private func diagnostic(_ raw: String, colour: Color) -> some View {
        let human = humanMessage(raw)
        return VStack(alignment: .leading, spacing: 3) {
            Text(human).font(.caption).foregroundStyle(colour).textSelection(.enabled).fixedSize(
                horizontal: false, vertical: true)
            if human != raw {
                DisclosureGroup("Service detail") {
                    Text(raw).font(.caption2.monospaced()).textSelection(.enabled).fixedSize(
                        horizontal: false, vertical: true)
                }.font(.caption2).foregroundStyle(.secondary)
            }
        }
    }

    // MARK: Status

    private var statusSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Physical reference").font(.headline)
            if session.activeObjectID.flatMap({ physical.exposure[$0] }) == nil {
                Label(
                    "Independent authoring requires keeping the main window's tracker boxes out of sight. The main-view link is paused; that alone does not establish independence.",
                    systemImage: "eye.slash"
                ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
            }
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
                ForEach(physical.staleProblems, id: \.self) { problem in staleRow(problem) }
            }
            if !physical.driftProblems.isEmpty {
                Text("Reviewed, but membership changed since").font(.caption.bold())
                    .foregroundStyle(.orange)
                ForEach(physical.driftProblems, id: \.self) { problem in
                    diagnostic("\(problem.record): \(problem.problem)", colour: .orange)
                }
                Text("Check each against the new points, then review it again.").font(.caption2)
                    .foregroundStyle(.secondary)
            }
            if let seen = session.activeObjectID.flatMap({ physical.exposure[$0] }) {
                Text(
                    "You have seen this object's estimate (\(seen)). Edits to it are saved as tracker-assisted."
                ).font(.caption2).foregroundStyle(.orange).fixedSize(
                    horizontal: false, vertical: true)
                if let objectID = session.activeObjectID, let object = physical.object(objectID),
                    (object.body.map {
                        $0.review.origin == .independent && $0.review.status != .reviewed
                    } ?? false)
                        || object.keyframes.contains(where: {
                            $0.review.origin == .independent && $0.review.status != .reviewed
                        })
                {
                    Button("Continue as assisted proposal") {
                        physical.continueAsAssisted(objectID: objectID)
                    }.controlSize(.small).disabled(!physical.canEdit || physical.needsReload)
                    Text(
                        "Reviewing after seeing an estimate is assisted too. Save this new proposal before reviewing it; previously reviewed independent records stay in history."
                    ).font(.caption2).foregroundStyle(.secondary)
                }
            } else if let objectID = session.activeObjectID {
                DisclosureGroup("I used the main-window estimate") {
                    TextField("Name the estimate, run and stage you used", text: $assistanceSource)
                        .textFieldStyle(.roundedBorder)
                    Text(
                        "This declaration stays with the object. Existing independent history is retained; current unsaved and later edits become tracker-assisted."
                    ).font(.caption2).foregroundStyle(.secondary)
                    Button("Declare assisted authoring") {
                        if physical.declareAssistance(objectID: objectID, source: assistanceSource)
                        {
                            assistanceSource = ""
                        }
                    }.controlSize(.small).disabled(
                        !physical.canEdit || physical.needsReload || physical.gestureInProgress
                            || assistanceSource.trimmingCharacters(in: .whitespacesAndNewlines)
                                .isEmpty
                    )
                }.font(.caption2)
            }
            if let error = physical.lastError { diagnostic(error, colour: .red) }
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

    private var trackerSeedSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            DisclosureGroup("Start from tracker estimate · assisted") {
                Text(
                    "Open a report in Compare, choose its estimate arm, then return here. Only a stated physical body centre at this exact object/frame can seed a pose; visible OBBs and medoids cannot."
                ).font(.caption2).foregroundStyle(.secondary)
                if let identity = inspector.armIdentity {
                    Text(identity.source).font(.caption2).textSelection(.enabled)
                    if let seed = try? session.trackerSeedEligibility() {
                        Text("Track \(seed.prediction.trackKey) · sample \(seed.sampleID)").font(
                            .caption2)
                        Button("Import report estimate as draft") {
                            session.seedPhysicalFromReport()
                        }.controlSize(.small).disabled(
                            !physical.canEdit || physical.needsReload || physical.isDirty
                                || !session.dirtySamples.isEmpty || session.physicalKeyframe != nil)
                    } else {
                        Text(
                            "No supported body seed for this object/frame. Place the anchor manually; the estimate's point meaning is retained."
                        ).font(.caption2).foregroundStyle(.orange)
                    }
                } else {
                    Text(
                        "No report open. Direct import of the main-window track is unavailable because that stream does not state the physical position's meaning."
                    ).font(.caption2).foregroundStyle(.secondary)
                }
                Text(
                    "Existing size and poses are kept. Imported values have no reference bounds: inspect the sketch, state bounds and assumptions, then save and review. Its source outline stays dashed pink while you edit."
                ).font(.caption2).foregroundStyle(.secondary)
            }.font(.caption2)
        }
    }

    private func bodySection(_ objectID: String) -> some View {
        let body = physical.object(objectID)?.body
        let saved = physical.savedBody(objectID: objectID)
        return VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("1 · Object size · all frames").font(.subheadline.bold())
                Spacer()
                if let saved { reviewBadge(saved.review.status) }
            }
            Text(
                "One size for the whole episode: length front to rear, width side to side. "
                    + "Fit it above, or set it here. Changing the size returns every pose to proposed."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            if body != nil {
                dimensionEditor("Length", objectID: objectID, \.length)
                dimensionEditor("Width", objectID: objectID, \.width)
                dimensionEditor("Height", objectID: objectID, \.height)
                DisclosureGroup("Method and assumptions") {
                    recordReviewFields(
                        method: bodyBinding(objectID, \.review.method, ""),
                        assumptions: bodyBinding(objectID, \.review.uncertaintyAssumptions, nil),
                        needsAssumptions: body?.declaresBounds ?? false)
                }.font(.caption2)
                HStack {
                    Button("Review body") {
                        guard let saved else { return }
                        Task {
                            await physical.review(
                                kind: .body, objectID: objectID, recordID: saved.bodyID)
                        }
                    }.disabled(
                        saved == nil
                            || (saved?.review.status == .reviewed
                                && !physical.hasDrift(record: saved?.bodyID))
                            || physical.isDirty || !physical.canEdit
                    ).help(
                        "Confirms the saved body. Save first; an unsaved edit cannot be reviewed.")
                    Spacer()
                    Button("Remove body", role: .destructive) {
                        physical.edit { PhysicalDraft.removeBody(objectID: objectID, from: &$0) }
                    }.disabled(!physical.canEdit)
                }.controlSize(.small)
            } else {
                Button("Add size by hand") {
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
        let current = session.currentSample?.sampleID
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
                Picker(
                    "",
                    selection: Binding(
                        get: { d.choice },
                        set: { c in update { PhysicalDraft.setChoice(c, of: &$0, citing: current) }
                        })
                ) {
                    ForEach(PhysicalDimensionChoice.allCases, id: \.self) { Text($0.label).tag($0) }
                }.pickerStyle(.segmented).labelsHidden().frame(width: 200).help(
                    "Full: measured end to end. At least: only part of it was seen, so it is a lower bound. Unknown: not stated."
                )
            }
            switch d.choice {
            case .unknown: EmptyView()
            case .atLeast: metres("at least", number(\.lowerM))
            case .full:
                HStack(spacing: 4) {
                    metres(
                        "value",
                        Binding(
                            get: { d.valueM },
                            set: { value in
                                update { PhysicalDraft.setDimensionValue(value, of: &$0) }
                            }))
                    metres(
                        "±",
                        Binding(
                            get: { d.best?.halfWidth },
                            set: { tolerance in
                                update {
                                    _ = PhysicalDraft.setDimensionTolerance(tolerance, of: &$0)
                                }
                            })
                    ).disabled(d.valueM == nil && !d.bounded)
                }
                if !d.bounded { hint("Enter ±: how far either way the value could be wrong.") }
                DisclosureGroup("Exact min/max") {
                    HStack(spacing: 4) {
                        metres("min", number(\.lowerM))
                        metres("max", number(\.upperM))
                    }
                }.font(.caption2)
            }
            if d.status != .unknown { evidenceCaption(d.status, d.support) }
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
                Text("2 · Pose at this frame").font(.subheadline.bold())
                Spacer()
                if let saved { reviewBadge(saved.review.status) }
            }
            Text(
                "Where the body is and which way it faces at this one frame. Fit it above, or place it by hand."
            ).font(.caption2).foregroundStyle(.secondary)
            if let sample {
                Text(
                    "\(sample.label.capitalized) · \(sample.recordingLabel) · \(sample.timestampNs) ns"
                ).font(.caption2.monospacedDigit()).foregroundStyle(.secondary).textSelection(
                    .enabled)
            }
            if let object = physical.object(objectID), !object.keyframes.isEmpty {
                Menu("Poses: \(object.keyframes.count) marked frames") {
                    ForEach(object.keyframes.sorted { $0.sampleID < $1.sampleID }, id: \.keyframeID)
                    { pose in
                        Button("Sample \(pose.sampleID) · \(pose.review.status.rawValue)") {
                            guard
                                let index = session.samples.firstIndex(where: {
                                    $0.sampleID == pose.sampleID
                                })
                            else { return }
                            if session.step(to: index) != nil {
                                physical.refuse(
                                    "Save or discard the current edits before choosing a pose frame."
                                )
                            }
                        }
                    }
                }.controlSize(.small)
            }
            if let k, let sample {
                keyframeEditor(objectID: objectID, k: k, sample: sample)
                HStack {
                    Button("Review saved pose") {
                        guard let saved else { return }
                        Task {
                            await physical.review(
                                kind: .keyframe, objectID: objectID, recordID: saved.keyframeID)
                        }
                    }.disabled(
                        saved == nil
                            || (saved?.review.status == .reviewed
                                && !physical.hasDrift(record: saved?.keyframeID))
                            || physical.isDirty || !physical.canEdit
                    ).help("Confirms the saved keyframe. It does not review the mask or the body.")
                    Spacer()
                    Button("Remove pose", role: .destructive) {
                        physical.edit {
                            PhysicalDraft.removeKeyframe(
                                objectID: objectID, sampleID: sample.sampleID, from: &$0)
                        }
                    }.disabled(!physical.canEdit)
                }.controlSize(.small)
            } else if let sample {
                Text("Fit the pose above, click in the Top view to place it, or add it by hand.")
                    .font(.caption2).foregroundStyle(.secondary)
                if let object = physical.object(objectID),
                    let source = PhysicalDraft.nearestKeyframe(of: object, to: sample)
                {
                    Button("Copy keyframe from sample \(source.sampleID)") {
                        let author = session.operatorName
                        let id = session.sessionID
                        physical.edit { objects in
                            let i = PhysicalDraft.index(of: objectID, in: &objects)
                            objects[i].keyframes.append(
                                PhysicalDraft.copy(source, to: sample, author: author, session: id))
                        }
                    }.controlSize(.small).disabled(!physical.canEdit).help(
                        "A proposal: what the source observed becomes inferred from its frames. "
                            + "Check it against this frame's returns before claiming anything observed."
                    )
                }
                Button("Add pose by hand") {
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
        func position(_ field: WritableKeyPath<PhysicalPosition, Double?>) -> Binding<Double?> {
            Binding(
                get: { k.position[keyPath: field] },
                set: { value in update { PhysicalDraft.setPosition(field, value, of: &$0) } })
        }
        let resolved = k.yaw.axis == .resolved
        let body = physical.object(objectID)?.body
        return VStack(alignment: .leading, spacing: 5) {
            poseSummary(k)

            Text("Ends").font(.caption.bold()).padding(.top, 4)
            if resolved {
                ForEach(
                    [("Front", \PhysicalKeyframe.front), ("Rear", \PhysicalKeyframe.rear)], id: \.0
                ) { name, end in
                    VStack(alignment: .leading, spacing: 1) {
                        Toggle(
                            "\(name) is a real end in this frame",
                            isOn: Binding(
                                get: { k[keyPath: end].status == .observed },
                                set: { on in
                                    update {
                                        PhysicalDraft.setEnd(seen: on, end, of: &$0, body: body)
                                    }
                                })
                        ).toggleStyle(.checkbox).font(.caption)
                        if k[keyPath: end].status != .observed {
                            Text(
                                k[keyPath: end].status == .inferred
                                    ? "Not seen here: placed by the body's length."
                                    : "Not seen here, and not placed."
                            ).font(.caption2).foregroundStyle(.secondary).padding(.leading, 18)
                        }
                    }
                }
                Text(
                    "Tick an end when this frame's returns reach the vehicle's real end, not the edge of something in front of it."
                ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
            } else {
                Text("No front or rear can be named until the axis is resolved.").font(.caption2)
                    .foregroundStyle(.secondary)
            }

            derivedSummary(objectID: objectID, k: k)

            DisclosureGroup("Adjust by hand") {
                VStack(alignment: .leading, spacing: 5) {
                    Text("Axis").font(.caption.bold())
                    Picker(
                        "",
                        selection: Binding(
                            get: { k.yaw.axis },
                            set: { a in update { PhysicalDraft.setAxis(a, of: &$0) } })
                    ) { ForEach(PhysicalAxisState.allCases, id: \.self) { Text($0.label).tag($0) } }
                    .labelsHidden()
                    if k.yaw.axis != .unknown {
                        HStack(spacing: 4) {
                            degrees(
                                "heading",
                                Binding(
                                    get: { k.yaw.yawRad.map(PhysicalUnits.degrees) },
                                    set: { d in
                                        update {
                                            // About the centre, as the ● handle turns.
                                            if let d {
                                                PhysicalDraft.turn(
                                                    &$0, toRad: PhysicalUnits.radians(d))
                                            } else {
                                                $0.yaw.yawRad = nil
                                            }
                                        }
                                    })
                            ).help(
                                "Direction of the body's front, anticlockwise from the pack's +X")
                            degrees(
                                "±",
                                Binding(
                                    get: { k.yaw.boundRad.map(PhysicalUnits.degrees) },
                                    set: { d in
                                        update { $0.yaw.boundRad = d.map(PhysicalUnits.radians) }
                                    }))
                        }
                    }

                    Text("Point fixed to the body").font(.caption.bold()).padding(.top, 4)
                    Picker(
                        "",
                        selection: Binding(
                            get: { k.anchor.kind },
                            set: { a in update { PhysicalDraft.setAnchor(a, of: &$0) } })
                    ) {
                        ForEach(PhysicalAnchorKind.allCases, id: \.self) { kind in
                            Text(kind.label).tag(kind).disabled(kind.isFace && !resolved)
                        }
                    }.labelsHidden().help(
                        resolved
                            ? "The point the position names. A face without an offset locates the face only."
                            : "A face can be named only when the axis is resolved.")
                    if k.anchor.kind.isFace {
                        HStack(spacing: 4) {
                            metres("to centre", number(\.anchor.offsetM))
                            metres("±", number(\.anchor.offsetBoundM))
                        }.help(
                            "Optional. Without both, the anchor locates the face and not the body centre."
                        )
                    }

                    Text("Position").font(.caption.bold()).padding(.top, 4)
                    Text("Click in Top to place ■; drag ■ to move and ● to turn.").font(.caption2)
                        .foregroundStyle(.secondary)
                    HStack(spacing: 4) {
                        metres("x", position(\.xM))
                        metres("y", position(\.yM))
                        metres("±", number(\.position.boundM)).help(
                            "Horizontal radius that bounds the anchor")
                    }
                    HStack(spacing: 4) {
                        Toggle(
                            "Anchor height (optional)",
                            isOn: Binding(
                                get: { k.position.zM != nil },
                                set: { on in
                                    update { $0.position.zM = on ? ($0.position.zM ?? 0) : nil }
                                })
                        ).font(.caption).help(
                            "The anchor's height in the pack's frame. Optional and unbounded; it is not the body's height."
                        )
                        if k.position.zM != nil { metres("z", number(\.position.zM)) }
                    }

                    sharedErrorEditor(objectID: objectID, k: k, sample: sample)

                    recordReviewFields(
                        method: Binding(
                            get: { k.review.method }, set: { m in update { $0.review.method = m } }),
                        assumptions: Binding(
                            get: { k.review.uncertaintyAssumptions },
                            set: { a in update { $0.review.uncertaintyAssumptions = a } }),
                        needsAssumptions: k.declaresBounds)
                }
            }.font(.caption)
        }
    }

    /// The pose at a glance: where, which way, and what is still missing.
    private func poseSummary(_ k: PhysicalKeyframe) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            if let x = k.position.xM, let y = k.position.yM {
                Text(
                    String(
                        format: "%@ at %.2f, %.2f%@", k.anchor.kind.label, x, y,
                        k.position.boundM.map { String(format: " ± %.2f m", $0) } ?? "")
                ).font(.caption.monospacedDigit())
                if k.position.boundM == nil {
                    hint("No ± on the position yet: state it under Adjust by hand.")
                }
            } else {
                Text("No position yet.").font(.caption2).foregroundStyle(.secondary)
            }
            if let yaw = k.yaw.yawRad {
                Text(
                    String(
                        format: "heading %.1f°%@ · %@", PhysicalUnits.degrees(yaw),
                        k.yaw.boundRad.map { String(format: " ± %.1f°", PhysicalUnits.degrees($0)) }
                            ?? "", k.yaw.axis.label)
                ).font(.caption.monospacedDigit())
                if k.yaw.boundRad == nil {
                    hint("No ± on the heading yet: state it under Adjust by hand.")
                }
            }
            if k.position.status != .unknown {
                evidenceCaption(k.position.status, k.position.support)
            }
        }
    }

    /// What the draft establishes, in the terms the scorer will use.
    private func derivedSummary(objectID: String, k: PhysicalKeyframe) -> some View {
        let g = physical.object(objectID).map {
            PhysicalGeometry.derive(object: $0, keyframe: k, gate: .preview)
        }
        return VStack(alignment: .leading, spacing: 1) {
            Text("Supported geometry · bounds required").font(.caption.bold()).padding(.top, 4)
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
                    if let invalid = v.invalid { diagnostic(invalid, colour: .red) }
                    ForEach(v.linkProblems, id: \.self) { p in
                        diagnostic("\(p.record): \(p.problem)", colour: .red)
                    }
                }
                if !v.resetReviews.isEmpty {
                    Text(
                        "Saving returns to proposed: \(v.resetReviews.map(humanMessage).joined(separator: ", "))"
                    ).font(.caption2).foregroundStyle(.orange).fixedSize(
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
            HStack {
                Button {
                    Task { await physical.save() }
                } label: {
                    shortcutLabel("Save proposal", key: "s")
                }.keyboardShortcut("s", modifiers: [.command, .shift]).disabled(
                    !physical.isDirty || !physical.canEdit || physical.needsReload
                ).help(
                    "S. Saves the draft as a proposal. Saving never reviews: review each saved "
                        + "record separately.")
                Button {
                    Task { if !(await session.saveCurrent(advance: true)) { NSSound.beep() } }
                } label: {
                    shortcutLabel("Save and next", key: "x")
                }.disabled(
                    !physical.canEdit || physical.needsReload
                        || session.sampleIndex >= session.samples.count - 1
                ).help("X. Saves any draft, then goes to the next frame.")
            }
            Text(
                "Edit → Save proposal → inspect in both views → Review. Reviewing a mask does not review a pose."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    // MARK: Repair

    /// A record that no longer holds against membership, with the two
    /// repairs: remove it, or move the object's references to another object.
    private func staleRow(_ problem: PhysicalLinkProblem) -> some View {
        let objectID = problem.record.split(separator: "\"").dropFirst().first.map(String.init)
        return VStack(alignment: .leading, spacing: 2) {
            diagnostic("\(problem.record): \(problem.problem)", colour: .orange)
            HStack {
                Button("Remove record", role: .destructive) {
                    physical.edit { _ = PhysicalDraft.remove(problem: problem, from: &$0) }
                }
                if let objectID {
                    Menu("Move to object…") {
                        ForEach(
                            session.sidecar.objects.filter {
                                $0.objectID != objectID && $0.status != .rejected
                            }
                        ) { target in
                            Button(session.displayName(objectID: target.objectID)) {
                                var refusal: String?
                                physical.edit {
                                    refusal = PhysicalDraft.reassign(
                                        from: objectID, to: target.objectID, in: &$0)
                                }
                                if let refusal { physical.refuse(refusal) }
                            }
                        }
                    }.menuStyle(.borderlessButton).fixedSize()
                }
            }.controlSize(.mini).disabled(!physical.canEdit)
        }
    }

    // MARK: Shared errors

    private static let components = [
        "position", "yaw", "anchor_offset", "length", "width", "height", "front", "rear",
    ]

    /// Names the observations several components rest on, so a reader can see
    /// that separate fields are not independent evidence.
    private func sharedErrorEditor(
        objectID: String, k: PhysicalKeyframe, sample: AnnotationSample
    ) -> some View {
        func update(_ change: @escaping (inout [PhysicalSharedError]) -> Void) {
            physical.edit { objects in
                PhysicalDraft.updateKeyframe(
                    objectID: objectID, sampleID: sample.sampleID, in: &objects
                ) {
                    var list = $0.sharedErrors ?? []
                    change(&list)
                    $0.sharedErrors = list.isEmpty ? nil : list
                }
            }
        }
        let list = k.sharedErrors ?? []
        return DisclosureGroup("Shared errors (\(list.count))") {
            ForEach(Array(list.enumerated()), id: \.offset) { index, item in
                VStack(alignment: .leading, spacing: 2) {
                    TextField(
                        "Observation",
                        text: Binding(
                            get: { item.observation },
                            set: { v in update { $0[index].observation = v } })
                    ).textFieldStyle(.roundedBorder).font(.caption2)
                    FlowToggles(options: Self.components, selected: Set(item.components)) {
                        component, on in
                        update {
                            var set = Set($0[index].components)
                            if on { set.insert(component) } else { set.remove(component) }
                            $0[index].components = set.sorted()
                        }
                    }
                    if item.components.count < 2 {
                        Text("Link at least two components.").font(.caption2).foregroundStyle(
                            .orange)
                    }
                    Button("Remove", role: .destructive) { update { $0.remove(at: index) } }
                        .controlSize(.mini)
                }
            }
            Button("Add shared error") {
                update { $0.append(PhysicalSharedError(observation: "", components: [])) }
            }.controlSize(.mini)
        }.font(.caption).disabled(!physical.canEdit)
    }

    // MARK: History

    @State private var showHistory = false
    @State private var confirmRestore: Int?

    private var historySection: some View {
        DisclosureGroup("History", isExpanded: $showHistory) {
            if physical.history.isEmpty {
                Text("No saved revisions.").font(.caption2).foregroundStyle(.secondary)
            }
            ForEach(physical.history) { r in
                HStack(alignment: .top) {
                    VStack(alignment: .leading, spacing: 0) {
                        Text(
                            "r\(r.revision)\(r.head ? " · current" : "") · \(r.change.operation ?? "")"
                        ).font(.caption2.bold())
                        Text("\(r.change.author) · \(r.updatedUTC.prefix(19))").font(.caption2)
                            .foregroundStyle(.secondary)
                        Text(
                            "\(r.objects) objects · \(r.keyframes) keyframes · \(r.reviewed) reviewed"
                        ).font(.caption2).foregroundStyle(.secondary)
                    }
                    Spacer()
                    if !r.head {
                        Button("Restore") { confirmRestore = r.revision }.controlSize(.mini)
                            .disabled(!physical.canEdit || physical.isDirty)
                    }
                }
            }
        }.font(.caption).onChange(of: showHistory) { _, open in
            if open { Task { await physical.loadHistory() } }
        }.onChange(of: physical.state?.revision) { _, _ in
            if showHistory { Task { await physical.loadHistory() } }
        }.alert(
            "Restore revision \(confirmRestore ?? 0)?",
            isPresented: Binding(
                get: { confirmRestore != nil }, set: { if !$0 { confirmRestore = nil } })
        ) {
            Button("Cancel", role: .cancel) { confirmRestore = nil }
            Button("Restore as a new revision") {
                if let revision = confirmRestore {
                    Task { await physical.restore(revision: revision) }
                }
                confirmRestore = nil
            }
        } message: {
            Text(
                "The current revision stays in history. Restored records keep their review, origin and the membership they were reviewed against."
            )
        }
    }

    // MARK: Pieces

    private func reviewBadge(_ status: ReviewStatus) -> some View {
        Text(status == .reviewed ? "reviewed" : status == .rejected ? "rejected" : "proposed").font(
            .caption2
        ).padding(.horizontal, 5).padding(.vertical, 1).background(
            (status == .reviewed ? Color.green : Color.secondary).opacity(0.25), in: Capsule())
    }

    private func hint(_ text: String) -> some View {
        Text(text).font(.caption2).foregroundStyle(.orange).fixedSize(
            horizontal: false, vertical: true)
    }

    /// How a component is known, in words: what the fit or the action stated.
    private func evidenceCaption(
        _ status: PhysicalEvidence, _ support: PhysicalSupport
    ) -> some View {
        let frames = support.frameList.map(String.init).joined(separator: ", ")
        let text: String
        switch status {
        case .observed: text = "seen in sample\(support.frameList.count == 1 ? "" : "s") \(frames)"
        case .inferred:
            text =
                frames.isEmpty
                ? "inferred from \(support.external ?? "an external reference")"
                : "inferred from sample\(support.frameList.count == 1 ? "" : "s") \(frames)"
        case .priorOnly: text = "class prior: \(support.external ?? "unnamed")"
        case .unknown: text = "not stated"
        }
        return Text(text).font(.caption2.monospacedDigit()).foregroundStyle(.secondary)
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

/// A wrapping row of toggles, one per option.
struct FlowToggles: View {
    let options: [String]
    let selected: Set<String>
    let set: (String, Bool) -> Void

    var body: some View {
        LazyVGrid(
            columns: [GridItem(.adaptive(minimum: 78), alignment: .leading)], alignment: .leading,
            spacing: 2
        ) {
            ForEach(options, id: \.self) { option in
                Toggle(
                    option.replacingOccurrences(of: "_", with: " "),
                    isOn: Binding(get: { selected.contains(option) }, set: { set(option, $0) })
                ).toggleStyle(.checkbox).font(.caption2)
            }
        }
    }
}
