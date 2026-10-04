//
//  PhysicalCompareAndRepairTests.swift
//  VelocityVisualiserTests
//
//  Compare mode against a report Go actually wrote, what seeing an estimate
//  does to later edits, and the repairs: copies that are proposals, stale
//  records removed or moved, history and restore, drift, drag handles, a
//  membership save that would claim a return twice, and the quit guard.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

private let repositoryRoot = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()

/// The report TestPhysicalReportSwiftFixture writes from a real Run.
private func reportFixture() throws -> Data {
    try Data(
        contentsOf: repositoryRoot.appendingPathComponent(
            "internal/lidar/perframeeval/testdata/physical_report_fixture.json"))
}

private func fixturePackDigest() throws -> String {
    let json = try JSONSerialization.jsonObject(with: reportFixture()) as! [String: Any]
    let physical = json["physical"] as! [String: Any]
    return (physical["reference"] as! [String: Any])["pack_digest"] as! String
}

/// The fixture with its pack digest (and optionally split role) replaced, in
/// a temporary file.
private func reportFile(packDigest: String, splitRole: String? = nil) throws -> URL {
    var text = String(decoding: try reportFixture(), as: UTF8.self)
    text = text.replacingOccurrences(of: try fixturePackDigest(), with: packDigest)
    if let splitRole {
        text = text.replacingOccurrences(
            of: "\"split_role\": \"tuning\"", with: "\"split_role\": \"\(splitRole)\"")
    }
    let url = FileManager.default.temporaryDirectory.appendingPathComponent(
        "report-\(UUID().uuidString).json")
    try Data(text.utf8).write(to: url)
    return url
}

struct PhysicalComparisonReportTests {
    @Test func decodesTheReportGoWrites() throws {
        let digest = try fixturePackDigest()
        let report = try PhysicalComparisonReport.decode(reportFixture(), packDigest: digest)
        #expect(report.arms.count == 2)
        #expect(report.arms.map(\.arm.label) == ["exact", "face_bias"])
        #expect(report.reference.physicalRevision == 1 && report.reference.splitRole == "tuning")
        let arm = report.arms[0]
        #expect(arm.instants.count == report.reference.expectedInstants)
        #expect(!arm.accounting.isEmpty)
        #expect(arm.arm.source.contains("exact") && arm.arm.source.contains("cv_kf_v1"))
        let withPrediction = try #require(
            arm.instants.first { $0.prediction != nil && $0.reference != nil })
        #expect(report.instants(arm: 0, sampleID: withPrediction.sampleID).contains(withPrediction))
        #expect(
            report.predictedObjects(arm: 0, sampleID: withPrediction.sampleID).contains(
                withPrediction.objectID))
        // The recorded reference converts to the overlay's geometry intact.
        let g = try #require(withPrediction.reference).geometry
        #expect(g.centre != nil || g.anchorPoint != nil)
        #expect(
            report.outcomeCounts(arm: 0, component: "centre").values.reduce(0, +)
                == arm.instants.count)
        #expect(report.instants(arm: 5, sampleID: 0).isEmpty)
    }

    @Test func refusesAReportForAnotherPackOrWithoutPhysicalScores() throws {
        #expect(
            throws: PhysicalReportError.wrongPack(
                report: try fixturePackDigest(), open: "sha256:other")
        ) { try PhysicalComparisonReport.decode(reportFixture(), packDigest: "sha256:other") }
        #expect(throws: PhysicalReportError.noPhysicalSection) {
            try PhysicalComparisonReport.decode(Data(#"{"schema":"x"}"#.utf8), packDigest: "p")
        }
        #expect(throws: (any Error).self) {
            try PhysicalComparisonReport.decode(Data("{".utf8), packDigest: "p")
        }
    }
}

