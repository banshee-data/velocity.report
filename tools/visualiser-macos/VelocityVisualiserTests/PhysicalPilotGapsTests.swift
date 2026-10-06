//
//  PhysicalPilotGapsTests.swift
//  VelocityVisualiserTests
//
//  The last gaps before the pilot: intensity presence per frame, following
//  instants and elevations in Compare, the length handle, the proposal
//  population, and the parts of the physical-reference work that had no test
//  of their own: overlay styles, handle positions, progress lines, service
//  refusals, client requests, and the renderer's flat background.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

private let repositoryRoot = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()

private func source(_ relative: String) throws -> String {
    try String(
        contentsOf: URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().appendingPathComponent("VelocityVisualiser/\(relative)"),
        encoding: .utf8
    ).filter { !$0.isWhitespace }
}

private func reportFixture() throws -> Data {
    try Data(
        contentsOf: repositoryRoot.appendingPathComponent(
            "internal/lidar/perframeeval/testdata/physical_report_fixture.json"))
}

private func fixturePackDigest() throws -> String {
    let json = try JSONSerialization.jsonObject(with: reportFixture()) as! [String: Any]
    return ((json["physical"] as! [String: Any])["reference"] as! [String: Any])["pack_digest"]
        as! String
}

// MARK: - Intensity presence per frame

struct IntensityPresenceTests {
    @Test func availabilityComesFromThePackAndThenTheSample() {
        #expect(IntensityAvailability(hasIntensity: false, sample: true) == .absent)
        #expect(IntensityAvailability(hasIntensity: false, sample: nil) == .absent)
        #expect(IntensityAvailability(hasIntensity: true, sample: nil) == .packFlag)
        #expect(IntensityAvailability(hasIntensity: true, sample: true) == .measured)
        #expect(IntensityAvailability(hasIntensity: true, sample: false) == .absentThisSample)
        #expect(
            IntensityAvailability.measured.available && IntensityAvailability.packFlag.available)
        #expect(
            !IntensityAvailability.absentThisSample.available
                && !IntensityAvailability.absent.available)
        let caveats = [IntensityAvailability.measured, .absentThisSample, .packFlag, .absent].map(
            \.caveat)
        #expect(Set(caveats).count == 4)
        #expect(caveats[1].contains("zero-fill") && caveats[2].contains("per frame"))
    }

    @Test func aSampleWithoutTheFieldDecodesAsUnknown() throws {
        let json = """
            {"sample_id": 0, "source_ordinal": 1, "source_frame_id": 100, "timestamp_ns": 5,
             "sensor_id": "s", "point_count": 0, "byte_offset": 0}
            """
        let legacy = try JSONDecoder().decode(AnnotationSample.self, from: Data(json.utf8))
        #expect(legacy.hasIntensity == nil)
        let flagged = try JSONDecoder().decode(
            AnnotationSample.self,
            from: Data(
                json.replacingOccurrences(
                    of: "\"byte_offset\": 0", with: "\"byte_offset\": 0, \"has_intensity\": false"
                ).utf8))
        #expect(flagged.hasIntensity == false)
    }

    @MainActor @Test func aMixedPackReadsEachFrameOnItsOwnTerms() throws {
        let points: [SyntheticPack.Point] = [(0, 5, 1, 1), (2, 5, 1, 1)]
        let dir = try SyntheticPack.write(
            [points, points, points], intensities: [[7, 9], [0, 0], [3, 4]], hasIntensity: true,
            sampleIntensity: [true, false, nil])
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        session.inspectIntensity = true
        let vp = OrthoViewport(
            halfHeight: 10, size: CGSize(width: 400, height: 400), centre: simd_float2(1, 5))
        let at = vp.screenPoint(from: session.basis(.top).project(simd_float3(0, 5, 1)))

        #expect(session.intensityAvailability == .measured)
        session.inspectReturns(in: .top, viewport: vp, at: at)
        #expect(session.inspection.current?.raw == 7)
        #expect(
            session.intensityDisplay.table(available: session.intensityAvailability.available)[7]
                != SIMD4(IntensityDisplaySpec.missingColour, 1))

        #expect(session.stepForward() == nil)
        #expect(session.intensityAvailability == .absentThisSample)
        session.inspectReturns(in: .top, viewport: vp, at: at)
        #expect(
            session.inspection.current != nil && session.inspection.current?.raw == nil,
            "a zero-filled frame read as measured zero")
        #expect(
            session.intensityDisplay.table(available: session.intensityAvailability.available)
                .allSatisfy { $0 == SIMD4(IntensityDisplaySpec.missingColour, 1) })

        #expect(session.stepForward() == nil)
        #expect(session.intensityAvailability == .packFlag)
        session.inspectReturns(in: .top, viewport: vp, at: at)
        #expect(session.inspection.current?.raw == 3)
    }
}

