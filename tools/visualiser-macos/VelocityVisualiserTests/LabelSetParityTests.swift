//
//  LabelSetParityTests.swift
//  VelocityVisualiserTests
//
//  The label-set format against Go's own bytes. Go's TestParityFixtures
//  writes internal/lidar/annotation/labelset/testdata/parity and checks it on
//  every run, so a change to either writer fails a test on the side that
//  changed. The state digest is over these bytes: a client that encoded one
//  float or one escape differently would record a revision Go cannot replay.
//

import Foundation
import Testing

@testable import VelocityVisualiser

struct LabelSetCanonicalJSONTests {
    /// canonical-values.json: the floats, strings, key and ranges whose
    /// encoding a hand-written writer is most likely to get wrong.
    @Test func awkwardValuesEncodeAsGoWritesThem() throws {
        let floats: [Double] = [
            0, 1, -0.5, 0.1, 0.75, 1e-7, 1.5e-5, 123_456_789, 1e21, 4.2, 1.8000000000000003,
        ]
        let strings = [
            "plain", "quote\"back\\slash", "slash/ <angle> & amp", "tab\tnewline\nbell\u{7}",
            "é ü 東京",
        ]
        let value = LabelSetJSON.object([
            "floats": .array(try floats.map { try LabelSetJSON.double($0) }),
            "strings": .array(strings.map(LabelSetJSON.string)),
            "key": .string(PacketKeys.hex(0x0123_4567_89ab_cdef)),
            "ranges": ReturnRanges([
                ReturnRange(start: 0, length: 3), ReturnRange(start: 10, length: 1),
            ]).json,
        ])
        let want = try parityFixture("canonical-values.json")
        #expect(value.canonicalLine == want)
        #expect(
            String(decoding: value.canonicalLine, as: UTF8.self)
                == String(decoding: want, as: UTF8.self))
    }

    @Test func numbersFollowTheECMAScriptForm() throws {
        let cases: [(Double, String)] = [
            (1e20, "100000000000000000000"), (1e-6, "0.000001"), (9.99e-7, "9.99e-7"),
            (-1e-7, "-1e-7"), (123.456, "123.456"), (5e-324, "5e-324"),
            (1.7976931348623157e308, "1.7976931348623157e+308"), (-0.0, "-0"), (2.5e21, "2.5e+21"),
        ]
        for (value, text) in cases {
            #expect(try LabelSetJSON.formatDouble(value) == text, "\(value)")
        }
        #expect(throws: LabelSetCodingError.self) { try LabelSetJSON.formatDouble(.nan) }
        #expect(throws: LabelSetCodingError.self) { try LabelSetJSON.formatDouble(.infinity) }
    }

    @Test func theFixtureParsesBackToItsOwnBytes() throws {
        let data = try parityFixture("canonical-values.json")
        let parsed = try LabelSetJSON.parse(data)
        #expect(parsed.canonicalLine == data)
    }

    @Test func parsingRefusesWhatIsNotJSON() {
        for bad in ["{", "[1,]", "{\"a\" 1}", "01", "\"\u{1}\"", "nul", "[1] 2"] {
            #expect(throws: LabelSetCodingError.self) { try LabelSetJSON.parse(Data(bad.utf8)) }
        }
    }

    @Test func unknownFieldsAreRefusedNotDropped() throws {
        // A field this build does not know would be lost on the next save.
        let line = #"{"domain_id":"fg-1","returns":[],"window_id":0,"extra":1}"#
        #expect(throws: LabelSetCodingError.self) {
            try LabelSetDomainReturns(json: LabelSetJSON.parse(Data(line.utf8)), "test")
        }
    }

    @Test func nonCanonicalRangesAreRefused() {
        for bad in ["[[0,0]]", "[[0,2],[2,1]]", "[[5,1],[1,1]]", "[[18446744073709551615,2]]"] {
            #expect(throws: LabelSetCodingError.self) {
                try ReturnRanges.decode(LabelSetJSON.parse(Data(bad.utf8)), "test")
            }
        }
    }
}

struct LabelSetParityTests {
    /// The fixture head is the third revision's state, encoded file by file.
    @Test func theHeadFilesMatchGoByteForByte() throws {
        let files = try goParityStates()[3].encodedFiles()
        for name in LabelSetFiles.state {
            let want = try parityFixture(name)
            #expect(files[name] == want, "\(name) differs from what Go writes")
        }
    }

