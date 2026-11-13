// RoadSurfaceTests.swift
// The road surface preview (layer 17): the model decoded from Gerardus's
// committed fixture, the store's dedupe, and the loader's handling of an
// absent layer, a truncated stream, a cancelled load and a late response.

import Foundation
import GRPCCore
import SwiftProtobuf
import Testing
import simd

@testable import VelocityVisualiser

/// The fixture Gerardus committed (tests/fixtures/road-surface).
enum RoadFixture {
    static let directory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
        .appendingPathComponent("Resources/road-surface")

    /// The fixture's two sites, from its manifest.
    static let siteA = RoadSiteSource(
        l16Token: "80858771f", originLatitudeDeg: 37.76671733393986,
        originLongitudeDeg: -122.48139688202696)
    static let siteB = RoadSiteSource(
        l16Token: "808587721", originLatitudeDeg: 37.76790811787357,
        originLongitudeDeg: -122.48139688202696)

    static func messages(site: String) throws -> [Gerardus_Align_V1alpha1_StreamReferenceResponse] {
        try decode(Data(contentsOf: directory.appendingPathComponent("stream-\(site).binpb")))
    }

    /// Length-delimited messages to the end of `data`. Bytes that end inside a
    /// message throw, as a stream cut short does.
    static func decode(_ data: Data) throws -> [Gerardus_Align_V1alpha1_StreamReferenceResponse] {
        let stream = InputStream(data: data)
        stream.open()
        defer { stream.close() }
        var out: [Gerardus_Align_V1alpha1_StreamReferenceResponse] = []
        while stream.hasBytesAvailable {
            out.append(
                try BinaryDelimited.parse(
                    messageType: Gerardus_Align_V1alpha1_StreamReferenceResponse.self, from: stream)
            )
        }
        return out
    }

    static func chunks(site: String) throws -> [(Gerardus_Align_V1alpha1_RoadSurfaceChunk, Bool)] {
        try messages(site: site).map { ($0.roadSurface, $0.layerComplete) }
    }

    static func store(site: String) throws -> RoadSurfaceStore {
        var store = RoadSurfaceStore()
        for (chunk, _) in try chunks(site: site) { store.append(chunk, frameToken: site) }
        return store
    }

    static func manifest() throws -> [String: Any] {
        let data = try Data(contentsOf: directory.appendingPathComponent("manifest.json"))
        let object = try JSONSerialization.jsonObject(with: data)
        return try #require(object as? [String: Any])
    }
}

struct RoadSurfaceModelTests {
    @Test func decodesBothStreamsWithTheManifestCounts() throws {
        let manifest = try RoadFixture.manifest()
        let expected = try #require(manifest["patches"] as? [[String: Any]])
        for site in ["80858771f", "808587721"] {
            let messages = try RoadFixture.messages(site: site)
            #expect(messages.count == 2)
            #expect(messages.allSatisfy { $0.layer == .roadSurface })
            #expect(messages.map(\.layerComplete) == [false, true])
            let store = try RoadFixture.store(site: site)
            #expect(store.patches.count == 5)
            #expect(store.rejected.isEmpty)
            #expect(store.corridors.map(\.corridorID) == ["fixture"])
            for want in expected {
                let id = try #require(want["patch_id"] as? String)
                let patch = try #require(store.patch(id: id))
                #expect(patch.vertices.count == want["vertices"] as? Int)
                #expect(patch.triangleCount == want["triangles"] as? Int)
                #expect(patch.geometryHash == want["geometry_hash"] as? String)
                #expect(patch.supportHash == want["support_hash"] as? String)
                let support = try #require(want["support"] as? [String: Int])
                #expect(patch.count(of: .observed) == support["observed"])
                #expect(patch.count(of: .interpolated) == support["interpolated"])
                #expect(patch.count(of: .missing) == support["missing"])
            }
        }
    }

