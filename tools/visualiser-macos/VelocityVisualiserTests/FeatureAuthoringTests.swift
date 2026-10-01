import Foundation
import SwiftProtobuf
import Testing
import simd

@testable import VelocityVisualiser

private func featurePoints(_ positions: [SIMD3<Float>]) -> PackPoints {
    PackPoints(x: positions.map(\.x), y: positions.map(\.y), z: positions.map(\.z))
}

struct FeatureSelectionTests {
    @Test func sphereSelectsOnlyCanonicalSavedObjectReturns() {
        let points = featurePoints([
            .zero, SIMD3(0.1, 0, 0), SIMD3(0.19, 0, 0), SIMD3(0.21, 0, 0), SIMD3(.nan, 0, 0),
        ])
        let sphere = FeatureSphere(centre: .zero, radius: 0.2)
        #expect(
            FeatureSelection.indices(
                points: points, domain: [4, 3, 1, 0, 0, -1, 99], sphere: sphere) == [0, 1])
        #expect(
            FeatureSelection.indices(
                points: points, domain: [0], sphere: FeatureSphere(centre: .zero, radius: .nan)
            ).isEmpty)
        #expect(
            FeatureSelection.indices(
                points: points, domain: [0],
                sphere: FeatureSphere(centre: SIMD3(.nan, 0, 0), radius: 0.2)
            ).isEmpty)
    }

    @Test func proposalUsesFreshTargetIndicesAndIgnoresAnotherObject() throws {
        let source = featurePoints([SIMD3(0.05, 0.05, 1), SIMD3(0.1, 0.1, 1.05)])
        // Return 0 belongs to a different object even though it is inside the sphere.
        let target = featurePoints([
            SIMD3(1.07, 0.07, 1), SIMD3(1.1, 0.1, 1.05), SIMD3(1.05, 0.05, 1),
        ])
        var observation = FeatureObservation()
        observation.pointIndices = [0, 1]
        observation.sphere = FeatureSphere(centre: SIMD3(0.075, 0.075, 1.025), radius: 0.2)
        let sphere = try FeatureSelection.propose(
            source: observation, sourcePoints: source, sourceDomain: [0, 1], targetPoints: target,
            targetDomain: [1, 2], seconds: 0.1)
        #expect(simd_distance(sphere.centre, observation.sphere.centre + SIMD3(1, 0, 0)) < 0.001)
        #expect(
            FeatureSelection.indices(points: target, domain: [1, 2], sphere: sphere) == [1, 2])
    }

    @Test func sparseMissingGapsAndImplausibleMovementStop() {
        let p = featurePoints([SIMD3(0.05, 0.05, 1), SIMD3(0.1, 0.1, 1.05)])
        var o = FeatureObservation()
        o.pointIndices = [0, 1]
        o.sphere = FeatureSphere(centre: SIMD3(0.075, 0.075, 1.025), radius: 0.2)
        for time in [0.0, -0.1, 0.6, Double.nan] {
            #expect(throws: (any Error).self) {
                try FeatureSelection.propose(
                    source: o, sourcePoints: p, sourceDomain: [0, 1], targetPoints: p,
                    targetDomain: [0, 1], seconds: time)
            }
        }
        #expect(throws: (any Error).self) {
            try FeatureSelection.propose(
                source: o, sourcePoints: p, sourceDomain: [0, 1], targetPoints: p, targetDomain: [],
                seconds: 0.1)
        }
        let far = featurePoints([SIMD3(50, 0, 1), SIMD3(50.1, 0, 1)])
        #expect(throws: (any Error).self) {
            try FeatureSelection.propose(
                source: o, sourcePoints: p, sourceDomain: [0, 1], targetPoints: far,
                targetDomain: [0, 1], seconds: 0.1)
        }
        o.pointIndices = [0]
        #expect(throws: (any Error).self) {
            try FeatureSelection.propose(
                source: o, sourcePoints: p, sourceDomain: [0, 1], targetPoints: p,
                targetDomain: [0, 1], seconds: 0.1)
        }
    }

    @Test func sparseOrCompetingLocalFitsDoNotBecomeAcceptedObservations() {
        let source = featurePoints([SIMD3(0.05, 0.05, 1), SIMD3(0.1, 0.1, 1.05)])
        var o = FeatureObservation()
        o.pointIndices = [0, 1]
        o.sphere = FeatureSphere(centre: SIMD3(0.075, 0.075, 1.025), radius: 0.2)
        let twins = featurePoints([
            SIMD3(-0.2, 0.05, 1), SIMD3(-0.15, 0.1, 1.05), SIMD3(0.3, 0.05, 1),
            SIMD3(0.35, 0.1, 1.05),
        ])
        #expect(throws: (any Error).self) {
            try FeatureSelection.propose(
                source: o, sourcePoints: source, sourceDomain: [0, 1], targetPoints: twins,
                targetDomain: [0, 1, 2, 3], seconds: 0.1)
        }
        let sparse = featurePoints([SIMD3(0.05, 0.05, 1)])
        #expect(throws: (any Error).self) {
            try FeatureSelection.propose(
                source: o, sourcePoints: source, sourceDomain: [0, 1], targetPoints: sparse,
                targetDomain: [0], seconds: 0.1)
        }
    }
}

