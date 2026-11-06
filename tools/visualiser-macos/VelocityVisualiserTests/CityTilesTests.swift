// CityTilesTests.swift
// Gerardus's city tiles: the tileset and glTF readers, selection by
// screen-space error, the LRU cache, the location rules, and the
// camera-relative origin (lidar-city-view-plan Item 6). Read against the
// shared fixture in testdata/city-view/tiles.

import Foundation
import MetalKit
import Testing
import simd

@testable import VelocityVisualiser

let cityTilesFixture = cityViewFixtures.appendingPathComponent("tiles")

func fixtureTileset() throws -> CityTileset {
    try CityTileset.decode(
        Data(contentsOf: cityTilesFixture.appendingPathComponent("tileset.json")))
}

struct CityTilesetTests {
    @Test func readsTheFixtureTree() throws {
        let tileset = try fixtureTileset()
        #expect(tileset.version == "1.1")
        #expect(tileset.tilesetVersion == "fixture")
        let content = tileset.root.all.filter(\.hasContent)
        #expect(content.count == 19)
        let levels = Dictionary(grouping: content, by: { $0.level ?? 0 }).mapValues(\.count)
        #expect(levels == [10: 1, 13: 2, 16: 16])
        #expect(tileset.root.contentURI == nil)
        #expect(tileset.root.origin == .zero)
        let l10 = try #require(tileset.root.children.first)
        #expect(l10.contentURI == "l10/808581.glb")
        #expect(l10.geometricError == 50)
        #expect(l10.origin == SIMD3(3397.102, 6187.965, -3.914))
    }

    @Test func composedTransformsAgreeWithTheStatedOrigins() throws {
        // Strip every extras.origin_city_enu_m and read the tree again: the
        // parent-relative transforms must compose to the same origins.
        let url = cityTilesFixture.appendingPathComponent("tileset.json")
        var json = try #require(
            try JSONSerialization.jsonObject(with: Data(contentsOf: url)) as? [String: Any])
        func strip(_ node: inout [String: Any]) {
            if var extras = node["extras"] as? [String: Any] {
                extras["origin_city_enu_m"] = nil
                node["extras"] = extras
            }
            if var children = node["children"] as? [[String: Any]] {
                for i in children.indices { strip(&children[i]) }
                node["children"] = children
            }
        }
        var root = try #require(json["root"] as? [String: Any])
        strip(&root)
        json["root"] = root
        let composed = try CityTileset.decode(JSONSerialization.data(withJSONObject: json))
        let stated = try fixtureTileset().contentTiles
        for (uri, tile) in composed.contentTiles {
            let expected = try #require(stated[uri]).origin
            #expect(simd_length(tile.origin - expected) < 0.01, "\(uri)")
        }
    }

    @Test func readsAnS2TokensLevel() {
        #expect(CityTileset.level(ofToken: "808581") == 10)
        #expect(CityTileset.level(ofToken: "8085805c") == 13)
        #expect(CityTileset.level(ofToken: "808580f4") == 13)
        #expect(CityTileset.level(ofToken: "808580593") == 16)
        #expect(CityTileset.level(ofToken: "808580f47") == 16)
    }

    @Test func refusesWhatIsNotATileset() {
        #expect(throws: CityTileset.ReadError.self) {
            try CityTileset.decode(Data(#"{"asset": {"version": "0.0"}, "root": {}}"#.utf8))
        }
        #expect(throws: CityTileset.ReadError.self) { try CityTileset.decode(Data("[]".utf8)) }
    }
}

struct GLBReaderTests {
    private func tile(_ path: String) throws -> CityTileContent {
        try GLBReader.read(Data(contentsOf: cityTilesFixture.appendingPathComponent(path)))
    }