    @Test func aTruncatedStreamThrowsRatherThanDecodingPartly() throws {
        let full = try Data(
            contentsOf: RoadFixture.directory.appendingPathComponent("stream-80858771f.binpb"))
        #expect(try RoadFixture.decode(full).count == 2)
        #expect(throws: (any Error).self) { try RoadFixture.decode(full.prefix(full.count - 40)) }
        // Cut cleanly after the first message: one message, no error, and the
        // layer never completed.
        let first = try RoadFixture.messages(site: "80858771f")[0]
        let size = try first.serializedData().count
        let prefix = size < 128 ? 1 : size < 16384 ? 2 : size < 2_097_152 ? 3 : 4
        let cut = try RoadFixture.decode(full.prefix(size + prefix))
        #expect(cut.count == 1)
        #expect(cut.allSatisfy { !$0.layerComplete })
    }

    @Test func neighbouringSitesAgreeOnGeometryHashes() throws {
        let a = try RoadFixture.store(site: "80858771f")
        let b = try RoadFixture.store(site: "808587721")
        #expect(a.patches.map(\.patchID) == b.patches.map(\.patchID))
        #expect(a.patches.map(\.geometryHash) == b.patches.map(\.geometryHash))
        #expect(a.patches.map(\.supportHash) == b.patches.map(\.supportHash))
    }