// MARK: - Following and elevations in Compare

struct CompareFollowingAndElevationTests {
    @Test func followingInstantsDecodeAndIndexBySample() throws {
        let report = try PhysicalComparisonReport.decode(
            reportFixture(), packDigest: fixturePackDigest())
        let arm = report.arms[0]
        #expect(arm.following.count == 20)
        let first = try #require(arm.following.first)
        #expect(first.followerObjectID == "obj_follow" && first.leaderObjectID == "obj_lead")
        #expect(first.referenceGap?.value == 5.5 && first.referenceGap?.status == .observed)
        #expect(first.predictedGap?.followerTrack == "seq-000001")
        #expect(report.following(arm: 0, sampleID: first.sampleID).contains(first))
        #expect(report.following(arm: 9, sampleID: 0).isEmpty)
        #expect(report.following(arm: 0, sampleID: 999).isEmpty)
    }

    @Test func aReportWithoutFollowingStillDecodes() throws {
        var json = try JSONSerialization.jsonObject(with: reportFixture()) as! [String: Any]
        var physical = json["physical"] as! [String: Any]
        for arm in ["arm_a", "arm_b"] {
            var a = physical[arm] as! [String: Any]
            a.removeValue(forKey: "following")
            physical[arm] = a
        }
        json["physical"] = physical
        let report = try PhysicalComparisonReport.decode(
            JSONSerialization.data(withJSONObject: json), packDigest: fixturePackDigest())
        #expect(report.arms.allSatisfy { $0.following.isEmpty })
    }

    @Test func elevationMarksShowEachLayerWhereTheViewLooks() throws {
        let report = try PhysicalComparisonReport.decode(
            reportFixture(), packDigest: fixturePackDigest())
        let instants = report.instants(arm: 0, sampleID: 0)
        let withBoth = try #require(instants.first { $0.reference != nil && $0.prediction != nil })
        // Front looks along +Y and the fixture's cars sit at y = 0, on the
        // plane, which belongs to the view looking at it.
        let front = CompareElevationMarks.marks([withBoth], basis: OrthoViewBasis(.front))
        #expect(front.count == 2)
        #expect(front.map(\.isPrediction) == [false, true])
        #expect(front.allSatisfy { $0.objectID == withBoth.objectID })
        // Side looks along -X, and the fixture's cars are at x > 0, behind it.
        #expect(CompareElevationMarks.marks([withBoth], basis: OrthoViewBasis(.side)).isEmpty)
        #expect(CompareElevationMarks.marks([withBoth], basis: OrthoViewBasis(.farSide)).count == 2)
        var noReference = withBoth
        noReference.reference = nil
        #expect(
            CompareElevationMarks.marks([noReference], basis: OrthoViewBasis(.front)).map(
                \.isPrediction) == [true])
    }
}

// MARK: - Length handle