/// The shared wire contract, with request counting and an uncertain commit.
/// Storage validation itself is covered by the Go tests against real packs.
private final class FakeFeatureService: @unchecked Sendable {
    private let lock = NSLock()
    var state: FeatureState
    var posts = 0
    var failAfterCommit = false
    var refuse = false
    var revisions: [UInt64: FeatureState] = [:]
    var ignoreRevision = false

    init(pack: AnnotationPack) {
        state = FeatureState()
        state.document.schema = "velocity.report/feature-proposals"
        state.document.schemaVersion = 1
        state.document.packDigest = pack.manifest.packDigest
        state.document.datasetID = pack.manifest.datasetID
        state.document.pointDomain = "legacy_pack"
        state.membershipDigest = "sha256:members"
        state.packDirectory = pack.directory.path
    }

    func handle(_ request: URLRequest) throws -> (HTTPURLResponse, Data) {
        lock.lock()
        defer { lock.unlock() }
        if refuse { throw URLError(.cannotConnectToHost) }
        if request.httpMethod != "POST", !ignoreRevision,
            let query = URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?
                .queryItems, let value = query.first(where: { $0.name == "revision" })?.value,
            let revision = UInt64(value), revision != state.document.revision
        {
            guard let retained = revisions[revision] else {
                return (
                    HTTPURLResponse(
                        url: request.url!, statusCode: 400, httpVersion: nil, headerFields: nil)!,
                    Data(#"{"error":"retained revision missing"}"#.utf8)
                )
            }
            return (
                HTTPURLResponse(
                    url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!,
                try retained.serializedData()
            )
        }
        if request.httpMethod == "POST" {
            posts += 1
            var bytes = request.httpBody ?? Data()
            if let stream = request.httpBodyStream {
                stream.open()
                defer { stream.close() }
                var buffer = [UInt8](repeating: 0, count: 4096)
                while stream.hasBytesAvailable {
                    let count = stream.read(&buffer, maxLength: buffer.count)
                    if count <= 0 { break }
                    bytes.append(buffer, count: count)
                }
            }
            let edit = try Velocity_Recording_V1_FeatureEdit(serializedBytes: bytes)
            guard edit.baseDigest == state.digest, edit.membershipDigest == state.membershipDigest
            else {
                return (
                    HTTPURLResponse(
                        url: request.url!, statusCode: 409, httpVersion: nil, headerFields: nil)!,
                    Data(#"{"error":"revision changed"}"#.utf8)
                )
            }
            revisions[state.document.revision] = state
            state.document = edit.document
            state.document.revision += 1
            state.digest = "sha256:revision-\(state.document.revision)"
            if failAfterCommit { throw URLError(.networkConnectionLost) }
        }
        return (
            HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!,
            try state.serializedData()
        )
    }
}

@MainActor private func featureSetup() throws -> (
    AnnotationPack, FeatureAPIClient, FakeFeatureService
) {
    let pack = try AnnotationPack.open(directory: PackFixture.write())
    let (session, baseURL, register) = AnnotationMockURLProtocol.makeSession()
    let service = FakeFeatureService(pack: pack)
    register { try service.handle($0) }
    return (pack, FeatureAPIClient(baseURL: baseURL, session: session), service)
}

private func featureSeed(_ pack: AnnotationPack) -> FeatureObservation {
    var observation = FeatureObservation()
    observation.sampleID = 0
    observation.timestampNs = pack.samples[0].timestampNs
    observation.pointIndices = [0, 1]
    observation.membershipRevision = 1
    observation.membershipDigest = "sha256:members"
    observation.method = "manual_sphere"
    observation.origin = "human_proposal"
    observation.sphere = FeatureSphere(centre: SIMD3(1.25, 1.125, 0.625), radius: 0.4)
    return observation
}

@MainActor struct FeatureAuthoringTests {
    @Test func retainedEvidenceIsReadOnlyAndReturningToHeadRestoresEditing() async throws {
        let (pack, client, service) = try featureSetup()
        defer { try? FileManager.default.removeItem(at: pack.directory) }
        let a = FeatureAuthoring(pack: pack, client: client)
        await a.load()
        a.name = "Mirror"
        a.geometry = .protrusion
        a.seed(featureSeed(pack), objectID: "car")
        #expect(
            await a.save(
                decision: .acceptedProposal, author: "op",
                membershipDigest: service.state.membershipDigest))
        a.seed(featureSeed(pack), objectID: "car")
        a.name = "Changed name"
        #expect(
            await a.save(
                decision: .acceptedProposal, author: "op",
                membershipDigest: service.state.membershipDigest))
        #expect(a.headRevision == 2)
        await a.load(revision: 1)
        #expect(a.viewingRevision == 1 && a.headRevision == 2 && !a.canEdit)
        #expect(a.name == "Mirror" && a.active?.observations[0].pointIndices == [0, 1])
        a.seed(featureSeed(pack), objectID: "car")
        #expect(!a.isDirty)
        #expect(
            !(await a.save(
                decision: .acceptedProposal, author: "op",
                membershipDigest: service.state.membershipDigest)))
        #expect(service.posts == 2)
        await a.load()
        #expect(a.viewingRevision == nil && a.canEdit && a.name == "Changed name")
        a.seed(featureSeed(pack), objectID: "car")
        await a.load(revision: 1)
        #expect(a.isDirty && a.viewingRevision == nil && a.state?.document.revision == 2)
    }