    @Test func missingVerticesAreNaNAndNoTriangleUsesThem() throws {
        let store = try RoadFixture.store(site: "80858771f")
        let patch = try #require(
            store.patch(id: "gms-preview:road/fixture/segment/unsupported/0"))
        let missing = patch.support.enumerated().filter { $0.element == .missing }.map(\.offset)
        #expect(!missing.isEmpty)
        for v in missing {
            #expect(patch.vertices[v].z.isNaN)
            #expect(patch.normals[v].z.isNaN || patch.normals[v].x.isNaN)
        }
        let used = Set(patch.indices.map(Int.init))
        #expect(used.isDisjoint(with: Set(missing)))
        #expect(patch.defect() == nil)
        // The profile stops: stations past the supported ground carry no height.
        #expect(patch.profile.contains { $0.isMissing })
        #expect(patch.profile.contains { !$0.isMissing })
        // A point over the missing ground has no height; none is borrowed.
        let gone = patch.vertices[missing[0]]
        #expect(patch.height(at: SIMD2(gone.x, gone.y)) == nil)
    }

    @Test func keepsSupportBoundariesAssessmentsStatsAndProfile() throws {
        let store = try RoadFixture.store(site: "80858771f")
        let bend = try #require(store.patch(id: "gms-preview:road/fixture/segment/bend/0"))
        #expect(bend.boundarySource == .inferredWidth)
        #expect(bend.boundarySource.isInferred)
        #expect(bend.count(of: .interpolated) > 0)
        #expect(bend.rings.count == 1 && !bend.rings[0].hole)
        #expect(bend.rings[0].edgeSource.count == bend.rings[0].points.count)
        #expect(bend.rings[0].edgeSource.contains(.inferredWidth))
        #expect(bend.assessments.count == 18)
        #expect(bend.assessments.allSatisfy { !$0.assessed })
        #expect(bend.assessments.first?.component == "C1")
        #expect(bend.stats.heldOutPoints > 0)
        #expect(bend.stats.residualRmsM.isFinite)
        #expect(bend.profile.count == 61)
        let island = try #require(store.patch(id: "gms-preview:road/fixture/segment/island/0"))
        #expect(island.rings.filter(\.hole).count == 1)
        let junction = try #require(
            store.patch(id: "gms-preview:road/fixture/junction/9000001/0"))
        #expect(junction.kind == .junction)
        // The layer header: hashes, accuracy, a gap, inventory.
        let info = try #require(store.corridors.first)
        #expect(info.version == 1)
        #expect(info.frameAccuracyM <= 0.002)
        #expect(!info.gaps.isEmpty)
        #expect(info.gaps.contains { $0.reason == "car_free" })
    }

    @Test func islandHoleStaysOpen() throws {
        let store = try RoadFixture.store(site: "80858771f")
        let island = try #require(store.patch(id: "gms-preview:road/fixture/segment/island/0"))
        let hole = try #require(island.rings.first { $0.hole })
        let c = hole.points.reduce(SIMD3<Float>.zero, +) / Float(hole.points.count)
        #expect(island.height(at: SIMD2(c.x, c.y)) == nil)
    }

    @Test func shiftedNeighbourLandsOnTheSameWorldPosition() throws {
        // One physical vertex through two site frames: A's coordinates equal
        // B's plus B's origin offset in A, to a few millimetres.
        let a = try RoadFixture.store(site: "80858771f")
        let b = try RoadFixture.store(site: "808587721")
        let shift = RoadFixture.siteB.offset(in: RoadFixture.siteA)
        let offset = SIMD3<Float>(Float(shift.x), Float(shift.y), 0)
        var worst: Float = 0
        for (pa, pb) in zip(a.patches, b.patches) {
            let moved = pb.translated(by: offset, frameToken: "80858771f")
            for (va, vb) in zip(pa.vertices, moved.vertices) where va.z.isFinite {
                worst = max(worst, simd_length(va - vb))
            }
        }
        #expect(worst < 0.01, "worst disagreement \(worst) m")
    }

    @Test func frameShiftMatchesTheManifestControlPoints() throws {
        let manifest = try RoadFixture.manifest()
        let transform = try #require(manifest["transform"] as? [String: Any])
        let points = try #require(transform["control_points"] as? [[String: Any]])
        let shift = RoadFixture.siteB.offset(in: RoadFixture.siteA)
        for point in points {
            let site = try #require(point["site"] as? [String: [Double]])
            let a = try #require(site["80858771f"])
            let b = try #require(site["808587721"])
            // a = b + shift.
            #expect(abs(a[0] - (b[0] + shift.x)) < 0.005)
            #expect(abs(a[1] - (b[1] + shift.y)) < 0.005)
        }
    }

    @Test func aCorruptPatchIsRejectedNotDrawn() {
        var bad = RoadPatch(
            patchID: "x", vertices: [SIMD3(0, 0, 0), SIMD3(1, 0, .nan), SIMD3(0, 1, 0)],
            normals: [SIMD3(0, 0, 1), SIMD3(0, 0, 1), SIMD3(0, 0, 1)], indices: [0, 1, 2],
            support: [.observed, .missing, .observed])
        #expect(bad.defect() == "triangle 0 uses a missing vertex")
        bad.indices = [0, 1, 9]
        #expect(bad.defect() != nil)
        var store = RoadSurfaceStore()
        store.add(bad)
        #expect(store.isEmpty)
        #expect(store.rejected.count == 1)
    }

    @Test func sameIdWithDifferentGeometryIsAConflict() throws {
        var store = try RoadFixture.store(site: "80858771f")
        var other = try #require(store.patches.first)
        other.geometryHash = "0000"
        store.add(other)
        #expect(store.patches.count == 5)
        #expect(store.conflicts == [other.patchID])
        #expect(store.duplicates == 1)
    }

    @Test func availabilityFollowsTheServerVersion() {
        #expect(RoadSurfaceAvailability.forServer(version: 1).isAvailable)
        #expect(RoadSurfaceAvailability.forServer(version: 0).message.contains("version 0"))
        #expect(RoadSurfaceAvailability.forServer(version: 7).message.contains("7"))
        #expect(
            RoadSurfaceAvailability.forRefusal(code: "invalidArgument", message: "unknown layer 17")
                != nil)
        #expect(RoadSurfaceAvailability.forRefusal(code: "internal", message: "boom") == nil)
    }
}

@MainActor struct RoadSurfaceLoaderTests {
    /// A server that answers each site with its fixture stream.
    private func fixtureService(version: UInt32 = 1) -> FakeAlignmentService {
        let service = FakeAlignmentService()
        service.roadSurfaceVersion = version
        service.roadSurfaceHandler = { request, onChunk in
            for (chunk, last) in try RoadFixture.chunks(site: request.l16Token) {
                await onChunk(chunk, last)
            }
        }
        return service
    }

