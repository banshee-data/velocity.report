// StreetTrafficLayer.swift
// The street painter's traffic layer (docs/plans/lidar-visualiser-lane-painter-plan.md,
// Item 1): which lane field is drawn under the marks and how, reading the
// open run's field from velocity.report and the site's summed field from
// Gerardus, the live fallback from the stream, and Send lane field.
//
// Send lane field uploads the open run's three fields (the whole run and its
// halves) against the accepted record that places them. It follows the
// alignment pane's one-long-call-on-the-wire rule: a second send waits for
// none, it is refused while one is out.

import Combine
import Foundation
import GRPCCore
import os

private let logger = Logger(subsystem: "report.velocity.visualiser", category: "StreetTraffic")

/// velocity.report's lane-field route, abstracted so tests can fake it.
protocol LaneFieldSource: Sendable {
    /// One part of a run's field; nil when the run is not found.
    func laneField(runID: String, part: String) async throws -> VelocityReportLaneField?
}

/// The route over HTTP: GET /api/lidar/runs/{run_id}/lane-field?part=…
struct VelocityReportLaneFieldClient: LaneFieldSource {
    var baseURL: @Sendable () -> URL = { VelocityReportServer.currentHTTPURL }
    var session: URLSession = APISession.shared

    func laneField(runID: String, part: String) async throws -> VelocityReportLaneField? {
        var components = URLComponents(
            url: baseURL().appendingPathComponent("api/lidar/runs/\(runID)/lane-field"),
            resolvingAgainstBaseURL: false)
        components?.queryItems = [URLQueryItem(name: "part", value: part)]
        guard let url = components?.url else { throw URLError(.badURL) }
        let (data, response) = try await session.data(from: url)
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        if status == 404 { return nil }
        guard status == 200 else {
            throw StreetTrafficError.server(
                status, String(decoding: data.prefix(300), as: UTF8.self))
        }
        return try JSONDecoder().decode(VelocityReportLaneField.self, from: data)
    }
}

enum StreetTrafficError: Error, LocalizedError, Equatable {
    case server(Int, String)
    var errorDescription: String? {
        switch self {
        case .server(let status, let body): return "velocity.report answered \(status): \(body)"
        }
    }
}

@available(macOS 15.0, *) @MainActor final class StreetTrafficLayer: ObservableObject {
    enum Choice: String, CaseIterable, Identifiable, Sendable {
        case site = "The site"
        case run = "This recording"
        case live = "Live"
        var id: String { rawValue }
    }

    static let parts = ["all", "first_half", "second_half"]

    @Published var visible = true { didSet { changed() } }
    @Published var choice: Choice = .site { didSet { changed() } }
    @Published var display = LaneFieldDisplay() { didSet { changed() } }
    @Published private(set) var runField: VelocityReportLaneField?
    @Published private(set) var siteField: LaneFieldGrid?
    @Published private(set) var liveGrid: LaneFieldGrid?
    @Published private(set) var isLoading = false
    @Published private(set) var isSending = false
    @Published private(set) var message: String?
    @Published private(set) var errorMessage: String?
    /// The run the run field is for.
    @Published private(set) var runID: String?

    private let alignment: AlignmentSession
    private let source: LaneFieldSource
    private var live = LaneFieldLiveAccumulator()
    private var liveFrames = 0
    /// Called when what is drawn changes.
    var onChange: (() -> Void)?

    init(alignment: AlignmentSession, source: LaneFieldSource = VelocityReportLaneFieldClient()) {
        self.alignment = alignment
        self.source = source
    }

    /// Whether the server takes and serves lane fields (GetServerInfo
    /// supports_lane_fields); without it the site field and Send are hidden.
    var serverSupportsFields: Bool { alignment.serverInfo?.supportsLaneFields == true }

    /// The field drawn now, if any.
    var shown: LaneFieldGrid? {
        guard visible else { return nil }
        switch choice {
        case .site: return siteField
        case .run: return runField?.grid
        case .live: return liveGrid
        }
    }

    // MARK: Reading

