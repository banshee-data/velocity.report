//
//  MigratedPackSuccessorTests.swift
//  VelocityVisualiserTests
//
//  A version 1 pack finds the version 2 pack its labels were migrated to: by
//  the label set's migrated_from.pack_dir, never by the copy's name. A set
//  migrated from another pack is not a match, and a broken link beside the
//  pack is passed over. The read-only banner then names the copy and its
//  label set, and the window opens it through the same grant path as a
//  generated pack.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

private let everything = SelectionPolygon(
    rectFrom: simd_float2(-100, -100), to: simd_float2(100, 100))

/// What `velocity lidar labels migrate` records of a version 1 pack.
func migratedFrom(packDir: String, revision: Int = 2232) throws -> LabelSetMigration {
    let text = """
        {"pack_digest": "sha256:95be6fb2", "dataset_id": "ds_synthetic",
         "pack_dir": "\(packDir)", "sidecar_revision": \(revision),
         "pcap_sha256": "sha256:2864ebde", "pcap_name": "kirk0.pcapng",
         "decoder_build": "0.5.1-pre45", "mapping_digest": "sha256:e9d93b33"}
        """
    return try LabelSetMigration(json: LabelSetJSON.parse(Data(text.utf8)), "test migration")
}

/// A label set's labels.json alone, which is all discovery reads, and a
/// pack directory linking to it.
private func linkedLabelSet(
    in folder: URL, pack: String, labelSet name: String, migratedFrom packDir: String?,
    revision: Int = 2232
) throws {
    let labels = folder.appendingPathComponent("labels").appendingPathComponent(name)
    try FileManager.default.createDirectory(at: labels, withIntermediateDirectories: true)
    let id = "id-\(name)"
    var manifest = LabelSetManifest(labelSetID: id, name: name)
    manifest.head = LabelSetHead(revision: revision, state: "sha256:00", updatedUTC: "")
    manifest.migratedFrom = try packDir.map { try migratedFrom(packDir: $0) }
    try LabelSetStore.writeAtomic(labels, LabelSetFiles.manifest, manifest.json.indentedData)
    let packDirectory = folder.appendingPathComponent(pack)
    try FileManager.default.createDirectory(at: packDirectory, withIntermediateDirectories: true)
    try LabelSetLink(labelSetID: id, path: "../labels/\(name)").write(packDirectory: packDirectory)
}

/// A version 1 pack and, beside it, an openable version 2 copy whose label
/// set says it was migrated from the version 1 pack, as the migration
/// leaves them.
@MainActor func migratedPackPair(
    successor name: String = "kirk0-v1-v2"
) throws -> (folder: URL, v1: URL, v2: URL) {
    let folder = labelSetTemporaryDirectory()
    let samples: [[SyntheticPack.Point]] = [[(0, 0, 0, 1)], [(1, 0, 0, 1)]]
    let v1 = try SyntheticPack.write(
        samples, directory: folder.appendingPathComponent("kirk0-v1"), version: 1)
    let v2 = try SyntheticPack.write(samples, directory: folder.appendingPathComponent(name))
    let session = try AnnotationSession(pack: try AnnotationPack.open(directory: v2))
    session.operatorName = "dd"
    _ = session.createObject(objectClass: "car")
    #expect(session.select(polygon: everything, mode: .replace))
    #expect(session.save())
    let link = try #require(try LabelSetLink.read(packDirectory: v2))
    let labels = link.directory(packDirectory: v2)
    var manifest = try LabelSetManifest.read(
        try LabelSetStore.readFile(labels, LabelSetFiles.manifest))
    manifest.migratedFrom = try migratedFrom(packDir: "kirk0-v1")
    try LabelSetStore.writeAtomic(labels, LabelSetFiles.manifest, manifest.json.indentedData)
    return (folder, v1, v2)
}

