// AnnotationPane.swift
// The controls beside the views: what the gesture means, what is drawn, and
// the review gate a mask has to pass before it counts as reference truth.
//
// The pane decides nothing about membership. It sets the mode and the tool and
// asks the session; the overlay in LassoOverlay.swift turns the drag into
// metres.

import AppKit
import SwiftUI
import simd

/// The annotation controls: object identity, view, slab, review and save.
struct AnnotationPane: View {
    /// The controls are two columns either side of the views, not one.
    enum Column {
        /// What is being labelled: the run and frame, progress, proposals and
        /// objects.
        case objects
        /// How: display, tools, slab, carrying, saving and review.
        case editing
    }

    static let columnWidth: CGFloat = 280

    @ObservedObject var session: AnnotationSession
    var column: Column = .objects

    /// Only used to write the map-marks.json line, which is why it is the
    /// view's and not the session's: the session holds what it can measure,
    /// not what the operator knows about where the tripod stood.
    @State private var mapMarksSiteID = ""
    /// Objects ticked to be merged into the one under edit.
    @State private var mergeSelection: Set<String> = []

    @State private var newObjectClass = "car"
    @State private var newObjectSubtype = ""
    @State private var showDiscardPrompt = false

    @State private var applyResult: String?
    @State private var showReviewAllPrompt = false
    @State private var proposalClass = "car"
    @State private var proposalSort = ProposalSort.mostFrames
    @State private var proposalFilter = ProposalFilter()
    @State private var splitClass = "pedestrian"

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                switch column {
                case .objects:
                    sourceSection
                    Divider()
                    progressSection
                    Divider()
                    proposalSection
                    Divider()
                    objectSection
                case .editing:
                    displaySection
                    Divider()
                    selectionSection
                    Divider()
                    slabSection
                    if session.activeObjectID != nil {
                        Divider()
                        propagationSection
                    }
                    if session.carried != nil {
                        Divider()
                        carriedSection
                    }
                    Divider()
                    reviewSection
                }
            }.padding(12)
        }.frame(width: AnnotationPane.columnWidth).alert(
            "Unsaved membership", isPresented: $showDiscardPrompt
        ) {
            Button("Keep editing", role: .cancel) {}
            Button("Discard and reload", role: .destructive) { session.reload() }
        } message: {
            Text("This frame has unsaved changes. Save it, or discard them, before moving on.")
        }
    }

    // MARK: Source

    // The main view's words, for the things that are the same thing: a run,
    // a frame, a label and who made it. "Source", "sample" and "operator" were
    // this window's own, and an operator who knew the main view had to work
    // out that a sample is a frame and that the operator field wanted their
    // name and not the name of a track.
    private var sourceSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Run").font(.headline)
            Text(session.pack.manifest.source.vrlogPath).font(.caption.monospaced()).textSelection(
                .enabled
            ).lineLimit(1).truncationMode(.middle).help(
                "The run this pack was cut from. Pack \(session.pack.manifest.datasetID).")
            if let pcap = session.pack.manifest.source.pcapBasename {
                Text(pcap).font(.caption2).foregroundStyle(.secondary).lineLimit(1).truncationMode(
                    .middle)
            }
            // Coverage is shown before any labelling: a foreground-only pack
            // cannot support whole-scene segmentation, and a mask saved in
            // ignorance of that reads as a stronger claim than it is.
            Text(session.pack.coverageCaveat).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)

            if let sample = session.currentSample {
                // The run's own frame number first: it is the one the main
                // view's timeline shows.
                Text(
                    "Frame \(sample.sourceOrdinal) · \(session.sampleIndex + 1) of "
                        + "\(session.samples.count) in this pack · \(session.currentPoints.count) points"
                ).font(.caption)
            }
            backgroundLine
            HStack {
                Button("Previous frame") { handleStep { session.stepBackward() } }.disabled(
                    session.sampleIndex == 0)
                Button("Next frame") { handleStep { session.stepForward() } }.disabled(
                    session.sampleIndex >= session.samples.count - 1)
            }.controlSize(.small)

            Text("Labelled by").font(.caption).padding(.top, 4)
            TextField("Your name", text: $session.operatorName).textFieldStyle(.roundedBorder).font(
                .caption
            ).help(
                "Who is labelling. Saved with every label you make, as the main view's track "
                    + "labels are. Not the name of an object or a track.")
        }
    }

    // Which settled background is behind this frame, and how long it has been.
    private var backgroundLine: some View {
        Group {
            if let background = session.currentBackground, let sample = session.currentSample {
                let age = sample.sourceOrdinal - background.sourceOrdinal
                HStack(spacing: 4) {
                    Image(systemName: "square.stack.3d.down.forward")
                    Text(
                        "Settled background \(background.backgroundID + 1) of "
                            + "\(session.pack.backgrounds.count) · \(background.pointCount) points · "
                            + (age == 0 ? "this frame" : "\(age) frames old"))
                }.font(.caption2).foregroundStyle(
                    session.backgroundUpdatedHere ? Color.yellow : Color.secondary)
            } else {
                Text("No settled background in this pack. Generate it again to carry one.").font(
                    .caption2
                ).foregroundStyle(.secondary)
            }
        }
    }

    // MARK: Progress

    private var progressSection: some View {
        let completeness = session.completeness
        return VStack(alignment: .leading, spacing: 6) {
            Text("This frame").font(.headline)
            HStack(alignment: .center, spacing: 10) {
                SectorRing(completeness: completeness) { session.fitViews(toSector: $0) }.frame(
                    width: 118, height: 118)
                VStack(alignment: .leading, spacing: 3) {
                    Text(String(format: "%.0f%% agreed", completeness.whole.fractionAgreed * 100))
                        .font(.caption.bold())
                    tallyLine(
                        "agreed", completeness.whole.agreed, AnnotationFrameStrip.agreedColour)
                    tallyLine(
                        "in question", completeness.whole.inQuestion,
                        AnnotationFrameStrip.inQuestionColour)
                    tallyLine("not labelled", completeness.whole.unlabelled, .gray)
                }
            }
            Text(
                "Foreground the tracker could use, in 22.5° sectors round the sensor, laid out as "
                    + "the top view is. Agreed is labelled by a person; in question is proposed or "
                    + "unsaved. Click a sector to go to what is left in it."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    private func tallyLine(_ label: String, _ count: Int, _ colour: Color) -> some View {
        HStack(spacing: 4) {
            Circle().fill(colour).frame(width: 7, height: 7)
            Text("\(count) \(label)").font(.caption2.monospacedDigit()).foregroundStyle(.secondary)
        }
    }

    // MARK: Proposals

    /// The proposals worth an operator's attention first: fixed clutter, and
    /// what moved some distance or lasted a couple of seconds. The rest is
    /// mostly the speckle of every frame, and is there when asked for.
    private var listedProposals: [ObjectProposal] {
        proposalSort.sorted(session.proposals.filter(proposalFilter.admits))
    }

    private var proposalSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Proposed objects").font(.headline)
            if let frame = session.proposalProgress {
                HStack(spacing: 6) {
                    ProgressView().controlSize(.small)
                    Text("Reading the pack · \(frame + 1) of \(session.samples.count)").font(
                        .caption.monospacedDigit())
                }
            } else {
                Button(session.proposals.isEmpty ? "Propose objects" : "Propose more") {
                    Task { await session.proposeObjects() }
                }.controlSize(.small).help(
                    "Finds what nothing yet covers: clutter that stays put, as one proposal "
                        + "a patch, and everything else as one proposal an object, followed "
                        + "through the frames. You grade each once. Asking again adds to the "
                        + "list; it never clears what is already there.")
            }

            if !session.proposals.isEmpty {
                let listed = listedProposals
                Text(
                    "\(listed.count) listed of \(session.proposals.count). Click one, step through "
                        + "its frames, then accept, split, merge or reject it."
                ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
                HStack(spacing: 4) {
                    Picker("Sort", selection: $proposalSort) {
                        ForEach(ProposalSort.allCases) { Text($0.label).tag($0) }
                    }.labelsHidden()
                    Picker("Type", selection: $proposalFilter.type) {
                        Text("All types").tag(String?.none)
                        ForEach(ProposalFilter.types(in: session.proposals), id: \.name) { type in
                            Text("\(type.name) (\(type.count))").tag(String?.some(type.name))
                        }
                    }.labelsHidden()
                }.controlSize(.small).font(.caption2)
                Toggle("List the small and short ones too", isOn: $proposalFilter.includeSmall)
                    .font(.caption2)
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 2) {
                        ForEach(listed) { proposal in proposalRow(proposal) }
                    }
                }.frame(maxHeight: 170)
                if let selected = session.selectedProposal { proposalActions(selected) }
            }
        }
    }

    private func proposalRow(_ proposal: ObjectProposal) -> some View {
        let isSelected = session.selectedProposalID == proposal.id
        return HStack(spacing: 5) {
            RoundedRectangle(cornerRadius: 2).fill(
                AnnotationPalette.colour(forClass: proposal.classGuess)
            ).frame(width: 8, height: 8)
            Text(
                "\(proposal.kind == .fixed ? "fixed" : "≈ " + proposal.classGuess) · frames "
                    + "\(proposal.firstFrame + 1)–\(proposal.lastFrame + 1) · ~\(proposal.meanPoints) pts"
                    + (proposal.kind == .moving
                        ? String(format: " · %.0f m", proposal.travelled) : "")
                    + (proposalSort == .steadiest
                        ? String(format: " · ±%.0f%%", proposal.unsteadiness * 100) : "")
            ).font(.caption2.monospacedDigit()).lineLimit(1)
            Spacer(minLength: 0)
        }.padding(.vertical, 2).padding(.horizontal, 4).background(
            isSelected ? Color.accentColor.opacity(0.25) : Color.clear,
            in: RoundedRectangle(cornerRadius: 3)
        ).contentShape(Rectangle()).onTapGesture {
            proposalClass = proposal.classGuess
            session.selectProposal(isSelected ? nil : proposal.id)
        }
    }

    // Grading one proposal. Split is by the frame on screen, because where a
    // chain ran from one car onto the next is something the operator sees
    // while stepping through it, not a number they know.
    private func proposalActions(_ proposal: ObjectProposal) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 4) {
                Picker("Class", selection: $proposalClass) {
                    ForEach(AnnotationPalette.classes) { Text($0.name).tag($0.name) }
                }.labelsHidden().font(.caption).frame(maxWidth: 110)
                Button("Accept") {
                    _ = session.acceptProposal(proposal.id, objectClass: proposalClass)
                }
                Button("Noise") { _ = session.acceptProposal(proposal.id, objectClass: "noise") }
                    .help("It is not an object: label its points as noise")
            }.controlSize(.small)
            HStack(spacing: 4) {
                Button("Accept up to here") {
                    _ = session.acceptProposal(
                        proposal.id, objectClass: proposalClass,
                        frames: proposal.firstFrame...session.sampleIndex)
                }.disabled(session.sampleIndex < proposal.firstFrame)
                Button("From here") {
                    _ = session.acceptProposal(
                        proposal.id, objectClass: proposalClass,
                        frames: session.sampleIndex...max(session.sampleIndex, proposal.lastFrame))
                }.disabled(session.sampleIndex > proposal.lastFrame)
            }.controlSize(.small).help(
                "Split: accept part of it, by the frame on screen. The rest stays proposed.")
            HStack(spacing: 4) {
                if let active = session.activeObjectID {
                    Button("Add to \(session.displayName(objectID: active))") {
                        _ = session.acceptProposal(
                            proposal.id, objectClass: proposalClass, into: active)
                    }.help(
                        "Merge: these are more frames of the object being edited. Frames it "
                            + "already has are left alone.")
                }
                Button("Dismiss") { session.dismissProposal(proposal.id) }
            }.controlSize(.small)
        }
    }

    // MARK: Display

    private var displaySection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Display").font(.headline)
            // The recorder's own classes, in the main view's colours. What is
            // hidden cannot be selected, so switching the background off is
            // also how a lasso is kept from taking the wall behind a van.
            let counts = session.classCounts
            HStack(spacing: 4) {
                Toggle(
                    "Background \(counts[PointClass.background, default: 0])",
                    isOn: $session.visibility.background)
                Toggle(
                    "Foreground \(counts[PointClass.foreground, default: 0])",
                    isOn: $session.visibility.foreground)
                Toggle(
                    "Ground \(counts[PointClass.ground, default: 0])",
                    isOn: $session.visibility.ground)
            }.toggleStyle(.button).controlSize(.small).font(.caption2)

            // The three states the progress bars count. Switching the settled
            // ones off leaves the work still to do on its own, and because a
            // hidden return cannot be selected, a lasso over what is left
            // cannot take back what is already agreed.
            let tally = session.completeness.whole
            HStack(spacing: 4) {
                Toggle("Agreed \(tally.agreed)", isOn: $session.visibility.agreed)
                Toggle("In question \(tally.inQuestion)", isOn: $session.visibility.inQuestion)
                Toggle("Unlabelled \(tally.unlabelled)", isOn: $session.visibility.unlabelled)
            }.toggleStyle(.button).controlSize(.small).font(.caption2).help(
                "Agreed is reviewed or labelled by hand; in question is saved but still the "
                    + "algorithm's word for it")

            // Ground is what the run's height band took out before clustering,
            // so that what is left on screen is what the tracker had to work
            // with.
            Text(groundCaption).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            if counts[PointClass.background, default: 0] == 0,
                session.pack.manifest.hasClassification
            {
                Text("No background in this sample: the recording kept foreground returns only.")
                    .font(.caption2).foregroundStyle(.secondary).fixedSize(
                        horizontal: false, vertical: true)
            }

            // The views hold their framing from one sample to the next. These
            // are the only things that move them, other than the operator.
            HStack(spacing: 4) {
                Text("Fit").font(.caption)
                Button("Sample") { session.fitViews(to: .sample) }
                Button("Foreground") { session.fitViews(to: .foreground) }.disabled(
                    !session.pack.manifest.hasClassification)
                Button("Selection") { session.fitViews(to: .selection) }.disabled(
                    session.selectionCount == 0)
            }.controlSize(.small)
            Text(
                "Scroll or pinch to zoom. Drag with the right button, or with control held, to pan."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    private var groundCaption: String {
        let band = session.heightBand
        guard band.removeGround else {
            return "Ground: this run removed nothing by height, so only recorded ground counts."
        }
        let rule = String(
            format: "Ground: below %.2f m or above %.2f m, what the run's height band removed",
            band.floorM, band.ceilingM)
        return session.heightBandIsAssumed
            ? rule + ". Assumed: this pack does not record its run's band. Generate it again to get"
                + " the run's own." : rule + "."
    }

    // MARK: Object

    private var objectSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Objects").font(.headline)
            // Said once, here, because nothing else on screen says it.
            Text(
                "An object is one real thing followed through the frames: this car, that wall. You "
                    + "label its points in each frame. It is your answer, where a track in the "
                    + "main view is the tracker's, and the two are kept apart so that a split "
                    + "track cannot split one. Click an object to edit it."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)

            HStack(spacing: 6) {
                RoundedRectangle(cornerRadius: 2).fill(
                    AnnotationPalette.colour(forClass: newObjectClass)
                ).frame(width: 10, height: 10)
                Picker("Class", selection: $newObjectClass) {
                    // Research subtypes stay in their own field: a dataset
                    // export must not enable a reserved production enum value
                    // through the class picker.
                    Section("Moves") {
                        ForEach(AnnotationPalette.classes.filter { $0.kind == .moving }) {
                            Text($0.name).tag($0.name)
                        }
                    }
                    Section("Does not move: misread as foreground") {
                        ForEach(AnnotationPalette.classes.filter { $0.kind == .fixed }) {
                            Text($0.name).tag($0.name)
                        }
                    }
                }.labelsHidden().font(.caption)
            }
            if let label = AnnotationPalette.annotationClass(named: newObjectClass)?.mainViewLabel {
                Text("The main view labels a track of this \"\(label)\".").font(.caption2)
                    .foregroundStyle(.secondary)
            }
            TextField("Subtype (optional)", text: $newObjectSubtype).textFieldStyle(.roundedBorder)
                .font(.caption)
            Button(session.selectionCount > 0 ? "New object from selection" : "New object") {
                // Refused with unsaved changes on another object: creating
                // one makes it the object under edit.
                guard session.activeObjectID == nil || session.navigationGuard() == nil else {
                    showDiscardPrompt = true
                    return
                }
                _ = session.createObject(
                    objectClass: newObjectClass,
                    subtype: newObjectSubtype.isEmpty ? nil : newObjectSubtype)
                session.secondViewChecked = false
            }

            if session.sidecar.objects.isEmpty {
                Text("No objects yet").font(.caption).foregroundStyle(.secondary)
            } else {
                ForEach(session.sidecar.objects) { object in objectRow(object) }
                mergeBar
            }

            if let active = session.activeObject, !session.removedFromSaved.isEmpty,
                session.selectionCount > 0
            {
                splitControls(active)
            }

            if let active = session.activeObject, AnnotationPalette.isFixed(active.objectClass) {
                Button("Apply to every frame") {
                    applyResult = session.applySelectionToAllSamples().map {
                        "Saved \(session.displayName(objectID: active.objectID)) into \($0) frames."
                    }
                }.disabled(session.selectionCount == 0).help(
                    "A \(active.objectClass) does not move. Saves its points into every frame from "
                        + "the space this selection occupies, leaving frames already labelled alone."
                )
                if let applyResult { Text(applyResult).font(.caption2).foregroundStyle(.secondary) }
            }
        }
    }

    private func objectRow(_ object: AnnotationObject) -> some View {
        HStack(spacing: 6) {
            Image(
                systemName: session.activeObjectID == object.objectID
                    ? "largecircle.fill.circle" : "circle")
            RoundedRectangle(cornerRadius: 2).fill(
                AnnotationPalette.colour(forClass: object.objectClass)
            ).frame(width: 10, height: 10)
            VStack(alignment: .leading, spacing: 0) {
                Text(
                    session.displayName(objectID: object.objectID)
                        + (object.subtype.map { " · \($0)" } ?? "")
                ).font(.caption).bold(session.activeObjectID == object.objectID)
                // How far through the pack the object has been followed.
                Text(
                    "labelled in \(session.savedSampleCount(objectID: object.objectID)) of "
                        + "\(session.samples.count) frames · "
                        + "\(session.reviewedSampleCount(objectID: object.objectID)) reviewed"
                ).font(.caption2).foregroundStyle(.secondary).help(object.objectID)
            }
            Spacer()
            // Of its frames, not of the object record: an object marked
            // reviewed whose frames are not has nothing that counts.
            let saved = session.savedSampleCount(objectID: object.objectID)
            let reviewed = session.reviewedSampleCount(objectID: object.objectID)
            Text(
                saved > 0 && reviewed == saved
                    ? "reviewed" : reviewed > 0 ? "part reviewed" : "proposed"
            ).font(.caption2).foregroundStyle(
                saved > 0 && reviewed == saved ? Color.green : Color.secondary)
            mergeTick(object)
        }.contentShape(Rectangle()).onTapGesture {
            if session.activate(objectID: object.objectID) != nil {
                showDiscardPrompt = true
                return
            }
            session.secondViewChecked = false
            applyResult = nil
        }
    }

    /// A tick to fold this object into the one under edit.
    ///
    /// On the row, and visible rather than behind a right-click, because a
    /// merge needs two objects named and this row is one of them. Several can
    /// be ticked: a chain that broke twice comes back as three.
    @ViewBuilder private func mergeTick(_ object: AnnotationObject) -> some View {
        if let active = session.activeObjectID, active != object.objectID {
            Toggle(
                isOn: Binding(
                    get: { mergeSelection.contains(object.objectID) },
                    set: { on in
                        if on {
                            mergeSelection.insert(object.objectID)
                        } else {
                            mergeSelection.remove(object.objectID)
                        }
                    })
            ) { Image(systemName: "arrow.triangle.merge") }.toggleStyle(.button).controlSize(.small)
                .help(
                    "Fold \(session.displayName(objectID: object.objectID)) into "
                        + session.displayName(objectID: active))
        }
    }

    /// The merge itself, shown only once something is ticked.
    @ViewBuilder private var mergeBar: some View {
        if let active = session.activeObjectID, !mergeSelection.isEmpty {
            let ticked = mergeSelection.subtracting([active])
            if !ticked.isEmpty {
                HStack(spacing: 6) {
                    Button("Merge \(ticked.count) into \(session.displayName(objectID: active))") {
                        if let frames = session.mergeObjects(Array(ticked), into: active) {
                            applyResult =
                                "Merged into \(session.displayName(objectID: active)): "
                                + "\(frames) frames, back in question until reviewed."
                        }
                        mergeSelection = []
                    }
                    Button("Clear") { mergeSelection = [] }
                }.controlSize(.small).font(.caption2)
                Text(
                    "Every frame of them becomes a frame of "
                        + "\(session.displayName(objectID: active)), and they are gone. Merged "
                        + "frames go back in question."
                ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
            }
        }
    }

    // MARK: Propagation

    // One press for the frames an operator would only have agreed with, one
    // at a time, at six seconds each.
    private var propagationSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Carry through the frames").font(.headline)
            if let frame = session.propagationProgress {
                HStack(spacing: 6) {
                    ProgressView().controlSize(.small)
                    Text("Frame \(frame + 1) of \(session.samples.count)").font(
                        .caption.monospacedDigit())
                    Spacer()
                    Button("Stop") { session.cancelPropagation() }.controlSize(.small)
                }
            } else {
                HStack(spacing: 4) {
                    Button("◀◀ Back") { Task { await session.propagate(direction: -1) } }
                        .keyboardShortcut("[", modifiers: .command)
                    Button("Forward ▶▶") { Task { await session.propagate(direction: 1) } }
                        .keyboardShortcut("]", modifiers: .command)
                }.controlSize(.small).disabled(session.selectionCount == 0)
            }
            if let outcome = session.lastPropagation {
                Text(outcome.summary).font(.caption2).foregroundStyle(
                    outcome.stop.needsOperator ? Color.orange : Color.secondary
                ).fixedSize(horizontal: false, vertical: true)
            }
            Text(
                "From this frame's saved points, on through every frame where the fit is one you "
                    + "would have accepted. It stops and hands over where the object is lost, "
                    + "doubles, could be in two places, or is further than it could have moved. "
                    + "What it writes is in question until you review it."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    // What was labelled as one object is two. The operator takes the second
    // one's points out of the first, which shows them ringed in red, and this
    // makes those a new object and divides the other frames the same way.
    private func splitControls(_ active: AnnotationObject) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(
                "\(session.removedFromSaved.count) points taken out of "
                    + "\(session.displayName(objectID: active.objectID)). If they are a second "
                    + "object, split them off."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            HStack(spacing: 4) {
                Picker("Class", selection: $splitClass) {
                    ForEach(AnnotationPalette.classes) { Text($0.name).tag($0.name) }
                }.labelsHidden().frame(maxWidth: 110)
                Button("Split off as new object") {
                    if let result = session.splitRemovedIntoNewObject(objectClass: splitClass) {
                        applyResult =
                            "Split \(session.displayName(objectID: result.objectID)) off through "
                            + "\(result.frames) frames."
                    }
                }
            }.controlSize(.small).help(
                "Divides this frame as you have, then carries the division through the other "
                    + "frames this object is labelled in, as far as the second object can be "
                    + "found. Every frame it changes goes back to proposed.")
            if let applyResult { Text(applyResult).font(.caption2).foregroundStyle(.secondary) }
        }.onAppear { splitClass = active.objectClass }
    }

    // MARK: Carried selection

    // The keys do the same from the editing view. These are here so that the
    // proposal can be dealt with without knowing the keys.
    private var carriedSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Carried from the last frame").font(.headline)
            Text(
                "\(session.carriedIndices.count) points, outlined in cyan. Move the outline over "
                    + "the object, then accept."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            HStack(spacing: 4) {
                let step = AnnotationSession.nudgeStep
                Button("◀") { session.nudgeCarried(right: -step, up: 0) }
                Button("▲") { session.nudgeCarried(right: 0, up: step) }
                Button("▼") { session.nudgeCarried(right: 0, up: -step) }
                Button("▶") { session.nudgeCarried(right: step, up: 0) }
                Button("Refit") { session.refitCarried() }.help(
                    "Search again for where the outline covers the most points, from here")
            }.controlSize(.small)
            HStack(spacing: 4) {
                Button("Accept and save") { if session.acceptCarried() { _ = session.save() } }
                    .keyboardShortcut(.return, modifiers: .command)
                Button("Accept") { session.acceptCarried() }
                Button("Dismiss") { session.dismissCarried() }
            }.controlSize(.small)
        }
    }

    // MARK: Selection

    private var selectionSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Selection").font(.headline)
            Picker("View", selection: $session.viewStandard) {
                ForEach(OrthoViewBasis.Standard.allCases, id: \.self) { Text($0.label).tag($0) }
            }.font(.caption)

            Picker("Tool", selection: $session.tool) {
                ForEach(SelectionTool.allCases) { Text($0.label).tag($0) }
            }.pickerStyle(.segmented).labelsHidden().onChange(of: session.tool) { _, _ in
                // Changing tool mid-stroke would apply one tool's gesture
                // under another's rule.
                session.cancelStroke()
            }

            Text("\(session.selectionCount) points selected").font(.caption)
            Text(session.tool.hint).font(.caption2).foregroundStyle(.secondary)
            legend

            switch session.tool {
            case .lasso: EmptyView()
            case .sphere: sphereControls
            case .column: columnControls
            }

            HStack {
                Button("Undo") { session.undo() }.disabled(!session.history.canUndo)
                Button("Redo") { session.redo() }.disabled(!session.history.canRedo)
                Button("Clear") { session.clearSelection() }.disabled(session.selectionCount == 0)
            }
        }
    }

    private var legend: some View {
        HStack(spacing: 8) {
            legendEntry(
                "saved",
                session.activeObject.map { AnnotationPalette.colour(forClass: $0.objectClass) }
                    ?? .gray)
            legendEntry("unsaved", AnnotationPalette.colour(AnnotationPalette.unsavedIndex))
            legendEntry("removed", AnnotationPalette.colour(AnnotationPalette.removedIndex))
            legendEntry("proposed", .cyan)
        }
    }

    private func legendEntry(_ label: String, _ colour: Color) -> some View {
        HStack(spacing: 3) {
            Circle().fill(colour).frame(width: 7, height: 7)
            Text(label).font(.caption2).foregroundStyle(.secondary)
        }
    }

    private var sphereControls: some View {
        HStack {
            Text("Radius").font(.caption)
            Slider(
                value: $session.sphereRadius,
                in: SelectionSphere.minimumRadius...SelectionSphere.maximumRadius)
            Text(String(format: "%.2f m", session.sphereRadius)).font(.caption.monospacedDigit())
                .frame(width: 52, alignment: .trailing)
        }.help("The brush's radius. [ and ] step it. Shift-scroll moves the brush in depth.")
    }

    private var columnControls: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("Brush").font(.caption)
                Slider(
                    value: $session.columnBrushRadius, in: 0...BrushStroke.maximumColumnRadius,
                    step: BrushStroke.columnRadiusStep)
                Text(
                    session.columnBrushRadius == 0
                        ? "1 column" : String(format: "%.2f m", session.columnBrushRadius)
                ).font(.caption.monospacedDigit()).frame(width: 60, alignment: .trailing)
            }

            // One toggle per voxel, ground at the left. The heights are the
            // point: "not the ground" is one click on the first of them.
            Text("Voxels, 0.5 m each from the ground").font(.caption2).foregroundStyle(.secondary)
            HStack(spacing: 3) {
                ForEach(0..<ColumnGrid.voxelCount, id: \.self) { k in
                    let bit = UInt8(1) << UInt8(k)
                    Toggle(
                        isOn: Binding(
                            get: { session.enabledVoxels & bit != 0 },
                            set: { on in
                                session.enabledVoxels =
                                    on ? session.enabledVoxels | bit : session.enabledVoxels & ~bit
                            })
                    ) { Text("\(k)").font(.caption2.monospacedDigit()).frame(width: 14) }
                    .toggleStyle(.button).controlSize(.small).help(
                        String(
                            format: "%.1f to %.1f m above the ground",
                            session.columnGrid.heightRange(ofVoxel: k).lowerBound,
                            session.columnGrid.heightRange(ofVoxel: k).upperBound))
                }
            }
            HStack {
                Button("Stack") { session.enabledVoxels = ColumnGrid.stackMask }.help(
                    "Voxels 1 to 4, 0.5 to 2.5 m: where road users are")
                Button("All") { session.enabledVoxels = ColumnGrid.allVoxelsMask }
            }.controlSize(.small)

            HStack {
                Text("Ground Z").font(.caption)
                Stepper(value: $session.columnGrid.groundZ, in: -20...20, step: 0.05) {
                    Text(String(format: "%.2f m", session.columnGrid.groundZ)).font(
                        .caption.monospacedDigit())
                }
                Button("Estimate") { session.estimateGround() }.controlSize(.small).help(
                    "The height below which a twentieth of this sample's returns lie")
            }
            HStack {
                Text("Grid angle").font(.caption)
                Stepper(value: $session.gridAzimuthDeg, in: 0...359, step: 1) {
                    Text(String(format: "%.0f°", session.gridAzimuthDeg)).font(
                        .caption.monospacedDigit())
                }.help("Turns the columns to follow the kerbs instead of the sensor's mounting")
                Button("Copy") {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(
                        session.mapMarksLine(siteID: mapMarksSiteID), forType: .string)
                }.controlSize(.small).help(
                    "Copies the map-marks.json line. That file owns this angle; this is a "
                        + "working value until it is recorded there.")
            }
            TextField("Site id for the copied line", text: $mapMarksSiteID).font(.caption2)
                .textFieldStyle(.roundedBorder).help(
                    "A pack records the sensor and the run, not which junction it stood at")

            Toggle("Show grid with other tools", isOn: $session.showColumnGrid).font(.caption)
        }
    }

    // MARK: Slab

    private var slabSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Depth slab").font(.headline)
            Text(session.viewStandard.depthAxisLabel).font(.caption).foregroundStyle(.secondary)
            if let slab = session.slab {
                // The slab is part of the selection rule, not a display
                // filter: it is what keeps a top-down lasso from taking a
                // lamp post along with the van beneath it.
                Text(String(format: "%.2f m to %.2f m", slab.minDepth, slab.maxDepth)).font(
                    .caption.monospaced())
                Slider(
                    value: Binding(
                        get: { Double(slab.minDepth) },
                        set: {
                            session.setSlab(DepthSlab(minDepth: Float($0), maxDepth: slab.maxDepth))
                        }), in: Double(slab.minDepth - 20)...Double(slab.maxDepth))
                Slider(
                    value: Binding(
                        get: { Double(slab.maxDepth) },
                        set: {
                            session.setSlab(DepthSlab(minDepth: slab.minDepth, maxDepth: Float($0)))
                        }), in: Double(slab.minDepth)...Double(slab.maxDepth + 20))
                Button("Reset to sample extent") { session.unpinSlab() }.font(.caption)
                if session.slabIsPinned {
                    Text("Kept from sample to sample until reset").font(.caption2).foregroundStyle(
                        .secondary)
                }
            } else {
                Text("No points in this sample").font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    // MARK: Review and save

    private var reviewSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Save").font(.headline)
            Picker("Visibility", selection: $session.maskVisibility) {
                ForEach(Visibility.allCases, id: \.self) { Text($0.label).tag($0) }
            }.font(.caption)
            Picker("Completeness", selection: $session.maskCompleteness) {
                ForEach(MaskCompleteness.allCases, id: \.self) { Text($0.label).tag($0) }
            }.font(.caption)

            // A membership seen from one angle has not been inspected for
            // contamination, so review is gated on the second view.
            Toggle("Checked from another view", isOn: $session.secondViewChecked).font(.caption)

            HStack {
                Button("Save points") { _ = session.save() }.keyboardShortcut(
                    "s", modifiers: .command)
                Button("Save and next") {
                    if session.save() { handleStep { session.stepForward() } }
                }.disabled(session.sampleIndex >= session.samples.count - 1)
            }
            HStack {
                Button("Review this frame") {
                    _ = session.markFrameReviewed(secondViewConfirmed: session.secondViewChecked)
                }.disabled(!session.secondViewChecked)
                Button("Review all frames…") { showReviewAllPrompt = true }.disabled(
                    !session.secondViewChecked || session.activeObjectID == nil)
            }
            if let id = session.activeObjectID {
                // Only a reviewed mask of a reviewed object is reference truth,
                // so this is the count that says how much of it there is.
                Text(
                    "\(session.reviewedSampleCount(objectID: id)) of "
                        + "\(session.savedSampleCount(objectID: id)) saved frames reviewed"
                ).font(.caption2).foregroundStyle(.secondary)
            }
            if session.navigationGuard() != nil {
                Text("Unsaved changes").font(.caption2).foregroundStyle(.orange)
            }
            Text("Revision \(session.sidecar.revision)").font(.caption2).foregroundStyle(.secondary)
        }.alert(
            "Review every frame of \(session.activeObjectName ?? "this object")?",
            isPresented: $showReviewAllPrompt
        ) {
            Button("Cancel", role: .cancel) {}
            Button("Mark all reviewed") {
                _ = session.markAllFramesReviewed(secondViewConfirmed: session.secondViewChecked)
            }
        } message: {
            Text(
                "This says you have been through every saved frame of this object and its points "
                    + "are right, including frames that were filled in for you. They count as "
                    + "reference truth from then on. Changing a frame's points afterwards puts "
                    + "that frame back to unreviewed.")
        }
    }

    private func handleStep(_ step: () -> AnnotationGuard?) {
        if step() != nil { showDiscardPrompt = true }
    }
}
