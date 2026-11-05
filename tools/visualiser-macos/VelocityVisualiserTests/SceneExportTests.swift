// SceneExportTests.swift
// The scene export reader and the nearby sites played from it
// (lidar-city-view-plan Item 8), against the committed scene fixture in
// testdata/city-view/scenes/columbus-broadway/assets.

import Foundation
import MetalKit
import Testing
import simd

@testable import VelocityVisualiser

let sceneFixture = cityViewFixtures.appendingPathComponent("scenes/columbus-broadway/assets")

struct SceneExportReaderTests {
    @Test func readsTheManifestHeaderIndexAndTimeline() async throws {
        let session = try await SceneSession.open(
            manifest: sceneFixture.appendingPathComponent("manifest.json"))
        #expect(session.title == "Columbus at Broadway")
        #expect(session.parts.count == 1)
        let info = try #require(session.infos.first)
        #expect(info.site == "columbus-broadway")
        #expect(info.sensorID == "hesai-pandar40p")
        #expect(info.startNs == 1_788_380_436_986_210_330)
        #expect(info.frameCount == 20)
        #expect(
            info.chunks == [
                SceneChunk(c: 0, n: 10, t0: 0, t1: 899_934),
                SceneChunk(c: 1, n: 10, t0: 10_200_023, t1: 11_100_280),
            ])
        #expect(info.bucketSeconds == 5)
        #expect(info.buckets.count == 3)
        #expect(abs(session.duration - 11.10028) < 1e-6)
    }

    @Test func framesCarryBoxesAndHeadingsOnly() async throws {
        let session = try await SceneSession.open(
            manifest: sceneFixture.appendingPathComponent("manifest.json"))
        let first = try #require(try await session.frame(at: 0).frame)
        #expect(first.t == 0)
        #expect(first.boxes.count == 56)
        // A box is drawn at its own centre, with its own heading.
        let box = try #require(first.boxes.first { $0.id == "0" })
        #expect(box.x == -13.74 && box.y == -9.19 && box.z == -1.54)
        #expect(box.length == 4.01 && box.width == 1.72 && box.height == 1.34)
        #expect(box.boxHeading == 2.666)
    }

    @Test func findsTheFrameAtOrBeforeATime() async throws {
        let session = try await SceneSession.open(
            manifest: sceneFixture.appendingPathComponent("manifest.json"))
        let part = session.parts[0]
        #expect(try await part.frame(atOffset: 150_000)?.t == 99_824)
        // Between the two chunks: the earlier one's last frame.
        #expect(try await part.frame(atOffset: 5_000_000)?.t == 899_934)
        #expect(try await part.frame(atOffset: 10_200_023)?.t == 10_200_023)
        #expect(try await part.frame(atOffset: 99_000_000)?.t == 11_100_280)
        #expect(await part.cachedChunks == [0, 1])
        // Scene time and capture time agree.
        #expect(session.captureNanos(at: 0) == 1_788_380_436_986_210_330)
    }

    @Test func refusesAMalformedScene() {
        #expect(throws: SceneError.self) {
            try SceneDecoding.partURLs(
                manifest: Data(#"{"parts": []}"#.utf8), at: URL(fileURLWithPath: "/m.json"))
        }
        #expect(throws: SceneError.self) {
            try SceneDecoding.partInfo(
                header: Data("{}".utf8),
                index: Data(#"{"chunks": [{"c": 0, "n": 1, "t0": 5, "t1": 1}]}"#.utf8),
                timeline: nil)
        }
        #expect(throws: SceneError.self) {
            try SceneDecoding.frames(chunk: Data(#"{"tr": []}"#.utf8))
        }
    }

    @Test func aChunkThatCannotBeReadIsNotAskedForAgain() async throws {
        let calls = Counter()
        let read: @Sendable (URL) async throws -> Data = { url in
            if url.lastPathComponent.hasPrefix("chunk_") {
                await calls.add()
                throw URLError(.fileDoesNotExist)
            }
            return try Data(contentsOf: url)
        }
        let session = try await SceneSession.open(
            manifest: sceneFixture.appendingPathComponent("manifest.json"), read: read)
        for _ in 0..<3 { _ = try? await session.frame(at: 0) }
        #expect(await calls.value == 1)
    }
}

actor Counter {
    private(set) var value = 0
    func add() { value += 1 }
}

struct SceneClockTests {
    @Test func eachSitePlaysOnItsOwnClockAndLoops() {
        let a = SceneClock(startedAt: 100, duration: 30)
        let b = SceneClock(startedAt: 112, duration: 30)
        #expect(a.sceneSeconds(at: 100) == 0)
        #expect(a.sceneSeconds(at: 115) == 15)
        #expect(a.sceneSeconds(at: 145) == 15)
        // Started at a different moment, the other site is somewhere else in
        // its own recording: the two are never one moment.
        #expect(b.sceneSeconds(at: 115) == 3)
        #expect(SceneClock(startedAt: 0, duration: 0).sceneSeconds(at: 50) == 0)
    }
}

@MainActor struct NearbySitesTests {
    private func site(_ id: String, at p: SIMD3<Double>, placed: Bool = true) -> CitySite {
        var s = CitySite(id: id, title: id)
        s.sceneAssets = "/scenes/columbus-broadway/assets/"
        if placed { s.sensorToCity = simd_double4x4(translation: p) } else { s.markerCityENU = p }
        return s
    }

    @Test func playsTheNearestPlacedSitesUpToTheLimit() {
        var sites = (0..<12).map { site("s\($0)", at: SIMD3(Double($0) * 100, 0, 0)) }
        sites.append(site("unplaced", at: SIMD3(1, 0, 0), placed: false))
        sites.append(site("far", at: SIMD3(0, 5_000, 0)))
        let chosen = NearbySites.choose(sites, focusedID: "s0", near: .zero)
        #expect(chosen.map(\.id) == ["s1", "s2", "s3", "s4", "s5", "s6", "s7", "s8"])
        #expect(
            NearbySites.choose(sites, focusedID: nil, near: SIMD3(0, 5_000, 0), limit: 1).map(\.id)
                == ["far"])
    }

    @Test func findsAScenesFilesFromTheIndexLocation() throws {
        let index = try fixtureSiteIndex()
        let columbus = try #require(index.site(id: "columbus-broadway"))
        // The fixture keeps scenes as a source checkout does, under scenes/.
        let indexURL = cityViewFixtures.appendingPathComponent("city/sites.json")
        #expect(
            SceneAssets.manifestURL(for: columbus, indexURL: indexURL)?.path
                == sceneFixture.appendingPathComponent("manifest.json").path)
        let web = try #require(URL(string: "https://example.org/city/sites.json"))
        #expect(
            SceneAssets.manifestURL(for: columbus, indexURL: web)?.absoluteString
                == "https://example.org/surveys/columbus-broadway/assets/manifest.json")
        #expect(
            SceneAssets.manifestURL(
                for: try #require(index.site(id: "embarcadero-broadway")), indexURL: indexURL)
                == nil)
    }

    @Test func eachNearbySiteKeepsToItselfOnItsOwnClock() async throws {
        let nearby = NearbySites()
        nearby.indexURL = { cityViewFixtures.appendingPathComponent("city/sites.json") }
        var clock = 1_000.0
        nearby.now = { clock }
        // Two sites playing the same files, 300 m apart, and the focused one.
        let a = site("site-a", at: SIMD3(0, 0, 0))
        var b = site("site-b", at: SIMD3(300, 0, 0))
        b.sceneAssets = nil
        b.id = "columbus-broadway"
        let focused = site("focused", at: SIMD3(150, 0, 0))
        nearby.update(sites: [a, b, focused], focusedID: "focused", near: SIMD3(150, 0, 0))
        await nearby.settle()
        #expect(nearby.playingIDs == ["columbus-broadway", "site-a"])
        clock += 0.5
        let frames = await nearby.frames()
        #expect(frames.map(\.siteID) == ["columbus-broadway", "site-a"])
        for frame in frames {
            // Every box is namespaced by its own site, and only its own.
            #expect(frame.boxes.allSatisfy { $0.id.hasPrefix(frame.siteID + "/") })
            #expect(frame.clock != nil)
            #expect(!frame.live)
        }
        #expect(frames[1].worldFromSensor == a.sensorToCity)
        let ids = frames.flatMap { $0.boxes.map(\.id) }
        #expect(Set(ids).count == ids.count, "no box is in two sites")
        #expect(!frames.contains { $0.siteID == "focused" })

        // Out of range, a site stops.
        nearby.update(sites: [a, b, focused], focusedID: "focused", near: SIMD3(5_000, 0, 0))
        #expect(nearby.playingIDs.isEmpty)
    }

    @Test func aSiteWithoutAScenePlaysNothing() async {
        let nearby = NearbySites()
        nearby.indexURL = { cityViewFixtures.appendingPathComponent("city/sites.json") }
        var lonely = site("lonely", at: .zero)
        lonely.sceneAssets = nil
        nearby.update(sites: [lonely], focusedID: nil, near: .zero)
        await nearby.settle()
        #expect(nearby.failure("lonely") == "no published scene")
        #expect(await nearby.frames().isEmpty)
    }

    @Test func aLivePiIsStreamedInItsOwnContext() {
        let site = site("pi-site", at: .zero)
        let stream = LiveSiteStream(site: site, client: VisualiserClient(address: "127.0.0.1:1"))
        var frame = FrameBundle()
        frame.timestampNanos = 1_788_380_400_000_000_000
        var t = Track(
            trackID: "7", sensorID: "s", state: .confirmed, hits: 5, misses: 0, observationCount: 5,
            firstSeenNanos: 1, lastSeenNanos: 2, speedMps: 6, maxSpeedMps: 6, classLabel: "car")
        t.x = 4
        t.y = 2
        frame.tracks = TrackSet(frameID: 1, timestampNanos: 1, tracks: [t], trails: [])
        stream.receive(frame)
        #expect(stream.boxes.map(\.id) == ["7"])
        #expect(stream.context.siteID == "pi-site")
        #expect(stream.context.tracks.allSeen.keys.sorted() == ["7"])
        #expect(stream.context.source == .live(sensorID: "pi-site"))
        #expect(NearbySites.namespaced(stream.boxes[0], site: "pi-site").id == "pi-site/7")
    }
}