    @Test func readsAStreetTile() throws {
        let content = try tile("l16/8085805c1.glb")
        #expect(content.ground.vertexCount == 1097)
        #expect(content.ground.indices.count == 6120)
        #expect(content.buildings.vertexCount == 16)
        #expect(content.buildings.indices.count == 60)
        #expect(content.streets.vertexCount == 108)
        #expect(content.streets.indices.count == 214)
        #expect(content.level == 16)
        #expect(content.s2 == "8085805c1")
        #expect(content.origin == SIMD3(3958.903, 4753.491, -3.004))
        #expect(
            content.labels == [
                StreetLabel(
                    name: "COLUMBUS AVE", anchor: SIMD3(-62.38, 41.91, 20), bearingDeg: 38.4,
                    streetClass: "3")
            ])
    }

    @Test func turnsGLTFsYUpIntoTheCitysZUp() throws {
        // The ground accessor's bounds in glTF: y (up) 19.99…22.33, z (−north)
        // −90.51…89.49.
        let ground = try tile("l16/8085805c1.glb").ground
        let up = stride(from: 2, to: ground.positions.count, by: 3).map { ground.positions[$0] }
        let north = stride(from: 1, to: ground.positions.count, by: 3).map { ground.positions[$0] }
        #expect(abs(up.min()! - 19.987409) < 1e-4 && abs(up.max()! - 22.325911) < 1e-4)
        #expect(abs(north.min()! + 89.491348) < 1e-4 && abs(north.max()! - 90.508652) < 1e-4)
    }

    @Test func readsEveryFixtureTileAndItsOriginMatchesTheTileset() throws {
        for (uri, tile) in try fixtureTileset().contentTiles {
            let content = try self.tile(uri)
            #expect(content.origin == tile.origin, "\(uri)")
            #expect(content.ground.indices.count % 3 == 0)
            #expect(content.buildings.indices.count % 3 == 0)
            #expect(content.streets.indices.count % 2 == 0)
            #expect(content.ground.indices.allSatisfy { Int($0) < content.ground.vertexCount })
        }
    }

    @Test func buildingsWindCounterClockwiseFromOutside() throws {
        // A roof triangle's normal, from its winding, points up.
        let b = try tile("l16/8085805c1.glb").buildings
        func p(_ i: UInt32) -> SIMD3<Float> {
            let k = Int(i) * 3
            return SIMD3(b.positions[k], b.positions[k + 1], b.positions[k + 2])
        }
        var upward = 0
        var downward = 0
        for t in stride(from: 0, to: b.indices.count, by: 3) {
            let n = simd_cross(
                p(b.indices[t + 1]) - p(b.indices[t]), p(b.indices[t + 2]) - p(b.indices[t]))
            if n.z > 0.5 * simd_length(n) { upward += 1 }
            if n.z < -0.5 * simd_length(n) { downward += 1 }
        }
        #expect(upward > 0)
        #expect(upward >= downward)
    }

    @Test func refusesWhatIsNotGLB() throws {
        #expect(throws: GLBReader.ReadError.notGLB) { try GLBReader.read(Data("glTF".utf8)) }
        let real = try Data(contentsOf: cityTilesFixture.appendingPathComponent("l13/808580f4.glb"))
        #expect(throws: GLBReader.ReadError.self) {
            try GLBReader.read(real.prefix(real.count / 2))
        }
    }
}

struct TileSelectionTests {
    private func view(eye: SIMD3<Double>, height: Double = 1000) -> TileView {
        TileView(eye: eye, viewportHeight: height, fovY: 60 * .pi / 180)
    }

    @Test func fromHighAboveTheCityLevelIsDrawnOnceLoaded() throws {
        let tileset = try fixtureTileset()
        let high = view(eye: SIMD3(3400, 4700, 20_000))
        let nothing = TileSelector.select(tileset, view: high) { _ in false }
        #expect(nothing.draw.isEmpty)
        #expect(nothing.load == ["l10/808581.glb"])
        let loaded = TileSelector.select(tileset, view: high) { _ in true }
        #expect(loaded.draw == ["l10/808581.glb"])
    }

