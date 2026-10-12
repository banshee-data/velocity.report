//
//  LabelSetTestSupport.swift
//  VelocityVisualiserTests
//
//  Shared pieces for the label-set tests: the Go parity fixtures' location,
//  Go's testState, and the cleanup a pack with label sets beside it needs.
//

import Foundation
import Testing

@testable import VelocityVisualiser

/// The repository root, from this file's own path.
let labelSetRepositoryRoot = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()

/// internal/lidar/annotation/labelset/testdata/parity: what Go's
/// TestParityFixtures writes and checks.
let labelSetParityDirectory = labelSetRepositoryRoot.appendingPathComponent(
    "internal/lidar/annotation/labelset/testdata/parity")

func parityFixture(_ name: String) throws -> Data {
    try Data(contentsOf: labelSetParityDirectory.appendingPathComponent(name))
}

/// Removes a pack written by SyntheticPack and the label sets kept beside it.
/// Other packs are removed alone.
func removePack(_ dir: URL) {
    let parent = dir.deletingLastPathComponent()
    if parent.lastPathComponent.hasPrefix("synthetic-labels-") {
        try? FileManager.default.removeItem(at: parent)
    } else {
        try? FileManager.default.removeItem(at: dir)
    }
}

/// A fresh empty directory for a label set test.
func labelSetTemporaryDirectory() -> URL {
    let dir = FileManager.default.temporaryDirectory.appendingPathComponent(
        "labelset-\(UUID().uuidString)")
    try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    return dir
}

/// Go's testState in state_test.go: two windows of a four-packet table with
/// one domain and two objects.
func goTestState() -> LabelSetState {
    var s = LabelSetState(
        packets: [0x1111, 0x2222, 0x3333, 0x4444],
        records: LabelSetRecords(objects: [
            AnnotationObject(
                objectID: "obj_a", objectClass: "car", confidence: 1, status: .reviewed),
            AnnotationObject(
                objectID: "obj_b", objectClass: "pedestrian", confidence: 0.75, status: .proposed),
        ]), domains: [LabelSetDomain(domainID: "fg-1", runID: "run-1")],
        windows: [
            LabelSetWindow(windowID: 0, start: 0, end: 800),
            LabelSetWindow(windowID: 1, start: 800, end: 1600),
        ],
        domainReturns: [
            LabelSetDomainReturns(
                domainID: "fg-1", windowID: 0,
                returns: ReturnRanges([
                    ReturnRange(start: 10, length: 50), ReturnRange(start: 400, length: 20),
                ])),
            LabelSetDomainReturns(
                domainID: "fg-1", windowID: 1,
                returns: ReturnRanges([ReturnRange(start: 0, length: 100)])),
        ],
        masks: [
            LabelSetMask(
                objectID: "obj_a", windowID: 0, domainID: "fg-1",
                members: ReturnRanges([ReturnRange(start: 10, length: 20)]),
                uncertain: ReturnRanges([ReturnRange(start: 30, length: 2)]),
                completeness: .partial, visibility: .present, status: .reviewed,
                provenance: Provenance(author: "dd", createdUTC: "2026-10-12T00:00:00Z")),
            LabelSetMask(
                objectID: "obj_b", windowID: 0, domainID: "fg-1",
                members: ReturnRanges([ReturnRange(start: 400, length: 5)]),
                completeness: .unreviewed, visibility: .unknown, status: .proposed,
                provenance: Provenance(
                    author: "proposer", algorithm: "cluster_chain", algorithmVersion: "1")),
            LabelSetMask(
                objectID: "obj_a", windowID: 1, domainID: "fg-1",
                members: ReturnRanges([ReturnRange(start: 0, length: 3)]), completeness: .complete,
                visibility: .partlyOccluded, status: .reviewed,
                pose: AnnotationPose(
                    centerX: 1.5, centerY: 0, centerZ: 0, length: 4.2, width: 1.8, height: 1.5,
                    yawRad: 0.1, confidence: 0.5, status: .proposed)),
        ])
    s.canonicalise()
    return s
}

/// Go's parityJournal: the four states the fixture journal moves through.
func goParityStates() -> [LabelSetState] {
    let s0 = LabelSetState().canonical
    let s1 = goTestState()
    var s2 = s1
    s2.masks[0].members = .union(
        s2.masks[0].members, ReturnRanges([ReturnRange(start: 40, length: 2)]))
    s2.masks[1].status = .rejected
    s2.records.dismissedProposals = ["cluster_chain@1-ABC"]
    s2.canonicalise()
    var s3 = s2
    s3.masks.removeFirst()
    s3.packets.append(0x5555)
    s3.records.objects[0].notes = "deliberately separate from obj_b: \"twins\" / <tab>\t"
    s3.canonicalise()
    return [s0, s1, s2, s3]
}

/// Adds one return to obj_a's mask in window 0, as Go's addMember does.
func addMember(_ store: LabelSetStore, _ offset: UInt64) {
    guard let i = store.state.maskIndex(objectID: "obj_a", windowID: 0) else {
        Issue.record("obj_a window 0 missing")
        return
    }
    store.state.masks[i].members = .union(
        store.state.masks[i].members, ReturnRanges([ReturnRange(start: offset, length: 1)]))
}

/// A label set made from Go's testState, as Go's newSet makes one.
func newLabelSet() throws -> (URL, LabelSetStore) {
    let dir = labelSetTemporaryDirectory().appendingPathComponent("kirk0-reference")
    let store = try LabelSetStore.create(
        at: dir, manifest: LabelSetManifest(name: "kirk0-reference", layout: .pandar40P),
        initial: goTestState(), change: LabelSetChange(author: "dd", operation: "create"))
    return (dir, store)
}

/// The journal's complete lines.
func journalLines(_ labelSetDirectory: URL) -> [String] {
    let text =
        (try? String(
            contentsOf: labelSetDirectory.appendingPathComponent(LabelSetFiles.journal),
            encoding: .utf8)) ?? ""
    return text.split(separator: "\n", omittingEmptySubsequences: true).map(String.init)
}