struct MigratedPackSearchTests {
    @Test func theSetMigratedFromThisPackNamesItsSuccessor() throws {
        let folder = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: folder) }
        try FileManager.default.createDirectory(
            at: folder.appendingPathComponent("kirk0-ad8b9438"), withIntermediateDirectories: true)
        // Named by the operator, not "<dir>-v2": found by the label set.
        try linkedLabelSet(
            in: folder, pack: "kirk0-relabelled", labelSet: "kirk0-reference",
            migratedFrom: "kirk0-ad8b9438")

        let search = MigratedPackSearch.beside(folder.appendingPathComponent("kirk0-ad8b9438"))
        let successor = try #require(search.successor)
        #expect(successor.packDirectory.lastPathComponent == "kirk0-relabelled")
        #expect(successor.labelSetName == "kirk0-reference")
        #expect(successor.labelSetRevision == 2232)
    }

    @Test func aFolderWithoutAMigratedCopyFindsNothing() throws {
        let folder = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let v1 = folder.appendingPathComponent("kirk0-ad8b9438")
        try FileManager.default.createDirectory(at: v1, withIntermediateDirectories: true)
        try FileManager.default.createDirectory(
            at: folder.appendingPathComponent("other-pack"), withIntermediateDirectories: true)
        // A version 2 pack linked to a set that was never migrated.
        try linkedLabelSet(
            in: folder, pack: "fresh-v2", labelSet: "fresh", migratedFrom: nil, revision: 3)
        #expect(MigratedPackSearch.beside(v1) == .notFound)
    }

    @Test func aSetMigratedFromAnotherPackIsNotThisPacksSuccessor() throws {
        let folder = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let v1 = folder.appendingPathComponent("columbus-055307")
        try FileManager.default.createDirectory(at: v1, withIntermediateDirectories: true)
        try linkedLabelSet(
            in: folder, pack: "columbus-191749-v2", labelSet: "columbus-1917",
            migratedFrom: "columbus-191749")
        #expect(MigratedPackSearch.beside(v1) == .notFound)
        // The other pack does find it.
        #expect(
            MigratedPackSearch.beside(folder.appendingPathComponent("columbus-191749")).successor?
                .labelSetName == "columbus-1917")
    }

    @Test func anUnreadableLinkIsPassedOver() throws {
        let folder = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let v1 = folder.appendingPathComponent("kirk0-v1")
        try FileManager.default.createDirectory(at: v1, withIntermediateDirectories: true)
        // Not JSON.
        let broken = folder.appendingPathComponent("a-broken")
        try FileManager.default.createDirectory(at: broken, withIntermediateDirectories: true)
        try Data("{ not json".utf8).write(to: broken.appendingPathComponent(LabelSetLink.filename))
        // A link to a label set that is not there.
        let dangling = folder.appendingPathComponent("b-dangling")
        try FileManager.default.createDirectory(at: dangling, withIntermediateDirectories: true)
        try LabelSetLink(labelSetID: "gone", path: "../labels/gone").write(packDirectory: dangling)
        // A link naming one label set where another is, migrated from this pack.
        try linkedLabelSet(
            in: folder, pack: "c-mismatched", labelSet: "mismatched", migratedFrom: "kirk0-v1")
        try LabelSetLink(labelSetID: "someone-else", path: "../labels/mismatched").write(
            packDirectory: folder.appendingPathComponent("c-mismatched"))
        // An unreadable labels.json.
        try linkedLabelSet(
            in: folder, pack: "d-corrupt", labelSet: "corrupt", migratedFrom: "kirk0-v1")
        try Data("{".utf8).write(
            to: folder.appendingPathComponent("labels/corrupt").appendingPathComponent(
                LabelSetFiles.manifest))
        #expect(MigratedPackSearch.beside(v1) == .notFound)

        // None of them hides a good one after it.
        try linkedLabelSet(
            in: folder, pack: "e-good", labelSet: "kirk0-reference", migratedFrom: "kirk0-v1")
        #expect(
            MigratedPackSearch.beside(v1).successor?.packDirectory.lastPathComponent == "e-good")
    }

    @Test func aFolderThatCannotBeListedIsSaidSo() {
        let pack = URL(fileURLWithPath: "/nonexistent-\(UUID().uuidString)/kirk0-v1")
        #expect(
            MigratedPackSearch.beside(pack)
                == .folderUnreadable(path: pack.deletingLastPathComponent().path))
    }
}

