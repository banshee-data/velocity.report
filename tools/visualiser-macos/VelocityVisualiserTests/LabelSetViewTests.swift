//
//  LabelSetViewTests.swift
//  VelocityVisualiserTests
//
//  Version 2 packs as views of a label set, mirroring Go's view_test.go: the
//  pack's returns and their invariants, two runs of one capture with
//  different packet tables joined by packet key, and an edit through one
//  run's view that keeps what only the other run showed.
//

import Foundation
import Testing

@testable import VelocityVisualiser

/// Go's writeTestPack: one sample per frame, each point at x = its return
/// number and y = its sample, in a pack whose source names `run`.
func writeViewPack(
    _ directory: URL, packets: [UInt64], frames: [SyntheticPack.Frame], run: String
) throws -> AnnotationPack {
    let samples = frames.enumerated().map { i, frame in
        frame.returns.map { r -> SyntheticPack.Point in (Float(r), Float(i), 1, 1) }
    }
    let dir = try SyntheticPack.write(
        samples, directory: directory, sourceStride: 1, packets: packets, frames: frames, run: run,
        configHash: "sha256:cfg-\(run)")
    return try AnnotationPack.open(directory: dir)
}

struct AnnotationPackVersion2Tests {
    @Test func returnsRoundTripAndTamperingIsRefused() throws {
        let base = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: base) }
        let dir = base.appendingPathComponent("pack")
        let pack = try writeViewPack(
            dir, packets: [1, 2, 3], frames: [(0, 2, [5, 6, 410]), (2, 1, [800, 801])], run: "run-a"
        )
        #expect(pack.manifest.schemaVersion == 2)
        #expect(pack.packets == [1, 2, 3])
        #expect(try pack.returns(sampleID: 1) == [800, 801])
        #expect(try pack.returns(sampleID: 0) == [5, 6, 410])
        #expect(pack.samples[1].returnStart == 800 && pack.samples[1].returnEnd == 1200)

        var bytes = try Data(contentsOf: dir.appendingPathComponent("returns.bin"))
        bytes[0] ^= 1
        try bytes.write(to: dir.appendingPathComponent("returns.bin"))
        #expect(throws: AnnotationPackError.self) { try AnnotationPack.open(directory: dir) }
    }

    @Test func theV2DigestCoversThePacketTable() throws {
        let base = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: base) }
        let dir = base.appendingPathComponent("pack")
        let pack = try writeViewPack(dir, packets: [1], frames: [(0, 1, [1])], run: "run-a")
        let m = pack.manifest
        let parts = [m.pointsSHA256, m.samplesSHA256, m.packetsSHA256!, m.returnsSHA256!]
        #expect(m.packDigest == AnnotationPack.digest(Data(parts.joined(separator: "\n").utf8)))
        #expect(m.returnLayout == .pandar40P)

        // A packet table swapped for another keeps its own digest but not the pack's.
        let other = PacketKeys.encode([2])
        try other.write(to: dir.appendingPathComponent("packets.bin"))
        #expect(throws: AnnotationPackError.self) { try AnnotationPack.open(directory: dir) }
    }

    @Test func disorderedOrStrayReturnsAreRefused() throws {
        let base = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: base) }
        let decreasing = base.appendingPathComponent("decreasing")
        #expect(throws: AnnotationPackError.self) {
            _ = try writeViewPack(decreasing, packets: [9], frames: [(0, 1, [7, 3])], run: "r")
        }
        let outside = base.appendingPathComponent("outside")
        #expect(throws: AnnotationPackError.self) {
            _ = try writeViewPack(outside, packets: [9], frames: [(0, 1, [3, 400])], run: "r")
        }
        let repeated = base.appendingPathComponent("repeated")
        #expect(throws: AnnotationPackError.self) {
            _ = try writeViewPack(repeated, packets: [9, 9], frames: [(0, 1, [3])], run: "r")
        }
    }
}