@MainActor struct LengthHandleTests {
    private func geometry(
        anchor: PhysicalAnchorKind, resolved: Bool = true, length: Bool = true
    ) -> PhysicalGeometry {
        var body = PhysicalBody(bodyID: "b")
        if length {
            body.length = PhysicalDimension(
                status: .observed, span: .full, lowerM: 4, upperM: 5, valueM: 4.5,
                support: .init(frames: [0]))
        }
        body.width = PhysicalDimension(
            status: .observed, span: .full, lowerM: 1.8, upperM: 2, support: .init(frames: [0]))
        var k = PhysicalKeyframe(keyframeID: "k", sampleID: 0, timestampNs: 0)
        k.anchor = PhysicalAnchor(
            kind: anchor, offsetM: anchor.isFace ? 2.25 : nil,
            offsetBoundM: anchor.isFace ? 0.1 : nil)
        k.position = PhysicalPosition(
            status: .observed, xM: 10, yM: 0, boundM: 0.2, support: .init(frames: [0]))
        k.yaw = PhysicalYaw(
            status: .observed, axis: resolved ? .resolved : .frontRearAmbiguous, yawRad: 0,
            boundRad: 0.05, support: .init(frames: [0]))
        k.front = PhysicalEndpoint(
            status: resolved ? .observed : .unknown, support: resolved ? .init(frames: [0]) : .none)
        k.rear = k.front
        return PhysicalGeometry.derive(
            object: PhysicalObject(objectID: "o", body: body, keyframes: [k]), keyframe: k,
            gate: .preview)
    }