    /// digests.txt: revision → state digest, for the empty state and each of
    /// the three revisions.
    @Test func everyRevisionDigestsAsGoSays() throws {
        let text = String(decoding: try parityFixture("digests.txt"), as: UTF8.self)
        let want = text.split(separator: "\n").map { $0.split(separator: " ")[1] }.map(String.init)
        let got = try goParityStates().map { try $0.digest() }
        #expect(got == want)
    }

    /// journal.ndjson: each delta, as Diff computes it and the canonical
    /// writer writes it.
    @Test func theJournalLinesMatchGoByteForByte() throws {
        let states = goParityStates()
        var out = Data()
        for i in 1..<states.count {
            var d = try LabelSetDelta.diff(from: states[i - 1], to: states[i])
            d.revision = i
            d.parentRevision = i - 1
            d.parentState = try states[i - 1].digest()
            d.state = try states[i].digest()
            d.change = LabelSetChange(
                author: "dd", session: "S1", operation: "save_mask",
                createdUTC: "2026-10-12T00:00:0\(i)Z")
            out.append(try d.json().canonicalLine)
        }
        #expect(out == (try parityFixture("journal.ndjson")))
    }

    @Test func theHeadFilesDecodeToTheSameState() throws {
        var files: [String: Data] = [:]
        for name in LabelSetFiles.state { files[name] = try parityFixture(name) }
        let state = try LabelSetState.decode(files: files).canonical
        try state.validate(layout: .pandar40P)
        #expect(state == goParityStates()[3])
        #expect(try state.encodedFiles() == files)
    }

    /// The fixture journal, applied line by line, rebuilds every revision to
    /// the digest Go recorded for it.
    @Test func replayingTheFixtureJournalReachesEveryDigest() throws {
        let dir = labelSetTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: dir) }
        for name in LabelSetFiles.state + [LabelSetFiles.journal] {
            try parityFixture(name).write(to: dir.appendingPathComponent(name))
        }
        let digests = String(decoding: try parityFixture("digests.txt"), as: UTF8.self).split(
            separator: "\n"
        ).map { String($0.split(separator: " ")[1]) }
        var manifest = LabelSetManifest(labelSetID: "parity", layout: .pandar40P)
        manifest.head = LabelSetHead(revision: 3, state: digests[3])
        try manifest.json.indentedData.write(to: dir.appendingPathComponent(LabelSetFiles.manifest))

        #expect(try LabelSetStore.verify(dir) == 3)
        for revision in 1...3 {
            #expect(
                try LabelSetStore.loadRevision(dir, revision: revision).digest()
                    == digests[revision])
        }
        let opened = try LabelSetStore.open(dir)
        #expect(opened.revision == 3)
        #expect(opened.state == goParityStates()[3])
    }

    @Test func validationRefusesWhatGoRefuses() {
        let cases: [String: (inout LabelSetState) -> Void] = [
            "duplicate packet": { $0.packets[1] = $0.packets[0] },
            "overlapping window": { $0.windows[1].start = 700 },
            "window past table": { $0.windows[1].end = 1601 },
            "member outside domain": {
                $0.masks[0].members = ReturnRanges([ReturnRange(start: 60, length: 1)])
            },
            "two objects claim a return": {
                $0.masks[1].members = ReturnRanges([ReturnRange(start: 15, length: 1)])
            },
            "member and uncertain": {
                $0.masks[0].uncertain = ReturnRanges([ReturnRange(start: 12, length: 1)])
            }, "unknown object": { $0.masks[0].objectID = "ghost" },
            "unknown domain": { $0.masks[0].domainID = "fg-2" },
            "non-foreground domain": { $0.domains[0].kind = "all_returns" },
            "duplicate dismissed": { $0.records.dismissedProposals = ["p", "p"] },
            "non-canonical set": {
                $0.masks[0].members = ReturnRanges([
                    ReturnRange(start: 10, length: 2), ReturnRange(start: 12, length: 1),
                ])
            },
        ]
        for (name, mutate) in cases {
            var s = goTestState()
            mutate(&s)
            #expect(throws: (any Error).self, "\(name)") { try s.validate(layout: .pandar40P) }
        }
        #expect(throws: Never.self) { try goTestState().validate(layout: .pandar40P) }
        #expect(goTestState().reviewedMasks.count == 2)
    }

    @Test func applyRefusesADeltaThatDoesNotFit() throws {
        let base = goTestState()
        var removeAbsent = LabelSetDelta()
        var m = LabelSetMaskDelta(objectID: "obj_a", windowID: 0)
        m.membersRemove = ReturnRanges([ReturnRange(start: 50, length: 1)])
        removeAbsent.masks = [m]
        #expect(throws: LabelSetApplyError.self) { try removeAbsent.apply(to: base) }

        var addPresent = LabelSetDelta()
        m = LabelSetMaskDelta(objectID: "obj_a", windowID: 0)
        m.membersAdd = ReturnRanges([ReturnRange(start: 10, length: 1)])
        addPresent.masks = [m]
        #expect(throws: LabelSetApplyError.self) { try addPresent.apply(to: base) }

        var noFields = LabelSetDelta()
        noFields.masks = [LabelSetMaskDelta(objectID: "obj_b", windowID: 1)]
        #expect(throws: LabelSetApplyError.self) { try noFields.apply(to: base) }

        #expect(throws: LabelSetApplyError.self) {
            var shrink = goTestState()
            shrink.packets.removeLast()
            _ = try LabelSetDelta.diff(from: base, to: shrink)
        }
    }

    @Test func diffThenApplyRoundTrips() throws {
        let states = goParityStates()
        for i in 1..<states.count {
            let d = try LabelSetDelta.diff(from: states[i - 1], to: states[i])
            #expect(try d.apply(to: states[i - 1]) == states[i])
            let reparsed = try LabelSetDelta(
                json: LabelSetJSON.parse(d.json().canonicalData), "test")
            #expect(try reparsed.apply(to: states[i - 1]) == states[i])
        }
        #expect(try LabelSetDelta.diff(from: states[2], to: states[2]).isEmpty)
    }
}

