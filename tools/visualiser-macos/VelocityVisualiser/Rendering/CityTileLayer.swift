// CityTileLayer.swift
// Draws Gerardus's city tiles in the renderer (lidar-city-view-plan Item 6):
// selects tiles by screen-space error for the current view, loads the ones
// it lacks off the main thread, keeps the loaded ones in an LRU cache of
// GPU buffers, and draws each tile as ground, buildings (flat shaded from
// each triangle's normal) and street lines: three draw calls a tile.
//
// Every tile is placed by its origin in the city frame, in Double, through
// the renderer's RenderOrigin, so the matrices the GPU sees stay small.

import Foundation
import Metal
import simd

/// One tile's GPU buffers.
final class GPUCityTile {
    struct Layer {
        var positions: MTLBuffer
        var indices: MTLBuffer
        var indexCount: Int
    }

    let id: String
    let origin: SIMD3<Double>
    let ground: Layer?
    let buildings: Layer?
    let streets: Layer?
    let labels: [StreetLabel]
    let byteCount: Int

    init?(id: String, origin: SIMD3<Double>, content: CityTileContent, device: MTLDevice) {
        func layer(_ mesh: TileMesh) -> Layer? {
            guard !mesh.isEmpty,
                let positions = device.makeBuffer(
                    bytes: mesh.positions, length: mesh.positions.count * 4,
                    options: .storageModeShared),
                let indices = device.makeBuffer(
                    bytes: mesh.indices, length: mesh.indices.count * 4, options: .storageModeShared
                )
            else { return nil }
            return Layer(positions: positions, indices: indices, indexCount: mesh.indices.count)
        }
        self.id = id
        self.origin = content.origin ?? origin
        ground = layer(content.ground)
        buildings = layer(content.buildings)
        streets = layer(content.streets)
        labels = content.labels
        byteCount = max(content.byteCount, 1)
    }
}

/// The colours the city is drawn in: quiet, so the traffic reads over it.
enum CityTileStyle {
    static let ground = SIMD4<Float>(0.19, 0.20, 0.23, 1)
    static let buildings = SIMD4<Float>(0.55, 0.57, 0.62, 1)
    static let streets = SIMD4<Float>(0.85, 0.75, 0.45, 0.9)
    /// About 256 MB of tiles: the whole city's L13 and the streets around
    /// the camera at L16.
    static let cacheBytes = 256 * 1024 * 1024
    static let concurrentLoads = 4
}

/// Matches CityTileUniforms in CityTiles.metal.
struct CityTileUniforms {
    var modelViewProjection: simd_float4x4
    var modelView: simd_float4x4
    var colour: SIMD4<Float>
    var fade: SIMD4<Float> = .zero
    var options: SIMD4<Float> = .zero
}

/// Used on the main thread only, as the renderer is: loads finish there.
final class CityTileLayer: @unchecked Sendable {
    private(set) var tileset: CityTileset?
    private(set) var tilesetURL: URL?
    private var tiles: [String: CityTile] = [:]
    private(set) var cache = LRUCache<GPUCityTile>(capacityBytes: CityTileStyle.cacheBytes)
    private var loading = Set<String>()
    private(set) var failed = [String: String]()
    /// What the last draw selected.
    private(set) var lastSelection = TileSelection()
    /// Called on the main actor when a tile arrives, so the view redraws.
    var onTileLoaded: (() -> Void)?
    /// Where the city frame sits in the renderer's world frame: identity in
    /// city mode, the inverse of the site's sensor-to-city in a site's frame.
    var worldFromCity = matrix_identity_double4x4
    /// Reads a tile's bytes; replaced by tests.
    var read: @Sendable (URL) async throws -> Data = { try await CityTileSource.read($0) }
    var isEnabled = true
    /// Pixels of error allowed before a tile is refined.
    var maximumError: Double = 16

    private let device: MTLDevice

    init(device: MTLDevice) { self.device = device }

    /// Uses a tileset, dropping every tile of the one before.
    func setTileset(_ tileset: CityTileset?, url: URL?) {
        self.tileset = tileset
        tilesetURL = url
        tiles = tileset?.contentTiles ?? [:]
        cache.removeAll()
        loading = []
        failed = [:]
        lastSelection = TileSelection()
    }

    func isLoaded(_ id: String) -> Bool { cache.contains(id) }