    @Test func missingOrWrongRevisionNeverMasqueradesAsTheRequestedEvidence() async throws {
        let (pack, client, service) = try featureSetup()
        defer { try? FileManager.default.removeItem(at: pack.directory) }
        service.state.document.revision = 2
        let a = FeatureAuthoring(pack: pack, client: client)
        await a.load()
        service.ignoreRevision = true
        await a.load(revision: 1)
        #expect(a.state == nil && a.readOnly && a.viewingRevision == 1)
        service.ignoreRevision = false
        await a.load(revision: 1)
        #expect(a.state == nil && !a.canEdit)
        #expect(throws: (any Error).self) { try FeatureDocument(serializedBytes: Data([0xff])) }
        // An offline archive may be inspected, but is never an editable head.
        let history = pack.directory.appendingPathComponent("feature-proposal-revisions")
        try FileManager.default.createDirectory(at: history, withIntermediateDirectories: true)
        var document = service.state.document
        document.revision = 1
        try document.serializedData().write(to: history.appendingPathComponent("0000000001.pb"))
        service.refuse = true
        await a.load(revision: 1)
        #expect(a.state?.document.revision == 1 && a.readOnly && !a.canEdit)
        document.revision = 3
        try document.serializedData().write(to: history.appendingPathComponent("0000000001.pb"))
        await a.load(revision: 1)
        #expect(a.state == nil && !a.canEdit)
        #expect(service.posts == 0)
    }

