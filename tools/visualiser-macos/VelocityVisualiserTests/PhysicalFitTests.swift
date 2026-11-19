//
//  PhysicalFitTests.swift
//  VelocityVisualiserTests
//
//  Fitting a physical reference to reviewed points, from the client's side:
//  the service's answer as Go writes it (the fixture Go's
//  TestPhysicalFitSwiftFixture keeps), how it enters the draft, and the rules
//  that replaced the evidence pickers.
//

import Foundation
import Testing

@testable import VelocityVisualiser

/// The repository root, from this file's own path.
private let repositoryRoot = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()

private func fitFixture() throws -> Data {
    try Data(
        contentsOf: repositoryRoot.appendingPathComponent(
            "internal/lidar/annotation/testdata/physical_fit_fixture.json"))
}

private func fitResult() throws -> PhysicalFitResult {
    try JSONDecoder().decode(PhysicalFitResult.self, from: try fitFixture())
}

struct PhysicalFitTests {
    @Test func lifecycleRequestIsExplicitAndLegacyRequestsRemainCompatible() throws {
        var request = PhysicalReferenceAPIClient.FitRequest(
            pack: "pack", packDigest: "sha256:p", membershipDigest: "sha256:m", objectID: "car",
            samples: [], author: "op", session: "s")
        var json = try #require(
            JSONSerialization.jsonObject(with: JSONEncoder().encode(request)) as? [String: Any])
        #expect(json["lifecycle"] == nil)
        request.lifecycle = true
        json = try #require(
            JSONSerialization.jsonObject(with: JSONEncoder().encode(request)) as? [String: Any])
        #expect(json["lifecycle"] as? Bool == true)
    }
    @Test func lifecycleReplacesAllDraftPosesButRequiresExplicitMetadata() throws {
        var fit = try fitResult()
        var objects: [PhysicalObject] = []
        #expect(PhysicalDraft.applyFit(fit, scope: .lifecycle, to: &objects) == 0)
        #expect(objects.isEmpty)
        let body = try #require(fit.object?.body)
        fit.lifecycle = PhysicalLifecycle(
            schema: "physical_lifecycle_refinement_v1", bodyID: body.bodyID, packDigest: "fixture",
            informationUntil: "9007199254740993", extentSamples: [], frames: [])
        objects = [
            PhysicalObject(
                objectID: fit.objectID,
                keyframes: [PhysicalKeyframe(keyframeID: "old", sampleID: 999, timestampNs: 1)])
        ]
        #expect(
            PhysicalDraft.applyFit(fit, scope: .lifecycle, to: &objects)
                == fit.object?.keyframes.count)
        #expect(objects[0].keyframes.allSatisfy { $0.sampleID != 999 })
        #expect(objects[0].body == body)
    }
    @Test func coverageInspectionPrefersAFrameWithAFittedPose() throws {
        let fit = try fitResult()
        let posed = try #require(fit.object?.keyframes.first?.sampleID)
        #expect(
            fit.inspectionSample(for: PhysicalFaceCoverage(name: "Front", samples: [-1, posed]))
                == posed)
        #expect(fit.inspectionSample(for: PhysicalFaceCoverage(name: "Front", samples: [])) == nil)
        #expect(fit.inspectionSample(for: PhysicalFaceCoverage(name: "Front", samples: [-1])) == -1)
    }
    @Test func extentInspectionIgnoresUnsupportedMaximumBoxes() throws {
        var fit = try fitResult()
        var object = try #require(fit.object)
        var body = try #require(object.body)
        body.length.status = .observed
        body.length.span = .full
        body.length.support = PhysicalSupport(frames: [3])
        body.width.status = .observed
        body.width.span = .full
        body.width.support = PhysicalSupport(frames: [3])
        var pose = try #require(object.keyframes.first)
        pose.sampleID = 3
        object.body = body
        object.keyframes = [pose]
        fit.fitted.objects = [object]
        var supported = try #require(fit.frames.first)
        supported.sampleID = 3
        supported.axis = "resolved"
        supported.skipped = nil
        supported.usedFor = ["length", "width"]
        supported.lengthSpanM = 4
        supported.widthSpanM = 2
        var unsupported = supported
        unsupported.sampleID = 4
        unsupported.lengthSpanM = 40
        unsupported.widthSpanM = 20
        // Candidate diagnostics can survive the final support/pose trimming.
        unsupported.usedFor = ["length", "width"]
        fit.frames = [unsupported, supported]
        #expect(fit.extentSample(width: false) == 3)
        #expect(fit.extentSample(width: true) == 3)
        var otherPose = pose
        otherPose.sampleID = 4
        fit.fitted.objects[0].keyframes.append(otherPose)
        #expect(fit.extentSample(width: true) == 3)
        fit.fitted.objects[0].body?.width.support = PhysicalSupport(frames: [3, 4])
        fit.fitted.objects[0].keyframes = [pose]
        #expect(fit.extentSample(width: true) == 3)
        fit.frames[1].axis = "axial"
        #expect(fit.extentSample(width: false) == nil)
        fit.frames[1].axis = "resolved"
        fit.frames[1].skipped = "occluded"
        #expect(fit.extentSample(width: true) == nil)
        fit.frames[1].skipped = nil
        fit.fitted.objects[0].body?.length.status = .inferred
        #expect(fit.extentSample(width: false) == nil)
        fit.fitted.objects[0].keyframes = []
        #expect(fit.extentSample(width: true) == nil)
    }
    @Test func bodyCoverageDoesNotCountUnresolvedOrSkippedFrames() throws {
        var fit = try fitResult()
        var frame = try #require(fit.frames.first)
        frame.axis = "resolved"
        frame.skipped = nil
        frame.front.seen = true
        frame.rear.seen = false
        frame.left.seen = false
        frame.right.seen = false
        fit.frames = [frame, frame]
        #expect(fit.faceCoverage.first?.samples == [frame.sampleID])
        #expect(fit.faceCoverage.dropFirst().allSatisfy { $0.samples.isEmpty })
        fit.frames[0].axis = "axial"
        fit.frames[1].skipped = "occluded"
        #expect(fit.faceCoverage.allSatisfy { $0.samples.isEmpty })
    }
    @Test func goFitDecodesWithItsDiagnostics() throws {
        let fit = try fitResult()
        #expect(fit.objectID == "car")
        let object = try #require(fit.object)
        let body = try #require(object.body)
        #expect(body.length.choice == .full && body.length.status == .observed)
        #expect(body.height.choice == .atLeast)
        #expect(body.review.method == "fit:mask_v1" && body.review.status == .proposed)
        #expect(!(body.review.uncertaintyAssumptions ?? "").isEmpty)
        #expect(!object.keyframes.isEmpty)
        let posed = try #require(object.keyframes.first)
        let frame = try #require(fit.frame(sampleID: posed.sampleID))
        #expect(frame.returns > 0 && !(frame.front.terms ?? []).isEmpty)
    }

    @Test func fittingTheObjectTakesTheSizeAndEveryPose() throws {
        let fit = try fitResult()
        let fitted = try #require(fit.object)
        // A pose at a frame the fit does not touch stays; one it fits is replaced.
        let untouched = PhysicalKeyframe(keyframeID: "kf_mine", sampleID: 999, timestampNs: 1)
        let replaced = PhysicalKeyframe(
            keyframeID: "kf_old", sampleID: fitted.keyframes[0].sampleID, timestampNs: 1)
        var objects = [
            PhysicalObject(
                objectID: "car", body: PhysicalBody(bodyID: "body_mine"),
                keyframes: [untouched, replaced])
        ]
        let placed = PhysicalDraft.applyFit(fit, scope: .object, to: &objects)
        #expect(placed == fitted.keyframes.count)
        #expect(objects[0].body == fitted.body)
        #expect(objects[0].keyframe(sampleID: 999) == untouched)
        #expect(objects[0].keyframes.allSatisfy { $0.keyframeID != "kf_old" })
        #expect(objects[0].keyframes.count == fitted.keyframes.count + 1)
    }

    @Test func fittingOnePoseKeepsTheDraftsSize() throws {
        let fit = try fitResult()
        let fitted = try #require(fit.object)
        let sample = fitted.keyframes[0].sampleID
        var mine = PhysicalBody(bodyID: "body_mine")
        mine.length.valueM = 4
        var objects = [PhysicalObject(objectID: "car", body: mine)]
        #expect(PhysicalDraft.applyFit(fit, scope: .pose(sampleID: sample), to: &objects) == 1)
        #expect(objects[0].body == mine)
        #expect(objects[0].keyframes.map(\.sampleID) == [sample])
        // With no size in the draft, the fitted one comes too.
        var empty: [PhysicalObject] = []
        PhysicalDraft.applyFit(fit, scope: .pose(sampleID: sample), to: &empty)
        #expect(empty.first?.body == fitted.body)
    }

    @Test func aDimensionIsStatedWithoutChoosingItsEvidence() {
        var d = PhysicalDimension()
        PhysicalDraft.setChoice(.full, of: &d, citing: 7)
        #expect(d.status == .inferred && d.span == .full && d.support.frameList == [7])
        PhysicalDraft.setChoice(.atLeast, of: &d, citing: 8)
        #expect(d.status == .observed && d.span == .partial && d.support.frameList == [8])
        #expect(d.upperM == nil && d.valueM == nil)
        PhysicalDraft.setChoice(.unknown, of: &d, citing: 8)
        #expect(d == PhysicalDimension())
        // A dimension that already has evidence keeps it.
        var fitted = PhysicalDimension(
            status: .observed, span: .full, lowerM: 4, upperM: 5, valueM: 4.5,
            support: PhysicalSupport(frames: [1, 2]))
        PhysicalDraft.setChoice(.full, of: &fitted, citing: 9)
        #expect(fitted.status == .observed && fitted.support.frameList == [1, 2])
    }

    @Test func anEndIsSeenHereOrPlacedByTheLength() {
        var k = PhysicalKeyframe(keyframeID: "kf", sampleID: 5, timestampNs: 1)
        k.yaw.axis = .resolved
        var body = PhysicalBody(bodyID: "b")
        body.length = PhysicalDimension(
            status: .observed, span: .full, lowerM: 4, upperM: 5,
            support: PhysicalSupport(frames: [2]))
        PhysicalDraft.setEnd(seen: true, \.front, of: &k, body: body)
        #expect(k.front.status == .observed && k.front.support.frameList == [5])
        PhysicalDraft.setEnd(seen: false, \.front, of: &k, body: body)
        #expect(k.front.status == .inferred && k.front.support.frameList == [2, 5])
        PhysicalDraft.setEnd(seen: false, \.rear, of: &k, body: PhysicalBody(bodyID: "partial"))
        #expect(k.rear == PhysicalEndpoint())
        k.yaw.axis = .frontRearAmbiguous
        PhysicalDraft.setEnd(seen: true, \.front, of: &k, body: body)
        #expect(k.front == PhysicalEndpoint(), "no end is named while the axis is ambiguous")
    }

    @Test func aTypedPositionIsAnObservationOfItsFrame() {
        var k = PhysicalKeyframe(keyframeID: "kf", sampleID: 3, timestampNs: 1)
        PhysicalDraft.setPosition(\.xM, 1.5, of: &k)
        #expect(k.position.status == .observed && k.position.support.frameList == [3])
        #expect(k.position.xM == 1.5)
    }

    @Test func handMadeBoundsStateTheirAssumptions() throws {
        var manual = PhysicalBody(bodyID: "b")
        manual.length = PhysicalDimension(
            status: .inferred, span: .full, lowerM: 4, upperM: 5,
            support: PhysicalSupport(frames: [1]))
        var fittedBody = try #require(try fitResult().object?.body)
        let fittedAssumptions = fittedBody.review.uncertaintyAssumptions
        var objects = [
            PhysicalObject(objectID: "a", body: manual),
            PhysicalObject(objectID: "b", body: fittedBody),
        ]
        PhysicalDraft.fillManualAssumptions(&objects)
        #expect(objects[0].body?.review.uncertaintyAssumptions == PhysicalDraft.manualAssumptions)
        #expect(objects[1].body?.review.uncertaintyAssumptions == fittedAssumptions)
        // An unknown body declares nothing and needs no assumptions.
        fittedBody = PhysicalBody(bodyID: "c")
        var empty = [PhysicalObject(objectID: "c", body: fittedBody)]
        PhysicalDraft.fillManualAssumptions(&empty)
        #expect(empty[0].body?.review.uncertaintyAssumptions == nil)
    }

    @Test @MainActor func theSessionFitsIntoTheDraftAsOneUndoStep() async throws {
        let (session, baseURL, register) = AnnotationMockURLProtocol.makeSession()
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(
            "fit-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let fake = FakePhysicalService(packDir: dir.path)
        fake.membershipDigest = "sha256:members"
        register { try fake.handle($0) }
        let physical = PhysicalReferenceSession(
            packDirectory: dir, packDigest: "sha256:pack", sessionID: "session-1",
            client: PhysicalReferenceAPIClient(baseURL: baseURL, session: session),
            membershipDigest: { "sha256:members" }, author: { "op" })
        physical.validationDelay = .milliseconds(1)
        await physical.load()
        fake.override["/api/lidar/annotations/physical/fit"] = (200, try fitFixture())
        #expect(await physical.fit(objectID: "car", scope: .object))
        let sent = try #require(fake.last("/api/lidar/annotations/physical/fit"))
        #expect(sent["object_id"] as? String == "car")
        #expect(sent["membership_digest"] as? String == "sha256:members")
        #expect(physical.object("car")?.body == (try fitResult().object?.body))
        #expect(physical.lastFit?.objectID == "car")
        #expect(physical.lastNote?.hasPrefix("Fitted length") == true)
        physical.undo()
        #expect(physical.object("car") == nil)
        #expect(physical.lastFit == nil)
        #expect(physical.lastNote == nil)
        physical.redo()
        #expect(physical.object("car")?.body != nil)
        #expect(physical.lastFit == nil, "Redo must not revive an unbound fit diagnostic")
        fake.override["/api/lidar/annotations/physical/fit"] = (200, try fitFixture())
        #expect(await physical.fit(objectID: "car", scope: .object))
        physical.edit { _ in }
        #expect(physical.lastFit != nil, "A no-op edit does not invalidate supporting evidence")
        physical.edit { $0.removeAll() }
        #expect(physical.lastFit == nil)
        fake.override["/api/lidar/annotations/physical/fit"] = (200, try fitFixture())
        #expect(await physical.fit(objectID: "car", scope: .object))
        physical.beginGesture()
        physical.updateGesture { $0.removeAll() }
        #expect(physical.lastFit == nil, "Stale fit links disappear during the drag")
        physical.endGesture()
        fake.override["/api/lidar/annotations/physical/fit"] = (200, try fitFixture())
        #expect(await physical.fit(objectID: "car", scope: .object))
        physical.discard()
        #expect(physical.lastFit == nil)
        fake.override["/api/lidar/annotations/physical/fit"] = (200, try fitFixture())
        #expect(await physical.fit(objectID: "car", scope: .object))
        await physical.load()
        #expect(physical.lastFit == nil)
        // A refusal leaves the draft alone and says why.
        fake.override["/api/lidar/annotations/physical/fit"] = (
            422,
            Data(
                #"{"error":"3 of object \"car\"'s masks are still proposed","code":"invalid"}"#.utf8
            )
        )
        #expect(!(await physical.fit(objectID: "car", scope: .object)))
        #expect(physical.lastError?.contains("still proposed") == true)
        #expect(physical.object("car") == nil)
        fake.override["/api/lidar/annotations/physical/fit"] = (200, try fitFixture())
        #expect(await physical.fit(objectID: "car", scope: .object))
        #expect(await physical.save())
        #expect(!physical.isDirty)
        #expect(physical.lastFit != nil, "Saving retains the proposal's support-frame links")
        #expect(await physical.restore(revision: 1))
        #expect(physical.lastFit == nil)
    }
}