@MainActor private func openSession() throws -> (AnnotationSession, FakePhysicalService, URL) {
    let dir = try SyntheticPack.write([
        SyntheticPack.car(at: simd_float2(5, 0)), SyntheticPack.car(at: simd_float2(6, 0)),
        SyntheticPack.car(at: simd_float2(7, 0)),
    ])
    let (urlSession, baseURL, register) = AnnotationMockURLProtocol.makeSession()
    let fake = FakePhysicalService(packDir: dir.resolvingSymlinksInPath().path)
    register { try fake.handle($0) }
    let session = try AnnotationSession(
        pack: try AnnotationPack.open(directory: dir),
        physicalClient: PhysicalReferenceAPIClient(baseURL: baseURL, session: urlSession))
    session.operatorName = "op"
    return (session, fake, dir)
}

@MainActor private func waitForPhysical(_ session: AnnotationSession) async {
    for _ in 0..<200 where session.physical.availability != .ready {
        try? await Task.sleep(for: .milliseconds(10))
    }
}

@MainActor struct PhysicalCompareModeTests {
    @Test func reportSeedKeepsSharedSizeAndRefusesToOverwritePose() async throws {
        let (session, fake, _) = try openSession()
        let object = session.createObject(objectClass: "car")
        #expect(session.save())
        let sample = try #require(session.currentSample)
        let originalBody = PhysicalBody(bodyID: "shared_size")
        fake.objects = [PhysicalObject(objectID: object.objectID, body: originalBody)]
        fake.digest = "sha256:saved"
        session.workMode = .physical
        await waitForPhysical(session)
        let url = try reportFile(packDigest: session.pack.manifest.packDigest)
        var json = try JSONSerialization.jsonObject(with: Data(contentsOf: url)) as! [String: Any]
        var physical = json["physical"] as! [String: Any]
        var arm = physical["arm_a"] as! [String: Any]
        var row = (arm["instants"] as! [[String: Any]])[0]
        row["object_id"] = object.objectID
        row["sample_id"] = sample.sampleID
        row["timestamp_ns"] = sample.timestampNs
        var prediction = row["prediction"] as! [String: Any]
        prediction["timestamp_ns"] = sample.timestampNs
        row["prediction"] = prediction
        var match = row["match"] as! [String: Any]
        match["offset_ns"] = 0
        row["match"] = match
        arm["instants"] = [row]
        physical["arm_a"] = arm
        json["physical"] = physical
        try JSONSerialization.data(withJSONObject: json).write(to: url)
        await session.reportInspector.open(
            url: url, packDigest: session.pack.manifest.packDigest, role: session.packRole,
            physical: session.physical)
        let seed = try session.trackerSeedFromReport()
        // The pane's check names the same row and mints nothing.
        let eligible = try session.trackerSeedEligibility()
        #expect(eligible.prediction == seed.prediction && eligible.sampleID == seed.sampleID)
        #expect(try session.trackerSeedEligibility() == eligible)
        #expect(try session.trackerSeedFromReport().body.bodyID != seed.body.bodyID)
        session.seedPhysicalFromReport()
        #expect(session.physical.object(object.objectID)?.body == originalBody)
        #expect(session.physicalKeyframe?.review.origin == .trackerAssisted)
        #expect(session.physicalKeyframe?.position.xM == seed.prediction.x)
        #expect(session.physical.seedGhost?.prediction == seed.prediction)
        let before = session.physical.draft
        session.seedPhysicalFromReport()
        #expect(session.physical.draft == before)
        session.physical.discard()
        #expect(session.physical.draft == fake.objects)
        #expect(session.physical.exposure[object.objectID] != nil)
    }