    @Test func revisionCannotBeCombinedWithEditsOrInvalidNumbers() async throws {
        let (pack, client, service) = try featureSetup()
        defer { try? FileManager.default.removeItem(at: pack.directory) }
        for revision in [UInt64(0), UInt64(Int32.max) + 1] {
            await #expect(throws: (any Error).self) {
                try await client.request(pack: pack, revision: revision)
            }
        }
        await #expect(throws: (any Error).self) {
            try await client.request(
                pack: pack, edit: Velocity_Recording_V1_FeatureEdit(), revision: 1)
        }
        #expect(service.posts == 0)
    }
    @Test func seedEditSaveReopenAndCancelNeverWriteMembership() async throws {
        let (pack, client, service) = try featureSetup()
        defer { try? FileManager.default.removeItem(at: pack.directory) }
        let authoring = FeatureAuthoring(pack: pack, client: client)
        await authoring.load()
        #expect(authoring.canEdit)
        authoring.name = "Mirror?"
        authoring.geometry = .protrusion
        authoring.semanticHint = "wing mirror"
        authoring.seed(featureSeed(pack), objectID: "car")
        #expect(service.posts == 0 && authoring.isDirty)
        authoring.choose("other")
        #expect(authoring.activeID.isEmpty)
        #expect(
            await authoring.save(
                decision: .acceptedProposal, author: "op",
                membershipDigest: service.state.membershipDigest))
        let id = authoring.activeID
        #expect(!id.isEmpty && !authoring.isDirty && service.posts == 1)
        let reopened = FeatureAuthoring(pack: pack, client: client)
        await reopened.load()
        reopened.choose(id)
        #expect(reopened.active?.name == "Mirror?")
        #expect(reopened.active?.observations.first?.pointIndices == [0, 1])
        #expect(reopened.active?.observations.first?.decision == .acceptedProposal)
        #expect(reopened.active?.hasAnchor == false)
        #expect(reopened.active?.partRelation == "unknown")
        reopened.seed(featureSeed(pack), objectID: "car")
        reopened.name = "Unsaved change"
        reopened.geometry = .edge
        reopened.cancel()
        #expect(reopened.name == "Mirror?" && reopened.geometry == .protrusion)
        #expect(!reopened.isDirty && service.posts == 1)
        #expect(
            !FileManager.default.fileExists(
                atPath: pack.directory.appendingPathComponent("annotations.json").path))
    }

    @Test func rejectedMissingAndOccludedSaveNoMeasuredSupport() async throws {
        let (pack, client, service) = try featureSetup()
        defer { try? FileManager.default.removeItem(at: pack.directory) }
        let a = FeatureAuthoring(pack: pack, client: client)
        await a.load()
        for decision in [FeatureDecision.rejected, .missing, .occluded] {
            a.seed(featureSeed(pack), objectID: "car")
            #expect(
                await a.save(
                    decision: decision, author: "op",
                    membershipDigest: service.state.membershipDigest))
            #expect(a.active?.observations[0].pointIndices.isEmpty == true)
            #expect(a.active?.observations[0].decision == decision)
        }
    }

    @Test func uncertainSaveRetainsDraftAndRequiresReloadWithoutRetry() async throws {
        let (pack, client, service) = try featureSetup()
        defer { try? FileManager.default.removeItem(at: pack.directory) }
        let a = FeatureAuthoring(pack: pack, client: client)
        await a.load()
        a.seed(featureSeed(pack), objectID: "car")
        #expect(
            !(await a.save(
                decision: .acceptedProposal, author: " ",
                membershipDigest: service.state.membershipDigest)))
        #expect(
            !(await a.save(decision: .acceptedProposal, author: "op", membershipDigest: "changed")))
        #expect(service.posts == 0)
        service.failAfterCommit = true
        #expect(
            !(await a.save(
                decision: .acceptedProposal, author: "op",
                membershipDigest: service.state.membershipDigest)))
        #expect(a.isDirty && a.readOnly && service.posts == 1)
        #expect(
            !(await a.save(
                decision: .acceptedProposal, author: "op",
                membershipDigest: service.state.membershipDigest)))
        #expect(service.posts == 1)
        a.cancel()
        service.failAfterCommit = false
        await a.load()
        #expect(a.canEdit && a.state?.document.revision == 1)
    }

    @Test func differentFolderAndFutureSchemaAreNotWritable() async throws {
        let (pack, client, service) = try featureSetup()
        defer { try? FileManager.default.removeItem(at: pack.directory) }
        service.state.packDirectory += "-copy"
        let a = FeatureAuthoring(pack: pack, client: client)
        await a.load()
        #expect(a.readOnly && !a.canEdit)
        service.state.packDirectory = pack.directory.path
        service.state.document.schemaVersion = 2
        await a.load()
        #expect(a.readOnly && !a.canEdit)
    }

    @Test func localDocumentReopensReadOnlyWithoutService() async throws {
        let (pack, client, service) = try featureSetup()
        defer { try? FileManager.default.removeItem(at: pack.directory) }
        try service.state.document.serializedData().write(
            to: pack.directory.appendingPathComponent("feature-proposals.pb"))
        service.refuse = true
        let a = FeatureAuthoring(pack: pack, client: client)
        await a.load()
        #expect(a.state != nil && a.readOnly && !a.canEdit)
    }

    @Test func featureModeGuardsNavigationAndCannotSaveObjectMasks() async throws {
        let (pack, client, service) = try featureSetup()
        defer { try? FileManager.default.removeItem(at: pack.directory) }
        let session = try AnnotationSession(pack: pack, featureClient: client)
        session.workMode = .features
        await session.features.load()
        session.features.seed(featureSeed(pack), objectID: "car")
        #expect(session.navigationGuard() == .unsavedFeature)
        #expect(session.stepForward() == .unsavedFeature)
        #expect(!(await session.saveCurrent(advance: false)))
        #expect(session.sidecar.masks.isEmpty && service.posts == 0)
        session.features.cancel()
        #expect(session.navigationGuard() == nil)
        #expect(AnnotationWorkMode.points.label == "Object Points")
        #expect(AnnotationWorkMode.features.label == "Facets")
    }
}

struct FeatureSharedWireTests {
    @Test func goAndSwiftShareExactPresenceIdentityAndTimestampBytes() throws {
        let repo = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let bytes = try Data(
            contentsOf: repo.appendingPathComponent(
                "proto/velocity_recording/v1/testdata/feature-proposals.pb"))
        let doc = try FeatureDocument(serializedBytes: bytes)
        #expect(doc.schema == "velocity.report/feature-proposals" && doc.schemaVersion == 1)
        #expect(doc.pointDomain == "legacy_pack" && doc.revision == 2)
        let feature = try #require(doc.features.first)
        #expect(feature.featureID == "persistent-corner" && feature.geometry == .corner)
        #expect(feature.semanticHint == "headlight" && !feature.hasAnchor)
        let o = try #require(feature.observations.first)
        #expect(o.timestampNs == 9_007_199_254_740_993)
        #expect(o.hasProposedFromSample && o.proposedFromSample == 0)
        #expect(o.pointIndices == [0, 17] && o.uncertainIndices == [18])
        #expect(o.sphere.radiusM == 0.2 && o.sphere.yM == -2.5)
        #expect(o.origin == "assisted_proposal")
        #expect(!doc.features[1].observations[0].hasProposedFromSample)
        #expect(doc.features[1].observations[0].pointIndices == [17])
        #expect(try doc.serializedData() == bytes)
        var future = bytes
        future.append(contentsOf: [0xa0, 0x06, 0x01])
        #expect(try FeatureDocument(serializedBytes: future).serializedData() == future)
    }
}

