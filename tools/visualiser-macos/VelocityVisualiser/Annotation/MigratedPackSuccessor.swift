// MigratedPackSuccessor.swift
// Finds the version 2 pack that carries a version 1 pack's migrated labels.
//
// `velocity lidar labels migrate` leaves the version 1 pack as it was and
// writes, beside it, a version 2 copy linked to a new label set. That label
// set's labels.json names the version 1 pack's directory in
// migrated_from.pack_dir. The copy's own name is whatever the operator chose,
// so it is found by that field and never by a suffix.

import Foundation

/// The version 2 pack a version 1 pack's labels were migrated to.
struct MigratedPackSuccessor: Equatable {
    var packDirectory: URL
    var labelSetName: String
    var labelSetRevision: Int
}

/// What looking beside a version 1 pack found.
enum MigratedPackSearch: Equatable {
    case found(MigratedPackSuccessor)
    case notFound
    /// The folder holding the pack could not be listed, so nothing beside it
    /// was looked at. In the sandbox: the app has not been given that folder.
    case folderUnreadable(path: String)

    var successor: MigratedPackSuccessor? {
        if case .found(let successor) = self { return successor }
        return nil
    }

    /// Looks in the folder holding `packDirectory` for a pack linked to a
    /// label set migrated from it. The first in name order is taken. A link
    /// that cannot be read or does not resolve, or a label set whose
    /// labels.json cannot be read or is not the one the link names, is
    /// passed over: one broken neighbour does not hide the pack after it.
    static func beside(_ packDirectory: URL) -> MigratedPackSearch {
        let name = packDirectory.lastPathComponent
        let folder = packDirectory.deletingLastPathComponent()
        let entries: [String]
        do { entries = try FileManager.default.contentsOfDirectory(atPath: folder.path) } catch {
            return .folderUnreadable(path: folder.path)
        }
        for entry in entries.sorted() where entry != name && !entry.hasPrefix(".") {
            let candidate = folder.appendingPathComponent(entry, isDirectory: true)
            guard let link = try? LabelSetLink.read(packDirectory: candidate) else { continue }
            let labels = link.directory(packDirectory: candidate)
            guard let data = try? LabelSetStore.readFile(labels, LabelSetFiles.manifest),
                let manifest = try? LabelSetManifest.read(data),
                manifest.labelSetID == link.labelSetID, manifest.migratedFrom?.packDir == name
            else { continue }
            return .found(
                MigratedPackSuccessor(
                    packDirectory: candidate,
                    labelSetName: manifest.name ?? labels.lastPathComponent,
                    labelSetRevision: manifest.head.revision))
        }
        return .notFound
    }
}