    @Test func openingAReportChecksProvenanceAndShowingItExposes() async throws {
        let (session, fake, _) = try openSession()
        fake.digest = "sha256:fixture-revision-bytes"
        session.workMode = .compare
        await waitForPhysical(session)
        #expect(!session.syncWithMainView)
        let inspector = session.reportInspector
        await inspector.open(
            url: try reportFile(packDigest: session.pack.manifest.packDigest),
            packDigest: session.pack.manifest.packDigest, role: session.packRole,
            physical: session.physical)
        let report = try #require(inspector.report)
        #expect(inspector.retained != nil, "the scored revision's bytes match, so provenance shows")
        session.exposeComparedObjects()
        let shown = report.predictedObjects(arm: 0, sampleID: session.currentSample!.sampleID)
        #expect(!shown.isEmpty)
        #expect(Set(session.physical.exposure.keys) == shown)
        #expect(session.physical.exposure.values.allSatisfy { $0.contains("exact") })

        // Back to authoring: the exposure stays.
        session.workMode = .physical
        #expect(Set(session.physical.exposure.keys) == shown)

        // A revision that no longer reads with the report's bytes shows no
        // provenance, and says why.
        fake.digest = "sha256:changed"
        await inspector.open(
            url: try reportFile(packDigest: session.pack.manifest.packDigest),
            packDigest: session.pack.manifest.packDigest, role: nil, physical: session.physical)
        #expect(
            inspector.retained == nil
                && inspector.provenanceNote?.contains("different bytes") == true)
        inspector.close()
        #expect(inspector.report == nil)
    }

    @Test func aHeldOutPackOrSplitIsNotCompared() async throws {
        let (session, _, _) = try openSession()
        let inspector = session.reportInspector
        await inspector.open(
            url: try reportFile(packDigest: session.pack.manifest.packDigest),
            packDigest: session.pack.manifest.packDigest, role: "held_out",
            physical: session.physical)
        #expect(inspector.report == nil && inspector.error?.contains("held out") == true)
        await inspector.open(
            url: try reportFile(
                packDigest: session.pack.manifest.packDigest, splitRole: "held_out"),
            packDigest: session.pack.manifest.packDigest, role: "tuning", physical: session.physical
        )
        #expect(inspector.report == nil && inspector.error?.contains("held-out") == true)
        await inspector.open(
            url: try reportFile(packDigest: "sha256:elsewhere"),
            packDigest: session.pack.manifest.packDigest, role: nil, physical: session.physical)
        #expect(inspector.report == nil && inspector.error?.contains("scored pack") == true)
    }

    @Test func thePackRoleIsReadFromSegmentJSON() throws {
        let dir = try SyntheticPack.write([SyntheticPack.car(at: simd_float2(5, 0))])
        try Data(#"{"role":"held_out"}"#.utf8).write(to: dir.appendingPathComponent("segment.json"))
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        #expect(session.packRole == "held_out" && !session.comparisonAllowed)
    }

    @Test func compareIsReadOnly() async throws {
        let (session, _, _) = try openSession()
        session.workMode = .compare
        await waitForPhysical(session)
        _ = session.createObject(objectClass: "car")
        session.placePhysicalAnchor(in: .top, at: simd_float2(1, 1))
        #expect(session.physicalKeyframe == nil)
        #expect(!(await session.saveCurrent(advance: false)))
        #expect(await session.saveCurrent(advance: true))
        #expect(session.sampleIndex == 1)
    }
}

struct PhysicalRepairDraftTests {
    private func sample(_ id: Int) -> AnnotationSample {
        AnnotationSample(
            sampleID: id, sourceOrdinal: id, sourceFrameID: 0, timestampNs: 1_000 + Int64(id) * 100,
            sensorID: "s", pointCount: 0, byteOffset: 0)
    }

    private func observedKeyframe(
        _ id: String, sample: Int, origin: PhysicalOrigin = .independent
    ) -> PhysicalKeyframe {
        var k = PhysicalKeyframe(
            keyframeID: id, sampleID: sample, timestampNs: 1_000 + Int64(sample) * 100)
        k.position = PhysicalPosition(
            status: .observed, xM: 1, yM: 2, zM: 0.5, boundM: 0.2, support: .init(frames: [sample]))
        k.yaw = PhysicalYaw(
            status: .observed, axis: .resolved, yawRad: 0.1, boundRad: 0.05,
            support: .init(frames: [sample, 9]))
        k.front = PhysicalEndpoint(status: .unknown)
        k.review.origin = origin
        if origin == .trackerAssisted { k.review.trackerSource = "est" }
        k.sharedErrors = [PhysicalSharedError(observation: "fit", components: ["position", "yaw"])]
        return k
    }