@MainActor struct FeatureSessionWorkflowTests {
    @Test func subsetPreviewCarriesShapeRatherThanItsEnvelope() async throws {
        let source: [SyntheticPack.Point] = [
            (0.05, 0.05, 1, 1), (1.05, 0.05, 1, 1), (0.55, 0.55, 1, 1), (3, 0, 1, 1),
        ]
        // Reorder canonical returns: IDs are specific to the target scan.
        let target: [SyntheticPack.Point] = [
            (1.55, 0.55, 1, 1), (4, 0, 1, 1), (2.05, 0.05, 1, 1), (1.05, 0.05, 1, 1),
        ]
        let dir = try SyntheticPack.write([source, target], sourceStride: 1)
        defer { try? FileManager.default.removeItem(at: dir) }
        let pack = try AnnotationPack.open(directory: dir)
        let (urlSession, baseURL, register) = AnnotationMockURLProtocol.makeSession()
        let service = FakeFeatureService(pack: pack)
        register { try service.handle($0) }
        let s = try AnnotationSession(
            pack: pack, featureClient: FeatureAPIClient(baseURL: baseURL, session: urlSession))
        s.operatorName = "operator"
        let object = s.createObject(objectClass: "car")
        let all = SelectionPolygon(rectFrom: SIMD2(-1, -1), to: SIMD2(5, 2))
        #expect(s.select(polygon: all, mode: .replace) && s.save())
        #expect(s.stepForward() == nil)
        #expect(s.select(polygon: all, mode: .replace) && s.save())
        #expect(s.stepBackward() == nil)
        service.state.membershipDigest = s.membershipDigest
        let maskBytes = try Data(contentsOf: dir.appendingPathComponent("annotations.json"))
        s.workMode = .features
        await s.features.load()
        var observation = FeatureObservation()
        observation.sampleID = 0
        observation.timestampNs = s.currentSample!.timestampNs
        observation.membershipDigest = s.membershipDigest
        observation.membershipRevision = UInt64(s.sidecar.revision)
        observation.sphere = FeatureSphere(centre: SIMD3(0.55, 0.05, 1), radius: 0.6)
        observation.pointIndices = [0, 1]
        observation.method = "manual_lasso"
        observation.origin = "human_proposal"
        s.features.seed(observation, objectID: object.objectID)
        #expect(
            await s.features.save(
                decision: .acceptedProposal, author: s.operatorName,
                membershipDigest: s.membershipDigest))
        s.proposeNextFeature()
        #expect(s.currentSample?.sampleID == 1)
        #expect(s.features.draft?.pointIndices == [2, 3])
        #expect(s.features.draft?.usesSubsetShape == true)
        #expect(s.features.draft?.origin == "assisted_proposal")
        s.resizeFeature(1)
        #expect(s.features.draft?.pointIndices == [2, 3], "Radius cannot expand a subset preview")
        #expect(
            await s.features.save(
                decision: .acceptedProposal, author: s.operatorName,
                membershipDigest: s.membershipDigest))
        s.editCurrentFeature()
        #expect(s.features.draft?.pointIndices == [2, 3])
        #expect(s.features.draft?.hasProposedFromSample == true)
        #expect(try Data(contentsOf: dir.appendingPathComponent("annotations.json")) == maskBytes)
    }

    @Test func sphereThenOneFramePreviewEditRejectAndOcclusionKeepMaskBytes() async throws {
        let source: [SyntheticPack.Point] = [(0.05, 0.05, 1, 1), (0.1, 0.1, 1.05, 1), (3, 0, 1, 1)]
        let target = source.map { ($0.x + 1, $0.y, $0.z, $0.classification) }
        let dir = try SyntheticPack.write([source, target, []], sourceStride: 1)
        defer { try? FileManager.default.removeItem(at: dir) }
        let pack = try AnnotationPack.open(directory: dir)
        let (urlSession, baseURL, register) = AnnotationMockURLProtocol.makeSession()
        let service = FakeFeatureService(pack: pack)
        register { try service.handle($0) }
        let client = FeatureAPIClient(baseURL: baseURL, session: urlSession)
        let s = try AnnotationSession(pack: pack, featureClient: client)
        s.operatorName = "operator"
        let object = s.createObject(objectClass: "car")
        let all = SelectionPolygon(rectFrom: SIMD2(-1, -1), to: SIMD2(5, 2))
        #expect(s.select(polygon: all, mode: .replace))
        #expect(s.save())
        #expect(s.stepForward() == nil)
        #expect(s.select(polygon: all, mode: .replace))
        #expect(s.save())
        #expect(s.stepBackward() == nil)
        service.state.membershipDigest = s.membershipDigest
        let before = try Data(contentsOf: dir.appendingPathComponent("annotations.json"))
        s.workMode = .features
        await s.features.load()
        let viewport = s.viewport(for: .top, size: CGSize(width: 600, height: 400))
        let pixel = viewport.screenPoint(from: s.basis(.top).project(SIMD3(0.05, 0.05, 1)))
        s.seedFeature(in: .top, viewport: viewport, at: pixel)
        #expect(s.features.draft?.pointIndices == [0, 1])
        #expect(s.features.radius == 0.2 && service.posts == 0)
        #expect(
            await s.features.save(
                decision: .acceptedProposal, author: s.operatorName,
                membershipDigest: s.membershipDigest))
        let id = s.features.activeID
        s.proposeNextFeature()
        #expect(s.currentSample?.sampleID == 1 && s.features.isDirty)
        #expect(s.features.draft?.pointIndices == [0, 1])
        #expect(s.features.draft?.hasProposedFromSample == true)
        #expect(s.features.draft?.proposedFromSample == 0)
        #expect(s.features.draft?.origin == "assisted_proposal")
        #expect(service.posts == 1, "preview must not write")
        #expect(s.stepForward() == .unsavedFeature)
        s.resizeFeature(0.15)
        #expect(s.features.draft?.method == "object_translation_edited")
        #expect(
            await s.features.save(
                decision: .acceptedProposal, author: s.operatorName,
                membershipDigest: s.membershipDigest))
        #expect(s.features.activeID == id && s.features.active?.observations.count == 2)
        #expect(s.adjacentFacetObservation(forward: false)?.sampleID == 0)
        #expect(s.goToFacetObservation(sampleID: 0))
        #expect(s.features.activeID == id && s.featureOverlay?.pointIndices == [0, 1])
        #expect(s.adjacentFacetObservation(forward: true)?.sampleID == 1)
        #expect(!s.goToFacetObservation(sampleID: 99))
        #expect(s.currentSample?.sampleID == 0)
        #expect(s.goToFacetObservation(sampleID: 1))
        s.editCurrentFeature()
        #expect(s.features.isDirty && s.features.draft?.origin == "assisted_proposal")
        #expect(!s.goToFacetObservation(sampleID: 0))
        #expect(s.currentSample?.sampleID == 1)
        #expect(
            await s.features.save(
                decision: .rejected, author: s.operatorName, membershipDigest: s.membershipDigest))
        #expect(s.features.active?.observations[1].pointIndices.isEmpty == true)
        #expect(s.stepForward() == nil && s.currentPoints.count == 0)
        s.stageFeatureAbsence()
        #expect(s.features.draft?.hasSphere == false)
        #expect(
            await s.features.save(
                decision: .occluded, author: s.operatorName, membershipDigest: s.membershipDigest))
        #expect(s.features.active?.observations.last?.decision == .occluded)
        #expect(s.features.active?.objectID == object.objectID)
        #expect(s.adjacentFacetObservation(forward: false)?.decision == .rejected)
        #expect(s.goToFacetObservation(sampleID: 0))
        #expect(s.goToFacetObservation(sampleID: 2))
        #expect(s.featureOverlay?.decision == .occluded && s.featureOverlay?.hasSphere == false)
        #expect(try Data(contentsOf: dir.appendingPathComponent("annotations.json")) == before)
        let reopened = FeatureAuthoring(pack: pack, client: client)
        await reopened.load()
        reopened.choose(id)
        #expect(
            reopened.active?.observations.map(\.decision) == [
                .acceptedProposal, .rejected, .occluded,
            ])
    }

    @Test func skippedSourceFrameCannotBePropagatedEvenIfSampleIDsAreAdjacent() async throws {
        let points: [SyntheticPack.Point] = [(0.05, 0.05, 1, 1), (0.1, 0.1, 1.05, 1)]
        let dir = try SyntheticPack.write([points, points])  // source ordinals 1 and 3
        defer { try? FileManager.default.removeItem(at: dir) }
        let pack = try AnnotationPack.open(directory: dir)
        let (urlSession, baseURL, register) = AnnotationMockURLProtocol.makeSession()
        let service = FakeFeatureService(pack: pack)
        register { try service.handle($0) }
        let s = try AnnotationSession(
            pack: pack, featureClient: FeatureAPIClient(baseURL: baseURL, session: urlSession))
        s.operatorName = "operator"
        let object = s.createObject(objectClass: "car")
        #expect(
            s.select(
                polygon: SelectionPolygon(rectFrom: SIMD2(-1, -1), to: SIMD2(1, 1)), mode: .replace)
        )
        #expect(s.save())
        service.state.membershipDigest = s.membershipDigest
        s.workMode = .features
        await s.features.load()
        var observation = s.featureObservation(sample: try #require(s.currentSample))
        observation.sphere = FeatureSphere(centre: SIMD3(0.05, 0.05, 1), radius: 0.2)
        observation.pointIndices = [0, 1]
        observation.method = "manual_sphere"
        s.features.seed(observation, objectID: object.objectID)
        #expect(
            await s.features.save(
                decision: .acceptedProposal, author: s.operatorName,
                membershipDigest: s.membershipDigest))
        s.proposeNextFeature()
        #expect(s.sampleIndex == 0 && !s.features.isDirty && service.posts == 1)
        #expect(s.features.message?.contains("skipped") == true)
    }
}