struct LabelSetViewMapTests {
    /// Go's TestViewMapAcrossRuns.
    @Test func viewMapAcrossRuns() throws {
        let base = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: base) }
        // Run A's pack: packets 10…13, two frames of two packets each.
        let a = try writeViewPack(
            base.appendingPathComponent("a"), packets: [10, 11, 12, 13],
            frames: [(0, 2, [5, 6, 7, 450]), (2, 2, [800, 900])], run: "run-a")
        let (layout, initial) = try LabelSetState.fromPack(a)
        var state = initial
        #expect(state.windows.count == 2 && state.domains.count == 1)
        #expect(state.domains[0].domainID == "fg-" + a.manifest.packDigest.dropFirst(7).prefix(12))
        #expect(state.domains[0].runID == "run-a")
        #expect(state.domains[0].configHash == "sha256:cfg-run-a")
        state.records.objects = [
            AnnotationObject(objectID: "obj_a", objectClass: "car", status: .reviewed)
        ]
        let vmA = try LabelSetViewMap(layout: layout, state: state, pack: a)
        let w0 = try #require(vmA.window(forSample: 0))
        let members = try vmA.offsets(sampleID: 0, indices: [0, 1, 3])
        state.masks = [
            LabelSetMask(
                objectID: "obj_a", windowID: w0, domainID: state.domains[0].domainID,
                members: members, completeness: .partial, visibility: .present, status: .reviewed)
        ]
        state.canonicalise()
        try state.validate(layout: layout)

        // Run B framed the same packets but kept different foreground, and
        // its packet table lists them in the same order with one extra
        // packet first.
        let b = try writeViewPack(
            base.appendingPathComponent("b"), packets: [9, 10, 11, 12, 13],
            frames: [(1, 2, [405, 406, 450, 850]), (3, 2, [1300])], run: "run-b")
        let skipped = try state.addPackDomain(layout: layout, pack: b)
        #expect(skipped.isEmpty)
        #expect(state.packets.count == 5 && state.windows.count == 2 && state.domains.count == 2)
        try state.validate(layout: layout)
        let vmB = try LabelSetViewMap(layout: layout, state: state, pack: b)

        // Run B shows returns 5, 6 and 450 (as 405, 406 and 850); run A's
        // mask holds those three.
        let (shown, hidden) = vmB.shown(sampleID: 0, state.masks[0].members)
        #expect(shown.count == 3 && hidden == 0)
        #expect(vmB.sampleIndex(0).shown(state.masks[0].members).indices == shown)

        // Editing through run B: dropping its point 0 removes return 5 only.
        let edited = try vmB.edit(sampleID: 0, state.masks[0].members, shownIndices: [1, 3])
        #expect(edited.count == 2 && !edited.contains(5) && edited.contains(450))

        // A member run B never showed survives an edit made through run B.
        let withHidden = ReturnRanges.union(
            state.masks[0].members, ReturnRanges([ReturnRange(start: 7, length: 1)]))
        let kept = try vmB.edit(sampleID: 0, withHidden, shownIndices: [0, 1, 3])
        #expect(kept.contains(7))
        #expect(vmB.shown(sampleID: 0, withHidden).hidden == 1)
    }

    /// Go's TestViewMapRefusesOtherLayout.
    @Test func aViewOfAnotherLayoutIsRefused() throws {
        let base = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: base) }
        let a = try writeViewPack(
            base.appendingPathComponent("a"), packets: [1], frames: [(0, 1, [1])], run: "run-a")
        let (_, state) = try LabelSetState.fromPack(a)
        #expect(throws: LabelSetError.self) {
            _ = try LabelSetViewMap(
                layout: ReturnLayout(model: "other", blocks: 6, channels: 32), state: state, pack: a
            )
        }
    }

    @Test func aPackCannotAddItsDomainTwice() throws {
        let base = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: base) }
        let a = try writeViewPack(
            base.appendingPathComponent("a"), packets: [1, 2], frames: [(0, 2, [1, 500])],
            run: "run-a")
        let (layout, initial) = try LabelSetState.fromPack(a)
        var state = initial
        let before = state
        #expect(throws: LabelSetError.self) { try state.addPackDomain(layout: layout, pack: a) }
        #expect(state == before, "a refused add leaves the state as it was")
    }

    @Test func aFrameAnotherRunFramedDifferentlyIsSkipped() throws {
        let base = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: base) }
        let a = try writeViewPack(
            base.appendingPathComponent("a"), packets: [10, 11, 12],
            frames: [(0, 2, [5]), (2, 1, [805])], run: "run-a")
        let (layout, initial) = try LabelSetState.fromPack(a)
        var state = initial
        // Run B's first frame starts one packet in: it overlaps window 0
        // without matching it.
        let b = try writeViewPack(
            base.appendingPathComponent("b"), packets: [11, 12], frames: [(0, 2, [7, 405])],
            run: "run-b")
        let skipped = try state.addPackDomain(layout: layout, pack: b)
        #expect(skipped == [0])
        #expect(state.windows.count == 2)
    }

    @Test func goPathBaseMatchesGo() {
        #expect(goPathBase("") == ".")
        #expect(goPathBase("/a/b/run.vrlog") == "run.vrlog")
        #expect(goPathBase("/a/b/") == "b")
        #expect(goPathBase("///") == "/")
        #expect(goPathBase("run") == "run")
    }
}