    @Test func aCopyIsAProposalInferredFromItsSource() {
        let source = observedKeyframe("kf_src", sample: 2, origin: .trackerAssisted)
        let copy = PhysicalDraft.copy(source, to: sample(5), author: "op", session: "s")
        #expect(
            copy.keyframeID != source.keyframeID && copy.sampleID == 5 && copy.timestampNs == 1_500)
        #expect(copy.position.status == .inferred && copy.position.support.frameList == [2])
        #expect(copy.yaw.status == .inferred && copy.yaw.support.frameList == [2, 9])
        #expect(copy.position.xM == 1 && copy.position.zM == 0.5)
        #expect(copy.sharedErrors == nil)
        #expect(
            copy.review.status == .proposed && copy.review.origin == .trackerAssisted
                && copy.review.trackerSource == "est")
        #expect(copy.review.method == "copy_of:kf_src")
        #expect(
            copy.review.uncertaintyAssumptions?.contains("not observed at this instant") == true)
        let object = PhysicalObject(
            objectID: "o", keyframes: [source, observedKeyframe("kf_far", sample: 9)])
        #expect(PhysicalDraft.nearestKeyframe(of: object, to: sample(4))?.keyframeID == "kf_src")
        #expect(PhysicalDraft.nearestKeyframe(of: object, to: sample(2))?.keyframeID == "kf_far")
    }

    @Test func reassigningMovesRecordsOrSaysWhyNot() {
        var objects = [
            PhysicalObject(
                objectID: "a", body: PhysicalBody(bodyID: "b1"),
                keyframes: [observedKeyframe("k1", sample: 1)]),
            PhysicalObject(objectID: "b", keyframes: [observedKeyframe("k2", sample: 2)]),
        ]
        #expect(PhysicalDraft.reassign(from: "a", to: "b", in: &objects) == nil)
        #expect(objects.map(\.objectID) == ["b"] && objects[0].body?.bodyID == "b1")
        #expect(Set(objects[0].keyframes.map(\.keyframeID)) == ["k1", "k2"])
        var clash = [
            PhysicalObject(objectID: "a", keyframes: [observedKeyframe("k1", sample: 1)]),
            PhysicalObject(objectID: "b", keyframes: [observedKeyframe("k2", sample: 1)]),
        ]
        #expect(
            PhysicalDraft.reassign(from: "a", to: "b", in: &clash)?.contains("sample 1") == true)
        var bodies = [
            PhysicalObject(objectID: "a", body: PhysicalBody(bodyID: "x")),
            PhysicalObject(objectID: "b", body: PhysicalBody(bodyID: "y")),
        ]
        #expect(PhysicalDraft.reassign(from: "a", to: "b", in: &bodies)?.contains("body") == true)
        #expect(PhysicalDraft.reassign(from: "a", to: "a", in: &bodies) != nil)
    }

    @Test func aStaleRecordIsRemovedByWhatTheServiceNamed() {
        var objects = [
            PhysicalObject(
                objectID: "a", body: PhysicalBody(bodyID: "b1"),
                keyframes: [observedKeyframe("k1", sample: 1)]),
            PhysicalObject(objectID: "c", keyframes: [observedKeyframe("k3", sample: 3)]),
        ]
        #expect(
            PhysicalDraft.remove(
                problem: .init(record: #"object "a" keyframe "k1""#, problem: "x"), from: &objects))
        #expect(objects[0].keyframes.isEmpty && objects[0].body != nil)
        #expect(
            PhysicalDraft.remove(
                problem: .init(record: #"object "a" body "b1""#, problem: "x"), from: &objects))
        #expect(!objects.contains { $0.objectID == "a" })
        #expect(
            PhysicalDraft.remove(
                problem: .init(record: #"object "c""#, problem: "x"), from: &objects))
        #expect(objects.isEmpty)
        #expect(
            !PhysicalDraft.remove(
                problem: .init(record: #"following "f""#, problem: "x"), from: &objects))
    }

    @Test func anEditAfterSeeingAnEstimateIsTrackerAssisted() {
        let saved = [
            PhysicalObject(
                objectID: "seen", body: PhysicalBody(bodyID: "b1"),
                keyframes: [observedKeyframe("k1", sample: 1)]),
            PhysicalObject(objectID: "unseen", keyframes: [observedKeyframe("k2", sample: 2)]),
        ]
        var after = saved
        after[0].keyframes[0].position.boundM = 0.5
        after[0].keyframes.append(observedKeyframe("k_new", sample: 4))
        after[1].keyframes[0].position.boundM = 0.5
        PhysicalDraft.applyExposure(
            before: saved, after: &after, exposure: ["seen": "exact · cv_kf_v1"])
        let edited = after[0].keyframes[0]
        #expect(edited.keyframeID != "k1", "a saved independent record was relabelled in place")
        #expect(
            edited.review.origin == .trackerAssisted
                && edited.review.trackerSource == "exact · cv_kf_v1")
        let created = after[0].keyframes[1]
        #expect(created.keyframeID == "k_new" && created.review.origin == .trackerAssisted)
        #expect(after[0].body == saved[0].body, "an untouched body was changed")
        #expect(
            after[1].keyframes[0].review.origin == .independent
                && after[1].keyframes[0].keyframeID == "k2")
        // Nothing seen: nothing changes.
        var plain = saved
        plain[0].keyframes[0].position.boundM = 0.5
        let expected = plain
        PhysicalDraft.applyExposure(before: saved, after: &plain, exposure: [:])
        #expect(plain == expected)
    }
}

@MainActor struct PhysicalRepairSessionTests {
    @Test func exposureIsKeptForThePackAcrossSessions() async throws {
        let suite = "physical-exposure-\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let dir = URL(fileURLWithPath: "/tmp/x")
        let first = PhysicalReferenceSession(
            packDirectory: dir, packDigest: "sha256:p", sessionID: "s", membershipDigest: { "" },
            author: { "op" }, exposureDefaults: defaults)
        first.expose(objectIDs: ["o1"], source: "arm A")
        first.expose(objectIDs: ["o1"], source: "arm B")
        #expect(first.exposure == ["o1": "arm A"], "the first estimate seen is what is named")
        let second = PhysicalReferenceSession(
            packDirectory: dir, packDigest: "sha256:p", sessionID: "s", membershipDigest: { "" },
            author: { "op" }, exposureDefaults: defaults)
        #expect(second.exposure == ["o1": "arm A"])
        let other = PhysicalReferenceSession(
            packDirectory: dir, packDigest: "sha256:q", sessionID: "s", membershipDigest: { "" },
            author: { "op" }, exposureDefaults: defaults)
        #expect(other.exposure.isEmpty)
    }

    @Test func historyAndRestore() async throws {
        let (session, fake, _) = try openSession()
        session.workMode = .physical
        await waitForPhysical(session)
        let physical = session.physical
        let object = session.createObject(objectClass: "car")
        physical.edit {
            PhysicalDraft.addBody(objectID: object.objectID, author: "op", session: "s", to: &$0)
        }
        #expect(await physical.save())
        physical.edit { PhysicalDraft.removeBody(objectID: object.objectID, from: &$0) }
        #expect(!(await physical.restore(revision: 1)), "restored over an unsaved draft")
        #expect(physical.lastError?.contains("Save or discard") == true)
        physical.discard()
        await physical.loadHistory()
        #expect(physical.history.map(\.revision) == [1])
        #expect(await physical.restore(revision: 1))
        let sent = try #require(fake.last("/api/annotations/physical/restore"))
        #expect(sent["revision"] as? Int == 1 && sent["base_digest"] as? String == "sha256:rev1")
        #expect(physical.state?.revision == 2 && physical.history.first?.revision == 2)
    }

    @Test func driftIsReadAndLetsARecordBeReviewedAgain() throws {
        var state = FakePhysicalService(packDir: "/x").state()
        state.reviewDrift = [
            .init(record: #"object "o" keyframe "kf_1""#, problem: "membership at sample 2 changed")
        ]
        let data = try JSONEncoder().encode(state)
        let decoded = try JSONDecoder().decode(PhysicalPackState.self, from: data)
        #expect(decoded.reviewDrift.count == 1)
        // A service that predates drift reports none.
        var old = try JSONSerialization.jsonObject(with: data) as! [String: Any]
        old.removeValue(forKey: "review_drift")
        let legacy = try JSONDecoder().decode(
            PhysicalPackState.self, from: JSONSerialization.data(withJSONObject: old))
        #expect(legacy.reviewDrift.isEmpty)
    }

    @Test func dragHandlesMoveAndTurnAsOneUndoStep() async throws {
        let (session, _, _) = try openSession()
        session.workMode = .physical
        await waitForPhysical(session)
        _ = session.createObject(objectClass: "car")
        session.placePhysicalAnchor(in: .top, at: simd_float2(2, 3))
        let id = try #require(session.activeObjectID)
        let sampleID = try #require(session.currentSample?.sampleID)
        session.physical.edit {
            PhysicalDraft.updateKeyframe(objectID: id, sampleID: sampleID, in: &$0) {
                PhysicalDraft.setAxis(.resolved, of: &$0)
                $0.yaw.yawRad = 0
                $0.yaw.boundRad = 0.1
                $0.position.boundM = 0.2
            }
        }
        let vp = OrthoViewport(halfHeight: 10, size: CGSize(width: 400, height: 400), centre: .zero)
        let anchor = vp.screenPoint(from: simd_float2(2, 3))
        #expect(session.physicalHandle(at: anchor, viewport: vp) == .move)
        #expect(
            session.physicalHandle(at: CGPoint(x: anchor.x + 34, y: anchor.y), viewport: vp)
                == .turn)
        #expect(
            session.physicalHandle(at: CGPoint(x: anchor.x + 80, y: anchor.y + 80), viewport: vp)
                == nil)

        let undoBefore = session.physical.canUndo
        session.beginPhysicalDrag()
        // Taken 0.1 m off-centre and moved 1 m right: the anchor moves 1 m.
        session.updatePhysicalDrag(.move, from: simd_float2(2.1, 3), to: simd_float2(2.6, 3))
        session.updatePhysicalDrag(.move, from: simd_float2(2.1, 3), to: simd_float2(3.1, 3))
        session.endPhysicalDrag()
        let moved = try #require(session.physicalKeyframe)
        #expect(
            abs((moved.position.xM ?? 0) - 3) < 1e-5 && abs((moved.position.yM ?? 0) - 3) < 1e-5)
        #expect(undoBefore)
        session.beginPhysicalDrag()
        session.updatePhysicalDrag(.turn, from: simd_float2(4, 3), to: simd_float2(3, 5))
        session.endPhysicalDrag()
        #expect(abs((session.physicalKeyframe?.yaw.yawRad ?? 0) - .pi / 2) < 1e-5)
        session.physical.undo()
        #expect(session.physicalKeyframe?.yaw.yawRad == 0, "a drag was more than one undo step")
        #expect(abs((session.physicalKeyframe?.position.xM ?? 0) - 3) < 1e-5)
        session.physical.undo()
        #expect(abs((session.physicalKeyframe?.position.xM ?? 0) - 2) < 1e-5)
    }
}

/// A Top-view rectangle over the synthetic car's columns: the car's points
/// sit at x = 5, 5.3 and 5.6 and y = 0, 0.3 and 0.6, two heights each, with
/// index = column * 6 + row * 2 + level.
private func columns(_ x0: Float, _ x1: Float) -> SelectionPolygon {
    SelectionPolygon(vertices: [
        simd_float2(x0, -0.1), simd_float2(x1, -0.1), simd_float2(x1, 0.7), simd_float2(x0, 0.7),
    ])
}

@MainActor struct PointClaimTests {
    @Test func conflictsFollowGosRule() {
        var s = Sidecar(datasetID: "d", packDigest: "p")
        s.masks = [
            FrameMask(objectID: "a", sampleID: 0, pointIndices: [1, 2]),
            FrameMask(objectID: "b", sampleID: 0, pointIndices: [2, 3]),
            FrameMask(objectID: "b", sampleID: 1, pointIndices: [1]),
        ]
        let conflicts = s.pointClaimConflicts()
        #expect(conflicts == [PointClaimConflict(sampleID: 0, point: 2, first: "a", second: "b")])
        #expect(
            conflicts[0].key
                == PointClaimConflict(sampleID: 0, point: 2, first: "b", second: "a").key)
    }

    @Test func aSaveThatWouldClaimAReturnTwiceIsRefused() throws {
        let dir = try SyntheticPack.write([SyntheticPack.car(at: simd_float2(5, 0))])
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        session.operatorName = "op"
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: columns(4.9, 5.35), mode: .replace))
        #expect(session.save())
        _ = session.createObject(objectClass: "van")
        #expect(session.select(polygon: columns(5.25, 5.7), mode: .replace))
        #expect(!session.save())
        #expect(session.lastError?.contains("would belong to both") == true)
        #expect(session.pointClaimConflicts.isEmpty)
        #expect(session.select(polygon: columns(5.55, 5.7), mode: .replace))
        #expect(session.save())
    }

    @Test func aPackThatAlreadyHasOneCanStillBeRepaired() throws {
        let dir = try SyntheticPack.write([SyntheticPack.car(at: simd_float2(5, 0))])
        let pack = try AnnotationPack.open(directory: dir)
        let store = SidecarStore(packDirectory: dir)
        var doc = try store.load(
            packDigest: pack.manifest.packDigest, datasetID: pack.manifest.datasetID)
        doc.sidecar.objects = [
            AnnotationObject(objectID: "a", objectClass: "car"),
            AnnotationObject(objectID: "b", objectClass: "car"),
        ]
        doc.sidecar.masks = [
            FrameMask(objectID: "a", sampleID: 0, pointIndices: [0, 1]),
            FrameMask(objectID: "b", sampleID: 0, pointIndices: [1, 12]),
        ]
        _ = try store.save(doc, change: Provenance(author: "old", operation: "seed"))
        let session = try AnnotationSession(pack: pack)
        session.operatorName = "op"
        #expect(session.pointClaimConflicts.count == 1)
        #expect(session.activate(objectID: "b") == nil)
        #expect(session.select(polygon: columns(5.55, 5.7), mode: .replace))
        #expect(session.save(), "the repair itself was refused")
        #expect(session.pointClaimConflicts.isEmpty)
    }
}

@MainActor struct QuitGuardTests {
    @Test func unsavedPhysicalWorkIsNamedWhenQuitting() async throws {
        let (session, _, _) = try openSession()
        session.workMode = .physical
        await waitForPhysical(session)
        let object = session.createObject(objectClass: "car")
        #expect(
            UnsavedWorkRegistry.shared.unsavedSummary?.contains(
                session.pack.directory.lastPathComponent) != true)
        session.physical.edit {
            PhysicalDraft.addBody(objectID: object.objectID, author: "op", session: "s", to: &$0)
        }
        #expect(
            UnsavedWorkRegistry.shared.unsavedSummary?.contains("physical-reference draft") == true)
        session.physical.discard()
        #expect(
            UnsavedWorkRegistry.shared.unsavedSummary?.contains(
                session.pack.directory.lastPathComponent) != true)
    }
}