@MainActor struct FacetActiveLimitTests {
    @Test func theFifthFacetWaitsForRetirementAndReactivationKeepsTheCap() async throws {
        let (pack, client, service) = try featureSetup()
        let a = FeatureAuthoring(pack: pack, client: client)
        await a.load()
        for i in 0..<4 {
            a.newFeature()
            a.name = "Facet \(i)"
            a.seed(featureSeed(pack), objectID: "car")
            #expect(
                await a.save(
                    decision: .acceptedProposal, author: "op",
                    membershipDigest: service.state.membershipDigest))
        }
        #expect(a.activeCount(objectID: "car") == 4)
        let first = try #require(a.state?.document.features.first?.featureID)
        a.newFeature()
        a.seed(featureSeed(pack), objectID: "car")
        #expect(
            !(await a.save(
                decision: .acceptedProposal, author: "op",
                membershipDigest: service.state.membershipDigest)))
        #expect(a.isDirty && service.posts == 4)
        a.cancel()
        a.choose(first)
        await a.setInactive(true, author: "op")
        #expect(a.activeCount(objectID: "car") == 3 && a.active?.observations.count == 1)
        a.newFeature()
        a.seed(featureSeed(pack), objectID: "car")
        #expect(
            await a.save(
                decision: .acceptedProposal, author: "op",
                membershipDigest: service.state.membershipDigest))
        a.choose(first)
        await a.setInactive(false, author: "op")
        #expect(a.active?.inactive == true && a.activeCount(objectID: "car") == 4)
        #expect(a.state?.document.features.count == 5)
    }
}

