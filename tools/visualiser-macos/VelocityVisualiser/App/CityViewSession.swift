// CityViewSession.swift
// The city view's own state (lidar-city-view-plan Items 5–8): where the
// site index is read from, the sites it lists, and the switch between them.
// Kept apart from AppState, which publishes on every frame, so the site
// picker and Settings redraw only when the city view changes.

import Combine
import Foundation

@available(macOS 15.0, *) @MainActor final class CityViewSession: ObservableObject {
    /// Where `city/sites.json` is read from: a directory holding `city/`,
    /// the file itself, or an http(s) URL. Kept beside
    /// `alignment.serviceAddress`.
    static let sitesLocationKey = "citySites.location"

    /// Where Gerardus's city tiles are read from: an epoch directory, the
    /// `city-tiles` directory, a `tileset.json`, or an http(s) URL.
    static let tilesLocationKey = "cityTiles.location"

    @Published private(set) var tilesLocation: String
    @Published private(set) var tileset: CityTileset?
    @Published private(set) var tilesetURL: URL?
    @Published private(set) var tilesError: String?
    /// Called with each tileset read, for the renderer.
    var onTileset: ((CityTileset?, URL?) -> Void)?

    /// City mode: the camera over the city tiles, with the sites as markers.
    @Published private(set) var isCityMode = false
    /// The markers and street names over the view, from the renderer.
    @Published var overlay = CityOverlay()
    /// Turns city mode on or off in the renderer; AppState's.
    var onCityMode: ((Bool) -> Void)?
    /// Each nearby site's own clock, for its marker: its recording's time
    /// of day at the frame it shows.
    @Published var siteClocks: [String: String] = [:]

    /// The index file the sites were read from, for finding their scenes.
    var indexURL: URL? { sitesURL.map(CitySiteIndex.indexURL(for:)) }
    /// The location the index was last read from: the chosen folder, or
    /// the typed path or URL.
    private(set) var sitesURL: URL?
    private var tilesSetting: CityLocationSetting {
        CityLocationSetting(key: Self.tilesLocationKey, defaults: defaults)
    }
    private var sitesSetting: CityLocationSetting {
        CityLocationSetting(key: Self.sitesLocationKey, defaults: defaults)
    }

    @Published private(set) var sitesLocation: String
    @Published private(set) var index: CitySiteIndex?
    @Published private(set) var loadError: String?
    @Published private(set) var isLoading = false
    /// The site being switched to while its replay starts.
    @Published private(set) var switchingTo: String?
    @Published private(set) var switchError: String?

    private let defaults: UserDefaults?
    /// Starts a site's frames; AppState's `switchSite`.
    var switcher: ((CitySite) async throws -> SiteSource)?
    /// The site the main view is at now.
    var currentSiteID: () -> String? = { nil }
    /// Called with each index read, for the alignment session's lookups.
    var onIndex: ((CitySiteIndex?) -> Void)?

    /// The site index bundled into the app at build time, from
    /// `public_html/src/city/sites.json`: used when no location is set.
    nonisolated static var bundledIndexURL: URL? {
        Bundle.main.url(forResource: "city-sites", withExtension: "json")
    }
    private let bundledIndex: URL?
    /// Reads the tileset's bytes; replaced by tests.
    var readTiles: @Sendable (URL) async throws -> Data = { try await CityTileSource.read($0) }

    /// The tiles the connected Gerardus server serves, used when no tiles
    /// location is set: `http://<host:port><city_tiles_path>`.
    @Published private(set) var serverTilesURL: URL?
    @Published private(set) var serverTilesEpoch: String?
    /// The server the tiles are from, for Settings.
    private(set) var serverTilesHost: String?

    /// Whether the index read is the app's own bundled copy.
    var usesBundledIndex: Bool {
        sitesLocation.isEmpty && sitesURL != nil && sitesURL == bundledIndex
    }
    /// Whether the tiles read are the server's.
    var usesServerTiles: Bool { tilesLocation.isEmpty && serverTilesURL != nil }

    init(defaults: UserDefaults?, bundledIndex: URL? = CityViewSession.bundledIndexURL) {
        self.defaults = defaults
        self.bundledIndex = bundledIndex
        sitesLocation = defaults?.string(forKey: Self.sitesLocationKey) ?? ""
        tilesLocation = defaults?.string(forKey: Self.tilesLocationKey) ?? ""
    }

    /// Saves a typed tiles location and reads its tileset.
    func useTilesLocation(_ text: String) async {
        tilesSetting.setTyped(text)
        tilesLocation = text.trimmingCharacters(in: .whitespacesAndNewlines)
        await loadTileset()
    }

    /// Saves a folder or file chosen in the open panel, with the bookmark
    /// the sandbox needs to read it, and reads its tileset.
    func chooseTilesLocation(_ url: URL) async {
        do { try tilesSetting.setChosen(url) } catch {
            setTileset(nil, url: nil, error: error.localizedDescription)
            return
        }
        tilesLocation = url.path
        await loadTileset()
    }

    /// The HTTP location a Gerardus server advertises for its tiles: on the
    /// gRPC host and port. Empty fields (no baked tiles, or an older server
    /// without them) mean none.
    nonisolated static func serverTilesURL(address: GerardusAddress?, path: String) -> URL? {
        let trimmed = path.trimmingCharacters(in: .whitespaces)
        guard let address, !trimmed.isEmpty else { return nil }
        var directory = trimmed.hasPrefix("/") ? trimmed : "/" + trimmed
        if !directory.hasSuffix("/") { directory += "/" }
        return URL(string: "http://\(address.description)\(directory)")
    }

