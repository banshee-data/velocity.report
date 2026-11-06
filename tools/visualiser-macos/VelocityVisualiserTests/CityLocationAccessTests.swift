// CityLocationAccessTests.swift
// Reading the city view's locations from inside the sandbox: `~` is the
// user's own home, a chosen folder is kept as a bookmark beside its path,
// typed text that is not that path drops it, the sandbox's refusal reads
// as what to do, and the city's buildings are drawn in full near a site
// until the site draws its own surroundings.

import Foundation
import MetalKit
import Testing

@testable import VelocityVisualiser

struct CityLocationAccessTests {
    @Test func tildeIsTheUsersOwnHome() {
        #expect(CityLocationAccess.expandTilde("~", home: "/Users/a") == "/Users/a")
        #expect(
            CityLocationAccess.expandTilde("~/code/x", home: "/Users/a") == "/Users/a/code/x")
        #expect(CityLocationAccess.expandTilde("/Volumes/x", home: "/Users/a") == "/Volumes/x")
        #expect(CityLocationAccess.expandTilde("~other/x", home: "/Users/a") == "~other/x")
        // Never the app's container, which the sandbox calls home.
        #expect(!CityLocationAccess.realHome.contains("/Library/Containers/"))
        let url = CityViewSession.url(for: "~/code")
        #expect(url?.path == CityLocationAccess.realHome + "/code")
    }

    @Test func typedURLsStayURLs() {
        #expect(CityViewSession.url(for: "https://example.org/tiles")?.scheme == "https")
        #expect(CityViewSession.url(for: "http://pi.local:8080/x")?.host == "pi.local")
        #expect(CityViewSession.url(for: "/Volumes/a b")?.path == "/Volumes/a b")
    }

    @Test func permissionErrorsAreRecognised() {
        #expect(CityLocationAccess.isPermissionDenied(CocoaError(.fileReadNoPermission)))
        #expect(
            CityLocationAccess.isPermissionDenied(
                NSError(domain: NSPOSIXErrorDomain, code: Int(EPERM))))
        let wrapped = NSError(
            domain: NSCocoaErrorDomain, code: 1,
            userInfo: [NSUnderlyingErrorKey: NSError(domain: NSPOSIXErrorDomain, code: Int(EACCES))])
        #expect(CityLocationAccess.isPermissionDenied(wrapped))
        #expect(!CityLocationAccess.isPermissionDenied(CocoaError(.fileReadNoSuchFile)))
        let text = CityLocationAccess.describe(
            CocoaError(.fileReadNoPermission), at: URL(fileURLWithPath: "/Volumes/x"))
        #expect(text.contains("Choose the folder with Choose… so the app may read it"))
        #expect(text.contains("/Volumes/x"))
    }

    @Test func aMissingPathIsLeftToTheReader() throws {
        try CityLocationAccess.checkReadable(URL(fileURLWithPath: "/nowhere/at/all"))
        try CityLocationAccess.checkReadable(URL(string: "https://example.org/x")!)
        try CityLocationAccess.checkReadable(cityTilesFixture)
    }

    @Test func aChosenFolderIsKeptAsABookmark() throws {
        let suite = "city-location-\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let setting = CityLocationSetting(key: "cityTiles.location", defaults: defaults)
        let folder = FileManager.default.temporaryDirectory.appendingPathComponent(suite)
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: folder) }

        try setting.setChosen(folder)
        #expect(setting.text == folder.path)
        #expect(defaults.data(forKey: "cityTiles.location.bookmark") != nil)
        let opened = try #require(setting.open(typed: setting.text))
        #expect(opened.standardizedFileURL.path == folder.standardizedFileURL.path)
        #expect(SecurityScopedLocations.shared.root(containing: opened.appendingPathComponent("a")) != nil)

        // Submitting the path shown keeps the bookmark; other text drops it.
        setting.setTyped(folder.path)
        #expect(defaults.data(forKey: "cityTiles.location.bookmark") != nil)
        setting.setTyped("/Volumes/elsewhere")
        #expect(defaults.data(forKey: "cityTiles.location.bookmark") == nil)
        #expect(setting.open(typed: setting.text)?.path == "/Volumes/elsewhere")
        #expect(SecurityScopedLocations.shared.root(containing: folder) == nil)
    }

    @MainActor @Test func theSessionReadsAChosenTilesFolder() async throws {
        let suite = "city-location-\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        defer {
            SecurityScopedLocations.shared.hold(nil, for: CityViewSession.tilesLocationKey)
            defaults.removePersistentDomain(forName: suite)
        }
        let city = CityViewSession(defaults: defaults)
        await city.chooseTilesLocation(cityTilesFixture)
        #expect(city.tileset != nil)
        #expect(city.tilesLocation == cityTilesFixture.path)
        // Read again at the next launch, from the bookmark.
        let again = CityViewSession(defaults: defaults)
        await again.loadTileset()
        #expect(again.tileset != nil)
    }
}

@MainActor struct CityDrawnFirstTests {
    @Test func buildingsFadeOnlyOnceTheSiteDrawsItsOwn() throws {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let view = MTKView(frame: CGRect(x: 0, y: 0, width: 800, height: 600))
        view.device = device
        let renderer = try #require(MetalRenderer(metalView: view))
        renderer.cityFade = (SIMD3(10, 20, 0), 60, 90)
        // A cold site: nothing of its own yet, so the city is drawn in full.
        #expect(!renderer.siteDrawsOwnSurroundings)
        #expect(renderer.cityBuildingFade == nil)
        var reference = ReferenceOverlayVertices()
        reference.buildingLines = [Float](repeating: 0, count: 14)
        renderer.setReferenceOverlay(reference)
        #expect(renderer.siteDrawsOwnSurroundings)
        #expect(renderer.cityBuildingFade?.centre == SIMD3(10, 20, 0))
        renderer.setReferenceOverlay(nil)
        #expect(renderer.cityBuildingFade == nil)
    }
}
