// NearbySites.swift
// The sites around the one being played, in city mode (lidar-city-view-plan
// Item 8). The connected server plays one replay at a time, so the others
// in view play from their published scene exports, read from files, at the
// street-range level of detail: boxes and headings. A site with its own Pi
// in the field and a live server kept for it is streamed live instead, one
// stream per Pi, each in its own SiteContext.
//
// Each site plays its own recording on its own clock, labelled with its own
// date and time of day, inside its own coverage, coloured by class. Track
// ids are namespaced by site, so nothing can join two sites' road users.

import Foundation
import simd

/// What one nearby site draws now.
struct NearbySiteFrame: Equatable, Sendable {
    var siteID: String
    var worldFromSensor: simd_double4x4
    /// Ids namespaced by site ("columbus-broadway/7").
    var boxes: [SceneBox]
    /// The recording's date and time of day at the frame shown.
    var clock: String?
    var live: Bool
}

/// Where a published scene's assets are, from the index's location.
enum SceneAssets {
    /// The scene's `assets/manifest.json`. The index names the assets by
    /// their web path (`/surveys/<id>/assets/`), relative to the site root
    /// that holds `city/sites.json`; a source checkout keeps them under
    /// `scenes/<id>/assets/` instead, which is tried too.
    static func manifestURL(
        for site: CitySite, indexURL: URL, fileManager: FileManager = .default
    ) -> URL? {
        let root = indexURL.deletingLastPathComponent().deletingLastPathComponent()
        var candidates = [URL]()
        if let assets = site.sceneAssets {
            let path = assets.hasPrefix("/") ? String(assets.dropFirst()) : assets
            candidates.append(
                root.appendingPathComponent(path).appendingPathComponent("manifest.json"))
        }
        candidates.append(
            root.appendingPathComponent("scenes").appendingPathComponent(site.id)
                .appendingPathComponent("assets/manifest.json"))
        guard root.isFileURL else { return site.sceneAssets == nil ? nil : candidates.first }
        return candidates.first { fileManager.fileExists(atPath: $0.path) }
    }
}

@MainActor final class NearbySites {
    /// Up to this many sites play at street range besides the focused one.
    static let streetRangeLimit = 8
    /// How far from the camera's target a site plays, metres.
    static let streetRangeM: Double = 1_500

    /// Which sites to play: placed, not the focused one, within range of
    /// the camera's target in the city, nearest first, at most the limit.
    nonisolated static func choose(
        _ sites: [CitySite], focusedID: String?, near target: SIMD3<Double>,
        limit: Int = streetRangeLimit, range: Double = streetRangeM
    ) -> [CitySite] {
        sites.compactMap { site -> (CitySite, Double)? in
            guard site.id != focusedID, let at = site.sensorCityPosition else { return nil }
            let d = simd_length(SIMD2(at.x - target.x, at.y - target.y))
            return d <= range ? (site, d) : nil
        }.sorted { $0.1 < $1.1 }.prefix(limit).map(\.0)
    }

    /// One site playing from its scene export.
    private final class ScenePlayer {
        let site: CitySite
        var session: SceneSession?
        var clock: SceneClock?
        var failed: String?
        var loading: Task<Void, Never>?
        var latest: SceneFrame?
        var latestPart = 0

        init(site: CitySite) { self.site = site }
    }

    private var players: [String: ScenePlayer] = [:]
    private var live: [String: LiveSiteStream] = [:]
    /// The index's location, to find each scene's files.
    var indexURL: () -> URL? = { nil }
    var read: @Sendable (URL) async throws -> Data = { try await StaticFiles.read($0) }
    /// The live server kept for a site, if any.
    var liveServer: (String) -> VelocityReportServer? = { _ in nil }
    var makeClient: (String) -> VisualiserClient = { VisualiserClient(address: $0) }
    var now: () -> Double = { Date().timeIntervalSinceReferenceDate }

    var playingIDs: [String] { (Array(players.keys) + Array(live.keys)).sorted() }
    func failure(_ siteID: String) -> String? { players[siteID]?.failed }

    /// Starts the sites that should play and stops the ones that should not.
    func update(sites: [CitySite], focusedID: String?, near target: SIMD3<Double>) {
        let wanted = Self.choose(sites, focusedID: focusedID, near: target)
        let ids = Set(wanted.map(\.id))
        for id in players.keys where !ids.contains(id) {
            players[id]?.loading?.cancel()
            players[id] = nil
        }
        for id in live.keys where !ids.contains(id) {
            live[id]?.stop()
            live[id] = nil
        }
        for site in wanted where players[site.id] == nil && live[site.id] == nil {
            if let server = liveServer(site.id) {
                let stream = LiveSiteStream(site: site, client: makeClient(server.streamAddress))
                stream.start()
                live[site.id] = stream
            } else {
                start(site)
            }
        }
    }

