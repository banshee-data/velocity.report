// SiteSwitching.swift
// Moving the visualiser from one site to another (lidar-city-view-plan
// Item 5), with calls velocity.report already serves:
//
//   GET  /api/lidar/clip-locations     what the connected server can replay
//   GET  /api/lidar/clips/{clip_id}    a clip's capture files
//   POST /api/lidar/replay/stop        stop whatever replay is running
//   GET  /api/lidar/data_source        whether one still is
//   POST /api/lidar/pcap/start         replay a site's captures
//
// The server replays one thing at a time and refuses a start while a
// replay runs, so a switch stops the running one, waits until the server
// says nothing is replaying, and only then starts the site's.
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
    /// Starts a replay of `files`, in order, as one recording. Throws
    /// `SiteSwitchError.replayBusy` when another replay is running.
    func startReplay(sensorID: String, files: [String]) async throws
    /// Stops whatever replay is running, returning the sensor to live.
    func stopReplay(sensorID: String) async throws
    /// Whether the server is replaying anything now.
    func isReplaying() async throws -> Bool
}

extension SiteReplayService {
    /// Stops the running replay, waits until the server reports none, then
    /// starts `files`. A start refused because a replay is running again
    /// (another client started one in between) goes round once more.
    func replaceReplay(
        sensorID: String, files: [String], attempts: Int = 3, timeout: Duration = .seconds(10),
        poll: Duration = .milliseconds(200)
    ) async throws {
        for attempt in 1...max(attempts, 1) {
            try await stopReplay(sensorID: sensorID)
            let clock = ContinuousClock()
            let deadline = clock.now.advanced(by: timeout)
            while try await isReplaying() {
                guard clock.now < deadline else { throw SiteSwitchError.replayDidNotStop }
                try await Task.sleep(for: poll)
            }
            do {
                return try await startReplay(sensorID: sensorID, files: files)
            } catch SiteSwitchError.replayBusy where attempt < attempts { continue }
        }
    }
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
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        if status == 409 { throw SiteSwitchError.replayBusy }
        guard (200..<300).contains(status) else { throw Self.refusal(data) }
    }

    func stopReplay(sensorID: String) async throws {
        var components = URLComponents(
            url: lookup.baseURL.appendingPathComponent("api/lidar/replay/stop"),
            resolvingAgainstBaseURL: false)
        components?.queryItems = [URLQueryItem(name: "sensor_id", value: sensorID)]
        guard let url = components?.url else { throw URLError(.badURL) }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.timeoutInterval = 30
        let (data, response) = try await session.data(for: request)
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        guard (200..<300).contains(status) else { throw Self.refusal(data) }
    }

    func isReplaying() async throws -> Bool {
        var request = URLRequest(
            url: lookup.baseURL.appendingPathComponent("api/lidar/data_source"))
        request.timeoutInterval = 10
        let (data, response) = try await session.data(for: request)
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        guard (200..<300).contains(status) else { throw Self.refusal(data) }
        return Self.replaying(data)
    }

    /// Whether a `data_source` answer says a replay is running.
    static func replaying(_ data: Data) -> Bool {
        let json = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] ?? [:]
        return json["pcap_in_progress"] as? Bool == true || json["replay_active"] as? Bool == true
    }

    /// The server's own reason, its JSON `error` when it gives one.
    static func refusal(_ data: Data) -> SiteSwitchError {
        let json = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any]
        let text = json?["error"] as? String ?? String(data: data, encoding: .utf8) ?? ""
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        return .replayRefused(trimmed.isEmpty ? "no reason given" : trimmed)
    }
}

enum SiteSwitchError: Error, LocalizedError, Equatable {
    case replayRefused(String)
    case nothingToPlay(String)
    /// The server is replaying something else and refused a start.
    case replayBusy
    /// The running replay was asked to stop and was still running.
    case replayDidNotStop

    var errorDescription: String? {
        switch self {
        case .replayRefused(let why): return "The server would not replay the site: \(why)"
        case .nothingToPlay(let title): return "The connected server holds no capture of \(title)."
        case .replayBusy:
            return "The server is still replaying something else after it was asked to stop."
        case .replayDidNotStop: return "The running replay did not stop within 10 seconds."
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
        // The running replay is stopped and the site's started before the
        // site is entered: a refused start leaves the current site shown.
        try await service.replaceReplay(sensorID: clip.sensorID, files: replay)
        enterSite(SiteContext(site: site, source: source), alignmentSite: clip)
        restartGRPCStream()
        return source
    }

    /// Leaves whatever site was chosen and goes back to showing what the
    /// server streams, with no site.
    func leaveCitySite() { enterSite(SiteContext()) }
}