    /// What the connected Gerardus server says about its tiles; nil when
    /// there is no server. Reads them again when the server or the epoch
    /// changes, unless a location set here overrides them.
    func useServer(
        _ address: GerardusAddress?, info: Gerardus_Align_V1alpha1_GetServerInfoResponse?
    ) async {
        let url = info.flatMap { Self.serverTilesURL(address: address, path: $0.cityTilesPath) }
        let epoch = info?.cityTilesEpoch.isEmpty == false ? info?.cityTilesEpoch : nil
        guard url != serverTilesURL || epoch != serverTilesEpoch else { return }
        serverTilesURL = url
        serverTilesEpoch = epoch
        serverTilesHost = url == nil ? nil : address?.description
        if tilesLocation.isEmpty { await loadTileset() }
    }

    /// Reads the tileset from the location set here or, without one, from
    /// the connected Gerardus server; without either, no tiles.
    func loadTileset() async {
        guard let location = tilesSetting.open(typed: tilesLocation) ?? serverTilesURL else {
            setTileset(nil, url: nil, error: nil)
            return
        }
        do {
            try CityLocationAccess.checkReadable(location)
            let url = try CityTileSource.tilesetURL(for: location)
            let read = try CityTileset.decode(await readTiles(url))
            setTileset(read, url: url, error: nil)
        } catch {
            setTileset(nil, url: nil, error: CityLocationAccess.describe(error, at: location))
        }
    }

    func setTileset(_ next: CityTileset?, url: URL?, error: String?) {
        tileset = next
        tilesetURL = url
        tilesError = error
        onTileset?(next, url)
    }

    /// What Settings says about the tiles.
    var tilesStatus: String {
        if let tilesError { return tilesError }
        guard let tileset else {
            return tilesLocation.isEmpty
                ? "No tiles: Gerardus is not connected, or serves none" : "No tiles read"
        }
        let tiles = tileset.root.all.filter(\.hasContent)
        let epoch = (tileset.tilesetVersion ?? serverTilesEpoch).map { " · epoch \($0)" } ?? ""
        let from = usesServerTiles ? " · from Gerardus at \(serverTilesHost ?? "the server")" : ""
        return "\(tiles.count) tiles\(epoch)\(from)"
    }

    var sites: [CitySite] { index?.sites ?? [] }

    /// City mode needs tiles to draw or sites to mark.
    var canShowCity: Bool { tileset != nil || !sites.isEmpty }

    func setCityMode(_ on: Bool) {
        let next = on && canShowCity
        guard next != isCityMode else { return }
        isCityMode = next
        if !next {
            overlay = CityOverlay()
            siteClocks = [:]
        }
        onCityMode?(next)
    }

    /// A typed path or URL as a location: `~` is the user's real home, not
    /// the sandbox's container; a bare path is a file URL.
    nonisolated static func url(for text: String) -> URL? { CityLocationSetting.url(for: text) }

    /// Saves a typed location and reads the index from it.
    func useSitesLocation(_ text: String) async {
        sitesSetting.setTyped(text)
        sitesLocation = text.trimmingCharacters(in: .whitespacesAndNewlines)
        await loadIndex()
    }

    /// Saves a folder or file chosen in the open panel, with its bookmark,
    /// and reads the index from it.
    func chooseSitesLocation(_ url: URL) async {
        do { try sitesSetting.setChosen(url) } catch {
            setIndex(nil, error: error.localizedDescription)
            return
        }
        sitesLocation = url.path
        await loadIndex()
    }

    /// Reads the index from the saved location; without one there is no
    /// index and the visualiser works as it did before the city view.
    func loadIndex() async {
        sitesURL = sitesSetting.open(typed: sitesLocation) ?? bundledIndex
        guard let url = sitesURL else {
            setIndex(nil, error: nil)
            return
        }
        isLoading = true
        do {
            try CityLocationAccess.checkReadable(url)
            let read = try await CitySiteIndex.load(from: url)
            setIndex(read, error: nil)
        } catch let error as CityLocationAccess.AccessError {
            setIndex(nil, error: error.localizedDescription)
        } catch {
            let shown = url.isFileURL ? url.path : url.absoluteString
            setIndex(
                nil,
                error: "No site index at \(shown): \(CityLocationAccess.describe(error, at: url))")
        }
        isLoading = false
    }

    /// Sets the index directly, for tests and for a fixture.
    func setIndex(_ next: CitySiteIndex?, error: String?) {
        index = next
        loadError = error
        onIndex?(next)
    }

    /// Switches the main view to `site`. One switch at a time.
    func select(_ site: CitySite) async {
        guard switchingTo == nil, let switcher else { return }
        switchingTo = site.id
        switchError = nil
        do { _ = try await switcher(site) } catch { switchError = error.localizedDescription }
        switchingTo = nil
    }

    /// The next or previous site after the current one.
    func step(_ by: Int) async {
        guard let next = index?.neighbour(of: currentSiteID(), step: by) else { return }
        await select(next)
    }

    /// The live server typed for a site, for Settings.
    func liveServerText(for siteID: String) -> String {
        defaults?.string(forKey: CityLiveServers.keyPrefix + siteID) ?? ""
    }

    func setLiveServer(_ text: String, for siteID: String) {
        objectWillChange.send()
        CityLiveServers.set(text, for: siteID, in: defaults)
    }
}
