// SiteContext.swift
// Everything the visualiser holds for one site, in one object
// (lidar-city-view-plan Item 5). Switching site replaces this object: the
// tracks, the background cache, the reference geometry, the street marks
// and the annotation frame of the site being left go with it, and nothing
// of one site's is kept to be shown at another.
//
// Each site is recorded on its own. Track ids are namespaced by site
// (`namespaced(_:)`) so that no code can join one site's road users to
// another's by accident.

import Foundation

/// The per-track state the stream accumulates at one site.
struct SiteTrackState {
    /// Ring buffer of recent samples per track (keyed by trackID).
    var history: [String: [AppState.TrackSample]] = [:]
    /// Persistent all-time max speed per track (survives ring-buffer eviction).
    var maxSpeed: [String: Float] = [:]
    /// Persistent all-time max hits per track across the current replay.
    var maxHits: [String: Int] = [:]
    /// Every track seen at this site in the current replay, kept after it
    /// leaves the field of view.
    var allSeen: [String: Track] = [:]
    /// Track IDs present in the most recent frame.
    var inView: Set<String> = []

    var isEmpty: Bool {
        history.isEmpty && maxSpeed.isEmpty && maxHits.isEmpty && allSeen.isEmpty && inView.isEmpty
    }

    /// Drops what the stream accumulated.
    mutating func clearStream() {
        history = [:]
        maxSpeed = [:]
        maxHits = [:]
        allSeen = [:]
        inView = []
    }
}

/// Where a site's frames come from.
enum SiteSource: Equatable, Sendable {
    /// Nothing chosen: whatever the connected server streams.
    case unchosen
    /// The connected server is this site's live sensor.
    case live(sensorID: String)
    /// A replay of the site's captures on the connected server.
    case replay(clipID: String, sensorID: String, files: [String])
    /// The connected server holds nothing for this site.
    case nothingToPlay
}

/// One site's state. A new one is made for every switch, so returning to a
/// site starts it clean rather than from what was left there.
@MainActor final class SiteContext: Identifiable {
    /// The site from `city/sites.json`; nil before one is chosen, when the
    /// visualiser shows whatever its server streams, as it always has.
    nonisolated let site: CitySite?
    nonisolated let source: SiteSource
    /// Distinguishes two visits to the same site.
    let id = UUID()

    var tracks = SiteTrackState()

    init(site: CitySite? = nil, source: SiteSource = .unchosen) {
        self.site = site
        self.source = source
    }

    /// The site's id, or "" before one is chosen.
    nonisolated var siteID: String { site?.id ?? "" }

    /// A track id that cannot collide with another site's.
    nonisolated func namespaced(_ trackID: String) -> String {
        site.map { "\($0.id)/\(trackID)" } ?? trackID
    }

    /// The site id the annotation pane offers for its map-marks line.
    var annotationSiteID: String { site?.id ?? "" }
}