    @Test func theHandleSitsAtTheEndFarthestFromTheAnchor() {
        let centre = geometry(anchor: .bodyCentre)
        #expect(PhysicalHandles.lengthHandlePoint(centre) == centre.ends[0])
        #expect(centre.ends[0] == centre.front, "a derived end is where the evidenced bumper is")
        let rear = geometry(anchor: .rearFace)
        #expect(PhysicalHandles.lengthHandlePoint(rear) == rear.ends[0])
        let front = geometry(anchor: .frontFace)
        #expect(PhysicalHandles.lengthHandlePoint(front) == front.ends[1])
        var noBumpers = centre
        noBumpers.front = nil
        noBumpers.rear = nil
        #expect(
            PhysicalHandles.lengthHandlePoint(noBumpers) != nil,
            "an unevidenced end still has a handle")
        #expect(PhysicalHandles.lengthHandlePoint(geometry(anchor: .leftFace)) == nil)
        #expect(
            PhysicalHandles.lengthHandlePoint(geometry(anchor: .bodyCentre, resolved: false)) == nil
        )
        #expect(
            PhysicalHandles.lengthHandlePoint(geometry(anchor: .bodyCentre, length: false)) == nil)
        let vp = OrthoViewport(
            halfHeight: 10, size: CGSize(width: 400, height: 400), centre: simd_float2(10, 0))
        let at = PhysicalHandles.positions(centre, basis: OrthoViewBasis(.top), viewport: vp)
        #expect(at.length != nil && at.move != nil && at.turn != nil)
        let none = PhysicalHandles.positions(
            PhysicalGeometry(anchorKind: .bodyCentre), basis: OrthoViewBasis(.top), viewport: vp)
        #expect(none.move == nil && none.turn == nil && none.length == nil)
    }

    @Test func theDraggedLengthIsMeasuredFromTheHeldAnchorAlongTheAxis() {
        let anchor = PhysicalPlanar(x: 10, y: 0, bound: 0.2)
        // A centre anchor: the pointer reaches one end, so the length is
        // twice that; the across-axis part of the pointer is ignored.
        #expect(
            abs(
                PhysicalHandles.draggedLength(
                    anchor: anchor, anchorKind: .bodyCentre, yawRad: 0,
                    pointer: simd_float2(12.5, 3)) - 5) < 1e-9)
        #expect(
            abs(
                PhysicalHandles.draggedLength(
                    anchor: anchor, anchorKind: .rearFace, yawRad: 0, pointer: simd_float2(14.2, -1)
                ) - 4.2) < 1e-5)
        #expect(
            abs(
                PhysicalHandles.draggedLength(
                    anchor: anchor, anchorKind: .frontFace, yawRad: 0, pointer: simd_float2(5.5, 0))
                    - 4.5) < 1e-9)
        // Turned a quarter, the axis is +Y.
        #expect(
            abs(
                PhysicalHandles.draggedLength(
                    anchor: anchor, anchorKind: .rearFace, yawRad: .pi / 2,
                    pointer: simd_float2(10, 4)) - 4) < 1e-9)
        #expect(
            PhysicalHandles.draggedLength(
                anchor: anchor, anchorKind: .rearFace, yawRad: 0, pointer: simd_float2(10, 0))
                == 0.1, "a body cannot be dragged to nothing")
    }

    @Test func settingTheLengthCarriesTheIntervalWithIt() {
        let start = PhysicalDimension(
            status: .observed, span: .full, lowerM: 4.3, upperM: 4.7, valueM: 4.5)
        var d = start
        PhysicalDraft.setLength(5, of: &d, from: start)
        #expect(d.valueM == 5 && abs(d.lowerM! - 4.8) < 1e-9 && abs(d.upperM! - 5.2) < 1e-9)
        let midpoint = PhysicalDimension(status: .inferred, span: .full, lowerM: 4, upperM: 5)
        d = midpoint
        PhysicalDraft.setLength(0.2, of: &d, from: midpoint)
        #expect(
            d.lowerM == 0 && abs(d.upperM! - 0.7) < 1e-9 && d.valueM == 0.2,
            "the lower bound went negative")
        let partial = PhysicalDimension(status: .observed, span: .partial, lowerM: 3)
        d = partial
        PhysicalDraft.setLength(6, of: &d, from: partial)
        #expect(d == partial, "a partial span has no length to drag")
    }

    @Test func draggingTheLengthRevisesTheBodyAsOneUndoStep() async throws {
        let dir = try SyntheticPack.write([SyntheticPack.car(at: simd_float2(5, 0))])
        let (urlSession, baseURL, register) = AnnotationMockURLProtocol.makeSession()
        let fake = FakePhysicalService(packDir: dir.resolvingSymlinksInPath().path)
        register { try fake.handle($0) }
        let session = try AnnotationSession(
            pack: try AnnotationPack.open(directory: dir),
            physicalClient: PhysicalReferenceAPIClient(baseURL: baseURL, session: urlSession))
        session.operatorName = "op"
        session.workMode = .physical
        for _ in 0..<200 where session.physical.availability != .ready {
            try? await Task.sleep(for: .milliseconds(10))
        }
        let object = session.createObject(objectClass: "car")
        session.placePhysicalAnchor(in: .top, at: simd_float2(10, 0))
        let sampleID = try #require(session.currentSample?.sampleID)
        session.physical.edit { objects in
            PhysicalDraft.addBody(
                objectID: object.objectID, author: "op", session: "s", to: &objects)
            PhysicalDraft.updateBody(objectID: object.objectID, in: &objects) {
                $0.length = PhysicalDimension(
                    status: .observed, span: .full, lowerM: 4, upperM: 5, valueM: 4.5,
                    support: .init(frames: [0]))
            }
            PhysicalDraft.updateKeyframe(
                objectID: object.objectID, sampleID: sampleID, in: &objects
            ) {
                PhysicalDraft.setAxis(.resolved, of: &$0)
                $0.yaw.yawRad = 0
                $0.yaw.boundRad = 0.05
                $0.position.boundM = 0.2
            }
        }
        let vp = OrthoViewport(
            halfHeight: 10, size: CGSize(width: 400, height: 400), centre: simd_float2(10, 0))
        let preview = try #require(session.physicalPreview)
        let handle = try #require(
            PhysicalHandles.positions(preview, basis: session.basis(.top), viewport: vp).length)
        #expect(session.physicalHandle(at: handle, viewport: vp) == .length)

        session.beginPhysicalDrag()
        session.updatePhysicalDrag(.length, from: simd_float2(12.25, 0), to: simd_float2(13, 0))
        session.updatePhysicalDrag(.length, from: simd_float2(12.25, 0), to: simd_float2(13, 0.5))
        session.endPhysicalDrag()
        let length = try #require(session.physical.object(object.objectID)?.body?.length)
        #expect(abs((length.valueM ?? 0) - 6) < 1e-6)
        #expect(abs((length.lowerM ?? 0) - 5.5) < 1e-6 && abs((length.upperM ?? 0) - 6.5) < 1e-6)
        #expect(session.physicalKeyframe?.position.xM == 10, "a length drag moved the anchor")
        session.physical.undo()
        #expect(session.physical.object(object.objectID)?.body?.length.valueM == 4.5)
    }
}