    @Test func atStreetLevelTheNearTilesRefine() throws {
        let tileset = try fixtureTileset()
        // Over Columbus Ave's L16 tile, 60 m up.
        let street = view(eye: SIMD3(3958, 4753, 60))
        let all = TileSelector.select(tileset, view: street) { _ in true }
        #expect(all.draw.contains("l16/8085805c1.glb"))
        #expect(!all.draw.contains("l10/808581.glb"))
        // Every tile drawn covers its own ground once: no tile with its parent.
        let tiles = tileset.contentTiles
        for id in all.draw {
            let tile = try #require(tiles[id])
            for child in tile.children { #expect(!all.draw.contains(child.id)) }
        }
    }

    @Test func aLoadingChildShowsItsParent() throws {
        let tileset = try fixtureTileset()
        let street = view(eye: SIMD3(3958, 4753, 60))
        // Only the city and district levels are loaded yet.
        let loaded: Set<String> = ["l10/808581.glb", "l13/8085805c.glb", "l13/808580f4.glb"]
        let partial = TileSelector.select(tileset, view: street) { loaded.contains($0) }
        #expect(partial.draw.contains("l13/8085805c.glb"))
        #expect(!partial.draw.contains("l16/8085805c1.glb"))
        #expect(partial.load.contains("l16/8085805c1.glb"))
        // Nothing loaded at all: the city level is asked for first.
        let cold = TileSelector.select(tileset, view: street) { _ in false }
        #expect(cold.load.first == "l10/808581.glb")
    }

    @Test func theErrorFallsWithDistance() throws {
        let tile = try #require(try fixtureTileset().root.children.first)
        let near = view(eye: tile.cityCentre + SIMD3(0, 0, 1_000)).screenSpaceError(tile)
        let far = view(eye: tile.cityCentre + SIMD3(0, 0, 30_000)).screenSpaceError(tile)
        #expect(near > far)
        #expect(far < 16)
    }

    @Test func theFrustumDropsWhatIsBehindTheEye() {
        var camera = Camera()
        camera.position = SIMD3(0, -30, 20)
        camera.target = SIMD3(0, 10, 0)
        let frustum = Frustum(clip: simd_double4x4(camera.projectionMatrix * camera.viewMatrix))
        #expect(frustum.contains(sphere: SIMD3(0, 10, 0), radius: 1))
        #expect(!frustum.contains(sphere: SIMD3(0, -200, 40), radius: 5))
        #expect(!frustum.contains(sphere: SIMD3(5_000, 10, 0), radius: 5))
        // A big sphere around the eye is always in.
        #expect(frustum.contains(sphere: SIMD3(0, -200, 40), radius: 500))
    }
}

struct LRUCacheTests {
    @Test func evictsTheLeastRecentlyUsedFirst() {
        var cache = LRUCache<Int>(capacityBytes: 30)
        cache.insert(1, for: "a", bytes: 10)
        cache.insert(2, for: "b", bytes: 10)
        cache.insert(3, for: "c", bytes: 10)
        _ = cache.get("a")
        let evicted = cache.insert(4, for: "d", bytes: 10)
        #expect(evicted == ["b"])
        #expect(cache.totalBytes == 30)
        #expect(cache.contains("a") && !cache.contains("b"))
    }

