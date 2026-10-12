//
//  LabelSetCapturesTests.swift
//  VelocityVisualiserTests
//
//  The label set section's lines about capture files: none verified, with
//  the command that verifies one; one, and four of which three are listed;
//  and the version 1 pack a migrated set came from.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

private let labelsDirectory = URL(fileURLWithPath: "/data/annotation/labels/kirk0-reference")

private func binding(
    _ name: String, sha256: String, found: Int, complete: Int, partial: Int = 0
) throws -> LabelSetBinding {
    let text = """
        {"name": "\(name)", "sha256": "\(sha256)", "bytes": 4096, "udp_port": 2369,
         "packets_found": \(found), "packets_missing": 0, "windows_complete": \(complete),
         "windows_partial": \(partial), "windows_missing": 0,
         "verified_utc": "2026-10-12T00:00:00Z", "verified_build": "0.5.1-pre45"}
        """
    return try LabelSetBinding(json: LabelSetJSON.parse(Data(text.utf8)), "test binding")
}

private func migration(packDir: String?) throws -> LabelSetMigration {
    let dir = packDir.map { "\"pack_dir\": \"\($0)\"," } ?? ""
    let text = """
        {"pack_digest": "sha256:95be6fb2", "dataset_id": "ds_41b9794747f16ebe", \(dir)
         "sidecar_revision": 2232, "pcap_sha256": "sha256:2864ebde", "pcap_name": "kirk0.pcapng",
         "decoder_build": "0.5.1-pre45", "mapping_digest": "sha256:e9d93b33"}
        """
    return try LabelSetMigration(json: LabelSetJSON.parse(Data(text.utf8)), "test migration")
}

private func lines(_ manifest: LabelSetManifest) -> [String] {
    LabelSetCaptures.lines(
        manifest: manifest, packets: 149741, windows: 832, directory: labelsDirectory)
}

struct LabelSetCapturesTests {
    @Test func aSetVerifiedAgainstNoFileSaysSoAndGivesTheCommand() {
        let manifest = LabelSetManifest(labelSetID: "dd6579b7", name: "kirk0-reference")
        #expect(
            lines(manifest) == [
                "Not verified against any capture file. Verify one with:",
                "velocity lidar labels bind --labels /data/annotation/labels/kirk0-reference "
                    + "--pcap <file>",
            ])
    }

    @Test func oneVerifiedFileIsOneLine() throws {
        var manifest = LabelSetManifest(labelSetID: "dd6579b7", name: "kirk0-reference")
        manifest.appliesTo = [
            try binding(
                "kirk0.pcapng", sha256: "sha256:2864ebde0123456789abcdef", found: 149741,
                complete: 832)
        ]
        #expect(
            lines(manifest) == [
                "Verified capture files:",
                "kirk0.pcapng · 2864ebde · 149741 of 149741 packets · 832 of 832 frames",
            ])
    }

    @Test func fourVerifiedFilesListThreeAndCountTheRest() throws {
        var manifest = LabelSetManifest(labelSetID: "5bdcf414", name: "columbus-broadway")
        manifest.appliesTo = [
            try binding("roll-1.pcap", sha256: "sha256:135bf02a99", found: 36001, complete: 200),
            try binding(
                "published.pcapng", sha256: "sha256:aa69875b88", found: 36001, complete: 200),
            try binding(
                "excerpt.pcapng", sha256: "sha256:0011223344", found: 1200, complete: 5, partial: 2),
            try binding("roll-2.pcap", sha256: "sha256:5566778899", found: 10, complete: 0),
        ]
        let out = LabelSetCaptures.lines(
            manifest: manifest, packets: 36001, windows: 200, directory: labelsDirectory)
        #expect(
            out == [
                "Verified capture files:",
                "roll-1.pcap · 135bf02a · 36001 of 36001 packets · 200 of 200 frames",
                "published.pcapng · aa69875b · 36001 of 36001 packets · 200 of 200 frames",
                "excerpt.pcapng · 00112233 · 1200 of 36001 packets · 5 of 200 frames, 2 partly",
                "and 1 more",
            ])
    }

    @Test func aMigratedSetNamesTheVersion1PackAndRevision() throws {
        var manifest = LabelSetManifest(labelSetID: "dd6579b7", name: "kirk0-reference")
        manifest.migratedFrom = try migration(packDir: "kirk0-ad8b9438")
        #expect(
            lines(manifest).last == "Migrated from version 1 pack kirk0-ad8b9438, revision 2232")
        #expect(lines(manifest).count == 3, "the unverified lines, then the migration")

        manifest.appliesTo = [
            try binding("kirk0.pcapng", sha256: "sha256:2864ebde", found: 149741, complete: 832)
        ]
        #expect(
            lines(manifest) == [
                "Verified capture files:",
                "kirk0.pcapng · 2864ebde · 149741 of 149741 packets · 832 of 832 frames",
                "Migrated from version 1 pack kirk0-ad8b9438, revision 2232",
            ])

        // A migration that did not record the pack's directory names its dataset.
        manifest.migratedFrom = try migration(packDir: nil)
        #expect(
            lines(manifest).last
                == "Migrated from version 1 pack ds_41b9794747f16ebe, revision 2232")
    }

    @Test func aDigestWithoutItsPrefixIsStillShortened() {
        #expect(LabelSetCaptures.shortDigest("sha256:2864ebde8b9a") == "2864ebde")
        #expect(LabelSetCaptures.shortDigest("2864ebde8b9a") == "2864ebde")
        #expect(LabelSetCaptures.shortDigest("sha256:abc") == "abc")
    }
}

/// The session shows the lines of the label set it saves into, read again
/// on reload, and none for a pack with no label set.
@MainActor struct LabelSetCapturesSessionTests {
    @Test func theSectionFollowsTheLabelSetOnDisk() throws {
        let dir = try SyntheticPack.write([SyntheticPack.car(at: .zero) + SyntheticPack.wall])
        defer { removePack(dir) }
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        session.operatorName = "dd"
        #expect(session.labelSetCaptureLines.isEmpty, "no label set before the first save")

        _ = session.createObject(objectClass: "car")
        #expect(
            session.select(
                polygon: SelectionPolygon(rectFrom: simd_float2(-1, -1), to: simd_float2(12, 12)),
                mode: .replace))
        #expect(session.save())
        let link = try #require(try LabelSetLink.read(packDirectory: dir))
        let labels = link.directory(packDirectory: dir)
        #expect(
            session.labelSetCaptureLines == [
                "Not verified against any capture file. Verify one with:",
                "velocity lidar labels bind --labels \(labels.path) --pcap <file>",
            ])

        // Go's bind records a file in labels.json, outside the state digest.
        var manifest = try LabelSetManifest.read(
            try LabelSetStore.readFile(labels, LabelSetFiles.manifest))
        manifest.appliesTo = [
            try binding("synthetic.pcapng", sha256: "sha256:feedface00", found: 1, complete: 1)
        ]
        try LabelSetStore.writeAtomic(labels, LabelSetFiles.manifest, manifest.json.indentedData)
        session.reload()
        let store = try LabelSetStore.open(labels)
        #expect(
            session.labelSetCaptureLines == [
                "Verified capture files:",
                "synthetic.pcapng · feedface · 1 of \(store.state.packets.count) packets · "
                    + "1 of \(store.state.windows.count) frames",
            ])
    }

    @Test func aVersion1PackHasNoCaptureLines() throws {
        let dir = try PackFixture.write()
        defer { removePack(dir) }
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        #expect(session.labelSetCaptureLines.isEmpty)
    }
}
