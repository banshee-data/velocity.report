//
//  PhysicalReferenceSessionTests.swift
//  VelocityVisualiserTests
//
//  The physical draft against a fake of the Go service: what is sent, what a
//  refusal leaves behind, and the rules the window applies around it — that a
//  review is of a saved record, that a lost answer blocks the next write, that
//  a stale answer cannot land on a newer draft, and that unsaved physical work
//  guards navigation like unsaved membership does.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

/// A stand-in for the service: one document, its revision and token, and a
/// record of every request.
final class FakePhysicalService: @unchecked Sendable {
    let lock = NSLock()
    var packDir: String
    var revision = 1
    var digest = ""
    var objects: [PhysicalObject] = []
    var membershipDigest = ""
    var requests: [(path: String, body: [String: Any])] = []
    /// When set, the next request to this path gets this answer instead.
    var override: [String: (Int, Data)] = [:]
    /// When set, the next request to this path fails in transport.
    var drop: Set<String> = []

    init(packDir: String) { self.packDir = packDir }

    func state() -> PhysicalPackState {
        PhysicalPackState(
            pack: "pack", packDir: packDir, datasetID: "ds", packDigest: "sha256:pack",
            exists: !digest.isEmpty, revision: revision, digest: digest,
            contentDigest: "sha256:content", head: true, membershipDigest: membershipDigest,
            membershipRevision: 1, stale: [],
            document: PhysicalReferenceDocument(
                schema: PhysicalReferenceDocument.schema, schemaVersion: 1, datasetID: "ds",
                packDigest: "sha256:pack", revision: revision, objects: objects))
    }