struct ReturnRangesTests {
    @Test func ofIsCanonicalWhateverTheOrder() {
        let set = ReturnRanges.of([5, 3, 4, 9, 3, 10])
        #expect(set.ranges == [ReturnRange(start: 3, length: 3), ReturnRange(start: 9, length: 2)])
        #expect(set.count == 5)
        #expect(set.contains(4) && !set.contains(6) && set.contains(10))
        #expect(set.members == [3, 4, 5, 9, 10])
        #expect(set.max == 11)
        #expect(ReturnRanges.of([]).isEmpty)
    }

    @Test func algebraMatchesSets() {
        var generator = SystemRandomNumberGenerator()
        for _ in 0..<200 {
            let a = Set(
                (0..<30).compactMap { _ in
                    Bool.random(using: &generator) ? UInt64.random(in: 0..<60) : nil
                })
            let b = Set(
                (0..<30).compactMap { _ in
                    Bool.random(using: &generator) ? UInt64.random(in: 0..<60) : nil
                })
            let ra = ReturnRanges.of(a)
            let rb = ReturnRanges.of(b)
            #expect(ReturnRanges.union(ra, rb) == .of(a.union(b)))
            #expect(ReturnRanges.intersect(ra, rb) == .of(a.intersection(b)))
            #expect(ReturnRanges.difference(ra, rb) == .of(a.subtracting(b)))
        }
    }

    @Test func layoutNumbersReturnsAsTheParserDoes() throws {
        let layout = ReturnLayout.pandar40P
        #expect(try layout.returnNumber(packet: 2, block: 3, channel: 7) == 2 * 400 + 3 * 40 + 6)
        let (p, b, c) = layout.locate(2 * 400 + 3 * 40 + 6)
        #expect(p == 2 && b == 3 && c == 7)
        #expect(throws: LabelSetCodingError.self) {
            try layout.returnNumber(packet: 0, block: 10, channel: 1)
        }
        #expect(throws: LabelSetCodingError.self) {
            try layout.returnNumber(packet: 0, block: 0, channel: 0)
        }
    }

    @Test func packetKeysAreTheFirstEightBytesOfTheDigest() throws {
        // SHA-256("") = e3b0c442 98fc1c14 9afbf4c8 …
        #expect(PacketKeys.key(of: Data()) == 0xe3b0_c442_98fc_1c14)
        #expect(PacketKeys.hex(0x1111) == "0000000000001111")
        #expect(try PacketKeys.parse("0123456789abcdef") == 0x0123_4567_89ab_cdef)
        #expect(throws: LabelSetCodingError.self) { try PacketKeys.parse("123") }
    }
}