    @Test func neverEvictsWhatIsDrawnNow() {
        var cache = LRUCache<Int>(capacityBytes: 20)
        cache.insert(1, for: "a", bytes: 10)
        cache.insert(2, for: "b", bytes: 10)
        let evicted = cache.insert(3, for: "c", bytes: 10, keep: ["a", "b"])
        #expect(evicted.isEmpty)
        #expect(cache.totalBytes == 30)
        let next = cache.insert(4, for: "d", bytes: 10, keep: ["c"])
        #expect(Set(next) == ["a", "b"])
        cache.insert(5, for: "d", bytes: 5)
        #expect(cache.totalBytes == 15)
    }
}

struct CityTileSourceTests {
    @Test func aLocationNamesAnEpochTheTilesFolderOrTheFile() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(
            "city-tiles-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: root) }
        let older = root.appendingPathComponent("epochs/20261101T000000Z")
        let newer = root.appendingPathComponent("epochs/20261105T200804Z")
        for epoch in [older, newer] {
            try FileManager.default.createDirectory(at: epoch, withIntermediateDirectories: true)
            try Data("{}".utf8).write(to: epoch.appendingPathComponent("tileset.json"))
        }
        // No `current` link (as over SMB): the newest epoch.
        #expect(
            try CityTileSource.tilesetURL(for: root).path
                == newer.appendingPathComponent("tileset.json").path)
        // An epoch directly.
        #expect(
            try CityTileSource.tilesetURL(for: older).path
                == older.appendingPathComponent("tileset.json").path)
        // A current link wins over the epochs.
        try FileManager.default.createSymbolicLink(
            at: root.appendingPathComponent("current"), withDestinationURL: older)
        #expect(
            try CityTileSource.tilesetURL(for: root).path
                == root.appendingPathComponent("current/tileset.json").path)
        // A file, and a URL.
        let file = older.appendingPathComponent("tileset.json")
        #expect(try CityTileSource.tilesetURL(for: file) == file)
        let web = try #require(URL(string: "https://example.org/city-tiles/epochs/x"))
        #expect(try CityTileSource.tilesetURL(for: web).lastPathComponent == "tileset.json")
        #expect(throws: CityTileSource.LocateError.self) {
            try CityTileSource.tilesetURL(for: root.appendingPathComponent("nothing"))
        }
    }

    @Test func ungzipsATile() throws {
        let gz: [UInt8] = [
            0x1f, 0x8b, 0x08, 0x08, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0x2e, 0x74, 0x78,
            0x74, 0x00, 0x4b, 0xce, 0x2c, 0xa9, 0x54, 0x28, 0xc9, 0xcc, 0x49, 0x2d, 0xd6, 0x51,
            0x48, 0xaf, 0xca, 0x2c, 0x28, 0x48, 0x4d, 0xe1, 0x4a, 0x26, 0x52, 0x0c, 0x00, 0xa2,
            0xa6, 0xd1, 0x06, 0x3c, 0x00, 0x00, 0x00,
        ]
        let text = try CityTileSource.gunzipIfNeeded(Data(gz))
        #expect(
            String(decoding: text, as: UTF8.self)
                == String(repeating: "city tiles, gzipped\n", count: 3))
        let plain = Data("glTF".utf8)
        #expect(try CityTileSource.gunzipIfNeeded(plain) == plain)
    }
}

struct RenderOriginTests {
    @Test func aFarTileIsPlacedWithoutFloat32Error() {
        // A building corner 4 km from the city origin, 3 mm from its tile's
        // origin, seen from a camera standing 20 m from it.
        let tileOrigin = SIMD3<Double>(3958.903, 4753.491, -3.004)
        let corner = SIMD3<Float>(0.003, 0.001, 0)
        var origin = RenderOrigin()
        origin.anchor = SIMD3(3950, 4750, 0)
        let model = origin.model(simd_double4x4(translation: tileOrigin))
        let local = model * SIMD4(corner, 1)
        let exact = tileOrigin + SIMD3<Double>(corner) - origin.anchor
        #expect(simd_length(SIMD3<Double>(SIMD3<Float>(local.x, local.y, local.z)) - exact) < 1e-5)
        // Built in Float32 in the city frame instead, the same point lands
        // hundreds of micrometres off.
        let naive = SIMD3<Float>(tileOrigin) + corner - SIMD3<Float>(origin.anchor)
        #expect(
            simd_length(SIMD3<Double>(naive) - exact)
                > simd_length(SIMD3<Double>(SIMD3<Float>(local.x, local.y, local.z)) - exact))
    }

    @Test func recentringMovesTheAnchorNotTheView() {
        var origin = RenderOrigin()
        #expect(origin.recentre(target: SIMD3(500, 0, 0)) == nil)
        let shift = origin.recentre(target: SIMD3(1500.4, -2200.6, 3))
        #expect(shift == SIMD3(1500, -2201, 3))
        #expect(origin.anchor == SIMD3(1500, -2201, 3))
        #expect(simd_length(origin.world(SIMD3(0.4, 0.4, 0)) - SIMD3(1500.4, -2200.6, 3)) < 1e-6)
    }

    @Test func theRendererKeepsTheCameraWhereItIsInTheWorld() throws {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let view = MTKView()
        view.device = device
        let renderer = try #require(MetalRenderer(metalView: view))
        renderer.camera.position = SIMD3(1990, 2010, 50)
        renderer.camera.target = SIMD3(2000, 2000, 0)
        let world = renderer.origin.world(renderer.camera.position)
        renderer.recentreOriginIfNeeded()
        #expect(renderer.origin.anchor == .zero, "the sensor frame never recentres")
        renderer.recentresOrigin = true
        renderer.recentreOriginIfNeeded()
        #expect(renderer.origin.anchor == SIMD3(2000, 2000, 0))
        #expect(renderer.camera.target == .zero)
        #expect(renderer.origin.world(renderer.camera.position) == world)
        renderer.setOrigin(anchor: .zero)
        #expect(renderer.camera.target == SIMD3(2000, 2000, 0))
    }
}

