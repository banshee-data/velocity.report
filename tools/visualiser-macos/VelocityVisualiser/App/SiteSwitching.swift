// SiteSwitching.swift
// Moving the visualiser from one site to another (lidar-city-view-plan
// Item 5), with calls velocity.report already serves:
//
//   GET  /api/lidar/clip-locations     what the connected server can replay
//   GET  /api/lidar/clips/{clip_id}    a clip's capture files
//   POST /api/lidar/pcap/start         replay a site's captures
//
// A site whose live sensor the operator has named (Settings ▸ City view,
// one velocity.report server per live Pi) is attached to instead: that
// server is the site's own and streams it live.
//
// A switch stops the current stream's site, makes a new SiteContext, asks
// each holder of per-site state to leave the old site (renderer, alignment,
// street painter through the alignment's reference, annotation through the
// context's site id), then starts the new site's frames.

import Foundation

/// The HTTP calls a switch makes. A protocol so tests replace the server.
protocol SiteReplayService: Sendable {
    /// Every located clip the connected server holds.
    func locatedSites() async throws -> [AlignmentSite]
    /// A clip's capture files, relative to the server's PCAP root.
    func captureFiles(clipID: String) async throws -> [String]
    /// Starts a replay of `files`, in order, as one recording.
    func startReplay(sensorID: String, files: [String]) async throws
}

struct VelocityReportSiteReplayService: SiteReplayService {
    let lookup: AlignmentSiteLookupClient
    private let session: URLSession

    init(baseURL: URL? = nil, session: URLSession = APISession.shared) {
        self.lookup = AlignmentSiteLookupClient(baseURL: baseURL, session: session)
        self.session = session
    }

    func locatedSites() async throws -> [AlignmentSite] { try await lookup.locatedSites() }

    func captureFiles(clipID: String) async throws -> [String] {
        try await lookup.captureFiles(clipID: clipID)
    }

    func startReplay(sensorID: String, files: [String]) async throws {
        var components = URLComponents(
            url: lookup.baseURL.appendingPathComponent("api/lidar/pcap/start"),
            resolvingAgainstBaseURL: false)
        components?.queryItems = [URLQueryItem(name: "sensor_id", value: sensorID)]
        guard let url = components?.url else { throw URLError(.badURL) }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let body: [String: Any] = files.count == 1 ? ["pcap_file": files[0]] : ["pcap_files": files]
        request.httpBody = try JSONSerialization.data(withJSONObject: body)
        request.timeoutInterval = 30
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            let text = String(data: data, encoding: .utf8) ?? ""
            throw SiteSwitchError.replayRefused(text.isEmpty ? "no reason given" : text)
        }
    }
}

enum SiteSwitchError: Error, LocalizedError, Equatable {
    case replayRefused(String)
    case nothingToPlay(String)

    var errorDescription: String? {
        switch self {
        case .replayRefused(let why): return "The server would not replay the site: \(why)"
        case .nothingToPlay(let title): return "The connected server holds no capture of \(title)."
        }
    }
}

/// Which of the server's clips is the city site's, and what to replay.
enum SiteSourceResolver {
    /// The clip whose captures include one of the site's, else the only
    /// clip at the site's L16 cell. `filesByClip` holds each clip's files.
    static func clip(
        for site: CitySite, clips: [AlignmentSite], filesByClip: [String: [String]]
    ) -> AlignmentSite? {
        let names = Set(site.captures.map { ($0 as NSString).lastPathComponent })
        if !names.isEmpty,
            let byFile = clips.first(where: { clip in
                (filesByClip[clip.clipID] ?? []).contains {
                    names.contains(($0 as NSString).lastPathComponent)
                }
            })
        {
            return byFile
        }
        guard !site.cells.l16.isEmpty else { return nil }
        // Only when one clip is there: of several, any could be the wrong
        // recording.
        let inCell = clips.filter { $0.l16Token == site.cells.l16 }
        return inCell.count == 1 ? inCell[0] : nil
    }

    /// The files to replay: the clip's captures the index names, in the
    /// clip's order, or all of them when the index names none of them.
    static func files(for site: CitySite, clipFiles: [String]) -> [String] {
        let names = Set(site.captures.map { ($0 as NSString).lastPathComponent })
        let named = clipFiles.filter { names.contains(($0 as NSString).lastPathComponent) }
        return named.isEmpty ? clipFiles : named
    }
}

/// The operator's live servers, one per site that has a Pi in the field.
/// Local preferences, never published: a live address is not public data.
enum CityLiveServers {
    static let keyPrefix = "cityView.liveServer."

    static func server(for siteID: String, in defaults: UserDefaults?) -> VelocityReportServer? {
        guard let text = defaults?.string(forKey: keyPrefix + siteID), !text.isEmpty else {
            return nil
        }
        return try? VelocityReportServer.parse(url: text)
    }

    static func set(_ text: String, for siteID: String, in defaults: UserDefaults?) {
        let trimmed = text.trimmingCharacters(in: .whitespaces)
        if trimmed.isEmpty {
            defaults?.removeObject(forKey: keyPrefix + siteID)
        } else {
            defaults?.set(trimmed, forKey: keyPrefix + siteID)
        }
    }
}

extension AppState {
    /// Leaves the current site for `site` and starts its frames: a live
    /// server named for it, else a replay of its captures on the connected
    /// server. Returns what it started, or throws why nothing could be.
    @discardableResult func switchSite(
        to site: CitySite, service: SiteReplayService? = nil, defaults: UserDefaults? = nil
    ) async throws -> SiteSource {
        let defaults = defaults ?? siteDefaults
        if let live = CityLiveServers.server(for: site.id, in: defaults) {
            let source = SiteSource.live(sensorID: site.id)
            enterSite(SiteContext(site: site, source: source))
            if !useVelocityReportServer(live) { restartGRPCStream() }
            return source
        }

        let service = service ?? siteReplayServiceOverride ?? VelocityReportSiteReplayService()
        let clips = try await service.locatedSites()
        var files = [String: [String]]()
        for clip in clips where clip.l16Token == site.cells.l16 || !site.captures.isEmpty {
            files[clip.clipID] = (try? await service.captureFiles(clipID: clip.clipID)) ?? []
        }
        guard let clip = SiteSourceResolver.clip(for: site, clips: clips, filesByClip: files) else {
            enterSite(SiteContext(site: site, source: .nothingToPlay))
            throw SiteSwitchError.nothingToPlay(site.title)
        }
        var clipFiles = files[clip.clipID] ?? []
        if clipFiles.isEmpty {
            clipFiles = (try? await service.captureFiles(clipID: clip.clipID)) ?? []
        }
        let replay = SiteSourceResolver.files(for: site, clipFiles: clipFiles)
        guard !replay.isEmpty else {
            enterSite(SiteContext(site: site, source: .nothingToPlay))
            throw SiteSwitchError.nothingToPlay(site.title)
        }
        let source = SiteSource.replay(clipID: clip.clipID, sensorID: clip.sensorID, files: replay)
        // Started first: a refused replay leaves the current site as it was.
        try await service.startReplay(sensorID: clip.sensorID, files: replay)
        enterSite(SiteContext(site: site, source: source), alignmentSite: clip)
        restartGRPCStream()
        return source
    }

    /// Leaves whatever site was chosen and goes back to showing what the
    /// server streams, with no site.
    func leaveCitySite() { enterSite(SiteContext()) }
}
