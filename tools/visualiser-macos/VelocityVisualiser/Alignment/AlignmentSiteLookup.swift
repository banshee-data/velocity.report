// AlignmentSiteLookup.swift
// Which S2 site the open capture was taken at, from velocity.report's own
// records. Gerardus keys everything by the site's L16 token, and its hint
// needs the eye-read position a clip carries:
//
//   GET /api/lidar/clip-locations           every located case, grouped by site
//   GET /api/lidar/runs/{run_id}       the run's clip and source file
//   GET /api/lidar/data_source         what the server is replaying now
//   GET /api/lidar/clips/{case_id}    a case's capture files
//
// The heading is not here: no velocity.report API serves site-index.json's
// north_azimuth_deg, so the operator types it into the pane.

import Foundation

/// A located clip: what PoseFromHint and UploadScan need.
struct AlignmentSite: Identifiable, Hashable, Sendable {
    var clipID: String
    var l16Token: String
    var l13Token: String
    /// The case's description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)".
    var description: String
    var sensorID: String
    /// The eye-read position, WGS 84.
    var latitude: Double
    var longitude: Double

    var id: String { clipID }

    /// "Columbus at Broadway": the description without the archive prefix and
    /// the duration, for the pane and for PrepareSite's provenance label.
    var name: String {
        var text = description
        if let colon = text.range(of: ": ") { text = String(text[colon.upperBound...]) }
        if let paren = text.range(of: " (") { text = String(text[..<paren.lowerBound]) }
        return text.isEmpty ? l16Token : text
    }

    /// The token as people read it: "80858-0f47" (geoindex.FamilyDisplay).
    var l16Display: String {
        guard l16Token.count > 5 else { return l16Token }
        return String(l16Token.prefix(5)) + "-" + String(l16Token.dropFirst(5))
    }
}

/// What velocity.report says about the capture that is open.
struct OpenCaptureEvidence: Equatable, Sendable {
    var clipID: String?
    /// The run's source file, then the file the server is replaying now.
    var sourcePaths: [String] = []
    var runID: String?
}

/// How the site was found, for the pane to say.
enum AlignmentSiteMatch: Equatable, Sendable {
    case clip
    case captureFile
    case captureName
    case chosen
    case none

    var explanation: String {
        switch self {
        case .clip: return "The run names this clip."
        case .captureFile: return "The capture is one of this case's files."
        case .captureName: return "The capture's name gives this site's cell."
        case .chosen: return "Chosen by hand."
        case .none: return "The open capture names no located case: choose the site."
        }
    }
}

enum AlignmentSiteResolver {
    /// Finds the site for the open capture, trying the strongest evidence
    /// first. `filesByCase` holds each case's capture paths when they were
    /// fetched; without them the file match is skipped.
    static func resolve(
        _ evidence: OpenCaptureEvidence, sites: [AlignmentSite], filesByCase: [String: [String]]
    ) -> (site: AlignmentSite, match: AlignmentSiteMatch)? {
        if let caseID = evidence.clipID, !caseID.isEmpty,
            let site = sites.first(where: { $0.clipID == caseID })
        {
            return (site, .clip)
        }

        for path in evidence.sourcePaths where !path.isEmpty {
            let name = (path as NSString).lastPathComponent
            if let site = sites.first(where: { site in
                (filesByCase[site.clipID] ?? []).contains { file in
                    path == file || path.hasSuffix("/" + file)
                        || (file as NSString).lastPathComponent == name
                }
            }) {
                return (site, .captureFile)
            }
        }

        for path in evidence.sourcePaths where !path.isEmpty {
            if let site = matchCaptureName(path, sites: sites) { return (site, .captureName) }
        }
        return nil
    }

    /// A VRLOG replay reports its directory as the source and no run, but the
    /// directory is named by the run that recorded it.
    static func runID(fromVRLogPath path: String) -> String? {
        let name = (path as NSString).lastPathComponent
        return UUID(uuidString: name) == nil ? nil : name
    }