    /// Reads the run's field (the whole run) and the site's summed field.
    func load(runID: String?) async {
        isLoading = true
        errorMessage = nil
        self.runID = runID
        if let runID {
            do { runField = try await source.laneField(runID: runID, part: "all") } catch {
                runField = nil
                errorMessage = "This recording's field: \(error.localizedDescription)"
            }
        } else {
            runField = nil
        }
        await loadSite()
        if choice == .site && siteField?.cellCount ?? 0 == 0 && (runField?.cellCount ?? 0) > 0 {
            choice = .run
        }
        if choice != .live && shown?.cellCount ?? 0 == 0 && !live.isEmpty { choice = .live }
        isLoading = false
        changed()
    }

    /// Gerardus's summed field, one call per class group.
    func loadSite() async {
        guard serverSupportsFields, let client = alignment.gerardusClient,
            let token = alignment.site?.l16Token
        else {
            siteField = nil
            return
        }
        var responses: [LaneFieldClassGroup: Gerardus_Align_V1alpha1_GetLaneFieldResponse] = [:]
        for group in LaneFieldClassGroup.allCases {
            do {
                if let r = try await client.getLaneField(
                    l16Token: token, classGroup: group.rawValue)
                {
                    responses[group] = r
                }
            } catch { errorMessage = "The site's field: " + GerardusClientError.describe(error) }
        }
        siteField = responses.isEmpty ? nil : LaneFieldGrid.site(responses)
    }

    // MARK: Live

    /// One frame of the stream, in the sensor frame. Cheap: counted only
    /// while the live layer is chosen or no stored field is there to draw.
    func noteFrame(_ tracks: [Track]) {
        guard visible, choice == .live || (runField == nil && siteField == nil) else { return }
        live.add(tracks)
        liveFrames += 1
        // Rebuilt about twice a second at 10 Hz, not every frame.
        if liveFrames % 5 == 0 {
            liveGrid = live.grid
            if choice == .live { changed() }
        }
    }

    func clearLive() {
        live = LaneFieldLiveAccumulator()
        liveGrid = nil
        liveFrames = 0
        changed()
    }

    // MARK: Send lane field

    /// Why Send lane field cannot run now; nil when it can.
    func sendBlocker(pendingSave: Bool) -> String? {
        if !serverSupportsFields { return "This Gerardus server does not take lane fields." }
        if isSending { return "A field is being sent." }
        if alignment.busy != nil {
            return "Gerardus is busy with another call; wait for it to finish."
        }
        if pendingSave { return "Save or discard the street-use changes first." }
        if runID == nil { return "Open a recorded run: the field is the run's." }
        if runField == nil { return "velocity.report has no field for this run." }
        if alignment.acceptedRecord == nil {
            return
                "Accept a pose for this capture first: the field is placed by the accepted record."
        }
        return nil
    }

    /// Uploads the run's three fields against the accepted record, then
    /// reads the site's field again.
    func send(pendingSave: Bool) async {
        guard sendBlocker(pendingSave: pendingSave) == nil, let runID,
            let client = alignment.gerardusClient, let token = alignment.site?.l16Token,
            let record = alignment.acceptedRecord
        else { return }
        isSending = true
        errorMessage = nil
        message = nil
        var stored = 0
        var repeated = 0
        for part in Self.parts {
            do {
                guard let field = try await source.laneField(runID: runID, part: part) else {
                    errorMessage = "velocity.report has no \(part) field for run \(runID)."
                    break
                }
                if field.cellCount == 0 { continue }
                let header = field.uploadHeader(l16Token: token, recordID: record.recordID)
                let r = try await client.uploadLaneField(
                    header: header, cells: field.uploadChunks())
                if r.alreadyStored { repeated += 1 } else { stored += 1 }
                logger.info("Lane field \(part) of \(runID): \(r.fieldID), \(r.cells) cells")
            } catch {
                errorMessage = "Sending the \(part) field: " + GerardusClientError.describe(error)
                break
            }
        }
        if errorMessage == nil {
            message =
                "Sent: \(stored) field\(stored == 1 ? "" : "s") stored"
                + (repeated > 0 ? ", \(repeated) already there" : "")
        }
        isSending = false
        await loadSite()
        changed()
    }

    private func changed() { onChange?() }
}
