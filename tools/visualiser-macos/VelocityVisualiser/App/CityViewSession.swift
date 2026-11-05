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

    init(defaults: UserDefaults?) {
        self.defaults = defaults
        sitesLocation = defaults?.string(forKey: Self.sitesLocationKey) ?? ""
    }

    var sites: [CitySite] { index?.sites ?? [] }

    /// A typed path or URL as a location: `~` expanded, a bare path a file URL.
    nonisolated static func url(for text: String) -> URL? {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        if let url = URL(string: trimmed), let scheme = url.scheme?.lowercased(),
            ["http", "https", "file"].contains(scheme)
        {
            return url
        }
        return URL(fileURLWithPath: (trimmed as NSString).expandingTildeInPath)
    }

    /// Saves a new location and reads the index from it.
    func useSitesLocation(_ text: String) async {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        sitesLocation = trimmed
        if trimmed.isEmpty {
            defaults?.removeObject(forKey: Self.sitesLocationKey)
        } else {
            defaults?.set(trimmed, forKey: Self.sitesLocationKey)
        }
        await loadIndex()
    }

    /// Reads the index from the saved location; without one there is no
    /// index and the visualiser works as it did before the city view.
    func loadIndex() async {
        guard let url = Self.url(for: sitesLocation) else {
            setIndex(nil, error: nil)
            return
        }
        isLoading = true
        do {
            let read = try await CitySiteIndex.load(from: url)
            setIndex(read, error: nil)
        } catch {
            setIndex(
                nil, error: "No site index at \(url.absoluteString): \(error.localizedDescription)")
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