    private func settle(_ loader: RoadSurfaceLoader) async {
        for _ in 0..<400 {
            if !loader.state.isLoading { return }
            try? await Task.sleep(for: .milliseconds(5))
        }
    }

    @Test func loadsBothBundlesIntoOneSceneWithoutDuplicates() async throws {
        let loader = RoadSurfaceLoader()
        let service = fixtureService()
        loader.open(
            corridor: "fixture", sites: [RoadFixture.siteA, RoadFixture.siteB], client: service,
            serverRoadSurfaceVersion: 1)
        await settle(loader)
        #expect(loader.state == .loaded)
        #expect(loader.store.patches.count == 5)
        #expect(loader.store.duplicates == 5)
        #expect(loader.store.conflicts.isEmpty)
        #expect(loader.store.complete)
        #expect(service.calls.filter { $0.hasPrefix("StreamRoadSurface") }.count == 2)
        #expect(loader.sceneToken == "80858771f")
        // Every patch is in the scene frame: the first site's.
        #expect(loader.store.patches.allSatisfy { $0.frameToken == "80858771f" || $0.frameToken == "" })
    }

    @Test func oldServerIsNeverAsked() async {
        let loader = RoadSurfaceLoader()
        let service = fixtureService(version: 0)
        loader.open(
            corridor: "fixture", sites: [RoadFixture.siteA], client: service,
            serverRoadSurfaceVersion: 0)
        #expect(loader.state == .unavailable(.serverHasNoRoadSurface(version: 0)))
        #expect(service.calls.filter { $0.hasPrefix("StreamRoadSurface") }.isEmpty)
        #expect(loader.store.isEmpty)
    }

    @Test func unknownLayerRefusalReadsAsAnOldServer() async {
        let loader = RoadSurfaceLoader()
        let service = fixtureService()
        service.roadSurfaceHandler = { _, _ in
            throw RPCError(code: .invalidArgument, message: "unknown layer 17")
        }
        loader.open(
            corridor: "fixture", sites: [RoadFixture.siteA], client: service,
            serverRoadSurfaceVersion: 1)
        await settle(loader)
        guard case .unavailable(.serverRejectsLayer(let detail)) = loader.state else {
            Issue.record("state \(loader.state)")
            return
        }
        #expect(detail.contains("unknown layer 17"))
    }

    @Test func anEmptyCompleteLayerIsSaidNotShownBlank() async {
        let loader = RoadSurfaceLoader()
        let service = fixtureService()
        service.roadSurfaceHandler = { _, onChunk in
            await onChunk(Gerardus_Align_V1alpha1_RoadSurfaceChunk(), true)
        }
        loader.open(
            corridor: "fixture", sites: [RoadFixture.siteA], client: service,
            serverRoadSurfaceVersion: 1)
        await settle(loader)
        #expect(loader.state == .unavailable(.emptyLayer))
    }

    @Test func aTruncatedStreamKeepsWhatArrivedAsPartial() async throws {
        let loader = RoadSurfaceLoader()
        let service = fixtureService()
        service.roadSurfaceHandler = { request, onChunk in
            let chunks = try RoadFixture.chunks(site: request.l16Token)
            await onChunk(chunks[0].0, false)
            throw RPCError(code: .unavailable, message: "connection reset")
        }
        loader.open(
            corridor: "fixture", sites: [RoadFixture.siteA], client: service,
            serverRoadSurfaceVersion: 1)
        await settle(loader)
        guard case .partial(let reason) = loader.state else {
            Issue.record("state \(loader.state)")
            return
        }
        #expect(reason.contains("connection reset"))
        #expect(loader.store.patches.count == 2)
        #expect(!loader.store.complete)
    }

