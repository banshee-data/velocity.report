// FreezeSplitSheet.swift
// Freezing a split from the annotation window.
//
// The window does not author the split: a draft file, in the CLI's format,
// says which packs, partitions and episodes it covers, by pack handle
// beneath the service's annotation folder. The window previews what the
// service would pin, including the physical revision and how much of it is
// reviewed, and only then freezes, through the same service rules the CLI
// applies. The preview is the freeze without the write.

import AppKit
import Combine
import SwiftUI
import UniformTypeIdentifiers

@MainActor final class FreezeSplitModel: ObservableObject {
    @Published var draftURL: URL?
    @Published var draft: SplitFreezeAPIClient.Draft?
    @Published var output = ""
    @Published var supersedes: String?
    @Published private(set) var preview: FreezePreview?
    /// The draft the preview describes; a changed draft needs a new preview.
    @Published private(set) var previewedDraft: URL?
    @Published private(set) var existing: [FrozenSplitListing] = []
    @Published private(set) var result: FreezeResult?
    @Published private(set) var error: String?
    @Published private(set) var busy = false

    private let client: SplitFreezeAPIClient
    private var generation = 0

    init(client: SplitFreezeAPIClient = SplitFreezeAPIClient()) { self.client = client }

    /// Whether the preview shown is of the draft chosen, and says it freezes.
    var canFreeze: Bool {
        guard let preview, previewedDraft == draftURL, draft != nil, !busy else { return false }
        return preview.wouldFreeze && !output.trimmingCharacters(in: .whitespaces).isEmpty
    }

    func chooseDraft(_ url: URL) {
        do {
            draft = try SplitFreezeAPIClient.loadDraft(at: url)
            draftURL = url
            preview = nil
            previewedDraft = nil
            result = nil
            error = nil
            if output.isEmpty {
                output = url.deletingPathExtension().lastPathComponent + "-frozen.json"
            }
        } catch {
            draft = nil
            draftURL = nil
            self.error = "Could not read the draft: \(error.localizedDescription)"
        }
    }

    func loadExisting() async {
        do { existing = try await client.splits() } catch {
            self.error = (error as? LocalizedError)?.errorDescription ?? String(describing: error)
        }
    }

    func runPreview() async {
        guard let draft, let draftURL else { return }
        generation &+= 1
        let asked = generation
        busy = true
        defer { if asked == generation { busy = false } }
        do {
            let p = try await client.preview(draft: draft, supersedes: supersedes)
            guard asked == generation else { return }
            preview = p
            previewedDraft = draftURL
            error = nil
        } catch {
            guard asked == generation else { return }
            preview = nil
            self.error = (error as? LocalizedError)?.errorDescription ?? String(describing: error)
        }
    }

    /// Freezes. Refused unless the preview shown is of this draft and says it
    /// would freeze: the button is never the only place the rules hold, but
    /// it never asks the service to write what the preview refused.
    @discardableResult func freeze(author: String) async -> Bool {
        guard canFreeze, let draft else { return false }
        guard !author.trimmingCharacters(in: .whitespaces).isEmpty else {
            error = "Enter your name under Labelled by: a frozen split says who answered for it."
            return false
        }
        generation &+= 1
        let asked = generation
        busy = true
        defer { if asked == generation { busy = false } }
        do {
            let r = try await client.freeze(
                draft: draft, author: author, output: output, supersedes: supersedes)
            guard asked == generation else { return false }
            result = r
            error = nil
            await loadExisting()
            return true
        } catch {
            guard asked == generation else { return false }
            self.error = (error as? LocalizedError)?.errorDescription ?? String(describing: error)
            return false
        }
    }
}

struct FreezeSplitSheet: View {
    @ObservedObject var session: AnnotationSession
    @StateObject private var model: FreezeSplitModel
    @Environment(\.dismiss) private var dismiss