// MARK: - The banner

@MainActor struct MigratedPackBannerTests {
    @Test func aMigratedVersion1PackNamesItsSuccessor() throws {
        let (folder, v1, v2) = try migratedPackPair()
        defer { try? FileManager.default.removeItem(at: folder) }
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: v1))
        let successor = try #require(session.migratedSuccessor)
        #expect(successor.packDirectory.standardizedFileURL == v2.standardizedFileURL)
        let revision = successor.labelSetRevision
        #expect(revision == 2, "created from the pack, then one save")
        #expect(
            session.readOnlyReason
                == "Legacy pack (version 1): read only. Its labels were migrated to kirk0-v1-v2 "
                + "(label set \(successor.labelSetName), revision \(revision)).")
        #expect(
            session.readOnlyReason?.contains("labels migrate") == false,
            "no command to run: it has been run")
    }

    @Test func withoutASuccessorTheBannerGivesTheMigration() throws {
        let folder = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let v1 = try SyntheticPack.write(
            [[(0, 0, 0, 1)]], directory: folder.appendingPathComponent("kirk0-v1"), version: 1)
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: v1))
        #expect(session.migratedSuccessor == nil)
        #expect(
            session.readOnlyReason
                == "Legacy pack (version 1): read only. Its labels cite this run's points, not "
                + "the capture's returns. Migrate it with "
                + "`velocity lidar labels migrate --pack \(v1.path) --pcap <file>`.")
    }

    @Test func aFolderThatCannotBeReadIsNamedInTheBanner() throws {
        let folder = labelSetTemporaryDirectory()
        let v1 = try SyntheticPack.write(
            [[(0, 0, 0, 1)]], directory: folder.appendingPathComponent("kirk0-v1"), version: 1)
        // Write and search, but not list: the pack still opens.
        try FileManager.default.setAttributes([.posixPermissions: 0o333], ofItemAtPath: folder.path)
        defer {
            try? FileManager.default.setAttributes(
                [.posixPermissions: 0o755], ofItemAtPath: folder.path)
            try? FileManager.default.removeItem(at: folder)
        }
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: v1))
        #expect(session.migratedSuccessor == nil)
        #expect(session.readOnlyReason?.contains("Migrate it with") == true)
        #expect(
            session.readOnlyReason?.hasSuffix(
                "The folder holding it could not be read, so a migrated copy beside it was not "
                    + "looked for.") == true)
    }

    @Test func theSuccessorIsFoundAgainOnReload() throws {
        let folder = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let v1 = try SyntheticPack.write(
            [[(0, 0, 0, 1)]], directory: folder.appendingPathComponent("kirk0-v1"), version: 1)
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: v1))
        #expect(session.migratedSuccessor == nil)
        // Migrated while the window was open.
        try linkedLabelSet(
            in: folder, pack: "kirk0-v1-v2", labelSet: "kirk0-reference", migratedFrom: "kirk0-v1")
        session.reload()
        #expect(session.migratedSuccessor?.labelSetName == "kirk0-reference")
    }

    @Test func theButtonIsRoutedThroughTheUnsavedChangesGuard() throws {
        let window = try String(
            contentsOf: labelSetRepositoryRoot.appendingPathComponent(
                "tools/visualiser-macos/VelocityVisualiser/UI/AnnotationWindow.swift"),
            encoding: .utf8)
        #expect(window.contains("guardedNavigate { controller.openMigratedSuccessor(at: url) }"))
    }
}