    /// The published capture stem is `<start>_<L13 display>_<site id>`, as in
    /// `20260902T1320_80858-0f4_columbus-broadway.pcapng`. The display names
    /// the site's L13 cell; when that cell holds several sites, the site id's
    /// words pick the one whose description names them all.
    static func matchCaptureName(_ path: String, sites: [AlignmentSite]) -> AlignmentSite? {
        let stem = ((path as NSString).lastPathComponent as NSString).deletingPathExtension
        let parts = stem.split(separator: "_", maxSplits: 2).map(String.init)
        guard parts.count == 3, parts[1].contains("-") else { return nil }
        let l13 = parts[1].replacingOccurrences(of: "-", with: "")
        let inCell = sites.filter { $0.l13Token == l13 }
        if inCell.count == 1 { return inCell[0] }
        let words = parts[2].lowercased().split(separator: "-").map(String.init)
        let named = inCell.filter { site in
            let description = site.description.lowercased()
            return words.allSatisfy { description.contains($0) }
        }
        // Several cases at one site share its token, so any of them will do
        // when they agree on it.
        guard let first = named.first, named.allSatisfy({ $0.l16Token == first.l16Token }) else {
            return nil
        }
        return first
    }
}

/// HTTP lookups against velocity.report.
struct AlignmentSiteLookupClient {
    /// A fixed server, for tests; nil follows Settings ▸ Servers at each call.
    let configuredBaseURL: URL?
    var baseURL: URL { configuredBaseURL ?? VelocityReportServer.currentHTTPURL }
    private let session: URLSession

    /// The same server, and the same override, as the other HTTP clients here.
    init(baseURL: URL? = nil, session: URLSession = APISession.shared) {
        self.configuredBaseURL = baseURL
        self.session = session
    }

    private struct ClipLocations: Decodable {
        struct District: Decodable { var sites: [Site]? }
        struct Site: Decodable {
            var s2L16Token: String
            var s2L13Token: String?
            var clips: [Clip]?
        }
        struct Clip: Decodable {
            var clipId: String
            var description: String?
            var sensorId: String?
            var originLat: Double
            var originLon: Double
        }
        var districts: [District]?
    }

    /// Every located clip, ordered by description.
    func locatedSites() async throws -> [AlignmentSite] {
        let map: ClipLocations = try await get("api/lidar/clip-locations")
        var sites = [AlignmentSite]()
        for area in map.districts ?? [] {
            for site in area.sites ?? [] {
                for c in site.clips ?? [] {
                    sites.append(
                        AlignmentSite(
                            clipID: c.clipId, l16Token: site.s2L16Token,
                            l13Token: site.s2L13Token ?? "", description: c.description ?? "",
                            sensorID: c.sensorId ?? "", latitude: c.originLat,
                            longitude: c.originLon))
                }
            }
        }
        return sites.sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }

    private struct Run: Decodable {
        var clipId: String?
        var sourcePath: String?
    }

    private struct DataSource: Decodable {
        var sourcePath: String?
        var pcapFile: String?
        var lastRunId: String?
        var replayActive: Bool?
    }

    /// What the server knows about the open capture: the run the operator
    /// loaded, or else whatever it is replaying now. Missing pieces are left
    /// out rather than reported: the operator can always choose the site.
    func openCaptureEvidence(runID: String?) async -> OpenCaptureEvidence {
        var evidence = OpenCaptureEvidence()
        var runToRead = runID
        if runToRead == nil, let source: DataSource = try? await get("api/lidar/data_source") {
            evidence.sourcePaths += [source.sourcePath, source.pcapFile].compactMap { $0 }
            // A live run is not a capture worth matching.
            if source.replayActive == true {
                runToRead = source.lastRunId
                if runToRead?.isEmpty ?? true, let path = source.sourcePath {
                    runToRead = AlignmentSiteResolver.runID(fromVRLogPath: path)
                }
            }
        }
        if let runToRead, !runToRead.isEmpty,
            let run: Run = try? await get("api/lidar/runs/\(runToRead)")
        {
            evidence.runID = runToRead
            evidence.clipID = run.clipId
            if let path = run.sourcePath { evidence.sourcePaths.insert(path, at: 0) }
        }
        return evidence
    }

    private struct Clip: Decodable {
        struct File: Decodable { var pcapFile: String }
        var files: [File]?
    }

    /// A case's capture files, relative to the server's PCAP root.
    func captureFiles(clipID: String) async throws -> [String] {
        let c: Clip = try await get("api/lidar/clips/\(clipID)")
        return (c.files ?? []).map(\.pcapFile).filter { !$0.isEmpty }
    }

    private func get<T: Decodable>(_ path: String) async throws -> T {
        var request = URLRequest(url: baseURL.appendingPathComponent(path))
        request.timeoutInterval = 5
        let (data, response) = try await session.data(for: request)
        if let http = response as? HTTPURLResponse, !(200..<300).contains(http.statusCode) {
            throw URLError(.badServerResponse)
        }
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(T.self, from: data)
    }
}