@MainActor struct FacetSubsetSelectionTests {
    @Test func lassoAndDepthSlabSaveExactSupportWithoutChangingTheObjectMask() async throws {
        let points: [SyntheticPack.Point] = [
            (0, 0, 1, 1), (0.1, 0.1, 1, 1), (0.05, 0.05, 3, 1), (1, 1, 1, 1),
        ]
        let dir = try SyntheticPack.write([points])
        let pack = try AnnotationPack.open(directory: dir)
        let (transport, url, register) = AnnotationMockURLProtocol.makeSession()
        let service = FakeFeatureService(pack: pack)
        register { try service.handle($0) }
        let s = try AnnotationSession(
            pack: pack, featureClient: FeatureAPIClient(baseURL: url, session: transport))
        s.operatorName = "op"
        _ = s.createObject(objectClass: "car")
        #expect(
            s.select(
                polygon: SelectionPolygon(rectFrom: SIMD2(-1, -1), to: SIMD2(2, 2)), mode: .replace)
        )
        #expect(s.save())
        service.state.membershipDigest = s.membershipDigest
        let mask = try Data(contentsOf: dir.appendingPathComponent("annotations.json"))
        s.workMode = .features
        await s.features.load()
        s.features.selectionTool = .lasso
        s.slab = DepthSlab(minDepth: -1.5, maxDepth: -0.5)
        s.selectFacet(polygon: SelectionPolygon(rectFrom: SIMD2(-0.2, -0.2), to: SIMD2(0.2, 0.2)))
        #expect(s.features.draft?.pointIndices == [0, 1])
        #expect(s.features.draft?.method == "manual_lasso")
        #expect(s.features.message?.contains("1 outside depth slab") == true)
        s.resizeFeature(2)
        #expect(
            s.features.draft?.pointIndices == [0, 1],
            "resizing a sphere must not broaden a lasso subset")
        s.features.geometry = .edge
        #expect(
            await s.features.save(
                decision: .acceptedProposal, author: "op", membershipDigest: s.membershipDigest))
        s.inspection.setHovered([
            IntensityReadout(
                sampleID: 0, sourceOrdinal: 1, pointIndex: 0, raw: nil, position: SIMD3(0, 0, 1),
                displayClass: 1, depth: -1)
        ])
        s.inspection.pin()
        s.usePinnedFacetReturn()
        #expect(s.features.registrationPoint == 0)
        s.inspection.setHovered([
            IntensityReadout(
                sampleID: 0, sourceOrdinal: 1, pointIndex: 3, raw: nil, position: SIMD3(1, 1, 1),
                displayClass: 1, depth: -1)
        ])
        s.inspection.pin()
        s.usePinnedFacetReturn()
        #expect(s.features.registrationPoint == 0, "an outside return became the facet anchor")
        s.editCurrentFeature()
        #expect(
            s.features.draft?.pointIndices == [0, 1] && s.features.draft?.method == "manual_lasso")
        s.selectFacet(
            polygon: SelectionPolygon(rectFrom: SIMD2(-0.2, -0.2), to: SIMD2(0.2, 0.2)),
            mode: .subtract)
        #expect(!s.features.isDirty)
        #expect(try Data(contentsOf: dir.appendingPathComponent("annotations.json")) == mask)
        #expect(s.features.active?.observations.first?.pointIndices == [0, 1])
    }
}