    func handle(_ request: URLRequest) throws -> (HTTPURLResponse, Data) {
        lock.lock()
        defer { lock.unlock() }
        let path = request.url!.path
        var body: [String: Any] = [:]
        if let stream = request.httpBodyStream {
            stream.open()
            var data = Data()
            var buffer = [UInt8](repeating: 0, count: 4096)
            while stream.hasBytesAvailable {
                let n = stream.read(&buffer, maxLength: buffer.count)
                if n <= 0 { break }
                data.append(buffer, count: n)
            }
            stream.close()
            body = (try? JSONSerialization.jsonObject(with: data) as? [String: Any]) ?? [:]
        } else if let data = request.httpBody {
            body = (try? JSONSerialization.jsonObject(with: data) as? [String: Any]) ?? [:]
        }
        requests.append((path, body))
        func respond(_ status: Int, _ data: Data) -> (HTTPURLResponse, Data) {
            (
                HTTPURLResponse(
                    url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!,
                data
            )
        }
        if drop.remove(path) != nil { throw URLError(.networkConnectionLost) }
        if let answer = override.removeValue(forKey: path) { return respond(answer.0, answer.1) }
        let encoder = JSONEncoder()
        switch path {
        case "/api/annotations/physical": return respond(200, try encoder.encode(state()))
        case "/api/annotations/physical/validate":
            return respond(
                200,
                try encoder.encode(
                    PhysicalEditResult(valid: true, linkProblems: [], resetReviews: [])))
        case "/api/annotations/physical/save":
            let sent = try JSONSerialization.data(withJSONObject: body["objects"] ?? [])
            objects = try JSONDecoder().decode([PhysicalObject].self, from: sent)
            for i in objects.indices {
                objects[i].body?.review.status = .proposed
                for k in objects[i].keyframes.indices {
                    objects[i].keyframes[k].review.status = .proposed
                }
            }
            revision = digest.isEmpty ? 1 : revision + 1
            digest = "sha256:rev\(revision)"
            return respond(
                200,
                try encoder.encode(
                    PhysicalEditResult(
                        valid: true, linkProblems: [], resetReviews: ["body/b"], state: state())))
        case "/api/annotations/physical/history":
            struct History: Encodable { var revisions: [PhysicalRevisionSummary] }
            let revisions = (1...max(revision, 1)).reversed().map {
                PhysicalRevisionSummary(
                    revision: $0, updatedUTC: "2026-09-29T00:00:00Z",
                    change: Provenance(author: "op"), restoredFrom: nil, digest: "sha256:rev\($0)",
                    contentDigest: "sha256:c", head: $0 == revision, objects: objects.count,
                    keyframes: 0, reviewed: 0)
            }
            return respond(200, try encoder.encode(History(revisions: revisions)))
        case "/api/annotations/physical/restore":
            revision += 1
            digest = "sha256:rev\(revision)"
            objects = []
            return respond(200, try encoder.encode(state()))
        case "/api/annotations/physical/review":
            let object = body["object_id"] as? String
            let record = body["record_id"] as? String
            for i in objects.indices where objects[i].objectID == object {
                if objects[i].body?.bodyID == record { objects[i].body?.review.status = .reviewed }
                for k in objects[i].keyframes.indices
                where objects[i].keyframes[k].keyframeID == record {
                    objects[i].keyframes[k].review.status = .reviewed
                }
            }
            revision += 1
            digest = "sha256:rev\(revision)"
            return respond(200, try encoder.encode(state()))
        default: return respond(404, Data(#"{"error":"no route","code":"not_found"}"#.utf8))
        }
    }

    func last(_ path: String) -> [String: Any]? {
        lock.lock()
        defer { lock.unlock() }
        return requests.last { $0.path == path }?.body
    }
}

@MainActor private func makeSession(
    packDir: URL, membership: String = "sha256:members", author: String = "op"
) -> (PhysicalReferenceSession, FakePhysicalService) {
    let (session, baseURL, register) = AnnotationMockURLProtocol.makeSession()
    let fake = FakePhysicalService(packDir: packDir.path)
    fake.membershipDigest = membership
    register { try fake.handle($0) }
    let physical = PhysicalReferenceSession(
        packDirectory: packDir, packDigest: "sha256:pack", sessionID: "session-1",
        client: PhysicalReferenceAPIClient(baseURL: baseURL, session: session),
        membershipDigest: { membership }, author: { author })
    physical.validationDelay = .milliseconds(1)
    return (physical, fake)
}

private func temporaryPackDir() -> URL {
    let dir = FileManager.default.temporaryDirectory.appendingPathComponent(
        "phys-\(UUID().uuidString)")
    try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    return dir
}

private func sample(_ id: Int) -> AnnotationSample {
    AnnotationSample(
        sampleID: id, sourceOrdinal: id, sourceFrameID: 0, timestampNs: 1_000 + Int64(id),
        sensorID: "s", pointCount: 0, byteOffset: 0)
}

@MainActor struct PhysicalReferenceSessionTests {
    @Test func assistanceDeclarationKeepsSavedHistoryAndSurvivesUndo() async throws {
        let (physical, fake) = makeSession(packDir: temporaryPackDir())
        fake.digest = "sha256:independent"
        var original = PhysicalBody(bodyID: "body_saved")
        original.review.status = .reviewed
        fake.objects = [PhysicalObject(objectID: "car", body: original)]
        await physical.load()
        physical.edit { $0[0].body?.length.valueM = 4 }
        #expect(physical.object("car")?.body?.review.origin == .independent)
        #expect(physical.declareAssistance(objectID: "car", source: "  replay run 7 · L5  "))
        #expect(physical.exposure["car"] == "replay run 7 · L5")
        #expect(physical.savedBody(objectID: "car") == original)
        #expect(physical.object("car")?.body?.review.origin == .trackerAssisted)
        #expect(physical.object("car")?.body?.bodyID != original.bodyID)
        physical.undo()
        #expect(physical.object("car")?.body?.length.valueM == 4)
        #expect(physical.object("car")?.body?.review.origin == .trackerAssisted)
        physical.undo()
        #expect(physical.object("car")?.body == original)
        #expect(!physical.isDirty, "Undo to unchanged saved history must not relabel it")
        physical.redo()
        #expect(physical.object("car")?.body?.review.origin == .trackerAssisted)
        #expect(await physical.save())
        #expect(fake.objects[0].body?.review.trackerSource == "replay run 7 · L5")
        physical.discard()
        #expect(physical.exposure["car"] == "replay run 7 · L5")
    }

    @Test func assistanceNeedsASourceAndDoesNotRewriteUntouchedRecords() async {
        let (physical, fake) = makeSession(packDir: temporaryPackDir())
        #expect(!physical.declareAssistance(objectID: "car", source: "run"))
        fake.digest = "sha256:saved"
        var original = PhysicalBody(bodyID: "b")
        original.review.status = .reviewed
        fake.objects = [PhysicalObject(objectID: "car", body: original)]
        await physical.load()
        #expect(!physical.declareAssistance(objectID: "car", source: " \n "))
        #expect(physical.exposure.isEmpty)
        #expect(physical.declareAssistance(objectID: "car", source: "main window"))
        #expect(!physical.isDirty)
        #expect(physical.savedBody(objectID: "car")?.review.origin == .independent)
        physical.edit { $0[0].body?.width.valueM = 2 }
        #expect(physical.object("car")?.body?.review.trackerSource == "main window")
        #expect(physical.declareAssistance(objectID: "car", source: "another estimate"))
        #expect(physical.exposure["car"] == "main window")
    }

    @Test func reviewingAfterExposureNeedsANewAssistedProposal() async {
        let (physical, fake) = makeSession(packDir: temporaryPackDir())
        fake.digest = "sha256:before-viewing"
        var pose = PhysicalKeyframe(keyframeID: "independent_pose", sampleID: 0, timestampNs: 1_000)
        pose.review.status = .reviewed
        let original = PhysicalBody(bodyID: "pending_body")
        fake.objects = [PhysicalObject(objectID: "car", body: original, keyframes: [pose])]
        await physical.load()
        physical.expose(objectIDs: ["car"], source: "online run 9")
        #expect(!physical.isDirty)
        #expect(!(await physical.review(kind: .body, objectID: "car", recordID: "pending_body")))
        #expect(fake.last("/api/annotations/physical/review") == nil)
        physical.continueAsAssisted(objectID: "car")
        #expect(physical.isDirty)
        #expect(physical.savedBody(objectID: "car") == original)
        #expect(physical.object("car")?.body?.bodyID != original.bodyID)
        #expect(physical.object("car")?.body?.review.origin == .trackerAssisted)
        #expect(
            physical.object("car")?.keyframes == [pose],
            "Reviewed independent history remains unchanged")
        // Undo returns to the saved record, unrelabelled; it still cannot be
        // reviewed as independent, so nothing is laundered by the undo.
        physical.undo()
        #expect(physical.object("car")?.body == original)
        #expect(!physical.isDirty)
        #expect(!(await physical.review(kind: .body, objectID: "car", recordID: "pending_body")))
        physical.redo()
        #expect(physical.object("car")?.body?.review.origin == .trackerAssisted)
        physical.discard()
        #expect(!physical.isDirty)
        physical.continueAsAssisted(objectID: "car")
        #expect(await physical.save())
        let savedID = physical.savedBody(objectID: "car")!.bodyID
        #expect(await physical.review(kind: .body, objectID: "car", recordID: savedID))
        #expect(physical.savedBody(objectID: "car")?.review.origin == .trackerAssisted)
    }

    @Test func undoingOneObjectLeavesAnotherSeenObjectsSavedProposalsAlone() async {
        let (physical, fake) = makeSession(packDir: temporaryPackDir())
        fake.digest = "sha256:two-objects"
        let bodyA = PhysicalBody(bodyID: "body_a")
        let bodyB = PhysicalBody(bodyID: "body_b")
        let poseB = PhysicalKeyframe(keyframeID: "kf_b", sampleID: 0, timestampNs: 1_000)
        fake.objects = [
            PhysicalObject(objectID: "a", body: bodyA),
            PhysicalObject(objectID: "b", body: bodyB, keyframes: [poseB]),
        ]
        await physical.load()
        // B's estimate was shown in Compare; A's never was.
        physical.expose(objectIDs: ["b"], source: "compare run 3")
        #expect(!physical.isDirty, "seeing an estimate is not an edit")

        physical.edit { $0[0].body?.length = PhysicalDimension(status: .observed, span: .full) }
        #expect(physical.isDirty)
        physical.undo()
        #expect(!physical.isDirty, "Undo to unchanged saved history must not relabel it")
        #expect(physical.object("b")?.body == bodyB)
        #expect(physical.object("b")?.keyframes == [poseB])
        physical.redo()
        #expect(physical.object("b")?.body?.bodyID == "body_b")
        #expect(physical.object("b")?.body?.review.origin == .independent)
        #expect(physical.object("b")?.keyframes.first?.keyframeID == "kf_b")

        // Saving A's edit writes B exactly as it was saved.
        #expect(await physical.save())
        let savedB = fake.objects.first { $0.objectID == "b" }
        #expect(savedB?.body?.bodyID == "body_b" && savedB?.body?.review.origin == .independent)
        #expect(savedB?.keyframes.map(\.keyframeID) == ["kf_b"])
        #expect(savedB?.keyframes.first?.review.origin == .independent)

        // An edit to B itself is still bound to what was seen.
        physical.edit { $0[1].body?.width = PhysicalDimension(status: .observed, span: .full) }
        #expect(physical.object("b")?.body?.review.origin == .trackerAssisted)
        #expect(physical.object("b")?.body?.review.trackerSource == "compare run 3")
        #expect(physical.object("b")?.body?.bodyID != "body_b")
        #expect(physical.object("b")?.keyframes.first?.keyframeID == "kf_b")
    }

    @Test func explicitDeclarationForksAPendingSavedProposal() async {
        let (physical, fake) = makeSession(packDir: temporaryPackDir())
        fake.digest = "sha256:pending"
        let original = PhysicalBody(bodyID: "pending")
        fake.objects = [PhysicalObject(objectID: "car", body: original)]
        await physical.load()
        #expect(physical.declareAssistance(objectID: "car", source: "main-window estimate"))
        #expect(physical.object("car")?.body?.review.origin == .trackerAssisted)
        #expect(physical.savedBody(objectID: "car") == original)
    }

    @Test func loadsReadyOnlyWhenTheServiceHoldsThisVeryFolder() async {
        let dir = temporaryPackDir()
        let (physical, fake) = makeSession(packDir: dir)
        await physical.load()
        #expect(physical.availability == .ready)
        #expect(physical.state?.revision == 1)
        #expect(!physical.isDirty)
        let query = fake.requests.first?.path
        #expect(query == "/api/annotations/physical")

        let (elsewhere, other) = makeSession(packDir: temporaryPackDir())
        other.packDir = dir.path
        await elsewhere.load()
        guard case .unavailable(let reason) = elsewhere.availability else {
            Issue.record("a pack the service does not hold was editable")
            return
        }
        #expect(reason.contains("service's copy"))
        elsewhere.edit { $0.append(PhysicalObject(objectID: "x")) }
        #expect(elsewhere.draft.isEmpty, "an unavailable session took an edit")
    }

    @Test func aRefusedLoadSaysWhyAndANewerSchemaOpensReadOnly() async {
        let dir = temporaryPackDir()
        let (physical, fake) = makeSession(packDir: dir)
        fake.override["/api/annotations/physical"] = (
            404, Data(#"{"error":"no readable pack","code":"pack_not_found"}"#.utf8)
        )
        await physical.load()
        guard case .unavailable(let reason) = physical.availability else {
            Issue.record("expected unavailable")
            return
        }
        #expect(reason.contains("--lidar-annotation-dir"))

        var future = fake.state()
        future.document.schemaVersion = 2
        fake.override["/api/annotations/physical"] = (200, try! JSONEncoder().encode(future))
        await physical.load()
        guard case .readOnly = physical.availability else {
            Issue.record("expected read-only for a newer schema")
            return
        }
        physical.edit { $0.append(PhysicalObject(objectID: "x")) }
        #expect(physical.draft.isEmpty, "a read-only document took an edit")
    }

    @Test func editsAreUndoableAndDiscardRestoresTheSavedDocument() async {
        let (physical, _) = makeSession(packDir: temporaryPackDir())
        await physical.load()
        physical.edit {
            PhysicalDraft.addBody(objectID: "obj_1", author: "op", session: "s", to: &$0)
        }
        physical.edit {
            PhysicalDraft.addKeyframe(
                objectID: "obj_1", sample: sample(0), author: "op", session: "s", to: &$0)
        }
        #expect(physical.isDirty && physical.canUndo)
        physical.undo()
        #expect(physical.object("obj_1")?.keyframes.isEmpty == true)
        physical.redo()
        #expect(physical.object("obj_1")?.keyframes.count == 1)
        physical.undo()
        physical.undo()
        #expect(!physical.isDirty && !physical.canUndo && physical.canRedo)
        physical.redo()
        physical.discard()
        #expect(!physical.isDirty && !physical.canUndo && !physical.canRedo)
        // An edit that changes nothing is not an undo step.
        physical.edit { _ in }
        #expect(!physical.canUndo)
    }

    @Test func saveSendsTheBaseTheMembershipAndTheDraft() async throws {
        let (physical, fake) = makeSession(packDir: temporaryPackDir(), membership: "sha256:m1")
        await physical.load()
        physical.edit {
            PhysicalDraft.addBody(objectID: "obj_1", author: "op", session: "s", to: &$0)
        }
        #expect(await physical.save())
        let sent = try #require(fake.last("/api/annotations/physical/save"))
        #expect(sent["base_revision"] as? Int == 1)
        #expect(sent["base_digest"] as? String == "")
        #expect(sent["membership_digest"] as? String == "sha256:m1")
        #expect(sent["author"] as? String == "op")
        #expect(
            sent["pack"] as? String
                == PhysicalReferenceAPIClient.handle(for: physical.localDirectory))
        #expect((sent["objects"] as? [Any])?.count == 1)
        #expect(!physical.isDirty)
        #expect(physical.state?.digest == "sha256:rev1")
        #expect(physical.lastNote?.contains("Review reset on body/b") == true)

        // The next save is made from the saved revision's token.
        physical.edit { PhysicalDraft.removeBody(objectID: "obj_1", from: &$0) }
        #expect(await physical.save())
        let next = try #require(fake.last("/api/annotations/physical/save"))
        #expect(next["base_digest"] as? String == "sha256:rev1")
    }

    @Test func aSaveWithoutAnAuthorIsRefusedBeforeTheRoundTrip() async {
        let (session, baseURL, register) = AnnotationMockURLProtocol.makeSession()
        let fake = FakePhysicalService(packDir: temporaryPackDir().path)
        register { try fake.handle($0) }
        let physical = PhysicalReferenceSession(
            packDirectory: URL(fileURLWithPath: fake.packDir), packDigest: "sha256:pack",
            sessionID: "s", client: PhysicalReferenceAPIClient(baseURL: baseURL, session: session),
            membershipDigest: { "" }, author: { "  " })
        await physical.load()
        physical.edit { PhysicalDraft.addBody(objectID: "o", author: "", session: "s", to: &$0) }
        #expect(!(await physical.save()))
        #expect(physical.lastError?.contains("Labelled by") == true)
        #expect(fake.last("/api/annotations/physical/save") == nil)
    }

    @Test func anInvalidSaveKeepsTheDraftAndShowsTheDiagnostics() async throws {
        let (physical, fake) = makeSession(packDir: temporaryPackDir())
        await physical.load()
        physical.edit {
            PhysicalDraft.addBody(objectID: "obj_1", author: "op", session: "s", to: &$0)
        }
        let draft = physical.draft
        let refusal = PhysicalEditResult(
            valid: false,
            invalid:
                "object \"obj_1\": a record that declares bounds must state its uncertainty assumptions",
            linkProblems: [], resetReviews: [])
        fake.override["/api/annotations/physical/save"] = (422, try JSONEncoder().encode(refusal))
        #expect(!(await physical.save()))
        #expect(physical.draft == draft && physical.isDirty)
        #expect(physical.validation?.invalid?.contains("uncertainty assumptions") == true)
        #expect(physical.lastError?.contains("uncertainty assumptions") == true)
        #expect(!physical.needsReload, "a refusal on content is not an unknown outcome")
    }

    @Test func aLostAnswerOrAConflictBlocksTheNextWriteUntilReload() async {
        let (physical, fake) = makeSession(packDir: temporaryPackDir())
        await physical.load()
        physical.edit { PhysicalDraft.addBody(objectID: "o", author: "op", session: "s", to: &$0) }
        fake.drop.insert("/api/annotations/physical/save")
        #expect(!(await physical.save()))
        #expect(physical.needsReload && physical.isDirty)
        #expect(!(await physical.save()), "wrote again without reading after an unknown outcome")
        #expect(fake.requests.filter { $0.path == "/api/annotations/physical/save" }.count == 1)

        // Reload keeping the draft: the draft survives, re-based on the head.
        fake.revision = 4
        fake.digest = "sha256:rev4"
        await physical.load(keepDraft: true)
        #expect(!physical.needsReload && physical.isDirty && physical.state?.revision == 4)

        fake.override["/api/annotations/physical/save"] = (
            409, Data(#"{"error":"changed","code":"conflict"}"#.utf8)
        )
        #expect(!(await physical.save()))
        #expect(physical.needsReload && physical.lastErrorCode == "conflict" && physical.isDirty)
        await physical.load()
        #expect(!physical.isDirty, "a plain reload discards the draft")
    }

    @Test func reviewIsOfASavedRecordAndNeverOfADraft() async throws {
        let (physical, fake) = makeSession(packDir: temporaryPackDir())
        await physical.load()
        physical.edit { PhysicalDraft.addBody(objectID: "o", author: "op", session: "s", to: &$0) }
        let bodyID = try #require(physical.object("o")?.body?.bodyID)
        #expect(!(await physical.review(kind: .body, objectID: "o", recordID: bodyID)))
        #expect(physical.lastError?.contains("Save or discard") == true)
        #expect(fake.last("/api/annotations/physical/review") == nil)

        #expect(await physical.save())
        #expect(await physical.review(kind: .body, objectID: "o", recordID: bodyID))
        let sent = try #require(fake.last("/api/annotations/physical/review"))
        #expect(sent["kind"] as? String == "body" && sent["record_id"] as? String == bodyID)
        #expect(sent["reviewer"] as? String == "op")
        #expect(sent["base_digest"] as? String == "sha256:rev1")
        #expect(physical.savedBody(objectID: "o")?.review.status == .reviewed)
        #expect(!physical.isDirty)
    }

    @Test func aValidationAnswerForAnOlderDraftIsDropped() async throws {
        let (physical, fake) = makeSession(packDir: temporaryPackDir())
        await physical.load()
        physical.validationDelay = .seconds(60)
        physical.edit { PhysicalDraft.addBody(objectID: "o", author: "op", session: "s", to: &$0) }
        await physical.validateNow()
        #expect(physical.validation?.valid == true)
        // A refusal arrives for the draft as it was; the draft has moved on.
        let stale = PhysicalEditResult(
            valid: false, invalid: "old", linkProblems: [], resetReviews: [])
        fake.override["/api/annotations/physical/validate"] = (200, try JSONEncoder().encode(stale))
        let asked = fake.requests.filter { $0.path == "/api/annotations/physical/validate" }.count
        let pending = Task { await physical.validateNow() }
        // Wait until that request has reached the service, so the edit below
        // lands while its answer is in flight.
        for _ in 0..<500
        where fake.requests.filter({ $0.path == "/api/annotations/physical/validate" }).count
            == asked
        { await Task.yield() }
        physical.edit {
            PhysicalDraft.addKeyframe(
                objectID: "o", sample: sample(1), author: "op", session: "s", to: &$0)
        }
        await pending.value
        #expect(physical.validation?.invalid != "old")
    }

    @Test func describeSaveNamesResetsAndRevisedBodies() {
        let text = PhysicalReferenceSession.describeSave(
            PhysicalEditResult(
                valid: true, linkProblems: [], resetReviews: ["keyframe/k1"],
                renamedBodies: ["body_a": "body_b"]), revision: 7)
        #expect(
            text.contains("revision 7") && text.contains("keyframe/k1")
                && text.contains("body_b (was body_a)"))
    }
}

/// The session side: physical authoring pauses the main-view link, places in
/// the pack's frame through a rotated grid, and guards navigation.
@MainActor struct AnnotationPhysicalModeTests {
    private func openSession() throws -> (AnnotationSession, FakePhysicalService) {
        let dir = try SyntheticPack.write([
            SyntheticPack.car(at: simd_float2(5, 0)), SyntheticPack.car(at: simd_float2(6, 0)),
        ])
        let pack = try AnnotationPack.open(directory: dir)
        let (urlSession, baseURL, register) = AnnotationMockURLProtocol.makeSession()
        let fake = FakePhysicalService(packDir: dir.resolvingSymlinksInPath().path)
        register { try fake.handle($0) }
        let session = try AnnotationSession(
            pack: pack,
            physicalClient: PhysicalReferenceAPIClient(baseURL: baseURL, session: urlSession))
        session.operatorName = "op"
        return (session, fake)
    }

    private func waitForLoad(_ session: AnnotationSession) async {
        for _ in 0..<200 where session.physical.availability != .ready {
            try? await Task.sleep(for: .milliseconds(10))
        }
    }

    @Test func physicalModeIsBlindToTheMainView() async throws {
        let (session, _) = try openSession()
        session.syncWithMainView = true
        session.workMode = .physical
        #expect(!session.syncWithMainView)
        session.syncWithMainView = true
        #expect(!session.syncWithMainView, "the link came back on during physical authoring")
        await waitForLoad(session)
        #expect(session.physical.availability == .ready)
        session.workMode = .points
        session.syncWithMainView = true
        #expect(session.syncWithMainView)
    }

    @Test func aTopViewClickPlacesInThePackFrameThroughTheGridRotation() async throws {
        let (session, _) = try openSession()
        session.workMode = .physical
        await waitForLoad(session)
        let object = session.createObject(objectClass: "car")
        session.gridAzimuthDeg = 90
        // With the views turned a quarter, screen right is world +Y.
        session.placePhysicalAnchor(in: .top, at: simd_float2(2, 0))
        let k = try #require(session.physicalKeyframe)
        #expect(abs((k.position.xM ?? 99) - 0) < 1e-5)
        #expect(abs((k.position.yM ?? 99) - 2) < 1e-5)
        #expect(k.position.zM == nil)
        #expect(k.sampleID == session.currentSample?.sampleID)
        #expect(k.timestampNs == session.currentSample?.timestampNs)
        // An elevation sets only the height, of a position that exists.
        session.placePhysicalAnchor(in: .front, at: simd_float2(9, 1.25))
        let raised = try #require(session.physicalKeyframe)
        #expect(raised.position.xM == k.position.xM && raised.position.yM == k.position.yM)
        #expect(abs((raised.position.zM ?? 0) - 1.25) < 1e-5)
        _ = object
    }

    @Test func anElevationClickWithNoPositionIsRefused() async throws {
        let (session, _) = try openSession()
        session.workMode = .physical
        await waitForLoad(session)
        _ = session.createObject(objectClass: "car")
        session.placePhysicalAnchor(in: .side, at: simd_float2(0, 1))
        #expect(session.physicalKeyframe == nil)
        #expect(session.physical.lastError?.contains("Top view") == true)
    }

    @Test func unsavedPhysicalWorkGuardsNavigationUntilDiscarded() async throws {
        let (session, _) = try openSession()
        session.workMode = .physical
        await waitForLoad(session)
        let object = session.createObject(objectClass: "car")
        // An object with no selection leaves no unsaved membership, so only
        // the physical draft is unsaved.
        #expect(session.navigationGuard() == nil)
        session.physical.edit {
            PhysicalDraft.addBody(objectID: object.objectID, author: "op", session: "s", to: &$0)
        }
        #expect(session.navigationGuard() == .unsavedPhysical)
        #expect(session.stepForward() == .unsavedPhysical)
        #expect(session.sampleIndex == 0)
        session.discardAllUnsaved()
        #expect(session.navigationGuard() == nil)
        #expect(session.stepForward() == nil)
    }

    @Test func pointsModeNeverAsksTheService() throws {
        let (session, fake) = try openSession()
        #expect(session.navigationGuard() == nil)
        #expect(session.physicalProgress(objectID: "x") == nil)
        #expect(fake.requests.isEmpty)
    }
}