    /// The tiles for a view; starts loading the ones it wants.
    func select(view: TileView) -> TileSelection {
        guard let tileset else { return TileSelection() }
        let selection = TileSelector.select(tileset, view: view) { cache.contains($0) }
        lastSelection = selection
        startLoads(selection.load)
        return selection
    }

    private func startLoads(_ wanted: [String]) {
        guard let base = tilesetURL?.deletingLastPathComponent() else { return }
        for id in wanted where loading.count < CityTileStyle.concurrentLoads {
            guard !loading.contains(id), failed[id] == nil, let tile = tiles[id],
                let uri = tile.contentURI
            else { continue }
            loading.insert(id)
            let url = base.appendingPathComponent(uri)
            let read = self.read
            Task.detached(priority: .userInitiated) { [weak self] in
                let result: Result<CityTileContent, Error>
                do { result = .success(try GLBReader.read(try await read(url))) } catch {
                    result = .failure(error)
                }
                await MainActor.run { self?.finish(id: id, origin: tile.origin, result: result) }
            }
        }
    }

    /// Awaits every load in flight, for tests.
    @MainActor func settle() async {
        while !loading.isEmpty { try? await Task.sleep(nanoseconds: 1_000_000) }
    }

    private func finish(id: String, origin: SIMD3<Double>, result: Result<CityTileContent, Error>) {
        loading.remove(id)
        guard tiles[id] != nil else { return }  // the tileset changed meanwhile
        switch result {
        case .success(let content):
            guard let gpu = GPUCityTile(id: id, origin: origin, content: content, device: device)
            else {
                failed[id] = "no buffers"
                return
            }
            cache.insert(gpu, for: id, bytes: gpu.byteCount, keep: Set(lastSelection.draw))
        case .failure(let error): failed[id] = error.localizedDescription
        }
        onTileLoaded?()
    }

    /// The loaded tiles to draw, in the order selected.
    func drawable(_ selection: TileSelection) -> [GPUCityTile] {
        selection.draw.compactMap { cache.get($0) }
    }

    /// Draws the selected tiles. `viewProjection` and `view` are the
    /// camera's, in the renderer's local frame.
    func draw(
        encoder: MTLRenderCommandEncoder, triangles: MTLRenderPipelineState,
        lines: MTLRenderPipelineState, origin: RenderOrigin, view: simd_float4x4,
        projection: simd_float4x4, selection: TileSelection, fade: SIMD4<Float>? = nil,
        fadeCity: SIMD3<Double>? = nil
    ) {
        for tile in drawable(selection) {
            let model = origin.model(worldFromCity * simd_double4x4(translation: tile.origin))
            let modelView = view * model
            var u = CityTileUniforms(
                modelViewProjection: projection * modelView, modelView: modelView,
                colour: CityTileStyle.ground)
            // Only the buildings fade near a played site: its scene draws its
            // own measured walls and roofs there.
            var buildingFade = SIMD4<Float>.zero
            if let fade, let fadeCity {
                let centre = SIMD2<Float>(
                    SIMD2(fadeCity.x - tile.origin.x, fadeCity.y - tile.origin.y))
                buildingFade = SIMD4(centre.x, centre.y, fade.z, fade.w)
            }
            func draw(
                _ layer: GPUCityTile.Layer?, _ type: MTLPrimitiveType, _ colour: SIMD4<Float>,
                flat: Bool
            ) {
                guard let layer else { return }
                u.colour = colour
                u.options.x = flat ? 1 : 0
                u.fade = flat ? buildingFade : .zero
                encoder.setVertexBuffer(layer.positions, offset: 0, index: 0)
                encoder.setVertexBytes(&u, length: MemoryLayout<CityTileUniforms>.stride, index: 1)
                encoder.setFragmentBytes(
                    &u, length: MemoryLayout<CityTileUniforms>.stride, index: 1)
                encoder.drawIndexedPrimitives(
                    type: type, indexCount: layer.indexCount, indexType: .uint32,
                    indexBuffer: layer.indices, indexBufferOffset: 0)
            }
            encoder.setRenderPipelineState(triangles)
            draw(tile.ground, .triangle, CityTileStyle.ground, flat: false)
            draw(tile.buildings, .triangle, CityTileStyle.buildings, flat: true)
            encoder.setRenderPipelineState(lines)
            draw(tile.streets, .line, CityTileStyle.streets, flat: false)
        }
    }
}