    init(session: AnnotationSession, model: FreezeSplitModel? = nil) {
        self.session = session
        self._model = StateObject(wrappedValue: model ?? FreezeSplitModel())
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Freeze a split").font(.title3.bold())
            Text(
                "A draft (the CLI's format) names the packs, partitions and episodes by pack folder "
                    + "beneath the service's annotation folder. The preview is what the service would pin; "
                    + "the same rules refuse the freeze whether it is asked here or on the command line."
            ).font(.caption).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            HStack {
                Button("Choose Draft…") { chooseDraft() }
                Text(model.draftURL?.lastPathComponent ?? "no draft chosen").font(.caption)
                    .lineLimit(1).truncationMode(.middle)
                Spacer()
                Button("Preview") { Task { await model.runPreview() } }.disabled(
                    model.draft == nil || model.busy)
            }
            HStack {
                Text("Write as").font(.caption)
                TextField("name.json", text: $model.output).textFieldStyle(.roundedBorder).font(
                    .caption)
                Picker("Supersedes", selection: $model.supersedes) {
                    Text("nothing (new lineage)").tag(String?.none)
                    ForEach(model.existing.filter { $0.error == nil }) { s in
                        Text("\(s.name) · r\(s.revision)").tag(String?.some(s.name))
                    }
                }.font(.caption)
            }
            if let error = model.error {
                Text(error).font(.caption).foregroundStyle(.red).fixedSize(
                    horizontal: false, vertical: true
                ).textSelection(.enabled)
            }
            if let preview = model.preview { previewView(preview) }
            if let result = model.result {
                Label(
                    "Frozen \(result.name) · revision \(result.revision) · \(result.splitDigest)",
                    systemImage: "snowflake"
                ).font(.caption).foregroundStyle(.green).textSelection(.enabled)
            }
            Spacer(minLength: 0)
            HStack {
                if model.busy { ProgressView().controlSize(.small) }
                Spacer()
                Button("Close") { dismiss() }
                Button("Freeze") { Task { await model.freeze(author: session.operatorName) } }
                    .keyboardShortcut(.defaultAction).disabled(!model.canFreeze).help(
                        "Writes the split once, beneath the service's annotation folder. Never overwrites."
                    )
            }
        }.padding(16).frame(minWidth: 560, minHeight: 420).task { await model.loadExisting() }
    }

    private func chooseDraft() {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.json]
        panel.canChooseDirectories = false
        panel.message = "Choose a split draft (velocity.report/split-draft JSON)."
        guard panel.runModal() == .OK, let url = panel.url else { return }
        model.chooseDraft(url)
    }

    private func previewView(_ p: FreezePreview) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 6) {
                if p.wouldFreeze {
                    Label(
                        "Would freeze · split digest \(p.splitDigest ?? "")",
                        systemImage: "checkmark.seal"
                    ).font(.caption).foregroundStyle(.green).textSelection(.enabled)
                } else {
                    Label("Would not freeze", systemImage: "xmark.octagon").font(.caption.bold())
                        .foregroundStyle(.red)
                }
                if !p.membershipProblems.isEmpty {
                    Text("Membership review").font(.caption.bold())
                    ForEach(p.membershipProblems, id: \.self) {
                        Text($0).font(.caption2).foregroundStyle(.orange).fixedSize(
                            horizontal: false, vertical: true)
                    }
                }
                if !p.physicalProblems.isEmpty {
                    Text("Physical references").font(.caption.bold())
                    ForEach(p.physicalProblems, id: \.self) {
                        Text($0).font(.caption2).foregroundStyle(.orange).fixedSize(
                            horizontal: false, vertical: true)
                    }
                }
                ForEach(p.packs) { pack in packView(pack) }
            }
        }.frame(maxHeight: 260)
    }

    private func packView(_ pack: FreezePreview.Pack) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text("Pack \(pack.datasetID) · \(pack.packDigest)").font(.caption.bold()).lineLimit(1)
                .truncationMode(.middle)
            Text("membership revision \(pack.sidecarRevision) · \(pack.sidecarSHA256)").font(
                .caption2
            ).lineLimit(1).truncationMode(.middle)
            Text(pack.objects.map { "\($0.objectID) → \($0.partition)" }.joined(separator: ", "))
                .font(.caption2).foregroundStyle(.secondary)
            if let physical = pack.physical {
                Text("physical revision \(physical.revision) · \(physical.sha256)").font(.caption2)
                    .lineLimit(1).truncationMode(.middle)
                ForEach(physical.objects) { o in
                    Text(
                        "\(session.sidecar.objects.contains { $0.objectID == o.objectID } ? session.displayName(objectID: o.objectID) : o.objectID): "
                            + "body \(o.body)\(o.bodyIndependent ? "" : " (not independent)") · "
                            + "\(o.keyframes) keyframes, \(o.reviewedKeyframes) reviewed, \(o.proposedKeyframes) proposed, "
                            + "\(o.trackerAssistedKeyframes) tracker-assisted"
                    ).font(.caption2)
                }
                if !physical.coverage.isEmpty {
                    Text(
                        "Unavailable in reviewed keyframes: "
                            + physical.coverage.sorted { $0.key < $1.key }.map {
                                "\($0.key) \($0.value)"
                            }.joined(separator: ", ")
                    ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                        horizontal: false, vertical: true)
                }
            } else {
                Text("no physical references: the split pins membership only").font(.caption2)
                    .foregroundStyle(.orange)
            }
        }.padding(6).background(
            Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 4))
    }
}