    @Test func aFailureBeforeAnyPatchIsFailedNotEmpty() async {
        let loader = RoadSurfaceLoader()
        let service = fixtureService()
        service.roadSurfaceHandler = { _, _ in
            throw RPCError(code: .unavailable, message: "no route")
        }
        loader.open(
            corridor: "fixture", sites: [RoadFixture.siteA], client: service,
            serverRoadSurfaceVersion: 1)
        await settle(loader)
        guard case .failed(let reason) = loader.state else {
            Issue.record("state \(loader.state)")
            return
        }
        #expect(reason.contains("no route"))
    }

    @Test func aMissingBundleIsNamedAndTheRestStillLoads() async {
        let loader = RoadSurfaceLoader()
        let service = fixtureService()
        service.roadSurfaceHandler = { request, onChunk in
            if request.l16Token == "808587721" {
                throw RPCError(code: .notFound, message: "no bundle")
            }
            for (chunk, last) in try RoadFixture.chunks(site: request.l16Token) {
                await onChunk(chunk, last)
            }
        }
        loader.open(
            corridor: "fixture", sites: [RoadFixture.siteA, RoadFixture.siteB], client: service,
            serverRoadSurfaceVersion: 1)
        await settle(loader)
        guard case .partial(let reason) = loader.state else {
            Issue.record("state \(loader.state)")
            return
        }
        #expect(reason.contains("808587721"))
        #expect(loader.store.patches.count == 5)
    }

    @Test func aLateResponseFromTheOldCorridorIsDiscarded() async throws {
        let loader = RoadSurfaceLoader()
        let service = fixtureService()
        // The first corridor's answer is held until the second has loaded.
        let gate = AsyncGate()
        service.roadSurfaceHandler = { request, onChunk in
            if request.l16Token == "808587721" { await gate.wait() }
            for (chunk, last) in try RoadFixture.chunks(site: request.l16Token) {
                await onChunk(chunk, last)
            }
        }
        loader.open(
            corridor: "old", sites: [RoadFixture.siteB], client: service,
            serverRoadSurfaceVersion: 1)
        try await Task.sleep(for: .milliseconds(30))
        loader.open(
            corridor: "new", sites: [RoadFixture.siteA], client: service,
            serverRoadSurfaceVersion: 1)
        await settle(loader)
        await gate.open()
        try await Task.sleep(for: .milliseconds(60))
        #expect(loader.corridorKey == "new")
        #expect(loader.state == .loaded)
        #expect(loader.store.patches.count == 5)
        #expect(loader.store.patches.allSatisfy { $0.frameToken == "80858771f" })
        #expect(loader.sceneToken == "80858771f")
    }

    @Test func cancellingKeepsWhatArrivedAndSaysSo() async throws {
        let loader = RoadSurfaceLoader()
        let service = fixtureService()
        let gate = AsyncGate()
        service.roadSurfaceHandler = { request, onChunk in
            let chunks = try RoadFixture.chunks(site: request.l16Token)
            await onChunk(chunks[0].0, false)
            await gate.wait()
            await onChunk(chunks[1].0, true)
        }
        loader.open(
            corridor: "fixture", sites: [RoadFixture.siteA], client: service,
            serverRoadSurfaceVersion: 1)
        try await Task.sleep(for: .milliseconds(40))
        #expect(loader.store.patches.count == 2)
        loader.cancel()
        #expect(loader.state == .partial(reason: "cancelled"))
        await gate.open()
        try await Task.sleep(for: .milliseconds(40))
        // The second chunk, late, changed nothing.
        #expect(loader.store.patches.count == 2)
        #expect(loader.discardedLate >= 1)
    }
}

/// Holds a task until a test lets it go.
actor AsyncGate {
    private var opened = false
    private var waiters: [CheckedContinuation<Void, Never>] = []
    func wait() async {
        if opened { return }
        await withCheckedContinuation { waiters.append($0) }
    }
    func open() {
        opened = true
        for w in waiters { w.resume() }
        waiters = []
    }
}