@MainActor struct FacetFutureContractTests {
    @Test func unknownFieldsRemainInspectableButCannotBeOverwritten() async throws {
        let (pack, client, service) = try featureSetup()
        var bytes = try service.state.document.serializedData()
        bytes.append(contentsOf: [0xa0, 0x06, 0x01])
        service.state.document = try FeatureDocument(serializedBytes: bytes)
        let a = FeatureAuthoring(pack: pack, client: client)
        await a.load()
        #expect(a.state != nil && a.readOnly && !a.canEdit)
        #expect(try a.state?.document.serializedData() == bytes)
        a.seed(featureSeed(pack), objectID: "car")
        #expect(!a.isDirty && service.posts == 0)
    }
}

@MainActor struct FacetRegistrationTransportTests {
    @Test func registerDetachAndUnconfirmedCommitPreserveTheFacetAndBlockRetry() async throws {
        let (pack, client, service) = try featureSetup()
        let a = FeatureAuthoring(pack: pack, client: client)
        await a.load()
        a.seed(featureSeed(pack), objectID: "car")
        #expect(
            await a.save(
                decision: .acceptedProposal, author: "op",
                membershipDigest: service.state.membershipDigest))
        let id = a.activeID
        var anchor = FacetBodyAnchor()
        anchor.coordinateDomain = "body_xy"
        anchor.method = "manual_named_return_v1"
        anchor.origin = "reference_seeded_proposal"
        anchor.partFrameRevision = 1
        anchor.xM = 1.2
        await a.registerAnchor(anchor, author: "op")
        #expect(a.active?.anchor.xM == 1.2 && a.active?.partRelation == "rigid_proposal")
        await a.registerAnchor(nil, author: "op")
        #expect(a.active?.hasAnchor == false && a.active?.partRelation == "unknown")
        #expect(a.activeID == id && a.active?.observations.count == 1)
        service.failAfterCommit = true
        await a.registerAnchor(anchor, author: "op")
        let posts = service.posts
        #expect(a.readOnly && a.active?.hasAnchor == false)
        #expect(service.state.document.features[0].hasAnchor)
        await a.registerAnchor(anchor, author: "op")
        #expect(service.posts == posts)
        service.failAfterCommit = false
        await a.load()
        #expect(a.canEdit && a.active?.anchor.xM == 1.2)
    }
}

@MainActor struct FacetProjectionExposureTests {
    @Test func hidingCancellingAndReopeningDoNotLaunderPreviewAssistance() async throws {
        let (pack, client, service) = try featureSetup()
        defer { try? FileManager.default.removeItem(at: pack.directory) }
        let suite = "facet-preview-test-" + UUID().uuidString
        let defaults = try #require(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let a = FeatureAuthoring(pack: pack, client: client, exposureDefaults: defaults)
        await a.load()
        a.geometry = .protrusion
        a.seed(featureSeed(pack), objectID: "car")
        #expect(
            await a.save(
                decision: .acceptedProposal, author: "op",
                membershipDigest: service.state.membershipDigest))
        let id = a.activeID
        let original = a.active?.observations[0]
        let projection = FacetBodyProjection(
            origin: SIMD2(1, 2), normal: nil, boundM: 0.2, normalBoundRad: nil, assisted: false,
            physicalRevision: 4, physicalDigest: "sha256:physical")
        a.seed(featureSeed(pack), objectID: "car")
        a.showBodyPreview(
            FacetBodyPreview(
                featureID: id, sampleID: 0, membershipDigest: service.state.membershipDigest,
                projection: projection), objectID: "car")
        #expect(a.draft?.origin == "assisted_proposal")
        #expect(a.active?.observations[0] == original, "viewing rewrote saved history")
        a.bodyPreview = nil
        a.cancel()
        a.seed(featureSeed(pack), objectID: "car")
        #expect(a.draft?.origin == "assisted_proposal")
        #expect(a.draft?.note.contains("physical revision 4") == true)
        a.cancel()
        let reopened = FeatureAuthoring(pack: pack, client: client, exposureDefaults: defaults)
        await reopened.load()
        reopened.choose(id)
        reopened.seed(featureSeed(pack), objectID: "car")
        #expect(reopened.draft?.origin == "assisted_proposal")
        #expect(
            await reopened.save(
                decision: .acceptedProposal, author: "op",
                membershipDigest: service.state.membershipDigest))
        #expect(reopened.active?.observations[0].origin == "assisted_proposal")
        reopened.cancel()
        var next = featureSeed(pack)
        next.sampleID = 1
        reopened.seed(next, objectID: "car")
        #expect(
            reopened.draft?.origin == "human_proposal",
            "an unseen frame acquired preview provenance")
    }
}