@MainActor struct NearbyRenderingTests {
    private func box(_ id: String, x: Float, speed: Float = 5, label: String = "car") -> SceneBox {
        SceneBox(
            id: id, x: x, y: 0, z: -1.5, length: 4, width: 2, height: 1.5, boxHeading: 0,
            heading: 0, speed: speed, classLabel: label)
    }

    @Test func boxesAreKeptToTheirCoverageAndColouredByClass() {
        let coverage = SiteCoverage(radius: 100)
        let floats = MetalRenderer.cityBoxInstances(
            [box("a", x: 10), box("b", x: 95, label: "pedestrian"), box("c", x: 150)],
            coverage: coverage)
        #expect(floats.count == 40)
        #expect(SIMD3(floats[16], floats[17], floats[18]) == ClassColour.colour(for: "car"))
        #expect(floats[19] == 1)
        #expect(floats[39] > 0 && floats[39] < 1)
        let lines = MetalRenderer.cityHeadingLines(
            [box("a", x: 10), box("still", x: 20, speed: 0)], coverage: coverage)
        // One heading line (2 vertices) and the ring (96 segments).
        #expect(lines.count == (2 + 96 * 2) * 7)
    }

    @Test func noDrawnObjectHoldsTwoSites() throws {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let view = MTKView()
        view.device = device
        let renderer = try #require(MetalRenderer(metalView: view))
        renderer.setCityMode(true)
        renderer.setNearbySites([
            NearbySiteFrame(
                siteID: "a", worldFromSensor: matrix_identity_double4x4,
                boxes: [NearbySites.namespaced(box("1", x: 5), site: "a")], clock: nil, live: false),
            NearbySiteFrame(
                siteID: "b", worldFromSensor: simd_double4x4(translation: SIMD3(400, 0, 0)),
                boxes: [
                    NearbySites.namespaced(box("1", x: 5), site: "b"),
                    NearbySites.namespaced(box("2", x: 8), site: "b"),
                ], clock: nil, live: false),
        ])
        #expect(renderer.nearby.map(\.siteID) == ["a", "b"])
        for drawable in renderer.nearby {
            #expect(drawable.boxIDs.allSatisfy { $0.hasPrefix(drawable.siteID + "/") })
        }
        #expect(renderer.nearby[1].boxCount == 2)
        renderer.setCityMode(false)
        #expect(renderer.nearby.isEmpty)
    }
}