// MARK: - The proposal population

struct ProposalPopulationTests {
    @Test func theHistogramOffersTheProposersExactMembers() throws {
        let section = try source("UI/IntensityInspectorSection.swift")
        #expect(section.contains("caseproposal"))
        #expect(section.contains("indices=session.proposalIndices"))
        #expect(section.contains("notareview"))
        #expect(
            IntensityInspectorSection.DistributionSource.allCases.map(\.label) == [
                "Saved mask", "Unsaved selection", "Proposal", "Facet subset",
            ])
    }
}

// MARK: - The rest of the branch

@MainActor struct PhysicalBranchAuditTests {
    @Test func overlayStyleTellsUnsavedFromProposedFromReviewed() {
        var saved = PhysicalKeyframe(keyframeID: "k", sampleID: 0, timestampNs: 0)
        let body = PhysicalBody(bodyID: "b")
        #expect(
            PhysicalReferenceOverlay.style(draft: saved, saved: nil, body: body, savedBody: body)
                == .unsaved)
        #expect(
            PhysicalReferenceOverlay.style(draft: saved, saved: saved, body: body, savedBody: body)
                == .proposed)
        saved.review.status = .reviewed
        #expect(
            PhysicalReferenceOverlay.style(draft: saved, saved: saved, body: body, savedBody: body)
                == .reviewed)
        var edited = saved
        edited.position.boundM = 1
        #expect(
            PhysicalReferenceOverlay.style(draft: edited, saved: saved, body: body, savedBody: body)
                == .unsaved)
        var otherBody = body
        otherBody.length.status = .inferred
        #expect(
            PhysicalReferenceOverlay.style(
                draft: saved, saved: saved, body: otherBody, savedBody: body) == .unsaved,
            "a changed body is unsaved for its keyframes")
        for style in [PhysicalReferenceOverlay.Style.unsaved, .proposed, .reviewed] {
            #expect(!style.label.isEmpty)
        }
        #expect(
            PhysicalReferenceOverlay.Style.reviewed.dash.isEmpty
                && !PhysicalReferenceOverlay.Style.proposed.dash.isEmpty)
    }

    @Test func progressLinesKeepTheThreeReviewsApart() async throws {
        let dir = try SyntheticPack.write([SyntheticPack.car(at: simd_float2(5, 0))])
        let (urlSession, baseURL, register) = AnnotationMockURLProtocol.makeSession()
        let fake = FakePhysicalService(packDir: dir.resolvingSymlinksInPath().path)
        var reviewedBody = PhysicalBody(bodyID: "b")
        reviewedBody.review.status = .reviewed
        var kf = PhysicalKeyframe(keyframeID: "k", sampleID: 0, timestampNs: 1_000_000_000)
        kf.review.status = .reviewed
        fake.objects = [
            PhysicalObject(
                objectID: "obj_a", body: reviewedBody,
                keyframes: [kf, PhysicalKeyframe(keyframeID: "k2", sampleID: 0, timestampNs: 0)])
        ]
        fake.digest = "sha256:x"
        register { try fake.handle($0) }
        let session = try AnnotationSession(
            pack: try AnnotationPack.open(directory: dir),
            physicalClient: PhysicalReferenceAPIClient(baseURL: baseURL, session: urlSession))
        #expect(
            session.physicalProgress(objectID: "obj_a") == nil,
            "asked the service before physical mode")
        session.workMode = .physical
        for _ in 0..<200 where session.physical.availability != .ready {
            try? await Task.sleep(for: .milliseconds(10))
        }
        #expect(
            session.physicalProgress(objectID: "obj_a") == "body reviewed · 2 keyframes, 1 reviewed"
        )
        #expect(session.physicalProgress(objectID: "obj_none") == "body none · no keyframes")
    }

    @Test func serviceRefusalsAreExplainedAndDriftIsFoundByRecordID() {
        #expect(
            PhysicalReferenceSession.unavailableReason(
                .refused(status: 404, code: "pack_not_found", message: "no pack")
            ).contains("--lidar-annotation-dir"))
        #expect(
            PhysicalReferenceSession.unavailableReason(
                .refused(status: 501, code: "not_configured", message: "x")
            ).contains("--lidar-annotation-dir"))
        #expect(PhysicalReferenceSession.unavailableReason(.transport("down")).contains("down"))
        let physical = PhysicalReferenceSession(
            packDirectory: URL(fileURLWithPath: "/tmp/p"), packDigest: "sha256:p", sessionID: "s",
            membershipDigest: { "" }, author: { "op" })
        #expect(!physical.hasDrift(record: "k1") && !physical.hasDrift(record: nil))
        #expect(
            PhysicalReferenceAPIError.invalid(
                PhysicalEditResult(
                    valid: false, invalid: nil, linkProblems: [.init(record: "r", problem: "p")],
                    resetReviews: [])
            ).errorDescription == "r: p")
        #expect(
            PhysicalReferenceAPIError.invalid(
                PhysicalEditResult(valid: false, invalid: nil, linkProblems: [], resetReviews: [])
            ).errorDescription == "The edit is invalid.")
        #expect(
            PhysicalReferenceAPIError.refused(status: 409, code: "conflict", message: "m").code
                == "conflict")
        #expect(PhysicalReferenceAPIError.decoding("bad").code == nil)
    }

    @Test func theClientSendsExactQueriesAndBodies() async throws {
        let (urlSession, baseURL, register) = AnnotationMockURLProtocol.makeSession()
        let fake = FakePhysicalService(packDir: "/x")
        register { try fake.handle($0) }
        let client = PhysicalReferenceAPIClient(baseURL: baseURL, session: urlSession)
        _ = try await client.load(handle: "run-a/pack", packDigest: "sha256:ab+cd", revision: 3)
        let url = try #require(fake.requests.last?.path)
        #expect(url == "/api/annotations/physical")
        _ = try await client.history(handle: "run-a/pack", packDigest: "sha256:p")
        #expect(fake.requests.last?.path == "/api/annotations/physical/history")
        _ = try await client.restore(
            .init(
                pack: "run-a/pack", packDigest: "sha256:p", baseRevision: 1, baseDigest: "",
                membershipDigest: "", revision: 1, author: "op", session: "s"))
        let sent = try #require(fake.last("/api/annotations/physical/restore"))
        #expect(
            sent["revision"] as? Int == 1 && sent["author"] as? String == "op"
                && sent["pack_digest"] as? String == "sha256:p")
        #expect(PhysicalReferenceAPIClient.defaultBaseURL.absoluteString.hasPrefix("http"))
    }

    @Test func statusSettersClearWhatUnknownCannotCarry() {
        var position = PhysicalPosition(
            status: .observed, xM: 1, yM: 2, zM: 3, boundM: 0.5, support: .init(frames: [1]))
        PhysicalDraft.setStatus(.inferred, of: &position)
        #expect(position.xM == 1 && position.status == .inferred)
        PhysicalDraft.setStatus(.unknown, of: &position)
        #expect(position == PhysicalPosition())
        var end = PhysicalEndpoint(status: .observed, support: .init(frames: [2]))
        PhysicalDraft.setStatus(.priorOnly, of: &end)
        #expect(end.support.frameList == [2])
        PhysicalDraft.setStatus(.unknown, of: &end)
        #expect(end == PhysicalEndpoint())
        var k = PhysicalKeyframe(keyframeID: "k", sampleID: 1, timestampNs: 1)
        k.anchor = PhysicalAnchor(kind: .rearFace, offsetM: 2, offsetBoundM: 0.1)
        PhysicalDraft.setAnchor(.leftFace, of: &k)
        #expect(k.anchor.offsetM == 2, "changing face kept the offset the operator typed")
        PhysicalDraft.setAnchor(.bodyCentre, of: &k)
        #expect(k.anchor.offsetM == nil && k.anchor.offsetBoundM == nil)
    }

    @Test func aDoubleClaimIsRefusedInWords() {
        let error = PointClaimError(conflicts: [
            PointClaimConflict(sampleID: 3, point: 41, first: "obj_a", second: "obj_b"),
            PointClaimConflict(sampleID: 3, point: 42, first: "obj_a", second: "obj_b"),
        ])
        #expect(
            error.description.contains("sample 3 point 41")
                && error.description.contains("and 1 more"))
        #expect(
            PointClaimError(conflicts: [error.conflicts[0]]).description.contains("obj_a and obj_b")
        )
    }

    @Test func renderersShareTheTableAndKeepTheBackdropFlat() throws {
        let composite = try source("Rendering/CompositePointCloudRenderer.swift")
        #expect(
            composite.contains(
                "ifbackgroundUniforms.padding.x==MetalRenderer.IntensityColouring.table.rawValue{backgroundUniforms.padding.x=MetalRenderer.IntensityColouring.flat.rawValue}"
            ))
        let viewports = try source("UI/AnnotationViewports.swift")
        #expect(viewports.contains("renderer.intensityColouring=display.enabled?.table:.flat"))
        let window = try source("UI/AnnotationWindow.swift")
        #expect(window.contains("colouring.spec.table(available:colouring.available)"))
        #expect(window.contains("WindowCloseGuard(blocked:session.navigationGuard()!=nil)"))
        let renderer = try source("Rendering/MetalRenderer.swift")
        #expect(
            renderer.contains("encoder.setFragmentBuffer(intensityTableBuffer,offset:0,index:0)"))
        #expect(renderer.contains("didSet{ifintensityTable!=oldValue{intensityTableBuffer=nil}}"))
    }

    @Test func describeSaveAndMembershipDidChangeAreQuietWithoutAState() async {
        let physical = PhysicalReferenceSession(
            packDirectory: URL(fileURLWithPath: "/tmp/p"), packDigest: "sha256:p", sessionID: "s",
            membershipDigest: { "" }, author: { "op" })
        await physical.membershipDidChange()
        #expect(physical.state == nil && physical.lastError == nil)
        #expect(
            PhysicalReferenceSession.describeSave(
                PhysicalEditResult(valid: true, linkProblems: [], resetReviews: []), revision: 2)
                == "Saved revision 2 as a proposal.")
        physical.discard()
        physical.undo()
        physical.redo()
        #expect(!physical.canUndo && !physical.canRedo && !physical.isDirty)
        #expect(!(await physical.save()))
        #expect(!(await physical.review(kind: .body, objectID: "o", recordID: "b")))
        #expect(!(await physical.restore(revision: 1)))
    }
}