@MainActor struct CityTileLayerTests {
    @Test func loadsSelectsAndDrawsTheFixture() async throws {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let layer = CityTileLayer(device: device)
        let url = cityTilesFixture.appendingPathComponent("tileset.json")
        layer.setTileset(try fixtureTileset(), url: url)
        let street = TileView(eye: SIMD3(3958, 4753, 60), viewportHeight: 1000, fovY: 1.047)
        // Each pass loads what the last one asked for, coarse to fine.
        for _ in 0..<8 {
            _ = layer.select(view: street)
            await layer.settle()
        }
        #expect(layer.failed.isEmpty)
        let selection = layer.select(view: street)
        #expect(selection.load.isEmpty)
        #expect(selection.draw.contains("l16/8085805c1.glb"))
        let drawn = layer.drawable(selection)
        #expect(drawn.count == selection.draw.count)
        let columbus = try #require(drawn.first { $0.id == "l16/8085805c1.glb" })
        #expect(columbus.labels.first?.name == "COLUMBUS AVE")
        #expect(columbus.buildings?.indexCount == 60)
        #expect(layer.cache.totalBytes > 0)
        // A new tileset drops every tile of the old.
        layer.setTileset(nil, url: nil)
        #expect(layer.cache.count == 0)
        #expect(layer.select(view: street) == TileSelection())
    }

    @Test func aFailedTileIsNotAskedForAgain() async throws {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let layer = CityTileLayer(device: device)
        layer.read = { _ in Data("not a tile".utf8) }
        layer.setTileset(
            try fixtureTileset(), url: cityTilesFixture.appendingPathComponent("tileset.json"))
        let high = TileView(eye: SIMD3(3400, 4700, 20_000), viewportHeight: 1000, fovY: 1.047)
        _ = layer.select(view: high)
        await layer.settle()
        #expect(layer.failed["l10/808581.glb"] != nil)
        _ = layer.select(view: high)
        await layer.settle()
        #expect(layer.cache.count == 0)
    }

    @Test func theSessionReadsTheTilesLocation() async throws {
        let city = CityViewSession(defaults: nil)
        var told: CityTileset?
        city.onTileset = { tileset, _ in told = tileset }
        await city.useTilesLocation(cityTilesFixture.path)
        #expect(city.tileset != nil && told != nil)
        #expect(city.tilesStatus == "19 tiles · epoch fixture")
        await city.useTilesLocation("/nowhere/at/all")
        #expect(city.tileset == nil)
        #expect(city.tilesError?.contains("No tileset.json") == true)
        await city.useTilesLocation("")
        #expect(city.tilesStatus == "No tiles: Gerardus is not connected, or serves none")
    }

    @Test func aPlacedSitePlacesTheCityAroundItsSensor() async throws {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let view = MTKView()
        view.device = device
        let renderer = try #require(MetalRenderer(metalView: view))
        let index = try fixtureSiteIndex()
        let columbus = try #require(index.site(id: "columbus-broadway"))
        renderer.leaveSite(for: SiteContext(site: columbus))
        #expect(renderer.cityTilesPlaced)
        // The sensor's own position in the city lands on the sensor frame's origin.
        let sensorCity = try #require(columbus.sensorCityPosition)
        let inSensor = renderer.cityTiles.worldFromCity * SIMD4(sensorCity, 1)
        #expect(simd_length(SIMD3(inSensor.x, inSensor.y, inSensor.z)) < 1e-6)
        #expect(renderer.cityFade?.centre == sensorCity)
        renderer.leaveSite(for: SiteContext(site: index.site(id: "soma1")))
        #expect(!renderer.cityTilesPlaced)
        #expect(renderer.cityFade == nil)
    }
}