    func stopAll() { update(sites: [], focusedID: nil, near: .zero) }

    private func start(_ site: CitySite) {
        let player = ScenePlayer(site: site)
        players[site.id] = player
        guard let index = indexURL(), let url = SceneAssets.manifestURL(for: site, indexURL: index)
        else {
            player.failed = "no published scene"
            return
        }
        let read = self.read
        player.loading = Task { [weak self, weak player] in
            do {
                let session = try await SceneSession.open(manifest: url, read: read)
                guard let player, let self else { return }
                player.session = session
                // Its own clock, from now.
                player.clock = SceneClock(startedAt: self.now(), duration: session.duration)
            } catch { player?.failed = error.localizedDescription }
        }
    }

    /// Awaits every scene opening, for tests.
    func settle() async { for player in players.values { await player.loading?.value } }

    /// Each playing site's frame now, on its own clock.
    func frames() async -> [NearbySiteFrame] {
        var out = [NearbySiteFrame]()
        let t = now()
        for (id, player) in players.sorted(by: { $0.key < $1.key }) {
            guard let session = player.session, let clock = player.clock,
                let worldFromSensor = player.site.sensorToCity
            else { continue }
            let seconds = clock.sceneSeconds(at: t)
            if let shown = try? await session.frame(at: seconds) {
                player.latest = shown.frame
                player.latestPart = shown.part
            }
            guard let frame = player.latest else { continue }
            out.append(
                NearbySiteFrame(
                    siteID: id, worldFromSensor: worldFromSensor,
                    boxes: frame.boxes.map { Self.namespaced($0, site: id) },
                    clock: CityOverlayLayout.clock(
                        timestampNanos: session.captureNanos(at: seconds)), live: false))
        }
        for (id, stream) in live.sorted(by: { $0.key < $1.key }) {
            guard let worldFromSensor = stream.site.sensorToCity else { continue }
            out.append(
                NearbySiteFrame(
                    siteID: id, worldFromSensor: worldFromSensor,
                    boxes: stream.boxes.map { Self.namespaced($0, site: id) },
                    clock: CityOverlayLayout.clock(timestampNanos: stream.timestampNanos),
                    live: true))
        }
        return out
    }

    nonisolated static func namespaced(_ box: SceneBox, site: String) -> SceneBox {
        var b = box
        b.id = "\(site)/\(box.id)"
        return b
    }
}

/// One live Pi's stream, in its own SiteContext.
@MainActor final class LiveSiteStream: VisualiserClientDelegate {
    let site: CitySite
    let context: SiteContext
    private let client: VisualiserClient
    private(set) var boxes: [SceneBox] = []
    private(set) var timestampNanos: Int64 = 0

    init(site: CitySite, client: VisualiserClient) {
        self.site = site
        self.context = SiteContext(site: site, source: .live(sensorID: site.id))
        self.client = client
    }

    func start() {
        client.delegate = self
        Task { try? await client.connect() }
    }

    func stop() {
        client.delegate = nil
        client.disconnect()
    }

    /// A frame from the Pi: its tracks become boxes, kept in its own context.
    func receive(_ frame: FrameBundle) {
        guard let tracks = frame.tracks?.tracks else { return }
        timestampNanos = frame.timestampNanos
        context.tracks.inView = Set(tracks.map(\.trackID))
        for track in tracks { context.tracks.allSeen[track.trackID] = track }
        boxes = tracks.map { t in
            SceneBox(
                id: t.trackID, x: t.boxCentreX, y: t.boxCentreY, z: t.z,
                length: t.bboxLength > 0 ? t.bboxLength : 1,
                width: t.bboxWidth > 0 ? t.bboxWidth : 1,
                height: t.bboxHeight > 0 ? t.bboxHeight : 1,
                boxHeading: t.bboxHeadingRad != 0 ? t.bboxHeadingRad : t.headingRad,
                heading: t.headingRad, speed: t.speedMps, classLabel: t.classLabel)
        }
    }

    nonisolated func clientDidConnect(_ client: VisualiserClient) {}
    nonisolated func clientDidDisconnect(_ client: VisualiserClient, error: Error?) {}
    nonisolated func clientDidFinishStream(_ client: VisualiserClient) {}
    nonisolated func client(_ client: VisualiserClient, didReceiveFrame frame: FrameBundle) {
        Task { @MainActor in self.receive(frame) }
    }
}
