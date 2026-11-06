// CityServerTilesTests.swift
// The city view needs no setup: the tiles come from the connected Gerardus
// server's advertised path, and the site index is the one bundled at build
// time. A location set in Settings overrides either; empty fields, or an
// older server without them, mean no tiles.

import Foundation
import SwiftProtobuf
import Testing

@testable import VelocityVisualiser

@MainActor struct CityServerTilesTests {
    private let address = GerardusAddress(host: "arrow-worker", port: 50061)

    private func info(path: String, epoch: String) -> Gerardus_Align_V1alpha1_GetServerInfoResponse {
        var info = Gerardus_Align_V1alpha1_GetServerInfoResponse()
        info.cityTilesPath = path
        info.cityTilesEpoch = epoch
        return info
    }

    /// A session whose tile reads come from the fixture, recording each URL.
    private func session(reads: CityTestBox<[URL]>) throws -> CityViewSession {
        let city = CityViewSession(defaults: nil, bundledIndex: nil)
        let data = try Data(contentsOf: cityTilesFixture.appendingPathComponent("tileset.json"))
        city.readTiles = { url in
            await MainActor.run { reads.value.append(url) }
            return data
        }
        return city
    }

    @Test func theServersPathIsOnItsGRPCHostAndPort() {
        #expect(
            CityViewSession.serverTilesURL(address: address, path: "/city-tiles/20261105T200804Z/")?
                .absoluteString == "http://arrow-worker:50061/city-tiles/20261105T200804Z/")
        #expect(
            CityViewSession.serverTilesURL(address: address, path: "city-tiles/current")?
                .absoluteString == "http://arrow-worker:50061/city-tiles/current/")
        #expect(CityViewSession.serverTilesURL(address: address, path: "") == nil)
        #expect(CityViewSession.serverTilesURL(address: nil, path: "/city-tiles/x/") == nil)
        let tileset = try? CityTileSource.tilesetURL(
            for: #require(CityViewSession.serverTilesURL(address: address, path: "/city-tiles/e/")))
        #expect(tileset?.absoluteString == "http://arrow-worker:50061/city-tiles/e/tileset.json")
    }

    @Test func withoutAnOverrideTheServersTilesAreRead() async throws {
        let reads = CityTestBox<[URL]>([])
        let city = try session(reads: reads)
        await city.useServer(address, info: info(path: "/city-tiles/E1/", epoch: "E1"))
        #expect(city.tileset != nil)
        #expect(city.usesServerTiles)
        #expect(reads.value.map(\.absoluteString) == ["http://arrow-worker:50061/city-tiles/E1/tileset.json"])
        #expect(city.tilesStatus.contains("from Gerardus at arrow-worker:50061"))
        // The same answer again reads nothing; a new epoch reads again.
        await city.useServer(address, info: info(path: "/city-tiles/E1/", epoch: "E1"))
        #expect(reads.value.count == 1)
        await city.useServer(address, info: info(path: "/city-tiles/E2/", epoch: "E2"))
        #expect(reads.value.last?.absoluteString == "http://arrow-worker:50061/city-tiles/E2/tileset.json")
        // Losing the server loses its tiles.
        await city.useServer(nil, info: nil)
        #expect(city.tileset == nil)
    }

    @Test func anOverrideWins() async throws {
        let reads = CityTestBox<[URL]>([])
        let city = try session(reads: reads)
        await city.useTilesLocation("https://example.org/tiles/")
        await city.useServer(address, info: info(path: "/city-tiles/E1/", epoch: "E1"))
        #expect(reads.value.map(\.absoluteString) == ["https://example.org/tiles/tileset.json"])
        #expect(!city.usesServerTiles)
        // Clearing the override goes back to the server's.
        await city.useTilesLocation("")
        #expect(reads.value.last?.absoluteString == "http://arrow-worker:50061/city-tiles/E1/tileset.json")
        #expect(city.usesServerTiles)
    }

    @Test func emptyFieldsAndAnOlderServerMeanNoTiles() async throws {
        let reads = CityTestBox<[URL]>([])
        let city = try session(reads: reads)
        await city.useServer(address, info: info(path: "", epoch: ""))
        #expect(city.tileset == nil && reads.value.isEmpty)
        // An older server's response has neither field: decoded, both empty.
        var older = Gerardus_Align_V1alpha1_GetServerInfoResponse()
        older.serverVersion = "0.1.0"
        older.streetUseRulesVersion = 1
        let decoded = try Gerardus_Align_V1alpha1_GetServerInfoResponse(
            serializedBytes: try older.serializedData())
        #expect(decoded.cityTilesPath.isEmpty && decoded.cityTilesEpoch.isEmpty)
        await city.useServer(address, info: decoded)
        #expect(city.tileset == nil && reads.value.isEmpty)
        #expect(city.tilesStatus.hasPrefix("No tiles"))
    }

    @Test func theBundledIndexIsUsedWithoutALocation() async throws {
        let fixture = cityViewFixtures.appendingPathComponent("sites.json")
        let city = CityViewSession(defaults: nil, bundledIndex: fixture)
        await city.loadIndex()
        #expect(city.sites.count == 3)
        #expect(city.usesBundledIndex)
        await city.useSitesLocation("/nowhere/at/all")
        #expect(city.sites.isEmpty && !city.usesBundledIndex)
        await city.useSitesLocation("")
        #expect(city.sites.count == 3 && city.usesBundledIndex)
    }

    @Test func theAppBundlesThePublishedIndex() throws {
        let url = try #require(CityViewSession.bundledIndexURL)
        let index = try CitySiteIndex.decode(Data(contentsOf: url))
        #expect(!index.sites.isEmpty)
    }
}

final class CityTestBox<T>: @unchecked Sendable {
    var value: T
    init(_ value: T) { self.value = value }
}
